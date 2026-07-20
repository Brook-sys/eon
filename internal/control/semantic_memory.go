package control

import (
	"context"
	"errors"
	"fmt"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/runtime/source"
)

// SemanticMemory persists the current semantic-memory view and its audit event
// in one store transaction. Model output never calls this service directly;
// the control surface supplies operator-authored memories only.
type SemanticMemory struct {
	Store port.Store
	Clock source.Clock
	IDs   source.IDGenerator
}

func NewSemanticMemory(store port.Store, clock source.Clock, ids source.IDGenerator) (*SemanticMemory, error) {
	if store == nil || clock == nil || ids == nil {
		return nil, errors.New("semantic memory requires store, clock, and ID generator")
	}
	return &SemanticMemory{Store: store, Clock: clock, IDs: ids}, nil
}

func (s *SemanticMemory) SaveMemory(ctx context.Context, memory domain.LongTermMemory) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	memory.StoredAt = s.Clock.Now().UTC()
	if err := memory.Validate(); err != nil {
		return err
	}
	eventID, err := s.IDs.NewID("event")
	if err != nil {
		return fmt.Errorf("generate memory event ID: %w", err)
	}
	event, err := (domain.MemoryStoredEvent{
		MemoryID: memory.ID,
		Key:      memory.Key,
		Scope:    memory.Scope,
		At:       memory.StoredAt,
	}).Event(domain.EventID(eventID))
	if err != nil {
		return fmt.Errorf("build memory stored event: %w", err)
	}
	return s.Store.Update(ctx, func(tx port.Transaction) error {
		if err := tx.SaveMemory(memory); err != nil {
			return err
		}
		_, err := tx.AppendEvent(event)
		return err
	})
}

func (s *SemanticMemory) DeleteMemory(ctx context.Context, id domain.MemoryID, reason string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	eventID, err := s.IDs.NewID("event")
	if err != nil {
		return false, fmt.Errorf("generate memory event ID: %w", err)
	}
	event, err := (domain.MemoryCompactedEvent{
		MemoryID: id,
		Reason:   reason,
		At:       s.Clock.Now(),
	}).Event(domain.EventID(eventID))
	if err != nil {
		return false, fmt.Errorf("build memory compacted event: %w", err)
	}

	deleted := false
	err = s.Store.Update(ctx, func(tx port.Transaction) error {
		for _, scope := range []domain.MemoryScope{domain.MemoryScopeMission, domain.MemoryScopeStrategy, domain.MemoryScopeAgent} {
			memories, err := tx.ListMemoriesByScope(scope)
			if err != nil {
				return err
			}
			for _, memory := range memories {
				if memory.ID == id {
					deleted = true
					break
				}
			}
			if deleted {
				break
			}
		}
		if !deleted {
			return nil
		}
		if err := tx.DeleteMemory(id); err != nil {
			return err
		}
		_, err := tx.AppendEvent(event)
		return err
	})
	if err != nil {
		return false, err
	}
	return deleted, err
}
