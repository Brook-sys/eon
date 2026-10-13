// Package supervisor é L1 (docs/ARCHITECTURE.md): o watchdog de processos
// imutável. Mantém o core vivo — spawn, backoff com jitter, restart;
// crash-loop detectado → re-promove o último binário bom (D41/ADR-0004).
// NUNCA para por conta própria (invariante 1). NUNCA é modificado pelo
// autômato — muda só via imagem da casca, via humano.
//
// Lei (ADR-0002): stdlib + statefile (util de filesystem). Não conhece o
// core como import — o core é um PROCESSO, gerenciado por sinais.
package supervisor

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"math/rand/v2"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/brook-sys/eon/internal/statefile"
)

// Duplicados deliberadamente de internal/domain: supervisor não importa
// domain (ADR-0002); os nomes são contrato de operação (ver domain.go).
const (
	envSandboxLevel = "EON_SANDBOX_LEVEL"
	envShellVersion = "EON_SHELL_VERSION"
)

// Config configura o watchdog. Todos os campos injetáveis — os testes usam
// cores falsos e constantes de tempo curtas.
type Config struct {
	DataDir        string
	SelfBin        string   // binário spawnado como core (default: próprio exe)
	CoreArgv       []string // argv após o binário (default: ["core","--data",DataDir])
	CoreEnv        []string // env extra para o core
	SandboxLevel   string   // detectado no boot pelo cmd/eon (L0 não detecta sozinho)
	ShellVersion   string   // versão da casca (main.version)
	BaseBackoff    time.Duration
	MaxBackoff     time.Duration
	CrashThreshold int // exits na janela → crash-loop
	CrashWindow    time.Duration
	HealthInterval time.Duration
}

// DefaultConfig: constantes de produção.
func DefaultConfig(dataDir, selfBin, sandboxLevel string) Config {
	return Config{
		DataDir:        dataDir,
		SelfBin:        selfBin,
		CoreArgv:       []string{"core", "--data", dataDir},
		SandboxLevel:   sandboxLevel,
		BaseBackoff:    time.Second,
		MaxBackoff:     5 * time.Minute,
		CrashThreshold: 5,
		CrashWindow:    5 * time.Minute,
		HealthInterval: 10 * time.Second,
	}
}

// ParseArgs: `eon supervisor [--data DIR]` (env EON_DATA, default /data).
func ParseArgs(args []string) (Config, error) {
	fs := flag.NewFlagSet("supervisor", flag.ContinueOnError)
	dataDir := fs.String("data", os.Getenv("EON_DATA"), "diretório de dados")
	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}
	if *dataDir == "" {
		*dataDir = "/data"
	}
	self, err := os.Executable()
	if err != nil {
		return Config{}, err
	}
	return DefaultConfig(*dataDir, self, ""), nil
}

// healthState é o contrato D53: supervisor publica; `eon health`, Docker,
// Portainer e o watchdog sidecar leem.
type healthState struct {
	V             int       `json:"v"`
	SupervisorPid int       `json:"supervisor_pid"`
	Boot          time.Time `json:"boot"`
	Heartbeat     time.Time `json:"heartbeat"`
	SandboxLevel  string    `json:"sandbox_level"`
	ShellVersion  string    `json:"shell_version"`
	TargetBinary  string    `json:"target_binary"`
	TargetVersion int       `json:"target_version"` // 0 = casca (shell)
	CorePid       int       `json:"core_pid"`
	CoreStarted   time.Time `json:"core_started"`
	CoreRestarts  int       `json:"core_restarts"`
	CrashLoops    int       `json:"crash_loops"`
	LastIncident  string    `json:"last_incident,omitempty"`
}

// HealthStale: heartbeat mais velho que isso = unhealthy.
const HealthStale = 60 * time.Second

// artifact: binário promovido em <data>/artifacts/eon/vN/ (eon + eon.sha256).
type artifact struct {
	version int
	dir     string
	bin     string
}

// target é o que o supervisor spawna como core.
type target struct {
	isArtifact bool
	version    int // 0 = casca (shell)
	path       string
}

func (t target) label() string {
	if t.isArtifact {
		return fmt.Sprintf("v%d", t.version)
	}
	return "shell"
}

// discoverArtifacts valida o SHA-256 de cada versão (D41: verificação
// pre-swap — binário corrompido não sobe) e devolve em ordem decrescente.
func discoverArtifacts(dataDir string) []artifact {
	base := filepath.Join(dataDir, "artifacts", "eon")
	entries, err := os.ReadDir(base)
	if err != nil {
		return nil
	}
	var out []artifact
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), "v") {
			continue
		}
		v, err := strconv.Atoi(strings.TrimPrefix(e.Name(), "v"))
		if err != nil || v < 1 {
			continue
		}
		dir := filepath.Join(base, e.Name())
		bin := filepath.Join(dir, "eon")
		sumFile := filepath.Join(dir, "eon.sha256")
		if !isRegularFile(bin) || !isRegularFile(sumFile) {
			continue
		}
		if !verifySHA256(bin, sumFile) {
			continue
		}
		out = append(out, artifact{version: v, dir: dir, bin: bin})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].version > out[j].version })
	return out
}

func isRegularFile(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.Mode().IsRegular()
}

func verifySHA256(bin, sumFile string) bool {
	want, err := os.ReadFile(sumFile)
	if err != nil {
		return false
	}
	fields := strings.Fields(string(want))
	if len(fields) == 0 {
		return false
	}
	f, err := os.Open(bin)
	if err != nil {
		return false
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return false
	}
	return strings.EqualFold(hex.EncodeToString(h.Sum(nil)), fields[0])
}

// selectTarget: último binário bom promovido; sem artefatos → a casca
// (D41: o supervisor dá boot no binário promovido, não no binário da casca —
// a casca é o fallback de última instância).
func selectTarget(dataDir, selfBin string) target {
	arts := discoverArtifacts(dataDir)
	if len(arts) > 0 {
		return target{isArtifact: true, version: arts[0].version, path: arts[0].bin}
	}
	return target{isArtifact: false, version: 0, path: selfBin}
}

// demote: crash-loop → re-promove o último binário bom (D41).
// vN → maior válido < N; sem inferior → casca. Da casca não há para onde
// cair: mantém vivo com MaxBackoff — NUNCA para (invariante 1).
func demote(cfg Config, cur target) (target, bool) {
	if !cur.isArtifact {
		return cur, false
	}
	for _, a := range discoverArtifacts(cfg.DataDir) { // ordem decrescente
		if a.version < cur.version {
			return target{isArtifact: true, version: a.version, path: a.bin}, true
		}
	}
	return target{isArtifact: false, version: 0, path: cfg.SelfBin}, true
}

// Run é o loop L1. Retorna 0 só em shutdown solicitado (ctx).
func Run(ctx context.Context, cfg Config) int {
	for _, d := range []string{
		filepath.Join(cfg.DataDir, "supervisor"),
		filepath.Join(cfg.DataDir, "core"),
		filepath.Join(cfg.DataDir, "jobs"),
		filepath.Join(cfg.DataDir, "artifacts", "eon"),
	} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "supervisor: %v\n", err)
			return 1
		}
	}

	logF, err := os.OpenFile(filepath.Join(cfg.DataDir, "supervisor", "core.log"),
		os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "supervisor: %v\n", err)
		return 1
	}
	defer logF.Close()
	incidents, err := os.OpenFile(filepath.Join(cfg.DataDir, "supervisor", "incidents.log"),
		os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "supervisor: %v\n", err)
		return 1
	}
	defer incidents.Close()

	var mu sync.Mutex
	state := healthState{
		V:             1,
		SupervisorPid: os.Getpid(),
		Boot:          time.Now().UTC(),
		SandboxLevel:  cfg.SandboxLevel,
		ShellVersion:  cfg.ShellVersion,
		TargetBinary:  "",
		TargetVersion: -1,
	}

	incident := func(msg string) {
		fmt.Fprintf(incidents, "%s\t%s\n", time.Now().UTC().Format(time.RFC3339), msg)
		mu.Lock()
		state.LastIncident = msg
		mu.Unlock()
		fmt.Fprintf(os.Stderr, "supervisor: INCIDENTE %s\n", msg)
	}

	// Health loop (D53): heartbeat atômico a cada HealthInterval.
	hbCtx, hbCancel := context.WithCancel(ctx)
	defer hbCancel()
	healthPath := filepath.Join(cfg.DataDir, "supervisor", "health.json")
	go func() {
		tick := time.NewTicker(cfg.HealthInterval)
		defer tick.Stop()
		for {
			mu.Lock()
			state.Heartbeat = time.Now().UTC()
			snap := state
			mu.Unlock()
			_ = statefile.Write(healthPath, snap)
			select {
			case <-hbCtx.Done():
				return
			case <-tick.C:
			}
		}
	}()

	tgt := selectTarget(cfg.DataDir, cfg.SelfBin)
	fmt.Fprintf(os.Stderr, "supervisor: boot data=%s sandbox=%s target=%s\n",
		cfg.DataDir, cfg.SandboxLevel, tgt.label())

	backoff := cfg.BaseBackoff
	var crashTimes []time.Time

	terminateCore := func(cmd *exec.Cmd, waitCh <-chan error) {
		_ = cmd.Process.Signal(syscall.SIGTERM)
		select {
		case <-waitCh:
		case <-time.After(10 * time.Second):
			_ = cmd.Process.Kill()
			<-waitCh
		}
		mu.Lock()
		state.CorePid = 0
		mu.Unlock()
	}

runLoop:
	for ctx.Err() == nil {
		cmd := exec.Command(tgt.path, cfg.CoreArgv...)
		cmd.Env = append(os.Environ(),
			envSandboxLevel+"="+cfg.SandboxLevel,
			envShellVersion+"="+cfg.ShellVersion)
		cmd.Env = append(cmd.Env, cfg.CoreEnv...)
		cmd.Stdout = logF
		cmd.Stderr = logF

		if err := cmd.Start(); err != nil {
			incident(fmt.Sprintf("spawn falhou target=%s: %v", tgt.label(), err))
			if !sleepCtx(ctx, backoff) {
				break runLoop
			}
			backoff = growBackoff(backoff, cfg)
			continue
		}

		mu.Lock()
		state.TargetBinary = tgt.path
		state.TargetVersion = tgt.version
		state.CorePid = cmd.Process.Pid
		state.CoreStarted = time.Now().UTC()
		mu.Unlock()

		waitCh := make(chan error, 1)
		go func() { waitCh <- cmd.Wait() }()

		select {
		case werr := <-waitCh:
			exitCode := 0
			if werr != nil {
				var ee *exec.ExitError
				if errors.As(werr, &ee) {
					exitCode = ee.ExitCode()
				} else {
					exitCode = -1
				}
			}
			mu.Lock()
			state.CorePid = 0
			state.CoreRestarts++
			mu.Unlock()

			if exitCode == 0 {
				// O core NUNCA deve sair sozinho (invariante 1):
				// anomalia registrada, reinicia do mesmo jeito.
				incident(fmt.Sprintf("anomalia: core saiu com 0 sem shutdown do supervisor (target %s)", tgt.label()))
			}

			// Janela deslizante de crashes.
			cut := time.Now().Add(-cfg.CrashWindow)
			fresh := crashTimes[:0]
			for _, t := range crashTimes {
				if t.After(cut) {
					fresh = append(fresh, t)
				}
			}
			crashTimes = append(fresh, time.Now())

			if len(crashTimes) >= cfg.CrashThreshold {
				mu.Lock()
				state.CrashLoops++
				mu.Unlock()
				incident(fmt.Sprintf("crash-loop: %d exits em %s no target %s (último exit=%d)",
					len(crashTimes), cfg.CrashWindow, tgt.label(), exitCode))

				if next, changed := demote(cfg, tgt); changed {
					incident(fmt.Sprintf("re-promove último binário bom: %s → %s", tgt.label(), next.label()))
					tgt = next
					crashTimes = nil
					backoff = cfg.BaseBackoff
					continue // reinicia imediato com o binário bom
				}
				// Sem para onde cair: mantém vivo com backoff no teto.
				crashTimes = nil
				backoff = cfg.MaxBackoff
			}

			if !sleepCtx(ctx, backoff) {
				break runLoop
			}
			backoff = growBackoff(backoff, cfg)

		case <-ctx.Done():
			terminateCore(cmd, waitCh)
			break runLoop
		}
	}

	mu.Lock()
	state.Heartbeat = time.Now().UTC()
	snap := state
	mu.Unlock()
	_ = statefile.Write(healthPath, snap)
	fmt.Fprintln(os.Stderr, "supervisor: shutdown")
	return 0
}

func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

func growBackoff(cur time.Duration, cfg Config) time.Duration {
	next := cur * 2
	if next > cfg.MaxBackoff {
		next = cfg.MaxBackoff
	}
	// Jitter ±20% — evita ressincronização de restarts.
	j := time.Duration(float64(next) * 0.2 * (rand.Float64()*2 - 1))
	if n := next + j; n > 0 {
		next = n
	}
	return next
}

// LoadState lê o health.json publicado pelo supervisor.
func LoadState(dataDir string) (healthState, error) {
	var st healthState
	err := statefile.Read(filepath.Join(dataDir, "supervisor", "health.json"), &st)
	return st, err
}

// Check avalia a saúde a partir do health.json (D53): supervisor vivo,
// heartbeat fresco, core vivo.
func Check(dataDir string) error {
	st, err := LoadState(dataDir)
	if err != nil {
		return err
	}
	if err := pidAlive(st.SupervisorPid); err != nil {
		return fmt.Errorf("supervisor (pid %d): %w", st.SupervisorPid, err)
	}
	if time.Since(st.Heartbeat) > HealthStale {
		return fmt.Errorf("heartbeat obsoleto (%s)", time.Since(st.Heartbeat).Round(time.Second))
	}
	if st.CorePid == 0 {
		return errors.New("core não está rodando")
	}
	if err := pidAlive(st.CorePid); err != nil {
		return fmt.Errorf("core (pid %d): %w", st.CorePid, err)
	}
	return nil
}

func pidAlive(pid int) error {
	if pid <= 0 {
		return errors.New("pid inválido")
	}
	err := syscall.Kill(pid, 0)
	if err == nil || errors.Is(err, syscall.EPERM) {
		return nil
	}
	return err
}

// HealthMain é o modo `eon health` (D53) — usado pelo HEALTHCHECK do
// container e legível a olho nu nos logs.
func HealthMain(args []string) int {
	fs := flag.NewFlagSet("health", flag.ContinueOnError)
	dataDir := fs.String("data", os.Getenv("EON_DATA"), "diretório de dados")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	if *dataDir == "" {
		*dataDir = "/data"
	}
	st, err := LoadState(*dataDir)
	if err != nil {
		fmt.Printf("unhealthy: %v\n", err)
		return 1
	}
	if err := Check(*dataDir); err != nil {
		fmt.Printf("unhealthy: %v\n", err)
		return 1
	}
	fmt.Printf("healthy: uptime=%s core_pid=%d restarts=%d crash_loops=%d sandbox=%s target=v%d shell=%s\n",
		time.Since(st.Boot).Round(time.Second), st.CorePid, st.CoreRestarts,
		st.CrashLoops, st.SandboxLevel, st.TargetVersion, st.ShellVersion)
	return 0
}

// RunSignals wiring para o cmd/eon: ctx de sinais + Run.
func RunSignals(cfg Config) int {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer stop()
	return Run(ctx, cfg)
}
