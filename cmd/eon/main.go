// Package main é o entrypoint único do EON (branch v2, ADR-0001).
//
// Um binário, três modos (docs/ARCHITECTURE.md):
//
//	eon supervisor  — L1: mantém o core vivo (PID 1 no container)
//	eon core        — L2: loop multiciclo (M0: boot + heartbeat)
//	eon worker      — L3: 1 job = 1 processo, sandbox cego, protocolo stdio
//	eon health      — healthcheck do supervisor (D53)
//	eon sandbox     — reporta o nível de sandbox detectado
//
// A raiz de composição: o ÚNICO lugar que pode importar tudo (ADR-0002). É
// aqui que a detecção de sandbox acontece no boot e é INJETADA no supervisor
// via config e no core via env — supervisor (stdlib) e core (sem infra)
// nunca detectam sozinhos.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"time"

	"github.com/brook-sys/eon/internal/core"
	"github.com/brook-sys/eon/internal/ctl"
	"github.com/brook-sys/eon/internal/domain"
	"github.com/brook-sys/eon/internal/sandbox"
	"github.com/brook-sys/eon/internal/storage"
	"github.com/brook-sys/eon/internal/supervisor"
	"github.com/brook-sys/eon/internal/worker"
)

// version é injetado no build (-ldflags "-X main.version=shell-vN").
var version = "dev"

func main() {
	// Papéis de subprocesso (probes do sandbox, aplicador do worker):
	// re-exec do próprio binário, ANTES de qualquer dispatch normal.
	if role := os.Getenv(domain.RoleEnv); role != "" {
		switch role {
		case domain.RoleProbe:
			os.Exit(sandbox.ProbeMain())
		case domain.RoleWorkerChild:
			os.Exit(worker.ChildMain())
		}
		fmt.Fprintf(os.Stderr, "eon: papel de processo desconhecido: %q\n", role)
		os.Exit(2)
	}

	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "supervisor":
		os.Exit(supervisorCmd())
	case "core":
		os.Exit(coreCmd(os.Args[2:]))
	case "ctl":
		os.Exit(ctl.Run(os.Args[2:], os.Stdout))
	case "worker":
		os.Exit(worker.Main(os.Args[2:]))
	case "health":
		os.Exit(supervisor.HealthMain(os.Args[2:]))
	case "sandbox":
		os.Exit(sandboxCmd())
	case "version":
		fmt.Printf("eon %s %s/%s\n", version, runtime.GOOS, runtime.GOARCH)
		os.Exit(0)
	default:
		usage()
		os.Exit(2)
	}
}

func supervisorCmd() int {
	cfg, err := supervisor.ParseArgs(os.Args[2:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "eon supervisor: %v\n", err)
		return 2
	}
	cfg.ShellVersion = version
	lvl, rep := sandbox.Detect()
	cfg.SandboxLevel = lvl.String()
	fmt.Fprintf(os.Stderr, "eon supervisor: sandbox=%s %s\n", lvl, sandbox.ReportJSON(rep))
	return supervisor.RunSignals(cfg)
}

func sandboxCmd() int {
	lvl, rep := sandbox.Detect()
	fmt.Printf("level=%s\n%s\n", lvl, sandbox.ReportJSON(rep))
	return 0
}

// coreCmd monta o mundo do core (ADR-0002: storage é a ÚNICA camada que
// conhece SQLite; este é o único lugar onde core e storage se encontram):
// abre a store no DB canônico, injeta os repositórios estreitos + o
// provider fake do M1 nas Deps do kernel e entrega ao core.Main.
func coreCmd(args []string) int {
	fs := flag.NewFlagSet("core", flag.ContinueOnError)
	dataDir := fs.String("data", os.Getenv("EON_DATA"), "diretório de dados")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	if *dataDir == "" {
		*dataDir = "/data"
	}

	store, err := storage.NewStore(filepath.Join(*dataDir, "core", "eon.db"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "eon core: storage: %v\n", err)
		return 1
	}
	defer store.Close()

	deps := core.Deps{
		Begin:         store.Begin,
		Missions:      store.Missions(),
		Tasks:         store.Tasks(),
		Steps:         store.Steps(),
		Events:        store.Events(),
		Outbox:        store.Outbox(),
		KV:            store.KV(),
		Provider:      core.NewFakeProvider(fakeQuotaFromEnv()),
		CycleInterval: cycleIntervalFromEnv(),
	}
	return core.Main(*dataDir, deps)
}

// fakeQuotaFromEnv: orçamento fake do provider M1 (D39). Padrão alto —
// o soak de esgotamento usa valor explícito; produção de verdade calibra
// por ADR quando os números forem reais.
func fakeQuotaFromEnv() int {
	if v := os.Getenv("EON_FAKE_QUOTA"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return 1_000_000
}

// cycleIntervalFromEnv: cadência de ciclo (0 → default do kernel).
// ENV, não flag: knobs operacionais são do supervisor/deploy, não da CLI.
func cycleIntervalFromEnv() time.Duration {
	if v := os.Getenv("EON_CYCLE_INTERVAL_MS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return time.Duration(n) * time.Millisecond
		}
	}
	return 0
}

func usage() {
	fmt.Fprint(os.Stderr, `eon — autômato que nunca para (v2)

uso: eon <modo> [flags]

modos:
  supervisor   L1: mantém o core vivo (PID 1 no container)
  core         L2: loop multiciclo
  ctl          a janela do operador (inspect, timeline — read-only)
  worker       L3: job via stdio (JobSpec JSON no stdin, JobResult no stdout)
  health       healthcheck do supervisor (D53)
  sandbox      reporta o nível de sandbox detectado (full/basic/none)
  version      versão da casca

flags comuns: --data DIR (default $EON_DATA ou /data)
`)
}
