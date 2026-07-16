package inspect_test

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/inspect"
	"motor-autonomo/internal/port"
)

func TestEventStreamSSEEmitsReadyAndExistingEvents(t *testing.T) {
	store, mission, _, now := seedRuntime(t)
	projector, err := inspect.NewProjector(store, inspect.RuntimeIdentity{Name: "motor-autonomo", Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	projector.Clock = func() time.Time { return now }
	api, err := inspect.NewAPI(projector)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(api.Handler())
	t.Cleanup(server.Close)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/events/stream?poll_ms=50&limit=10", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("content-type = %q", ct)
	}

	scanner := bufio.NewScanner(resp.Body)
	// SSE frames can be large; keep a comfortable buffer.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var (
		sawReady bool
		events   []domain.Event
		deadline = time.Now().Add(3 * time.Second)
	)
	for time.Now().Before(deadline) && (!sawReady || len(events) < 2) {
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil && !strings.Contains(err.Error(), "context canceled") {
				t.Fatal(err)
			}
			break
		}
		line := scanner.Text()
		if !strings.HasPrefix(line, "event: ") {
			continue
		}
		name := strings.TrimPrefix(line, "event: ")
		// consume until data line
		var data string
		for scanner.Scan() {
			next := scanner.Text()
			if strings.HasPrefix(next, "data: ") {
				data = strings.TrimPrefix(next, "data: ")
				break
			}
			if next == "" {
				break
			}
		}
		switch name {
		case "ready":
			sawReady = true
		case "event":
			var ev domain.Event
			if err := json.Unmarshal([]byte(data), &ev); err != nil {
				t.Fatalf("decode event: %v data=%s", err, data)
			}
			events = append(events, ev)
		}
	}
	cancel()
	if !sawReady {
		t.Fatal("expected ready SSE event")
	}
	if len(events) < 2 {
		t.Fatalf("expected seeded events on stream, got %d", len(events))
	}
	if events[0].Sequence == 0 || events[0].MissionRevision != mission.ID {
		t.Fatalf("first event = %#v", events[0])
	}
}

func TestEventStreamResumesFromAfterSequence(t *testing.T) {
	store, mission, _, now := seedRuntime(t)
	// Append one more event after the seed pair so resume can skip the first.
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		_, err := tx.AppendEvent(domain.Event{
			SchemaVersion: domain.SchemaVersionV1, ID: "event_extra", Kind: "test.extra",
			OccurredAt: now.Add(2 * time.Second), MissionRevision: mission.ID, PayloadRef: "extra",
		})
		return err
	}); err != nil {
		t.Fatal(err)
	}

	projector, err := inspect.NewProjector(store, inspect.RuntimeIdentity{Name: "motor-autonomo", Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	projector.Clock = func() time.Time { return now }
	api, err := inspect.NewAPI(projector)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(api.Handler())
	t.Cleanup(server.Close)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/events/stream?after_sequence=1&poll_ms=50", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var sequences []uint64
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && len(sequences) < 2 {
		if !scanner.Scan() {
			break
		}
		line := scanner.Text()
		if !strings.HasPrefix(line, "event: event") {
			continue
		}
		for scanner.Scan() {
			next := scanner.Text()
			if strings.HasPrefix(next, "data: ") {
				var ev domain.Event
				if err := json.Unmarshal([]byte(strings.TrimPrefix(next, "data: ")), &ev); err != nil {
					t.Fatal(err)
				}
				sequences = append(sequences, ev.Sequence)
				break
			}
			if next == "" {
				break
			}
		}
	}
	cancel()
	if len(sequences) == 0 {
		t.Fatal("expected events after sequence 1")
	}
	for _, seq := range sequences {
		if seq <= 1 {
			t.Fatalf("stream replayed sequence %d", seq)
		}
	}
}
