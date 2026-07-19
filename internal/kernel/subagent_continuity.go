package kernel

import (
	"context"
	"errors"

	"motor-autonomo/internal/domain"
)

const defaultSubagentDispatchLimit = 4

// SubagentTask is a bounded, persistable work description supplied by the
// continuity frontier. TaskID is also used as the spawn idempotency key.
type SubagentTask struct {
	TaskID      string
	Task        string
	ContextMode string
	Labels      map[string]string
}

// SubagentTaskSource exposes pending work without granting the continuity
// strategy authority to mutate canonical state directly.
type SubagentTaskSource interface {
	PendingSubagentTasks(context.Context, domain.MissionRevisionID, int) ([]SubagentTask, error)
}

// SubagentContinuityFamily dispatches pending subagent work through the
// bounded SessionManager authority boundary.
type SubagentContinuityFamily struct {
	Manager     SessionManager
	Source      SubagentTaskSource
	MaxDispatch int
}

// Name identifies the continuity strategy.
func (f SubagentContinuityFamily) Name() string {
	return "subagent_orchestration"
}

// Replenish admits at most MaxDispatch pending tasks. Spawn is idempotent by
// TaskID, and reaching the manager concurrency limit is a normal bounded stop,
// not a retry loop or a continuity failure.
func (f SubagentContinuityFamily) Replenish(ctx context.Context, revID domain.MissionRevisionID) (ContinuityResult, error) {
	if f.Manager == nil || f.Source == nil {
		return ContinuityResult{}, nil
	}
	limit := f.MaxDispatch
	if limit <= 0 {
		limit = defaultSubagentDispatchLimit
	}
	tasks, err := f.Source.PendingSubagentTasks(ctx, revID, limit)
	if err != nil {
		return ContinuityResult{}, err
	}
	if len(tasks) > limit {
		tasks = tasks[:limit]
	}

	result := ContinuityResult{}
	for _, task := range tasks {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		labels := cloneLabels(task.Labels)
		if labels == nil {
			labels = make(map[string]string, 1)
		}
		labels["task_id"] = task.TaskID
		_, err := f.Manager.Spawn(ctx, SubagentSpec{
			Task:        task.Task,
			ContextMode: task.ContextMode,
			Labels:      labels,
		})
		if errors.Is(err, ErrSessionLimit) {
			break
		}
		if err != nil {
			return result, err
		}
		result.Admitted++
		result.Changed = true
	}
	return result, nil
}

func cloneLabels(labels map[string]string) map[string]string {
	if labels == nil {
		return nil
	}
	copy := make(map[string]string, len(labels))
	for key, value := range labels {
		copy[key] = value
	}
	return copy
}
