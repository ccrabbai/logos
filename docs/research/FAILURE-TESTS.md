# Logos Failure Matrix and Test Evidence

**Status:** Audit worksheet. Test names and coverage must be confirmed against the current checkout before marking a scenario verified.

## Failure matrix

| Scenario | Expected behavior to define | Evidence required | Current assessment |
|---|---|---|---|
| Nil Produce request or record | Return `InvalidArgument`; do not append | Server unit test | Reported covered |
| Nil Consume request | Return `InvalidArgument`; do not read | Server unit test | Reported covered |
| Missing peer/auth information | Reject as unauthenticated | `authenticate` tests | Reported covered |
| Non-TLS or unverified peer | Reject as unauthenticated | `authenticate` tests | Reported covered |
| Empty certificate CommonName | Reject as unauthenticated | `authenticate` tests | Reported covered |
| Authenticated producer identity | Use authenticated identity rather than caller-supplied producer ID | Produce / ProduceStream tests | Reported covered after test-helper fix |
| Store append returns an error | Return failure; do not falsely acknowledge success; treat uncertain store instance as unusable per DD-002 | Injected Store failure | Reported covered |
| Store append succeeds, Index write fails | Attempt rollback of uncommitted tail; do not advance `nextOffset` | Injected Index failure | Reported covered |
| Rollback fails | Mark state unusable or fail closed; do not continue blindly | Injected rollback failure | Must audit |
| Process stops during append | Recovery retains only records allowed by the documented crash/durability contract | Controlled interruption/restart | Not established by ordinary unit tests |
| Missing index | Rebuild or fail explicitly according to implemented policy | Recovery test with deleted index | Reported covered |
| Corrupt/truncated index | Rebuild or fail explicitly according to implemented policy | Recovery corruption test | Reported covered |
| Truncated/invalid store frame | Apply explicit policy; do not silently invent a valid record | Recovery malformed-frame test | Reported covered |
| Restart after successful appends | Restore expected records and logical offsets within the stated durability boundary | Close/reopen test | Some recovery tests reported; exact coverage must be checked |
| Segment store capacity reached | Rotate or reject according to Log behavior; preserve offsets | Segment/Log boundary tests | Reported covered |
| Segment index capacity reached | Rotate or reject according to Log behavior; preserve offsets | Segment/Log boundary tests | Reported covered |
| ConsumeStream reaches end of available range | Follow documented behavior and terminate on cancellation | Streaming test | Reported covered |
| Client disconnect / stream Send failure | Return or handle the stream error without corrupting storage | Streaming test | Must audit |
| Unauthorized RPC | Reject before invoking handler | End-to-end gRPC interceptor test | Direct handler tests do not prove interceptor enforcement |
| TLS/RBAC integration | Only authenticated and authorized RPCs reach the handler | bufconn or local TLS integration test | Reported covered |
| Observability exporter unavailable | Startup/runtime behavior follows documented policy | Integration test or documented operational test | Reported covered |

## Interpretation rules

1. A test that injects a returned error is not equivalent to a real process crash.
2. A close/reopen test is not equivalent to sudden power loss.
3. Unit tests of `authenticate` and direct handler methods do not, by themselves, verify the complete interceptor chain.
4. A passing full suite establishes that the tests passed; it does not prove untested scenarios.
5. Link each verified row to its exact test name and source file.

## Reported test commands

```bash
go test ./...
go test -race ./...
```

The functional suite was reported passing for all packages. The project-wide race run was incomplete because Windows returned `Access is denied` while starting `internal/segment`'s test executable. See `DURABILITY-AND-TEST-STATUS.md`.

## Gate

Before closing this document, replace every “must audit” / “reported covered” status with a verified test name, an explicit gap, or a documented reason the scenario is out of scope.
