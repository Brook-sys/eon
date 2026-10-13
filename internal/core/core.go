// Package core é L2: o loop multiciclo, reiniciável, com TODO estado
// canônico em SQLite (que chega no M1 — docs/plan/03-LOOP.md).
//
// M0: boot detectável (boot.json), heartbeat contínuo (heartbeat.json) e
// shutdown limpo em SIGTERM. O nível de sandbox chega por ENV injetado pelo
// supervisor — o core NUNCA detecta sozinho (core não importa infra, ADR-0002).
//
// Lei (ADR-0002): importa stdlib + domain + port + statefile (util de
// filesystem, semântica de stdlib estendida — exceção folha documentada).
package core

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/brook-sys/eon/internal/domain"
	"github.com/brook-sys/eon/internal/statefile"
)

type bootInfo struct {
	Boot         time.Time `json:"boot"`
	Pid          int       `json:"pid"`
	SandboxLevel string    `json:"sandbox_level"`
	ShellVersion string    `json:"shell_version"`
}

type heartbeatInfo struct {
	Beat  time.Time `json:"beat"`
	Pid   int       `json:"pid"`
	Beats int64     `json:"beats"`
}

// Main é o modo `eon core`.
func Main(args []string) int {
	fs := flag.NewFlagSet("core", flag.ContinueOnError)
	dataDir := fs.String("data", os.Getenv("EON_DATA"), "diretório de dados")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	if *dataDir == "" {
		*dataDir = "/data"
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer stop()
	if err := Run(ctx, *dataDir); err != nil {
		fmt.Fprintf(os.Stderr, "core: %v\n", err)
		return 1
	}
	return 0
}

// Run: boot → heartbeat até o contexto morrer → shutdown limpo.
func Run(ctx context.Context, dataDir string) error {
	dir := filepath.Join(dataDir, "core")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("criar %s: %w", dir, err)
	}

	boot := bootInfo{
		Boot:         time.Now().UTC(),
		Pid:          os.Getpid(),
		SandboxLevel: os.Getenv(domain.EnvSandboxLevel),
		ShellVersion: os.Getenv(domain.EnvShellVersion),
	}
	if err := statefile.Write(filepath.Join(dir, "boot.json"), boot); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "core: boot sandbox=%q shell=%q data=%s\n",
		boot.SandboxLevel, boot.ShellVersion, dataDir)

	tick := time.NewTicker(5 * time.Second)
	defer tick.Stop()
	hb := heartbeatInfo{Pid: os.Getpid()}
	writeHB := func() {
		hb.Beat = time.Now().UTC()
		hb.Beats++
		_ = statefile.Write(filepath.Join(dir, "heartbeat.json"), hb)
	}
	writeHB()

	for {
		select {
		case <-ctx.Done():
			fmt.Fprintln(os.Stderr, "core: shutdown")
			return nil
		case <-tick.C:
			writeHB()
		}
	}
}
