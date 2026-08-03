package views

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestComponentsRender(t *testing.T) {
	ctx := context.Background()

	t.Run("StatCard", func(t *testing.T) {
		var buf bytes.Buffer
		err := StatCard("Total Eventos", "1.234", "últimas 24h", "var(--ok)").Render(ctx, &buf)
		if err != nil {
			t.Fatalf("StatCard.Render falhou: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "Total Eventos") || !strings.Contains(out, "1.234") || !strings.Contains(out, "últimas 24h") {
			t.Errorf("StatCard output inesperado: %s", out)
		}
	})

	t.Run("StatusDot", func(t *testing.T) {
		var buf bytes.Buffer
		err := StatusDot(true, "Inspect API Conectada").Render(ctx, &buf)
		if err != nil {
			t.Fatalf("StatusDot.Render falhou: %v", err)
		}
		if !strings.Contains(buf.String(), "Inspect API Conectada") || !strings.Contains(buf.String(), "bg-[var(--ok)]") {
			t.Errorf("StatusDot (ok=true) output inesperado: %s", buf.String())
		}
	})

	t.Run("EmptyState", func(t *testing.T) {
		var buf bytes.Buffer
		err := EmptyState("Nenhum evento encontrado").Render(ctx, &buf)
		if err != nil {
			t.Fatalf("EmptyState.Render falhou: %v", err)
		}
		if !strings.Contains(buf.String(), "Nenhum evento encontrado") {
			t.Errorf("EmptyState output inesperado: %s", buf.String())
		}
	})

	t.Run("SeverityTone", func(t *testing.T) {
		if tone := SeverityTone("critical"); tone != "var(--err)" {
			t.Errorf("esperava var(--err), obteve %s", tone)
		}
		if tone := SeverityTone("healthy"); tone != "var(--ok)" {
			t.Errorf("esperava var(--ok), obteve %s", tone)
		}
	})
}
