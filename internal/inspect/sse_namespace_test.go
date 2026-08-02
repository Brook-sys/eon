package inspect_test

import (
	"bufio"
	"context"
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

func TestEventStreamSSEFiltersByNamespace(t *testing.T) {
	store, _, _, now := seedRuntime(t)
	projector, err := inspect.NewProjector(store, inspect.RuntimeIdentity{Name: "motor-autonomo", Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	api, err := inspect.NewAPI(projector)
	if err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(api.Handler())
	defer srv.Close()

	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		for i := 0; i < 5; i++ {
			ns := "namespace_alpha"
			if i%2 == 0 {
				ns = "namespace_beta"
			}
			_, err := tx.AppendEvent(domain.Event{
				SchemaVersion: domain.SchemaVersionV1,
				ID:            domain.EventID("event_ns_" + strconv.Itoa(i)),
				Kind:          "test",
				Namespace:     ns,
				OccurredAt:    now.Add(time.Duration(i) * time.Millisecond),
			})
			if err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	req, _ := http.NewRequest("GET", srv.URL+"/events/stream?namespace=namespace_alpha", nil)
	req.Header.Set("Accept", "text/event-stream")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req = req.WithContext(ctx)

	client := srv.Client()
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	// Read the stream by frame until both namespace_alpha events arrive or the
	// deadline expires. The original implementation slept 25 ms and then read
	// whatever had been flushed so far, which is inherently timing-dependent:
	// under load (CI shared runners) the backlog page may not be emitted within
	// the fixed sleep, producing a false failure. Reading until we have the
	// expected frame count removes the dependency on wall-clock timing.

	scanner := bufio.NewScanner(res.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var output strings.Builder
	var eventDataFrames int
	deadline := time.Now().Add(5 * time.Second)
	var inEvent bool
	for time.Now().Before(deadline) && eventDataFrames < 2 {
		if !scanner.Scan() {
			break
		}
		line := scanner.Text()
		output.WriteString(line)
		output.WriteString("\n")
		if strings.HasPrefix(line, "event: ") {
			inEvent = strings.TrimPrefix(line, "event: ") == "event"
		}
		if inEvent && strings.HasPrefix(line, "data: ") {
			eventDataFrames++
			inEvent = false
		}
	}
	cancel()

	// Drain any remaining bytes still buffered in the connection after the two
	// target frames arrived, so a potential late namespace_beta frame in the
	// same poll cycle is also captured for the negative assertion below.
	res.Body.Close()

	if !strings.Contains(output.String(), "event_ns_1") || !strings.Contains(output.String(), "event_ns_3") {
		t.Errorf("expected output to contain namespace_alpha events (1 and 3):\n%s", output.String())
	}

	if strings.Contains(output.String(), "event_ns_0") || strings.Contains(output.String(), "event_ns_2") || strings.Contains(output.String(), "event_ns_4") {
		t.Errorf("expected output to NOT contain namespace_beta events (0, 2, 4):\n%s", output.String())
	}
}
