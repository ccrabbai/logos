# Logos

**Logos** is a distributed log service prototype implemented in Go, based initially on the architecture presented in *Distributed Services with Go* by Travis Jeffery.

The project started as an implementation exercise, but has evolved into a systems-engineering prototype for exploring the design of distributed storage services: their storage abstractions, failure semantics, networking, security, observability, and operational behavior.

The project is also part of my preparation for graduate research in distributed systems and systems engineering.

---

## Overview

At a high level, Logos exposes a gRPC API over a persistent commit log:

```text
                         ┌────────────────────┐
                         │      Client        │
                         └─────────┬──────────┘
                                   │
                              gRPC / mTLS
                                   │
                                   ▼
                         ┌────────────────────┐
                         │   gRPC Server      │
                         │                    │
                         │ Produce / Consume  │
                         │ ProduceStream      │
                         │ ConsumeStream      │
                         └─────────┬──────────┘
                                   │
                                   ▼
                         ┌────────────────────┐
                         │        Log         │
                         │                    │
                         │ Segment management │
                         └─────────┬──────────┘
                                   │
                    ┌──────────────┴──────────────┐
                    ▼                             ▼
             ┌──────────────┐              ┌──────────────┐
             │   Segment    │              │    Segment   │
             │              │              │              │
             │ Store+Index  │              │ Store+Index  │
             └──────┬───────┘              └──────────────┘
                    │
              ┌─────┴─────┐
              ▼           ▼
          ┌───────┐   ┌───────┐
          │ Store │   │ Index │
          └───┬───┘   └───────┘
              │
              ▼
             Disk
```

The storage path is deliberately separated into layers:

```text
Log
 ↓
Segment
 ├── Store
 └── Index
```

The networking layer sits above the storage engine:

```text
Client
  ↓
gRPC
  ↓
Server
  ↓
Log
  ↓
Segment
  ↓
Store / Index
  ↓
Disk
```

---

## What Has Been Implemented

### 1. Persistent commit-log storage

Logos implements a persistent append-only log organized into segments.

The storage engine is structured around:

* `Log`
* `Segment`
* `Store`
* `Index`
* `Record`

The responsibilities are separated as follows:

| Component | Responsibility                                           |
| --------- | -------------------------------------------------------- |
| `Log`     | Coordinates segments and determines where records belong |
| `Segment` | Coordinates the store and index for one segment          |
| `Store`   | Persists serialized record bytes                         |
| `Index`   | Maps logical record offsets to physical positions        |
| `Record`  | Represents the logical log record                        |

A simplified read path is:

```text
Read(offset)
    │
    ▼
   Log
    │
    ▼
 locate Segment
    │
    ▼
 Segment.Read(offset)
    │
    ▼
 Index.Read(offset)
    │
    ▼
 physical position
    │
    ▼
 Store.Read(position)
    │
    ▼
 Record
```

An append follows the reverse coordination:

```text
Append(record)
    │
    ▼
   Log
    │
    ▼
 active Segment
    │
    ├──────────────► Store.Append()
    │                       │
    │                       ▼
    │                 physical position
    │
    └──────────────► Index.Write()
                            │
                            ▼
                     offset → position
```

---

### 2. Record serialization and storage format

Records are serialized before being persisted.

The storage format includes metadata such as:

```text
┌─────────┬─────────────┬───────┬───────────┬─────────┐
│ Version │ RecordLength│ CRC32 │ Timestamp │ Payload │
└─────────┴─────────────┴───────┴───────────┴─────────┘
```

The storage layer therefore deals with physical byte representation, while the higher layers operate on logical records and offsets.

---

### 3. Segmented log

The log is divided into segments rather than storing the entire log in one file.

Conceptually:

```text
Segment 0
offsets: 0 ─────────────── N

Segment 1
offsets: N+1 ───────────── M

Segment 2
offsets: M+1 ───────────── ...
```

The active segment receives new records until its configured capacity is reached, after which the log can create and operate on a new segment.

This provides the basis for managing a growing log without requiring the entire log to be represented by a single storage structure.

---

## gRPC Service

Logos exposes its storage engine through gRPC.

The service currently provides:

### Produce

Appends a record to the log and returns its assigned offset.

```text
Client
  │
  │ Produce(record)
  ▼
Server
  │
  ▼
Log.Append()
  │
  ▼
offset
  │
  ▼
Client
```

### Consume

Reads a record at a requested offset.

```text
Client
  │
  │ Consume(offset)
  ▼
Server
  │
  ▼
Log.Read(offset)
  │
  ▼
Record
  │
  ▼
Client
```

### ProduceStream

Provides bidirectional streaming for producing records.

### ConsumeStream

Provides server-side streaming for consuming records sequentially.

The consume stream maintains the current offset and advances it after successfully sending a record:

```text
offset 0
   │
   ▼
read record 0
   │
   ▼
send record 0
   │
   ▼
offset++
   │
   ▼
offset 1
   │
   ▼
read record 1
   │
   ▼
...
```

When the requested offset is beyond the current log boundary, the stream can continue waiting for new records rather than terminating the stream.

---

## Error Handling

The server translates storage-layer errors into gRPC status errors.

For example, attempting to consume beyond the current log boundary produces an offset-out-of-range condition.

This keeps the storage implementation independent of the transport layer while allowing the gRPC API to expose meaningful protocol-level errors.

---

# Security

Logos is being developed with the three-layer security model commonly used by distributed services:

```text
1. Encryption
       ↓
2. Authentication
       ↓
3. Authorization
```

## TLS / mTLS

The gRPC communication is secured using TLS.

For machine-to-machine communication, Logos uses mutual TLS:

```text
                  Logos CA
                 /        \
                /          \
               ▼            ▼
        Server certificate  Client certificate
               │            │
               ▼            ▼
          Logos Server   Logos Client
                \          /
                 \        /
                  \      /
                  mTLS
```

The CA provides the trust root used to validate certificates.

mTLS provides:

* encryption of data in transit;
* server authentication;
* client authentication.

Private keys are kept separate from certificates and should not be committed to source control.

## Authorization

Authentication answers:

> Who is this client?

Authorization answers:

> What is this client allowed to do?

Logos separates these concerns so that an authenticated client can subsequently be assigned permissions such as:

```text
READ
WRITE
READ + WRITE
```

Authorization is implemented using access-control rules rather than treating successful authentication as unrestricted access.

---

# Observability

Logos is instrumented using OpenTelemetry.

The observability path is:

```text
                  Logos
                    │
          ┌─────────┴─────────┐
          │                   │
        Traces              Metrics
          │                   │
          └─────────┬─────────┘
                    │
                  OTLP
                    │
                    ▼
              OpenTelemetry
                Collector
                    │
             ┌──────┴───────┐
             ▼              ▼
           Jaeger       Prometheus
```

The service uses:

* OpenTelemetry for telemetry;
* OTLP/gRPC for exporting telemetry;
* Jaeger for distributed tracing;
* Prometheus for metrics;
* structured logging through Go's `slog`.

The service identifies itself as:

```text
service.name = logos
environment  = development
```

The gRPC client and server are instrumented so that request traces can be followed across the network boundary.

---

# Testing

The project includes tests for the server and its gRPC behavior.

The server tests exercise scenarios including:

```text
Produce → Consume
ProduceStream
ConsumeStream
Consume beyond log boundary
```

The server tests run against a real local log instance rather than mocking the entire storage stack.

This allows the test to verify the integration between:

```text
gRPC client
    ↓
gRPC server
    ↓
Log
    ↓
Segment
    ↓
Store / Index
```

Lower-level components can be tested independently where mocking an interface provides value—for example, testing whether one layer correctly handles a dependency failure.

The intention is to distinguish between:

* unit tests for individual component behavior;
* integration tests for interactions between components;
* service-level tests for the externally visible gRPC API.

---

# Local Development

## Prerequisites

Install:

* Go
* Docker
* Docker Compose
* CFSSL/CFSSLJSON for certificate generation

Verify the tools:

```bash
go version
docker --version
docker compose version
cfssl version
cfssljson --help
```

---

# Project Structure

The project is organized approximately as follows:

```text
logos/
│
├── api/
│   └── v1/
│       ├── log.proto
│       └── generated gRPC/protobuf code
│
├── cmd/
│   ├── server/
│   └── client/
│
├── internal/
│   └── log/
│       ├── config/
│       ├── log/
│       ├── server/
│       ├── observability/
│       └── util/
│
├── docker-compose.yml
│
└── README.md
```

The exact generated files may vary depending on the current protobuf generation workflow.

---

# Running Logos Locally

## 1. Start the observability infrastructure

Start the local Docker Compose stack:

```bash
docker compose up -d
```

Check the running containers:

```bash
docker ps
```

The local observability environment consists of:

```text
OpenTelemetry Collector
        │
        ├──► Jaeger
        │
        └──► Prometheus
```

Jaeger provides the tracing UI.

The Jaeger UI is available at:

```text
http://localhost:16686
```

The OpenTelemetry Collector receives OTLP/gRPC telemetry on:

```text
localhost:4317
```

---

## 2. Start the Logos server

From the repository root:

```bash
go run ./cmd/server
```

The server starts the Logos gRPC service and listens on the configured local address.

The development client connects to:

```text
localhost:50051
```

---

## 3. Run the client

In a second terminal:

```bash
go run ./cmd/client
```

The client can be used to exercise the Logos API.

A typical interaction is:

```text
Client
  │
  ├── Produce("hello")
  │        │
  │        ▼
  │      Logos
  │        │
  │        ▼
  │      offset 0
  │
  └── Consume(0)
           │
           ▼
       "hello"
```

---

# Running Tests

Run the complete Go test suite:

```bash
go test ./...
```

For verbose output:

```bash
go test -v ./...
```

To run only the server package:

```bash
go test ./internal/log/server/...
```

The tests create temporary storage where required and clean it up after execution.

---

# Observing a Request

A useful local workflow is to run Logos together with the observability stack:

```text
Terminal 1
──────────
docker compose up -d


Terminal 2
──────────
go run ./cmd/server


Terminal 3
──────────
go run ./cmd/client
```

Then open Jaeger:

```text
localhost:16686
```

A request can then be followed through the service:

```text
Client
  │
  │ gRPC
  ▼
Server
  │
  ▼
Log
  │
  ▼
Segment
  │
  ├── Store
  └── Index
```

This makes the prototype useful not only for implementing functionality, but also for examining the behavior of a distributed service through telemetry.

---

# Configuration

Runtime configuration is kept separate from the implementation so that the service can be configured without changing the storage or networking code.

Important configuration concerns include:

* log directory;
* segment size;
* gRPC listening address;
* TLS certificates;
* CA certificate;
* observability endpoint;
* development environment settings.

Configuration should be supplied through the project's configuration mechanism rather than hard-coded into individual components.

---

# Design Philosophy

Logos is intentionally built as a layered system.

The primary design boundary is:

```text
Network
   │
   ▼
Server
   │
   ▼
Log
   │
   ▼
Segment
   │
   ├── Store
   └── Index
```

Each layer has a distinct responsibility.

For example:

```text
Store
→ physical persistence

Index
→ logical offset → physical position

Segment
→ coordinates Store and Index

Log
→ coordinates Segments

Server
→ translates gRPC requests into log operations
```

This separation makes it possible to reason about individual components independently and to replace or experiment with implementations beneath stable interfaces.

---

# Engineering Questions Explored

Beyond simply reproducing an existing implementation, Logos is being used to examine the assumptions behind distributed storage systems.

Some of the questions being explored include:

* What does a log assume about the underlying storage medium?
* Where should record metadata be assigned?
* What constitutes a successful append?
* How should failures propagate across storage layers?
* How should indexes be rebuilt after an incomplete write?
* How should segment boundaries be handled?
* What happens when a record is larger than the remaining segment capacity?
* How should a streaming consumer behave at the current end of the log?
* Where should transport errors be translated into storage/API errors?
* How should authentication and authorization be separated?
* How can distributed-service behavior be observed through traces and metrics?

These questions are part of the project's broader purpose: using implementation to develop the ability to identify assumptions, failure modes, and design trade-offs in distributed systems.

---

# Research Context

Logos is a **research-oriented prototype**, not a production distributed database.

The purpose of the project is to demonstrate practical systems experience while developing a deeper understanding of distributed storage and service architecture.

The project deliberately moves through several layers:

```text
Storage primitives
       ↓
Persistent log
       ↓
Segmented storage
       ↓
Network service
       ↓
Security
       ↓
Observability
       ↓
Failure behavior
       ↓
Systems research questions
```

The implementation provides a concrete system against which architectural assumptions can be studied rather than treating distributed systems purely as an abstract topic.

The project is therefore intended to demonstrate:

* ability to understand an existing systems design;
* ability to implement the design in Go;
* ability to reason about interfaces and abstraction boundaries;
* ability to test system behavior;
* ability to instrument a distributed service;
* ability to investigate failure semantics;
* ability to identify limitations and open engineering questions.

---

# Current Status

Logos currently provides a working prototype of a persistent gRPC log service with:

* [x] Persistent append-only log
* [x] Segmented storage
* [x] Record serialization
* [x] Persistent store
* [x] Offset index
* [x] Produce API
* [x] Consume API
* [x] Produce streaming API
* [x] Consume streaming API
* [x] gRPC error handling
* [x] TLS/mutual TLS security
* [x] Client authentication
* [x] Access-control authorization
* [x] OpenTelemetry instrumentation
* [x] Distributed tracing
* [x] Metrics
* [x] Structured logging
* [x] Local Docker-based observability stack
* [x] Automated tests for service behavior

The implementation continues to evolve as new design assumptions and failure cases are investigated.

---

# Relationship to the Original Book

Logos began from the architecture presented in:

> Travis Jeffery, *Distributed Services with Go*

The book provides the initial reference architecture and implementation path.

The project does not treat that implementation as the final design.

For significant design decisions, Logos maintains a **Design Decision Log (DDL)** documenting:

```text
Book architecture
       ↓
Our understanding
       ↓
Problem / assumption identified
       ↓
Alternative considered
       ↓
Agreed implementation
```

This allows changes to the implementation to be understood as deliberate engineering decisions rather than unexplained deviations from the reference implementation.

---

# Roadmap

Future work will continue to investigate the behavior and assumptions of the system rather than simply adding features.

Potential areas include:

* deeper failure testing;
* storage recovery;
* index reconstruction;
* segment recovery;
* concurrency behavior;
* storage-performance characterization;
* alternative storage backends;
* stronger observability;
* distributed deployment experiments;
* investigation of hardware/storage assumptions;
* identification of research questions arising from observed system limitations.

---

# Author

**Chukwudi Christian Okolo**

B.Eng. Electronic Engineering
University of Nigeria, Nsukka

Logos is part of my preparation for graduate research in distributed systems and systems engineering.

---
