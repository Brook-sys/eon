# Integração Groq e NVIDIA NIM

Status: plano aceito para implementação incremental  
Data: 2026-07-17

## Objetivo

Tratar Groq e NVIDIA NIM como provedores OpenAI-compatible de primeira classe, sem acoplar o kernel a APIs específicas. A integração deve maximizar o uso seguro dos tiers gratuitos, respeitar limites por modelo/contexto, permitir preferência e fallback explícitos e manter toda saída de modelo sob `PROPOSE_ONLY`.

## Estado atual verificado

O runtime já possui a base correta:

- adapter desacoplado `internal/provider/openai`, endpoint Chat Completions e contrato mínimo texto→texto;
- seleção explícita de `max_tokens` ou `max_completion_tokens`;
- contexto declarado usado no compilador de prompt;
- `ProviderProfile` conservador, probe orçamentado e adaptação reversível;
- provider primário + um fallback;
- `ResourceGate` persistente com concorrência, RPM, RPD, TPM, circuito e cooldown;
- configuração versionada e dashboard genérico de drafts.

Isso permite uma chamada simples aos dois serviços, mas **ainda não satisfaz** o requisito operacional completo.

## Lacunas e riscos encontrados

1. **Identidade de recurso excessivamente global.** `model.complete` usa hoje `model:default`. Assim, chamadas de modelos diferentes compartilham o mesmo bucket e o runtime não aproveita limites independentes da Groq.
2. **Apenas dois bindings.** Primário + fallback único não representam uma lista ordenada de modelos, preferências por tipo de operação, disponibilidade e circuitos independentes.
3. **Limites não configuráveis no binding.** Os limites de modelo vêm de `DefaultMVPLimits`, não do dashboard/config do provider.
4. **Timeout HTTP não é explícito.** O adapter usa `http.DefaultClient`; a lease não substitui deadline de rede. Um endpoint lento pode prender a chamada até o contexto externo terminar.
5. **`Retry-After` não chega ao ResourceGate.** O adapter classifica HTTP 429 como retryable, porém descarta `Retry-After` e os headers de quota. O executor reporta falha com `nil`, desperdiçando a informação mais precisa.
6. **Reserva subcontabiliza recuperação.** O gate é adquirido uma vez por operação, enquanto correção, simplificação e fallback podem efetuar várias chamadas. Rate limit deve ser reservado/reportado por tentativa e pelo binding realmente usado.
7. **TPM usa estimativa, não reconcilia usage real.** Isso é conservador somente se a estimativa for boa; respostas e reasoning tokens podem elevar consumo real.
8. **NVIDIA NIM não pode ser modelado só como 40 RPM.** Bloqueios antecipados correlacionados a contexto exigem simultaneamente RPM conservador, TPM/bytes/context budget, concorrência baixa, feedback de 429 e cooldown adaptativo.
9. **Compatibilidade OpenAI não é uniformidade de modelo.** Modelos NIM podem divergir em roles, ferramentas, JSON, streaming, parâmetros de reasoning e campos extras. O baseline precisa continuar texto→texto, sem enviar opções não confirmadas.
10. **Dashboard atual é genérico.** Ele não oferece editor específico de bindings/modelos, ordem de preferência, timeout, limites, circuit breaker, contexto e estado observado por modelo.
11. **Catálogo de modelos é mutável.** IDs e disponibilidade podem mudar. Não se deve hardcodear uma enumeração fechada nem prometer suporte universal.

## Contrato proposto

### 1. Provider e binding são entidades diferentes

Um provider descreve transporte/autenticação:

- `kind`: `groq`, `nvidia_nim` ou `openai_compatible`;
- `base_url`;
- `api_key_env` (somente referência, nunca segredo);
- defaults de timeout, resposta máxima e política de retry.

Um binding descreve um modelo executável:

- `id` estável local;
- `provider_ref`;
- `model_id` opaco enviado à API;
- `enabled`;
- `priority`/ordem de preferência;
- classes de operação preferidas;
- `context_tokens`, `max_output_tokens` e margem de contexto;
- dialect e quirks confirmados;
- `ResourceLimit` próprio;
- timeout total e, futuramente, timeout de primeira resposta/stream idle;
- estado de saúde/circuito observado.

IDs de modelo ficam abertos; modelos recentes podem ser adicionados via dashboard sem release de código, desde que permaneçam no baseline texto→texto ou tenham profile confirmado.

### 2. Chave de rate limit

Cada tentativa deve adquirir dois recursos em ordem determinística:

- `model-provider:<provider>` para limites globais da conta/API;
- `model-binding:<binding-id>` para limites por modelo.

Para Groq, o bucket por binding/modelo é obrigatório e deve absorver os headers oficiais de request/token quota. Para NIM hospedado, o bucket global é obrigatório e o bucket por modelo permite aprender isolamento/pressão sem assumir que todos os modelos compartilham exatamente o mesmo backend.

### 3. Política inicial conservadora

Os valores são defaults editáveis, não alegações de quota contratual.

**Groq**

- obter limites reais do dashboard/headers da conta sempre que disponíveis;
- não codificar um único RPM para todos os modelos;
- `MaxConcurrent` inicial 1 por binding, aumentando por configuração/evidência;
- respeitar `Retry-After` e headers `x-ratelimit-*`;
- circuitos independentes permitem usar outro modelo quando somente um bucket saturou.

**NVIDIA NIM hospedado**

- teto configurado inicial de 40 RPM deve ser tratado como máximo anunciado, não garantia;
- default operacional sugerido: 1 chamada concorrente, 20–30 RPM, margem de contexto 25–35%, cooldown no primeiro 429 e backoff exponencial limitado;
- habilitar TPM estimado e/ou bytes de prompt quando a quota exata não for publicada;
- reduzir temporariamente o contexto preferido após 429 sem `Retry-After`, em degraus monotônicos e reversíveis;
- não alternar imediatamente vários modelos do mesmo provider após 429 global: o provider gate precisa impedir tempestade de fallback.

### 4. Seleção e fallback

O roteador recebe uma lista ordenada de bindings elegíveis e escolhe o primeiro que:

1. esteja habilitado;
2. possua profile suficiente para a operação;
3. comporte prompt + saída no context budget;
4. tenha ResourceGate disponível;
5. não esteja em circuito/cooldown.

A seleção deve ser persistida em evento com motivo e sem segredo. Fallback não aumenta autoridade nem orçamento: cada tentativa consome `Budget.ModelCalls`. Erros 400 de incompatibilidade demovem feature/profile; 401/403 desabilitam o binding até intervenção; 404/model-not-found marca indisponibilidade; 429 aplica quota/cooldown; 5xx/transporte permitem próximo binding conforme budget.

### 5. Compatibilidade de modelos

Baseline comum obrigatório:

- `POST /v1/chat/completions`;
- uma mensagem `user` com texto;
- resposta não-streaming;
- nenhum tool, seed, JSON schema ou parâmetro específico;
- limite de saída explicitamente configurado;
- resposta e erro com bytes limitados.

Capacidades adicionais só são usadas por profile declarado/probado/override. Campos específicos de reasoning devem ficar em uma extensão allowlisted por provider/modelo, nunca em mapa JSON arbitrário vindo do modelo.

Os nomes citados pelo operador devem ser tratados como intenções de catálogo. O `model_id` exato precisa ser validado contra a API/documentação no momento de configurar, pois nomes comerciais e versões mudam. O sistema não deve rejeitar um ID novo só por não estar compilado numa lista local.

## Dashboard

Adicionar uma seção **Modelos / Provedores** sobre configuração versionada, com:

- cards de provider (Groq, NVIDIA NIM, custom);
- bindings editáveis e reordenáveis;
- toggle enabled e preferência;
- modelo/ID, context/output, dialect, timeouts e max response bytes;
- limites globais e por modelo: concorrência, RPM, RPD, TPM, failure threshold, cooldown base/max;
- política de margem/redução de contexto;
- validação e diff antes de aplicar;
- teste/probe explícito com orçamento e aviso de que consome quota;
- projeção read-only de usage, headers observados, próxima disponibilidade, falhas, circuito, último status e seleção recente;
- API key apenas como nome de variável/secret ref; nunca campo de valor.

Aplicabilidade: mudanças de preferência/limite podem ser `NEXT_OPERATION`; base URL, credencial, client timeout e criação/remoção de provider provavelmente exigem `RESTART`. O receipt deve dizer qual caso ocorreu.

## Fases de implementação

### P0 — correção de transporte e quota

- [x] enriquecer erro OpenAI-compatible com `RetryAfter` padrão parseado sem corpo de erro;
- [x] configurar timeout HTTP explícito no adapter (default limitado; override por config);
- [x] propagar `RetryAfter` e delay explícito ao gate/circuit breaker de recursos (FR-RES-001);
- [ ] adicionar metadados de rate limit allowlisted/redigidos ao log (opcional);
- testes de 429, `Retry-After` delta/data, headers inválidos, timeout e não vazamento.

### P1 — bindings e rate limit por modelo

- [x] introduzir contrato de IDs/provider kinds e bindings múltiplos sem segredo;
- [x] ligar o catálogo `MODELS` ativo ao bootstrap/config store, projetando primário e fallback habilitados por prioridade/ID;
- [x] usar resource keys compostas e tipadas (`model-provider:<provider>` + `model-binding:<binding>`);
- [x] adquirir/reportar ambos os gates por tentativa, incluindo correção, simplificação e fallback, sem dupla contagem do budget de `ModelCalls`;
- [x] fazer preflight dos gates antes da lease e reutilizar a reserva na primeira tentativa, evitando operação `RUNNING` apenas por throttle conhecido;
- [x] reconciliar tokens observados de `usage` contra a reserva estimada, sem refund inseguro de calls já consumidas;
- [x] ampliar testes de relógio virtual para demonstrar explicitamente: Groq modelo A saturado não bloqueia B; 429 global NIM bloqueia todos os bindings NIM; fallback respeita budget e gates do binding efetivamente usado.
  - Evidência: `TestGroqBindingQuotaIsolation`, `TestNIMProviderRetryAfterBlocksAllBindings` e `TestModelExecutorFallbackProviderSucceeds`; o preflight agora mantém os permits durante a primeira chamada real e os libera nos caminhos sem efeito antes de `Complete`.

### P2 — roteamento e adaptação de contexto

- [x] núcleo puro de roteamento ordenado por preferência/contexto/saúde, com razões auditáveis e hidratação de `ResourceUsage`;
- [x] integrar o roteador ao executor multi-binding além do primário + fallback;
- [x] taxonomia de falhas HTTP por binding;
- [x] redução reversível de contexto para pressão NIM: 400 classificado como request inválido ativa teto de -25% por nível, até -75%, com retry sob o mesmo budget; o sinal é persistido por binding, sobrevive a restart e recupera um nível após dois sucessos, sem alterar o perfil declarado;
  - Evidência: `ContextPressureState`, `ReductionForPressure`, recompilação no `ModelExecutor` e eventos `operation.model_adaptation` com `reason=context_pressure_reduction`.
  - Limite deliberado: a recuperação gradual está definida pela política pura, mas o estado ainda não persiste entre dispatches; só promover persistência se avaliação live comprovar pressão recorrente entre operações.
- [x] eventos/inspect de decisão.

### P3 — configuração e dashboard

- [x] novo escopo versionado `MODELS`, com providers/bindings sem segredos;
- [x] API/config draft, validação, diff, apply/rollback;
- [x] UI de configuração e projeções operacionais: o dashboard lista `MODELS`, oferece template seguro desabilitado, usa somente `api_key_env` e reaproveita preview/diff/histórico/rollback genéricos;
- [x] smoke E2E do wiring com catálogo Groq/NIM fake: testes de bootstrap montam múltiplos bindings/adapters e limites compostos sem acessar rede.

O template da UI é deliberadamente não executável até o operador substituir o
`model_id` opaco e marcar o binding como habilitado. Quotas no template são
limites conservadores locais, não afirmações sobre quotas atuais do provedor.

### P4 — catálogo e avaliação live

- descoberta opcional de `/v1/models` com allowlist/validação e cache, nunca como autoridade automática;
- presets versionados para modelos prioritários confirmados;
- matriz cognitiva live por binding/modelo/contexto/formato;
- registrar resultados e ajustar preferência com decisão explícita do operador.

## Critérios de aceite

- dois modelos Groq com limites independentes podem ser usados sem bloquear um ao outro;
- um 429 NIM com `Retry-After` bloqueia novas tentativas do provider até o instante persistido;
- um 429 NIM sem header abre cooldown e reduz contexto de forma limitada/reversível;
- timeout de rede encerra a tentativa e libera slot;
- correções e fallbacks consomem chamadas e quota individualmente;
- ordem de modelos, timeout e todos os limites relevantes são editáveis no dashboard com diff/rollback;
- modelo novo pode ser configurado por ID opaco sem alteração de código;
- feature não confirmada nunca é enviada;
- API keys e corpos de erro não aparecem em config, eventos, inspect ou dashboard.

## Fontes primárias consultadas

- Groq OpenAI compatibility: https://console.groq.com/docs/openai
- Groq API reference: https://console.groq.com/docs/api-reference
- Groq rate limits: https://console.groq.com/docs/rate-limits
- NVIDIA NIM LLM APIs: https://docs.api.nvidia.com/nim/reference/llm-apis
- NVIDIA hosted Chat Completions endpoint observado nas páginas de modelo: `https://integrate.api.nvidia.com/v1/chat/completions`

As quotas e o catálogo são dados mutáveis; presets devem registrar data/fonte e nunca substituir headers/erros observados ou configuração do operador.

## P2 — roteamento e escopo de falha (2026-07-17)

- o roteador considera circuitos duráveis de provider e binding antes da seleção;
- HTTP 429 de `nvidia_nim` é classificado como cooldown provider-wide; Groq permanece binding-wide;
- reporte composto libera como sucesso o permit fora do escopo da falha, evitando contaminação cruzada de circuitos;
- eventos de policy de falha registram `provider_id` e `binding_id` validados pela configuração (sem URL, chave ou corpo de erro);
- o executor legado continua válido: kind vazio mantém a classificação conservadora binding-wide.
