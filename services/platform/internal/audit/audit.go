// Package audit records the operations NFR-001 requires an audit trail for:
// import, execution, permission confirmation, download and deletion (CORE-008).
//
// This is not logging. slog carries operational detail, gets sampled, rotated
// and thrown away; an audit event is a durable row written inside the same
// transaction as the change it describes, so an operation that committed can
// never be missing from the trail and one that rolled back can never appear in
// it (iron rule 9).
//
// Events carry identifiers and outcome only — never package content, prompts,
// tokens or secrets (iron rule 11, NFR-002). That restraint is what makes the
// 400 day retention of PDM-006 §6 affordable and low-risk.
package audit

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/services/platform/internal/platform/db/gen"
)

// The audited vocabulary. Values are stable strings: they end up in stored rows
// that outlive any refactor of these constants.
const (
	ActionLogin              = "auth.login"
	ActionLogout             = "auth.logout"
	ActionSkillImport        = "skill.import"
	ActionSkillVersionCreate = "skill.version_create"
	ActionSkillFork          = "skill.fork"
	ActionSkillDelete        = "skill.delete"
	ActionSkillTakedown      = "skill.takedown"
	ActionAccountDeleteAsk   = "account.deletion_requested"
	ActionAccountDeleteStop  = "account.deletion_cancelled"
	ActionAccountPurge       = "account.purged"
	// NFR-001 requires an audit trail for execution. A run's state history lives
	// in run_status_transitions; these rows answer the different question of who
	// asked for it (transitions the worker makes carry no actor).
	ActionRunCreate     = "run.create"
	ActionRunTransition = "run.transition"
	ActionRunCancelAsk  = "run.cancel_requested"
)

// Resource types the actions above refer to.
const (
	ResourceSession = "session"
	ResourceSkill   = "skill"
	ResourceVersion = "skill_version"
	ResourceAccount = "account"
	ResourceRun     = "run"
)

// Event is one audited operation.
type Event struct {
	Actor        pgtype.UUID // zero = anonymous or platform-initiated
	Workspace    pgtype.UUID // zero when the operation has no workspace
	Action       string
	ResourceType string
	ResourceID   pgtype.UUID
	// Metadata holds identifiers and outcome flags. Never user content.
	Metadata map[string]any
}

// Log appends ev using q, which must be the caller's transaction handle so the
// event commits with the change it records (iron rule 9). Passing a pool-backed
// *gen.Queries is only correct for operations that are a single statement.
func Log(ctx context.Context, q *gen.Queries, ev Event) error {
	meta := []byte("{}")
	if len(ev.Metadata) > 0 {
		encoded, err := json.Marshal(ev.Metadata)
		if err != nil {
			return err
		}
		meta = encoded
	}
	return q.InsertAuditEvent(ctx, gen.InsertAuditEventParams{
		ActorUserID:  ev.Actor,
		WorkspaceID:  ev.Workspace,
		Action:       ev.Action,
		ResourceType: ev.ResourceType,
		ResourceID:   ev.ResourceID,
		Metadata:     meta,
	})
}
