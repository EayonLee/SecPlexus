Fix-forward for #96, which merged while CI was red.

## Why CI failed

All five failing jobs came from four causes, none of them product behaviour:

| Cause | Fixed by |
|---|---|
| IAM now rejects two service clients sharing a token; the preflight test sent one token for both | the test supplies a separate `SPX_INGEST_IAM_TOKEN` |
| The preflight test drove the removed `SPX_CONSOLE_BIND_PORT` | it sets `SPX_CONSOLE_PORT`, the only port a service has now |
| The two integration tests added in #96 read the shared database, which is only migrated on a developer machine | they use the isolated migrated-database helper |
| Two assertions pinned 7 permissions and "all delegable" | compared against the catalog instead |

Additionally: the pull-connector navigation assertion outlived the feature, and the address rename left duplicate object keys in the e2e harness (the six biome errors).

## Verification

```
make test                       69 web tests + script suite pass
go build / go vet               OK
go test ./src/...               39 packages, 0 failures
make lint-go                    0 issues
integration · iam               11 packages ok (PostgreSQL + Redis)
integration · ingest            13 packages ok
web lint                        no errors
```

The one behavioural change worth calling out: the baseline role no longer grants `iam.users.read`. Listing accounts is not a "logged in and done" capability, and the earlier inclusion existed only because the SSE route borrowed that permission before `console.events.read` was added.
