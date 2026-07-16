package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// ChannelCursor is a durable, non-authoritative transport checkpoint for one
// channel adapter. It never grants model or capability authority: values only
// help adapters resume external collection (for example Telegram getUpdates
// offset) after process restart.
//
// Monotonicity is enforced on Cursor (non-decreasing) with optimistic
// concurrency on Revision so concurrent writers do not clobber each other.
type ChannelCursor struct {
	SchemaVersion int    `json:"schema_version"`
	Channel       string `json:"channel"`
	// Cursor is the next opaque transport position (Telegram: update_id+1).
	Cursor int64 `json:"cursor"`
	// Revision is the optimistic concurrency token for this channel key.
	Revision  uint64    `json:"revision"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (c ChannelCursor) Validate() error {
	if c.SchemaVersion != SchemaVersionV1 {
		return fmt.Errorf("unsupported channel cursor schema version %d", c.SchemaVersion)
	}
	channel := strings.TrimSpace(c.Channel)
	if channel == "" {
		return errors.New("channel cursor requires channel")
	}
	if len(channel) > 64 {
		return errors.New("channel cursor channel exceeds byte limit")
	}
	if c.Cursor < 0 {
		return errors.New("channel cursor must be non-negative")
	}
	if c.UpdatedAt.IsZero() {
		return errors.New("channel cursor requires updated_at")
	}
	return nil
}

// AdvanceChannelCursor returns the next durable cursor when transport reports a
// higher position. Equal values are a pure replay (ok, same revision). A lower
// candidate is a conflict so adapters do not rewind acknowledged positions.
func AdvanceChannelCursor(current ChannelCursor, nextCursor int64, now time.Time) (ChannelCursor, error) {
	if err := current.Validate(); err != nil {
		return ChannelCursor{}, fmt.Errorf("validate channel cursor: %w", err)
	}
	if now.IsZero() {
		return ChannelCursor{}, errors.New("channel cursor advance requires occurrence time")
	}
	if nextCursor < 0 {
		return ChannelCursor{}, errors.New("channel cursor must be non-negative")
	}
	if nextCursor < current.Cursor {
		return ChannelCursor{}, fmt.Errorf("%w: channel cursor must not decrease", ErrConflict)
	}
	if nextCursor == current.Cursor {
		// Pure replay: no revision bump.
		return current, nil
	}
	out := current
	out.Cursor = nextCursor
	out.Revision = current.Revision + 1
	out.UpdatedAt = now.UTC()
	if err := out.Validate(); err != nil {
		return ChannelCursor{}, err
	}
	return out, nil
}

// InitialChannelCursor seeds a first-seen channel position. Cursor may be zero
// when the transport has not yet reported a position.
func InitialChannelCursor(channel string, cursor int64, now time.Time) (ChannelCursor, error) {
	out := ChannelCursor{
		SchemaVersion: SchemaVersionV1,
		Channel:       strings.TrimSpace(channel),
		Cursor:        cursor,
		Revision:      0,
		UpdatedAt:     now.UTC(),
	}
	if err := out.Validate(); err != nil {
		return ChannelCursor{}, err
	}
	if cursor < 0 {
		return ChannelCursor{}, errors.New("channel cursor must be non-negative")
	}
	return out, nil
}
