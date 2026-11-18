The ingest and mapping pages had never been exercised through a browser. Getting a single assertion to run required four fixes that only an end-to-end test could find.

## What the pages exposed

**The UI could not reach ingest.** `webEndpoints` never handed Web an ingest address, so `next.config` used its hardcoded fallback — the reverse proxy pointed at a port this stack does not use. It now derives `SPX_INGEST_ORIGIN` from the ingest service and `webEnvironment` lets the key through.

**Every ingest API call failed with "Missing API response model".** The four API modules bypassed the shared `request` helper, so no response boundary was attached and the interceptor rejected the response before the page saw data. They now use `request`, gaining the same Zod validation the IAM modules have.

**Three contract mismatches behind the helper:**

| Was | Now |
|---|---|
| `CreateInputResponse.Input` serialised as `source` | `input` |
| `Mapping.Spec` / `MappingRequest.Spec` | `definition` (column, repository and page already said so) |
| create returned 200 | 201 via `result.Created`, matching IAM; router test updated |

**ingest had no `url` field at all.** `SPX_POSTGRES_URL` was silently ignored — the DSN was always rebuilt from host/port/name — so ingest was the only module that could connect to a different database than every tool around it. This is what made the harness's `ingest migrate` fail. Fixed, and the migration error now wraps its cause instead of printing "could not apply migrations" and discarding the SQL error.

## The new case

`ingest.spec.ts` drives the real pages: create a mapping, create an input, read the one-time token the page displays, then push a device event with that token and expect 202 — through the Web reverse proxy, into ingest, out to Kafka.

It polls until the registry reload admits the new input. A just-created input is deliberately not routable yet (the loader refreshes on an interval); asserting immediately would be testing a contract the system does not offer.

## Supporting changes

- The e2e harness now builds, migrates and starts ingest, requires `SPX_TEST_KAFKA_BROKERS` (or `--docker`, which boots a single-node KRaft Kafka), and includes `ingest.log` in the secret scan.
- `scripts/start.mjs` migrates ingest before serving, like it already did for IAM.
- Plan index statuses were stale: `ingest-and-normalization` and `ingest-naming-and-authorization` still read "not implemented" after being merged, and `audit-role-settings-consolidation` read "in progress" after its acceptance points landed.

## Verification

```
playwright e2e         10 passed (9 auth + 1 ingest)
go test ./src/...      39 packages, 0 failures
ingest integration     13 packages ok (real PG + Redis + Kafka)
web typecheck          OK
web unit               69 passed
golangci-lint          0 issues
gofmt                  0 files
```

## Known gap (pre-existing, not from this change)

`scripts/build.test.mjs` fails on Windows: the fixture writes `go`/`pnpm` shims that `shell: true` cannot resolve. It fails identically on `main` — verified in a pristine worktree — and passes on Linux CI.
