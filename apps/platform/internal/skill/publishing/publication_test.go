package publishing

import (
	"strings"
	"testing"
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
			refused := releaseGate(tc.skill, tc.rightsAttested)
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
