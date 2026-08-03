# Design System — `motor-autonomo` Dashboard (v2)

**Status:** proposto
**Data:** 2026-08-03
**Escopo:** todas as páginas `/dash/*` (v2 Templ). O dashboard legado (`/dashboard`, SPA inline) fica congelado; não recebe melhorias visuais.

---

## 1. Stack consolidada

| Camada | Tecnologia | Papel |
|---|---|---|
| Templates | **Templ** (Go) | Renderização server-side, type-safe, compilada |
| Estilo | **Tailwind CSS v4** (CLI, build local) | Utility-first sobre tokens CSS |
| Reatividade mínima | **Alpine.js** (vendor, ~15 KB) | Estado local de componente (filtros, tabs, toggles) |
| Interação servidor | **HTMX** (vendor, ~14 KB) | Troca de fragmentos HTML sem SPA |
| Tempo real | **SSE** (`/events/stream` → futuro proxy `/dash/api/events/stream`) | Live tail, contadores ao vivo |
| Fontes | system font stack + JetBrains Mono (vendor woff2) para dados técnicos |

> **Regra de ouro:** nenhum runtime JS além de Alpine+HTMX. Nenhum framework SPA. Nenhum CSS escrito à mão fora dos tokens do §2.

Estado atual: `layout.templ` já carrega `htmx.min.js` e `alpine.min.js` de `/dash/assets/` (vendor local, zero CDN). O Tailwind hoje é um **build pré-gerado** (`internal/dashboard/assets/app.css`, Tailwind v4.3.3) com classes arbitrárias `[var(--token)]`. Faltam: (a) config declarada no repo, (b) tokens nomeados no Tailwind, (c) componentes reutilizáveis além de `badge`/`card`.

---

## 2. Tokens de design (single source of truth)

Os tokens são **CSS custom properties** definidos uma única vez em `layout.templ`. Toda a UI consome `var(--token)` via classes Tailwind arbitrárias ou, após a Etapa 2 do roadmap, via classes semânticas (`bg-panel`, `text-muted`).

### 2.1 Cor (dark theme único, sem light mode por ora)

| Token | Valor | Uso |
|---|---|---|
| `--bg` | `#0f1419` | fundo da página |
| `--panel` | `#1a2332` | cards, sidebar, superfícies elevadas |
| `--panel-2` | `#243247` | hover, superfícies aninhadas (hoje hardcoded no sidebar) |
| `--border` | `#2d3a4d` | bordas, divisores |
| `--text` | `#e7ecf3` | texto primário |
| `--muted` | `#8b9bb4` | texto secundário, labels, metadados |
| `--accent` | `#5b9fd4` | ações primárias, links, estado ativo, info |
| `--ok` | `#3d9a6a` | sucesso, healthy, PASS |
| `--warn` | `#c9a227` | aviso, degraded, retry |
| `--err` | `#c45c5c` | erro, critical, FAIL, stale |

**Semântica obrigatória:** severity `critical`→`--err`, `warning`→`--warn`, `info`→`--accent`; status `healthy/ok/pass`→`--ok`, `degraded`→`--warn`, `unhealthy/fail`→`--err`. Nenhuma cor fora desta tabela (exceção: `--panel-2`, já em uso).

### 2.2 Tipografia

| Token | Valor | Uso |
|---|---|---|
| `--sans` | system stack | texto geral |
| `--mono` | `ui-monospace, SFMono, Menlo, Consolas` | IDs, hashes, timestamps, JSON, locators |
| escala | `text-xs` (12px) meta/uppercase; `text-sm` (14px) corpo; `text-xl` (20px) título de página; `text-2xl` (24px) número KPI | — |

Regras: 14px é o corpo padrão; números de KPI em `text-2xl font-bold`; labels de card em `text-xs uppercase tracking-widest text-[var(--muted)]` (já padronizado no componente `card`).

### 2.3 Espaçamento e forma

- Grid de 4px (spacing Tailwind padrão). Densidades: `p-3` compacto (tabelas), `p-4` padrão (cards), `p-6` relaxado (página).
- Raios: `rounded-md` (6px) controles; `rounded-lg` (8px) cards; `rounded-full` apenas badges/pills.
- Sombras: nenhuma no dark theme (bordas fazem a hierarquia).
- Larguras: sidebar fixa `w-52`; conteúdo `max-w-6xl`; tabelas sempre em `.overflow-x-auto`.

### 2.4 Elevação e foco

- Hierarquia por cor de fundo: `bg` < `panel` < `panel-2` (hover/ativo).
- Foco de teclado: `outline` 1px `--accent` (adicionar na Etapa 2; hoje inexistente).
- Transições: `transition-colors` 150ms em hover de links/botões. Nada mais animado além disso e do spinner de loading.

---

## 3. Componentes (biblioteca Templ)

Estado atual: apenas `badge(label, kind)` e `card(title)`, ambos em `layout.templ`. Roadmap de componentes (cada um entra com teste de render e uso em ≥1 página):

| # | Componente | Assinatura | Motivação |
|---|---|---|---|
| C1 | `card` (existente) | `card(title)` | ok, manter |
| C2 | `badge` (existente) | `badge(label, kind)` | adicionar kind `info` já existe; documentar mapeamento severity→kind |
| C3 | `statCard` | `statCard(label, value, sub, tone)` | KPIs hoje duplicados 4× por página (overview, knowledge) |
| C4 | `dataTable` | cabeçalho + slots de linha + empty state | eliminar `<table>` repetido 7×; empty state padronizado |
| C5 | `pager` | `pager(prefix)` | bloco Anterior/Próxima + "x–y de z" idêntico em 4 páginas |
| C6 | `filterBar` | slots de filtro + botão buscar + contador | barra idêntica em frontier/knowledge/events |
| C7 | `tabBar` | `tabBar(tabs)` | hoje só em knowledge; será reusada em detalhes |
| C8 | `statusDot` | `statusDot(ok, labels)` | ponto verde/vermelho "Inspect API acessível" repetido em toda página |
| C9 | `emptyState` | `emptyState(msg)` | mensagem + ação opcional |
| C10 | `spinner` / `skeleton` | HTMX `hx-indicator` + placeholder shimmer | feedback de carregamento uniforme |
| C11 | `toast` | região fixa + `showToast(msg, kind)` | feedback de ações do operador (Etapa 5, Control API) |
| C12 | `kbdHint` | `kbd("g")` `kbd("k")` | atalhos de teclado (Etapa 6) |

Componentes server-side puros (C1–C9) são funções Templ; os que têm estado (C10–C12) trazem um micro-script Alpine inline documentado.

---

## 4. Padrões de UX (obrigatórios em toda página)

1. **Estado de conexão sempre visível** no topo (status dot + "atualizado há Xs" + botão atualizar).
2. **Números reconhecíveis:** `1.234.567` (locale pt-BR), timestamps em `dd/mm hh:mm:ss`, durações humanizadas (`1,2s`, `350ms`, `2min atrás`).
3. **IDs técnicos truncados** a 8 chars com `title` tooltip full + fonte mono.
4. **Toda tabela tem:** cabeçalho `uppercase text-xs`, linhas `divide-y`, empty state explícito, paginação quando aplicável.
5. **Links internos navegam a entidade:** locator→fonte, run_id/event_id→detalhe, commit→commits. (Hoje nenhuma página tem drill-down; é a principal lacuna de UX.)
6. **Feedback de carregamento:** HTMX `hx-indicator` com spinner; nunca trocar conteúdo sem indicador.
7. **Polling unificado:** 5s para KPIs/listas; SSE para eventos ao vivo quando a Etapa 4 entregar o proxy. Nenhuma página inventa intervalo próprio.
8. **Erros da Inspect API** aparecem como banner no topo (não só o dot vermelho) com a mensagem retornada.
9. **Ações destrutivas** (Etapa 5) exigem confirmação (modal Alpine ou `confirm()` estilizado) e terminam em toast.
10. **Responsividade mínima:** sidebar colapsa para ícones/topbar abaixo de 768px; grids de KPI vão de 4→2 colunas (`md:`). Dashboard é desktop-first, mas não pode quebrar em tablet.

---

## 5. Arquitetura de interação

```
Browser ──GET /dash/<página>──▶ V2Server ──Templ──▶ HTML completo (SSR)
   │                                                    │ Alpine: estado local (filtros, tabs)
   ├──HTMX hx-get /dash/api/... ──proxy──▶ Inspect API ─┘ hx-swap em fragmentos
   └──SSE /dash/api/events/stream ────────▶ (Etapa 4) contadores e live tail
```

- **Navegação entre páginas:** link `<a>` clássico com SSR completo (sem `hx-boost` inicialmente; avaliar boost na Etapa 6 se houver ganho perceptível).
- **Filtros e paginação:** Alpine atualiza estado + dispara `htmx.trigger` no container da tabela, que faz `hx-get` com querystring. A tabela retorna é **fragmento Templ** renderizado por novos handlers `/dash/partials/<recurso>` (Etapa 3) em vez de JSON+template no cliente.
- **Mutações (Etapa 5):** HTMX `hx-post` → `/dash/control/...` → proxy para Control API → resposta é fragmento com toast + `HX-Trigger` para recarregar a lista afetada.

> Decisão-chave: o proxy atual `/dash/api/*` devolve **JSON** (ótimo para os testes e reuso). A Etapa 3 adiciona `/dash/partials/*` que devolve **HTML** para o HTMX. Os dois coexistem: JSON para dados brutos/testes, HTML para interação.

---

## 6. Acessibilidade (floor mínimo)

- `lang="pt-BR"` (já existe).
- Todo controle com `<label>` associado (hoje parcial: labels envolvem inputs, ok, mas faltam `for`/`id` em alguns).
- Navegação por teclado: ordem de tab natural; foco visível (§2.4); atalhos documentados (Etapa 6).
- Contraste: todos os pares texto/fundo da tabela §2.1 ≥ 4.5:1 (verificar `--muted` sobre `--panel`: #8b9bb4/#1a2332 ≈ 7:1, ok).
- Tabelas com `<th scope>`, badges com texto (nunca cor pura como único sinal).
- `aria-live="polite"` na região de toast e no contador de eventos SSE.

---

## 7. Anti-objetivos (não fazer)

- ❌ SPA, React/Vue, bundler JS, npm no runtime
- ❌ CSS custom fora dos tokens (nada de `#243247` solto — vira `--panel-2`)
- ❌ light mode / theme switcher (fora de escopo)
- ❌ gráficos canvas/SVG custom (fora de escopo; se um dia precisar, sparkline SVG inline mínima)
- ❌ WebSockets (SSE é suficiente e já existe)
- ❌ frameworks CSS adicionais (Bootstrap etc.)
- ❌ mexer no dashboard legado `/dashboard`

---

## 8. Referência rápida do que já está conforme

- Sidebar com estado ativo, `card`, `badge`, tokens cor/espacamento nas 7 páginas, HTMX+Alpine vendor locais, zero CDN, proxy same-origin (`/dash/api/*`) funcionando e testado.

## 9. Lacunas conhecidas (baseline 2026-08-03)

1. 4 handlers duplicam estrutura idêntica de tabela+filtro+pager (events, frontier, alerts, knowledge).
2. Nenhum drill-down entidade→detalhe (endpoints `/events/{id}`, `/knowledge/*/{id}` existem na Inspect API e não têm página).
3. Polling manual via `fetch()` em Alpine; sem indicador de loading; sem tratamento de erro além do dot.
4. `handleEventStream` (SSE) existe na Inspect API mas não está exposto no dashboard v2.
5. Sem foco visível, sem atalhos, sem toasts, sem confirmações (porque ainda não há mutações na v2).
6. `app.css` é build órfão: sem `tailwind.config` no repo, regenerar exige adivinhar o comando.
7. Documentação do operador (`docs/dashboard.md`) descreve apenas a SPA legada.
