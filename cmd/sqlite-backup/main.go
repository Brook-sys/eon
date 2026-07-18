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

	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
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
		if err := encoder.Encode(report); err != nil {
			return fmt.Errorf("encode backup report: %w", err)
		}
		return nil
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
		if err := encoder.Encode(verification); err != nil {
			return fmt.Errorf("encode verification report: %w", err)
		}
		return nil
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
		if err := encoder.Encode(report); err != nil {
			return fmt.Errorf("encode restore report: %w", err)
		}
		return nil
	default:
		return fmt.Errorf("unsupported mode %q (want backup, verify, or restore)", *mode)
	}
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
