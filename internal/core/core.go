// Package core é L2: o loop multiciclo, reiniciável, com TODO estado
// canônico em SQLite (docs/plan/03-LOOP.md). O core NUNCA conhece
// storage: recebe os repositórios do port montados pela raiz de
// composição (cmd/eon) — ADR-0002. O nível de sandbox chega por ENV
// injetado pelo supervisor — o core NUNCA detecta sozinho.
//
// Lei (ADR-0002): importa stdlib + domain + port + statefile (util de
// filesystem, semântica de stdlib estendida — exceção folha documentada).
package core

import (
	"context"
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
	Beat     time.Time `json:"beat"`
	Pid      int       `json:"pid"`
	Beats    int64     `json:"beats"`
	Cycle    int64     `json:"cycle"`
	Failures int64     `json:"failures"`
}

// Main é o modo `eon core`: boot detectável → kernel → loop até SIGTERM.
// Shutdown limpo sai com 0 — é o contrato que o supervisor usa para
// distinguir crash de shutdown. Deps chegam prontas da raiz de composição.
func Main(dataDir string, deps Deps) int {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer stop()
	if err := Run(ctx, dataDir, deps); err != nil {
		fmt.Fprintf(os.Stderr, "core: %v\n", err)
		return 1
	}
	return 0
}

// Run: boot detectável (boot.json) → Bootstrap do kernel (missão stub +
// reconciliação de órfãs) → loop: um ciclo, drenagem de outbox,
// heartbeat por batida. Erro de CICLO derruba a tentativa (log +
// contador), nunca o autômato (invariante 3); erro de BOOT derruba o
// processo — o supervisor decide (backoff/restart).
func Run(ctx context.Context, dataDir string, deps Deps) error {
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

	k := NewKernel(deps)
	if err := k.Bootstrap(ctx); err != nil {
		return fmt.Errorf("bootstrap: %w", err)
	}

	interval := deps.CycleInterval
	if interval <= 0 {
		interval = defaultCycleInterval
	}

	hb := heartbeatInfo{Pid: os.Getpid(), Cycle: k.CycleCount()}
	writeHB := func() {
		hb.Beat = time.Now().UTC()
		hb.Beats++
		_ = statefile.Write(filepath.Join(dir, "heartbeat.json"), hb)
	}
	writeHB()

	tick := time.NewTicker(interval)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			fmt.Fprintln(os.Stderr, "core: shutdown")
			return nil
		case <-tick.C:
			if err := k.Cycle(ctx); err != nil {
				hb.Failures++
				k.log.Printf("ciclo %d derrubado (o loop segue): %v", k.CycleCount(), err)
			} else {
				hb.Cycle = k.CycleCount()
			}
			if err := k.DrainOutbox(ctx); err != nil {
				k.log.Printf("outbox: drenagem falhou (tenta de novo no próximo ciclo): %v", err)
			}
			writeHB()
		}
	}
}
