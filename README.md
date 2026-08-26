# golang-mysql

MySQL schema evolution and query performance, proven with real numbers:
versioned migrations, an indexing case study, and monthly table
partitioning with a working pruning demo. One entity, `audit_log`, carries
the whole story: an actor's activity history query that starts unindexed,
gets indexed, then gets partitioned by month. Mirrors `golang-postgresql`
domain-for-domain, ported to MySQL rather than translated line by line -
see "Differs from Postgres" below for where the two diverge and why.

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
latency from a real timed query - see "Differs from Postgres" for why)
plus the full `EXPLAIN FORMAT=JSON` output.

## Schema (`migrations/`)

1. **`0001_create_audit_log_table`** - plain, unindexed `audit_log`
   (`actor_id`, `action`, `entity_type`, `entity_id`, `metadata json`,
   `created_at datetime(6)`). Deliberately the "before" state.
2. **`0002_add_actor_id_index`** - `CREATE INDEX ... ON audit_log (actor_id,
   created_at DESC)`. Composite, not just `actor_id`, so it also covers the
   `ORDER BY ... LIMIT` for free.
3. **`0003_partition_audit_log_by_month`** - two plain `ALTER TABLE`
   statements: move the primary key to `(id, created_at)` (every unique key
   on a partitioned InnoDB table must include the partition column), then
   `PARTITION BY RANGE COLUMNS (created_at)`. Unlike Postgres, MySQL can
   partition an existing table in place - no build-new-table/copy/rename
   dance, no leftover unpartitioned copy. `down` is
   `ALTER TABLE audit_log REMOVE PARTITIONING` plus restoring the original
   single-column primary key.

## Indexing case study

Query: `SELECT ... FROM audit_log WHERE actor_id = ? ORDER BY created_at
DESC LIMIT 50`, a user's activity history lookup. Seeded with 1,000,000
rows across 20,000 actors (actor `8237` below has 81 matching rows, a
realistic selectivity).

**Before** (`migrate goto 1`, no index):

```
ALL on audit_log, Execution Time: 243.49 ms
```

Full table scan (`access_type: "ALL"`) plus a separate filesort step
(`using_filesort: true`) to satisfy the `ORDER BY`, since there's no index
to walk in order.

**After** (`migrate goto 2`, composite index added):

```
ref using idx_audit_log_actor_id_created_at on audit_log, Execution Time: 0.72 ms
```

**~340x faster** (243.49 ms -> 0.72 ms). `using_filesort` also flips to
`false`: the composite index doesn't just find the matching rows, its
column order already satisfies `ORDER BY created_at DESC`, so the sort
step disappears too - not just the scan.

## Partitioning

Partitioned by month on `created_at`, four explicit partitions
(`p2026_05` .. `p2026_08`) bracketing the seeded data's date range, plus a
`p_max` catch-all (`VALUES LESS THAN (MAXVALUE)` - MySQL's equivalent of
Postgres's `DEFAULT` partition).

**Why `created_at` and monthly**: audit logs are append-only and almost
always queried by recency. Partitioning on the column every write and most
reads touch means inserts and range reads land in one partition, and old
months drop with a fast metadata-only `ALTER TABLE ... DROP PARTITION`
instead of a row-by-row `DELETE`.

**Proof of pruning**: a `created_at`-range query touches only the matching
partition.

```sql
EXPLAIN FORMAT=JSON
SELECT * FROM audit_log
WHERE created_at >= '2026-08-01' AND created_at < '2026-09-01' LIMIT 50;
```

```json
"partitions": ["p2026_08"]
```

`rows_examined_per_scan` (251,572) matches `p2026_08`'s own row count
exactly - only that partition was ever touched. Row counts show an even
spread, nothing fell into `p_max`:

| partition | rows |
|---|---|
| p2026_05 | 250,403 |
| p2026_06 | 243,461 |
| p2026_07 | 251,572 |
| p2026_08 | 245,050 |
| p_max | 0 |

**Tradeoffs**: the actor-lookup query doesn't prune, since `actor_id` isn't
the partition key. A real `EXPLAIN FORMAT=JSON` run against the partitioned
table confirms it: `"partitions": ["p2026_05", "p2026_06", "p2026_07",
"p2026_08", "p_max"]` - every partition checked, via each partition's own
copy of the index. Partitioning and the index solve different problems,
and both are needed. Rows outside the four declared months land in
`p_max`; moving them into a real partition later needs `ALTER TABLE ...
REORGANIZE PARTITION`, with no automatic rebalancing. A production setup
would add a cron job to pre-create next month's partition before rows
start arriving for it; out of scope here.

## Differs from Postgres

Concrete, verified-live differences for anyone comparing this repo with
`golang-postgresql` (all checked against a real `mysql:8.4` container, not
assumed from documentation):

- **Partitioning is in-place.** Postgres cannot `ALTER TABLE ... PARTITION
  BY` an existing table - it requires building a new partitioned table,
  copying data, and renaming into place, leaving the old table around
  purely so `down` has something to restore. MySQL's `ALTER TABLE ...
  PARTITION BY RANGE COLUMNS (...)` works directly on the live table, and
  `ALTER TABLE ... REMOVE PARTITIONING` cleanly reverses it. Confirmed
  live: existing rows survive the alter and route into the correct
  partition immediately, no data movement step required.
- **`RANGE COLUMNS` rejects `TIMESTAMP`.** MySQL's `PARTITION BY RANGE
  COLUMNS` accepts `DATETIME` but errors on `TIMESTAMP` (`ERROR 1659:
  Field 'created_at' is of a not allowed type for this type of
  partitioning`). `created_at` is `DATETIME(6)` here, which - unlike
  Postgres's `TIMESTAMPTZ` or MySQL's own `TIMESTAMP` - stores whatever
  value it's given with no timezone conversion. The app writes and reads
  UTC consistently itself; there's no server-side conversion to rely on.
- **`MAXVALUE` vs `DEFAULT` partition.** Postgres's catch-all partition is
  declared with `PARTITION ... DEFAULT`. MySQL's equivalent is
  `PARTITION p_max VALUES LESS THAN (MAXVALUE)` - a boundary value, not a
  distinct partition kind.
- **`EXPLAIN ANALYZE` has no JSON format.** Postgres's `EXPLAIN (ANALYZE,
  BUFFERS, FORMAT JSON)` gives one call structured plan + real timing.
  MySQL's `EXPLAIN ANALYZE` only supports `FORMAT=TREE` (confirmed live:
  `EXPLAIN ANALYZE FORMAT=JSON` errors with "This version of MySQL doesn't
  yet support 'EXPLAIN ANALYZE with JSON format'"). This repo's `bench`
  therefore gets plan **shape** from plain `EXPLAIN FORMAT=JSON`
  (structured, no execution) and **latency** from Go wall-clock timing
  around the real query - two calls instead of Postgres's one. MySQL's
  JSON shape also differs structurally from Postgres's: a single object
  (not a top-level array), the access-describing node
  (`table_name`/`access_type`/`key`/`partitions`) nested under different
  wrapper keys depending on the query (`ordering_operation` for a sort,
  `grouping_operation` for `GROUP BY`) rather than Postgres's uniform
  `Plans` array, so `bench_service.go`'s plan walker searches the whole
  tree instead of assuming a fixed path.
- **Isolation level and locking.** MySQL/InnoDB defaults to
  `REPEATABLE READ` (`SELECT @@transaction_isolation`); Postgres defaults
  to `READ COMMITTED`. Confirmed live with a two-session demo: a
  transaction runs `SELECT * FROM audit_log WHERE actor_id = 5 FOR UPDATE`
  when zero rows match `actor_id = 5` - InnoDB still takes a gap lock on
  that index range. A second session's `INSERT ... (actor_id=5, ...)` then
  blocks and eventually fails with `ERROR 1205: Lock wait timeout
  exceeded`, even though the first transaction never touched a real row.
  Under Postgres's `READ COMMITTED` + pure row-level MVCC locking, there
  is nothing to lock when no row matches, so the equivalent insert would
  not block.
- **No `COPY`.** Postgres's pgx driver has a binary `COPY` protocol
  (`CopyFrom`) that seeds 1M rows in ~2s. `database/sql` + MySQL have no
  equivalent bulk-load primitive short of `LOAD DATA INFILE` (server-side
  file access, more operational surface than this demo needs). The
  repository's `BatchInsert` instead builds chunked multi-row
  `INSERT ... VALUES (...),(...),...` statements inside one transaction
  per batch - still fast enough in practice: 1,000,000 rows in ~8-12s.
- **`AUTO_INCREMENT` vs `SERIAL`, `JSON` vs `JSONB`.** Postgres's
  `BIGSERIAL` is sugar over a sequence with a default; MySQL's
  `BIGINT AUTO_INCREMENT` is a column attribute with no separate sequence
  object, and there's no `pg_get_serial_sequence`/`setval` equivalent to
  reseed it - `ALTER TABLE ... AUTO_INCREMENT = N` instead. MySQL's `JSON`
  type has no binary/indexable-by-default variant like `JSONB`; a
  non-literal column default needs the `DEFAULT (expr)` syntax (here,
  `DEFAULT (JSON_OBJECT())`), only valid since MySQL 8.0.13.
- **No `RETURNING`.** Postgres's `INSERT ... RETURNING` hands back the
  inserted row (including DB-generated `created_at`) in one round trip.
  MySQL has no `RETURNING`; `Create` does `Exec` + `LastInsertId()` then a
  follow-up `SELECT ... WHERE id = ?` to read back the row.
- **`[]byte` params into a `JSON` column fail.** A real driver quirk hit
  while building this: passing `json.Marshal`'s `[]byte` result straight
  as a query parameter for a `JSON` column errors with `Error 3144:
  Cannot create a JSON value from a string with CHARACTER SET 'binary'` -
  go-sql-driver/mysql sends `[]byte` as a binary-charset parameter, which
  MySQL's `JSON` type refuses to implicitly convert. Fixed by passing
  `string(metadata)` instead.
- **golang-migrate operational quirks.** MySQL DDL auto-commits (no
  transactional `ALTER TABLE`), so a migration killed partway through
  leaves golang-migrate's version row `dirty` and needs a manual
  `migrate force`, unlike Postgres where a failed migration rolls back
  cleanly. Separately, golang-migrate's MySQL driver needs
  `multiStatements=true` on its DSN to run a migration file with more than
  one statement (migration `0003` has two `ALTER TABLE`s) - the app's own
  runtime connection deliberately omits that flag, since it never needs
  to run multiple statements per query.

## API

Two endpoints, standard envelope (`{code, message, data}`):

- `POST /audit-log` - create one entry
- `GET /audit-log?actor_id=X&limit=Y` - the case-study query (`limit`
  defaults to 50)

See `curl/flow.md` for examples.

`migrate`, `seed`, and `bench` stay CLI subcommands (`go run . <cmd>`), not
endpoints - they're one-time proof steps for this README, not things an API
client should trigger.

## Tests

```bash
go test ./...
```

- `service/bench_service_test.go` - parses `EXPLAIN FORMAT=JSON` output,
  including the plain table-scan, indexed, pruned-partition, and
  `ordering_operation`-wrapped shapes MySQL actually returns at this
  scale.
- `service/seed_service_test.go` - batch splitting during seeding, with a
  mocked repository.
- `audit_log_integration_test.go` - against a real MySQL: migrations
  round-trip cleanly; the actor query's plan is a full table scan
  (`access_type: "ALL"`) before the index and not after; a row lands in
  the right month's partition (checked directly via
  `... PARTITION (p2026_06) WHERE id = ?`); a `created_at`-range query
  prunes to one partition; both endpoints return the right envelope and
  validation errors.

The integration test owns the database it connects to: it migrates down to
clean, seeds its own data, and migrates back down when done. Run it
against a scratch MySQL, not the `docker compose` stack from option A, or
it will wipe that demo data.

## Not done on purpose

No cron-based automatic future-partition creation, no `p_max`-partition
rebalancing tooling, no `LOAD DATA INFILE` (see "No `COPY`" above), no
retry/backoff on seeding, no auth on the API (out of scope for this
project - see `golang-fiber-jwt-auth`).
