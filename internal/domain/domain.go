// Package domain carrega os contratos puros do EON — o vocabulário mínimo
// definido em docs/plan/05-DOMAIN-AND-PERSISTENCE.md (13 conceitos,
// D04/D33/ADR-0005).
//
// Lei (ADR-0002): este pacote importa SÓ stdlib. Nenhuma dependência externa,
// nenhum I/O, nenhum tipo que referencie infraestrutura. Contratos entram aqui
// quando são compartilhados entre frentes/capabilities/processos — nunca
// antes disso.
package domain

// JobSpec é o contrato de 1 job L3: entra pelo stdin do worker, responde no
// stdout como JobResult (docs/ARCHITECTURE.md — L3 WORKERS).
//
// Argv[0] é o caminho, FORA do sandbox, de um executável ESTÁTICO (sem
// PT_INTERP). O worker o copia para o filesystem scratch, entra no sandbox
// (chroot+seccomp/namespaces) e o executa como /job com Argv[1:] — cego:
// sem rede, sem segredos, filesystem scratch (invariante 6).
type JobSpec struct {
	ID        string   `json:"id"`
	Argv      []string `json:"argv"`
	TimeoutMs int64    `json:"timeout_ms"`
}

// JobResult é a resposta do worker. Recusa é resultado LEGÍTIMO: nível de
// sandbox `none` nunca executa (fail-closed, D09) — quem chamou decide o que
// fazer com a recusa; o worker nunca "dá um jeito" de executar mesmo assim.
type JobResult struct {
	ID            string `json:"id"`
	ExitCode      int    `json:"exit_code"`
	TimedOut      bool   `json:"timed_out"`
	Refused       bool   `json:"refused"`
	RefusalReason string `json:"refusal_reason,omitempty"`
	DurationMs    int64  `json:"duration_ms"`
	StdoutFile    string `json:"stdout_file"`
	StderrFile    string `json:"stderr_file"`
	Sandbox       string `json:"sandbox"`
}

// Contratos de processo (L1/L2/L3, docs/ARCHITECTURE.md). O binário único
// re-executa a SI MESMO em papéis especiais via EON_PROCESS_ROLE — probes do
// sandbox e aplicador do sandbox do worker. Sem segundos binários, sem
// plugins, sem superfície extra.
const (
	// RoleEnv seleciona o papel do processo re-executado.
	RoleEnv = "EON_PROCESS_ROLE"
	// RoleProbe: probe de sandbox (Detect) — roda em subprocesso.
	RoleProbe = "sandbox-probe"
	// RoleWorkerChild: aplica o sandbox e faz execve do job.
	RoleWorkerChild = "worker-child"

	// EnvSandboxLevel: nível detectado no boot — injetado pelo cmd/eon no
	// supervisor, pelo supervisor no core (o core NUNCA detecta sozinho:
	// core não importa infra — ADR-0002).
	EnvSandboxLevel = "EON_SANDBOX_LEVEL"
	// EnvShellVersion: versão da casca (injetada pelo supervisor).
	EnvShellVersion = "EON_SHELL_VERSION"
)
