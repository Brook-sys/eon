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

## 4. Superfícies do dashboard

### 4.1 Visão geral

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

### 4.2 Timeline ao vivo

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

### 4.3 Inspetor de execução

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

### 4.4 Missão e agenda

- visualizar texto original, `MissionSpec` normalizada e histórico de revisões;
- propor alteração com diff e análise de impacto antes de ativar;
- ver inquiries, dependências, prioridades, condições de parada e próxima revisão;
- criar candidata de inquiry ou mensagem de orientação sem inserção direta em `READY`;
- repriorizar ou cancelar por comando sujeito a política;
- distinguir itens autogerados, recorrentes e criados pelo operador.

### 4.5 Configuração

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

Toda mudança segue `draft → validate → impact preview → apply → receipt`. Campos secretos guardam apenas referência a secret store ou variável externa; a UI nunca recupera o valor existente em texto claro.

Configuração informa também sua aplicabilidade:

- `HOT`: aplicável sem interromper o ciclo;
- `NEXT_CYCLE`: aplicada na próxima fronteira segura;
- `RESTART_REQUIRED`: exige reinício coordenado;
- `IMMUTABLE`: requer nova missão, migração ou não pode ser alterada.

### 4.6 Interação e aprovações

A interface deve permitir:

- enviar mensagem/evento externo;
- responder perguntas pendentes;
- aceitar ou rejeitar aprovações com motivo;
- anexar fonte autorizada;
- solicitar reavaliação antecipada;
- pausar novos despachos;
- retomar;
- cancelar inquiry/operação ou missão;
- solicitar shutdown gracioso.

`PAUSE` interrompe novos despachos, não mata automaticamente trabalho em voo. `CANCEL` registra intenção e tenta chegar a uma fronteira segura. Efeitos externos ambíguos entram em reconciliação. Um `EMERGENCY_STOP` futuro pode ser mais agressivo, mas deve documentar claramente o risco de estado externo desconhecido.

### 4.7 Conhecimento e artefatos

- navegar fontes, versões, fragmentos, observações, claims e evidências;
- visualizar proveniência e citações exatas;
- comparar commits e revisões de artefatos;
- identificar claims sem evidência, conflitos e artifacts obsoletos;
- exportar uma visão auditável sem expor segredos.

## 5. Modelo de eventos observáveis

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
- budget/rate limit;
- falha e reconciliação;
- configuração e revisão de missão.

O event log canônico registra fatos de domínio e controle. Traces e métricas são projeções/exportações derivadas. OpenTelemetry pode ser usado para interoperabilidade de traces, métricas e convenções GenAI, mas não substitui o log auditável nem determina semântica do kernel.

## 6. Segurança e autorização

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

## 7. Consistência e experiência ao vivo

A UI é eventualmente consistente com o kernel, mas cada comando possui recibo consultável. Uma ação não deve aparecer como concluída apenas porque o HTTP retornou sucesso de aceitação.

Estados visuais devem diferenciar:

- pedido recebido;
- comando aceito;
- aplicação em andamento;
- efeito confirmado;
- rejeitado;
- resultado desconhecido/em reconciliação.

Ao perder o stream, o cliente retoma por `last_event_sequence` e reconcilia via query. A timeline não depende de conexão contínua do navegador.

## 8. Escopo incremental

### Slice A — API de inspeção

- health/version;
- estado da missão e scheduler;
- agenda e operação atual;
- consulta paginada ao event log;
- detalhe correlacionado de operação, tentativa, chamada e commit.

### Slice B — controle seguro

- command inbox idempotente;
- pause/resume/shutdown gracioso;
- external event/message inbox;
- respostas e aprovações;
- recibos e optimistic concurrency.

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
- configuração;
- conhecimento e artifacts.

### Slice E — interoperabilidade

- métricas e traces OpenTelemetry;
- exportador opcional para plataforma externa de observabilidade;
- alertas e políticas de retenção.

## 9. Critérios de aceitação do primeiro dashboard

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

## 10. Referências de desenho

Preflight de soluções existentes indica três ideias reutilizáveis, sem adotar seus runtimes como dependência do núcleo:

- Temporal Web UI: inspeção de execução durável, estado e histórico como referência de experiência operacional;
- OpenTelemetry e convenções GenAI: interoperabilidade de traces, métricas, atributos de provider/modelo e uso de tokens;
- Langfuse: referência para navegação de traces e avaliação de chamadas de modelos.

A decisão inicial é implementar um control plane próprio e estreito sobre os contratos do motor, exportando telemetria por padrões existentes. Uma plataforma externa de observabilidade pode ser opcional, mas não deve possuir autoridade nem ser requisito para recuperação, auditoria ou continuidade.
