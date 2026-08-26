package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"
)

const emptyMetadataJSON = "{}"

type auditLogRow struct {
	ID         int64
	ActorID    int64
	Action     string
	EntityType string
	EntityID   *int64
	Metadata   string
	CreatedAt  time.Time
}

func (auditLogRow) TableName() string {
	return "audit_log"
}

type auditLogRepositoryDB struct {
	db *gorm.DB
}

func NewAuditLogRepositoryDB(db *gorm.DB) AuditLogRepository {
	return auditLogRepositoryDB{db: db}
}

func (r auditLogRepositoryDB) Create(ctx context.Context, entry AuditLog) (*AuditLog, error) {
	row, err := toRow(entry)
	if err != nil {
		return nil, fmt.Errorf("create audit log: %w", err)
	}

	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return nil, fmt.Errorf("create audit log: %w", err)
	}

	created, err := row.toDomain()
	if err != nil {
		return nil, fmt.Errorf("create audit log: %w", err)
	}

	return &created, nil
}

func (r auditLogRepositoryDB) ListByActor(ctx context.Context, params ListByActorParams) (ListByActorResult, error) {
	var total int64
	if err := r.db.WithContext(ctx).Model(&auditLogRow{}).
		Where("actor_id = ?", params.ActorID).
		Count(&total).Error; err != nil {
		return ListByActorResult{}, fmt.Errorf("count audit log by actor: %w", err)
	}

	offset := (params.Page - 1) * params.Limit
	var rows []auditLogRow
	if err := r.db.WithContext(ctx).
		Where("actor_id = ?", params.ActorID).
		Order("created_at DESC, id DESC").
		Offset(offset).Limit(params.Limit).
		Find(&rows).Error; err != nil {
		return ListByActorResult{}, fmt.Errorf("list audit log by actor: %w", err)
	}

	entries := make([]AuditLog, len(rows))
	for i, row := range rows {
		entry, err := row.toDomain()
		if err != nil {
			return ListByActorResult{}, fmt.Errorf("list audit log by actor: %w", err)
		}
		entries[i] = entry
	}

	return ListByActorResult{Items: entries, TotalItems: total}, nil
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

func toRow(entry AuditLog) (auditLogRow, error) {
	metadata, err := marshalMetadata(entry.Metadata)
	if err != nil {
		return auditLogRow{}, err
	}

	return auditLogRow{
		ID:         entry.ID,
		ActorID:    entry.ActorID,
		Action:     entry.Action,
		EntityType: entry.EntityType,
		EntityID:   entry.EntityID,
		Metadata:   metadata,
		CreatedAt:  entry.CreatedAt,
	}, nil
}

func (row auditLogRow) toDomain() (AuditLog, error) {
	entry := AuditLog{
		ID:         row.ID,
		ActorID:    row.ActorID,
		Action:     row.Action,
		EntityType: row.EntityType,
		EntityID:   row.EntityID,
		CreatedAt:  row.CreatedAt,
	}

	if len(row.Metadata) > 0 {
		if err := json.Unmarshal([]byte(row.Metadata), &entry.Metadata); err != nil {
			return AuditLog{}, fmt.Errorf("unmarshal audit log metadata: %w", err)
		}
	}

	return entry, nil
}
