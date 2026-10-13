---
name: eon-verify
description: Roda o gate completo do EON v2 (build, test, vet, fmt, boundary) e reporta. ÚNICO meio legítimo de declarar algo "verificado" (Constituição Art. 1). Use antes de declarar qualquer trabalho pronto, antes de commitar código e sempre que a verificação for citada.
---

# eon-verify — o gate é o juiz único

Você **não pode** declarar "feito", "funcionando", "verificado", "green" ou equivalente sem executar este checklist até o fim. Uma lei, dois executores: o autômato F3 lê ESTE MESMO checklist (D39).

## Checklist (ordem fixa, tudo do root do repo)

```sh
CGO_ENABLED=0 go build ./...
CGO_ENABLED=0 go test ./... -count=1
go vet ./...
test -z "$(gofmt -l .)"           # saída vazia = ok
go run ./tools/boundary
```

## Regras

1. CGO_ENABLED=0 é OBRIGATÓRIO em build e test — binário/teste dinâmico é rejeitado pelo sandbox por construção (ADR-0001).
2. `-count=1` impede cache falso-positivo.
3. `gofmt -l .` imprimindo QUALQUER arquivo = reprova (não "formatar depois": formatar AGORA e re-rodar).
4. Boundary checker imprimindo violação = reprova. Não existe "violação aceitável"; se a fronteira está errada, mude `rules.json` via ADR (docs/adr/), nunca ignore.
5. Teste pulado (SKIP) conta como NÃO verificado para o que ele cobria — reporte skips explicitamente.
6. Um único passo vermelho = tudo vermelho. Não proseguir para commit.
7. Reporte o resultado cru (comandos + saída resumida), sem eufemismo: "reprovou" é "reprovou".

## Saída esperada (reporte neste formato)

- build: OK/REPROVOU
- test: OK/REPROVOU (N pacotes; skips: lista)
- vet: OK/REPROVOU
- fmt: OK/REPROVOU
- boundary: OK/REPROVOU (pacotes, linhas)
- veredito: VERIFICADO / NÃO VERIFICADO

## Pós-gate

- VERDE → apenas agora commit (Constituição Art. 4: commit pequeno, conventional).
- VERMELHO → corrija a CAUSA, re-rodar o gate inteiro. Nunca "esconder" vermelho.
