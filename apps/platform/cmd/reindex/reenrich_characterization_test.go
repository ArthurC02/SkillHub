package main

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestTheEnrichmentBatchIsTwoHundredUnlessAPositiveCountIsGiven(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value string
		want  int32
	}{
		{"unset", "", 200},
		{"zero", "0", 200},
		{"negative", "-3", 200},
		{"not a number", "many", 200},
		{"one", "1", 1},
		{"explicit count", "7", 7},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("REINDEX_BATCH", tc.value)
			if got := batchSize(); got != tc.want {
				t.Errorf("REINDEX_BATCH=%q gives batch %d, want %d", tc.value, got, tc.want)
			}
		})
	}
}

func TestReEnrichmentReportsACatalogueItCouldNotRead(t *testing.T) {
	pool, err := pgxpool.New(context.Background(), "postgres://nobody@127.0.0.1:1/none?connect_timeout=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := requeueCatalogueForReenrichment(context.Background(), pool, "keep"); err == nil {
		t.Error("an unreadable catalogue was reported as queued for re-enrichment")
	}
}
