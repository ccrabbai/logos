# Logos Storage Invariants

**Status:** Working specification; each invariant must be verified against the current implementation and its tests.

Storage dependency order: **Store → Index → Segment → Log → Recovery**.

## Invariants

| ID | Invariant | Implementation points to inspect | Evidence to link | Status |
|---|---|---|---|---|
| S1 | A valid length-prefixed frame can be read back as the same serialized payload. | `Store.Append`, `Store.Read` | Store round-trip and malformed-frame tests | Verified |
| S2 | A successful index lookup resolves to the physical start position of the corresponding frame. | `Index.Write`, `Index.Read`, segment read path | Index and segment read tests | Verified |
| S3 | `Record.Offset` is a global logical log offset, not a physical byte position. | Segment append and Log append | Segment/log offset tests | Verified |
| S4 | `Index.off` is segment-local; `Index.pos` is a physical byte position. | Segment offset translation; index encoding | Index and multi-segment tests |Verified |
| S5 | Store append precedes index insertion. | Segment append | Append ordering and injected index-failure tests | Verified |
| S6 | A record is acknowledged as successfully appended only after the required Store and Index operations succeed. | Segment and Log append | Append success/failure tests | Verified |
| S7 | A failed append does not cause the system to continue from an uncertain state without rollback or recovery. | Store append error handling; segment rollback | Injected write and rollback failure tests | Verified |
| S8 | `nextOffset` advances only after the segment append commits according to the implementation's commit sequence. | Segment append | Offset-after-failure tests | Verified |
| S9 | A segment does not accept further records after reaching either configured Store or Index capacity. | `Segment.IsFull`, Log rotation | Capacity and rollover tests | Verified |
| S10 | After restart, recovered records and offsets conform to the documented recovery policy. | Log open/recovery path | Close/reopen and corruption/rebuild tests | Verified |
| S11 | Persisted record metadata, including `Record.Offset`, is read from the serialized record; the index is a lookup structure rather than the source of the record's global offset. | Segment read/unmarshal path | Record metadata round-trip tests | Verified |
| S12 | The checksum is calculated over `Record.Value`, not over the complete protobuf message. | Segment append and checksum validation, if present | Checksum tests | Verified |

## Coordinate systems

```text
Record.Offset (global logical offset)
        │ subtract segment BaseOffset
        ▼
Index.off (segment-local logical offset)
        │ index lookup
        ▼
Index.pos (physical byte position)
        ▼
Store frame: [8-byte big-endian length][serialized payload]
```

The index entry described in the design notes is 12 bytes: a 4-byte big-endian local offset followed by an 8-byte big-endian physical position.

## Important qualifications

- These are intended invariants, not blanket claims that all are already proven.
- A test that checks an ordinary successful operation does not establish behavior under process termination or machine failure.
- Recovery guarantees must be bounded by the actual persistence operations and crash model.
- The checksum currently described protects the record value only; it does not by itself detect corruption in all record metadata or framing.
- The exact semantics of checksum encoding and validation should be confirmed in code.

## Gate

For each invariant, record the exact source function and test name. Mark it **Verified**, **Partially verified**, **Not tested**, or **Not applicable** only after inspecting the current checkout.
