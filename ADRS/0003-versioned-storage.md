# ADR-0003: Backend versionado desacoplado; Dolt condicionado a spike

Status: Proposed

## Contexto

A base precisa de transações, histórico, diff, branches, merge, auditoria, proveniência e recuperação. Dolt fornece primitivas Git-like sobre dados SQL, mas ainda não foi medido no workload do projeto.

## Decisão proposta

Definir interfaces de armazenamento e semântica de `ChangeSet`/`Commit` independentes do backend. Manter Dolt como candidato líder e compará-lo com SQLite + event log; PostgreSQL entra se concorrência multiusuária justificar.

## Critérios de aceitação do Dolt

- correção sob crash e retry;
- integração Go estável;
- diff/branch/merge úteis para changesets epistemológicos;
- footprint aceitável;
- backup e restauração claros;
- desempenho adequado no cenário de spike;
- ausência de acoplamento que impeça migração.

## Consequências

- o primeiro vertical slice usa um store em memória e/ou adapter mínimo;
- nenhuma regra de domínio depende de SQL específico do Dolt;
- merge semântico continuará na camada epistemológica;
- versionamento não substitui tempos válido/transacional nem W3C PROV.
