# 09 — Constituição do Desenvolvimento IA

> Este documento é a lei suprema para **qualquer IA (ou humano) que trabalhe no EON v2**. Ele existe porque a v1 morreu de processo, não de código: 554 "fases" para um plano de 8, 158 clones em `cmd/`, 3.273 arquivos de `results/` comitados, 70MB de binários no git, um `fix-test.py` que reescrevia testes para passar, ledger de 1,5MB, god packages. A constituição transforma a autópsia em proibições.

## Artigos

### Art. 1 — O CI é o juiz único de "verificado"
Nada é "feito" sem: `go build ./...`, `go test ./...`, `go vet ./...`, `gofmt -l` vazio, **boundary checker** verde (regras de import do 02-ARCHITECTURE.md). Nenhuma afirmação de verificação vale sem ter rodado o gate completo.

### Art. 2 — Proibições absolutas (autópsia da v1)
1. **Nunca** commitar binários, artefatos de build, `results/`, dumps de campanha.
2. **Nunca** criar `cmd/phaseNNN*` ou qualquer clone de experimento — existe **um** harness de campanha parametrizado.
3. **Nunca** editar teste para o código passar (o `fix-test.py` da v1 é o exemplo canônico da vergonha). Se o teste está errado, mude o teste com justificativa explícita no commit; se o código está errado, mude o código.
4. **Nunca** god packages: limite de tamanho/complexidade por pacote no checker; interface-união de escrita é proibida.
5. **Nunca** estado canônico escrito fora do kernel/changeset.
6. **Nunca** segredo em flag, prompt, commit, artifact ou sandbox.
7. **Nunca** ledger gigante: journal curto (arquivo mensal, limite de linhas), detalhe mora no git.

### Art. 3 — ADRs obrigatórios
Toda decisão estrutural (novo conceito de domínio, nova frente, mudança de fronteira de módulo, mudança de storage, mudança de protocolo) tem ADR **antes** do código. Os ADRs do M0 partem das decisões fechadas do `00-DECISIONS.md`.

### Art. 4 — Passos pequenos e verificáveis
- Cada entrega: objetivo limitado + requisito/invariante citado + implementação + testes proporcionais + evidência de recuperação quando aplicável + docs se contrato mudou.
- Proibido: mega-PR, mudança de 10 coisas, refatoração fantasma sem ADR.
- "Pronto" = gate verde + critério de aceite do marco satisfeito (10-MILESTONES.md).

### Art. 5 — Campanhas de modelo (evidence, not authority)
- **Um** harness de campanha (`eonctl campaign`): hipótese explícita + limites rígidos + formato de relatório versionado.
- Resultado de campanha nunca muda estado oficial sozinho; vira Lesson/Evidence que alimenta decisão com ADR.
- Campanhas live não rodam no CI; CI usa fixtures/replays herméticos.

### Art. 6 — A IA não se auto-promove
O teto de autonomia (D16) muda só por concessão (ou default). A F3 pode **propor**; nunca aplica elevação por conta própria. O supervisor e esta constituição são imunes à F3.

### Art. 7 — Falha exige análise
Todo bug encontrado em si mesmo vira Lesson com causa e regra mecânica antes de virar Issue/melhoria. Reprodução primeiro (teste que falha), correção depois.

### Art. 8 — Documentação é contrato
Quando um contrato muda, os docs mudam no mesmo commit. Docs mentirosos são piores que sem docs (v1: `NEXT_STEPS.md` 430 fases atrasado).

### Art. 9 — Estado do planejamento
Enquanto o pacote `plan/` existir, toda mudança de plano passa por ele (README.md: "Como iterar"). O pacote só é dissolvido quando promovido a `docs/` no M0.

## Skills OpenCode (D38 — definidas)

Conjunto de 6 skills com checklists rígidos, a materializar em `.opencode/` no M0:

| Skill | Função |
|---|---|
| `eon-verify` | Roda o gate completo (build/test/vet/fmt/boundaries) e reporta — único meio de declarar "verificado" |
| `eon-adr` | Scaffold de ADR a partir de template, com checagem de conflito com decisões existentes |
| `eon-step` | Checklist de implementação por passo pequeno: contrato → testes → código → gate → docs → journal |
| `eon-journal` | Adiciona entrada curta e estruturada no journal (formato rígido, recusa entradas gigantes) |
| `eon-campaign` | Protocolo de campanha live: hipótese, limites, execução via harness único, relatório |
| `eon-plan` | Guia de iteração do pacote `plan/` (para sessões de planejamento como esta) |

AGENTS.md (raiz) referencia a constituição e é a primeira leitura obrigatória de qualquer agente.

**Uma lei, dois executores (D39)**: os checklists das skills são arquivos de dados — a IA de desenvolvimento lê via skill OpenCode; o autômato lê o mesmo checklist no pipeline F3, e o gate `eon-verify` é o mesmo comando que roda no sandbox. Drift entre "verificado pela IA" e "verificado pelo autômato" é impossível por construção.

## CI/CD (workflow GitHub Actions, no M0)

1. Gate completo (Art. 1) em todo PR/push na v2;
2. Boundary checker custom Go+AST (D32) como step obrigatório — inclui **teto de diff** (D45): >500 linhas em código não-gerado reprova;
3. Checagem anti-lixo: nenhum binário/artefato >X KB comitável (reprova o PR);
4. Build de imagem-casca → GHCR com **tags imutáveis `shell-vN`** (D54; casca — D11) — cadência: só quando supervisor/Dockerfile/CI mudam + gatilho manual (D47); deploy e testes de continuidade via **Portainer** (D49–D51): `StackUpdate` com repull ou webhook do stack `eon`, verificação fora-de-banda de health/logs;
5. Migrações testadas em cadeia (05);
6. Versão do Go pinada (D42): builder image fixada por digest; upgrade de toolchain = Improvement deliberada.

## Perguntas abertas

- Nenhuma pendente neste documento — fila centralizada no [00-DECISIONS.md](00-DECISIONS.md).
