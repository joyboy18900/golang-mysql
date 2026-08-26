package handler

import (
	"errors"

	"golang-mysql/errs"

	"github.com/gofiber/fiber/v2"
)

type Envelope struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data"`
}

func sendSuccess(c *fiber.Ctx, status int, message string, data any) error {
	return c.Status(status).JSON(Envelope{Code: status, Message: message, Data: data})
}

func handleError(c *fiber.Ctx, err error) error {
	var appErr errs.AppError
	if errors.As(err, &appErr) {
		return c.Status(appErr.Code).JSON(Envelope{Code: appErr.Code, Message: appErr.Message, Data: nil})
	}
	return c.Status(fiber.StatusInternalServerError).
		JSON(Envelope{Code: fiber.StatusInternalServerError, Message: "unexpected error", Data: nil})
}
