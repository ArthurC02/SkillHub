package catalog

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/pgvector/pgvector-go"
)

func TestOnlyAPendingEnrichmentStartsItsAttemptsOver(t *testing.T) {
	for status, restarts := range map[EnrichmentStatus]bool{EnrichmentPending: true, EnrichmentEnriched: false} {
		got := enrichedDocumentOf(EnrichedSkillProjection{EnrichmentStatus: string(status)})
		if got.RestartEnrichmentAttempts != restarts {
			t.Errorf("a %s projection restarts attempts = %v, want %v", status, got.RestartEnrichmentAttempts, restarts)
		}
	}
	if len(AllEnrichmentStatuses()) != 2 {
		t.Fatalf("enrichment statuses = %v; decide whether each new one starts its attempts over", AllEnrichmentStatuses())
	}
}

func TestADocumentIsListedOnceEnrichedOrOnceAVectorCanFindIt(t *testing.T) {
	emb := pgvector.NewVector([]float32{1})
	for _, tc := range []struct {
		name      string
		status    EnrichmentStatus
		embedding *pgvector.Vector
		listable  bool
	}{
		{"pending without a vector", EnrichmentPending, nil, false},
		{"pending with a vector", EnrichmentPending, &emb, true},
		{"enriched without a vector", EnrichmentEnriched, nil, true},
		{"enriched with a vector", EnrichmentEnriched, &emb, true},
	} {
		got := enrichedDocumentOf(EnrichedSkillProjection{EnrichmentStatus: string(tc.status), Embedding: tc.embedding})
		if got.Listable != tc.listable {
			t.Errorf("%s: listable = %v, want %v", tc.name, got.Listable, tc.listable)
		}
	}
}

func TestAnUnmeasuredListingIsWrittenUnverifiedAndOnlyTheReviewedNewestVersionIsCurated(t *testing.T) {
	reviewed := pgtype.UUID{Bytes: [16]byte{7}, Valid: true}
	newer := pgtype.UUID{Bytes: [16]byte{8}, Valid: true}

	current := listingOf(pgtype.UUID{}, ListingFacts{CurationTier: string(TierCurated), CuratedVersionID: reviewed, LatestVersionID: reviewed})
	movedOn := listingOf(pgtype.UUID{}, ListingFacts{CurationTier: string(TierCurated), CuratedVersionID: reviewed, LatestVersionID: newer})

	if !current.Curated || movedOn.Curated {
		t.Fatalf("curated: reviewed newest = %v, newer version = %v; want true then false", current.Curated, movedOn.Curated)
	}
	if *current.AgentCapability != "unverified" || *current.AgentRuntime != "unverified" || *current.AgentRuntimeImage != "" {
		t.Fatalf("unmeasured compatibility = %q/%q/%q, want unverified/unverified and no image",
			*current.AgentCapability, *current.AgentRuntime, *current.AgentRuntimeImage)
	}
}

func TestListingOfCarriesLiveListingFacts(t *testing.T) {
	skillID := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	latestVersionID := pgtype.UUID{Bytes: [16]byte{2}, Valid: true}
	curatedVersionID := pgtype.UUID{Bytes: [16]byte{3}, Valid: true}
	category, source := "automation", "curated"
	verifiedAt := pgtype.Timestamptz{Time: time.Unix(123, 0), Valid: true}
	measuredAt := pgtype.Timestamptz{Time: time.Unix(456, 0), Valid: true}

	got := listingOf(skillID, ListingFacts{
		Redistribution:         string(RedistributionGenerated),
		Category:               &category,
		CategorySource:         &source,
		CurationTier:           string(TierCurated),
		CuratedVersionID:       curatedVersionID,
		LatestVersionID:        latestVersionID,
		VerifiedAt:             verifiedAt,
		LatestPackageObjectKey: "packages/latest.tar",
		AgentCapability:        "supported",
		AgentRuntime:           "python",
		AgentRuntimeImage:      "runtime@sha256:test",
		AgentMeasuredAt:        measuredAt,
	})

	if got.SkillID != skillID || !got.Generated || got.Category != &category || got.CategorySource != &source || got.LatestVersionID != latestVersionID || got.VerifiedAt != verifiedAt {
		t.Fatalf("listing omitted base facts: %+v", got)
	}
	if got.LatestPackageObjectKey == nil || *got.LatestPackageObjectKey != "packages/latest.tar" || got.CuratedVersionID != curatedVersionID {
		t.Fatalf("listing omitted version facts: %+v", got)
	}
	if got.AgentMeasuredAt != measuredAt || got.AgentCapability == nil || *got.AgentCapability != "supported" || got.AgentRuntime == nil || *got.AgentRuntime != "python" || got.AgentRuntimeImage == nil || *got.AgentRuntimeImage != "runtime@sha256:test" {
		t.Fatalf("listing omitted compatibility facts: %+v", got)
	}
}

func TestIndexSkillRejectsMissingListingReaderBeforeWrite(t *testing.T) {
	err := (&Service{}).IndexSkill(context.Background(), nil, SkillProjection{})
	if err == nil || !strings.Contains(err.Error(), "live listing facts reader") {
		t.Fatalf("missing listing reader error = %v", err)
	}
}

func TestRebuildIndexRejectsMissingReadersBeforeWrite(t *testing.T) {
	listingReader := func(context.Context, gen.DBTX, pgtype.UUID) (ListingFacts, bool, error) {
		return ListingFacts{}, false, nil
	}
	liveSkillsReader := func(context.Context, gen.DBTX) ([]IndexSkillFacts, error) {
		return nil, nil
	}

	tests := []struct {
		name string
		svc  *Service
		want string
	}{
		{name: "listing facts", svc: &Service{}, want: "live listing facts reader"},
		{name: "live skills", svc: &Service{ReadLiveListingFacts: listingReader}, want: "live skills reader"},
		{name: "live IDs", svc: &Service{ReadLiveListingFacts: listingReader, ReadLiveSkills: liveSkillsReader}, want: "live skill IDs reader"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, _, err := test.svc.RebuildIndex(context.Background())
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("RebuildIndex() error = %v, want %q", err, test.want)
			}
		})
	}
}
