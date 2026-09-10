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

type DBTX interface {
	Exec(context.Context, string, ...interface{}) (pgconn.CommandTag, error)
	Query(context.Context, string, ...interface{}) (pgx.Rows, error)
	QueryRow(context.Context, string, ...interface{}) pgx.Row
}

const (
	ActionLogin              = "auth.login"
	ActionLogout             = "auth.logout"
	ActionSkillImport        = "skill.import"
	ActionSkillVersionCreate = "skill.version_create"
	ActionSkillFork          = "skill.fork"

	ActionSkillGenerateFailed = "skill.generate_failed"
	ActionSkillDelete         = "skill.delete"
	ActionSkillTakedown       = "skill.takedown"

	ActionSkillRestrict   = "skill.access_restrict"
	ActionSkillUnrestrict = "skill.access_unrestrict"

	ActionSkillRedistribution = "skill.redistribution_set"

	ActionOperatorRoster = "operator.roster"

	ActionBetaRoster = "beta.roster"

	ActionFeatureFlags      = "feature_flags.roster"
	ActionAccountDeleteAsk  = "account.deletion_requested"
	ActionAccountDeleteStop = "account.deletion_cancelled"
	ActionAccountPurge      = "account.purged"

	ActionRunCreate     = "run.create"
	ActionRunTransition = "run.transition"
	ActionRunCancelAsk  = "run.cancel_requested"

	ActionRunPermissionsConfirm = "run.permissions_confirmed"

	ActionRunRefused = "run.refused"

	ActionRunCleanup = "run.cleanup"

	ActionTestCaseDelete = "test_case.delete"
	ActionDatasetDelete  = "dataset.delete"

	ActionArtifactDownload = "artifact.download"
	ActionArtifactDelete   = "artifact.delete"

	ActionObjectMissing = "storage.object_missing"

	ActionSourceUnavailable = "import_source.unavailable"
	ActionSourceRestored    = "import_source.restored"

	ActionSourceChanged = "import_source.changed"

	ActionDispatchHalt   = "dispatch.halted"
	ActionDispatchResume = "dispatch.resumed"

	ActionOperatorRefused = "operator.refused"
)

const (
	ResourceSession  = "session"
	ResourceSkill    = "skill"
	ResourceVersion  = "skill_version"
	ResourceAccount  = "account"
	ResourceRun      = "run"
	ResourceTestCase = "test_case"
	ResourceDataset  = "dataset"
	ResourceArtifact = "artifact"

	ResourceImportSource = "import_source"

	ResourceOperatorRoster = "operator_roster"

	ResourceOperatorRoute = "operator_route"
	ResourceBetaRoster    = "beta_roster"

	ResourceFeatureFlags = "feature_flags"

	ResourceDispatch = "dispatch"
)

type Event struct {
	Actor        pgtype.UUID
	Workspace    pgtype.UUID
	Action       string
	ResourceType string
	ResourceID   pgtype.UUID

	Metadata map[string]any
}

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

type Record struct {
	Action     string
	OccurredAt time.Time
	Metadata   map[string]any
}

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

		if len(r.Metadata) > 0 {
			_ = json.Unmarshal(r.Metadata, &rec.Metadata)
		}
		out = append(out, rec)
	}
	return out, nil
}
