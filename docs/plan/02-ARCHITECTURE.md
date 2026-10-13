# 02 — Blueprint Arquitetural

## Visão geral em camadas

```
docker (restart=unless-stopped, healthcheck)            ← casca externa
└─ L1  SUPERVISOR — processo raiz, imutável, mínimo
   ├─ watchdog: mantém o core vivo (spawn, backoff, restart)
   ├─ crash-loop detectado → re-promove último binário bom (D41)
   ├─ promoção/rollback de binário via pipe local com o core
   └─ NUNCA é modificado pelo autômato (muda só via imagem GHCR, via humano)
└─ L2  CORE — reiniciável, TODO estado canônico em SQLite
   ├─ KERNEL: loop multiciclo (03-LOOP.md), scheduler, estados, orçamentos
   ├─ FRENTES: F1 projeto · F2 pensamentos · F3 self (contrato WorkFront)
   ├─ COGNIÇÃO ADAPTATIVA: calibração → nível → estratégia (06-COGNITION.md)
   ├─ CAPABILITIES (tipadas, fail-closed):
   │    model (OpenAI-compat) · web (opcional) · file · git · exec (via sandbox)
   ├─ CONTROL API: unix socket local + token (CLI e dashboard são clientes)
   └─ spawna workers L3 quando precisa de exec isolada
└─ L3  WORKERS — efêmeros, descartáveis
   └─ 1 job = 1 processo: sandbox (full/basic) + job spec JSON via stdio
        + timeout/SIGKILL + outputs por arquivo; cego: sem rede, sem segredos
```

**Por que três camadas**: goroutine não se mata em Go — um `go build` travado ou OOM dentro do core mataria o loop. Worker = processo filho isolado, matável, que carrega a falha consigo. É o princípio de falha-segura aplicado à arquitetura de processos. Sem sandbox disponível, degrada para fail-closed (sem `exec`; F3-de-código pendente; F1/F2 determinísticas seguem vivas).

**Um binário, três modos**: `eon supervisor` (PID 1), `eon core`, `eon worker`. Estático (CGO off, SQLite puro-Go), imagem Alpine final pequena + git.

## Sandbox cego (D09)

| Nível | Mecanismo | Onde funciona | Capacidade F3 |
|---|---|---|---|
| `full` | user+mount+net namespaces, overlay, cópia do repo | kernels com userns não privilegiado + seccomp liberado | validação completa |
| `basic` | **chroot** (cap default do Docker) + **seccomp** bloqueando rede (BPF, sem privilégio) + rlimits (CPU/mem/nproc/fsize) + timeout/kill | qualquer container Docker padrão (o caso-alvo: Alpine via GHCR) | build/teste offline (deps vendorizadas) |
| `none` | exec recusado (fail-closed) | ambientes paranoicos | F3-de-código pendente; resto vivo |

Princípios: sem rede no worker; zero segredos no ambiente do worker; filesystem = cópia scratch do repo; detecção de nível no boot; toda job é auditável (comando, cwd, timeout, resultado).

## Layout de módulos (branch v2)

```
cmd/eon/                 entrypoint único (supervisor|core|worker por flag)
internal/domain/         contratos mínimos, puros           [só stdlib]
internal/port/           interfaces estreitas por repositório/capability
internal/core/           loop multiciclo, scheduler, estados  [importa domain+port; NUNCA frentes]
internal/fronts/{project,thoughts,self}/   implementações WorkFront
internal/cognition/      calibração, ladder de formatos, parsers, context compiler
internal/capabilities/{model,web,file,git,exec}/
internal/storage/        sqlite + migrações + event log + outbox
internal/supervisor/     L1
internal/sandbox/        full/basic/none + detecção
docs/  ARCHITECTURE · INVARIANTS · CONSTITUTION · adr/ · runbooks/ · knowledge/ (export F1)
plan/                    (este pacote — promovido/dissolvido em M0)
.opencode/               skills + AGENTS.md (a lei para qualquer IA que trabalhe aqui)
```

## Regras de dependência (enforçadas por CI — checker custom Go+AST, D32)

1. `domain` importa só stdlib.
2. `port` importa `domain`.
3. `core` importa `domain`, `port` — nunca `fronts`, nunca infra direta.
4. `fronts` importam `domain`, `port`, `cognition` (via port) — nunca `core` internamente, nunca storage direto.
5. `capabilities` importam `port` — nunca `core`.
6. `storage` implementa `port` — não conhece ninguém acima.
7. `supervisor` é independente: só filesystem/processos/pipe. Não importa `core`.
8. Toda exceção a estas regras exige ADR.
9. **Proibido**: god packages (limite de tamanho/imports por pacote no checker), interfaces-união de escrita, estado canônico fora do kernel.

## Persistência (resumo — detalhe em 05)

SQLite real: tabelas por agregado + `events` (append-only) + `outbox`. WAL; transação por ChangeSet; migrações versionadas embutidas. Contratos de storage **estreitos por repositório** (a anti-god-interface). Contract suites + crash-replay herdados como padrão de teste da v1.

## Git — a casa é a branch `v2` deste repo (D11)

- Boot: clone do origin (branch `v2`) para volume `/data/repo` (ou cópia da imagem); identidade `eon <eon@autonomo>`.
- `auto/*` = experimentos; `v2` = **só estado promovido**; `tags eon/vN` = versões de binário.
- Push com token do env — só no core, nunca no sandbox.
- Humano e autômato convivem na v2; conflito = PendingDemand (nunca bloqueia).
- GHCR rebuilda a casca; a melhoria viva de binário é local (build no sandbox `basic`, supervisor troca).

## Deploy e operação — via Portainer (rodada 5: D49–D55)

- **Portainer é a plataforma de deploy e dos testes de continuidade**. O stack `eon` (id 52, ambiente `local` id 3) é reaproveitado: compose reescrito para a casca v2, mantendo nome, volume `eon-data` e identidade. Portainer pertence à camada-casca imutável: o autômato **nunca** recebe credenciais Portainer nem `docker.sock`.
- **Imagem**: `ghcr.io/brook-sys/eon:shell-vN` — **tags imutáveis** (D54); cada deploy reponta o compose via `StackUpdate` (repull); rollback da casca = repontar para `N-1`.
- **Healthcheck**: `exec eon health` — supervisor publica heartbeat em arquivo de estado interno (D53); zero superfície de rede. Docker, Portainer e o watchdog leem o status nativamente.
- **Watchdog**: sidecar `autoheal` no próprio stack eon (D52) — reinicia o container eon se ficar unhealthy (label-scoped); `docker.sock` somente no sidecar, read-only; o container eon nunca o enxerga.
- **Porta do dashboard**: `0.0.0.0:8089→8080` (D55 — escolha do operador, homelab em rede de confiança); M5 re-visita a exposição como item não-bloqueante.
- **Fluxo de deploy (runbook)**: CI publica `shell-vN` no GHCR → `StackUpdate` com repull (ou webhook do stack) → verificação fora-de-banda: container up, health verde, tail de logs, eventos (D50).
- Runbooks: deploy-via-portainer, continuidade (drills), backup/restore SQLite, observação de orçamento, rollback manual de emergência, promoção de binário.
- Segredos: env vars do stack Portainer (model key, git token); nunca flags, nunca prompts, nunca sandbox.

## Degradação como filosofia de design

Todo recurso pode acabar: modelo (→ sleep-time compute F2/F3-determinístico), sandbox `full` (→ `basic`), sandbox `basic` (→ `none`), rede (→ trabalho local), dashboard (→ CLI), GitHub (→ git local, push pendente no outbox). Cada degradação tem comportamento definido e auditável; nenhuma leva à parada.

## Perguntas abertas

- Nenhuma pendente neste documento — fila centralizada no [00-DECISIONS.md](00-DECISIONS.md).
