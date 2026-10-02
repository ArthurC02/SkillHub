package publishing

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestANonListableSnapshotIsNotExposed(t *testing.T) {
	versionID := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	workspaceID := pgtype.UUID{Bytes: [16]byte{2}, Valid: true}
	skillID := pgtype.UUID{Bytes: [16]byte{3}, Valid: true}
	state := ExposureState{
		Status: StatusPublished, SkillID: skillID, OwnerWorkspaceID: workspaceID,
		VersionID: versionID, Approved: true, Concluded: true, ReviewedDigest: "reviewed",
	}
	svc := Service{
		ReadSkill: func(context.Context, pgtype.UUID, pgtype.UUID) (SkillFacts, bool, error) {
			return SkillFacts{ID: skillID, Redistribution: redistributionAllowed}, true, nil
		},
		ReadSearchSnapshot: func(context.Context, pgtype.UUID) (SearchSnapshot, bool, error) {
			return SearchSnapshot{VersionID: versionID, Digest: "reviewed", Listable: false}, true, nil
		},
	}

	exposed, _, err := svc.exposedNow(context.Background(), state)
	if err != nil {
		t.Fatal(err)
	}
	if exposed {
		t.Fatal("non-listable search text is exposed")
	}
	projection, err := svc.catalogExposure(context.Background(), state)
	if err != nil {
		t.Fatal(err)
	}
	if projection.State != CatalogSearchNotReady {
		t.Fatalf("owner state = %q, want %q", projection.State, CatalogSearchNotReady)
	}
}

func TestTheCatalogueScopeReadsEveryCandidateSkillInOneCall(t *testing.T) {
	ws := pgtype.UUID{Bytes: [16]byte{9}, Valid: true}
	skill := func(n byte) pgtype.UUID { return pgtype.UUID{Bytes: [16]byte{n}, Valid: true} }
	state := func(n byte, status Status, approved bool) ExposureState {
		return ExposureState{Status: status, SkillID: skill(n), OwnerWorkspaceID: ws, VersionID: skill(n + 100), Approved: approved}
	}
	states := []ExposureState{
		state(1, StatusPublished, true),
		state(2, StatusPublished, false),
		state(3, StatusDelisted, true),
		state(4, StatusPublished, true),
		state(5, StatusPublished, true),
		state(6, StatusPublished, true),
	}
	var calls int
	var asked []SkillRef
	svc := Service{ReadSkills: func(_ context.Context, refs []SkillRef) (map[SkillRef]SkillFacts, error) {
		calls++
		asked = refs
		return map[SkillRef]SkillFacts{
			{WorkspaceID: ws, SkillID: skill(1)}: {ID: skill(1), Redistribution: redistributionAllowed},
			{WorkspaceID: ws, SkillID: skill(5)}: {ID: skill(5), Redistribution: redistributionAllowed, TakenDown: true},
			{WorkspaceID: ws, SkillID: skill(6)}: {ID: skill(6), Redistribution: redistributionSelfSupplied},
		}, nil
	}}

	exposed, err := svc.exposedAmong(context.Background(), states)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("the skills were read in %d calls, want one batch", calls)
	}
	if len(asked) != 4 {
		t.Fatalf("asked about %d skills, want only the 4 approved published ones", len(asked))
	}
	if len(exposed) != 1 || exposed[0].SkillID != skill(1) {
		t.Fatalf("exposed %+v, want only the available redistributable skill 1 (2 unapproved, 3 delisted, 4 gone, 5 taken down, 6 self-supplied, not cleared for the catalogue)", exposed)
	}
}

func TestNoCandidateMeansNoSkillRead(t *testing.T) {
	svc := Service{ReadSkills: func(context.Context, []SkillRef) (map[SkillRef]SkillFacts, error) {
		t.Fatal("read skills with no approved published publication")
		return nil, nil
	}}
	if exposed, err := svc.exposedAmong(context.Background(), []ExposureState{{Status: StatusPublished}}); err != nil || exposed != nil {
		t.Fatalf("got %v, %v", exposed, err)
	}
}

func TestACurrentListableSnapshotIsExposedOnlyForAnApprovedRedistributableSkill(t *testing.T) {
	versionID := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	svc := Service{
		ReadSearchSnapshot: func(context.Context, pgtype.UUID) (SearchSnapshot, bool, error) {
			return SearchSnapshot{VersionID: versionID, Digest: "reviewed", Listable: true}, true, nil
		},
	}
	for name, c := range map[string]struct {
		approved bool
		skill    SkillFacts
		exposed  bool
	}{
		"approved and redistributable":         {true, SkillFacts{Redistribution: redistributionAllowed}, true},
		"not approved":                         {false, SkillFacts{Redistribution: redistributionAllowed}, false},
		"approved but supplied by its creator": {true, SkillFacts{Redistribution: redistributionSelfSupplied}, false},
		"approved but generated":               {true, SkillFacts{Redistribution: redistributionGenerated}, false},
		"approved but taken down":              {true, SkillFacts{Redistribution: redistributionAllowed, TakenDown: true}, false},
	} {
		state := ExposureState{Status: StatusPublished, VersionID: versionID, Approved: c.approved, Concluded: true, ReviewedDigest: "reviewed"}
		exposed, _, err := svc.exposedFor(context.Background(), state, c.skill, true)
		if err != nil {
			t.Fatal(err)
		}
		if exposed != c.exposed {
			t.Errorf("%s: exposed = %v, want %v", name, exposed, c.exposed)
		}
	}
}

func TestAReviewIsStaleOnceAnythingItJudgedHasMoved(t *testing.T) {
	seen := ExposureState{
		ReleaseID: pgtype.UUID{Bytes: [16]byte{1}, Valid: true}, Sequence: 2, Status: StatusPublished,
		SkillID: pgtype.UUID{Bytes: [16]byte{2}, Valid: true}, OwnerWorkspaceID: pgtype.UUID{Bytes: [16]byte{3}, Valid: true},
		VersionID: pgtype.UUID{Bytes: [16]byte{4}, Valid: true}, ReviewedDigest: "old",
	}
	other := pgtype.UUID{Bytes: [16]byte{9}, Valid: true}
	for _, c := range []struct {
		name  string
		move  func(*ExposureState)
		stale bool
	}{
		{"nothing moved", func(*ExposureState) {}, false},
		{"only its last verdict's digest", func(s *ExposureState) { s.ReviewedDigest = "new" }, false},
		{"a newer release", func(s *ExposureState) { s.ReleaseID = other }, true},
		{"another review landed", func(s *ExposureState) { s.Sequence++ }, true},
		{"it was withdrawn", func(s *ExposureState) { s.Status = "withdrawn" }, true},
		{"it points at another skill", func(s *ExposureState) { s.SkillID = other }, true},
		{"its owner changed", func(s *ExposureState) { s.OwnerWorkspaceID = other }, true},
		{"its version changed", func(s *ExposureState) { s.VersionID = other }, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			now := seen
			c.move(&now)
			if got := !now.sameReleaseAs(seen); got != c.stale {
				t.Errorf("stale = %v, want %v", got, c.stale)
			}
		})
	}
}
