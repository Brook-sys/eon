// Package secretvault provides a small encrypted, local credential store.
// Secret values are never returned by its metadata API or written in plaintext.
package secretvault

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	formatVersion     = 1
	iterations        = 600_000
	maxSecretSize     = 16 << 10
	autoLockAfter     = 15 * time.Minute
	maxFailedAttempts = 5
	lockoutDuration   = 30 * time.Second
)

var (
	ErrLocked                = errors.New("credential vault is locked")
	ErrInitialized           = errors.New("credential vault is already initialized")
	ErrUninitialized         = errors.New("credential vault is not initialized")
	ErrInvalidPassword       = errors.New("invalid vault password")
	ErrInvalidPasswordLength = errors.New("vault password must contain 12 to 1024 characters")
	ErrInvalidSecretName     = errors.New("invalid secret name")
	ErrInvalidSecretValue    = errors.New("secret value is required and must not exceed 16 KiB")
	ErrAccountLockedOut      = errors.New("vault is locked out due to too many failed attempts")
	ErrInvalidBackupPath     = errors.New("backup path is required")
	ErrInvalidBackupFormat   = errors.New("invalid or unsupported backup format")
	ErrImportConflict        = errors.New("import conflict: a secret with this name already exists")
	ErrInvalidImportMode     = errors.New("invalid import mode")
	ErrSecretExpired         = errors.New("secret has expired")
	ErrInvalidTTL            = errors.New("invalid secret ttl duration")
	pathLocks                sync.Map
)

// ImportMode controls how Import handles name conflicts.
type ImportMode int

const (
	// ImportModeFail returns ErrImportConflict on the first conflicting name.
	ImportModeFail ImportMode = iota
	// ImportModeSkip silently skips conflicting secrets.
	ImportModeSkip
	// ImportModeOverwrite replaces existing secrets with the imported value.
	ImportModeOverwrite
)

// ImportOptions configures Import behaviour.
type ImportOptions struct {
	Mode ImportMode
}

func lockForPath(path string) *sync.Mutex {
	lock, _ := pathLocks.LoadOrStore(path, &sync.Mutex{})
	return lock.(*sync.Mutex)
}

type envelope struct {
	Version    int    `json:"version"`
	Iterations int    `json:"iterations"`
	Salt       string `json:"salt"`
	Verifier   string `json:"verifier"`
	Nonce      string `json:"nonce"`
	Ciphertext string `json:"ciphertext"`
}

type record struct {
	Value     string    `json:"value"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	ExpiresAt time.Time `json:"expires_at,omitempty"`
}

type payload struct {
	Secrets map[string]record `json:"secrets"`
}

type Metadata struct {
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	ExpiresAt time.Time `json:"expires_at,omitempty"`
	Expired   bool      `json:"expired,omitempty"`
}

type AuditEvent struct {
	Timestamp  time.Time `json:"timestamp"`
	Action     string    `json:"action"`
	SecretName string    `json:"secret_name,omitempty"`
	Status     string    `json:"status"`
	Detail     string    `json:"detail,omitempty"`
}

// ResolveResult holds the outcome of resolving a single secret name.
type ResolveResult struct {
	Name  string `json:"name"`
	Value string `json:"value,omitempty"`
	Error string `json:"error,omitempty"`
}

// SecretEntry holds metadata for a single secret without exposing its value.
type SecretEntry struct {
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	ExpiresAt time.Time `json:"expires_at,omitempty"`
	Expired   bool      `json:"expired,omitempty"`
}

type Status struct {
	Initialized bool       `json:"initialized"`
	Locked      bool       `json:"locked"`
	Secrets     []Metadata `json:"secrets,omitempty"`
}

type Vault struct {
	mu          sync.Mutex
	path        string
	key         []byte
	data        payload
	lastUsed    time.Time
	failedCount int
	lockedUntil time.Time
	now         func() time.Time
	audit       []AuditEvent
}

func New(path string) (*Vault, error) {
	return NewWithClock(path, time.Now)
}

// NewWithClock injects the clock used for activity and inactivity expiry.
func NewWithClock(path string, now func() time.Time) (*Vault, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("vault path is required")
	}
	if now == nil {
		return nil, errors.New("vault clock is required")
	}
	return &Vault{path: path, data: payload{Secrets: map[string]record{}}, now: now}, nil
}

func (v *Vault) recordAuditLocked(action, secretName, status string) {
	evt := AuditEvent{
		Timestamp:  v.now().UTC(),
		Action:     action,
		SecretName: secretName,
		Status:     status,
	}
	v.audit = append(v.audit, evt)
	if len(v.audit) > 1000 {
		v.audit = v.audit[len(v.audit)-1000:]
	}
}

func (v *Vault) AuditLog() []AuditEvent {
	v.mu.Lock()
	defer v.mu.Unlock()
	res := make([]AuditEvent, len(v.audit))
	copy(res, v.audit)
	return res
}

func (v *Vault) Status() Status {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.expireLocked()
	_, err := os.Stat(v.path)
	s := Status{Initialized: err == nil, Locked: len(v.key) == 0}
	if len(v.key) != 0 {
		now := v.now()
		for name, r := range v.data.Secrets {
			m := Metadata{Name: name, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, ExpiresAt: r.ExpiresAt}
			if !r.ExpiresAt.IsZero() && !now.Before(r.ExpiresAt) {
				m.Expired = true
			}
			s.Secrets = append(s.Secrets, m)
		}
		sort.Slice(s.Secrets, func(i, j int) bool { return s.Secrets[i].Name < s.Secrets[j].Name })
	}
	return s
}

func (v *Vault) Initialize(password string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.expireLocked()
	if _, err := os.Stat(v.path); err == nil {
		v.recordAuditLocked("initialize", "", "failure")
		return ErrInitialized
	} else if !errors.Is(err, os.ErrNotExist) {
		v.recordAuditLocked("initialize", "", "failure")
		return err
	}
	if err := validatePassword(password); err != nil {
		v.recordAuditLocked("initialize", "", "failure")
		return err
	}
	salt := make([]byte, 32)
	if _, err := rand.Read(salt); err != nil {
		v.recordAuditLocked("initialize", "", "failure")
		return err
	}
	key, err := derive(password, salt, iterations)
	if err != nil {
		v.recordAuditLocked("initialize", "", "failure")
		return err
	}
	v.key = key
	v.data = payload{Secrets: map[string]record{}}
	if err := v.saveLocked(salt, iterations); err != nil {
		zero(v.key)
		v.key = nil
		v.recordAuditLocked("initialize", "", "failure")
		return err
	}
	v.lastUsed = v.now()
	v.recordAuditLocked("initialize", "", "success")
	return nil
}

func (v *Vault) Unlock(password string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.expireLocked()
	if v.isLockedOutLocked() {
		v.recordAuditLocked("unlock", "", "failure")
		return ErrAccountLockedOut
	}
	raw, err := os.ReadFile(v.path)
	if errors.Is(err, os.ErrNotExist) {
		v.recordAuditLocked("unlock", "", "failure")
		return ErrUninitialized
	}
	if err != nil {
		v.recordAuditLocked("unlock", "", "failure")
		return err
	}
	var env envelope
	if err = json.Unmarshal(raw, &env); err != nil {
		v.recordAuditLocked("unlock", "", "failure")
		return fmt.Errorf("decode vault: %w", err)
	}
	if env.Version != formatVersion || env.Iterations < 100_000 {
		v.recordAuditLocked("unlock", "", "failure")
		return errors.New("unsupported vault format")
	}
	salt, err := base64.StdEncoding.DecodeString(env.Salt)
	if err != nil {
		v.recordAuditLocked("unlock", "", "failure")
		return errors.New("invalid vault salt")
	}
	key, err := derive(password, salt, env.Iterations)
	if err != nil {
		v.recordAuditLocked("unlock", "", "failure")
		return err
	}
	want, err := base64.StdEncoding.DecodeString(env.Verifier)
	if err != nil {
		v.recordAuditLocked("unlock", "", "failure")
		return errors.New("invalid vault verifier")
	}
	got := verifier(key)
	if len(want) != len(got) || subtle.ConstantTimeCompare(want, got) != 1 {
		zero(key)
		v.recordFailedAttemptLocked()
		v.recordAuditLocked("unlock", "", "failure")
		return ErrInvalidPassword
	}
	nonce, err := base64.StdEncoding.DecodeString(env.Nonce)
	if err != nil {
		zero(key)
		v.recordAuditLocked("unlock", "", "failure")
		return errors.New("invalid vault nonce")
	}
	ct, err := base64.StdEncoding.DecodeString(env.Ciphertext)
	if err != nil {
		zero(key)
		v.recordAuditLocked("unlock", "", "failure")
		return errors.New("invalid vault ciphertext")
	}
	plain, err := open(key, nonce, ct)
	if err != nil {
		zero(key)
		v.recordFailedAttemptLocked()
		v.recordAuditLocked("unlock", "", "failure")
		return ErrInvalidPassword
	}
	var p payload
	err = json.Unmarshal(plain, &p)
	zero(plain)
	if err != nil {
		zero(key)
		v.recordAuditLocked("unlock", "", "failure")
		return errors.New("invalid vault payload")
	}
	if p.Secrets == nil {
		p.Secrets = map[string]record{}
	}
	zero(v.key)
	v.key = key
	v.data = p
	v.lastUsed = v.now()
	v.failedCount = 0
	v.lockedUntil = time.Time{}
	v.recordAuditLocked("unlock", "", "success")
	return nil
}

func (v *Vault) Lock() {
	v.mu.Lock()
	defer v.mu.Unlock()
	zero(v.key)
	v.key = nil
	v.data = payload{Secrets: map[string]record{}}
	v.lastUsed = time.Time{}
	v.recordAuditLocked("lock", "", "success")
}

func (v *Vault) Close() {
	v.mu.Lock()
	defer v.mu.Unlock()
	zero(v.key)
	v.key = nil
	v.data = payload{Secrets: map[string]record{}}
	v.lastUsed = time.Time{}
	v.failedCount = 0
	v.lockedUntil = time.Time{}
	v.recordAuditLocked("close", "", "success")
}

func (v *Vault) Put(name, value string) error {
	return v.PutWithExpiry(name, value, time.Time{})
}

func (v *Vault) PutWithTTL(name, value string, ttl time.Duration) error {
	if ttl <= 0 {
		return ErrInvalidTTL
	}
	v.mu.Lock()
	now := v.now().UTC()
	v.mu.Unlock()
	return v.PutWithExpiry(name, value, now.Add(ttl))
}

func (v *Vault) PutWithExpiry(name, value string, expiresAt time.Time) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.expireLocked()
	if len(v.key) == 0 {
		v.recordAuditLocked("put", name, "failure")
		return ErrLocked
	}
	if err := validateName(name); err != nil {
		v.recordAuditLocked("put", name, "failure")
		return err
	}
	if value == "" || len(value) > maxSecretSize {
		v.recordAuditLocked("put", name, "failure")
		return ErrInvalidSecretValue
	}
	pathLock := lockForPath(v.path)
	pathLock.Lock()
	defer pathLock.Unlock()
	if err := v.reloadWithCurrentKeyLocked(); err != nil {
		v.recordAuditLocked("put", name, "failure")
		return err
	}
	now := v.now().UTC()
	r, ok := v.data.Secrets[name]
	if !ok {
		r.CreatedAt = now
	}
	r.Value = value
	r.UpdatedAt = now
	if !expiresAt.IsZero() {
		r.ExpiresAt = expiresAt.UTC()
	} else {
		r.ExpiresAt = time.Time{}
	}
	v.data.Secrets[name] = r
	v.lastUsed = v.now()
	err := v.saveWithCurrentKeyLocked()
	if err == nil {
		v.recordAuditLocked("put", name, "success")
	} else {
		v.recordAuditLocked("put", name, "failure")
	}
	return err
}

func (v *Vault) Delete(name string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.expireLocked()
	if len(v.key) == 0 {
		v.recordAuditLocked("delete", name, "failure")
		return ErrLocked
	}
	if err := validateName(name); err != nil {
		v.recordAuditLocked("delete", name, "failure")
		return err
	}
	pathLock := lockForPath(v.path)
	pathLock.Lock()
	defer pathLock.Unlock()
	if err := v.reloadWithCurrentKeyLocked(); err != nil {
		v.recordAuditLocked("delete", name, "failure")
		return err
	}
	delete(v.data.Secrets, name)
	v.lastUsed = v.now()
	err := v.saveWithCurrentKeyLocked()
	if err == nil {
		v.recordAuditLocked("delete", name, "success")
	} else {
		v.recordAuditLocked("delete", name, "failure")
	}
	return err
}

func (v *Vault) Resolve(name string) (string, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.expireLocked()
	if len(v.key) == 0 {
		v.recordAuditLocked("resolve", name, "failure")
		return "", ErrLocked
	}
	r, ok := v.data.Secrets[name]
	if !ok {
		v.recordAuditLocked("resolve", name, "failure")
		return "", os.ErrNotExist
	}
	v.lastUsed = v.now()
	if !r.ExpiresAt.IsZero() && !v.now().Before(r.ExpiresAt) {
		v.recordAuditLocked("resolve", name, "expired")
		return "", ErrSecretExpired
	}
	v.recordAuditLocked("resolve", name, "success")
	return r.Value, nil
}

// ResolveAll resolves multiple secret names in a single locked call. Each name
// produces a ResolveResult; a missing or expired secret sets Error but does not
// abort the batch. The vault's inactivity timer is refreshed once.
func (v *Vault) ResolveAll(names []string) []ResolveResult {
	results := make([]ResolveResult, len(names))
	for i := range names {
		results[i].Name = names[i]
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	v.expireLocked()
	if len(v.key) == 0 {
		for i := range results {
			results[i].Error = ErrLocked.Error()
			v.recordAuditLocked("resolve", results[i].Name, "failure")
		}
		return results
	}
	v.lastUsed = v.now()
	now := v.now()
	hasSuccess := false
	for i, name := range names {
		r, ok := v.data.Secrets[name]
		if !ok {
			results[i].Error = os.ErrNotExist.Error()
			v.recordAuditLocked("resolve", name, "failure")
			continue
		}
		if !r.ExpiresAt.IsZero() && !now.Before(r.ExpiresAt) {
			results[i].Error = ErrSecretExpired.Error()
			v.recordAuditLocked("resolve", name, "expired")
			continue
		}
		results[i].Value = r.Value
		results[i].Error = ""
		hasSuccess = true
		v.recordAuditLocked("resolve", name, "success")
	}
	_ = hasSuccess
	return results
}

// ListSecrets returns metadata for all stored secrets without exposing values.
// The vault must be unlocked. Expired secrets are marked as expired but still
// listed. Results are sorted by name for deterministic output.
func (v *Vault) ListSecrets() ([]SecretEntry, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.expireLocked()
	if len(v.key) == 0 {
		v.recordAuditLocked("list", "", "failure")
		return nil, ErrLocked
	}
	now := v.now()
	entries := make([]SecretEntry, 0, len(v.data.Secrets))
	for name, r := range v.data.Secrets {
		e := SecretEntry{
			Name:      name,
			CreatedAt: r.CreatedAt,
			UpdatedAt: r.UpdatedAt,
			ExpiresAt: r.ExpiresAt,
		}
		if !r.ExpiresAt.IsZero() && !now.Before(r.ExpiresAt) {
			e.Expired = true
		}
		entries = append(entries, e)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	v.lastUsed = v.now()
	v.recordAuditLocked("list", "", "success")
	return entries, nil
}

// Rotate updates the value of an existing secret while preserving its
// CreatedAt timestamp. This is a security best practice for credential
// rotation. The vault must be unlocked and the secret must already exist.
func (v *Vault) Rotate(name, newValue string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.expireLocked()
	if len(v.key) == 0 {
		v.recordAuditLocked("rotate", name, "failure")
		return ErrLocked
	}
	if err := validateName(name); err != nil {
		v.recordAuditLocked("rotate", name, "failure")
		return err
	}
	if newValue == "" || len(newValue) > maxSecretSize {
		v.recordAuditLocked("rotate", name, "failure")
		return ErrInvalidSecretValue
	}
	pathLock := lockForPath(v.path)
	pathLock.Lock()
	defer pathLock.Unlock()
	if err := v.reloadWithCurrentKeyLocked(); err != nil {
		v.recordAuditLocked("rotate", name, "failure")
		return err
	}
	r, ok := v.data.Secrets[name]
	if !ok {
		v.recordAuditLocked("rotate", name, "failure")
		return os.ErrNotExist
	}
	r.Value = newValue
	r.UpdatedAt = v.now().UTC()
	v.data.Secrets[name] = r
	v.lastUsed = v.now()
	err := v.saveWithCurrentKeyLocked()
	if err == nil {
		v.recordAuditLocked("rotate", name, "success")
	} else {
		v.recordAuditLocked("rotate", name, "failure")
	}
	return err
}

// DeleteAll deletes all secrets from the vault atomically in a single write operation.
func (v *Vault) DeleteAll() (int, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.expireLocked()
	if len(v.key) == 0 {
		v.recordAuditLocked("delete_all", "", "failure")
		return 0, ErrLocked
	}
	pathLock := lockForPath(v.path)
	pathLock.Lock()
	defer pathLock.Unlock()
	if err := v.reloadWithCurrentKeyLocked(); err != nil {
		v.recordAuditLocked("delete_all", "", "failure")
		return 0, err
	}
	count := len(v.data.Secrets)
	if count == 0 {
		v.recordAuditLocked("delete_all", "", "success")
		return 0, nil
	}
	v.data.Secrets = map[string]record{}
	v.lastUsed = v.now()
	if err := v.saveWithCurrentKeyLocked(); err != nil {
		v.recordAuditLocked("delete_all", "", "failure")
		return 0, err
	}
	v.recordAuditLocked("delete_all", "", "success")
	return count, nil
}

// PurgeExpired removes all expired secrets from the vault and persists the
// change. The vault must be unlocked. Returns the number of secrets removed.
func (v *Vault) PurgeExpired() (int, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.expireLocked()
	if len(v.key) == 0 {
		v.recordAuditLocked("purge_expired", "", "failure")
		return 0, ErrLocked
	}
	pathLock := lockForPath(v.path)
	pathLock.Lock()
	defer pathLock.Unlock()
	if err := v.reloadWithCurrentKeyLocked(); err != nil {
		v.recordAuditLocked("purge_expired", "", "failure")
		return 0, err
	}
	now := v.now()
	var removed int
	for name, r := range v.data.Secrets {
		if !r.ExpiresAt.IsZero() && !now.Before(r.ExpiresAt) {
			delete(v.data.Secrets, name)
			removed++
		}
	}
	if removed == 0 {
		v.lastUsed = v.now()
		v.recordAuditLocked("purge_expired", "", "success")
		return 0, nil
	}
	v.lastUsed = v.now()
	err := v.saveWithCurrentKeyLocked()
	if err == nil {
		v.recordAuditLocked("purge_expired", "", "success")
	} else {
		v.recordAuditLocked("purge_expired", "", "failure")
	}
	return removed, err
}

// RotateWithExpiry updates the value of an existing secret while preserving
// its CreatedAt timestamp and optionally setting a new expiration time.
// The vault must be unlocked and the secret must already exist.
func (v *Vault) RotateWithExpiry(name, newValue string, expiresAt time.Time) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.expireLocked()
	if len(v.key) == 0 {
		v.recordAuditLocked("rotate", name, "failure")
		return ErrLocked
	}
	if err := validateName(name); err != nil {
		v.recordAuditLocked("rotate", name, "failure")
		return err
	}
	if newValue == "" || len(newValue) > maxSecretSize {
		v.recordAuditLocked("rotate", name, "failure")
		return ErrInvalidSecretValue
	}
	pathLock := lockForPath(v.path)
	pathLock.Lock()
	defer pathLock.Unlock()
	if err := v.reloadWithCurrentKeyLocked(); err != nil {
		v.recordAuditLocked("rotate", name, "failure")
		return err
	}
	r, ok := v.data.Secrets[name]
	if !ok {
		v.recordAuditLocked("rotate", name, "failure")
		return os.ErrNotExist
	}
	r.Value = newValue
	r.UpdatedAt = v.now().UTC()
	if !expiresAt.IsZero() {
		r.ExpiresAt = expiresAt.UTC()
	} else {
		r.ExpiresAt = time.Time{}
	}
	v.data.Secrets[name] = r
	v.lastUsed = v.now()
	err := v.saveWithCurrentKeyLocked()
	if err == nil {
		v.recordAuditLocked("rotate", name, "success")
	} else {
		v.recordAuditLocked("rotate", name, "failure")
	}
	return err
}

// RotateWithTTL updates the value of an existing secret and sets its
// expiration to now + ttl.
func (v *Vault) RotateWithTTL(name, newValue string, ttl time.Duration) error {
	if ttl <= 0 {
		return ErrInvalidTTL
	}
	v.mu.Lock()
	now := v.now().UTC()
	v.mu.Unlock()
	return v.RotateWithExpiry(name, newValue, now.Add(ttl))
}

func (v *Vault) ChangePassword(oldPassword, newPassword string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.expireLocked()
	if len(v.key) == 0 {
		v.recordAuditLocked("rekey", "", "failure")
		return ErrLocked
	}
	if v.isLockedOutLocked() {
		v.recordAuditLocked("rekey", "", "failure")
		return ErrAccountLockedOut
	}
	if err := validatePassword(newPassword); err != nil {
		v.recordAuditLocked("rekey", "", "failure")
		return err
	}
	raw, err := os.ReadFile(v.path)
	if err != nil {
		v.recordAuditLocked("rekey", "", "failure")
		return err
	}
	var env envelope
	if err = json.Unmarshal(raw, &env); err != nil {
		v.recordAuditLocked("rekey", "", "failure")
		return fmt.Errorf("decode vault: %w", err)
	}
	salt, err := base64.StdEncoding.DecodeString(env.Salt)
	if err != nil {
		v.recordAuditLocked("rekey", "", "failure")
		return errors.New("invalid vault salt")
	}
	testKey, err := derive(oldPassword, salt, env.Iterations)
	if err != nil {
		v.recordAuditLocked("rekey", "", "failure")
		return err
	}
	if len(testKey) != len(v.key) || subtle.ConstantTimeCompare(testKey, v.key) != 1 {
		zero(testKey)
		v.recordFailedAttemptLocked()
		v.recordAuditLocked("rekey", "", "failure")
		return ErrInvalidPassword
	}
	zero(testKey)

	pathLock := lockForPath(v.path)
	pathLock.Lock()
	defer pathLock.Unlock()

	if err := v.reloadWithCurrentKeyLocked(); err != nil {
		v.recordAuditLocked("rekey", "", "failure")
		return err
	}
	newSalt := make([]byte, 32)
	if _, err := rand.Read(newSalt); err != nil {
		v.recordAuditLocked("rekey", "", "failure")
		return err
	}
	newKey, err := derive(newPassword, newSalt, iterations)
	if err != nil {
		v.recordAuditLocked("rekey", "", "failure")
		return err
	}
	oldKey := v.key
	v.key = newKey
	if err := v.saveLocked(newSalt, iterations); err != nil {
		zero(newKey)
		v.key = oldKey
		v.recordAuditLocked("rekey", "", "failure")
		return err
	}
	zero(oldKey)
	v.lastUsed = v.now()
	v.failedCount = 0
	v.lockedUntil = time.Time{}
	v.recordAuditLocked("rekey", "", "success")
	return nil
}

func (v *Vault) Export(backupPath, backupPassword string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.expireLocked()
	if len(v.key) == 0 {
		return ErrLocked
	}
	backupPath = strings.TrimSpace(backupPath)
	if backupPath == "" {
		return ErrInvalidBackupPath
	}
	if err := validatePassword(backupPassword); err != nil {
		return err
	}
	pathLock := lockForPath(v.path)
	pathLock.Lock()
	defer pathLock.Unlock()

	if err := v.reloadWithCurrentKeyLocked(); err != nil {
		return err
	}
	salt := make([]byte, 32)
	if _, err := rand.Read(salt); err != nil {
		return err
	}
	exportKey, err := derive(backupPassword, salt, iterations)
	if err != nil {
		return err
	}
	defer zero(exportKey)

	plain, err := json.Marshal(v.data)
	if err != nil {
		return err
	}
	block, err := aes.NewCipher(exportKey)
	if err != nil {
		return err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return err
	}
	ct := gcm.Seal(nil, nonce, plain, []byte("eon:credential-vault:v1"))
	zero(plain)
	env := envelope{
		Version:    formatVersion,
		Iterations: iterations,
		Salt:       base64.StdEncoding.EncodeToString(salt),
		Verifier:   base64.StdEncoding.EncodeToString(verifier(exportKey)),
		Nonce:      base64.StdEncoding.EncodeToString(nonce),
		Ciphertext: base64.StdEncoding.EncodeToString(ct),
	}
	raw, err := json.Marshal(env)
	if err != nil {
		return err
	}
	dir := filepath.Dir(backupPath)
	if err = os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".export-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err = tmp.Chmod(0600); err == nil {
		_, err = tmp.Write(raw)
	}
	if err == nil {
		err = tmp.Sync()
	}
	closeErr := tmp.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = os.Rename(tmpName, backupPath); err != nil {
		return err
	}
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	err = d.Sync()
	closeErr = d.Close()
	if err != nil {
		return err
	}
	v.lastUsed = v.now()
	return closeErr
}

// Import reads an encrypted export file created by Export and merges the
// secrets into the current vault. The vault must be unlocked. If a secret
// from the backup already exists in the vault, Import returns
// ErrImportConflict for the first conflict and no changes are applied.
func (v *Vault) Import(backupPath, backupPassword string) error {
	return v.ImportWithOptions(backupPath, backupPassword, ImportOptions{Mode: ImportModeFail})
}

// ImportWithOptions reads an encrypted export file created by Export and merges the
// secrets into the current vault according to the specified ImportOptions.
func (v *Vault) ImportWithOptions(backupPath, backupPassword string, opts ImportOptions) error {
	if opts.Mode != ImportModeFail && opts.Mode != ImportModeSkip && opts.Mode != ImportModeOverwrite {
		return ErrInvalidImportMode
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	v.expireLocked()
	if len(v.key) == 0 {
		return ErrLocked
	}
	backupPath = strings.TrimSpace(backupPath)
	if backupPath == "" {
		return ErrInvalidBackupPath
	}
	if err := validatePassword(backupPassword); err != nil {
		return err
	}
	pathLock := lockForPath(v.path)
	pathLock.Lock()
	defer pathLock.Unlock()

	if err := v.reloadWithCurrentKeyLocked(); err != nil {
		return err
	}

	raw, err := os.ReadFile(backupPath)
	if err != nil {
		return err
	}
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return ErrInvalidBackupFormat
	}
	if env.Version != formatVersion {
		return ErrInvalidBackupFormat
	}
	salt, err := base64.StdEncoding.DecodeString(env.Salt)
	if err != nil || len(salt) == 0 {
		return ErrInvalidBackupFormat
	}
	nonce, err := base64.StdEncoding.DecodeString(env.Nonce)
	if err != nil || len(nonce) == 0 {
		return ErrInvalidBackupFormat
	}
	ct, err := base64.StdEncoding.DecodeString(env.Ciphertext)
	if err != nil || len(ct) == 0 {
		return ErrInvalidBackupFormat
	}
	exportKey, err := derive(backupPassword, salt, env.Iterations)
	if err != nil {
		return err
	}
	defer zero(exportKey)

	// Verify the backup key via the verifier before attempting decrypt.
	expectedVerifier := verifier(exportKey)
	gotVerifier, err := base64.StdEncoding.DecodeString(env.Verifier)
	if err != nil || subtle.ConstantTimeCompare(expectedVerifier, gotVerifier) != 1 {
		return ErrInvalidPassword
	}

	plain, err := open(exportKey, nonce, ct)
	if err != nil {
		return ErrInvalidPassword
	}
	defer zero(plain)

	var imported payload
	if err := json.Unmarshal(plain, &imported); err != nil {
		return ErrInvalidBackupFormat
	}
	if imported.Secrets == nil {
		return ErrInvalidBackupFormat
	}

	// Validate names and value sizes before modifying state.
	for name, rec := range imported.Secrets {
		if err := validateName(name); err != nil {
			return err
		}
		if rec.Value == "" || len(rec.Value) > maxSecretSize {
			return ErrInvalidSecretValue
		}
	}

	// Detect conflicts if Mode is ImportModeFail.
	if opts.Mode == ImportModeFail {
		for name := range imported.Secrets {
			if _, exists := v.data.Secrets[name]; exists {
				return ErrImportConflict
			}
		}
	}

	now := v.now().UTC()
	var modified bool
	for name, rec := range imported.Secrets {
		_, exists := v.data.Secrets[name]
		if exists && opts.Mode == ImportModeSkip {
			continue
		}
		rec.CreatedAt = now
		rec.UpdatedAt = now
		v.data.Secrets[name] = rec
		modified = true
	}
	v.lastUsed = v.now()
	if !modified && len(imported.Secrets) > 0 {
		return nil
	}
	return v.saveWithCurrentKeyLocked()
}

func (v *Vault) isLockedOutLocked() bool {
	if !v.lockedUntil.IsZero() {
		if v.now().Before(v.lockedUntil) {
			return true
		}
		v.lockedUntil = time.Time{}
		v.failedCount = 0
	}
	return false
}

func (v *Vault) recordFailedAttemptLocked() {
	v.failedCount++
	if v.failedCount >= maxFailedAttempts {
		v.lockedUntil = v.now().Add(lockoutDuration)
	}
}

func (v *Vault) expireLocked() {
	if len(v.key) != 0 && !v.lastUsed.IsZero() && v.now().Sub(v.lastUsed) >= autoLockAfter {
		v.key = nil
		v.data = payload{Secrets: map[string]record{}}
		v.lastUsed = time.Time{}
	}
}

func (v *Vault) saveWithCurrentKeyLocked() error {
	raw, err := os.ReadFile(v.path)
	if err != nil {
		return err
	}
	var env envelope
	if err = json.Unmarshal(raw, &env); err != nil {
		return err
	}
	salt, err := base64.StdEncoding.DecodeString(env.Salt)
	if err != nil {
		return err
	}
	return v.saveLocked(salt, env.Iterations)
}

func (v *Vault) reloadWithCurrentKeyLocked() error {
	raw, err := os.ReadFile(v.path)
	if err != nil {
		return err
	}
	var env envelope
	if err = json.Unmarshal(raw, &env); err != nil {
		return fmt.Errorf("decode vault: %w", err)
	}
	nonce, err := base64.StdEncoding.DecodeString(env.Nonce)
	if err != nil {
		return errors.New("invalid vault nonce")
	}
	ct, err := base64.StdEncoding.DecodeString(env.Ciphertext)
	if err != nil {
		return errors.New("invalid vault ciphertext")
	}
	plain, err := open(v.key, nonce, ct)
	if err != nil {
		return ErrInvalidPassword
	}
	defer zero(plain)
	var p payload
	if err = json.Unmarshal(plain, &p); err != nil {
		return errors.New("invalid vault payload")
	}
	if p.Secrets == nil {
		p.Secrets = map[string]record{}
	}
	v.data = p
	return nil
}
func (v *Vault) saveLocked(salt []byte, iter int) error {
	plain, err := json.Marshal(v.data)
	if err != nil {
		return err
	}
	block, err := aes.NewCipher(v.key)
	if err != nil {
		return err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return err
	}
	ct := gcm.Seal(nil, nonce, plain, []byte("eon:credential-vault:v1"))
	zero(plain)
	env := envelope{Version: formatVersion, Iterations: iter, Salt: base64.StdEncoding.EncodeToString(salt), Verifier: base64.StdEncoding.EncodeToString(verifier(v.key)), Nonce: base64.StdEncoding.EncodeToString(nonce), Ciphertext: base64.StdEncoding.EncodeToString(ct)}
	raw, err := json.Marshal(env)
	if err != nil {
		return err
	}
	dir := filepath.Dir(v.path)
	if err = os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".vault-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err = tmp.Chmod(0600); err == nil {
		_, err = tmp.Write(raw)
	}
	if err == nil {
		err = tmp.Sync()
	}
	closeErr := tmp.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = os.Rename(tmpName, v.path); err != nil {
		return err
	}
	// Persist the directory-entry replacement, not only the temporary bytes.
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	err = d.Sync()
	closeErr = d.Close()
	if err != nil {
		return err
	}
	return closeErr
}
func derive(password string, salt []byte, iter int) ([]byte, error) {
	return pbkdf2.Key(sha256.New, password, salt, iter, 32)
}
func verifier(key []byte) []byte {
	h := sha256.New()
	h.Write([]byte("eon:credential-vault:verify:v1"))
	h.Write(key)
	return h.Sum(nil)
}
func open(key, nonce, ct []byte) ([]byte, error) {
	b, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	g, err := cipher.NewGCM(b)
	if err != nil {
		return nil, err
	}
	return g.Open(nil, nonce, ct, []byte("eon:credential-vault:v1"))
}
func validatePassword(p string) error {
	if len(p) < 12 || len(p) > 1024 {
		return ErrInvalidPasswordLength
	}
	return nil
}
func validateName(n string) error {
	if n == "" || len(n) > 256 {
		return ErrInvalidSecretName
	}
	if strings.TrimSpace(n) != n {
		return ErrInvalidSecretName
	}
	if strings.ContainsAny(n, "\x00\r\n\t") {
		return ErrInvalidSecretName
	}
	if strings.Contains(n, "\\") || strings.Contains(n, "//") || strings.HasPrefix(n, "/") || strings.HasSuffix(n, "/") {
		return ErrInvalidSecretName
	}
	for _, part := range strings.Split(n, "/") {
		if part == "." || part == ".." || strings.TrimSpace(part) != part || part == "" {
			return ErrInvalidSecretName
		}
	}
	return nil
}
func zero(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
