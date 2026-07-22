# Análise — Fase 79

- Hipótese: frames SSE de fronteira (`ready` e `page`) precisam do mesmo espelho decimal textual exato já exigido em `event`, pois seus campos JSON numéricos também perdem precisão acima de `2^53-1` no navegador.
- Provider/modelo: NVIDIA NIM `meta/llama-3.1-70b-instruct` (rotação após Groq `llama-3.1-8b-instant` na Fase 78).
- Campanha bounded: exatamente 1 chamada, contexto 2048, teto 128 tokens de saída, timeout 30 s, sem retry.
- Resultado: HTTP/provider concluído sem erro; 220 tokens de entrada, 13 de saída, 3244 ms; zero provider errors, validações, 429 ou timeouts.
- Avaliação: 1/1 sintática e semanticamente correta, com `rule=require_exact_decimal_mirror_on_ready_and_page_before_cursor_update`; verdict observacional `QUALIFIED`.
- Comparação: o mesmo NIM 70B também produziu framing estrito na Fase 64, enquanto modelos NIM menores/mais raciocinadores tiveram variabilidade e truncamento em tarefas recentes. Esta amostra curta reforça adequação pontual do 70B para CHOICE estrito, sem promover preferência automática.
- Decisão: aceitar como evidência da regra implementada; resultados live não alteram estado nem autoridade do kernel.
- Próximo experimento: variar formato/modelo em uma tarefa que cubra integridade do payload terminal sem aumentar chamadas neste ciclo.
