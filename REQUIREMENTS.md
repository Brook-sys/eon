# Requisitos do Runtime Epistemológico

Status: baseline v0.2

Os termos normativos em maiúsculas seguem BCP 14, conforme `GLOSSARY.md`. Cada requisito possui um identificador estável. Documentos, código e testes SHOULD referenciar estes IDs em vez de copiar o texto normativo. `INVARIANTS.md` formaliza as propriedades transversais e `FAILURE_TAXONOMY.md` normaliza falhas e recuperação.

## 1. Escopo e autoridade

O runtime é um artefato experimental. Requisitos de confiabilidade, modularidade, portabilidade, configuração e observabilidade existem para sustentar experimentos reproduzíveis e execução contínua controlada; não implicam objetivo comercial. O núcleo MUST permanecer neutro quanto à finalidade econômica de uma missão autorizada e MUST NOT derivar objetivo próprio de monetização.

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

**Implementação (2026-07-16):** `domain.UserAmendment` / `DiffMissionRevisions` / `PreviewMissionImpact` (puro) e `mission.Acceptor.Accept` (append+activate da nova `MissionRevision`, cancelamento de operations/inquiries não-terminais da revisão anterior via `EventCancel`, abandono de work opportunities OPEN/DEFERRED, eventos de auditoria). Testes em `internal/domain/mission_amendment_test.go` e `internal/mission/amend_test.go` (memory + reopen SQLite).

**Superfície HTTP/UI (2026-07-16):** Control API `GET /missions/{missionID}/active`, `POST /missions/amendments/preview` (puro, sem escrita) e `POST /missions/amendments/accept` (fail-closed em no-op/bloqueado; acceptor opcional → 503 se não wired). Bootstrap adapta `mission.Acceptor` em `control.MissionAmendmentAcceptor`. Dashboard experimental: seção **Emenda de missão (FR-AUTH-004)** com carregar ativa / preview / accept append-only.

## 2. Investigação e agenda

### FR-AGENDA-001 — Admissão rastreável

Toda `Inquiry` MUST derivar de um `InquiryCandidate` com origem, ganho epistemológico esperado, novidade, custo, risco, condição de resposta, condição de parada e revisão futura quando adiada.

**Evidência de aceitação:** candidato incompleto ou duplicado é rejeitado com razão persistida.

### FR-AGENDA-002 — Operação contratada

Toda `Operation` MUST instanciar uma `OperationSpec` versionada, declarar inputs/read set, budget, formato de saída, validadores, retry/fallback e autoridade máxima.

**Evidência de aceitação:** operação sem spec existente ou incompatível não entra em `READY`.

### FR-AGENDA-003 — Replenishment diversificado e limitado

Agenda vazia MUST disparar imediatamente estratégias diversificadas de continuidade. Cada estratégia e cada ciclo MUST aplicar escopo, deduplicação, budget e limite de admissões, mas o runtime MUST tentar outra família legítima quando uma frente se esgotar.

**Evidência de aceitação:** uma agenda vazia produz despacho por outra família ou diagnóstico `CONTINUITY_BLOCKED`, sem crescimento ilimitado, repetição equivalente ou estado global de repouso.

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

### FR-MODEL-005 — Descoberta de capacidade limitada

O runtime MUST representar capacidades e limites do provider/modelo como perfil versionado, distinguindo valores declarados, confirmados por probe, inferidos empiricamente e sobrescritos pelo operador. Capacidade desconhecida MUST NOT ser presumida disponível. Probes MUST possuir budget, timeout, cache e política explícita de reavaliação.

**Evidência de aceitação:** provider que anuncia recurso incompatível é rebaixado sem interromper o caminho texto→texto; probes não se repetem indefinidamente e seus resultados ficam auditáveis.

### FR-MODEL-006 — Adaptação progressiva e reversível

O runtime SHOULD explorar capacidades confiáveis — como formatos estruturados, tool calling, contexto maior ou competência superior — para melhorar eficiência ou qualidade. Toda promoção MUST preservar a semântica da `OperationSpec`, autoridade exclusiva do kernel, validação externa e fallback para nível mais simples. Falha de uma otimização MUST permitir degradação segura sem perder a `Operation` nem duplicar efeito lógico.

**Evidência de aceitação:** a mesma operação executa no baseline texto→texto e em perfil enriquecido; injeção de falha em JSON mode, tool calling ou limite de contexto provoca fallback seguro e resultado oficial equivalente quando semanticamente possível.

### FR-MODEL-007 — Uso conservador de contexto e horizonte

Janela de contexto maior MUST NOT implicar inclusão automática de histórico integral, abandono de microturnos ou expansão ilimitada do plano. Aumento de contexto, opções e horizonte MUST ser limitado por política e justificado por necessidade da operação ou ganho empírico.

**Evidência de aceitação:** testes com perfis 2k, 8k e maiores demonstram margens de segurança, seleção localizada de evidência e limites explícitos de expansão.

## 5. Continuidade e execução durável

### FR-DUR-001 — Estado operacional persistido

Toda unidade não terminal MUST possuir `OperationalState` persistido e condição explícita de reavaliação, como prontidão, dependência, `not_before`, evento, aprovação ou lease expirável.

**Evidência de aceitação:** reinício reconstrói filas e esperas sem depender de contexto conversacional.

### FR-DUR-002 — Ausência de repouso global

Enquanto a missão estiver `ACTIVE`, o runtime MUST NOT entrar em `Rest`, `IDLE` ou estado global equivalente. Sem trabalho admissível imediato, MUST executar descoberta de lacunas, revisão, manutenção, verificação, síntese, melhoria ou outra família autorizada de continuidade. Esperas temporais ou por evento MUST permanecer locais às unidades dependentes.

**Evidência de aceitação:** cenários com agenda vazia, frente esgotada e linha bloqueada demonstram seleção de outra família ou `CONTINUITY_BLOCKED` explícito, nunca `Rest` global.

### FR-DUR-003 — Leases recuperáveis

Execução concorrente MUST usar lease persistido. Lease expirado MUST tornar a operação reconciliável ou novamente elegível sem presumir ausência de efeito externo.

**Evidência de aceitação:** crash de worker e expiração controlada não perdem operação nem duplicam efeito lógico.

### FR-DUR-004 — Entrega e deduplicação

Efeitos externos MUST usar `IdempotencyKey` quando suportado ou reconciliação explícita. Eventos locais a publicar MUST usar outbox confirmada atomicamente com a mudança correspondente.

**Evidência de aceitação:** replay de evento, callback e outbox mantém um único efeito lógico.

### FR-DUR-005 — Tempo determinístico

Deadlines, retries, backoff, leases e cadência MUST depender de `Clock` injetável. Jitter MUST depender de `RandomSource` injetável.

**Evidência de aceitação:** testes não aguardam relógio real e reproduzem a mesma sequência com fontes controladas.

### FR-DUR-006 — Falha normalizada e recuperação segura

Toda tentativa malsucedida MUST produzir `FailureRecord` tipado com código, classe, locus, disposição de recuperação, certeza do efeito, escopo, correlação e política aplicada. Efeito `UNKNOWN` ou `PARTIAL` MUST ser reconciliado antes de retry, salvo garantia documentada de idempotência no destino.

**Evidência de aceitação:** contract tests mapeiam falhas de adapters, distinguem timeout antes/depois do envio e impedem segundo efeito sob resposta ambígua.

### FR-DUR-007 — Continuidade permanente com fronteira renovável

Enquanto a missão estiver ativa e o armazenamento operacional, o runtime MUST preservar uma frontier renovável e MUST NOT alcançar conclusão global implícita ou repouso global. Após mudança relevante no estado, conclusão, falha ou invalidação, MUST atualizar próximos trabalhos, sementes de replenishment ou obrigações recorrentes derivados da missão. Não havendo trabalho admissível imediato em uma frente, MUST tentar outra família autorizada; incapacidade global MUST produzir `CONTINUITY_BLOCKED` com causas explícitas. Continuidade MUST NOT depender de geração artificial de atividade, retries ilimitados, resposta do usuário, evento externo ou recurso opcional de modelo.

**Evidência de aceitação:** cenários de conclusão de todas as operações atuais, bloqueio, silêncio do usuário, degradação de provider e agenda vazia resultam em outra linha/família admissível ou falha de continuidade diagnosticada, sem repouso, busy loop, perda da missão ou conclusão global.

### FR-DUR-008 — Preferência por avanço seguro

Quando estratégias equivalentes estiverem disponíveis, a política SHOULD preferir operações menores, idempotentes, verificáveis e reversíveis, com checkpoint antes de fronteiras frágeis. O runtime MUST degradar qualidade, velocidade, frequência ou sofisticação antes de comprometer integridade, auditabilidade, recuperabilidade ou continuidade global.

**Evidência de aceitação:** fault-injection demonstra que falha de otimização ou capability não essencial preserva estado, fallback, scheduler vivo e possibilidade de avanço por outra linha.

### FR-DUR-009 — Horizonte abastecido e frontier limitada

Enquanto a missão estiver `ACTIVE`, o runtime MUST manter um horizonte executável curto por marcas versionadas de `low_watermark`, alvo e máximo, reabastecendo-o preventivamente a partir de uma frontier persistida. A frontier MUST aceitar decomposição e melhoria recursiva de trabalho e conhecimento anteriores, mas MUST aplicar limites de cardinalidade, profundidade, fan-out e budget. O modelo MAY propor oportunidades; somente lógica determinística validada MAY deduplicar, priorizar, admitir, adiar ou eliminar trabalho.

**Evidência de aceitação:** testes demonstram replenishment antes da agenda vazia, impedem admissão acima dos limites, rejeitam paráfrases sem delta, preservam derivação pai-filho e continuam encontrando trabalho em famílias diferentes.

### FR-DUR-010 — Bloqueio localizado e trabalho independente

Espera por resposta do usuário, aprovação, callback, recurso ou dependência MUST bloquear somente as `Inquiry`s e `Operation`s cuja precondição dependa dela. O scheduler MUST continuar considerando trabalho independente e o replenisher SHOULD derivar manutenção, validação ou investigação não dependente quando alinhada à missão.

**Evidência de aceitação:** uma pergunta ao usuário permanece pendente enquanto outras linhas progridem; ausência indefinida de resposta não paralisa o runtime nem causa repetição da pergunta fora da política.

### FR-DUR-011 — Manutenção e melhoria recorrentes

A missão ativa MUST poder declarar obrigações recorrentes de revisão, revalidação, atualização, auditoria, avaliação do harness e busca de lacunas. Tais obrigações MUST possuir cadência, budget, critério de delta e política anti-repetição, fornecendo continuidade legítima após a conclusão de frentes finitas sem fabricar atividade vazia.

**Evidência de aceitação:** relógio virtual demonstra criação limitada de operações recorrentes, ausência de duplicação antes da cadência e novos increments após mudança de evidência, capacidade ou estado.

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

### FR-CTRL-001 — Autonomia supervisionável

O runtime MUST continuar sob missão ativa sem depender da interface humana, mas MUST permanecer limitado por missão, políticas, capabilities e budgets versionados pelo operador. O operador MUST poder inspecionar estado, pausar novos despachos, retomar, cancelar e solicitar shutdown gracioso sem escrever diretamente no armazenamento canônico.

**Evidência de aceitação:** fechar ou reiniciar o dashboard não interrompe o kernel; comandos autorizados produzem transições e recibos, e comandos inválidos não causam mutação parcial.

### FR-CTRL-002 — Comandos idempotentes e auditáveis

Toda mutação originada por CLI, API ou dashboard MUST entrar como `OperatorCommand` tipado, autenticado, autorizado, persistido e idempotente, com revisão esperada quando aplicável. Aceitação HTTP MUST NOT ser confundida com efeito confirmado; o resultado MUST possuir recibo consultável e estado explícito, incluindo reconciliação quando a certeza do efeito for desconhecida.

**Evidência de aceitação:** timeout e replay com a mesma chave produzem um único efeito lógico; conflito de revisão, permissão ou intenção é rejeitado com evento correlacionado.

### FR-CTRL-003 — Eventos e mensagens externas

O runtime MUST aceitar `ExternalEvent`s tipados, limitados e correlacionáveis, incluindo mensagem do operador, resposta a pergunta, fonte autorizada e sinal de disponibilidade. Conteúdo externo MUST permanecer dado não confiável e MUST NOT se tornar política, configuração, capability ou comando privilegiado por interpretação textual.

**Evidência de aceitação:** mensagem acorda ou atualiza somente linhas compatíveis com a política; prompt injection no conteúdo não amplia autoridade; replay é deduplicado.

### FR-CTRL-004 — Configuração versionada

Configuração mutável MUST possuir schema e revisão, validação, diff, análise de impacto e regra de aplicação `HOT`, `NEXT_CYCLE`, `RESTART_REQUIRED` ou `IMMUTABLE`. Segredos MUST ser referenciados indiretamente. Mudanças de missão continuam sujeitas a `FR-AUTH-004`.

**Evidência de aceitação:** edição concorrente é detectada; configuração inválida não altera o runtime; aplicação diferida ocorre somente em fronteira segura e produz recibo.

### FR-CTRL-005 — Inspeção correlacionada e ao vivo

A Control API MUST permitir consultar e acompanhar, por sequência retomável, missão, agenda, scheduler, operações, tentativas, chamadas de modelo, validações, changesets, commits, artifacts, budgets, falhas, comandos e eventos externos. A explicação MUST ser reconstruível de registros oficiais e MUST NOT depender de cadeia de pensamento oculta do modelo.

**Evidência de aceitação:** após desconexão, o cliente retoma da última sequência e reconstrói a mesma timeline; uma operação pode ser navegada do motivo de seleção ao commit ou rejeição.

### FR-CTRL-006 — Aprovação e interrupção seguras

O plano de controle MUST distinguir pausa de novos despachos, cancelamento cooperativo, shutdown gracioso e eventual parada emergencial. Interrupção MUST NOT presumir ausência de efeito externo em voo; efeitos `UNKNOWN` ou `PARTIAL` continuam sujeitos a reconciliação. Aprovações e rejeições MUST registrar ator, escopo, revisão e motivo.

**Evidência de aceitação:** pausa preserva trabalho em voo e impede novo despacho; cancelamento em fronteira ambígua entra em reconciliação; aprovação repetida não duplica efeito.

### FR-CTRL-007 — Isolamento da interface

Dashboard e exportadores de telemetria MUST ser dispensáveis para execução, recuperação e auditoria canônica. Eles MUST NOT possuir acesso de escrita direta ao banco nem autoridade superior à Control API. Falha, lentidão ou ausência da interface MUST NOT paralisar o kernel.

**Evidência de aceitação:** fault injection derruba UI/stream/exportador enquanto o runtime persiste progresso e depois permite reconstrução por event log e read models.

### FR-CTRL-008 — Perguntas estruturadas e não bloqueantes

O runtime MAY propor perguntas ao operador para obter preferência, esclarecimento, confirmação ou informação ausente. Toda pergunta entregue MUST possuir identidade persistida, contexto, impacto, opções estáveis quando aplicáveis, resposta livre ou pedido de contexto quando permitido, escopo bloqueado e política de ausência de resposta. Espera por resposta MUST bloquear somente unidades dependentes e MUST NOT interromper scheduling, replenishment ou execução de outras frentes.

**Evidência de aceitação:** várias perguntas permanecem pendentes enquanto operações independentes progridem; resposta, expiração ou silêncio alteram somente o escopo declarado; reinício preserva perguntas e correlações.

### FR-CTRL-009 — Correlação inequívoca de respostas

Toda resposta MUST referenciar exatamente um `question_id` e MUST ser autenticada, deduplicada e validada contra canal, ator, status e revisão. Dashboard MUST enviar o identificador no formulário. Telegram MUST usar callback de botão ou reply à mensagem correlacionada; texto solto ambíguo MUST NOT ser associado por inferência quando houver múltiplas perguntas candidatas.

**Evidência de aceitação:** testes com respostas fora de ordem, duplicadas, tardias, em chat incorreto e com múltiplas perguntas pendentes não aplicam resposta à pergunta errada.

### FR-CTRL-010 — Canais dashboard e Telegram

O primeiro escopo MUST suportar dashboard e SHOULD suportar Telegram por bot configurado pelo operador, mantendo domínio e estado canônico independentes do transporte. Configuração MUST permitir habilitar, priorizar e limitar canais e destinatários. Credenciais do Telegram MUST ser referências secretas; entregas MUST usar outbox e recebimentos MUST entrar por `ExternalEvent` deduplicado.

**Evidência de aceitação:** a mesma pergunta pode ser entregue em canal configurado sem duplicar seu estado; falha ou ausência de Telegram não paralisa dashboard nem kernel; replay não cria segunda resposta lógica.

### FR-CTRL-011 — Política antispam de perguntas

Perguntas propostas por modelo MUST passar por gate determinístico de necessidade, impacto, alternativas, duplicação, prioridade e budget de interrupção. O runtime MUST limitar perguntas pendentes e taxa por janela, aplicar cooldown, quiet hours e agrupamento quando configurados, e MUST NOT reenviar automaticamente a mesma pergunta em cada ciclo. Perguntar SHOULD ser reservado a informação que possa mudar decisão relevante e não esteja disponível por meio autorizado mais barato.

**Evidência de aceitação:** gerador adversarial de perguntas equivalentes resulta em uma pergunta ou digest limitado; perguntas de baixo impacto são suprimidas; lembretes cessam após resposta, expiração ou substituição.

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
