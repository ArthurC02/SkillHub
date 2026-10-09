package publishing

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	identity "github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/audit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
)

type ExposureDecision string

type CatalogExposureState string

const (
	ExposureApproved ExposureDecision = "approved"
	ExposureRevoked  ExposureDecision = "revoked"
)

const (
	CatalogListed         CatalogExposureState = "listed"
	CatalogAwaitingReview CatalogExposureState = "awaiting_review"
	CatalogRevoked        CatalogExposureState = "revoked"
	CatalogReviewOutdated CatalogExposureState = "review_outdated"
	CatalogNotEligible    CatalogExposureState = "not_eligible"
	CatalogSearchNotReady CatalogExposureState = "search_not_ready"
	CatalogUnreleased     CatalogExposureState = "unreleased"
)

func AllExposureDecisions() []ExposureDecision {
	return []ExposureDecision{ExposureApproved, ExposureRevoked}
}

type SearchSnapshot struct {
	VersionID       pgtype.UUID
	Name            string
	Summary         string
	EnrichedSummary string
	TaskExamples    string
	Tags            json.RawMessage
	Limitations     string
	Enriched        bool
	Listable        bool
	Digest          string
}

type ExposureState struct {
	PublicationID    pgtype.UUID
	Publisher        string
	Name             string
	Status           Status
	SkillID          pgtype.UUID
	OwnerWorkspaceID pgtype.UUID
	ReleaseID        pgtype.UUID
	VersionID        pgtype.UUID
	VersionNumber    int32
	ContentHash      string
	ReleasedAt       time.Time
	Sequence         int32
	Concluded        bool
	Approved         bool
	ReviewedDigest   string
}

type Exposure struct {
	SkillID          pgtype.UUID
	VersionID        pgtype.UUID
	SnapshotDigest   string
	OwnerWorkspaceID pgtype.UUID
}

type CatalogExposure struct {
	State CatalogExposureState
}

type ExposureReview struct {
	Sequence       int32
	ReleaseID      pgtype.UUID
	ContentHash    string
	SnapshotDigest string
	Decision       ExposureDecision
	Reason         string
	ReviewerUserID pgtype.UUID
	ReviewedAt     time.Time
}

type ExposureCase struct {
	State           ExposureState
	Snapshot        *SearchSnapshot
	Exposed         bool
	ApprovalProblem ExposureProblem
	History         []ExposureReview
}

type ExposureInput struct {
	ReleaseID        pgtype.UUID
	ExpectedSequence int32
	ExpectedDigest   string
	Decision         ExposureDecision
	Reason           string
}

type ExposureProblem string

const (
	ExposureStale           ExposureProblem = "review_stale"
	ExposureReasonMissing   ExposureProblem = "reason_missing"
	ExposureDecisionUnknown ExposureProblem = "decision_unknown"
	ExposureNotPublished    ExposureProblem = "not_published"
	ExposureNotAvailable    ExposureProblem = "not_available"
	ExposureNotAllowed      ExposureProblem = "redistribution_not_allowed"
	ExposureSnapshotMissing ExposureProblem = "snapshot_not_current"
	ExposureSnapshotPending ExposureProblem = "snapshot_pending"
)

var exposureProblemWords = map[ExposureProblem]string{
	ExposureStale:           "這份審核的前提已經過期：有新的 Release，或別人已經審過。重新打開這一筆，看過現在的內容再送出",
	ExposureReasonMissing:   "核准或撤銷都要寫理由",
	ExposureDecisionUnknown: "審核結論只能是核准或撤銷",
	ExposureNotPublished:    "這個發佈物已經撤回，不能核准曝光",
	ExposureNotAvailable:    "這個 Skill 目前被下架、被保留或已刪除，不能核准曝光",
	ExposureNotAllowed:      "可散布判定不是 allowed：要先以既有的可散布判定動詞附授權證據判成 allowed，才能核准曝光",
	ExposureSnapshotMissing: "搜尋索引裡的內容不是這一筆 Release 的版本（還沒索引，或作者之後上傳了新版本），不能核准審核者沒看過的內容",
	ExposureSnapshotPending: "這一版的搜尋內容還在補充，補完之前的文字不是最後會被搜尋與顯示的那一份",
}

type ExposureError struct {
	Problem ExposureProblem
}

func (e *ExposureError) Error() string {
	return exposureProblemWords[e.Problem]
}

func exposureStateOf(row gen.ListExposureStatesRow) ExposureState {
	var version int32
	if row.VersionNumber != nil {
		version = *row.VersionNumber
	}
	concluded := row.ReviewedReleaseID.Valid && row.ReviewedReleaseID == row.ReleaseID
	return ExposureState{
		PublicationID: row.PublicationID, Publisher: row.PublisherName, Name: row.Name, Status: Status(row.Status),
		SkillID: row.SkillID, OwnerWorkspaceID: row.PublisherWorkspaceID,
		ReleaseID: row.ReleaseID, VersionID: row.SkillVersionID, VersionNumber: version,
		ContentHash: row.ContentHash, ReleasedAt: row.ReleasedAt.Time, Sequence: row.Sequence,
		Concluded: concluded, Approved: concluded && ExposureDecision(row.Decision) == ExposureApproved,
		ReviewedDigest: row.ReviewedSnapshotDigest,
	}
}

func ownerExposureStateOf(row gen.ListWorkspaceSkillPublicationsRow) ExposureState {
	concluded := row.ReviewedReleaseID.Valid && row.ReviewedReleaseID == row.LatestReleaseID
	var version int32
	if row.LatestVersionNumber != nil {
		version = *row.LatestVersionNumber
	}
	return ExposureState{
		PublicationID: row.PublicationID, Publisher: row.PublisherName, Name: row.Name,
		Status: Status(row.Status), SkillID: row.SkillID, OwnerWorkspaceID: row.PublisherWorkspaceID,
		ReleaseID: row.LatestReleaseID, VersionID: row.LatestVersionID, VersionNumber: version,
		ReleasedAt: row.LatestReleasedAt.Time,
		Sequence:   row.ExposureSequence, Concluded: concluded,
		Approved:       concluded && ExposureDecision(row.ExposureDecision) == ExposureApproved,
		ReviewedDigest: row.ReviewedSnapshotDigest,
	}
}

func exposureStates(ctx context.Context, q *gen.Queries, publicationID pgtype.UUID) ([]ExposureState, error) {
	rows, err := q.ListExposureStates(ctx, publicationID)
	if err != nil {
		return nil, err
	}
	out := make([]ExposureState, 0, len(rows))
	for _, row := range rows {
		out = append(out, exposureStateOf(row))
	}
	return out, nil
}

func (s *Service) requireSnapshotRead() error {
	if s.ReadSearchSnapshot == nil {
		return errors.New("publishing: search snapshot read is not configured")
	}
	return nil
}

func (s *Service) exposureEligible(ctx context.Context, state ExposureState) (bool, error) {
	if state.Status != StatusPublished {
		return false, nil
	}
	skill, found, err := s.ReadSkill(ctx, state.OwnerWorkspaceID, state.SkillID)
	if err != nil {
		return false, err
	}
	return exposable(state.Status, skill, found), nil
}

func exposable(status Status, skill SkillFacts, found bool) bool {
	return availabilityOf(status, skill, found) == AvailabilityAvailable && skill.Redistribution == redistributionAllowed
}

func approvalProblem(state ExposureState, skill SkillFacts, skillFound bool, snapshot *SearchSnapshot) ExposureProblem {
	switch {
	case state.Status != StatusPublished:
		return ExposureNotPublished
	case !skillFound || skill.TakenDown || skill.AccessRestricted:
		return ExposureNotAvailable
	case skill.Redistribution != redistributionAllowed:
		return ExposureNotAllowed
	case snapshot == nil || snapshot.VersionID != state.VersionID:
		return ExposureSnapshotMissing
	case !snapshot.Enriched:
		return ExposureSnapshotPending
	default:
		return ""
	}
}

func (s *Service) catalogExposure(ctx context.Context, state ExposureState) (CatalogExposure, error) {
	eligible, err := s.exposureEligible(ctx, state)
	if err != nil {
		return CatalogExposure{}, err
	}
	if !eligible {
		return CatalogExposure{State: CatalogNotEligible}, nil
	}
	if !state.Concluded {
		return CatalogExposure{State: CatalogAwaitingReview}, nil
	}
	if !state.Approved {
		return CatalogExposure{State: CatalogRevoked}, nil
	}
	if err := s.requireSnapshotRead(); err != nil {
		return CatalogExposure{}, err
	}
	snapshot, found, err := s.ReadSearchSnapshot(ctx, state.SkillID)
	if err != nil {
		return CatalogExposure{}, err
	}
	if !found {
		return CatalogExposure{State: CatalogSearchNotReady}, nil
	}
	if snapshot.VersionID != state.VersionID || snapshot.Digest != state.ReviewedDigest {
		return CatalogExposure{State: CatalogReviewOutdated}, nil
	}
	if !snapshot.Listable {
		return CatalogExposure{State: CatalogSearchNotReady}, nil
	}
	return CatalogExposure{State: CatalogListed}, nil
}

func (s *Service) ExposedSkills(ctx context.Context) ([]Exposure, error) {
	states, err := exposureStates(ctx, gen.New(s.Pool), pgtype.UUID{})
	if err != nil {
		return nil, err
	}
	return s.exposedAmong(ctx, states)
}

func (s *Service) exposedAmong(ctx context.Context, states []ExposureState) ([]Exposure, error) {
	candidates := make([]ExposureState, 0, len(states))
	refs := make([]SkillRef, 0, len(states))
	for _, state := range states {
		if state.mayBeExposed() {
			candidates = append(candidates, state)
			refs = append(refs, SkillRef{WorkspaceID: state.OwnerWorkspaceID, SkillID: state.SkillID})
		}
	}
	if len(candidates) == 0 {
		return nil, nil
	}
	skills, err := s.ReadSkills(ctx, refs)
	if err != nil {
		return nil, err
	}
	var out []Exposure
	for i, state := range candidates {
		skill, found := skills[refs[i]]
		if exposable(state.Status, skill, found) {
			out = append(out, Exposure{
				SkillID: state.SkillID, VersionID: state.VersionID,
				SnapshotDigest: state.ReviewedDigest, OwnerWorkspaceID: state.OwnerWorkspaceID,
			})
		}
	}
	return out, nil
}

func (state ExposureState) mayBeExposed() bool {
	return state.Approved && state.Status == StatusPublished
}

func (s *Service) exposedNow(ctx context.Context, state ExposureState) (bool, error) {
	var skill SkillFacts
	found := false
	if state.mayBeExposed() {
		var err error
		if skill, found, err = s.ReadSkill(ctx, state.OwnerWorkspaceID, state.SkillID); err != nil {
			return false, err
		}
	}
	exposed, _, err := s.exposedFor(ctx, state, skill, found)
	return exposed, err
}

func (s *Service) exposedFor(ctx context.Context, state ExposureState, skill SkillFacts, skillFound bool) (bool, *SearchSnapshot, error) {
	if err := s.requireSnapshotRead(); err != nil {
		return false, nil, err
	}
	snapshot, found, err := s.ReadSearchSnapshot(ctx, state.SkillID)
	if err != nil || !found {
		return false, nil, err
	}
	current := snapshot.Listable && snapshot.VersionID == state.VersionID && snapshot.Digest == state.ReviewedDigest
	return state.Approved && exposable(state.Status, skill, skillFound) && current, &snapshot, nil
}

func (s *Service) ExposureQueue(ctx context.Context) ([]ExposureState, error) {
	states, err := exposureStates(ctx, gen.New(s.Pool), pgtype.UUID{})
	if err != nil {
		return nil, err
	}
	var queue []ExposureState
	for _, state := range states {
		if state.Status != StatusPublished {
			continue
		}
		if !state.Concluded {
			queue = append(queue, state)
			continue
		}
		if !state.Approved {
			continue
		}
		exposed, err := s.exposedNow(ctx, state)
		if err != nil {
			return nil, err
		}
		if !exposed {
			queue = append(queue, state)
		}
	}
	return queue, nil
}

func (s *Service) ExposureCase(ctx context.Context, publisher, name string) (ExposureCase, bool, error) {
	q := gen.New(s.Pool)
	row, err := q.GetPublicPublication(ctx, gen.GetPublicPublicationParams{PublisherName: publisher, Name: name})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !row.SkillID.Valid) {
		return ExposureCase{}, false, nil
	}
	if err != nil {
		return ExposureCase{}, false, err
	}
	states, err := exposureStates(ctx, q, row.ID)
	if err != nil || len(states) == 0 {
		return ExposureCase{}, false, err
	}
	out := ExposureCase{State: states[0]}
	var skill SkillFacts
	var found bool
	if out.State.Status == StatusPublished {
		skill, found, err = s.ReadSkill(ctx, out.State.OwnerWorkspaceID, out.State.SkillID)
		if err != nil {
			return ExposureCase{}, false, err
		}
	}
	out.Exposed, out.Snapshot, err = s.exposedFor(ctx, out.State, skill, found)
	if err != nil {
		return ExposureCase{}, false, err
	}
	out.ApprovalProblem = approvalProblem(out.State, skill, found, out.Snapshot)
	reviews, err := q.ListExposureReviews(ctx, row.ID)
	if err != nil {
		return ExposureCase{}, false, err
	}
	for _, r := range reviews {
		out.History = append(out.History, ExposureReview{
			Sequence: r.Sequence, ReleaseID: r.ReleaseID, ContentHash: r.ContentHash, SnapshotDigest: r.SnapshotDigest,
			Decision: ExposureDecision(r.Decision), Reason: r.Reason, ReviewerUserID: r.ReviewerUserID,
			ReviewedAt: r.ReviewedAt.Time,
		})
	}
	return out, true, nil
}

func (s *Service) ReviewExposure(
	ctx context.Context, reviewer identity.User, publisher, name string, in ExposureInput,
) (ExposureCase, error) {
	if err := s.requireSnapshotRead(); err != nil {
		return ExposureCase{}, err
	}
	reason := strings.TrimSpace(in.Reason)
	switch {
	case in.Decision != ExposureApproved && in.Decision != ExposureRevoked:
		return ExposureCase{}, &ExposureError{ExposureDecisionUnknown}
	case reason == "":
		return ExposureCase{}, &ExposureError{ExposureReasonMissing}
	}

	reviewed, err := currentExposure(ctx, gen.New(s.Pool), publisher, name)
	if err != nil {
		return ExposureCase{}, err
	}
	if reviewed.ReleaseID != in.ReleaseID || reviewed.Sequence != in.ExpectedSequence {
		return ExposureCase{}, &ExposureError{ExposureStale}
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return ExposureCase{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := gen.New(tx)
	publicationID, state, err := lockCurrentExposure(ctx, q, publisher, name)
	if err != nil {
		return ExposureCase{}, err
	}
	if !state.sameReleaseAs(reviewed) {
		return ExposureCase{}, &ExposureError{ExposureStale}
	}
	digest, err := s.approvalDigest(ctx, state, in)
	if err != nil {
		return ExposureCase{}, err
	}
	review, err := q.InsertExposureReview(ctx, gen.InsertExposureReviewParams{
		PublicationID: publicationID, Sequence: state.Sequence + 1, ReleaseID: state.ReleaseID,
		ContentHash: state.ContentHash, SnapshotDigest: digest, Decision: string(in.Decision),
		Reason: reason, ReviewerUserID: reviewer.ID,
	})
	if err != nil {
		return ExposureCase{}, err
	}
	if err := audit.Log(ctx, tx, audit.Event{
		Actor: reviewer.ID, Action: audit.ActionExposureReview,
		ResourceType: audit.ResourcePublication, ResourceID: publicationID,
		Metadata: map[string]any{
			"publisher": publisher, auditKeyName: name, "decision": string(in.Decision), "reason": reason,
			"sequence": review.Sequence, auditKeyReleaseID: pgconv.UUIDString(state.ReleaseID),
			auditKeyContentHash: state.ContentHash, "snapshot_digest": digest,
		},
	}); err != nil {
		return ExposureCase{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ExposureCase{}, err
	}
	out, _, err := s.ExposureCase(ctx, publisher, name)
	return out, err
}

func currentExposure(ctx context.Context, q *gen.Queries, publisher, name string) (ExposureState, error) {
	row, err := q.GetPublicPublication(ctx, gen.GetPublicPublicationParams{PublisherName: publisher, Name: name})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !row.SkillID.Valid) {
		return ExposureState{}, ErrNotFound
	}
	if err != nil {
		return ExposureState{}, err
	}
	states, err := exposureStates(ctx, q, row.ID)
	if err != nil {
		return ExposureState{}, err
	}
	if len(states) == 0 {
		return ExposureState{}, ErrNotFound
	}
	return states[0], nil
}

func (state ExposureState) sameReleaseAs(other ExposureState) bool {
	return state.ReleaseID == other.ReleaseID && state.Sequence == other.Sequence && state.Status == other.Status &&
		state.SkillID == other.SkillID && state.OwnerWorkspaceID == other.OwnerWorkspaceID && state.VersionID == other.VersionID
}

func lockCurrentExposure(ctx context.Context, q *gen.Queries, publisher, name string) (pgtype.UUID, ExposureState, error) {
	publicationID, err := q.LockPublicationByAddress(ctx, gen.LockPublicationByAddressParams{PublisherName: publisher, Name: name})
	if errors.Is(err, pgx.ErrNoRows) {
		return pgtype.UUID{}, ExposureState{}, ErrNotFound
	}
	if err != nil {
		return pgtype.UUID{}, ExposureState{}, err
	}
	states, err := exposureStates(ctx, q, publicationID)
	if err != nil {
		return pgtype.UUID{}, ExposureState{}, err
	}
	if len(states) == 0 {
		return pgtype.UUID{}, ExposureState{}, ErrNotFound
	}
	return publicationID, states[0], nil
}

func (s *Service) approvalDigest(ctx context.Context, state ExposureState, in ExposureInput) (string, error) {
	snapshot, found, err := s.ReadSearchSnapshot(ctx, state.SkillID)
	if err != nil {
		return "", err
	}
	if in.Decision == ExposureRevoked {
		return snapshot.Digest, nil
	}
	if state.Status != StatusPublished {
		return "", &ExposureError{ExposureNotPublished}
	}
	skill, skillFound, err := s.ReadSkill(ctx, state.OwnerWorkspaceID, state.SkillID)
	if err != nil {
		return "", err
	}
	var current *SearchSnapshot
	if found {
		current = &snapshot
	}
	if problem := approvalProblem(state, skill, skillFound, current); problem != "" {
		return "", &ExposureError{problem}
	}
	if snapshot.Digest != in.ExpectedDigest {
		return "", &ExposureError{ExposureStale}
	}
	return snapshot.Digest, nil
}
