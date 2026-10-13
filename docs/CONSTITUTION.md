# Constituição do Desenvolvimento IA — EON v2

> Este documento é a lei suprema para **qualquer IA (ou humano) que trabalhe no EON v2**. Ele existe porque a v1 morreu de processo, não de código: 554 "fases" para um plano de 8, 158 clones em `cmd/`, 3.273 arquivos de `results/` comitados, 70MB de binários no git, um `fix-test.py` que reescrevia testes para passar, ledger de 1,5MB, god packages. A constituição transforma a autópsia em proibições.
>
> Promovida de `docs/plan/09-CONSTITUTION.md` (M0). A fonte original permanece intacta como registro histórico.

## Artigos

### Art. 1 — O gate é o juiz único de "verificado"
Nada é "feito" sem o gate completo, nesta ordem:
```sh
CGO_ENABLED=0 go build ./...
CGO_ENABLED=0 go test ./... -count=1
go vet ./...
test -z "$(gofmt -l .)"
go run ./tools/boundary
```
Nenhuma afirmação de verificação vale sem ter rodado tudo. O mesmo gate roda no CI (`.github/workflows/ci.yml`) e será lido pelo pipeline F3 do autômato — **uma lei, dois executores** (D39). CGO off é lei (binário estático, D07); testes rodam com CGO off senão o binário de teste fica dinâmico e o sandbox o rejeita por construção.

### Art. 2 — Proibições absolutas (autópsia da v1)
1. **Nunca** commitar binários, artefatos de build, `results/`, dumps de campanha.
2. **Nunca** criar `cmd/phaseNNN*` ou qualquer clone de experimento — existe **um** harness de campanha parametrizado.
3. **Nunca** editar teste para o código passar (o `fix-test.py` da v1 é o exemplo canônico da vergonha). Se o teste está errado, mude o teste com justificativa explícita no commit; se o código está errado, mude o código.
4. **Nunca** god packages: orçamento de linhas por pacote no checker (`tools/boundary/rules.json`); interface-união de escrita é proibida.
5. **Nunca** estado canônico escrito fora do kernel/changeset.
6. **Nunca** segredo em flag, prompt, commit, artifact ou sandbox.
7. **Nunca** ledger gigante: journal curto (arquivo mensal, limite de linhas), detalhe mora no git.

### Art. 3 — ADRs obrigatórios
Toda decisão estrutural (novo conceito de domínio, nova frente, mudança de fronteira de módulo, mudança de storage, mudança de protocolo) tem ADR **antes** do código. Os ADRs fundacionais partem das decisões fechadas do `docs/plan/00-DECISIONS.md`. Decisões vivas nascem em `docs/adr/` (processo D48).

### Art. 4 — Passos pequenos e verificáveis
- Cada entrega: objetivo limitado + requisito/invariante citado + implementação + testes proporcionais + evidência de recuperação quando aplicável + docs se contrato mudou.
- Proibido: mega-PR, mudança de 10 coisas, refatoração fantasma sem ADR.
- Teto de diff: >500 linhas em PR reprova (D45).
- "Pronto" = gate verde + critério de aceite do marco satisfeito (`docs/plan/10-MILESTONES.md`).

### Art. 5 — Campanhas de modelo (evidence, not authority)
- **Um** harness de campanha (`eonctl campaign`): hipótese explícita + limites rígidos + formato de relatório versionado.
- Resultado de campanha nunca muda estado oficial sozinho; vira Lesson/Evidence que alimenta decisão com ADR.
- Campanhas live não rodam no CI; CI usa fixtures/replays herméticos.

### Art. 6 — A IA não se auto-promove
O teto de autonomia (D16) muda só por concessão (ou default). A F3 pode **propor**; nunca aplica elevação por conta própria. O supervisor e esta constituição são imunes à F3.

### Art. 7 — Falha exige análise
Todo bug encontrado em si mesmo vira Lesson com causa e regra mecânica antes de virar Issue/melhoria. Reprodução primeiro (teste que falha), correção depois. (Exemplo canônico do M0: prctl com ponteiro em arg2 — encontrado por bisseção empírica, teste de prova em `internal/sandbox`.)

### Art. 8 — Documentação é contrato
Quando um contrato muda, os docs mudam no mesmo commit. Docs mentirosos são piores que sem docs (v1: `NEXT_STEPS.md` 430 fases atrasado).

### Art. 9 — Decisões vivas são ADRs
O pacote de planejamento foi promovido a `docs/plan/` (registro histórico congelado, D01–D55). Nova decisão estrutural = novo ADR em `docs/adr/`; editar o pacote histórico é proibido.

## Skills OpenCode (D38/D39 — materializadas)

Conjunto de 6 skills com checklists rígidos, em `.opencode/skills/`:

| Skill | Função |
|---|---|
| `eon-verify` | Roda o gate completo e reporta — único meio de declarar "verificado" |
| `eon-adr` | Scaffold de ADR a partir do template, com checagem de conflito com decisões existentes |
| `eon-step` | Checklist de implementação por passo pequeno: contrato → testes → código → gate → docs → journal |
| `eon-journal` | Adiciona entrada curta e estruturada no journal (formato rígido, recusa entradas gigantes) |
| `eon-campaign` | Protocolo de campanha live: hipótese, limites, execução via harness único, relatório |
| `eon-plan` | Guia de leitura do pacote histórico `docs/plan/` (não de edição) |

AGENTS.md (raiz) referencia esta constituição e é a primeira leitura obrigatória de qualquer agente.

**Uma lei, dois executores (D39)**: os checklists das skills são arquivos de dados — a IA de desenvolvimento lê via skill OpenCode; o autômato lê o mesmo checklist no pipeline F3, e o gate `eon-verify` é o mesmo comando que roda no sandbox. Drift entre "verificado pela IA" e "verificado pelo autômato" é impossível por construção.

## CI/CD (GitHub Actions)

1. Gate completo (Art. 1) em todo PR/push na `v2`;
2. Boundary checker custom Go+AST (D32) como step obrigatório — inclui **teto de diff** (D45): >500 linhas em código não-gerado reprova o PR;
3. Checagem anti-lixo: nenhum binário/artefato grande comitável (reprova);
4. Job em container Alpine (root) rodando os testes de sandbox de verdade — chroot+seccomp provados no CI;
5. Build de imagem-casca → GHCR com **tags imutáveis `shell-vN`** (D54) — workflow manual (D47); deploy e testes de continuidade via **Portainer** (D49–D51): `StackUpdate` com repull ou webhook do stack `eon`, verificação fora-de-banda de health/logs (runbook `docs/runbooks/deploy-via-portainer.md`);
6. Migrações testadas em cadeia (a partir do M1);
7. Versão do Go pinada (D42); upgrade de toolchain = Improvement deliberada.

## Pendências deste documento

- Nenhuma — fila centralizada no `docs/plan/00-DECISIONS.md` (histórico) e em `docs/adr/` (vivas).
