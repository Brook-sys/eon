# EON v2

> Um autômato em Go que **nunca para**, trabalha em **três frentes modulares** intercaladas com execução cognitiva **serial**, **melhora a si mesmo** de forma graduada, reversível e falha-segura versionando no próprio git, **calibra o harness à capacidade do modelo** disponível (do 70B ao free-tier mais básico), e opera com **zero intervenção humana obrigatória**, permanecendo 100% observável e interferível em tempo real.

A branch `v2` é a casa da reescrita completa. A v1 permanece intacta na `main` como referência/arquivo morto — sua autópsia virou a constituição deste projeto.

## Um binário, três modos

```
eon supervisor   # L1: mantém o core vivo (PID 1 no container)
eon core         # L2: loop multiciclo (estado canônico em SQLite)
eon worker       # L3: 1 job = 1 processo, sandbox cego, protocolo stdio
eon health       # healthcheck do supervisor (D53)
eon sandbox      # reporta o nível de sandbox detectado
```

## Leitura obrigatória (nesta ordem)

1. [docs/CONSTITUTION.md](docs/CONSTITUTION.md) — a lei suprema do desenvolvimento
2. [docs/INVARIANTS.md](docs/INVARIANTS.md) — a lei do sistema
3. [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) — camadas, módulos, fronteiras
4. [docs/adr/](docs/adr/README.md) — registro vivo de decisões
5. [docs/plan/](docs/plan/README.md) — o pacote de planejamento congelado (D01–D55)

## Gate (único meio de declarar "verificado")

```sh
CGO_ENABLED=0 go build ./...
CGO_ENABLED=0 go test ./... -count=1
go vet ./...
test -z "$(gofmt -l .)"
go run ./tools/boundary
```

## Estado

- **Marco**: M0 — Fundação (branch `v2`, constituição, ADRs, esqueleto, binário 3 modos, sandbox `basic` PoC)
- Roadmap completo: [docs/plan/10-MILESTONES.md](docs/plan/10-MILESTONES.md)
