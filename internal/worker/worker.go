// Package worker implementa L3 (docs/ARCHITECTURE.md): 1 job = 1 processo,
// sandbox cego, protocolo stdio — domain.JobSpec entra pelo stdin,
// domain.JobResult sai pelo stdout.
//
// Cegueira por construção (invariante 6): o job roda sem rede (seccomp e/ou
// netns), sem segredos (env VAZIO no execve), filesystem scratch (chroot com
// só o binário do job). Recusa é resultado legítimo: nível `none` NUNCA
// executa (fail-closed, D09).
//
// Lei (ADR-0002): importa stdlib + domain + sandbox + statefile.
package worker

import (
	"debug/elf"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/brook-sys/eon/internal/domain"
	"github.com/brook-sys/eon/internal/sandbox"
	"github.com/brook-sys/eon/internal/statefile"
)

// Rlimits do job — aplicadas no child ANTES do execve, herdadas pelo job.
// 0 = não aplica. NPROC fica de fora no M0 de propósito: a semântica per-uid
// em host de dev compartilhado pode quebrar forks inocentes; revisitar no M5.
type Rlimits struct {
	CPUSecs    int64 `json:"cpu_secs"`
	ASBytes    int64 `json:"as_bytes"`
	FSizeBytes int64 `json:"fsize_bytes"`
	NoFile     int64 `json:"no_file"`
}

// DefaultRlimits: CPU 10min, AS 1GiB (go build no sandbox depois precisa),
// FSIZE 512MiB, NOFILE 1024.
func DefaultRlimits() Rlimits {
	return Rlimits{CPUSecs: 600, ASBytes: 1 << 30, FSizeBytes: 512 << 20, NoFile: 1024}
}

const (
	jobRootEnv       = "EON_JOB_ROOT"
	jobArgvEnv       = "EON_JOB_ARGV"
	jobLimitsEnv     = "EON_JOB_LIMITS"
	jobSeccompEnv    = "EON_JOB_SECCOMP"
	defaultTimeoutMs = 60_000
)

// ChildMain aplica o sandbox no processo CHAMADOR e faz execve("/job").
// Só é alcançado via re-exec (domain.RoleWorkerChild) — subprocesso
// descartável. Falha antes do exec → 126 com motivo no stderr (capturado no
// arquivo de stderr do job).
func ChildMain() int {
	root := os.Getenv(jobRootEnv)
	var argv []string
	if err := json.Unmarshal([]byte(os.Getenv(jobArgvEnv)), &argv); err != nil {
		fmt.Fprintf(os.Stderr, "worker-child: argv inválido: %v\n", err)
		return 126
	}
	var rl Rlimits
	if err := json.Unmarshal([]byte(os.Getenv(jobLimitsEnv)), &rl); err != nil {
		fmt.Fprintf(os.Stderr, "worker-child: rlimits inválidas: %v\n", err)
		return 126
	}
	if root == "" || len(argv) == 0 {
		fmt.Fprintln(os.Stderr, "worker-child: contrato quebrado (root/argv vazios)")
		return 126
	}
	// Ordem: chroot → cwd → rlimits → seccomp → execve.
	if err := syscall.Chroot(root); err != nil {
		fmt.Fprintf(os.Stderr, "worker-child: chroot: %v\n", err)
		return 126
	}
	if err := syscall.Chdir("/"); err != nil {
		fmt.Fprintf(os.Stderr, "worker-child: chdir: %v\n", err)
		return 126
	}
	if err := applyRlimits(rl); err != nil {
		fmt.Fprintf(os.Stderr, "worker-child: rlimits: %v\n", err)
		return 126
	}
	if os.Getenv(jobSeccompEnv) == "1" {
		prog, err := sandbox.BlockNetworkFilter()
		if err != nil {
			fmt.Fprintf(os.Stderr, "worker-child: %v\n", err)
			return 126
		}
		if err := sandbox.InstallSeccomp(prog); err != nil {
			fmt.Fprintf(os.Stderr, "worker-child: %v\n", err)
			return 126
		}
	}
	// Env VAZIO: zero segredos no job (invariante 6).
	if err := syscall.Exec("/job", argv, []string{}); err != nil {
		fmt.Fprintf(os.Stderr, "worker-child: execve /job: %v\n", err)
		return 126
	}
	return 126 // inalcançável
}

func applyRlimits(rl Rlimits) error {
	type lim struct {
		res      int
		cur, max uint64
	}
	for _, l := range []lim{
		{syscall.RLIMIT_CPU, uint64(rl.CPUSecs), uint64(rl.CPUSecs)},
		{syscall.RLIMIT_AS, uint64(rl.ASBytes), uint64(rl.ASBytes)},
		{syscall.RLIMIT_FSIZE, uint64(rl.FSizeBytes), uint64(rl.FSizeBytes)},
		{syscall.RLIMIT_NOFILE, uint64(rl.NoFile), uint64(rl.NoFile)},
	} {
		if l.cur == 0 {
			continue
		}
		if err := syscall.Setrlimit(l.res, &syscall.Rlimit{Cur: l.cur, Max: l.max}); err != nil {
			return fmt.Errorf("setrlimit(%d): %w", l.res, err)
		}
	}
	return nil
}

// Options configura 1 execução de job.
type Options struct {
	DataDir string
	Level   sandbox.Level
	Limits  Rlimits
}

// Run executa 1 job (L3). O resultado também é gravado em
// <data>/jobs/<id>/result.json — toda job é auditável: comando, timeout,
// resultado (docs/ARCHITECTURE.md).
func Run(spec domain.JobSpec, opts Options) (domain.JobResult, error) {
	result := domain.JobResult{ID: spec.ID, ExitCode: -1, Sandbox: "none"}

	level := opts.Level
	if level == sandbox.LevelAuto {
		level, _ = sandbox.Detect()
	}
	result.Sandbox = level.String()

	refuse := func(reason string) (domain.JobResult, error) {
		result.Refused = true
		result.RefusalReason = reason
		_ = statefile.Write(filepath.Join(opts.DataDir, "jobs", spec.ID, "result.json"), result)
		return result, nil
	}

	if spec.ID == "" {
		return result, errors.New("worker: spec sem id")
	}
	if level == sandbox.LevelNone {
		return refuse("nível de sandbox none — exec recusado fail-closed (D09)")
	}
	if len(spec.Argv) == 0 || spec.Argv[0] == "" {
		return refuse("argv vazio")
	}

	base := filepath.Join(opts.DataDir, "jobs", spec.ID)
	root := filepath.Join(base, "root")
	if err := os.MkdirAll(root, 0o755); err != nil {
		return result, fmt.Errorf("worker: scratch: %w", err)
	}

	// Só binário ESTÁTICO: chroot não tem interpretador dinâmico.
	static, err := isStaticELF(spec.Argv[0])
	if err != nil {
		return refuse(fmt.Sprintf("binário ilegível: %v", err))
	}
	if !static {
		return refuse("binário dinâmico (PT_INTERP) não roda no chroot — compile estático")
	}

	jobPath := filepath.Join(root, "job")
	if err := copyExecutable(spec.Argv[0], jobPath); err != nil {
		return result, fmt.Errorf("worker: copiar job: %w", err)
	}

	stdoutPath := filepath.Join(base, "stdout")
	stderrPath := filepath.Join(base, "stderr")
	outF, err := os.Create(stdoutPath)
	if err != nil {
		return result, err
	}
	defer outF.Close()
	errF, err := os.Create(stderrPath)
	if err != nil {
		return result, err
	}
	defer errF.Close()

	self, err := os.Executable()
	if err != nil {
		return result, err
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return result, err
	}

	// Argv[0] vira /job dentro do chroot; o resto passa intacto.
	argv := append([]string{"/job"}, spec.Argv[1:]...)
	argvJSON, err := json.Marshal(argv)
	if err != nil {
		return result, err
	}
	limitsJSON, err := json.Marshal(opts.Limits)
	if err != nil {
		return result, err
	}

	cmd := exec.Command(self)
	cmd.Env = append(os.Environ(),
		domain.RoleEnv+"="+domain.RoleWorkerChild,
		jobRootEnv+"="+absRoot,
		jobArgvEnv+"="+string(argvJSON),
		jobLimitsEnv+"="+string(limitsJSON),
		jobSeccompEnv+"=1",
	)
	cmd.Stdout = outF
	cmd.Stderr = errF
	attr := &syscall.SysProcAttr{Setsid: true} // pgroup próprio → kill de grupo no timeout
	if level == sandbox.LevelFull {
		// Namespaces: netns isola a rede por construção; userns dá as caps
		// para o child chrootar sem privilégio no host.
		attr.Cloneflags = syscall.CLONE_NEWUSER | syscall.CLONE_NEWNS | syscall.CLONE_NEWNET
		attr.UidMappings = []syscall.SysProcIDMap{{ContainerID: 0, HostID: os.Getuid(), Size: 1}}
		attr.GidMappings = []syscall.SysProcIDMap{{ContainerID: 0, HostID: os.Getgid(), Size: 1}}
		attr.GidMappingsEnableSetgroups = false
	}
	cmd.SysProcAttr = attr

	if err := cmd.Start(); err != nil {
		return result, fmt.Errorf("worker: spawn: %w", err)
	}

	timeout := time.Duration(spec.TimeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = defaultTimeoutMs * time.Millisecond
	}
	var timedOut atomic.Bool
	timer := time.AfterFunc(timeout, func() {
		timedOut.Store(true)
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) // pgroup inteiro
	})

	start := time.Now()
	werr := cmd.Wait()
	timer.Stop()
	result.DurationMs = time.Since(start).Milliseconds()

	switch {
	case timedOut.Load():
		result.TimedOut = true
		result.ExitCode = -1
	case werr == nil:
		result.ExitCode = 0
	default:
		var exitErr *exec.ExitError
		if errors.As(werr, &exitErr) {
			if status, ok := exitErr.Sys().(syscall.WaitStatus); ok && status.Signaled() {
				result.ExitCode = -1
			} else {
				result.ExitCode = exitErr.ExitCode()
			}
		} else {
			result.ExitCode = -1
		}
	}

	result.StdoutFile = filepath.Join("jobs", spec.ID, "stdout")
	result.StderrFile = filepath.Join("jobs", spec.ID, "stderr")
	_ = statefile.Write(filepath.Join(base, "result.json"), result)
	return result, nil
}

// Main é o modo `eon worker`: JobSpec no stdin, JobResult no stdout.
// Erro de PROTOCOLO (stdin ilegível, data dir inacessível) → stderr + exit 1.
// Recusa e exits do JOB são resultado legítimo → exit 0 do worker.
func Main(args []string) int {
	fs := flag.NewFlagSet("worker", flag.ContinueOnError)
	dataDir := fs.String("data", os.Getenv("EON_DATA"), "diretório de dados")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	if *dataDir == "" {
		*dataDir = "/data"
	}

	var spec domain.JobSpec
	dec := json.NewDecoder(io.LimitReader(os.Stdin, 4<<20))
	if err := dec.Decode(&spec); err != nil {
		fmt.Fprintf(os.Stderr, "worker: spec inválida no stdin: %v\n", err)
		return 1
	}
	res, err := Run(spec, Options{DataDir: *dataDir, Level: sandbox.LevelAuto, Limits: DefaultRlimits()})
	if err != nil {
		fmt.Fprintf(os.Stderr, "worker: %v\n", err)
		return 1
	}
	out, err := json.Marshal(res)
	if err != nil {
		fmt.Fprintf(os.Stderr, "worker: %v\n", err)
		return 1
	}
	fmt.Println(string(out))
	return 0
}

func isStaticELF(path string) (bool, error) {
	f, err := elf.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()
	for _, p := range f.Progs {
		if p.Type == elf.PT_INTERP {
			return false, nil
		}
	}
	return true, nil
}

func copyExecutable(src, dst string) error {
	// M0: binários de job são pequenos; leitura inteira é suficiente.
	// M5 (endurecimento): streaming + limite de tamanho.
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, b, 0o755)
}
