package supervisor

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"
)

// TestMain despacha os papéis de core FALSO: os testes re-executam ESTE
// binário de teste como core de verdade (processo real, exit codes reais).
func TestMain(m *testing.M) {
	switch os.Getenv("EON_TEST_ROLE") {
	case "":
		os.Exit(m.Run())
	case "core-sleep":
		c := make(chan os.Signal, 1)
		signal.Notify(c, syscall.SIGTERM, syscall.SIGINT)
		<-c
		os.Exit(0)
	case "core-crash":
		os.Exit(1)
	case "core-crash-slow":
		time.Sleep(150 * time.Millisecond)
		os.Exit(1)
	}
	os.Exit(2)
}

// --- helpers ---

func writeScript(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func plantArtifact(t *testing.T, dataDir string, version int, body string) {
	t.Helper()
	vdir := filepath.Join(dataDir, "artifacts", "eon", "v"+strconv.Itoa(version))
	if err := os.MkdirAll(vdir, 0o755); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(vdir, "eon")
	script := "#!/bin/sh\n" + body
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(vdir, "eon.sha256"), []byte(sha256Hex(t, script)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func sha256Hex(t *testing.T, s string) string {
	t.Helper()
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func waitFor(t *testing.T, timeout time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("%s: condição não satisfeita em %v", what, timeout)
}

func stateOf(t *testing.T, dataDir string) (healthState, bool) {
	t.Helper()
	st, err := LoadState(dataDir)
	if err != nil {
		return healthState{}, false
	}
	return st, true
}

func fastCfg(dir, self string) Config {
	return Config{
		DataDir:        dir,
		SelfBin:        self,
		CoreArgv:       nil,
		BaseBackoff:    10 * time.Millisecond,
		MaxBackoff:     25 * time.Millisecond,
		CrashThreshold: 1000, // não queremos demote nos testes de revival
		CrashWindow:    time.Hour,
		HealthInterval: 25 * time.Millisecond,
	}
}

// --- testes ---

// TestRunRevivesCrashingCore (aceite M0): core que crasha é ressuscitado com
// backoff — o supervisor nunca desiste.
func TestRunRevivesCrashingCore(t *testing.T) {
	dir := t.TempDir()
	crasher := writeScript(t, dir, "crasher.sh", "exit 1\n")
	cfg := fastCfg(dir, crasher)
	cfg.CrashThreshold = 1000

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan int, 1)
	go func() { done <- Run(ctx, cfg) }()

	waitFor(t, 3*time.Second, "restarts >= 2", func() bool {
		st, ok := stateOf(t, dir)
		return ok && st.CoreRestarts >= 2
	})

	cancel()
	select {
	case code := <-done:
		if code != 0 {
			t.Fatalf("Run deveria retornar 0 no shutdown, veio %d", code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run não terminou após cancel")
	}
}

// TestRunRevivesKilledCore (aceite M0): kill -9 no core → supervisor spawna
// outro core.
func TestRunRevivesKilledCore(t *testing.T) {
	dir := t.TempDir()
	sleeper := writeScript(t, dir, "sleeper.sh", "sleep 30\n")
	cfg := fastCfg(dir, sleeper)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan int, 1)
	go func() { done <- Run(ctx, cfg) }()

	// espera o primeiro core
	waitFor(t, 3*time.Second, "core rodando", func() bool {
		st, ok := stateOf(t, dir)
		return ok && st.CorePid > 0
	})
	st, _ := stateOf(t, dir)
	pid1 := st.CorePid

	// mata o core à força
	if err := syscall.Kill(pid1, syscall.SIGKILL); err != nil {
		t.Fatalf("kill: %v", err)
	}

	// supervisor ressuscita com um NOVO pid
	waitFor(t, 3*time.Second, "core novo", func() bool {
		st, ok := stateOf(t, dir)
		return ok && st.CorePid > 0 && st.CorePid != pid1
	})

	cancel()
	select {
	case code := <-done:
		if code != 0 {
			t.Fatalf("Run: %d", code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run não terminou após cancel")
	}
}

// TestCrashLoopDemotesToLastGoodBinary (aceite M0): crash-loop no binário
// promovido mais novo → re-promove o último binário bom (D41).
func TestCrashLoopDemotesToLastGoodBinary(t *testing.T) {
	dir := t.TempDir()
	plantArtifact(t, dir, 1, "sleep 30\n")               // bom: fica vivo
	plantArtifact(t, dir, 2, "exit 1\n")                 // ruim: crasha
	shell := writeScript(t, dir, "shell.sh", "exit 1\n") // casca também ruim — não deve ser escolhida

	cfg := fastCfg(dir, shell)
	cfg.CrashThreshold = 3

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan int, 1)
	go func() { done <- Run(ctx, cfg) }()

	// target inicial = v2 (mais novo); crash-loop → v1 (bom)
	waitFor(t, 5*time.Second, "demote v2→v1", func() bool {
		st, ok := stateOf(t, dir)
		return ok && st.TargetVersion == 1
	})

	st, _ := stateOf(t, dir)
	if st.CrashLoops < 1 {
		t.Errorf("crash_loops deveria ser >= 1, veio %d", st.CrashLoops)
	}
	if st.LastIncident == "" {
		t.Error("incidente deveria estar registrado")
	}
	// o core (v1, sleep 30) está vivo de verdade
	waitFor(t, 3*time.Second, "core v1 vivo", func() bool {
		st, ok := stateOf(t, dir)
		return ok && st.CorePid > 0
	})

	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run não terminou após cancel")
	}
}

// TestHealthLifecycle: health.json reflete supervisor+core vivos; após
// shutdown, Check reprova.
func TestHealthLifecycle(t *testing.T) {
	dir := t.TempDir()
	cfg := fastCfg(dir, os.Args[0]) // core = este binário de teste
	cfg.CoreEnv = []string{"EON_TEST_ROLE=core-sleep"}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan int, 1)
	go func() { done <- Run(ctx, cfg) }()

	waitFor(t, 3*time.Second, "health OK", func() bool {
		return Check(dir) == nil
	})
	st, _ := stateOf(t, dir)
	if st.CorePid <= 0 || st.SupervisorPid <= 0 {
		t.Fatalf("pids ausentes: %+v", st)
	}

	cancel()
	select {
	case code := <-done:
		if code != 0 {
			t.Fatalf("Run: %d", code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run não terminou após cancel")
	}
	if err := Check(dir); err == nil {
		t.Fatal("Check deveria reprovar com supervisor morto")
	}
}

// TestSelectTargetPrefersNewestValid: artefato com SHA corrompido é
// ignorado; o mais novo VÁLIDO vence; demote cai para o inferior válido.
func TestSelectTargetPrefersNewestValid(t *testing.T) {
	dir := t.TempDir()
	plantArtifact(t, dir, 1, "sleep 30\n")
	plantArtifact(t, dir, 2, "sleep 30\n")
	plantArtifact(t, dir, 3, "sleep 30\n")
	// corrompe o sha do v3 (D41: verificação pre-swap rejeita)
	if err := os.WriteFile(filepath.Join(dir, "artifacts", "eon", "v3", "eon.sha256"),
		[]byte("deadbeef\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	tgt := selectTarget(dir, "/nope")
	if !tgt.isArtifact || tgt.version != 2 {
		t.Fatalf("esperava v2 válido, veio %+v", tgt)
	}

	cfg := fastCfg(dir, "/nope")
	next, changed := demote(cfg, tgt)
	if !changed || next.version != 1 {
		t.Fatalf("demote v2 deveria cair para v1, veio %+v (changed=%v)", next, changed)
	}
	next, changed = demote(cfg, next)
	if !changed || next.isArtifact || next.path != "/nope" {
		t.Fatalf("demote v1 deveria cair para a casca, veio %+v", next)
	}
	next, changed = demote(cfg, next)
	if changed {
		t.Fatalf("demote da casca não tem para onde cair, veio %+v (changed=%v)", next, changed)
	}
}

// TestDiscoverArtifactsRejectsBrokenBinary: binário promovido truncado
// (sha não bate) nunca é escolhido.
func TestDiscoverArtifactsRejectsBrokenBinary(t *testing.T) {
	dir := t.TempDir()
	plantArtifact(t, dir, 5, "sleep 30\n")
	// trunca o binário pós-promoção
	bin := filepath.Join(dir, "artifacts", "eon", "v5", "eon")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nsl"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := discoverArtifacts(dir); len(got) != 0 {
		t.Fatalf("binário truncado deveria ser rejeitado, veio %d artefatos", len(got))
	}
}
