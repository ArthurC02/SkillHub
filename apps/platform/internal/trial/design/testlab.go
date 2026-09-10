package testlab

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/audit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
)

const (
	MaxFileBytes = 25 << 20

	MaxTestCaseBytes = 100 << 20

	MaxFilesPerTestCase = 20

	DatasetRetention = 90 * 24 * time.Hour
)

const (
	MaxNameBytes      = 200
	MaxPromptBytes    = 32 << 10
	MaxCriterionBytes = 2000
	MaxCriteria       = 50

	maxConfirmedCreationCriteria = 12
)

var (
	ErrNotFound = errors.New("找不到這個 Test Case")

	ErrInvalid = errors.New("請求內容無效")

	ErrLimitExceeded             = errors.New("超過上限")
	errRegistryReadNotConfigured = errors.New("testlab: registry owner read is not configured")
	errPersistenceNotConfigured  = errors.New("testlab: persistence is not configured")
)

type SkillFacts struct {
	Name    string
	Summary *string
}

type ObjectStore interface {
	Put(ctx context.Context, key string, data []byte) error

	Get(ctx context.Context, key string) ([]byte, error)
	Remove(ctx context.Context, key string) error
}

type CriteriaSuggester interface {
	SuggestCriteria(ctx context.Context, req llmclient.SuggestCriteriaRequest) (*llmclient.SuggestCriteriaResponse, error)
}

type Service struct {
	Pool  *pgxpool.Pool
	Store ObjectStore

	ReadSkill       func(context.Context, pgtype.UUID, pgtype.UUID) (SkillFacts, bool, error)
	MayStoreObjects func(context.Context, gen.DBTX, pgtype.UUID) (bool, error)

	LLM CriteriaSuggester
}

type Criterion struct {
	ID   string `json:"id"`
	Text string `json:"text"`

	Source string `json:"source"`

	ConfirmedAt *time.Time `json:"confirmed_at"`
}

const (
	SourceUser      = "user"
	SourceSuggested = "suggested"
)

type Rubric struct {
	Version string       `json:"version"`
	Items   []RubricItem `json:"items"`
}

type RubricItem struct {
	ID   string `json:"id"`
	Text string `json:"text"`

	Weight *float64 `json:"weight,omitempty"`

	EvidenceRequired bool `json:"evidence_required"`
}

const MaxRubricItems = 50

const MaxRubricVersionBytes = 200

func (s *Service) SetRubric(ctx context.Context, ws identity.Workspace, id pgtype.UUID, r *Rubric) (gen.TestCase, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return gen.TestCase{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := gen.New(tx)

	tc, err := q.LockTestCase(ctx, gen.LockTestCaseParams{ID: id, WorkspaceID: ws.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return gen.TestCase{}, ErrNotFound
	}
	if err != nil {
		return gen.TestCase{}, err
	}
	criteria, err := DecodeCriteria(tc.AcceptanceCriteria)
	if err != nil {
		return gen.TestCase{}, err
	}

	var encoded []byte
	if r != nil {
		clean, err := validateRubric(*r, criteria)
		if err != nil {
			return gen.TestCase{}, err
		}
		if encoded, err = json.Marshal(clean); err != nil {
			return gen.TestCase{}, err
		}
	}
	updated, err := q.UpdateTestCaseRubric(ctx, gen.UpdateTestCaseRubricParams{
		ID: tc.ID, WorkspaceID: ws.ID, Rubric: encoded,
	})
	if err != nil {
		return gen.TestCase{}, err
	}
	return updated, tx.Commit(ctx)
}

func validateRubric(r Rubric, criteria []Criterion) (Rubric, error) {
	r.Version = strings.TrimSpace(r.Version)
	switch {
	case r.Version == "":
		return Rubric{}, fmt.Errorf("%w: rubric 的 version 不能空白", ErrInvalid)
	case len(r.Version) > MaxRubricVersionBytes:
		return Rubric{}, fmt.Errorf("%w: rubric 的 version 最多 %d bytes", ErrInvalid, MaxRubricVersionBytes)
	case len(r.Items) == 0:

		return Rubric{}, fmt.Errorf("%w: rubric 至少要有一條；要移除 rubric 請送 null", ErrInvalid)
	case len(r.Items) > MaxRubricItems:
		return Rubric{}, fmt.Errorf("%w: 一個 Test Case 的 rubric 最多 %d 條", ErrLimitExceeded, MaxRubricItems)
	}

	known := make(map[string]bool, len(criteria))
	for _, c := range criteria {
		known[c.ID] = true
	}
	seen := make(map[string]bool, len(r.Items))
	items := make([]RubricItem, 0, len(r.Items))
	for _, it := range r.Items {
		it.Text = strings.TrimSpace(it.Text)
		switch {
		case !known[it.ID]:
			return Rubric{}, fmt.Errorf(
				"%w: rubric 條目 %q 對不上任何驗收條件；條目的 id 必須是它要強化的那條驗收條件", ErrInvalid, it.ID)
		case seen[it.ID]:
			return Rubric{}, fmt.Errorf("%w: 驗收條件 %q 有兩條 rubric", ErrInvalid, it.ID)
		case it.Text == "":
			return Rubric{}, fmt.Errorf("%w: rubric 條目的文字不能空白", ErrInvalid)
		case len(it.Text) > MaxCriterionBytes:
			return Rubric{}, fmt.Errorf("%w: rubric 條目的文字最多 %d bytes", ErrInvalid, MaxCriterionBytes)
		}
		seen[it.ID] = true
		items = append(items, it)
	}
	r.Items = items
	return r, nil
}

func DecodeRubric(raw []byte) (*Rubric, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	r := &Rubric{}
	if err := json.Unmarshal(raw, r); err != nil {
		return nil, fmt.Errorf("decode rubric: %w", err)
	}
	return r, nil
}

func (s *Service) CreateTestCase(ctx context.Context, ws identity.Workspace, skillID pgtype.UUID, name, prompt string) (gen.TestCase, error) {
	name, prompt, err := validateDraft(name, prompt)
	if err != nil {
		return gen.TestCase{}, err
	}
	if s.ReadSkill == nil {
		return gen.TestCase{}, errRegistryReadNotConfigured
	}
	q := gen.New(s.Pool)
	_, found, err := s.ReadSkill(ctx, ws.ID, skillID)
	if !found && err == nil {
		return gen.TestCase{}, ErrNotFound
	}
	if err != nil {
		return gen.TestCase{}, err
	}
	return q.CreateTestCase(ctx, gen.CreateTestCaseParams{
		WorkspaceID:        ws.ID,
		SkillID:            skillID,
		Name:               name,
		UserPrompt:         prompt,
		AcceptanceCriteria: []byte("[]"),
	})
}

func (s *Service) CreateTestCaseWithCriteria(
	ctx context.Context, tx pgx.Tx, ws identity.Workspace, skillID pgtype.UUID, name, prompt string, criteria []string,
) (gen.TestCase, error) {
	name, prompt, err := validateDraft(name, prompt)
	if err != nil {
		return gen.TestCase{}, err
	}
	if len(criteria) > maxConfirmedCreationCriteria {
		return gen.TestCase{}, fmt.Errorf("%w: 一個 Test Case 最多 %d 條驗收條件", ErrLimitExceeded, maxConfirmedCreationCriteria)
	}
	list := make([]Criterion, len(criteria))
	now := time.Now().UTC()
	for i, text := range criteria {
		text, err = validateCriterion(text)
		if err != nil {
			return gen.TestCase{}, err
		}
		list[i] = Criterion{ID: newCriterionID(), Text: text, Source: SourceUser, ConfirmedAt: &now}
	}

	encoded, err := json.Marshal(list)
	if err != nil {
		return gen.TestCase{}, err
	}
	return gen.New(tx).CreateTestCase(ctx, gen.CreateTestCaseParams{
		WorkspaceID:        ws.ID,
		SkillID:            skillID,
		Name:               name,
		UserPrompt:         prompt,
		AcceptanceCriteria: encoded,
	})
}

func (s *Service) GetTestCase(ctx context.Context, ws identity.Workspace, id pgtype.UUID) (gen.TestCase, error) {
	tc, err := gen.New(s.Pool).GetTestCase(ctx, gen.GetTestCaseParams{ID: id, WorkspaceID: ws.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return gen.TestCase{}, ErrNotFound
	}
	return tc, err
}

type TestCaseSummary struct {
	TestCase  gen.TestCase
	SkillName string
}

func (s *Service) ListTestCases(
	ctx context.Context, ws identity.Workspace, skillID pgtype.UUID, limit, offset int32,
) ([]TestCaseSummary, error) {
	if s.ReadSkill == nil {
		return nil, errRegistryReadNotConfigured
	}
	q := gen.New(s.Pool)
	var rows []gen.TestCase
	if skillID.Valid {
		all, err := q.ListTestCasesForSkill(ctx, gen.ListTestCasesForSkillParams{
			SkillID: skillID, WorkspaceID: ws.ID,
		})
		if err != nil {
			return nil, err
		}

		slices.Reverse(all)
		rows = page(all, limit, offset)
	} else {
		var err error
		if rows, err = q.ListTestCases(ctx, gen.ListTestCasesParams{
			WorkspaceID: ws.ID, Limit: limit, Offset: offset,
		}); err != nil {
			return nil, err
		}
	}

	names := make(map[pgtype.UUID]string, len(rows))
	out := make([]TestCaseSummary, 0, len(rows))
	for _, tc := range rows {
		name, seen := names[tc.SkillID]
		if !seen {

			if skill, found, err := s.ReadSkill(ctx, ws.ID, tc.SkillID); err == nil && found {
				name = skill.Name
			} else if err != nil {
				return nil, err
			}
			names[tc.SkillID] = name
		}
		out = append(out, TestCaseSummary{TestCase: tc, SkillName: name})
	}
	return out, nil
}

func page[T any](rows []T, limit, offset int32) []T {
	if offset > 0 {
		if int(offset) >= len(rows) {
			return nil
		}
		rows = rows[offset:]
	}
	if limit > 0 && int(limit) < len(rows) {
		rows = rows[:limit]
	}
	return rows
}

type Case struct {
	ID                 pgtype.UUID
	WorkspaceID        pgtype.UUID
	SkillID            pgtype.UUID
	Name               string
	UserPrompt         string
	AcceptanceCriteria []byte
	Rubric             []byte
}

func (s *Service) CasesForSkill(ctx context.Context, workspaceID, skillID pgtype.UUID) ([]Case, error) {
	if s == nil || s.Pool == nil {
		return nil, errPersistenceNotConfigured
	}
	rows, err := gen.New(s.Pool).ListTestCasesForSkill(ctx, gen.ListTestCasesForSkillParams{
		SkillID: skillID, WorkspaceID: workspaceID,
	})
	if err != nil {
		return nil, err
	}
	out := make([]Case, len(rows))
	for i, row := range rows {
		out[i] = Case{
			ID: row.ID, WorkspaceID: row.WorkspaceID, SkillID: row.SkillID,
			Name: row.Name, UserPrompt: row.UserPrompt,
			AcceptanceCriteria: row.AcceptanceCriteria, Rubric: row.Rubric,
		}
	}
	return out, nil
}

type Draft struct {
	TestCaseID pgtype.UUID
	SkillID    pgtype.UUID
	Name       string
	UserPrompt string
	Criteria   []Criterion

	Rubric   *Rubric
	Datasets []DatasetFile

	DatasetTotalBytes int64
}

type DatasetFile struct {
	DatasetID   string `json:"dataset_id"`
	FileName    string `json:"file_name"`
	ContentType string `json:"content_type"`
	SizeBytes   int64  `json:"size_bytes"`
	ContentHash string `json:"content_hash"`
}

func (s *Service) ReadDraft(ctx context.Context, workspaceID, testCaseID pgtype.UUID) (Draft, error) {
	if s == nil || s.Pool == nil {
		return Draft{}, errPersistenceNotConfigured
	}
	q := gen.New(s.Pool)
	tc, err := q.GetTestCase(ctx, gen.GetTestCaseParams{ID: testCaseID, WorkspaceID: workspaceID})
	if errors.Is(err, pgx.ErrNoRows) {
		return Draft{}, ErrNotFound
	}
	if err != nil {
		return Draft{}, err
	}
	return draftFromRow(ctx, q, workspaceID, tc)
}

func draftFromRow(ctx context.Context, q *gen.Queries, workspaceID pgtype.UUID, tc gen.TestCase) (Draft, error) {
	criteria, err := DecodeCriteria(tc.AcceptanceCriteria)
	if err != nil {
		return Draft{}, err
	}
	rubric, err := DecodeRubric(tc.Rubric)
	if err != nil {
		return Draft{}, err
	}
	rows, err := q.ListDatasets(ctx, gen.ListDatasetsParams{
		TestCaseID: tc.ID, WorkspaceID: workspaceID,
	})
	if err != nil {
		return Draft{}, err
	}
	draft := Draft{
		TestCaseID: tc.ID,
		SkillID:    tc.SkillID,
		Name:       tc.Name,
		UserPrompt: tc.UserPrompt,
		Criteria:   criteria,
		Rubric:     rubric,
		Datasets:   make([]DatasetFile, 0, len(rows)),
	}
	for _, d := range rows {
		draft.Datasets = append(draft.Datasets, DatasetFile{
			DatasetID:   pgconv.UUIDString(d.ID),
			FileName:    d.FileName,
			ContentType: d.ContentType,
			SizeBytes:   d.SizeBytes,
			ContentHash: d.ContentHash,
		})
		draft.DatasetTotalBytes += d.SizeBytes
	}
	return draft, nil
}

func (s *Service) UpdateTestCase(ctx context.Context, ws identity.Workspace, id pgtype.UUID, name, prompt string) (gen.TestCase, error) {
	name, prompt, err := validateDraft(name, prompt)
	if err != nil {
		return gen.TestCase{}, err
	}
	tc, err := gen.New(s.Pool).UpdateTestCase(ctx, gen.UpdateTestCaseParams{
		ID: id, WorkspaceID: ws.ID, Name: name, UserPrompt: prompt,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return gen.TestCase{}, ErrNotFound
	}
	return tc, err
}

func validateDraft(name, prompt string) (string, string, error) {
	name = strings.TrimSpace(name)
	prompt = strings.TrimSpace(prompt)
	switch {
	case name == "":
		return "", "", fmt.Errorf("%w: 名稱不能空白", ErrInvalid)
	case len(name) > MaxNameBytes:
		return "", "", fmt.Errorf("%w: 名稱最多 %d bytes", ErrInvalid, MaxNameBytes)

	case prompt == "":
		return "", "", fmt.Errorf("%w: Prompt 不能空白", ErrInvalid)
	case len(prompt) > MaxPromptBytes:
		return "", "", fmt.Errorf("%w: Prompt 最多 %d bytes", ErrInvalid, MaxPromptBytes)
	}
	return name, prompt, nil
}

type DeleteResult struct {
	DatasetsDeleted int
}

func (s *Service) DeleteTestCase(ctx context.Context, ws identity.Workspace, id pgtype.UUID) (DeleteResult, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return DeleteResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := gen.New(tx)

	tc, err := q.SoftDeleteTestCase(ctx, gen.SoftDeleteTestCaseParams{ID: id, WorkspaceID: ws.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return DeleteResult{}, ErrNotFound
	}
	if err != nil {
		return DeleteResult{}, err
	}
	removed, err := q.SoftDeleteDatasetsByTestCase(ctx, gen.SoftDeleteDatasetsByTestCaseParams{
		TestCaseID: tc.ID, WorkspaceID: ws.ID,
	})
	if err != nil {
		return DeleteResult{}, err
	}
	if err := audit.Log(ctx, tx, audit.Event{
		Actor:        ws.OwnerUserID,
		Workspace:    ws.ID,
		Action:       audit.ActionTestCaseDelete,
		ResourceType: audit.ResourceTestCase,
		ResourceID:   tc.ID,
		Metadata:     map[string]any{"datasets_deleted": len(removed)},
	}); err != nil {
		return DeleteResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return DeleteResult{}, err
	}

	for _, d := range removed {
		s.removeDatasetObject(ctx, d)
	}
	return DeleteResult{DatasetsDeleted: len(removed)}, nil
}

func (s *Service) AddCriterion(
	ctx context.Context, ws identity.Workspace, id pgtype.UUID, text, source string,
) (gen.TestCase, error) {
	text, err := validateCriterion(text)
	if err != nil {
		return gen.TestCase{}, err
	}
	if source != SourceSuggested {
		source = SourceUser
	}
	return s.mutateCriteria(ctx, ws, id, func(list []Criterion) ([]Criterion, error) {
		if len(list) >= MaxCriteria {
			return nil, fmt.Errorf("%w: 一個 Test Case 最多 %d 條驗收條件", ErrLimitExceeded, MaxCriteria)
		}
		return append(list, Criterion{ID: newCriterionID(), Text: text, Source: source}), nil
	})
}

func (s *Service) UpdateCriterion(ctx context.Context, ws identity.Workspace, id pgtype.UUID, criterionID string, text *string, confirmed *bool) (gen.TestCase, error) {
	var newText string
	if text != nil {
		var err error
		if newText, err = validateCriterion(*text); err != nil {
			return gen.TestCase{}, err
		}
	}
	return s.mutateCriteria(ctx, ws, id, func(list []Criterion) ([]Criterion, error) {
		i := indexOfCriterion(list, criterionID)
		if i < 0 {
			return nil, fmt.Errorf("%w: 找不到這條驗收條件", ErrNotFound)
		}
		if text != nil && newText != list[i].Text {
			list[i].Text = newText
			list[i].ConfirmedAt = nil

			list[i].Source = SourceUser
		}
		if confirmed != nil {
			if *confirmed {
				now := time.Now().UTC()
				list[i].ConfirmedAt = &now
			} else {
				list[i].ConfirmedAt = nil
			}
		}
		return list, nil
	})
}

func (s *Service) DeleteCriterion(ctx context.Context, ws identity.Workspace, id pgtype.UUID, criterionID string) (gen.TestCase, error) {
	return s.mutateCriteria(ctx, ws, id, func(list []Criterion) ([]Criterion, error) {
		i := indexOfCriterion(list, criterionID)
		if i < 0 {
			return nil, fmt.Errorf("%w: 找不到這條驗收條件", ErrNotFound)
		}
		return append(list[:i], list[i+1:]...), nil
	})
}

func (s *Service) mutateCriteria(ctx context.Context, ws identity.Workspace, id pgtype.UUID, fn func([]Criterion) ([]Criterion, error)) (gen.TestCase, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return gen.TestCase{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := gen.New(tx)

	tc, err := q.LockTestCase(ctx, gen.LockTestCaseParams{ID: id, WorkspaceID: ws.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return gen.TestCase{}, ErrNotFound
	}
	if err != nil {
		return gen.TestCase{}, err
	}
	list, err := DecodeCriteria(tc.AcceptanceCriteria)
	if err != nil {
		return gen.TestCase{}, err
	}
	if list, err = fn(list); err != nil {
		return gen.TestCase{}, err
	}
	encoded, err := json.Marshal(list)
	if err != nil {
		return gen.TestCase{}, err
	}
	updated, err := q.UpdateTestCaseCriteria(ctx, gen.UpdateTestCaseCriteriaParams{
		ID: tc.ID, WorkspaceID: ws.ID, AcceptanceCriteria: encoded,
	})
	if err != nil {
		return gen.TestCase{}, err
	}

	if updated, err = pruneRubric(ctx, q, ws, updated, list); err != nil {
		return gen.TestCase{}, err
	}
	return updated, tx.Commit(ctx)
}

func pruneRubric(ctx context.Context, q *gen.Queries, ws identity.Workspace, tc gen.TestCase, criteria []Criterion) (gen.TestCase, error) {
	rubric, err := DecodeRubric(tc.Rubric)
	if err != nil || rubric == nil {
		return tc, err
	}
	known := make(map[string]bool, len(criteria))
	for _, c := range criteria {
		known[c.ID] = true
	}
	kept := make([]RubricItem, 0, len(rubric.Items))
	for _, it := range rubric.Items {
		if known[it.ID] {
			kept = append(kept, it)
		}
	}
	if len(kept) == len(rubric.Items) {
		return tc, nil
	}
	var encoded []byte
	if len(kept) > 0 {
		rubric.Items = kept
		if encoded, err = json.Marshal(rubric); err != nil {
			return tc, err
		}
	}
	return q.UpdateTestCaseRubric(ctx, gen.UpdateTestCaseRubricParams{
		ID: tc.ID, WorkspaceID: ws.ID, Rubric: encoded,
	})
}

func DecodeCriteria(raw []byte) ([]Criterion, error) {
	list := []Criterion{}
	if len(raw) == 0 {
		return list, nil
	}
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, fmt.Errorf("decode acceptance criteria: %w", err)
	}
	return list, nil
}

func validateCriterion(text string) (string, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", fmt.Errorf("%w: 驗收條件的文字不能空白", ErrInvalid)
	}
	if len(text) > MaxCriterionBytes {
		return "", fmt.Errorf("%w: 驗收條件的文字最多 %d bytes", ErrInvalid, MaxCriterionBytes)
	}
	return text, nil
}

func indexOfCriterion(list []Criterion, id string) int {
	for i := range list {
		if list[i].ID == id {
			return i
		}
	}
	return -1
}

func newCriterionID() string { return pgconv.UUIDString(newUUID()) }

func humanMB(n int64) string { return fmt.Sprintf("%d MB", n>>20) }
