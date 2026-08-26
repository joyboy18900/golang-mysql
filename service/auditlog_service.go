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
	if req.Page < 0 {
		return nil, errs.NewValidationError("page must be a positive integer")
	}

	page := req.Page
	if page == 0 {
		page = 1
	}
	limit := req.Limit
	if limit <= 0 {
		limit = defaultListLimit
	}

	result, err := s.repo.ListByActor(ctx, repository.ListByActorParams{ActorID: req.ActorID, Page: page, Limit: limit})
	if err != nil {
		logs.Error(err)
		return nil, errs.NewUnexpectedError()
	}

	pages := 0
	if result.TotalItems > 0 {
		pages = int((result.TotalItems + int64(limit) - 1) / int64(limit))
	}

	items := make([]AuditLogResponse, len(result.Items))
	for i, entry := range result.Items {
		items[i] = toAuditLogResponse(entry)
	}

	return &ListAuditLogResponse{
		Data:       items,
		Pagination: Pagination{Page: page, Limit: limit, TotalItems: result.TotalItems, TotalPages: pages},
	}, nil
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
