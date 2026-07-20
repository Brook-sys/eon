package peersync

import (
	"context"
	"errors"

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

var (
	ErrConflictResolverRequired = errors.New("peer sync canonicalization requires a conflict resolver")
	ErrConflictEscalated        = errors.New("peer sync event requires operator escalation")
	ErrInvalidDisposition       = errors.New("peer sync resolver returned an invalid disposition")
)
