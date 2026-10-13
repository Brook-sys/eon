// Package sandbox implementa o sandbox cego (D09/ADR-0011): exec externa sem
// rede, sem segredos, filesystem scratch — com detecção no boot e degradação
// auditável full → basic → none (fail-closed).
//
// Lei (ADR-0002): importa só stdlib.
//
// Todos os probes e a aplicação do sandbox rodam em SUBPROCESSO (re-exec do
// binário via domain.RoleEnv): seccomp é irreversível e herdado por execve —
// instalar filtro no processo errado mutilaria o chamador. O filho aplica
// chroot → rlimits → seccomp → execve("/job").
package sandbox

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/brook-sys/eon/internal/domain"
)

// Level é o nível de sandbox disponível (D09).
type Level int

const (
	// LevelNone: exec recusado (fail-closed) — sem sandbox não há job.
	LevelNone Level = iota
	// LevelBasic: chroot + seccomp bloqueando rede + rlimits.
	LevelBasic
	// LevelFull: + user/mount/net namespaces (rede isolada por construção).
	LevelFull
)

// LevelAuto instrui o worker a detectar o nível no momento do job.
const LevelAuto Level = -1

func (l Level) String() string {
	switch l {
	case LevelFull:
		return "full"
	case LevelBasic:
		return "basic"
	default:
		return "none"
	}
}

// Report detalha o que foi provado no boot (vai para logs/telemetria).
// UserNS = o clone com namespaces passou (TELEMETRIA — não basta para full).
// Full = a cadeia inteira provada DENTRO do userns: chroot + seccomp + saída
// de rede negada. Incidente do primeiro CI: o runner do GitHub deixa o
// clone+maps passarem e nega o chroot dentro do userns — um probe que só
// provasse o clone reportaria "full" e o worker morreria no primeiro job.
type Report struct {
	UserNS  bool   `json:"userns"`
	Full    bool   `json:"full_proven"`
	Chroot  bool   `json:"chroot"`
	Seccomp bool   `json:"seccomp_net_proven"`
	Level   string `json:"level"`
}

// Detect determina o nível disponível — cada probe é um subprocesso que
// PROVA a capacidade que o nível realmente usa (D09: nível nunca é assumido):
//
//	Full  := cadeia inteira provada dentro do userns: chroot + seccomp +
//	         saída de rede negada (netns isola por construção)
//	Basic := chroot direto + seccomp provado
//	None  := todo o resto — exec recusado, fail-closed
func Detect() (Level, Report) {
	detectOnce.Do(func() {
		rep := Report{
			UserNS:  probeUserNS(),
			Full:    probeFullChain(),
			Chroot:  probeChroot(),
			Seccomp: probeSeccompNet(),
		}
		switch {
		case rep.Full:
			detectedLevel, detectedReport = LevelFull, rep
		case rep.Chroot && rep.Seccomp:
			detectedLevel, detectedReport = LevelBasic, rep
		default:
			detectedLevel, detectedReport = LevelNone, rep
		}
		detectedReport.Level = detectedLevel.String()
	})
	return detectedLevel, detectedReport
}

var (
	detectOnce     sync.Once
	detectedLevel  Level
	detectedReport Report
)

// ProbeMain executa o papel de probe indicado por EON_SANDBOX_PROBE.
// Dispatched pelo cmd/eon (produção) e pelo TestMain (testes) — roda sempre
// em subprocesso, nunca no processo chamador.
func ProbeMain() int {
	switch os.Getenv(probeKindEnv) {
	case probeUserNSKind:
		return 0 // o Cloneflags do pai é a prova: se spawnou, userns funciona
	case probeChrootKind:
		dir := os.Getenv(probeDirEnv)
		if err := syscall.Chroot(dir); err != nil {
			return 1
		}
		if err := syscall.Chdir("/"); err != nil {
			return 1
		}
		return 0
	case probeSeccompKind:
		prog, err := BlockNetworkFilter()
		if err != nil {
			return 1
		}
		if err := InstallSeccomp(prog); err != nil {
			return 1
		}
		// A prova: com o filtro ativo, discar TEM que falhar com EPERM.
		_, err = net.DialTimeout("tcp", "127.0.0.1:1", 2*time.Second)
		if err == nil {
			return 1 // rede aberta: filtro inútil
		}
		if errors.Is(err, os.ErrPermission) || errors.Is(err, syscall.EPERM) ||
			strings.Contains(err.Error(), "operation not permitted") ||
			strings.Contains(err.Error(), "permission denied") {
			return 0
		}
		return 1
	case probeFullKind:
		// A cadeia inteira do nível full, DENTRO do userns — o que o
		// worker-child vai fazer de verdade. Caps capturadas ANTES do
		// chroot (depois, /proc não existe no scratch).
		status := procStatusCaps()
		dir := os.Getenv(probeDirEnv)
		if err := syscall.Chroot(dir); err != nil {
			fmt.Fprintf(os.Stderr, "sandbox-probe full-chain: chroot: %v\n%s", err, status)
			return 1
		}
		if err := syscall.Chdir("/"); err != nil {
			fmt.Fprintf(os.Stderr, "sandbox-probe full-chain: chdir: %v\n", err)
			return 1
		}
		prog, err := BlockNetworkFilter()
		if err != nil {
			fmt.Fprintf(os.Stderr, "sandbox-probe full-chain: filtro: %v\n", err)
			return 1
		}
		if err := InstallSeccomp(prog); err != nil {
			fmt.Fprintf(os.Stderr, "sandbox-probe full-chain: seccomp: %v\n%s", err, status)
			return 1
		}
		// Rede no nível full: EPERM do filtro OU netns sem interfaces —
		// discar TEM que falhar; sucesso = sandbox furado, probe reprova.
		if _, err := net.DialTimeout("tcp", "127.0.0.1:1", 2*time.Second); err == nil {
			fmt.Fprintln(os.Stderr, "sandbox-probe full-chain: rede acessível dentro do sandbox — reprova")
			return 1
		}
		return 0
	}
	return 2
}

const (
	probeKindEnv     = "EON_SANDBOX_PROBE"
	probeDirEnv      = "EON_SANDBOX_PROBE_DIR"
	probeUserNSKind  = "userns"
	probeFullKind    = "full-chain"
	probeChrootKind  = "chroot"
	probeSeccompKind = "seccomp-net"
)

// probeRun re-executa o binário atual no papel de probe. Saída descartada —
// o código de saída é a resposta.
func probeRun(kind string, extraEnv []string, attr *syscall.SysProcAttr) bool {
	self, err := os.Executable()
	if err != nil {
		return false
	}
	cmd := exec.Command(self)
	cmd.Env = append(os.Environ(),
		domain.RoleEnv+"="+domain.RoleProbe,
		probeKindEnv+"="+kind)
	cmd.Env = append(cmd.Env, extraEnv...)
	cmd.SysProcAttr = attr
	return cmd.Run() == nil
}

// usernsAttr é o spawn do nível full: userns dá as caps ao child dentro do
// novo namespace; NEWNS dá um mount namespace próprio; NEWNET isola a rede
// por construção. HostID → ContainerID 0: o child é root NO seu namespace.
func usernsAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		// clone com CLONE_NEWUSER não tem a restrição single-thread do
		// unshare — o filho ganha caps no novo userns (rootless pattern).
		Cloneflags: syscall.CLONE_NEWUSER | syscall.CLONE_NEWNS | syscall.CLONE_NEWNET,
		UidMappings: []syscall.SysProcIDMap{{
			ContainerID: 0, HostID: os.Getuid(), Size: 1,
		}},
		GidMappings: []syscall.SysProcIDMap{{
			ContainerID: 0, HostID: os.Getgid(), Size: 1,
		}},
		GidMappingsEnableSetgroups: false,
	}
}

// probeUserNS prova SÓ o clone com namespaces (telemetria do report —
// sozinho não autoriza nível nenhum: ver probeFullChain).
func probeUserNS() bool {
	return probeRun(probeUserNSKind, nil, usernsAttr())
}

// probeFullChain prova a CADEIA que o worker-child executa no nível full,
// dentro do userns: chroot no scratch → seccomp → saída de rede negada.
// Um clone bem-sucedido não basta (D09: provar o que se usa, não um
// correlato) — foi exatamente assim que o primeiro CI pegou o runner do
// GitHub: clone+maps passam, chroot dentro do userns é negado (EPERM).
func probeFullChain() bool {
	dir, err := os.MkdirTemp("", "eon-full-probe")
	if err != nil {
		return false
	}
	defer os.RemoveAll(dir)
	return probeRun(probeFullKind, []string{probeDirEnv + "=" + dir}, usernsAttr())
}

// procStatusCaps captura as linhas de caps/seccomp/apparmor do processo —
// diagnóstico de ops: quando um host deixa o clone passar e nega o chroot,
// o motivo aparece aqui no stderr do probe.
func procStatusCaps() string {
	b, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return ""
	}
	var sb strings.Builder
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(line, "Cap") || strings.HasPrefix(line, "Seccomp") ||
			strings.HasPrefix(line, "NoNewPrivs") || strings.HasPrefix(line, "apparmor") {
			sb.WriteString(line + "\n")
		}
	}
	return sb.String()
}

func probeChroot() bool {
	dir, err := os.MkdirTemp("", "eon-chroot-probe")
	if err != nil {
		return false
	}
	defer os.RemoveAll(dir)
	return probeRun(probeChrootKind, []string{probeDirEnv + "=" + dir}, nil)
}

func probeSeccompNet() bool {
	return probeRun(probeSeccompKind, nil, nil)
}

// ReportJSON serializa o report (para linhas de log do boot).
func ReportJSON(r Report) string {
	b, err := json.Marshal(r)
	if err != nil {
		return "{}"
	}
	return string(b)
}
