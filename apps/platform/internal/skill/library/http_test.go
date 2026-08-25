package registry

import (
	"strings"
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

	// ADR-042 決策 6. Same row as the fork above plus an ancestor the SQL was
	// willing to hand back, which it only does when the content hashes match now,
	// the ancestor is in the public catalogue, and it is neither deleted nor taken
	// down. It stays `scanned` because an inherited measurement is a value with an
	// attribution, not an absence — and the timestamp is the ancestor's, which is
	// older than the fork on purpose.
	older := pgtype.Timestamptz{Time: time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC), Valid: true}
	inh := verificationOf(gen.ListSkillsRow{
		VerifiedAt:           at,
		InheritedFromSkillID: pgtype.UUID{Valid: true},
		InheritedFromName:    "PDF Summariser",
		InheritedVerifiedAt:  older,
	})
	if inh.Value != "scanned" || inh.ScannedAt == nil {
		t.Fatalf("identical bytes carry the ancestor's scan: %+v", inh)
	}
	if *inh.ScannedAt != "2026-07-01T09:00:00Z" {
		t.Errorf("the inherited time is the ancestor's import, not the fork: %q", *inh.ScannedAt)
	}
	if inh.Label == imported.Label {
		t.Error("an inherited scan and a local one must not read as the same provenance")
	}
	if !strings.Contains(inh.Note, "PDF Summariser") {
		t.Errorf("ADR-042 forbids inheriting silently; the ancestor is unnamed: %q", inh.Note)
	}

	empty := verificationOf(gen.ListSkillsRow{})
	if empty.Value != "not_applicable" {
		t.Errorf("no version means nothing to scan, got %q", empty.Value)
	}

	// §2.9 again, from the other end: every state is worded, so none of them can
	// reach a screen as a blank or as an English enum value.
	for _, v := range []skillVerification{imported, forked, inh, empty} {
		if v.Label == "" || v.Note == "" {
			t.Errorf("state %q has no wording: %+v", v.Value, v)
		}
	}
}

// 02 §2.2: 不得顯示沒被強制的承諾. This one sentence is the whole user-facing
// statement of what a deletion did, and for its entire life it ended with a
// deadline -- "retained for the 30-day grace period, then purged" -- that no
// code in this repo has ever carried out.
//
// The assertion is deliberately about the absence, not the presence. A test that
// only checked the note mentions frozen snapshots would stay green the moment
// somebody appends the deadline back, and appending it back is exactly the edit
// that reintroduces the defect. So: if this note ever names a purge or a window
// again, the purge job has to exist first, and this test has to be changed by
// the person who wrote it -- which is the point at which they will read why.
func TestTheDeletionNoteDoesNotPromiseAPurgeNothingPerforms(t *testing.T) {
	for _, banned := range []string{"purge", "grace", "30-day", "30 day", "days"} {
		if strings.Contains(strings.ToLower(deletionNote), banned) {
			t.Errorf("the note claims a deletion deadline (%q) and nothing in this repo enforces one: %q",
				banned, deletionNote)
		}
	}
	// The other half: stripping the false promise must not leave the user with
	// less than WS-005 requires, which is the scope of what just happened.
	for _, required := range []string{"search", "frozen", "forks"} {
		if !strings.Contains(deletionNote, required) {
			t.Errorf("the note stopped saying what the deletion covers (%q missing): %q",
				required, deletionNote)
		}
	}
}
