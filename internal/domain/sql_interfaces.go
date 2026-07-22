package domain

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
)

func scanJSON(src interface{}, dest interface{}) error {
	if src == nil {
		return nil
	}
	switch v := src.(type) {
	case []byte:
		return json.Unmarshal(v, dest)
	case string:
		return json.Unmarshal([]byte(v), dest)
	default:
		return fmt.Errorf("unsupported scan type for JSON: %T", src)
	}
}

func (b Budget) Value() (driver.Value, error) {
	return json.Marshal(b)
}

func (b *Budget) Scan(src interface{}) error {
	return scanJSON(src, b)
}

func (m MissionRevision) Value() (driver.Value, error) {
	return json.Marshal(m)
}

func (m *MissionRevision) Scan(src interface{}) error {
	return scanJSON(src, m)
}

func (q Question) Value() (driver.Value, error) {
	return json.Marshal(q)
}

func (q *Question) Scan(src interface{}) error {
	return scanJSON(src, q)
}

func (i InquiryCandidate) Value() (driver.Value, error) {
	return json.Marshal(i)
}

func (i *InquiryCandidate) Scan(src interface{}) error {
	return scanJSON(src, i)
}

func (i Inquiry) Value() (driver.Value, error) {
	return json.Marshal(i)
}

func (i *Inquiry) Scan(src interface{}) error {
	return scanJSON(src, i)
}

func (o OperationSpec) Value() (driver.Value, error) {
	return json.Marshal(o)
}

func (o *OperationSpec) Scan(src interface{}) error {
	return scanJSON(src, o)
}

func (o Operation) Value() (driver.Value, error) {
	return json.Marshal(o)
}

func (o *Operation) Scan(src interface{}) error {
	return scanJSON(src, o)
}

func (s SubagentRecord) Value() (driver.Value, error) {
	return json.Marshal(s)
}

func (s *SubagentRecord) Scan(src interface{}) error {
	return scanJSON(src, s)
}
