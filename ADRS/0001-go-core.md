# ADR-0001: Go como linguagem do núcleo

Status: Accepted

## Contexto

O runtime precisa ser portátil, econômico, concorrente, observável, tolerante a falhas e distribuível para máquinas modestas.

## Decisão

Implementar kernel, domínio epistemológico, adapters e CLI em Go.

## Consequências

- contratos do kernel serão tipos Go estáticos;
- concorrência usará goroutines com limites explícitos e `context.Context`;
- testes usarão `testing`, fuzzing nativo e race detector;
- bindings ou bibliotecas disponíveis apenas em outras linguagens exigirão processo/adaptador externo ou substituto;
- consumo e portabilidade serão medidos, não presumidos.
