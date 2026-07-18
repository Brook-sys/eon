package sqlite_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/gob"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/storage/memory"
	storage "motor-autonomo/internal/storage/sqlite"

	_ "modernc.org/sqlite"
)

func TestOnlineBackupPreservesCheckpointAndReopens(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "runtime.sqlite")
	source, err := storage.Open(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()

	now := time.Date(2026, 7, 17, 15, 0, 0, 0, time.UTC)
	mission := domain.MissionRevision{
		SchemaVersion: 1, ID: "mission_revision_1", MissionID: "mission_1", Revision: 1,
		OriginalText: "backup test", Purpose: "durability", Domains: []string{"storage"},
		Policies: []string{"wal"}, Budget: domain.Budget{ModelCalls: 1, Tokens: 100, Attempts: 1},
		Status: domain.MissionActive, Provenance: "test", AcceptedAt: now,
	}
	if err := source.Update(context.Background(), func(tx port.Transaction) error {
		if err := tx.AppendMissionRevision(mission); err != nil {
			return err
		}
		return tx.ActivateMissionRevision(mission.MissionID, mission.ID)
	}); err != nil {
		t.Fatal(err)
	}

	destPath := filepath.Join(dir, "backup", "runtime-backup.sqlite")
	report, err := source.BackupTo(context.Background(), destPath, storage.BackupOptions{})
	if err != nil {
		t.Fatalf("backup: %v", err)
	}
	if report.DestinationPath != destPath || report.SQLiteVersion == "" {
		t.Fatalf("report = %#v", report)
	}
	if report.CheckpointRows != 1 {
		t.Fatalf("checkpoint rows = %d", report.CheckpointRows)
	}
	if report.CheckpointFormat != memory.CheckpointFormatVersion || report.IntegrityCheck != "ok" {
		t.Fatalf("backup verification report = %#v", report)
	}
	if report.FileSize <= 0 || len(report.SHA256) != sha256.Size*2 {
		t.Fatalf("backup identity = %#v", report)
	}
	if info, err := os.Stat(destPath); err != nil {
		t.Fatal(err)
	} else if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("backup mode = %04o, want 0600", got)
	}
	verified, err := storage.VerifyBackupWithOptions(destPath, storage.VerificationOptions{ExpectedSHA256: report.SHA256})
	if err != nil {
		t.Fatalf("verify pinned digest: %v", err)
	}
	if verified.FileSize != report.FileSize || verified.SHA256 != report.SHA256 {
		t.Fatalf("verification identity = %#v, report = %#v", verified, report)
	}

	// Source must remain usable after backup.
	if err := source.View(context.Background(), func(r port.Reader) error {
		active, err := r.ActiveMissionRevision("mission_1")
		if err != nil {
			return err
		}
		if active.ID != mission.ID {
			t.Fatalf("source active = %#v", active)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	// Destination must reopen with the same logical state.
	restored, err := storage.Open(destPath)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	if err := restored.View(context.Background(), func(r port.Reader) error {
		active, err := r.ActiveMissionRevision("mission_1")
		if err != nil {
			return err
		}
		if active.OriginalText != mission.OriginalText || active.ID != mission.ID {
			t.Fatalf("restored active = %#v", active)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestRestoreToVerifiesAndReopensCheckpoint(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "runtime.sqlite")
	backupPath := filepath.Join(dir, "backup.sqlite")
	restoredPath := filepath.Join(dir, "restored.sqlite")

	source, err := storage.Open(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 18, 10, 0, 0, 0, time.UTC)
	mission := domain.MissionRevision{
		SchemaVersion: 1, ID: "mission_revision_restore", MissionID: "mission_restore", Revision: 1,
		OriginalText: "restore test", Purpose: "durability", Domains: []string{"storage"},
		Policies: []string{"verified restore"}, Budget: domain.Budget{ModelCalls: 1, Tokens: 100, Attempts: 1},
		Status: domain.MissionActive, Provenance: "test", AcceptedAt: now,
	}
	if err := source.Update(context.Background(), func(tx port.Transaction) error {
		if err := tx.AppendMissionRevision(mission); err != nil {
			return err
		}
		return tx.ActivateMissionRevision(mission.MissionID, mission.ID)
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := source.BackupTo(context.Background(), backupPath, storage.BackupOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := source.Close(); err != nil {
		t.Fatal(err)
	}

	report, err := storage.RestoreTo(context.Background(), backupPath, restoredPath, storage.BackupOptions{PageSteps: 1})
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if report.SourcePath != backupPath || report.DestinationPath != restoredPath || report.IntegrityCheck != "ok" {
		t.Fatalf("restore report = %#v", report)
	}
	if report.SourceSHA256 == "" {
		t.Fatalf("restore did not record verified source identity: %#v", report)
	}

	restored, err := storage.Open(restoredPath)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	if err := restored.View(context.Background(), func(r port.Reader) error {
		active, err := r.ActiveMissionRevision(mission.MissionID)
		if err != nil {
			return err
		}
		if active.ID != mission.ID || active.OriginalText != mission.OriginalText {
			t.Fatalf("restored active = %#v", active)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestRestoreToRejectsExistingDestinationAndInvalidSource(t *testing.T) {
	dir := t.TempDir()
	backupPath := filepath.Join(dir, "backup.sqlite")
	store, err := storage.Open(backupPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	existing := filepath.Join(dir, "existing.sqlite")
	if err := os.WriteFile(existing, []byte("preserve me"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := storage.RestoreTo(context.Background(), backupPath, existing, storage.BackupOptions{}); err == nil {
		t.Fatal("restore overwrote existing destination")
	}
	contents, err := os.ReadFile(existing)
	if err != nil || string(contents) != "preserve me" {
		t.Fatalf("existing destination changed: %q, %v", contents, err)
	}

	invalid := filepath.Join(dir, "invalid.sqlite")
	if err := os.WriteFile(invalid, []byte("not sqlite"), 0o600); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(dir, "must-not-exist.sqlite")
	if _, err := storage.RestoreTo(context.Background(), invalid, destination, storage.BackupOptions{}); err == nil {
		t.Fatal("invalid restore source accepted")
	}
	if _, err := os.Stat(destination); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("invalid restore left destination: %v", err)
	}
}

func TestRestoreToPinsExpectedSourceDigest(t *testing.T) {
	dir := t.TempDir()
	backupPath := filepath.Join(dir, "backup.sqlite")
	store, err := storage.Open(backupPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	verification, err := storage.VerifyBackup(backupPath)
	if err != nil {
		t.Fatal(err)
	}

	destination := filepath.Join(dir, "restored.sqlite")
	wrongDigest := strings.Repeat("0", sha256.Size*2)
	if wrongDigest == verification.SHA256 {
		wrongDigest = strings.Repeat("1", sha256.Size*2)
	}
	if _, err := storage.RestoreToWithOptions(context.Background(), backupPath, destination, storage.RestoreOptions{
		ExpectedSHA256: wrongDigest,
	}); err == nil {
		t.Fatal("restore accepted a source with the wrong expected digest")
	}
	if _, err := os.Stat(destination); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("digest mismatch left destination: %v", err)
	}

	report, err := storage.RestoreToWithOptions(context.Background(), backupPath, destination, storage.RestoreOptions{
		ExpectedSHA256: verification.SHA256,
	})
	if err != nil {
		t.Fatalf("restore with pinned digest: %v", err)
	}
	if report.SourceSHA256 != verification.SHA256 {
		t.Fatalf("source digest = %q, want %q", report.SourceSHA256, verification.SHA256)
	}
}

func TestOnlineBackupRejectsExistingDestination(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "runtime.sqlite")
	source, err := storage.Open(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	dest := filepath.Join(dir, "already.sqlite")
	if _, err := source.BackupTo(context.Background(), dest, storage.BackupOptions{}); err != nil {
		t.Fatalf("first backup: %v", err)
	}
	if _, err := source.BackupTo(context.Background(), dest, storage.BackupOptions{}); err == nil {
		t.Fatal("overwrite accepted")
	}
}

func TestOnlineBackupEmptyStore(t *testing.T) {
	dir := t.TempDir()
	source, err := storage.Open(filepath.Join(dir, "empty.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	dest := filepath.Join(dir, "empty-backup.sqlite")
	report, err := source.BackupTo(context.Background(), dest, storage.BackupOptions{PageSteps: 1})
	if err != nil {
		t.Fatalf("backup empty: %v", err)
	}
	if report.CheckpointRows != 0 {
		t.Fatalf("expected 0 checkpoint rows on empty store, got %d", report.CheckpointRows)
	}
	restored, err := storage.Open(dest)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
}

func TestVerifyBackupRejectsDigestMismatchAndInvalidExpectation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "runtime.sqlite")
	store, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Update(context.Background(), func(port.Transaction) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	verification, err := storage.VerifyBackup(path)
	if err != nil {
		t.Fatal(err)
	}
	wrong := strings.Repeat("0", sha256.Size*2)
	if wrong == verification.SHA256 {
		wrong = strings.Repeat("f", sha256.Size*2)
	}
	if _, err := storage.VerifyBackupWithOptions(path, storage.VerificationOptions{ExpectedSHA256: wrong}); err == nil {
		t.Fatal("digest mismatch accepted")
	}
	if _, err := storage.VerifyBackupWithOptions(path, storage.VerificationOptions{ExpectedSHA256: "not-a-digest"}); err == nil {
		t.Fatal("invalid expected digest accepted")
	}
}

func TestVerifyBackupRejectsCheckpointVersionMismatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "runtime.sqlite")
	store, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Update(context.Background(), func(port.Transaction) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE runtime_checkpoint SET format_version = 1 WHERE id = 1`); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	_, err = storage.VerifyBackup(path)
	if !errors.Is(err, memory.ErrCheckpointFormatMismatch) {
		t.Fatalf("verification error = %v, want format mismatch", err)
	}
}

func TestVerifyBackupRejectsTamperedCheckpointPayload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "runtime.sqlite")
	store, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Update(context.Background(), func(port.Transaction) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	var payload []byte
	if err := db.QueryRow(`SELECT payload FROM runtime_checkpoint WHERE id = 1`).Scan(&payload); err != nil {
		db.Close()
		t.Fatal(err)
	}
	var envelope struct {
		FormatVersion int
		PayloadDigest [sha256.Size]byte
		Payload       []byte
	}
	if err := gob.NewDecoder(bytes.NewReader(payload)).Decode(&envelope); err != nil {
		db.Close()
		t.Fatal(err)
	}
	envelope.Payload[len(envelope.Payload)-1] ^= 0xff
	var tampered bytes.Buffer
	if err := gob.NewEncoder(&tampered).Encode(envelope); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE runtime_checkpoint SET payload = ? WHERE id = 1`, tampered.Bytes()); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	_, err = storage.VerifyBackup(path)
	if !errors.Is(err, memory.ErrCheckpointIntegrity) {
		t.Fatalf("verification error = %v, want checkpoint integrity failure", err)
	}
}

func TestClosedCopyToRejectsMissingAndSymlinkSources(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "missing.sqlite")
	destination := filepath.Join(dir, "backup.sqlite")
	if _, err := storage.ClosedCopyTo(context.Background(), missing, destination, storage.BackupOptions{}); err == nil {
		t.Fatal("ClosedCopyTo accepted missing source")
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Fatalf("missing source was created: %v", err)
	}
	if _, err := os.Stat(destination); !os.IsNotExist(err) {
		t.Fatalf("destination exists after missing source: %v", err)
	}

	realPath := filepath.Join(dir, "real.sqlite")
	store, err := storage.Open(realPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	symlinkPath := filepath.Join(dir, "source-link.sqlite")
	if err := os.Symlink(realPath, symlinkPath); err != nil {
		t.Fatal(err)
	}
	if _, err := storage.ClosedCopyTo(context.Background(), symlinkPath, destination, storage.BackupOptions{}); err == nil {
		t.Fatal("ClosedCopyTo followed source symlink")
	}
	if _, err := os.Stat(destination); !os.IsNotExist(err) {
		t.Fatalf("destination exists after symlink source: %v", err)
	}
}
