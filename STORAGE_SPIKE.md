# Spike comparativo de armazenamento

Status: concluído v1.0 — decisão em ADR-0003

## Objetivo

Comparar Dolt e SQLite + event log sob o mesmo contrato observável, sem selecionar previamente o backend e sem acoplar o domínio a SQL específico.

## Preflight de soluções existentes

- Dolt já oferece SQL compatível com MySQL e primitivas nativas de branch, commit, diff e merge, acessíveis por procedures e system tables. O spike deve usar essas primitivas, não reimplementá-las.
- SQLite já fornece transações atômicas; em WAL, commits são registros anexados ao log, leitores mantêm snapshots e há somente um writer por vez. O spike deve usar transações e WAL nativos, acrescentando somente o event log epistemológico exigido pelo domínio.
- Para Go sem CGO, `modernc.org/sqlite` é um driver `database/sql` disponível, mas sua própria documentação alerta para o acoplamento exato de versão com `modernc.org/libc`; isso entra no custo de dependências e manutenção.
- Dolt deve ser exercitado como processo externo. O contract adapter inicial usa `dolt sql` por invocação para evitar embedding e manter a mesma representação de checkpoint do SQLite; o runner medido deve usar `dolt sql-server` + driver MySQL para não contar startup de processo em cada operação e para testar sessões, branches e crash do servidor real.

Fontes primárias:

- https://www.dolthub.com/docs/introduction/what-is-dolt/
- https://www.dolthub.com/docs/sql-reference/version-control/dolt-sql-procedures/
- https://sqlite.org/wal.html
- https://pkg.go.dev/modernc.org/sqlite

## Unidade comparável

Cada adapter deve implementar `port.Store` e executar:

1. `contract.TestStore`, para semântica funcional e transacional comum;
2. `contract.TestDurableStore`, para reopen após fronteira de processo, rollback persistente e idempotência durável;
3. o workload e as medições descritos abaixo.

O harness durável cria uma localização isolada por teste, abre o store, fecha todas as conexões/processos e reabre os mesmos dados. Reiniciar somente um wrapper em memória não satisfaz o contrato.

## Dataset determinístico

Usar seed fixa e IDs derivados, nunca relógio ou aleatoriedade de sistema:

- 1.000 fontes sintéticas de texto, com snapshots entre 1 e 8 KiB;
- segmentação completa de cada fonte;
- 10.000 claims;
- 30.000 vínculos de evidência;
- 1.000 changesets aceitos e commits lógicos;
- duas investigações divergentes a partir da mesma base;
- pelo menos 100 conflitos estruturais deliberados e 100 conflitos epistemológicos representados como dados, não confundidos com conflito SQL.

O gerador deve produzir um manifesto com seed, contagens e SHA-256 do dataset para garantir que ambos os adapters recebem a mesma entrada.

## Fases medidas

1. criar e migrar uma base vazia;
2. carregar fontes e fragmentos;
3. carregar claims e evidências;
4. aplicar changesets em transações unitárias e em lotes configuráveis;
5. consultar head, histórico de entidade e impacto por dependência;
6. criar duas linhas de investigação;
7. produzir diff, introduzir conflitos e reconciliar/mergear;
8. fechar e reabrir o backend;
9. reconstruir uma projeção descartável;
10. executar crash subprocessado em pontos antes e depois do commit durável.

## Crash harness

Crash real não será simulado retornando erro no mesmo processo. Um subprocesso recebe um diretório de dados e um failpoint, grava um marcador de intenção, executa a transação e termina abruptamente no ponto solicitado. Outro processo reabre a base e classifica o resultado como:

- `NOT_APPLIED`: nenhuma parte oficial visível;
- `APPLIED`: commit, recibo, evento e estado canônico completos;
- `INVALID_PARTIAL`: qualquer combinação intermediária, que reprova o backend/configuração.

Repetir cada failpoint ao menos 30 vezes. Para SQLite, registrar `journal_mode`, `synchronous` e política de checkpoint. Para Dolt, separar commit SQL da criação do commit Dolt e medir explicitamente essa fronteira; não assumir atomicidade entre as duas sem evidência.

## Métricas e artefatos

Registrar em JSON e resumir em Markdown:

- versão exata do backend, driver e Go;
- sistema operacional/arquitetura;
- tempo wall-clock e alocações quando mensuráveis;
- p50, p95 e p99 por fase/operação;
- throughput;
- bytes em disco antes/depois e após compactação/GC/checkpoint;
- tempo de startup, reopen e recovery;
- tamanho e clareza do diff;
- quantidade e tipo de conflitos;
- tempo de reconstrução de projeção;
- número de processos, arquivos e passos operacionais;
- linhas de adapter/migração/harness como sinal auxiliar, nunca critério isolado;
- falhas, retries e resultados ambíguos.

Cada execução gera `results/<backend>/<run-id>/manifest.json`, `metrics.json`, logs sanitizados e relatório. Resultados brutos não entram no estado canônico do runtime.

## Critérios de decisão

Bloqueadores absolutos:

- falhar algum contract test;
- produzir `INVALID_PARTIAL` após crash;
- exigir que regras de domínio dependam de SQL específico;
- não permitir backup/restauração local verificável;
- licença ou dependência incompatível com operação local gratuita.

Pontuação posterior aos bloqueadores:

- correção e recuperação: 30%;
- diff/branch/merge úteis para revisão epistemológica: 20%;
- simplicidade operacional total: 20%;
- desempenho e footprint no workload: 15%;
- manutenção, integração Go e portabilidade: 10%;
- backup/exportação: 5%.

Diferenças menores que 10% em latência ou footprint são tratadas como inconclusivas sem repetição e intervalo de dispersão. Recursos nativos só recebem crédito quando eliminam código/risco no adapter real.

## Ordem de implementação

1. implementar gerador e runner backend-neutral;
2. implementar adapter SQLite mínimo e fazê-lo passar nas suites;
3. implementar adapter Dolt mínimo e fazê-lo passar nas mesmas suites;
4. executar carga reduzida para corrigir o harness;
5. executar carga completa em ambiente controlado;
6. registrar ADR final com dados brutos referenciados.

O adapter contratual Dolt usa o mesmo checkpoint binário integral do cenário SQLite, permitindo comparação sem antecipar um schema relacional específico. Seus testes exigem `DOLT_BIN` explícito e exercitam um repositório real, inclusive close/reopen.

O baseline do harness agora existe em `internal/storage/spike`: gera dataset e manifesto canônicos por seed, executa ingestão/claims/consultas somente pela `port.Store`, suporta lotes configuráveis, mede duração/throughput e p50/p95/p99 de batches ou consultas, soma o footprint lógico de arquivos sem seguir symlinks e reabre o backend em processo novo para classificar uma intenção como `NOT_APPLIED` ou `APPLIED`. As métricas versionadas registram footprint antes/depois/delta e um writer atômico gera `manifest.json`, `metrics.json` e `report.md`, recusando artefatos cujo SHA-256 não corresponda ao dataset medido. Hooks privados nos adapters marcam fronteiras de commit sem ampliar a porta de domínio. O worker `cmd/storage-spike-worker` publica um marcador de intenção sincronizado e encerra abruptamente nos hooks; testes subprocessados comprovam as fronteiras SQLite e, quando `DOLT_BIN` é explícito, Dolt CLI. Campanhas configuráveis exigem no mínimo 30 trials independentes, registram cada saída do worker e agregam `NOT_APPLIED`/`APPLIED`/`INVALID_PARTIAL`; saída normal do worker ou qualquer estado parcial reprova a campanha. A classificação composta exige visibilidade completa e consistente de evento, commit, recibo, head da missão, idempotência concluída e entidade canônica. O adapter medido `ServerStore` mantém um único `dolt sql-server` persistente, conecta por `database/sql` com driver MySQL, controla readiness/shutdown e passa as mesmas suites funcional e durável. Seus hooks separam `SQL COMMIT`, que torna o working set visível, de `DOLT_COMMIT`, que cria a versão auditável. O worker possui modo `dolt-server`, mata abruptamente servidor e writer nas três fronteiras e, após reopen, também consulta `dolt_status`.

As campanhas oficiais completas foram executadas em 2026-07-15 com Dolt 2.2.0. Os artefatos auditáveis em `results/dolt-server/2026-07-15/crash` preservam todos os trials e agregados: antes de `SQL COMMIT`, 30/30 foram `NOT_APPLIED`; após `SQL COMMIT` e antes de `DOLT_COMMIT`, 30/30 foram `INVALID_PARTIAL`; após `DOLT_COMMIT`, 30/30 foram `APPLIED`. Portanto, a separação entre commit SQL e commit Dolt não é apenas risco teórico: na configuração medida ela viola o bloqueador absoluto de ausência de estado parcial. O adapter Dolt por CLI não foi usado como número principal porque inclui startup por atualização e une SQL write a `DOLT_COMMIT` numa única invocação.

O workload comum completo também foi executado em 2026-07-15, com dataset SHA-256 `89e2e35498bfa091feab5004e6bf4a5fb0b984b6f241260836cf26498f4eaa04`, batch 100, Go 1.26.5 e artefatos em `results/{sqlite,dolt-server}/2026-07-15/workload`. Em uma execução por backend, SQLite 3.50.4 ocupou 43.666.696 bytes e Dolt 2.2.0 ocupou 170.181.869 bytes (3,90×). Dolt foi 1,48× mais rápido ao carregar claims (229 contra 154 ops/s), enquanto consultas ficaram dentro de 2%; a variância observada durante as execuções do harness confirma que números de latência de uma única rodada são apenas direcionais e não satisfazem o limiar de decisão. O comando reproduzível `cmd/storage-spike-runner` registra versão real do backend, driver, Go/plataforma, batch, percentis e footprint.

Resultado decisório final: Dolt falhou um bloqueador absoluto por `INVALID_PARTIAL`; desempenho bruto, diff ou branch não podem compensar essa falha. SQLite passa o contrato e os cenários de crash atuais e foi aceito pela ADR-0003 como backend do MVP. Dolt foi rejeitado na configuração medida; uma tentativa futura de reconciliação exige novo spike e não bloqueia o vertical slice.

Síntese de complexidade: o adapter SQLite possui 165 linhas Go de produção; os adapters Dolt CLI + servidor possuem 560, além de processo externo, lifecycle, porta, configuração e driver MySQL. O número de linhas é apenas auxiliar, mas a diferença operacional é material. Backup SQLite deverá ser comprovado por runbook/teste antes de dados não descartáveis; o backend permite cópia com store fechado ou API de backup/checkpoint.

Limite explícito do contrato atual: branch/diff/merge e impacto reverso genérico não são observáveis pela `port.Store`. Como Dolt falhou um bloqueador absoluto, métricas backend-native adicionais não foram usadas para pontuação. Uma necessidade futura deverá ser atendida por extensão backend-neutral ou novo ADR, sem fingir equivalência no runner comum.
