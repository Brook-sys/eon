package kernel

import (
	"context"

	"motor-autonomo/internal/domain"
)

// SubagentContinuityStrategy is a bounded adapter reserved for delegable
// continuity work. The first slice deliberately admits nothing until the
// persisted SubagentTask frontier is introduced; this prevents synthetic work.
type SubagentContinuityStrategy struct {
	Sessions SessionManager
}

func (s SubagentContinuityStrategy) Name() string { return "subagent_orchestration" }

func (s SubagentContinuityStrategy) Replenish(context.Context, domain.MissionRevisionID) (ContinuityResult, error) {
	return ContinuityResult{}, nil
}
