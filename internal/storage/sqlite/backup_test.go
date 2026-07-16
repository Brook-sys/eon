package sqlite_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	storage "motor-autonomo/internal/storage/sqlite"
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
