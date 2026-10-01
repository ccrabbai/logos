# EXP-001 Environment Record

## Status
**Partially recorded — complete before final measurements.** Do not guess missing details.

## Known from pilot output
- Operating system: Windows (`goos: windows`)
- Architecture: amd64
- CPU reported by Go: Intel(R) Core(TM) i3-10110U CPU @ 2.10GHz
- Go package: `github.com/ccrabbai/logos/internal/core`
- Benchmark process reported `-4` (GOMAXPROCS=4)

## To record from the actual machine
- Date/time | local timezone: 2026-10-01 16:23 | (UTC+01:00) West Central Africa
- Windows edition | version: Windows 11 Pro | 25H2
- Go version (`go version`): go version go1.26.3 windows/amd64
- CPU model and logical processor count: Intel(R) Core(TM) i3-10110U CPU @ 2.10GHz (2.59 GHz)
- Installed RAM: 16GB
- Storage device type/model (SSD/HDD/NVMe, if known): SSD
- Filesystem of the temporary directory used by `b.TempDir()`: NTFS
- Free disk space (if relevant): 64GB
- Power mode / whether on AC power: on AC Power
- Significant background workloads:
- Other relevant system conditions:

## Repository and build
- Repository: `github.com/ccrabbai/logos`
- Git commit (`git rev-parse HEAD`): 0b2f989e2499123346f1dd9c3c42398185dff9ce
- Working tree (`git status --short`): Nothing
- Go module/dependency state: go.mod and go.sum are present and consistent. Dependencies are resolved successfully, and go test ./... passes.
- Benchmark source path: `internal/core/store_benchmark_test.go`
- Benchmark name: `BenchmarkStoreAppend`
- Build tags or special flags (if any):

## Pilot command
```powershell
go test ./internal/core -run '^$' -bench '^BenchmarkStoreAppend$' -benchmem -benchtime=3s -count=3
```

## Notes
The CPU and OS fields above are copied from the pilot output. All other missing fields must be collected from the actual machine. The current environment record is not yet sufficient for independent reproduction.
