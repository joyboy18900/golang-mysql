package service

import (
	"context"

	"golang-mysql/errs"
	"golang-mysql/logs"
	"golang-mysql/repository"
)

const DefaultListLimit = 50

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

func (s auditLogService) ListByActor(ctx context.Context, actorID int64, limit int) ([]AuditLogResponse, error) {
	if actorID <= 0 {
		return nil, errs.NewValidationError("actor_id is required")
	}
	if limit <= 0 {
		limit = DefaultListLimit
	}

	entries, err := s.repo.ListByActor(ctx, actorID, limit)
	if err != nil {
		logs.Error(err)
		return nil, errs.NewUnexpectedError()
	}

	responses := make([]AuditLogResponse, len(entries))
	for i, entry := range entries {
		responses[i] = toAuditLogResponse(entry)
	}

	return responses, nil
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
