// main_test.go — testes de INTEGRAÇÃO da raiz de composição: o único
// lugar do repositório onde core encontra storage (ADR-0002). O kernel
// nunca viu SQLite; aqui os dois se encontram pela fiação de produção.
//
// O teste de processo re-executa ESTE binário pela fiação de verdade
// (main → coreCmd → storage → Deps → core.Main) — o mesmo caminho que o
// supervisor spawna. Sem atalho de teste: o contrato é o processo.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/brook-sys/eon/internal/core"
	"github.com/brook-sys/eon/internal/domain"
	"github.com/brook-sys/eon/internal/port"
	"github.com/brook-sys/eon/internal/statefile"
	"github.com/brook-sys/eon/internal/storage"
)

// TestMain despacha o papel de core REAL: o teste de processo re-executa
// ESTE binário pela fiação de produção (coreCmd → storage → Deps →
// core.Main). Nenhum outro pacote pode fazer este teste — core não
// importa storage, nem em teste.
func TestMain(m *testing.M) {
	if os.Getenv("EON_TEST_ROLE") == "core-main" {
		os.Exit(coreCmd([]string{"--data", os.Getenv("EON_TEST_DATA")}))
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

// testHeartbeat espelha o JSON do heartbeat (o struct canônico é do
// pacote core; aqui lê o ARQUIVO, o contrato entre processos).
type testHeartbeat struct {
	Beat     time.Time `json:"beat"`
	Pid      int       `json:"pid"`
	Beats    int64     `json:"beats"`
	Cycle    int64     `json:"cycle"`
	Failures int64     `json:"failures"`
}

// TestCoreProcessContract (herança M0, agora com kernel M1 de verdade):
// boot.json, heartbeat rolando COM ciclos, eon.db vivo, SIGTERM → exit 0
// limpo — o contrato supervisor↔core pela fiação real.
func TestCoreProcessContract(t *testing.T) {
	dir := t.TempDir()
	cmd := exec.Command(os.Args[0])
	cmd.Env = append(os.Environ(),
		"EON_TEST_ROLE=core-main",
		"EON_TEST_DATA="+dir,
		"EON_FAKE_QUOTA=1000000",
		"EON_CYCLE_INTERVAL_MS=50",
		domain.EnvSandboxLevel+"=basic", // simulando injeção do supervisor
	)
	var buf bytes.Buffer
	cmd.Stderr = &buf
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer cmd.Process.Kill()

	coreDir := filepath.Join(dir, "core")
	waitForFile(t, filepath.Join(coreDir, "boot.json"))
	waitForFile(t, filepath.Join(coreDir, "eon.db"))

	// heartbeat rolando COM ciclos: o kernel precisa de >=2 batidas e
	// pelo menos 1 ciclo de verdade antes do shutdown.
	hbPath := filepath.Join(coreDir, "heartbeat.json")
	deadline := time.Now().Add(10 * time.Second)
	for {
		var hb testHeartbeat
		if err := statefile.Read(hbPath, &hb); err == nil && hb.Beats >= 2 && hb.Cycle >= 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("kernel não acumulou ciclos em 10s (hb=%+v stderr=%q)", hb, buf.String())
		}
		time.Sleep(20 * time.Millisecond)
	}

	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("core deveria sair 0 no SIGTERM, veio %v (stderr=%q)", err, buf.String())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("core não fez shutdown em 5s")
	}

	var boot struct {
		SandboxLevel string `json:"sandbox_level"`
	}
	if err := statefile.Read(filepath.Join(coreDir, "boot.json"), &boot); err != nil {
		t.Fatalf("boot.json: %v", err)
	}
	if boot.SandboxLevel != "basic" {
		t.Errorf("sandbox_level injetada via env deveria ser \"basic\", veio %q", boot.SandboxLevel)
	}
}

// --- kernel × storage: a bateria de integração (SQLite real) ---

// newKernelEnv: storage real + kernel com provider fake. Sem fakes de
// repositório aqui — o alvo do teste É o casamento kernel×SQLite.
func newKernelEnv(t *testing.T, quota int) (*core.Kernel, *storage.Store, *core.FakeProvider) {
	t.Helper()
	store, err := storage.NewStore(filepath.Join(t.TempDir(), "eon.db"))
	if err != nil {
		t.Fatalf("storage: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	provider := core.NewFakeProvider(quota)
	deps := core.Deps{
		Begin:    store.Begin,
		Missions: store.Missions(),
		Tasks:    store.Tasks(),
		Steps:    store.Steps(),
		Events:   store.Events(),
		Outbox:   store.Outbox(),
		KV:       store.KV(),
		Provider: provider,
		Log:      log.New(io.Discard, "", 0),
	}
	k := core.NewKernel(deps)
	if err := k.Bootstrap(context.Background()); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	return k, store, provider
}

func driveCycles(t *testing.T, k *core.Kernel, n int) {
	t.Helper()
	ctx := context.Background()
	for i := 0; i < n; i++ {
		if err := k.Cycle(ctx); err != nil {
			t.Fatalf("ciclo %d derrubado: %v", i+1, err)
		}
	}
}

func allEvents(t *testing.T, s *storage.Store) []domain.Event {
	t.Helper()
	var out []domain.Event
	var seq int64
	for {
		batch, err := s.Events().After(seq, 200)
		if err != nil {
			t.Fatalf("events.After(%d): %v", seq, err)
		}
		if len(batch) == 0 {
			return out
		}
		out = append(out, batch...)
		seq = batch[len(batch)-1].Seq
	}
}

func transitionChain(t *testing.T, events []domain.Event, taskID string) []string {
	t.Helper()
	var chain []string
	for _, e := range events {
		if e.AggregateID != taskID || e.Type != domain.EventTaskTransition {
			continue
		}
		var p map[string]string
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			t.Fatalf("payload de transition ilegível: %v", err)
		}
		chain = append(chain, p["from"]+"→"+p["to"]+":"+p["reason"])
	}
	return chain
}

// allTasksOfFront: união das tarefas da frente sobre os estados vivos e
// terminais do M1 — para asserções de proveniência (a cadeia marcha).
func allTasksOfFront(t *testing.T, s *storage.Store, front string) []domain.Task {
	t.Helper()
	var out []domain.Task
	for _, status := range []domain.TaskStatus{
		domain.TaskNew, domain.TaskReady, domain.TaskRunning, domain.TaskVerifying,
		domain.TaskSucceeded, domain.TaskThrottled, domain.TaskReplanning, domain.TaskFailed,
	} {
		batch, err := s.Tasks().ListByStatus(front, status, 500)
		if err != nil {
			t.Fatalf("ListByStatus(%s,%s): %v", front, status, err)
		}
		out = append(out, batch...)
	}
	return out
}

// TestKernelTaskLifecycle: o caminho feliz COMPLETO de uma task por
// frente — replenish → claim → microturn → verify → SUCCEEDED + filho —
// com a máquina de estados e o event log íntegros.
func TestKernelTaskLifecycle(t *testing.T) {
	k, store, _ := newKernelEnv(t, 1_000_000)
	driveCycles(t, k, 60)

	for _, front := range domain.Fronts {
		done, err := store.Tasks().ListByStatus(front, domain.TaskSucceeded, 100)
		if err != nil {
			t.Fatal(err)
		}
		if len(done) == 0 {
			t.Fatalf("frente %s sem task SUCCEEDED após 60 ciclos", front)
		}
		parent := done[0]

		// step imutável com hashes (citação auditável)
		steps, err := store.Steps().ByTask(parent.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(steps) != 1 {
			t.Fatalf("task %s: %d steps, queria 1", parent.ID, len(steps))
		}
		wantKind := domain.StepKindModel
		if front == domain.FrontThoughts {
			wantKind = domain.StepKindCapability
		}
		if steps[0].Kind != wantKind || steps[0].Output == "" || steps[0].InputHash == "" {
			t.Fatalf("step de %s fora do contrato: %+v", front, steps[0])
		}

		// cadeia de transições: a lei da máquina de estados no event log
		chain := transitionChain(t, allEvents(t, store), parent.ID)
		want := []string{
			string(domain.TaskNew) + "→" + string(domain.TaskReady) + ":replenished",
			string(domain.TaskReady) + "→" + string(domain.TaskRunning) + ":claim",
			string(domain.TaskRunning) + "→" + string(domain.TaskVerifying) + ":step-done",
			string(domain.TaskVerifying) + "→" + string(domain.TaskSucceeded) + ":verified",
		}
		if len(chain) != len(want) {
			t.Fatalf("task %s: cadeia %v, queria %v", parent.ID, chain, want)
		}
		for i := range want {
			if chain[i] != want[i] {
				t.Fatalf("task %s: cadeia[%d]=%q, queria %q", parent.ID, i, chain[i], want[i])
			}
		}

		// candidato-filho com proveniência do pai (C1) — em QUALQUER
		// status: em 60 ciclos o filho da primeira task já pode ter
		// succeedido e deixado neto no lugar (a cadeia marcha).
		tasks := allTasksOfFront(t, store, front)
		foundChild := false
		for _, tk := range tasks {
			if tk.DerivedFrom == parent.ID {
				foundChild = true
				break
			}
		}
		if !foundChild {
			t.Fatalf("task %s: nenhum sucessor com derived_from do pai", parent.ID)
		}
	}
}

// TestKernelQuotaExhaustionF2Survives (aceite M1): cota esgota → F1/F3
// throttlam, F2 determinística segue produzindo, o loop NUNCA para nem
// derruba ciclos.
func TestKernelQuotaExhaustionF2Survives(t *testing.T) {
	const quota = 16 * 10 // 10 microturns de modelo
	k, store, provider := newKernelEnv(t, quota)
	driveCycles(t, k, 400)

	if provider.QuotaLeft() != 0 {
		t.Fatalf("cota deveria estar morta: sobrou %d", provider.QuotaLeft())
	}
	if n := provider.Calls(); n != quota/16 {
		t.Fatalf("provider: %d chamadas, queria %d", n, quota/16)
	}

	// o caminho THROTTLED foi exercitado (reason quota-exhausted no log)
	throttled := false
	for _, e := range allEvents(t, store) {
		if e.Type != domain.EventTaskTransition {
			continue
		}
		var p map[string]string
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			t.Fatal(err)
		}
		if p["reason"] == "quota-exhausted" {
			throttled = true
		}
	}
	if !throttled {
		t.Fatal("nenhuma task passou por quota-exhausted")
	}

	// F2 (thoughts) segue sucedendo com a cota morta (D12)
	done, err := store.Tasks().ListByStatus(domain.FrontThoughts, domain.TaskSucceeded, 500)
	if err != nil {
		t.Fatal(err)
	}
	if len(done) < 15 {
		t.Fatalf("F2 deveria seguir produzindo pós-cota: %d sucedidas", len(done))
	}
	// e cada uma com step capability real
	steps, err := store.Steps().ByTask(done[len(done)-1].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(steps) != 1 || steps[0].Kind != domain.StepKindCapability {
		t.Fatalf("última F2 sem step capability: %+v", steps)
	}
}

// TestKernelContinuityBlocked (aceite M1, C4): replenisher quebrado →
// CONTINUITY_BLOCKED diagnosticado no event log — nunca idle silencioso,
// nunca ciclo derrubado.
func TestKernelContinuityBlocked(t *testing.T) {
	dir := t.TempDir()
	store, err := storage.NewStore(filepath.Join(dir, "eon.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	deps := core.Deps{
		Begin:    store.Begin,
		Missions: store.Missions(),
		Tasks:    store.Tasks(),
		Steps:    store.Steps(),
		Events:   store.Events(),
		Outbox:   store.Outbox(),
		KV:       store.KV(),
		Provider: core.NewFakeProvider(1_000_000),
		Replenisher: func(u port.Unit, front string) (*domain.Task, error) {
			return nil, fmt.Errorf("replenish quebrado: %s", front)
		},
		Log: log.New(io.Discard, "", 0),
	}
	k := core.NewKernel(deps)
	if err := k.Bootstrap(context.Background()); err != nil {
		t.Fatal(err)
	}
	driveCycles(t, k, 5)

	blocked, created := 0, 0
	for _, e := range allEvents(t, store) {
		switch e.Type {
		case domain.EventContinuityBlocked:
			blocked++
		case domain.EventTaskCreated:
			created++
		}
	}
	if blocked == 0 {
		t.Fatal("fila vazia com replenisher quebrado e NENHUM continuity_blocked — idle silencioso")
	}
	if created != 0 {
		t.Fatalf("replenisher quebrado criou %d tasks", created)
	}
}

// TestKernelReconcileOrphans (aceite M1): RUNNING órfã de crash volta
// por REPLANNING→READY no boot — sem perda, sem duplicação, com events.
func TestKernelReconcileOrphans(t *testing.T) {
	_, store, _ := newKernelEnv(t, 1_000_000)
	ctx := context.Background()

	// simula crash-in-flight: task parada em RUNNING sem executor
	now := time.Now().UTC().Truncate(time.Second)
	orphan := &domain.Task{
		ID: "t-orphan", MissionID: core.StubMissionID, Front: domain.FrontProject,
		Title: "órfã de crash", ExpectedGain: "x", Novelty: "x", CostEstimate: "x",
		StopCondition: "x", Status: domain.TaskRunning, Attempt: 1,
		CreatedAt: now, UpdatedAt: now,
	}
	u, err := store.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Tasks().Put(u, orphan); err != nil {
		t.Fatal(err)
	}
	if err := u.Commit(); err != nil {
		t.Fatal(err)
	}

	// novo kernel (novo boot) reconcilia
	deps := core.Deps{
		Begin: store.Begin, Missions: store.Missions(), Tasks: store.Tasks(),
		Steps: store.Steps(), Events: store.Events(), Outbox: store.Outbox(),
		KV: store.KV(), Provider: core.NewFakeProvider(1_000_000),
		Log: log.New(io.Discard, "", 0),
	}
	k2 := core.NewKernel(deps)
	if err := k2.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}

	tk, err := store.Tasks().Get("t-orphan")
	if err != nil {
		t.Fatal(err)
	}
	if tk.Status != domain.TaskReady {
		t.Fatalf("órfã deveria voltar READY, veio %s", tk.Status)
	}
	chain := transitionChain(t, allEvents(t, store), "t-orphan")
	want := []string{
		string(domain.TaskRunning) + "→" + string(domain.TaskReplanning) + ":boot-reconcile:RUNNING",
		string(domain.TaskReplanning) + "→" + string(domain.TaskReady) + ":boot-reconcile",
	}
	if len(chain) != len(want) {
		t.Fatalf("cadeia da reconciliação: %v, queria %v", chain, want)
	}
	for i := range want {
		if chain[i] != want[i] {
			t.Fatalf("cadeia[%d]=%q, queria %q", i, chain[i], want[i])
		}
	}
}

// TestKernelDrainOutbox: o mecanismo transactional outbox — marca done +
// event no mesmo changeset.
func TestKernelDrainOutbox(t *testing.T) {
	k, store, _ := newKernelEnv(t, 1_000_000)
	ctx := context.Background()

	u, err := store.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	item, err := store.Outbox().Put(u, "fake-effect", []byte(`{"x":1}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := u.Commit(); err != nil {
		t.Fatal(err)
	}

	if err := k.DrainOutbox(ctx); err != nil {
		t.Fatal(err)
	}
	pending, err := store.Outbox().Pending(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Fatalf("outbox não drenou: %d pendentes", len(pending))
	}
	drained := false
	for _, e := range allEvents(t, store) {
		if e.Type == domain.EventOutboxDrained && e.AggregateID == item.ID {
			drained = true
		}
	}
	if !drained {
		t.Fatalf("item %s drenado sem event", item.ID)
	}
}

// TestKernelCycleCounterPersists: contador de ciclos é estado canônico —
// boot novo retoma de onde parou (crash não zera o relógio).
func TestKernelCycleCounterPersists(t *testing.T) {
	dir := t.TempDir()
	store, err := storage.NewStore(filepath.Join(dir, "eon.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	deps := core.Deps{
		Begin: store.Begin, Missions: store.Missions(), Tasks: store.Tasks(),
		Steps: store.Steps(), Events: store.Events(), Outbox: store.Outbox(),
		KV: store.KV(), Provider: core.NewFakeProvider(1_000_000),
		Log: log.New(io.Discard, "", 0),
	}

	k1 := core.NewKernel(deps)
	if err := k1.Bootstrap(context.Background()); err != nil {
		t.Fatal(err)
	}
	driveCycles(t, k1, 30)
	if got := k1.CycleCount(); got != 30 {
		t.Fatalf("ciclos em memória: %d, queria 30", got)
	}

	k2 := core.NewKernel(deps)
	if err := k2.Bootstrap(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := k2.CycleCount(); got != 30 {
		t.Fatalf("boot novo zerou o relógio: %d, queria 30", got)
	}
	if err := k2.Cycle(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := k2.CycleCount(); got != 31 {
		t.Fatalf("ciclo pós-reboot: %d, queria 31", got)
	}
}

// TestKernelBootstrapIdempotent: dois boots não duplicam missão nem
// reconciliam o que não precisa.
func TestKernelBootstrapIdempotent(t *testing.T) {
	k, store, _ := newKernelEnv(t, 1_000_000)
	if err := k.Bootstrap(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := k.Bootstrap(context.Background()); err != nil {
		t.Fatal(err)
	}
	created := 0
	for _, e := range allEvents(t, store) {
		if e.Type == domain.EventMissionCreated {
			created++
		}
	}
	if created != 1 {
		t.Fatalf("missão criada %d vezes — boot não é idempotente", created)
	}
}

// frontSequence: a sequência de frentes escolhidas, na ordem dos ciclos —
// extraída do event log (a auditoria é o canal, não a memória).
func frontSequence(t *testing.T, s *storage.Store) []string {
	t.Helper()
	var out []string
	for _, e := range allEvents(t, s) {
		if e.Type != domain.EventKernelCycle {
			continue
		}
		var p struct {
			Front string `json:"front"`
		}
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			t.Fatalf("payload kernel.cycle ilegível: %v", err)
		}
		out = append(out, p.Front)
	}
	return out
}

// TestKernelSoak1000 (aceite M1, docs/plan/10): 1.200 ciclos sem
// intervenção com cota morta no comecinho — o loop inteiro sobrevive,
// cada ciclo deixa exatamente UM kernel.cycle no log, nada pendura em
// estados vivos e a F2 segue produzindo até o fim (D12).
func TestKernelSoak1000(t *testing.T) {
	const quota = 16 * 10 // cota morre depois de 10 microturns de modelo
	const cycles = 1200
	k, store, provider := newKernelEnv(t, quota)
	driveCycles(t, k, cycles)

	if got := k.CycleCount(); got != cycles {
		t.Fatalf("contador: %d, queria %d", got, cycles)
	}
	if provider.QuotaLeft() != 0 || provider.Calls() != quota/16 {
		t.Fatalf("cota: calls=%d left=%d — a morte deveria ser exata", provider.Calls(), provider.QuotaLeft())
	}

	// UM event kernel.cycle por ciclo — o log é a verdade do loop
	var cycleEvents int64
	for _, e := range allEvents(t, store) {
		if e.Type == domain.EventKernelCycle {
			cycleEvents++
		}
	}
	if cycleEvents != cycles {
		t.Fatalf("%d events kernel.cycle para %d ciclos", cycleEvents, cycles)
	}

	// nada pendurado: sem RUNNING/VERIFYING órfãs num fim de ciclo limpo
	for _, front := range domain.Fronts {
		for _, status := range []domain.TaskStatus{domain.TaskRunning, domain.TaskVerifying} {
			alive, err := store.Tasks().ListByStatus(front, status, 10)
			if err != nil {
				t.Fatal(err)
			}
			if len(alive) != 0 {
				t.Fatalf("frente %s com %d tasks em %s no fim do soak", front, len(alive), status)
			}
		}
	}

	// a F2 segue produzindo com a cota morta há mais de mil ciclos — e o
	// crescimento não parou no comecinho: a última succeedida é mais
	// recente que a primeira (ListByStatus ordena por created_at ASC)
	done, err := store.Tasks().ListByStatus(domain.FrontThoughts, domain.TaskSucceeded, 500)
	if err != nil {
		t.Fatal(err)
	}
	if len(done) < 50 {
		t.Fatalf("F2 produziu %d em %d ciclos com cota morta — a lifeline falhou", len(done), cycles)
	}
	if !done[len(done)-1].CreatedAt.After(done[0].CreatedAt) {
		t.Fatal("a F2 parou de produzir no comecinho do soak")
	}
}

// TestKernelPicksDeterministic (D26): mesma sequência de recompensas →
// mesmos picks. Dois soaks independentes, mesma cota: as frentes
// escolhidas são IDÊNTICAS ciclo a ciclo — zero RNG, auditável de fora.
func TestKernelPicksDeterministic(t *testing.T) {
	run := func() []string {
		k, store, _ := newKernelEnv(t, 16*10)
		driveCycles(t, k, 300)
		return frontSequence(t, store)
	}
	a, b := run(), run()
	if len(a) != 300 || len(b) != 300 {
		t.Fatalf("soak curto: %d/%d ciclos", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("pick %d divergiu: %q vs %q — RNG escondido no scheduler", i+1, a[i], b[i])
		}
	}
}
