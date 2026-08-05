package secretvault

// BatchAuditFilterItem represents a single secret filter request for batch audit log queries.
type BatchAuditFilterItem struct {
	SecretName string `json:"secret_name"`
	Action     string `json:"action,omitempty"`
	Status     string `json:"status,omitempty"`
}

// BatchAuditFilterResult represents the audit log result for a single secret name in a batch audit query.
type BatchAuditFilterResult struct {
	SecretName string       `json:"secret_name"`
	Found      bool         `json:"found"`
	Events     []AuditEvent `json:"events,omitempty"`
	Error      string       `json:"error,omitempty"`
}

// BatchAuditFilter returns filtered audit log events for up to 100 secret names/filters in a single operation.
// Locked vault returns ErrLocked.
func (v *Vault) BatchAuditFilter(items []BatchAuditFilterItem) ([]BatchAuditFilterResult, error) {
	v.mu.Lock()
	defer v.mu.Unlock()

	if len(v.key) == 0 {
		v.recordAuditLocked("batch_audit_filter", "", "failure")
		return nil, ErrLocked
	}

	now := v.now()
	v.lastUsed = now

	results := make([]BatchAuditFilterResult, len(items))
	for i, item := range items {
		res := BatchAuditFilterResult{SecretName: item.SecretName}

		if item.SecretName != "" {
			if err := validateName(item.SecretName); err != nil {
				res.Error = err.Error()
				results[i] = res
				continue
			}
		}

		filter := AuditFilter{
			SecretName: item.SecretName,
			Action:     item.Action,
			Status:     item.Status,
		}

		var events []AuditEvent
		for _, evt := range v.audit {
			if filter.Action != "" && evt.Action != filter.Action {
				continue
			}
			if filter.Status != "" && evt.Status != filter.Status {
				continue
			}
			if filter.SecretName != "" && evt.SecretName != filter.SecretName {
				continue
			}
			events = append(events, evt)
		}

		if len(events) > 0 {
			res.Found = true
			res.Events = events
		}

		results[i] = res
	}

	v.recordAuditLocked("batch_audit_filter", "", "success")
	return results, nil
}
