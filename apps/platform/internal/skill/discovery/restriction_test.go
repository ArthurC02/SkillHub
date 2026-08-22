package catalog

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestRestrictionServiceRejectsInvalidInputBeforeDatabase(t *testing.T) {
	svc := &Service{} // nil Pool proves validation happens before database access.
	zeroUUID := pgtype.UUID{}
	tests := []struct {
		name string
		call func() error
		want string
	}{
		{"blank reason", func() error {
			_, err := svc.SetRestriction(context.Background(), zeroUUID, zeroUUID, "  ", "reviewed")
			return err
		}, "reason is required"},
		{"unknown reason", func() error {
			_, err := svc.SetRestriction(context.Background(), zeroUUID, zeroUUID, "invented", "reviewed")
			return err
		}, "unknown reason code"},
		{"blank set note", func() error {
			_, err := svc.SetRestriction(context.Background(), zeroUUID, zeroUUID, "license-review", "  ")
			return err
		}, "note is required"},
		{"long set note", func() error {
			_, err := svc.SetRestriction(context.Background(), zeroUUID, zeroUUID, "license-review", strings.Repeat("x", maxOperatorNoteBytes+1))
			return err
		}, "note is too long"},
		{"blank clear note", func() error {
			_, err := svc.ClearRestriction(context.Background(), zeroUUID, zeroUUID, "  ")
			return err
		}, "note is required"},
		{"long clear note", func() error {
			_, err := svc.ClearRestriction(context.Background(), zeroUUID, zeroUUID, strings.Repeat("x", maxOperatorNoteBytes+1))
			return err
		}, "note is too long"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.call()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("got %v, want error containing %q", err, tt.want)
			}
		})
	}
}
