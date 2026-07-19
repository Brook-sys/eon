package bootstrap

import (
	"time"

	"motor-autonomo/internal/kernel"
	"motor-autonomo/internal/tool"
	"motor-autonomo/internal/tool/subagent"
)

// SubagentOptions configures optional subagent orchestration tools.
type SubagentOptions struct {
	Enabled bool
}

// buildSubagent sets up the sessions_spawn tool if enabled.
func buildSubagent(opts *SubagentOptions, clock interface{ Now() time.Time }) (tool.Provider, error) {
	if opts == nil || !opts.Enabled {
		return nil, nil // Disabled by default
	}

	sm := kernel.NewLocalSessionManager(clock)
	t := subagent.NewSessionsSpawnTool(sm)

	cat, err := tool.NewCatalog(t)
	return cat, err
}
