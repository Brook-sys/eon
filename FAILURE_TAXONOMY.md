# Taxonomia de Falhas do Runtime Epistemológico

Status: baseline v0.1

Esta taxonomia classifica falhas para decisão determinística, persistência e observabilidade. Ela não substitui erros concretos dos adapters: normaliza-os na fronteira do domínio.

## 1. Princípios

1. Falha, estado operacional e ação de recuperação são conceitos distintos.
2. O tipo da falha MUST ser estável e legível por máquina; a mensagem humana MUST NOT controlar retry ou transição.
3. A mesma causa técnica MAY exigir respostas diferentes conforme operação, tentativa, budget, certeza de efeito e política.
4. Falha desconhecida MUST falhar fechada: nenhum efeito adicional é autorizado até classificação ou reconciliação.
5. Retry MUST ser limitado, persistido e condicionado à segurança de repetição.
6. Uma falha tratada internamente que permita concluir a operação não torna a operação final malsucedida; ainda assim, tentativas e recuperações relevantes permanecem auditáveis.
7. Conteúdo de erro é dado não confiável e MUST passar por redaction antes de eventos, métricas ou artifacts.

## 2. Registro normalizado

Cada tentativa malsucedida MUST produzir um `FailureRecord` imutável:

```json
{
  "failure_id": "failure_...",
  "code": "DEPENDENCY_RATE_LIMITED",
  "class": "DEPENDENCY",
  "locus": "MODEL_PROVIDER",
  "retry_disposition": "RETRY_AFTER",
  "effect_state": "NOT_APPLIED",
  "scope": "ATTEMPT",
  "operation_id": "operation_...",
  "attempt": 2,
  "occurred_at": "...",
  "retry_at": "...",
  "safe_detail": "provider rejected the request by quota policy",
  "cause_ref": "artifact_redacted_...",
  "evidence_receipt_refs": ["receipt_..."],
  "policy_version": "failure-policy@1"
}
```

Campos normativos:

- `code`: identificador estável da causa operacional normalizada;
- `class`: família usada para políticas e métricas;
- `locus`: fronteira que detectou a falha;
- `retry_disposition`: resposta permitida, não ordem automática para executá-la;
- `effect_state`: conhecimento sobre eventual efeito lógico ou externo;
- `scope`: menor unidade cujo progresso foi interrompido;
- correlação, tentativa, instante, política e recibos;
- detalhe seguro e referência separada ao material diagnóstico redigido.

O registro SHOULD preservar a cadeia causal sem depender de strings para classificação. Adapters HTTP MAY reter `status`, `Retry-After` e um problem type RFC 9457 como metadados, mas esses campos não substituem `code` nem a política do domínio.

## 3. Eixos de classificação

### 3.1 Classe

| Classe | Significado | Exemplos |
|---|---|---|
| `VALIDATION` | entrada, saída ou schema não satisfaz contrato | missão inválida, resposta do modelo truncada |
| `AUTHORITY` | política, permissão ou escopo proíbe a ação | capability não autorizada, aprovação ausente |
| `RESOURCE` | budget ou capacidade local não permite continuar agora | agenda cheia, budget de retry esgotado |
| `DEPENDENCY` | serviço ou recurso externo não cumpriu o contrato | timeout HTTP, 429, 5xx, arquivo indisponível |
| `CONFLICT` | precondição, versão-base ou concorrência tornou a proposta obsoleta | stale changeset, lease perdido |
| `INTEGRITY` | dado persistido ou recibo viola integridade | checksum divergente, schema incompatível |
| `SECURITY` | entrada ou comportamento ameaça limites de confiança | tipo de conteúdo divergente, segredo detectado |
| `PROGRESS` | execução válida não produz avanço admissível | repetição, ausência de delta, ciclo de replanejamento |
| `INTERNAL` | bug ou violação de invariante no runtime | transição impossível, panic recuperado |

### 3.2 Locus

Valores iniciais: `MISSION`, `AGENDA`, `KERNEL`, `POLICY`, `MODEL_PROVIDER`, `SOURCE`, `CAPABILITY`, `VALIDATOR`, `KNOWLEDGE_STORE`, `EVENT_LOG`, `OUTBOX`, `ARTIFACT_STORE` e `OBSERVER`.

### 3.3 Disposição de recuperação

| Disposição | Semântica |
|---|---|
| `NO_RETRY` | repetir a mesma tentativa não é permitido |
| `RETRY_NOW` | uma nova tentativa equivalente pode ser elegível dentro do budget |
| `RETRY_AFTER` | persistir `not_before` derivado de política ou sinal confiável |
| `REPLAN` | mudar estratégia, formato, provider ou decomposição antes de tentar novamente |
| `RECONCILE` | descobrir o estado real antes de repetir qualquer efeito |
| `REQUIRE_APPROVAL` | esperar decisão explícita do operador |
| `QUARANTINE` | isolar entrada/estado para inspeção sem contaminar o canônico |
| `PAUSE_MISSION` | impedir novo progresso da missão até intervenção ou reparo |

### 3.4 Certeza do efeito

- `NOT_STARTED`: capability não foi invocada;
- `NOT_APPLIED`: há evidência de que nenhum efeito ocorreu;
- `APPLIED`: há recibo de que o efeito lógico ocorreu;
- `UNKNOWN`: timeout, crash ou resposta ambígua impede saber;
- `PARTIAL`: sistema externo admite efeito parcial e exige compensação ou inspeção.

`UNKNOWN` e `PARTIAL` MUST NOT ser convertidos diretamente em retry de efeito. Exigem `RECONCILE`, salvo se a mesma `IdempotencyKey` possuir garantia documentada no destino.

### 3.5 Escopo

- `ATTEMPT`: somente a tentativa atual falhou;
- `OPERATION`: a estratégia da operação não pode continuar;
- `INQUIRY`: a investigação requer reformulação ou encerrou budget;
- `MISSION`: não há progresso seguro sob a revisão ativa;
- `RUNTIME`: integridade global ou armazenamento impede operação segura.

## 4. Códigos mínimos

Códigos iniciais e resposta padrão, sempre subordinados à política versionada:

| Código | Classe | Disposição padrão |
|---|---|---|
| `INPUT_INVALID` | `VALIDATION` | `NO_RETRY` |
| `MODEL_OUTPUT_INVALID` | `VALIDATION` | `REPLAN` |
| `CAPABILITY_DENIED` | `AUTHORITY` | `NO_RETRY` |
| `APPROVAL_REQUIRED` | `AUTHORITY` | `REQUIRE_APPROVAL` |
| `BUDGET_EXHAUSTED` | `RESOURCE` | `NO_RETRY` |
| `CAPACITY_THROTTLED` | `RESOURCE` | `RETRY_AFTER` |
| `DEPENDENCY_RATE_LIMITED` | `DEPENDENCY` | `RETRY_AFTER` |
| `DEPENDENCY_TRANSIENT` | `DEPENDENCY` | `RETRY_AFTER` |
| `DEPENDENCY_PERMANENT` | `DEPENDENCY` | `NO_RETRY` |
| `EFFECT_UNKNOWN` | `DEPENDENCY` | `RECONCILE` |
| `BASE_VERSION_STALE` | `CONFLICT` | `REPLAN` |
| `LEASE_LOST` | `CONFLICT` | `RECONCILE` |
| `DUPLICATE_INTENT` | `CONFLICT` | `RECONCILE` |
| `SCHEMA_INCOMPATIBLE` | `INTEGRITY` | `PAUSE_MISSION` |
| `STATE_CORRUPT` | `INTEGRITY` | `PAUSE_MISSION` |
| `UNTRUSTED_CONTENT_REJECTED` | `SECURITY` | `QUARANTINE` |
| `SECRET_EXPOSURE_BLOCKED` | `SECURITY` | `QUARANTINE` |
| `NO_EPISTEMIC_DELTA` | `PROGRESS` | `REPLAN` |
| `REPETITION_DETECTED` | `PROGRESS` | `REPLAN` |
| `INVARIANT_VIOLATION` | `INTERNAL` | `PAUSE_MISSION` |
| `UNCLASSIFIED_FAILURE` | `INTERNAL` | `PAUSE_MISSION` |

Novos códigos MUST documentar classe, locus possível, disposição padrão, certeza de efeito esperada e testes. Códigos removidos MUST permanecer traduzíveis durante a janela de compatibilidade dos eventos persistidos.

## 5. Regras de transição

- `RETRY_NOW` e `RETRY_AFTER` consomem budget de tentativa e MUST terminar em estado persistido elegível, nunca em espera dentro do worker.
- `REPLAN` normalmente leva a `REPLANNING`; se o budget de estratégia acabou, leva a `EXHAUSTED`.
- `REQUIRE_APPROVAL` leva a `WAITING_APPROVAL` com pedido e escopo persistidos.
- `RECONCILE` cria ou agenda operação determinística de reconciliação antes de liberar a intenção original.
- `QUARANTINE` preserva hash, proveniência e razão, mas MUST impedir promoção ao `KnowledgeState`.
- falha não recuperável da operação leva a `FAILED` somente quando a política determina que nova estratégia, espera ou intervenção não são aplicáveis;
- cancelamento solicitado não é falha e deve usar `CANCELLED` com evento próprio.

## 6. Observabilidade e testes

Métricas e traces SHOULD usar `code` ou classe de cardinalidade limitada, nunca mensagens livres. A classificação de erro é contextual: por exemplo, `404` pode ser resultado esperado de uma sondagem ou `DEPENDENCY_PERMANENT` quando a operação exigia o recurso.

A suite mínima MUST cobrir:

1. mapeamento de erros de adapters para códigos do domínio;
2. 429 com e sem `Retry-After` usando relógio virtual;
3. timeout antes do envio e timeout com efeito desconhecido;
4. resposta inválida do modelo até esgotar reparo/retry;
5. conflito de versão-base sem mutação parcial;
6. lease perdido e reconciliação;
7. corrupção/incompatibilidade que pausa com falha fechada;
8. redaction de segredo e limitação de cardinalidade;
9. código desconhecido convertido em `UNCLASSIFIED_FAILURE`.

## 7. Referências de desenho

- Temporal Failures: separa tipos de falha, retryability e histórico durável: <https://docs.temporal.io/references/failures>
- OpenTelemetry — Recording errors: recomenda classificação contextual e `error.type` consistente: <https://opentelemetry.io/docs/specs/semconv/general/recording-errors/>
- RFC 9457 — Problem Details for HTTP APIs: formato interoperável para detalhes de erro HTTP: <https://www.rfc-editor.org/rfc/rfc9457>

Essas referências orientam interoperabilidade; a semântica oficial de recuperação pertence à política versionada deste runtime.
