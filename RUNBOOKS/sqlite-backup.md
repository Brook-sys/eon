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
// report inclui DestinationPath, SQLiteVersion, FileSize, SHA256,
// ApplicationID, UserVersion, SchemaVersion, SchemaObjects, SchemaSHA256,
// CheckpointRows, CheckpointFormat,
// CheckpointSHA256, IntegrityCheck e ForeignKeyCheck
```

Comportamento:

1. adquire o lock de escrita do adapter (serializa com `Update`);
2. copia páginas via `modernc.org/sqlite` `NewBackup` / `Step` / `Finish` (`sqlite3_backup_*`);
3. executa `PRAGMA integrity_check(1)` e `foreign_key_check`, exige os marcadores de header `application_id=0x4d415554` (`MAUT`) e `user_version=1`, e verifica versão externa, SHA-256, framing e decode integral do `runtime_checkpoint` quando existir;
4. registra separadamente o SHA-256 do payload versionado do checkpoint, permitindo confrontar a identidade lógica mesmo quando a cópia SQLite possui bytes físicos diferentes.
5. deixa a origem aberta e utilizável.

Regras:

- o destino **não** pode existir (fail-closed contra overwrite);
- a cópia é construída e verificada em arquivo temporário no mesmo diretório,
  sincronizada em disco, publicada atomicamente sem substituir um path criado
  concorrentemente e fica com permissão `0600`; o diretório é sincronizado
  depois da publicação e da remoção do nome temporário;
- diretórios pais são criados;
- cancelamento de `context` interrompe o step e remove o destino incompleto.

## Procedimento offline

1. Pare o runtime (ou chame `store.Close()`).
2. Execute o comando operacional, que falha se o destino já existir e imprime o relatório verificado em JSON:

```sh
go run ./cmd/sqlite-backup \
  -mode=backup \
  -source=/var/lib/motor-autonomo/runtime.sqlite \
  -destination=/var/backups/motor-autonomo/runtime-YYYYMMDD.sqlite
```

A API equivalente é `ClosedCopyTo(ctx, sourcePath, destPath, options)`: ela
exige que a origem já exista como arquivo regular (não cria banco ausente e não
segue symlink), sem arquivos `-wal`, `-shm` ou `-journal` adjacentes, reabre a
origem em modo SQLite `read-only`/`immutable`, faz `BackupTo` e fecha. A presença
de sidecar torna o estado lógico ambíguo e exige shutdown/checkpoint limpo antes
do backup. O fluxo não executa configuração ou migração na origem, não cria
WAL/SHM e compara inode, tamanho e SHA-256 antes/depois da cópia; qualquer
mutação concorrente ou sidecar surgido durante a cópia remove o destino e falha.

Para auditar uma cópia já existente sem migração ou escrita:

```sh
go run ./cmd/sqlite-backup \
  -mode=verify \
  -path=/var/backups/motor-autonomo/runtime-YYYYMMDD.sqlite
```

A API equivalente é `sqlite.VerifyBackup(path)`. A auditoria abre o artefato
imutavelmente, executa `PRAGMA integrity_check(1)` (incluindo constraints),
exige `PRAGMA foreign_key_check` sem violações e confere que toda linha de
`runtime_checkpoint` usa o único ID canônico `1`. Depois disso,
com SQLite `mode=ro&immutable=1`: não cria banco ausente, não executa migração
ou configuração e não deixa WAL/SHM/journal ao lado do backup. O resultado
válido registra `IntegrityCheck == "ok"`, `file_size` e o SHA-256 do arquivo
completo. A auditoria também exige os marcadores SQLite `application_id`
(`MAUT`) e `user_version` canônico, distinguindo um banco do runtime de outro
arquivo SQLite que por acaso possua uma tabela homônima; divergência de versão,
adulteração do payload ou framing inválido
tornam a cópia não restaurável. Preserve o JSON do backup ao lado do artefato ou em
inventário durável. Depois de copiar o backup para outro volume/host, fixe a
identidade registrada na verificação:

```sh
go run ./cmd/sqlite-backup \
  -mode=verify \
  -path=/mnt/restore/runtime-YYYYMMDD.sqlite \
  -expected-sha256=<sha256_emitido_no_backup> \
  -expected-schema-version=<schema_version_emitido_no_backup> \
  -expected-schema-objects=<schema_objects_emitido_no_backup> \
  -expected-schema-sha256=<schema_sha256_emitido_no_backup> \
  -expected-checkpoint-sha256=<checkpoint_sha256_emitido_no_backup> \
  -expected-checkpoint-rows=<checkpoint_rows_emitido_no_backup> \
  -expected-checkpoint-format=<checkpoint_format_emitido_no_backup>
```

A opção `-expected-sha256` fixa a identidade física transferida,
`-expected-schema-version`, `-expected-schema-objects` e
`-expected-schema-sha256` fixam a identidade da tabela canônica
`runtime_checkpoint` observada no inventário. A auditoria exige exatamente um
objeto de schema de aplicação — essa tabela — e rejeita tabelas, índices,
triggers ou views adicionais, mesmo que `runtime_checkpoint` permaneça válida,
`-expected-checkpoint-sha256` fixa a identidade lógica do estado serializado e
`-expected-checkpoint-rows`/`-expected-checkpoint-format` fixam o framing do
inventário, inclusive o caso válido vazio (`0`/`0`). Valores ausentes continuam
significando apenas calcular e reportar; valores fornecidos inválidos ou
divergentes são rejeitados. A auditoria
calcula o hash antes e depois de `integrity_check`/decode e falha se o arquivo mudar
durante a verificação. O path auditado precisa ser um arquivo regular direto:
symlinks são recusados, e a identidade do inode é conferida entre hash,
abertura SQLite e hash final para impedir troca de path durante a auditoria.
A auditoria também exige um artefato standalone: qualquer `-wal`, `-shm` ou
`-journal` adjacente é recusado antes e depois da leitura, pois o modo immutable
não executa recovery e não pode certificar com segurança um main file stale.

Alternativa manual: com o store fechado, confirme que não restaram WAL/SHM/journal e copie o arquivo principal para um diretório frio; em seguida verifique a cópia com o comando acima antes de considerá-la restaurável. Se houver sidecar residual, reabra/feche o store de forma limpa ou faça checkpoint suportado antes de copiar; não promova apenas o main file.

## Restauração

1. Pare escritores no destino.
2. Restaure para **um path novo**. O comando verifica integralmente o backup antes de copiar e recusa overwrite:

```sh
go run ./cmd/sqlite-backup \
  -mode=restore \
  -source=/var/backups/motor-autonomo/runtime-YYYYMMDD.sqlite \
  -destination=/var/lib/motor-autonomo/runtime-restored.sqlite \
  -expected-sha256=<sha256_emitido_no_backup> \
  -expected-schema-version=<schema_version_emitido_no_backup> \
  -expected-schema-objects=<schema_objects_emitido_no_backup> \
  -expected-schema-sha256=<schema_sha256_emitido_no_backup> \
  -expected-checkpoint-sha256=<checkpoint_sha256_emitido_no_backup> \
  -expected-checkpoint-rows=<checkpoint_rows_emitido_no_backup> \
  -expected-checkpoint-format=<checkpoint_format_emitido_no_backup>
```

Os digests e campos de framing esperados vinculam a restauração ao arquivo e ao checkpoint
selecionados no inventário,
em vez de apenas a qualquer SQLite estruturalmente válido. A implementação
também verifica novamente a origem depois da cópia e remove o destino se a
origem tiver mudado durante a restauração; isso torna verificável a exigência
de que o backup esteja offline. Além disso, compara contagem, versão e
`checkpoint_sha256` entre a origem verificada e o destino restaurado; qualquer
divergência lógica remove o destino antes da publicação operacional.

3. Inspecione o relatório JSON (`integrity_check == "ok"`, `source_sha256` igual ao digest selecionado, SHA-256/bytes do destino, formato esperado e um checkpoint quando aplicável).
4. Abra/promova o path restaurado somente depois das verificações operacionais. Para substituir o path canônico, mova o arquivo antigo para retenção segura e faça a promoção com o runtime parado; o comando deliberadamente não sobrescreve.
5. Verifique no mínimo:
   - `ActiveMissionRevision` da missão esperada;
   - `Events` recentes / head de commit se aplicável;
   - um `go test` da suite de contrato do backend se o artefato for promovido a dados não descartáveis.

A API equivalente com identidade fixada é `sqlite.RestoreToWithOptions(ctx,
backupPath, newRuntimePath, sqlite.RestoreOptions{ExpectedSHA256: digest})`.
Para fixar também o estado lógico e seu framing, preencha
`ExpectedCheckpointSHA256`, `ExpectedCheckpointRows` e
`ExpectedCheckpointFormat` com os valores do relatório inventariado.
`RestoreTo` permanece como atalho compatível, mas ainda fixa internamente o
digest observado na primeira verificação, verifica novamente a origem após a
cópia e verifica o destino por meio de `BackupTo`.

## O que **não** fazer

- `cp runtime.sqlite backup.sqlite` com o processo ainda em `Update` (risco de checkpoint rasgado / WAL incompleto).
- Tratar telemetria OTel ou dumps parciais como backup canônico.
- Restaurar um backup por cima de um store aberto.
- Automatizar overwrite do banco canônico sem preservar o arquivo anterior e sem promoção explícita.

## Testes de regressão

- `go test ./cmd/sqlite-backup ./internal/storage/sqlite`
- `TestRunBackupAndVerify` (inclui restore para path novo)
- `TestRunRejectsUnsafeOrIncompleteArguments`
- `go test ./internal/storage/sqlite -run Backup`
- `TestOnlineBackupPreservesCheckpointAndReopens`
- `TestRestoreToVerifiesAndReopensCheckpoint`
- `TestRestoreToRejectsExistingDestinationAndInvalidSource`
- `TestOnlineBackupRejectsExistingDestination`
- `TestOnlineBackupEmptyStore`
- `TestVerifyBackupRejectsDigestMismatchAndInvalidExpectation`
- `TestVerifyBackupRejectsSymlinkPath`
- `TestVerifyBackupRejectsCheckpointVersionMismatch`
- `TestVerifyBackupRejectsTamperedCheckpointPayload`
- `TestVerifyAndClosedCopyRejectSQLiteSidecars`
- `TestVerifyBackupDoesNotMutateSourceOrCreateSidecars`
- `TestClosedCopyToDoesNotMutateSourceOrCreateSidecars`
