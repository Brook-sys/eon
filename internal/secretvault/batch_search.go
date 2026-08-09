package secretvault

import "sort"
import "strings"

// BatchSearchItem represents a single search query in a batch operation.
type BatchSearchItem struct {
	Prefix    string `json:"prefix,omitempty"`
	Substring string `json:"substring,omitempty"`
}

// BatchSearchResult holds the matching secrets for a single search query.
type BatchSearchResult struct {
	Item    BatchSearchItem `json:"item"`
	Secrets []SecretEntry   `json:"secrets,omitempty"`
	Error   string          `json:"error,omitempty"`
}

// BatchSearch performs up to 100 search queries over vault secrets in a single lock acquisition.
// Locked vault returns ErrLocked.
func (v *Vault) BatchSearch(items []BatchSearchItem) ([]BatchSearchResult, error) {
	v.mu.Lock()
	defer v.mu.Unlock()

	if len(v.key) == 0 {
		v.recordAuditLocked("batch_search", "", "failure")
		return nil, ErrLocked
	}

	now := v.now()
	v.lastUsed = now

	// Prepare metadata once for all queries
	allSecrets := make([]SecretEntry, 0, len(v.data.Secrets))
	for name, sec := range v.data.Secrets {
		expired := !sec.ExpiresAt.IsZero() && !now.Before(sec.ExpiresAt)
		allSecrets = append(allSecrets, SecretEntry{
			Name:      name,
			CreatedAt: sec.CreatedAt,
			UpdatedAt: sec.UpdatedAt,
			ExpiresAt: sec.ExpiresAt,
			Expired:   expired,
		})
	}
	sort.Slice(allSecrets, func(i, j int) bool { return allSecrets[i].Name < allSecrets[j].Name })

	results := make([]BatchSearchResult, len(items))
	for i, item := range items {
		res := BatchSearchResult{Item: item}

		var matches []SecretEntry
		for _, sec := range allSecrets {
			if item.Prefix != "" && !strings.HasPrefix(sec.Name, item.Prefix) {
				continue
			}
			if item.Substring != "" && !strings.Contains(strings.ToLower(sec.Name), strings.ToLower(item.Substring)) {
				continue
			}
			matches = append(matches, sec)
		}

		if len(matches) > 0 {
			res.Secrets = matches
		}
		results[i] = res
	}

	v.recordAuditLocked("batch_search", "", "success")
	return results, nil
}
