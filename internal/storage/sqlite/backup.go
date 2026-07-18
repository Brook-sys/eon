package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"motor-autonomo/internal/storage/memory"

	"modernc.org/sqlite"
)

// BackupReport is the durable result of an online SQLite backup (ADR-0003).
// The destination is a standalone SQLite database file that can be reopened
// with Open after the source store remains online.
type BackupReport struct {
	SourcePath       string        `json:"source_path,omitempty"`
	DestinationPath  string        `json:"destination_path"`
	PagesCopied      int64         `json:"pages_copied"`
	Duration         time.Duration `json:"duration"`
	SQLiteVersion    string        `json:"sqlite_version"`
	CheckpointRows   int           `json:"checkpoint_rows"`
	CheckpointFormat int           `json:"checkpoint_format,omitempty"`
	IntegrityCheck   string        `json:"integrity_check"`
}

// BackupVerification records both SQLite page-level verification and the
// runtime checkpoint's framing, digest and decodability. Empty stores are
// valid and therefore report zero checkpoint rows and format.
type BackupVerification struct {
	CheckpointRows   int    `json:"checkpoint_rows"`
	CheckpointFormat int    `json:"checkpoint_format,omitempty"`
	IntegrityCheck   string `json:"integrity_check"`
}

// BackupOptions configures online backup behavior.
type BackupOptions struct {
	// PageSteps is the number of pages copied per sqlite3_backup_step call.
	// Zero or negative means copy all remaining pages in one step.
	PageSteps int32
}

// BackupTo creates a consistent online copy of the store database at destPath
// using modernc.org/sqlite's NewBackup API (sqlite3_backup_*). The source store
// remains open. Copying only the main file while writers are active is not a
// valid procedure; this method is the supported alternative.
//
// destPath must not already exist. Parent directories are created when missing.
func (s *Store) BackupTo(ctx context.Context, destPath string, options BackupOptions) (BackupReport, error) {
	if s == nil {
		return BackupReport{}, errors.New("sqlite store is nil")
	}
	if err := ctx.Err(); err != nil {
		return BackupReport{}, err
	}
	destPath = filepath.Clean(destPath)
	if destPath == "" || destPath == "." {
		return BackupReport{}, errors.New("backup destination path is required")
	}
	if _, err := os.Stat(destPath); err == nil {
		return BackupReport{}, fmt.Errorf("backup destination already exists: %s", destPath)
	} else if !errors.Is(err, os.ErrNotExist) {
		return BackupReport{}, fmt.Errorf("stat backup destination: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return BackupReport{}, fmt.Errorf("create backup parent directory: %w", err)
	}

	// Hold the write lock so domain Updates cannot interleave with the page
	// copy of the checkpoint blob. Views remain blocked only for the short
	// duration of the backup; MaxOpenConns is already 1.
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		return BackupReport{}, errors.New("sqlite store is closed")
	}

	started := time.Now()
	var version string
	if err := s.db.QueryRowContext(ctx, `SELECT sqlite_version()`).Scan(&version); err != nil {
		return BackupReport{}, fmt.Errorf("query sqlite version for backup: %w", err)
	}

	conn, err := s.db.Conn(ctx)
	if err != nil {
		return BackupReport{}, fmt.Errorf("acquire sqlite connection for backup: %w", err)
	}
	defer conn.Close()

	var pages int64
	err = conn.Raw(func(driverConn any) error {
		backuper, ok := driverConn.(interface {
			NewBackup(string) (*sqlite.Backup, error)
		})
		if !ok {
			return fmt.Errorf("sqlite driver connection does not support NewBackup: %T", driverConn)
		}
		backup, err := backuper.NewBackup(destPath)
		if err != nil {
			return fmt.Errorf("start sqlite online backup: %w", err)
		}
		defer backup.Finish()

		steps := options.PageSteps
		if steps == 0 {
			steps = -1
		}
		for {
			if err := ctx.Err(); err != nil {
				return err
			}
			more, err := backup.Step(steps)
			if err != nil {
				return fmt.Errorf("sqlite backup step: %w", err)
			}
			// Step(-1) copies all remaining pages; when steps > 0 we approximate
			// progress by counting successful calls until done.
			if steps < 0 {
				pages = 1
			} else {
				pages++
			}
			if !more {
				break
			}
		}
		return nil
	})
	if err != nil {
		_ = os.Remove(destPath)
		return BackupReport{}, err
	}

	// Verify destination has the checkpoint row and can load.
	verification, err := VerifyBackup(destPath)
	if err != nil {
		_ = os.Remove(destPath)
		return BackupReport{}, err
	}

	return BackupReport{
		DestinationPath:  destPath,
		PagesCopied:      pages,
		Duration:         time.Since(started),
		SQLiteVersion:    version,
		CheckpointRows:   verification.CheckpointRows,
		CheckpointFormat: verification.CheckpointFormat,
		IntegrityCheck:   verification.IntegrityCheck,
	}, nil
}

// ClosedCopyTo copies a store that has already been Closed by reopening the
// source path for the duration of the backup. Prefer BackupTo for online
// stores. This helper exists for offline runbook paths.
func ClosedCopyTo(ctx context.Context, sourcePath, destPath string, options BackupOptions) (BackupReport, error) {
	store, err := Open(sourcePath)
	if err != nil {
		return BackupReport{}, err
	}
	defer store.Close()
	report, err := store.BackupTo(ctx, destPath, options)
	if err != nil {
		return BackupReport{}, err
	}
	report.SourcePath = sourcePath
	return report, nil
}

// RestoreTo verifies an existing standalone backup before creating a new
// canonical runtime database at destPath. Both paths must be offline: callers
// must stop all writers before restore. The destination must not exist, which
// keeps replacement of an active or valuable database an explicit operator
// action rather than an accidental overwrite.
func RestoreTo(ctx context.Context, backupPath, destPath string, options BackupOptions) (BackupReport, error) {
	if err := ctx.Err(); err != nil {
		return BackupReport{}, err
	}
	if _, err := VerifyBackup(backupPath); err != nil {
		return BackupReport{}, fmt.Errorf("verify restore source: %w", err)
	}
	report, err := ClosedCopyTo(ctx, backupPath, destPath, options)
	if err != nil {
		return BackupReport{}, fmt.Errorf("restore verified backup: %w", err)
	}
	return report, nil
}

// VerifyBackup opens a SQLite backup and verifies the database pages plus the
// runtime checkpoint's external version, integrity digest and complete decode.
// It performs no migration or write.
func VerifyBackup(path string) (BackupVerification, error) {
	if _, err := os.Stat(path); err != nil {
		return BackupVerification{}, fmt.Errorf("stat backup for verification: %w", err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return BackupVerification{}, fmt.Errorf("open backup for verification: %w", err)
	}
	defer db.Close()
	if _, err := db.Exec(`PRAGMA query_only=ON`); err != nil {
		return BackupVerification{}, fmt.Errorf("set backup verification read-only: %w", err)
	}
	var integrity string
	if err := db.QueryRow(`PRAGMA quick_check`).Scan(&integrity); err != nil {
		return BackupVerification{}, fmt.Errorf("verify backup sqlite pages: %w", err)
	}
	if integrity != "ok" {
		return BackupVerification{}, fmt.Errorf("verify backup sqlite pages: quick_check=%q", integrity)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM runtime_checkpoint WHERE id = 1`).Scan(&count); err != nil {
		return BackupVerification{}, fmt.Errorf("verify backup checkpoint: %w", err)
	}
	if count != 1 {
		// Empty store still has no checkpoint row until first Update; that is
		// legal. Count 0 is accepted for fresh files.
		if count == 0 {
			return BackupVerification{IntegrityCheck: integrity}, nil
		}
		return BackupVerification{}, fmt.Errorf("verify backup checkpoint: got %d rows, want at most one", count)
	}
	var formatVersion int
	var payload []byte
	if err := db.QueryRow(`SELECT format_version, payload FROM runtime_checkpoint WHERE id = 1`).Scan(&formatVersion, &payload); err != nil {
		return BackupVerification{}, fmt.Errorf("read backup checkpoint payload: %w", err)
	}
	if len(payload) == 0 {
		return BackupVerification{}, errors.New("backup checkpoint payload is empty")
	}
	if err := memory.ValidateExternalCheckpoint(formatVersion, payload); err != nil {
		return BackupVerification{}, fmt.Errorf("validate backup checkpoint: %w", err)
	}
	if _, err := memory.NewFromBinary(payload); err != nil {
		return BackupVerification{}, fmt.Errorf("decode backup checkpoint: %w", err)
	}
	return BackupVerification{
		CheckpointRows:   count,
		CheckpointFormat: formatVersion,
		IntegrityCheck:   integrity,
	}, nil
}
