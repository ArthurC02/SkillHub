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
	// 02:SEC-011: the platform operator's two licensing-hold actions and the
	// roster that decides who may perform them. Split into set and clear because
	// "somebody lifted a hold" is the event a review actually looks for, and a
	// single action name would make it a metadata field to filter on.
	ActionSkillRestrict   = "skill.access_restrict"
	ActionSkillUnrestrict = "skill.access_unrestrict"
	// ActionOperatorRoster is written once per API start with the operator list
	// the process came up with. It is the minimum satisfaction of 02:SEC-011
	// 「授予或撤銷 operator 角色本身也是 audit event」 for a roster that lives in
	// deployment configuration: the grant is a config change plus a restart, and
	// this row is the only place that fact is durably recorded. What it cannot
	// give is who made the change or when they made it — that needs the role
	// table SEC-011 describes, and this is deliberately not it.
	ActionOperatorRoster    = "operator.roster"
	ActionAccountDeleteAsk  = "account.deletion_requested"
	ActionAccountDeleteStop = "account.deletion_cancelled"
	ActionAccountPurge      = "account.purged"
	// NFR-001 requires an audit trail for execution. A run's state history lives
	// in run_status_transitions; these rows answer the different question of who
	// asked for it (transitions the worker makes carry no actor).
	ActionRunCreate     = "run.create"
	ActionRunTransition = "run.transition"
	ActionRunCancelAsk  = "run.cancel_requested"
	// SEC-002 gate B: who agreed to which pre-run permission summary, and when
	// (02:TEST-005). The confirmation row itself is the gate; this is the trail.
	ActionRunPermissionsConfirm = "run.permissions_confirmed"
	// NFR-001 requires an audit trail for deletion (CORE-007). Uploads are not
	// audited: the dataset row itself already records what was uploaded and when.
	ActionTestCaseDelete = "test_case.delete"
	ActionDatasetDelete  = "dataset.delete"
	// ADR-007 lists Artifact download and user data deletion by name, and they
	// were the last two operations on that list with nothing to record them —
	// M1 had neither a download nor a download artifact to delete (03:CORE-008).
	//
	// The download event is NOT the download record of WS-004, and neither
	// replaces the other. This row is a compliance record kept for 400 days that
	// stores identifiers only; that one is a product feature the owner reads and
	// it outlives the artifact's bytes. One table would bind each to the other's
	// stricter rule (packaging-design §7.2, ADR-029 decision 3).
	ActionArtifactDownload = "artifact.download"
	ActionArtifactDelete   = "artifact.delete"
	// Platform-initiated and therefore actor-less: the reconciler found the bytes
	// behind a live row missing and marked the row to stop claiming them
	// (04 丙-9). Audited rather than only logged, because it is user content
	// becoming unavailable without the user asking — the thing an audit trail
	// exists to make traceable, and slog is sampled and thrown away.
	ActionObjectMissing = "storage.object_missing"
)

// Resource types the actions above refer to.
const (
	ResourceSession  = "session"
	ResourceSkill    = "skill"
	ResourceVersion  = "skill_version"
	ResourceAccount  = "account"
	ResourceRun      = "run"
	ResourceTestCase = "test_case"
	ResourceDataset  = "dataset"
	ResourceArtifact = "artifact"
	// ResourceOperatorRoster has no resource_id: the roster is the deployment's
	// configuration, not a row anything can point at.
	ResourceOperatorRoster = "operator_roster"
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
