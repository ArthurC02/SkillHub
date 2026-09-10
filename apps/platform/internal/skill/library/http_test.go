package registry

import (
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

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

	forked := verificationOf(gen.ListSkillsRow{VerifiedAt: at})
	if forked.Value != "not_measured" {
		t.Errorf("a fork was measured nowhere in this workspace, got %q", forked.Value)
	}
	if forked.ScannedAt != nil {
		t.Errorf("a state with no measurement must carry no timestamp: %q", *forked.ScannedAt)
	}

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

	for _, v := range []skillVerification{imported, forked, inh, empty} {
		if v.Label == "" || v.Note == "" {
			t.Errorf("state %q has no wording: %+v", v.Value, v)
		}
	}
}

func TestTheDeletionNoteDoesNotPromiseAPurgeNothingPerforms(t *testing.T) {
	for _, banned := range []string{"purge", "grace", "30-day", "30 day", "days", "天後", "寬限", "清除"} {
		if strings.Contains(strings.ToLower(deletionNote), banned) {
			t.Errorf("the note claims a deletion deadline (%q) and nothing in this repo enforces one: %q",
				banned, deletionNote)
		}
	}

	for _, required := range []string{"搜尋", "凍結", "Fork"} {
		if !strings.Contains(deletionNote, required) {
			t.Errorf("the note stopped saying what the deletion covers (%q missing): %q",
				required, deletionNote)
		}
	}
}
