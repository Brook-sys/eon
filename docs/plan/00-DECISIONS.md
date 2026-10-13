# Registro de Decisões Fundacionais — EON v2

> Fonte única de verdade do que foi decidido. Toda decisão aqui tem status `FECHADA` ou `ABERTA`.
> Uma decisão só muda por iteração explícita do plano — nunca silenciosamente na conversa.

## Fundações

| # | Decisão | Status | Justificativa |
|---|---|---|---|
| D01 | **Linguagem: Go** | FECHADA | Binário estático, concorrência robusta, tipagem forte para contratos, SQLite puro-Go, CI rápido. Herda o ADR-0001 da v1 |
| D02 | **Modularidade = módulos em tempo de compilação** | FECHADA | Interfaces estreitas + registro em compile-time. Fronteiras enforçadas por CI. Plugins em runtime (WASM/hashicorp) e frentes como processos separados foram rejeitados por custo/complexidade |
| D03 | **Persistência: SQLite híbrido** | FECHADA | Tabelas canônicas por agregado + event log append-only + outbox. Nada de blob-checkpoint (o pecado da v1). Consultável por SQL direto, rastreável por replay |
| D04 | **Domínio: novo, mínimo e evolutivo** | FECHADA | Vocabulário novo (Mission/Task/Step/Artifact/Proposal/ChangeSet/Evidence/Lesson/Memory/Improvement/PendingDemand/Event). Conceito novo só entra com ADR |
| D05 | **Contrato de modelo: só OpenAI-compatible** | FECHADA | `/v1/chat/completions` cobre Ollama local, Groq, NIM, OpenRouter, LM Studio etc. Um adapter com perfis de dialeto. Piso de design: modelos 7B instrução-fraca. Sem dependência de features modernas (tool calling nativo, JSON mode como requisito) |
| D06 | **3 camadas: supervisor imutável / core restartable / workers efêmeros** | FECHADA | L1 = watchdog + promoção/rollback de binário (~centenas de linhas, nunca modificado pelo autômato). L2 = loop + frentes + cognição. L3 = workers descartáveis sob demanda (JSON via stdio, sem daemon). Resolve a tensão "nunca para" × "muda o próprio código". Degrada para 2 camadas com fail-closed (sem exec, missão viva) |
| D07 | **Um único binário estático, três modos de processo** | FECHADA | `eon supervisor` (PID 1), `eon core`, `eon worker`. Zero dependências de runtime; imagem final Alpine ~30MB com git |
| D08 | **Deploy: container Docker leve (Alpine) via GHCR** | FECHADA | Sem runtime Docker aninhado (impossível criar containers filhos). Workflow GHCR no CI. `restart=unless-stopped` + healthcheck cobrem a morte do container: "nunca para" é propriedade do sistema (docker + supervisor + estado durável) |
| D09 | **Sandbox: puro-Go, com níveis de degradação** | FECHADA | `full` (userns+mountns+netns+overlay) → `basic` (chroot + seccomp sem rede + rlimits + timeout/kill — garantido em qualquer container Docker padrão) → `none` (exec recusado, fail-closed). Princípio do **sandbox cego**: sem rede, sem segredos, filesystem scratch. Ameaça: desastre próprio + injeção web (não adversário com exploits de kernel) |
| D10 | **Execução cognitiva serial estrita** | FECHADA | Uma chamada de modelo em voo, sempre. Capacidades determinísticas podem ter paralelismo limitado e explícito. Justificativa: free-tier, consistência (τ-bench: pass^8 < 25% mesmo para modelos fortes), determinismo do kernel, simplicidade sem leases/corrida por cota |
| D11 | **Git: origin = este repo, branch `v2` é a casa** | FECHADA | Runtime clona a v2 para volume no boot; autômato commita em `auto/*`; promoção = merge na v2 + tag `eon/vN` + push; supervisor troca binário construído no sandbox. GHCR rebuilda só a casca. GitHub = diário público da vida do autômato |
| D12 | **Realidade de modelos: free tier + degradação offline** | FECHADA | Cap diário rígido de requests/tokens. Esgotou cota → F2 determinística e F3-sandbox continuam; loop nunca dorme (vira sleep-time compute, ver D22) |
| D13 | **Missões: N lógicas, 1 executando por vez** | FECHADA | Por paradigma podem ser simultâneas; na execução nunca (serialidade D10) |

## Operação e operador

| # | Decisão | Status | Justificativa |
|---|---|---|---|
| D14 | **Operador: zero intervenção obrigatória** | FECHADA | Perguntas ao humano = PendingDemand (não-bloqueante, TTL, escopo local). O autômato continua iterando em outras frentes enquanto espera; planeja para não acumular perguntas. Humano visualiza tudo em tempo real e interfere quando quiser — nunca precisa |
| D15 | **Interfaces: CLI primeiro, dashboard no Marco 2** | FECHADA | Log estruturado + `eonctl` desde M1; dashboard web (timeline SSE, diffs, orçamento) no M2, desenhado já no blueprint |

## Auto-melhoria

| # | Decisão | Status | Justificativa |
|---|---|---|---|
| D16 | **Autonomia graduada com teto configurável** | FECHADA | Níveis L0 (prompts) → L1 (políticas/capacidades) → L2 (código com gates). Teto por config (default L2); elevação é proposta com evidência, humano **pode** conceder, nunca precisa |
| D17 | **Falha-segura obrigatória** | FECHADA | Nenhuma melhoria/erro/injeção derruba o autômato — derruba só a tentativa, que é revertida e analisada. Reverter sem análise registrada é violação de invariante |
| D18 | **Reversibilidade total via git próprio** | FECHADA | 1 melhoria = 1 commit atômico. Promoção com gates + janela de observação + rollback automático. Arquivo de variantes estilo Darwin Gödel Machine: tentativas aplicadas E rejeitadas ficam versionadas com análise |
| D19 | **F3 valida as próprias mudanças com exec no sandbox** | FECHADA | Shell irrestrito **dentro** do sandbox cego (D09). Dependências vendorizadas para build/teste 100% offline (regra constitucional) |
| D25 | **Janela de observação F3 definida** (fecha A04) | FECHADA | **Adaptativa com cap**: mínimo = N ciclos completos cobrindo cada frente ≥1 vez sem anomalias ativas; máximo = 48h (cap para não travar promoção); estende se ambiente mudar materialmente (troca de binding, emenda de missão). **Triggers**: HARD = rollback imediato (crash-loop do core, violação de invariante, falha de build/boot do novo binário); SOFT = rollback + Lesson (regressão >20% de progresso-verificado-por-custo com amostra mínima; anomalia de orçamento >50% do normal). **Baseline**: snapshot trailing pré-apply (últimas 24h ou N ciclos) das mesmas métricas, gravado como Artifact vinculado ao Improvement — comparação apples-to-apples, auditável e reproduzível |

## Frentes e cognição

| # | Decisão | Status | Justificativa |
|---|---|---|---|
| D20 | **Três frentes modulares desde o início, intercaladas** | FECHADA | F1 (projeto/epistêmica), F2 (pensamentos/curadoria), F3 (self). Contrato uniforme `WorkFront`; adicionar frente = registrar implementação, zero mudança no kernel |
| D21 | **Primeira missão F1: estudar o próprio domínio** | FECHADA | Pesquisar e documentar agentes contínuos, auto-melhoria, harnesses para modelos fracos → base de conhecimento citada exportada como markdown no repo. Auto-referencial, verificável e a lista de leitura semente já existe (04-SCIENCE.md) |
| D22 | **Loop multiciclo C0–C4** | FECHADA | Substitui o loop linear da v1: passo (microturn + verificação por passo) / tarefa (decomposição + Lesson) / reflexão (memória, sleep-time compute) / evolução (F3, recalibração) / vida (frontier QD, missão). Ver 03-LOOP.md |
| D23 | **Web: local primeiro, web opcional** | FECHADA | F1 começa com repo e documentos locais; fetch web direto + search API free-tier (env) como capabilities opcionais com degradação graciosa. Sem infraestrutura externa obrigatória (SearXNG rejeitado como dependência) |
| D24 | **Formato da Lesson mecânica definido** (fecha A01) | FECHADA | **3 tipos nucleares de regra** (mínimo extensível; novos tipos só por ADR): `DEMOTE` (rebaixa prioridade de candidatos de uma classe), `BLOCK` (elimina classe de candidatos), `CONSTRAINT` (restringe parâmetros, ex.: `max_tokens ≥ 1024` para um perfil). **Autoridade graduada por corroboração**: 1ª Lesson de uma classe → regra SOFT (só demotion); 2ª Lesson independente da mesma classe → promove a HARD BLOCK. Um único fracasso nunca cega uma frente; dois independentes sim — sem humano necessário. **Expiração**: toda regra tem `review_after` + decaimento de peso; candidato pode atravessar um BLOCK declarando explicitamente "por que desta vez é diferente" — se falhar de novo, a regra sai reforçada |
| D26 | **Scheduler: bandit definido** (fecha A02 e A14) | FECHADA | **3 braços = frentes** (F1/F2/F3) no início; sub-famílias como braços só por ADR futuro. **Algoritmo: UCB1 com janela descontada** — recompensas antigas decaem; dado as estatísticas persistidas no store, a escolha é determinística (auditável, reproduzível, sem RNG). **Recompensa**: cada frente normaliza seu progresso-verificado-por-custo em [0,1] contra a própria história recente (score robusto: mediana+MAD clampada) — braços comparáveis em escala comum. **Piso de justiça**: cada frente é escolhida ≥1 vez por K ciclos (K≈20), independente da estatística |
| D27 | **Protocolo de calibração definido** (fecha A03 e A18) | FECHADA | **4 famílias de probes baratas** (~12–15 chamadas): formato (JSON e delimitado), instrução exata (string literal), coerência multi-passo (cadeia 2–3 passos), janela útil (recall em contexto preenchido); pass^5 nos críticos. **Atribuição conservadora por mínimo (fail-safe)**: nível = min(nível de formato, nível de instrução), ≥4/5 no probe crítico; coerência e janela útil **nunca sobem** o nível — só ajustam granularidade de decomposição e tamanho de contexto. **Recalibração**: boot + troca de binding/perfil + anomalia (taxa de parse failure acima do limiar dispara re-probe de formato) + semanal com cap de gasto; resultados cacheados por (perfil, modelo, versão) |

## Resiliência operacional

| # | Decisão | Status | Justificativa |
|---|---|---|---|
| D28 | **Gatilhos do Ciclo 2 definidos** (fecha A13) | FECHADA | **Híbrido**: rotina base = a cada N tarefas concluídas (N≈10) + idade da memória (itens >X dias sem reflexão entram na fila); **acelerador** = cota esgotada/idle → F2 vira full-time determinística (sleep-time compute, D12). Reflexão é rotina de saúde, não sobra |
| D29 | **Tratamento de erros de provider definido** (fecha A19) | FECHADA | **Tabela de classes**: 429/Retry-After → espera exata do header (herança empírica Groq da v1); 5xx/timeout → backoff exponencial com jitter e cap; 4xx de payload → 1 reparo determinístico, nunca retry cego; parse failure → ladder de reparo/degradação de formato; cota diária esgotada → THROTTLED + agendamento + Ciclo 2 acelera. **Circuit breaker**: 5 falhas consecutivas → nível efetivo degrada a L0 imediatamente; half-open com probe barata (~15min) restaura quando passa. Erro transiente nunca vira Lesson espúria |
| D30 | **Política de compactação do SQLite definida** (fecha A05) | FECHADA | **Idade+uso, auditável**: artifacts >90d sem acesso → blob comprimido fora das tabelas quentes; memórias de baixa confiança/importância → candidatas à compressão. Toda compactação é changeset com event (nunca DELETE silencioso). Executor: família F2-higiene, comprovada por Evidence. Teto de tamanho com alerta |
| D31 | **Protocolo de falha fatal de storage definido** (fecha A06) | FECHADA | SQLite irrecuperável → **incidente registrado** (log local + evento pós-recuperação) → storage novo → re-bootstrap do essencial (missão, config, calibração) → **F2 re-importa a essência**: export periódico versionado como markdown no repo git (missão, lessons ativas, memórias de alta confiança), **delta incremental a cada janela do Ciclo 2 com mudanças relevantes** (perda máx ≈ 1 janela), committado pelo core via outbox. Autômato segue em modo degradado — nunca parado esperando humano |

## Refino de design (rodada 4)

| # | Decisão | Status | Justificativa |
|---|---|---|---|
| D32 | **Boundary checker: custom em Go + AST** (fecha A07) | FECHADA | Checker próprio (~centenas de linhas): `go/parser` lê regras declarativas (allow/deny por pacote + limites de tamanho/complexidade por pacote); roda no CI como step e via skill `eon-verify`. Total controle, zero dependência externa — e é exatamente o tipo de código pequeno e determinístico que o próprio autômato pode melhorar depois |
| D33 | **`frontier_candidate` é agregado próprio** (fecha A17) | FECHADA | Tabela própria com lifecycle próprio: candidatos nascem de propostas (não executam); campos: nicho (frente × tema × dificuldade), score do nicho, idade, status. Lifecycle diferente de Task justifica agregado separado |
| D34 | **Merge policy: `--no-ff` + tag sempre** (fecha A21) | FECHADA | Toda promoção na v2 é merge commit explícito + tag `eon/vN`: cada melhoria promovida é um nó legível no histórico; reverter = reverter o merge; linhagem do `auto/improvement-N` preservada (essencial para o arquivo DGM) |
| D35 | **Esquecimento supervisionado: o próprio autômato, por policy versionada** (fecha A24) | FECHADA | Alvo de compressão/esquecimento passa por checklist determinístico versionado (idade + confiança + importância + ausência de uso + não-referenciado por lessons ativas); resultado é changeset com event; operador pode reverter via git. A política de esquecimento é ela própria uma Improvement L1 — melhora pelo pipeline da F3 |
| D36 | **Export de conhecimento F1: coleção por tema + índice de claims** (fecha A23) | FECHADA | `docs/knowledge/`: índice de claims ativas por tema + páginas por tema (cada claim → citações byte-a-byte → artifacts com hash) + log de revisões. Legível por humano e autômato, diffável no git, índice verificável por script |
| D37 | **F3 lê código via mapa determinístico** (fecha A22) | FECHADA | Ferramentas determinísticas (grep, índice de símbolos via AST, métricas, resultados de testes recentes) constroem um mapa do código; a F3 recebe trechos curtos e direcionados + métricas — nunca o repo inteiro. Propostas em formato estrito com diff pequeno. Respeita a janela útil de modelos fracos |

## Execução, ferramental e encerramento (rodada final)

| # | Decisão | Status | Justificativa |
|---|---|---|---|
| D38 | **Skills: 6 com checklists rígidos** (fecha A08) | FECHADA | `eon-verify`, `eon-adr`, `eon-step`, `eon-journal`, `eon-campaign`, `eon-plan` — SKILL.md em `.opencode/` com gatilhos declarados e checklists rígidos. Cada skill é pequena, versionada, e o checklist é **dado legível** (pré-requisito do D39) |
| D39 | **Checklist compartilhado: uma lei, dois executores** (fecha A26) | FECHADA | Os checklists das skills são arquivos de dados: a IA de desenvolvimento lê via skill OpenCode; o autômato lê o MESMO checklist no pipeline F3, e o gate `eon-verify` é O MESMO comando que roda no sandbox. Drift entre "verificado pela IA" e "verificado pelo autômato" é impossível por construção |
| D40 | **Watchdog externo desde já** (fecha A10) | FECHADA | Camada extra além do docker restart + healthcheck, imune ao autômato (roda fora do container eon). **Forma concretizada em D52**: sidecar autoheal no stack Portainer; runbook obrigatório desde o M1 |
| D41 | **Binários bons: 3 + SHA-256 no evento** (fecha A11 e A20) | FECHADA | Últimos 3 binários bons em `/data/artifacts/eon/vN`; SHA-256 de cada gravado no evento de promoção e **re-verificado pelo supervisor antes do swap**. Rotação N=3 garante alvo de rollback |
| D42 | **Go pinado por minor + upgrade deliberado** (fecha A12) | FECHADA | Versão pinada no `go.mod` + builder image fixada por digest; upgrade de Go é Improvement deliberada com suite completa + janela de observação. Reprodutibilidade do build offline (deps vendorizadas) exige versão estável |
| D43 | **Publica os próprios resultados** (fecha A15) | FECHADA | Relatórios periódicos de operação (continuidade, promoções/rollbacks, calibração, custos) publicados em `docs/knowledge/` como parte da missão F1 — o estudo publica o experimento que ele mesmo é |
| D44 | **Índices canônicos + cursor pagination** (fecha A16) | FECHADA | Índice canônico por agregado (id, status, timestamps); `events` por (aggregate_type, aggregate_id, seq); FTS5 para memory; timeline com paginação por cursor (seq), sem OFFSET. O schema nasce com os índices que o blueprint precisa; o resto sobe por migração quando métrica pedir |
| D45 | **Commits: conventional + teto de diff no CI** (fecha A25) | FECHADA | Conventional commits + teto de diff enforçado pelo boundary checker (>500 linhas em código não-gerado reprova e exige quebra em passos menores). Mesma lei para humano e autômato |
| D46 | **Ordem do M2: adapter → calibração → missão F1** (fecha A27) | FECHADA | Dependências encadeadas: sem adapter não há calibração; sem calibração não se sabe qual formato pedir. Evita lessons espúrias sobre o modelo por harness errado |
| D47 | **GHCR: só quando a casca muda + gatilho manual** (fecha A28) | FECHADA | A imagem rebuilda apenas quando supervisor/Dockerfile/CI mudam (raro — supervisor é imutável) + gatilho manual; binários promovidos são troca local, não passam pela imagem (coerente com D11) |
| D48 | **Plano estável: consistência → congelar → M0** (fecha A29) | FECHADA | Encerradas as decisões: (1) passada final de consistência dos 12 docs; (2) plano declarado ESTÁVEL e congelado; (3) M0 começa. Mudanças futuras de design só por ADR |

## Deploy e operação via Portainer (rodada 5)

| # | Decisão | Status | Justificativa |
|---|---|---|---|
| D49 | **Portainer = plataforma de deploy e testes de continuidade** | FECHADA | Stack `eon` (id 52, ambiente `local` id 3) reaproveitado: compose reescrito para a casca v2, mantendo nome, volume `eon-data` e identidade. Portainer pertence à **camada-casca imutável**: o autômato **nunca** recebe credenciais Portainer nem `docker.sock` — casca muda só via humano/CI (coerente com D09/D40). Segredos = env vars do stack (mascaradas pelo Portainer) |
| D50 | **Fluxo de deploy com acompanhamento** | FECHADA | CI publica `shell-vN` no GHCR → deploy via `StackUpdate` com repull (ou webhook do stack, a criar) → **verificação fora-de-banda**: container up, health verde, tail de logs, eventos — o "acompanhar o deploy para captura de bugs" vira runbook obrigatório |
| D51 | **Chaos drills de continuidade via Portainer (M5)** | FECHADA | `StackStop/Start`, kill de container, observação de recuperação, restart counts e logs — executados pelas ferramentas Portainer e documentados como runbook |
| D52 | **Watchdog = sidecar autoheal no stack eon** (forma do D40) | FECHADA | Segundo serviço no mesmo stack: monitora o health status e reinicia o eon se ficar unhealthy (padrão autoheal, label-scoped). `docker.sock` montado **somente no sidecar** (read-only); o container eon nunca o enxerga. Tudo versionado no compose — zero configuração no host |
| D53 | **Healthcheck = `exec eon health`** | FECHADA | Subcomando do binário; supervisor publica heartbeat num arquivo de estado interno; `eon health` lê e responde. Zero superfície de rede; Docker, Portainer e o sidecar leem o status nativamente |
| D54 | **Tags imutáveis `shell-vN`** | FECHADA | Cada release da casca é tag imutável no GHCR; deploy reponta o compose (StackUpdate); rollback da casca = repontar para `N-1`. Reversibilidade aplicada à camada-casca |
| D55 | **Dashboard publicado em `0.0.0.0:8089→8080`** | FECHADA | Escolha deliberada do operador (homelab em rede de confiança; observabilidade em tempo real priorizada). M5 re-visita a exposição como item de endurecimento **não-bloqueante** |

## Decisões abertas (fila de iteração)

| # | Questão | Contexto |
|---|---|---|
| A09 | ID arXiv do paper AI Parenting | Não é decisão de design — é a primeira tarefa de pesquisa da F1; o plano não depende dela |
