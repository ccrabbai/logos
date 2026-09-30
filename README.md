# Logos

**Logos** is a distributed log service prototype implemented in Go, initially developed by following the architecture and implementation approach presented in Travis Jeffery's *Distributed Services with Go*.

The project has since evolved beyond the book's implementation into a practical systems-engineering exercise focused on **storage abstractions, failure semantics, distributed-service communication, security, observability, and testing**.

Logos is primarily a **research-oriented prototype and learning project** rather than a production-ready distributed storage system.

## Architecture

```text
                         Client
                           │
                        gRPC/mTLS
                           │
                           ▼
                    ┌─────────────┐
                    │   Server    │
                    └──────┬──────┘
                           │
                           ▼
                       ┌───────┐
                       │  Log  │
                       └───┬───┘
                           │
                     ┌─────▼─────┐
                     │  Segment  │
                     └─────┬─────┘
                           │
                 ┌─────────┴─────────┐
                 ▼                   ▼
             ┌────────┐          ┌────────┐
             │  Store │          │  Index │
             └────┬───┘          └────┬───┘
                  │                   │
                  └─────────┬─────────┘
                            ▼
                           Disk
```

The architecture separates the log service into layers with distinct responsibilities:

* **Server** — exposes the log API over gRPC.
* **Log** — coordinates append and read operations.
* **Segment** — manages logical portions of the log.
* **Store** — persists serialized records.
* **Index** — maps logical record offsets to physical file positions.
* **Disk** — provides persistent storage.

This separation allows individual storage and service abstractions to be examined independently and as part of the overall system.

## What I Implemented

### Persistent Log Storage

Logos implements an append-oriented persistent log with:

* Append-only record storage.
* Segmented log structure.
* Record serialization and deserialization.
* File-backed persistent storage.
* Offset-to-file-position indexing.
* Segment creation and management.
* Append and read semantics across the storage layers.
* Recovery-related handling for persistent log state.

The storage implementation deliberately separates logical record management from physical persistence and indexing.

### gRPC Service

Logos exposes its log functionality through a gRPC service supporting:

* Producing records.
* Consuming records.
* Streaming production.
* Streaming consumption.
* Offset boundary handling.
* gRPC error propagation.
* Client cancellation.

The streaming consumer continuously consumes records from the requested offset while respecting client cancellation.

### Security

The service communication layer is secured using:

* TLS.
* Mutual TLS (mTLS).
* Certificate-based client and server authentication.
* An internally generated certificate authority.
* Service-level authorization controls.

Certificate generation is automated through the project's Makefile.

### Observability

Logos uses **OpenTelemetry** for service observability.

```text
                         Logos
                           │
             ┌─────────────┼─────────────┐
             │             │             │
          Traces        Metrics      Structured Logs
             │             │             │
             └─────────────┼─────────────┘
                           ▼
                OpenTelemetry Collector
                       │          │
                       ▼          ▼
                    Jaeger    Prometheus
```

Structured application logging uses Go's `slog`.

The local observability environment is provided through Docker Compose.

### Testing

The project contains tests across the storage and service layers, including:

* Store persistence and reads.
* Index behavior.
* Segment coordination.
* Log behavior.
* gRPC service behavior.
* Streaming behavior.
* Error and boundary conditions.

The tests are intended to verify both individual component behavior and interactions between the layers.

## Project Structure

```text
logos/
├── api/
│   └── v1/
├── auth/
├── cmd/
│   ├── server/
│   └── client/
├── internal/
│   ├── config/
│   ├── core/
│   ├── log/
│   ├── recovery/
│   ├── segment/
│   ├── server/
│   ├── observability/
│   └── util/
├── proto/
├── Makefile
└── README.md
```

---

# Local Development

## Prerequisites

The following tools are required:

* Go
* Buf
* Docker
* Docker Compose
* CFSSL
* CFSSLJSON

Verify the installations:

```bash
go version
buf --version
docker --version
docker compose version
cfssl version
cfssljson --help
```

The project Makefile can also install the development tools required by the project.

## Running Logos Locally

The **Makefile is the preferred entry point for the common development workflow**.

### 1. Install Development Tools

```bash
make install-tools
```

This installs the tools required to build, generate, test, and operate the project locally.

### 2. Generate TLS/mTLS Certificates

```bash
make gencert
```

This generates the certificates required by the server and client for mutual TLS authentication.

### 3. Start the Observability Stack

```bash
make docker-comp-up
```

This starts the Docker-based observability services.

Verify the containers:

```bash
docker ps
```

The local interfaces are available at:

* Jaeger: `http://localhost:16686`
* Prometheus: `http://localhost:9091`

To stop the observability stack:

```bash
make docker-comp-down
```

### 4. Start the Logos Server

```bash
make start-server
```

The server starts using the project's configured mTLS and observability settings.

### 5. Run the Client

In another terminal:

```bash
make start-client
```

The client connects to the Logos server using the generated certificates and exercises the log API.

## Running Tests

Run the complete test suite:

```bash
make test
```

For verbose output:

```bash
go test -v ./...
```

## Useful Make Commands

| Command                 | Purpose                             |
| ----------------------- | ----------------------------------- |
| `make install-tools`    | Install required development tools  |
| `make gencert`          | Generate TLS/mTLS certificates      |
| `make docker-comp-up`   | Start the local observability stack |
| `make docker-comp-down` | Stop the observability stack        |
| `make start-server`     | Start the Logos server              |
| `make start-client`     | Run the client                      |
| `make test`             | Run the test suite                  |

---

# Design Decisions

The implementation is accompanied by a **Design Decision Log (DDL)** documenting important architectural decisions made throughout the project.

Each decision examines:

1. **Book architecture** — how the original implementation approaches the problem.
2. **Our understanding** — the underlying design assumptions, trade-offs, and system behavior.
3. **Logos decision** — the deliberate implementation decision made for the project.

The DDL therefore documents the evolution of the system rather than treating Logos as a direct reproduction of the book.

This process is particularly useful for examining the assumptions behind storage abstractions and the boundaries between logical operations and physical persistence.

# Research Context

Logos is being developed as a practical systems prototype in preparation for graduate research in distributed systems and systems engineering.

The project provides hands-on experience with:

* Distributed storage architecture.
* Persistent data structures.
* Failure handling.
* Networking and RPC.
* Service security.
* Observability.
* Testing and verification.
* System abstraction boundaries.

The goal is **not to claim a novel research contribution from Logos itself**.

Instead, Logos serves as a concrete systems artifact through which existing designs can be implemented, examined, questioned, and used to identify problems that may be suitable for future research.

# Status

Logos is an **experimental research-oriented prototype**.

It is intended for:

* Learning.
* Systems experimentation.
* Architectural analysis.
* Testing distributed-service concepts.
* Building practical systems-engineering experience.
* Exploring research questions in distributed storage and systems.

It is **not currently intended for production deployment**.

# Author

**Chukwudi Christian Okolo**

B.Eng. Electronic Engineering
University of Nigeria, Nsukka