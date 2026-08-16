// Package testlab owns test cases, their acceptance criteria, their uploaded
// datasets, and the immutable snapshot a run executes (TEST-001/003/004/010,
// ADR-002 Test Lab).
//
// The split that matters here: a test case is an editable draft, a
// test_case_snapshots row is the frozen fact of what one run actually used
// (ADR-003, iron rule 4). Editing a draft never rewrites what a past run did.
//
// Nothing in this package executes uploaded content. Datasets are stored as
// opaque bytes and only ever unpacked inside the sandbox (iron rule 1).
package testlab

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/services/platform/internal/audit"
	"github.com/ArthurC02/skillhub/services/platform/internal/llmclient"
	"github.com/ArthurC02/skillhub/services/platform/internal/platform/db/gen"
)

// PDM-005 §5.1. These are the only enforcement point for the dataset limits;
// there is no second copy in the handler or the UI.
const (
	// MaxFileBytes caps one uploaded file.
	MaxFileBytes = 25 << 20 // 25 MiB
	// MaxTestCaseBytes caps the live total across one test case's files.
	MaxTestCaseBytes = 100 << 20 // 100 MiB
	// MaxFilesPerTestCase caps the live file count of one test case.
	MaxFilesPerTestCase = 20
	// DatasetRetention is the storage window shown before upload (PDM-005 §5.1,
	// PDM-006 §6). Users may delete earlier; nothing extends it.
	DatasetRetention = 90 * 24 * time.Hour
)

// Prompt and criteria caps. 02:TEST-001 fixes only "non-blank"; these bounds
// exist because an unbounded text column reachable from an unauthenticated-shaped
// request is a denial-of-service surface, not because a product rule asked.
//
// ponytail: flat byte caps. If a real prompt ever hits the ceiling, the number to
// revisit is MaxPromptBytes against the PDM-005 300K input-token budget, which is
// roughly an order of magnitude above this.
const (
	MaxNameBytes      = 200
	MaxPromptBytes    = 32 << 10 // 32 KiB
	MaxCriterionBytes = 2000
	MaxCriteria       = 50
)

var (
	// ErrNotFound: not visible in the caller's workspace. Deliberately the same
	// answer for "does not exist" and "belongs to someone else" (WS-006).
	ErrNotFound = errors.New("test case not found")
	// ErrInvalid: the request violates a documented input rule.
	ErrInvalid = errors.New("invalid request")
	// ErrLimitExceeded: a PDM-005 quota would be broken. Messages name the limit
	// that was hit and nothing about the system behind it (02:TEST-002).
	ErrLimitExceeded = errors.New("limit exceeded")
)

// ObjectStore is the slice of object storage the test lab needs. Remove is
// idempotent, which is what makes dataset deletion safe to retry (iron rule 9).
type ObjectStore interface {
	Put(ctx context.Context, key string, data []byte) error
	// Get reads a stored dataset back. Used only to read a delimited file's header
	// row for TEST-002; the bytes stay in this process (see suggest.go).
	Get(ctx context.Context, key string) ([]byte, error)
	Remove(ctx context.Context, key string) error
}

// CriteriaSuggester is the internal LLM service, as this package needs it
// (TEST-002). An interface rather than the concrete client so the test lab can be
// wired without one — a deployment with no LLM service answers "suggestions are
// unavailable" and the manual path is unaffected.
type CriteriaSuggester interface {
	SuggestCriteria(ctx context.Context, req llmclient.SuggestCriteriaRequest) (*llmclient.SuggestCriteriaResponse, error)
}

type Service struct {
	Pool  *pgxpool.Pool
	Store ObjectStore
	// LLM proposes acceptance criteria (TEST-002). Nil is a supported deployment.
	LLM CriteriaSuggester
}

// Criterion is one acceptance condition (TEST-003). Stored as an element of the
// test_cases.acceptance_criteria JSON array rather than in its own table: the
// snapshot column and evaluations.criterion_results already carry the same shape
// as JSON, so a table here would only add a join and a second representation.
type Criterion struct {
	ID   string `json:"id"`
	Text string `json:"text"`
	// Source is who proposed the condition: "user" for one the user typed,
	// "suggested" for one TEST-002 got from the model. EVAL-001 reads this to keep
	// a model's proposal from being presented as the user's own judgement.
	// Rewriting a suggested criterion's text turns it into the user's (see
	// UpdateCriterion): the wording is then theirs, and only the idea was proposed.
	Source string `json:"source"`
	// ConfirmedAt is the user's explicit agreement (TEST-003 確認). Nil means
	// proposed but not agreed to. Editing the text clears it: a confirmation
	// applies to the words that were confirmed, not to the criterion's identity.
	ConfirmedAt *time.Time `json:"confirmed_at"`
}

const (
	SourceUser      = "user"
	SourceSuggested = "suggested"
)

// Rubric is CONTENT-007's editable strengthening of the acceptance criteria
// (llm-internal.yaml Rubric, plus the version string). It is not a second
// mechanism and not a score sheet: its items map onto the same
// `criterion_results` entries the criteria produce, and nothing in the platform
// does arithmetic on `Weight` (evaluation-design §6.4, writing-rubrics.md §2.4).
//
// Version is stored with the items rather than beside them because it is a fact
// about this exact wording: changing any item's text, weight or evidence flag is
// a new rubric version and therefore another judge regression (02:EVAL-013
// clause 3). It is not sent to the judge — llm-internal's Rubric is
// additionalProperties:false and has no version field — it is what
// `evaluations.rubric_version` records.
type Rubric struct {
	Version string       `json:"version"`
	Items   []RubricItem `json:"items"`
}

// RubricItem strengthens exactly one acceptance criterion.
//
// ID is the criterion's id and not an id of its own. That is the whole data
// semantics of the feature: /judge-run answers one verdict per *criterion*, and
// Go drops any id it did not send, so an item whose id names no criterion has
// nowhere for its verdict to be stored and "per-item quoted evidence" would not
// exist in the data (writing-rubrics.md §2.1).
type RubricItem struct {
	ID   string `json:"id"`
	Text string `json:"text"`
	// Weight is the author's relative-importance signal for the model. Optional,
	// and a pointer so "no weight given" stays distinct from "weight 0".
	Weight *float64 `json:"weight,omitempty"`
	// EvidenceRequired is per item rather than a global switch: a rubric may
	// reasonably demand a quote for "cites its sources" and not for a criterion
	// about something being absent, which a quote cannot show (§2.3).
	EvidenceRequired bool `json:"evidence_required"`
}

// MaxRubricItems mirrors llm-internal.yaml Rubric.items maxItems.
const MaxRubricItems = 50

// MaxRubricVersionBytes bounds the version label. Same reasoning as the other
// caps here: a text column reachable from a request needs a ceiling.
const MaxRubricVersionBytes = 200

// SetRubric replaces the draft's rubric, or clears it when r is nil (TEST-012,
// CONTENT-007). Editing it never touches a past run: what a run was judged
// against is the copy frozen in its snapshot (iron rule 4).
func (s *Service) SetRubric(ctx context.Context, ws gen.Workspace, id pgtype.UUID, r *Rubric) (gen.TestCase, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return gen.TestCase{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := gen.New(tx)

	// Locked and re-read rather than validated against whatever the client last
	// saw: the ids an item may name are this test case's criteria *now*, and a
	// concurrent criterion deletion would otherwise let a dangling item through.
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

// validateRubric enforces the contract's bounds and the one rule the database
// cannot check: every item names a criterion of this same test case.
func validateRubric(r Rubric, criteria []Criterion) (Rubric, error) {
	r.Version = strings.TrimSpace(r.Version)
	switch {
	case r.Version == "":
		return Rubric{}, fmt.Errorf("%w: rubric version must not be blank", ErrInvalid)
	case len(r.Version) > MaxRubricVersionBytes:
		return Rubric{}, fmt.Errorf("%w: rubric version must be at most %d bytes", ErrInvalid, MaxRubricVersionBytes)
	case len(r.Items) == 0:
		// An empty rubric is not a rubric. Clearing one is done by sending null,
		// which is a statement the reader can tell apart from "there are items but
		// they all went missing".
		return Rubric{}, fmt.Errorf("%w: a rubric must have at least one item; send null to remove it", ErrInvalid)
	case len(r.Items) > MaxRubricItems:
		return Rubric{}, fmt.Errorf("%w: at most %d rubric items per test case", ErrLimitExceeded, MaxRubricItems)
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
				"%w: rubric item %q does not name an acceptance criterion of this test case; "+
					"an item's id is the criterion it strengthens", ErrInvalid, it.ID)
		case seen[it.ID]:
			return Rubric{}, fmt.Errorf("%w: two rubric items for criterion %q", ErrInvalid, it.ID)
		case it.Text == "":
			return Rubric{}, fmt.Errorf("%w: rubric item text must not be blank", ErrInvalid)
		case len(it.Text) > MaxCriterionBytes:
			return Rubric{}, fmt.Errorf("%w: rubric item text must be at most %d bytes", ErrInvalid, MaxCriterionBytes)
		}
		seen[it.ID] = true
		items = append(items, it)
	}
	r.Items = items
	return r, nil
}

// DecodeRubric reads the stored column. Exported so the run and evaluation
// domains read a rubric the same way this package writes one. A NULL column is
// no rubric, which is not the same as an empty one.
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

// CreateTestCase records a new draft against a skill in the caller's workspace
// (TEST-001). The skill is re-read under the workspace scope first: skill_id
// arrives from the client, and the foreign key alone would happily accept
// another workspace's skill (iron rule 3).
func (s *Service) CreateTestCase(ctx context.Context, ws gen.Workspace, skillID pgtype.UUID, name, prompt string) (gen.TestCase, error) {
	name, prompt, err := validateDraft(name, prompt)
	if err != nil {
		return gen.TestCase{}, err
	}
	q := gen.New(s.Pool)
	if _, err := q.GetSkill(ctx, gen.GetSkillParams{ID: skillID, WorkspaceID: ws.ID}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return gen.TestCase{}, ErrNotFound
		}
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

// GetTestCase reads one draft in the caller's workspace.
func (s *Service) GetTestCase(ctx context.Context, ws gen.Workspace, id pgtype.UUID) (gen.TestCase, error) {
	tc, err := gen.New(s.Pool).GetTestCase(ctx, gen.GetTestCaseParams{ID: id, WorkspaceID: ws.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return gen.TestCase{}, ErrNotFound
	}
	return tc, err
}

// ListTestCases returns the caller's drafts, newest first.
func (s *Service) ListTestCases(ctx context.Context, ws gen.Workspace) ([]gen.TestCase, error) {
	return gen.New(s.Pool).ListTestCases(ctx, gen.ListTestCasesParams{
		WorkspaceID: ws.ID, Limit: 100, Offset: 0,
	})
}

// UpdateTestCase edits the draft's name and prompt (TEST-001).
func (s *Service) UpdateTestCase(ctx context.Context, ws gen.Workspace, id pgtype.UUID, name, prompt string) (gen.TestCase, error) {
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
		return "", "", fmt.Errorf("%w: name must not be blank", ErrInvalid)
	case len(name) > MaxNameBytes:
		return "", "", fmt.Errorf("%w: name must be at most %d bytes", ErrInvalid, MaxNameBytes)
	// 02:TEST-001 "每個 Run 必須包含非空白 User Prompt" — enforced at the draft, so
	// a run can never be started from a test case that has no prompt.
	case prompt == "":
		return "", "", fmt.Errorf("%w: user_prompt must not be blank", ErrInvalid)
	case len(prompt) > MaxPromptBytes:
		return "", "", fmt.Errorf("%w: user_prompt must be at most %d bytes", ErrInvalid, MaxPromptBytes)
	}
	return name, prompt, nil
}

// DeleteResult states what the deletion covered (WS-002 "系統應說明刪除範圍").
type DeleteResult struct {
	DatasetsDeleted int
	// SnapshotsRetained is not a number here on purpose: snapshots are frozen by
	// the 0005 trigger and stay whatever they were. See the note in the handler.
}

// DeleteTestCase soft-deletes the draft and takes its live datasets with it,
// objects included (WS-002, CORE-007). Snapshots taken by past runs are
// untouched — they are the record of what those runs executed (ADR-003), and
// the dataset content hash inside them outlives the file itself.
//
// Objects are removed after the commit, not inside it: object storage is not
// transactional, and a failed removal leaves an orphan the retention sweep
// collects rather than a row that claims a file exists when it does not.
func (s *Service) DeleteTestCase(ctx context.Context, ws gen.Workspace, id pgtype.UUID) (DeleteResult, error) {
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
	if err := audit.Log(ctx, q, audit.Event{
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
		s.removeObject(ctx, d.ObjectKey)
	}
	return DeleteResult{DatasetsDeleted: len(removed)}, nil
}

// AddCriterion appends a user-authored acceptance condition (TEST-003).
func (s *Service) AddCriterion(ctx context.Context, ws gen.Workspace, id pgtype.UUID, text string) (gen.TestCase, error) {
	text, err := validateCriterion(text)
	if err != nil {
		return gen.TestCase{}, err
	}
	return s.mutateCriteria(ctx, ws, id, func(list []Criterion) ([]Criterion, error) {
		if len(list) >= MaxCriteria {
			return nil, fmt.Errorf("%w: at most %d acceptance criteria per test case", ErrLimitExceeded, MaxCriteria)
		}
		return append(list, Criterion{ID: newCriterionID(), Text: text, Source: SourceUser}), nil
	})
}

// UpdateCriterion edits and/or confirms one criterion (TEST-003). Both are the
// same statement because they are the same write: confirming is agreeing to the
// text as it stands, so a caller that changes the text and confirms in one
// request has confirmed the new text, and a caller that only changes the text
// has withdrawn the old confirmation.
func (s *Service) UpdateCriterion(ctx context.Context, ws gen.Workspace, id pgtype.UUID, criterionID string, text *string, confirmed *bool) (gen.TestCase, error) {
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
			return nil, fmt.Errorf("%w: acceptance criterion not found", ErrNotFound)
		}
		if text != nil && newText != list[i].Text {
			list[i].Text = newText
			list[i].ConfirmedAt = nil
			// The user rewrote it, so it is theirs now (TEST-002). Keeping
			// `suggested` on text the model never wrote would misattribute it in
			// the evaluation report.
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

// DeleteCriterion removes one criterion from the draft (TEST-003).
func (s *Service) DeleteCriterion(ctx context.Context, ws gen.Workspace, id pgtype.UUID, criterionID string) (gen.TestCase, error) {
	return s.mutateCriteria(ctx, ws, id, func(list []Criterion) ([]Criterion, error) {
		i := indexOfCriterion(list, criterionID)
		if i < 0 {
			return nil, fmt.Errorf("%w: acceptance criterion not found", ErrNotFound)
		}
		return append(list[:i], list[i+1:]...), nil
	})
}

// mutateCriteria applies fn to the stored criteria list under a row lock, so two
// concurrent edits to the same JSON array cannot silently drop one of them.
func (s *Service) mutateCriteria(ctx context.Context, ws gen.Workspace, id pgtype.UUID, fn func([]Criterion) ([]Criterion, error)) (gen.TestCase, error) {
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
	// A rubric item is addressed by the criterion it strengthens, so deleting a
	// criterion can orphan one. Pruned here, in the same transaction, rather than
	// left for the judge to drop: an item nothing will ever answer is not a rubric
	// the user can see and edit, and every criterion mutation routes through here.
	if updated, err = pruneRubric(ctx, q, ws, updated, list); err != nil {
		return gen.TestCase{}, err
	}
	return updated, tx.Commit(ctx)
}

// pruneRubric drops rubric items whose criterion no longer exists. A rubric left
// with nothing at all becomes NULL: an empty item list is not a rubric.
func pruneRubric(ctx context.Context, q *gen.Queries, ws gen.Workspace, tc gen.TestCase, criteria []Criterion) (gen.TestCase, error) {
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

// DecodeCriteria reads the stored JSON array. Exported so the run and evaluation
// domains read the criteria the same way this package writes them.
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
		return "", fmt.Errorf("%w: criterion text must not be blank", ErrInvalid)
	}
	if len(text) > MaxCriterionBytes {
		return "", fmt.Errorf("%w: criterion text must be at most %d bytes", ErrInvalid, MaxCriterionBytes)
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

func newCriterionID() string { return uuidString(newUUID()) }

// humanMB renders a byte cap the way the user-facing limit is written, so an
// error message says "25 MB" and not "26214400".
func humanMB(n int64) string { return fmt.Sprintf("%d MB", n>>20) }
