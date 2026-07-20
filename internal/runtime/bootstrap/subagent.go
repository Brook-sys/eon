package bootstrap

import (
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/kernel"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/runtime/source"
	"motor-autonomo/internal/tool"
	"motor-autonomo/internal/tool/subagent"
	"motor-autonomo/internal/tool/yield"
)

// SubagentOptions configures optional subagent orchestration tools.
type SubagentOptions struct {
	Enabled       bool
	MaxConcurrent int
	MaxAttempts   int
	Timeout       time.Duration
}

// buildSubagent sets up one shared bounded manager, durable lifecycle records,
// and the sessions_spawn / sessions_yield tools when enabled.
func buildSubagent(opts *SubagentOptions, store port.Store, clock interface{ Now() time.Time }, missionID domain.MissionID) (tool.Provider, kernel.SessionManager, error) {
	if opts == nil || !opts.Enabled {
		return nil, nil, nil // Disabled by default
	}
	maxConcurrent := opts.MaxConcurrent
	if maxConcurrent <= 0 {
		maxConcurrent = 4
	}
	local, err := kernel.NewLocalSessionManagerWithPolicy(clock, kernel.SessionPolicy{MaxConcurrent: maxConcurrent})
	if err != nil {
		return nil, nil, err
	}
	sm, err := kernel.NewPersistentSessionManager(local, store, clock, kernel.PersistentSessionPolicy{
		MissionID: missionID, MaxAttempts: opts.MaxAttempts, Timeout: opts.Timeout,
	})
	if err != nil {
		return nil, nil, err
	}
	tSpawn := subagent.NewSessionsSpawnTool(sm)
	tYield := yield.NewSessionsYieldTool()
	tools := []tool.Tool{tSpawn, tYield}

	cat, err := tool.NewCatalog(tools...)
	return cat, sm, err
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
