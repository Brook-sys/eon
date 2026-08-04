package views

// StandardNav returns the complete navigation menu items for v2 layout.
func StandardNav(activePath string) []NavItem {
	return []NavItem{
		{Href: "/dash", Label: "Visão geral", Active: activePath == "/dash", KeyHint: "o"},
		{Href: "/dash/events", Label: "Eventos", Active: activePath == "/dash/events", KeyHint: "e"},
		{Href: "/dash/models", Label: "Modelos & LLMs", Active: activePath == "/dash/models", KeyHint: "m"},
		{Href: "/dash/resources", Label: "Recursos & Gates", Active: activePath == "/dash/resources", KeyHint: "r"},
		{Href: "/dash/frontier", Label: "Fronteira & Ações", Active: activePath == "/dash/frontier", KeyHint: "f"},
		{Href: "/dash/alerts", Label: "Alertas & Telemetria", Active: activePath == "/dash/alerts", KeyHint: "a"},
		{Href: "/dash/knowledge", Label: "Conhecimento", Active: activePath == "/dash/knowledge", KeyHint: "k"},
		{Href: "/dashboard", Label: "Dashboard legado ↗", External: true},
	}
}
