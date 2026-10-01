# EXP-001 — Store Buffering Performance

**Status:** Locked
**Experiment ID:** EXP-001
**Component:** `internal/core.Store`
**Purpose:** Measure the effect of Store write-buffer size on append performance under a fixed workload.

---

## 1. Research Question

> How does the configured Store write-buffer size affect append performance for a fixed 1 KiB record workload?

The experiment evaluates whether changing the Store's configurable buffering policy produces measurable differences in append throughput and benchmark-level execution cost.

---

## 2. Independent Variable

The independent variable is:

**`Config.Store.BufferBytes`**

The following configurations are fixed for the experiment:

| Configuration   | Buffer size |
| --------------- | ----------: |
| `buffer_0`      |     0 bytes |
| `buffer_4096`   |       4 KiB |
| `buffer_16384`  |      16 KiB |
| `buffer_65536`  |      64 KiB |
| `buffer_262144` |     256 KiB |

No additional buffer configurations will be introduced after the protocol is locked.

---

## 3. Workload

Each benchmark operation appends:

* Payload size: **1,024 bytes**
* Payload contents: deterministic byte pattern
* Append concurrency: **single benchmark worker**
* Record framing: existing Store framing implementation
* Filesystem and hardware: unchanged across configurations

The benchmark creates a fresh temporary Store for each configuration.

---

## 4. Dependent Variables

### Primary

* **Payload throughput (`MB/s`)**

The benchmark uses `b.SetBytes(1024)`, therefore the reported throughput represents payload bytes processed per second rather than payload-plus-frame bytes.

### Secondary

* `ns/op` — average benchmark time per append operation
* `allocs/op`
* `B/op`

### Correctness

Each tested buffer configuration must independently pass record append/read-back verification.

---

## 5. Latency Measurement Boundary

EXP-001 does **not** report:

* median latency;
* p95 latency;
* p99 latency;
* per-operation latency distributions.

The Go benchmark's `ns/op` value is treated as an aggregate average measurement.

Per-operation latency instrumentation is deliberately excluded because the Store append path is short enough that additional timing instrumentation could alter the workload being measured.

Tail-latency analysis, if required later, will be treated as a separate experiment.

---

## 6. Controls

The following remain fixed:

* payload size;
* payload contents;
* number of benchmark workers;
* Store implementation;
* benchmark implementation;
* Go version;
* operating system;
* CPU;
* filesystem;
* repository revision;
* benchmark duration;
* number of repetitions;
* correctness procedure.

Only `Config.Store.BufferBytes` varies between experimental configurations.

---

## 7. Benchmark Procedure

The final campaign uses:

```powershell
go test ./internal/core `
  -run '^$' `
  -bench '^BenchmarkStoreAppend$' `
  -benchmem `
  -benchtime=3s `
  -count=5
```

Each buffer configuration is therefore measured for five benchmark repetitions.

The complete benchmark output is retained as raw experimental evidence.

---

## 8. Correctness Verification

Before the performance campaign:

```powershell
go test ./internal/core
```

The correctness test must verify that records written under every configured buffer size can subsequently be read back correctly.

The performance benchmark itself does not substitute for this correctness test.

---

## 9. Environment Capture

The following information is recorded before the final campaign:

```powershell
go version
git rev-parse HEAD
git status --short
```

The experiment record also includes:

* operating system;
* architecture;
* CPU;
* filesystem/storage environment;
* repository revision;
* Go version;
* benchmark command.

---

## 10. Result Handling

All five repetitions for every configuration are retained.

Runs are not selected according to performance.

No result is removed because it is slower or faster than the others.

If a run fails, the failure and its cause are recorded before any rerun.

Statistical summaries are calculated only after the complete raw output has been captured.

---

## 11. Interpretation Boundary

EXP-001 can establish observations about the effect of Store buffering under this specific workload and environment.

It cannot establish:

* general performance across all hardware;
* general performance across all filesystems;
* production-scale performance;
* concurrent-client performance;
* durability or `fsync` performance;
* distributed-system performance;
* superiority of Logos over another storage system.

These questions are outside the scope of this experiment.

---

## 12. Protocol Status

This protocol is **locked**.

After the final campaign begins, changes to:

* buffer configurations;
* workload;
* benchmark implementation;
* repetitions;
* benchmark duration;
* measurement definitions

constitute a protocol change and require a new experiment revision rather than silently modifying EXP-001.
