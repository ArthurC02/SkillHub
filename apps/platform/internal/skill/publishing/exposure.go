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

const (
	ExposureApproved ExposureDecision = "approved"
	ExposureRevoked  ExposureDecision = "revoked"
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
	State    ExposureState
	Snapshot *SearchSnapshot
	Exposed  bool
	History  []ExposureReview
}

type ExposureInput struct {
	ReleaseID        pgtype.UUID
	ExpectedSequence int32
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

func (s *Service) eligible(ctx context.Context, state ExposureState) (bool, error) {
	if state.Status != StatusPublished || !state.Approved {
		return false, nil
	}
	skill, found, err := s.ReadSkill(ctx, state.OwnerWorkspaceID, state.SkillID)
	if err != nil {
		return false, err
	}
	return availabilityOf(state.Status, skill, found) == AvailabilityAvailable && skill.Redistribution == redistributionAllowed, nil
}

func (s *Service) ExposedSkills(ctx context.Context) ([]Exposure, error) {
	states, err := exposureStates(ctx, gen.New(s.Pool), pgtype.UUID{})
	if err != nil {
		return nil, err
	}
	var out []Exposure
	for _, state := range states {
		ok, err := s.eligible(ctx, state)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, Exposure{
				SkillID: state.SkillID, VersionID: state.VersionID,
				SnapshotDigest: state.ReviewedDigest, OwnerWorkspaceID: state.OwnerWorkspaceID,
			})
		}
	}
	return out, nil
}

func (s *Service) exposedNow(ctx context.Context, state ExposureState) (bool, *SearchSnapshot, error) {
	if err := s.requireSnapshotRead(); err != nil {
		return false, nil, err
	}
	snapshot, found, err := s.ReadSearchSnapshot(ctx, state.SkillID)
	if err != nil || !found {
		return false, nil, err
	}
	ok, err := s.eligible(ctx, state)
	if err != nil {
		return false, nil, err
	}
	current := snapshot.VersionID == state.VersionID && snapshot.Digest == state.ReviewedDigest
	return ok && current, &snapshot, nil
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
		exposed, _, err := s.exposedNow(ctx, state)
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
	out.Exposed, out.Snapshot, err = s.exposedNow(ctx, out.State)
	if err != nil {
		return ExposureCase{}, false, err
	}
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

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return ExposureCase{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := gen.New(tx)
	publicationID, err := q.LockPublicationByAddress(ctx, gen.LockPublicationByAddressParams{PublisherName: publisher, Name: name})
	if errors.Is(err, pgx.ErrNoRows) {
		return ExposureCase{}, ErrNotFound
	}
	if err != nil {
		return ExposureCase{}, err
	}
	states, err := exposureStates(ctx, q, publicationID)
	if err != nil {
		return ExposureCase{}, err
	}
	if len(states) == 0 {
		return ExposureCase{}, ErrNotFound
	}
	state := states[0]
	if state.ReleaseID != in.ReleaseID || state.Sequence != in.ExpectedSequence {
		return ExposureCase{}, &ExposureError{ExposureStale}
	}
	digest, err := s.approvalDigest(ctx, state, in.Decision)
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
			"publisher": publisher, "name": name, "decision": string(in.Decision), "reason": reason,
			"sequence": review.Sequence, "release_id": pgconv.UUIDString(state.ReleaseID),
			"content_hash": state.ContentHash, "snapshot_digest": digest,
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

func (s *Service) approvalDigest(ctx context.Context, state ExposureState, decision ExposureDecision) (string, error) {
	snapshot, found, err := s.ReadSearchSnapshot(ctx, state.SkillID)
	if err != nil {
		return "", err
	}
	if decision == ExposureRevoked {
		return snapshot.Digest, nil
	}
	if state.Status != StatusPublished {
		return "", &ExposureError{ExposureNotPublished}
	}
	skill, skillFound, err := s.ReadSkill(ctx, state.OwnerWorkspaceID, state.SkillID)
	if err != nil {
		return "", err
	}
	if !skillFound || skill.TakenDown || skill.AccessRestricted {
		return "", &ExposureError{ExposureNotAvailable}
	}
	if skill.Redistribution != redistributionAllowed {
		return "", &ExposureError{ExposureNotAllowed}
	}
	if !found || snapshot.VersionID != state.VersionID {
		return "", &ExposureError{ExposureSnapshotMissing}
	}
	if !snapshot.Enriched {
		return "", &ExposureError{ExposureSnapshotPending}
	}
	return snapshot.Digest, nil
}
