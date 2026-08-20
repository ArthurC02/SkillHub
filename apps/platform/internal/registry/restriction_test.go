package registry

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

// A nil transaction is deliberate: every case here must be rejected before the
// function reaches the database, and a nil pointer dereference is a louder
// failure than a passing assertion would be.
func TestSetAccessRestrictionRejectsABlankReason(t *testing.T) {
	for name, reason := range map[string]string{
		"empty":      "",
		"whitespace": "  ",
	} {
		t.Run(name, func(t *testing.T) {
			if err := SetAccessRestriction(context.Background(), nil, pgtype.UUID{}, &reason); !errors.Is(err, ErrEmptyRestriction) {
				t.Errorf("SetAccessRestriction err = %v", err)
			}
		})
	}
}
