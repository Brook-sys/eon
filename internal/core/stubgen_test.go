package core

import (
	"testing"
	"time"

	"github.com/brook-sys/eon/internal/domain"
)

// Stub é trabalho de mentira? Não — é trabalho HONESTO de marco: campos
// obrigatórios declarados, id determinístico, proveniência explícita.
// Testado como contrato: mesmo contador → mesma task; numeração
// monotônica dentro do mesmo Unit; campos obrigatórios sempre presentes.

func TestNewStubTaskDeterministicByID(t *testing.T) {
	kv := newFakeKV()
	now := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)

	tk, err := newStubTask(fakeUnit{}, kv, domain.FrontProject, "gen:project", now)
	if err != nil {
		t.Fatal(err)
	}
	if tk.ID != "t-p-00001" {
		t.Fatalf("id determinístico: %q, queria t-p-00001", tk.ID)
	}
	if tk.MissionID != StubMissionID || tk.Front != domain.FrontProject {
		t.Fatalf("task nasceu órfã: %+v", tk)
	}
	if tk.DerivedFrom != "gen:project" {
		t.Fatalf("proveniência perdida: %q", tk.DerivedFrom)
	}
}

func TestNewStubTaskCounterMonotonic(t *testing.T) {
	kv := newFakeKV()
	now := time.Now().UTC()

	a, err := newStubTask(fakeUnit{}, kv, domain.FrontProject, "gen:project", now)
	if err != nil {
		t.Fatal(err)
	}
	b, err := newStubTask(fakeUnit{}, kv, domain.FrontProject, a.ID, now)
	if err != nil {
		t.Fatal(err)
	}
	if a.ID == b.ID {
		t.Fatalf("ids duplicados: %s", a.ID)
	}
	if b.DerivedFrom != a.ID {
		t.Fatalf("candidato-filho sem proveniência do pai: %q", b.DerivedFrom)
	}
	// contadores por frente são INDEPENDENTES (project e thoughts não
	// brigam pela mesma numeração)
	c, err := newStubTask(fakeUnit{}, kv, domain.FrontThoughts, "gen:thoughts", now)
	if err != nil {
		t.Fatal(err)
	}
	if c.ID != "t-th-00001" {
		t.Fatalf("contador de frente independente: %q, queria t-th-00001", c.ID)
	}
}

func TestNewStubTaskMandatoryFields(t *testing.T) {
	kv := newFakeKV()
	now := time.Now().UTC()
	for _, front := range domain.Fronts {
		tk, err := newStubTask(fakeUnit{}, kv, front, "gen:"+front, now)
		if err != nil {
			t.Fatal(err)
		}
		if tk.ID == "" || tk.MissionID == "" || tk.Front == "" || tk.Title == "" ||
			tk.ExpectedGain == "" || tk.Novelty == "" || tk.CostEstimate == "" ||
			tk.StopCondition == "" || tk.DerivedFrom == "" {
			t.Fatalf("frente %s: campo obrigatório vazio: %+v", front, tk)
		}
		if tk.Status != domain.TaskNew {
			t.Fatalf("task nasce NEW (máquina de estados), veio %s", tk.Status)
		}
	}
}

func TestStepKindOf(t *testing.T) {
	if StepKindOf(domain.FrontProject) != domain.StepKindModel {
		t.Fatal("F1 é microturn de modelo no M1")
	}
	if StepKindOf(domain.FrontThoughts) != domain.StepKindCapability {
		t.Fatal("F2 é a frente determinística (D12)")
	}
	if StepKindOf(domain.FrontSelf) != domain.StepKindModel {
		t.Fatal("F3 é microturn de modelo no M1")
	}
}
