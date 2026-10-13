package worker

import (
	"bytes"
	"debug/elf"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/brook-sys/eon/internal/domain"
	"github.com/brook-sys/eon/internal/sandbox"
)

// TestMain despacha TODOS os papéis que este pacote re-executa:
//   - sandbox-probe  → probes do Detect() (worker chama Detect)
//   - worker-child   → aplicador de sandbox + execve do job
//   - worker-main    → protocolo stdio completo (para o teste end-to-end)
//   - -job-role X    → o JOB em si (este binário dentro do chroot, env vazio)
func TestMain(m *testing.M) {
	switch role := os.Getenv(domain.RoleEnv); role {
	case "":
	case domain.RoleProbe:
		os.Exit(sandbox.ProbeMain())
	case domain.RoleWorkerChild:
		os.Exit(ChildMain())
	case "worker-main":
		os.Exit(Main([]string{"--data", os.Getenv("EON_TEST_DATA")}))
	default:
		os.Exit(2)
	}
	if r := jobRoleFromArgs(); r != "" {
		os.Exit(runJobRole(r))
	}
	os.Exit(m.Run())
}

func jobRoleFromArgs() string {
	for i, a := range os.Args {
		if a == "-job-role" && i+1 < len(os.Args) {
			return os.Args[i+1]
		}
	}
	return ""
}

// runJobRole: o job executado DENTRO do sandbox. Env é vazio; argv carrega o
// papel. Cada papel imprime um marcador no stdout — o teste lê o arquivo de
// captura e a prova é o marcador.
func runJobRole(role string) int {
	switch role {
	case "dial":
		_, err := net.DialTimeout("tcp", "127.0.0.1:1", 2*time.Second)
		if err != nil {
			fmt.Printf("DIAL-ERR: %v\n", err) // a prova: rede bloqueada
			return 0                          // o job RODOU — sair 0 é certo
		}
		fmt.Println("DIAL-OK: rede ABERTA — VIOLAÇÃO")
		return 0
	case "sleep":
		time.Sleep(30 * time.Second)
		return 0
	case "touch":
		if err := os.WriteFile("touched", []byte("ok"), 0o644); err != nil {
			fmt.Printf("TOUCH-ERR: %v\n", err)
			return 1
		}
		fmt.Println("TOUCH-OK")
		return 0
	case "unixsock":
		l, err := net.Listen("unix", "s.sock")
		if err != nil {
			fmt.Printf("UNIX-ERR: %v\n", err)
			return 1
		}
		fmt.Println("UNIX-OK")
		l.Close()
		return 0
	}
	return 2
}

// --- helpers ---

func skipIfNoSandbox(t *testing.T) sandbox.Level {
	t.Helper()
	lvl, rep := sandbox.Detect()
	if lvl == sandbox.LevelNone {
		t.Skipf("sem sandbox neste ambiente (report=%s) — exec é fail-closed; prová-lo requer caps/userns (CI roda em container root, homelab tem userns)", sandbox.ReportJSON(rep))
	}
	return lvl
}

func skipIfNotStatic(t *testing.T) {
	t.Helper()
	f, err := elf.Open(os.Args[0])
	if err != nil {
		t.Skipf("não consegui inspecionar o binário de teste: %v", err)
	}
	defer f.Close()
	for _, p := range f.Progs {
		if p.Type == elf.PT_INTERP {
			t.Skip("binário de teste é dinâmico — rode o gate com CGO_ENABLED=0 (skill eon-verify)")
		}
	}
}

func readCapture(t *testing.T, dir, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, rel))
	if err != nil {
		t.Fatalf("captura %s: %v", rel, err)
	}
	return string(b)
}

func runJob(t *testing.T, dir, id string, argv []string, timeoutMs int64) domain.JobResult {
	t.Helper()
	res, err := Run(domain.JobSpec{ID: id, Argv: argv, TimeoutMs: timeoutMs},
		Options{DataDir: dir, Level: sandbox.LevelAuto, Limits: DefaultRlimits()})
	if err != nil {
		t.Fatalf("worker.Run: %v", err)
	}
	return res
}

// --- testes ---

// TestWorkerRefusesWhenLevelNone (aceite M0): nível `none` → exec recusado
// fail-closed. Roda em QUALQUER ambiente — a recusa é a garantia.
func TestWorkerRefusesWhenLevelNone(t *testing.T) {
	dir := t.TempDir()
	res, err := Run(domain.JobSpec{ID: "refused", Argv: []string{os.Args[0], "-job-role", "touch"}, TimeoutMs: 5000},
		Options{DataDir: dir, Level: sandbox.LevelNone, Limits: DefaultRlimits()})
	if err != nil {
		t.Fatalf("worker.Run: %v", err)
	}
	if !res.Refused {
		t.Fatalf("esperava recusa fail-closed, veio %+v", res)
	}
	if res.ExitCode != -1 {
		t.Errorf("exit code de recusa deveria ser -1, veio %d", res.ExitCode)
	}
	if res.Sandbox != "none" {
		t.Errorf("sandbox reportado %q, queria none", res.Sandbox)
	}
}

// TestWorkerExecutesJobAndBlocksNetwork (aceite M0): o job RODA no sandbox e
// a rede está bloqueada COMPROVADA (a discagem falha dentro do chroot).
func TestWorkerExecutesJobAndBlocksNetwork(t *testing.T) {
	lvl := skipIfNoSandbox(t)
	skipIfNotStatic(t)
	dir := t.TempDir()

	res := runJob(t, dir, "net", []string{os.Args[0], "-job-role", "dial"}, 15_000)
	if res.Refused {
		t.Fatalf("job legítimo recusado: %+v", res)
	}
	if res.ExitCode != 0 {
		t.Fatalf("job deveria rodar e sair 0, veio %d (stderr: %q)", res.ExitCode, readCapture(t, dir, res.StderrFile))
	}
	out := readCapture(t, dir, res.StdoutFile)
	if strings.Contains(out, "DIAL-OK") || !strings.Contains(out, "DIAL-ERR") {
		t.Fatalf("rede deveria estar bloqueada no sandbox %s; stdout=%q", res.Sandbox, out)
	}
	if res.Sandbox != lvl.String() {
		t.Errorf("sandbox reportado %q diverge do detectado %q", res.Sandbox, lvl)
	}
}

// TestWorkerTimeoutKillsJob (aceite M0): timeout → SIGKILL no pgroup do job.
func TestWorkerTimeoutKillsJob(t *testing.T) {
	skipIfNoSandbox(t)
	skipIfNotStatic(t)
	dir := t.TempDir()

	res := runJob(t, dir, "slow", []string{os.Args[0], "-job-role", "sleep"}, 400)
	if !res.TimedOut {
		t.Fatalf("esperava timeout, veio %+v", res)
	}
	if res.ExitCode != -1 {
		t.Errorf("job morto por sinal deveria ter exit -1, veio %d", res.ExitCode)
	}
	if res.DurationMs > 5_000 {
		t.Errorf("kill demorou demais: %dms", res.DurationMs)
	}
}

// TestWorkerScratchFS: o job enxerga SÓ o filesystem scratch (cwd = / do
// chroot) — prova do isolamento de filesystem.
func TestWorkerScratchFS(t *testing.T) {
	skipIfNoSandbox(t)
	skipIfNotStatic(t)
	dir := t.TempDir()

	res := runJob(t, dir, "touch", []string{os.Args[0], "-job-role", "touch"}, 15_000)
	if res.Refused || res.ExitCode != 0 {
		t.Fatalf("job touch: %+v (stderr=%q)", res, readCapture(t, dir, res.StderrFile))
	}
	b, err := os.ReadFile(filepath.Join(dir, "jobs", "touch", "root", "touched"))
	if err != nil {
		t.Fatalf("o job deveria ter escrito no scratch: %v", err)
	}
	if string(b) != "ok" {
		t.Errorf("conteúdo inesperado: %q", b)
	}
}

// TestWorkerUnixSocketStillWorks: AF_UNIX continua permitido (o filtro não
// bloqueia além da conta — pipes e sockets locais do próprio job vivos).
func TestWorkerUnixSocketStillWorks(t *testing.T) {
	skipIfNoSandbox(t)
	skipIfNotStatic(t)
	dir := t.TempDir()

	res := runJob(t, dir, "unix", []string{os.Args[0], "-job-role", "unixsock"}, 15_000)
	if res.Refused || res.ExitCode != 0 {
		t.Fatalf("job unixsock: %+v (stderr=%q)", res, readCapture(t, dir, res.StderrFile))
	}
	if out := readCapture(t, dir, res.StdoutFile); !strings.Contains(out, "UNIX-OK") {
		t.Errorf("AF_UNIX deveria funcionar no sandbox; stdout=%q", out)
	}
}

// TestWorkerRefusesDynamicBinary: binário dinâmico é recusado ANTES de
// rodar — chroot não tem interpretador.
func TestWorkerRefusesDynamicBinary(t *testing.T) {
	skipIfNoSandbox(t)
	dir := t.TempDir()

	gcc, err := exec.LookPath("gcc")
	if err != nil {
		t.Skip("sem gcc para fabricar binário dinâmico de teste")
	}
	src := filepath.Join(dir, "hello.c")
	bin := filepath.Join(dir, "hello")
	if err := os.WriteFile(src, []byte("int main(){return 0;}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(gcc, "-o", bin, src).CombinedOutput(); err != nil {
		t.Fatalf("gcc: %v (%s)", err, out)
	}

	res := runJob(t, dir, "dyn", []string{bin}, 15_000)
	if !res.Refused {
		t.Fatalf("binário dinâmico deveria ser recusado, veio %+v", res)
	}
	if !strings.Contains(strings.ToLower(res.RefusalReason), "din") {
		t.Errorf("motivo inesperado: %q", res.RefusalReason)
	}
}

// TestWorkerStdioProtocolEndToEnd: o protocolo stdio inteiro — spec JSON no
// stdin de um processo worker, resultado JSON no stdout.
func TestWorkerStdioProtocolEndToEnd(t *testing.T) {
	skipIfNoSandbox(t)
	skipIfNotStatic(t)
	dir := t.TempDir()

	spec := domain.JobSpec{ID: "proto", Argv: []string{os.Args[0], "-job-role", "touch"}, TimeoutMs: 15_000}
	specJSON, err := json.Marshal(spec)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0])
	cmd.Env = append(os.Environ(),
		domain.RoleEnv+"=worker-main",
		"EON_TEST_DATA="+dir)
	cmd.Stdin = bytes.NewReader(specJSON)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		t.Fatalf("worker-main: %v", err)
	}
	var res domain.JobResult
	if err := json.Unmarshal(stdout.Bytes(), &res); err != nil {
		t.Fatalf("protocolo stdio: %v (stdout=%q)", err, stdout.String())
	}
	if res.ID != "proto" || res.Refused || res.ExitCode != 0 {
		t.Fatalf("resultado: %+v", res)
	}
}
