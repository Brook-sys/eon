Update: Resolve Tool Loop Constraints

Neste ciclo identificamos a necessidade de proteger o core execution agent contra tool call loops infinitos. O motor ja possuia uma mecanica de fallback para enviar o erro de validacao diretamente para o modelo via instrucao FallbackPrompt, permitindo auto-correcao. Contudo o incremento de maxCalls nao possuia nenhum teto de profundidade, podendo levar a um busy-loop esgotando o limite do provider.

1. Guard-rail Limit: Editamos internal/kernel/model_executor.go na linha de incremento do loop multi-turn. Foi adicionado um teto relativo baseado no Budget da task, com limite hardcoded de profundidade configurado em 15 chamadas extras de ferramentas. Se o modelo entrar num loop repetindo um erro ou nao convergindo em 15 iteracoes consecutivas, a execucao falha emitindo error.
2. Validacao: Revisados internal/tool/dispatcher.go e testes confirmando que falhas de JSON, validacoes de schema e ferramentas ausentes enviam os prompts correspondentes de fallback para auto-correcao.
3. Atualizado o tracking no CONTINUOUS_DEVELOPMENT.md.
