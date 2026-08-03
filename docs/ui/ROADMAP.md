# Roadmap UI/UX — Dashboard v2

**Status:** aprovado para execução incremental em heartbeats
**Data:** 2026-08-03
**Base:** `docs/ui/DESIGN_SYSTEM.md` (tokens, componentes, padrões). Ler antes de qualquer item.

Princípio de corte: **1 etapa = 2–4 ciclos de heartbeat**, cada ciclo entrega uma fatia verificável (testes + templ generate + suite completa + página screenshotável). Nenhuma etapa bloqueia as seguintes por refactors grandes: mudanças são aditivas.

---

## Etapa 0 — Fundação do design system (2–3 ciclos)

**Objetivo:** uma base única e regenerável de tokens + componentes, sem mudar aparência visível.

| Ciclo | Entrega | Verificação |
|---|---|---|
| 0.1 | `tailwind.config.js` no repo + script documentado (`Makefile` target `assets`) regenerando `internal/dashboard/assets/app.css` idêntico em bytes ao atual; `--panel-2` entra como token; mapa severity→cor em `views/severity.go` | regenerar CSS == sem diff; `go build` |
| 0.2 | Componentes C3 `statCard`, C5 `pager`, C8 `statusDot` + C9 `emptyState` em `layout.templ`/novo `components.templ`; render tests (`server_v2_test.go` markers) | testes verdes; zero mudança visual |
| 0.3 | Refactor das 7 páginas para consumir C3/C5/C8/C9 (eliminando duplicação mensurável: −X linhas, reportar no commit) | snapshots de markers idênticos; suite completa |

**Conhecimento necessário:** Tailwind CLI standalone + `@theme`; Templ components/composition; nada novo de JS.

## Etapa 1 — Polish visual imediato (2 ciclos)

| Ciclo | Entrega |
|---|---|
| 1.1 | Hierarquia tipográfica e números pt-BR (`toLocaleString`), timestamps padronizados, `tabular-nums` em KPIs/tabelas; revisão de densidade (padding/alinhamento) nas 7 páginas com checklist do DESIGN_SYSTEM §4 |
| 1.2 | Estados de foco de teclado (outline `--accent`), hover states uniformes, `title` tooltips em IDs truncados em **todas** as tabelas, badges com mapeamento severity centralizado aplicado em alerts/resources/knowledge |

**Conhecimento necessário:** Tailwind `focus-visible`, `tabular-nums`; revisão WCAG de contraste (ferramenta: cálculo manual dos pares, já feito §6).

## Etapa 2 — HTMX de verdade: fragmentos parciais (3 ciclos)

**Objetivo:** trocar `fetch()`+JSON+`innerText` manual por `hx-get`+fragmentos Templ, com indicadores de loading e banners de erro.

| Ciclo | Entrega |
|---|---|
| 2.1 | Handlers `/dash/partials/overview`, `/dash/partials/alerts` (primeiros, menor risco): handlers Go que renderizam **fragmentos Templ** consultando a Inspect API server-side; páginas viram shell + `hx-get` + `hx-trigger="load, every 5s"`; spinner `hx-indicator`; banner de erro padronizado |
| 2.2 | Partials para events + frontier (tabelas paginadas: pager vira `hx-get` com querystring, sem Alpine de offset) |
| 2.3 | Partials para knowledge (4 tabs) + models + resources; remover o JS de fetch/polling morto; `docker-teste` de regressão visual via markers |

**Conhecimento necessário:** HTMX `hx-get/hx-swap/hx-trigger/hx-indicator/hx-target`, `HX-Request` header server-side; Templ para fragmentos (funções parciais); como o proxy `/dash/api` coexiste.

## Etapa 3 — Live via SSE (2 ciclos)

| Ciclo | Entrega |
|---|---|
| 3.1 | Proxy SSE: `GET /dash/api/events/stream` no V2Server encaminhando para Inspect `/events/stream` (streaming passthrough com flush; timeout e cancelamento de context; teste com servidor fake) |
| 3.2 | `hx-sse` (ou `sse-swap` via extensão SSE do HTMX — vendor do arquivo `ext/sse.js`) na página de eventos: live tail real; badge "ao vivo"; contador de eventos na sidebar/overview atualizado via SSE |

**Conhecimento necessário:** SSE (`text/event-stream`, flush, retry automático do browser), HTMX SSE extension, graceful degradation (SSE falhou→volta polling 5s).

## Etapa 4 — Drill-down e navegação entre entidades (3 ciclos)

| Ciclo | Entrega |
|---|---|
| 4.1 | Página `/dash/events/{id}` consumindo `GET /events/{id}`: payload completo, tipo, correlation, links para run/commit; breadcrumb de volta |
| 4.2 | Detalhe de conhecimento: `/dash/knowledge/sources/{id}`, `/claims/{id}`, `/observations/{id}` com evidence links (supporting/contradicting) navegáveis |
| 4.3 | `/dash/commits` + `/dash/commits/{id}` (payload JSON pretty, author, timestamp) — fecha o trio de listas+detalhe que falta |

**Conhecimento necessário:** padrões existentes; nada novo.

## Etapa 5 — Ações do operador (Control API, 3–4 ciclos)

**Pré-condição:** revisar permissões — o v2 hoje é read-only por decisão (`server_v2.go` comenta "Read-only GET proxy; no mutation routes"). Esta etapa muta, exige decisão explícita do usuário antes do ciclo 5.1.

| Ciclo | Entrega (controle fino, um recurso por ciclo) |
|---|---|
| 5.1 | Toast system (C11) + proxy `/dash/control/` autenticado internamente + primeira ação segura: retry de tarefa DLQ |
| 5.2 | Reply de workflow pendurado (`POST /v1/runs/{run_id}/reply`) via modal HTMX |
| 5.3 | Ações de fila: requeue/cancel com confirmação |
| 5.4 | Despesas/circuit-breaker: toggle de modelo habilitado (já existe modelo `enabled` na Control API) |

**Conhecimento necessário:** endpoints Control API (`internal/control/httpapi.go`), padrões de confirmação HTMX (`hx-confirm`), idempotência.

## Etapa 6 — Atalhos, docs e acabamento (2 ciclos)

| Ciclo | Entrega |
|---|---|
| 6.1 | Atalhos de teclado globais (`g e` eventos, `g k` conhecimento, `?` abre cheatsheet modal), `kbdHint` C12; foco inicial sensato por página |
| 6.2 | Reescrever `docs/dashboard.md` para a v2 (7+ páginas, padrões de leitura, como adicionar uma página nova — checklist apontando DESIGN_SYSTEM + componentes); arquivar seção legada claramente |

---

## Governança do roadmap

- Cada heartbeat que pegar um item deste roadmap registra no `CONTINUOUS_DEVELOPMENT.md` com a tag `[dash-ux Etapa X.Y]` e referência ao DESIGN_SYSTEM.
- Se uma etapa não cabe em 1 ciclo, o ciclo entrega a fatia testável e reabre o item — nunca commit de página pela metade sem markers.
- Change budget (do spec anti-regressão, §Clarifications): manter páginas individuais < ~350 linhas Templ após refactors; components.templ cresce livremente com testes.
- Ordem é a recomendada, não rígida: Etapa 4 (detalhes) pode ser puxada antes se o trabalho de campaigns precisar inspecionar eventos individuais com frequência.
