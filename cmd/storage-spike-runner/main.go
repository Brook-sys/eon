//go:build ignore

// Command storage-spike-runner executes the same deterministic measured
// workload against one durable backend and writes reviewable artifacts.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"motor-autonomo/internal/port"
	"motor-autonomo/internal/storage/dolt"
	"motor-autonomo/internal/storage/spike"
	"motor-autonomo/internal/storage/sqlite"
)

type closeStore interface {
	port.Store
	Close() error
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	backend := flag.String("backend", "", "backend: sqlite or dolt-server")
	datasetName := flag.String("dataset", "reduced", "dataset: reduced or full")
	dataDir := flag.String("data-dir", "", "empty directory for backend data")
	outputDir := flag.String("output", "", "directory for manifest, metrics and report")
	batchSize := flag.Int("batch-size", 100, "updates per transaction")
	doltBinary := flag.String("dolt-bin", os.Getenv("DOLT_BIN"), "path to Dolt binary")
	flag.Parse()

	if *backend == "" || *dataDir == "" || *outputDir == "" || *batchSize <= 0 {
		return errors.New("backend, data-dir, output and a positive batch-size are required")
	}
	if err := requireEmptyDirectory(*dataDir); err != nil {
		return err
	}
	var config spike.DatasetConfig
	switch *datasetName {
	case "reduced":
		config = spike.ReducedConfig()
	case "full":
		config = spike.FullConfig()
	default:
		return fmt.Errorf("unsupported dataset %q", *datasetName)
	}
	dataset, manifest, err := spike.Generate(config)
	if err != nil {
		return err
	}

	var store closeStore
	var backendVersion, driverVersion string
	switch *backend {
	case "sqlite":
		var sqliteStore *sqlite.Store
		sqliteStore, err = sqlite.Open(filepath.Join(*dataDir, "runtime.sqlite"))
		store = sqliteStore
		if err == nil {
			var version string
			version, err = sqliteStore.RuntimeVersion()
			backendVersion = "SQLite " + version
		}
		driverVersion = "modernc.org/sqlite v1.39.1"
	case "dolt-server":
		if strings.TrimSpace(*doltBinary) == "" {
			return errors.New("dolt-bin or DOLT_BIN is required for dolt-server")
		}
		store, err = dolt.OpenServer(*doltBinary, filepath.Join(*dataDir, "runtime"))
		backendVersion = commandVersion(*doltBinary)
		driverVersion = "github.com/go-sql-driver/mysql v1.9.3"
	default:
		return fmt.Errorf("unsupported backend %q", *backend)
	}
	if err != nil {
		return fmt.Errorf("open %s: %w", *backend, err)
	}
	closed := false
	defer func() {
		if !closed {
			_ = store.Close()
		}
	}()

	runner := spike.Runner{
		BatchSize: *batchSize, FootprintRoot: *dataDir,
		BackendVersion: backendVersion, DriverVersion: driverVersion,
	}
	metrics, err := runner.Run(context.Background(), *backend, store, dataset, manifest)
	if err != nil {
		return err
	}
	if err := store.Close(); err != nil {
		return fmt.Errorf("close %s: %w", *backend, err)
	}
	closed = true
	if err := spike.WriteArtifacts(*outputDir, manifest, metrics); err != nil {
		return err
	}
	fmt.Printf("wrote %s workload artifacts to %s\n", *backend, *outputDir)
	return nil
}

func requireEmptyDirectory(path string) error {
	entries, err := os.ReadDir(path)
	if errors.Is(err, os.ErrNotExist) {
		return os.MkdirAll(path, 0o755)
	}
	if err != nil {
		return fmt.Errorf("inspect data directory: %w", err)
	}
	if len(entries) != 0 {
		return fmt.Errorf("data directory must be empty: %s", path)
	}
	return nil
}

func commandVersion(binary string) string {
	output, err := exec.Command(binary, "version").CombinedOutput()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(output))
}
