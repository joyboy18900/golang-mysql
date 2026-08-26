package service

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"

	"golang-mysql/errs"
	"golang-mysql/repository"
)

func encodeCursor(cursor repository.AuditLogCursor) string {
	raw := fmt.Sprintf("%d:%d", cursor.CreatedAt.UnixMicro(), cursor.ID)
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func decodeCursor(encoded string) (*repository.AuditLogCursor, error) {
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, errs.NewValidationError("invalid cursor")
	}

	parts := strings.SplitN(string(raw), ":", 2)
	if len(parts) != 2 {
		return nil, errs.NewValidationError("invalid cursor")
	}

	micros, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return nil, errs.NewValidationError("invalid cursor")
	}

	id, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return nil, errs.NewValidationError("invalid cursor")
	}

	return &repository.AuditLogCursor{
		CreatedAt: time.UnixMicro(micros).UTC(),
		ID:        id,
	}, nil
}
