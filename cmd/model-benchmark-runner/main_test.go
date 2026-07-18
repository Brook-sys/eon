package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
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

func TestCampaignModeRunsMultipleBindingsAndWritesAggregate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		var request struct {
			Model    string `json:"model"`
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		answer := "value=7"
		if strings.Contains(request.Messages[0].Content, "JSON object") {
			answer = `{"value":"7"}`
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"model":%q,"choices":[{"message":{"role":"assistant","content":%q}}],"usage":{"prompt_tokens":10,"completion_tokens":2}}`, request.Model, answer)
	}))
	defer server.Close()

	dir := t.TempDir()
	fixture := filepath.Join(dir, "fixture.json")
	if err := os.WriteFile(fixture, []byte(`{"schema_version":1,"name":"tiny","cases":[{"id":"extract","operation":"EXTRACT","task":"Extract.","required_facts":[{"id":"f","text":"Value is 7.","priority":1}],"optional_facts":[],"constraints":[],"formats":["CHOICE","JSON"],"expected":{"value":"7"}}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(dir, "campaign.json")
	body := fmt.Sprintf(`{"schema_version":1,"name":"multi","fixture_path":%q,"context_tokens":[2048],"max_calls":4,"max_output_tokens":16,"max_total_output_tokens":64,"timeout_seconds":30,"models":[{"provider":"test","binding_id":"a","base_url":%q,"model":"model-a","api_key_env":"TEST_KEY","max_output_field":"max_tokens"},{"provider":"test","binding_id":"b","base_url":%q,"model":"model-b","api_key_env":"TEST_KEY","max_output_field":"max_tokens"}]}`, fixture, server.URL, server.URL)
	if err := os.WriteFile(manifest, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "out")
	if err := runCampaign(manifest, out); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"manifest.json", "campaign.json", "campaign.md", "a/report.json", "b/report.json"} {
		if _, err := os.Stat(filepath.Join(out, path)); err != nil {
			t.Fatalf("missing %s: %v", path, err)
		}
	}
}
