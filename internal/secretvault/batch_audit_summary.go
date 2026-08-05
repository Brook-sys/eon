package secretvault

// BatchAuditSummaryItem represents a single filter query item in a batch audit summary request.
type BatchAuditSummaryItem struct {
	SecretName string `json:"secret_name,omitempty"`
	Action     string `json:"action,omitempty"`
	Status     string `json:"status,omitempty"`
}

// BatchAuditSummaryResult holds the AuditSummary for a single query item in a batch audit summary operation.
type BatchAuditSummaryResult struct {
	Item    BatchAuditSummaryItem `json:"item"`
	Summary AuditSummary          `json:"summary"`
	Error   string                `json:"error,omitempty"`
}

// BatchAuditSummary computes AuditSummary for up to 100 filter items in a single operation.
// Like AuditSummary, it inspects in-memory audit metadata without requiring vault unlock.
func (v *Vault) BatchAuditSummary(items []BatchAuditSummaryItem) []BatchAuditSummaryResult {
	results := make([]BatchAuditSummaryResult, len(items))
	for i, item := range items {
		res := BatchAuditSummaryResult{Item: item}

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

		res.Summary = v.AuditSummary(filter)
		results[i] = res
	}

	return results
}
