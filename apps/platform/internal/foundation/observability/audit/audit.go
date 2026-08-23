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
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

// DBTX is the smallest database handle sqlc needs. A pgx transaction and a
// pgxpool.Pool both satisfy it, so callers preserve their existing atomicity
// without exporting generated Queries as Audit's transport.
type DBTX interface {
	Exec(context.Context, string, ...interface{}) (pgconn.CommandTag, error)
	Query(context.Context, string, ...interface{}) (pgx.Rows, error)
	QueryRow(context.Context, string, ...interface{}) pgx.Row
}

// The audited vocabulary. Values are stable strings: they end up in stored rows
// that outlive any refactor of these constants.
const (
	ActionLogin              = "auth.login"
	ActionLogout             = "auth.logout"
	ActionSkillImport        = "skill.import"
	ActionSkillVersionCreate = "skill.version_create"
	ActionSkillFork          = "skill.fork"
	// GEN-003 requires a generation that produced nothing to still leave the
	// workspace something it can look at. There is no matching success action:
	// an accepted generated package IS an import and is recorded as one, with
	// source_type = generated in its metadata — "every package this workspace
	// accepted" stays one history rather than two that have to be merged.
	ActionSkillGenerateFailed = "skill.generate_failed"
	ActionSkillDelete         = "skill.delete"
	ActionSkillTakedown       = "skill.takedown"
	// 02:SEC-011: the platform operator's two licensing-hold actions and the
	// roster that decides who may perform them. Split into set and clear because
	// "somebody lifted a hold" is the event a review actually looks for, and a
	// single action name would make it a metadata field to filter on.
	ActionSkillRestrict   = "skill.access_restrict"
	ActionSkillUnrestrict = "skill.access_unrestrict"
	// ActionSkillRedistribution records an operator changing the
	// redistribution verdict (0027/0036). One action for all directions,
	// unlike the restrict/unrestrict pair above: that column has two states
	// and reads naturally as a verb, this one has four and "set to blocked"
	// is not the opposite of "set to unknown".
	ActionSkillRedistribution = "skill.redistribution_set"
	// ActionOperatorRoster is written once per API start with the operator list
	// the process came up with. It is the minimum satisfaction of 02:SEC-011
	// 「授予或撤銷 operator 角色本身也是 audit event」 for a roster that lives in
	// deployment configuration: the grant is a config change plus a restart, and
	// this row is the only place that fact is durably recorded. What it cannot
	// give is who made the change or when they made it — that needs the role
	// table SEC-011 describes, and this is deliberately not it.
	ActionOperatorRoster = "operator.roster"
	// ActionBetaRoster is the same minimum for BETA-001's invite list (ADR-028
	// 決策 1 sends it to the ActionOperatorRoster precedent by name): written once
	// per API start with the cohort the process came up with, because the invite
	// itself is a configuration change plus a restart. Same limit as above — it
	// says who is on the list now, never who added them or when.
	ActionBetaRoster = "beta.roster"
	// ADR-052 left one question open by name — 「曝光旗標要不要有稽核事件」 — and
	// named its own weakness in the same paragraph: 「沒有任何機制會告訴我們它被誤
	// 開過」. This is that mechanism, in the shape the two rosters above already
	// use: written once per API start, recording the deployment's own
	// configuration rather than a grant. It answers "was this ever on, and from
	// when", which is the only question a mis-opened exposure flag raises after
	// the fact — 01 §11.2's first funnel segment has one chance with twelve
	// people, and nothing else would say it had been spent.
	ActionFeatureFlags      = "feature_flags.roster"
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
	// The other half of gate B. Until this existed the trail recorded every run
	// that started and none that was stopped, which is backwards: a run that ran
	// leaves a run row, a status history and an outbox event behind it, while a
	// refusal used to leave a Prometheus counter — an aggregate that names no
	// workspace, no version and no time, and is thrown away on the retention the
	// metrics store happens to have. "Who kept being told no, on what, and why"
	// is the question a security review actually opens the trail to ask.
	//
	// One action with a reason in metadata rather than one action per condition:
	// unlike the restrict/unrestrict pairs above, these are not different events a
	// review looks for separately — they are one event, "the gate said no", and the
	// condition is the detail. The reason strings are the metrics.RunRefused labels,
	// deliberately the same vocabulary so a spike on the dashboard and the rows
	// explaining it are searched with one word.
	ActionRunRefused = "run.refused"
	// RUN-007's teardown outcome. runs.cleanup_status carries the current answer
	// and is overwritten by the next attempt; this records each attempt that
	// concluded, so a sandbox that took three passes to release is still visible
	// afterwards. Platform-initiated, so actor-less — as with ActionObjectMissing,
	// it is a thing that happened to a user's resources without the user asking.
	ActionRunCleanup = "run.cleanup"
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
	// INGEST-010: an imported package's upstream source stopped resolving, or
	// started resolving again. Written only on the change, never on the repeat —
	// the sweep re-probes every source on a schedule, and a row per probe would
	// bury the two moments that matter under thousands that say nothing new.
	// Split into the two directions for the reason the licensing-hold pair is
	// split: "this source came back" is the event somebody looks for, not a
	// metadata field to filter on. Actor-less; the probe is platform-initiated.
	ActionSourceUnavailable = "import_source.unavailable"
	ActionSourceRestored    = "import_source.restored"
	// 02:SEC-010's P1 first action and ADR-022 X-04's drain/suspend, which are one
	// switch (03:SEC-012). Both triggers write both events: an operator declaring a
	// P1 carries an actor, the reconciler crossing a threshold carries none, and the
	// action names are the same because "is the platform dispatching" has one
	// history. Split into halted and resumed for the reason the licensing-hold pair
	// is split — "somebody resumed the fleet" is the event a review looks for, and
	// one action name would make it a metadata field to filter on.
	ActionDispatchHalt   = "dispatch.halted"
	ActionDispatchResume = "dispatch.resumed"
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
	// ResourceImportSource is the skill_sources row a package was imported from,
	// not the skill or version built out of it: availability is a property of the
	// upstream URL, and the immutable snapshot we hold stays valid either way.
	ResourceImportSource = "import_source"
	// ResourceOperatorRoster and ResourceBetaRoster have no resource_id: a roster
	// is the deployment's configuration, not a row anything can point at.
	ResourceOperatorRoster = "operator_roster"
	ResourceBetaRoster     = "beta_roster"
	// ResourceFeatureFlags has no resource_id for the same reason the two rosters
	// above have none: a deployment's flag set is configuration, not a row.
	ResourceFeatureFlags = "feature_flags"
	// ResourceDispatch is the execution plane's dispatch switch (0030). The
	// resource_id is the dispatch_halts row, so a halt and the resume that ended it
	// are two events pointing at one incident.
	ResourceDispatch = "dispatch"
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

// Log appends ev using db, which must be the caller's transaction handle so the
// event commits with the change it records (iron rule 9). Passing a pool is only
// correct for operations whose audit append is intentionally a single statement.
func Log(ctx context.Context, db DBTX, ev Event) error {
	if db == nil {
		return errors.New("audit: database handle is not configured")
	}
	meta := []byte("{}")
	if len(ev.Metadata) > 0 {
		encoded, err := json.Marshal(ev.Metadata)
		if err != nil {
			return err
		}
		meta = encoded
	}
	return gen.New(db).InsertAuditEvent(ctx, gen.InsertAuditEventParams{
		ActorUserID:  ev.Actor,
		WorkspaceID:  ev.Workspace,
		Action:       ev.Action,
		ResourceType: ev.ResourceType,
		ResourceID:   ev.ResourceID,
		Metadata:     meta,
	})
}

// Record is one stored row as a caller reads it back.
//
// Deliberately narrower than Event: no actor and no resource id, because the one
// question anything asks of this trail from a product screen is "what happened
// here and when". A screen that wanted to name the actor would be a different
// feature with a different authorisation question, and adding the field now
// would answer that question by accident.
type Record struct {
	Action     string
	OccurredAt time.Time
	Metadata   map[string]any
}

// ListForWorkspace answers "what happened in this workspace", newest first.
//
// This is the read half 02:GEN-003 asks for by name (「在工作區留下可查的失敗
// 紀錄」). It lives here rather than in the context that writes the rows because
// audit_events belongs to this package (db/query-owners.yaml): a caller does a
// Go function call into foundation, not a cross-context query.
//
// An empty actions slice returns nothing rather than everything. A caller that
// forgot to say what it wanted gets the answer that discloses least.
func ListForWorkspace(
	ctx context.Context, db DBTX, workspaceID pgtype.UUID, actions []string, limit int32,
) ([]Record, error) {
	if db == nil {
		return nil, errors.New("audit: database handle is not configured")
	}
	if len(actions) == 0 {
		return nil, nil
	}
	rows, err := gen.New(db).ListWorkspaceAuditEvents(ctx, gen.ListWorkspaceAuditEventsParams{
		WorkspaceID: workspaceID,
		Limit:       limit,
		Actions:     actions,
	})
	if err != nil {
		return nil, err
	}
	out := make([]Record, 0, len(rows))
	for _, r := range rows {
		rec := Record{Action: r.Action, OccurredAt: r.CreatedAt.Time}
		// A row whose metadata will not decode is still a row that happened, and
		// the timestamp is the part the screen needs most. Dropping the whole
		// record would turn a display defect into a missing history.
		if len(r.Metadata) > 0 {
			_ = json.Unmarshal(r.Metadata, &rec.Metadata)
		}
		out = append(out, rec)
	}
	return out, nil
}
