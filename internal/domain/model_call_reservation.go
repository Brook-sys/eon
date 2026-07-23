package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// ModelCallReservation durably spends one operation-lifetime model-call slot
// before provider invocation. A reservation without a completion receipt is
// intentionally ambiguous and therefore remains consumed after recovery.
type ModelCallReservation struct {
	SchemaVersion int         `json:"schema_version"`
	OperationID   OperationID `json:"operation_id"`
	Attempt       uint32      `json:"attempt"`
	ModelCall     uint32      `json:"model_call"`
	BindingID     string      `json:"binding_id,omitempty"`
	ReservedAt    time.Time   `json:"reserved_at"`
}

func (r ModelCallReservation) Validate() error {
	if r.SchemaVersion != SchemaVersionV1 {
		return fmt.Errorf("model call reservation schema version must be %d", SchemaVersionV1)
	}
	if strings.TrimSpace(string(r.OperationID)) == "" {
		return errors.New("model call reservation operation id is required")
	}
	if r.Attempt == 0 {
		return errors.New("model call reservation attempt must be positive")
	}
	if r.ModelCall == 0 {
		return errors.New("model call reservation model call must be positive")
	}
	if r.ReservedAt.IsZero() {
		return errors.New("model call reservation time is required")
	}
	return nil
}
