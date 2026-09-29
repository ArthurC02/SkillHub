package publishing

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestAPublisherNameFollowsTheSkillNameRuleAndRefusesReservedWords(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want NameProblem
	}{
		{"one character", "a", ""},
		{"sixty-four characters", strings.Repeat("a", 64), ""},
		{"sixty-five characters", strings.Repeat("a", 65), NameShape},
		{"empty", "", NameShape},
		{"leading hyphen", "-tools", NameShape},
		{"trailing hyphen", "tools-", NameShape},
		{"two hyphens in a row", "my--tools", NameShape},
		{"upper case", "MyTools", NameShape},
		{"underscore", "my_tools", NameShape},
		{"digits and single hyphens", "team-42-tools", ""},
		{"a reserved word", "skillhub", NameReserved},
		{"a reserved word spelled with a hyphen", "skill-hub", NameReserved},
		{"a reserved word spelled with several hyphens", "s-k-i-l-l-h-u-b", NameReserved},
		{"a name that only contains a reserved word", "skillhub-fans", ""},
		{"a vendor name", "openai", NameReserved},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := publisherNameProblem(tc.in); got != tc.want {
				t.Errorf("publisherNameProblem(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestAPublicationNameFollowsTheSkillNameRuleButMayUseAReservedWord(t *testing.T) {
	if got := publicationNameProblem("skillhub"); got != "" {
		t.Errorf("a publication name inside the author's own namespace was refused as reserved: %q", got)
	}
	if got := publicationNameProblem("Bad Name"); got != NameShape {
		t.Errorf("publicationNameProblem(%q) = %q, want %q", "Bad Name", got, NameShape)
	}
}

func TestTheReleaseGateAnswersEveryRedistributionValueAndTheHold(t *testing.T) {
	cases := []struct {
		name           string
		skill          SkillFacts
		rightsAttested bool
		want           Refusal
	}{
		{"allowed releases without a statement", SkillFacts{Redistribution: "allowed"}, false, ""},
		{"self supplied needs the author's statement", SkillFacts{Redistribution: "self_supplied"}, false, RefusedRightsNotAttested},
		{"self supplied with the statement releases", SkillFacts{Redistribution: "self_supplied"}, true, ""},
		{"generated needs the author's statement", SkillFacts{Redistribution: "generated"}, false, RefusedRightsNotAttested},
		{"generated with the statement releases", SkillFacts{Redistribution: "generated"}, true, ""},
		{"blocked refuses even with a statement", SkillFacts{Redistribution: "blocked"}, true, RefusedNotRedistributable},
		{"unknown refuses even with a statement", SkillFacts{Redistribution: "unknown"}, true, RefusedLicenseUnknown},
		{"a value outside the vocabulary refuses", SkillFacts{Redistribution: "shared"}, true, RefusedLicenseUnknown},
		{"a hold refuses an allowed skill", SkillFacts{Redistribution: "allowed", AccessRestricted: true}, true, RefusedLicenseHold},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			refused := releaseGate(tc.skill, PublishInput{RightsAttested: tc.rightsAttested}.attestation())
			var got Refusal
			if refused != nil {
				got = refused.Reason
				if refused.Message == "" {
					t.Error("a refusal arrived without a sentence for the author")
				}
			}
			if got != tc.want {
				t.Errorf("releaseGate = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestAPublicAddressReportsWhyItNoLongerOffersItsContent(t *testing.T) {
	cases := []struct {
		name   string
		status Status
		skill  SkillFacts
		found  bool
		want   Availability
	}{
		{"published and releasable", StatusPublished, SkillFacts{Redistribution: "self_supplied"}, true, AvailabilityAvailable},
		{"delisted wins over everything", StatusDelisted, SkillFacts{TakenDown: true}, true, AvailabilityDelisted},
		{"the skill was deleted", StatusPublished, SkillFacts{}, false, AvailabilityWithdrawn},
		{"taken down", StatusPublished, SkillFacts{TakenDown: true, Redistribution: "allowed"}, true, AvailabilityTakenDown},
		{"held", StatusPublished, SkillFacts{AccessRestricted: true, Redistribution: "allowed"}, true, AvailabilityHeld},
		{"judged blocked after release", StatusPublished, SkillFacts{Redistribution: "blocked"}, true, AvailabilityNotRedistributed},
		{"judged unknown after release", StatusPublished, SkillFacts{Redistribution: "unknown"}, true, AvailabilityNotRedistributed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := availabilityOf(tc.status, tc.skill, tc.found); got != tc.want {
				t.Errorf("availabilityOf = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestOwnerAvailabilityUsesTheSameSkillFactsAsThePublicAddress(t *testing.T) {
	readFailed := errors.New("skill facts unavailable")
	versionReadFailed := errors.New("version facts unavailable")
	cases := []struct {
		name         string
		status       Status
		skill        SkillFacts
		skillFound   bool
		skillErr     error
		hasRelease   bool
		versionFound bool
		versionErr   error
		want         Availability
		wantErr      bool
	}{
		{"published and releasable", StatusPublished, SkillFacts{Redistribution: "allowed"}, true, nil, true, true, nil, AvailabilityAvailable, false},
		{"delisted does not need another read", StatusDelisted, SkillFacts{}, false, readFailed, false, false, versionReadFailed, AvailabilityDelisted, false},
		{"deleted", StatusPublished, SkillFacts{}, false, nil, true, false, nil, AvailabilityWithdrawn, false},
		{"taken down", StatusPublished, SkillFacts{TakenDown: true, Redistribution: "allowed"}, true, nil, true, false, nil, AvailabilityTakenDown, false},
		{"held", StatusPublished, SkillFacts{AccessRestricted: true, Redistribution: "allowed"}, true, nil, true, false, nil, AvailabilityHeld, false},
		{"not redistributable", StatusPublished, SkillFacts{Redistribution: "blocked"}, true, nil, true, false, nil, AvailabilityNotRedistributed, false},
		{"no release", StatusPublished, SkillFacts{Redistribution: "allowed"}, true, nil, false, false, nil, AvailabilityWithdrawn, false},
		{"latest version was deleted", StatusPublished, SkillFacts{Redistribution: "allowed"}, true, nil, true, false, nil, AvailabilityWithdrawn, false},
		{"owner facts failed", StatusPublished, SkillFacts{}, false, readFailed, true, false, nil, "", true},
		{"version facts failed", StatusPublished, SkillFacts{Redistribution: "allowed"}, true, nil, true, false, versionReadFailed, "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := Service{ReadSkill: func(context.Context, pgtype.UUID, pgtype.UUID) (SkillFacts, bool, error) {
				return tc.skill, tc.skillFound, tc.skillErr
			}, ReadVersion: func(context.Context, pgtype.UUID, pgtype.UUID) (VersionFacts, bool, error) {
				return VersionFacts{}, tc.versionFound, tc.versionErr
			}}
			var releases []Release
			if tc.hasRelease {
				releases = []Release{{VersionID: pgtype.UUID{Valid: true}}}
			}
			delivery, err := svc.publicationAvailability(
				context.Background(), pgtype.UUID{}, Publication{Status: tc.status, Releases: releases},
			)
			if (err != nil) != tc.wantErr {
				t.Fatalf("publicationAvailability error = %v, want error %v", err, tc.wantErr)
			}
			if delivery.Availability != tc.want {
				t.Errorf("publicationAvailability = %q, want %q", delivery.Availability, tc.want)
			}
		})
	}
}

func TestOwnerAcquisitionKeepsPackageAvailabilityAndInvitationEligibilitySeparate(t *testing.T) {
	handler := Handler{
		DownloadsOpenToUninvited: false,
		InviteRosterConfigured:   func() bool { return true },
	}
	view := handler.ownView(Publication{
		Status: StatusPublished, Availability: AvailabilityAvailable, Releases: []Release{{VersionID: pgtype.UUID{Valid: true}}},
	})

	if !view.Acquisition.Available {
		t.Fatal("acquisition.available = false, want the package offer to remain available")
	}
	if !strings.Contains(view.Acquisition.Note, invitedOnlyNote) {
		t.Errorf("acquisition.note = %q, want the invitation condition", view.Acquisition.Note)
	}
	if view.Availability.Value != string(AvailabilityAvailable) || view.Availability.Label != "提供中" {
		t.Errorf("availability = %#v, want the server-owned available state", view.Availability)
	}
}
