package packaging

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
)

func TestCreateFailsClosedWithoutRetention(t *testing.T) {
	service := &Service{}
	_, err := service.Create(context.Background(), identity.Workspace{}, pgtype.UUID{}, pgtype.UUID{}, "standard", false)
	if !errors.Is(err, ErrRetentionNotConfigured) {
		t.Fatalf("Create() error = %v, want ErrRetentionNotConfigured", err)
	}
}
