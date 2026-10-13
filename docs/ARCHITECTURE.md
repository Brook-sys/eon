# Arquitetura — EON v2 (documento vivo)

> Promovido de `docs/plan/02-ARCHITECTURE.md` (M0) e mantido COMO ESTÁ. Quando o código diverge daqui, ou o código está errado ou este doc não foi atualizado no commit — os dois são violações (Constituição Art. 8).

## Visão geral em camadas

```
docker (restart=unless-stopped, healthcheck)            ← casca externa
└─ L1  SUPERVISOR — processo raiz, imutável, mínimo
   ├─ watchdog: mantém o core vivo (spawn, backoff, restart)
   ├─ crash-loop detectado → re-promove último binário bom (D41)
   ├─ promoção/rollback de binário via pipe local com o core
   └─ NUNCA é modificado pelo autômato (muda só via imagem GHCR, via humano)
└─ L2  CORE — reiniciável, TODO estado canônico em SQLite
   ├─ KERNEL: loop multiciclo (docs/plan/03-LOOP.md), scheduler, estados, orçamentos
   ├─ FRENTES: F1 projeto · F2 pensamentos · F3 self (contrato WorkFront)
   ├─ COGNIÇÃO ADAPTATIVA: calibração → nível → estratégia (docs/plan/06-COGNITION.md)
   ├─ CAPABILITIES (tipadas, fail-closed):
   │    model (OpenAI-compat) · web (opcional) · file · git · exec (via sandbox)
   ├─ CONTROL API: unix socket local + token (CLI e dashboard são clientes)
   └─ spawna workers L3 quando precisa de exec isolada
└─ L3  WORKERS — efêmeros, descartáveis
   └─ 1 job = 1 processo: sandbox + job spec JSON via stdio
        + timeout/SIGKILL + outputs por arquivo; cego: sem rede, sem segredos
```

**Por que três camadas**: goroutine não se mata em Go — um `go build` travado ou OOM dentro do core mataria o loop. Worker = processo filho isolado, matável, que carrega a falha consigo. É o princípio de falha-segura aplicado à arquitetura de processos. Sem sandbox disponível, degrada para fail-closed (sem `exec`; F3-de-código pendente; F1/F2 determinísticas seguem vivas).

## Um binário, três modos (M0: implementado)

```
eon supervisor   # L1 — PID 1 no container; mantém o core vivo
eon core         # L2 — loop multiciclo (M0: boot + heartbeat; M1: loop + SQLite)
eon worker       # L3 — JobSpec JSON no stdin, JobResult JSON no stdout
eon health       # healthcheck (D53) — lê <data>/supervisor/health.json
eon sandbox      # reporta o nível de sandbox detectado (debug/ops)
eon version      # versão da casca
```

Estático (CGO off, SQLite puro-Go quando chegar), imagem Alpine final pequena + git. O binário re-executa a SI MESMO em papéis de subprocesso via `EON_PROCESS_ROLE` (`sandbox-probe`, `worker-child`) — sem segundos binários (D07).

### Contratos de processo (M0: implementado)

- **Data dir**: `--data DIR` (default `$EON_DATA` ou `/data`). Layout: `supervisor/` (health.json, core.log, incidents.log), `core/` (boot.json, heartbeat.json), `jobs/<id>/` (root/, stdout, stderr, result.json), `artifacts/eon/vN/` (binários promovidos).
- **Boot**: `cmd/eon` detecta o sandbox (raiz de composição) e injeta no supervisor via config e no core via `EON_SANDBOX_LEVEL` — supervisor (stdlib-only) e core (sem infra) nunca detectam sozinhos.
- **Binário promovido (D41)**: supervisor dá boot no MAIS NOVO artefato `artifacts/eon/vN/eon` com SHA-256 válido (`eon.sha256`); sem artefatos → a casca. Crash-loop → demote para o inferior válido → casca → mantém vivo no teto de backoff (nunca para).
- **Health (D53)**: `health.json` atômico a cada 10s; `eon health` reprova se supervisor morto, heartbeat >60s, ou core morto.
- **Job (L3)**: `JobSpec{id, argv[], timeout_ms}` — argv[0] é caminho FORA do sandbox de um executável ESTÁTICO; worker o copia para o scratch, child aplica chroot→rlimits→seccomp→`execve("/job", argv, env=[])`. Recusas legítimas: nível `none`, argv vazio, binário dinâmico (PT_INTERP). Timeout → SIGKILL no pgroup inteiro.

## Sandbox cego (M0: detecção + basic PoC implementados)

| Nível | Mecanismo | Onde funciona | Capacidade F3 |
|---|---|---|---|
| `full` | user+mount+net namespaces (netns isola a rede por construção) + chroot + seccomp | kernels com userns não privilegiado (homelab) | validação completa |
| `basic` | **chroot** (cap default do Docker) + **seccomp** bloqueando rede (BPF hand-rolled, sem privilégio) + rlimits (CPU/AS/fsize/nofile) + timeout/kill | qualquer container Docker padrão (o caso-alvo: Alpine via GHCR) | build/teste offline (deps vendorizadas) |
| `none` | exec recusado (fail-closed) | ambientes paranoicos | F3-de-código pendente; resto vivo |

Princípios: sem rede no worker; zero segredos no ambiente do worker; filesystem = scratch com só o binário do job; detecção de nível no boot **por probes que PROVAM o que o nível usa** — o probe de `full` replica a cadeia do child dentro do userns (chroot → seccomp → discar TEM que falhar), o de `basic` prova chroot direto + filtro seccomp real com discagem EPERM; o clone userns sozinho é telemetria, nunca nível (incidente do primeiro CI: runner que deixa o clone passar e nega o chroot teria feito o probe mentir "full"). Toda job é auditável (comando, timeout, resultado em `jobs/<id>/result.json`). NPROC fica fora no M0 (semântica per-uid em host compartilhado) — revisitado no M5.

## Layout de módulos (branch v2)

```
cmd/eon/                 entrypoint único (supervisor|core|worker|health|sandbox|version)
internal/domain/         contratos mínimos, puros           [só stdlib]
internal/statefile/      util de estado atômico (exceção folha documentada)
internal/port/           interfaces estreitas por repositório/capability (M1+)
internal/core/           loop multiciclo, scheduler, estados  [importa domain+port+statefile; NUNCA frentes]
internal/fronts/{project,thoughts,self}/   implementações WorkFront (M3+)
internal/cognition/      calibração, ladder de formatos, parsers, context compiler (M2+)
internal/capabilities/{model,web,file,git,exec}/   (M2+)
internal/storage/        sqlite + migrações + event log + outbox (M1+)
internal/supervisor/     L1 [só stdlib + statefile]
internal/sandbox/        detecção full/basic/none + seccomp BPF [stdlib + domain]
internal/worker/         L3, protocolo stdio [stdlib + domain + sandbox + statefile]
tools/boundary/          checker AST de fronteiras (lei D32)
docs/  ARCHITECTURE · INVARIANTS · CONSTITUTION · adr/ · runbooks/ · knowledge/ (export F1) · plan/ (histórico)
.opencode/skills/        6 skills (a lei para qualquer IA que trabalhe aqui)
```

## Regras de dependência (enforçadas por `tools/boundary` — D32)

A lei É o `tools/boundary/rules.json` (CI + skill `eon-verify` + teste do próprio checker). Resumo:

1. `domain` importa só stdlib.
2. `port` importa `domain`.
3. `core` importa `domain`, `port` (+`statefile`, util folha) — nunca `fronts`, nunca infra direta.
4. `fronts` importam `domain`, `port`, `cognition` — nunca `core`, nunca storage direto, nunca uma outra frente.
5. `capabilities` importam `port` (+`cognition` no caso do model) — nunca `core`.
6. `storage` implementa `port` — não conhece ninguém acima.
7. `supervisor` é independente: stdlib + `statefile`. Não importa `core` (o core é um processo, não um import).
8. Pacote interno não registrado no rules.json = violação; dependência externa não registrada = violação (D19: dependência é ADR).
9. Toda exceção exige ADR. Proibido: god packages (orçamento de linhas), interfaces-união de escrita, estado canônico fora do kernel.

## Persistência (M1 — detalhe em docs/plan/05)

SQLite real: tabelas por agregado + `events` (append-only) + `outbox`. WAL; transação por ChangeSet; migrações versionadas embutidas. Contratos de storage **estreitos por repositório**. Contract suites + crash-replay como padrão de teste.

## Git — a casa é a branch `v2` deste repo (D11)

- Boot: clone do origin (branch `v2`) para volume `/data/repo`; identidade `eon <eon@autonomo>`.
- `auto/*` = experimentos; `v2` = **só estado promovido**; `tags eon/vN` = versões de binário.
- Push com token do env — só no core, nunca no sandbox.
- Humano e autômato convivem na v2; conflito = PendingDemand (nunca bloqueia).
- GHCR rebuilda a casca; a melhoria viva de binário é local (build no sandbox, supervisor troca via artefatos `vN`).

## Deploy e operação — via Portainer (D49–D55)

- **Portainer é a plataforma de deploy e dos testes de continuidade**. O stack `eon` (id 52, ambiente `local` id 3) é reaproveitado: compose reescrito para a casca v2 (fonte: `deploy/stack-eon.compose.yml`), mantendo nome, volume `eon-data` e identidade. Portainer pertence à camada-casca imutável: o autômato **nunca** recebe credenciais Portainer nem `docker.sock`.
- **Imagem**: `ghcr.io/brook-sys/eon:shell-vN` — **tags imutáveis** (D54); cada deploy reponta o compose via `StackUpdate` (repull); rollback da casca = repontar para `N-1`.
- **Healthcheck**: `exec eon health` (D53); zero superfície de rede. Docker, Portainer e o watchdog leem o status nativamente.
- **Watchdog**: sidecar `autoheal` no próprio stack (D52) — reinicia o container eon se unhealthy; `docker.sock` somente no sidecar, read-only.
- **Dashboard**: `0.0.0.0:8089→8080` (D55 — escolha do operador, homelab em rede de confiança); M5 re-visita a exposição como item não-bloqueante.
- **Fluxo de deploy**: CI publica `shell-vN` no GHCR → `StackUpdate` com repull → verificação fora-de-banda: container up, health verde, tail de logs, eventos (D50). Runbook: `docs/runbooks/deploy-via-portainer.md`.
- Segredos: env vars do stack Portainer (model key, git token); nunca flags, nunca prompts, nunca sandbox.

## Degradação como filosofia de design

Todo recurso pode acabar: modelo (→ sleep-time compute F2/F3-determinístico), sandbox `full` (→ `basic`), sandbox `basic` (→ `none`), rede (→ trabalho local), dashboard (→ CLI), GitHub (→ git local, push pendente no outbox). Cada degradação tem comportamento definido e auditável; nenhuma leva à parada.
