# Matriz OpenAI-compatible

Status: baseline v0.1

## Escopo portátil do MVP

O nome “OpenAI-compatible” não constitui um contrato único. Para o MVP, um
perfil é compatível somente quando comprova o subconjunto abaixo por teste de
contrato contra a versão implantada:

- `POST /v1/chat/completions` não-streaming;
- uma única mensagem `user` com conteúdo textual;
- uma única escolha `assistant` com conteúdo textual não vazio;
- limite explícito de saída por `max_tokens` **ou**
  `max_completion_tokens`, selecionado no perfil;
- `temperature` numérica;
- `usage.prompt_tokens` e `usage.completion_tokens` não negativos;
- erros HTTP classificáveis sem incorporar o corpo da resposta ao diagnóstico.

Streaming, Responses API, tools, multimodalidade, seed e structured outputs não
fazem parte do contrato interno. Podem existir no servidor, mas o kernel deve
continuar capaz de operar com protocolo textual e validar qualquer estrutura
depois da resposta.

## Matriz documental

| Implementação | Chat Completions texto | Campo de limite recomendado | Usage | Recursos fora do mínimo | Perfil inicial |
| --- | --- | --- | --- | --- | --- |
| OpenAI | Sim | `max_completion_tokens` para modelos atuais; `max_tokens` é legado e incompatível com alguns modelos | Documentado, mas o objeto é opcional na API geral | Responses, tools, streaming, conteúdo multipartes | `openai-current` |
| Ollama | Sim, compatibilidade declaradamente parcial | Exemplos documentam `max_tokens` | Deve ser comprovado por versão/modelo no contract test | Responses, vision, tools e structured outputs em subconjuntos próprios | `ollama-chat` |
| vLLM | Sim quando o modelo possui chat template | Aceita parâmetros Chat API; extensões e parâmetros ignorados variam por versão | Deve ser comprovado no deploy | Completions, Responses e structured outputs; alguns parâmetros não suportados | `vllm-chat` |
| llama.cpp server | Sim | O servidor expõe rota OpenAI-compatible; campo exato deve ser fixado pelo teste da versão | Deve ser comprovado no deploy | Responses, schema-constrained JSON, tools e multimodal | `llamacpp-chat` |

“Deve ser comprovado” é intencional: documentação de produto não substitui
teste contra a combinação servidor, versão, modelo e chat template realmente
usada.

## Perfis do adapter

O adapter Go oferece dois dialetos mínimos explícitos:

- `max_tokens` (default conservador para o fake e servidores legados);
- `max_completion_tokens` (perfil OpenAI atual).

Não há fallback automático entre campos. Repetir uma solicitação com outro
dialeto após erro pode duplicar custo ou trabalho no provedor. A seleção deve
ser configuração versionada e validada antes da operação.

## Contract test de implantação

Cada provider/modelo candidato deve executar, sem dados sensíveis:

1. completar um prompt curto com saída textual;
2. confirmar o campo de limite configurado e uma saída dentro do budget;
3. confirmar forma da escolha e contadores de usage exigidos pelo MVP;
4. provocar 4xx e verificar classificação sem vazamento de corpo;
5. provocar resposta malformada e excessiva;
6. registrar versão do servidor, identificador do modelo, contexto anunciado e
   resultado, sem registrar credencial ou prompt operacional.

Falha em usage pode futuramente ser degradada para contagem local, mas essa
mudança requer contrato explícito; o adapter atual rejeita usage ausente.

## Fontes primárias consultadas

- OpenAI Chat API reference: <https://developers.openai.com/api/reference/resources/chat>
- Ollama OpenAI compatibility: <https://docs.ollama.com/api/openai-compatibility>
- vLLM OpenAI-Compatible Server: <https://docs.vllm.ai/en/stable/serving/online_serving/openai_compatible_server/>
- llama.cpp server README: <https://github.com/ggml-org/llama.cpp/blob/master/tools/server/README.md>

Consultadas em 2026-07-15. Capacidades mudam; a matriz é um baseline de perfil,
não uma promessa permanente de compatibilidade.
