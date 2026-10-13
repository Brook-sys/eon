---
name: eon-step
description: Checklist de implementação por passo pequeno no EON v2 (contrato → testes → código → gate → docs → journal → commit). Use para QUALQUER entrega de código ou doc estrutural. Proíbe mega-entrega: um passo = um objetivo limitado (Constituição Art. 4, D45).
---

# eon-step — passo pequeno, verificável, único

## Antes de começar

1. **Defina o passo em UMA frase** (ex.: "supervisor detecta crash-loop e re-promove binário"). Se não cabe numa frase, são dois passos.
2. **Cite a origem**: decision (Dxx), ADR-NNNN, ou issue/milestone (`docs/plan/10-MILESTONES.md`). Passo sem origem é invenção — a v1 morreu disso.
3. **Decisão estrutural envolvida?** (conceito/fronteira/protocolo/storage) → pare, use a skill `eon-adr` PRIMEIRO.

## Checklist de execução

1. [ ] Contrato primeiro: assinaturas/tipos/constantes em `internal/domain` (ou interface em `internal/port`) ANTES da implementação.
2. [ ] Testes proporcionais ao risco junto com o código (não depois, não "depois eu escrevo"): comportamento crítico = teste obrigatório; teste de prova quando o M0 pediu (ex.: rede bloqueada comprovada, violação proposital reprova).
3. [ ] Implementação minimal — nada além do passo definido. Funcionalidade futura é passo futuro.
4. [ ] Respeite `tools/boundary/rules.json`: import fora da allowlist = pare e repense (ou ADR mudando a fronteira).
5. [ ] Docs no MESMO passo se contrato mudou (Constituição Art. 8: `docs/ARCHITECTURE.md`/`docs/CONSTITUTION.md` divergindo do código = violação).
6. [ ] Rodar skill `eon-verify` (gate completo). Verde = pré-requisito de commit, não de "terminar".
7. [ ] Commit ATÔMICO conventional (`feat|fix|docs|test|refactor|ci(scope): <o que em uma linha>`), corpo explicando o PORQUÊ e citando a origem.
8. [ ] Entrada no journal via skill `eon-journal` (ou linha extra na entrada do dia).

## Proibições (a autópsia da v1 é a autoridade)

- ❌ Mega-PR / 10 mudanças num passo — teto de diff >500 linhas reprova (D45).
- ❌ `cmd/phaseNNN` ou clone de experimento — UM harness parametrizado.
- ❌ Editar teste para código passar — se o teste está errado, justifique MUDANÇA DO TESTE no commit; senão mude o código.
- ❌ Commitar binários, `results/`, dumps, artefatos de campanha.
- ❌ God package/interface-união — orçamento de linhas do rules.json é teto real.
- ❌ Segredo em flag/prompt/commit/artifact.
- ❌ Estado canônico fora do kernel/changeset.

## Ao terminar

Reporte: passo (frase), origem citada, gate (veredito cru da skill eon-verify), commit hash, o que ficou pendente (se nada: "nada pendente").
