// Command sqlite-backup creates, verifies, or restores a standalone backup of
// the canonical SQLite store. Mutating modes are deliberately offline: stop
// the runtime first so there is a single process responsible for each path.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"

	storage "motor-autonomo/internal/storage/sqlite"
)

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("sqlite-backup", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	mode := fs.String("mode", "backup", "operation: backup, verify, or restore")
	source := fs.String("source", "", "source SQLite path (backup mode) or verified backup path (restore mode)")
	destination := fs.String("destination", "", "new standalone backup path (backup mode) or new runtime path (restore mode)")
	pageSteps := fs.Int("page-steps", 0, "pages per sqlite backup step (0 = all remaining)")
	path := fs.String("path", "", "existing backup path (verify mode)")
	expectedSHA256 := fs.String("expected-sha256", "", "expected 64-character SHA-256 (verify or restore mode)")
	expectedSchemaSHA256 := fs.String("expected-schema-sha256", "", "expected 64-character runtime schema SHA-256 (verify or restore mode)")
	expectedCheckpointSHA256 := fs.String("expected-checkpoint-sha256", "", "expected 64-character runtime checkpoint payload SHA-256 (verify or restore mode)")
	reportPath := fs.String("report-path", "", "optional new path for an atomically published JSON report")
	var expectedSchemaVersion optionalInt
	var expectedSchemaObjects optionalInt
	var expectedPageSize optionalInt
	var expectedPageCount optionalInt
	var expectedCheckpointRows optionalInt
	var expectedCheckpointFormat optionalInt
	fs.Var(&expectedCheckpointRows, "expected-checkpoint-rows", "expected runtime checkpoint row count, 0 or 1 (verify or restore mode)")
	fs.Var(&expectedCheckpointFormat, "expected-checkpoint-format", "expected non-negative runtime checkpoint format (verify or restore mode)")
	fs.Var(&expectedSchemaVersion, "expected-schema-version", "expected non-negative SQLite schema version (verify or restore mode)")
	fs.Var(&expectedSchemaObjects, "expected-schema-objects", "expected canonical runtime schema object count, exactly 1 (verify or restore mode)")
	fs.Var(&expectedPageSize, "expected-page-size", "expected positive SQLite page size in bytes (verify or restore mode)")
	fs.Var(&expectedPageCount, "expected-page-count", "expected positive SQLite page count (verify or restore mode)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments: %v", fs.Args())
	}

	switch *mode {
	case "backup":
		if *source == "" {
			return errors.New("backup mode requires -source")
		}
		if *destination == "" {
			return errors.New("backup mode requires -destination")
		}
		if *pageSteps < 0 || *pageSteps > math.MaxInt32 {
			return fmt.Errorf("page-steps must be between 0 and %d", math.MaxInt32)
		}
		report, err := storage.ClosedCopyTo(ctx, *source, *destination, storage.BackupOptions{PageSteps: int32(*pageSteps)})
		if err != nil {
			return fmt.Errorf("backup: %w", err)
		}
		return emitReport(stdout, *reportPath, report)
	case "verify":
		if *path == "" {
			return errors.New("verify mode requires -path")
		}
		verification, err := storage.VerifyBackupWithOptions(*path, storage.VerificationOptions{
			ExpectedSHA256:           *expectedSHA256,
			ExpectedPageSize:         expectedPageSize.Pointer(),
			ExpectedPageCount:        expectedPageCount.Pointer(),
			ExpectedSchemaVersion:    expectedSchemaVersion.Pointer(),
			ExpectedSchemaObjects:    expectedSchemaObjects.Pointer(),
			ExpectedSchemaSHA256:     *expectedSchemaSHA256,
			ExpectedCheckpointSHA256: *expectedCheckpointSHA256,
			ExpectedCheckpointRows:   expectedCheckpointRows.Pointer(),
			ExpectedCheckpointFormat: expectedCheckpointFormat.Pointer(),
		})
		if err != nil {
			return fmt.Errorf("verify: %w", err)
		}
		return emitReport(stdout, *reportPath, verification)
	case "restore":
		if *source == "" {
			return errors.New("restore mode requires -source")
		}
		if *destination == "" {
			return errors.New("restore mode requires -destination")
		}
		if *pageSteps < 0 || *pageSteps > math.MaxInt32 {
			return fmt.Errorf("page-steps must be between 0 and %d", math.MaxInt32)
		}
		report, err := storage.RestoreToWithOptions(ctx, *source, *destination, storage.RestoreOptions{
			ExpectedSHA256:           *expectedSHA256,
			ExpectedPageSize:         expectedPageSize.Pointer(),
			ExpectedPageCount:        expectedPageCount.Pointer(),
			ExpectedSchemaVersion:    expectedSchemaVersion.Pointer(),
			ExpectedSchemaObjects:    expectedSchemaObjects.Pointer(),
			ExpectedSchemaSHA256:     *expectedSchemaSHA256,
			ExpectedCheckpointSHA256: *expectedCheckpointSHA256,
			ExpectedCheckpointRows:   expectedCheckpointRows.Pointer(),
			ExpectedCheckpointFormat: expectedCheckpointFormat.Pointer(),
			Backup:                   storage.BackupOptions{PageSteps: int32(*pageSteps)},
		})
		if err != nil {
			return fmt.Errorf("restore: %w", err)
		}
		return emitReport(stdout, *reportPath, report)
	default:
		return fmt.Errorf("unsupported mode %q (want backup, verify, or restore)", *mode)
	}
}

func emitReport(stdout io.Writer, reportPath string, report any) error {
	payload, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("encode report: %w", err)
	}
	payload = append(payload, '\n')
	if reportPath != "" {
		if err := writeReportAtomic(reportPath, payload); err != nil {
			return fmt.Errorf("publish report: %w", err)
		}
	}
	if _, err := stdout.Write(payload); err != nil {
		return fmt.Errorf("write report: %w", err)
	}
	return nil
}

// writeReportAtomic gives the inventory report the same fail-closed publication
// posture as the backup itself: create a verified temporary inode, fsync it,
// link it without replacement, then fsync the containing directory.
func writeReportAtomic(path string, payload []byte) error {
	path = filepath.Clean(path)
	if path == "" || path == "." {
		return errors.New("report path is required")
	}
	if _, err := os.Lstat(path); err == nil {
		return fmt.Errorf("report path already exists: %s", path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect report path: %w", err)
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create report parent directory: %w", err)
	}
	temp, err := os.CreateTemp(dir, ".sqlite-backup-report-*")
	if err != nil {
		return fmt.Errorf("create temporary report: %w", err)
	}
	tempPath := temp.Name()
	removeTemp := true
	defer func() {
		if removeTemp {
			_ = os.Remove(tempPath)
		}
	}()
	if err := temp.Chmod(0o600); err != nil {
		temp.Close()
		return fmt.Errorf("restrict report permissions: %w", err)
	}
	if _, err := temp.Write(payload); err != nil {
		temp.Close()
		return fmt.Errorf("write temporary report: %w", err)
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return fmt.Errorf("sync temporary report: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close temporary report: %w", err)
	}
	if err := os.Link(tempPath, path); err != nil {
		return fmt.Errorf("publish report without replacement: %w", err)
	}
	if err := os.Remove(tempPath); err != nil {
		_ = os.Remove(path)
		return fmt.Errorf("remove temporary report name: %w", err)
	}
	removeTemp = false
	directory, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("open report directory: %w", err)
	}
	defer directory.Close()
	if err := directory.Sync(); err != nil {
		return fmt.Errorf("sync report directory: %w", err)
	}
	return nil
}

type optionalInt struct {
	set   bool
	value int
}

func (value *optionalInt) String() string {
	if value == nil || !value.set {
		return ""
	}
	return fmt.Sprint(value.value)
}

func (value *optionalInt) Set(raw string) error {
	parsed, err := strconv.Atoi(raw)
	if err != nil {
		return fmt.Errorf("must be an integer: %w", err)
	}
	value.set = true
	value.value = parsed
	return nil
}

func (value optionalInt) Pointer() *int {
	if !value.set {
		return nil
	}
	copy := value.value
	return &copy
}
