package activity

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

type Source string

const (
	SourceRun        Source = "run"
	SourceEvaluation Source = "evaluation"
	SourceCreation   Source = "creation"
	SourcePackaging  Source = "packaging"
	SourcePublishing Source = "publishing"
)

var Sources = []Source{
	SourceRun,
	SourceEvaluation,
	SourceCreation,
	SourcePackaging,
	SourcePublishing,
}

type Classification string

const (
	NeedsAttention Classification = "needs_attention"
	InProgress     Classification = "in_progress"
	Recent         Classification = "recent"
	Neutral        Classification = "neutral"
)

type Status struct {
	Value string `json:"value"`
	Label string `json:"label"`
	Note  string `json:"note"`
}

type Context struct {
	SkillID         string `json:"skill_id,omitempty"`
	SkillName       string `json:"skill_name,omitempty"`
	SkillVersionID  string `json:"skill_version_id,omitempty"`
	TestCaseID      string `json:"test_case_id,omitempty"`
	ArtifactName    string `json:"artifact_name,omitempty"`
	Publisher       string `json:"publisher,omitempty"`
	PublicationName string `json:"publication_name,omitempty"`
}

type Continuation struct {
	Kind            string `json:"kind"`
	RunID           string `json:"run_id,omitempty"`
	SessionID       string `json:"session_id,omitempty"`
	ArtifactID      string `json:"artifact_id,omitempty"`
	Publisher       string `json:"publisher,omitempty"`
	PublicationName string `json:"publication_name,omitempty"`
}

type Item struct {
	Kind           string         `json:"kind"`
	SourceID       string         `json:"source_id"`
	Summary        string         `json:"summary"`
	Classification Classification `json:"classification"`
	Status         Status         `json:"status"`
	ActivityAt     time.Time      `json:"activity_at"`
	Context        *Context       `json:"context,omitempty"`
	Continuation   Continuation   `json:"continuation"`
}

type Page struct {
	Complete   bool     `json:"complete"`
	Sources    []Source `json:"sources"`
	Items      []Item   `json:"items"`
	NextCursor string   `json:"next_cursor,omitempty"`
}

type RunFact struct {
	RunID          string
	SkillID        string
	SkillName      string
	SkillVersionID string
	TestCaseID     string
	Classification Classification
	Status         Status
	ActivityAt     time.Time
}

type EvaluationFact struct {
	RunID          string
	Classification Classification
	Status         Status
	ActivityAt     time.Time
}

type CreationFact struct {
	SessionID      string
	Summary        string
	Classification Classification
	Status         Status
	ActivityAt     time.Time
}

type PackagingFact struct {
	ArtifactID     string
	SkillVersionID string
	FileName       string
	Classification Classification
	Status         Status
	ActivityAt     time.Time
}

type PublishingFact struct {
	PublicationID   string
	SkillID         string
	LatestVersionID string
	Publisher       string
	Name            string
	Classification  Classification
	Status          Status
	ActivityAt      time.Time
}

type Service struct {
	ReadRuns        func(context.Context, pgtype.UUID) ([]RunFact, error)
	ReadEvaluations func(context.Context, pgtype.UUID) ([]EvaluationFact, error)
	ReadCreations   func(context.Context, pgtype.UUID) ([]CreationFact, error)
	ReadPackaging   func(context.Context, pgtype.UUID) ([]PackagingFact, error)
	ReadPublishing  func(context.Context, pgtype.UUID) ([]PublishingFact, error)
}

type sourceFacts struct {
	runs         []RunFact
	evaluations  []EvaluationFact
	creations    []CreationFact
	artifacts    []PackagingFact
	publications []PublishingFact
	unavailable  []Source
}

type UnavailableError struct {
	Sources []Source
}

func (e *UnavailableError) Error() string {
	return "workspace activity is incomplete"
}

var ErrInvalidCursor = errors.New("invalid activity cursor")

func (s *Service) List(
	ctx context.Context, workspaceID pgtype.UUID, encodedCursor string, limit int,
) (Page, error) {
	if limit < 1 || limit > 100 {
		return Page{}, fmt.Errorf("activity limit must be between 1 and 100")
	}
	var pageCursor *cursor
	if encodedCursor != "" {
		decoded, err := decodeCursor(encodedCursor)
		if err != nil {
			return Page{}, err
		}
		pageCursor = &decoded
	}
	facts := s.readAll(ctx, workspaceID)
	if len(facts.unavailable) > 0 {
		return Page{}, &UnavailableError{Sources: facts.unavailable}
	}
	items := combine(facts.runs, facts.evaluations, facts.creations, facts.artifacts, facts.publications)
	sort.Slice(items, func(i, j int) bool { return compare(items[i], items[j]) < 0 })
	if pageCursor != nil {
		items = slices.DeleteFunc(items, func(item Item) bool { return compareCursor(item, *pageCursor) <= 0 })
	}
	page := Page{Complete: true, Sources: slices.Clone(Sources), Items: items}
	if len(page.Items) > limit {
		page.Items = page.Items[:limit]
		page.NextCursor = encodeCursor(page.Items[len(page.Items)-1])
	}
	return page, nil
}

func (s *Service) readAll(
	ctx context.Context, workspaceID pgtype.UUID,
) sourceFacts {
	var facts sourceFacts
	var err error
	facts.runs, err = callReader(ctx, workspaceID, s.ReadRuns)
	if err != nil {
		facts.unavailable = append(facts.unavailable, SourceRun)
	}
	facts.evaluations, err = callReader(ctx, workspaceID, s.ReadEvaluations)
	if err != nil {
		facts.unavailable = append(facts.unavailable, SourceEvaluation)
	}
	facts.creations, err = callReader(ctx, workspaceID, s.ReadCreations)
	if err != nil {
		facts.unavailable = append(facts.unavailable, SourceCreation)
	}
	facts.artifacts, err = callReader(ctx, workspaceID, s.ReadPackaging)
	if err != nil {
		facts.unavailable = append(facts.unavailable, SourcePackaging)
	}
	facts.publications, err = callReader(ctx, workspaceID, s.ReadPublishing)
	if err != nil {
		facts.unavailable = append(facts.unavailable, SourcePublishing)
	}
	return facts
}

func callReader[T any](
	ctx context.Context, workspaceID pgtype.UUID, reader func(context.Context, pgtype.UUID) ([]T, error),
) ([]T, error) {
	if reader == nil {
		return nil, errors.New("activity reader is not configured")
	}
	return reader(ctx, workspaceID)
}

func combine(
	runs []RunFact,
	evaluations []EvaluationFact,
	creations []CreationFact,
	artifacts []PackagingFact,
	publications []PublishingFact,
) []Item {
	evaluationByRun := make(map[string]EvaluationFact, len(evaluations))
	for _, fact := range evaluations {
		evaluationByRun[fact.RunID] = fact
	}
	items := make([]Item, 0, len(runs)+len(creations)+len(artifacts)+len(publications))
	for _, fact := range runs {
		classification, status, activityAt := fact.Classification, fact.Status, fact.ActivityAt
		if evaluation, ok := evaluationByRun[fact.RunID]; ok {
			if classificationRank(evaluation.Classification) < classificationRank(classification) ||
				(classificationRank(evaluation.Classification) == classificationRank(classification) && evaluation.ActivityAt.After(activityAt)) {
				classification, status = evaluation.Classification, evaluation.Status
			}
			if evaluation.ActivityAt.After(activityAt) {
				activityAt = evaluation.ActivityAt
			}
		}
		items = append(items, Item{
			Kind: "run", SourceID: fact.RunID, Summary: fact.SkillName + " 的試跑",
			Classification: classification, Status: status, ActivityAt: activityAt,
			Context: &Context{
				SkillID: fact.SkillID, SkillName: fact.SkillName,
				SkillVersionID: fact.SkillVersionID, TestCaseID: fact.TestCaseID,
			},
			Continuation: Continuation{Kind: "run", RunID: fact.RunID},
		})
	}
	for _, fact := range creations {
		items = append(items, Item{
			Kind: "creation_session", SourceID: fact.SessionID, Summary: fact.Summary,
			Classification: fact.Classification, Status: fact.Status, ActivityAt: fact.ActivityAt,
			Continuation: Continuation{Kind: "creation_session", SessionID: fact.SessionID},
		})
	}
	for _, fact := range artifacts {
		items = append(items, Item{
			Kind: "packaging_artifact", SourceID: fact.ArtifactID, Summary: fact.FileName,
			Classification: fact.Classification, Status: fact.Status, ActivityAt: fact.ActivityAt,
			Context:      &Context{SkillVersionID: fact.SkillVersionID, ArtifactName: fact.FileName},
			Continuation: Continuation{Kind: "packaging_artifact", ArtifactID: fact.ArtifactID},
		})
	}
	for _, fact := range publications {
		items = append(items, Item{
			Kind: "skill_publication", SourceID: fact.PublicationID,
			Summary:        fact.Publisher + "/" + fact.Name,
			Classification: fact.Classification, Status: fact.Status, ActivityAt: fact.ActivityAt,
			Context: &Context{
				SkillID: fact.SkillID, SkillVersionID: fact.LatestVersionID,
				Publisher: fact.Publisher, PublicationName: fact.Name,
			},
			Continuation: Continuation{
				Kind: "skill_publication", Publisher: fact.Publisher, PublicationName: fact.Name,
			},
		})
	}
	return items
}

type cursor struct {
	Rank       int    `json:"rank"`
	ActivityAt string `json:"activity_at"`
	Kind       string `json:"kind"`
	SourceID   string `json:"source_id"`
}

func encodeCursor(item Item) string {
	payload, _ := json.Marshal(cursor{
		Rank: classificationRank(item.Classification), ActivityAt: item.ActivityAt.UTC().Format(time.RFC3339Nano),
		Kind: item.Kind, SourceID: item.SourceID,
	})
	return base64.RawURLEncoding.EncodeToString(payload)
}

func decodeCursor(encoded string) (cursor, error) {
	payload, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return cursor{}, ErrInvalidCursor
	}
	var value cursor
	if err := json.Unmarshal(payload, &value); err != nil || value.Rank < 0 || value.Rank > 3 || value.Kind == "" || value.SourceID == "" {
		return cursor{}, ErrInvalidCursor
	}
	if _, err := time.Parse(time.RFC3339Nano, value.ActivityAt); err != nil {
		return cursor{}, ErrInvalidCursor
	}
	return value, nil
}

func compare(left, right Item) int {
	if rank := classificationRank(left.Classification) - classificationRank(right.Classification); rank != 0 {
		return rank
	}
	if !left.ActivityAt.Equal(right.ActivityAt) {
		if left.ActivityAt.After(right.ActivityAt) {
			return -1
		}
		return 1
	}
	if left.Kind < right.Kind {
		return -1
	}
	if left.Kind > right.Kind {
		return 1
	}
	if left.SourceID < right.SourceID {
		return -1
	}
	if left.SourceID > right.SourceID {
		return 1
	}
	return 0
}

func compareCursor(item Item, value cursor) int {
	activityAt, _ := time.Parse(time.RFC3339Nano, value.ActivityAt)
	return compare(item, Item{
		Classification: classificationFromRank(value.Rank), ActivityAt: activityAt,
		Kind: value.Kind, SourceID: value.SourceID,
	})
}

func classificationRank(value Classification) int {
	const neutralRank = 3

	switch value {
	case NeedsAttention:
		return 0
	case InProgress:
		return 1
	case Recent:
		return 2
	default:
		return neutralRank
	}
}

func classificationFromRank(rank int) Classification {
	return []Classification{NeedsAttention, InProgress, Recent, Neutral}[rank]
}
