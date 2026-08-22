package testlab

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestReconcileFaceRefusesWithoutPersistence(t *testing.T) {
	svc := &Service{}
	if _, err := svc.ClaimedReconcileCandidates(t.Context(), 1); err == nil {
		t.Error("claimed candidates read succeeded without a pool")
	}
	if err := svc.MarkDatasetObjectLost(t.Context(), nil, pgtype.UUID{}); err == nil {
		t.Error("dataset mark succeeded without a transaction")
	}
}
