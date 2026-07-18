package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"motor-autonomo/internal/storage/memory"

	"modernc.org/sqlite"
)

// BackupReport is the durable result of an online SQLite backup (ADR-0003).
// The destination is a standalone SQLite database file that can be reopened
// with Open after the source store remains online.
type BackupReport struct {
	ReportSchema     string        `json:"report_schema"`
	Operation        string        `json:"operation"`
	SourcePath       string        `json:"source_path,omitempty"`
	SourceSHA256     string        `json:"source_sha256,omitempty"`
	DestinationPath  string        `json:"destination_path"`
	PagesCopied      int64         `json:"pages_copied"`
	Duration         time.Duration `json:"duration"`
	SQLiteVersion    string        `json:"sqlite_version"`
	FileSize         int64         `json:"file_size"`
	PageSize         int           `json:"page_size"`
	PageCount        int           `json:"page_count"`
	SHA256           string        `json:"sha256"`
	ApplicationID    int           `json:"application_id"`
	UserVersion      int           `json:"user_version"`
	SchemaVersion    int           `json:"schema_version"`
	SchemaObjects    int           `json:"schema_objects"`
	SchemaSHA256     string        `json:"schema_sha256"`
	CheckpointRows   int           `json:"checkpoint_rows"`
	CheckpointFormat int           `json:"checkpoint_format,omitempty"`
	CheckpointSHA256 string        `json:"checkpoint_sha256,omitempty"`
	IntegrityCheck   string        `json:"integrity_check"`
	ForeignKeyCheck  string        `json:"foreign_key_check"`
}

// BackupVerification records both SQLite page-level verification and the
// runtime checkpoint's framing, digest and decodability. Empty stores are
// valid and therefore report zero checkpoint rows and format.
type BackupVerification struct {
	ReportSchema     string `json:"report_schema"`
	Operation        string `json:"operation"`
	FileSize         int64  `json:"file_size"`
	PageSize         int    `json:"page_size"`
	PageCount        int    `json:"page_count"`
	SHA256           string `json:"sha256"`
	ApplicationID    int    `json:"application_id"`
	UserVersion      int    `json:"user_version"`
	SchemaVersion    int    `json:"schema_version"`
	SchemaObjects    int    `json:"schema_objects"`
	SchemaSHA256     string `json:"schema_sha256"`
	CheckpointRows   int    `json:"checkpoint_rows"`
	CheckpointFormat int    `json:"checkpoint_format,omitempty"`
	CheckpointSHA256 string `json:"checkpoint_sha256,omitempty"`
	IntegrityCheck   string `json:"integrity_check"`
	ForeignKeyCheck  string `json:"foreign_key_check"`
}

// VerificationOptions optionally pins the digest recorded when the backup was
// created or transferred. Empty means compute and report without comparison.
type VerificationOptions struct {
	ExpectedSHA256           string
	ExpectedPageSize         *int
	ExpectedPageCount        *int
	ExpectedSchemaVersion    *int
	ExpectedSchemaObjects    *int
	ExpectedSchemaSHA256     string
	ExpectedCheckpointSHA256 string
	// ExpectedCheckpointRows and ExpectedCheckpointFormat pin the logical
	// framing recorded in an inventory. Nil means report without comparison.
	// A zero row count is meaningful for a valid empty store.
	ExpectedCheckpointRows   *int
	ExpectedCheckpointFormat *int
}

// BackupOptions configures online backup behavior.
type BackupOptions struct {
	// PageSteps is the number of pages copied per sqlite3_backup_step call.
	// Zero or negative means copy all remaining pages in one step.
	PageSteps int32
}

// RestoreOptions pins the identity of the backup selected by the operator and
// configures the page copy. Even without an explicit digest, RestoreTo records
// the verified source digest and rejects source changes during the restore.
type RestoreOptions struct {
	ExpectedSHA256           string
	ExpectedPageSize         *int
	ExpectedPageCount        *int
	ExpectedSchemaVersion    *int
	ExpectedSchemaObjects    *int
	ExpectedSchemaSHA256     string
	ExpectedCheckpointSHA256 string
	ExpectedCheckpointRows   *int
	ExpectedCheckpointFormat *int
	Backup                   BackupOptions
}

const (
	backupReportSchema    = "motor-autonomo.sqlite-backup-report.v1"
	backupOperation       = "backup"
	restoreOperation      = "restore"
	verificationOperation = "verify"
)

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
	tempFile, err := os.CreateTemp(filepath.Dir(destPath), ".sqlite-backup-*")
	if err != nil {
		return BackupReport{}, fmt.Errorf("reserve backup temporary path: %w", err)
	}
	tempPath := tempFile.Name()
	if err := tempFile.Close(); err != nil {
		_ = os.Remove(tempPath)
		return BackupReport{}, fmt.Errorf("close backup temporary file: %w", err)
	}
	if err := os.Remove(tempPath); err != nil {
		return BackupReport{}, fmt.Errorf("prepare backup temporary path: %w", err)
	}
	published := false
	defer func() {
		if !published {
			_ = os.Remove(tempPath)
		}
	}()

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

	err = conn.Raw(func(driverConn any) error {
		backuper, ok := driverConn.(interface {
			NewBackup(string) (*sqlite.Backup, error)
		})
		if !ok {
			return fmt.Errorf("sqlite driver connection does not support NewBackup: %T", driverConn)
		}
		backup, err := backuper.NewBackup(tempPath)
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
			if !more {
				break
			}
		}
		return nil
	})
	if err != nil {
		return BackupReport{}, err
	}
	if err := os.Chmod(tempPath, 0o600); err != nil {
		return BackupReport{}, fmt.Errorf("restrict backup permissions: %w", err)
	}

	// Verify destination has the checkpoint row and can load.
	verification, err := VerifyBackup(tempPath)
	if err != nil {
		return BackupReport{}, err
	}
	if err := syncRegularFile(tempPath); err != nil {
		return BackupReport{}, fmt.Errorf("sync verified backup: %w", err)
	}
	if err := publishBackupNoReplace(tempPath, destPath); err != nil {
		return BackupReport{}, err
	}
	published = true

	return BackupReport{
		ReportSchema:     backupReportSchema,
		Operation:        backupOperation,
		DestinationPath:  destPath,
		PagesCopied:      int64(verification.PageCount),
		Duration:         time.Since(started),
		SQLiteVersion:    version,
		FileSize:         verification.FileSize,
		PageSize:         verification.PageSize,
		PageCount:        verification.PageCount,
		SHA256:           verification.SHA256,
		ApplicationID:    verification.ApplicationID,
		UserVersion:      verification.UserVersion,
		SchemaVersion:    verification.SchemaVersion,
		SchemaObjects:    verification.SchemaObjects,
		SchemaSHA256:     verification.SchemaSHA256,
		CheckpointRows:   verification.CheckpointRows,
		CheckpointFormat: verification.CheckpointFormat,
		CheckpointSHA256: verification.CheckpointSHA256,
		IntegrityCheck:   verification.IntegrityCheck,
		ForeignKeyCheck:  verification.ForeignKeyCheck,
	}, nil
}

// publishBackupNoReplace makes a verified temporary backup visible without a
// check-then-create race. Link is atomic and fails if another process created
// destPath after the initial validation; removing the temporary name leaves
// the published inode and its restrictive permissions intact.
func publishBackupNoReplace(tempPath, destPath string) error {
	if err := os.Link(tempPath, destPath); err != nil {
		if _, statErr := os.Lstat(destPath); statErr == nil {
			return fmt.Errorf("backup destination already exists: %s", destPath)
		}
		return fmt.Errorf("publish backup without overwrite: %w", err)
	}
	if err := syncDirectory(filepath.Dir(destPath)); err != nil {
		_ = os.Remove(destPath)
		return fmt.Errorf("sync published backup directory: %w", err)
	}
	if err := os.Remove(tempPath); err != nil {
		_ = os.Remove(destPath)
		return fmt.Errorf("remove published backup temporary name: %w", err)
	}
	if err := syncDirectory(filepath.Dir(destPath)); err != nil {
		return fmt.Errorf("sync backup temporary-name removal: %w", err)
	}
	return nil
}

func syncRegularFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return file.Sync()
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

// ClosedCopyTo copies a store that has already been Closed by reopening the
// source path for the duration of the backup. Prefer BackupTo for online
// stores. This helper exists for offline runbook paths.
func ClosedCopyTo(ctx context.Context, sourcePath, destPath string, options BackupOptions) (BackupReport, error) {
	if err := rejectSQLiteSidecars(sourcePath); err != nil {
		return BackupReport{}, fmt.Errorf("inspect offline backup source: %w", err)
	}
	beforeSize, beforeDigest, beforeIdentity, err := hashBackupFile(sourcePath)
	if err != nil {
		return BackupReport{}, fmt.Errorf("inspect offline backup source: %w", err)
	}
	store, err := openReadOnlyStore(sourcePath, beforeIdentity)
	if err != nil {
		return BackupReport{}, err
	}
	defer store.Close()
	report, err := store.BackupTo(ctx, destPath, options)
	if err != nil {
		return BackupReport{}, err
	}
	if err := rejectSQLiteSidecars(sourcePath); err != nil {
		_ = os.Remove(destPath)
		return BackupReport{}, fmt.Errorf("reinspect offline backup source: %w", err)
	}
	afterSize, afterDigest, afterIdentity, err := hashBackupFile(sourcePath)
	if err != nil {
		_ = os.Remove(destPath)
		return BackupReport{}, fmt.Errorf("reinspect offline backup source: %w", err)
	}
	if !os.SameFile(beforeIdentity, afterIdentity) || beforeSize != afterSize || beforeDigest != afterDigest {
		_ = os.Remove(destPath)
		return BackupReport{}, errors.New("offline backup source changed during copy")
	}
	report.SourcePath = sourcePath
	report.SourceSHA256 = beforeDigest
	return report, nil
}

// openReadOnlyStore loads an offline source without running the normal store
// configuration path. In particular it must not create tables, switch journal
// mode, migrate the checkpoint, or leave WAL/SHM sidecars next to a backup
// artifact selected for restore.
func openReadOnlyStore(path string, expectedIdentity os.FileInfo) (*Store, error) {
	db, err := openReadOnlyDatabase(path, expectedIdentity)
	if err != nil {
		return nil, fmt.Errorf("open offline backup source read-only: %w", err)
	}
	core, err := load(db)
	if err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db, core: core}, nil
}

// openReadOnlyDatabase binds SQLite to the already-inspected regular inode and
// disables creation, journaling and migration. It is shared by offline copy
// and verification so merely auditing a backup cannot mutate the artifact or
// leave WAL/SHM sidecars next to it.
func openReadOnlyDatabase(path string, expectedIdentity os.FileInfo) (*sql.DB, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve backup source: %w", err)
	}
	dsn := (&url.URL{
		Scheme:   "file",
		Path:     filepath.ToSlash(absPath),
		RawQuery: "immutable=1&mode=ro",
	}).String()
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	if err := requireSameRegularPath(path, expectedIdentity); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// RestoreTo verifies an existing standalone backup before creating a new
// canonical runtime database at destPath. Both paths must be offline: callers
// must stop all writers before restore. The destination must not exist, which
// keeps replacement of an active or valuable database an explicit operator
// action rather than an accidental overwrite.
func RestoreTo(ctx context.Context, backupPath, destPath string, options BackupOptions) (BackupReport, error) {
	return RestoreToWithOptions(ctx, backupPath, destPath, RestoreOptions{Backup: options})
}

// RestoreToWithOptions verifies and pins the source backup before copying it.
// The source is verified again after the copy; if it changed, the destination
// is removed so an offline-restore contract violation cannot be promoted.
func RestoreToWithOptions(ctx context.Context, backupPath, destPath string, options RestoreOptions) (BackupReport, error) {
	if err := ctx.Err(); err != nil {
		return BackupReport{}, err
	}
	sourceVerification, err := VerifyBackupWithOptions(backupPath, VerificationOptions{
		ExpectedSHA256:           options.ExpectedSHA256,
		ExpectedPageSize:         options.ExpectedPageSize,
		ExpectedPageCount:        options.ExpectedPageCount,
		ExpectedSchemaVersion:    options.ExpectedSchemaVersion,
		ExpectedSchemaObjects:    options.ExpectedSchemaObjects,
		ExpectedSchemaSHA256:     options.ExpectedSchemaSHA256,
		ExpectedCheckpointSHA256: options.ExpectedCheckpointSHA256,
		ExpectedCheckpointRows:   options.ExpectedCheckpointRows,
		ExpectedCheckpointFormat: options.ExpectedCheckpointFormat,
	})
	if err != nil {
		return BackupReport{}, fmt.Errorf("verify restore source: %w", err)
	}
	report, err := ClosedCopyTo(ctx, backupPath, destPath, options.Backup)
	if err != nil {
		return BackupReport{}, fmt.Errorf("restore verified backup: %w", err)
	}
	if report.CheckpointRows != sourceVerification.CheckpointRows ||
		report.PageSize != sourceVerification.PageSize ||
		report.PageCount != sourceVerification.PageCount ||
		report.ApplicationID != sourceVerification.ApplicationID ||
		report.UserVersion != sourceVerification.UserVersion ||
		report.SchemaVersion != sourceVerification.SchemaVersion ||
		report.SchemaObjects != sourceVerification.SchemaObjects ||
		report.SchemaSHA256 != sourceVerification.SchemaSHA256 ||
		report.CheckpointFormat != sourceVerification.CheckpointFormat ||
		report.CheckpointSHA256 != sourceVerification.CheckpointSHA256 {
		_ = os.Remove(destPath)
		return BackupReport{}, errors.New("restore destination checkpoint identity differs from verified source")
	}
	if _, err := VerifyBackupWithOptions(backupPath, VerificationOptions{ExpectedSHA256: sourceVerification.SHA256}); err != nil {
		_ = os.Remove(destPath)
		return BackupReport{}, fmt.Errorf("reverify restore source: %w", err)
	}
	report.SourceSHA256 = sourceVerification.SHA256
	report.Operation = restoreOperation
	return report, nil
}

// VerifyBackup opens a SQLite backup and verifies the database pages plus the
// runtime checkpoint's external version, integrity digest and complete decode.
// It performs no migration or write.
func VerifyBackup(path string) (BackupVerification, error) {
	return VerifyBackupWithOptions(path, VerificationOptions{})
}

// VerifyBackupWithOptions additionally compares the file digest against an
// expected SHA-256. It hashes before and after the SQLite/checkpoint audit so a
// concurrently changing artifact is rejected rather than certified.
func VerifyBackupWithOptions(path string, options VerificationOptions) (BackupVerification, error) {
	expected, err := normalizeExpectedSHA256(options.ExpectedSHA256)
	if err != nil {
		return BackupVerification{}, err
	}
	expectedCheckpoint, err := normalizeExpectedSHA256(options.ExpectedCheckpointSHA256)
	if err != nil {
		return BackupVerification{}, fmt.Errorf("expected checkpoint SHA-256: %w", err)
	}
	expectedSchema, err := normalizeExpectedSHA256(options.ExpectedSchemaSHA256)
	if err != nil {
		return BackupVerification{}, fmt.Errorf("expected schema SHA-256: %w", err)
	}
	if err := rejectSQLiteSidecars(path); err != nil {
		return BackupVerification{}, err
	}
	beforeSize, beforeDigest, beforeIdentity, err := hashBackupFile(path)
	if err != nil {
		return BackupVerification{}, err
	}
	if expected != "" && beforeDigest != expected {
		return BackupVerification{}, fmt.Errorf("verify backup digest: got %s, want %s", beforeDigest, expected)
	}

	db, err := openReadOnlyDatabase(path, beforeIdentity)
	if err != nil {
		return BackupVerification{}, fmt.Errorf("open backup for verification read-only: %w", err)
	}
	var integrity string
	if err := db.QueryRow(`PRAGMA integrity_check(1)`).Scan(&integrity); err != nil {
		db.Close()
		return BackupVerification{}, fmt.Errorf("verify backup sqlite pages: %w", err)
	}
	if integrity != "ok" {
		db.Close()
		return BackupVerification{}, fmt.Errorf("verify backup sqlite pages and constraints: integrity_check=%q", integrity)
	}
	foreignKeyRows, err := db.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		db.Close()
		return BackupVerification{}, fmt.Errorf("verify backup foreign keys: %w", err)
	}
	if foreignKeyRows.Next() {
		foreignKeyRows.Close()
		db.Close()
		return BackupVerification{}, errors.New("verify backup foreign keys: violation found")
	}
	if err := foreignKeyRows.Err(); err != nil {
		foreignKeyRows.Close()
		db.Close()
		return BackupVerification{}, fmt.Errorf("verify backup foreign keys: %w", err)
	}
	if err := foreignKeyRows.Close(); err != nil {
		db.Close()
		return BackupVerification{}, fmt.Errorf("close backup foreign-key check: %w", err)
	}
	var applicationID, userVersion int
	var pageSize, pageCount int
	if err := db.QueryRow(`PRAGMA page_size`).Scan(&pageSize); err != nil {
		db.Close()
		return BackupVerification{}, fmt.Errorf("read backup page size: %w", err)
	}
	if pageSize <= 0 {
		db.Close()
		return BackupVerification{}, fmt.Errorf("verify backup page size: got %d, want positive", pageSize)
	}
	if err := db.QueryRow(`PRAGMA page_count`).Scan(&pageCount); err != nil {
		db.Close()
		return BackupVerification{}, fmt.Errorf("read backup page count: %w", err)
	}
	if pageCount <= 0 {
		db.Close()
		return BackupVerification{}, fmt.Errorf("verify backup page count: got %d, want positive", pageCount)
	}
	if int64(pageSize)*int64(pageCount) != beforeSize {
		db.Close()
		return BackupVerification{}, fmt.Errorf("verify backup page inventory: page_size=%d page_count=%d imply %d bytes, file has %d", pageSize, pageCount, int64(pageSize)*int64(pageCount), beforeSize)
	}
	if err := db.QueryRow(`PRAGMA application_id`).Scan(&applicationID); err != nil {
		db.Close()
		return BackupVerification{}, fmt.Errorf("read backup application id: %w", err)
	}
	if applicationID != runtimeApplicationID {
		db.Close()
		return BackupVerification{}, fmt.Errorf("verify backup application id: got %d, want %d", applicationID, runtimeApplicationID)
	}
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&userVersion); err != nil {
		db.Close()
		return BackupVerification{}, fmt.Errorf("read backup user version: %w", err)
	}
	if userVersion != runtimeUserVersion {
		db.Close()
		return BackupVerification{}, fmt.Errorf("verify backup user version: got %d, want %d", userVersion, runtimeUserVersion)
	}
	var schemaVersion int
	if err := db.QueryRow(`PRAGMA schema_version`).Scan(&schemaVersion); err != nil {
		db.Close()
		return BackupVerification{}, fmt.Errorf("read backup schema version: %w", err)
	}
	var schemaObjects int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_schema WHERE name NOT LIKE 'sqlite_%'`).Scan(&schemaObjects); err != nil {
		db.Close()
		return BackupVerification{}, fmt.Errorf("count backup runtime schema objects: %w", err)
	}
	if schemaObjects != 1 {
		db.Close()
		return BackupVerification{}, fmt.Errorf("verify backup runtime schema: got %d user objects, want exactly runtime_checkpoint", schemaObjects)
	}
	var schemaType, schemaName, tableName, schemaSQL string
	if err := db.QueryRow(`SELECT type, name, tbl_name, sql FROM sqlite_schema WHERE name NOT LIKE 'sqlite_%'`).Scan(&schemaType, &schemaName, &tableName, &schemaSQL); err != nil {
		db.Close()
		return BackupVerification{}, fmt.Errorf("read backup runtime schema: %w", err)
	}
	if schemaType != "table" || schemaName != "runtime_checkpoint" || tableName != "runtime_checkpoint" || strings.TrimSpace(schemaSQL) == "" {
		db.Close()
		return BackupVerification{}, fmt.Errorf("verify backup runtime schema: unexpected object type=%q name=%q table=%q", schemaType, schemaName, tableName)
	}
	schemaDigest := sha256.Sum256([]byte(schemaSQL))
	var count, canonicalCount int
	if err := db.QueryRow(`SELECT COUNT(*), COUNT(*) FILTER (WHERE id = 1) FROM runtime_checkpoint`).Scan(&count, &canonicalCount); err != nil {
		db.Close()
		return BackupVerification{}, fmt.Errorf("verify backup checkpoint: %w", err)
	}
	if count != canonicalCount {
		db.Close()
		return BackupVerification{}, fmt.Errorf("verify backup checkpoint: got %d total rows but %d canonical id=1 rows", count, canonicalCount)
	}
	verification := BackupVerification{
		ReportSchema:    backupReportSchema,
		Operation:       verificationOperation,
		FileSize:        beforeSize,
		PageSize:        pageSize,
		PageCount:       pageCount,
		SHA256:          beforeDigest,
		ApplicationID:   applicationID,
		UserVersion:     userVersion,
		SchemaVersion:   schemaVersion,
		SchemaObjects:   schemaObjects,
		SchemaSHA256:    hex.EncodeToString(schemaDigest[:]),
		CheckpointRows:  count,
		IntegrityCheck:  integrity,
		ForeignKeyCheck: "ok",
	}
	if options.ExpectedPageSize != nil {
		if *options.ExpectedPageSize <= 0 {
			db.Close()
			return BackupVerification{}, errors.New("expected page size must be positive")
		}
		if verification.PageSize != *options.ExpectedPageSize {
			db.Close()
			return BackupVerification{}, fmt.Errorf("verify backup page size: got %d, want %d", verification.PageSize, *options.ExpectedPageSize)
		}
	}
	if options.ExpectedPageCount != nil {
		if *options.ExpectedPageCount <= 0 {
			db.Close()
			return BackupVerification{}, errors.New("expected page count must be positive")
		}
		if verification.PageCount != *options.ExpectedPageCount {
			db.Close()
			return BackupVerification{}, fmt.Errorf("verify backup page count: got %d, want %d", verification.PageCount, *options.ExpectedPageCount)
		}
	}
	if options.ExpectedSchemaVersion != nil {
		if *options.ExpectedSchemaVersion < 0 {
			db.Close()
			return BackupVerification{}, errors.New("expected schema version must not be negative")
		}
		if verification.SchemaVersion != *options.ExpectedSchemaVersion {
			db.Close()
			return BackupVerification{}, fmt.Errorf("verify backup schema version: got %d, want %d", verification.SchemaVersion, *options.ExpectedSchemaVersion)
		}
	}
	if options.ExpectedSchemaObjects != nil {
		if *options.ExpectedSchemaObjects != 1 {
			db.Close()
			return BackupVerification{}, errors.New("expected schema objects must be 1 for the canonical runtime schema")
		}
		if verification.SchemaObjects != *options.ExpectedSchemaObjects {
			db.Close()
			return BackupVerification{}, fmt.Errorf("verify backup schema objects: got %d, want %d", verification.SchemaObjects, *options.ExpectedSchemaObjects)
		}
	}
	if expectedSchema != "" && verification.SchemaSHA256 != expectedSchema {
		db.Close()
		return BackupVerification{}, fmt.Errorf("verify backup schema digest: got %s, want %s", verification.SchemaSHA256, expectedSchema)
	}
	if count != 1 {
		// Empty store still has no checkpoint row until first Update; that is
		// legal. Count 0 is accepted for fresh files.
		if count != 0 {
			db.Close()
			return BackupVerification{}, fmt.Errorf("verify backup checkpoint: got %d rows, want at most one", count)
		}
	} else {
		var formatVersion int
		var payload []byte
		if err := db.QueryRow(`SELECT format_version, payload FROM runtime_checkpoint WHERE id = 1`).Scan(&formatVersion, &payload); err != nil {
			db.Close()
			return BackupVerification{}, fmt.Errorf("read backup checkpoint payload: %w", err)
		}
		if len(payload) == 0 {
			db.Close()
			return BackupVerification{}, errors.New("backup checkpoint payload is empty")
		}
		if err := memory.ValidateExternalCheckpoint(formatVersion, payload); err != nil {
			db.Close()
			return BackupVerification{}, fmt.Errorf("validate backup checkpoint: %w", err)
		}
		if _, err := memory.NewFromBinary(payload); err != nil {
			db.Close()
			return BackupVerification{}, fmt.Errorf("decode backup checkpoint: %w", err)
		}
		verification.CheckpointFormat = formatVersion
		payloadDigest := sha256.Sum256(payload)
		verification.CheckpointSHA256 = hex.EncodeToString(payloadDigest[:])
	}
	if expectedCheckpoint != "" && verification.CheckpointSHA256 != expectedCheckpoint {
		db.Close()
		return BackupVerification{}, fmt.Errorf("verify backup checkpoint digest: got %q, want %s", verification.CheckpointSHA256, expectedCheckpoint)
	}
	if options.ExpectedCheckpointRows != nil {
		if *options.ExpectedCheckpointRows < 0 || *options.ExpectedCheckpointRows > 1 {
			db.Close()
			return BackupVerification{}, errors.New("expected checkpoint rows must be 0 or 1")
		}
		if verification.CheckpointRows != *options.ExpectedCheckpointRows {
			db.Close()
			return BackupVerification{}, fmt.Errorf("verify backup checkpoint rows: got %d, want %d", verification.CheckpointRows, *options.ExpectedCheckpointRows)
		}
	}
	if options.ExpectedCheckpointFormat != nil {
		if *options.ExpectedCheckpointFormat < 0 {
			db.Close()
			return BackupVerification{}, errors.New("expected checkpoint format must not be negative")
		}
		if verification.CheckpointFormat != *options.ExpectedCheckpointFormat {
			db.Close()
			return BackupVerification{}, fmt.Errorf("verify backup checkpoint format: got %d, want %d", verification.CheckpointFormat, *options.ExpectedCheckpointFormat)
		}
	}
	if err := db.Close(); err != nil {
		return BackupVerification{}, fmt.Errorf("close backup verification database: %w", err)
	}

	if err := rejectSQLiteSidecars(path); err != nil {
		return BackupVerification{}, err
	}
	afterSize, afterDigest, afterIdentity, err := hashBackupFile(path)
	if err != nil {
		return BackupVerification{}, err
	}
	if !os.SameFile(beforeIdentity, afterIdentity) || afterSize != beforeSize || afterDigest != beforeDigest {
		return BackupVerification{}, errors.New("verify backup digest: file changed during verification")
	}
	return verification, nil
}

// rejectSQLiteSidecars requires a backup selected for offline copy or restore
// to be a standalone SQLite artifact. A WAL, SHM or rollback journal next to
// the main file makes its logical state ambiguous: immutable read-only opens
// deliberately ignore recovery files, so accepting them could certify or copy
// a stale main database after an unclean shutdown.
func rejectSQLiteSidecars(path string) error {
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		sidecar := path + suffix
		if _, err := os.Lstat(sidecar); err == nil {
			return fmt.Errorf("backup is not standalone: SQLite sidecar exists: %s", sidecar)
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("inspect SQLite sidecar %s: %w", sidecar, err)
		}
	}
	return nil
}

func normalizeExpectedSHA256(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "", nil
	}
	decoded, err := hex.DecodeString(value)
	if err != nil || len(decoded) != sha256.Size {
		return "", errors.New("expected SHA-256 must be exactly 64 hexadecimal characters")
	}
	return value, nil
}

func hashBackupFile(path string) (int64, string, os.FileInfo, error) {
	pathInfo, err := os.Lstat(path)
	if err != nil {
		return 0, "", nil, fmt.Errorf("inspect backup path for digest: %w", err)
	}
	if pathInfo.Mode()&os.ModeSymlink != 0 || !pathInfo.Mode().IsRegular() {
		return 0, "", nil, fmt.Errorf("backup path is not a regular file: %s", path)
	}
	file, err := os.Open(path)
	if err != nil {
		return 0, "", nil, fmt.Errorf("open backup for digest: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return 0, "", nil, fmt.Errorf("stat backup for digest: %w", err)
	}
	if !info.Mode().IsRegular() || !os.SameFile(pathInfo, info) {
		return 0, "", nil, fmt.Errorf("backup path changed before digest: %s", path)
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return 0, "", nil, fmt.Errorf("hash backup: %w", err)
	}
	if err := requireSameRegularPath(path, info); err != nil {
		return 0, "", nil, fmt.Errorf("hash backup: %w", err)
	}
	return info.Size(), hex.EncodeToString(hash.Sum(nil)), info, nil
}

func requireSameRegularPath(path string, expected os.FileInfo) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("reinspect backup path: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("backup path is not a regular file: %s", path)
	}
	if expected == nil || !os.SameFile(expected, info) {
		return fmt.Errorf("backup path identity changed: %s", path)
	}
	return nil
}
