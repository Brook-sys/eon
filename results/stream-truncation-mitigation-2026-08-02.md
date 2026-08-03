# Erros de truncamento / stream abortado — mitigação ativa

## Sintoma observado (2026-08-02)
Múltiplas respostas minhas foram cortadas no meio e substituídas pela mensagem de retry genérica do OpenClaw ("Sinto muito, mas encontrei um erro inesperado..."). O modelo respondia, mas a resposta nunca chegava completa ao usuário.

## Causa raiz identificada
1. **Provider upstream instável**: o `9router` (proxy para Groq/NIM) não tem `timeoutSeconds` configurado. O default implícito do OpenClaw é ~120 segundos. Chamadas longas — especialmente as que geram código Templ volumoso (até ~400 linhas) — frequentemente passam de 2 minutos no backend, e o stream é abortado pelo gateway antes de completar.
2. **Respostas monolíticas**: eu estava gerando arquivos grandes (ex: `knowledge.templ` completo) num único bloco `write`. O output excedia o budget de tokens/stream, e o provedor cortava a conexão.

## Mitigações aplicadas agora
1. **`timeoutSeconds: 300`** adicionado ao provider `9router` na configuração do OpenClaw. Isso dá margem para respostas longas sem corte prematuro.
2. **Chunking de outputs grandes**: a partir de agora, vou dividir arquivos longos em múltiplos `exec` com `cat >>` heredocs em vez de `write` monolítico de uma vez. Reduz drasticamente a chance de timeout.
3. **Confirm flashes intermediários**: vou enviar mensagens de progresso curtas entre chunks ("parte 1/3 ok", etc.) para manter o stream vivo e dar visibilidade.

## Como prevenir no futuro
- Regra pessoal: nunca gerar mais de ~150 linhas de código num único `write`/`exec`. Chunkar em partes menores.
- Se o provider continuar instável, investigar se o 9router em si precisa de ajustes (proxy timeout, rate-limiting, etc.).
