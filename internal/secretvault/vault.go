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
}

type payload struct {
	Secrets map[string]record `json:"secrets"`
}

type Metadata struct {
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
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

func (v *Vault) Status() Status {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.expireLocked()
	_, err := os.Stat(v.path)
	s := Status{Initialized: err == nil, Locked: len(v.key) == 0}
	if len(v.key) != 0 {
		for name, r := range v.data.Secrets {
			s.Secrets = append(s.Secrets, Metadata{Name: name, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt})
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
		return ErrInitialized
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := validatePassword(password); err != nil {
		return err
	}
	salt := make([]byte, 32)
	if _, err := rand.Read(salt); err != nil {
		return err
	}
	key, err := derive(password, salt, iterations)
	if err != nil {
		return err
	}
	v.key = key
	v.data = payload{Secrets: map[string]record{}}
	if err := v.saveLocked(salt, iterations); err != nil {
		zero(v.key)
		v.key = nil
		return err
	}
	v.lastUsed = v.now()
	return nil
}

func (v *Vault) Unlock(password string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.expireLocked()
	if v.isLockedOutLocked() {
		return ErrAccountLockedOut
	}
	raw, err := os.ReadFile(v.path)
	if errors.Is(err, os.ErrNotExist) {
		return ErrUninitialized
	}
	if err != nil {
		return err
	}
	var env envelope
	if err = json.Unmarshal(raw, &env); err != nil {
		return fmt.Errorf("decode vault: %w", err)
	}
	if env.Version != formatVersion || env.Iterations < 100_000 {
		return errors.New("unsupported vault format")
	}
	salt, err := base64.StdEncoding.DecodeString(env.Salt)
	if err != nil {
		return errors.New("invalid vault salt")
	}
	key, err := derive(password, salt, env.Iterations)
	if err != nil {
		return err
	}
	want, err := base64.StdEncoding.DecodeString(env.Verifier)
	if err != nil {
		return errors.New("invalid vault verifier")
	}
	got := verifier(key)
	if len(want) != len(got) || subtle.ConstantTimeCompare(want, got) != 1 {
		zero(key)
		v.recordFailedAttemptLocked()
		return ErrInvalidPassword
	}
	nonce, err := base64.StdEncoding.DecodeString(env.Nonce)
	if err != nil {
		zero(key)
		return errors.New("invalid vault nonce")
	}
	ct, err := base64.StdEncoding.DecodeString(env.Ciphertext)
	if err != nil {
		zero(key)
		return errors.New("invalid vault ciphertext")
	}
	plain, err := open(key, nonce, ct)
	if err != nil {
		zero(key)
		v.recordFailedAttemptLocked()
		return ErrInvalidPassword
	}
	var p payload
	err = json.Unmarshal(plain, &p)
	zero(plain)
	if err != nil {
		zero(key)
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
	return nil
}

func (v *Vault) Lock() {
	v.mu.Lock()
	defer v.mu.Unlock()
	zero(v.key)
	v.key = nil
	v.data = payload{Secrets: map[string]record{}}
	v.lastUsed = time.Time{}
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
}

func (v *Vault) Put(name, value string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.expireLocked()
	if len(v.key) == 0 {
		return ErrLocked
	}
	if err := validateName(name); err != nil {
		return err
	}
	if value == "" || len(value) > maxSecretSize {
		return ErrInvalidSecretValue
	}
	pathLock := lockForPath(v.path)
	pathLock.Lock()
	defer pathLock.Unlock()
	if err := v.reloadWithCurrentKeyLocked(); err != nil {
		return err
	}
	now := v.now().UTC()
	r, ok := v.data.Secrets[name]
	if !ok {
		r.CreatedAt = now
	}
	r.Value = value
	r.UpdatedAt = now
	v.data.Secrets[name] = r
	v.lastUsed = v.now()
	return v.saveWithCurrentKeyLocked()
}

func (v *Vault) Delete(name string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.expireLocked()
	if len(v.key) == 0 {
		return ErrLocked
	}
	if err := validateName(name); err != nil {
		return err
	}
	pathLock := lockForPath(v.path)
	pathLock.Lock()
	defer pathLock.Unlock()
	if err := v.reloadWithCurrentKeyLocked(); err != nil {
		return err
	}
	delete(v.data.Secrets, name)
	v.lastUsed = v.now()
	return v.saveWithCurrentKeyLocked()
}

func (v *Vault) Resolve(name string) (string, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.expireLocked()
	if len(v.key) == 0 {
		return "", ErrLocked
	}
	r, ok := v.data.Secrets[name]
	if !ok {
		return "", os.ErrNotExist
	}
	v.lastUsed = v.now()
	return r.Value, nil
}

func (v *Vault) ChangePassword(oldPassword, newPassword string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.expireLocked()
	if len(v.key) == 0 {
		return ErrLocked
	}
	if v.isLockedOutLocked() {
		return ErrAccountLockedOut
	}
	if err := validatePassword(newPassword); err != nil {
		return err
	}
	raw, err := os.ReadFile(v.path)
	if err != nil {
		return err
	}
	var env envelope
	if err = json.Unmarshal(raw, &env); err != nil {
		return fmt.Errorf("decode vault: %w", err)
	}
	salt, err := base64.StdEncoding.DecodeString(env.Salt)
	if err != nil {
		return errors.New("invalid vault salt")
	}
	testKey, err := derive(oldPassword, salt, env.Iterations)
	if err != nil {
		return err
	}
	if len(testKey) != len(v.key) || subtle.ConstantTimeCompare(testKey, v.key) != 1 {
		zero(testKey)
		v.recordFailedAttemptLocked()
		return ErrInvalidPassword
	}
	zero(testKey)

	pathLock := lockForPath(v.path)
	pathLock.Lock()
	defer pathLock.Unlock()

	if err := v.reloadWithCurrentKeyLocked(); err != nil {
		return err
	}
	newSalt := make([]byte, 32)
	if _, err := rand.Read(newSalt); err != nil {
		return err
	}
	newKey, err := derive(newPassword, newSalt, iterations)
	if err != nil {
		return err
	}
	oldKey := v.key
	v.key = newKey
	if err := v.saveLocked(newSalt, iterations); err != nil {
		zero(newKey)
		v.key = oldKey
		return err
	}
	zero(oldKey)
	v.lastUsed = v.now()
	v.failedCount = 0
	v.lockedUntil = time.Time{}
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
	if n == "" || len(n) > 256 || strings.ContainsAny(n, "\x00\r\n") {
		return ErrInvalidSecretName
	}
	return nil
}
func zero(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
