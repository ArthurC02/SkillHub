package registry

import (
	"errors"
	"reflect"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/shared/skillpkg"
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

func TestATakedownWithoutAReasonIsRefusedAndLeavesTheSkillUp(t *testing.T) {
	for name, reason := range map[string]string{"empty": "", "whitespace": " \t\n"} {
		t.Run(name, func(t *testing.T) {
			s := &SkillRoot{}

			s.TakeDown(reason)

			if s.TakenDown() {
				t.Fatal("a takedown without a reason took the skill down")
			}
			assertSkillEvents(t, s, Refused{Reason: RefusedTakedownReasonMissing})
			if refused, _ := s.Refusal(); !errors.Is(refused.err(), ErrTakedownReasonRequired) {
				t.Fatalf("refusal maps to %v, want ErrTakedownReasonRequired", refused.err())
			}
		})
	}
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

func TestCurationIsRecordedOnTheNewestVersionOfACatalogueSkillAndWithdrawnFromAnySkill(t *testing.T) {
	newest := pgtype.UUID{Bytes: [16]byte{15: 7}, Valid: true}
	versioned := newestVersion{id: newest, exists: true, number: 3}
	for _, tc := range []struct {
		name        string
		from        gen.Skill
		newest      newestVersion
		to          CurationTier
		inCatalogue bool
		want        Event
		wantRow     gen.Skill
	}{
		{"curating a catalogue skill binds its newest version",
			gen.Skill{CurationTier: "indexed"}, versioned, CurationCurated, true,
			CurationSet{Before: CurationIndexed, After: CurationCurated, VersionID: newest},
			gen.Skill{CurationTier: "curated", CuratedVersionID: newest}},
		{"a skill outside the catalogue cannot be curated",
			gen.Skill{CurationTier: "indexed"}, versioned, CurationCurated, false,
			Refused{Reason: RefusedCurationOutsideCatalog},
			gen.Skill{CurationTier: "indexed"}},
		{"a catalogue skill without a version has nothing to review",
			gen.Skill{CurationTier: "indexed"}, newestVersion{}, CurationCurated, true,
			Refused{Reason: RefusedNoVersion},
			gen.Skill{CurationTier: "indexed"}},
		{"withdrawing clears the reviewed version even outside the catalogue",
			gen.Skill{CurationTier: "curated", CuratedVersionID: newest}, versioned, CurationIndexed, false,
			CurationSet{Before: CurationCurated, After: CurationIndexed},
			gen.Skill{CurationTier: "indexed"}},
		{"a tier the column does not hold is refused",
			gen.Skill{CurationTier: "indexed"}, versioned, CurationTier("external"), true,
			Refused{Reason: RefusedUnknownCurationTier},
			gen.Skill{CurationTier: "indexed"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &SkillRoot{row: tc.from, newest: tc.newest}

			s.SetCuration(tc.to, tc.inCatalogue)

			assertSkillEvents(t, s, tc.want)
			if s.row.CurationTier != tc.wantRow.CurationTier || s.row.CuratedVersionID != tc.wantRow.CuratedVersionID {
				t.Fatalf("row = %q/%v, want %q/%v", s.row.CurationTier, s.row.CuratedVersionID,
					tc.wantRow.CurationTier, tc.wantRow.CuratedVersionID)
			}
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

func TestSkillSnapshotsDoNotAliasInputsOrOutputs(t *testing.T) {
	t.Run("skill rows and DTOs keep independent strings", func(t *testing.T) {
		summary, restriction, category, source := "summary", "hold", "documents", "owner"
		s := startSkill(gen.Skill{
			Summary: &summary, AccessRestriction: &restriction, Category: &category, CategorySource: &source,
		}, RedistributionUnknown)

		summary, restriction, category, source = "changed", "changed", "changed", "changed"
		first := s.Skill()
		if *first.Summary != "summary" || *first.AccessRestriction != "hold" || *first.Category != "documents" || *first.CategorySource != "owner" {
			t.Fatalf("skill after input mutation = %+v, want original strings", first)
		}

		*first.Summary, *first.AccessRestriction, *first.Category, *first.CategorySource = "output", "output", "output", "output"
		second := s.Skill()
		if *second.Summary != "summary" || *second.AccessRestriction != "hold" || *second.Category != "documents" || *second.CategorySource != "owner" {
			t.Fatalf("skill after output mutation = %+v, want original strings", second)
		}
	})

	t.Run("restriction and category events keep independent pointers", func(t *testing.T) {
		reason := "hold"
		category := CategoryDocuments
		s := &SkillRoot{}
		s.Restrict(&reason)
		s.Categorize(&category)

		reason, category = "changed", CategoryWriting
		if got := s.Skill().AccessRestriction; got == nil || *got != "hold" {
			t.Fatalf("restriction after input mutation = %v, want hold", got)
		}
		first := s.Events()[1].(SkillCategorized)
		if first.Category == nil || *first.Category != CategoryDocuments || first.Source == nil || *first.Source != CategorySourceOwner {
			t.Fatalf("category event after input mutation = %+v, want owner documents", first)
		}

		*first.Category, *first.Source = CategoryWriting, CategorySourceCurated
		second := s.Events()[1].(SkillCategorized)
		if second.Category == nil || *second.Category != CategoryDocuments || second.Source == nil || *second.Source != CategorySourceOwner {
			t.Fatalf("category event after output mutation = %+v, want owner documents", second)
		}
	})

	t.Run("improvements and version DTOs keep independent pointers and IDs", func(t *testing.T) {
		firstID := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
		secondID := pgtype.UUID{Bytes: [16]byte{2}, Valid: true}
		improvement := Improvement{SuggestionIDs: []pgtype.UUID{firstID}}
		content := packageContent(t, "hash", false).ImprovedBy(improvement)
		content.manifest = []byte("manifest")
		license, licenseSource := "MIT", "LICENSE"
		content.license, content.licenseSource = &license, &licenseSource
		improvement.SuggestionIDs[0] = secondID
		s := &SkillRoot{}
		s.AddVersion(content)
		content.manifest[0] = 'x'
		*content.license, *content.licenseSource = "changed", "changed"
		content.improvedBy.SuggestionIDs[0] = secondID
		if string(s.pending.manifest) != "manifest" {
			t.Fatalf("pending manifest after input mutation = %q, want manifest", s.pending.manifest)
		}
		if s.pending.license == nil || *s.pending.license != "MIT" {
			t.Fatalf("pending license after input mutation = %v, want MIT", s.pending.license)
		}
		if s.pending.licenseSource == nil || *s.pending.licenseSource != "LICENSE" {
			t.Fatalf("pending license source after input mutation = %v, want LICENSE", s.pending.licenseSource)
		}
		first := s.Events()[0].(SkillVersionAdded)
		if first.ImprovedBy == nil || len(first.ImprovedBy.SuggestionIDs) != 1 || first.ImprovedBy.SuggestionIDs[0] != firstID {
			t.Fatalf("version event after input mutation = %+v, want first suggestion", first)
		}

		first.ImprovedBy.SuggestionIDs[0] = secondID
		second := s.Events()[0].(SkillVersionAdded)
		if second.ImprovedBy == nil || len(second.ImprovedBy.SuggestionIDs) != 1 || second.ImprovedBy.SuggestionIDs[0] != firstID {
			t.Fatalf("version event after output mutation = %+v, want first suggestion", second)
		}

		license, licenseSource = "MIT", "manifest"
		s.added = gen.SkillVersion{LicenseExpression: &license, LicenseSource: &licenseSource}
		version := s.AddedVersion()
		*version.LicenseExpression, *version.LicenseSource = "output", "output"
		again := s.AddedVersion()
		if *again.LicenseExpression != "MIT" || *again.LicenseSource != "manifest" {
			t.Fatalf("version after output mutation = %+v, want original licenses", again)
		}
	})

	t.Run("package manifests do not retain the report pointer", func(t *testing.T) {
		report := skillpkg.Report{Manifest: &skillpkg.Manifest{Name: "fresh", Description: "summary"}}
		s, err := SkillFromPackage(pgtype.UUID{}, report, RedistributionUnknown)
		if err != nil {
			t.Fatal(err)
		}
		report.Manifest.Description = "changed"
		if got := s.Skill().Summary; got == nil || *got != "summary" {
			t.Fatalf("package summary after report mutation = %v, want summary", got)
		}
	})
}

func TestAGeneratedSkillTakesOnlyGeneratedContent(t *testing.T) {
	for _, tc := range []struct {
		name      string
		skill     Redistribution
		generated bool
		want      Event
	}{
		{"uploaded content on an uploaded skill", RedistributionUnknown, false, SkillVersionAdded{VersionNumber: 1, ContentHash: "h"}},
		{"generated content on a generated skill", RedistributionGenerated, true, SkillVersionAdded{VersionNumber: 1, ContentHash: "h"}},
		{"generated content on a skill of unknown provenance", RedistributionUnknown, true, SkillVersionAdded{VersionNumber: 1, ContentHash: "h"}},
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
		SkillVersionAdded{VersionNumber: 1, ContentHash: "source-bytes"})
}

func TestASavedVersionLendsTheSkillItsSummary(t *testing.T) {
	s := skillMarked(RedistributionUnknown, newestVersion{})
	content := packageContent(t, "h", false)

	s.AddVersion(content)
	s.AdoptNewestSummary()

	if got := s.Skill().Summary; got == nil || *got != "fixture" {
		t.Fatalf("summary = %v, want the new version's", got)
	}
	assertSkillEvents(t, s, SkillVersionAdded{VersionNumber: 1, ContentHash: "h"}, SkillDescribed{})
}

func TestANewVersionTakesTheNumberAfterTheNewestAndBecomesTheNewest(t *testing.T) {
	s := skillMarked(RedistributionUnknown, newestVersion{exists: true, number: 4, license: LicenseClaim{Expression: "MIT", Source: "LICENSE"}})
	content := packageContent(t, "h", false)
	license, source := "Apache-2.0", "manifest"
	content.license, content.licenseSource = &license, &source

	s.AddVersion(content)

	assertSkillEvents(t, s, SkillVersionAdded{VersionNumber: 5, ContentHash: "h"})
	if want := (LicenseClaim{Expression: "Apache-2.0", Source: "manifest"}); s.NewestLicense() != want {
		t.Fatalf("newest licence = %+v, want the added version's %+v", s.NewestLicense(), want)
	}
}

func TestARefusedVersionDoesNotTakeANumber(t *testing.T) {
	s := skillMarked(RedistributionGenerated, newestVersion{exists: true, number: 4})

	s.AddVersion(packageContent(t, "uploaded", false))
	s.AddVersion(packageContent(t, "generated", true))

	assertSkillEvents(t, s, Refused{Reason: RefusedGeneratedNameCollision}, SkillVersionAdded{VersionNumber: 5, ContentHash: "generated"})
}

func (s *SkillRoot) ID() pgtype.UUID { return s.row.ID }

func (s *SkillRoot) Deleted() bool { return s.row.DeletedAt.Valid }

func (s *SkillRoot) Restriction() AccessRestriction { return RestrictionFrom(s.row.AccessRestriction) }

func (s *SkillRoot) Events() []Event {
	if s.events == nil {
		return nil
	}
	events := make([]Event, len(s.events))
	for i, event := range s.events {
		events[i] = cloneEvent(event)
	}
	return events
}
