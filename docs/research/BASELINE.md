# Project Logos — Baseline

**Status:** Evidence-based working baseline  
**Purpose:** Establish what is currently known before further implementation changes.

## Scope and evidence boundary

This baseline reflects the project details and command output available during the current review. The full repository checkout, current Git commit, working-tree status, and every source/test file were not independently inspected in this review. Accordingly, repository-specific statements below are provisional until checked against the current checkout.

## Project summary

Logos is a Go distributed-log service prototype initially based on Travis Jeffery's *Distributed Services with Go*. It has evolved into a systems-engineering exercise around record metadata, append-only storage, segment-local indexing, recovery, gRPC service behavior, mutual TLS, authorization, and observability.

## Known component map

| Component | Responsibility established by the current project notes |
|---|---|
| Store | Stores length-prefixed serialized records and tracks physical size/position. |
| Index | Maps segment-local logical offsets to physical byte positions using fixed-width entries. |
| Segment | Coordinates Store and Index, assigns record metadata, handles segment-local/global offset translation, and determines fullness. |
| Log | Provides the logical append/read interface and manages segments. |
| Recovery | Reopens/reconstructs log state and handles index/store consistency according to implemented recovery policy. |
| Server | Exposes unary and streaming gRPC operations, authenticates TLS peers, applies authorization, and instruments requests. |
| Config / Observability | Configure TLS and initialize logging, metrics, and tracing. |

Confirm every responsibility against the current source before treating this table as a final source map.

## Test status reported by the developer

The following commands were run from the project checkout, with this output reported in the conversation:

- `go test ./...` — **PASS** for all packages.
- `go test -race ./...` — **INCOMPLETE**. Most packages completed, but Windows returned `fork/exec ...\\segment.test.exe: Access is denied.` for `internal/segment`. This is an execution failure, not a reported race. It does not establish that the entire race-enabled suite passed.

The race test should be rerun independently, for example with `go test -race ./internal/segment`, and preferably in a clean environment if the Windows error persists.

## Known decisions and artifacts

- DD-001: record metadata (value, global offset, checksum, producer identity, timestamp).
- DD-002: physical length-prefixed framing and append failure semantics.
- DD-003: segment-local fixed-width index.
- DD-004: segment coordination, commit ordering, and rollback boundary.
- DD-005: previously identified as the `log.go` review; its actual document must be compared with current `internal/log` code before editing.
- Existing README, Makefile, Docker Compose, certificate-generation instructions, and repository structure require a fresh-checkout verification.

## Known limitations / evidence gaps

1. Current Git commit and working-tree status have not been recorded in this baseline.
2. The project-wide race-enabled run did not complete.
3. Passing tests do not, on their own, prove every crash, partial-write, or filesystem durability scenario.
4. The exact append durability boundary needs to be documented against the actual flush/sync behavior.
5. Failure injection coverage must be audited for Store write failures, Index write failures, rollback failures, interrupted appends, and missing/corrupt indexes.
6. The exact recovery behavior for malformed/truncated frames and index reconstruction must be confirmed against current source and tests.
7. README/Makefile commands and observability endpoints must be verified from a clean checkout.
8. A controlled experiment and raw results have not yet been produced.

## Required baseline capture

Run from the repository root and append the actual results to this document:

```bash
git rev-parse HEAD: 37a504d36e38b81d66d290d8a095eab4bdc57cd8
git status --short: 
go version: go version go1.26.3 windows/amd64
go test ./...: completed successfully for all listed packages:
    auth
    internal/config
    internal/core
    internal/log
    internal/observability
    internal/recovery
    internal/segment
    internal/server
go test -race ./... did not complete. The packages shown before the failure passed, but Windows failed to launch the segment test executable: fork/exec C:\Users\hp\AppData\Local\Temp\go-build...\segment.test.exe: Access is denied.
go test -race ./internal/segment:  completed successfully
```

## Baseline gate

This baseline is final only when the commit, working-tree state, source/test paths, and commands are captured from the actual checkout and all claims below are linked to source files or tests.
