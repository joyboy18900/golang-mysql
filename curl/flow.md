# Manual test flow

Walkthrough for exercising the audit log API by hand. `docker compose up`
migrates and starts the server.

## Start

```bash
docker compose up -d --build
docker compose ps
docker compose logs app --tail 20   # should show "server started on port 8080"
```

## 1. Create audit log entries

```bash
curl -X POST http://localhost:8080/audit-log \
  -H "Content-Type: application/json" \
  -d '{"actor_id":42,"action":"login","entity_type":"session"}'
```

```json
{ "code": 201, "message": "audit log entry created", "data": { "id": 1, "actor_id": 42, "action": "login", "entity_type": "session", "entity_id": null, "metadata": {}, "created_at": "2026-08-26T15:49:39.779Z" } }
```

```bash
curl -X POST http://localhost:8080/audit-log \
  -H "Content-Type: application/json" \
  -d '{"actor_id":42,"action":"update","entity_type":"order","metadata":{"foo":"bar"}}'
```

```json
{ "code": 201, "message": "audit log entry created", "data": { "id": 2, "actor_id": 42, "action": "update", "entity_type": "order", "entity_id": null, "metadata": {"foo":"bar"}, "created_at": "2026-08-26T15:49:39.81Z" } }
```

## 2. List an actor's activity history, one page at a time

```bash
curl "http://localhost:8080/audit-log?actor_id=42&limit=1"
```

```json
{ "code": 200, "message": "audit log entries retrieved", "data": { "items": [ { "id": 2, "actor_id": 42, "action": "update", "entity_type": "order", "entity_id": null, "metadata": {"foo":"bar"}, "created_at": "2026-08-26T15:49:39.81Z" } ], "next_cursor": "MTc4Nzc1OTM3OTgxMDAwMDoy" } }
```

`limit` defaults to 50. Pass `next_cursor` back as `cursor` to get the next
page:

```bash
curl "http://localhost:8080/audit-log?actor_id=42&limit=1&cursor=MTc4Nzc1OTM3OTgxMDAwMDoy"
```

```json
{ "code": 200, "message": "audit log entries retrieved", "data": { "items": [ { "id": 1, "actor_id": 42, "action": "login", "entity_type": "session", "entity_id": null, "metadata": {}, "created_at": "2026-08-26T15:49:39.779Z" } ], "next_cursor": null } }
```

`next_cursor: null` means this was the last page.

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

Malformed cursor:

```bash
curl "http://localhost:8080/audit-log?actor_id=42&cursor=not-a-real-cursor"
```

```json
{ "code": 422, "message": "invalid cursor", "data": null }
```

## Stop

```bash
docker compose down -v
```
