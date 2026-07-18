package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"motor-autonomo/internal/storage/sqlite"
)

func TestRunBackupAndVerify(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "runtime.sqlite")
	backupPath := filepath.Join(dir, "backup.sqlite")
	reportPath := filepath.Join(dir, "inventory", "backup.json")

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
		"-report-path=" + reportPath,
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
	if report.ReportSchema != "motor-autonomo.sqlite-backup-report.v1" || report.Operation != "backup" {
		t.Fatalf("unexpected report framing: %+v", report)
	}
	reportBytes, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(reportBytes, backupOut.Bytes()) {
		t.Fatalf("durable report differs from stdout\nfile: %s\nout: %s", reportBytes, backupOut.Bytes())
	}
	if info, err := os.Stat(reportPath); err != nil {
		t.Fatal(err)
	} else if info.Mode().Perm() != 0o600 {
		t.Fatalf("report mode = %o, want 600", info.Mode().Perm())
	}
	if err := run(context.Background(), []string{
		"-mode=verify", "-path=" + backupPath, "-report-path=" + reportPath,
	}, &bytes.Buffer{}); err == nil {
		t.Fatal("existing report path was overwritten")
	}
	if after, err := os.ReadFile(reportPath); err != nil {
		t.Fatal(err)
	} else if !bytes.Equal(after, reportBytes) {
		t.Fatal("existing report changed after rejected overwrite")
	}

	var verifyOut bytes.Buffer
	if err := run(context.Background(), []string{
		"-mode=verify", "-path=" + backupPath,
		"-expected-sha256=" + report.SHA256,
		"-expected-page-size=" + fmt.Sprint(report.PageSize),
		"-expected-page-count=" + fmt.Sprint(report.PageCount),
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
	if verification.ReportSchema != report.ReportSchema || verification.Operation != "verify" {
		t.Fatalf("unexpected verification framing: %+v", verification)
	}
	var inventoryVerifyOut bytes.Buffer
	if err := run(context.Background(), []string{
		"-mode=verify", "-path=" + backupPath, "-inventory=" + reportPath,
	}, &inventoryVerifyOut); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(inventoryVerifyOut.Bytes(), verifyOut.Bytes()) {
		t.Fatalf("inventory verification differs from explicit verification\ninventory: %s\nexplicit: %s", inventoryVerifyOut.Bytes(), verifyOut.Bytes())
	}
	if err := run(context.Background(), []string{
		"-mode=verify", "-path=" + backupPath, "-inventory=" + reportPath,
		"-expected-sha256=" + report.SHA256,
	}, &bytes.Buffer{}); err == nil {
		t.Fatal("inventory was combined with explicit expectations")
	}
	if err := run(context.Background(), []string{"-mode=verify", "-path=" + backupPath, "-expected-sha256=invalid"}, &bytes.Buffer{}); err == nil {
		t.Fatal("invalid expected digest accepted")
	}

	restoredPath := filepath.Join(dir, "restored.sqlite")
	var restoreOut bytes.Buffer
	if err := run(context.Background(), []string{
		"-mode=restore", "-source=" + backupPath, "-destination=" + restoredPath,
		"-expected-sha256=" + report.SHA256,
		"-expected-page-size=" + fmt.Sprint(report.PageSize),
		"-expected-page-count=" + fmt.Sprint(report.PageCount),
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
	if restoreReport.ReportSchema != report.ReportSchema || restoreReport.Operation != "restore" {
		t.Fatalf("unexpected restore framing: %+v", restoreReport)
	}
	if _, err := sqlite.VerifyBackup(restoredPath); err != nil {
		t.Fatalf("verify restored runtime: %v", err)
	}
	inventoryRestoredPath := filepath.Join(dir, "restored-from-inventory.sqlite")
	if err := run(context.Background(), []string{
		"-mode=restore", "-source=" + backupPath, "-destination=" + inventoryRestoredPath,
		"-inventory=" + reportPath,
	}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if _, err := sqlite.VerifyBackup(inventoryRestoredPath); err != nil {
		t.Fatalf("verify inventory-restored runtime: %v", err)
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
		{"-mode=backup", "-source=x", "-destination=y", "-inventory=z"},
		{"-mode=verify"},
		{"-mode=verify", "-path=x", "-expected-checkpoint-rows=not-an-int"},
		{"-mode=verify", "-path=x", "-expected-checkpoint-rows=2"},
		{"-mode=verify", "-path=x", "-expected-checkpoint-format=-1"},
		{"-mode=verify", "-path=x", "-expected-schema-objects=2"},
		{"-mode=verify", "-path=x", "-expected-page-size=0"},
		{"-mode=verify", "-path=x", "-expected-page-count=-1"},
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

func TestLoadInventoryRejectsUntrustedJSON(t *testing.T) {
	dir := t.TempDir()
	for name, payload := range map[string]string{
		"unknown.json":   `{"unknown":true}`,
		"trailing.json":  `{} {}`,
		"invalid.json":   `{"file_size":1,"page_size":4096,"page_count":1,"application_id":0,"user_version":1,"schema_objects":1,"integrity_check":"ok","foreign_key_check":"ok"}`,
		"digest.json":    `{"file_size":4096,"page_size":4096,"page_count":1,"application_id":1296127316,"user_version":1,"schema_objects":1,"integrity_check":"ok","foreign_key_check":"ok"}`,
		"future.json":    `{"report_schema":"motor-autonomo.sqlite-backup-report.v2","operation":"backup","file_size":4096,"page_size":4096,"page_count":1,"application_id":1296127316,"user_version":1,"schema_objects":1,"integrity_check":"ok","foreign_key_check":"ok"}`,
		"operation.json": `{"report_schema":"motor-autonomo.sqlite-backup-report.v1","operation":"delete","file_size":4096,"page_size":4096,"page_count":1,"application_id":1296127316,"user_version":1,"schema_objects":1,"integrity_check":"ok","foreign_key_check":"ok"}`,
	} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(payload), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := loadInventory(path); err == nil {
			t.Fatalf("inventory %s unexpectedly accepted", name)
		}
	}
}
