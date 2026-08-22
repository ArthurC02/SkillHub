package registry

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

// The trap this guards: `verified_at` means "the newest version's creation time"
// everywhere else, and for a fork that is the moment somebody pressed Fork with
// nothing scanned. Serving it would print a fresh-looking timestamp on the one
// case where nothing was measured — the one rendering 設計系統 §2.9 rates worse
// than a blank, and a blank is already forbidden.
func TestVerificationDistinguishesForkFromImport(t *testing.T) {
	at := pgtype.Timestamptz{Time: time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC), Valid: true}
	src := pgtype.UUID{Valid: true}

	imported := verificationOf(gen.ListSkillsRow{VerifiedAt: at, VerifiedSourceID: src})
	if imported.Value != "scanned" || imported.ScannedAt == nil {
		t.Fatalf("an imported version is the one case with a real scan time: %+v", imported)
	}
	if *imported.ScannedAt != "2026-08-01T10:00:00Z" {
		t.Errorf("scanned_at = %q", *imported.ScannedAt)
	}

	// Fork leaves source_id NULL because skill_sources rows belong to the origin
	// workspace (see Fork). Same created_at shape as above, so a version-time-only
	// implementation cannot tell these two apart — which is the whole point.
	forked := verificationOf(gen.ListSkillsRow{VerifiedAt: at})
	if forked.Value != "not_measured" {
		t.Errorf("a fork was measured nowhere in this workspace, got %q", forked.Value)
	}
	if forked.ScannedAt != nil {
		t.Errorf("a state with no measurement must carry no timestamp: %q", *forked.ScannedAt)
	}

	empty := verificationOf(gen.ListSkillsRow{})
	if empty.Value != "not_applicable" {
		t.Errorf("no version means nothing to scan, got %q", empty.Value)
	}

	// §2.9 again, from the other end: every state is worded, so none of them can
	// reach a screen as a blank or as an English enum value.
	for _, v := range []skillVerification{imported, forked, empty} {
		if v.Label == "" || v.Note == "" {
			t.Errorf("state %q has no wording: %+v", v.Value, v)
		}
	}
}
