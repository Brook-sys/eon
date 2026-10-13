# 10 — Roadmap de Marcos, Critérios de Aceite e Riscos

> Cada marco tem escopo fechado e critérios **verificáveis**. Nenhum marco começa sem o anterior aceito. Nada é declarado "pronto" fora do gate (Art. 1 da constituição).

## M0 — Fundação

**Escopo**: branch `v2` criada; pacote `plan/` promovido a `docs/`; constituição + ADRs fundacionais (partindo do 00-DECISIONS.md); CI completo com boundary checker; skills OpenCode; esqueleto de módulos vazios com fronteiras; binário único `eon` nos 3 modos (supervisor spawna/mata/ressuscita core; worker responde job spec via stdio); sandbox `basic` PoC.

**Aceite quando**:
- [ ] CI verde **enforçando** fronteiras (uma violação proposital reprova);
- [ ] supervisor mata o core e o ressuscita com backoff; crash-loop → redeploy do último binário bom;
- [ ] worker executa job em chroot+seccomp (rede bloqueada comprovada por teste), timeout/kill funcionando;
- [ ] detectar nível de sandbox no boot (`full`/`basic`/`none`) com teste para cada;
- [ ] ADRs 0001–000N aceitos; skills funcionando.

## M1 — Core durável

**Escopo**: SQLite (schema, migrações, event log, outbox); domínio mínimo; máquina de estados; loop multiciclo (C0–C4) com **provider fake** determinístico; scheduler com bandit v1; `eonctl` (inspect, timeline); crash-replay; kit de deploy com watchdog sidecar no stack Portainer (D40/D52) + runbook deploy-via-portainer (D50).

**Aceite quando**:
- [ ] missão-stub roda 1.000+ ciclos sem intervenção;
- [ ] `kill -9` mid-transaction em 30 pontos → reopen: sem perda, sem duplicação (suite de crash-replay);
- [ ] fila vazia → replenishment legítimo ou `CONTINUITY_BLOCKED` diagnosticado — nunca idle;
- [ ] cota fake esgota → Ciclo 2 acelera (F2 determinística roda), loop segue;
- [ ] `eonctl` mostra timeline completa do que aconteceu.

## M2 — Cognição viva

**Escopo** (ordem interna: adapter → calibração → missão, D46): adapter OpenAI-compat (perfis de dialeto herdando o empírico da v1); calibração pass^k → níveis; ladder de formatos + parser ladder (tabela, não clones); microturnos; n-sampling; budgets reais; **missão F1 real** (estudar o próprio domínio) end-to-end com free-tier — incluindo os relatórios de operação (D43); dashboard v1 (timeline SSE, orçamento).

**Aceite quando**:
- [ ] F1 produz artifacts citados end-to-end com um modelo free-tier real;
- [ ] calibração atribui nível e o harness degrada formato conforme o nível (teste com dois modelos de capacidade distinta);
- [ ] cota real esgota → degradação para L0 auditável;
- [ ] dashboard mostra o autômato vivo em tempo real; fechá-lo não afeta nada.

## M3 — Frentes completas

**Escopo**: F2 completa (memory stream, reflexões em árvore, síntese de lessons, revalidação); F3 nos níveis L0/L1 (prompts/políticas) com sandbox + janela de observação + rollback; git integration (auto/*, push); Lessons mecânicas aplicadas pelo scheduler.

**Aceite quando**:
- [ ] melhoria de prompt promovida com observação limpa (documentada);
- [ ] uma melhoria propositalmente ruim é revertida automaticamente com Lesson completa — **sem interromper o loop um segundo**;
- [ ] Lesson negativa bloqueia mecanicamente a re-proposta (teste);
- [ ] export markdown da F1 visível no repo (docs/knowledge/).

## M4 — Self-código

**Escopo**: F3 nível L2: build no sandbox, promoção de binário pelo supervisor, merge v2 + tag + push, crash-loop protection, protocolo de elevação de autonomia.

**Aceite quando**:
- [ ] o autômato aplica uma melhoria de **código** real em si mesmo, end-to-end (sandbox → commit → swap → observação → merge+tag+push);
- [ ] uma melhoria de código ruim derruba métrica → rollback automático do binário + revert + Lesson;
- [ ] proposta de elevação de autonomia é registrada como Improvement especial e espera concessão (não se auto-aplica).

## M5 — Endurecimento e vida pública

**Escopo**: adversariais (injeção, dados conflitantes, envenenamento de contexto); chaos drills (storage fail, kill loops, cota zero prolongada — executados via Portainer, D51); observabilidade profunda; runbooks (watchdog sidecar D52, deploy-via-portainer D50); deploy 24/7 via GHCR+Portainer (D49) com a missão real rodando; re-visita da exposição do dashboard (D55, não-bloqueante); relatório público de operação (D43) publicando os resultados do estudo.

**Aceite quando**:
- [ ] bateria adversarial: injeção não escala além do sandbox cego (sem rede/secrets provados por telemetria);
- [ ] incidente simulado de cada classe sobrevivido **sem parada não-forçada**;
- [ ] chaos drills de continuidade executados via Portainer e documentados (D51);
- [ ] semana de operação contínua com relatório de continuidade (zero paradas não-forçadas);
- [ ] runbooks completos testados (backup/restore, rollback manual, emergência de storage).

## Métrica central a acompanhar desde M2

Taxa de promoção vs rollback da F3; progresso verificado por custo por frente; tempo em CONTINUITY_BLOCKED (→ 0); aderência a formato por nível; uptime de continuidade (paradas não-forçadas → 0).

## Riscos e mitigações

| Risco | Mítigação |
|---|---|
| userns indisponível no host | degradação `basic` é o caso-alvo garantido (chroot+seccomp) |
| conflito git humano × autômato na v2 | PendingDemand + trabalho em outras frentes; rebase quando resolvido |
| mudança de política free-tier | perfis de dialeto recalibráveis; degradação L0 é sempre segura |
| injeção escala além do sandbox cego | rede/secrets fora do worker por construção; auditoria de jobs; D43: publicar resultados |
| crescimento do SQLite | política de compactação F2 (D30); compactação auditável com events |
| calibração gasta cota | probes baratos, cache por (perfil, modelo, versão) (D27) |
| auto-melhoria estagna (DGM mostra rollbacks frequentes) | propostas menores, arquivo de variantes consultado antes de propor, janelas maiores |
| autômato propõe mudanças perigosas demais | níveis + gates + constituição imune (Art. 6) + supervisor imutável |
| perdemos o controle do processo de novo | a constituição é testável (CI enforça); journal com limite; um harness; passos pequenos; **o plano antes do código** |

## Perguntas abertas

- Nenhuma pendente neste documento — fila centralizada no [00-DECISIONS.md](00-DECISIONS.md).
