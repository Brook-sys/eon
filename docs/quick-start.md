# Guia de Início Rápido (Quick Start) — `motor-autonomo`

Este guia permite colocar o **`motor-autonomo`** em execução e configurá-lo através do painel do operador (*dashboard*) em menos de 5 minutos.

---

## 1. Pré-requisitos

Antes de iniciar, certifique-se de ter instalado no seu ambiente:

1.  **Go (versão 1.22 ou superior):**
    Verifique a instalação com:
    ```bash
    go version
    ```
2.  **Chave de API de um provedor de LLM:**
    Uma chave de API válida para um dos seguintes provedores suportados:
    *   **Groq:** Chave no formato `gsk_...` (obtenha em [console.groq.com](https://console.groq.com/))
    *   **NVIDIA NIM:** Chave no formato `nvapi-...` (obtenha em [build.nvidia.com](https://build.nvidia.com/))
    *   **OpenAI ou Ollama local** (se preferir rodar localmente sem chave externa).

---

## 2. Build do Projeto

Navegue até a raiz do repositório `motor-autonomo` e compile o binário do runtime usando o comando `go build`:

```bash
cd /home/node/.openclaw/workspace/motor-autonomo
go build -o runtime ./cmd/runtime
```

Isso criará o executável `./runtime` na raiz do diretório.

---

## 3. Inicializar o Runtime

Inicialize o motor ativando o painel web embutido (`-dashboard`), configurando o endereço de escuta (`-listen`), selecionando o modo de armazenamento em memória (`-store memory`) e criando uma missão demonstrativa inicial (`-mission-id`):

```bash
./runtime \
  -listen 127.0.0.1:8080 \
  -dashboard \
  -store memory \
  -mission-id mission_demo_01 \
  -mission-text "Investigar repositório e consolidar base epistemológica de conhecimento"
```

### Saída esperada no terminal:
```text
runtime starting: listen=127.0.0.1:8080 store=memory dashboard=true mission_id=mission_demo_01
[dashboard] server listening at http://127.0.0.1:8080/ (API base: /api)
[inspect] http api listening at http://127.0.0.1:8080/api/inspect/
[control] http api listening at http://127.0.0.1:8080/api/control/
[vault] http api listening at http://127.0.0.1:8080/api/vault/
```

O runtime agora está rodando e pronto para receber conexões!

---

## 4. Abrir o Painel no Browser

Abra o seu navegador de preferência e acesse:

👉 **[http://127.0.0.1:8080/](http://127.0.0.1:8080/)** *(ou `http://127.0.0.1:8080/dashboard`)*

Você verá a interface do operador com a barra lateral (*sidebar*) e o status do sistema indicando `DESCONECTADO` ou `CONECTADO` no topo superior direito.

---

## 5. Carregar uma Missão

1. No topo do painel (seção de **Contexto da Missão**), verifique o campo **`mission_id`**.
2. Ele já estará preenchido com `mission_demo_01` (ou o ID digitado no comando de inicialização).
3. Clique no botão **`Atualizar`**.
4. Clique no botão **`Conectar timeline`**.

A caixa de **Overview** exibirá os metadados da missão (`mission_demo_01`), o estado do despacho (`PAUSED` ou `ACTIVE`) e as estatísticas do runtime.

---

## 6. Configurar um Provedor de Modelo (ex: Groq ou NVIDIA NIM)

Para que o motor execute raciocínios autônomos, é necessário cadastrar um provedor de modelo e salvar a credencial de forma segura.

### Passo 6.1: Desbloquear o Cofre de Credenciais
1. Na barra lateral do painel, clique em **Provedores e modelos** (`models`).
2. Localize o bloco **Cofre de credenciais (Vault API)** no final da página.
3. No campo **`Senha mestra`**, digite uma senha com no mínimo 12 caracteres (ex.: `SenhaSeguraDoMotor123!`).
4. Clique em **`Criar ou desbloquear`**.
5. O status mudará para **`Cofre desbloqueado`**.

### Passo 6.2: Cadastrar o Provedor via Formulário
No formulário **Adicionar/Editar Provedor**, preencha os campos conforme seu provedor:

#### Exemplo A: Groq
*   **Identificador:** `groq-demo`
*   **Tipo:** `groq` (ou `openai_compatible`)
*   **Endereço da API:** `https://api.groq.com/openai/v1`
*   **Modelo:** `llama-3.3-70b-versatile`
*   **Janela de contexto:** `8192`
*   **Máximo de saída:** `512`
*   **Chave da API:** Digite sua chave `gsk_...` (será salva no cofre cifrada com AES-256-GCM).

#### Exemplo B: NVIDIA NIM
*   **Identificador:** `nvidia-nim-demo`
*   **Tipo:** `nvidia_nim` (ou `openai_compatible`)
*   **Endereço da API:** `https://integrate.api.nvidia.com/v1`
*   **Modelo:** `meta/llama-3.1-70b-instruct`
*   **Janela de contexto:** `8192`
*   **Máximo de saída:** `512`
*   **Chave da API:** Digite sua chave `nvapi-...`

### Passo 6.3: Criar Rascunho
Clique no botão **`Criar rascunho`**. O painel enviará a credencial para o Cofre e gerará um rascunho (*draft*) no escopo `MODELS`.

### Passo 6.4: Validar e Aplicar a Configuração
1. Abaixo do formulário, na seção de **Rascunhos (Drafts)**, você verá o rascunho recém-criado com status `OPEN`.
2. Clique no botão **`Validate`** no cartão do draft. O status mudará para `VALIDATED`.
3. Clique no botão **`Apply`**. O status mudará para `APPLIED` e a nova revisão de configuração passará a valer imediatamente!

---

## 7. Ver o Estado no Painel

1. Retorne à área **Visão geral** na barra lateral.
2. Na seção **Overview**, observe que o provedor ativo agora é listado em `Provider Profile`.
3. Na seção **Models / Resources / Context Pressure**, clique no botão **`Postura por binding`** para confirmar que seu modelo foi registrado e habilitado.
4. Na linha do tempo SSE (**Timeline** em **Monitoramento**), observe que novos eventos de ciclo e auditoria começam a ser transmitidos.

---

## 8. Responder a uma Pergunta do Sistema

Quando o motor autônomo encontra uma incerteza ou atinge um ponto de decisão marcado para auditoria, ele publica uma **pergunta pendente**:

1. Na área **Visão geral**, role até a seção **Perguntas Pendentes**.
2. Caso exista uma pergunta listada, clique no botão **`Responder`** do card.
3. O painel rolará automaticamente para o formulário de **Resposta Correlacionada** na área **Missão**, preenchendo o `question_id` e a `expected_revision`.
4. No campo **`texto / option_ids (CSV)`**, digite a resposta ou a opção desejada.
5. Clique em **`Enviar resposta`**.

A pergunta será resolvida e o runtime retomará o despacho da operação correspondente.

---

## 9. Próximos Passos

Agora que seu ambiente está funcionando:

*   Consulte a documentação completa em **[`docs/dashboard.md`](./dashboard.md)** para entender em detalhes cada botão, campo e endpoint da API.
*   Explore o **Navegador de Conhecimento** (área `knowledge`) para inspecionar alegações (*claims*), fontes e evidências geradas pelo motor.
*   Utilize o **Inspetor de Execução** (área `monitor`) para conferir os *ChangeSets* e a linhagem imutável de cada commit.
