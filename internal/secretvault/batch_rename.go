package secretvault

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

// BatchRenameItem describes a single rename request (Source -> Destination).
type BatchRenameItem struct {
	Source      string `json:"source"`
	Destination string `json:"destination"`
}

// BatchRenameResult summarizes the result of a BatchRename operation.
type BatchRenameResult struct {
	Renamed []string          `json:"renamed"`
	Errors  map[string]string `json:"errors,omitempty"`
}

// BatchRename renames multiple secrets in a single atomic vault transaction.
// It validates each item (source existence, destination validity, self-rename,
// collision within the batch or vault) and applies valid renames to a staged
// copy of the vault data, persisting them with a single atomic disk write.
func (v *Vault) BatchRename(items []BatchRenameItem) (BatchRenameResult, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.expireLocked()

	if len(v.key) == 0 {
		v.recordAuditLocked("batch_rename", "", "failure")
		return BatchRenameResult{}, ErrLocked
	}

	if err := v.reloadWithCurrentKeyLocked(); err != nil {
		v.recordAuditLocked("batch_rename", "", "failure")
		return BatchRenameResult{}, err
	}

	result := BatchRenameResult{
		Renamed: []string{},
	}

	if len(items) == 0 {
		v.recordAuditLocked("batch_rename", "0 renamed", "success")
		return result, nil
	}

	errs := make(map[string]string)
	destSeen := make(map[string]bool)

	// Stage mutation map
	stagedSecrets := make(map[string]record, len(v.data.Secrets))
	for k, val := range v.data.Secrets {
		stagedSecrets[k] = val
	}

	now := v.now()

	for idx, item := range items {
		src := strings.TrimSpace(item.Source)
		dst := strings.TrimSpace(item.Destination)
		itemKey := fmt.Sprintf("[%d]", idx)
		if src != "" {
			itemKey = src
		}

		if src == "" || dst == "" {
			errs[itemKey] = "source and destination must not be empty"
			continue
		}
		if err := validateName(src); err != nil {
			errs[itemKey] = fmt.Sprintf("invalid source name: %v", err)
			continue
		}
		if err := validateName(dst); err != nil {
			errs[itemKey] = fmt.Sprintf("invalid destination name: %v", err)
			continue
		}
		if destSeen[dst] {
			errs[itemKey] = fmt.Sprintf("duplicate destination name in batch: %s", dst)
			continue
		}

		// Check if source exists in staged map
		rec, exists := stagedSecrets[src]
		if !exists {
			errs[itemKey] = os.ErrNotExist.Error()
			continue
		}

		if src == dst {
			// Self-rename is a metadata refresh
			rec.UpdatedAt = now
			stagedSecrets[src] = rec
			destSeen[dst] = true
			result.Renamed = append(result.Renamed, dst)
			continue
		}

		// Perform rename in staged map
		delete(stagedSecrets, src)
		rec.UpdatedAt = now
		stagedSecrets[dst] = rec
		destSeen[dst] = true
		result.Renamed = append(result.Renamed, dst)
	}

	if len(errs) > 0 {
		result.Errors = errs
	}
	sort.Strings(result.Renamed)

	if len(result.Renamed) == 0 {
		v.recordAuditLocked("batch_rename", fmt.Sprintf("0 renamed (%d errors)", len(errs)), "success")
		return result, nil
	}

	// Commit staged map atomically
	origSecrets := v.data.Secrets
	v.data.Secrets = stagedSecrets

	if err := v.saveWithCurrentKeyLocked(); err != nil {
		v.data.Secrets = origSecrets
		v.recordAuditLocked("batch_rename", "", "failure")
		return BatchRenameResult{}, err
	}

	summary := fmt.Sprintf("%d renamed", len(result.Renamed))
	if len(errs) > 0 {
		summary = fmt.Sprintf("%d renamed (%d errors)", len(result.Renamed), len(errs))
	}
	v.recordAuditLocked("batch_rename", summary, "success")

	return result, nil
}
