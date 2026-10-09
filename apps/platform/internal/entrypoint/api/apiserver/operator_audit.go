package apiserver

import (
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
		audit.ActionProposalPropose,
		audit.ActionProposalApprove,
		audit.ActionProposalReject,
		audit.ActionProposalExpire,
		audit.ActionProposalStart,
		audit.ActionProposalRequeue,
		audit.ActionProposalSucceed,
		audit.ActionProposalFail,
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
	NextCursor string                   `json:"next_cursor,omitempty"`
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
	var beforeAt time.Time
	var beforeID int64
	if values, present := r.URL.Query()["cursor"]; present {
		if len(values) != 1 || offset != 0 {
			httpx.WriteError(w, http.StatusBadRequest, "cursor cannot be repeated or combined with offset")
			return
		}
		beforeAt, beforeID, err = parseOperatorAuditCursor(values[0])
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	filter := operatorActions
	filter.BeforeAt, filter.BeforeID = beforeAt, beforeID
	records, err := audit.ListPlatform(r.Context(), h.DB, filter, limit+1, offset)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "audit log lookup failed")
		return
	}
	response := operatorAuditResponse{}
	if len(records) > int(limit) {
		records = records[:limit]
		response.NextCursor = operatorAuditCursor(records[len(records)-1])
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

func parseOperatorAuditCursor(raw string) (time.Time, int64, error) {
	at, rawID, found := strings.Cut(raw, "_")
	when, timeErr := time.Parse(time.RFC3339Nano, at)
	id, idErr := strconv.ParseInt(rawID, 10, 64)
	if !found || timeErr != nil || idErr != nil || id <= 0 {
		return time.Time{}, 0, fmt.Errorf("cursor is invalid")
	}
	return when, id, nil
}

func operatorAuditCursor(record audit.Record) string {
	return record.OccurredAt.UTC().Format(time.RFC3339Nano) + "_" + strconv.FormatInt(record.ID, 10)
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
