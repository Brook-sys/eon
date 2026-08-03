package views

import "fmt"

// SeverityTone retorna o token CSS de cor correspondente à severidade/status.
func SeverityTone(severity string) string {
	switch severity {
	case "critical", "err", "error", "unhealthy", "fail", "FAIL":
		return "var(--err)"
	case "warning", "warn", "degraded", "retry":
		return "var(--warn)"
	case "healthy", "ok", "pass", "PASS":
		return "var(--ok)"
	case "info", "notice":
		return "var(--accent)"
	default:
		return "var(--muted)"
	}
}

// SeverityBadgeClass retorna as classes Tailwind/CSS para a badge com base no tom.
func SeverityBadgeClass(severity string) string {
	tone := SeverityTone(severity)
	return fmt.Sprintf("border text-xs px-2 py-0.5 rounded-full font-mono font-medium inline-flex items-center gap-1.5 style=\"border-color:%s; color:%s; bg-opacity-10\"", tone, tone)
}
