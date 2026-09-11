package run

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/audit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/metrics"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/ArthurC02/skillhub/apps/platform/internal/product/entitlements"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/design"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/evidence"
)

var (
	ErrNotFound = errors.New("run not found")

	ErrPreflightTargetNotFound = errors.New("找不到這個 Skill 版本或 Test Case")

	ErrRunFinished               = errors.New("run has already finished")
	errRegistryReadNotConfigured = errors.New("run: registry owner read is not configured")
	errRunLinkMissing            = errors.New("run: its skill version or test case snapshot is gone")
)

const artifactCleanupTimeout = 5 * time.Second

type SkillFacts struct {
	AccessRestriction *string
}

type VersionFacts struct {
	ID               pgtype.UUID
	SkillID          pgtype.UUID
	ContentHash      string
	PackageObjectKey string
}

type VersionSummary struct {
	SkillID   pgtype.UUID
	SkillName string
}

type RunSummary struct {
	gen.ListWorkspaceRunsRow
	SkillID    pgtype.UUID
	SkillName  string
	TestCaseID pgtype.UUID
}

type Linkage struct {
	SkillID    pgtype.UUID
	TestCaseID pgtype.UUID
}

type ContentSource struct {
	WorkspaceIsCatalog bool

	CurationTier string

	CuratedVersionIsThisOne bool
}

const providerUnassigned = "unassigned"

type Service struct {
	Pool *pgxpool.Pool

	TestLab *testlab.Service

	ReadSkill   func(context.Context, pgtype.UUID, pgtype.UUID) (SkillFacts, bool, error)
	ReadVersion func(context.Context, pgtype.UUID, pgtype.UUID) (VersionFacts, bool, error)

	ReadVersionSummaries func(context.Context, pgtype.UUID, []pgtype.UUID) (map[pgtype.UUID]VersionSummary, error)

	ReadContentSource func(context.Context, pgtype.UUID, pgtype.UUID) (ContentSource, bool, error)

	Credits func(usd float64) (credits int64, ok bool)

	CreditReserve func(ctx context.Context, tx pgx.Tx, workspaceID pgtype.UUID, reservedUSDMicros int64) (ok bool, err error)

	CreditSettle func(ctx context.Context, tx pgx.Tx, workspaceID, runID pgtype.UUID, usdMicros *int64, reservedUSDMicros int64) error

	WorkspaceCreatedAt func(context.Context, pgtype.UUID) (time.Time, error)

	ActiveArtifactReferences func(ctx context.Context, db gen.DBTX, objectKey string) (int64, error)

	Queue *river.Client[pgx.Tx]

	Providers *Registry

	Store ObjectStore

	MaxAttempts int

	PollInterval time.Duration

	TraceSigner *trace.Signer

	Trace *trace.Service

	MaskerCanary func() []string

	TraceIngestBaseURL string

	Quota policy.QuotaLimits

	Gateway *Gateway
}

func (s *Service) requireTestLab() error {
	if s.TestLab == nil {
		return errors.New("run: test lab service not injected")
	}
	return nil
}

func (s *Service) requireRunLinks() error {
	if s.ReadVersionSummaries == nil {
		return errRegistryReadNotConfigured
	}
	return s.requireTestLab()
}

func (s *Service) maxAttempts() int {
	if s.MaxAttempts > 0 {
		return s.MaxAttempts
	}
	return defaultMaxAttempts
}

func (s *Service) pollInterval() time.Duration {
	if s.PollInterval > 0 {
		return s.PollInterval
	}
	return defaultPollInterval
}

func (s *Service) providers() *Registry {
	if s.Providers == nil {
		return &Registry{}
	}
	return s.Providers
}

func (s *Service) queries() *gen.Queries { return gen.New(s.Pool) }

type ResourceLimits struct {
	VCPU                 float64 `json:"vcpu"`
	MemoryBytes          int64   `json:"memory_bytes"`
	DiskBytes            int64   `json:"disk_bytes"`
	MaxPIDs              int     `json:"max_pids"`
	MaxOpenFiles         int     `json:"max_open_files"`
	WallClockSoftSeconds int     `json:"wall_clock_soft_seconds"`
	WallClockHardSeconds int     `json:"wall_clock_hard_seconds"`
	ArtifactTotalBytes   int64   `json:"artifact_total_bytes"`
	ArtifactFileBytes    int64   `json:"artifact_file_bytes"`
	TokenBudget          struct {
		MaxInputTokens  int `json:"max_input_tokens"`
		MaxOutputTokens int `json:"max_output_tokens"`
	} `json:"token_budget"`
}

func DefaultResourceLimits() ResourceLimits {
	l := ResourceLimits{
		VCPU:                 2,
		MemoryBytes:          4 << 30,
		DiskBytes:            8 << 30,
		MaxPIDs:              256,
		MaxOpenFiles:         1024,
		WallClockSoftSeconds: 600,
		WallClockHardSeconds: 900,
		ArtifactTotalBytes:   100 << 20,
		ArtifactFileBytes:    25 << 20,
	}

	l.TokenBudget.MaxInputTokens = 300_000
	l.TokenBudget.MaxOutputTokens = 60_000
	return l
}

type policySnapshot struct {
	ResourceLimits ResourceLimits `json:"resource_limits"`
	Egress         EgressPolicy   `json:"egress"`
}

func defaultPolicy() policySnapshot {
	allow := []egressAllow{}
	if url := GatewayURL(); url != "" {
		allow = append(allow, egressAllow{Purpose: "model_gateway", URL: url})
	}
	return policySnapshot{
		ResourceLimits: DefaultResourceLimits(),
		Egress:         EgressPolicy{Mode: "default_deny", Allow: allow},
	}
}

func defaultPolicySnapshot() ([]byte, error) { return json.Marshal(defaultPolicy()) }

type CreateParams struct {
	WorkspaceID pgtype.UUID
	Actor       pgtype.UUID
	SkillID     pgtype.UUID
	VersionID   pgtype.UUID
	TestCaseID  pgtype.UUID

	ConfirmedSummaryHash string
}

func (s *Service) Create(ctx context.Context, p CreateParams) (gen.Run, error) {
	if err := s.requireTestLab(); err != nil {
		return gen.Run{}, err
	}
	run, err := s.create(ctx, p)
	if err != nil {
		s.auditRefusal(ctx, p, err)
	}
	return run, err
}

func (s *Service) auditRefusal(ctx context.Context, p CreateParams, err error) {
	var reason string
	r, isRefusal := errors.AsType[refusal](err)
	switch {
	case isRefusal:
		reason = r.reason

	case errors.Is(err, ErrPermissionsNotConfirmed):
		reason = "permissions_unconfirmed"
	case errors.Is(err, ErrNoCompatibleProvider):
		reason = "capability_mismatch"
	default:
		return
	}

	if logErr := audit.Log(ctx, s.Pool, audit.Event{
		Actor:        p.Actor,
		Workspace:    p.WorkspaceID,
		Action:       audit.ActionRunRefused,
		ResourceType: audit.ResourceVersion,
		ResourceID:   p.VersionID,
		Metadata:     map[string]any{"reason": reason},
	}); logErr != nil {
		slog.Error("recording a refused run failed", "reason", reason, "error", logErr)
	}
}

func (s *Service) create(ctx context.Context, p CreateParams) (gen.Run, error) {
	if s.ReadVersion == nil || s.ReadSkill == nil {
		return gen.Run{}, errRegistryReadNotConfigured
	}

	if err := s.requireDispatchable(ctx); err != nil {
		return gen.Run{}, err
	}

	version, found, err := s.ReadVersion(ctx, p.WorkspaceID, p.VersionID)
	if !found && err == nil {

		return gen.Run{}, ErrPreflightTargetNotFound
	}
	if err != nil {
		return gen.Run{}, err
	}

	if version.SkillID != p.SkillID {
		return gen.Run{}, ErrNotFound
	}

	skill, found, err := s.ReadSkill(ctx, p.WorkspaceID, p.SkillID)
	if !found && err == nil {
		return gen.Run{}, ErrNotFound
	}
	if err != nil {
		return gen.Run{}, err
	}
	if err := s.requireNotAccessRestricted(skill); err != nil {
		return gen.Run{}, err
	}

	policy, err := defaultPolicySnapshot()
	if err != nil {
		return gen.Run{}, err
	}

	var decoded policySnapshot
	if err := json.Unmarshal(policy, &decoded); err != nil {
		return gen.Run{}, err
	}
	if err := s.checkSchedulable(ctx, decoded); err != nil {
		return gen.Run{}, err
	}

	if err := s.requireScanNotBlocking(ctx, version.PackageObjectKey); err != nil {
		return gen.Run{}, err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return gen.Run{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.queries().WithTx(tx)

	if err := s.requireRunSlot(ctx, q, p.WorkspaceID); err != nil {
		return gen.Run{}, err
	}

	if err := s.requireQuota(ctx, tx, p.WorkspaceID); err != nil {
		return gen.Run{}, err
	}

	if err := s.requireCredit(ctx, tx, p.WorkspaceID); err != nil {
		return gen.Run{}, err
	}

	testCase, err := s.TestLab.LockDraft(ctx, tx, p.WorkspaceID, p.TestCaseID)
	if errors.Is(err, testlab.ErrNotFound) {
		return gen.Run{}, ErrNotFound
	}
	if err != nil {
		return gen.Run{}, err
	}
	if testCase.SkillID != p.SkillID {
		return gen.Run{}, ErrNotFound
	}
	if err := s.requirePermissionConfirmation(ctx, q, p, testCase, version); err != nil {
		return gen.Run{}, err
	}

	snapshot, err := s.TestLab.CreateSnapshot(ctx, tx, p.WorkspaceID, p.TestCaseID)
	if errors.Is(err, testlab.ErrNotFound) {
		return gen.Run{}, ErrNotFound
	}
	if err != nil {
		return gen.Run{}, err
	}

	run, err := q.CreateRun(ctx, gen.CreateRunParams{
		WorkspaceID:        p.WorkspaceID,
		SkillVersionID:     version.ID,
		TestCaseSnapshotID: snapshot.ID,
		Provider:           providerUnassigned,

		RuntimeSnapshot: []byte("{}"),
		PolicySnapshot:  policy,
	})
	if err != nil {
		return gen.Run{}, err
	}

	if err := s.record(ctx, q, tx, run, nil, pgtype.UUID{}, "已收到這次 Run 的請求", p.Actor, audit.ActionRunCreate); err != nil {
		return gen.Run{}, err
	}

	if s.Queue != nil {

		if _, err := s.Queue.InsertTx(ctx, tx, JobArgs{
			RunID:       pgconv.UUIDString(run.ID),
			WorkspaceID: pgconv.UUIDString(run.WorkspaceID),
		}, executeInsertOpts()); err != nil {
			return gen.Run{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return gen.Run{}, err
	}
	metrics.RunCreated.Inc()
	return run, nil
}

func (s *Service) Get(ctx context.Context, workspaceID, runID pgtype.UUID) (gen.Run, error) {
	run, err := s.queries().GetRun(ctx, gen.GetRunParams{ID: runID, WorkspaceID: workspaceID})
	if errors.Is(err, pgx.ErrNoRows) {
		return gen.Run{}, ErrNotFound
	}
	return run, err
}

func (s *Service) List(
	ctx context.Context, workspaceID, testCaseID pgtype.UUID, limit, offset int32,
) ([]RunSummary, error) {
	if err := s.requireRunLinks(); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > maxRunPageSize {
		limit = defaultRunPageSize
	}
	if offset < 0 {
		offset = 0
	}
	var snapshotIDs []pgtype.UUID
	if testCaseID.Valid {
		ids, err := s.TestLab.SnapshotIDsForTestCase(ctx, workspaceID, testCaseID)
		if err != nil {
			return nil, err
		}
		if len(ids) == 0 {
			return []RunSummary{}, nil
		}
		snapshotIDs = ids
	}
	rows, err := s.queries().ListWorkspaceRuns(ctx, gen.ListWorkspaceRunsParams{
		WorkspaceID: workspaceID, SnapshotIds: snapshotIDs, PageSize: limit, PageOffset: offset,
	})
	if err != nil {
		return nil, err
	}
	versionIDs := make([]pgtype.UUID, len(rows))
	caseSnapshotIDs := make([]pgtype.UUID, len(rows))
	for i, row := range rows {
		versionIDs[i], caseSnapshotIDs[i] = row.SkillVersionID, row.TestCaseSnapshotID
	}
	versions, testCases, err := s.runLinks(ctx, workspaceID, versionIDs, caseSnapshotIDs)
	if err != nil {
		return nil, err
	}
	summaries := make([]RunSummary, len(rows))
	for i, row := range rows {
		version, versionFound := versions[row.SkillVersionID]
		testCaseID, caseFound := testCases[row.TestCaseSnapshotID]
		if !versionFound || !caseFound {
			return nil, fmt.Errorf("%w: run %s", errRunLinkMissing, pgconv.UUIDString(row.ID))
		}
		summaries[i] = RunSummary{
			ListWorkspaceRunsRow: row, SkillID: version.SkillID, SkillName: version.SkillName, TestCaseID: testCaseID,
		}
	}
	return summaries, nil
}

func (s *Service) runLinks(
	ctx context.Context, workspaceID pgtype.UUID, versionIDs, snapshotIDs []pgtype.UUID,
) (map[pgtype.UUID]VersionSummary, map[pgtype.UUID]pgtype.UUID, error) {
	versions, err := s.ReadVersionSummaries(ctx, workspaceID, versionIDs)
	if err != nil {
		return nil, nil, err
	}
	testCases, err := s.TestLab.SnapshotTestCases(ctx, workspaceID, snapshotIDs)
	if err != nil {
		return nil, nil, err
	}
	return versions, testCases, nil
}

const (
	defaultRunPageSize = 50
	maxRunPageSize     = 200
)

func (s *Service) Artifacts(
	ctx context.Context, workspaceID, runID pgtype.UUID,
) ([]gen.Artifact, bool, error) {

	run, err := s.Get(ctx, workspaceID, runID)
	if err != nil {
		return nil, false, err
	}
	rows, err := s.queries().ListRunArtifacts(ctx, gen.ListRunArtifactsParams{
		RunID: runID, WorkspaceID: workspaceID,
	})
	if err != nil {
		return nil, false, err
	}
	return rows, run.ArtifactsTruncated, nil
}

func (s *Service) DeleteArtifact(
	ctx context.Context, ws identity.Workspace, runID, artifactID pgtype.UUID,
) error {
	if s.ActiveArtifactReferences == nil {
		return errors.New("run: artifact reference counter not injected; refusing delete")
	}
	lookup, err := gen.New(s.Pool).GetRunArtifactForDelete(ctx, gen.GetRunArtifactForDeleteParams{
		ArtifactID: artifactID, RunID: runID, WorkspaceID: ws.ID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	conn, err := s.Pool.Acquire(ctx)
	if err != nil {
		return err
	}
	lockKey := "artifact-object:" + lookup.ObjectKey
	locked := false
	defer func() {
		if !locked {
			conn.Release()
			return
		}
		unlockCtx, cancel := context.WithTimeout(context.Background(), artifactCleanupTimeout)
		defer cancel()
		if _, err := gen.New(conn).UnlockRunArtifactObjectSession(unlockCtx, lockKey); err != nil {
			slog.Error("run artifact object lock could not be released; closing connection", "error", err)
			_ = conn.Hijack().Close(context.Background())
			return
		}
		conn.Release()
	}()
	if err := gen.New(conn).LockRunArtifactObjectSession(ctx, lockKey); err != nil {
		return err
	}
	locked = true
	tx, err := conn.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := gen.New(tx)

	row, err := q.SoftDeleteRunArtifact(ctx, gen.SoftDeleteRunArtifactParams{
		ArtifactID: artifactID, RunID: runID, WorkspaceID: ws.ID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := audit.Log(ctx, tx, audit.Event{
		Actor: ws.OwnerUserID, Workspace: ws.ID,
		Action: audit.ActionArtifactDelete, ResourceType: audit.ResourceArtifact,
		ResourceID: row.ID,
		Metadata:   map[string]any{"run_id": pgconv.UUIDString(runID)},
	}); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	if row.PurgedAt.Valid || s.Store == nil {
		return nil
	}

	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), artifactCleanupTimeout)
	defer cancel()
	shared, err := s.ActiveArtifactReferences(cleanupCtx, conn, row.ObjectKey)
	if err == nil && shared == 0 {
		err = s.Store.Remove(cleanupCtx, row.ObjectKey)
	}
	if err == nil {
		err = gen.New(conn).MarkRunOutputPurged(cleanupCtx, row.ID)
	}
	if err != nil {
		slog.Warn("run artifact object not removed; cleanup will retry",
			"object_key", row.ObjectKey, "error", err)
	}
	return nil
}

func (s *Service) Linkage(ctx context.Context, workspaceID, runID pgtype.UUID) (Linkage, error) {
	if err := s.requireRunLinks(); err != nil {
		return Linkage{}, err
	}
	row, err := s.queries().GetRunLinkage(ctx, gen.GetRunLinkageParams{
		RunID: runID, WorkspaceID: workspaceID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Linkage{}, ErrNotFound
	}
	if err != nil {
		return Linkage{}, err
	}
	versions, testCases, err := s.runLinks(ctx, workspaceID,
		[]pgtype.UUID{row.SkillVersionID}, []pgtype.UUID{row.TestCaseSnapshotID})
	if err != nil {
		return Linkage{}, err
	}
	version, versionFound := versions[row.SkillVersionID]
	testCaseID, caseFound := testCases[row.TestCaseSnapshotID]
	if !versionFound || !caseFound {
		return Linkage{}, fmt.Errorf("%w: run %s", errRunLinkMissing, pgconv.UUIDString(runID))
	}
	return Linkage{SkillID: version.SkillID, TestCaseID: testCaseID}, nil
}

func (s *Service) History(ctx context.Context, workspaceID, runID pgtype.UUID) ([]gen.RunStatusTransition, error) {
	return s.queries().ListRunStatusTransitions(ctx, gen.ListRunStatusTransitionsParams{
		RunID: runID, WorkspaceID: workspaceID,
	})
}

func (s *Service) Attempts(ctx context.Context, workspaceID, runID pgtype.UUID) ([]gen.RunAttempt, error) {
	return s.queries().ListRunAttempts(ctx, gen.ListRunAttemptsParams{
		RunID: runID, WorkspaceID: workspaceID,
	})
}

func (s *Service) RequestCancel(ctx context.Context, workspaceID, runID, actor pgtype.UUID) (gen.Run, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return gen.Run{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.queries().WithTx(tx)

	run, err := q.RequestRunCancel(ctx, gen.RequestRunCancelParams{ID: runID, WorkspaceID: workspaceID})
	if errors.Is(err, pgx.ErrNoRows) {

		if _, err := s.Get(ctx, workspaceID, runID); err != nil {
			return gen.Run{}, err
		}
		return gen.Run{}, ErrRunFinished
	}
	if err != nil {
		return gen.Run{}, err
	}

	if err := audit.Log(ctx, tx, audit.Event{
		Actor: actor, Workspace: run.WorkspaceID, Action: audit.ActionRunCancelAsk,
		ResourceType: audit.ResourceRun, ResourceID: run.ID,
		Metadata: map[string]any{"status_at_request": string(run.Status)},
	}); err != nil {
		return gen.Run{}, err
	}
	return run, tx.Commit(ctx)
}

func nowUTC() string { return time.Now().UTC().Format(time.RFC3339) }
