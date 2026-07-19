package bootstrap

import (
	"time"

	"motor-autonomo/internal/kernel"
	"motor-autonomo/internal/tool"
	"motor-autonomo/internal/tool/subagent"
	"motor-autonomo/internal/tool/yield"
)

// SubagentOptions configures optional subagent orchestration tools.
type SubagentOptions struct {
	Enabled bool
}

// buildSubagent sets up the sessions_spawn and sessions_yield tools if enabled.
func buildSubagent(opts *SubagentOptions, clock interface{ Now() time.Time }) (tool.Provider, error) {
	if opts == nil || !opts.Enabled {
		return nil, nil // Disabled by default
	}

	sm := kernel.NewLocalSessionManager(clock)
	tSpawn := subagent.NewSessionsSpawnTool(sm)
	tYield := yield.NewSessionsYieldTool()

	cat, err := tool.NewCatalog(tSpawn, tYield)
	return cat, err
}
