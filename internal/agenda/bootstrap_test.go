package agenda

import (
	"context"
	"errors"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/runtime/source"
	"motor-autonomo/internal/storage/memory"
)

func TestBootstrapperPersistsRecoverableWorkAtomically(t *testing.T) {
	store := memory.New()
	seedCatalog(t, store)
	now := time.Date(2026, 7, 15, 14, 0, 0, 0, time.UTC)
	bootstrapper := Bootstrapper{Store: store, Clock: source.NewManualClock(now), IDs: source.NewSequenceIDGenerator(1)}
	work, err := bootstrapper.Create(context.Background(), validWorkSpec())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if work.Operation.ID != "operation_0000000000000004" || work.Operation.IdempotencyKey != "idempotency_0000000000000005" {
		t.Fatalf("generated work = %#v", work)
	}
	if err := store.View(context.Background(), func(r port.Reader) error {
		q, err := r.Question(work.Question.ID)
		if err != nil {
			return err
		}
		i, err := r.Inquiry(work.Inquiry.ID)
		if err != nil {
			return err
		}
		o, err := r.Operation(work.Operation.ID)
		if err != nil {
			return err
		}
		if q.MissionRevision != "revision_1" || i.State != domain.StateReady || i.Reevaluation.Kind != domain.ReevaluateReady || o.State != domain.StateReady || o.Reevaluation.Kind != domain.ReevaluateReady {
			t.Fatalf("recovered work is incomplete: q=%#v i=%#v o=%#v", q, i, o)
		}
		events, err := r.Events(0, 10)
		if err != nil {
			return err
		}
		if len(events) != 1 || events[0].Kind != EventAgendaWorkCreated || events[0].InquiryID != i.ID || events[0].OperationID != o.ID || !events[0].OccurredAt.Equal(now) {
			t.Fatalf("events = %#v", events)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestBootstrapperRejectsMissingCatalogEntriesWithoutPartialWrites(t *testing.T) {
	tests := []struct {
		name                  string
		seedMission, seedSpec bool
	}{
		{name: "missing mission", seedSpec: true},
		{name: "missing operation spec", seedMission: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := memory.New()
			if err := store.Update(context.Background(), func(tx port.Transaction) error {
				if tc.seedMission {
					if err := tx.AppendMissionRevision(mission()); err != nil {
						return err
					}
				}
				if tc.seedSpec {
					if err := tx.AppendOperationSpec(operationSpec()); err != nil {
						return err
					}
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			bootstrapper := Bootstrapper{Store: store, Clock: source.NewManualClock(time.Now()), IDs: source.NewSequenceIDGenerator(1)}
			work, err := bootstrapper.Create(context.Background(), validWorkSpec())
			if !errors.Is(err, port.ErrNotFound) {
				t.Fatalf("error = %v, want ErrNotFound", err)
			}
			if err := store.View(context.Background(), func(r port.Reader) error {
				_, qerr := r.Question(work.Question.ID)
				events, eerr := r.Events(0, 10)
				if !errors.Is(qerr, port.ErrNotFound) || eerr != nil || len(events) != 0 {
					t.Fatalf("partial state: question=%v events=%#v error=%v", qerr, events, eerr)
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func seedCatalog(t *testing.T, store port.Store) {
	t.Helper()
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		if err := tx.AppendMissionRevision(mission()); err != nil {
			return err
		}
		return tx.AppendOperationSpec(operationSpec())
	}); err != nil {
		t.Fatal(err)
	}
}

func mission() domain.MissionRevision {
	return domain.MissionRevision{SchemaVersion: 1, ID: "revision_1", MissionID: "mission_1", Revision: 1, OriginalText: "investigate", Purpose: "knowledge", Domains: []string{"runtime"}, Policies: []string{"cite"}, Status: domain.MissionActive, Provenance: "user", AcceptedAt: time.Date(2026, 7, 15, 13, 0, 0, 0, time.UTC)}
}
func operationSpec() domain.OperationSpec {
	return domain.OperationSpec{SchemaVersion: 1, ID: "extract@1", ContractVersion: 1, TemplateVersion: 1, InputSchema: "refs", OutputSchema: "proposal", Budget: domain.Budget{ModelCalls: 1, Tokens: 1000, Attempts: 1}, MaxOutputTokens: 100, SafetyMargin: 50, Validators: []string{"schema"}, RetryPolicy: "none", FallbackPolicy: "fail", MaximumAuthority: domain.AuthorityProposeOnly}
}
func validWorkSpec() WorkSpec {
	return WorkSpec{MissionRevision: "revision_1", QuestionText: "What is supported?", QuestionOrigin: "mission", Relevance: "primary", AnswerCondition: "two sources", DerivedFrom: []string{"mission purpose"}, ExpectedProgress: "reduce uncertainty", Novelty: "first inquiry", EstimatedCost: domain.Budget{ModelCalls: 1, Tokens: 1000, Attempts: 1}, Risk: domain.RiskLow, SourcePlan: []string{"primary sources"}, StopCondition: "answer or budget", ReviewAfter: time.Date(2026, 7, 16, 14, 0, 0, 0, time.UTC), AdmissionReason: "highest expected gain", InquiryBudget: domain.Budget{ModelCalls: 2, Tokens: 2000, Attempts: 2}, OperationSpecID: "extract@1", ReadSet: []string{"source_1"}, InputRefs: []string{"question context"}, ExpectedOutput: "proposed change set"}
}
