package objreconcile

import (
	"context"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestTheSweepPurgesExpiredArtifactsAndDownloadIntentsEachWithItsOwnOwner(t *testing.T) {
	store := &purgeLedger{}
	recorder := func(label string) MarkFunc {
		return func(_ context.Context, _ pgx.Tx, id pgtype.UUID) error {
			store.steps = append(store.steps, label+" "+string('0'+id.Bytes[0]))
			return nil
		}
	}
	s := &Service{
		Pool:                       unreachablePool(t),
		Store:                      store,
		ListExpiredArtifacts:       listing(candidate(1, "artifact-bytes")),
		ListDownloadIntents:        listing(candidate(2, "intent-bytes")),
		RecordArtifactPurged:       recorder("artifact purged"),
		RecordDownloadIntentPurged: recorder("intent purged"),
		GuardArtifactRemoval:       store.guard,
	}

	if err := s.purgeExpired(context.Background()); err != nil {
		t.Fatalf("purgeExpired: %v", err)
	}
	want := []string{
		"guard artifact-bytes", "remove artifact-bytes", "artifact purged 1",
		"guard intent-bytes", "remove intent-bytes", "intent purged 2",
	}
	if !slices.Equal(store.steps, want) {
		t.Errorf("steps = %v, want %v", store.steps, want)
	}
}
