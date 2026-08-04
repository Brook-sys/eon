package secretvault

import (
	"fmt"
	"time"
)

// TokenRefreshItem represents a secret to be refreshed with a new value and optional TTL.
type TokenRefreshItem struct {
	Name     string `json:"name"`
	NewValue string `json:"new_value"`
	TTL      string `json:"ttl,omitempty"`
}

func (item TokenRefreshItem) ParseTTL() (time.Duration, error) {
	if item.TTL == "" {
		return 0, nil
	}
	return time.ParseDuration(item.TTL)
}

// TokenRefreshResult holds the outcome of a RefreshToken or BatchRefreshToken operation.
type TokenRefreshResult struct {
	Total     int      `json:"total"`
	Refreshed []string `json:"refreshed"`
	Errors    []string `json:"errors,omitempty"`
}

// RefreshToken rotates an existing secret or creates it if missing, applying the given value and TTL.
func (v *Vault) RefreshToken(name, newValue string, ttl time.Duration) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.expireLocked()

	if len(v.key) == 0 {
		v.recordAuditLocked("refresh_token", name, "failure")
		return ErrLocked
	}

	if err := validateName(name); err != nil {
		v.recordAuditLocked("refresh_token", name, "failure")
		return err
	}

	if newValue == "" || len(newValue) > maxSecretSize {
		v.recordAuditLocked("refresh_token", name, "failure")
		return ErrInvalidSecretValue
	}

	if ttl < 0 {
		v.recordAuditLocked("refresh_token", name, "failure")
		return ErrInvalidTTL
	}

	pathLock := lockForPath(v.path)
	pathLock.Lock()
	defer pathLock.Unlock()

	if err := v.reloadWithCurrentKeyLocked(); err != nil {
		v.recordAuditLocked("refresh_token", name, "failure")
		return fmt.Errorf("reload error: %w", err)
	}

	now := v.now().UTC()
	var expiresAt time.Time
	if ttl > 0 {
		expiresAt = now.Add(ttl).UTC()
	}

	r, ok := v.data.Secrets[name]
	if !ok {
		r.CreatedAt = now
	}
	r.Value = newValue
	r.UpdatedAt = now
	r.ExpiresAt = expiresAt

	v.data.Secrets[name] = r
	v.lastUsed = v.now()

	err := v.saveWithCurrentKeyLocked()
	if err == nil {
		v.recordAuditLocked("refresh_token", name, "success")
	} else {
		v.recordAuditLocked("refresh_token", name, "failure")
	}
	return err
}

// BatchRefreshToken refreshes multiple secrets atomically in a single file write.
func (v *Vault) BatchRefreshToken(items []TokenRefreshItem) (TokenRefreshResult, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.expireLocked()

	res := TokenRefreshResult{
		Total:     len(items),
		Refreshed: []string{},
		Errors:    []string{},
	}

	if len(v.key) == 0 {
		v.recordAuditLocked("batch_refresh_token", "", "failure")
		return res, ErrLocked
	}

	if len(items) == 0 {
		return res, nil
	}

	pathLock := lockForPath(v.path)
	pathLock.Lock()
	defer pathLock.Unlock()

	if err := v.reloadWithCurrentKeyLocked(); err != nil {
		v.recordAuditLocked("batch_refresh_token", "", "failure")
		return res, fmt.Errorf("reload error: %w", err)
	}

	now := v.now().UTC()
	seenNames := make(map[string]bool)
	stagedChanges := make(map[string]record)

	for i, item := range items {
		if err := validateName(item.Name); err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("item %d (%s): %v", i, item.Name, err))
			continue
		}
		if seenNames[item.Name] {
			res.Errors = append(res.Errors, fmt.Sprintf("item %d (%s): duplicate name in batch", i, item.Name))
			continue
		}
		seenNames[item.Name] = true

		if item.NewValue == "" || len(item.NewValue) > maxSecretSize {
			res.Errors = append(res.Errors, fmt.Sprintf("item %d (%s): %v", i, item.Name, ErrInvalidSecretValue))
			continue
		}

		dur, err := item.ParseTTL()
		if err != nil || dur < 0 {
			res.Errors = append(res.Errors, fmt.Sprintf("item %d (%s): %v", i, item.Name, ErrInvalidTTL))
			continue
		}

		var expiresAt time.Time
		if dur > 0 {
			expiresAt = now.Add(dur).UTC()
		}

		r, ok := v.data.Secrets[item.Name]
		if !ok {
			r.CreatedAt = now
		}
		r.Value = item.NewValue
		r.UpdatedAt = now
		r.ExpiresAt = expiresAt

		stagedChanges[item.Name] = r
		res.Refreshed = append(res.Refreshed, item.Name)
	}

	if len(res.Refreshed) == 0 {
		v.recordAuditLocked("batch_refresh_token", "", "failure")
		return res, nil
	}

	for k, vRec := range stagedChanges {
		v.data.Secrets[k] = vRec
	}
	v.lastUsed = v.now()

	err := v.saveWithCurrentKeyLocked()
	if err == nil {
		v.recordAuditLocked("batch_refresh_token", fmt.Sprintf("%d refreshed", len(res.Refreshed)), "success")
	} else {
		v.recordAuditLocked("batch_refresh_token", "", "failure")
	}
	return res, err
}
