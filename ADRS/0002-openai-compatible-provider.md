# ADR-0002: OpenAI-compatible como adapter principal de modelos

Status: Accepted

## Contexto

O runtime não deve depender de um modelo, fornecedor ou servidor específico. APIs chamadas de OpenAI-compatible variam bastante em endpoints, parâmetros e recursos.

## Decisão

Implementar um contrato interno `ModelProvider` e um adapter OpenAI-compatible com perfis de capacidades e quirks.

O contrato mínimo continua sendo texto para texto. Chat Completions, Responses, streaming, JSON Schema e tool calling são dialetos/capacidades opcionais.

## Consequências

- tipos externos não vazam para o domínio;
- cada provider passa por contract tests;
- capabilities são configuráveis e sondáveis;
- degradação para protocolo textual permanece obrigatória;
- compatibilidade será declarada por matriz de recursos, não por um booleano.
