# Análise — Fase 80

- Hipótese: o frame terminal SSE precisa provar que descreve exatamente o último cursor já aceito; sem esse vínculo, um payload terminal pode ser apresentado como evidência apesar de ID ausente, arredondado, regressivo ou divergente.
- Provider/modelo: Groq `llama-3.3-70b-versatile` (rotação após NVIDIA NIM `meta/llama-3.1-70b-instruct` na Fase 79; também variou o porte Groq em relação ao 8B da Fase 78).
- Campanha bounded: exatamente 1 chamada, operação `SYNTHESIZE`, formato `CHOICE`, contexto 2048, teto 128 tokens de saída, timeout 30 s, sem retry.
- Resultado: HTTP/provider concluído sem erro; 234 tokens de entrada, 70 de saída, 582 ms; zero provider errors, 429 ou timeouts e 1 erro de validação.
- Avaliação: a saída repetiu o envelope compilado (`TASK/FACTS/CONSTRAINTS/ALLOWED OUTPUTS/ANSWER`) em vez de emitir somente `rule=...`. O conteúdo do campo `ANSWER` contém exatamente a regra esperada, portanto a intenção semântica é compatível, mas o contrato estrito ficou 0/1 sintático e 0/1 semanticamente aceito pelo oracle, com verdict observacional `INCOMPATIBLE`.
- Comparação: diferente do NIM 70B da Fase 79, que obedeceu ao framing CHOICE em 13 tokens, este Groq 70B ecoou o prompt estruturado. A amostra registra sensibilidade de framing/modelo e não promove preferência automática.
- Decisão: manter a regra implementada por evidência determinística e testes; o resultado live é evidência de variabilidade do adapter cognitivo, não autoridade sobre estado ou protocolo.
- Próximo experimento: variar formato ou reforçar delimitação de resposta em uma tarefa não SSE, evitando encadear mais chamadas neste ciclo.
