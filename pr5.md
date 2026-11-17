## Real broker, real behaviour

The ordering logic was covered by unit tests; the wire behaviour was not. With `SPX_TEST_KAFKA_BROKERS` set, two cases now run against an actual broker:

| Case | What it proves |
|---|---|
| `TestConsumerReadsBacklogFromEarliest` | three records produced before the consumer starts are all consumed — the point of `ConsumeResetOffset(AtStart)` |
| `TestOffsetsStayUncommittedWhenPublishFails` | a consumer whose publisher always fails cannot commit, so a second consumer in the same group receives the same record again |

The second one is the observable evidence for "publish before commit". A fake publisher cannot show it, which is why this needed a broker.

## Three things the run exposed

**The integration runner never ran ingest.** `tests/integration/run.mjs` only listed `iam` and `console`, so every ingest integration package had been invisible to CI. Added.

**The config tests were environment dependent.** They assert YAML defaults but inherited the caller's shell, so sourcing `.env` made them fail with values from the local stack. I had been writing that off as "local noise" — it is not: a test that changes result based on ambient variables is broken. A `TestMain` now clears `SPX_*` before running.

**`.env` and `.env.example` had drifted.** `.env` still carried the removed `SPX_WEB_BIND_HOST`, put IAM session keys under the Redis header, left ingest keys floating at the end, orphaned comments ("同上，只绑定回环地址"), and missed keys the template required. Both files are now generated from a single definition — same sections, same order, same comments — so they cannot drift again.

## Verification against 10.41.84.81 (PostgreSQL, Redis, Kafka)

```
go build / go vet      OK
go test ./src/...      39 packages, 0 failures
make lint-go           0 issues
gofmt -l src/          0 files
ingest integration     all packages ok, incl. both Kafka cases
iam integration        all packages ok
check-config           iam / console / ingest valid with the regenerated .env
```

One local-only failure remains, unrelated to this change: `TestBackupRestoreRevokesRestoredCredentialsBeforeServing` needs a `pg_dump` matching the server major version; the machine's client tool is older. CI installs `postgresql-client-16` and passes.
