# Test Plan: Habilitação de Model Presets sem API Keys Live

Como estamos bloqueados de prosseguir com chamadas live a modelos por indisponibilidade de chaves para Groq/NVIDIA NIM e também ausência de nodes do Ollama, a abordagem será prosseguir com validações via mock (probes de teste) para o componente de reload atômico MODELS no boundary de ciclo e fechar as tarefas pendentes de documentação relacionadas ao ciclo atual, declarando a suspensão controlada e atômica da fase de verificação live.

Passos planejados:
1. Criar um fake provider / model binding em testes `motor-autonomo/internal/kernel/` que simule a configuração MODELS live.
2. Concluir e detalhar o bloqueio em `CONTINUOUS_DEVELOPMENT.md`
3. Garantir que as alterações estão devidamente commiteds sem quebrar a pipeline de deploy.
