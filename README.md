# Logos

**Logos** is a distributed log service prototype implemented in Go, initially developed by following the architecture and implementation approach presented in Travis Jeffery's *Distributed Services with Go*.

The project has since evolved beyond a book implementation into a practical systems-engineering and research-readiness exercise focused on:

* persistent storage abstractions
* record and index management
* segment lifecycle and recovery
* append and failure semantics
* gRPC service communication
* TLS/mTLS and authorization
* observability
* automated testing
* reproducible systems experiments
* documenting design decisions and limitations

Logos is a **research-oriented prototype, not a production-ready distributed storage system**.

---

## 1. Overview

At a high level, Logos separates the logical distributed-log service from the physical storage implementation:

```text
                         ┌──────────────────┐
                         │      Client      │
                         └────────┬─────────┘
                                  │
                           gRPC + mTLS
                                  │
                         ┌────────▼─────────┐
                         │      Server      │
                         │ Auth / gRPC /     │
                         │ Observability    │
                         └────────┬─────────┘
                                  │
                         ┌────────▼─────────┐
                         │       Log        │
                         │ Logical offsets  │
                         │ Segment routing  │
                         │ Recovery         │
                         └────────┬─────────┘
                                  │
                         ┌────────▼─────────┐
                         │     Segment      │
                         │ Lifecycle /      │
                         │ coordination     │
                         └───────┬─┬────────┘
                                 │ │
                    ┌────────────┘ └────────────┐
                    │                           │
             ┌──────▼──────┐             ┌──────▼──────┐
             │    Store    │             │    Index    │
             │ Record data │             │ Offset →    │
             │ Append/read │             │ file pos.   │
             └──────┬──────┘             └──────┬──────┘
                    │                           │
                    └────────────┬──────────────┘
                                 │
                         ┌───────▼────────┐
                         │ Filesystem /   │
                         │     Disk       │
                         └────────────────┘
```

The principal storage path is:

```text
Client
  ↓
gRPC service
  ↓
Log
  ↓
Segment
  ↓
Store + Index
  ↓
Filesystem
```

Recovery operates across the persisted log state to restore the logical state of the service after restart.

---

# 2. What Is Implemented

## Persistent Log Storage

Logos implements an append-oriented persistent storage path consisting of:

* `Log`
* `Segment`
* `Store`
* `Index`
* `Recovery`

Records are serialized before being persisted to the Store.

The physical Store uses a length-prefixed record format:

```text
┌──────────────────────┬────────────────────────────┐
│ Record Length (8 B)  │ Serialized Record          │
│ Big Endian           │ Protobuf payload           │
└──────────────────────┴────────────────────────────┘
```

The segment index maps logical offsets to physical positions in the Store:

```text
Logical Offset ─────────► Physical File Position
```

The current index entry is composed of:

```text
4-byte offset + 8-byte file position
```

The implementation therefore separates:

* logical record identity
* physical record storage
* physical lookup
* segment lifecycle
* log-level offset management

---

## Record Metadata

The current record contains:

* payload
* logical offset
* checksum
* producer identifier
* creation timestamp

The protobuf definition is located under:

```text
proto/logs/v1/record.proto
```

Generated Go code is located under:

```text
api/logs/v1/
```

---

## Segment Management

The Segment layer coordinates:

* Store
* Index
* segment lifecycle
* segment capacity
* record append operations
* segment-local indexing

The Log layer manages the collection of segments and provides the higher-level logical log abstraction.

---

## Recovery

Recovery is implemented separately from the normal append path under:

```text
internal/recovery/
```

The recovery implementation addresses restoration of log state across persisted segment boundaries.

Research documentation related to recovery and durability is maintained under:

```text
docs/research/
```

including:

```text
DURABILITY-AND-TEST-STATUS.md
FAILURE-TESTS.md
STORAGE-INVARIANTS.md
```

---

# 3. Storage and Durability Boundary

Logos currently uses buffered file I/O.

This distinction is important:

> A successful write to the application buffer, or a successful buffer flush, is not equivalent to an `fsync`-level durability guarantee.

The current implementation therefore should not be interpreted as guaranteeing survival across every possible:

* process failure
* operating-system failure
* filesystem failure
* hardware failure
* power-loss event

The repository documents this limitation explicitly rather than treating buffered writes as equivalent to durable storage.

This distinction is part of the project's research and systems-engineering analysis.

---

# 4. gRPC Service

Logos exposes its service through gRPC.

The protobuf definitions are located under:

```text
proto/logs/v1/
```

Generated Go bindings are located under:

```text
api/logs/v1/
```

The service implementation is located under:

```text
internal/server/
```

The service supports the implemented log operations, including:

* producing records
* consuming records
* streaming operations
* offset-based consumption
* offset boundary handling
* gRPC error propagation
* client cancellation handling

The server and client entry points are:

```text
cmd/server/main.go
cmd/client/main.go
```

---

# 5. Security

Logos includes transport security and service authorization.

## TLS / mTLS

The authentication configuration is located under:

```text
auth/
```

Certificate generation is automated through the Makefile using:

* CFSSL
* CFSSLJSON

The certificate setup generates credentials for:

* the Certificate Authority
* the server
* the client
* an intruder client used for authorization/security testing

The generated certificate material is moved to the user's local Logos configuration directory.

---

## Authorization

Authorization configuration consists of:

```text
model.conf
policy.csv
```

These files are used by the authorization layer under:

```text
auth/
```

The repository initially stores them under:

```text
auth/files/
```

However, the Makefile expects these files at the project root when the certificate/configuration setup is performed.

Therefore, before running the setup commands, copy:

```text
auth/files/model.conf
auth/files/policy.csv
```

into the **repository root**.

After `make gencert`, the Makefile moves the generated `.pem`, `.csr`, `.csv`, and `.conf` files into the user's `.proglog` directory.

On Windows:

```text
%USERPROFILE%\.proglog
```

On Linux/macOS:

```text
~/.proglog
```

This keeps runtime configuration and credentials outside the source tree.

---

# 6. Observability

Logos uses OpenTelemetry-based observability.

The observability implementation is located under:

```text
internal/observability/
```

including:

```text
internal/observability/
├── docker-compose.yml
├── otel-collector-config.yaml
├── prometheus.yml
├── logging.go
├── metric.go
└── tracing.go
```

The project uses structured logging and OpenTelemetry instrumentation for the service.

The local observability environment is provided through Docker Compose.

Start it with:

```bash
make docker-comp-up
```

Stop it with:

```bash
make docker-comp-down
```

The exact local observability endpoints are defined by:

```text
internal/observability/docker-compose.yml
internal/observability/otel-collector-config.yaml
internal/observability/prometheus.yml
```

---

# 7. Testing

Testing is performed at multiple layers.

The repository contains tests covering areas including:

* Store
* Index
* Segment
* Log
* Recovery
* TLS/configuration
* Authorization
* Server behavior
* Observability

Run the normal Go test suite with:

```bash
go test ./...
```

Run tests with verbose output:

```bash
go test -v ./...
```

Run the race-enabled suite:

```bash
go test -race ./...
```

The Makefile's `test` target also runs:

```bash
go test -race ./...
```

Therefore:

```bash
make test
```

is the project's race-enabled test command.

### Windows note

Race-enabled testing may encounter Windows environment-specific process execution problems such as temporary executable access errors. Such an error is different from an actual Go race detector report.

When this occurs, the normal test suite should be run separately to distinguish implementation failures from the local test environment.

---

# 8. Research Documentation

Logos maintains a separate research documentation trail under:

```text
docs/research/
```

Current material includes:

```text
docs/research/
├── BASELINE.md
├── DURABILITY-AND-TEST-STATUS.md
├── FAILURE-TESTS.md
├── STORAGE-INVARIANTS.md
└── experiments/
```

The purpose of this material is to document:

* system assumptions
* correctness invariants
* failure scenarios
* durability boundaries
* test evidence
* experimental protocols
* measured results
* limitations
* research questions

This separates **engineering implementation** from **research claims**.

---

# 9. Design Decisions

Important architectural decisions are recorded under:

```text
docs/design-decisions/
```

Current decision records include:

```text
DD-001-record-metadata.md
DD-002-physical-record-storage-and-append-failure.md
DD-003-segment-index-design.md
DD-004-segment-coordination-and-commit.md
DD-005-log-review-audit.md
```

Each decision record is intended to connect:

```text
Problem
   ↓
Book / conventional architecture
   ↓
Underlying assumptions
   ↓
Logos decision
   ↓
Implementation
   ↓
Tests / evidence
   ↓
Consequences
   ↓
Limitations / open questions
```

The goal is not simply to document what the code does, but why the system is structured that way and what assumptions the design makes.

---

# 10. Research Experiment: EXP-001

The project includes a reproducible storage experiment investigating the effect of Store write-buffer size on append performance.

The experiment is located under:

```text
docs/research/experiments/EXP-001-store-buffering/
```

The experiment evaluates:

```text
Config.Store.BufferBytes
```

using fixed 1 KiB records and the following buffer configurations:

```text
0 B
4 KiB
16 KiB
64 KiB
256 KiB
```

The primary measurement is payload throughput.

Secondary measurements include:

* nanoseconds per operation
* bytes allocated per operation
* allocations per operation

The experiment also includes independent correctness verification through append/read-back testing.

The experiment records:

```text
EXP-001.md
protocol.md
environment.md
RUN-EXPERIMENT.md
analysis/
results/
```

The benchmark is deliberately limited to the Store append path. It is therefore **not an end-to-end distributed-system performance evaluation**.

The results should be interpreted as evidence about the measured workload and environment rather than as a universal storage-buffer recommendation.

---

# 11. Project Structure

```text
logos/
├── api/
│   └── logs/
│       └── v1/
│           ├── record.pb.go
│           ├── service.pb.go
│           └── service_grpc.pb.go
│
├── auth/
│   ├── Auth Setup Runbook.odt
│   ├── authorization_test.go
│   ├── authorizer.go
│   ├── ca-config.json
│   ├── ca-csr.json
│   ├── client-csr.json
│   ├── server-csr.json
│   └── files/
│       ├── model.conf
│       └── policy.csv
│
├── cmd/
│   ├── client/
│   │   └── main.go
│   └── server/
│       ├── main.go
│       └── store_test_files/
│
├── docs/
│   ├── design-decisions/
│   │   ├── DD-001-record-metadata.md
│   │   ├── DD-002-physical-record-storage-and-append-failure.md
│   │   ├── DD-003-segment-index-design.md
│   │   ├── DD-004-segment-coordination-and-commit.md
│   │   └── DD-005-log-review-audit.md
│   │
│   └── research/
│       ├── BASELINE.md
│       ├── DURABILITY-AND-TEST-STATUS.md
│       ├── FAILURE-TESTS.md
│       ├── STORAGE-INVARIANTS.md
│       └── experiments/
│
├── internal/
│   ├── config/
│   ├── core/
│   ├── log/
│   ├── observability/
│   ├── recovery/
│   ├── segment/
│   ├── server/
│   └── util/
│
├── proto/
│   └── logs/
│       └── v1/
│
├── Makefile
├── buf.gen.yaml
├── buf.lock
├── buf.yaml
├── go.mod
├── go.sum
└── README.md
```

---

# 12. Prerequisites

The project requires:

* Go
* Docker
* Docker Compose
* Buf
* CFSSL
* CFSSLJSON
* golangci-lint for the `format` target

The exact versions used for reproducible experiments should be recorded in the corresponding experiment environment document.

---

# 13. Local Development

## Step 1 — Clone the repository

Clone the repository and change into its directory:

```bash
git clone <repository-url>
cd logos
```

---

## Step 2 — Prepare authorization configuration

The repository contains the initial authorization files here:

```text
auth/files/model.conf
auth/files/policy.csv
```

The Makefile's certificate/configuration setup expects `.conf` and `.csv` files in the project root before they are moved to the user's local configuration directory.

Copy these files into the repository root.

### Windows PowerShell

```powershell
Copy-Item auth\files\model.conf .
Copy-Item auth\files\policy.csv .
```

### Linux/macOS

```bash
cp auth/files/model.conf .
cp auth/files/policy.csv .
```

After setup, these files will be moved into the user's `.proglog` directory by `make gencert`.

---

## Step 3 — Initialize the local configuration directory

Run:

```bash
make init
```

This creates the local Logos configuration directory.

On Windows:

```text
%USERPROFILE%\.proglog
```

On Linux/macOS:

```text
~/.proglog
```

---

## Step 4 — Prepare Go and Buf dependencies

Run:

```bash
make install-tools
```

This currently performs:

```bash
go tool buf dep update
go mod tidy
```

This target manages the project's Go and Buf dependencies. It does not install external executables such as CFSSL or golangci-lint.

---

## Step 5 — Generate certificates and move configuration

After the authorization files have been copied into the project root, run:

```bash
make gencert
```

This generates:

```text
CA
Server certificate
Client certificate
Intruder-client certificate
```

and moves generated:

```text
*.pem
*.csr
*.csv
*.conf
```

files into the local `.proglog` configuration directory.

---

# 14. Run Logos Locally

## Start the observability stack

If observability services are required:

```bash
make docker-comp-up
```

---

## Start the server

Open a terminal and run:

```bash
make start-server
```

This executes:

```bash
cd ./cmd/server && go run main.go
```

---

## Start the client

In another terminal:

```bash
make start-client
```

This executes:

```bash
cd ./cmd/client && go run main.go
```

---

## Stop the observability stack

When finished:

```bash
make docker-comp-down
```

---

# 15. Makefile Commands

The Makefile is the authoritative source for the project's local-development commands.

| Command                 | Purpose                                             |
| ----------------------- | --------------------------------------------------- |
| `make init`             | Create the local `.proglog` configuration directory |
| `make install-tools`    | Update Buf dependencies and Go modules              |
| `make gencert`          | Generate certificates and move configuration files  |
| `make test`             | Run the Go test suite with the race detector        |
| `make start-server`     | Start the Logos server                              |
| `make start-client`     | Start the Logos client                              |
| `make docker-comp-up`   | Start the local observability stack                 |
| `make docker-comp-down` | Stop the local observability stack                  |
| `make format`           | Run golangci-lint with automatic fixes              |
| `make lint-proto`       | Run Buf protobuf linting                            |
| `make generate-proto`   | Generate Go code from protobuf definitions          |

The `lint-breaking` target is intended for protobuf compatibility checking but currently contains a repository/branch placeholder and should be configured before being used as a project compatibility gate.

---

# 16. Protobuf Development

Protocol definitions are located under:

```text
proto/logs/v1/
```

Run protobuf linting with:

```bash
make lint-proto
```

Generate Go protobuf code with:

```bash
make generate-proto
```

Generated files are written under:

```text
api/logs/v1/
```

---

# 17. Code Formatting

Run:

```bash
make format
```

This invokes:

```bash
go tool golangci-lint run --fix -v ./...
```

The command therefore performs linting and automatic fixes rather than merely running `gofmt`.

---

# 18. Reproducibility

A major goal of Logos is that implementation claims should be traceable to evidence.

The repository therefore separates:

```text
Source code
    ↓
Tests
    ↓
Design decisions
    ↓
Research documentation
    ↓
Experiments
    ↓
Measured results
```

For experiments, the repository records:

* research question
* independent variable
* workload
* controls
* benchmark command
* environment
* repetitions
* raw results
* summarized results
* analysis
* limitations

This allows an experiment to be rerun without relying solely on the author's interpretation of the result.

---

# 19. Current Research Position

Logos is deliberately **not presented as a novel distributed-storage research contribution**.

Its purpose at the current stage is to provide a concrete systems artifact through which to demonstrate the ability to:

1. understand an existing distributed-system architecture;
2. identify its underlying hardware and systems assumptions;
3. implement and modify those abstractions;
4. reason about correctness and failure;
5. design controlled experiments;
6. collect reproducible evidence;
7. distinguish measured behavior from guarantees;
8. document limitations and unresolved questions.

The project therefore serves as an engineering and research-readiness artifact rather than as a claim that the underlying storage architecture itself is novel.

---

# 20. Known Limitations

Logos currently has several deliberate or known limitations.

### Durability

Buffered writes are not equivalent to `fsync`-level durability.

### Replication

The current project should not be described as a replicated, fault-tolerant distributed storage system.

### Consensus

Logos does not currently implement a distributed consensus protocol such as Raft or BFT consensus.

### Benchmark scope

EXP-001 measures the Store append path under a controlled workload. It does not establish:

* cluster-wide performance;
* network performance;
* concurrent client performance;
* replicated-storage performance;
* failure-recovery performance;
* end-to-end gRPC latency;
* production workload performance.

### Environment dependence

Benchmark results are dependent on the tested:

* CPU
* operating system
* filesystem
* storage device
* Go runtime
* workload
* benchmark configuration

They should therefore not be interpreted as universal performance characteristics.

### Production readiness

Logos is a research-oriented prototype and is not presented as production-ready infrastructure.

---

# 21. Research and Engineering Evidence

The project maintains three complementary forms of evidence.

### Design evidence

```text
docs/design-decisions/
```

Documents architectural decisions and their consequences.

### Correctness evidence

```text
docs/research/
```

Documents invariants, failure scenarios, durability boundaries, and test status.

### Experimental evidence

```text
docs/research/experiments/
```

Documents controlled measurements and their results.

Together, these provide a traceable path from:

```text
Design assumption
       ↓
Implementation
       ↓
Test
       ↓
Experiment
       ↓
Observed behavior
       ↓
Limitation
       ↓
Research question
```

---

# 22. Status

**Current status: Research-oriented systems prototype**

The project has progressed from following an existing distributed-service implementation to independently examining:

* storage abstractions
* segment and index design
* append semantics
* failure handling
* recovery
* service security
* observability
* reproducible experimentation

The project intentionally separates what has been **implemented**, what has been **tested**, what has been **measured**, and what remains an **open research question**.

---

# 23. Author

**Chukwudi Christian Okolo**

B.Eng. Electronic Engineering
University of Nigeria, Nsukka

Logos is maintained as part of the author's preparation for graduate research in computer systems and distributed systems.
