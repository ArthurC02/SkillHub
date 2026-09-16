package packaging

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

func TestAPackageIsReusedOnlyWhileItCanStillBeServed(t *testing.T) {
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	at := func(d time.Duration) pgtype.Timestamptz { return pgtype.Timestamptz{Time: now.Add(d), Valid: true} }
	live := gen.ListDownloadArtifactsWithIdentityRow{ScanStatus: string(ScanAvailable), ExpiresAt: at(time.Second)}
	for _, tc := range []struct {
		what string
		row  func(gen.ListDownloadArtifactsWithIdentityRow) gen.ListDownloadArtifactsWithIdentityRow
		want bool
	}{
		{"available and not yet expired", func(r gen.ListDownloadArtifactsWithIdentityRow) gen.ListDownloadArtifactsWithIdentityRow { return r }, true},
		{"expiring this very moment", func(r gen.ListDownloadArtifactsWithIdentityRow) gen.ListDownloadArtifactsWithIdentityRow {
			r.ExpiresAt = at(0)
			return r
		}, false},
		{"still being checked", func(r gen.ListDownloadArtifactsWithIdentityRow) gen.ListDownloadArtifactsWithIdentityRow {
			r.ScanStatus = string(ScanQuarantined)
			return r
		}, false},
		{"rejected by the check", func(r gen.ListDownloadArtifactsWithIdentityRow) gen.ListDownloadArtifactsWithIdentityRow {
			r.ScanStatus = string(ScanRejected)
			return r
		}, false},
		{"deleted by its owner", func(r gen.ListDownloadArtifactsWithIdentityRow) gen.ListDownloadArtifactsWithIdentityRow {
			r.DeletedAt = at(-time.Hour)
			return r
		}, false},
		{"bytes already purged", func(r gen.ListDownloadArtifactsWithIdentityRow) gen.ListDownloadArtifactsWithIdentityRow {
			r.PurgedAt = at(-time.Hour)
			return r
		}, false},
	} {
		_, got := reusableArtifact([]gen.ListDownloadArtifactsWithIdentityRow{tc.row(live)}, now)
		if got != tc.want {
			t.Errorf("%s: reusable = %v, want %v", tc.what, got, tc.want)
		}
	}
}

func TestTheNewestServablePackageIsReusedPastNewerOnesThatAreGone(t *testing.T) {
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	expired := gen.ListDownloadArtifactsWithIdentityRow{ArtifactID: pgtype.UUID{Bytes: [16]byte{1}, Valid: true},
		ScanStatus: string(ScanAvailable), ExpiresAt: pgtype.Timestamptz{Time: now.Add(-time.Minute), Valid: true}}
	older := gen.ListDownloadArtifactsWithIdentityRow{ArtifactID: pgtype.UUID{Bytes: [16]byte{2}, Valid: true},
		ScanStatus: string(ScanAvailable), ExpiresAt: pgtype.Timestamptz{Time: now.Add(time.Hour), Valid: true}}
	oldest := older
	oldest.ArtifactID = pgtype.UUID{Bytes: [16]byte{3}, Valid: true}
	got, ok := reusableArtifact([]gen.ListDownloadArtifactsWithIdentityRow{expired, older, oldest}, now)
	if !ok || got.ArtifactID != older.ArtifactID {
		t.Fatalf("reused %v (found=%v), want the newest servable package %v", got.ArtifactID, ok, older.ArtifactID)
	}
	if _, ok := reusableArtifact(nil, now); ok {
		t.Fatal("a package was reused when none had been built")
	}
}
