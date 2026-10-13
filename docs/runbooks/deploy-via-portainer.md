# Runbook: Deploy da casca via Portainer

> Obrigatório desde o M1 (D40/D50). O autômato **não participa** deste fluxo: Portainer e GHCR pertencem à camada-casca imutável. Quem executa é o operador (ou o CI com credenciais que o autômato nunca vê).

## Pré-requisitos

- [ ] Workflow `shell-image.yml` já publicou `ghcr.io/brook-sys/eon:shell-vN` no GHCR (workflow manual, D47/D54)
- [ ] Credenciais Portainer acessíveis ao operador (fora do repositório)
- [ ] Backup mental do estado atual: qual `shell-vN` está rodando agora (para rollback)

## Fluxo de deploy

1. **Publicar a imagem**: GitHub Actions → workflow `Shell image` → `Run workflow` com input `N` → aguardar a tag `shell-vN` aparecer no GHCR. Tags são imutáveis (D54): nunca regrave uma tag existente.
2. **Repontar o stack**: Portainer → Stack `eon` (id 52, ambiente `local` id 3) → editar `deploy/stack-eon.compose.yml` → imagem `ghcr.io/brook-sys/eon:shell-vN` → **Update the stack** com *re-pull image* habilitado.
3. **Verificação FORA-DE-BANDA (nunca pule)** (D50):
   - [ ] Container `eon_eon` (ou `eon` do stack) está **running** (não restarting)
   - [ ] Health status **healthy** (healthcheck `eon health`; se starting, aguardar `start_period` de 30s)
   - [ ] `docker logs` do container: linha de boot do supervisor + detecção de sandbox + "core spawned" — sem panic
   - [ ] Sidecar `eon-watchdog` running (autoheal)
   - [ ] `restart count` do container eon = 0 desde o deploy
   - [ ] Heartbeat avançando (`/data/supervisor/health.json` atualizando; ver via `docker exec` ou logs)
4. **Registrar**: entrada curta no journal (`docs/journal/`) com `N`, SHA da imagem e resultado da verificação.

## Rollback da casca (D54)

1. Portainer → Stack `eon` → repontar a imagem para `shell-v(N-1)` → Update com re-pull.
2. Mesma verificação fora-de-banda do fluxo normal.
3. Incidente no journal: por que voltou, o que o `shell-vN` fez de errado.

## Troubleshooting

| Sintoma | Diagnóstico | Ação |
|---|---|---|
| Container restarting em loop | `docker logs`: primeiro panic/crash do supervisor | Rollback `N-1`; a casca NUNCA se recupera sozinha (é imutável por desenho) |
| Health `unhealthy` mas processo vivo | heartbeat parou >60s | Investigar `/data` (volume montado?); rollback se persistir |
| Sidecar reiniciando o eon sem unhealthy visível | label `autoheal` escopado errado | Conferir compose: label só no serviço eon |
| Imagem não puxa | GHCR privado/credencial | Registry access no ambiente Portainer; a tag existe? |
| Volume sumiu | stack recriado sem o volume `eon-data` | NUNCA remover o volume; se perdeu, emergency-storage runbook |

## Invariantes deste runbook

- Nenhum passo depende do autômato estar vivo (casca muda com o autômato morto, de propósito).
- Nenhum segredo em texto: env vars do stack, mascaradas pelo Portainer.
- Deploy sem verificação fora-de-banda **não é deploy** — é aposta.
