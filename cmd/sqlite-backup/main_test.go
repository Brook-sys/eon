package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"

	"motor-autonomo/internal/storage/sqlite"
)

func TestRunBackupAndVerify(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "runtime.sqlite")
	backupPath := filepath.Join(dir, "backup.sqlite")

	store, err := sqlite.Open(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	// Opening and closing creates a valid empty canonical store, which is enough
	// to exercise the operational command without coupling it to domain fixtures.
	var backupOut bytes.Buffer
	if err := run(context.Background(), []string{
		"-mode=backup", "-source=" + sourcePath, "-destination=" + backupPath,
	}, &backupOut); err != nil {
		t.Fatal(err)
	}
	var report sqlite.BackupReport
	if err := json.Unmarshal(backupOut.Bytes(), &report); err != nil {
		t.Fatalf("decode report: %v\n%s", err, backupOut.String())
	}
	if report.SourcePath != sourcePath || report.DestinationPath != backupPath || report.IntegrityCheck != "ok" || report.SHA256 == "" || report.FileSize <= 0 {
		t.Fatalf("unexpected report: %+v", report)
	}

	var verifyOut bytes.Buffer
	if err := run(context.Background(), []string{
		"-mode=verify", "-path=" + backupPath,
		"-expected-sha256=" + report.SHA256,
		"-expected-schema-version=" + fmt.Sprint(report.SchemaVersion),
		"-expected-schema-objects=1",
		"-expected-schema-sha256=" + report.SchemaSHA256,
		"-expected-checkpoint-sha256=" + report.CheckpointSHA256,
		"-expected-checkpoint-rows=0",
		"-expected-checkpoint-format=0",
	}, &verifyOut); err != nil {
		t.Fatal(err)
	}
	var verification sqlite.BackupVerification
	if err := json.Unmarshal(verifyOut.Bytes(), &verification); err != nil {
		t.Fatalf("decode verification: %v", err)
	}
	if verification.IntegrityCheck != "ok" || verification.SHA256 != report.SHA256 || verification.FileSize != report.FileSize {
		t.Fatalf("unexpected verification: %+v", verification)
	}
	if err := run(context.Background(), []string{"-mode=verify", "-path=" + backupPath, "-expected-sha256=invalid"}, &bytes.Buffer{}); err == nil {
		t.Fatal("invalid expected digest accepted")
	}

	restoredPath := filepath.Join(dir, "restored.sqlite")
	var restoreOut bytes.Buffer
	if err := run(context.Background(), []string{
		"-mode=restore", "-source=" + backupPath, "-destination=" + restoredPath,
		"-expected-sha256=" + report.SHA256,
		"-expected-schema-version=" + fmt.Sprint(report.SchemaVersion),
		"-expected-schema-objects=1",
		"-expected-schema-sha256=" + report.SchemaSHA256,
		"-expected-checkpoint-sha256=" + report.CheckpointSHA256,
		"-expected-checkpoint-rows=0",
		"-expected-checkpoint-format=0",
	}, &restoreOut); err != nil {
		t.Fatal(err)
	}
	var restoreReport sqlite.BackupReport
	if err := json.Unmarshal(restoreOut.Bytes(), &restoreReport); err != nil {
		t.Fatalf("decode restore report: %v\n%s", err, restoreOut.String())
	}
	if restoreReport.SourcePath != backupPath || restoreReport.SourceSHA256 != report.SHA256 || restoreReport.DestinationPath != restoredPath || restoreReport.IntegrityCheck != "ok" {
		t.Fatalf("unexpected restore report: %+v", restoreReport)
	}
	if _, err := sqlite.VerifyBackup(restoredPath); err != nil {
		t.Fatalf("verify restored runtime: %v", err)
	}
	if err := run(context.Background(), []string{
		"-mode=restore", "-source=" + backupPath, "-destination=" + filepath.Join(dir, "wrong-digest.sqlite"),
		"-expected-sha256=" + string(bytes.Repeat([]byte("0"), 64)),
	}, &bytes.Buffer{}); err == nil {
		t.Fatal("restore accepted wrong expected digest")
	}
}

func TestRunRejectsUnsafeOrIncompleteArguments(t *testing.T) {
	for _, args := range [][]string{
		{"-mode=backup"},
		{"-mode=backup", "-source=x"},
		{"-mode=backup", "-source=x", "-destination=y", "-page-steps=-1"},
		{"-mode=backup", "-source=x", "-destination=y", "-page-steps=2147483648"},
		{"-mode=verify"},
		{"-mode=verify", "-path=x", "-expected-checkpoint-rows=not-an-int"},
		{"-mode=verify", "-path=x", "-expected-checkpoint-rows=2"},
		{"-mode=verify", "-path=x", "-expected-checkpoint-format=-1"},
		{"-mode=verify", "-path=x", "-expected-schema-objects=2"},
		{"-mode=restore"},
		{"-mode=restore", "-source=x"},
		{"-mode=restore", "-source=x", "-destination=y", "-page-steps=-1"},
		{"-mode=restore", "-source=x", "-destination=y", "-page-steps=2147483648"},
		{"-mode=unknown"},
	} {
		if err := run(context.Background(), args, &bytes.Buffer{}); err == nil {
			t.Fatalf("args %v unexpectedly succeeded", args)
		}
	}
}
