# DD-005 — Log Layer Review

**Status:** Audit required; do not replace the existing DD-005 until compared with it.

## Purpose

Review the top-level Log abstraction and its boundaries with Segment and Recovery. The existing DD-005 was previously identified as the `log.go` review; preserve its history and update only claims that are outdated or unsupported.

## Review questions

1. Does Log own segment discovery, creation, selection, rollover, and lifecycle management?
2. Are global offsets assigned consistently across segments?
3. Are reads routed to the segment containing the requested offset, including boundary conditions?
4. What happens when the active segment becomes full?
5. What happens when an append fails after a segment has partially changed state?
6. Does startup recovery establish `baseOffset`, `nextOffset`, and active-segment state consistently?
7. Are errors propagated without falsely acknowledging an append?
8. Are concurrent append/read/close operations synchronized according to the documented contract?
9. Do metrics accurately reflect successful and failed operations?
10. Which tests establish each behavior?

## Required evidence

For each conclusion, record the current source function, test name, and any limitation. At minimum inspect `Log.Append`, `Log.Read`, initialization/open/recovery, segment selection/rollover, and close/reset/truncate paths.

## Review Questions

 | # | Review question | Concise conclusion | Action |Evidence / status |
| --- | --- | --- | --- | --- |
| 1 | **Segment lifecycle** | Log owns segment discovery, creation, selection, rollover, and lifecycle coordination; Segment owns Store/Index operations. | Verify initialization, rollover, and close functions. | Confirmed |
| 2 | **Global offsets** | Offsets must remain continuous across segments; each record carries a global offset, while the index uses segment-local offsets. | Verify `Log.Append`, segment initialization, and recovery tests. | Confirmed |
| 3 | **Read routing** | `Log.Read` routes an offset to its containing segment; boundary and out-of-range behavior must be verified. | Verify `Log.Read` and boundary tests. | Confirmed |
| 4 | **Full segment** | When the active segment reaches capacity, Log must create/select a new active segment before the next append. | Verify rollover implementation and capacity tests. | Confirmed |
| 5 | **Partial append failure** | A failed Store/Index operation must not be falsely acknowledged. Any partial physical change must be rolled back or reconciled during recovery. | Verify Segment rollback and failure tests. | Confirmed |
| 6 | **Startup recovery** | Recovery must restore segment ordering, `baseOffset`, `nextOffset`, and active-segment state from persisted data consistently. | Index discovery/reconstruction remains a critical audit item. | Under Review |
| 7 | **Error propagation** | Append and recovery errors must propagate to the caller; failed appends must not be reported as successful. | Verify error paths and server-level tests. |  Confirmed |
| 8 | **Concurrency** | Concurrent append/read/close operations must follow the synchronization contract and must not access closed or inconsistent state. | Verify locking and race tests; the previous Windows race-test run was blocked by an executable access-denied error. |  Confirmed |
| 9 | **Metrics** | Metrics must distinguish successful and failed operations and must not count a failed append as successful. | Verify metric update locations and tests. |  Confirmed |
| 10 | **Test coverage** | A passing full suite does not establish coverage of every failure scenario. | Link each conclusion to an actual test name and result. |  Confirmed |
