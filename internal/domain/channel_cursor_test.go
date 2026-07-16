package domain

import (
	"errors"
	"testing"
	"time"
)

func TestChannelCursorValidateAndAdvance(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	cur, err := InitialChannelCursor("telegram", 0, now)
	if err != nil {
		t.Fatal(err)
	}
	if cur.Revision != 0 || cur.Cursor != 0 {
		t.Fatalf("initial = %#v", cur)
	}
	if err := cur.Validate(); err != nil {
		t.Fatal(err)
	}

	advanced, err := AdvanceChannelCursor(cur, 56, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if advanced.Cursor != 56 || advanced.Revision != 1 {
		t.Fatalf("advanced = %#v", advanced)
	}

	// Equal cursor is pure replay.
	same, err := AdvanceChannelCursor(advanced, 56, now.Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if same.Revision != advanced.Revision || same.Cursor != 56 {
		t.Fatalf("replay = %#v", same)
	}

	// Rewind is conflict.
	if _, err := AdvanceChannelCursor(advanced, 10, now.Add(3*time.Minute)); !errors.Is(err, ErrConflict) {
		t.Fatalf("rewind error = %v", err)
	}

	if _, err := InitialChannelCursor("", 1, now); err == nil {
		t.Fatal("expected empty channel error")
	}
	if _, err := InitialChannelCursor("telegram", -1, now); err == nil {
		t.Fatal("expected negative cursor error")
	}
}
