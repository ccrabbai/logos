# DD-001 — Record Metadata

**Status:** Agreed; implementation/evidence
**Scope:** Logical record fields and their semantics.

## Context

The original record in Travis Jeffery's *Distributed Services with Go* contains the application value and a logical offset. Logos adds metadata needed for integrity checks, producer identification, and temporal context.

## Book architecture

```protobuf
message Record {
  bytes value = 1;
  uint64 offset = 2;
}
```

## Logos design

```protobuf
message Record {
  bytes value = 1;
  uint64 offset = 2;
  bytes checksum = 3;
  string producer_id = 4;
  int64 created_at = 5;
}
```

Confirm the current `.proto` definition before treating this snippet as the exact repository schema.

## Field semantics

| Field | Meaning | Origin / handling |
|---|---|---|
| `value` | Application payload | Supplied by the caller |
| `offset` | Global logical position in the log; not a physical byte position | Assigned by the log/segment |
| `checksum` | Detects corruption of `value` | Calculated over `value`, not the complete Record |
| `producer_id` | Identifies the originating application/client | Authenticated identity is applied by the server; segment may supply a default when empty |
| `created_at` | Timestamp associated with the record | System-generated; unit and encoding must match implementation |

## Decision

Retain these fields and their distinct semantics. Do not add a key yet: Logos is a general ordered log, and no demonstrated requirement currently justifies keyed-record semantics.

## Consequences and limitations

- A checksum over `value` does not protect every metadata field or the framing.
- `producer_id` denotes the producer/client, not the Logos server that stores the record.
- The checksum algorithm/encoding, timestamp unit, producer-ID format, and physical encoding consequences must be stated from the actual implementation.
- The record's global offset must not be confused with the segment-local index offset or physical store position.

## Evidence to attach

- Current `.proto` file and generated API.
- Segment append function that assigns offset, timestamp, producer ID, and checksum.
- Tests for metadata assignment, checksum behavior, and protobuf round-trip.
- Server tests proving that authenticated identity overrides a caller-supplied producer ID.

## Evidence
- Current  `.proto` files: `proto/logs/v1/record.proto`, `proto/logs/v1/service.proto`
- Generated API: `api/logs/v1/record.pb.go`, `proto/logs/v1/service.pb.go`, `proto/logs/v1/service_grpc.pb.go`
- Implementation: `internal/segment/segment.go`
  - `Segment.Append()`
- Tests: `internal/segment/segment_test.go`
  - `TestSegmentAppend`
  - `TestSegmentAppendAssignsMetadata`
  - `TestSegmentCorruptPayloadFailsChecksum`
  - `TestSegmentAppendAndRead`
- Server Tests: `internal/server/server_test.go`
  - `TestProduce Ln-311`

## Revisit trigger

Revise this decision if implementation evidence exposes integrity gaps, ambiguous timestamp semantics, identity requirements, or a workload that requires keyed records. Preserve the revision history and explain the reason for any change.
