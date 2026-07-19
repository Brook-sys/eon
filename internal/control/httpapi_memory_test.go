package control_test

import (
	"bytes"
	"encoding/json"
	"motor-autonomo/internal/control"
	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/runtime/source"
	"motor-autonomo/internal/storage/memory"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSemanticMemoryEndpoints(t *testing.T) {
	store := memory.New()
	cmdInbox, _ := control.NewCommandInbox(store, control.ReceiptFactoryFrom(source.NewManualClock(time.Now()), source.NewSequenceIDGenerator(1)))
	evtInbox, _ := control.NewExternalEventInbox(store, control.DispositionFactoryFrom(source.NewManualClock(time.Now())))
	api, err := control.NewAPI(
		cmdInbox,
		evtInbox,
		source.NewManualClock(time.Now()),
		source.NewSequenceIDGenerator(1),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	api.SemanticMemory, _ = control.NewSemanticMemory(store, source.NewManualClock(time.Now()), source.NewSequenceIDGenerator(100))
	api.SemanticMemoryReader = store

	server := httptest.NewServer(api.Handler())
	defer server.Close()

	// 1. Submit a memory
	submitPayload := map[string]interface{}{
		"id":    "mem_1",
		"key":   "test_key",
		"scope": "mission",
		"value": "Important context",
	}
	body, _ := json.Marshal(submitPayload)
	resp, err := http.Post(server.URL+"/memories", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("failed to post memory: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// 2. List memories (no scope)
	resp, err = http.Get(server.URL + "/memories")
	if err != nil {
		t.Fatalf("failed to get memories: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}
	var memories []domain.LongTermMemory
	if err := json.NewDecoder(resp.Body).Decode(&memories); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	resp.Body.Close()

	if len(memories) != 1 || memories[0].ID != "mem_1" {
		t.Fatalf("expected 1 memory with ID mem_1, got %+v", memories)
	}

	// 3. List memories (with scope filter)
	resp, err = http.Get(server.URL + "/memories?scope=strategy")
	if err != nil {
		t.Fatalf("failed to get memories: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}
	var stratMemories []domain.LongTermMemory
	if err := json.NewDecoder(resp.Body).Decode(&stratMemories); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	resp.Body.Close()

	if len(stratMemories) != 0 {
		t.Fatalf("expected 0 strategy memories, got %d", len(stratMemories))
	}

	// 4. Delete memory
	req, _ := http.NewRequest(http.MethodDelete, server.URL+"/memories/mem_1", nil)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to delete memory: %v", err)
	}
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// 5. Verify deletion
	resp, err = http.Get(server.URL + "/memories")
	if err != nil {
		t.Fatalf("failed to get memories after deletion: %v", err)
	}
	if err := json.NewDecoder(resp.Body).Decode(&memories); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	resp.Body.Close()

	if len(memories) != 0 {
		t.Fatalf("expected 0 memories after deletion, got %d", len(memories))
	}
}
