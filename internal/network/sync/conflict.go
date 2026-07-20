package peersync

import (
	"context"
	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
)

type EventConflictResolver interface {
	ResolveConflict(ctx context.Context, local port.Reader, remote domain.Event) (ConflictDisposition, error)
}

type ConflictDisposition string

const (
	DispositionApply    ConflictDisposition = "APPLY"
	DispositionDiscard  ConflictDisposition = "DISCARD"
	DispositionEscalate ConflictDisposition = "ESCALATE"
)
