package bootstrap

import (
	"time"

	"motor-autonomo/internal/kernel"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/runtime/source"
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
	tools := []tool.Tool{tSpawn, tYield}

	cat, err := tool.NewCatalog(tools...)
	return cat, err
}

func buildSubagentRemote(opts *SubagentOptions, clock interface{ Now() time.Time }, peerCaller port.PeerCaller, ids source.IDGenerator, callerID string) (tool.Provider, error) {
	if opts == nil || !opts.Enabled {
		return nil, nil // Disabled by default
	}

	sm := kernel.NewLocalSessionManager(clock)
	tSpawn := subagent.NewSessionsSpawnTool(sm)
	tYield := yield.NewSessionsYieldTool()
	tools := []tool.Tool{tSpawn, tYield}

	if peerCaller != nil && ids != nil && callerID != "" {
		tRemote := subagent.NewRemoteTool(peerCaller, callerID, ids)
		tools = append(tools, tRemote)
	}

	cat, err := tool.NewCatalog(tools...)
	return cat, err
}
