# golang-mysql

MySQL schema evolution and query performance, proven with real numbers:
versioned migrations, an indexing case study, and monthly table
partitioning with a working pruning demo. One entity, `audit_log`, carries
the whole story: an actor's activity history query that starts unindexed,
gets indexed, then gets partitioned by month. Mirrors `golang-postgresql`
domain-for-domain, ported to MySQL rather than translated line by line -
see "Differs from Postgres" below.

## Run

### Option A: docker compose (full stack, already migrated and seeded)

```bash
docker compose up -d --build
docker compose logs seed --tail 5   # "seeded 1000000 rows total"
curl "http://localhost:8080/audit-log?actor_id=42"
```

Runs `migrate up` (through the partitioned schema), seeds 1,000,000 rows
(~10s via batched `INSERT`), then starts the API on `:8080`. See
`curl/flow.md` for requests. **Does not reproduce the before/after numbers
below** - the index and partitioning already exist by the time `app`
starts. Use option B for that.

### Option B: manual before/after reproduction

Requires a local MySQL reachable via `config.yaml` (or `docker compose up
-d mysql`).

```bash
go run . migrate goto 1     # table exists, no index - the "before" state
go run . seed 1000000       # ~1M rows, batched INSERT, ANALYZE TABLE at the end
go run . bench <actor_id>   # capture "before" numbers (pick an actor_id from the seeded data)
go run . migrate goto 2     # add the index - the "after" state
go run . bench <actor_id>   # capture "after" numbers
go run . migrate goto 3     # convert to monthly partitions
```

`bench` prints a one-line summary (plan shape from `EXPLAIN FORMAT=JSON`,
latency from a real timed query) plus the full `EXPLAIN FORMAT=JSON`
output.

## Schema (`migrations/`)

1. **`0001_create_audit_log_table`** - plain, unindexed `audit_log`
   (`actor_id`, `action`, `entity_type`, `entity_id`, `metadata json`,
   `created_at datetime(6)`). Deliberately the "before" state.
2. **`0002_add_actor_id_index`** - composite index on `(actor_id,
   created_at DESC)`, so it covers the `ORDER BY ... LIMIT` too.
3. **`0003_partition_audit_log_by_month`** - two `ALTER TABLE`
   statements: move the primary key to `(id, created_at)` (a partitioned
   InnoDB table's unique keys must include the partition column), then
   `PARTITION BY RANGE COLUMNS (created_at)`. Unlike Postgres, MySQL
   partitions an existing table in place - no build-new/copy/rename, no
   leftover unpartitioned copy.

## Benchmark Data

Query: `SELECT ... FROM audit_log WHERE actor_id = ? ORDER BY created_at
DESC LIMIT 50`. Seeded with 1,000,000 rows across 20,000 actors.

**Indexing** (actor `8237`, 81 matching rows):

| | plan | filesort | latency |
|---|---|---|---|
| before (`migrate goto 1`) | `ALL` (full scan) | yes | 243.49 ms |
| after (`migrate goto 2`) | `ref` (index) | no | 0.72 ms |

~340x faster. The composite index's column order already satisfies
`ORDER BY created_at DESC`, so the filesort step disappears too.

**Partition pruning** (`created_at`-range query on the partitioned
table): `EXPLAIN FORMAT=JSON` reports `"partitions": ["p2026_08"]` only;
`rows_examined_per_scan` (251,572) matches that partition's own row count
exactly.

| partition | rows |
|---|---|
| p2026_05 | 251,280 |
| p2026_06 | 242,981 |
| p2026_07 | 250,987 |
| p2026_08 | 251,572 |
| p_max | 0 |

## Key Technical Takeaways / Gotchas

- Partitioned by month on `created_at`, four explicit partitions
  (`p2026_05`..`p2026_08`) plus `p_max` catch-all - old months drop with a
  metadata-only `ALTER TABLE ... DROP PARTITION` instead of a row-by-row
  `DELETE`.
- The actor-lookup query doesn't prune (`actor_id` isn't the partition
  key) - every partition gets checked. Partitioning and the index solve
  different problems; both are needed.
- Rows outside the four declared months land in `p_max`; moving them out
  later needs `ALTER TABLE ... REORGANIZE PARTITION`, no automatic
  rebalancing.

## Differs from Postgres

Verified live against a real `mysql:8.4` container, not assumed from docs:

- In-place partitioning: `ALTER TABLE ... PARTITION BY RANGE COLUMNS`
  works directly on the live table; Postgres needs build-new/copy/rename.
- `RANGE COLUMNS` rejects `TIMESTAMP` (`ERROR 1659`); `created_at` uses
  `DATETIME(6)` instead, so the app owns UTC handling itself.
- Catch-all partition is `p_max VALUES LESS THAN (MAXVALUE)`, not
  Postgres's `DEFAULT` partition kind.
- No JSON `EXPLAIN ANALYZE` (`FORMAT=TREE` only) - `bench` gets plan
  shape from plain `EXPLAIN FORMAT=JSON` and latency from Go wall-clock
  timing, two calls instead of Postgres's one.
- `REPEATABLE READ` + gap locks by default (Postgres: `READ COMMITTED`):
  a `SELECT ... FOR UPDATE` matching zero rows still blocks a later
  `INSERT` into that range with a lock-wait timeout.
- No `COPY`/bulk-load primitive: seeding uses chunked multi-row `INSERT`
  in one transaction per batch (~8-12s for 1M rows vs pgx `CopyFrom`'s
  ~2s).
- `AUTO_INCREMENT` (column attribute, no sequence object) instead of
  `SERIAL`; `JSON` (no `JSONB`) needs `DEFAULT (JSON_OBJECT())` syntax for
  a non-literal default.
- No `RETURNING`: `Create` does `Exec` + `LastInsertId()` + a follow-up
  `SELECT`.
- `[]byte` params into a `JSON` column fail (binary-charset error) - pass
  `string(metadata)` instead.
- golang-migrate needs `multiStatements=true` on its DSN for
  multi-statement migration files; MySQL DDL auto-commits, so a failed
  migration leaves the version row `dirty` and needs a manual `migrate
  force <N>`.

## API

- `POST /audit-log` - create one entry
- `GET /audit-log?actor_id=X&limit=Y` - the case-study query (`limit`
  defaults to 50)

See `curl/flow.md` for examples.

## Not done on purpose

- No cron-based automatic future-partition creation, no `p_max`-partition
  rebalancing tooling, no `LOAD DATA INFILE`.
- No retry/backoff on seeding.
- No auth on the API (see `golang-fiber-jwt-auth`).

## Tests

```bash
go test ./...
```

- `bench_service_test.go` - parses `EXPLAIN FORMAT=JSON` output, including
  the plain table-scan, indexed, and pruned-partition shapes.
- `seed_service_test.go` - batch splitting during seeding, mocked
  repository.
- `audit_log_integration_test.go` - migrations round-trip; plan is a full
  table scan before the index and not after; rows land in the right
  partition; range queries prune; both endpoints return the right
  envelope/errors.

The integration test owns its own database - run it against a scratch
MySQL, not the `docker compose` stack from option A, or it will wipe that
demo data.
