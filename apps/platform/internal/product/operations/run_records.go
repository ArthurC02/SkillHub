package operations

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/httpx"
)

const recentRunLimit = 50

type runView struct {
	ID            string          `json:"id"`
	Agent         string          `json:"agent"`
	Status        string          `json:"status"`
	Reason        string          `json:"reason,omitempty"`
	StartedAt     string          `json:"started_at"`
	FinishedAt    string          `json:"finished_at,omitempty"`
	LastStepAt    string          `json:"last_step_at,omitempty"`
	Result        json.RawMessage `json:"result,omitempty"`
	Steps         int32           `json:"steps"`
	UsdMicros     int64           `json:"usd_micros"`
	UnpricedSteps int32           `json:"unpriced_steps"`
}

type stepView struct {
	Seq              int32  `json:"seq"`
	Tool             string `json:"tool"`
	Arguments        string `json:"arguments"`
	Result           string `json:"result"`
	Model            string `json:"model"`
	PromptTokens     int64  `json:"prompt_tokens"`
	CompletionTokens int64  `json:"completion_tokens"`
	UsdMicros        *int64 `json:"usd_micros,omitempty"`
	CreatedAt        string `json:"created_at"`
}

func (s *Service) RecentRuns(ctx context.Context) ([]gen.ListPlatformAgentRunsRow, error) {
	return gen.New(s.Pool).ListPlatformAgentRuns(ctx, recentRunLimit)
}

func (s *Service) RunSteps(ctx context.Context, run pgtype.UUID) ([]gen.ListPlatformAgentStepsRow, error) {
	return gen.New(s.Pool).ListPlatformAgentSteps(ctx, run)
}

func (h *Handler) Runs(w http.ResponseWriter, r *http.Request) {
	rows, err := h.Svc.RecentRuns(r.Context())
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "agent run lookup failed")
		return
	}
	runs := make([]runView, len(rows))
	for i, row := range rows {
		runs[i] = runViewFromRow(row)
	}
	var total int64
	if len(rows) > 0 {
		total = rows[0].Total
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"runs": runs, "total": total})
}

func runViewFromRow(row gen.ListPlatformAgentRunsRow) runView {
	view := runView{
		ID: pgconv.UUIDString(row.ID), Agent: row.Agent, Status: row.Status, StartedAt: pgconv.RFC3339(row.StartedAt),
		FinishedAt: pgconv.RFC3339(row.FinishedAt), Result: row.Result,
		LastStepAt: pgconv.RFC3339(row.LastStepAt),
		Steps:      row.Steps, UsdMicros: row.UsdMicros, UnpricedSteps: row.UnpricedSteps,
	}
	if row.Reason != nil {
		view.Reason = *row.Reason
	}
	return view
}

func (h *Handler) Steps(w http.ResponseWriter, r *http.Request) {
	var run pgtype.UUID
	if err := run.Scan(r.PathValue("id")); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "run id must be a UUID")
		return
	}
	rows, err := h.Svc.RunSteps(r.Context(), run)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "agent step lookup failed")
		return
	}
	steps := make([]stepView, len(rows))
	for i, row := range rows {
		steps[i] = stepView{
			Seq: row.Seq, Tool: row.Tool, Arguments: row.Arguments, Result: row.Result, Model: row.Model,
			PromptTokens: row.PromptTokens, CompletionTokens: row.CompletionTokens, UsdMicros: row.UsdMicros,
			CreatedAt: pgconv.RFC3339(row.CreatedAt),
		}
	}
	httpx.WriteJSON(w, http.StatusOK, map[string][]stepView{"steps": steps})
}
