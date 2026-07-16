package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseContexts(t *testing.T) {
	got, err := parseContexts("2048, 4096,8192")
	if err != nil || len(got) != 3 || got[1] != 4096 {
		t.Fatalf("got=%v err=%v", got, err)
	}
	for _, value := range []string{"", "2048,2048", "0", "bad"} {
		if _, err := parseContexts(value); err == nil {
			t.Fatalf("expected %q to fail", value)
		}
	}
}

func TestOfflineOracleMode(t *testing.T) {
	// Integration-ish: build and run the CLI offline path without network.
	dir := t.TempDir()
	bin := filepath.Join(dir, "model-benchmark-runner")
	build := exec.Command("go", "build", "-o", bin, ".")
	build.Dir = "."
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	outDir := filepath.Join(dir, "artifacts")
	cmd := exec.Command(bin,
		"-mode", "offline-oracle",
		"-fixtures", "../../internal/evaluation/testdata/cognitive-v1.json",
		"-contexts", "2048",
		"-out", outDir,
	)
	cmd.Dir = "."
	body, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run: %v\n%s", err, body)
	}
	text := string(body)
	if !strings.Contains(text, "mode=offline-oracle") || !strings.Contains(text, "verdict=PASS") {
		t.Fatalf("unexpected stdout: %s", text)
	}
	md, err := os.ReadFile(filepath.Join(outDir, "report.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(md), "## Interpretation") {
		t.Fatalf("report.md missing interpretation")
	}
}
