package packaging

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestReconcileFaceRefusesWithoutPersistence(t *testing.T) {
	svc := &Service{}
	if _, err := svc.ExpiredReconcileCandidates(t.Context(), 1); err == nil {
		t.Error("expired candidates read succeeded without a pool")
	}
	if _, err := svc.ClaimedReconcileCandidates(t.Context(), 1); err == nil {
		t.Error("claimed candidates read succeeded without a pool")
	}
	if err := svc.MarkArtifactPurged(t.Context(), nil, pgtype.UUID{}); err == nil {
		t.Error("artifact mark succeeded without a transaction")
	}
}
