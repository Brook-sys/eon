# Runbook — Backup e restauração SQLite (MVP)

Status: operacional para o adapter canônico `internal/storage/sqlite` (ADR-0003).

## Artefato canônico

- Banco principal (arquivo SQLite) **mais** WAL/SHM quando o store está aberto.
- O payload de domínio vive em `runtime_checkpoint` (BLOB versionado). O event log e o estado lógico estão serializados nesse checkpoint; não há segundo store paralelo.

## Procedimento suportado (online)

Use a API Go de backup online — **não** copie só o arquivo `.sqlite` enquanto o runtime escreve:

```go
store, err := sqlite.Open(path)
// ...
report, err := store.BackupTo(ctx, destPath, sqlite.BackupOptions{})
// report inclui DestinationPath, SQLiteVersion, CheckpointRows
```

Comportamento:

1. adquire o lock de escrita do adapter (serializa com `Update`);
2. copia páginas via `modernc.org/sqlite` `NewBackup` / `Step` / `Finish` (`sqlite3_backup_*`);
3. verifica legibilidade do destino (`runtime_checkpoint` quando existir);
4. deixa a origem aberta e utilizável.

Regras:

- o destino **não** pode existir (fail-closed contra overwrite);
- diretórios pais são criados;
- cancelamento de `context` interrompe o step e remove o destino incompleto.

## Procedimento offline

1. Pare o runtime (ou chame `store.Close()`).
2. `ClosedCopyTo(ctx, sourcePath, destPath, options)` reabre a origem, faz `BackupTo` e fecha.
3. Alternativa manual: com o store fechado, copie o arquivo principal (e WAL residual se existir) para um diretório frio; em seguida reabra com `sqlite.Open` e rode contract checks.

## Restauração

1. Pare escritores no destino.
2. Substitua o arquivo de runtime pelo backup (ou aponte o path do processo para o backup).
3. `sqlite.Open(backupPath)` deve carregar o checkpoint sem erro.
4. Verifique no mínimo:
   - `ActiveMissionRevision` da missão esperada;
   - `Events` recentes / head de commit se aplicável;
   - um `go test` da suite de contrato do backend se o artefato for promovido a dados não descartáveis.

## O que **não** fazer

- `cp runtime.sqlite backup.sqlite` com o processo ainda em `Update` (risco de checkpoint rasgado / WAL incompleto).
- Tratar telemetria OTel ou dumps parciais como backup canônico.
- Restaurar um backup por cima de um store aberto.

## Testes de regressão

- `go test ./internal/storage/sqlite -run Backup`
- `TestOnlineBackupPreservesCheckpointAndReopens`
- `TestOnlineBackupRejectsExistingDestination`
- `TestOnlineBackupEmptyStore`
