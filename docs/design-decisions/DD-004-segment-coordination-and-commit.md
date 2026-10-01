# DD-004 — Segment Coordination, Record Commit, and Consistency

**Status:** Agreed; implementation/evidence 
## Context

A Segment coordinates the Store and Index, assigns record metadata, translates global offsets to local index offsets, and manages segment capacity.

## Responsibilities

- Assign global `Record.Offset`.
- Populate required metadata before protobuf serialization.
- Append serialized records to the Store.
- Insert the segment-local index entry.
- Translate global offsets to local offsets for lookup.
- Coordinate reads, capacity checks, and lifecycle operations.

## Append sequence and commit boundary

```text
assign metadata
      ↓
protobuf marshal
      ↓
Store.Append
      ↓
Index.Write
      ↓
advance nextOffset
      ↓
return success
```

The intended commit boundary is after both Store append and Index write succeed. `nextOffset` should not advance after a failed append.

## Rollback policy

If Store append succeeds but Index insertion fails, the segment attempts to remove only the uncommitted tail from the Store. The described rollback flushes pending buffered bytes, truncates the file to the append's starting position, restores Store size, and resets the buffered writer.

This policy must be verified against the current code and tests. If rollback itself fails, the segment must not continue as though Store and Index remain consistent.

The Store's separate policy for an append/write error is defined in DD-002: treat the Store instance as unusable until recovery/reopen. Do not assume the rollback path applies to every Store write failure.

## Read path

```text
global offset
      │ subtract baseOffset after bounds check
      ▼
local index offset
      ↓
physical position
      ↓
Store.Read
      ↓
protobuf unmarshal
      ↓
persisted Record
```

The persisted `Record.Offset` is intended to remain authoritative; the index is used to locate the record, not to reconstruct its global offset.

## Fullness

A segment is considered full when either Store size or Index valid-data size reaches its configured maximum. Log rotation behavior and exact boundary semantics must be verified together.

## Crash recovery boundary

In-process rollback cannot run after process termination. Startup recovery must reconcile the Store and Index according to the implemented policy. Do not claim a particular tail-truncation or index-rebuild algorithm until the current recovery code and tests establish it.

## Evidence to attach

- Segment append/read/fullness functions.
- Tests for offset assignment, store-before-index ordering, index failure, rollback, and capacity boundaries.
- Log rollover tests.
- Recovery tests for restart, missing index, and incomplete tail, where present.

## Evidence

- Implementation: `internal/segment/segment.go`
  - `Segment.Append()`
  - `Segment.Read()`
  - `Segment.IsFull()`
- Segment Tests: `internal/segment/segment_test.go`
  - `TestSegmentAppendAndRead`
  - `TestSegmentReadBeforeBaseOffset`
  - `TestSegmentReadBeyondNextOffset`
  - `TestSegmentIsFullByStoreCapacity`
  - `TestSegmentIsFullByIndexCapacity`
  - `TestSegmentReopensAndRecoversRecords`
- Log Tests: `internal/log/log_test.go`
  - `TestLogRollover`
  - `TestLogPersistsAcrossRestart`
- Log Tests: `internal/recovery/recovery_test.go`
  - `TestAlignBoundariesRollsBackIncompleteFinalFrame`
  - `TestAlignBoundariesTruncatesUnindexedStoreData`


## Revisit trigger

Revise this decision if the Store/Index commit sequence can acknowledge inconsistent state, rollback can remove committed data, or recovery cannot restore the state promised by the durability contract.
