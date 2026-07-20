package subagent

import (
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/runtime/source"
)

// NewRemoteTool initializes the remote tool wrapping PeerCaller.
func NewRemoteTool(caller port.PeerCaller, callerID string, ids source.IDGenerator) *RemoteTool {
	return &RemoteTool{
		Delegator: &SubagentDelegator{Caller: caller},
		CallerID:  callerID,
		IDGen:     ids,
	}
}
