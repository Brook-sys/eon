# ADR-0003: SQLite + event log como backend canônico do MVP

Status: Accepted — 2026-07-15

## Contexto

A base precisa de transações, histórico, auditoria, proveniência e recuperação. Diff, branches e merge são úteis para revisão, mas não podem enfraquecer a atomicidade de `Commit` do domínio. Dolt e SQLite + event log foram implementados sob o mesmo `port.Store`, suites funcionais/duráveis e workload determinístico descrito em `STORAGE_SPIKE.md`.

## Decisão

Usar **SQLite em WAL com `synchronous=FULL`, event log e commits lógicos do domínio** como backend canônico do MVP.

Manter `ChangeSet`, `Commit`, versionamento e repositórios independentes do backend. Dolt fica rejeitado para o MVP na configuração medida com `dolt sql-server` 2.2.0; sua reconsideração exige novo protocolo que elimine ou reconcilie deterministicamente a janela entre `SQL COMMIT` e `DOLT_COMMIT`, seguido pela repetição integral dos contract tests e crash campaigns.

PostgreSQL permanece fora do MVP e só deve entrar em novo ADR se concorrência multiusuária ou operação remota se tornar requisito.

## Evidência

- Ambos os adapters passam as suites funcional e durável, preservando o domínio desacoplado.
- A campanha oficial Dolt executou 30 crashes em cada fronteira. Antes de `SQL COMMIT`, 30/30 resultados foram `NOT_APPLIED`; depois de `DOLT_COMMIT`, 30/30 foram `APPLIED`; na janela intermediária, 30/30 foram `INVALID_PARTIAL`.
- `INVALID_PARTIAL` viola o bloqueador absoluto do spike e `FR-KNOW-004`; vantagens de diff, branch, merge ou desempenho não podem compensá-lo.
- No dataset comum de SHA-256 `89e2e35498bfa091feab5004e6bf4a5fb0b984b6f241260836cf26498f4eaa04`, SQLite 3.50.4 ocupou 43.666.696 bytes e Dolt 2.2.0 ocupou 170.181.869 bytes, ou 3,90× mais.
- As latências de uma única rodada foram direcionais: Dolt foi mais rápido na carga de claims, SQLite na carga de fontes e consultas ficaram dentro de 2%. Esses números não decidem a escolha porque não têm repetição suficiente e Dolt já falhou um bloqueador.
- O adapter de produção medido possui 165 linhas Go para SQLite contra 560 para Dolt; Dolt também requer processo servidor, lifecycle, porta local, configuração e driver MySQL. LOC é apenas sinal auxiliar, mas concorda com a menor complexidade operacional do SQLite.
- Artefatos brutos: `results/{sqlite,dolt-server}/2026-07-15/workload` e `results/dolt-server/2026-07-15/crash`.

## Diff, branch e merge

O MVP não adotará branches físicas de banco. O histórico oficial permanece composto por `ProposedChangeSet`, `Commit`, eventos e versões lógicas consultáveis pelo contrato do domínio. Divergência e conflito epistemológico continuam representados como dados; merge semântico permanece no kernel.

As primitivas nativas do Dolt são valiosas, mas não foram pontuadas depois do bloqueador absoluto. Se revisão por branch se tornar requisito, ela deverá ser introduzida por uma extensão backend-neutral ou por novo spike, sem fazer o domínio depender de SQL Dolt.

## Backup e restauração

O artefato canônico do MVP é o banco SQLite local junto de seus arquivos WAL quando houver conexão aberta. Backup e restauração operacionais deverão usar a API de backup/checkpoint do SQLite ou cópia com o store fechado, seguidos por reopen e contract check; copiar somente o arquivo principal durante escrita não é procedimento válido. O runbook e o teste de backup entram antes de dados não descartáveis em produção.

## Consequências

- o runtime pode avançar sobre o adapter SQLite sem alterar tipos do domínio;
- event log e commits lógicos, não commits do banco, definem a auditoria epistemológica;
- nenhuma regra de domínio depende de SQL específico do Dolt;
- merge semântico continuará na camada epistemológica;
- versionamento não substitui tempos válido/transacional nem W3C PROV;
- Dolt permanece código de spike e referência experimental, não dependência operacional do MVP;
- a decisão pode ser revista por novo ADR com evidência de crash que satisfaça `FR-KNOW-004`.
