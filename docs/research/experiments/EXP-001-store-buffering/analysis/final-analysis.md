# EXP-001 — Final Analysis

## 1. Objective

EXP-001 evaluated whether the configured Store write-buffer size affects append performance for a fixed 1 KiB workload.
Five configurations were tested with five benchmark repetitions each.

## 2. Primary measurement

The primary outcome was payload throughput in MB/s.
The benchmark used:

```go
b.SetBytes(int64(benchmarkRecordBytes))
```

with `benchmarkRecordBytes = 1024`.
Therefore the reported MB/s represents payload throughput, not payload plus the Store's 8-byte frame header.

## 3. Summary

| Buffer | Mean MB/s | Median MB/s | SD | CV | Mean ns/op |
|---|---|---|---|---|---|
| 0 B | 424.83 | 398.20 | 86.60 | 20.38% | 2477 |
| 4 KiB | 402.75 | 426.57 | 55.50 | 13.78% | 2583 |
| 16 KiB | 474.41 | 501.91 | 168.86 | 35.59% | 2414 |
| 64 KiB | 613.40 | 604.60 | 37.64 | 6.14% | 1675 |
| 256 KiB | 603.45 | 572.95 | 102.86 | 17.05% | 1731 |

All configurations reported 1,152 B/op and 1 allocs/op.

## 4. Observations

### 4.1 Buffer size affected measured throughput
The mean throughput varied materially across configurations. The 64 KiB and 256 KiB configurations were approximately 44% and 42% above the 0 B mean respectively.

### 4.2 The relationship was not strictly monotonic
Increasing the buffer did not produce a strictly increasing sequence of throughput:
- 0 B: 424.83 MB/s
- 4 KiB: 402.75 MB/s
- 16 KiB: 474.41 MB/s
- 64 KiB: 613.40 MB/s
- 256 KiB: 603.45 MB/s

Thus the experiment does not support a simple claim that every increase in buffer size produces a corresponding increase in throughput.

### 4.3 Variability differed substantially
The 16 KiB configuration had the highest CV at approximately 35.59%.
The 64 KiB configuration had the lowest CV at approximately 6.14%.
This means the 64 KiB measurements were comparatively stable across the five repetitions, while the 16 KiB measurements were much more variable.

### 4.4 Allocation behavior did not change
Every configuration reported:
- 1,152 B/op
- 1 allocs/op

Within the benchmark's measurement model, changing buffer size therefore did not alter the reported per-operation allocation profile.

## 5. Correctness

The independent correctness verification for the tested buffer configurations passed before the final campaign.
The performance benchmark itself was not used as the correctness test.

## 6. What can be concluded

Under the fixed 1 KiB single-worker workload and recorded test environment, Store buffering was associated with measurable differences in append throughput and average benchmark time. The 64 KiB and 256 KiB configurations produced substantially higher mean throughput than the smaller configurations tested, while 64 KiB showed the lowest run-to-run variability in this campaign.

## 7. What cannot be concluded

The experiment does not establish:
- a universally optimal buffer size;
- performance on other CPUs or storage devices;
- performance on other filesystems;
- performance for other record sizes;
- concurrent-client performance;
- distributed Logos performance;
- production performance;
- durability or fsync performance;
- p95/p99 latency.

## 8. Threats to validity

- **Hardware and environment:** Only one Windows amd64 environment was measured.
- **Workload:** Only 1 KiB records and a single worker were tested.
- **Parameter space:** Only five buffer sizes were tested.
- **Storage semantics:** The benchmark measures the configured Store append path. It does not establish durable persistence latency.
- **Variability:** Run-to-run variability was non-trivial for several configurations. No observations were discarded because they were inconvenient.
- **Benchmark scope:** The experiment isolates `internal/core.Store.Append`; it does not measure gRPC, TLS, authorization, segment coordination, recovery, or distributed communication.

## 9. Reproducibility assessment

The campaign used a locked protocol, five repetitions per configuration, a fixed command, and preserved complete raw output.
**Assessment:** PASS with documented environmental limitations.

## 10. Research significance boundary

EXP-001 should not be presented as a novel storage research contribution.
Its value at this stage is methodological and engineering-oriented: it demonstrates that Logos contains an explicit systems parameter that can be isolated, measured, reproduced, and analyzed while keeping the scope and limitations of the evidence explicit.
