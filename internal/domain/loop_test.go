package domain

import (
	"encoding/json"
	"testing"
	"time"
)

// A máquina de estados é lei do loop (03-LOOP.md) — tabela é testada como
// tabela: cada caminho legal passa, cada atalho ilegal reprova. Modelos
// propõem; esta tabela decide (invariante 2).

func TestCanTransitionLegalPaths(t *testing.T) {
	legal := [][2]TaskStatus{
		{TaskNew, TaskReady},
		{TaskNew, TaskCancelled},
		{TaskReady, TaskRunning},
		{TaskRunning, TaskVerifying},
		{TaskRunning, TaskWaitingTime},
		{TaskRunning, TaskWaitingEvent},
		{TaskRunning, TaskWaitingOperator},
		{TaskRunning, TaskThrottled},
		{TaskRunning, TaskBlocked},
		{TaskRunning, TaskExhausted},
		{TaskRunning, TaskReplanning},
		{TaskRunning, TaskFailed},
		{TaskVerifying, TaskSucceeded},
		{TaskVerifying, TaskFailed},
		{TaskVerifying, TaskReplanning},
		{TaskWaitingTime, TaskReady},
		{TaskWaitingOperator, TaskReady},
		{TaskThrottled, TaskReady},
		{TaskBlocked, TaskReady},
		{TaskReplanning, TaskReady},
		{TaskExhausted, TaskCancelled},
	}
	for _, c := range legal {
		if !CanTransition(c[0], c[1]) {
			t.Errorf("CanTransition(%s→%s)=false, queria true", c[0], c[1])
		}
	}
}

func TestCanTransitionIllegalPaths(t *testing.T) {
	illegal := [][2]TaskStatus{
		{TaskNew, TaskRunning},          // pular READY é atalho proibido
		{TaskRunning, TaskSucceeded},    // sucesso exige VERIFYING
		{TaskVerifying, TaskRunning},    // verificação não volta atrás
		{TaskSucceeded, TaskRunning},    // terminal é congelado
		{TaskSucceeded, TaskReplanning}, // terminal é congelado
		{TaskFailed, TaskReady},         // morto não ressuscita: filho novo
		{TaskCancelled, TaskReady},      // cancelado é história
		{TaskExhausted, TaskReady},      // orçamento morto: só cancela
		{TaskThrottled, TaskRunning},    // throttled volta por READY
		{TaskReady, TaskWaitingTime},    // espera só nasce de RUNNING
	}
	for _, c := range illegal {
		if CanTransition(c[0], c[1]) {
			t.Errorf("CanTransition(%s→%s)=true, queria false", c[0], c[1])
		}
	}
}

// Cancelar é sempre direito do operador/kernel — de qualquer estado vivo.
func TestCancelAlwaysAllowedWhileAlive(t *testing.T) {
	for _, from := range []TaskStatus{
		TaskNew, TaskReady, TaskRunning, TaskVerifying, TaskWaitingTime,
		TaskWaitingEvent, TaskWaitingOperator, TaskThrottled, TaskBlocked,
		TaskExhausted, TaskReplanning,
	} {
		if !CanTransition(from, TaskCancelled) {
			t.Errorf("CanTransition(%s→CANCELLED)=false, cancelar é sempre direito", from)
		}
	}
}

// TestTaskEventJSONRoundTrip: as tags JSON são contrato (storage serializa,
// eonctl lê, replay compara) — round-trip estável é o teste.
func TestTaskEventJSONRoundTrip(t *testing.T) {
	nb := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	task := Task{
		ID:            "t1",
		MissionID:     "m1",
		Front:         FrontProject,
		Title:         "estudar o loop",
		DerivedFrom:   "seed",
		ExpectedGain:  "clareza",
		Novelty:       "primeira passada",
		CostEstimate:  "baixo",
		StopCondition: "resumo citado",
		Status:        TaskReady,
		NotBefore:     &nb,
		Attempt:       2,
		CreatedAt:     nb,
		UpdatedAt:     nb,
	}
	b, err := json.Marshal(task)
	if err != nil {
		t.Fatal(err)
	}
	var back Task
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if back.ID != task.ID || back.MissionID != task.MissionID || back.Front != task.Front ||
		back.Title != task.Title || back.DerivedFrom != task.DerivedFrom ||
		back.ExpectedGain != task.ExpectedGain || back.Novelty != task.Novelty ||
		back.CostEstimate != task.CostEstimate || back.StopCondition != task.StopCondition ||
		back.Status != task.Status || back.Attempt != task.Attempt {
		t.Fatalf("round-trip mudou campos simples:\n %+v\n %+v", task, back)
	}
	if back.NotBefore == nil || !back.NotBefore.Equal(nb) {
		t.Fatalf("round-trip do not_before: %v queria %v", back.NotBefore, nb)
	}
	if !back.CreatedAt.Equal(task.CreatedAt) || !back.UpdatedAt.Equal(task.UpdatedAt) {
		t.Fatal("round-trip mudou timestamps")
	}

	ev := Event{
		Seq:           7,
		AggregateType: "task",
		AggregateID:   "t1",
		Type:          EventTaskTransition,
		PayloadHash:   "abc",
		Payload:       []byte(`{"de":"READY","para":"RUNNING"}`),
		OccurredAt:    nb,
	}
	b, err = json.Marshal(ev)
	if err != nil {
		t.Fatal(err)
	}
	var evBack Event
	if err := json.Unmarshal(b, &evBack); err != nil {
		t.Fatal(err)
	}
	if evBack.Seq != ev.Seq || evBack.Type != ev.Type || string(evBack.Payload) != string(ev.Payload) {
		t.Fatalf("round-trip mudou o event: %+v vs %+v", ev, evBack)
	}
}
