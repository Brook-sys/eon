package inspect_test

import (
	"context"
	"io"
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
	req = req.WithContext(ctx)

	client := srv.Client()
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(25 * time.Millisecond)
	cancel()

	defer res.Body.Close()

	buf, _ := io.ReadAll(res.Body)
	output := string(buf)

	if !strings.Contains(output, "event_ns_1") || !strings.Contains(output, "event_ns_3") {
		t.Errorf("expected output to contain namespace_alpha events (1 and 3):\n%s", output)
	}

	if strings.Contains(output, "event_ns_0") || strings.Contains(output, "event_ns_2") || strings.Contains(output, "event_ns_4") {
		t.Errorf("expected output to NOT contain namespace_beta events (0, 2, 4):\n%s", output)
	}
}
