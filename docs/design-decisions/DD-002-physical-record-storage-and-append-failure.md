# DD-002 — Physical Record Storage and Append Failure Semantics

**Status:** Agreed; implementation/evidence
**Scope:** Physical framing, byte positions, and behavior when append fails.

## Context

Records are variable-length serialized payloads. The Store needs framing that allows it to identify each payload and determine the physical byte position where a frame begins.

## Decision: length-prefixed framing

Each physical frame is:

```text
[8-byte big-endian payload length][serialized payload]
```

The Store position identifies the beginning of the frame. The segment index maps a segment-local logical offset to this physical position.

## Write construction

The intended implementation constructs a complete frame before writing it to the buffer:

```go
frame := make([]byte, lenWidth+len(p))
enc.PutUint64(frame[:lenWidth], uint64(len(p)))
copy(frame[lenWidth:], p)

n, err := s.buf.Write(frame)
if err != nil {
    return 0, 0, err
}
```

## Append failure policy

A Store append error leaves the Store instance in an uncertain state. The agreed policy is to treat that Store instance as unusable until it is closed and recovered/reopened; do not blindly retry at the previous `size` value.

This is distinct from a later Index failure after Store append succeeds. DD-004 describes an attempted rollback of that uncommitted tail. The implementation must be checked to ensure these failure paths are not conflated.

## Alternatives considered

- Separate writes for length and payload: rejected because the prefix may be accepted before the payload write fails.
- Continue after an uncertain write using unchanged size: rejected because the next append could produce invalid framing.
- Treat buffered write success as a complete durability guarantee: rejected; buffering and filesystem persistence are separate concerns.

## Consequences and limitations

- A complete frame simplifies sequential scanning and recovery.
- A failed write may still leave partial data in a buffer or file.
- Crash consistency depends on flush/sync policy and recovery behavior, not framing alone.

## Evidence to attach

- `Store.Append` implementation and frame encoding.
- Store round-trip and injected write-failure tests.
- Segment test for Index failure and rollback.
- Recovery tests for incomplete or malformed trailing frames.

## Evidence

- Implementation: `internal/core/store.go`
  - `Store.Append()`
- Store Tests: `internal/core/store_test.go`
  - `TestStoreAppendAndRead`
  - `TestStoreAppendWriteFailure`
- Segment Tests: `internal/segment/segment_test.go`
  - `TestSegmentRecoveryTruncatesUnindexedSuffix`
  - Recovery Tests: `internal/recovery/recovery_test.go`
  - `TestAlignBoundariesRollsBackIncompleteFinalFrame`
  - `TestAlignBoundariesRollsBackMultipleInvalidIndexEntries`

## Revisit trigger

Revisit if failure injection shows that the Store remains usable after uncertain writes, if rollback cannot restore a consistent tail, or if the durability contract requires stronger synchronization.
