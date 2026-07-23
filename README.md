# EON — Eternal Orchestration Node

<p align="center">
  <strong>A durable, inspectable orchestration runtime for continuous, mission-driven work.</strong>
</p>

<p align="center">
  English · <a href="README.pt-BR.md">Português (Brasil)</a>
</p>

> [!IMPORTANT]
> EON is an experimental research and learning project. It is under active development and is not yet a finished, production-ready product.

## Why EON?

**EON** stands for:

- **Eternal**
- **Orchestration**
- **Node**

The name captures the project's central idea: a node designed to preserve state, coordinate bounded work, survive interruptions, and keep advancing an operator-defined mission across execution cycles.

“Eternal” does **not** mean uncontrolled, immortal, or endlessly busy. It means that continuity is a first-class runtime property: while a mission is active, EON does not silently forget accepted work or treat an empty short-term queue as global completion. It either finds the next legitimate increment, waits locally on a persisted condition, or reports an explicit continuity blocker.

## What is EON?

EON is a Go runtime for studying **durable, supervised autonomy under real operational constraints**. It coordinates missions, inquiries, operations, evidence, model calls, retries, waits, approvals, budgets, and recovery through deterministic contracts and persistent state.

The project explores a practical question:

> How much reliable, continuous progress can a deterministic orchestration system obtain from small, old, inexpensive, rate-limited, or otherwise constrained language models?

Instead of treating an LLM as an all-powerful agent, EON treats the model as a limited text solver. The kernel retains authority over scheduling, state transitions, capabilities, budgets, validation, and commits.

## Project status

EON currently contains an extensive experimental Go runtime, deterministic and durability-focused test suites, an operator control plane and dashboard, OpenAI-compatible model adapters, SQLite persistence, bounded web and file capabilities, model evaluation campaigns, observability, and experimental distributed/subagent components.

The architecture and contracts are still evolving. Interfaces, schemas, flags, and storage formats may change while the research converges.

## Core goals

- **Continuous mission-driven progress:** keep a renewable work frontier while a mission is active.
- **Durable execution:** persist accepted work, waits, retries, leases, receipts, and recovery conditions.
- **Deterministic authority:** keep official decisions and effects under kernel and policy control.
- **Weak-model resilience:** operate through a minimal `text → text` contract and degrade safely.
- **Human supervision:** allow operators to observe, pause, resume, amend, approve, and audit.
- **Bounded resource use:** make calls, tokens, concurrency, retries, time, and queue growth explicit.
- **Evidence-based development:** use reproducible tests and controlled live campaigns to find actual failure modes.
- **Provider portability:** isolate OpenAI-compatible dialect differences behind adapters and profiles.
- **Crash recovery and idempotency:** resume without losing intent or blindly duplicating effects.
- **Inspectable reasoning paths:** expose inputs, policies, decisions, outputs, validators, and receipts—not hidden chain-of-thought.

## Non-goals

EON is not intended to be:

- an unrestricted general-purpose automation agent;
- a shell controlled directly by model output;
- a system that invents its own mission, authority, or economic goals;
- a wrapper that depends on native tool calling or one specific model provider;
- a busy loop that manufactures meaningless activity to appear autonomous;
- a replacement for explicit authorization, validation, or human oversight.

## Design principles

1. **The kernel is deterministic.** Models may propose; validated code decides.
2. **State lives outside the model context.** Restart and recovery do not depend on conversational memory.
3. **Every effect crosses a typed capability boundary.** Unknown or unauthorized capabilities fail closed.
4. **Every model output is untrusted.** Raw output is preserved, normalized, validated, and converted into a proposal before it can affect canonical state.
5. **Waiting is local, not global.** One operation may wait for time, quota, an event, or approval while independent work continues.
6. **Budgets are scheduling inputs.** Limits are modeled explicitly rather than handled as improvised exceptions.
7. **The short-term agenda is renewable.** Completing a task should reveal, validate, or generate the next bounded increment.
8. **Recovery is designed, not assumed.** Leases, idempotency keys, event logs, reconciliation, and virtual clocks are part of the architecture.
9. **Model features are optional optimizations.** JSON mode, schemas, tools, streaming, and larger contexts must preserve the baseline text protocol and safe fallback.
10. **Observability must explain official behavior.** Audit records show what the system used and decided without requiring private model reasoning.

## Runtime model

A simplified operation lifecycle is:

```text
observe → select → prepare → act → verify → commit → repeat
```

The wider runtime loop is:

```text
recover
  → observe persisted state, capacity, and time
  → ingest optional external events
  → replenish and prioritize the agenda
  → dispatch bounded operations
  → validate results
  → commit accepted changes
  → update the work frontier
  → continue
```

Typical persisted operation states include:

```text
NEW → READY → RUNNING → VERIFYING → SUCCEEDED
              │             │
              ├─ WAITING_TIME
              ├─ WAITING_EVENT
              ├─ WAITING_APPROVAL
              ├─ THROTTLED
              ├─ BLOCKED_DEPENDENCY
              └─ REPLANNING / EXHAUSTED / FAILED / CANCELLED
```

An empty executable queue is not automatically interpreted as mission completion. EON applies bounded replenishment strategies or emits an explicit `CONTINUITY_BLOCKED` diagnosis.

## Architecture

```text
┌──────────────────────────────────────────────────────────────┐
│ Interfaces: CLI, Control API, dashboard, channel adapters    │
├──────────────────────────────────────────────────────────────┤
│ Control plane: missions, policies, commands, events, approval│
├──────────────────────────────────────────────────────────────┤
│ Kernel: supervisor, scheduler, state transitions, recovery   │
├──────────────────────────────────────────────────────────────┤
│ Agenda: work frontier, admission, replenishment, priority    │
├──────────────────────────────────────────────────────────────┤
│ Epistemic state: sources, observations, claims, evidence     │
├──────────────────────────────────────────────────────────────┤
│ Cognition: operation specs, prompts, models, validation      │
├──────────────────────────────────────────────────────────────┤
│ Capabilities: model, web, file, tools, clock, telemetry      │
├──────────────────────────────────────────────────────────────┤
│ Persistence: canonical state, event log, outbox, artifacts   │
└──────────────────────────────────────────────────────────────┘
```

### Main packages

```text
cmd/runtime/                    main process and CLI
internal/domain/                domain contracts and pure transitions
internal/kernel/                scheduler, execution, recovery, admission
internal/agenda/                frontier bootstrap and agenda logic
internal/mission/               mission loading, revision, and amendment
internal/provider/openai/       OpenAI-compatible model adapter
internal/prompt/                bounded prompt compilation
internal/storage/memory/        deterministic in-memory store
internal/storage/sqlite/        canonical durable MVP backend
internal/control/               operator commands and Control API
internal/dashboard/             experimental operator dashboard
internal/observability/         audit, metrics, and tracing integration
internal/evaluation/            offline and live model campaigns
internal/network/               experimental peer/network components
internal/tool/                  bounded tool executors
```

## Model integration

EON's universal cognitive contract is intentionally small:

```text
text input → text output
```

The primary adapter targets OpenAI-compatible APIs, but compatibility is treated as a profile rather than a boolean. Deployments may differ in endpoints, roles, token fields, error formats, streaming, structured output, tool calling, context limits, and usage reporting.

Optional capabilities are adopted progressively:

```text
native JSON Schema
  → JSON mode
  → delimited fields
  → closed-token response
  → short text plus parser
  → deterministic or human fallback
```

A failed optimization must not silently expand model authority, lose the operation, or duplicate an external effect.

## Persistence and recovery

The canonical MVP backend is **SQLite with an event log and logical domain commits**. The choice followed a comparative storage spike and crash testing against Dolt. Domain contracts remain backend-neutral, but SQLite currently provides the accepted operational path.

The persistence model emphasizes:

- atomic canonical changes;
- append-only audit events;
- idempotent intent and result handling;
- persisted leases and wake conditions;
- explicit reconciliation of ambiguous effects;
- restart-safe queues and waits;
- verified backup and restore procedures.

See [ADR-0003](ADRS/0003-versioned-storage.md) and the [SQLite backup runbook](RUNBOOKS/sqlite-backup.md).

## Control plane and operator supervision

The dashboard and Control API are clients of the runtime, not the runtime itself. They provide a supervised interface for:

- inspecting missions, inquiries, operations, events, budgets, and failures;
- observing current and historical execution;
- submitting typed, idempotent operator commands;
- pausing, resuming, cancelling, or amending missions;
- answering correlated operator questions;
- reviewing model calls, validation, changesets, and commits;
- following live updates without writing directly to canonical storage.

Closing the dashboard must not stop an active mission. See [CONTROL_PLANE.md](CONTROL_PLANE.md).

## Safety and authority model

EON follows a proposal-before-effect model:

```text
model or external input
  → untrusted artifact
  → parsing and normalization
  → typed proposal
  → deterministic validation
  → policy and capability authorization
  → atomic commit or explicit rejection
```

Important boundaries:

- model output cannot directly mutate canonical state;
- model output cannot grant itself capabilities;
- secrets must not be stored in prompts, commits, or diagnostic bodies;
- hostile external content is treated as data, not privileged instruction;
- shell execution is disabled unless explicitly enabled;
- file access is confined to configured roots;
- retries are bounded and effect ambiguity requires reconciliation;
- the operator remains the source of mission and policy authority.

## Requirements

- **Go 1.24 or newer**, matching `go.mod`.
- A C compiler is not required for the canonical pure-Go SQLite adapter.
- Optional provider credentials are needed only for live model campaigns or model-backed runtime operations.
- Optional infrastructure may be required for SearXNG, OTLP export, Telegram, or experimental P2P scenarios.

## Quick start

### 1. Clone and enter the repository

```bash
git clone https://github.com/Brook-sys/eon.git
cd eon
```

The active development branch may differ while the project is experimental. Check the repository's available branches if the default branch has not yet been configured.

### 2. Run the test suite

```bash
go test ./...
```

Additional verification commonly used by the project:

```bash
go test -race ./...
go vet ./...
gofmt -l .
```

Some long-running, live, crash, or provider-specific campaigns are intentionally gated by environment variables or explicit commands and are not part of the default offline suite.

### 3. Start a local in-memory runtime

```bash
go run ./cmd/runtime \
  -store=memory \
  -listen=127.0.0.1:8080
```

The experimental dashboard is enabled by default. Open:

```text
http://127.0.0.1:8080/
```

This starts the process and control surfaces, but useful autonomous execution still requires an installed mission and the capabilities needed by that mission.

### 4. Start with durable SQLite storage

```bash
mkdir -p .local

go run ./cmd/runtime \
  -store=sqlite \
  -sqlite-path=.local/eon.db \
  -listen=127.0.0.1:8080
```

Do not copy only the main SQLite file while the runtime is writing. Use the verified backup workflow documented in [RUNBOOKS/sqlite-backup.md](RUNBOOKS/sqlite-backup.md).

### 5. Enable an OpenAI-compatible model provider

Keep credentials in environment variables, never in committed files:

```bash
export EON_MODEL_API_KEY='replace-me'

go run ./cmd/runtime \
  -store=sqlite \
  -sqlite-path=.local/eon.db \
  -model \
  -model-base-url=https://your-provider.example/v1 \
  -model-name=your-model \
  -model-api-key-env=EON_MODEL_API_KEY \
  -model-context-tokens=8000 \
  -model-max-output-field=max_tokens
```

Provider behavior and supported flags vary. Read [OPENAI_COMPATIBILITY.md](OPENAI_COMPATIBILITY.md) before adding a deployment.

To inspect all runtime flags:

```bash
go run ./cmd/runtime -help
```

## Testing philosophy

EON is developed through two complementary forms of evidence.

### Deterministic verification

- unit and table-driven tests;
- reusable storage contract suites;
- crash and restart matrices;
- virtual clocks and deterministic random sources;
- fuzzing and malformed-output corpora;
- race detection and static analysis;
- bounded fake providers and replay adapters;
- architecture and dependency checks.

### Controlled live “fire tests”

Live campaigns exercise real providers—especially Groq and NVIDIA NIM—with explicit hypotheses and hard limits. Campaigns measure more than whether a request returned successfully. They examine:

- semantic and syntactic correctness;
- instruction and output-contract adherence;
- latency and token usage;
- truncation and malformed framing;
- `429` and `Retry-After` behavior;
- timeout, retry, fallback, and recovery paths;
- queue pressure, concurrency, and resource consumption;
- behavior across model sizes, families, formats, and context windows.

Results are evidence, not authority. A model response never changes official state or model preference by itself; findings must be interpreted, reproduced, and converted into validated code, policy, prompt, or observability changes.

## Documentation map

- [ARCHITECTURE.md](ARCHITECTURE.md) — architectural thesis, layers, and runtime model.
- [REQUIREMENTS.md](REQUIREMENTS.md) — normative functional and non-functional requirements.
- [TECHNICAL_REQUIREMENTS.md](TECHNICAL_REQUIREMENTS.md) — accepted technical constraints and module boundaries.
- [INVARIANTS.md](INVARIANTS.md) — authority, continuity, safety, and progress invariants.
- [FAILURE_TAXONOMY.md](FAILURE_TAXONOMY.md) — normalized failures and recovery dispositions.
- [GLOSSARY.md](GLOSSARY.md) — normative project vocabulary.
- [CONTROL_PLANE.md](CONTROL_PLANE.md) — operator API and dashboard architecture.
- [CONTINUOUS_WORK.md](CONTINUOUS_WORK.md) — legitimate continuous-work families and anti-busywork policy.
- [WEAK_MODEL_PROTOCOL.md](WEAK_MODEL_PROTOCOL.md) — protocol for constrained models and persistent microturns.
- [OPENAI_COMPATIBILITY.md](OPENAI_COMPATIBILITY.md) — portable OpenAI-compatible subset and provider profiles.
- [PROVIDER_INTEGRATION_GROQ_NVIDIA.md](PROVIDER_INTEGRATION_GROQ_NVIDIA.md) — live provider integration and evaluation guidance.
- [CONTINUOUS_DEVELOPMENT.md](CONTINUOUS_DEVELOPMENT.md) — active development record and backlog.
- [ADRS](ADRS/) — accepted architectural decisions.
- [RUNBOOKS](RUNBOOKS/) — operational procedures.

## Current architectural decisions

- [ADR-0001](ADRS/0001-go-core.md): Go is the core implementation language.
- [ADR-0002](ADRS/0002-openai-compatible-provider.md): the primary model integration is an isolated OpenAI-compatible adapter.
- [ADR-0003](ADRS/0003-versioned-storage.md): SQLite plus event log is the canonical MVP backend.

## Contributing

The project values changes that are small enough to verify and substantial enough to produce observable progress. A contribution should normally include:

1. a clearly bounded objective;
2. the relevant requirement or invariant;
3. implementation and proportionate tests;
4. evidence for failure and recovery behavior;
5. documentation updates when contracts change;
6. `git diff --check` and the applicable Go verification commands.

For model, prompt, parsing, routing, quota, or recovery changes, include controlled live evidence when credentials and provider access are available. Never commit API keys, provider secrets, raw private prompts, or sensitive artifacts.

## License

No license file is currently present. Until a license is added, no open-source license or redistribution grant should be assumed.

---

**Eternal. Orchestration. Node.**

Persistent enough to continue, constrained enough to remain under control.
