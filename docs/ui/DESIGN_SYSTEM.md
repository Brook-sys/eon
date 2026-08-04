# Design System — `motor-autonomo` Dashboard v2

**Status:** Ativo / Produção
**Versão:** 2.1.0 (Design System Standarized & System Architecture Enforced)
**Escopo:** Todas as superfícies operacionais sob `/dash/*` (Go Templ + Alpine.js + HTMX + Tailwind CSS v4)

---

## 1. Princípios de Design & Arquitetura de Sistema

O Design System do **`motor-autonomo`** é construído sob os princípios de **Observabilidade de Alta Fidelidade**, **Resiliência Server-First**, e **Interface Dark-First de Precisão Operacional** (inspirado na estética e UX do Vercel, Linear e Supabase).

### 1.1 Princípios Fundamentais

1. **Clareza Informacional Sem Poluição Visual:**
   - Evita cards aninhados pesados, bordas duplicadas ou sombras chamativas.
   - Utiliza contraste sutil de fundo (`--bg` vs `--panel` vs `--panel-sub`), tipografia precisa e indicadores de estado coloridos (`--ok`, `--warn`, `--err`, `--accent`).
2. **Resiliência e Recuperação Transparente:**
   - A interface monitora proativamente a conectividade com as APIs do runtime (`/dash/api/control/` e `/dash/api/inspect/`).
   - Notifica o operador instantaneamente via Toast ou Banners sobre desconexões ou retries sem travar a navegação.
3. **Desempenho Server-Side Type-Safe com Templ:**
   - Todo o HTML é compilado em Go via Templ, garantindo tempo de resposta sub-milissegundo para a primeira pintura (SSR).
   - Interações dinâmicas são orquestradas por Alpine.js (~15 KB) e HTMX (~14 KB) sem peso de frameworks SPA.
4. **Navegação Eficiente e Atalhos de Teclado:**
   - Atalhos globais sem interferir em campos de formulário ou `input/textarea`.
   - Atalho `?` abre o cheatsheet interativo de navegação rápida em qualquer página.
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

### 2.3 Tipografia e Escala

- **Corpo da Página:** `14px / 1.5` (`var(--sans)`).
- **Labels de Seção:** `11px uppercase tracking-wider` (`var(--muted)` font-bold).
- **Valores KPI / Stat:** `24px (text-2xl)` ou `30px (text-3xl)` (`var(--mono)` font-bold).
- **Elementos Técnicos (IDs, Hashes, JSON, Logs):** `12px` (`var(--mono)`).

---

## 3. Biblioteca de Componentes Templ (`internal/dashboard/views/components.templ`)

Toda a interface v2 consome componentes Templ reutilizáveis e fortemente tipados.

| Componente | Assinatura | Descrição / Uso |
|---|---|---|
| `Card` | `card(title string)` | Conteiner com borda sutil, título em uppercase e padding responsivo |
| `StatCard` | `statCard(label, value, sub, tone)` | Exibição de KPI/métrica individual com destaque visual |
| `StatusDot` | `statusDot(ok bool, label string)` | Indicador de pulso verde/vermelho com rótulo semântico |
| `Badge` | `badge(label, kind string)` | Pill de status (`success`, `warning`, `error`, `info`, `neutral`) |
| `AlertBanner` | `alertBanner(title, msg, kind)` | Caixa de alerta em destaque no topo das páginas |
| `EmptyState` | `emptyState(msg string)` | Estado vazio padronizado com borda pontilhada e mensagem explicativa |
| `Pager` | `pager(prefix string, current, total int, hasPrev, hasNext bool)` | Barra de paginação uniforme com estados desabilitados |
| `Kbd` | `kbd(key string)` | Tecla de atalho visual padronizada (`<kbd>g</kbd>`) |
| `LoadingSkeleton` | `loadingSkeleton(rows int)` | Animação de carregamento (shimmer) para tabelas e listas |
| `CopyButton` | `copyButton(text string)` | Botão compacto para cópia de JSON/IDs com feedback de toast |

---

## 4. Atalhos de Teclado Nativos (`layout.templ`)

O dashboard v2 inclui navegação global por teclado ativada quando o usuário pressiona a sequência correspondente (fora de campos de formulário):

- <kbd>g</kbd> <kbd>o</kbd> → Ir para **Visão Geral** (`/dash`)
- <kbd>g</kbd> <kbd>e</kbd> → Ir para **Eventos** (`/dash/events`)
- <kbd>g</kbd> <kbd>m</kbd> → Ir para **Modelos & LLMs** (`/dash/models`)
- <kbd>g</kbd> <kbd>r</kbd> → Ir para **Recursos & Gates** (`/dash/resources`)
- <kbd>g</kbd> <kbd>f</kbd> → Ir para **Fronteira & Ações** (`/dash/frontier`)
- <kbd>g</kbd> <kbd>a</kbd> → Ir para **Alertas & Telemetria** (`/dash/alerts`)
- <kbd>g</kbd> <kbd>k</kbd> → Ir para **Conhecimento** (`/dash/knowledge`)
- <kbd>?</kbd> → Abrir / Fechar o **Cheatsheet de Atalhos**

---

## 5. Diretrizes de Qualidade e Manutenibilidade

1. **Compilação Templ Obrigatória:** Qualquer alteração em arquivos `.templ` exige a execução de `templ generate ./internal/dashboard/views/`.
2. **Check de Formatação:** O código gerado deve passar limpo por `git diff --check`.
3. **Testes Unitários de Componentes:** Novos componentes devem ter casos de teste correspondentes em `components_test.go`.
4. **Respeito às Regras de Execução:** Toda iteração de sistema exige validação com a suíte de testes Go (`go test ./...`) e inferência live obrigatória (Regra 6).
