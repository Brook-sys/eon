package core

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/brook-sys/eon/internal/statefile"
)

// TestMain despacha o papel de core para os testes do supervisor re-executarem
// ESTE binário como core de verdade (processo real, sinais reais).
func TestMain(m *testing.M) {
	if os.Getenv("EON_TEST_ROLE") == "core-main" {
		os.Exit(Main([]string{"--data", os.Getenv("EON_TEST_DATA")}))
	}
	os.Exit(m.Run())
}

func waitForFile(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("arquivo não apareceu em 5s: %s", path)
}

// TestCoreBootHeartbeatShutdown: boot.json no boot, heartbeat rolando,
// SIGTERM → exit 0 limpo. É o contrato que o supervisor depende para
// distinguir crash de shutdown.
func TestCoreBootHeartbeatShutdown(t *testing.T) {
	dir := t.TempDir()
	cmd := exec.Command(os.Args[0])
	cmd.Env = append(os.Environ(),
		"EON_TEST_ROLE=core-main",
		"EON_TEST_DATA="+dir,
		domainEnvSandbox+"=basic", // simulando injeção do supervisor
	)
	var buf bytes.Buffer
	cmd.Stderr = &buf
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	waitForFile(t, filepath.Join(dir, "core", "boot.json"))
	waitForFile(t, filepath.Join(dir, "core", "heartbeat.json"))

	cmd.Process.Signal(syscall.SIGTERM)
	err := cmd.Wait()
	if err != nil {
		t.Fatalf("core deveria sair 0 no SIGTERM, veio %v (stderr=%q)", err, buf.String())
	}

	var boot bootInfo
	if err := statefile.Read(filepath.Join(dir, "core", "boot.json"), &boot); err != nil {
		t.Fatalf("boot.json: %v", err)
	}
	if boot.SandboxLevel != "basic" {
		t.Errorf("sandbox_level injetada via env deveria ser \"basic\", veio %q", boot.SandboxLevel)
	}
	var hb heartbeatInfo
	if err := statefile.Read(filepath.Join(dir, "core", "heartbeat.json"), &hb); err != nil {
		t.Fatalf("heartbeat.json: %v", err)
	}
	if hb.Beats < 1 || hb.Pid == 0 {
		t.Errorf("heartbeat inválido: %+v", hb)
	}
}

// domainEnvSandbox duplica domain.EnvSandboxLevel — teste white-box do pacote,
// sem import extra (o contrato real vive em internal/domain).
const domainEnvSandbox = "EON_SANDBOX_LEVEL"
