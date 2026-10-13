// Package storage é a persistência: SQLite real (puro-Go) com tabelas por
// agregado + events append-only + outbox, WAL, transação por ChangeSet,
// migrações versionadas embutidas (D03/D44, ADR-0003). Sem blob checkpoints.
//
// Lei (ADR-0002): storage implementa port — não conhece ninguém acima;
// contratos estreitos por repositório.
//
// M0: vazio por desenho (M1 — schema SQL é tarefa do M1, docs/plan/README).
package storage
