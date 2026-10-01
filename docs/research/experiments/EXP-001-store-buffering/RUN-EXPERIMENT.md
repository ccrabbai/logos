# EXP-001 — Experiment Runbook

## 1. Verify repository state

From the Logos repository root:

```powershell
git status --short
git rev-parse HEAD
go version
```

Record the output in `environment.md`.

## 2. Verify correctness and tests

```powershell
go test ./internal/core
go test ./...
```

Both should pass before the final performance campaign.

## 3. Run the locked campaign

```powershell
go test ./internal/core `
  -run '^$' `
  -bench '^BenchmarkStoreAppend$' `
  -benchmem `
  -benchtime=3s `
  -count=5
```

## 4. Capture raw output

Copy the complete terminal output without editing it to:
`results/raw/final.txt`

The raw file must retain the benchmark header, all repetitions, and final `PASS/ok` lines.

## 5. Record environment

At minimum record:
- OS
- architecture
- CPU
- Go version
- repository commit
- working-tree status
- filesystem/storage
- benchmark command

## 6. Analyze

Use all retained repetitions.
Calculate:
- mean throughput
- median throughput
- standard deviation
- coefficient of variation
- mean ns/op

Do not select the fastest run or remove inconvenient observations.

## 7. Reproducibility record

- Experiment ID: EXP-001
- Repository commit: 
- Go version: 
- Operating system: Windows
- Architecture: amd64
- CPU: Intel(R) Core(TM) i3-10110U CPU @ 2.10GHz
- Filesystem/storage: 
- Benchmark command: `go test ./internal/core -run '^$' -bench '^BenchmarkStoreAppend$' -benchmem -benchtime=3s -count=5`
- Date: 
- Result: PASS

## 8. Completion criterion

EXP-001 is complete when the protocol, runbook, environment, raw output, summary, analysis, correctness evidence, and limitations are all retained together.
