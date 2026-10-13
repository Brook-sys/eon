// Package statefile escreve e lê arquivos JSON de estado de forma atômica
// (tmp + rename no mesmo diretário) — health.json, boot.json, heartbeat.json.
//
// Lei (ADR-0002): importa só stdlib.
package statefile

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Write serializa v e grava atomicamente em path (tmp + rename).
func Write(path string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("statefile %s: %w", filepath.Base(path), err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Read carrega o JSON de path em v.
func Read(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}
