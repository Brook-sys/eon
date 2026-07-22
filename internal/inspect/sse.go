package inspect

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
)

const (
	defaultSSEPollInterval = 250 * time.Millisecond
	sseKeepAliveInterval   = 10 * time.Second
	maxSSEImmediatePages   = 8 // bound projection work before yielding to the poll timer
)

type sseDrainPacer struct {
	immediatePages int
}

func sseIdleTicksBeforeKeepAlive(poll time.Duration) int {
	// Round up so the configured interval is a lower bound even when poll does
	// not divide it exactly. Tying keepalive cadence to a fixed tick count made
	// poll_ms=5000 wait 200 seconds, long enough for ordinary proxies to reap an
	// otherwise healthy stream.
	return int((sseKeepAliveInterval + poll - 1) / poll)
}

// sseEventPayload carries the event sequence twice on purpose: Sequence keeps
// the stable inspect JSON shape, while SequenceDecimal gives browser clients an
// exact representation beyond JavaScript's safe integer range. The SSE id and
// SequenceDecimal must describe the same durable log position.
type sseEventPayload struct {
	domain.Event
	SequenceDecimal string `json:"sequence_decimal"`
}

// continueImmediately permits short bursts while a finite backlog remains,
// but forces a timer yield after a bounded number of pages. Without this
// pacing, continuous ingestion can keep HasMore true forever and monopolize a
// handler goroutine in projection/flush work without observing the poll pace.
func (p *sseDrainPacer) continueImmediately(hasMore bool) bool {
	if !hasMore {
		p.immediatePages = 0
		return false
	}
	p.immediatePages++
	if p.immediatePages < maxSSEImmediatePages {
		return true
	}
	p.immediatePages = 0
	return false
}

// StreamEventsHandler streams append-only event pages as Server-Sent Events.
// Clients resume with Last-Event-ID or ?after_sequence=. The stream never mutates state.
func (a *API) StreamEventsHandler() http.HandlerFunc {
	return a.handleEventStream
}

func (a *API) handleEventStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "stream_unsupported", "response writer does not support flushing")
		return
	}

	after, err := parseUint64Default(r.URL.Query().Get("after_sequence"), 0)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_after_sequence", "after_sequence must be an unsigned integer")
		return
	}
	if last := strings.TrimSpace(r.Header.Get("Last-Event-ID")); last != "" {
		parsed, parseErr := strconv.ParseUint(last, 10, 64)
		if parseErr != nil {
			writeError(w, http.StatusBadRequest, "invalid_last_event_id", "Last-Event-ID must be an unsigned integer sequence")
			return
		}
		after = parsed
	}

	limit, err := parseIntDefault(r.URL.Query().Get("limit"), DefaultEventPageLimit)
	if err != nil || limit <= 0 {
		writeError(w, http.StatusBadRequest, "invalid_limit", "limit must be a positive integer")
		return
	}
	if limit > MaxEventPageLimit {
		limit = MaxEventPageLimit
	}

	poll := defaultSSEPollInterval
	if raw := strings.TrimSpace(r.URL.Query().Get("poll_ms")); raw != "" {
		ms, parseErr := strconv.Atoi(raw)
		if parseErr != nil || ms < 50 || ms > 5000 {
			writeError(w, http.StatusBadRequest, "invalid_poll_ms", "poll_ms must be between 50 and 5000")
			return
		}
		poll = time.Duration(ms) * time.Millisecond
	}

	filter := EventFilter{
		AfterSequence:   after,
		Limit:           limit,
		Kind:            r.URL.Query().Get("kind"),
		MissionRevision: domain.MissionRevisionID(r.URL.Query().Get("mission_revision_id")),
		InquiryID:       domain.InquiryID(r.URL.Query().Get("inquiry_id")),
		OperationID:     domain.OperationID(r.URL.Query().Get("operation_id")),
		CommitID:        domain.CommitID(r.URL.Query().Get("commit_id")),
	}

	var highWater uint64
	if err := a.Projector.Store.View(r.Context(), func(reader port.Reader) error {
		highWater = reader.LatestEventSequence()
		return nil
	}); err != nil {
		writeStoreError(w, err)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store, no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()
	if after > highWater {
		// Never certify a resume point beyond the durable log tail. Such a cursor
		// can otherwise leave a syntactically healthy stream permanently blind,
		// especially at max uint64. Keep the mismatch visible and require the
		// operator to choose whether to reset after a store restore/replacement.
		cursor := strconv.FormatUint(highWater, 10)
		_ = writeSSE(w, flusher, "cursor_ahead", cursor, map[string]any{
			"schema_version":          domain.SchemaVersionV1,
			"requested_after_decimal": strconv.FormatUint(after, 10),
			"high_water_decimal":      cursor,
		})
		return
	}

	// Hello event helps the client confirm the stream is live.
	// Preserve the accepted resume cursor in the ready frame. Browsers update
	// Last-Event-ID for every SSE frame carrying an id; resetting it to zero here
	// would make a disconnect before the next event/page replay the log prefix.
	if err := writeSSE(w, flusher, "ready", strconv.FormatUint(after, 10), map[string]any{
		"schema_version":         domain.SchemaVersionV1,
		"after_sequence":         after,
		"after_sequence_decimal": strconv.FormatUint(after, 10),
		"generated_at":           a.Projector.Clock().UTC().Format(time.RFC3339Nano),
		"runtime":                a.Projector.Runtime,
	}); err != nil {
		return
	}

	ctx := r.Context()
	idleTicks := 0
	idleTicksBeforeKeepAlive := sseIdleTicksBeforeKeepAlive(poll)
	drainPacer := sseDrainPacer{}
	for {
		if err := ctx.Err(); err != nil {
			return
		}
		page, listErr := a.Projector.ListEvents(ctx, filter)
		if listErr != nil {
			// "error" is reserved by EventSource for reconnectable transport
			// failures. Keep terminal application failure on a distinct channel so
			// browsers do not confuse a network retry with a command to stop. Carry
			// the last accepted cursor as both SSE id and exact decimal payload so a
			// terminal frame cannot ambiguously advance or rewind browser state.
			cursor := strconv.FormatUint(filter.AfterSequence, 10)
			_ = writeSSE(w, flusher, "terminal_error", cursor, map[string]any{
				"schema_version":         domain.SchemaVersionV1,
				"code":                   "stream_list_failed",
				"message":                "event projection failed",
				"after_sequence_decimal": cursor,
			})
			return
		}
		previousAfter := filter.AfterSequence
		if len(page.Events) > 0 {
			idleTicks = 0
			for _, event := range page.Events {
				id := strconv.FormatUint(event.Sequence, 10)
				if err := writeSSE(w, flusher, "event", id, sseEventPayload{
					Event:           event,
					SequenceDecimal: id,
				}); err != nil {
					return
				}
			}
		}
		// ListEvents may examine thousands of non-matching events after the last
		// emitted match (or return no matches at all). Advance by the examined
		// cursor, not only by emitted event IDs, otherwise a sparse filtered stream
		// can rescan the same bounded window forever and never reach a later match.
		filter.AfterSequence = page.NextSequence
		if len(page.Events) > 0 || page.NextSequence > previousAfter {
			if err := writeSSE(w, flusher, "page", strconv.FormatUint(page.NextSequence, 10), map[string]any{
				"schema_version":        domain.SchemaVersionV1,
				"after_sequence":        page.AfterSequence,
				"next_sequence":         page.NextSequence,
				"next_sequence_decimal": strconv.FormatUint(page.NextSequence, 10),
				"has_more":              page.HasMore,
				"count":                 len(page.Events),
			}); err != nil {
				return
			}
			if drainPacer.continueImmediately(page.HasMore) {
				// Drain finite backlog in bounded bursts. The pacer forces a
				// poll-timer yield if ingestion keeps HasMore true continuously.
				continue
			}
		} else {
			drainPacer.continueImmediately(false)
		}
		if len(page.Events) == 0 {
			idleTicks++
			if idleTicks >= idleTicksBeforeKeepAlive {
				idleTicks = 0
				if _, err := fmt.Fprintf(w, ": keepalive %s\n\n", a.Projector.Clock().UTC().Format(time.RFC3339Nano)); err != nil {
					return
				}
				flusher.Flush()
			}
		}

		timer := time.NewTimer(poll)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func writeSSE(w http.ResponseWriter, flusher http.Flusher, eventName, id string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if eventName != "" {
		if _, err := fmt.Fprintf(w, "event: %s\n", eventName); err != nil {
			return err
		}
	}
	if id != "" {
		if _, err := fmt.Fprintf(w, "id: %s\n", id); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(w, "data: %s\n\n", body); err != nil {
		return err
	}
	flusher.Flush()
	return nil
}
