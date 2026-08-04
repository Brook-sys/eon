package secretvault

import (
	"fmt"
	"sort"
)

// BatchPurgeResult holds the outcome of purging a specific slice of expired secrets.
type BatchPurgeResult struct {
	Purged []string `json:"purged"`
}

// BatchPurgeExpired removes a given slice of expired secrets from the vault atomically.
// Secrets in the slice that are not expired, missing, or active are ignored.
// Returns sorted names of secrets that were actually purged.
func (v *Vault) BatchPurgeExpired(names []string) (BatchPurgeResult, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.expireLocked()
	if len(v.key) == 0 {
		v.recordAuditLocked("batch_purge_expired", "", "failure")
		return BatchPurgeResult{}, ErrLocked
	}
	pathLock := lockForPath(v.path)
	pathLock.Lock()
	defer pathLock.Unlock()
	if err := v.reloadWithCurrentKeyLocked(); err != nil {
		v.recordAuditLocked("batch_purge_expired", "", "failure")
		return BatchPurgeResult{}, err
	}

	now := v.now()
	var purged []string
	for _, name := range names {
		r, ok := v.data.Secrets[name]
		if ok && !r.ExpiresAt.IsZero() && !now.Before(r.ExpiresAt) {
			delete(v.data.Secrets, name)
			purged = append(purged, name)
		}
	}

	sort.Strings(purged)
	res := BatchPurgeResult{Purged: purged}
	if len(purged) == 0 {
		v.lastUsed = v.now()
		v.recordAuditLocked("batch_purge_expired", "0 purged", "success")
		return res, nil
	}

	v.lastUsed = v.now()
	err := v.saveWithCurrentKeyLocked()
	if err == nil {
		v.recordAuditLocked("batch_purge_expired", fmt.Sprintf("%d purged", len(purged)), "success")
	} else {
		v.recordAuditLocked("batch_purge_expired", "", "failure")
	}
	return res, err
}
