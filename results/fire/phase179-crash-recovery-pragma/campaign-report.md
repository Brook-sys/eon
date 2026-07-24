# Fase 179 — Crash recovery observável FULL vs NORMAL

**Schema:** `motor-autonomo.sqlite-crash-recovery-pragma.v1`

**Hipótese:** Sob SIGKILL (crash de processo), ambos `synchronous=FULL` e `synchronous=NORMAL` preservam dados comprometidos e descartam dados não comprometidos, porque o WAL replay do SQLite é atômico independentemente do nível synchronous. A diferença observável entre FULL e NORMAL aparece somente sob power-loss/crash do SO, que esta campanha não simula.

## Cenários

### Pre-commit crash (SIGKILL antes do COMMIT)

Subprocesso abre transação, escreve idempotency record, atinge `FailpointBeforeDurableCommit` e bloqueia. Parent SIGKILL. Transação não comprometida deve reverter.

| Synchronous | Uncommitted lost | Subsequent write OK | Reopen clean |
|-------------|------------------|---------------------|--------------|
| FULL        | ✓                | ✓                   | ✓            |
| NORMAL      | ✓                | ✓                   | ✓            |

### Post-commit crash (SIGKILL após COMMIT)

Subprocesso comprometa transação, atinge `FailpointAfterDurableCommit` e bloqueia. Parent SIGKILL. Dados comprometidos devem sobreviver; handle stale deve perder CAS, recarregar e retry com sucesso.

| Synchronous | Committed survived | Stale CAS conflict | Retry OK | Reopen clean |
|-------------|--------------------|--------------------|----------|--------------|
| FULL        | ✓                  | ✓                  | ✓        | ✓            |
| NORMAL      | ✓                  | ✓                  | ✓        | ✓            |

## Interpretação

**Equivalência observável confirmada.** Sob SIGKILL (process crash), o SO garante que buffers do filesystem sejam flushed, então o WAL no disco reflete o estado comprometido independentemente do `synchronous` level. A diferença FULL vs NORMAL — fsync no commit vs não — só importa se o próprio SO perder energia antes de flush.

**Decisão:** Manter `FULL` como default de produção. `NORMAL` permanece opt-in experimental. Esta campanha prova que `NORMAL` não introduz regressão observável de crash-recovery a nível de processo, mas não autoriza adoção operacional sem campanha de power-loss compatível com a garantia pretendida.

**Próximo recorte:** Isolar a contribuição do WAL checkpoint automático no reopen. Medir se `PRAGMA wal_autocheckpoint` atual afeta o tempo de reopen sob NORMAL vs FULL após N commits.
