# Documentação do Painel do Operador (`dashboard`) — `motor-autonomo`

O painel do operador do **`motor-autonomo`** é a interface web de comando, controle e observabilidade servida pelo pacote `internal/dashboard`.

No projeto existem duas superfícies:
1. **v1 (Legacy):** Interface baseada em Vanilla JS (servida em `/` e `/dashboard`).
2. **v2 (Design System Standarized & Templ):** Nova interface construída com Go Templ, HTMX, Alpine.js e Tailwind CSS v4, servida na rota **/dash/** (`http://localhost:8080/dash/`).

---

## 1. Visão Geral do Painel v2 (`/dash/`)

### O que é
A versão v2 do painel do operador traz uma arquitetura de componentes server-side (Templ) fortemente embasada no Design System oficial do projeto (`docs/ui/DESIGN_SYSTEM.md`). Ela fornece visualização em tempo real da telemetria, streaming SSE de eventos, e **controles operacionais completos**:
- Pausar / Retomar / Cancelar missão ativa
- Responder perguntas de auditoria do operador em tempo real
- Cadastrar e gerenciar provedores LLM (sem modelo fixo obrigatório)
- Descobrir dinamicamente modelos disponíveis via endpoint `/v1/models` OpenAI
- Cadastrar preferências e bindings opcionais de modelos com prioridades e limites de contexto
- Desbloquear e persistir segredos no cofre cifrado local (AES-256-GCM)
- Visualizar pressões de contexto, catálogo de conhecimento e estado de recursos do runtime

### URLs do Navegador (v2)

| Página | Rota | Descrição |
|---|---|---|
| **Visão Geral** | `http://127.0.0.1:8080/dash/` | Métricas KPI, controles de missão, Q&A do operador, streaming de eventos |
| **Eventos** | `http://127.0.0.1:8080/dash/events` | Explorador de eventos com resumo, busca por ID/kind e cópia de JSON |
| **Detalhes de Evento** | `http://127.0.0.1:8080/dash/events/{id}` | Visão técnica detalhada de um evento individual |
| **Modelos & LLMs** | `http://127.0.0.1:8080/dash/models` | Provedores LLM, descoberta dinâmica `/v1/models`, bindings e cofre |
| **Recursos & Gates** | `http://127.0.0.1:8080/dash/resources` | Recursos de sistema, retenção de dados e limites |
| **Fronteira & Ações** | `http://127.0.0.1:8080/dash/frontier` | Oportunidades de fronteira, higiene de estado e ações |
| **Alertas & Telemetria**| `http://127.0.0.1:8080/dash/alerts` | Status de alertas ativos, severidades e exportação OTel |
| **Conhecimento** | `http://127.0.0.1:8080/dash/knowledge` | Catálogo de conhecimento, fontes, observações e alegações |

---

## 2. Atalhos de Teclado Globais & Cheatsheet (`?`)

O painel v2 inclui suporte a navegação por teclado sem interferir em inputs de texto:

- <kbd>g</kbd> <kbd>o</kbd> → Visão Geral (`/dash`)
- <kbd>g</kbd> <kbd>e</kbd> → Explorador de Eventos (`/dash/events`)
- <kbd>g</kbd> <kbd>m</kbd> → Modelos & LLMs (`/dash/models`)
- <kbd>g</kbd> <kbd>r</kbd> → Recursos & Gates (`/dash/resources`)
- <kbd>g</kbd> <kbd>f</kbd> → Fronteira & Ações (`/dash/frontier`)
- <kbd>g</kbd> <kbd>a</kbd> → Alertas & Telemetria (`/dash/alerts`)
- <kbd>g</kbd> <kbd>k</kbd> → Base de Conhecimento (`/dash/knowledge`)
- <kbd>?</kbd> → Abrir / Fechar Cheatsheet de Atalhos de Teclado

---

## 3. Arquitetura do Design System & Componentes Templ

Toda a interface consome a biblioteca de componentes em `internal/dashboard/views/components.templ`:

- `@card(title)`: Conteiner padrão de seção com bordas e título estilizado
- `@statCard(label, value, sub, tone)`: Exibição de KPI com cor e subtítulo
- `@statusDot(ok, label)`: Indicador de status ativo com animação de pulso
- `@badge(label, kind)`: Badge semântica (`success`, `warning`, `error`, `info`, `neutral`)
- `@alertBanner(title, msg, kind)`: Banner de aviso e alertas no topo
- `@emptyState(msg)`: Caixas de estado vazio para tabelas sem dados
- `@pager(prefix, current, total, hasPrev, hasNext)`: Controle de paginação
- `@kbd(key)`: Tecla de atalho estilizada
- `@copyButton(text)`: Botão compacto de cópia para a área de transferência com toast

---

## 4. Filosofia de Segurança e Isolamento do Vault

- **Zero Secrets na UI:** Tokens e chaves de API nunca são expostos em texto puro nas respostas HTTP.
- **Cofre Local Cifrado:** As chaves fornecidas na tela de Modelos são gravadas via `POST /dash/api/vault/secrets/provider/{id}/api-key` no cofre local seguro (AES-256-GCM).
- **Desbloqueio sob Demanda:** O operador pode desbloquear o cofre inserindo a senha mestra diretamente na barra do cofre.

---

## 5. Como Adicionar uma Nova Página no Dashboard v2

### Passo 1: Criar o Template Templ
Crie um arquivo em `internal/dashboard/views/<sua_pagina>.templ`:

```templ
package views

templ SuaPagina() {
    @layout("Título da Sua Página", "/dash/sua-pagina", StandardNav("/dash/sua-pagina")) {
        <div class="space-y-4">
            @card("Título do Bloco") {
                <p class="text-sm font-mono text-[var(--text)]">Conteúdo aqui...</p>
            }
        </div>
    }
}
```

### Passo 2: Gerar os Arquivos Go
```bash
go run github.com/a-h/templ/cmd/templ@latest generate ./internal/dashboard/views/
```

### Passo 3: Registrar a Rota no `server_v2.go`
```go
mux.Handle("GET /dash/sua-pagina", http.HandlerFunc(s.handleSuaPagina))
```

E implemente o handler correspondente retornando `component.Render(r.Context(), w)`.
