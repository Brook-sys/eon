# Control Plane e Dashboard Operacional

Status: arquitetura proposta v0.1

## 1. Objetivo

O dashboard não é uma visualização decorativa do runtime. Ele é a superfície humana do plano de controle: deve permitir configurar, observar, intervir e auditar o motor sem conceder ao navegador acesso direto ao estado canônico.

Embora deva ser organizado e fácil de operar, o dashboard é antes de tudo um **instrumento experimental**. Sua função é tornar a autonomia estudável: permitir observar ciclos longos, reconstruir decisões, introduzir estímulos controlados, comparar configurações e compreender falhas. Não há requisito de aparência comercial, multi-tenancy, billing, marketplace ou outras características de produto SaaS.

A propriedade desejada é **autonomia supervisionável**:

- o runtime continua sob uma missão ativa sem depender de comandos humanos;
- o operador pode observar o que ocorreu, o que está ocorrendo e por que o próximo passo foi selecionado;
- toda intervenção humana entra como comando ou evento tipado, autorizado, persistido e auditável;
- o operador pode limitar, pausar, retomar, cancelar ou alterar a missão de forma explícita;
- nenhuma ação da interface contorna invariantes, validação, budgets ou autoridade do kernel.

O dashboard MUST NOT ser necessário para a continuidade do runtime. Reiniciar ou fechar a interface não interrompe a missão. O dashboard lê projeções e envia comandos ao runtime; não é o runtime.

## 2. Princípios

1. **Controle explícito:** toda mutação mostra intenção, escopo, impacto esperado e resultado.
2. **Observabilidade por construção:** eventos, tentativas, chamadas, validações e commits têm correlação estável.
3. **Sem escrita direta no banco:** UI e API não alteram tabelas canônicas diretamente.
4. **Configuração versionada:** mudanças produzem revisão, diff, validação e auditoria.
5. **Autonomia limitada:** missão, políticas, capabilities e budgets delimitam tudo que o runtime pode derivar e executar.
6. **Intervenção segura:** pausa, cancelamento e alteração não presumem que efeitos externos em voo deixaram de ocorrer.
7. **Redação e minimização:** segredos e conteúdo sensível não aparecem por padrão em eventos, traces ou telas.
8. **Explicação operacional, não pensamento oculto:** a interface expõe inputs autorizados, regra aplicada, alternativas registradas, decisão oficial, chamadas, outputs e validadores; não depende de cadeia de pensamento privada do modelo.
9. **Reconstrução auditável:** uma execução deve poder ser explicada a partir do estado persistido, event log, recibos e artefatos.
10. **Degradação independente:** falha da telemetria derivada ou da UI não pode corromper nem paralisar o kernel.
11. **Interação não bloqueante:** uma pergunta ao operador cria uma espera apenas para as linhas que dependem da resposta; o restante da missão continua normalmente.
12. **Perguntar com parcimônia:** o runtime só contacta o operador quando a informação possui valor decisório suficiente e não pode ser obtida com segurança por estado, fonte, regra ou alternativa reversível.

## 3. Arquitetura

```text
Browser
  │ HTTPS / local authenticated session
  ▼
Control API ───────────────► Command Inbox ─────► Kernel
  │                              │                  │
  │                              └── receipts ◄────┘
  │
  ├── read models / projections ◄── Event Log + canonical store
  ├── live stream (SSE initially)
  └── artifact access with authorization/redaction
```

### 3.1 Control API

A API possui dois caminhos separados:

- **queries:** leitura de projeções, eventos, configurações, artifacts, métricas e estado;
- **commands:** intenção de mudança submetida ao kernel com idempotência, autorização e precondições.

REST/JSON é suficiente para o MVP. Server-Sent Events é preferido inicialmente para atualização ao vivo por ser unidirecional e simples; WebSocket só deve ser introduzido se houver necessidade demonstrada de interação bidirecional de baixa latência.

### 3.2 Command Inbox

Toda ação mutável vira um `OperatorCommand` persistido antes da execução:

```json
{
  "schema_version": 1,
  "command_id": "cmd_...",
  "idempotency_key": "...",
  "actor_id": "operator_...",
  "kind": "PAUSE_MISSION",
  "target": {"mission_id": "mission_..."},
  "expected_revision": 12,
  "reason": "manutenção programada",
  "submitted_at": "..."
}
```

Estados mínimos: `RECEIVED`, `VALIDATING`, `ACCEPTED`, `REJECTED`, `APPLYING`, `APPLIED`, `RECONCILING`, `FAILED`.

O recibo informa se o comando foi aceito, rejeitado, aplicado ou se requer reconciliação. Timeout da API não autoriza reenvio cego; o cliente consulta por `command_id` ou repete a mesma `idempotency_key`.

### 3.3 External Event Inbox

Mensagens e sinais externos entram como `ExternalEvent`, nunca como instrução privilegiada:

```json
{
  "schema_version": 1,
  "event_id": "ext_...",
  "source": "operator-dashboard",
  "kind": "USER_MESSAGE",
  "mission_id": "mission_...",
  "correlation_id": "...",
  "content": {"media_type": "text/plain", "text": "..."},
  "received_at": "..."
}
```

O evento pode atualizar fatos observados, responder uma pergunta, despertar uma espera ou gerar candidata a revisão de missão. Seu texto não se torna política, configuração ou capability. O kernel determina o efeito permitido e registra a decisão.

### 3.4 Canais de comunicação

Comunicação humana usa adapters substituíveis sobre os mesmos objetos canônicos. O primeiro escopo oferece:

- **dashboard**, canal nativo e de maior fidelidade;
- **Telegram Bot API**, usando um bot configurado pelo operador.

Discord, email e outros canais ficam fora do primeiro slice, mas não exigem mudança no domínio. Canal é transporte, não identidade da pergunta nem autoridade. Configuração versionada define canais habilitados, prioridade, destinatário autorizado, classes de mensagem permitidas, limites e fallback. Tokens do bot permanecem em secret store ou variável externa e nunca são retornados pela UI, event log ou API.

Quando mais de um canal estiver habilitado, a política pode escolher um canal primário e outro de fallback. Entrega duplicada entre canais não cria duas perguntas: cada tentativa referencia o mesmo `question_id`, e a primeira resposta válida resolve o objeto canônico conforme a política.

## 4. Perguntas ao operador

### 4.1 Objeto canônico

Uma pergunta é persistida antes da entrega e independe do canal:

```json
{
  "schema_version": 1,
  "question_id": "ask_...",
  "mission_id": "mission_...",
  "mission_revision_id": "mission_revision_...",
  "inquiry_id": "inquiry_...",
  "operation_id": "operation_...",
  "revision": 1,
  "kind": "SINGLE_CHOICE_WITH_OTHER",
  "prompt": "Que estilo você prefere?",
  "context": "A escolha orientará somente a apresentação do artifact X.",
  "options": [
    {"option_id": "elegant", "label": "Elegante"},
    {"option_id": "modern", "label": "Moderno"},
    {"option_id": "minimal", "label": "Minimalista"},
    {"option_id": "cyberpunk", "label": "Cyberpunk"}
  ],
  "allow_other": true,
  "allow_clarification": true,
  "allow_skip": true,
  "blocking_scope": [
    {"kind": "ARTIFACT", "reference": "artifact_x"}
  ],
  "fallback_policy": "CONTINUE_OTHER_WORK",
  "dedup_signature": "style:artifact_x",
  "priority": 50,
  "status": "PENDING",
  "created_at": "...",
  "expires_at": "..."
}
```

A revisão da pergunta participa da correlação otimista: respostas declaram `expected_question_revision`, e perguntas terminais não aceitam nova transição.

Tipos iniciais:

- `SINGLE_CHOICE`;
- `MULTIPLE_CHOICE`;
- `SINGLE_CHOICE_WITH_OTHER`;
- `FREE_TEXT`;
- `CONFIRMATION`;
- `CLARIFICATION`.

Toda pergunta oferece, quando semanticamente aplicável:

- opções explícitas com IDs estáveis;
- resposta livre `OTHER`;
- ação `NEED_CONTEXT` para indicar incompreensão, falta de contexto ou possível inadequação da pergunta;
- ação `SKIP` ou `NO_PREFERENCE` quando a decisão admite fallback seguro;
- contexto curto explicando por que a resposta seria útil e o que ela afetará.

### 4.2 Semântica não bloqueante

Perguntar é uma operação válida, mas não altera a missão global para espera. Apenas unidades listadas em `blocking_scope` podem entrar em `WAITING_EVENT`. O scheduler continua despachando e expandindo todas as linhas independentes.

Se não houver resposta:

- trabalho independente continua;
- a pergunta permanece pendente, expira ou é substituída segundo política;
- uma linha dependente pode usar default declarado, alternativa reversível ou permanecer localmente bloqueada;
- o runtime não repete a mesma pergunta automaticamente a cada ciclo;
- ausência de resposta não causa `CONTINUITY_BLOCKED` enquanto existir qualquer trabalho independente legítimo.

Uma resposta tardia é aceita, rejeitada ou tratada como nova orientação conforme revisão, expiração e efeito já produzido. Ela nunca reverte silenciosamente trabalho oficial.

### 4.3 Correlação de resposta

`UserAnswer` deve conter `answer_id`, `question_id`, ator, conteúdo tipado, canal, identificadores de transporte, revisão esperada e timestamps. A correlação canônica é sempre `question_id`; texto livre não deve ser associado por “melhor palpite” quando houver ambiguidade.

No dashboard, cada pergunta possui formulário próprio e o POST inclui diretamente `question_id` e revisão.

No Telegram:

- opções usam inline keyboard e `callback_data` opaco, assinado ou mapeado server-side, contendo referência curta à pergunta e à opção;
- respostas livres usam reply explícito à mensagem da pergunta, idealmente com `ForceReply`;
- o adapter persiste o vínculo `chat_id + message_id → question_id`;
- callbacks são deduplicados por `callback_query.id` e mensagens por `update_id`/identificador estável;
- resposta sem reply, callback ou código inequívoco não é vinculada automaticamente se houver mais de uma pergunta pendente; o bot solicita que o usuário escolha a pergunta correta;
- `callback_data` não carrega segredo nem autoridade e respeita o limite do Telegram; o servidor revalida ator, chat, pergunta, status e opção.

A Bot API oficial oferece inline keyboards, callback queries, reply parameters e `ForceReply`, suficientes para correlação forte no primeiro adapter. Ainda assim, a mensagem e o callback são entradas não confiáveis até autenticação, deduplicação e validação local.

### 4.4 Política antispam e carga humana

O modelo pode propor uma pergunta, mas um `QuestionGate` determinístico decide se ela será criada e entregue. A proposta declara:

- decisão ou trabalho afetado;
- informação ausente;
- alternativas já tentadas;
- ganho esperado da resposta;
- custo de não perguntar;
- urgência e prazo;
- possibilidade de default ou decisão reversível;
- assinatura semântica para deduplicação.

O gate rejeita ou agrupa perguntas quando:

- a informação pode ser obtida de estado, fontes autorizadas ou regra barata;
- já existe pergunta equivalente pendente ou recentemente respondida;
- a resposta não mudaria decisão admissível;
- existe default seguro, barato e reversível com baixo custo de correção;
- o budget de interrupção humana foi consumido;
- a linha não tem prioridade suficiente;
- a pergunta é vaga, excessivamente ampla ou sem contexto.

Controles versionados incluem máximo de perguntas pendentes, taxa por janela, cooldown por assinatura e tópico, horário silencioso, agrupamento em digest, prioridade mínima, canais permitidos e política de lembrete. O padrão é sem lembretes; quando autorizados, são poucos, deduplicados e cessam após resposta, expiração ou substituição.

O núcleo inicial retorna `ADMIT`, `SUPPRESS` ou `DEFER`. Admissão não equivale a entrega: persistência da decisão, criação canônica da pergunta e publicação na outbox pertencem à fronteira transacional seguinte. Agrupamento em digest, budget persistido e lembretes continuam como incrementos posteriores.

## 5. Superfícies do dashboard

### 5.1 Visão geral

Deve mostrar, sem exigir inspeção manual de logs:

- estado do processo e versão do runtime;
- missão e revisão ativas;
- estado global: `RUNNING`, `EXPANDING`, `PAUSED`, `DEGRADED`, `STOPPING` ou `FAULTED`;
- operação atual ou estratégia de continuidade em execução;
- operação atual e duração;
- agenda por estado;
- bloqueios, aprovações e falhas recentes;
- consumo e saldo de budgets;
- providers disponíveis e condição de saúde;
- taxa de progresso e sinais de repetição/estagnação.

### 5.2 Timeline ao vivo

A timeline é derivada do event log e deve permitir filtrar por:

- missão, inquiry, operação, tentativa, chamada de modelo e commit;
- tipo e severidade do evento;
- intervalo temporal;
- sucesso, rejeição, retry, fallback ou falha;
- ator: kernel, operador, provider ou adapter.

Cada item mostra correlação e links para pai/filhos. A UI deve conseguir reconstruir uma árvore como:

```text
MissionRevision
└── Inquiry
    └── Operation
        ├── prompt compilation
        ├── model call attempt
        ├── output preservation
        ├── validation
        ├── ProposedChangeSet
        └── Commit / rejection / retry
```

### 5.3 Inspetor de execução

Para cada operação/tentativa:

- `OperationSpec` e versão;
- motivo oficial da seleção e regra de prioridade aplicada;
- read set e fatos incluídos/omitidos;
- budget previsto e efetivo;
- template e prompt compilado, conforme política de acesso;
- provider, modelo, perfil de capacidade e parâmetros não secretos;
- timestamps, latência, tokens e códigos de término;
- resposta bruta preservada, normalizada e redigida conforme política;
- resultado de cada validador;
- retries, reparos e fallback;
- changeset proposto, diff e resultado do commit;
- falhas e certeza de efeito.

O sistema não promete exibir raciocínio interno não fornecido pelo modelo. A explicação deve ser sustentada por fatos e decisões oficiais registradas pelo runtime.

### 5.4 Missão e agenda

- visualizar texto original, `MissionSpec` normalizada e histórico de revisões;
- propor alteração com diff e análise de impacto antes de ativar;
- ver inquiries, dependências, prioridades, condições de parada e próxima revisão;
- criar candidata de inquiry ou mensagem de orientação sem inserção direta em `READY`;
- repriorizar ou cancelar por comando sujeito a política;
- distinguir itens autogerados, recorrentes e criados pelo operador.

### 5.5 Configuração

Configurações editáveis devem ser schemas versionados e agrupados por escopo:

- runtime/processo;
- missão;
- scheduler e cadência;
- providers/modelos e perfis;
- budgets, rate limits e concorrência;
- aquisição de fontes;
- retenção, redaction e observabilidade;
- capabilities e aprovações;
- políticas de fallback e intervenção.
- canais de comunicação, Telegram, roteamento, quiet hours e budget de perguntas.

Toda mudança segue `draft → validate → impact preview → apply → receipt`. Campos secretos guardam apenas referência a secret store ou variável externa; a UI nunca recupera o valor existente em texto claro.

Configuração informa também sua aplicabilidade:

- `HOT`: aplicável sem interromper o ciclo;
- `NEXT_CYCLE`: aplicada na próxima fronteira segura;
- `RESTART_REQUIRED`: exige reinício coordenado;
- `IMMUTABLE`: requer nova missão, migração ou não pode ser alterada.

### 5.6 Interação, perguntas e aprovações

A interface deve permitir:

- enviar mensagem/evento externo;
- responder perguntas pendentes;
- ver por que cada pergunta foi feita, seu escopo bloqueado, validade e canal de entrega;
- responder por opção, texto livre, `outro`, `não entendi/preciso de contexto` ou `sem preferência`, quando permitido;
- aceitar ou rejeitar aprovações com motivo;
- anexar fonte autorizada;
- solicitar reavaliação antecipada;
- pausar novos despachos;
- retomar;
- cancelar inquiry/operação ou missão;
- solicitar shutdown gracioso.

`PAUSE` interrompe novos despachos, não mata automaticamente trabalho em voo. `CANCEL` registra intenção e tenta chegar a uma fronteira segura. Efeitos externos ambíguos entram em reconciliação. Um `EMERGENCY_STOP` futuro pode ser mais agressivo, mas deve documentar claramente o risco de estado externo desconhecido.

### 5.7 Conhecimento e artefatos

- navegar fontes, versões, fragmentos, observações, claims e evidências;
- visualizar proveniência e citações exatas;
- comparar commits e revisões de artefatos;
- identificar claims sem evidência, conflitos e artifacts obsoletos;
- exportar uma visão auditável sem expor segredos.

## 6. Modelo de eventos observáveis

O envelope comum deve conter ao menos:

- `event_id`, `schema_version`, `sequence`;
- `occurred_at`, `recorded_at`;
- `event_type`, `severity`;
- `mission_id`, `inquiry_id`, `operation_id`, `attempt_id` quando aplicável;
- `trace_id`, `span_id`, `parent_span_id` quando aplicável;
- `actor_type`, `actor_id`;
- `payload_ref` ou payload limitado;
- classificação de sensibilidade/redaction.

Categorias iniciais:

- lifecycle do runtime;
- scheduler e seleção;
- operação e transição;
- chamada de provider/modelo;
- capability e efeito externo;
- validação e changeset;
- commit e artifact;
- comando do operador;
- evento externo;
- pergunta, tentativa de entrega, resposta, expiração e supressão antispam;
- budget/rate limit;
- falha e reconciliação;
- configuração e revisão de missão.

O event log canônico registra fatos de domínio e controle. Traces e métricas são projeções/exportações derivadas. OpenTelemetry pode ser usado para interoperabilidade de traces, métricas e convenções GenAI, mas não substitui o log auditável nem determina semântica do kernel.

## 7. Segurança e autorização

O MVP local pode iniciar com um único operador autenticado, mas os contratos devem comportar papéis futuros:

- `VIEWER`: somente leitura redigida;
- `OPERATOR`: eventos, pausa/retomada e ações operacionais limitadas;
- `APPROVER`: decisões de aprovação;
- `ADMIN`: configuração, missão e capabilities.

Requisitos mínimos:

- bind local por padrão; exposição de rede é opt-in;
- autenticação e proteção CSRF para mutações;
- autorização no servidor, nunca somente na UI;
- auditoria de login e comandos mutáveis;
- redaction antes de persistência/exportação quando possível;
- limites de tamanho e taxa para eventos/mensagens;
- optimistic concurrency por revisão esperada;
- confirmação reforçada para mudanças de alto impacto;
- nenhuma renderização insegura de Markdown/HTML vindo de fontes ou modelos.
- allowlist de Telegram `user_id` e `chat_id`, validação do webhook quando usado e rotação do token por referência secreta;
- rejeição de resposta emitida por ator ou chat não autorizado, mesmo quando conhece um `question_id`.

## 8. Consistência e experiência ao vivo

A UI é eventualmente consistente com o kernel, mas cada comando possui recibo consultável. Uma ação não deve aparecer como concluída apenas porque o HTTP retornou sucesso de aceitação.

Estados visuais devem diferenciar:

- pedido recebido;
- comando aceito;
- aplicação em andamento;
- efeito confirmado;
- rejeitado;
- resultado desconhecido/em reconciliação.

Ao perder o stream, o cliente retoma por `last_event_sequence` e reconcilia via query. A timeline não depende de conexão contínua do navegador.

## 9. Escopo incremental

### Slice A — API de inspeção

- health/version — implementado (`inspect.API` `GET /health`, `GET /version`);
- estado da missão e scheduler — implementado em `GET /overview` e `GET /missions/{id}` (process mode, dispatch mode, agenda counts);
- agenda e operação atual — implementado (`GET /missions/{id}/operations`, summaries por estado);
- consulta paginada ao event log — implementado (`GET /events` com `after_sequence`, `limit` e filtros de correlação; `GET /events/{id}`);
- detalhe correlacionado de operação e commit — implementado (`GET /operations/{id}`, `GET /commits/{id}`, `GET /commands/{id}` via projeções somente-leitura sobre store + event log);
- residual: SSE ao vivo, redaction fina de payloads sensíveis e submit de comandos via HTTP (Slice B residual).

### Slice B — controle seguro

- command inbox idempotente — implementado (`control.CommandInbox` + store); transports só submetem;
- pause/resume/cancel/shutdown gracioso — implementados no domínio e `kernel.CommandProcessor`; scheduler usa `ControlState.AllowsDispatch` para bloquear **novo** despacho sob pause/cancel/stopping, sem matar in-flight nem impedir retomada de waits locais;
- external event/message inbox genérico — parcial (respostas de pergunta têm inbox dedicada; mensagem/despertar genéricos ainda abertos);
- respostas e aprovações — implementados no slice de perguntas;
- contratos e persistência de `OperatorQuestion`/`UserAnswer`, correlação por identidade/revisão, waits locais e núcleo determinístico do `QuestionGate` — implementados;
- persistência da decisão do gate e integração com inbox/outbox — residual;
- recibos e optimistic concurrency — implementados (`CommandReceipt` monotônico, revisão de missão esperada, `SaveControlState` com revisão monotônica);
- residual explícito: crash-replay subprocessado do processador de comandos e superfícies HTTP de submit/consulta.

### Slice C — configuração versionada

- schemas e drafts;
- validação e diff;
- aplicação por fronteira segura;
- histórico e rollback quando semanticamente suportado.

### Slice D — dashboard web

- overview;
- timeline ao vivo por SSE;
- inspetor de execução;
- missão/agenda;
- interação e aprovações;
- caixa de perguntas pendentes e formulários correlacionados;
- configuração;
- conhecimento e artifacts.

### Slice E — Telegram

- configuração segura do bot próprio do operador;
- entrega por outbox;
- inline keyboards para opções;
- reply/`ForceReply` para texto livre;
- ingestão deduplicada de updates e callbacks;
- allowlist de ator/chat;
- correlação `message_id/callback → question_id`;
- respostas tardias, expiradas, ambíguas e concorrentes.

### Slice F — interoperabilidade

- métricas e traces OpenTelemetry;
- exportador opcional para plataforma externa de observabilidade;
- alertas e políticas de retenção.

## 10. Critérios de aceitação do primeiro dashboard

O primeiro dashboard é funcional quando um operador consegue:

1. iniciar a interface sem conceder acesso direto ao banco;
2. observar uma missão ativa e a operação ou estratégia de continuidade atual;
3. acompanhar uma operação do despacho ao commit/rejeição;
4. inspecionar cada chamada de modelo com parâmetros, tokens, latência, output permitido e validação;
5. enviar uma mensagem externa e ver seu evento, decisão e efeito correlacionados;
6. pausar novos despachos e retomar sem perder trabalho;
7. alterar configuração por revisão validada e recibo;
8. responder uma aprovação pendente;
9. sobreviver a refresh/desconexão reconstruindo a timeline pela sequência persistida;
10. demonstrar que fechar ou quebrar a UI não interrompe a continuidade do kernel;
11. impedir acesso ou mutação não autorizados;
12. manter segredos fora de eventos, traces, prompts exibidos e responses de erro.
13. mostrar e responder perguntas por formulários vinculados sem bloquear trabalho independente;
14. demonstrar limite, deduplicação e agrupamento de perguntas propostas pelo modelo.

## 11. Referências de desenho

Preflight de soluções existentes indica três ideias reutilizáveis, sem adotar seus runtimes como dependência do núcleo:

- Temporal Web UI: inspeção de execução durável, estado e histórico como referência de experiência operacional;
- OpenTelemetry e convenções GenAI: interoperabilidade de traces, métricas, atributos de provider/modelo e uso de tokens;
- Langfuse: referência para navegação de traces e avaliação de chamadas de modelos.

A decisão inicial é implementar um control plane próprio e estreito sobre os contratos do motor, exportando telemetria por padrões existentes. Uma plataforma externa de observabilidade pode ser opcional, mas não deve possuir autoridade nem ser requisito para recuperação, auditoria ou continuidade.
