package handler

import (
	"strconv"

	"golang-mysql/errs"
	"golang-mysql/service"

	"github.com/gofiber/fiber/v2"
)

type auditLogHandler struct {
	auditLogSvc service.AuditLogService
}

func NewAuditLogHandler(auditLogSvc service.AuditLogService) auditLogHandler {
	return auditLogHandler{auditLogSvc: auditLogSvc}
}

func (h auditLogHandler) Create(c *fiber.Ctx) error {
	var req service.CreateAuditLogRequest
	if err := c.BodyParser(&req); err != nil {
		return handleError(c, errs.NewValidationError("invalid request body"))
	}

	resp, err := h.auditLogSvc.Create(c.Context(), req)
	if err != nil {
		return handleError(c, err)
	}

	return sendSuccess(c, fiber.StatusCreated, "audit log entry created", resp)
}

func (h auditLogHandler) ListByActor(c *fiber.Ctx) error {
	actorID, err := strconv.ParseInt(c.Query("actor_id"), 10, 64)
	if err != nil {
		return handleError(c, errs.NewValidationError("actor_id query parameter is required"))
	}

	page := 0
	if raw := c.Query("page"); raw != "" {
		page, err = strconv.Atoi(raw)
		if err != nil || page < 1 {
			return handleError(c, errs.NewValidationError("page must be a positive integer"))
		}
	}

	limit := 0
	if raw := c.Query("limit"); raw != "" {
		limit, err = strconv.Atoi(raw)
		if err != nil {
			return handleError(c, errs.NewValidationError("limit must be an integer"))
		}
	}

	resp, err := h.auditLogSvc.ListByActor(c.Context(), service.ListAuditLogRequest{
		ActorID: actorID,
		Page:    page,
		Limit:   limit,
	})
	if err != nil {
		return handleError(c, err)
	}

	return sendSuccess(c, fiber.StatusOK, "audit log entries retrieved", resp)
}
