# Análise — Fase 82

- Hipótese: em `event` SSE com JSON parseável, `schema_version` precisa ser validado antes de o dashboard alterar seu cursor aceito; caso contrário, uma versão incompatível consegue avançar a retomada mesmo sendo rejeitada como evidência.
- Provider/modelo: NVIDIA NIM `meta/llama-3.1-70b-instruct`, rotacionado após Groq `llama-3.1-8b-instant` na Fase 81.
- Campanha bounded: exatamente 1 chamada live, contexto 2048, teto 128 tokens de saída, timeout 30 s, sem retry.
- Resultado: 222 tokens de entrada, 14 de saída, 1342 ms; zero provider errors, validações, 429 ou timeouts.
- Avaliação: 1/1 sintática e semanticamente correta, com `rule=validate_parseable_schema_before_cursor_mutation_or_evidence`; verdict observacional `QUALIFIED`.
- Comparação: confirma a evidência Groq da Fase 81 com provider/modelo distintos e framing estrito; não promove preferência nem concede autoridade ao modelo.
- Decisão: validar versão antes da mutação para payload parseável; manter a exceção explícita de JSON irrecuperavelmente malformado, cujo ID já aceito pelo EventSource é preservado e rotulado para evitar replay.
- Próximo experimento: variar formato em futura mudança cognitiva real, sem aumentar chamadas neste ciclo.
