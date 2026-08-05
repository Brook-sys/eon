package secretvault

import "time"

// BatchTouchItem represents a single secret touch request with a specific TTL.
type BatchTouchItem struct {
	Name string        `json:"name"`
	TTL  time.Duration `json:"ttl"`
}

// BatchTouchResult holds per-item status and updated metadata for batch touch operations.
type BatchTouchResult struct {
	Name      string      `json:"name"`
	Status    string      `json:"status"` // "touched", "failed", "invalid_name", "not_found"
	Error     string      `json:"error,omitempty"`
	ExpiresAt time.Time   `json:"expires_at,omitempty"`
	Entry     SecretEntry `json:"entry,omitempty"`
}

// BatchTouchResponse holds the overall result of a BatchTouch call.
type BatchTouchResponse struct {
	Processed int                `json:"processed"`
	Touched   int                `json:"touched"`
	Failed    int                `json:"failed"`
	Results   []BatchTouchResult `json:"results"`
}

// BatchTouch extends expiration TTL for up to 100 targeted secrets with per-secret custom TTLs.
// It processes items atomically per secret, saving updated vault state in a single disk write.
// Returns ErrLocked if the vault is locked.
func (v *Vault) BatchTouch(items []BatchTouchItem) (BatchTouchResponse, error) {
	v.mu.Lock()
	defer v.mu.Unlock()

	if len(v.key) == 0 {
		v.recordAuditLocked("batch_touch", "", "failure")
		return BatchTouchResponse{}, ErrLocked
	}

	now := v.now()
	v.lastUsed = now

	resp := BatchTouchResponse{
		Results: make([]BatchTouchResult, 0, len(items)),
	}

	mutated := false

	for _, item := range items {
		resp.Processed++
		res := BatchTouchResult{Name: item.Name}

		if err := validateName(item.Name); err != nil {
			res.Status = "invalid_name"
			res.Error = err.Error()
			resp.Failed++
			resp.Results = append(resp.Results, res)
			v.recordAuditLocked("touch", item.Name, "failure")
			continue
		}

		sec, exists := v.data.Secrets[item.Name]
		if !exists {
			res.Status = "not_found"
			res.Error = "secret not found"
			resp.Failed++
			resp.Results = append(resp.Results, res)
			v.recordAuditLocked("touch", item.Name, "failure")
			continue
		}

		if item.TTL <= 0 {
			res.Status = "failed"
			res.Error = "TTL must be positive"
			resp.Failed++
			resp.Results = append(resp.Results, res)
			v.recordAuditLocked("touch", item.Name, "failure")
			continue
		}

		newExpiry := now.Add(item.TTL)
		sec.ExpiresAt = newExpiry
		v.data.Secrets[item.Name] = sec

		res.Status = "touched"
		res.ExpiresAt = newExpiry
		res.Entry = SecretEntry{
			Name:      item.Name,
			CreatedAt: sec.CreatedAt,
			UpdatedAt: sec.UpdatedAt,
			ExpiresAt: newExpiry,
		}
		resp.Touched++
		resp.Results = append(resp.Results, res)

		v.recordAuditLocked("touch", item.Name, "success")
		mutated = true
	}

	if mutated {
		if err := v.saveWithCurrentKeyLocked(); err != nil {
			v.recordAuditLocked("batch_touch", "", "failure")
			return BatchTouchResponse{}, err
		}
	}

	v.recordAuditLocked("batch_touch", "", "success")
	return resp, nil
}
