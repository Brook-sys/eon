package secretvault

import "os"

// BatchSecretHistoryResult represents the history audit events or error for a secret name in a batch query.
type BatchSecretHistoryResult struct {
	Name    string       `json:"name"`
	Found   bool         `json:"found"`
	History []AuditEvent `json:"history,omitempty"`
	Error   string       `json:"error,omitempty"`
}

// BatchSecretHistory returns audit history for up to 100 secret names in a single operation.
// Secrets with no history and no current record return Found: false without aborting valid results.
// Locked vault returns ErrLocked.
func (v *Vault) BatchSecretHistory(names []string) ([]BatchSecretHistoryResult, error) {
	v.mu.Lock()
	defer v.mu.Unlock()

	if len(v.key) == 0 {
		return nil, ErrLocked
	}

	now := v.now()
	v.lastUsed = now

	results := make([]BatchSecretHistoryResult, len(names))
	for i, name := range names {
		res := BatchSecretHistoryResult{Name: name}

		if err := validateName(name); err != nil {
			res.Error = err.Error()
			results[i] = res
			continue
		}

		// Filter audit log for secret name
		var filtered []AuditEvent
		for _, ev := range v.audit {
			if ev.SecretName == name {
				filtered = append(filtered, ev)
			}
		}

		_, exists := v.data.Secrets[name]
		if len(filtered) == 0 && !exists {
			res.Found = false
			res.Error = os.ErrNotExist.Error()
			results[i] = res
			continue
		}

		res.Found = true
		if len(filtered) == 0 {
			res.History = []AuditEvent{}
		} else {
			res.History = make([]AuditEvent, len(filtered))
			copy(res.History, filtered)
		}
		results[i] = res
	}

	return results, nil
}
