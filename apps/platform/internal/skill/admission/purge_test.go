package ingest

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestPurgeWorkspaceRefusesWithoutTheVersionSourceRead(t *testing.T) {
	if err := (&Service{}).PurgeWorkspace(context.Background(), nil, pgtype.UUID{}); err == nil {
		t.Error("PurgeWorkspace ran without knowing which import sources versions still use")
	}
}
