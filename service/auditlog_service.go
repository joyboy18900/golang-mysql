package service

import (
	"context"

	"golang-mysql/errs"
	"golang-mysql/logs"
	"golang-mysql/repository"
)

const defaultListLimit = 50

type auditLogService struct {
	repo repository.AuditLogRepository
}

func NewAuditLogService(repo repository.AuditLogRepository) AuditLogService {
	return auditLogService{repo: repo}
}

func (s auditLogService) Create(ctx context.Context, req CreateAuditLogRequest) (*AuditLogResponse, error) {
	if req.ActorID <= 0 || req.Action == "" || req.EntityType == "" {
		return nil, errs.NewValidationError("actor_id, action and entity_type are required")
	}

	metadata := req.Metadata
	if metadata == nil {
		metadata = map[string]any{}
	}

	created, err := s.repo.Create(ctx, repository.AuditLog{
		ActorID:    req.ActorID,
		Action:     req.Action,
		EntityType: req.EntityType,
		EntityID:   req.EntityID,
		Metadata:   metadata,
	})
	if err != nil {
		logs.Error(err)
		return nil, errs.NewUnexpectedError()
	}

	resp := toAuditLogResponse(*created)
	return &resp, nil
}

func (s auditLogService) ListByActor(ctx context.Context, req ListAuditLogRequest) (*ListAuditLogResponse, error) {
	if req.ActorID <= 0 {
		return nil, errs.NewValidationError("actor_id is required")
	}

	limit := req.Limit
	if limit <= 0 {
		limit = defaultListLimit
	}

	var after *repository.AuditLogCursor
	if req.Cursor != "" {
		decoded, err := decodeCursor(req.Cursor)
		if err != nil {
			return nil, err
		}
		after = decoded
	}

	entries, err := s.repo.ListByActor(ctx, repository.ListByActorParams{
		ActorID: req.ActorID,
		Limit:   limit + 1,
		After:   after,
	})
	if err != nil {
		logs.Error(err)
		return nil, errs.NewUnexpectedError()
	}

	var nextCursor *string
	if len(entries) > limit {
		entries = entries[:limit]
		last := entries[len(entries)-1]
		encoded := encodeCursor(repository.AuditLogCursor{CreatedAt: last.CreatedAt, ID: last.ID})
		nextCursor = &encoded
	}

	items := make([]AuditLogResponse, len(entries))
	for i, entry := range entries {
		items[i] = toAuditLogResponse(entry)
	}

	return &ListAuditLogResponse{Items: items, NextCursor: nextCursor}, nil
}

func toAuditLogResponse(entry repository.AuditLog) AuditLogResponse {
	return AuditLogResponse{
		ID:         entry.ID,
		ActorID:    entry.ActorID,
		Action:     entry.Action,
		EntityType: entry.EntityType,
		EntityID:   entry.EntityID,
		Metadata:   entry.Metadata,
		CreatedAt:  entry.CreatedAt,
	}
}
