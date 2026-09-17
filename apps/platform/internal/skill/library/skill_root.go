package registry

import (
	"slices"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/messaging/outbox"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
)

type Refusal string

const (
	RefusedAlreadyTakenDown        Refusal = "already_taken_down"
	RefusedTakedownReasonMissing   Refusal = "takedown_reason_missing"
	RefusedEmptyRestriction        Refusal = "empty_restriction"
	RefusedUnknownRedistribution   Refusal = "unknown_redistribution"
	RefusedProvenanceNotAssertable Refusal = "provenance_not_assertable"
	RefusedLicenseClaimMissing     Refusal = "license_claim_missing"
	RefusedNoVersion               Refusal = "no_version"
	RefusedNoLicenseRecorded       Refusal = "no_license_recorded"
	RefusedLicenseMismatch         Refusal = "license_mismatch"
	RefusedGeneratedIsPermanent    Refusal = "generated_is_permanent"
	RefusedGeneratedNameCollision  Refusal = "generated_name_collision"
	RefusedUnknownCurationTier     Refusal = "unknown_curation_tier"
	RefusedCurationOutsideCatalog  Refusal = "curation_outside_catalogue"
)

type LicenseClaim struct {
	Expression string
	Source     string
}

func (c LicenseClaim) Complete() bool {
	return strings.TrimSpace(c.Expression) != "" && strings.TrimSpace(c.Source) != ""
}

func (c LicenseClaim) matches(recorded LicenseClaim) bool {
	return strings.EqualFold(strings.TrimSpace(c.Expression), recorded.Expression) &&
		strings.EqualFold(strings.TrimSpace(c.Source), recorded.Source)
}

func (r Redistribution) OperatorRefusal() (Refusal, bool) {
	switch r {
	case RedistributionAllowed, RedistributionBlocked, RedistributionUnknown:
		return "", false
	case RedistributionSelfSupplied, RedistributionGenerated:
		return RefusedProvenanceNotAssertable, true
	}
	return RefusedUnknownRedistribution, true
}

type Event interface{ eventType() string }

type Refused struct {
	Reason   Refusal
	Recorded LicenseClaim
}

func (r Refused) Error() string { return "registry: refused: " + string(r.Reason) }

func (r Refused) err() error {
	switch r.Reason {
	case RefusedAlreadyTakenDown:
		return ErrAlreadyTakenDown
	case RefusedTakedownReasonMissing:
		return ErrTakedownReasonRequired
	case RefusedEmptyRestriction:
		return ErrEmptyRestriction
	case RefusedNoVersion:
		return ErrNotFound
	}
	return r
}

type SkillTakenDown struct{}

type AccessRestricted struct {
	Reason string `json:"reason"`
}

type AccessRestrictionLifted struct{}

type RedistributionSet struct {
	Before Redistribution `json:"before"`
	After  Redistribution `json:"after"`
}

type CurationSet struct {
	Before    CurationTier `json:"before"`
	After     CurationTier `json:"after"`
	VersionID pgtype.UUID  `json:"version_id"`
}

type SkillCategorized struct {
	Category *Category       `json:"category"`
	Source   *CategorySource `json:"category_source"`
}

type SkillDeleted struct{}

type SkillCreated struct {
	Redistribution      Redistribution `json:"redistribution"`
	ForkedFromSkillID   pgtype.UUID    `json:"forked_from_skill_id"`
	ForkedFromVersionID pgtype.UUID    `json:"forked_from_version_id"`
}

type SkillVersionAdded struct {
	VersionID     pgtype.UUID  `json:"version_id"`
	VersionNumber int32        `json:"version_number"`
	ContentHash   string       `json:"content_hash"`
	ImprovedBy    *Improvement `json:"improved_by"`
}

type Improvement struct {
	EvaluationID  pgtype.UUID   `json:"evaluation_id"`
	SuggestionIDs []pgtype.UUID `json:"suggestion_ids"`
}

type SkillDescribed struct{}

func (SkillCreated) eventType() string      { return outbox.SkillCreated }
func (SkillVersionAdded) eventType() string { return outbox.SkillVersionAdded }
func (SkillDescribed) eventType() string    { return outbox.SkillDescribed }

func (Refused) eventType() string                 { return "" }
func (SkillTakenDown) eventType() string          { return outbox.SkillTakenDown }
func (AccessRestricted) eventType() string        { return outbox.SkillAccessRestricted }
func (AccessRestrictionLifted) eventType() string { return outbox.SkillAccessRestrictionLifted }
func (RedistributionSet) eventType() string       { return outbox.SkillRedistributionSet }
func (CurationSet) eventType() string             { return outbox.SkillCurationSet }
func (SkillCategorized) eventType() string        { return outbox.SkillCategorized }
func (SkillDeleted) eventType() string            { return outbox.SkillDeleted }

type newestVersion struct {
	id      pgtype.UUID
	exists  bool
	number  int32
	license LicenseClaim
}

type SkillRoot struct {
	row            gen.Skill
	takedownReason string
	newest         newestVersion
	pending        VersionContent
	added          gen.SkillVersion
	events         []Event
	saved          int
}

func startSkill(row gen.Skill, redistribution Redistribution) *SkillRoot {
	row = cloneSkillRow(row)
	if redistribution == "" {
		redistribution = RedistributionUnknown
	}
	row.Redistribution = string(redistribution)
	s := &SkillRoot{row: row}
	s.record(SkillCreated{
		Redistribution:    redistribution,
		ForkedFromSkillID: row.ForkedFromSkillID, ForkedFromVersionID: row.ForkedFromVersionID,
	})
	return s
}

func forkOf(workspaceID pgtype.UUID, name string, source gen.Skill, from gen.SkillVersion) *SkillRoot {
	fork := startSkill(gen.Skill{
		WorkspaceID: workspaceID, Name: name, Summary: source.Summary,
		ForkedFromSkillID: source.ID, ForkedFromVersionID: from.ID,
		AccessRestriction: source.AccessRestriction,
		Category:          source.Category, CategorySource: source.CategorySource,
	}, Redistribution(source.Redistribution))
	fork.AddVersion(copiedContent(from, fork.Generated()))
	return fork
}

func (s *SkillRoot) Skill() Skill { return skillDTO(s.row) }

func (s *SkillRoot) AddedVersion() Version { return versionDTO(s.added) }

func (s *SkillRoot) AcceptsContent(generated bool) bool { return generated || !s.Generated() }

func (s *SkillRoot) Generated() bool { return s.Redistribution() == RedistributionGenerated }

func (s *SkillRoot) TakenDown() bool { return s.row.TakedownAt.Valid }

func (s *SkillRoot) Redistribution() Redistribution { return Redistribution(s.row.Redistribution) }

func (s *SkillRoot) NewestLicense() LicenseClaim { return s.newest.license }

func (s *SkillRoot) Refusal() (Refused, bool) {
	for _, event := range s.events {
		if refused, ok := event.(Refused); ok {
			return refused, true
		}
	}
	return Refused{}, false
}

func (s *SkillRoot) TakeDown(reason string) {
	if s.TakenDown() {
		s.record(Refused{Reason: RefusedAlreadyTakenDown})
		return
	}
	if strings.TrimSpace(reason) == "" {
		s.record(Refused{Reason: RefusedTakedownReasonMissing})
		return
	}
	s.row.TakedownAt, s.takedownReason = pgtype.Timestamptz{Valid: true}, reason
	s.record(SkillTakenDown{})
}

func (s *SkillRoot) Restrict(reason *string) {
	if reason != nil && !RestrictionFrom(reason).InEffect() {
		s.record(Refused{Reason: RefusedEmptyRestriction})
		return
	}
	s.row.AccessRestriction = pgconv.Clone(reason)
	if reason == nil {
		s.record(AccessRestrictionLifted{})
		return
	}
	s.record(AccessRestricted{Reason: *reason})
}

func (s *SkillRoot) SetRedistribution(to Redistribution, claim LicenseClaim) {
	if refused, ok := s.redistributionRefusal(to, claim); ok {
		s.record(refused)
		return
	}
	before := s.Redistribution()
	s.row.Redistribution = string(to)
	s.record(RedistributionSet{Before: before, After: to})
}

func (s *SkillRoot) redistributionRefusal(to Redistribution, claim LicenseClaim) (Refused, bool) {
	if reason, refused := to.OperatorRefusal(); refused {
		return Refused{Reason: reason}, true
	}
	if to == RedistributionAllowed {
		switch {
		case !claim.Complete():
			return Refused{Reason: RefusedLicenseClaimMissing}, true
		case !s.newest.exists:
			return Refused{Reason: RefusedNoVersion}, true
		case !s.newest.license.Complete():
			return Refused{Reason: RefusedNoLicenseRecorded}, true
		case !claim.matches(s.newest.license):
			return Refused{Reason: RefusedLicenseMismatch, Recorded: s.newest.license}, true
		}
	}
	if s.Redistribution() == RedistributionGenerated {
		return Refused{Reason: RefusedGeneratedIsPermanent}, true
	}
	return Refused{}, false
}

func (s *SkillRoot) SetCuration(to CurationTier, inCatalogue bool) {
	if refused, ok := s.curationRefusal(to, inCatalogue); ok {
		s.record(refused)
		return
	}
	before := CurationTier(s.row.CurationTier)
	s.row.CurationTier, s.row.CuratedVersionID = string(to), pgtype.UUID{}
	if to == CurationCurated {
		s.row.CuratedVersionID = s.newest.id
	}
	s.record(CurationSet{Before: before, After: to, VersionID: s.row.CuratedVersionID})
}

func (s *SkillRoot) curationRefusal(to CurationTier, inCatalogue bool) (Refused, bool) {
	switch {
	case !slices.Contains(AllCurationTiers(), to):
		return Refused{Reason: RefusedUnknownCurationTier}, true
	case to == CurationIndexed:
		return Refused{}, false
	case !inCatalogue:
		return Refused{Reason: RefusedCurationOutsideCatalog}, true
	case !s.newest.exists:
		return Refused{Reason: RefusedNoVersion}, true
	}
	return Refused{}, false
}

func (s *SkillRoot) Categorize(category *Category) {
	var source *CategorySource
	s.row.Category, s.row.CategorySource = nil, nil
	if category != nil {
		owner := CategorySourceOwner
		value, from := string(*category), string(owner)
		source, s.row.Category, s.row.CategorySource = &owner, &value, &from
	}
	s.record(SkillCategorized{Category: category, Source: source})
}

func (s *SkillRoot) AddVersion(content VersionContent) {
	if !s.AcceptsContent(content.generated) {
		s.record(Refused{Reason: RefusedGeneratedNameCollision})
		return
	}
	content = cloneVersionContent(content)
	s.pending = content
	s.newest = newestVersion{exists: true, number: s.newest.number + 1, license: content.licenseClaim()}
	s.record(SkillVersionAdded{
		VersionNumber: s.newest.number, ContentHash: content.contentHash, ImprovedBy: content.improvedBy,
	})
}

func (s *SkillRoot) AdoptNewestSummary() {
	summary := s.pending.summary
	s.row.Summary = &summary
	s.record(SkillDescribed{})
}

func (s *SkillRoot) Delete() {
	s.row.DeletedAt = pgtype.Timestamptz{Valid: true}
	s.record(SkillDeleted{})
}

func (s *SkillRoot) record(event Event) { s.events = append(s.events, cloneEvent(event)) }

func cloneSkillRow(row gen.Skill) gen.Skill {
	row.Summary = pgconv.Clone(row.Summary)
	row.TakedownReason = pgconv.Clone(row.TakedownReason)
	row.AccessRestriction = pgconv.Clone(row.AccessRestriction)
	row.Category = pgconv.Clone(row.Category)
	row.CategorySource = pgconv.Clone(row.CategorySource)
	return row
}

func cloneVersionContent(content VersionContent) VersionContent {
	content.manifest = slices.Clone(content.manifest)
	content.license = pgconv.Clone(content.license)
	content.licenseSource = pgconv.Clone(content.licenseSource)
	content.improvedBy = cloneImprovement(content.improvedBy)
	return content
}

func cloneImprovement(improvement *Improvement) *Improvement {
	cloned := pgconv.Clone(improvement)
	if cloned != nil {
		cloned.SuggestionIDs = slices.Clone(improvement.SuggestionIDs)
	}
	return cloned
}

func cloneEvent(event Event) Event {
	switch event := event.(type) {
	case SkillCategorized:
		return SkillCategorized{Category: pgconv.Clone(event.Category), Source: pgconv.Clone(event.Source)}
	case SkillVersionAdded:
		event.ImprovedBy = cloneImprovement(event.ImprovedBy)
		return event
	}
	return event
}
