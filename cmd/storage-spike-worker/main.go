// Command storage-spike-worker executes one durable mutation and deliberately
// terminates at a configured adapter boundary. It exists only for the storage
// comparison harness; the runtime never invokes it.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"motor-autonomo/internal/storage/dolt"
	"motor-autonomo/internal/storage/spike"
	"motor-autonomo/internal/storage/sqlite"
)

const crashExitCode = 86

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
}

func run() error {
	backend := flag.String("backend", "", "backend: sqlite or dolt")
	path := flag.String("path", "", "backend data path")
	failpoint := flag.String("failpoint", "", "adapter failpoint")
	intentPath := flag.String("intent", "", "JSON crash intent path")
	markerPath := flag.String("marker", "", "durable worker-intention marker path")
	doltBinary := flag.String("dolt-bin", "", "explicit Dolt executable path")
	flag.Parse()

	if strings.TrimSpace(*path) == "" || strings.TrimSpace(*failpoint) == "" || strings.TrimSpace(*intentPath) == "" || strings.TrimSpace(*markerPath) == "" {
		return errors.New("backend, path, failpoint, intent, and marker are required")
	}
	intentBytes, err := os.ReadFile(*intentPath)
	if err != nil {
		return fmt.Errorf("read crash intent: %w", err)
	}
	var intent spike.CrashIntent
	if err := json.Unmarshal(intentBytes, &intent); err != nil {
		return fmt.Errorf("decode crash intent: %w", err)
	}
	if err := intent.Event.ValidateForAppend(); err != nil {
		return fmt.Errorf("validate crash intent: %w", err)
	}
	if err := writeMarker(*markerPath, intentBytes); err != nil {
		return err
	}

	ctx := context.Background()
	switch *backend {
	case "sqlite":
		wanted := sqlite.Failpoint(*failpoint)
		if wanted != sqlite.FailpointBeforeDurableCommit && wanted != sqlite.FailpointAfterDurableCommit {
			return fmt.Errorf("unknown sqlite failpoint %q", *failpoint)
		}
		store, err := sqlite.OpenWithOptions(*path, sqlite.Options{Failpoint: func(got sqlite.Failpoint) {
			if got == wanted {
				crashNow()
			}
		}})
		if err != nil {
			return err
		}
		defer store.Close()
		if err := spike.ApplyCrashIntent(ctx, store, intent); err != nil {
			return err
		}
		return fmt.Errorf("configured failpoint %q was not reached", *failpoint)
	case "dolt":
		if strings.TrimSpace(*doltBinary) == "" {
			return errors.New("dolt-bin is required for the Dolt backend")
		}
		wanted := dolt.Failpoint(*failpoint)
		if wanted != dolt.FailpointBeforeSQLAndDoltCommit && wanted != dolt.FailpointAfterSQLAndDoltCommit {
			return fmt.Errorf("unknown dolt failpoint %q", *failpoint)
		}
		store, err := dolt.OpenWithOptions(*doltBinary, *path, dolt.Options{Failpoint: func(got dolt.Failpoint) {
			if got == wanted {
				crashNow()
			}
		}})
		if err != nil {
			return err
		}
		defer store.Close()
		if err := spike.ApplyCrashIntent(ctx, store, intent); err != nil {
			return err
		}
		return fmt.Errorf("configured failpoint %q was not reached", *failpoint)
	default:
		return fmt.Errorf("unknown backend %q", *backend)
	}
}

func writeMarker(path string, content []byte) error {
	temporary := path + ".tmp"
	file, err := os.OpenFile(temporary, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create crash marker: %w", err)
	}
	if _, err := file.Write(content); err != nil {
		file.Close()
		return fmt.Errorf("write crash marker: %w", err)
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return fmt.Errorf("sync crash marker: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close crash marker: %w", err)
	}
	if err := os.Rename(temporary, path); err != nil {
		return fmt.Errorf("publish crash marker: %w", err)
	}
	directory, err := os.Open(filepath.Dir(path))
	if err != nil {
		return fmt.Errorf("open crash marker directory: %w", err)
	}
	defer directory.Close()
	if err := directory.Sync(); err != nil {
		return fmt.Errorf("sync crash marker directory: %w", err)
	}
	return nil
}

func crashNow() {
	process, err := os.FindProcess(os.Getpid())
	if err == nil {
		_ = process.Kill()
	}
	os.Exit(crashExitCode)
}
