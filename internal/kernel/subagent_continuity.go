package kernel

import (
	"context"

	"motor-autonomo/internal/domain"
)

// SubagentContinuityFamily implementa ContinuityStrategy para orquestrar
// tarefas de sub-agente (SubagentTasks) não terminadas no nível do motor.
type SubagentContinuityFamily struct {
	Manager SessionManager
}

// Name identifica a estratégia de continuidade.
func (f SubagentContinuityFamily) Name() string {
	return "subagent_orchestration"
}

// Replenish varre tarefas de subagentes não terminadas e as orquestra.
// No atual nível de abstração estrutural, atua como mock placeholder.
func (f SubagentContinuityFamily) Replenish(ctx context.Context, revID domain.MissionRevisionID) (ContinuityResult, error) {
	return ContinuityResult{}, nil
}
