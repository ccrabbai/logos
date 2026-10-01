# Durability Contract and Test Status

## Durability contract: conservative statement

Logos appends serialized records to a length-prefixed store and writes a corresponding index entry. The segment-level append sequence described by the design is:

1. Assign record metadata and serialize the record.
2. Append the framed payload to the Store.
3. Write the segment-local index entry.
4. Advance the next logical offset only after the required operations succeed.
5. Return success to the caller.

This is an **operation-level commit sequence**, not proof of durability against every failure model.

The available evidence does not justify claiming that every successful append survives sudden process termination, operating-system failure, or power loss. Such claims depend on when buffered bytes are flushed, whether the file and index are synchronized, filesystem behavior, and the recovery policy. These details must be verified in the current implementation.

### Contract to document after source audit

Specify separately:

- **Append acknowledgement:** exactly when `Append` returns success.
    Append returns success only after both Store.Append and Index.Write succeed and the Segment advances its committed offset state. A successful return does not, by itself, guarantee survival of a machine or power failure.
- **Process-crash recovery:** what records are retained after abrupt process termination.
    Recovery must distinguish complete, valid records from incomplete or uncommitted tail data. A record written to the Store but not successfully indexed may remain physically present after a crash; it must not automatically be treated as a successfully acknowledged append. The exact retention and truncation behavior depends on the implemented recovery path.
- **Machine/power-failure durability:** whether and when file and index synchronization is performed.
    The Store uses buffered writes, and the index is mmap-backed. Successful append acknowledgement must not be described as a power-failure durability guarantee unless the implementation synchronizes the relevant data and metadata before returning. The precise Flush/Sync/fsync behavior and ordering must be confirmed in the current Store, Index, and close paths.
- **Index reconstruction:** whether the index is rebuildable from the store, and how malformed tails are handled.
    The index is a derived lookup structure; the persisted records are the source of record contents and offsets. However, the available implementation history is inconsistent about whether startup currently rebuilds a missing index from the Store. Do not claim automatic reconstruction or malformed-tail truncation as verified until the current recovery implementation and tests establish both behaviors.
- **Failed append:** whether the Store/Segment is poisoned, rolled back, or requires close/reopen.
    If the Store append succeeds but the index write fails, the Store and Segment may be inconsistent. The intended implementation attempts to roll back the uncommitted Store tail. A failed Store write must not be blindly retried against potentially invalid buffered state; recovery or close/reopen may be required.
- **Rollback failure:** what prevents further writes to an inconsistent instance.
    If rollback fails, the instance must not accept further writes as though its state were consistent. It must enter a failed/unusable state or otherwise enforce a close-and-recover/reopen requirement. Document this as a guarantee only if the implementation explicitly enforces it; logging the error alone is insufficient.

Do not use “durable” as a synonym for “written to a buffered writer” or “indexed successfully.”

## Append failure semantics

DD-002 states that a Store append failure makes that Store instance unusable until close/recovery/reopen, because the physical state may be uncertain. DD-004 separately describes an attempted rollback when the Store append succeeds but the subsequent Index write fails. These policies address different failure points and should remain explicitly distinguished.

The rollback path must itself be verified. If rollback fails, the implementation must not continue as if the Store and Index were consistent.

## Test status reported on 2026-10-01

### Functional tests

`go test ./...` completed successfully for all listed packages:

- `auth`
- `internal/config`
- `internal/core`
- `internal/log`
- `internal/observability`
- `internal/recovery`
- `internal/segment`
- `internal/server`

The API, command, and util packages reported no test files.

### Race-enabled tests

`go test -race ./...` did not complete. The packages shown before the failure passed, but Windows failed to launch the segment test executable:

```text
fork/exec C:\Users\hp\AppData\Local\Temp\go-build...\segment.test.exe: Access is denied.
```

This is a test-process execution error. It is not a race report, and it does not establish that the entire race-enabled suite passed.

Recommended next diagnostic command:

```powershell
go test -race ./internal/segment:  completed successfully
```

## Evidence boundary

The test outcomes above are based on command output supplied during the project conversation. They should be retained alongside the actual command output in project notes or CI logs.
