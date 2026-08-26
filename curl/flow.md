# Manual test flow

Walkthrough for exercising the audit log API by hand. `docker compose up`
migrates, seeds 1,000,000 rows, and starts the server - see the README for
the separate manual before/after indexing flow.

## Start

```bash
docker compose up -d --build
docker compose ps
docker compose logs app --tail 20   # should show "server started on port 8080"
```

## 1. Create an audit log entry

```bash
curl -X POST http://localhost:8080/audit-log \
  -H "Content-Type: application/json" \
  -d '{"actor_id":42,"action":"login","entity_type":"session"}'
```

```json
{ "code": 201, "message": "audit log entry created", "data": { "id": 1000001, "actor_id": 42, "action": "login", "entity_type": "session", "entity_id": null, "metadata": {}, "created_at": "2026-08-26T12:25:03.9061Z" } }
```

## 2. List an actor's activity history

This is the query the indexing case study is built around.

```bash
curl "http://localhost:8080/audit-log?actor_id=42"
```

```json
{ "code": 200, "message": "audit log entries retrieved", "data": [ { "id": 1000001, "actor_id": 42, "action": "login", "entity_type": "session", "entity_id": null, "metadata": {}, "created_at": "2026-08-26T12:25:03.9061Z" } ] }
```

`limit` defaults to 50 and can be overridden: `?actor_id=42&limit=10`.

## 3. Rejection cases

Missing `actor_id`:

```bash
curl "http://localhost:8080/audit-log"
```

```json
{ "code": 422, "message": "actor_id query parameter is required", "data": null }
```

Missing required fields on create:

```bash
curl -X POST http://localhost:8080/audit-log \
  -H "Content-Type: application/json" \
  -d '{"actor_id":42}'
```

```json
{ "code": 422, "message": "actor_id, action and entity_type are required", "data": null }
```

## Stop

```bash
docker compose down -v
```
