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
	ActionSkillCuration       = "skill.curation_set"

	ActionOperatorRoster = "operator.roster"

	ActionBetaRoster = "beta.roster"

	ActionFeatureFlags      = "feature_flags.roster"
	ActionAccountDeleteAsk  = "account.deletion_requested"
	ActionAccountDeleteStop = "account.deletion_cancelled"
	ActionAccountPurge      = "account.purged"

	ActionEventDeadLettered = "event.dead_lettered"

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

	ActionModelBudgetSet        = "model_budget.set"
	ActionEvaluationSettingsSet = "evaluation_settings.set"
	ActionRunSettingsSet        = "run_settings.set"

	ActionCreditGrant   = "credit.grant"
	ActionAccountLookup = "account.lookup"
	ActionCreditLookup  = "credit.lookup"

	ActionPublisherRegister   = "publisher.register"
	ActionPublicationRelease  = "publication.release"
	ActionPublicationDelist   = "publication.delist"
	ActionBundleVersionCreate = "bundle.version.create"
	ActionExposureReview      = "publication.exposure.review"

	ActionAgentEnable       = "platform_agent.enabled"
	ActionAgentDisable      = "platform_agent.disabled"
	ActionAgentBrakeEngage  = "platform_agent.brake_engaged"
	ActionAgentBrakeRelease = "platform_agent.brake_released"
	ActionAgentSpendCapSet  = "platform_agent.spend_cap_set"

	ActionFindingOpen        = "platform_agent_finding.opened"
	ActionFindingReopen      = "platform_agent_finding.reopened"
	ActionFindingRecover     = "platform_agent_finding.recovered"
	ActionFindingAcknowledge = "platform_agent_finding.acknowledged"
	ActionFindingResolve     = "platform_agent_finding.resolved"
	ActionFindingDismiss     = "platform_agent_finding.dismissed"
	ActionProposalPropose    = "platform_agent_proposal.proposed"
	ActionProposalApprove    = "platform_agent_proposal.approved"
	ActionProposalReject     = "platform_agent_proposal.rejected"
	ActionProposalExpire     = "platform_agent_proposal.expired"
	ActionProposalStart      = "platform_agent_proposal.started"
	ActionProposalRequeue    = "platform_agent_proposal.requeued"
	ActionProposalSucceed    = "platform_agent_proposal.succeeded"
	ActionProposalFail       = "platform_agent_proposal.failed"
)

const ScopeOperator = "operator"

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

	ResourceModelBudget        = "model_budget"
	ResourceEvaluationSettings = "evaluation_settings"
	ResourceRunSettings        = "run_settings"

	ResourceCreditAccount = "credit_account"

	ResourcePublisher   = "publisher"
	ResourcePublication = "publication"
	ResourceBundle      = "bundle"

	ResourceDomainEvent = "domain_event"

	ResourcePlatformAgent         = "platform_agent"
	ResourcePlatformAgentFinding  = "platform_agent_finding"
	ResourcePlatformAgentProposal = "platform_agent_proposal"
)

type ActorKind string

const (
	ActorPerson ActorKind = "person"
	ActorAgent  ActorKind = "agent"
	ActorSystem ActorKind = "system"
)

type Event struct {
	Actor        pgtype.UUID
	Agent        pgtype.UUID
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
		ActorAgentID: ev.Agent,
		WorkspaceID:  ev.Workspace,
		Action:       ev.Action,
		ResourceType: ev.ResourceType,
		ResourceID:   ev.ResourceID,
		Metadata:     meta,
	})
}

type Record struct {
	Actor        pgtype.UUID
	Agent        pgtype.UUID
	Workspace    pgtype.UUID
	Action       string
	ResourceType string
	ResourceID   pgtype.UUID
	OccurredAt   time.Time
	Metadata     map[string]any
}

func (r Record) ActorKind() ActorKind {
	switch {
	case r.Agent.Valid:
		return ActorAgent
	case r.Actor.Valid:
		return ActorPerson
	default:
		return ActorSystem
	}
}

type PlatformFilter struct {
	Actions []string

	ScopedActions []string
	Scope         string
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
	return records(rows), nil
}

func ListPlatform(ctx context.Context, db DBTX, filter PlatformFilter, limit, offset int32) ([]Record, error) {
	if db == nil {
		return nil, errors.New("audit: database handle is not configured")
	}
	rows, err := gen.New(db).ListPlatformAuditEvents(ctx, gen.ListPlatformAuditEventsParams{
		Actions:       filter.Actions,
		ScopedActions: filter.ScopedActions,
		Scope:         filter.Scope,
		PageLimit:     limit,
		PageOffset:    offset,
	})
	if err != nil {
		return nil, err
	}
	return records(rows), nil
}

func records(rows []gen.AuditEvent) []Record {
	out := make([]Record, 0, len(rows))
	for _, r := range rows {
		rec := Record{
			Actor: r.ActorUserID, Agent: r.ActorAgentID, Workspace: r.WorkspaceID, Action: r.Action,
			ResourceType: r.ResourceType, ResourceID: r.ResourceID, OccurredAt: r.CreatedAt.Time,
		}
		if len(r.Metadata) > 0 {
			_ = json.Unmarshal(r.Metadata, &rec.Metadata)
		}
		out = append(out, rec)
	}
	return out
}

type DailyCount struct {
	Day    time.Time
	Action string
	Count  int64
}

func DailyPlatform(ctx context.Context, db DBTX, filter PlatformFilter, since time.Time) ([]DailyCount, error) {
	if db == nil {
		return nil, errors.New("audit: database handle is not configured")
	}
	rows, err := gen.New(db).CountPlatformAuditEventsByDay(ctx, gen.CountPlatformAuditEventsByDayParams{
		Since:   pgtype.Timestamptz{Time: since, Valid: true},
		Actions: filter.Actions, ScopedActions: filter.ScopedActions, Scope: filter.Scope,
	})
	if err != nil {
		return nil, err
	}
	out := make([]DailyCount, 0, len(rows))
	for _, r := range rows {
		out = append(out, DailyCount{Day: r.Day.Time, Action: r.Action, Count: r.Events})
	}
	return out, nil
}
