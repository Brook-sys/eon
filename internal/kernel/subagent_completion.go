package kernel

import (
	"context"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
)

// SubagentCompletionProcessor is responsible for discovering child subagent
// sessions that have reached a terminal state (COMPLETE or FAILED) and
// ingesting those results as ExternalEvents into the parent mission log.
type SubagentCompletionProcessor struct {
	Manager SessionManager
	Store   port.Store
	Clock   interface{ Now() time.Time }
	IDs     interface{ NewID(string) (string, error) }
}

// ProcessCompletedSessions checks pending subagent work.
func (p SubagentCompletionProcessor) ProcessCompletedSessions(ctx context.Context, revID domain.MissionRevisionID) (int, error) {
	// This is a stub for Fase 19 implementation.
	return 0, nil
}
