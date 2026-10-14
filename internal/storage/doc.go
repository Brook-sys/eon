// Package storage é a persistência: SQLite real (puro-Go) com tabelas por
// agregado + events append-only + outbox, WAL, transação por ChangeSet,
// migrações versionadas embutidas (D03/D44, ADR-0003). Sem blob checkpoints.
//
// Lei (ADR-0002): storage implementa port — não conhece ninguém acima;
// contratos estreitos por repositório.
//
// M1 (ADR-0016): driver modernc.org/sqlite (puro-Go, CGO off, FTS5 provado
// por teste) — storage é o ÚNICO importador; schema e repositórios chegam
// nos passos seguintes do M1 (docs/plan/05).
package storage
