# Requisitos do Runtime Epistemológico

Status: baseline v0.2

Os termos normativos em maiúsculas seguem BCP 14, conforme `GLOSSARY.md`. Cada requisito possui um identificador estável. Documentos, código e testes SHOULD referenciar estes IDs em vez de copiar o texto normativo. `INVARIANTS.md` formaliza as propriedades transversais e `FAILURE_TAXONOMY.md` normaliza falhas e recuperação.

## 1. Escopo e autoridade

### FR-AUTH-001 — Missão versionada

O runtime MUST operar sob uma `MissionRevision` ativa e MUST preservar o texto original, a revisão e a proveniência da `MissionSpec` correspondente.

**Evidência de aceitação:** carregar uma missão válida; rejeitar ausência, revisão inválida ou mudança silenciosa.

### FR-AUTH-002 — Autoridade exclusiva do kernel

Somente política e código determinístico do kernel MAY admitir `InquiryCandidate`, autorizar capability, consumir budget oficial, aceitar `ProposedChangeSet` ou produzir `Commit`. Uma saída de modelo MUST NOT executar esses efeitos diretamente.

**Evidência de aceitação:** testes demonstram que saída válida, inválida ou maliciosa do modelo permanece proposta até validação e autorização.

### FR-AUTH-003 — Capabilities delimitadas

Toda interação externa MUST ocorrer por capability tipada, versionada e autorizada. Shell arbitrário, execução de código, publicação, mensagens e alterações fora do armazenamento gerenciado MUST NOT integrar o MVP.

**Evidência de aceitação:** capability desconhecida ou fora do escopo falha fechada antes de qualquer efeito.

### FR-AUTH-004 — Mudança de missão explícita

Uma `UserAmendment` MUST produzir candidata a nova revisão, diff semântico e análise de impacto. A agenda MUST ser reconciliada somente após aceitação da revisão.

**Evidência de aceitação:** teste preserva revisão anterior e identifica itens mantidos, invalidados, cancelados ou repriorizados.

## 2. Investigação e agenda

### FR-AGENDA-001 — Admissão rastreável

Toda `Inquiry` MUST derivar de um `InquiryCandidate` com origem, ganho epistemológico esperado, novidade, custo, risco, condição de resposta, condição de parada e revisão futura quando adiada.

**Evidência de aceitação:** candidato incompleto ou duplicado é rejeitado com razão persistida.

### FR-AGENDA-002 — Operação contratada

Toda `Operation` MUST instanciar uma `OperationSpec` versionada, declarar inputs/read set, budget, formato de saída, validadores, retry/fallback e autoridade máxima.

**Evidência de aceitação:** operação sem spec existente ou incompatível não entra em `READY`.

### FR-AGENDA-003 — Replenishment limitado

Agenda vazia MUST disparar replenishment antes de `Rest`. O replenisher MUST aplicar escopo, deduplicação, budget e limite de admissões por ciclo.

**Evidência de aceitação:** relógio virtual demonstra replenishment limitado seguido por despacho ou repouso, sem crescimento ilimitado.

### FR-AGENDA-004 — Progresso observável

Uma `InquiryCandidate` autogerada MUST declarar mudança observável esperada no `KnowledgeState`. Produção textual, repetição ou reflexão sem delta esperado MUST NOT bastar para admissão.

**Evidência de aceitação:** política rejeita candidato sem ganho, novidade ou condição de parada.

## 3. Estado epistemológico

### FR-KNOW-001 — Proveniência de observações

Toda `Observation` MUST ser ancorada em `SourceFragment` ou `EvidenceReceipt` recuperável. Quando ancorada em fragmento, MUST preservar uma citação exata que confira byte a byte com o fragmento imutável; a declaração interpretativa permanece separada dessa citação. Identidade da fonte MUST NOT ser interpretada como veracidade.

**Evidência de aceitação:** observação sem âncora, com âncora inexistente ou com citação divergente é recusada atomicamente.

### FR-KNOW-006 — Segmentação verificável

Todo `SourceFragment` MUST referenciar uma `SourceVersion`, registrar offsets de bytes e hash do conteúdo exato. A segmentação completa de uma versão MUST ser ordenada, sem lacunas ou sobreposições, preservar fronteiras UTF-8 para texto e permitir round-trip byte a byte do snapshot original.

**Evidência de aceitação:** contract tests rejeitam lacunas, sobreposições, hash divergente e linhagem incorreta; teste com Unicode recompõe exatamente o snapshot.

### FR-KNOW-002 — Claims e evidências separados

`Claim` MUST preservar qualificadores relevantes e MUST NOT incorporar citações como parte da proposição. Apoio, oposição e qualificação MUST ser representados por `EvidenceLink` tipado.

**Evidência de aceitação:** validação rejeita claim sem qualificadores exigidos ou vínculo com tipo desconhecido.

### FR-KNOW-003 — Mudança oficial por changeset

Alterações canônicas MUST ser propostas por `ProposedChangeSet` imutável sobre versão-base, validadas e aceitas antes de tentativa de commit.

**Evidência de aceitação:** escrita direta e changeset com precondição ou base obsoleta são recusados sem mutação parcial.

### FR-KNOW-004 — Commit atômico e versionado

Um `Commit` MUST aplicar todas ou nenhuma das mudanças aceitas e MUST produzir identidade lógica, nova versão e recibo durável. Retry da mesma intenção MUST NOT criar segundo efeito lógico.

**Evidência de aceitação:** falhas injetadas antes, durante e depois do commit resultam em estado anterior ou novo estado completo, nunca intermediário.

### FR-KNOW-005 — Artefatos derivados

`KnowledgeArtifact` MUST registrar commit-base e dependências. Mudança em dependência MUST tornar o artefato afetado detectavelmente obsoleto; artefatos MUST NOT substituir o estado canônico.

**Evidência de aceitação:** alteração de claim identifica somente artefatos dependentes para refresh ou regeneração.

## 4. Modelo e contexto

### FR-MODEL-001 — Contrato universal texto→texto

O runtime MUST funcionar com provider cujo único recurso seja completar texto de entrada com texto de saída. JSON mode, tool calling, streaming e roles são otimizações opcionais.

**Evidência de aceitação:** vertical slice passa contra servidor falso somente texto usando Chat Completions OpenAI-compatible.

### FR-MODEL-002 — Saída validada

Toda saída de modelo MUST ser preservada como artefato bruto, normalizada e validada contra o contrato da operação antes de originar proposta tipada.

**Evidência de aceitação:** corpus de respostas truncadas, extras, desconhecidas e adversariais não causa panic nem efeito oficial.

### FR-MODEL-003 — Contexto sob budget

O prompt MUST ser compilado de template versionado e fatos selecionados, reservar output e margem de segurança e respeitar o menor budget entre operação e provider.

**Evidência de aceitação:** testes de fronteira e fuzz demonstram que compilação não excede budget e falha explicitamente quando conteúdo obrigatório não cabe.

### FR-MODEL-004 — Recuperação limitada

Reparo e retry MUST possuir budget persistido. O runtime SHOULD preferir normalização determinística e correção localizada a reenvio integral; ao esgotar opções seguras, MUST usar fallback, adiar ou solicitar intervenção.

**Evidência de aceitação:** modelo sempre inválido termina em estado explícito sem loop de chamadas.

## 5. Continuidade e execução durável

### FR-DUR-001 — Estado operacional persistido

Toda unidade não terminal MUST possuir `OperationalState` persistido e condição explícita de reavaliação, como prontidão, dependência, `not_before`, evento, aprovação ou lease expirável.

**Evidência de aceitação:** reinício reconstrói filas e esperas sem depender de contexto conversacional.

### FR-DUR-002 — Repouso sem busy loop

Sem trabalho admissível, o runtime MUST persistir `Rest` e próxima condição de reavaliação e MUST bloquear em timer/evento em vez de sondagem contínua.

**Evidência de aceitação:** relógio virtual comprova ausência de novos ciclos antes do próximo instante ou evento.

### FR-DUR-003 — Leases recuperáveis

Execução concorrente MUST usar lease persistido. Lease expirado MUST tornar a operação reconciliável ou novamente elegível sem presumir ausência de efeito externo.

**Evidência de aceitação:** crash de worker e expiração controlada não perdem operação nem duplicam efeito lógico.

### FR-DUR-004 — Entrega e deduplicação

Efeitos externos MUST usar `IdempotencyKey` quando suportado ou reconciliação explícita. Eventos locais a publicar MUST usar outbox confirmada atomicamente com a mudança correspondente.

**Evidência de aceitação:** replay de evento, callback e outbox mantém um único efeito lógico.

### FR-DUR-005 — Tempo determinístico

Deadlines, retries, backoff, leases, cadência e repouso MUST depender de `Clock` injetável. Jitter MUST depender de `RandomSource` injetável.

**Evidência de aceitação:** testes não aguardam relógio real e reproduzem a mesma sequência com fontes controladas.

### FR-DUR-006 — Falha normalizada e recuperação segura

Toda tentativa malsucedida MUST produzir `FailureRecord` tipado com código, classe, locus, disposição de recuperação, certeza do efeito, escopo, correlação e política aplicada. Efeito `UNKNOWN` ou `PARTIAL` MUST ser reconciliado antes de retry, salvo garantia documentada de idempotência no destino.

**Evidência de aceitação:** contract tests mapeiam falhas de adapters, distinguem timeout antes/depois do envio e impedem segundo efeito sob resposta ambígua.

## 6. Recursos, segurança e observabilidade

### FR-RES-001 — Budgets e backpressure

Calls, tokens, bytes, concorrência, retries e duração de ciclo MUST possuir limites explícitos. Saturação MUST produzir espera persistida ou rejeição, não filas ilimitadas nem workers bloqueados.

**Evidência de aceitação:** cenários de 429, cota diária e agenda cheia preservam intenção dentro da política e não excedem limites.

### FR-RES-002 — Aquisição hostil por padrão

Conteúdo de fonte MUST ser tratado como dado não confiável. Aquisição MUST impor limites de bytes, timeout, tipos aceitos e separação entre conteúdo e instruções do prompt.

**Evidência de aceitação:** fixtures com prompt injection, tipo divergente e corpo excessivo não ampliam autoridade nem excedem limites.

### FR-OBS-001 — Eventos auditáveis

Transições, autorizações, rejeições, chamadas de modelo, validações, mudanças e falhas MUST gerar eventos ou recibos correlacionáveis com missão, inquiry, operação e commit quando aplicável.

**Evidência de aceitação:** uma execução completa pode ser explicada sem consultar raciocínio oculto do modelo.

### FR-OBS-002 — Segredos protegidos

Segredos MUST NOT ser persistidos em prompts, respostas de erro, eventos, artifacts ou commits epistemológicos. Configuração MUST referenciá-los indiretamente.

**Evidência de aceitação:** teste de redaction cobre headers e campos sensíveis conhecidos.

## 7. Requisitos não funcionais

### NFR-PORT-001 — Portabilidade

O núcleo MUST ser implementado em Go e SHOULD produzir binários para as plataformas oficialmente suportadas sem exigir runtime de linguagem adicional.

### NFR-MOD-001 — Desacoplamento

Domínio e kernel MUST NOT importar adapters concretos de provider ou persistência. Fronteiras substituíveis MUST possuir contract tests reutilizáveis.

### NFR-TEST-001 — Testabilidade offline

Testes unitários e do kernel MUST executar sem rede, modelo remoto, processo externo ou relógio real. O conjunto aplicável MUST incluir unitários, race detector, vet e fuzz/property tests para parsers e invariantes críticos.

### NFR-REL-001 — Recuperação determinística

Dado o mesmo estado persistido, eventos e fontes injetadas, recuperação e seleção determinísticas MUST produzir a mesma decisão oficial, salvo desempate explicitamente alimentado por `RandomSource` registrado.

### NFR-PERF-001 — Operação limitada

O primeiro vertical slice SHOULD operar serialmente e MUST limitar goroutines, conexões, resposta HTTP, conteúdo de fonte, tokens e tamanho de agenda por configuração validada.

### NFR-EVOL-001 — Compatibilidade versionada

Schemas persistidos, `OperationSpec`s, templates, eventos e interfaces públicas MUST possuir versão e política explícita de compatibilidade ou migração.

## 8. Matriz inicial de rastreabilidade

| Área | Requisitos primários | Verificação planejada |
|---|---|---|
| Missão e autoridade | FR-AUTH-001..004 | validação, diff e policy tests |
| Agenda e progresso | FR-AGENDA-001..004 | testes de admissão e relógio virtual |
| Conhecimento | FR-KNOW-001..005 | schemas, invariantes e commit tests |
| Modelo fraco | FR-MODEL-001..004 | servidor falso, corpus e fuzz |
| Continuidade | FR-DUR-001..006 | crash/replay, reconciliação e relógio virtual |
| Recursos e segurança | FR-RES-001..002, FR-OBS-001..002 | falhas injetadas, taxonomia e redaction |
| Arquitetura | NFR-* | import checks, contract tests, CI e builds |
