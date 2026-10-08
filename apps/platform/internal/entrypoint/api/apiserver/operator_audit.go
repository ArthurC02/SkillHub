package apiserver

import (
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	identity "github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/audit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/httpx"
)

var operatorActions = audit.PlatformFilter{
	Actions: []string{
		audit.ActionSkillRestrict,
		audit.ActionSkillUnrestrict,
		audit.ActionSkillRedistribution,
		audit.ActionSkillCuration,
		audit.ActionCreditGrant,
		audit.ActionDispatchHalt,
		audit.ActionDispatchResume,
		audit.ActionModelBudgetSet,
		audit.ActionExposureReview,
		audit.ActionAccountLookup,
		audit.ActionCreditLookup,
		audit.ActionAgentEnable,
		audit.ActionAgentDisable,
		audit.ActionAgentBrakeEngage,
		audit.ActionAgentBrakeRelease,
		audit.ActionFindingOpen,
		audit.ActionFindingReopen,
		audit.ActionFindingRecover,
		audit.ActionFindingAcknowledge,
		audit.ActionFindingResolve,
		audit.ActionFindingDismiss,
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
	ActorKind    audit.ActorKind `json:"actor_kind"`
	ActorUserID  *string         `json:"actor_user_id"`
	ActorAgentID *string         `json:"actor_agent_id"`
	Action       string          `json:"action"`
	ResourceType string          `json:"resource_type"`
	ResourceID   *string         `json:"resource_id"`
	WorkspaceID  *string         `json:"workspace_id"`
	OccurredAt   string          `json:"occurred_at"`
	Metadata     map[string]any  `json:"metadata"`
}

type operatorAuditResponse struct {
	Events     []operatorAuditEventView `json:"events"`
	NextBefore string                   `json:"next_before,omitempty"`
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
	query := r.URL.Query()
	var workspaceID pgtype.UUID
	if query.Has("workspace_id") {
		if err := workspaceID.Scan(query.Get("workspace_id")); err != nil || !workspaceID.Valid {
			httpx.WriteError(w, http.StatusBadRequest, "workspace_id must be a UUID")
			return
		}
	}
	beforeAt, beforeID, err := parseAuditCursor(query.Get("before"))
	if err != nil || (query.Has("before") && (beforeAt.IsZero() || query.Has("offset"))) {
		httpx.WriteError(w, http.StatusBadRequest, "before must be a cursor this endpoint returned and cannot be combined with offset")
		return
	}
	records, err := audit.ListPlatform(r.Context(), h.DB, operatorActions, audit.PlatformPage{
		Limit: limit + 1, Offset: offset, WorkspaceID: workspaceID, BeforeAt: beforeAt, BeforeID: beforeID,
	})
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "audit log lookup failed")
		return
	}
	response := operatorAuditResponse{}
	if len(records) > int(limit) {
		records = records[:limit]
		last := records[len(records)-1]
		response.NextBefore = auditCursor(last.OccurredAt, last.ID)
	}
	events := make([]operatorAuditEventView, 0, len(records))
	for _, rec := range records {
		events = append(events, operatorAuditEventView{
			ActorKind: rec.ActorKind(), ActorUserID: optionalUUID(rec.Actor), ActorAgentID: optionalUUID(rec.Agent),
			Action: rec.Action, ResourceType: rec.ResourceType,
			ResourceID: optionalUUID(rec.ResourceID), WorkspaceID: optionalUUID(rec.Workspace),
			OccurredAt: rec.OccurredAt.UTC().Format(time.RFC3339), Metadata: rec.Metadata,
		})
	}
	response.Events = events
	httpx.WriteJSON(w, http.StatusOK, response)
}

func parseAuditCursor(raw string) (time.Time, int64, error) {
	if raw == "" {
		return time.Time{}, 0, nil
	}
	at, rawID, _ := strings.Cut(raw, "_")
	t, timeErr := time.Parse(time.RFC3339Nano, at)
	id, idErr := strconv.ParseInt(rawID, 10, 64)
	if timeErr != nil || idErr != nil || id < 1 {
		return time.Time{}, 0, errors.New("before must be a cursor this endpoint returned")
	}
	return t, id, nil
}

func auditCursor(at time.Time, id int64) string {
	return at.UTC().Format(time.RFC3339Nano) + "_" + strconv.FormatInt(id, 10)
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

func sessionActorID(r *http.Request) (pgtype.UUID, bool) {
	user, ok := identity.SessionUser(r.Context())
	if !ok {
		return pgtype.UUID{}, false
	}
	return user.ID, true
}
