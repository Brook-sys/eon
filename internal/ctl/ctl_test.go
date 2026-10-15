// ctl_test.go — a janela do operador lê o que o kernel escreveu.
// Fixtures montadas PELO STORAGE (ctl não sabe escrever — leitor por
// construção); a bateria de integração em cmd/eon prova o casamento
// kernel-vivo × ctl-vivo (WAL, duas conexões).
package ctl

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/brook-sys/eon/internal/domain"
	"github.com/brook-sys/eon/internal/storage"
)

// fixtureDB: o mínimo que o kernel M1 escreve — missão, task READY,
// events de criação/transição, contador de ciclos — montado pelo
// storage de verdade e lido pela ctl de verdade.
func fixtureDB(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "eon.db")
	s, err := storage.NewStore(path)
	if err != nil {
		t.Fatalf("storage: %v", err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	ctx := context.Background()
	u, err := s.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	m := &domain.Mission{ID: "m-stub", Title: "Missão stub M1", Purpose: "provar continuidade",
		Status: domain.MissionActive, CreatedAt: now, UpdatedAt: now}
	if err := s.Missions().Put(u, m); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Events().Append(u, "mission", m.ID, domain.EventMissionCreated, []byte(`{"id":"m-stub"}`)); err != nil {
		t.Fatal(err)
	}
	tk := &domain.Task{ID: "t-p-00001", MissionID: "m-stub", Front: domain.FrontProject,
		Title: "estudar o domínio: passada 1", ExpectedGain: "g", Novelty: "n",
		CostEstimate: "c", StopCondition: "s", Status: domain.TaskReady,
		CreatedAt: now, UpdatedAt: now}
	if err := s.Tasks().Put(u, tk); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Events().Append(u, "task", tk.ID, domain.EventTaskCreated, []byte(`{"id":"t-p-00001"}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Events().Append(u, "task", tk.ID, domain.EventTaskTransition,
		[]byte(`{"from":"NEW","to":"READY","reason":"replenished"}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Events().Append(u, "kernel", "kernel", domain.EventKernelCycle,
		[]byte(`{"cycle":7,"front":"project","outcome":"replenished","task":"t-p-00001","reward":0,"tokens":0}`)); err != nil {
		t.Fatal(err)
	}
	if err := s.KV().Set(u, kvKeyKernelCycle, []byte("7")); err != nil {
		t.Fatal(err)
	}
	if err := u.Commit(); err != nil {
		t.Fatal(err)
	}
	s.Close()
	return path
}

// firstField: primeira palavra da linha — tabwriter alinha com espaços,
// então asserção de seq lê campo, não byte de tab.
func firstField(s string) string {
	f := strings.Fields(s)
	if len(f) == 0 {
		return ""
	}
	return f[0]
}

func TestInspectShowsTheLoop(t *testing.T) {
	var buf bytes.Buffer
	if err := Inspect(&buf, fixtureDB(t)); err != nil {
		t.Fatalf("inspect: %v", err)
	}
	out := buf.String()
	for _, want := range []string{
		"m-stub",
		"Missão stub M1",
		"cycles:   7",
		"front project",
		"READY 1 · SUCCEEDED 0",
		"outcome=replenished",
		"0 pendente",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("inspect sem %q:\n%s", want, out)
		}
	}
}

func TestInspectOnEmptyDBIsHonest(t *testing.T) {
	path := filepath.Join(t.TempDir(), "eon.db")
	s, err := storage.NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()

	var buf bytes.Buffer
	if err := Inspect(&buf, path); err != nil {
		t.Fatalf("inspect em banco vazio: %v", err)
	}
	if !strings.Contains(buf.String(), "(nenhum ciclo registrado ainda)") {
		t.Fatalf("inspect em banco vazio mentiu:\n%s", buf.String())
	}
}

func TestTimelineCompleteAndNewestFirst(t *testing.T) {
	var buf bytes.Buffer
	if err := Timeline(&buf, fixtureDB(t), 0, 0, ""); err != nil {
		t.Fatalf("timeline: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	// 4 events no fixture + header
	if len(lines) != 5 {
		t.Fatalf("timeline: %d linhas, queria 5 (header+4):\n%s", len(lines), buf.String())
	}
	// mais novo primeiro: seq 4 (kernel.cycle) é a primeira linha de dados
	if firstField(lines[1]) != "4" {
		t.Fatalf("timeline não é mais-novo-primeiro, linha 1: %q", lines[1])
	}
	// payload cru é citação
	if !strings.Contains(lines[1], `"front":"project"`) {
		t.Fatalf("payload cru sumiu: %q", lines[1])
	}
}

func TestTimelineFilterAndCursor(t *testing.T) {
	path := fixtureDB(t)

	var buf bytes.Buffer
	if err := Timeline(&buf, path, 0, 0, "t-p-00001"); err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n")[1:] {
		if !strings.Contains(line, "task/t-p-00001") {
			t.Fatalf("--task vazou event de outro agregado: %q", line)
		}
	}

	buf.Reset()
	if err := Timeline(&buf, path, 0, 1, ""); err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(buf.String(), "\n"); n != 2 {
		t.Fatalf("--limit 1: %d linhas (header+1), queria 2:\n%s", n, buf.String())
	}

	// cursor: --after 3 só mostra o seq 4
	buf.Reset()
	if err := Timeline(&buf, path, 3, 0, ""); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 2 || firstField(lines[1]) != "4" {
		t.Fatalf("--after 3: queria só o seq 4:\n%s", buf.String())
	}
}

// bigLogDB: fixture com MAIS events que o batch do cursor (500) — o
// regime de produção, onde o bug de M1.6 congelava a janela no
// primeiro lote. O último event é um kernel.cycle que SÓ o fim do log
// contém.
func bigLogDB(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "eon.db")
	s, err := storage.NewStore(path)
	if err != nil {
		t.Fatalf("storage: %v", err)
	}
	defer s.Close()
	ctx := context.Background()
	for i := 0; i < 600; i++ {
		u, err := s.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.Events().Append(u, "task", "t-p-00001", domain.EventTaskTransition,
			[]byte(fmt.Sprintf(`{"i":%d}`, i))); err != nil {
			t.Fatal(err)
		}
		if err := u.Commit(); err != nil {
			t.Fatal(err)
		}
	}
	u, err := s.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Events().Append(u, "kernel", "kernel", domain.EventKernelCycle,
		[]byte(`{"cycle":601,"front":"thoughts","outcome":"succeeded","task":"t-th-00601","reward":1,"tokens":0}`)); err != nil {
		t.Fatal(err)
	}
	if err := s.KV().Set(u, kvKeyKernelCycle, []byte("601")); err != nil {
		t.Fatal(err)
	}
	if err := u.Commit(); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestTimelineEndFirstPastTheBatch (regressão do bug achado em
// produção): com o log maior que o batch, a janela default mostra o
// FIM da história, não o primeiro lote dela.
func TestTimelineEndFirstPastTheBatch(t *testing.T) {
	path := bigLogDB(t)

	var buf bytes.Buffer
	if err := Timeline(&buf, path, 0, 5, ""); err != nil {
		t.Fatalf("timeline: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 6 { // header + 5
		t.Fatalf("timeline: %d linhas, queria 6:\n%s", len(lines), buf.String())
	}
	if firstField(lines[1]) != "601" {
		t.Fatalf("timeline não começa no fim do log: %q", lines[1])
	}
	if !strings.Contains(lines[1], `"outcome":"succeeded"`) {
		t.Fatalf("primeira linha sem o payload do último event: %q", lines[1])
	}
	if firstField(lines[2]) != "600" {
		t.Fatalf("timeline não desce do fim: %q", lines[2])
	}

	// percurso completo: do 601 até o 1 — o cursor Before atravessa a
	// fronteira do batch sem repetir nem pular event.
	buf.Reset()
	if err := Timeline(&buf, path, 0, 0, ""); err != nil {
		t.Fatalf("timeline completa: %v", err)
	}
	lines = strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 602 { // header + 601
		t.Fatalf("timeline completa: %d linhas, queria 602", len(lines))
	}
	if firstField(lines[1]) != "601" || firstField(lines[601]) != "1" {
		t.Fatalf("timeline completa não vai do fim ao começo: %q…%q",
			lines[1], lines[601])
	}
}

// TestInspectLastPastTheBatch: o `last` do inspect é o ÚLTIMO desfecho
// do loop, não o último do primeiro lote — em produção o kernel seguiu
// vivo por 350+ ciclos com a janela dizendo "congelado no ciclo 73".
func TestInspectLastPastTheBatch(t *testing.T) {
	var buf bytes.Buffer
	if err := Inspect(&buf, bigLogDB(t)); err != nil {
		t.Fatalf("inspect: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "ciclo 601") || !strings.Contains(out, "outcome=succeeded") {
		t.Fatalf("inspect last não é o fim do log:\n%s", out)
	}
}

func TestRunDispatch(t *testing.T) {
	var buf bytes.Buffer
	if code := Run([]string{}, &buf); code != 2 {
		t.Fatalf("sem subcomando: code=%d", code)
	}
	if code := Run([]string{"estranho", "--data", t.TempDir()}, &buf); code != 2 {
		t.Fatalf("subcomando desconhecido: code=%d", code)
	}
	// banco ausente: erro honesto de leitura, exit 1
	buf.Reset()
	if code := Run([]string{"inspect", "--data", t.TempDir()}, &buf); code != 1 {
		t.Fatalf("inspect sem banco: code=%d, saida=%s", code, buf.String())
	}
}
