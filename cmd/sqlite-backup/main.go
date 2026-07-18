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
	inventoryPath := fs.String("inventory", "", "existing backup inventory JSON whose identities must match (verify or restore mode)")
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
	verificationOptions := storage.VerificationOptions{
		ExpectedSHA256:           *expectedSHA256,
		ExpectedPageSize:         expectedPageSize.Pointer(),
		ExpectedPageCount:        expectedPageCount.Pointer(),
		ExpectedSchemaVersion:    expectedSchemaVersion.Pointer(),
		ExpectedSchemaObjects:    expectedSchemaObjects.Pointer(),
		ExpectedSchemaSHA256:     *expectedSchemaSHA256,
		ExpectedCheckpointSHA256: *expectedCheckpointSHA256,
		ExpectedCheckpointRows:   expectedCheckpointRows.Pointer(),
		ExpectedCheckpointFormat: expectedCheckpointFormat.Pointer(),
	}
	if *inventoryPath != "" {
		if *mode != "verify" && *mode != "restore" {
			return errors.New("-inventory is supported only in verify or restore mode")
		}
		if hasExplicitExpectations(fs) {
			return errors.New("-inventory cannot be combined with explicit -expected-* flags")
		}
		inventory, err := loadInventory(*inventoryPath)
		if err != nil {
			return fmt.Errorf("load inventory: %w", err)
		}
		verificationOptions = inventory.verificationOptions()
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
		verification, err := storage.VerifyBackupWithOptions(*path, verificationOptions)
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
			ExpectedSHA256:           verificationOptions.ExpectedSHA256,
			ExpectedPageSize:         verificationOptions.ExpectedPageSize,
			ExpectedPageCount:        verificationOptions.ExpectedPageCount,
			ExpectedSchemaVersion:    verificationOptions.ExpectedSchemaVersion,
			ExpectedSchemaObjects:    verificationOptions.ExpectedSchemaObjects,
			ExpectedSchemaSHA256:     verificationOptions.ExpectedSchemaSHA256,
			ExpectedCheckpointSHA256: verificationOptions.ExpectedCheckpointSHA256,
			ExpectedCheckpointRows:   verificationOptions.ExpectedCheckpointRows,
			ExpectedCheckpointFormat: verificationOptions.ExpectedCheckpointFormat,
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

const maxInventoryBytes = 64 << 10

type backupInventory struct {
	SourcePath       string `json:"source_path"`
	SourceSHA256     string `json:"source_sha256"`
	DestinationPath  string `json:"destination_path"`
	PagesCopied      int64  `json:"pages_copied"`
	Duration         int64  `json:"duration"`
	SQLiteVersion    string `json:"sqlite_version"`
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
	CheckpointFormat int    `json:"checkpoint_format"`
	CheckpointSHA256 string `json:"checkpoint_sha256"`
	IntegrityCheck   string `json:"integrity_check"`
	ForeignKeyCheck  string `json:"foreign_key_check"`
}

func loadInventory(path string) (backupInventory, error) {
	file, err := os.Open(filepath.Clean(path))
	if err != nil {
		return backupInventory{}, err
	}
	defer file.Close()
	decoder := json.NewDecoder(io.LimitReader(file, maxInventoryBytes+1))
	decoder.DisallowUnknownFields()
	var inventory backupInventory
	if err := decoder.Decode(&inventory); err != nil {
		return backupInventory{}, fmt.Errorf("decode JSON: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return backupInventory{}, errors.New("inventory contains trailing JSON")
		}
		return backupInventory{}, fmt.Errorf("decode trailing JSON: %w", err)
	}
	if info, err := file.Stat(); err != nil {
		return backupInventory{}, err
	} else if info.Size() > maxInventoryBytes {
		return backupInventory{}, fmt.Errorf("inventory exceeds %d bytes", maxInventoryBytes)
	}
	if inventory.FileSize <= 0 || inventory.PageSize <= 0 || inventory.PageCount <= 0 {
		return backupInventory{}, errors.New("inventory physical identity is incomplete")
	}
	if inventory.FileSize != int64(inventory.PageSize)*int64(inventory.PageCount) {
		return backupInventory{}, errors.New("inventory page geometry does not match file size")
	}
	if inventory.ApplicationID != 0x4d415554 || inventory.UserVersion != 1 {
		return backupInventory{}, errors.New("inventory application identity is not canonical")
	}
	if inventory.SchemaVersion < 0 || inventory.SchemaObjects != 1 || inventory.CheckpointRows < 0 || inventory.CheckpointRows > 1 || inventory.CheckpointFormat < 0 {
		return backupInventory{}, errors.New("inventory logical identity is invalid")
	}
	if inventory.IntegrityCheck != "ok" || inventory.ForeignKeyCheck != "ok" {
		return backupInventory{}, errors.New("inventory does not record successful integrity checks")
	}
	if len(inventory.SHA256) != sha256HexLength || len(inventory.SchemaSHA256) != sha256HexLength ||
		(inventory.CheckpointRows == 1 && len(inventory.CheckpointSHA256) != sha256HexLength) ||
		(inventory.CheckpointRows == 0 && inventory.CheckpointSHA256 != "") {
		return backupInventory{}, errors.New("inventory digest identity is incomplete")
	}
	return inventory, nil
}

func (inventory backupInventory) verificationOptions() storage.VerificationOptions {
	return storage.VerificationOptions{
		ExpectedSHA256:           inventory.SHA256,
		ExpectedPageSize:         intPointer(inventory.PageSize),
		ExpectedPageCount:        intPointer(inventory.PageCount),
		ExpectedSchemaVersion:    intPointer(inventory.SchemaVersion),
		ExpectedSchemaObjects:    intPointer(inventory.SchemaObjects),
		ExpectedSchemaSHA256:     inventory.SchemaSHA256,
		ExpectedCheckpointSHA256: inventory.CheckpointSHA256,
		ExpectedCheckpointRows:   intPointer(inventory.CheckpointRows),
		ExpectedCheckpointFormat: intPointer(inventory.CheckpointFormat),
	}
}

func intPointer(value int) *int { return &value }

const sha256HexLength = 64

func hasExplicitExpectations(fs *flag.FlagSet) bool {
	found := false
	fs.Visit(func(entry *flag.Flag) {
		if len(entry.Name) >= len("expected-") && entry.Name[:len("expected-")] == "expected-" {
			found = true
		}
	})
	return found
}
