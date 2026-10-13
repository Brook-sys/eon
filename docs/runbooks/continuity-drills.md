# Runbook: Chaos drills de continuidade via Portainer

> D40/D51. Executados pelas ferramentas Portainer a partir do M5; a primeira passada completa roda no aceite do M5. Cada drill tem resultado registrado no journal. Objetivo: provar empiricamente a invariante 1 — nunca para.

## Regras do jogo

- Drills rodam no ambiente `local` (Portainer), no stack `eon` — nunca em outro stack.
- Cada drill: estado observado ANTES, ação, estado observado DEPOIS (tempos de recuperação), veredito.
- Vermelho = o autômato não recuperou sozinho (violação de invariante a investigar).
- Drills são apostas seguras: se falharem, o docker restart + supervisor + sidecar ainda existem como camadas de defesa.

## Drills

### D1 — Matar o core (SIGKILL no processo)
1. Confirmar estado: health verde, heartbeat avançando.
2. `docker exec <container> kill -9 <pid do core>` (core NÃO é PID 1).
3. **Esperado**: supervisor detecta, reinicia core com backoff; heartbeat retoma; zero downtime do container; `restarts` do core +1; evento em `incidents.log`.
   - Verificado no primeiro deploy: reviveu em ~2s (pid 21 → 415), `core_restarts` 0→1, heartbeat retomou. `incidents.log` fica vazio — só anomalias de exit-0-sem-pedido geram linha; morte por sinal conta no `core_restarts` (by design).

### D2 — Matar o supervisor (o container inteiro morre)
> Corrigido empiricamente no primeiro deploy (2026-10-13). Três variantes com resultados DIFERENTES — todas lei do Docker/kernel, todas verificadas:
1. **`kill -9 1` de DENTRO (exec): NO-OP.** O kernel blinda o PID 1 contra sinais fatais vindos do próprio namespace — o supervisor é imortal dentro do próprio container. Não serve de drill.
2. **`docker kill` (API/Portainer): parada manual RESPEITADA.** O restart policy `unless-stopped` não traz de volta o que o operador parou de fora — é interferibilidade funcionando, não falha de continuidade. Traga de volta com `docker start` (Portainer: container → Start).
3. **`kill -15 1` de DENTRO (exec): o drill certo.** Supervisor trata SIGTERM → shutdown limpo (exit 0, zero órfãos) → Docker reinicia automaticamente (restarts +1, boot novo) → health verde. **Esperado**: recuperação total sem intervenção; `restarts` do container +1; novo `boot` no health.json; zero crash_loops.

### D3 — Crash-loop do binário promovido
1. Corromper o artefato `vN` atual (ex.: truncar o binário em `/data/artifacts/eon/vN/eon`).
2. **Esperado**: supervisor re-VERIFICA SHA-256, rejeita, demote para `vN-1` ou casca; container nunca restarta por causa disso; incidente registrado; sistema vivo.

### D4 — Stack stop/start (Portainer)
1. Portainer → Stack `eon` → Stop → aguardar 60s → Start.
2. **Esperado**: mesmo comportamento do D2 para ambos os serviços (eon + watchdog); volume `eon-data` preserva TODO o estado; boot re-attacha.

### D5 — Rede/modelo indisponível (degradação)
1. Bloquear saída HTTP do container (ou usar um modelo key inválida temporariamente).
2. **Esperado**: autômato NÃO para — degrada para trabalho determinístico (sleep-time compute, D12/D22), registra THROTTLED; fila continua movendo.

### D6 — Falha fatal de storage
1. Este drill usa `docs/runbooks/emergency-storage.md` com um storage de sacrifício.
2. **Esperado**: incidente registrado, re-bootstrap, modo degradado, nunca repouso silencioso.

## Registro

Cada execução vira entrada no journal (`docs/journal/`): drill, data, veredito (verde/vermelho), tempo de recuperação, lição. Vermelho gera Issue + Lesson — nunca fica sem análise (invariante 3).
