package creation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	identity "github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"math"
	"os"
	"time"
)

var (
	ErrNotFound       = errors.New("creation: session unavailable")
	ErrConflict       = errors.New("creation: stale revision")
	ErrReplayMismatch = errors.New("creation: command reused")
	ErrInvalidCommand = errors.New("creation: invalid command")
	ErrLimit          = errors.New("creation: limit reached")
	ErrUnavailable    = errors.New("creation: capability unavailable")
	// ErrDeadline: the session's wall clock (Limits.SessionTimeout) ran out. Kept
	// apart from ErrLimit so the API can say "time", not "budget".
	ErrDeadline = errors.New("creation: session deadline passed")
	// ErrBudgetOutOfBand: Create was asked for a budget outside
	// [MaxCallCostUSD, MaxCostUSD]; the API names the band in its reply.
	ErrBudgetOutOfBand = errors.New("creation: budget outside the permitted band")
)

const (
	// MaxMessages is the transcript ceiling both the pre-call gate and the
	// proposal check use. llm-internal.yaml caps messages at 100; a step may
	// append two (assistant + tool), so the gate must leave room for both.
	MaxMessages = 98
	// MaxTextRunes mirrors llm-internal.yaml's maxLength on message, brief,
	// diagram_understanding and draft_validation.report. A truncation marker
	// must fit INSIDE it, not after it.
	MaxTextRunes = 20000
	// MaxDiagramBytes is the decoded size cap for an uploaded flow diagram,
	// enforced at the command and again when the bytes reach the worker.
	MaxDiagramBytes = 4_000_000 // one-number: creationMaxDiagramBytes
	// MaxAcceptanceCriteria mirrors llm-internal.yaml's maxItems on
	// acceptance_criteria; each item is capped at MaxCriterionRunes there too.
	MaxAcceptanceCriteria = 12
	MaxCriterionRunes     = 500
)

type Limits struct {
	MaxCostUSD      float64       `json:"max_cost_usd"`
	MaxCallCostUSD  float64       `json:"max_call_cost_usd"`
	MaxSteps        int           `json:"max_steps"`
	MaxToolCalls    int           `json:"max_tool_calls"`
	CallTimeout     time.Duration `json:"call_timeout"`
	SessionTimeout  time.Duration `json:"session_timeout"`
	Retention       time.Duration `json:"retention"`
	MaxOutputTokens int           `json:"max_output_tokens"`
}

func (l Limits) Valid() bool {
	return finite(l.MaxCostUSD) && finite(l.MaxCallCostUSD) && l.MaxCostUSD > 0 && l.MaxCallCostUSD > 0 && l.MaxCallCostUSD <= l.MaxCostUSD && l.MaxSteps > 0 && l.MaxToolCalls > 0 && l.CallTimeout >= time.Second && l.CallTimeout <= 120*time.Second && l.SessionTimeout >= l.CallTimeout && l.Retention >= l.SessionTimeout && l.MaxOutputTokens > 0 && l.MaxOutputTokens <= 16000
}
func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
func LimitsFromEnv() (Limits, error) {
	var v struct {
		MaxCostUSD      float64 `json:"max_cost_usd"`
		MaxCallCostUSD  float64 `json:"max_call_cost_usd"`
		MaxSteps        int     `json:"max_steps"`
		MaxToolCalls    int     `json:"max_tool_calls"`
		CallTimeout     int64   `json:"call_timeout_seconds"`
		SessionTimeout  int64   `json:"session_timeout_seconds"`
		Retention       int64   `json:"retention_seconds"`
		MaxOutputTokens int     `json:"max_output_tokens"`
	}
	if err := json.Unmarshal([]byte(os.Getenv("CREATION_LIMITS_JSON")), &v); err != nil {
		return Limits{}, ErrUnavailable
	}
	for _, n := range []int64{v.CallTimeout, v.SessionTimeout, v.Retention} {
		if n <= 0 || n > math.MaxInt64/int64(time.Second) {
			return Limits{}, ErrUnavailable
		}
	}
	l := Limits{v.MaxCostUSD, v.MaxCallCostUSD, v.MaxSteps, v.MaxToolCalls, time.Duration(v.CallTimeout) * time.Second, time.Duration(v.SessionTimeout) * time.Second, time.Duration(v.Retention) * time.Second, v.MaxOutputTokens}
	if !l.Valid() {
		return Limits{}, ErrUnavailable
	}
	return l, nil
}
func Exposed() bool { return os.Getenv("CREATION_EXPOSED") == "on" }

type Reference struct {
	SkillID       string `json:"skill_id"`
	VersionID     string `json:"version_id"`
	Name          string `json:"name"`
	Confirmed     bool   `json:"confirmed"`
	Available     bool   `json:"available"`
	Description   string `json:"description,omitempty"`
	Compatibility string `json:"compatibility,omitempty"`
	AllowedTools  string `json:"allowed_tools,omitempty"`
}
type Draft struct {
	Revision    int64                    `json:"revision"`
	ContentHash string                   `json:"content_hash"`
	Skill       llmclient.GeneratedSkill `json:"skill"`
	Validation  string                   `json:"validation"`
	Blocked     bool                     `json:"blocked"`
}
type Candidate struct {
	SkillID   string `json:"skill_id"`
	VersionID string `json:"version_id"`
	RunID     string `json:"run_id,omitempty"`
	// TestCaseID is the Test Case Go created from the confirmed acceptance
	// criteria when the candidate was materialized (05 R-46 (b)); the trial run
	// that feeds the review phase is expected to use it.
	TestCaseID string `json:"test_case_id,omitempty"`
}
type Snapshot struct {
	Messages []llmclient.CreationMessage `json:"messages"`
	Brief    string                      `json:"brief"`
	// AcceptanceCriteria are proposed by the model together with the brief and
	// confirmed with it (confirm_brief binds both). Observable sentences, not
	// prose inside the brief: at materialize they become a Test Case.
	AcceptanceCriteria   []string    `json:"acceptance_criteria"`
	BriefConfirmed       bool        `json:"brief_confirmed"`
	DiagramUnderstanding string      `json:"diagram_understanding"`
	DiagramConfirmed     bool        `json:"diagram_confirmed"`
	References           []Reference `json:"references"`
	PendingAction        string      `json:"pending_action"`
	BudgetUSD            float64     `json:"budget_usd"`
	ReservedUSD          float64     `json:"reserved_usd"`
	SpentUSD             *float64    `json:"spent_usd,omitempty"`
	UsageUnknown         bool        `json:"usage_unknown"`
	Steps                int         `json:"steps"`
	ToolCalls            int         `json:"tool_calls"`
	Draft                *Draft      `json:"draft,omitempty"`
	PreviousDraft        *Draft      `json:"previous_draft,omitempty"`
	Candidate            *Candidate  `json:"candidate,omitempty"`
	DiagramFingerprint   string      `json:"diagram_fingerprint,omitempty"`
	DiagramMediaType     string      `json:"diagram_media_type,omitempty"`
	DiagramBytes         int         `json:"diagram_bytes,omitempty"`
	Model                string      `json:"model,omitempty"`
	PromptVersion        string      `json:"prompt_version,omitempty"`
}
type envelope struct {
	Snapshot        Snapshot    `json:"snapshot"`
	Limits          Limits      `json:"limits"`
	StartHash       string      `json:"start_hash"`
	Deadline        time.Time   `json:"deadline"`
	ActiveReceipt   pgtype.UUID `json:"active_receipt"`
	ActiveDeadline  time.Time   `json:"active_deadline"`
	ExistingSkillID string      `json:"existing_skill_id,omitempty"`
	PreviousDraft   *Draft      `json:"previous_draft,omitempty"`
}
type View struct {
	ID        string    `json:"id"`
	Revision  int64     `json:"revision"`
	State     string    `json:"state"`
	Snapshot  Snapshot  `json:"snapshot"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	ExpiresAt time.Time `json:"expires_at"`
	// Deadline is the session-timeout clock (distinct from ExpiresAt, the
	// retention clock); after it every command except cancel is refused.
	Deadline time.Time `json:"deadline"`
}
type Provenance struct {
	Brief, Model, PromptVersion, ExistingSkillID string
	Inputs                                       []byte
}
type Service struct {
	Pool   *pgxpool.Pool
	Limits Limits
	LLM    interface {
		CreationStep(context.Context, llmclient.CreationStepRequest) (*llmclient.CreationStepResponse, error)
	}
	Insert           func(context.Context, pgx.Tx, JobArgs) error
	ResolveReference func(context.Context, identity.Workspace, string, string) (Reference, llmclient.GenerateReference, error)
	SearchReferences func(context.Context, identity.Workspace, string) ([]Reference, error)
	ValidateDraft    func(context.Context, llmclient.GeneratedSkill) (string, string, bool, error)
	Materialize      func(context.Context, identity.Workspace, llmclient.GeneratedSkill, Provenance, func(context.Context, pgx.Tx, Candidate) error) error
	ReadRun          func(context.Context, identity.Workspace, string, Candidate) (string, error)
	// CreateAcceptanceTestCase writes the confirmed acceptance criteria as a Test Case of
	// the candidate skill, inside the materialize transaction; returns its id.
	CreateAcceptanceTestCase func(ctx context.Context, tx pgx.Tx, ws identity.Workspace, skillID, name, prompt string, criteria []string) (string, error)
	IssueKey                 func(context.Context, string, string, float64, time.Duration) (string, error)
	RevokeKey                func(context.Context, string) error
}

func digest(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func UUID(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	return fmt.Sprintf("%x-%x-%x-%x-%x", id.Bytes[:4], id.Bytes[4:6], id.Bytes[6:8], id.Bytes[8:10], id.Bytes[10:])
}
func ParseID(v string) (pgtype.UUID, error) {
	var id pgtype.UUID
	err := id.Scan(v)
	if err != nil || !id.Valid {
		return id, ErrInvalidCommand
	}
	return id, nil
}
func decode(row gen.CreationSession) (envelope, error) {
	var e envelope
	err := json.Unmarshal(row.Snapshot, &e)
	return e, err
}
func view(row gen.CreationSession) (View, error) {
	e, err := decode(row)
	return View{UUID(row.ID), row.Revision, row.State, e.Snapshot, row.CreatedAt.Time, row.UpdatedAt.Time, row.ExpiresAt.Time, e.Deadline}, err
}
func live(row gen.CreationSession) bool { return row.ExpiresAt.Time.After(time.Now()) }
func terminal(state string) bool        { return state == "saved" || state == "cancelled" }
func (s *Service) Get(ctx context.Context, ws identity.Workspace, id pgtype.UUID) (View, error) {
	row, err := gen.New(s.Pool).GetCreationSession(ctx, gen.GetCreationSessionParams{ID: id, WorkspaceID: ws.ID})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !live(row)) {
		return View{}, ErrNotFound
	}
	if err != nil {
		return View{}, err
	}
	return view(row)
}
func (s *Service) List(ctx context.Context, ws identity.Workspace) ([]View, error) {
	rows, err := gen.New(s.Pool).ListCreationSessions(ctx, ws.ID)
	if err != nil {
		return nil, err
	}
	out := make([]View, 0, len(rows))
	for _, r := range rows {
		v, err := view(r)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}
func advance(ctx context.Context, tx pgx.Tx, row gen.CreationSession, state, event string, e envelope) (gen.CreationSession, error) {
	b, err := json.Marshal(e)
	if err != nil {
		return row, err
	}
	q := gen.New(tx)
	r, err := q.AdvanceCreationSession(ctx, gen.AdvanceCreationSessionParams{ID: row.ID, WorkspaceID: row.WorkspaceID, ExpectedRevision: row.Revision, State: state, Snapshot: b})
	if errors.Is(err, pgx.ErrNoRows) {
		return r, ErrConflict
	}
	if err != nil {
		return r, err
	}
	err = q.AppendCreationEvent(ctx, gen.AppendCreationEventParams{SessionID: r.ID, WorkspaceID: r.WorkspaceID, Revision: r.Revision, EventType: event, Snapshot: b})
	return r, err
}
func (*Service) PurgeWorkspace(ctx context.Context, tx pgx.Tx, ws pgtype.UUID) error {
	return gen.New(tx).PurgeCreationWorkspace(ctx, ws)
}
