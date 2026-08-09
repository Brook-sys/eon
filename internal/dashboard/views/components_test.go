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

	t.Run("Badge", func(t *testing.T) {
		var buf bytes.Buffer
		err := Badge("Passou", "success").Render(ctx, &buf)
		if err != nil {
			t.Fatalf("Badge.Render falhou: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "Passou") || !strings.Contains(out, "bg-[var(--ok)]/10") {
			t.Errorf("Badge output inesperado: %s", out)
		}
	})

	t.Run("Card", func(t *testing.T) {
		var buf bytes.Buffer
		err := Card("Título do Card").Render(ctx, &buf)
		if err != nil {
			t.Fatalf("Card.Render falhou: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "Título do Card") {
			t.Errorf("Card output inesperado: %s", out)
		}
	})

	t.Run("AlertBanner", func(t *testing.T) {
		var buf bytes.Buffer
		err := AlertBanner("Atenção", "Mensagem de teste de erro", "error").Render(ctx, &buf)
		if err != nil {
			t.Fatalf("AlertBanner.Render falhou: %v", err)
		}
		out := buf.String()
		// Modificado de "bg-[var(--err)]/10" para verificar a classe exata nova "bg-[var(--err)]/5"
		if !strings.Contains(out, "Atenção") || !strings.Contains(out, "Mensagem de teste de erro") || !strings.Contains(out, "bg-[var(--err)]/5") {
			t.Errorf("AlertBanner output inesperado: %s", out)
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

	t.Run("Kbd", func(t *testing.T) {
		var buf bytes.Buffer
		err := Kbd("g").Render(ctx, &buf)
		if err != nil {
			t.Fatalf("Kbd.Render falhou: %v", err)
		}
		if !strings.Contains(buf.String(), "g") || !strings.Contains(buf.String(), "<kbd") {
			t.Errorf("Kbd output inesperado: %s", buf.String())
		}
	})

	t.Run("LoadingSkeleton", func(t *testing.T) {
		var buf bytes.Buffer
		err := LoadingSkeleton(3).Render(ctx, &buf)
		if err != nil {
			t.Fatalf("LoadingSkeleton.Render falhou: %v", err)
		}
		if !strings.Contains(buf.String(), "animate-pulse") {
			t.Errorf("LoadingSkeleton output inesperado: %s", buf.String())
		}
	})

	t.Run("CopyButton", func(t *testing.T) {
		var buf bytes.Buffer
		err := CopyButton("texto_para_copiar").Render(ctx, &buf)
		if err != nil {
			t.Fatalf("CopyButton.Render falhou: %v", err)
		}
		if !strings.Contains(buf.String(), "texto_para_copiar") || !strings.Contains(buf.String(), "Copiar") {
			t.Errorf("CopyButton output inesperado: %s", buf.String())
		}
	})

	t.Run("Pager", func(t *testing.T) {
		var buf bytes.Buffer
		err := Pager("/dash/events", 1, 5, false, true).Render(ctx, &buf)
		if err != nil {
			t.Fatalf("Pager.Render falhou: %v", err)
		}
		out := buf.String()
		// Modificado de "Próxima &rarr;" para apenas verificar o texto base "Próxima" e "Página",
		// pois a nova refatoração usa o ícone SVG ao invés do código HTML de seta.
		if !strings.Contains(out, "Página") || !strings.Contains(out, "Próxima") {
			t.Errorf("Pager output inesperado: %s", out)
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
