---
name: eon-journal
description: Adicionar entrada curta e estruturada ao journal mensal do EON v2 (docs/journal/AAAA-MM.md). Use após cada entrega, deploy, incidente ou drill. Entradas são append-only, máximo ~10 linhas — detalhe mora no git (Constituição Art. 2.7).
---

# eon-journal — curto, estruturado, append-only

## Formato

Arquivo: `docs/journal/AAAA-MM.md` (crie no mês novo com o cabeçalho de regras). Entrada:

```md
## AAAA-MM-DD · <marco-ou-contexto> · <tipo>

- <o que foi entregue/mudou/incidiu> — <ref: commit hash / ADR-NNNN / Dxx>.
- <evidência em uma linha: gate vermelho/verde, métrica, veredito do drill>.
```

Tipos válidos: entrega · deploy · incidente · drill · decisão · retificação.

## Checklist

1. [ ] Entrada é NOVA no fim do arquivo (append-only; corrigir entrada antiga = nova entrada `retificação` citando a antiga).
2. [ ] Máximo ~10 linhas por entrada. Passou disso? O detalhe deveria estar no commit/ADR — linke, não narre.
3. [ ] Toda afirmação tem referência verificável (hash, ADR, decision, métrica citada). Sem "parece que funcionou".
4. [ ] Vermelho registrado como vermelho (incidente/drill reprovado NÃO é suavizado).
5. [ ] Journal continua pequeno: arquivo mensal; se um mês passa de ~200 linhas, o processo está escrevendo journal em vez de commitar — reduza, não arquive.

## Proibições

- ❌ Entrada de "status" sem entrega (o journal não é diário de bordo existencial).
- ❌ Duplicar conteúdo de commit/ADR (referencie).
- ❌ Editar/reescrever história — retificação é entrada nova.
