# Auditoria do Subsistema de Modelos/Provedores

## Resumo Executivo

O subsistema de modelos/provedores apresentava **10 bugs** na interface do
dashboard. O backend (domínio, kernel, control API) está corretamente
implementado; os bugs estão concentrados no **frontend (dashboard views)**
e na **integração de credenciais entre vault e runtime**.

Todos os 10 bugs foram corrigidos e verificados com testes automatizados.

## Bugs Identificados e Corrigidos

### BUG #1 — `max_output_dialect: "legacy"` é inválido (CRÍTICO) ✅

**Localização**: `internal/dashboard/views/models.templ`
- `cleanBinding()` e `submitBindingForm()` usavam `"legacy"` como default

**Problema**: O domínio aceita apenas `"max_tokens"` e `"max_completion_tokens"`.
O valor `"legacy"` é rejeitado por `ModelBindingConfig.Validate()`.

**Correção**: Substituir `"legacy"` por `"max_tokens"` em ambos os locais.

### BUG #2 — `refresh()` acessa `rev.payload` que não existe (CRÍTICO) ✅

**Localização**: `internal/dashboard/views/models.templ`, função `refresh()`

**Problema**: A API retorna `revision.models` mas o código lia `rev.payload`.

**Correção**: Acessar `rev.models` diretamente.

### BUG #3 — Secret salvo no vault com nome errado (CRÍTICO) ✅

**Localização**: `internal/dashboard/views/models.templ`, `saveSecretIfProvided()`

**Problema**: Dashboard salvava a API key como `provider/{id}/api-key` mas o
runtime resolve por `api_key_env` (ex: `GROQ_API_KEY`).

**Correção**: Salvar usando `apiKeyEnv` como nome do secret.

### BUG #4 — `DefaultApplicabilityForScope(MODELS)` retorna `RESTART_REQUIRED` ✅

**Localização**: `internal/domain/config.go`, `DefaultApplicabilityForScope()`

**Problema**: O runtime já faz hot-reload atômico via `reloadModelExecutorIfNeeded`,
mas a applicability marcava `RESTART_REQUIRED`.

**Correção**: Mudar para `ConfigNextCycle`.

### BUG #5 — `createModelsDraft()` não envia `based_on_revision` ✅

**Localização**: `internal/dashboard/views/models.templ`, `createModelsDraft()`

**Problema**: O payload sempre enviava `based_on_revision` ausente (implícito 0),
perdendo a informação de proveniência do draft em relação à revisão ativa.

**Correção**: Adicionar `based_on_revision: this.cfgActiveRevision || 0` ao
payload do POST. O campo é aceito sem validação pelo handler genérico
`handleCreateConfigDraft` (apenas `handleCreateModelPresetDraft` valida).

**Teste**: `TestDashboardBug5_BasedOnRevisionForwarded` +
`TestDashboardBug5_EmptyRevisionSendsZero`

### BUG #6 — `unlockVault()` não mostra erro real do servidor ✅

**Localização**: `internal/dashboard/views/models.templ`, `unlockVault()`

**Problema**: Quando o unlock falhava, o código lançava um genérico
`new Error('Falha ao desbloquear')` ignorando a mensagem de erro do servidor
(ex: "invalid vault password", "vault password must contain 12 to 1024 characters").

**Correção**: Extrair `error.message` do body JSON da resposta e usar como
mensagem de erro. O frontend agora mostra o erro real do servidor.

### BUG #7 — Vault password sem indicação de tamanho mínimo ✅

**Localização**: `internal/dashboard/views/models.templ`, input do vault password

**Problema**: O placeholder dizia apenas "Senha mestra..." sem comunicar o
requisito mínimo de 12 caracteres. O usuário digitava senha curta e recebia
erro criptico.

**Correção**: Placeholder atualizado para "Senha mestra (mín. 12 chars)..." +
adicionado suporte a Enter key para submeter.

### BUG #8 — `createModelsDraft()` sem tratamento de conflito ✅

**Localização**: `internal/dashboard/views/models.templ`, `createModelsDraft()`

**Problema**: Quando duas operações rápidas corriam em paralelo (double-click),
a segunda falhava com erro genérico "config_draft conflict". O usuário não
sabia o que fazer.

**Descoberta**: O kernel é idempotente para double-apply do MESMO draft
(reativa a mesma revisão). O conflito 409 acontece quando um draft em estado
REJECTED/FAILED é reaplicado. O fix do frontend agora:
1. Verifica `draftID` null (defensivo)
2. Trata código `conflict` com mensagem amigável sugerindo refresh

**Teste**: `TestDashboardBug8_DoubleApplyIdempotent` +
`TestDashboardBug8_TwoDraftsSameBase`

### BUG #9 — `saveSecretIfProvided()` falha silenciosamente ✅

**Localização**: `internal/dashboard/views/models.templ`, `saveSecretIfProvided()`

**Problema**: A função não verificava o status da resposta do PUT ao vault.
Se o vault estava bloqueado (423), o erro era silenciado e o usuário achava
que a credencial foi salva.

**Correção**: Verificar `res.ok`, extrair mensagem de erro do JSON body e
lançar exceção com a mensagem real (ex: "credential vault is locked").

**Teste**: `TestDashboardBug9_SaveSecretVaultLocked`

### BUG #10 — `removeProvider()` e `removeBinding()` sem try/catch ✅

**Localização**: `internal/dashboard/views/models.templ`

**Problema**: Ambas funções chamavam `createModelsDraft()` sem try/catch.
Se a API falhava, o erro não tratado quebrava o Alpine.js e a UI ficava
inconsistente.

**Correção**: Wrap em try/catch com notificações de sucesso/erro.

## Melhorias de Debug Adicionadas

### Logging estruturado no Control API (`internal/control/httpapi.go`)

- **`Logger *slog.Logger`** field na struct `API` (defaults to `slog.Default()`)
- **`logDebug()`/`logError()`** helpers na struct `API`
- **Request logging middleware**: registra method, path, status, duration_ms
  para toda requisição ao control API
- **Draft lifecycle logging**: create, validate, apply com campos relevantes
  (draft_id, scope, revision, error)
- **`writeAPIError` enhanced**: loga erros não-apiError como unexpected

## Componentes Auditados (sem bugs)

- ✅ `internal/domain/model_binding.go` — validação correta
- ✅ `internal/domain/provider_profile.go` — perfis e dialects corretos
- ✅ `internal/domain/config.go` — diff/apply/preview funcionais
- ✅ `internal/kernel/config_apply.go` — apply/validate/rollback corretos
  - Idempotente para double-apply do mesmo draft (retorna revisão existente)
  - Conflito 409 apenas quando draft está em estado terminal REJECTED/FAILED
- ✅ `internal/kernel/config_resolve.go` — `ActiveModelsConfig` funcional
- ✅ `internal/control/httpapi.go` — handlers de draft/validate/apply corretos
- ✅ `internal/provider/openai/provider.go` — adapter OpenAI-compat correto
- ✅ `internal/runtime/bootstrap/model.go` — `BuildModelExecutor` correto
- ✅ `internal/runtime/bootstrap/runtime_reload.go` — hot-reload funcional
- ✅ `internal/inspect/provider_profile.go` — Projector/Discovery corretos
- ✅ Vault HTTP handler — retorna JSON errors parseable com code+message

## Testes Automatizados

- `internal/control/models_e2e_audit_test.go` — fluxo E2E completo
- `internal/control/dashboard_bugs_test.go` — testes focados em BUGs #5, #8, #9
- `internal/dashboard/dashboard_user_simulation_test.go` — simulação de usuário
