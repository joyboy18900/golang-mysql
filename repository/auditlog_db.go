package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

const auditLogListByActorQuery = `SELECT id, actor_id, action, entity_type, entity_id, metadata, created_at
FROM audit_log
WHERE actor_id = ?
ORDER BY created_at DESC
LIMIT ?`

const emptyMetadataJSON = "{}"

type auditLogRepositoryDB struct {
	db *sql.DB
}

func NewAuditLogRepositoryDB(db *sql.DB) AuditLogRepository {
	return auditLogRepositoryDB{db: db}
}

func (r auditLogRepositoryDB) Create(ctx context.Context, entry AuditLog) (*AuditLog, error) {
	metadata, err := marshalMetadata(entry.Metadata)
	if err != nil {
		return nil, fmt.Errorf("create audit log: %w", err)
	}

	result, err := r.db.ExecContext(ctx,
		`INSERT INTO audit_log (actor_id, action, entity_type, entity_id, metadata)
		 VALUES (?, ?, ?, ?, ?)`,
		entry.ActorID, entry.Action, entry.EntityType, entry.EntityID, metadata,
	)
	if err != nil {
		return nil, fmt.Errorf("create audit log: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("create audit log: %w", err)
	}

	var createdAt time.Time
	if err := r.db.QueryRowContext(ctx, "SELECT created_at FROM audit_log WHERE id = ?", id).
		Scan(&createdAt); err != nil {
		return nil, fmt.Errorf("create audit log: %w", err)
	}

	created := entry
	created.ID = id
	created.CreatedAt = createdAt

	return &created, nil
}

func (r auditLogRepositoryDB) ListByActor(ctx context.Context, actorID int64, limit int) ([]AuditLog, error) {
	rows, err := r.db.QueryContext(ctx, auditLogListByActorQuery, actorID, limit)
	if err != nil {
		return nil, fmt.Errorf("list audit log by actor: %w", err)
	}
	defer rows.Close()

	entries := make([]AuditLog, 0, limit)
	for rows.Next() {
		entry, err := scanAuditLog(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("scan audit log: %w", err)
		}
		entries = append(entries, *entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate audit log rows: %w", err)
	}

	return entries, nil
}

func marshalMetadata(metadata map[string]any) (string, error) {
	if len(metadata) == 0 {
		return emptyMetadataJSON, nil
	}

	encoded, err := json.Marshal(metadata)
	if err != nil {
		return "", fmt.Errorf("marshal audit log metadata: %w", err)
	}

	return string(encoded), nil
}

func scanAuditLog(scan func(dest ...any) error) (*AuditLog, error) {
	var entry AuditLog
	var metadata []byte

	if err := scan(&entry.ID, &entry.ActorID, &entry.Action, &entry.EntityType,
		&entry.EntityID, &metadata, &entry.CreatedAt); err != nil {
		return nil, err
	}

	if len(metadata) > 0 {
		if err := json.Unmarshal(metadata, &entry.Metadata); err != nil {
			return nil, fmt.Errorf("unmarshal audit log metadata: %w", err)
		}
	}

	return &entry, nil
}
