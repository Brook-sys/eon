package inspect

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"motor-autonomo/internal/domain"
)

const (
	defaultSSEPollInterval = 250 * time.Millisecond
	maxSSEIdleTicks        = 40 // ~10s with default poll; send keep-alive comment
)

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

	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store, no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	// Hello event helps the client confirm the stream is live.
	// Preserve the accepted resume cursor in the ready frame. Browsers update
	// Last-Event-ID for every SSE frame carrying an id; resetting it to zero here
	// would make a disconnect before the next event/page replay the log prefix.
	if err := writeSSE(w, flusher, "ready", strconv.FormatUint(after, 10), map[string]any{
		"schema_version": domain.SchemaVersionV1,
		"after_sequence": after,
		"generated_at":   a.Projector.Clock().UTC().Format(time.RFC3339Nano),
		"runtime":        a.Projector.Runtime,
	}); err != nil {
		return
	}

	ctx := r.Context()
	idleTicks := 0
	for {
		if err := ctx.Err(); err != nil {
			return
		}
		page, listErr := a.Projector.ListEvents(ctx, filter)
		if listErr != nil {
			_ = writeSSE(w, flusher, "error", "", map[string]any{
				"code":    "stream_list_failed",
				"message": "event projection failed",
			})
			return
		}
		previousAfter := filter.AfterSequence
		if len(page.Events) > 0 {
			idleTicks = 0
			for _, event := range page.Events {
				id := strconv.FormatUint(event.Sequence, 10)
				if err := writeSSE(w, flusher, "event", id, event); err != nil {
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
				"after_sequence": page.AfterSequence,
				"next_sequence":  page.NextSequence,
				"has_more":       page.HasMore,
				"count":          len(page.Events),
			}); err != nil {
				return
			}
			if page.HasMore {
				// Drain without sleeping while backlog remains.
				continue
			}
		}
		if len(page.Events) == 0 {
			idleTicks++
			if idleTicks >= maxSSEIdleTicks {
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
