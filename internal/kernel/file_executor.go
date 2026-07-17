package kernel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/runtime/source"
)

// File path audit event kinds. File bytes are untrusted source data only
// (FR-RES-002); never policy, never executable instructions.
const (
	EventOperationFileInvoked  = "operation.file_invoked"
	EventOperationFileVerified = "operation.file_verified"
)

// FileRoot is a named absolute directory the operator authorized for READ_ONLY
// file.discover / file.read. Path must already be cleaned and absolute.
type FileRoot struct {
	Name string
	Path string
}

// FileExecutor runs READ_ONLY file.discover / file.read under authorized roots
// with FR-RES-001 PolicyEngine + ResourceGate when Authorizer is set.
// Path resolution is fail-closed: no traversal outside roots, no symlink escape.
type FileExecutor struct {
	Store port.Store
	Clock source.Clock
	IDs   source.IDGenerator
	Roots []FileRoot
	// Authorizer is optional FR-RES-001 enforcement (nil = legacy allow).
	Authorizer *CapabilityAuthorizer
	LeaseTTL   time.Duration
	// MaxReadBytes caps file.read content (0 = 1 MiB default).
	MaxReadBytes int64
	// MaxDiscoverEntries caps directory listing entries (0 = 256 default).
	MaxDiscoverEntries int
}

// FileExecuteResult summarizes one file-backed Execute call.
type FileExecuteResult struct {
	OperationID domain.OperationID
	Completed   bool
	Skipped     bool
	SkipReason  string
	LeaseRef    string
	ArtifactID  domain.ArtifactID
	Capability  string
	// BytesRead is set for file.read.
	BytesRead int
	// EntryCount is set for file.discover.
	EntryCount int
}

func (e FileExecutor) validateDeps() error {
	if e.Store == nil || e.Clock == nil || e.IDs == nil {
		return errors.New("file executor dependencies are incomplete")
	}
	if len(e.Roots) == 0 {
		return errors.New("file executor requires at least one authorized root")
	}
	for _, root := range e.Roots {
		if strings.TrimSpace(root.Name) == "" || !filepath.IsAbs(root.Path) {
			return fmt.Errorf("file root %q must have a name and absolute path", root.Name)
		}
	}
	return nil
}

func (e FileExecutor) leaseTTL() time.Duration {
	if e.LeaseTTL <= 0 {
		return 5 * time.Minute
	}
	return e.LeaseTTL
}

func (e FileExecutor) maxReadBytes() int64 {
	if e.MaxReadBytes > 0 {
		return e.MaxReadBytes
	}
	return 1 << 20
}

func (e FileExecutor) maxDiscoverEntries() int {
	if e.MaxDiscoverEntries > 0 {
		return e.MaxDiscoverEntries
	}
	return 256
}

// FileEligible reports whether an OperationSpec should run on the file path.
func FileEligible(spec domain.OperationSpec) bool {
	if err := spec.Validate(); err != nil {
		return false
	}
	id := string(spec.ID)
	if strings.HasPrefix(id, "continuity.") {
		return false
	}
	if spec.MaximumAuthority == domain.AuthorityProposeOnly && fileCapabilityFromSpec(spec) == "" {
		return false
	}
	capName := fileCapabilityFromSpec(spec)
	return capName == "file.discover" || capName == "file.read"
}

// fileCapabilityFromSpec maps OperationSpec identity/schemas to a catalog capability.
func fileCapabilityFromSpec(spec domain.OperationSpec) string {
	id := strings.ToLower(string(spec.ID))
	in := strings.ToLower(spec.InputSchema)
	out := strings.ToLower(spec.OutputSchema)
	switch {
	case strings.HasPrefix(id, "file.discover") || strings.Contains(in, "file.discover") || strings.Contains(out, "file.discover"):
		return "file.discover"
	case strings.HasPrefix(id, "file.read") || strings.Contains(in, "file.read") || strings.Contains(out, "file.read"):
		return "file.read"
	default:
		return ""
	}
}

// FileReadCost is the ResourceGate cost for one file.read call.
func FileReadCost(expectedBytes int64) domain.ResourceCost {
	if expectedBytes < 0 {
		expectedBytes = 0
	}
	return domain.ResourceCost{Slots: 1, Calls: 1, Bytes: expectedBytes}
}

// FileDiscoverCost is the ResourceGate cost for one file.discover call.
func FileDiscoverCost() domain.ResourceCost {
	return domain.ResourceCost{Slots: 1, Calls: 1}
}

// FileCapabilityEstimatedBudget is the Budget slice reserved for file ops.
func FileCapabilityEstimatedBudget(spec domain.OperationSpec) domain.Budget {
	attempts := 1
	var bytes int64
	if spec.Budget.Bytes > 0 {
		bytes = spec.Budget.Bytes
		if bytes > 1<<20 {
			bytes = 1 << 20
		}
	}
	return domain.Budget{
		Attempts: attempts,
		Bytes:    bytes,
	}
}

func (e FileExecutor) releaseResourcePermit(ctx context.Context, operation domain.Operation, permit *domain.ResourcePermit, success bool) {
	if e.Authorizer == nil || permit == nil {
		return
	}
	_ = e.Authorizer.ReportCapability(ctx, operation, permit, success, nil)
}

// Execute runs one READY, file-eligible operation.
func (e FileExecutor) Execute(ctx context.Context, operationID domain.OperationID) (FileExecuteResult, error) {
	if err := e.validateDeps(); err != nil {
		return FileExecuteResult{}, err
	}
	if operationID == "" {
		return FileExecuteResult{}, errors.New("operation id is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	var result FileExecuteResult
	result.OperationID = operationID

	var (
		operation domain.Operation
		spec      domain.OperationSpec
		leaseRef  string
		now       time.Time
		permit    *domain.ResourcePermit
		capName   string
	)

	err := e.Store.View(ctx, func(r port.Reader) error {
		op, err := r.Operation(operationID)
		if err != nil {
			return err
		}
		if op.State.Terminal() {
			result.Skipped = true
			result.SkipReason = "terminal"
			return nil
		}
		if op.State != domain.StateReady {
			result.Skipped = true
			result.SkipReason = "not_ready"
			return nil
		}
		loadedSpec, err := r.OperationSpec(op.SpecID)
		if err != nil {
			return fmt.Errorf("load operation spec %s: %w", op.SpecID, err)
		}
		if !FileEligible(loadedSpec) {
			result.Skipped = true
			result.SkipReason = "not_file_eligible"
			return nil
		}
		operation = op
		spec = loadedSpec
		capName = fileCapabilityFromSpec(loadedSpec)
		result.Capability = capName
		return nil
	})
	if err != nil {
		return result, err
	}
	if result.Skipped {
		return result, nil
	}

	args, argsErr := parseFileArgs(operation, capName)
	if argsErr != nil {
		result.Skipped = true
		result.SkipReason = "invalid_file_args"
		return result, nil
	}
	resolved, resolveErr := e.resolvePath(args)
	if resolveErr != nil {
		result.Skipped = true
		result.SkipReason = "path_not_authorized"
		return result, nil
	}

	if e.Authorizer != nil {
		reserve := CapabilityReserveRequest{
			Capability:      capName,
			ArgsDigest:      fileArgsDigest(capName, args, resolved.Rel),
			Operation:       operation,
			Spec:            spec,
			EstimatedCost:   FileCapabilityEstimatedBudget(spec),
			AvailableBudget: spec.Budget,
			Priority:        0,
		}
		switch capName {
		case "file.discover":
			reserve.ResourceCost = FileDiscoverCost()
			reserve.DefaultResource = "file:authorized-root"
		case "file.read":
			reserve.ResourceCost = FileReadCost(0)
			reserve.DefaultResource = "file:authorized-root"
		}
		auth, authErr := e.Authorizer.ReserveCapability(ctx, reserve)
		if authErr != nil {
			return result, authErr
		}
		if auth.Throttled {
			result.Skipped = true
			result.SkipReason = auth.SkipReason
			if result.SkipReason == "" {
				result.SkipReason = "resource_throttled"
			}
			return result, nil
		}
		if !auth.Allowed {
			result.Skipped = true
			result.SkipReason = auth.SkipReason
			if result.SkipReason == "" {
				result.SkipReason = "policy_deny"
			}
			return result, nil
		}
		permit = auth.Permit
	}

	// Phase 1: claim lease READY → RUNNING.
	err = e.Store.Update(ctx, func(tx port.Transaction) error {
		op, err := tx.Operation(operationID)
		if err != nil {
			return err
		}
		if op.State.Terminal() {
			result.Skipped = true
			result.SkipReason = "terminal"
			return nil
		}
		if op.State != domain.StateReady {
			result.Skipped = true
			result.SkipReason = "not_ready"
			return nil
		}
		loadedSpec, err := tx.OperationSpec(op.SpecID)
		if err != nil {
			return fmt.Errorf("load operation spec %s: %w", op.SpecID, err)
		}
		if !FileEligible(loadedSpec) {
			result.Skipped = true
			result.SkipReason = "not_file_eligible"
			return nil
		}
		leaseID, err := e.IDs.NewID("lease")
		if err != nil {
			return fmt.Errorf("generate lease id: %w", err)
		}
		if strings.TrimSpace(leaseID) == "" {
			return errors.New("generated lease id must not be empty")
		}
		now = e.Clock.Now().UTC()
		until := now.Add(e.leaseTTL())
		leaseRef = FormatLeaseRef(leaseID, op.ID, op.Attempt+1, until)
		result.LeaseRef = leaseRef

		snap := domain.OperationalSnapshot{State: op.State, Reevaluation: op.Reevaluation}
		running, err := domain.Transition(snap, domain.TransitionInput{Event: domain.EventDispatch, Reference: leaseRef})
		if err != nil {
			return fmt.Errorf("dispatch: %w", err)
		}
		op.State = running.State
		op.Reevaluation = running.Reevaluation
		op.Attempt++
		if err := tx.SaveOperation(op); err != nil {
			return err
		}
		if _, err := tx.AppendEvent(domain.Event{
			SchemaVersion:   domain.SchemaVersionV1,
			ID:              domain.EventID(fmt.Sprintf("%s:dispatched:%d", op.ID, op.Attempt)),
			Kind:            EventOperationDispatched,
			OccurredAt:      now,
			MissionRevision: op.MissionRevision,
			InquiryID:       op.InquiryID,
			OperationID:     op.ID,
			PayloadRef:      leaseRef + ";capability=" + capName,
		}); err != nil {
			return err
		}
		operation = op
		spec = loadedSpec
		return nil
	})
	if err != nil {
		e.releaseResourcePermit(ctx, operation, permit, false)
		return result, err
	}
	if result.Skipped {
		e.releaseResourcePermit(ctx, operation, permit, false)
		return result, nil
	}

	// Phase 2: host effect outside the write transaction.
	var (
		auditBody map[string]any
		effectOK  bool
		effectErr error
	)
	switch capName {
	case "file.discover":
		entries, err := e.discover(resolved, args.Pattern)
		if err != nil {
			effectErr = fmt.Errorf("file discover: %w", err)
			break
		}
		result.EntryCount = len(entries)
		auditBody = map[string]any{
			"capability": "file.discover",
			"root":       resolved.RootName,
			"path":       resolved.Rel,
			"pattern":    args.Pattern,
			"entries":    entries,
			"trust":      "untrusted_source_data",
		}
		effectOK = true
	case "file.read":
		content, media, err := e.readFile(resolved)
		if err != nil {
			effectErr = fmt.Errorf("file read: %w", err)
			break
		}
		result.BytesRead = len(content)
		auditBody = map[string]any{
			"capability": "file.read",
			"root":       resolved.RootName,
			"path":       resolved.Rel,
			"bytes":      len(content),
			"media_type": media,
			// Content is audit-bound and untrusted; large bodies stay truncated at max.
			"content": string(content),
			"trust":   "untrusted_source_data",
		}
		effectOK = true
	default:
		effectErr = fmt.Errorf("unsupported file capability %q", capName)
	}

	if !effectOK {
		failErr := e.failRunning(ctx, operation, leaseRef, effectErr)
		e.releaseResourcePermit(ctx, operation, permit, false)
		return result, failErr
	}

	// Phase 3: VERIFYING → audit artifact → SUCCEEDED.
	err = e.Store.Update(ctx, func(tx port.Transaction) error {
		op, err := tx.Operation(operationID)
		if err != nil {
			return err
		}
		if op.State != domain.StateRunning || op.Reevaluation.Reference != leaseRef {
			return fmt.Errorf("%w: operation lease changed during file effect", port.ErrConflict)
		}
		verifying, err := domain.Transition(
			domain.OperationalSnapshot{State: op.State, Reevaluation: op.Reevaluation},
			domain.TransitionInput{Event: domain.EventBeginVerify, Reference: leaseRef},
		)
		if err != nil {
			return fmt.Errorf("begin verify: %w", err)
		}
		op.State = verifying.State
		op.Reevaluation = verifying.Reevaluation

		now = e.Clock.Now().UTC()
		artifact, err := e.buildFileArtifact(tx, op, spec, leaseRef, capName, auditBody, now)
		if err != nil {
			return err
		}
		if artifact.ID != "" {
			if err := tx.AppendKnowledgeArtifact(artifact); err != nil {
				return fmt.Errorf("append file artifact: %w", err)
			}
			result.ArtifactID = artifact.ID
		}

		done, err := domain.Transition(
			domain.OperationalSnapshot{State: op.State, Reevaluation: op.Reevaluation},
			domain.TransitionInput{Event: domain.EventSucceed},
		)
		if err != nil {
			return fmt.Errorf("succeed: %w", err)
		}
		op.State = done.State
		op.Reevaluation = done.Reevaluation
		if err := tx.SaveOperation(op); err != nil {
			return err
		}

		payload := leaseRef + ";capability=" + capName + ";path=" + resolved.Rel
		if result.EntryCount > 0 {
			payload += fmt.Sprintf(";entries=%d", result.EntryCount)
		}
		if result.BytesRead > 0 {
			payload += fmt.Sprintf(";bytes=%d", result.BytesRead)
		}

		for _, event := range []domain.Event{
			{
				SchemaVersion: domain.SchemaVersionV1,
				ID:            domain.EventID(fmt.Sprintf("%s:file_invoked:%d", op.ID, op.Attempt)),
				Kind:          EventOperationFileInvoked,
				OccurredAt:    now, MissionRevision: op.MissionRevision, InquiryID: op.InquiryID,
				OperationID: op.ID, PayloadRef: payload,
			},
			{
				SchemaVersion: domain.SchemaVersionV1,
				ID:            domain.EventID(fmt.Sprintf("%s:file_verified:%d", op.ID, op.Attempt)),
				Kind:          EventOperationFileVerified,
				OccurredAt:    now, MissionRevision: op.MissionRevision, InquiryID: op.InquiryID,
				OperationID: op.ID, PayloadRef: payload,
			},
			{
				SchemaVersion: domain.SchemaVersionV1,
				ID:            domain.EventID(fmt.Sprintf("%s:succeeded:%d", op.ID, op.Attempt)),
				Kind:          EventOperationSucceeded,
				OccurredAt:    now, MissionRevision: op.MissionRevision, InquiryID: op.InquiryID,
				OperationID: op.ID, PayloadRef: payload,
			},
		} {
			if _, err := tx.AppendEvent(event); err != nil {
				return err
			}
		}
		result.Completed = true
		return nil
	})
	if err != nil {
		e.releaseResourcePermit(ctx, operation, permit, false)
		return result, err
	}
	e.releaseResourcePermit(ctx, operation, permit, true)
	return result, nil
}

type fileArgs struct {
	Root    string
	Path    string
	Pattern string
}

type resolvedFilePath struct {
	RootName string
	RootPath string
	Rel      string
	Abs      string
}

func parseFileArgs(operation domain.Operation, capability string) (fileArgs, error) {
	var out fileArgs
	// InputRefs convention:
	//   root:<name>
	//   path:<relative-or-dot>
	//   pattern:<glob> (discover)
	//   bare relative path as last resort for read
	for _, ref := range operation.InputRefs {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}
		lower := strings.ToLower(ref)
		switch {
		case strings.HasPrefix(lower, "root:"):
			out.Root = strings.TrimSpace(ref[len("root:"):])
		case strings.HasPrefix(lower, "path:"):
			out.Path = strings.TrimSpace(ref[len("path:"):])
		case strings.HasPrefix(lower, "pattern:"):
			out.Pattern = strings.TrimSpace(ref[len("pattern:"):])
		case strings.HasPrefix(lower, "file:"):
			out.Path = strings.TrimSpace(ref[len("file:"):])
		default:
			// Accept a single bare relative path when no path: prefix was given.
			if out.Path == "" && !strings.Contains(ref, ":") {
				out.Path = ref
			}
		}
	}
	if out.Path == "" {
		out.Path = "."
	}
	if out.Pattern == "" && capability == "file.discover" {
		out.Pattern = "*"
	}
	// Reject absolute and parent-escape tokens early (still re-checked after Clean).
	if filepath.IsAbs(out.Path) {
		return out, errors.New("file path must be relative to an authorized root")
	}
	if strings.Contains(out.Path, "\x00") {
		return out, errors.New("file path contains NUL")
	}
	return out, nil
}

func fileArgsDigest(capability string, args fileArgs, rel string) string {
	return fmt.Sprintf("%s:root=%s:path=%s:pattern=%s", capability, args.Root, rel, args.Pattern)
}

func (e FileExecutor) resolvePath(args fileArgs) (resolvedFilePath, error) {
	root, err := e.pickRoot(args.Root)
	if err != nil {
		return resolvedFilePath{}, err
	}
	raw := strings.ReplaceAll(strings.TrimSpace(args.Path), "\\", "/")
	if raw == "" {
		raw = "."
	}
	if filepath.IsAbs(raw) || strings.HasPrefix(raw, "/") {
		return resolvedFilePath{}, errors.New("file path must be relative to an authorized root")
	}
	// Reject any ".." segment before Clean can collapse absolute-style escapes.
	for _, seg := range strings.Split(raw, "/") {
		if seg == ".." {
			return resolvedFilePath{}, errors.New("path escapes authorized root")
		}
	}
	rel := filepath.Clean(raw)
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return resolvedFilePath{}, errors.New("path escapes authorized root")
	}
	if rel == "." {
		rel = ""
	}
	abs := root.Path
	if rel != "" {
		abs = filepath.Join(root.Path, rel)
	}
	abs = filepath.Clean(abs)
	// Lexical containment under the configured root path.
	if abs != root.Path && !strings.HasPrefix(abs, root.Path+string(os.PathSeparator)) {
		return resolvedFilePath{}, errors.New("path escapes authorized root")
	}
	rootReal, err := filepath.EvalSymlinks(root.Path)
	if err != nil {
		return resolvedFilePath{}, fmt.Errorf("authorized root %q: %w", root.Name, err)
	}
	rootReal = filepath.Clean(rootReal)
	targetReal := abs
	if info, statErr := os.Lstat(abs); statErr == nil {
		resolved, err := filepath.EvalSymlinks(abs)
		if err != nil {
			return resolvedFilePath{}, fmt.Errorf("resolve path: %w", err)
		}
		targetReal = filepath.Clean(resolved)
		_ = info
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return resolvedFilePath{}, statErr
	}
	if targetReal != rootReal && !strings.HasPrefix(targetReal, rootReal+string(os.PathSeparator)) {
		return resolvedFilePath{}, errors.New("path escapes authorized root")
	}
	displayRel := rel
	if displayRel == "" {
		displayRel = "."
	}
	return resolvedFilePath{
		RootName: root.Name,
		RootPath: root.Path,
		Rel:      displayRel,
		Abs:      abs,
	}, nil
}

func (e FileExecutor) pickRoot(name string) (FileRoot, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		if len(e.Roots) == 1 {
			return e.Roots[0], nil
		}
		// Prefer root named "default" when multiple.
		for _, r := range e.Roots {
			if r.Name == "default" {
				return r, nil
			}
		}
		return FileRoot{}, errors.New("root name required when multiple authorized roots are configured")
	}
	for _, r := range e.Roots {
		if r.Name == name {
			return r, nil
		}
	}
	return FileRoot{}, fmt.Errorf("unknown authorized root %q", name)
}

type fileEntry struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	IsDir bool   `json:"is_dir"`
	Size  int64  `json:"size,omitempty"`
}

func (e FileExecutor) discover(resolved resolvedFilePath, pattern string) ([]fileEntry, error) {
	info, err := os.Stat(resolved.Abs)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, errors.New("discover path is not a directory")
	}
	if pattern == "" {
		pattern = "*"
	}
	// Bound listing: read directory entries without following the dir as symlink escape
	// (resolvePath already constrained Abs).
	dirEntries, err := os.ReadDir(resolved.Abs)
	if err != nil {
		return nil, err
	}
	limit := e.maxDiscoverEntries()
	out := make([]fileEntry, 0, min(len(dirEntries), limit))
	// Deterministic order.
	sort.Slice(dirEntries, func(i, j int) bool {
		return dirEntries[i].Name() < dirEntries[j].Name()
	})
	for _, de := range dirEntries {
		name := de.Name()
		matched, err := filepath.Match(pattern, name)
		if err != nil {
			return nil, fmt.Errorf("invalid discover pattern: %w", err)
		}
		if !matched {
			continue
		}
		rel := name
		if resolved.Rel != "." && resolved.Rel != "" {
			rel = filepath.ToSlash(filepath.Join(resolved.Rel, name))
		}
		entry := fileEntry{Name: name, Path: rel, IsDir: de.IsDir()}
		if !de.IsDir() {
			if fi, err := de.Info(); err == nil {
				entry.Size = fi.Size()
			}
		}
		out = append(out, entry)
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (e FileExecutor) readFile(resolved resolvedFilePath) ([]byte, string, error) {
	info, err := os.Lstat(resolved.Abs)
	if err != nil {
		return nil, "", err
	}
	if info.IsDir() {
		return nil, "", errors.New("file.read path is a directory")
	}
	if info.Mode()&os.ModeSymlink != 0 {
		// Symlink target already constrained in resolvePath; open via resolved path.
	}
	if !info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 {
		// Allow only regular files (or symlinks to files opened below).
		if info.Mode().Type() != 0 && info.Mode()&os.ModeSymlink == 0 {
			return nil, "", fmt.Errorf("unsupported file mode %s", info.Mode())
		}
	}
	f, err := os.Open(resolved.Abs)
	if err != nil {
		return nil, "", err
	}
	defer f.Close()
	// Re-check after open (TOCTOU): must be regular file.
	st, err := f.Stat()
	if err != nil {
		return nil, "", err
	}
	if st.IsDir() {
		return nil, "", errors.New("file.read path is a directory")
	}
	limit := e.maxReadBytes()
	limited := io.LimitReader(f, limit+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return nil, "", err
	}
	if int64(len(body)) > limit {
		return nil, "", fmt.Errorf("file exceeds max read bytes %d", limit)
	}
	media := "application/octet-stream"
	if isMostlyText(body) {
		media = "text/plain"
	}
	return body, media, nil
}

func isMostlyText(b []byte) bool {
	if len(b) == 0 {
		return true
	}
	sample := b
	if len(sample) > 512 {
		sample = sample[:512]
	}
	for _, c := range sample {
		if c == 0 {
			return false
		}
	}
	return true
}

func (e FileExecutor) buildFileArtifact(
	tx port.Transaction,
	operation domain.Operation,
	spec domain.OperationSpec,
	leaseRef string,
	capability string,
	body map[string]any,
	now time.Time,
) (domain.KnowledgeArtifact, error) {
	baseCommit := domain.GenesisCommitID
	if head, err := tx.HeadCommit(operation.MissionRevision); err == nil {
		baseCommit = head.ID
	}
	id, err := e.IDs.NewID("artifact")
	if err != nil {
		return domain.KnowledgeArtifact{}, fmt.Errorf("generate artifact id: %w", err)
	}
	if body == nil {
		body = map[string]any{}
	}
	body["operation_id"] = string(operation.ID)
	body["spec_id"] = string(spec.ID)
	body["lease_ref"] = leaseRef
	body["capability"] = capability
	body["produced_at"] = now.UTC().Format(time.RFC3339Nano)
	encoded, err := json.Marshal(body)
	if err != nil {
		return domain.KnowledgeArtifact{}, err
	}
	kind := "file_discover_report"
	if capability == "file.read" {
		kind = "file_read_report"
	}
	deps := []string{
		"operation:" + string(operation.ID),
		"spec:" + string(spec.ID),
		"lease:" + leaseRef,
	}
	art := domain.KnowledgeArtifact{
		SchemaVersion: domain.SchemaVersionV1,
		ID:            domain.ArtifactID(id),
		Kind:          kind,
		BaseCommitID:  baseCommit,
		Dependencies:  deps,
		ContentRef:    "inline:" + kind,
		Content:       string(encoded),
	}
	if err := art.Validate(); err != nil {
		return domain.KnowledgeArtifact{}, err
	}
	return art, nil
}

func (e FileExecutor) failRunning(ctx context.Context, operation domain.Operation, leaseRef string, cause error) error {
	failErr := e.Store.Update(ctx, func(tx port.Transaction) error {
		op, err := tx.Operation(operation.ID)
		if err != nil {
			return err
		}
		if op.State != domain.StateRunning || op.Reevaluation.Reference != leaseRef {
			return nil
		}
		next, err := domain.Transition(
			domain.OperationalSnapshot{State: op.State, Reevaluation: op.Reevaluation},
			domain.TransitionInput{Event: domain.EventRequestReplan, Reference: leaseRef},
		)
		if err != nil {
			return err
		}
		ready, err := domain.Transition(next, domain.TransitionInput{Event: domain.EventResume})
		if err != nil {
			return err
		}
		op.State = ready.State
		op.Reevaluation = ready.Reevaluation
		if err := tx.SaveOperation(op); err != nil {
			return err
		}
		_, err = tx.AppendEvent(domain.Event{
			SchemaVersion:   domain.SchemaVersionV1,
			ID:              domain.EventID(fmt.Sprintf("%s:file_fail:%d:%d", op.ID, op.Attempt, e.Clock.Now().UnixNano())),
			Kind:            "operation.file_failed",
			OccurredAt:      e.Clock.Now().UTC(),
			MissionRevision: op.MissionRevision,
			InquiryID:       op.InquiryID,
			OperationID:     op.ID,
			PayloadRef:      leaseRef + ";error_class=file_io",
		})
		return err
	})
	if failErr != nil {
		return fmt.Errorf("%v; also failed to replan operation: %w", cause, failErr)
	}
	return cause
}

// Silence unused import if fs is needed for future WalkDir.
var _ = fs.ErrInvalid
