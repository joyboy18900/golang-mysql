package service

import (
	"testing"
	"time"

	"golang-mysql/errs"
	"golang-mysql/repository"
)

func TestEncodeDecodeCursorRoundTrip(t *testing.T) {
	tests := []struct {
		name   string
		cursor repository.AuditLogCursor
	}{
		{
			name: "typical value",
			cursor: repository.AuditLogCursor{
				CreatedAt: time.Date(2026, time.August, 26, 12, 0, 0, 123000, time.UTC),
				ID:        42,
			},
		},
		{
			name: "zero id",
			cursor: repository.AuditLogCursor{
				CreatedAt: time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC),
				ID:        0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded := encodeCursor(tt.cursor)

			decoded, err := decodeCursor(encoded)
			if err != nil {
				t.Fatalf("decodeCursor() error = %v", err)
			}

			if !decoded.CreatedAt.Equal(tt.cursor.CreatedAt) {
				t.Errorf("CreatedAt = %v, want %v", decoded.CreatedAt, tt.cursor.CreatedAt)
			}
			if decoded.ID != tt.cursor.ID {
				t.Errorf("ID = %d, want %d", decoded.ID, tt.cursor.ID)
			}
		})
	}
}

func TestDecodeCursorErrors(t *testing.T) {
	tests := []struct {
		name    string
		encoded string
	}{
		{name: "malformed base64", encoded: "not-valid-base64!!!"},
		{name: "valid base64 but no separator", encoded: "aGVsbG93b3JsZA"},
		{name: "valid base64 with non-numeric parts", encoded: "Zm9vOmJhcg"},
		{name: "empty string", encoded: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := decodeCursor(tt.encoded)
			if err == nil {
				t.Fatalf("decodeCursor(%q) error = nil, want error", tt.encoded)
			}

			appErr, ok := err.(errs.AppError)
			if !ok {
				t.Fatalf("decodeCursor(%q) error type = %T, want errs.AppError", tt.encoded, err)
			}
			if appErr.Message != "invalid cursor" {
				t.Errorf("message = %q, want %q", appErr.Message, "invalid cursor")
			}
		})
	}
}
