package apiserver

import (
	"fmt"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/audit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/httpx"
)

var operatorActions = audit.PlatformFilter{
	Actions: []string{
		audit.ActionSkillRestrict,
		audit.ActionSkillUnrestrict,
		audit.ActionSkillRedistribution,
		audit.ActionCreditGrant,
		audit.ActionDispatchHalt,
		audit.ActionDispatchResume,
		audit.ActionAccountLookup,
		audit.ActionCreditLookup,
	},
	ScopedActions: []string{audit.ActionSkillTakedown},
	Scope:         audit.ScopeOperator,
}

const (
	defaultAuditPageSize = 50
	maxAuditPageSize     = 100
)

type operatorAuditHandler struct {
	DB audit.DBTX
}

type operatorAuditEventView struct {
	ActorUserID  *string        `json:"actor_user_id"`
	Action       string         `json:"action"`
	ResourceType string         `json:"resource_type"`
	ResourceID   *string        `json:"resource_id"`
	WorkspaceID  *string        `json:"workspace_id"`
	OccurredAt   string         `json:"occurred_at"`
	Metadata     map[string]any `json:"metadata"`
}

func (h *operatorAuditHandler) List(w http.ResponseWriter, r *http.Request) {
	limit, err := queryInt32(r, "limit", defaultAuditPageSize, 1, maxAuditPageSize)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	offset, err := queryInt32(r, "offset", 0, 0, math.MaxInt32)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	records, err := audit.ListPlatform(r.Context(), h.DB, operatorActions, limit, offset)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "audit log lookup failed")
		return
	}
	events := make([]operatorAuditEventView, 0, len(records))
	for _, rec := range records {
		events = append(events, operatorAuditEventView{
			ActorUserID: optionalUUID(rec.Actor), Action: rec.Action, ResourceType: rec.ResourceType,
			ResourceID: optionalUUID(rec.ResourceID), WorkspaceID: optionalUUID(rec.Workspace),
			OccurredAt: rec.OccurredAt.UTC().Format(time.RFC3339), Metadata: rec.Metadata,
		})
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"events": events})
}

func queryInt32(r *http.Request, name string, fallback, low, high int64) (int32, error) {
	q := r.URL.Query()
	if !q.Has(name) {
		return int32(fallback), nil
	}
	n, err := strconv.ParseInt(q.Get(name), 10, 32)
	if err != nil || n < low || n > high {
		return 0, fmt.Errorf("query parameter %s must be a whole number between %d and %d", name, low, high)
	}
	return int32(n), nil
}

func optionalUUID(id pgtype.UUID) *string {
	if !id.Valid {
		return nil
	}
	s := pgconv.UUIDString(id)
	return &s
}
