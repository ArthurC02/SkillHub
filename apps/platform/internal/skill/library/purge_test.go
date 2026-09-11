package registry

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestPurgesRefuseWithoutTheReferenceReads(t *testing.T) {
	ctx := context.Background()
	if err := (&Service{}).PurgeWorkspace(ctx, nil, pgtype.UUID{}); err == nil {
		t.Error("PurgeWorkspace ran without knowing what still references a skill")
	}
	if _, err := (&Service{}).PurgeDeletedSkills(ctx, time.Hour, 10); err == nil {
		t.Error("PurgeDeletedSkills ran without knowing what still references a skill")
	}
}
