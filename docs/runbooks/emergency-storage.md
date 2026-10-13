# Runbook: Emergência de storage (falha fatal do SQLite)

> D31/ADR-0003. Quem executa a RECUPERAÇÃO é o próprio autômato (o protocolo é implementado no core); este runbook descreve o protocolo e o que o operador faz se até o protocolo falhar. Meta: perda máxima ≈ 1 janela do Ciclo 2 — nunca repouso silencioso.

## O protocolo (implementado a partir do M1; o autômato segue sozinho)

1. **Detecção**: SQLite irrecuperável (corrupção que `PRAGMA integrity_check`/replay não resolve) — o core entra em modo degradado, NÃO para.
2. **Incidente**: registro local imediato (log + incidents) + evento pós-recuperação.
3. **Storage novo**: arquivo SQLite novo criado ao lado; migrações rodam do zero.
4. **Re-bootstrap do essencial**: missão, config, calibração (cache de nível por perfil/modelo) reconstituem das fontes canônicas de config + defaults.
5. **Re-import da essência (F2)**: o export periódico versionado como markdown no repo git (`docs/knowledge/` + export de estado essencial) é re-importado — missão, lessons ativas, memórias de alta confiança. Delta incremental por janela do Ciclo 2 limita a perda.
6. **Modo degradado**: o autômato segue operando com memória reduzida; a F2 reconstrói por importância/recência à medida que reflete. Autômato NUNCA fica parado esperando humano.

## Estado do export (pré-requisito vivo desde o M2)

- Export essencial: markdown versionado, committado pelo core via outbox, a cada janela do Ciclo 2 com mudanças relevantes.
- Se o git local também foi perdido: re-clone do origin (branch `v2`) — o GitHub é o diário público; último push pendente no outbox é o pior caso.

## Se o protocolo falhar (operação manual do operador)

1. NÃO deletar nada: preservar `/data` inteiro para autópsia (cp antes de qualquer ato).
2. Confirmar sintoma: logs do core (modo degradado? loop de erro?), `incidents.log`.
3. Ação mínima: reiniciar o container (docker/Portainer) — o boot refaz detecção e tenta o protocolo de novo.
4. Se persistir: `StackUpdate` para `shell-vN` anterior (rollback de casca) + investigar `/data` manualmente.
5. Pior caso documentado: storage novo vazio → autômato recomeça missão + calibração do zero. É perda grave, **mas o autômato continua vivo** — continuidade vem antes de memória.

## Invariantes

- O operador nunca é obrigatório: todos os passos automáticos têm default definido.
- Toda ação manual vira Lesson depois (por que o automático não deu conta?).
- Falha fatal de storage é a ÚNICA parada "aceitável" da invariante 1 — e mesmo ela é incidente registrado + reinício limpo, nunca silêncio.
