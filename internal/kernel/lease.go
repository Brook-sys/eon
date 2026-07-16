package kernel

import (
	"fmt"
	"strings"
	"time"

	"motor-autonomo/internal/domain"
)

const leaseUntilMarker = ":until="

// FormatLeaseRef binds a lease identity to operation, attempt, and absolute
// expiry so crash reconcilers can decide without process-local memory
// (FR-DUR-003).
func FormatLeaseRef(leaseID string, operationID domain.OperationID, attempt uint32, until time.Time) string {
	return fmt.Sprintf("%s:op=%s:attempt=%d%s%s",
		strings.TrimSpace(leaseID),
		operationID,
		attempt,
		leaseUntilMarker,
		until.UTC().Format(time.RFC3339Nano),
	)
}

// ParseLeaseDeadline extracts the absolute expiry from a lease reference.
// Legacy refs without :until= return ok=false so reconcilers can skip them
// rather than invent a deadline.
func ParseLeaseDeadline(ref string) (time.Time, bool) {
	ref = strings.TrimSpace(ref)
	i := strings.LastIndex(ref, leaseUntilMarker)
	if i < 0 {
		return time.Time{}, false
	}
	raw := ref[i+len(leaseUntilMarker):]
	if raw == "" {
		return time.Time{}, false
	}
	if t, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return t.UTC(), true
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t.UTC(), true
	}
	return time.Time{}, false
}

// LeaseExpired reports whether a RUNNING/VERIFYING reevaluation lease has
// passed its absolute deadline at now.
func LeaseExpired(reevaluation domain.ReevaluationCondition, now time.Time) bool {
	if reevaluation.Kind != domain.ReevaluateLease || reevaluation.Reference == "" {
		return false
	}
	until, ok := ParseLeaseDeadline(reevaluation.Reference)
	if !ok {
		return false
	}
	return !now.Before(until)
}
