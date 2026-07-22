package inspect_test

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/inspect"
	"motor-autonomo/internal/port"
)

type failingReadStore struct{ err error }

func (s failingReadStore) View(context.Context, func(port.Reader) error) error { return s.err }

func TestEventStreamSSEEmitsOneTerminalErrorFrameAndEnds(t *testing.T) {
	projector, err := inspect.NewProjector(failingReadStore{err: errors.New("projection unavailable")}, inspect.RuntimeIdentity{Name: "motor-autonomo", Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	projector.Clock = func() time.Time { return time.Date(2026, 7, 21, 18, 40, 0, 0, time.UTC) }
	api, err := inspect.NewAPI(projector)
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/events/stream?poll_ms=50&limit=10", nil)
	api.Handler().ServeHTTP(recorder, req)

	body := recorder.Body.String()
	if got := strings.Count(body, "event: ready\n"); got != 1 {
		t.Fatalf("ready frames = %d, body=%q", got, body)
	}
	if got := strings.Count(body, "event: terminal_error\n"); got != 1 {
		t.Fatalf("terminal error frames = %d, body=%q", got, body)
	}
	if strings.Contains(body, "event: error\n") {
		t.Fatalf("terminal application failure used reserved EventSource error type: %q", body)
	}
	if !strings.Contains(body, `"code":"stream_list_failed"`) {
		t.Fatalf("terminal error code missing: %q", body)
	}
	if strings.Contains(body, "projection unavailable") {
		t.Fatalf("internal projection error leaked: %q", body)
	}
}

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
		decimals []string
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
			var payload struct {
				domain.Event
				SequenceDecimal string `json:"sequence_decimal"`
			}
			if err := json.Unmarshal([]byte(data), &payload); err != nil {
				t.Fatalf("decode event: %v data=%s", err, data)
			}
			events = append(events, payload.Event)
			decimals = append(decimals, payload.SequenceDecimal)
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
	if decimals[0] != strconv.FormatUint(events[0].Sequence, 10) {
		t.Fatalf("first event decimal sequence = %q, numeric sequence = %d", decimals[0], events[0].Sequence)
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

func TestEventStreamReadyPreservesAcceptedResumeCursor(t *testing.T) {
	store, _, _, now := seedRuntime(t)
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

	for _, tc := range []struct {
		name       string
		queryAfter string
		headerLast string
		wantID     string
	}{
		{name: "query cursor", queryAfter: "1", wantID: "1"},
		{name: "Last-Event-ID wins", queryAfter: "1", headerLast: "2", wantID: "2"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/events/stream?after_sequence="+tc.queryAfter+"&poll_ms=50", nil)
			if err != nil {
				t.Fatal(err)
			}
			if tc.headerLast != "" {
				req.Header.Set("Last-Event-ID", tc.headerLast)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()

			scanner := bufio.NewScanner(resp.Body)
			var eventName, eventID, eventData string
			for scanner.Scan() {
				line := scanner.Text()
				switch {
				case strings.HasPrefix(line, "event: "):
					eventName = strings.TrimPrefix(line, "event: ")
				case strings.HasPrefix(line, "id: "):
					eventID = strings.TrimPrefix(line, "id: ")
				case strings.HasPrefix(line, "data: "):
					eventData = strings.TrimPrefix(line, "data: ")
				case line == "":
					if eventName != "ready" {
						t.Fatalf("first frame = %q, want ready", eventName)
					}
					if eventID != tc.wantID {
						t.Fatalf("ready id = %q, want accepted cursor %q", eventID, tc.wantID)
					}
					var payload struct {
						AfterSequenceDecimal string `json:"after_sequence_decimal"`
					}
					if err := json.Unmarshal([]byte(eventData), &payload); err != nil {
						t.Fatalf("decode ready payload: %v data=%s", err, eventData)
					}
					if payload.AfterSequenceDecimal != tc.wantID {
						t.Fatalf("ready decimal = %q, want accepted cursor %q", payload.AfterSequenceDecimal, tc.wantID)
					}
					return
				}
			}
			if err := scanner.Err(); err != nil {
				t.Fatal(err)
			}
			t.Fatal("stream ended before ready frame")
		})
	}
}

func TestFilteredEventStreamAdvancesAcrossBoundedSparseWindows(t *testing.T) {
	store, _, _, now := seedRuntime(t)
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		for i := 0; i < 5001; i++ {
			if _, err := tx.AppendEvent(domain.Event{
				SchemaVersion: domain.SchemaVersionV1,
				ID:            domain.EventID("event_stream_noise_" + strconv.Itoa(i+1)),
				Kind:          "test.unrelated",
				OccurredAt:    now.Add(time.Duration(i+1) * time.Millisecond),
				PayloadRef:    "noise",
			}); err != nil {
				return err
			}
		}
		_, err := tx.AppendEvent(domain.Event{
			SchemaVersion: domain.SchemaVersionV1,
			ID:            "event_stream_sparse_match",
			Kind:          "test.sparse_match",
			OccurredAt:    now.Add(6 * time.Second),
			PayloadRef:    "match",
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

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/events/stream?kind=test.sparse_match&poll_ms=50&limit=1", nil)
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
	for scanner.Scan() {
		if scanner.Text() != "event: event" {
			continue
		}
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			var event domain.Event
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event); err != nil {
				t.Fatal(err)
			}
			if event.ID != "event_stream_sparse_match" {
				t.Fatalf("event = %#v", event)
			}
			return
		}
	}
	if err := scanner.Err(); err != nil && ctx.Err() == nil {
		t.Fatal(err)
	}
	t.Fatal("filtered stream did not advance beyond the first bounded sparse window")
}
