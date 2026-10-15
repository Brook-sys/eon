// contract_test.go — contract suite por repositório contra SQLite REAL
// (docs/plan/05 §Testes de persistência: a herança boa da v1). O que estes
// testes provam é o que o kernel M1.4 ASSUME — quebra aqui = loop mentindo.
package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/brook-sys/eon/internal/domain"
	"github.com/brook-sys/eon/internal/port"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := NewStore(filepath.Join(t.TempDir(), "eon.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func mustBegin(t *testing.T, s *Store) port.Unit {
	t.Helper()
	u, err := s.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return u
}

// Contrato de precisão: timestamps do schema são SEGUNDOS unix (D44 —
// cursor de paginação é por seq, nunca por tempo). Fixtures truncam.

func fixtureMission(id string) *domain.Mission {
	now := time.Now().UTC().Truncate(time.Second)
	return &domain.Mission{
		ID: id, Title: "Missão stub", Purpose: "provar o loop M1",
		Status: domain.MissionActive, CreatedAt: now, UpdatedAt: now,
	}
}

func fixtureTask(id, missionID, front string, status domain.TaskStatus, createdAt time.Time) *domain.Task {
	return &domain.Task{
		ID: id, MissionID: missionID, Front: front, Title: "t-" + id,
		ExpectedGain: "ganho esperado", Novelty: "novidade declarada",
		CostEstimate: "baixo", StopCondition: "critério de parada",
		Status: status, CreatedAt: createdAt, UpdatedAt: createdAt,
	}
}

func commit(t *testing.T, u port.Unit) {
	t.Helper()
	if err := u.Commit(); err != nil {
		t.Fatal(err)
	}
}

func putMission(t *testing.T, s *Store, m *domain.Mission) {
	t.Helper()
	u := mustBegin(t, s)
	if err := s.Missions().Put(u, m); err != nil {
		t.Fatal(err)
	}
	commit(t, u)
}

func putTask(t *testing.T, s *Store, tk *domain.Task) {
	t.Helper()
	u := mustBegin(t, s)
	if err := s.Tasks().Put(u, tk); err != nil {
		t.Fatal(err)
	}
	commit(t, u)
}

func TestMissionContract(t *testing.T) {
	s := newTestStore(t)
	m := fixtureMission("m-1")
	putMission(t, s, m)

	got, err := s.Missions().Get("m-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != m.Title || got.Status != m.Status || !got.CreatedAt.Equal(m.CreatedAt) {
		t.Fatalf("round-trip da missão mudou dados: %+v vs %+v", got, m)
	}
	if _, err := s.Missions().Get("m-fantasma"); !errors.Is(err, port.ErrNotFound) {
		t.Fatalf("missão fantasma: erro=%v, queria ErrNotFound", err)
	}
}

func TestTaskContract(t *testing.T) {
	s := newTestStore(t)
	putMission(t, s, fixtureMission("m-1"))

	nb := time.Now().Add(2 * time.Hour).UTC().Truncate(time.Second)
	tk := fixtureTask("t-1", "m-1", domain.FrontProject, domain.TaskReady, time.Now().UTC().Truncate(time.Second))
	tk.NotBefore = &nb
	tk.DerivedFrom = "seed"
	putTask(t, s, tk)

	got, err := s.Tasks().Get("t-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.NotBefore == nil || !got.NotBefore.Equal(nb) {
		t.Fatalf("not_before round-trip: %v queria %v", got.NotBefore, nb)
	}
	if got.DerivedFrom != "seed" || got.Status != domain.TaskReady {
		t.Fatalf("task round-trip errada: %+v", got)
	}

	// upsert: mesma id, novo status
	got.Status = domain.TaskRunning
	putTask(t, s, got)
	again, err := s.Tasks().Get("t-1")
	if err != nil {
		t.Fatal(err)
	}
	if again.Status != domain.TaskRunning {
		t.Fatalf("upsert não atualizou status: %s", again.Status)
	}
}

func TestNextReadyContract(t *testing.T) {
	s := newTestStore(t)
	now := time.Now().UTC()
	putMission(t, s, fixtureMission("m-1"))

	old := fixtureTask("t-old", "m-1", domain.FrontProject, domain.TaskReady, now.Add(-120*time.Second))
	young := fixtureTask("t-young", "m-1", domain.FrontProject, domain.TaskReady, now.Add(-60*time.Second))
	future := fixtureTask("t-future", "m-1", domain.FrontProject, domain.TaskReady, now.Add(-90*time.Second))
	later := now.Add(1 * time.Hour)
	future.NotBefore = &later
	otherFront := fixtureTask("t-thoughts", "m-1", domain.FrontThoughts, domain.TaskReady, now.Add(-300*time.Second))
	running := fixtureTask("t-running", "m-1", domain.FrontProject, domain.TaskRunning, now.Add(-240*time.Second))

	for _, tk := range []*domain.Task{old, young, future, otherFront, running} {
		putTask(t, s, tk)
	}

	got, err := s.Tasks().NextReady(domain.FrontProject, now)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "t-old" {
		t.Fatalf("NextReady pegou %s, queria a mais antiga t-old (ordem determinística)", got.ID)
	}
	n, err := s.Tasks().CountReady(domain.FrontProject, now)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("CountReady=%d, queria 2 (old+young; futuro/running/outra frente fora)", n)
	}
	if _, err := s.Tasks().NextReady(domain.FrontSelf, now); !errors.Is(err, port.ErrNotFound) {
		t.Fatalf("fila vazia: erro=%v, queria ErrNotFound", err)
	}
}

// TestListByStatusContract: filtro por frente+status com ordem
// determinística — a base da reconciliação de boot e do backlog THROTTLED.
func TestListByStatusContract(t *testing.T) {
	s := newTestStore(t)
	now := time.Now().UTC()
	putMission(t, s, fixtureMission("m-1"))
	putTask(t, s, fixtureTask("t-r", "m-1", domain.FrontProject, domain.TaskRunning, now))
	putTask(t, s, fixtureTask("t-o", "m-1", domain.FrontThoughts, domain.TaskRunning, now.Add(-30*time.Second)))
	putTask(t, s, fixtureTask("t-ready", "m-1", domain.FrontProject, domain.TaskReady, now.Add(-10*time.Second)))

	got, err := s.Tasks().ListByStatus(domain.FrontProject, domain.TaskRunning, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "t-r" {
		t.Fatalf("ListByStatus(project,RUNNING): %+v — filtro frente/status furado", got)
	}

	// limit 0 = vazio (contrato de LIMIT), e status sem tarefas = vazio.
	if got, err := s.Tasks().ListByStatus(domain.FrontProject, domain.TaskRunning, 0); err != nil || len(got) != 0 {
		t.Fatalf("limit 0: %+v err=%v", got, err)
	}
	if got, err := s.Tasks().ListByStatus(domain.FrontSelf, domain.TaskThrottled, 10); err != nil || len(got) != 0 {
		t.Fatalf("status vazio: %+v err=%v", got, err)
	}
}

func TestStepContract(t *testing.T) {
	s := newTestStore(t)
	now := time.Now().UTC()
	putMission(t, s, fixtureMission("m-1"))
	putTask(t, s, fixtureTask("t-1", "m-1", domain.FrontProject, domain.TaskRunning, now))

	for i, out := range []string{"primeira resposta", "segunda resposta"} {
		st := &domain.Step{
			ID: "s-" + string(rune('a'+i)), TaskID: "t-1", Seq: i + 1,
			Kind: domain.StepKindModel, Input: "passo", InputHash: "ih",
			Output: out, OutputHash: "oh-" + out, DurationMs: int64(5 + i), CreatedAt: now,
		}
		u := mustBegin(t, s)
		if err := s.Steps().Put(u, st); err != nil {
			t.Fatal(err)
		}
		commit(t, u)
	}

	steps, err := s.Steps().ByTask("t-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(steps) != 2 || steps[0].Seq != 1 || steps[1].Seq != 2 {
		t.Fatalf("ByTask ordem errada: %+v", steps)
	}
	if steps[1].Output != "segunda resposta" {
		t.Fatalf("payload do passo mudou: %+v", steps[1])
	}
}

func TestEventLogContract(t *testing.T) {
	s := newTestStore(t)
	appendEvent := func(payload string) domain.Event {
		u := mustBegin(t, s)
		e, err := s.Events().Append(u, "kernel", "k-1", domain.EventKernelCycle, []byte(payload))
		if err != nil {
			t.Fatal(err)
		}
		commit(t, u)
		return e
	}

	e1 := appendEvent(`{"n":1}`)
	e2 := appendEvent(`{"n":2}`)
	e3 := appendEvent(`{"n":3}`)
	if !(e1.Seq < e2.Seq && e2.Seq < e3.Seq) {
		t.Fatalf("seq não monotônico: %d %d %d", e1.Seq, e2.Seq, e3.Seq)
	}
	sum := sha256.Sum256(e1.Payload)
	if e1.PayloadHash != hex.EncodeToString(sum[:]) {
		t.Fatal("payload_hash não é o sha256 do payload")
	}

	page1, err := s.Events().After(0, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(page1) != 2 || page1[0].Seq != e1.Seq || page1[1].Seq != e2.Seq {
		t.Fatalf("cursor After(0,2) errado: %+v", page1)
	}
	page2, err := s.Events().After(page1[1].Seq, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(page2) != 1 || page2[0].Seq != e3.Seq {
		t.Fatalf("cursor After(%d,10) errado: %+v", page1[1].Seq, page2)
	}
}

// TestEventLogLatestBefore: a janela do FIM do log. Latest/Before é o
// par que impede a janela do operador de congelar no primeiro lote
// quando o log passa do batch (bug de M1.6 achado em produção).
func TestEventLogLatestBefore(t *testing.T) {
	s := newTestStore(t)
	appendEvent := func(n int) domain.Event {
		u := mustBegin(t, s)
		e, err := s.Events().Append(u, "kernel", "k-1", domain.EventKernelCycle,
			[]byte(fmt.Sprintf(`{"n":%d}`, n)))
		if err != nil {
			t.Fatal(err)
		}
		commit(t, u)
		return e
	}
	var last domain.Event
	for i := 0; i < 5; i++ {
		last = appendEvent(i)
	}

	// Latest: os mais novos, descendente.
	head, err := s.Events().Latest(3)
	if err != nil {
		t.Fatal(err)
	}
	if len(head) != 3 || head[0].Seq != last.Seq || head[1].Seq != last.Seq-1 || head[2].Seq != last.Seq-2 {
		t.Fatalf("Latest(3) não desce do fim: %+v", head)
	}

	// Before: continuação para trás, sem repetir a fronteira.
	tail, err := s.Events().Before(head[2].Seq, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(tail) != 2 || tail[0].Seq != last.Seq-3 || tail[1].Seq != last.Seq-4 {
		t.Fatalf("Before(%d,10) não continua para trás: %+v", head[2].Seq, tail)
	}

	// antes do começo: vazio honesto, sem erro
	empty, err := s.Events().Before(1, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(empty) != 0 {
		t.Fatalf("Before(1,10) devia ser vazio: %+v", empty)
	}
}

func TestOutboxContract(t *testing.T) {
	s := newTestStore(t)
	u := mustBegin(t, s)
	it, err := s.Outbox().Put(u, "git_push", []byte(`{"ref":"v2"}`))
	if err != nil {
		t.Fatal(err)
	}
	commit(t, u)

	pending, err := s.Outbox().Pending(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].ID != it.ID || pending[0].Done {
		t.Fatalf("pending errado: %+v", pending)
	}
	u = mustBegin(t, s)
	if err := s.Outbox().MarkDone(u, it.ID); err != nil {
		t.Fatal(err)
	}
	commit(t, u)
	pending, _ = s.Outbox().Pending(10)
	if len(pending) != 0 {
		t.Fatalf("MarkDone não drenou: %+v", pending)
	}
}

func TestKVContract(t *testing.T) {
	s := newTestStore(t)
	u := mustBegin(t, s)
	if err := s.KV().Set(u, "kernel.cycle", []byte("42")); err != nil {
		t.Fatal(err)
	}
	commit(t, u)

	v, err := s.KV().Get("kernel.cycle")
	if err != nil {
		t.Fatal(err)
	}
	if string(v) != "42" {
		t.Fatalf("kv round-trip: %q", v)
	}
	if _, err := s.KV().Get("chave.fantasma"); !errors.Is(err, port.ErrNotFound) {
		t.Fatalf("kv fantasma: erro=%v", err)
	}
}

// TestChangesetAtomicity: a lei do ChangeSet (D03) — estado + event na
// MESMA transação; rollback descarta TUDO, commit persiste TUDO. Perda/
// duplicação sob kill -9 é provada no crash-replay (M1.8); aqui é a
// SEMÂNTICA.
func TestChangesetAtomicity(t *testing.T) {
	s := newTestStore(t)

	// lote rejeitado: nada persiste
	u := mustBegin(t, s)
	_ = s.Missions().Put(u, fixtureMission("m-x"))
	_ = s.Tasks().Put(u, fixtureTask("t-x", "m-x", domain.FrontSelf, domain.TaskNew, time.Now().UTC()))
	_, _ = s.Events().Append(u, "mission", "m-x", domain.EventMissionCreated, []byte(`{}`))
	if err := u.Rollback(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Missions().Get("m-x"); !errors.Is(err, port.ErrNotFound) {
		t.Fatal("rollback não descartou a missão")
	}
	if _, err := s.Tasks().Get("t-x"); !errors.Is(err, port.ErrNotFound) {
		t.Fatal("rollback não descartou a task")
	}
	evs, _ := s.Events().After(0, 100)
	if len(evs) != 0 {
		t.Fatalf("rollback não descartou o event: %d", len(evs))
	}

	// lote aplicado: tudo persiste
	u = mustBegin(t, s)
	_ = s.Missions().Put(u, fixtureMission("m-x"))
	_ = s.Tasks().Put(u, fixtureTask("t-x", "m-x", domain.FrontSelf, domain.TaskNew, time.Now().UTC()))
	if _, err := s.Events().Append(u, "mission", "m-x", domain.EventMissionCreated, []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	commit(t, u)
	if _, err := s.Missions().Get("m-x"); err != nil {
		t.Fatal("commit não persistiu a missão")
	}
	evs, _ = s.Events().After(0, 100)
	if len(evs) != 1 {
		t.Fatalf("commit não persistiu o event: %d", len(evs))
	}
}

// TestForeignKeyEnforced: FK é lei do schema (D03) — task órfã reprova.
// foreign_keys=ON tem que valer em TODA conexão do pool (driver.go: DSN).
func TestForeignKeyEnforced(t *testing.T) {
	s := newTestStore(t)
	u := mustBegin(t, s)
	err := s.Tasks().Put(u, fixtureTask("t-orfa", "missao-inexistente", domain.FrontSelf, domain.TaskNew, time.Now().UTC()))
	_ = u.Rollback()
	if err == nil {
		t.Fatal("task órfã foi aceita — foreign_keys não está ON nesta conexão")
	}
}

// TestMigrateChainReopen: migrar é idempotente — reopen não reaplica,
// user_version fica no teto da cadeia, tabelas de pé.
func TestMigrateChainReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "eon.db")
	s1, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	var v1 int
	if err := s1.db.QueryRow(`PRAGMA user_version`).Scan(&v1); err != nil {
		t.Fatal(err)
	}
	if want := migrations[len(migrations)-1].version; v1 != want {
		t.Fatalf("user_version=%d, queria %d", v1, want)
	}
	s1.Close()

	s2, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	var v2 int
	if err := s2.db.QueryRow(`PRAGMA user_version`).Scan(&v2); err != nil {
		t.Fatal(err)
	}
	if v2 != v1 {
		t.Fatalf("reopen mudou user_version: %d → %d", v1, v2)
	}
	var n int
	if err := s2.db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 7 {
		t.Fatalf("tabelas=%d, queria 7 (mission/task/step/events/outbox/kv_meta + sqlite_sequence do AUTOINCREMENT)", n)
	}
}
