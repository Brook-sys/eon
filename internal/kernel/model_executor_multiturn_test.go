package kernel

import (
	"context"
	"encoding/json"
	"motor-autonomo/internal/changeset"
	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/prompt"
	"motor-autonomo/internal/runtime/source"
	"motor-autonomo/internal/storage/memory"
	"motor-autonomo/internal/tool"
	"testing"
	"time"
)

type multiTurnToolProvider struct {
	calls int
}

func (p *multiTurnToolProvider) ID() string {
	return "multiturn-fake"
}

func (p *multiTurnToolProvider) Kind() domain.ProviderKind {
	return domain.ProviderKindOpenAICompatible
}

func (p *multiTurnToolProvider) Profile() domain.ProviderProfile {
	return domain.ProviderProfile{
		MaxOutputTokens:  1024,
		MaxContextTokens: 4096,
	}
}

func (p *multiTurnToolProvider) Complete(ctx context.Context, req port.CompletionRequest) (port.CompletionResult, error) {
	return p.CompleteWithTools(ctx, req, nil)
}

func (p *multiTurnToolProvider) CompleteWithTools(ctx context.Context, req port.CompletionRequest, tools []port.ToolDefinition) (port.CompletionResult, error) {
	p.calls++

	if p.calls == 1 {
		// Turn 1: model wants to use the weather tool
		return port.CompletionResult{
			Model: "multiturn-fake",
			ToolCalls: []port.ToolCall{
				{
					ID:        "tc_123",
					Name:      "get_weather",
					Arguments: `{"location": "San Francisco"}`,
				},
			},
		}, nil
	} else if p.calls == 2 {
		// Turn 2: model sees weather tool result, wants to use the search tool

		foundWeatherFact := false
		for _, fact := range req.Prompt {
			// Facts append to the prompt.
			_ = fact
			foundWeatherFact = true
		}

		if !foundWeatherFact {
			return port.CompletionResult{}, port.ErrConflict // fake error
		}

		return port.CompletionResult{
			Model: "multiturn-fake",
			ToolCalls: []port.ToolCall{
				{
					ID:        "tc_456",
					Name:      "search_web",
					Arguments: `{"query": "hotels in San Francisco"}`,
				},
			},
		}, nil
	} else {
		// Turn 3: model sees both tools, produces a final proposal
		return port.CompletionResult{
			Model: "multiturn-fake",
			Text:  `{"changes":[{"kind":"ADD","entity_type":"observation","entity_id":"obs_final","payload_ref":"payload"}],"expected_delta":"one observation","validator_ids":["schema"]}`,
		}, nil
	}
}

type dummyTool struct {
	def port.ToolDefinition
	res string
}

func (d *dummyTool) Definition() port.ToolDefinition { return d.def }
func (d *dummyTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	return d.res, nil
}

func TestModelExecutorMultiTurn(t *testing.T) {
	now := time.Date(2026, 7, 19, 14, 0, 0, 0, time.UTC).UTC()
	clock := source.NewManualClock(now)
	ids := source.NewSequenceIDGenerator(1)
	store := memory.New()

	ctx := context.Background()
	seedModelAgenda(t, store, now)

	prov := &multiTurnToolProvider{}
	processor, _ := changeset.New(changeset.Config{Store: store, Clock: clock, IDs: ids, PolicyVersion: "policy@model-test"})

	// Catalog setup
	weatherDef := port.ToolDefinition{
		Name:        "get_weather",
		Description: "Get weather",
		Parameters:  []byte(`{"type":"object","properties":{"location":{"type":"string"}}}`),
	}
	searchDef := port.ToolDefinition{
		Name:        "search_web",
		Description: "Search web",
		Parameters:  []byte(`{"type":"object","properties":{"query":{"type":"string"}}}`),
	}
	tools, _ := tool.NewCatalog(
		&dummyTool{def: weatherDef, res: "Sunny, 72F"},
		&dummyTool{def: searchDef, res: "Hotel A, Hotel B"},
	)

	e := ModelExecutor{
		Store:               store,
		Clock:               clock,
		PolicyVersion:       "policy@model-test",
		IDs:                 ids,
		PrimaryProviderID:   "p1",
		PrimaryBindingID:    "b1",
		PrimaryProviderKind: domain.ProviderKindOpenAICompatible,
		Provider:            prov,
		Providers: map[string]port.ModelProvider{
			"b1": prov,
		},
		Changes: processor,
		Compiler: prompt.Compiler{
			Estimator:             prompt.ConservativeEstimator{},
			ProviderContextTokens: 4096,
		},
		Tools: tools,
	}

	res, err := e.Execute(ctx, "operation_model")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !res.Completed {
		t.Errorf("expected Completed=true, got result %#v", res)
	}
	if res.Done {
		t.Errorf("expected Done=false because tool calls were handled internally")
	}

	if res.ModelCalls != 3 {
		t.Errorf("expected 3 model calls, got %d", res.ModelCalls)
	}
}

type infiniteLoopToolProvider struct {
	calls int
}

func (p *infiniteLoopToolProvider) ID() string { return "inf-fake" }
func (p *infiniteLoopToolProvider) Kind() domain.ProviderKind {
	return domain.ProviderKindOpenAICompatible
}
func (p *infiniteLoopToolProvider) Profile() domain.ProviderProfile {
	return domain.ProviderProfile{MaxOutputTokens: 1024, MaxContextTokens: 4096}
}
func (p *infiniteLoopToolProvider) Complete(ctx context.Context, req port.CompletionRequest) (port.CompletionResult, error) {
	return p.CompleteWithTools(ctx, req, nil)
}
func (p *infiniteLoopToolProvider) CompleteWithTools(ctx context.Context, req port.CompletionRequest, tools []port.ToolDefinition) (port.CompletionResult, error) {
	p.calls++
	return port.CompletionResult{
		Model: "inf-fake",
		ToolCalls: []port.ToolCall{
			{ID: "tc_inf", Name: "get_weather", Arguments: `{"location": "San Francisco"}`},
		},
	}, nil
}

func TestModelExecutorInfiniteLoopToolPrevention(t *testing.T) {
	now := time.Date(2026, 7, 19, 14, 0, 0, 0, time.UTC).UTC()
	clock := source.NewManualClock(now)
	ids := source.NewSequenceIDGenerator(1)
	store := memory.New()

	ctx := context.Background()
	seedModelAgenda(t, store, now)

	prov := &infiniteLoopToolProvider{}
	processor, _ := changeset.New(changeset.Config{Store: store, Clock: clock, IDs: ids, PolicyVersion: "policy@model-test"})

	weatherDef := port.ToolDefinition{
		Name:        "get_weather",
		Description: "Get weather",
		Parameters:  []byte(`{"type":"object","properties":{"location":{"type":"string"}}}`),
	}
	tools, _ := tool.NewCatalog(&dummyTool{def: weatherDef, res: "Sunny, 72F"})

	e := ModelExecutor{
		Store:               store,
		Clock:               clock,
		PolicyVersion:       "policy@model-test",
		IDs:                 ids,
		PrimaryProviderID:   "p2",
		PrimaryBindingID:    "b2",
		PrimaryProviderKind: domain.ProviderKindOpenAICompatible,
		Provider:            prov,
		Providers: map[string]port.ModelProvider{
			"b2": prov,
		},
		Changes: processor,
		Compiler: prompt.Compiler{
			Estimator:             prompt.ConservativeEstimator{},
			ProviderContextTokens: 4096,
		},
		Tools: tools,
	}

	res, err := e.Execute(ctx, "operation_model")
	if err == nil {
		t.Fatalf("expected error from exhaustion, got nil")
	}

	if res.Done {
		t.Errorf("expected Done=false on exhaustion, got true")
	}
}
