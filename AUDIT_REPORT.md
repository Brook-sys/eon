# Auditoria do Subsistema de Modelos/Provedores

## Resumo Executivo

O subsistema de modelos/provedores apresenta **4 bugs críticos** que bloqueiam
totalmente a operação via dashboard. O backend (domínio, kernel, control API)
está corretamente implementado; os bugs estão concentrados no **frontend
(dashboard views)** e na **integração de credenciais entre vault e runtime**.

## Bugs Identificados

### BUG #1 — `max_output_dialect: "legacy"` é inválido (CRÍTICO)

**Localização**: `internal/dashboard/views/models.templ`
- `cleanBinding()` linha ~507: `max_output_dialect: b.max_output_dialect || "legacy"`
- `submitBindingForm()` linha ~580: `max_output_dialect: "legacy"` (hardcoded)

**Problema**: O domínio define `MaxOutputDialectLegacy = "max_tokens"` e
`MaxOutputDialectCompletion = "max_completion_tokens"`. O valor `"legacy"`
NÃO é válido e é rejeitado por `ModelBindingConfig.Validate()`.

**Sintoma**: Toda tentativa de criar ou editar bindings via dashboard falha
na validação do draft com erro `unsupported max output dialect "legacy"`.

**Correção**: Substituir `"legacy"` por `"max_tokens"` em ambos os locais.

### BUG #2 — `refresh()` acessa `rev.payload` que não existe (CRÍTICO)

**Localização**: `internal/dashboard/views/models.templ`, função `refresh()`

**Problema**: A API retorna:
```json
{"schema_version": 1, "revision": {"revision": 1, "models": {...}, ...}}
```

O código frontend faz:
```js
const rev = activeRev?.revision || activeRev;
if (rev && rev.payload) {  // ← rev.payload não existe
    this.cfgPayload = rev.payload.models || rev.payload || ...;
}
```

O campo correto seria `rev.models` (sem `payload`). Resultado: `cfgPayload`
nunca é preenchido a partir da API, ficando sempre `{ providers: [], bindings: [] }`.

**Sintoma**: Dashboard mostra "Nenhum provedor configurado" mesmo quando há
provedores ativos no store.

**Correção**: Acessar `rev.models` diretamente:
```js
if (rev) {
    this.cfgActiveRevision = Number(rev.revision || 0);
    this.cfgPayload = rev.models || { providers: [], bindings: [] };
}
```

### BUG #3 — Secret salvo no vault com nome errado (CRÍTICO)

**Localização**: `internal/dashboard/views/models.templ`, `saveSecretIfProvided()`

**Problema**: O dashboard salva a API key no vault com o nome
`provider/{id}/api-key` (ex: `provider/groq/api-key`). Mas o runtime, ao montar
providers, chama `SecretResolver.Resolve(apiKeyEnv)` onde `apiKeyEnv` é o mesmo
valor do campo `api_key_env` do provider config (ex: `GROQ_API_KEY`).

O `Resolve("GROQ_API_KEY")` procura um secret chamado `GROQ_API_KEY` no vault,
mas o dashboard salvou como `provider/groq/api-key`.

**Sintoma**: Mesmo após salvar a API key no vault via dashboard, o runtime não
encontra a credencial. Log: `resolve model credential: file does not exist`.

**Correção**: Salvar o secret no vault usando `apiKeyEnv` como nome:
```js
await fetch('/dash/api/vault/secrets/' + encodeURIComponent(p.api_key_env), { ... });
```

### BUG #4 — `DefaultApplicabilityForScope(MODELS)` retorna `RESTART_REQUIRED` (DESIGN)

**Localização**: `internal/domain/config.go`, `DefaultApplicabilityForScope()`

**Problema**: O comentário diz "no atomic in-process catalog reload yet" mas
`runtime_reload.go` implementa justamente isso: `reloadModelExecutorIfNeeded`
reconstrói o executor atomicamente quando a versão muda. A aplicability
`RESTART_REQUIRED` está desatualizada em relação ao runtime.

**Impacto**: O draft é validado e aplicado (não é bloqueado), mas o preview
marca `RestartRequired = true`, que pode confundir o operador. O runtime
ainda recarrega corretamente no próximo ciclo. Não é bloqueante mas é
incorreto.

**Correção**: Mudar para `ConfigNextCycle` para refletir que o hot-reload
já funciona via `reloadModelExecutorIfNeeded`.

## Componentes Auditados (sem bugs)

- ✉ `internal/domain/model_binding.go` — validação correta
- ✉ `internal/domain/provider_profile.go` — perfis e dialects corretos
- ✉ `internal/domain/config.go` — diff/apply/preview funcionais
- ✉ `internal/kernel/config_apply.go` — apply/validate/rollback corretos
- ✉ `internal/kernel/config_resolve.go` — `ActiveModelsConfig` funcional
- ✉ `internal/control/httpapi.go` — handlers de draft/validate/apply corretos
- ✉ `internal/provider/openai/provider.go` — adapter OpenAI-compat correto
- ✉ `internal/runtime/bootstrap/model.go` — `BuildModelExecutor` correto
- ✉ `internal/runtime/bootstrap/runtime_reload.go` — hot-reload funcional
- ✉ `internal/inspect/provider_profile.go` — Projector/Discovery corretos
