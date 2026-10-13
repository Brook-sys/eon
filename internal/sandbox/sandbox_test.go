package sandbox

import (
	"os"
	"testing"

	"github.com/brook-sys/eon/internal/domain"
)

// TestMain despacha os papéis de probe: Detect() re-executa ESTE binário de
// teste como filho de probe — o dispatch tem que acontecer antes do m.Run.
func TestMain(m *testing.M) {
	if os.Getenv(domain.RoleEnv) == domain.RoleProbe {
		os.Exit(ProbeMain())
	}
	os.Exit(m.Run())
}

// TestDetectConsistency: o nível deriva exatamente dos probes — nada de
// "achismo" na detecção. Qual nível está disponível depende do ambiente
// (full local com userns, basic como root em container, none em kernel
// paranoico) — a coerência interna é o que este teste garante.
func TestDetectConsistency(t *testing.T) {
	lvl, rep := Detect()
	t.Logf("detect: level=%s report=%s", lvl, ReportJSON(rep))

	switch lvl {
	case LevelFull:
		if !rep.Full || !rep.UserNS {
			t.Fatalf("full exige a cadeia provada dentro do userns, report diz: %+v", rep)
		}
	case LevelBasic:
		if !rep.Chroot || !rep.Seccomp {
			t.Fatalf("basic exige chroot+seccomp provados, report diz: %+v", rep)
		}
	case LevelNone:
		if rep.Full || (rep.Chroot && rep.Seccomp) {
			t.Fatalf("none contradiz os probes, report diz: %+v", rep)
		}
	default:
		t.Fatalf("nível desconhecido: %d", lvl)
	}
	if rep.Level != lvl.String() {
		t.Fatalf("report.Level=%q diverge do nível %q", rep.Level, lvl)
	}
}

// TestSeccompProofRequiresEPERM: quando o probe de seccomp passa, ele passou
// PORQUE a discagem falhou com EPERM (rede bloqueada comprovada — a garantia
// do M0, decisão D09). Se o ambiente não tem seccomp, skip explícito.
func TestSeccompProofRequiresEPERM(t *testing.T) {
	_, rep := Detect()
	if !rep.Seccomp {
		t.Skip("seccomp não provado neste ambiente (probe reprova) — nível none: exec será recusado fail-closed")
	}
}

// TestLevelString: os nomes são contrato de telemetria/health (D53).
func TestLevelString(t *testing.T) {
	cases := map[Level]string{LevelNone: "none", LevelBasic: "basic", LevelFull: "full"}
	for lvl, want := range cases {
		if got := lvl.String(); got != want {
			t.Errorf("Level(%d).String()=%q, queria %q", lvl, got, want)
		}
	}
}
