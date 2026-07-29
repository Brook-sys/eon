package bootstrap

import (
	"context"
	"fmt"

	"motor-autonomo/internal/kernel"
	"motor-autonomo/internal/tool"
)

// reloadModelExecutorIfNeeded reconstrui o ModelExecutor atomicamente se a
// revisão ativa de MODELS mudar entre ciclos.
func (rt *Runtime) reloadModelExecutorIfNeeded(ctx context.Context) error {
	models, found, err := kernel.ActiveModelsConfig(ctx, rt.Store)
	if err != nil {
		return err
	}
	var currentVersion string
	if rt.Model != nil && rt.Model.ModelsConfig != nil {
		currentVersion = rt.Model.ModelsConfig.Version
	}
	var activeVersion string
	if found {
		activeVersion = models.Version
	}
	if currentVersion != activeVersion {
		rt.logger.Printf("runtime: models config reload detected (current=%q new=%q), rebuilding executor", currentVersion, activeVersion)
		modelExec, err := BuildModelExecutor(rt.Opts, rt.Store, rt.Clock, rt.IDs, rt.Telemetry)
		if err != nil {
			rt.logger.Printf("runtime: model executor rebuild failed: %v", err)
			return err
		}
		if modelExec != nil && rt.subagentTools != nil {
			merged, mergeErr := tool.MergeProviders(modelExec.Tools, rt.subagentTools)
			if mergeErr != nil {
				rt.logger.Printf("runtime: model executor tool merge failed: %v", mergeErr)
				return fmt.Errorf("merge model and subagent tools after reload: %w", mergeErr)
			}
			modelExec.Tools = merged
		}
		rt.mu.Lock()
		rt.Model = modelExec
		rt.Executor.Model = modelExec
		rt.mu.Unlock()
	}
	return nil
}
