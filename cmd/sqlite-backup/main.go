// Command sqlite-backup creates or verifies a standalone backup of the
// canonical SQLite store. Backup mode is deliberately offline: stop the
// runtime first so there is a single process responsible for the source path.
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
	mode := fs.String("mode", "backup", "operation: backup or verify")
	source := fs.String("source", "", "source SQLite path (backup mode)")
	destination := fs.String("destination", "", "new standalone backup path (backup mode)")
	pageSteps := fs.Int("page-steps", 0, "pages per sqlite backup step (0 = all remaining)")
	path := fs.String("path", "", "existing backup path (verify mode)")
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
		verification, err := storage.VerifyBackup(*path)
		if err != nil {
			return fmt.Errorf("verify: %w", err)
		}
		if err := encoder.Encode(verification); err != nil {
			return fmt.Errorf("encode verification report: %w", err)
		}
		return nil
	default:
		return fmt.Errorf("unsupported mode %q (want backup or verify)", *mode)
	}
}
