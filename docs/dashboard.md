# Documentação do Painel do Operador (`dashboard`) — `motor-autonomo`

O painel do operador do **`motor-autonomo`** é uma interface web moderna servida pelo pacote `internal/dashboard`.

No projeto existem duas superfícies:
1. **v1 (Legacy):** Interface baseada em Vanilla JS (servida em `/` e `/dashboard`).
2. **v2 (Design System & HTMX):** Nova interface construída com Go Templ, HTMX, Alpine.js e Tailwind CSS v4, servida na rota **/dash/** (`http://localhost:8080/dash/`).

---

## 1. Visão Geral do Painel v2 (`/dash/`)

### O que é
A versão v2 do painel do operador traz uma arquitetura de componentes server-side (Templ) baseada no Design System oficial (`docs/ui/DESIGN_SYSTEM.md`). Ela fornece visualização em tempo real de eventos, posture de modelos LLM, uso de recursos, fonte de conhecimento e métricas do sistema com alinhamento numérico legível e suporte a atalhos de teclado.

### Como acessar
Quando o runtime é iniciado com a flag `-dashboard=true` (padrão) e a flag `-listen` configurada (ex.: `127.0.0.1:8080`), o painel v2 fica acessível nas seguintes URLs do seu navegador:

*   **Página inicial v2:** `http://127.0.0.1:8080/dash/`
*   **Eventos:** `http://127.0.0.1:8080/dash/events`
*   **Detalhes de Evento:** `http://127.0.0.1:8080/dash/events/{id}`
*   **Modelos:** `http://127.0.0.1:8080/dash/models`
*   **Recursos:** `http://127.0.0.1:8080/dash/resources`
*   **Fronteira:** `http://127.0.0.1:8080/dash/frontier`
*   **Alertas:** `http://127.0.0.1:8080/dash/alerts`
*   **Conhecimento:** `http://127.0.0.1:8080/dash/knowledge`

### Atalhos de Teclado (v2)
Você pode navegar rapidamente entre as páginas pressionando as teclas no teclado:
*   `g` + `o`: Ir para Visão Geral (`/dash/`)
*   `g` + `e`: Ir para Explorador de Eventos (`/dash/events`)
*   `g` + `m`: Ir para Postura de Modelos (`/dash/models`)
*   `g` + `k`: Ir para Base de Conhecimento (`/dash/knowledge`)
*   `?`: Abrir modal Cheatsheet de Atalhos de Teclado

---

## 2. Como Adicionar uma Nova Página no Dashboard v2

Para adicionar uma nova sub-página no dashboard v2, siga os 3 passos abaixo:

### Passo 1: Criar o Template Templ
Crie um arquivo em `internal/dashboard/views/<sua_pagina>.templ`.

```templ
package views

templ SuaPagina() {
    @layout("Título da Sua Página", "/dash/sua-pagina", []NavItem{
        {Href: "/dash", Label: "Visão geral"},
        {Href: "/dash/sua-pagina", Label: "Sua Página", Active: true},
    }) {
        <div class="space-y-4">
            @card("Título do Bloco") {
                <p class="text-sm font-mono text-[var(--text)]">Conteúdo aqui...</p>
            }
        </div>
    }
}
```
Gere os arquivos Go com:
```bash
go run github.com/a-h/templ/cmd/templ@latest generate ./internal/dashboard/views/
```

### Passo 2: Registrar o Handler no `server_v2.go`
No arquivo `internal/dashboard/server_v2.go`:

1. Registre a rota em `Handler()`:
```go
mux.Handle("GET /dash/sua-pagina", http.HandlerFunc(s.handleSuaPagina))
```

2. Escreva o handler:
```go
func (s *V2Server) handleSuaPagina(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "text/html; charset=utf-8")
    w.Header().Set("Cache-Control", "no-store")
    component := views.SuaPagina()
    if err := component.Render(r.Context(), w); err != nil {
        s.Logger.Printf("render sua pagina: %v", err)
    }
}
```

### Passo 3: Adicionar o Link na Sidebar (`sidebar.templ`)
Edite `internal/dashboard/views/sidebar.templ` e inclua o item na lista de navegação.

---

## 3. Painel Legacy (v1)

A versão v1 permanece acessível em `http://127.0.0.1:8080/` e `http://127.0.0.1:8080/dashboard` como fallback e para envio de comandos de controle (mutações da Control API).

### Filosofia de Segurança e Isolamento
*   **Zero Secrets na UI:** Tokens e chaves de API nunca são renderizados na tela.
*   **Write-only Vault:** Inserção de chaves via cofre local cifrado com AES-256-GCM.


2.  **Overview (`data-view="home mission"`)**
    *   **Propósito:** Exibe metadados de execução do runtime (`process_mode`, `control_revision`, `event_head_sequence`, `pending_commands`, `pending_questions`, `evicted_subagents`) e estatísticas da missão (status, revisão ativa, propósito, despacho, agenda, horizonte, fronteira e diagnósticos de continuidade).
    *   **Botões de Operação (`#missionOps`):**
        *   `Pause dispatch` (`#btnPause`): Envia o comando `PAUSE_MISSION` via `POST /api/control/commands` para pausar novos despachos.
        *   `Resume dispatch` (`#btnResume`): Envia o comando `RESUME_MISSION` para retomar o processamento de tarefas.
        *   `Cancel mission` (`#btnCancel`): Envia o comando `CANCEL_MISSION` (solicita confirmação do operador).

3.  **Alertas / Telemetria (`data-view="home monitor"`)**
    *   **Propósito:** Exibe o snapshot de alertas derivados (com severidades `critical`, `warning` ou `info`) e a postura de exportação OTel.
    *   **Botões:**
        *   `Atualizar alertas` (`#btnAlertsRefresh`): Requisita `GET /api/inspect/alerts?mission_id=...`.
        *   `GET /telemetry` (`#btnTelemetry`): Requisita `GET /api/inspect/telemetry`.

4.  **Perguntas Pendentes (`data-view="home mission"`)**
    *   **Propósito:** Lista as dúvidas de auditoria ou interrupção geradas pelo runtime que exigem resposta humana antes de prosseguir.
    *   **Botões por Card:**
        *   `Responder` (`data-fill`): Copia o `question_id` e `revision` para a seção de resposta da área de **Missão** e foca o campo de resposta.

#### Quando Usar
Usada no dia a dia para monitorar se a missão está progredindo, se o despacho está ativo ou pausado e para responder rapidamente a qualquer bloqueio ou pergunta do sistema.

#### Dicas Práticas
*   Mantenha a linha do tempo conectada (SSE) para ver eventos chegando em tempo real.
*   Se o status da agenda ou horizonte indicar `replenish` ou `PAUSED`, verifique o bloco de diagnósticos de continuidade (*continuity_blocked*).

---

### Área 2: Missão (`mission`)

#### O que faz
Permite inspecionar em profundidade a missão ativa, submeter emendas formais de missão (FR-AUTH-004) e enviar respostas correlacionadas a perguntas do sistema.

#### Seções e Blocos
1.  **Contexto & Overview:** (Mesmos blocos compartilhados com a Visão Geral).
2.  **Emenda de Missão (FR-AUTH-004) (`data-view="mission advanced"`)**
    *   **Propósito:** Propor alterações na especificação da missão de forma append-only sem mutar a revisão ativa in-place.
    *   **Campos:**
        *   `base_revision` (`#amendBase`): Número da revisão base (ex.: `1`).
        *   `candidate_revision` (`#amendCandidate`): Número da revisão candidata (ex.: `2`).
        *   `status` (`#amendStatus`): Estado desejado (`ACTIVE`, `PAUSED`, `CANCELLED`).
        *   `purpose` (`#amendPurpose`): Texto do propósito da revisão candidata.
        *   `original_text` (`#amendText`): Texto completo da especificação da missão.
        *   `domains (CSV)` (`#amendDomains`): Domínios de atuação (ex.: `epistemology,runtime`).
        *   `policies (CSV)` (`#amendPolicies`): Políticas aplicáveis (ex.: `fail_closed`).
        *   `standing_objectives (CSV)` (`#amendStanding`): Objetivos permanentes.
        *   `recurring_obligations (JSON Array)` (`#amendRecurring`): Array JSON de obrigações recorrentes (FR-DUR-011).
        *   `budget.model_calls` (`#amendBudgetCalls`): Limite de chamadas a modelos LLM.
        *   `budget.tokens` (`#amendBudgetTokens`): Limite de orçamento em tokens.
        *   `reason` (`#amendReason`): Motivo explícito informado pelo operador (obrigatório).
    *   **Botões:**
        *   `Carregar ativa` (`#btnAmendLoad`): Requisita `GET /api/control/missions/{mission_id}/active` e preenche os campos do formulário.
        *   `Preview` (`#btnAmendPreview`): Envia `POST /api/control/missions/amendments/preview` para calcular o diff e avaliar o impacto sem alterar o banco.
        *   `Accept (append)` (`#btnAmendAccept`): Envia `POST /api/control/missions/amendments/accept` para promover a revisão candidata a nova ativa.

3.  **Resposta Correlacionada (`data-view="mission"`)**
    *   **Propósito:** Enviar respostas tipadas a perguntas pendentes.
    *   **Campos:**
        *   `question_id` (`#answerQuestionId`): ID da pergunta sendo respondida.
        *   `expected_revision` (`#answerRevision`): Versão da pergunta esperada (previne respostas obsoletas).
        *   `kind` (`#answerKind`): Tipo de resposta (`TEXT`, `OPTION`, `MULTI_OPTION`, `CONFIRM`, `SKIP`).
        *   `texto / option_ids (CSV)` (`#answerBody`): Conteúdo em texto ou lista de opções separadas por vírgula.
        *   `idempotency_key` (`#answerIdem`): Chave de idempotência (auto-gerada se vazia).
    *   **Botões:**
        *   `Enviar resposta` (`#btnAnswer`): Dispara `POST /api/control/questions/{qid}/answers`.

#### Quando Usar
Usada quando é necessário alterar os objetivos ou o orçamento de uma missão em andamento, ou para responder detalhadamente a uma solicitação de decisão do motor.

#### Dicas Práticas
*   Sempre clique em `Preview` antes de `Accept (append)` na emenda de missão para verificar se o impacto não foi bloqueado (*impact.blocked*).

---

### Área 3: Monitoramento (`monitor`)

#### O que faz
Focada em auditoria técnica e diagnóstico operacional do runtime. Permite rastrear execuções individuais, visualizar eventos via SSE em tempo real e monitorar recursos.

#### Seções e Blocos
1.  **Inspetor de Execução (`data-view="monitor advanced"`)**
    *   **Propósito:** Inspecionar os detalhes completos de uma operação (`operation`), commit (`commit`) ou comando (`command`).
    *   **Campos:**
        *   `tipo` (`#inspKind`): Seletor entre `operation`, `commit` e `command`.
        *   `id` (`#inspId`): ID do recurso a inspecionar.
    *   **Botões:**
        *   `Inspecionar` (`#btnInspLoad`): Executa a requisição correspondente na Inspect API (`/api/inspect/operations/{id}`, `/commits/{id}` ou `/commands/{id}`).
    *   **Navegação por Abas (`#inspTabs`):**
        *   `Resumo`: Mostra uma tabela de chave-valor com metadados principais.
        *   `Linhagem`: Exibe a árvore de procedência (inquiry, spec, head commit).
        *   `ChangeSet`: Exibe os conjuntos de alterações propostos e aceitos.
        *   `Raw / validação`: Exibe saídas do modelo (redigidas) e recibos de validação.
        *   `Eventos`: Exibe os eventos de auditoria correlacionados.
        *   `JSON`: Exibe a estrutura JSON bruta retornada pela API.

2.  **Timeline (SSE) (`data-view="monitor advanced"`)**
    *   **Propósito:** Exibe o fluxo contínuo de eventos do sistema em tempo real via Server-Sent Events.
    *   **Campos de Filtro:**
        *   `after_sequence` (`#afterSeq`): Sequência inicial para consumo de eventos (padrão `0`).
        *   `filtro kind` (`#eventKind`): Filtrar por tipo de evento (ex.: `operation.completed`).
        *   `filtro namespace` (`#eventNamespace`): Filtrar por namespace.
        *   `filtro request_id` (`#eventRequestId`): Filtrar por ID de requisição correlacionada.
    *   **Caixa de Saída (`#timeline`):** Exibe até 400 linhas / 64 KB de log em formato monospace.

3.  **Models / Resources / Context Pressure (`data-view="models monitor advanced"`)**
    *   **Propósito:** Visualizar a saúde e a pressão sobre os modelos e recursos do sistema.
    *   **Campos:** `resource_id` (`#resourceId`), `binding_id` (`#pressureBindingId`).
    *   **Botões:**
        *   `Postura por binding` (`#btnModelBindingsList`): `GET /api/inspect/model-bindings`.
        *   `Listar resources` (`#btnResourcesList`): `GET /api/inspect/resources`.
        *   `Listar context pressure` (`#btnContextPressureList`): `GET /api/inspect/model-context-pressures`.
        *   `Resource` (`#btnResourceDetail`): `GET /api/inspect/resources/{id}`.
        *   `Pressure` (`#btnPressureDetail`): `GET /api/inspect/model-context-pressures/{id}`.

#### Quando Usar
Usada ao investigar uma falha em uma operação específica, verificar se ocorreu *circuit breaker* em algum recurso ou depurar o fluxo de eventos via SSE.

#### Dicas Práticas
*   Se um evento indicar `subagent.lease_evicted`, utilize o Inspetor de Execução para checar o estado da operação correspondente.

---

### Área 4: Conhecimento (`knowledge`)

#### O que faz
Navegador de leitura do grafo de conhecimento acumulado pelo sistema, permitindo explorar alegações (*claims*), fontes (*sources*), observações (*observations*), artefatos (*artifacts*), fronteira de trabalho (*frontier*) e histórico de commits.

#### Seções e Blocos
1.  **Navegador de Conhecimento (`data-view="knowledge"`)**
    *   **Propósito:** Consultar o repositório epistemológico de forma totalmente segura (dados confidenciais são redigidos pela API).
    *   **Campos & Filtros:**
        *   `coleção` (`#knowKind`): Select (`claims`, `sources`, `observations`, `artifacts`).
        *   `id (detalhe)` (`#knowId`): ID do item para busca direta.
        *   Checkboxes: `só claims sem evidência`, `só claims com contradição`, `só artifacts stale`, `só observations linkadas`.
        *   `q / texto` (`#knowQ`): Busca por texto simples em declarações/localizadores.
        *   `kind` (`#knowKindFilter`): Filtro por tipo específico.
        *   `provenance` (`#knowProvenance`): Filtro de proveniência de observações.
    *   **Botões:**
        *   `Listar` (`#btnKnowList`): `GET /api/inspect/knowledge/{coleção}?`.
        *   `Detalhe` (`#btnKnowDetail`): `GET /api/inspect/knowledge/{coleção}/{id}`.
        *   `Catálogo` (`#btnKnowRefresh`): Exibe totais agregados (`GET /api/inspect/knowledge`).

2.  **Frontier / Higiene (`data-view="knowledge advanced"`)**
    *   **Propósito:** Visualizar as oportunidades de trabalho (*WorkOpportunity*) e executar simulações (*dry-run*) de higiene da fronteira.
    *   **Campos:** `status` (`#frontStatus`), `family` (`#frontFamily`), `opportunity id` (`#frontOppId`).
    *   **Botões:**
        *   `Listar` (`#btnFrontList`): `GET /api/inspect/frontier?`.
        *   `Dry-run hygiene` (`#btnFrontHygiene`): `GET /api/inspect/frontier/hygiene?mission_id=...`.
        *   `Detalhe` (`#btnFrontDetail`): `GET /api/inspect/frontier/opportunities/{id}`.

3.  **Commits / Provider (`data-view="knowledge advanced"`)**
    *   **Propósito:** Listar commits canônicos gravados no banco e verificar o perfil do provedor.
    *   **Campos:** `mission_revision_id` (`#commitRev`), `head_only` (`#commitHeadOnly`), `limit` (`#commitLimit`).
    *   **Botões:**
        *   `Listar commits` (`#btnCommitList`): `GET /api/inspect/commits?`.
        *   `Perfil declarado` (`#btnProviderProfile`): `GET /api/inspect/provider/profile`.
        *   `Probe live` (`#btnProviderProbe`): `GET /api/inspect/provider/profile/probe`.

#### Quando Usar
Usada para validar a base de conhecimento construída pelo agente autônomo, verificar contradições registradas ou inspecionar a árvore de oportunidades de planejamento.

---

### Área 5: Provedores e Modelos (`models`)

#### O que faz
Interface simplificada para cadastro e gerenciamento de provedores de IA compatíveis com a API da OpenAI (Groq, NVIDIA NIM, Ollama, OpenAI) sem a necessidade de editar arquivos JSON manualmente.

#### Seções e Blocos
1.  **Lista de Provedores (`#providerCards`)**
    *   Exibe cartões com os provedores cadastrados na configuração ativa, seus modelos atrelados (`bindings`) e status (`Ativo` / `Desativado`).
    *   **Ações nos Cards:** `Editar`, `Ativar/Desativar modelos`, `Remover`.

2.  **Formulário de Cadastro/Edição de Provedor (`#providerForm`)**
    *   **Campos:**
        *   `Identificador` (`#providerID`): ID único (ex.: `groq-principal`, apenas letras minúsculas, números, ponto, hífen e underline).
        *   `Tipo` (`#providerKind`): Select (`openai_compatible`, `groq`, `nvidia_nim`).
        *   `Endereço da API` (`#providerURL`): URL base do provedor (ex.: `https://api.groq.com/openai/v1`).
        *   `Modelo` (`#providerModel`): Nome exato do modelo (ex.: `llama-3.3-70b-versatile`).
        *   `Janela de contexto` (`#providerContext`): Limite de contexto em tokens (ex.: `8192`).
        *   `Máximo de saída` (`#providerOutput`): Máximo de tokens na resposta (deve ser estritamente menor que o contexto).
        *   `Chamadas simultâneas` (`#providerConcurrency`): Concorrência máxima (padrão `1`).
        *   `Prioridade` (`#providerPriority`): Prioridade de seleção (ex.: `10`).
        *   `Chave da API` (`#providerSecret`): Campo de senha write-only. Se preenchido, a chave é enviada diretamente ao Cofre (`Vault API`).
        *   `Referência temporária da credencial` (`#providerKeyRef`): Nome da variável de ambiente de referência (ex.: `GROQ_API_KEY`).
        *   `Timeout` (`#providerTimeout`): Tempo limite em segundos (ex.: `45`).
    *   **Botões:**
        *   `Criar rascunho` (`type="submit"`): Armazena o segredo no Vault (se fornecido) e gera um rascunho (*draft*) no escopo `MODELS`.
        *   `Limpar` (`#btnProviderCancel`): Restaura o formulário para o estado inicial.

3.  **Cofre de Credenciais (`Vault API`)**
    *   **Status (`#vaultState`):** Indica se o cofre está inicializado, bloqueado ou desbloqueado.
    *   **Campos:** `Senha mestra` (`#vaultPassword`, mínimo 12 caracteres).
    *   **Botões:**
        *   `Criar ou desbloquear` (`#btnVaultUnlock`): Inicializa ou desbloqueia o cofre local (`POST /api/vault/initialize` ou `/unlock`).
        *   `Bloquear agora` (`#btnVaultLock`): Bloqueia o cofre (`POST /api/vault/lock`).

#### Quando Usar
Usada no *setup* inicial para conectar o motor a provedores de LLM externos ou locais e gerenciar suas credenciais com segurança.

---

### Área 6: Avançado (`advanced`)

#### O que faz
Área técnica centralizadora que reúne a gestão de **Configuração Versionada** (escopos `INTERRUPTION`, `HORIZON`, `RUNTIME`, `SCHEDULER`, `CHANNELS`, `MODELS`), presets de modelo e atalhos para inspecionar recursos técnicos que aparecem em múltiplas telas.

#### Seções e Blocos
1.  **Configuração Versionada (`data-view="models advanced"`)**
    *   **Filtros:** `scope` (`#cfgScope`), `draft status filter` (`#cfgStatus`).
    *   **Painel Esquerdo (Estado Atual):**
        *   *Revisão ativa:* Exibe a versão e payload ativos.
        *   *Histórico de revisões:* Permite visualizar detalhes ou acionar **Rollback semântico** (`POST /api/control/config/revisions/rollback`), que re-aplica um payload ancestral criando uma nova revisão.
        *   *Drafts:* Lista rascunhos em aberto, validados ou aplicados.
    *   **Painel Direito (Criação e Operação de Drafts):**
        *   `based_on_revision` (`#cfgBasedOn`), `applicability` (`#cfgApplicability`), `reason` (`#cfgReason`), `payload JSON` (`#cfgPayload`).
        *   Botões: `Criar draft` (`#btnCfgCreate`), `Preencher default` (`#btnCfgFillDefault`).
    *   **Presets de Modelo Qualificados:**
        *   Permite selecionar presets pré-qualificados do sistema.
        *   Botões: `Carregar presets` (`#btnPresetRefresh`), `Criar draft desabilitado` (`#btnPresetDraft`), `Preview de habilitação` (`#btnPresetEnablePreview`), `Habilitar via novo draft` (`#btnPresetEnableDraft`).

#### Quando Usar
Usada por operadores avançados para alterar parâmetros operacionais do runtime (limites de interrupção, agendador, canais, janelas do horizonte) através do ciclo rigoroso de `Draft` -> `Validate` -> `Apply`.

---

## 3. Endpoints da API Utilizados pelo Painel

O painel interage com três APIs internas através do prefixo `/api`. **Nenhum segredo ou chave privada é exposto pelas respostas das APIs.**

### Control API (`/api/control/`)
*   `GET /api/control/questions?mission_id={id}&status=PENDING` — Lista perguntas pendentes.
*   `POST /api/control/questions/{id}/answers` — Submete resposta a uma pergunta.
*   `POST /api/control/commands` — Envia comandos de missão (`PAUSE_MISSION`, `RESUME_MISSION`, `CANCEL_MISSION`).
*   `GET /api/control/missions/{id}/active` — Obtém a revisão de missão ativa.
*   `POST /api/control/missions/amendments/preview` — Simula impacto e diff de emenda de missão.
*   `POST /api/control/missions/amendments/accept` — Aplica emenda de missão append-only.
*   `GET /api/control/config/revisions/active?scope={scope}` — Obtém revisão ativa de um escopo de configuração.
*   `GET /api/control/config/revisions?scope={scope}` — Lista o histórico de revisões.
*   `POST /api/control/config/revisions/rollback` — Realiza rollback semântico de configuração.
*   `GET /api/control/config/drafts?scope={scope}` — Lista rascunhos de configuração.
*   `GET /api/control/config/drafts/{id}` — Detalhes de um rascunho.
*   `POST /api/control/config/drafts` — Cria novo rascunho de configuração.
*   `POST /api/control/config/drafts/{id}/validate` — Valida um rascunho (gera preview/diff).
*   `POST /api/control/config/drafts/{id}/apply` — Aplica um rascunho validado como nova revisão imutável.
*   `GET /api/control/config/drafts/{id}/receipt` — Obtém o recibo de aplicação de um rascunho.
*   `GET /api/control/model-presets` — Lista presets de modelos disponíveis no catálogo.
*   `POST /api/control/model-presets/{id}/drafts` — Cria draft desabilitado a partir de um preset.
*   `POST /api/control/model-presets/{id}/enablement-preview` — Simula a habilitação de um preset.
*   `POST /api/control/model-presets/{id}/enable-drafts` — Cria draft de habilitação de um preset.

### Inspect API (`/api/inspect/`)
*   `GET /api/inspect/health` — Estado de saúde do runtime e do banco de dados.
*   `GET /api/inspect/overview?mission_id={id}` — Dados consolidados de visão geral da missão e runtime.
*   `GET /api/inspect/alerts?mission_id={id}` — Snapshot de alertas derivados.
*   `GET /api/inspect/telemetry` — Configuração e estatísticas da postura de telemetria OTel.
*   `GET /api/inspect/events/stream?after_sequence={seq}` — Stream de eventos em tempo real (SSE).
*   `GET /api/inspect/operations/{id}` — Inspeção detalhada de uma operação.
*   `GET /api/inspect/commits/{id}` — Inspeção detalhada de um commit.
*   `GET /api/inspect/commands/{id}` — Inspeção detalhada de um comando de controle.
*   `GET /api/inspect/knowledge` — Métricas agregadas do catálogo de conhecimento.
*   `GET /api/inspect/knowledge/{claims|sources|observations|artifacts}` — Listagem paginada de coleções de conhecimento.
*   `GET /api/inspect/knowledge/{coleção}/{id}` — Detalhe individual de um elemento de conhecimento.
*   `GET /api/inspect/frontier` — Listagem da fronteira de oportunidades de trabalho (*WorkOpportunity*).
*   `GET /api/inspect/frontier/hygiene?mission_id={id}` — Simulação (*dry-run*) de limpeza e higiene da fronteira.
*   `GET /api/inspect/frontier/opportunities/{id}` — Detalhes de uma oportunidade de trabalho.
*   `GET /api/inspect/commits` — Histórico paginado de commits.
*   `GET /api/inspect/provider/profile` — Perfil declarado do provedor de modelos.
*   `GET /api/inspect/provider/profile/probe` — Sondagem (*probe*) em tempo real do provedor.
*   `GET /api/inspect/model-bindings` — Postura de bindings ativos e limites de modelo.
*   `GET /api/inspect/resources` — Lista de recursos e status de *circuit breaker*.
*   `GET /api/inspect/resources/{id}` — Detalhe de um recurso específico.
*   `GET /api/inspect/model-context-pressures` — Lista de monitoramento de pressão de contexto de modelos.
*   `GET /api/inspect/model-context-pressures/{id}` — Detalhe de pressão de contexto de um binding.

### Vault API (`/api/vault/`) *(Disponível apenas em localhost)*
*   `GET /api/vault/status` — Verifica se o cofre de credenciais está inicializado e desbloqueado.
*   `POST /api/vault/initialize` — Inicializa o cofre com uma senha mestra (mínimo 12 caracteres).
*   `POST /api/vault/unlock` — Desbloqueia o cofre com a senha mestra.
*   `POST /api/vault/lock` — Bloqueia o cofre.
*   `PUT /api/vault/secrets/provider/{id}/api-key` — Armazena com segurança (write-only, AES-256-GCM) a chave de API de um provedor.

---

## 4. Glossário de Termos do Domínio

*   **Mission (Missão):** A especificação formal do trabalho atribuído ao motor autônomo. Contém objetivos, regras, domínios, políticas, obrigações recorrentes e teto de orçamento.
*   **Inquiry (Indagação):** Uma questão epistemológica ou problema formulado pelo sistema que precisa ser investigado ou resolvido.
*   **Operation (Operação):** A unidade de execução de trabalho (uma tentativa de raciocínio ou ação) gerada a partir de uma *inquiry*. Possui ciclo de vida de estados (`RUNNING`, `PAUSED`, `STOPPED`, `FAILED`, `VALIDATED`, `APPLIED`).
*   **Commit:** Registro imutável gravado no banco de dados canônico que formaliza a aceitação de uma alteração no grafo de conhecimento.
*   **ChangeSet:** O conjunto de mutações propostas por uma operação e submetidas para validação/aceite.
*   **Claim (Alegação):** Uma proposição de conhecimento que pode ser corroborada ou contradita por evidências.
*   **Evidence (Evidência):** Dados ou apontamentos que ligam observações e fontes a uma alegação.
*   **Observation (Observação):** Uma declaração extraída de uma fonte com rastreabilidade de procedência.
*   **Source (Fonte):** A origem de um dado lido pelo sistema (ex.: arquivo local, documento HTTP ou fixture).
*   **Artifact (Artefato):** Produto derivado persistido gerado por operações ou rotinas de auditoria.
*   **Frontier / WorkOpportunity (Fronteira):** O reservatório de oportunidades de trabalho descobertas e pendentes de execução.
*   **Horizon (Horizonte):** Componente do runtime que gerencia a quantidade ideal de candidatos a tarefas prontas (`ready_count`) para manter o motor alimentado sem sobrecarregar a memória.
*   **Agenda:** Componente responsável pelo despacho e execução das tarefas que atingiram o estado pronto (*ready*).
*   **Draft (Rascunho):** Uma proposta de alteração de configuração versionada que exige validação previa e aplicação formal antes de surtir efeito.

---

## 5. Troubleshooting Comum

### 1. A missão não carrega ou o Overview mostra "carregue uma missão"
*   **Causa:** O campo `mission_id` no bloco de **Contexto** está vazio ou contém um identificador inexistente no armazenamento ativo.
*   **Solução:** Digite o ID correto da missão no campo `mission_id` e clique no botão `Atualizar`. Se o runtime foi iniciado com `-mission-id=SUA_MISSAO`, o campo já virá preenchido.

### 2. Linha do tempo SSE não conecta ou exibe "SSE protocol error" / "SSE cursor ahead"
*   **Causa:**
    *   **SSE protocol error:** Ocorreu uma incompatibilidade no *handshake* ou um *frame* fora do padrão (ex.: payload > 512 KB ou sem `schema_version: 1`).
    *   **SSE cursor ahead:** O valor informado em `after_sequence` é maior do que o maior evento já registrado no servidor (*high-water mark*).
*   **Solução:**
    *   Se o erro for *cursor ahead*, redefina o campo `after_sequence` para `0` ou para o valor sugerido na mensagem e clique em `Conectar timeline`.
    *   Se os erros de reconexão esgotarem as 6 tentativas (*SSE retry exhausted*), clique manualmente em `Conectar timeline` para reiniciar a contagem.

### 3. Não consiga salvar a Chave de API do Provedor ("Cofre bloqueado" ou "Ainda não inicializado")
*   **Causa:** O Cofre de Credenciais local está bloqueado ou ainda não foi configurado com uma senha mestra.
*   **Solução:**
    1.  Vá para a área **Provedores e modelos** (`models`).
    2.  No painel **Cofre de credenciais**, informe uma senha mestra (mínimo 12 caracteres) no campo `Senha mestra`.
    3.  Clique em `Criar ou desbloquear`.
    4.  Após a mensagem "Cofre desbloqueado", o campo `Chave da API` no formulário de provedores estará habilitado para escrita.

### 4. Criação ou Aplicação de Draft bloqueada ("validation blocked")
*   **Causa:** O payload JSON enviado não respeita a estrutura do escopo ou contém divergências de parâmetros (ex.: `max_output_tokens` maior ou igual a `context_tokens`).
*   **Solução:**
    *   Clique em `Validate` no card do draft para ver o resumo do erro em `cfgDetail`.
    *   Corrija o payload JSON no formulário e crie um novo rascunho com o parâmetro ajustado.
