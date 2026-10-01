 # DD-003 — Segment Index Design

**Status:** Agreed; implementation/evidence

## Problem

Records have variable physical sizes, so a record's physical location cannot be calculated directly from its logical offset. The segment needs an efficient mapping from local logical position to physical byte position.

## Decision

Retain a segment-local, fixed-width index with 12-byte entries:

```text
[4-byte big-endian local offset][8-byte big-endian physical position]
```

The index uses a memory-mapped file and preallocated capacity. Its valid-data size is distinct from the physical file capacity. Synchronization and final truncation behavior must be verified against the implementation.

## Coordinate systems

- `Record.Offset`: global logical log offset.
- `Index.off`: segment-local logical offset.
- `Index.pos`: physical byte position of the Store frame.

For a segment with `baseOffset = 100`, global record offset `102` maps to local index offset `2`; that index entry points to the physical frame position.

```text
global Record.Offset
        │ minus baseOffset
        ▼
local Index.off
        │ lookup
        ▼
physical Index.pos
        ▼
Store frame
```

## Rationale and consequences

Fixed-width entries permit direct lookup by entry position. The index is an access structure, not the authoritative source of the record's persisted global offset.

The design depends on the Store and Index remaining consistent. Store data should be appended before its index entry. Failure and recovery behavior are addressed in DD-004 and the storage failure matrix.

## Evidence to attach

- Index entry encoding and lookup functions.
- Memory-map, sync, close, and truncate functions.
- Index tests for round-trip, bounds, truncation, and persistence.
- Segment tests proving global-to-local offset translation.

## Evidence

- Implementation: `internal/core/index.go`
  - `Index.Write()`
  - `Index.Read()`
  - `Index.Flush()`
  - `Index.Close()`
  - `Index.Truncate()`
- Index Tests: `internal/core/index_test.go`
  - `TestIndexWriteAndRead`
  - `TestIndexEmptyReadReturnsEOF`
  - `TestIndexPersistsAcrossReopen`
  - `TestIndexCapacity`
  - `TestIndexTruncateRemovesLogicalEntries`
- Segment Tests: `internal/segment/segment_test.go`
  - `TestSegmentAppendAndRead`

## Limitations / revisit triggers

Revisit if index capacity wastes unacceptable space for target workloads, if recovery cannot reliably reconstruct it, or if measurements show that the current memory-mapped design is a bottleneck.
