package registry

import (
	"reflect"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

func assertSkillEvents(t *testing.T, s *SkillRoot, want ...Event) {
	t.Helper()
	if got := s.Events(); !reflect.DeepEqual(got, want) {
		t.Fatalf("events = %#v, want %#v", got, want)
	}
}

func skillMarked(redistribution Redistribution, newest newestVersion) *SkillRoot {
	return &SkillRoot{row: gen.Skill{Redistribution: string(redistribution)}, newest: newest}
}

func TestTakingDownASkillHidesItOnce(t *testing.T) {
	s := &SkillRoot{}

	s.TakeDown("the licence was withdrawn")

	if !s.TakenDown() {
		t.Fatal("the skill is still up after being taken down")
	}
	assertSkillEvents(t, s, SkillTakenDown{})

	again := &SkillRoot{row: gen.Skill{TakedownAt: pgtype.Timestamptz{Valid: true}}}
	again.TakeDown("a second report")

	if !again.TakenDown() {
		t.Fatal("a refused takedown brought the skill back")
	}
	assertSkillEvents(t, again, Refused{Reason: RefusedAlreadyTakenDown})
}

func TestAnAccessRestrictionNeedsAReasonAndLiftsWithNone(t *testing.T) {
	held, blank, spaces := "license-review", "", "  "
	cases := []struct {
		name      string
		reason    *string
		inEffect  bool
		wantEvent Event
	}{
		{"a reason code holds the skill", &held, true, AccessRestricted{Reason: held}},
		{"no reason lifts the hold", nil, false, AccessRestrictionLifted{}},
		{"an empty reason is refused", &blank, true, Refused{Reason: RefusedEmptyRestriction}},
		{"a whitespace reason is refused", &spaces, true, Refused{Reason: RefusedEmptyRestriction}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			previous := "takedown-review"
			s := &SkillRoot{row: gen.Skill{AccessRestriction: &previous}}

			s.Restrict(tc.reason)

			if s.Restriction().InEffect() != tc.inEffect {
				t.Fatalf("restriction in effect = %v, want %v", s.Restriction().InEffect(), tc.inEffect)
			}
			assertSkillEvents(t, s, tc.wantEvent)
		})
	}
}

func TestReleasingASkillRequiresTheLicenceItsNewestVersionRecords(t *testing.T) {
	mit := newestVersion{exists: true, license: LicenseClaim{Expression: "MIT", Source: "LICENSE"}}
	matching := LicenseClaim{Expression: " mit ", Source: "license"}
	cases := []struct {
		name   string
		skill  *SkillRoot
		to     Redistribution
		claim  LicenseClaim
		want   Event
		stored Redistribution
	}{
		{"self_supplied is provenance, not a verdict", skillMarked(RedistributionUnknown, mit),
			RedistributionSelfSupplied, matching, Refused{Reason: RefusedProvenanceNotAssertable}, RedistributionUnknown},
		{"generated is provenance, not a verdict", skillMarked(RedistributionUnknown, mit),
			RedistributionGenerated, matching, Refused{Reason: RefusedProvenanceNotAssertable}, RedistributionUnknown},
		{"a value outside the vocabulary", skillMarked(RedistributionUnknown, mit),
			Redistribution("shared"), matching, Refused{Reason: RefusedUnknownRedistribution}, RedistributionUnknown},
		{"releasing without a licence source", skillMarked(RedistributionUnknown, mit),
			RedistributionAllowed, LicenseClaim{Expression: "MIT"}, Refused{Reason: RefusedLicenseClaimMissing}, RedistributionUnknown},
		{"releasing with a whitespace expression", skillMarked(RedistributionUnknown, mit),
			RedistributionAllowed, LicenseClaim{Expression: " ", Source: "LICENSE"}, Refused{Reason: RefusedLicenseClaimMissing}, RedistributionUnknown},
		{"releasing a skill that has no version", skillMarked(RedistributionUnknown, newestVersion{}),
			RedistributionAllowed, matching, Refused{Reason: RefusedNoVersion}, RedistributionUnknown},
		{"releasing when the newest version records no licence",
			skillMarked(RedistributionUnknown, newestVersion{exists: true, license: LicenseClaim{Source: "LICENSE"}}),
			RedistributionAllowed, matching, Refused{Reason: RefusedNoLicenseRecorded}, RedistributionUnknown},
		{"releasing under a licence the version does not record", skillMarked(RedistributionUnknown, mit),
			RedistributionAllowed, LicenseClaim{Expression: "Apache-2.0", Source: "LICENSE"},
			Refused{Reason: RefusedLicenseMismatch, Recorded: mit.license}, RedistributionUnknown},
		{"releasing under the recorded licence, any case and padding", skillMarked(RedistributionUnknown, mit),
			RedistributionAllowed, matching, RedistributionSet{Before: RedistributionUnknown, After: RedistributionAllowed},
			RedistributionAllowed},
		{"blocking needs no licence evidence", skillMarked(RedistributionAllowed, newestVersion{}),
			RedistributionBlocked, LicenseClaim{}, RedistributionSet{Before: RedistributionAllowed, After: RedistributionBlocked},
			RedistributionBlocked},
		{"a generated skill cannot be blocked", skillMarked(RedistributionGenerated, mit),
			RedistributionBlocked, LicenseClaim{}, Refused{Reason: RefusedGeneratedIsPermanent}, RedistributionGenerated},
		{"a generated skill cannot be released even with evidence", skillMarked(RedistributionGenerated, mit),
			RedistributionAllowed, matching, Refused{Reason: RefusedGeneratedIsPermanent}, RedistributionGenerated},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.skill.SetRedistribution(tc.to, tc.claim)

			if tc.skill.Redistribution() != tc.stored {
				t.Fatalf("redistribution = %q, want %q", tc.skill.Redistribution(), tc.stored)
			}
			assertSkillEvents(t, tc.skill, tc.want)
		})
	}
}

func TestTheOwnerCategorizesOrClearsTheCategory(t *testing.T) {
	documents, owner := CategoryDocuments, CategorySourceOwner
	s := &SkillRoot{}

	s.Categorize(&documents)
	s.Categorize(nil)

	assertSkillEvents(t, s,
		SkillCategorized{Category: &documents, Source: &owner},
		SkillCategorized{})
}

func TestDeletingASkillMarksItDeleted(t *testing.T) {
	s := &SkillRoot{}

	s.Delete()

	if !s.Deleted() {
		t.Fatal("the skill is not deleted")
	}
	assertSkillEvents(t, s, SkillDeleted{})
}

func packageContent(t *testing.T, hash string, generated bool) VersionContent {
	t.Helper()
	content, err := ContentFromPackage(NewVersion{
		ContentHash: hash, Report: passingReport("a new summary"),
	}, generated)
	if err != nil {
		t.Fatal(err)
	}
	return content
}

func TestANewSkillStartsWithAKnownRedistribution(t *testing.T) {
	for _, tc := range []struct {
		name  string
		given Redistribution
		want  Redistribution
	}{
		{"an unstated verdict is unknown", "", RedistributionUnknown},
		{"a stated verdict is kept", RedistributionGenerated, RedistributionGenerated},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, err := SkillFromPackage(pgtype.UUID{}, passingReport("fresh"), tc.given)
			if err != nil {
				t.Fatal(err)
			}

			if s.Redistribution() != tc.want || s.Skill().Name != "fresh" {
				t.Fatalf("new skill = %q named %q, want %q named fresh", s.Redistribution(), s.Skill().Name, tc.want)
			}
			assertSkillEvents(t, s, SkillCreated{Redistribution: tc.want})
		})
	}
}

func TestAGeneratedSkillTakesOnlyGeneratedContent(t *testing.T) {
	for _, tc := range []struct {
		name      string
		skill     Redistribution
		generated bool
		want      Event
	}{
		{"uploaded content on an uploaded skill", RedistributionUnknown, false, SkillVersionAdded{ContentHash: "h"}},
		{"generated content on a generated skill", RedistributionGenerated, true, SkillVersionAdded{ContentHash: "h"}},
		{"generated content on a skill of unknown provenance", RedistributionUnknown, true, SkillVersionAdded{ContentHash: "h"}},
		{"uploaded content on a generated skill", RedistributionGenerated, false, Refused{Reason: RefusedGeneratedNameCollision}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := skillMarked(tc.skill, newestVersion{})

			s.AddVersion(packageContent(t, "h", tc.generated))

			assertSkillEvents(t, s, tc.want)
		})
	}
}

func TestAForkCarriesItsSourceAndItsFirstVersion(t *testing.T) {
	held, documents, curated := "license-review", string(CategoryDocuments), string(CategorySourceCurated)
	source := gen.Skill{
		ID: pgtype.UUID{Bytes: [16]byte{15: 1}, Valid: true}, Redistribution: string(RedistributionGenerated),
		AccessRestriction: &held, Category: &documents, CategorySource: &curated,
	}
	from := gen.SkillVersion{ID: pgtype.UUID{Bytes: [16]byte{15: 2}, Valid: true}, ContentHash: "source-bytes"}

	fork := forkOf(pgtype.UUID{}, "tidy-csv-fork", source, from)

	got := fork.Skill()
	if got.ForkedFromSkillID != source.ID || got.ForkedFromVersionID != from.ID ||
		!fork.Restriction().InEffect() || got.Category == nil || *got.Category != documents {
		t.Fatalf("fork = %+v, want the source's lineage, hold and category", got)
	}
	assertSkillEvents(t, fork,
		SkillCreated{Redistribution: RedistributionGenerated, ForkedFromSkillID: source.ID, ForkedFromVersionID: from.ID},
		SkillVersionAdded{ContentHash: "source-bytes"})
}

func TestASavedVersionLendsTheSkillItsSummary(t *testing.T) {
	s := skillMarked(RedistributionUnknown, newestVersion{})
	content := packageContent(t, "h", false)

	s.AddVersion(content)
	s.AdoptNewestSummary()

	if got := s.Skill().Summary; got == nil || *got != "fixture" {
		t.Fatalf("summary = %v, want the new version's", got)
	}
	assertSkillEvents(t, s, SkillVersionAdded{ContentHash: "h"}, SkillDescribed{})
}
