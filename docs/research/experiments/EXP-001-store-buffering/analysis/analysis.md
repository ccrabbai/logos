# EXP-001 Analysis

## Status
**Preliminary pilot analysis only. No final conclusion.**

## 1. Pilot observations
The supplied pilot used five buffer configurations (`0`, `4096`, `16384`, `65536`, and `262144` bytes), three repetitions per configuration, a fixed 1,024-byte payload, and the command documented in `protocol.md`.

Reported values:

| Buffer bytes | Run 1 ns/op | Run 2 ns/op | Run 3 ns/op | Run 1 MB/s | Run 2 MB/s | Run 3 MB/s |
|---:|---:|---:|---:|---:|---:|---:|
| 0 | 2428 | 3322 | 3428 | 421.68 | 308.28 | 298.70 |
| 4096 | 3329 | 3115 | 2467 | 307.59 | 328.77 | 415.07 |
| 16384 | 2580 | 3258 | 2512 | 396.89 | 314.30 | 407.71 |
| 65536 | 2114 | 1809 | 1882 | 484.44 | 565.94 | 544.03 |
| 262144 | 1866 | 2633 | 1900 | 548.65 | 388.97 | 539.05 |

All listed runs reported `1152 B/op` and `1 allocs/op`.

## 2. Preliminary interpretation
- The repetitions vary materially within several configurations. The cause is unknown; possible contributors include scheduling, CPU frequency/thermal behavior, background activity, storage/filesystem effects, and measurement noise.
- The 65,536-byte configuration had comparatively consistent and high throughput in these pilot runs, but this is a pilot observation only and must not be called the winner.
- The 262,144-byte configuration varied substantially between repetitions.
- Identical reported allocation metrics across these runs do not mean that buffer capacity has no memory cost. `B/op` is benchmark allocation accounting per operation and does not represent the writer's total retained buffer capacity.

## 3. Timing discrepancy to investigate
The command used `-benchtime=3s -count=3`, but the test process reported `ok ... 107.445s`. The per-sub-benchmark timings and test harness overhead do not obviously account for this total. Investigate with a timed rerun and, if necessary, run one buffer configuration at a time. Record findings before locking the protocol.

## 3.1 Runtime follow-up

A follow-up wall-clock measurement was performed using:

```powershell
Measure-Command {
    go test ./internal/core -run '^$' -bench '^BenchmarkStoreAppend$' -benchmem -benchtime=1s -count=1
}
```
The measured total wall-clock time was approximately 11.91 seconds.

This indicates that the earlier 107.445-second total from the three-repetition, three-second benchmark command is not representative of the current short-run wall-clock behavior. The earlier value is therefore retained as part of the pilot record but is not treated as an experimental finding.

## 4. Measurement limitations
- Go's `ns/op` is the average elapsed time per benchmark operation for a run. It is not median latency and does not provide p95/p99.
- Throughput uses `b.SetBytes(1024)`, so MB/s represents payload throughput; each stored frame also includes an 8-byte length header.
- The benchmark measures the Store append path only, not the complete Segment/Log/gRPC path.
- Flushing a `bufio.Writer` is not equivalent to a durability guarantee such as a successful `fsync`.
- File-size checks do not prove that each record can be read back and matches the expected payload.
- Three repetitions are useful for a pilot but may be insufficient for a strong conclusion.

## 5. Required next steps
1. Complete `environment.md` from the actual machine.
2. Investigate the total elapsed time.
3. Add record read-back correctness verification.
4. Decide whether a separate latency-distribution harness is justified.
5. Lock the protocol before final data collection.
6. Preserve all raw outputs and derive summaries reproducibly.

## 6. Conclusion
No final performance conclusion is justified from the current pilot. It is useful for identifying possible buffer sizes and measurement issues, not for selecting an optimal configuration.

## 7. Latency measurement decision

The final experiment will use Go benchmark `ns/op` as a secondary performance measurement rather than reporting median/p95/p99 latency.

Per-operation latency instrumentation was not added to the throughput benchmark because timing each append introduces additional work into a short append path and could affect the measured behavior.

Consequently, this experiment makes no claims about tail latency.
