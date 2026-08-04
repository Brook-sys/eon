package views

// StandardNav returns the complete navigation menu items for v2 layout.
func StandardNav(activePath string) []NavItem {
	return []NavItem{
		{Href: "/dash", Label: "Visão geral", Active: activePath == "/dash"},
		{Href: "/dash/events", Label: "Eventos", Active: activePath == "/dash/events"},
		{Href: "/dash/models", Label: "Modelos & LLMs", Active: activePath == "/dash/models"},
		{Href: "/dash/resources", Label: "Recursos & Gates", Active: activePath == "/dash/resources"},
		{Href: "/dash/frontier", Label: "Fronteira & Ações", Active: activePath == "/dash/frontier"},
		{Href: "/dash/alerts", Label: "Alertas & Telemetria", Active: activePath == "/dash/alerts"},
		{Href: "/dash/knowledge", Label: "Conhecimento", Active: activePath == "/dash/knowledge"},
		{Href: "/dashboard", Label: "Dashboard legado ↗", External: true},
	}
}
