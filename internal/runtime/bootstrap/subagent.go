package bootstrap

import (
	"context"
	"errors"
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
	// TransportPeerID pins every admitted session to one deployment-authorized
	// mTLS peer. It is not exposed as a model-controlled tool argument.
	TransportPeerID string
}

// buildSubagent sets up one shared bounded manager, durable lifecycle records,
// and the sessions_spawn / sessions_yield tools when enabled.
func buildSubagent(opts *SubagentOptions, store port.Store, clock interface{ Now() time.Time }, ids source.IDGenerator, missionID domain.MissionID) (tool.Provider, *kernel.PersistentSessionManager, error) {
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
	if err := restoreSubagents(context.Background(), store, local); err != nil {
		return nil, nil, err
	}
	sm, err := kernel.NewPersistentSessionManager(local, store, clock, ids, kernel.PersistentSessionPolicy{
		MissionID: missionID, MaxAttempts: opts.MaxAttempts, Timeout: opts.Timeout,
	})
	if err != nil {
		return nil, nil, err
	}
	trustedLabels := map[string]string(nil)
	if opts.TransportPeerID != "" {
		trustedLabels = map[string]string{kernel.SubagentTransportPeerLabel: opts.TransportPeerID}
	}
	tSpawn := subagent.NewSessionsSpawnToolWithTrustedLabels(sm, trustedLabels)
	tYield := yield.NewSessionsYieldTool()
	tools := []tool.Tool{tSpawn, tYield}

	cat, err := tool.NewCatalog(tools...)
	return cat, sm, err
}

func restoreSubagents(ctx context.Context, store port.Store, manager kernel.SessionManager) error {
	return store.View(ctx, func(reader port.Reader) error {
		for _, state := range []domain.SubagentState{domain.SubagentStatePending, domain.SubagentStateRunning} {
			records, err := reader.SubagentRecordsByState(state, 0)
			if err != nil {
				return err
			}
			for _, record := range records {
				sessionState := kernel.SessionStatePending
				if record.State == domain.SubagentStateRunning {
					sessionState = kernel.SessionStateRunning
				}
				labels := map[string]string{"task_id": record.TaskID}
				if record.TransportPeerID != "" {
					labels[kernel.SubagentTransportPeerLabel] = record.TransportPeerID
				}
				status := kernel.SubagentStatus{
					ID:        kernel.SessionID(record.ID),
					Attempt:   record.Attempt,
					State:     sessionState,
					Spec:      kernel.SubagentSpec{Task: record.Task, ContextMode: record.ContextMode, Labels: labels},
					StartedAt: record.StartedAt,
				}
				if err := manager.Restore(ctx, status); err != nil {
					return err
				}
				// Receiver execution commits its terminal receipt before publishing to
				// the process-local manager. Rehydrate that durable terminal first so a
				// crash in between cannot leave the receiver active until its deadline.
				terminal, err := reader.TerminalSubagentSpawnReceiptForReceiver(record.ID, record.Attempt)
				if err == nil {
					observation, err := kernel.TerminalSubagentObservation(terminal)
					if err != nil {
						return err
					}
					if err := manager.PublishStatus(ctx, observation); err != nil {
						return err
					}
					continue
				}
				if !errors.Is(err, port.ErrNotFound) {
					return err
				}
				winner, err := reader.AppliedSubagentStatusIngressWinner(record.ID, record.Attempt)
				if err != nil {
					if errors.Is(err, port.ErrNotFound) {
						continue
					}
					return err
				}
				if err := manager.PublishStatus(ctx, kernel.SubagentObservation{ID: kernel.SessionID(winner.SessionID), Attempt: winner.Attempt, State: kernel.SessionState(winner.State), Result: winner.Result, Failure: winner.Failure}); err != nil {
					return err
				}
			}
		}
		return nil
	})
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
