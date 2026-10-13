package sandbox

// seccomp.go — filtro BPF hand-rolled, puro stdlib (D19: zero dependências;
// D09: rede bloqueada comprovada por probe, não assumida).
//
// O programa bloqueia socket(AF_INET) e socket(AF_INET6) com ERRNO|EPERM e
// permite todo o resto. Arquitetura desconhecida → KILL_PROCESS (fail-closed
// de kernel, não de software). AF_UNIX segue permitido (pipes/sockets locais
// do próprio job).
//
// Layout do programa (11 instruções — saltos contados à mão; MUDOU, RE-CONTA):
//
//	 0: ld    arch
//	 1: jeq   AUDIT_ARCH   ok→2   bad→10 (KILL)
//	 2: ld    nr
//	 3: jeq   SYS_SOCKET   sim→4  não→9 (ALLOW)
//	 4: ld    arg0 (domínio)
//	 5: jeq   AF_INET      sim→6  não→7
//	 6: ret   ERRNO|EPERM
//	 7: jeq   AF_INET6     sim→8  não→9 (ALLOW)
//	 8: ret   ERRNO|EPERM
//	 9: ret   ALLOW
//	10: ret   KILL_PROCESS

import (
	"fmt"
	"runtime"
	"syscall"
	"unsafe"
)

// Constantes BPF/seccomp (linux/filter.h, linux/seccomp.h, linux/audit.h).
const (
	bpfLD  = 0x00
	bpfW   = 0x00
	bpfABS = 0x20
	bpfJMP = 0x05
	bpfJEQ = 0x10
	bpfK   = 0x00
	bpfRET = 0x06

	seccompRetAllow       = 0x7FFF0000
	seccompRetErrno       = 0x00050000
	seccompRetKillProcess = 0x80000000

	prSetNoNewPrivs   = 38
	prSetSeccomp      = 22
	seccompModeFilter = 2

	afINET  = 2
	afINET6 = 10

	// seccomp_data: nr(0..3), arch(4..7), args[0](16..23).
	offNr   uint32 = 0
	offArch uint32 = 4
	offArg0 uint32 = 16
)

func auditArch() (uint32, error) {
	switch runtime.GOARCH {
	case "amd64":
		return 0xC000003E, nil // AUDIT_ARCH_X86_64
	case "arm64":
		return 0xC00000B7, nil // AUDIT_ARCH_AARCH64
	default:
		return 0, fmt.Errorf("sandbox: arquitetura sem filtro seccomp suportado: %s", runtime.GOARCH)
	}
}

func sysSocketNr() (uint32, error) {
	switch runtime.GOARCH {
	case "amd64":
		return 41, nil
	case "arm64":
		return 198, nil
	default:
		return 0, fmt.Errorf("sandbox: sem número de socket(2) para %s", runtime.GOARCH)
	}
}

func fOp(code uint16, k uint32) syscall.SockFilter {
	return syscall.SockFilter{Code: code, Jt: 0, Jf: 0, K: k}
}

func fJmp(k uint32, jt, jf uint8) syscall.SockFilter {
	return syscall.SockFilter{Code: bpfJMP | bpfJEQ | bpfK, Jt: jt, Jf: jf, K: k}
}

func fRet(k uint32) syscall.SockFilter {
	return syscall.SockFilter{Code: bpfRET | bpfK, K: k}
}

// BlockNetworkFilter devolve o programa BPF que bloqueia rede AF_INET/6.
// Exportado para o worker-child (internal/worker) — o único outro lugar com
// direito legal de instalar sandbox (ADR-0002).
func BlockNetworkFilter() ([]syscall.SockFilter, error) {
	arch, err := auditArch()
	if err != nil {
		return nil, err
	}
	sockNr, err := sysSocketNr()
	if err != nil {
		return nil, err
	}
	return []syscall.SockFilter{
		fOp(bpfLD|bpfW|bpfABS, offArch),               // 0
		fJmp(arch, 0, 8),                              // 1: não→10 KILL
		fOp(bpfLD|bpfW|bpfABS, offNr),                 // 2
		fJmp(sockNr, 0, 5),                            // 3: não→9 ALLOW
		fOp(bpfLD|bpfW|bpfABS, offArg0),               // 4
		fJmp(afINET, 0, 1),                            // 5: sim→6, não→7
		fRet(seccompRetErrno | uint32(syscall.EPERM)), // 6
		fJmp(afINET6, 0, 1),                           // 7: sim→8, não→9
		fRet(seccompRetErrno | uint32(syscall.EPERM)), // 8
		fRet(seccompRetAllow),                         // 9
		fRet(seccompRetKillProcess),                   // 10
	}, nil
}

// installSeccomp... → InstallSeccomp instala o filtro NO PROCESSO CHAMADOR.
// Irreversível e herdado por execve — por isso só é chamado em subprocesso
// (probe ou worker-child), nunca no core/supervisor/test runner.
func InstallSeccomp(prog []syscall.SockFilter) error {
	// NO_NEW_PRIVS antes do filtro: exigido para instalar sem privilégio e
	// impede execve de binários setuid dentro do scratch.
	if err := prctl(prSetNoNewPrivs, 1); err != nil {
		return fmt.Errorf("sandbox: prctl NO_NEW_PRIVS: %w", err)
	}
	fp := &syscall.SockFprog{Len: uint16(len(prog)), Filter: &prog[0]}
	// PR_SET_SECCOMP: arg2 = MODO (SECCOMP_MODE_FILTER), arg3 = ponteiro do
	// sock_fprog. (Bug histórico do M0: passar o ponteiro em arg2 → kernel
	// interpreta o ponteiro como modo → EINVAL. Descoberto por bisseção
	// empírica — Constituição Art. 7: reprodução primeiro.)
	if err := prctl(prSetSeccomp, seccompModeFilter, uintptr(unsafe.Pointer(fp))); err != nil {
		return fmt.Errorf("sandbox: prctl SECCOMP_MODE_FILTER: %w", err)
	}
	return nil
}

// prctl: option + até 4 argumentos nas posições arg2..arg5.
func prctl(option uintptr, args ...uintptr) error {
	var a2, a3, a4, a5 uintptr
	switch len(args) {
	case 4:
		a5 = args[3]
		fallthrough
	case 3:
		a4 = args[2]
		fallthrough
	case 2:
		a3 = args[1]
		fallthrough
	case 1:
		a2 = args[0]
	}
	_, _, errno := syscall.Syscall6(syscall.SYS_PRCTL, option, a2, a3, a4, a5, 0)
	if errno != 0 {
		return errno
	}
	return nil
}
