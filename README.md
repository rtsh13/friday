# Friday — AI-Powered Network Telemetry Debugger

> Debug infrastructure. Not dashboards. Not tickets.

[![Dashboard](https://img.shields.io/badge/Dashboard-000000?style=for-the-badge&logo=vercel&logoColor=white)](https://tgifriday.vercel.app)
[![DocLM](https://img.shields.io/badge/DocLM-FF375F?style=for-the-badge&logo=huggingface&logoColor=white)](https://huggingface.co/ashutoshrp06/DocLM)
[![Go](https://img.shields.io/badge/Go_1.24.2-00ADD8?style=for-the-badge&logo=go&logoColor=white)](https://go.dev/)
[![Qwen2.5-Coder-3B](https://img.shields.io/badge/Qwen2.5--Coder--3B-7C3AED?style=for-the-badge&logo=alibabacloud&logoColor=white)](https://huggingface.co/Qwen/Qwen2.5-Coder-3B)

**Friday** is a production-grade, single-executable CLI tool for network engineers. It combines a fine-tuned large language model (**DocLM**), a fully local RAG pipeline, and an atomic transaction engine to diagnose complex network failures and apply fixes — with guaranteed rollback on failure. All inference runs locally. Not one byte of diagnostic data ever leaves your infrastructure.

---

## Table of Contents

- [Overview](#overview)
- [Key Features](#key-features)
- [Architecture](#architecture)
- [How It Works](#how-it-works)
- [The Four Validation Gates](#the-four-validation-gates)
- [DocLM — The Fine-Tuned Model](#doclm--the-fine-tuned-model)
- [RAG Pipeline](#rag-pipeline)
- [Variable Resolution and Output Chaining](#variable-resolution-and-output-chaining)
- [Execution Strategies](#execution-strategies)
- [Function Registry](#function-registry)
- [Technology Stack](#technology-stack)
- [Prerequisites](#prerequisites)
- [Installation](#installation)
- [First-Run Sequence](#first-run-sequence)
- [Usage](#usage)
- [Configuration](#configuration)
- [Project Structure](#project-structure)
- [Performance Targets](#performance-targets)
- [Error Handling and Recovery](#error-handling-and-recovery)
- [Security Model](#security-model)
- [Contributing](#contributing)
- [License](#license)

---

## Overview

Friday accepts natural language queries from network engineers and orchestrates a sequence of pre-defined Go functions to diagnose and remediate issues. DocLM does not generate shell commands. It does not execute arbitrary code. It selects from a fixed, whitelisted registry of 15 functions, specifies their parameters, and chains their outputs together using a typed variable resolution system.

Every destructive operation passes through four validation gates, a dry-run check, an explicit user confirmation prompt showing before/after values, and an automatic LIFO rollback mechanism if anything fails mid-transaction.

**Example session:**

```
> My gRPC stream is dropping packets, diagnose and fix

[1/4] check_tcp_health (interface=eth0, port=50051)
      ✓ ESTABLISHED — retransmits:47 — rec_buffer:6291456

[2/4] analyze_grpc_stream (port=${previous.port}, duration=10)
      ✓ drop_rate:4.6% — flow_control_events:23

[3/4] inspect_network_buffers
      ✓ rmem_max:212992 — recommended:6291456 — ⚠ too small

⚠ DESTRUCTIVE OPERATION — user confirmation required
  execute_sysctl_command
  Parameter : net.core.rmem_max
  Current   : 212992
  New       : 6291456
  Reversible: Yes (automatic rollback if failure)

Proceed? [y/N]: y

[4/4] execute_sysctl_command (snapshot saved → rollback ready)
      ✓ committed — 11.5s total

Root Cause : TCP receive buffer too small (212 KB vs recommended 6 MB)
Action Taken: Increased net.core.rmem_max to 6 MB
Monitor with: ss -ti | grep 50051

>
```

---

## Key Features

### AI-Assisted Diagnostics
- **DocLM** — a fine-tuned Qwen2.5-Coder-3B model with a LoRA adapter, served locally via vLLM, trained specifically on network telemetry debugging scenarios, gRPC failure patterns, TCP tuning runbooks, YANG model structures, and kernel parameter documentation
- Local RAG pipeline using a MiniLM-L6-v2 ONNX model compiled into the binary and Qdrant for vector search — no HTTP service, no startup latency
- Conversation history retained across queries for multi-turn debugging sessions (last 10 messages, 4,000-token window)

### Atomic Transaction Execution
- Three-phase execution engine: Read → Analyze → Modify
- State snapshots captured before every destructive operation
- LIFO rollback stack that restores the system to its exact prior state on any failure
- Dry-run validation against the live system before any change is committed

### Output Chaining
- DocLM-generated function calls reference outputs from prior functions using a typed variable syntax — `${previous.port}`, `${func[2].recommended_rmem_max}`, `${func.check_tcp_health.interface}`
- Nested field access and array indexing supported
- Smart fallback auto-injects missing parameters when a unique type-matching value exists in the execution context

### LLM Self-Healing
- On invalid variable references, Friday sends the error and the complete list of available fields back to DocLM for one corrective retry
- Typo in a field name (`${previous.reccomendation}`) gets auto-corrected if a unique match exists
- If the retry also fails, the transaction is aborted with a precise, actionable error message

### Four Validation Gates
- **Gate 1:** Input sanitization — length bounds, UTF-8 validation, injection pattern detection
- **Gate 2:** RAG retrieval quality — score threshold ≥0.7, max 5 chunks, diversity enforced
- **Gate 3:** Response validation — JSON schema, function whitelist check, parameter type validation, dependency graph analysis, anti-hallucination grounding score ≥0.6
- **Gate 4:** Pre-modify dry-run — all variables resolved, dry-run executed against the live system, full before/after preview, explicit user confirmation

### Fully Offline Operation
- No external API calls at any point in the pipeline
- MiniLM-L6-v2 ONNX embedding model embedded directly in the binary via `go:embed`
- Qdrant and vLLM run as local Docker containers
- Your network telemetry, routing topology, and internal addressing never leave your perimeter

---

## Architecture

```
User Query
    │
    ▼
Input Validation (Gate 1)
  Length bounds, UTF-8, injection pattern detection
    │
    ▼
RAG Pipeline
  Query Embedding  →  MiniLM-L6-v2 ONNX (embedded in binary)
  Vector Search    →  Qdrant (local Docker container)
  Quality Filter   →  Score ≥0.7, max 5 chunks (Gate 2)
    │
    ▼
Prompt Construction
  System prompt + Function registry (with output schemas)
  + Variable resolution rules + RAG context
  + Conversation history + Current query
    │
    ▼
DocLM Inference  (vLLM + Qwen2.5-Coder-3B + LoRA Adapter, T=0.1)
  Output: JSON with reasoning, execution_strategy, function list
    │
    ▼
Response Validation (Gate 3)
  JSON schema · Function whitelist · Parameter types
  Dependency graph · Grounding score ≥0.6
    │          │
    │     [fixable error] → DocLM retry (1x) → re-validate
    │
    ▼
Transaction Executor
    │
    ├─ PHASE 1 — READ   : Non-destructive queries
    │    check_tcp_health, analyze_grpc_stream, capture_packets …
    │    Failure here → stop cleanly, no state altered
    │
    ├─ PHASE 2 — ANALYZE : Safe CPU-bound analysis
    │    analyze_memory_leak, parse_yang_model, analyze_core_dump …
    │    Failure here → stop cleanly, no state altered
    │
    └─ PHASE 3 — MODIFY  : Destructive operations
         For each function:
           1. Dry-run validation (Gate 4)
           2. User confirmation (before/after preview)
           3. State snapshot → rollback stack push
           4. Execute with timeout + retry (max 2)
           5. SUCCESS → continue │ FAILURE → LIFO rollback
    │
    ▼
Result Aggregation + Conversation Context Update
    │
    ▼
Response Formatter
  Execution timeline · Success/warning/error indicators
  Rollback notifications · Root cause summary
```

---

## How It Works

### Phase 1 — Read (Always Safe)

All non-destructive operations execute first: TCP health checks, gRPC stream analysis, packet captures, network buffer reads. Zero mutations occur in this phase. If any read-phase function fails, the session stops cleanly with no system state altered and no cleanup required.

### Phase 2 — Analyze (Safe, CPU-Intensive)

Analysis functions run on the data collected in Phase 1: memory leak detection, core dump analysis, YANG model parsing, telemetry correlation. These are safe operations that only consume CPU. Failure stops the session cleanly.

### Phase 3 — Modify (Destructive, With Full Rollback)

Before execution, every modify-phase function goes through:

1. **Variable resolution** — all `${...}` references resolved to actual values from the execution context
2. **Dry-run validation** — executed against the live system to check permissions, parameter validity, and resource availability without making changes
3. **User confirmation** — full before/after preview shown, explicit `[y/N]` prompt required
4. **State snapshot** — current system state captured and pushed to the rollback stack
5. **Execution** — function runs with timeout enforcement and up to 2 retries on transient errors

On any failure in Phase 3, the rollback stack unwinds in LIFO order:

```
Execution order:    Rollback order:
1. Function A  →    4. Undo Function C
2. Function B  →    3. Undo Function B
3. Function C  →    2. Undo Function A
4. Function D  →    (failed — triggers rollback)
```

---

## The Four Validation Gates

No LLM output reaches the network without passing every gate. Hallucinations are caught. Type mismatches are caught. Circular dependencies are caught.

| Gate | Name | What It Checks |
|------|------|----------------|
| **Gate 1** | Input Validation | Length 5–2,000 chars · UTF-8 validity · Injection pattern detection · Sanitization |
| **Gate 2** | Retrieval Quality | Similarity score ≥0.7 · Max 5 chunks · Diversity filter |
| **Gate 3** | Response Validation | JSON schema · Function existence (whitelist) · Parameter types · Variable reference pre-check · Dependency graph · Circular dependency detection · Grounding score ≥0.6 · Safety blacklist |
| **Gate 4** | Pre-Modify | Variable resolution · Dry-run against live system · Permission check · Resource availability · User confirmation with before/after preview |

If Gate 3 detects a fixable error (e.g. a typo in a variable reference), the error and the list of available fields are sent back to DocLM for one retry. If the retry also fails, the transaction is aborted and a precise error is returned to the user.

---

## DocLM — The Fine-Tuned Model

**DocLM** is a LoRA adapter trained on top of Qwen2.5-Coder-3B, specifically fine-tuned for network telemetry debugging. It was trained on:

- Network troubleshooting runbooks (TCP, gRPC, gNMI, YANG)
- Kernel parameter documentation and tuning guides
- gRPC and OpenConfig protocol specifications
- Packet drop and flow control failure scenarios
- Multi-step diagnostic reasoning chains

DocLM outputs strict JSON with a `reasoning` block, an `execution_strategy` field, and a `functions` array where each entry references the whitelisted function registry. It does not generate shell commands. It does not reference functions outside the registry. Anti-hallucination grounding checks at Gate 3 enforce this at runtime.

**Inference configuration:**

| Setting | Value |
|---------|-------|
| Base model | Qwen2.5-Coder-3B |
| Adapter | LoRA (fine-tuned) |
| Inference server | vLLM |
| Temperature | 0.1 (deterministic) |
| Max output tokens | 2,048 |

---

## RAG Pipeline

The RAG pipeline operates entirely offline. No component makes an external network call.

**Embedding:** MiniLM-L6-v2 compiled into the binary as an ONNX model via `go:embed`. Embeddings are generated in-process with zero startup latency and no HTTP service dependency.

**Vector search:** Qdrant running as a local Docker container with a pre-indexed collection covering gRPC, TCP, gNMI, YANG, and kernel tuning documentation. Top-5 retrieval with a similarity score threshold of 0.7 and diversity filtering to prevent redundant chunks.

**Prompt assembly order:**
1. Master system prompt
2. Full function registry with parameter schemas and output schemas
3. Variable resolution rules
4. Retrieved RAG context (up to 5 chunks)
5. Conversation history (last 10 messages with execution results)
6. Current user query

**Total prompt budget:** ~4,000 tokens

**Fallback chain:**
- Qdrant unavailable → cached recent chunks used for retrieval
- ONNX model error → keyword search with degraded (dummy) embeddings
- Neither failure terminates the session

---

## Variable Resolution and Output Chaining

DocLM can reference the output of earlier functions when constructing parameters for later ones. The resolver supports three syntax forms:

| Syntax | Resolves to |
|--------|-------------|
| `${previous.field}` | Specified field from the last function's output |
| `${func[N].field}` | Specified field from the function at index N |
| `${func.function_name.field}` | Specified field from the named function |
| `${previous.nested.deep.field}` | Nested object field access |
| `${previous.array[0]}` | Array element access |

**Restrictions:** Variable references are simple field access paths only. Arithmetic, conditionals, and method calls are explicitly not permitted. If computation is needed, DocLM performs it in its `reasoning` block and passes the resolved constant value directly in the function parameters.

**Smart fallback:** If DocLM omits a required parameter but exactly one prior function output contains a field of the matching name and type, Friday auto-injects the value and logs a warning. Ambiguous matches (multiple candidates) are never auto-injected.

**Self-healing example:**

```
DocLM output:
  "value": "${previous.reccomendation}"   ← typo

Gate 3 / Dry-run detects:
  Field 'reccomendation' not found in previous function output.
  Available fields: port (integer), recommendation (string), drop_rate (float)

Sent back to DocLM for retry:
  DocLM corrects to: "${previous.recommendation}"

Retry validates → execution continues.
```

---

## Execution Strategies

DocLM selects one strategy for each function sequence. The strategy governs how failures in the sequence are handled.

| Strategy | Behaviour |
|----------|-----------|
| `stop_on_error` | Abort immediately on any failure. Default for critical diagnostic chains. |
| `skip_on_error` | Skip the failed function and all functions that declare `depends_on` it. Continue independent branches. |
| `retry_with_llm` | On failure, send the error to DocLM and request an alternative approach. |
| `ask_user` | On each failure, prompt the user: stop, skip, or retry. |

Functions marked `"critical": true` always trigger immediate rollback on failure, regardless of the selected strategy.

**Dependency declaration:**

```json
{
  "functions": [
    {
      "name": "check_tcp_health",
      "params": {"interface": "eth0", "port": 50051},
      "critical": false,
      "depends_on": []
    },
    {
      "name": "execute_sysctl_command",
      "params": {"parameter": "net.core.rmem_max", "value": "${func[2].recommended_rmem_max}"},
      "critical": true,
      "depends_on": [2]
    }
  ]
}
```

---

## Function Registry

All 15 functions are declared in `functions.yaml`. Each entry specifies: phase (`read` / `analyze` / `modify`), parameter schema with types, output schema (used by the variable resolver), timeout, reversibility, and the rollback function to call on failure.

| Function | Phase | Destructive | Description |
|----------|-------|-------------|-------------|
| `check_tcp_health` | read | No | TCP connection state, retransmit count, queue sizes, recommended buffer sizes |
| `analyze_grpc_stream` | read | No | gRPC stream monitoring — drop rate via sequence gaps, flow control event count |
| `check_grpc_health` | read | No | gRPC health check RPC with round-trip latency measurement |
| `capture_packets` | read | No | Read-only packet capture on a specified interface |
| `inspect_network_buffers` | read | No | Kernel network buffer settings from `/proc/sys/`, compared against recommended values |
| `trace_gnmi_subscription` | read | No | gNMI path subscription — streams structured telemetry updates |
| `check_interface_stats` | read | No | Interface error counters, drop counts, utilisation from `/proc/net/dev` |
| `analyze_memory_leak` | analyze | No | RSS growth tracking over a sampling window — identifies leak locations |
| `parse_yang_model` | analyze | No | Parses and validates a YANG data model against OpenConfig standards |
| `validate_yang_data` | analyze | No | Validates a gNMI update payload against a parsed YANG schema |
| `analyze_core_dump` | analyze | No | GDB batch-mode analysis — crash signal, backtrace, thread state |
| `correlate_telemetry` | analyze | No | Cross-references multiple execution context outputs to identify causal chains |
| `execute_sysctl_command` | modify | **Yes** | Kernel parameter modification via `sysctl -w` — snapshot captured, user confirmation required, auto-reversed on failure |
| `restart_service` | modify | **Yes** | System service restart — prior state captured, reversible via rollback stack |
| `restore_sysctl_value` | modify | **Yes** | Restores a kernel parameter to its snapshot-captured prior value — called exclusively by the rollback engine |

To add a new function: declare it in `functions.yaml`, implement its handler in `internal/functions/`, and register it in the dispatcher switch in `internal/executor/executor.go`. If it is a `modify`-phase reversible function, implement its rollback function and reference it in the `atomicity.rollback_function` field.

---

## Technology Stack

| Component | Technology | Purpose |
|-----------|-----------|---------|
| CLI | Go 1.24.2 + Cobra | User interface and interactive loop |
| Fine-tuned LLM | DocLM (Qwen2.5-Coder-3B + LoRA) | Function call generation |
| Inference server | vLLM | Local LLM serving |
| Embedding model | ONNX Runtime + MiniLM-L6-v2 | Local embeddings, compiled into binary |
| Vector database | Qdrant (local Docker) | Semantic search over documentation |
| Transaction engine | Custom Go | Three-phase atomicity and rollback |
| Variable resolver | Custom Go | Typed output chaining between functions |
| Snapshot manager | Custom Go | State capture and restoration |
| Configuration | Viper | User-editable config |
| Logging | Zap | Structured, leveled audit logs |
| Function registry | YAML | Declarative function definitions |

---

## Prerequisites

- Docker Desktop (or Docker Engine + Docker Compose) installed and running
- Go 1.24.2 or later (only needed when building from source)
- Linux host required for full function support (`ss`, `sysctl`, `/proc` filesystem)
- Root or `sudo` access required for destructive system functions (sysctl modification, service restarts)
- A trained LoRA adapter placed in `models/lora_adapter/` before first run

---

## Installation

**Option 1 — Pre-built binary (recommended)**

```bash
tar -xzf friday-<version>-linux-amd64.tar.gz
cd friday-<version>
./friday
```

**Option 2 — Build from source**

```bash
git clone https://github.com/<your-org>/friday.git
cd friday
go build -o friday ./cmd/friday
```

---

## First-Run Sequence

On first launch, the binary performs the following automatically:

1. Verifies Docker is available and running
2. Pulls and starts the vLLM container with the Qwen2.5-Coder-3B base model and LoRA adapter
3. Pulls and starts the Qdrant container with the pre-indexed vector database
4. Loads the embedded ONNX embedding model from the binary
5. Validates all 15 entries in `functions.yaml` against their Go implementations
6. Opens the interactive prompt

The system is ready when the `>` prompt appears. First-run startup typically takes 60–120 seconds depending on hardware while the vLLM container loads the model weights.

---

## Usage

**Start an interactive session:**

```bash
./friday
```

**Built-in commands:**

| Command | Description |
|---------|-------------|
| `help` | Show available built-in commands and example queries |
| `clear` | Clear conversation history |
| `dry-run` | Validate the next query plan without executing it |
| `exit` | Quit the session |

**Example queries:**

```
> Check TCP health on eth0 port 50051
> Analyze gRPC stream on port 50052 for 30 seconds
> My gRPC stream is dropping packets, diagnose and fix
> Inspect current kernel network buffer settings
> What was the recommended buffer size from the last check?
> Trace gNMI subscription on path /interfaces/interface/state/counters
```

Conversation history is preserved within a session. You can reference results from prior queries in follow-up questions and DocLM will chain outputs accordingly using the variable resolution system.

---

## Configuration

`config.yaml` is the primary user-editable configuration file:

```yaml
llm:
  endpoint: "http://localhost:8000"
  model: "qwen2.5-coder-3b"
  lora_adapter: "models/lora_adapter"
  temperature: 0.1
  max_tokens: 2048

rag:
  qdrant_endpoint: "http://localhost:6333"
  collection: "telemetry-docs"
  top_k: 5
  score_threshold: 0.7

conversation:
  max_messages: 10
  max_tokens: 4000

execution:
  default_timeout_seconds: 30
  max_retries: 2
```

The LoRA adapter weights must be placed in `models/lora_adapter/` before first run. The pre-indexed Qdrant vector database is included in `data/vector_db/` in the distribution package.

---

## Project Structure

```
friday/
├── friday                          # Single Go binary
├── docker-compose.yml              # vLLM and Qdrant service definitions
├── config.yaml                     # User-editable configuration
├── functions.yaml                  # Function registry (15 functions)
├── internal/
│   ├── executor/
│   │   ├── executor.go             # Function dispatcher
│   │   ├── transaction.go          # Three-phase transaction engine
│   │   ├── variables.go            # Variable resolution and output chaining
│   │   └── snapshot.go             # State snapshot and rollback manager
│   ├── functions/
│   │   ├── network/
│   │   │   ├── tcp.go              # TCP health check
│   │   │   └── grpc.go             # gRPC health check and stream analysis
│   │   ├── system/
│   │   │   ├── buffers.go          # Network buffer inspection
│   │   │   └── sysctl.go           # Sysctl modification and restoration
│   │   └── debugging/
│   │       └── core.go             # Core dump analysis
│   ├── rag/
│   │   ├── embedder.go             # ONNX-based embedding via MiniLM-L6-v2
│   │   ├── retriever.go            # Qdrant vector search
│   │   └── models/
│   │       ├── minilm-l6-v2.onnx   # Embedded model (go:embed)
│   │       └── vocab.json
│   ├── llm/
│   │   ├── client.go               # vLLM HTTP client
│   │   └── validator.go            # Response validation and grounding check
│   └── cli/
│       └── session.go              # Interactive loop and conversation manager
├── data/
│   └── vector_db/                  # Pre-indexed Qdrant storage
└── models/
    └── lora_adapter/               # DocLM LoRA weights (not included in repo)
```

---

## Performance Targets

| Metric | Target |
|--------|--------|
| End-to-end latency | < 15 seconds (simple 3-function query) |
| DocLM inference | < 3 seconds (2K output tokens) |
| Vector search | < 200 ms (top-5 retrieval) |
| Variable resolution | < 50 ms per function |
| Dry-run validation | < 2 seconds (entire modify phase) |
| State snapshot | < 500 ms per function |
| Full rollback | < 5 seconds |

---

## Error Handling and Recovery

| Scenario | Detection | Recovery |
|----------|-----------|----------|
| Invalid variable reference | Dry-run validation | DocLM retry once → fail with clear error to user |
| Function execution timeout | Timeout enforcement | Retry up to 2 times → rollback if in modify phase |
| Transient network error | Error pattern matching | Auto-retry with exponential backoff |
| Permission denied | Execution error | Fail immediately, no retry |
| Modify phase failure | Any error in modify phase | Immediate LIFO rollback, all snapshots restored |
| Rollback failure | Rollback execution error | Log all errors, warn user, request manual intervention |
| Circular dependency | Dependency graph analysis | Reject at Gate 3, send back to DocLM to fix |
| LLM hallucination | Grounding check ≥0.6 | Reject response, request regeneration |
| Qdrant unavailable | Connection error | Fall back to cached recent chunks |
| ONNX model error | Runtime error | Fall back to keyword search with degraded embeddings |

---

## Security Model

- **No arbitrary code execution.** DocLM selects only from the whitelisted function registry. The runtime cannot invoke anything outside it.
- **Whitelist enforcement at Gate 3.** Unknown function names in DocLM output are rejected before any execution attempt.
- **Input sanitization at Gate 1.** Injection patterns are detected and rejected before the query reaches DocLM.
- **User confirmation for every destructive operation.** No sysctl value is written, no service is restarted, without an explicit `y` from the user after reviewing the before/after preview.
- **Structured audit logging.** Every function execution — successful or failed — is written to a structured Zap log.
- **State snapshots as forensic artifacts.** Snapshots are retained for the duration of the session, enabling post-incident review of exactly what was changed and when.
- **Complete local operation.** No data leaves the host. All inference, retrieval, and execution is on-prem.

---

## Contributing

Contributions are welcome. Please read this section before opening a pull request.

**Getting started:**

1. Fork the repository and clone your fork
2. Create a feature branch from `main`: `git checkout -b feature/your-feature-name`
3. Make your changes
4. Run all existing tests: `go test ./...`
5. Add tests for any new behaviour
6. Open a pull request against `main` with a clear description of what was changed and why

**Code standards:**

- All code must be written in Go 1.24.2 or later
- Follow standard Go formatting (`gofmt`) — CI enforces this
- All exported functions and types must have godoc comments
- Error messages must be descriptive and actionable
- New functions added to `functions.yaml` must have a corresponding implementation in `internal/functions/` and a case in the dispatcher in `internal/executor/executor.go`

**Adding a new network function:**

1. Declare the function in `functions.yaml` with its phase, parameter schema, output schema, timeout, and atomicity metadata
2. Implement the handler in the appropriate package under `internal/functions/`
3. Register it in the dispatcher switch in `internal/executor/executor.go`
4. If the function is `modify`-phase and reversible, implement its rollback function and set the `atomicity.rollback_function` field accordingly
5. Add integration tests under `internal/functions/<package>/<function>_test.go`

Please open a GitHub Issue before starting work on a significant change to avoid duplicate effort and get early feedback on direction.

---

## License

This project is licensed under the [MIT License](LICENSE).