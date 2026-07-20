package peersync

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
)

const Capability = "event.sync.v1"

var (
	ErrInvalidPeerIdentity = errors.New("peer sync origin does not match authenticated caller")
	ErrCursorGap           = errors.New("peer sync batch does not continue durable cursor")
	ErrUnexpectedResponse  = errors.New("unexpected peer sync response")
)

// Handler is the narrow authority-free sync surface attached to the router.
type Handler interface {
	Handle(context.Context, string, string, domain.PeerSyncMessage) (domain.PeerSyncMessage, error)
}

// Service stores authenticated remote frames as evidence and serves bounded
// pulls from the local event log. It has no API that applies remote events to
// canonical local state.
type Service struct {
	store port.Store
	now   func() time.Time
}

// PullCheckpoint identifies crash-test boundaries in the outbound
// pull -> durable commit -> acknowledgement flow.
type PullCheckpoint string

const (
	PullAfterResponse PullCheckpoint = "AFTER_RESPONSE"
	PullAfterCommit   PullCheckpoint = "AFTER_COMMIT"
)

// PullResult reports transport progress only. Remote events remain in the
// authority-free inbox and are not applied to canonical state.
type PullResult struct {
	Events       int
	NextSequence uint64
}

func NewService(store port.Store, now func() time.Time) (*Service, error) {
	if store == nil || now == nil {
		return nil, ErrInvalidFrame
	}
	return &Service{store: store, now: now}, nil
}

// PullOnce performs one bounded outbound sync step. Before requesting new
// data it re-sends the acknowledgement implied by the durable inbound cursor;
// this makes a crash after commit but before ACK recoverable without a new
// pending-ack table. ACK and PULL identifiers are deterministic per
// peer/stream/sequence, so retries are idempotent at the remote inbox.
func (s *Service) PullOnce(ctx context.Context, caller port.PeerCaller, peerID, localID, streamID string, checkpoint func(PullCheckpoint) error) (PullResult, error) {
	if caller == nil || !validSyncID(peerID) || !validSyncID(localID) || !validSyncID(streamID) || peerID == localID {
		return PullResult{}, ErrInvalidPeerIdentity
	}
	after, exists, err := s.inboundPosition(ctx, peerID, streamID)
	if err != nil {
		return PullResult{}, err
	}
	if exists {
		ack := domain.PeerSyncMessage{SchemaVersion: domain.SchemaVersionV1, StreamID: streamID, MessageID: syncMessageID("ack", peerID, streamID, after), Kind: domain.PeerSyncAck, OriginID: localID, NextSequence: after}
		if _, err := callPeerSync(ctx, caller, peerID, ack); err != nil {
			return PullResult{}, fmt.Errorf("resume peer sync ack: %w", err)
		}
	}
	pull := domain.PeerSyncMessage{SchemaVersion: domain.SchemaVersionV1, StreamID: streamID, MessageID: syncMessageID("pull", peerID, streamID, after), Kind: domain.PeerSyncPull, OriginID: localID, AfterSequence: after}
	batch, err := callPeerSync(ctx, caller, peerID, pull)
	if err != nil {
		return PullResult{}, fmt.Errorf("pull peer sync batch: %w", err)
	}
	if batch.Kind != domain.PeerSyncEventBatch || batch.OriginID != peerID || batch.StreamID != streamID || batch.MessageID != pull.MessageID || batch.AfterSequence != after {
		return PullResult{}, ErrUnexpectedResponse
	}
	if checkpoint != nil {
		if err := checkpoint(PullAfterResponse); err != nil {
			return PullResult{}, err
		}
	}
	ack, err := s.Handle(ctx, peerID, localID, batch)
	if err != nil {
		return PullResult{}, fmt.Errorf("commit peer sync batch: %w", err)
	}
	if checkpoint != nil {
		if err := checkpoint(PullAfterCommit); err != nil {
			return PullResult{}, err
		}
	}
	if _, err := callPeerSync(ctx, caller, peerID, ack); err != nil {
		return PullResult{}, fmt.Errorf("ack peer sync batch: %w", err)
	}
	return PullResult{Events: len(batch.Events), NextSequence: batch.NextSequence}, nil
}

func (s *Service) inboundPosition(ctx context.Context, peerID, streamID string) (uint64, bool, error) {
	var cursor domain.PeerSyncCursor
	err := s.store.View(ctx, func(reader port.Reader) error {
		var err error
		cursor, err = reader.PeerSyncCursor(peerID, peerID, streamID, domain.PeerSyncInbound)
		return err
	})
	if errors.Is(err, port.ErrNotFound) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return cursor.NextSequence, true, nil
}

func callPeerSync(ctx context.Context, caller port.PeerCaller, peerID string, message domain.PeerSyncMessage) (domain.PeerSyncMessage, error) {
	payload, err := Encode(message)
	if err != nil {
		return domain.PeerSyncMessage{}, err
	}
	response, err := caller.Call(ctx, domain.PeerRPCRequest{RequestID: syncMessageID("rpc", peerID, message.StreamID, message.NextSequence+message.AfterSequence), PeerID: peerID, Capability: Capability, Payload: payload})
	if err != nil {
		return domain.PeerSyncMessage{}, err
	}
	decoded, err := Decode(response.Payload)
	if err != nil {
		return domain.PeerSyncMessage{}, ErrUnexpectedResponse
	}
	return decoded, nil
}

func syncMessageID(kind, peerID, streamID string, sequence uint64) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%s\x00%s\x00%d", kind, peerID, streamID, sequence)))
	return fmt.Sprintf("%s-%x", kind, sum[:12])
}

func validSyncID(value string) bool {
	return value != "" && len(value) <= 128 && value == strings.TrimSpace(value)
}

func (s *Service) Handle(ctx context.Context, callerID, localID string, message domain.PeerSyncMessage) (domain.PeerSyncMessage, error) {
	if err := message.Validate(); err != nil {
		return domain.PeerSyncMessage{}, ErrInvalidFrame
	}
	if callerID != message.OriginID || localID == "" {
		return domain.PeerSyncMessage{}, ErrInvalidPeerIdentity
	}
	switch message.Kind {
	case domain.PeerSyncHello:
		if err := s.record(ctx, message, nil); err != nil {
			return domain.PeerSyncMessage{}, err
		}
		return domain.PeerSyncMessage{SchemaVersion: domain.SchemaVersionV1, StreamID: message.StreamID, MessageID: message.MessageID, Kind: domain.PeerSyncAck, OriginID: localID}, nil
	case domain.PeerSyncPull:
		var events []domain.Event
		if err := s.store.View(ctx, func(reader port.Reader) error {
			var err error
			events, err = reader.Events(message.AfterSequence, domain.MaxPeerSyncEvents)
			return err
		}); err != nil {
			return domain.PeerSyncMessage{}, err
		}
		next := message.AfterSequence
		if len(events) > 0 {
			next = events[len(events)-1].Sequence
		}
		return domain.PeerSyncMessage{SchemaVersion: domain.SchemaVersionV1, StreamID: message.StreamID, MessageID: message.MessageID, Kind: domain.PeerSyncEventBatch, OriginID: localID, AfterSequence: message.AfterSequence, NextSequence: next, Events: events}, nil
	case domain.PeerSyncEventBatch:
		var cursor domain.PeerSyncCursor
		err := s.store.Update(ctx, func(tx port.Transaction) error {
			_, created, err := s.putInTx(tx, callerID, message)
			if err != nil || !created {
				return err
			}
			current, cursorErr := tx.PeerSyncCursor(callerID, message.OriginID, message.StreamID, domain.PeerSyncInbound)
			cursorExists := cursorErr == nil
			if cursorErr != nil && !errors.Is(cursorErr, port.ErrNotFound) {
				return cursorErr
			}
			if cursorErr == nil {
				cursor = current
			}
			if message.AfterSequence != cursor.NextSequence {
				return ErrCursorGap
			}
			var next domain.PeerSyncCursor
			if cursorExists {
				next, err = domain.AdvancePeerSyncCursor(cursor, message.NextSequence, s.now().UTC())
			} else {
				next, err = domain.InitialPeerSyncCursor(callerID, message.OriginID, message.StreamID, domain.PeerSyncInbound, message.NextSequence, s.now().UTC())
			}
			if err != nil {
				return err
			}
			return tx.SavePeerSyncCursor(next, cursor.Revision)
		})
		if err != nil {
			return domain.PeerSyncMessage{}, err
		}
		return domain.PeerSyncMessage{SchemaVersion: domain.SchemaVersionV1, StreamID: message.StreamID, MessageID: message.MessageID, Kind: domain.PeerSyncAck, OriginID: localID, NextSequence: message.NextSequence}, nil
	case domain.PeerSyncAck:
		err := s.store.Update(ctx, func(tx port.Transaction) error {
			_, created, err := s.putInTx(tx, callerID, message)
			if err != nil || !created {
				return err
			}
			current, cursorErr := tx.PeerSyncCursor(callerID, localID, message.StreamID, domain.PeerSyncOutboundAck)
			cursorExists := cursorErr == nil
			if cursorErr != nil && !errors.Is(cursorErr, port.ErrNotFound) {
				return cursorErr
			}
			var next domain.PeerSyncCursor
			if cursorExists {
				next, err = domain.AdvancePeerSyncCursor(current, message.NextSequence, s.now().UTC())
			} else {
				next, err = domain.InitialPeerSyncCursor(callerID, localID, message.StreamID, domain.PeerSyncOutboundAck, message.NextSequence, s.now().UTC())
			}
			if err != nil {
				return err
			}
			return tx.SavePeerSyncCursor(next, current.Revision)
		})
		if err != nil {
			return domain.PeerSyncMessage{}, err
		}
		return domain.PeerSyncMessage{SchemaVersion: domain.SchemaVersionV1, StreamID: message.StreamID, MessageID: message.MessageID, Kind: domain.PeerSyncAck, OriginID: localID, NextSequence: message.NextSequence}, nil
	case domain.PeerSyncError:
		if err := s.record(ctx, message, nil); err != nil {
			return domain.PeerSyncMessage{}, err
		}
		return domain.PeerSyncMessage{SchemaVersion: domain.SchemaVersionV1, StreamID: message.StreamID, MessageID: message.MessageID, Kind: domain.PeerSyncAck, OriginID: localID, NextSequence: message.NextSequence}, nil
	default:
		return domain.PeerSyncMessage{}, ErrInvalidFrame
	}
}

func (s *Service) record(ctx context.Context, message domain.PeerSyncMessage, extra func(port.Transaction) error) error {
	return s.store.Update(ctx, func(tx port.Transaction) error {
		if err := s.recordInTx(tx, message); err != nil {
			return err
		}
		if extra != nil {
			return extra(tx)
		}
		return nil
	})
}

func (s *Service) recordInTx(tx port.Transaction, message domain.PeerSyncMessage) error {
	_, _, err := s.putInTx(tx, message.OriginID, message)
	return err
}

func (s *Service) putInTx(tx port.Transaction, peerID string, message domain.PeerSyncMessage) (domain.PeerSyncInboxRecord, bool, error) {
	record := domain.PeerSyncInboxRecord{SchemaVersion: domain.SchemaVersionV1, PeerID: peerID, OriginID: message.OriginID, MessageID: message.MessageID, StreamID: message.StreamID, ReceivedAt: s.now().UTC(), Message: message}
	stored, created, err := tx.PutPeerSyncInboxRecord(record)
	if err != nil {
		return domain.PeerSyncInboxRecord{}, false, fmt.Errorf("save peer sync inbox: %w", err)
	}
	return stored, created, nil
}
