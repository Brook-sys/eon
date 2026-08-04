# Design System — `motor-autonomo` Dashboard v2

**Status:** Ativo / Produção
**Versão:** 2.2.0 (Polidez de Componentes, Espaçamento de Bordas e Usabilidade)
**Escopo:** Todas as superfícies operacionais sob `/dash/*` (Go Templ + Alpine.js + HTMX + Tailwind CSS v4)

---

## 1. Princípios de Design & Arquitetura de Sistema

O Design System do **`motor-autonomo`** é construído sob os princípios de **Observabilidade de Alta Fidelidade**, **Resiliência Server-First**, e **Interface Dark-First de Precisão Operacional** (inspirado na estética e UX do Vercel, Linear e Supabase).

### 1.1 Princípios Fundamentais

1. **Clareza Informacional Sem Poluição Visual:**
   - Evita cards aninhados pesados, bordas duplicadas ou sombras chamativas.
   - Utiliza contraste sutil de fundo (`--bg` vs `--panel` vs `--panel-sub`), tipografia precisa e indicadores de estado coloridos (`--ok`, `--warn`, `--err`, `--accent`).
   - Espaçamento generoso de bordas (`p-5` a `p-6` em cards, `px-3.5 py-2` em inputs) para evitar colisão visual de textos e elementos.
2. **Resiliência e Recuperação Transparente:**
   - A interface monitora proativamente a conectividade com as APIs do runtime (`/dash/api/control/` e `/dash/api/inspect/`).
   - Notifica o operador instantaneamente via Toast ou Banners sobre desconexões ou retries sem travar a navegação.
3. **Desempenho Server-Side Type-Safe com Templ:**
   - Todo o HTML é compilado em Go via Templ, garantindo tempo de resposta sub-milissegundo para a primeira pintura (SSR).
   - Interações dinâmicas são orquestradas por Alpine.js (~15 KB) e HTMX (~14 KB) sem peso de frameworks SPA.
4. **Navegação Limpa e Intuitiva:**
   - Barra lateral organizada com rótulos claros e destaques visuais para a rota ativa.
   - Sem atalhos invasivos de teclado de tecla única, garantindo digitação fluida em inputs sem disparo acidental de navegação.
5. **Acessibilidade e Usabilidade em Telas de Operação:**
   - Suporte completo a navegação por teclado (`Tab`, `:focus-visible` com anel de foco `--accent`).
   - Rótulos ARIA semânticos (`role="navigation"`, `role="main"`, `aria-label`).
   - Cópia em 1-clique de payloads JSON, IDs de missão e comandos com feedback tátil.

---

## 2. Tokens de Design (Single Source of Truth)

Os tokens do Design System são definidos como **CSS Custom Properties** globais em `layout.templ` e consumidos diretamente por classes Tailwind CSS e componentes Templ.

### 2.1 Cores e Modos (Dark Theme Operacional)

```css
:root {
    /* Superfícies & Fundos */
    --bg: #090d16;             /* Fundo da página principal */
    --panel: #111726;          /* Superfície de cards e painéis */
    --panel-sub: #172033;      /* Sub-painéis, caixas de código e inputs */
    --panel-hover: #1e293b;    /* Hover de elementos clicáveis */

    /* Bordas e Divisores */
    --border: rgba(255, 255, 255, 0.08);       /* Bordas padrão */
    --border-subtle: rgba(255, 255, 255, 0.04);/* Divisores internos leves */
    --border-focus: rgba(56, 189, 248, 0.5);   /* Anel de foco ativo */

    /* Tipografia e Textos */
    --text: #f1f5f9;           /* Texto primário de alto contraste */
    --muted: #94a3b8;          /* Texto secundário e labels */
    --subtle: #64748b;         /* Texto terciário e timestamps antigos */

    /* Accent & Severidades Semânticas */
    --accent: #38bdf8;         /* Ações primárias, links, info, seleção */
    --accent-subtle: rgba(56, 189, 248, 0.15);
    --ok: #34d399;             /* Sucesso, saudável, PASS, ativo */
    --warn: #fbbf24;           /* Alertas, degradação, retries */
    --err: #f87171;            /* Erros críticos, falhas, bloqueios */

    /* Tipografia Stack */
    --sans: system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
    --mono: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
}
```

### 2.2 Mapeamento de Severidades Semânticas

| Severidade / Status | Token CSS | Amostra Hex | Uso Recomendado |
|---|---|---|---|
| `healthy` / `ok` / `pass` / `success` | `--ok` | `#34d399` | Runtime operacional, teste OK, missão ativa |
| `warning` / `warn` / `degraded` / `retry` | `--warn` | `#fbbf24` | Pressão de contexto alta, alerta amarelo |
| `critical` / `err` / `unhealthy` / `fail` | `--err` | `#f87171` | Falha de API, cofre bloqueado, erro de runtime |
| `info` / `notice` | `--accent` | `#38bdf8` | Informação neutra, estado ativo de nav |
| `neutral` | `--muted` | `#94a3b8` | Metadados genéricos, inativo |

---

## 3. Inventário de Componentes Reutilizáveis (Templ)

Todos os componentes vivem em `internal/dashboard/views/components.templ`:

1. `@StatCard(label, value, sub, tone string)` — Card de métrica KPI com espaçamento interno generoso.
2. `@StatusDot(ok bool, label string)` — Indicador viva de pulso para status de conexões.
3. `@Badge(label, kind string)` — Pílula semântica padronizada com bordas suaves.
4. `@Card(title string)` — Conteiner principal de seção com padding de 1.5rem (`p-6`).
5. `@AlertBanner(title, msg, kind string)` — Banner destacado para alertas e mensagens críticas.
6. `@EmptyState(msg string)` — Caixa de estado vazio padronizada para tabelas e listas.
7. `@Kbd(key string)` — Renderizador visual de teclas/instruções em linha.
8. `@LoadingSkeleton(rows int)` — Indicador de carregamento com efeito de brilho (*shimmer*).
9. `@CopyButton(textToCopy string)` — Botão compacto de cópia para clipboard com toast.
10. `@Pager(prefix, current, total, hasPrev, hasNext)` — Controles de paginação com espaçamento refinado.
