package domain

import (
	"errors"
	"strings"
	"time"
)

// PeerSyncMessageKind identifies one frame in the bounded event-sync protocol.
// Synced data is evidence about a peer; it never directly mutates local
// canonical state.
type PeerSyncMessageKind string

const (
	PeerSyncHello      PeerSyncMessageKind = "HELLO"
	PeerSyncPull       PeerSyncMessageKind = "PULL"
	PeerSyncEventBatch PeerSyncMessageKind = "EVENT_BATCH"
	PeerSyncAck        PeerSyncMessageKind = "ACK"
	PeerSyncError      PeerSyncMessageKind = "ERROR"
)

const MaxPeerSyncEvents = 128

// PeerSyncMessage multiplexes independent streams over the existing
// authenticated peer RPC transport. Sequence numbers are scoped to OriginID.
type PeerSyncMessage struct {
	SchemaVersion int                 `json:"schema_version"`
	StreamID      string              `json:"stream_id"`
	MessageID     string              `json:"message_id"`
	Kind          PeerSyncMessageKind `json:"kind"`
	OriginID      string              `json:"origin_id"`
	AfterSequence uint64              `json:"after_sequence,omitempty"`
	NextSequence  uint64              `json:"next_sequence,omitempty"`
	Events        []Event             `json:"events,omitempty"`
	ErrorCode     string              `json:"error_code,omitempty"`
}

func (m PeerSyncMessage) Validate() error {
	if m.SchemaVersion != SchemaVersionV1 || !validPeerSyncID(m.StreamID) || !validPeerSyncID(m.MessageID) || !validPeerSyncID(m.OriginID) {
		return errors.New("peer sync message identity is invalid")
	}
	if len(m.Events) > MaxPeerSyncEvents {
		return errors.New("peer sync event batch exceeds limit")
	}
	switch m.Kind {
	case PeerSyncHello:
		if m.AfterSequence != 0 || m.NextSequence != 0 || len(m.Events) != 0 || m.ErrorCode != "" {
			return errors.New("HELLO contains invalid fields")
		}
	case PeerSyncPull:
		if m.NextSequence != 0 || len(m.Events) != 0 || m.ErrorCode != "" {
			return errors.New("PULL contains invalid fields")
		}
	case PeerSyncEventBatch:
		if m.ErrorCode != "" || m.NextSequence < m.AfterSequence {
			return errors.New("EVENT_BATCH cursor is invalid")
		}
		previous := m.AfterSequence
		for _, event := range m.Events {
			if event.ValidatePersisted() != nil || event.Sequence <= previous || event.Sequence > m.NextSequence {
				return errors.New("EVENT_BATCH contains invalid event sequence")
			}
			previous = event.Sequence
		}
		if len(m.Events) == 0 && m.NextSequence != m.AfterSequence {
			return errors.New("empty EVENT_BATCH advances cursor")
		}
	case PeerSyncAck:
		if m.AfterSequence != 0 || len(m.Events) != 0 || m.ErrorCode != "" {
			return errors.New("ACK contains invalid fields")
		}
	case PeerSyncError:
		if m.AfterSequence != 0 || m.NextSequence != 0 || len(m.Events) != 0 || !validPeerSyncID(m.ErrorCode) {
			return errors.New("ERROR contains invalid fields")
		}
	default:
		return errors.New("unknown peer sync message kind")
	}
	return nil
}

func validPeerSyncID(value string) bool {
	return value != "" && len(value) <= 128 && strings.TrimSpace(value) == value
}

// PeerAgendaReplica is an authority-free projection received from a peer.
// Version is a source-local logical counter; UpdatedAt is evidence only and is
// used after Version for deterministic convergence, never as local authority.
type PeerAgendaReplica struct {
	SchemaVersion int       `json:"schema_version"`
	OriginID      string    `json:"origin_id"`
	EntityID      string    `json:"entity_id"`
	Version       uint64    `json:"version"`
	UpdatedAt     time.Time `json:"updated_at"`
	State         string    `json:"state"`
	PayloadHash   string    `json:"payload_hash"`
}

func (r PeerAgendaReplica) Validate() error {
	if r.SchemaVersion != SchemaVersionV1 || !validPeerSyncID(r.OriginID) || !validPeerSyncID(r.EntityID) || r.Version == 0 || r.UpdatedAt.IsZero() || !validPeerSyncID(r.State) || !validPeerSyncID(r.PayloadHash) {
		return errors.New("peer agenda replica is invalid")
	}
	return nil
}

// ResolvePeerAgendaReplica converges records only within the same peer-owned
// namespace. Cross-origin records cannot overwrite one another. Equal-version
// divergent payloads are resolved deterministically but reported as conflict.
func ResolvePeerAgendaReplica(current, incoming PeerAgendaReplica) (PeerAgendaReplica, bool, error) {
	if err := current.Validate(); err != nil {
		return PeerAgendaReplica{}, false, err
	}
	if err := incoming.Validate(); err != nil {
		return PeerAgendaReplica{}, false, err
	}
	if current.OriginID != incoming.OriginID || current.EntityID != incoming.EntityID {
		return PeerAgendaReplica{}, false, errors.New("peer agenda replica namespace mismatch")
	}
	if incoming.Version > current.Version {
		return incoming, false, nil
	}
	if incoming.Version < current.Version {
		return current, false, nil
	}
	if incoming.State == current.State && incoming.PayloadHash == current.PayloadHash {
		if incoming.UpdatedAt.After(current.UpdatedAt) {
			return incoming, false, nil
		}
		return current, false, nil
	}
	// Same logical version with different content is a protocol conflict. Pick a
	// stable winner so replicas converge while preserving the conflict signal.
	if replicaOrder(incoming) > replicaOrder(current) {
		return incoming, true, nil
	}
	return current, true, nil
}

func replicaOrder(r PeerAgendaReplica) string {
	return r.UpdatedAt.UTC().Format(time.RFC3339Nano) + "\x00" + r.PayloadHash + "\x00" + r.State
}
