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
	"fmt"
	"os"
	"runtime"

	"github.com/brook-sys/eon/internal/core"
	"github.com/brook-sys/eon/internal/domain"
	"github.com/brook-sys/eon/internal/sandbox"
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
		os.Exit(core.Main(os.Args[2:]))
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

func usage() {
	fmt.Fprint(os.Stderr, `eon — autômato que nunca para (v2)

uso: eon <modo> [flags]

modos:
  supervisor   L1: mantém o core vivo (PID 1 no container)
  core         L2: loop multiciclo
  worker       L3: job via stdio (JobSpec JSON no stdin, JobResult no stdout)
  health       healthcheck do supervisor (D53)
  sandbox      reporta o nível de sandbox detectado (full/basic/none)
  version      versão da casca

flags comuns: --data DIR (default $EON_DATA ou /data)
`)
}
