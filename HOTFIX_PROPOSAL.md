# Análise de Correções Críticas & Abordagem (Groq/NIM)

## O que foi feito (Análise do Estado)
O runtime implementou uma base determinística forte e concluiu as camadas de persistência, UI (control plane) e event log. No entanto, esbarrou em bloqueios na **Fase 5 (Avaliação Cognitiva)**. A tentativa de usar providers remotos (NVIDIA NIM/Groq) no lugar do Ollama local resultou em falhas nos testes em rede:
- O Llama 1B e Llama 3.3 70B travaram completamente com timeouts de 90s, registrando latência absurda e retorno vazio sem erro de compilação.
- O Llama 3.1 8B conseguiu responder algumas queries, mas falhou 25 de 33 vezes na fase de validação (erros de payload JSON/sintaxe).
- Os contratos recém implementados de fallback, bindings e context pressure se provaram incompletos no mundo real contra endpoints severamente asfixiados (rate limited).

## Novas Abordagens e Correções Exigidas (Riscos Tratados)

1. **Timeout Silencioso (Network Bleed)**
   **Problema:** O executor do kernel está passando do limite e a thread morre aguardando o socket de rede por até 90 segundos.
   **Correção de Abordagem:** O adapter `internal/provider/openai/provider.go` deve obrigatoriamente herdar o `context.Context` com timeout da Lease da agenda. Se a lease tem 10 segundos restantes, o provider deve ser cancelado em 9.5s localmente via `context.WithTimeout`, traduzindo isso em um retorno `EXHAUSTED` interno e liberando o runtime para tentar o `FallbackModelUsed`.

2. **Fragilidade Estrutural (JSON Validator Bloqueando Modelos Fracos)**
   **Problema:** Llama menores sofrem alucinações mínimas que quebram o parser rígido do kernel.
   **Melhoria de Abordagem:** Habilitar a fase offline `Format = DELIMITED` como padrão em vez de JSON para modelos que reportam capacidades fracas (`DeclaredProfile.weak`). A tabela reporta `DELIMITED` com menos falhas e melhor throughput; o runtime já tem parser, basta ser escolhido ativamente pelo model router.

3. **Rate Limits do NIM/Groq**
   **Risco:** Sem quota management local estrito, requests vão explodir 429 Too Many Requests e queimar tokens de fallback.
   **Correção de Abordagem:** Implementar `domain.QuotaManager` ou circuito no ResourceGate. Não invocar inferência se o bucket do binding correspondente já se declarou "esgotado para o minuto atual".

## Nova Correção Crítica (2026-07-18)

O mapeamento das URLs no catálogo de recursos continha `/v1` pre-pendido hardcoded nas BaseURLs ao mesmo tempo que o adapter as concatenava novamente. O `internal/provider/openai/provider.go` executava `base.Path = strings.TrimRight(base.Path, "/") + "/v1/chat/completions"`. Logo, `https://integrate.api.nvidia.com/v1` resultava em `https://integrate.api.nvidia.com/v1/v1/chat/completions`, produzindo HTTP 404.

O caminho foi corrigido para aceitar tanto a raiz do provider quanto uma BaseURL já terminada em `/v1`, sem duplicar o segmento. A correção foi verificada com testes locais e chamadas live aos adapters NIM e Groq.
