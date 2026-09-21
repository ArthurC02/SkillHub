package run

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

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

	ErrPreflightTargetNotFound = errors.New("run: no such skill version or test case")

	ErrRunFinished               = errors.New("run has already finished")
	errRegistryReadNotConfigured = errors.New("run: registry owner read is not configured")
	errRunLinkMissing            = errors.New("run: its skill version or test case snapshot is gone")
)

type SkillFacts struct {
	AccessRestricted        bool
	AccessRestrictionReason string
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
	ID             pgtype.UUID
	Status         string
	StatusReason   string
	Provider       string
	FailureClass   string
	CleanupStatus  string
	SkillID        pgtype.UUID
	SkillName      string
	SkillVersionID pgtype.UUID
	TestCaseID     pgtype.UUID
	CreatedAt      *time.Time
	StartedAt      *time.Time
	FinishedAt     *time.Time
}

type RunView struct {
	ID                 pgtype.UUID
	Status             string
	StatusReason       string
	SkillVersionID     pgtype.UUID
	TestCaseSnapshotID pgtype.UUID
	Provider           string
	FailureClass       string
	CleanupStatus      string
	CancelRequestedAt  *time.Time
	CreatedAt          *time.Time
	StartedAt          *time.Time
	FinishedAt         *time.Time
	ArtifactsTruncated bool
}

type Linkage struct {
	SkillID    pgtype.UUID
	TestCaseID pgtype.UUID
}

type Artifact struct {
	ID          pgtype.UUID
	FileName    string
	ContentType string
	SizeBytes   int64
	ContentHash string
	CreatedAt   *time.Time
	ExpiresAt   *time.Time
	Purged      bool
}

type StatusTransition struct {
	From       string
	To         string
	Reason     string
	OccurredAt *time.Time
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

	ReadSkill   func(ctx context.Context, workspaceID, skillID pgtype.UUID) (SkillFacts, bool, error)
	ReadVersion func(ctx context.Context, workspaceID, versionID pgtype.UUID) (VersionFacts, bool, error)

	ReadVersionSummaries func(context.Context, pgtype.UUID, []pgtype.UUID) (map[pgtype.UUID]VersionSummary, error)

	ReadContentSource func(ctx context.Context, workspaceID, versionID pgtype.UUID) (ContentSource, bool, error)

	Credits func(usd float64) (credits int64, ok bool)

	CreditReserve func(ctx context.Context, tx pgx.Tx, workspaceID pgtype.UUID, reservedUSD float64) (ok bool, err error)

	CreditSettle func(ctx context.Context, tx pgx.Tx, workspaceID, runID pgtype.UUID, costUSD *float64, reservedUSD float64) error

	WorkspaceCreatedAt func(context.Context, pgtype.UUID) (time.Time, error)

	ActiveArtifactReferences func(ctx context.Context, db gen.DBTX, objectKey string) (int64, error)

	ClearSightings func(ctx context.Context, tx pgx.Tx, ids []pgtype.UUID) error

	Queue RunQueue

	Providers *Registry

	Store ObjectStore

	MaxAttempts int

	PollInterval time.Duration

	SlotWaitInterval time.Duration

	Now func() time.Time

	LastOrphanScan func(context.Context) (time.Time, bool, error)

	TraceSigner *trace.Signer

	Trace *trace.Service

	MaskerCanary func() []string

	TraceIngestBaseURL string

	Quota policy.QuotaLimits

	Gateway ModelGateway

	Deployment Deployment
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

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func (s *Service) slotWaitInterval() time.Duration {
	if s.SlotWaitInterval > 0 {
		return s.SlotWaitInterval
	}
	return defaultSlotWaitInterval
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
	ResourceLimits   ResourceLimits    `json:"resource_limits"`
	Egress           EgressPolicy      `json:"egress"`
	Model            string            `json:"model,omitempty"`
	MinimumIsolation IsolationStrength `json:"minimum_isolation"`
	CleanMode        bool              `json:"clean_mode"`
}

func (p policySnapshot) reachesAModel() bool {
	return slices.ContainsFunc(p.Egress.Allow, func(a egressAllow) bool {
		return a.Purpose == modelGatewayPurpose
	})
}

func defaultPolicy(deployment Deployment) policySnapshot {
	allow := []egressAllow{}
	if url := deployment.GatewayURL; url != "" {
		allow = append(allow, egressAllow{Purpose: modelGatewayPurpose, URL: url})
	}
	return policySnapshot{
		ResourceLimits:   DefaultResourceLimits(),
		Egress:           EgressPolicy{Mode: "default_deny", Allow: allow},
		Model:            deployment.Model,
		MinimumIsolation: deployment.RequiredIsolation(),
		CleanMode:        deployment.CleanMode,
	}
}

func defaultPolicySnapshot(deployment Deployment) ([]byte, error) {
	return json.Marshal(defaultPolicy(deployment))
}

type CreateParams struct {
	WorkspaceID pgtype.UUID
	Actor       pgtype.UUID
	SkillID     pgtype.UUID
	VersionID   pgtype.UUID
	TestCaseID  pgtype.UUID

	ConfirmedSummaryHash string
}

func (s *Service) Create(ctx context.Context, p CreateParams) (RunView, error) {
	if err := s.requireTestLab(); err != nil {
		return RunView{}, err
	}
	run, err := s.create(ctx, p)
	if err != nil {
		s.auditRefusal(ctx, p, err)
	}
	return runView(run), err
}

func (s *Service) auditRefusal(ctx context.Context, p CreateParams, err error) {
	r, isRefusal := errors.AsType[refusal](err)
	if !isRefusal {
		return
	}

	if logErr := audit.Log(ctx, s.Pool, audit.Event{
		Actor:        p.Actor,
		Workspace:    p.WorkspaceID,
		Action:       audit.ActionRunRefused,
		ResourceType: audit.ResourceVersion,
		ResourceID:   p.VersionID,
		Metadata:     map[string]any{"reason": r.reason},
	}); logErr != nil {
		slog.Error("recording a refused run failed", "reason", r.reason, "error", logErr)
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

	policy, err := defaultPolicySnapshot(s.Deployment)
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

	testCase, err := testlab.LockDraft(ctx, tx, p.WorkspaceID, p.TestCaseID)
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

	requested := startRun(gen.Run{
		WorkspaceID:        p.WorkspaceID,
		SkillVersionID:     version.ID,
		TestCaseSnapshotID: snapshot.ID,
		Provider:           providerUnassigned,

		RuntimeSnapshot: []byte("{}"),
		PolicySnapshot:  policy,
	})
	if err := s.saveRun(ctx, tx, requested, p.Actor); err != nil {
		return gen.Run{}, err
	}
	run := requested.Row()

	if s.Queue != nil {
		if err := s.Queue.DriveInTx(ctx, tx, run); err != nil {
			return gen.Run{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return gen.Run{}, err
	}
	metrics.RunCreated.Inc()
	return run, nil
}

func (s *Service) Get(ctx context.Context, workspaceID, runID pgtype.UUID) (RunView, error) {
	run, err := s.load(ctx, workspaceID, runID)
	return runView(run), err
}

func (s *Service) load(ctx context.Context, workspaceID, runID pgtype.UUID) (gen.Run, error) {
	run, err := s.queries().GetRun(ctx, gen.GetRunParams{ID: runID, WorkspaceID: workspaceID})
	if errors.Is(err, pgx.ErrNoRows) {
		return gen.Run{}, ErrNotFound
	}
	return run, err
}

func runView(row gen.Run) RunView {
	return RunView{
		ID:                 row.ID,
		Status:             string(row.Status),
		StatusReason:       deref(row.StatusReason),
		SkillVersionID:     row.SkillVersionID,
		TestCaseSnapshotID: row.TestCaseSnapshotID,
		Provider:           row.Provider,
		FailureClass:       deref(row.FailureClass),
		CleanupStatus:      string(row.CleanupStatus),
		CancelRequestedAt:  timePointer(row.CancelRequestedAt),
		CreatedAt:          timePointer(row.CreatedAt),
		StartedAt:          timePointer(row.StartedAt),
		FinishedAt:         timePointer(row.FinishedAt),
		ArtifactsTruncated: row.ArtifactsTruncated,
	}
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
		summaries[i] = runSummary(row, version, testCaseID)
	}
	return summaries, nil
}

func runSummary(row gen.ListWorkspaceRunsRow, version VersionSummary, testCaseID pgtype.UUID) RunSummary {
	return RunSummary{
		ID:             row.ID,
		Status:         string(row.Status),
		StatusReason:   deref(row.StatusReason),
		Provider:       row.Provider,
		FailureClass:   deref(row.FailureClass),
		CleanupStatus:  string(row.CleanupStatus),
		SkillID:        version.SkillID,
		SkillName:      version.SkillName,
		SkillVersionID: row.SkillVersionID,
		TestCaseID:     testCaseID,
		CreatedAt:      timePointer(row.CreatedAt),
		StartedAt:      timePointer(row.StartedAt),
		FinishedAt:     timePointer(row.FinishedAt),
	}
}

func timePointer(ts pgtype.Timestamptz) *time.Time {
	if !ts.Valid {
		return nil
	}
	t := ts.Time
	return &t
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
) ([]Artifact, bool, error) {

	run, err := s.load(ctx, workspaceID, runID)
	if err != nil {
		return nil, false, err
	}
	rows, err := s.queries().ListRunArtifacts(ctx, gen.ListRunArtifactsParams{
		RunID: runID, WorkspaceID: workspaceID,
	})
	if err != nil {
		return nil, false, err
	}
	artifacts := make([]Artifact, len(rows))
	for i, row := range rows {
		artifacts[i] = artifact(row)
	}
	return artifacts, run.ArtifactsTruncated, nil
}

func artifact(row gen.Artifact) Artifact {
	return Artifact{
		ID:          row.ID,
		FileName:    row.FileName,
		ContentType: row.ContentType,
		SizeBytes:   row.SizeBytes,
		ContentHash: row.ContentHash,
		CreatedAt:   timePointer(row.CreatedAt),
		ExpiresAt:   timePointer(row.ExpiresAt),
		Purged:      row.PurgedAt.Valid,
	}
}

func (s *Service) DeleteArtifact(
	ctx context.Context, ws identity.Workspace, runID, artifactID pgtype.UUID,
) error {
	return artifactLifecycle{
		pool: s.Pool, store: s.Store,
		activeReferences: s.ActiveArtifactReferences,
		markPurged:       s.MarkRunOutputPurged,
	}.Delete(ctx, ws, runID, artifactID)
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

func (s *Service) History(ctx context.Context, workspaceID, runID pgtype.UUID) ([]StatusTransition, error) {
	rows, err := s.queries().ListRunStatusTransitions(ctx, gen.ListRunStatusTransitionsParams{
		RunID: runID, WorkspaceID: workspaceID,
	})
	if err != nil {
		return nil, err
	}
	transitions := make([]StatusTransition, len(rows))
	for i, row := range rows {
		transitions[i] = StatusTransition{
			From: derefStatus(row.FromStatus), To: string(row.ToStatus), Reason: deref(row.Reason), OccurredAt: timePointer(row.OccurredAt),
		}
	}
	return transitions, nil
}

func derefStatus(status *gen.RunStatus) string {
	if status == nil {
		return ""
	}
	return string(*status)
}

func (s *Service) Attempts(ctx context.Context, workspaceID, runID pgtype.UUID) ([]gen.RunAttempt, error) {
	return s.queries().ListRunAttempts(ctx, gen.ListRunAttemptsParams{
		RunID: runID, WorkspaceID: workspaceID,
	})
}

func (s *Service) RequestCancel(ctx context.Context, workspaceID, runID, actor pgtype.UUID) (RunView, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return RunView{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	r, err := loadRun(ctx, s.queries().WithTx(tx), workspaceID, runID)
	if errors.Is(err, pgx.ErrNoRows) {
		return RunView{}, ErrNotFound
	}
	if err != nil {
		return RunView{}, err
	}
	r.RequestCancel()
	if err := s.saveRun(ctx, tx, r, actor); err != nil {
		return RunView{}, err
	}
	run := r.Row()

	if err := audit.Log(ctx, tx, audit.Event{
		Actor: actor, Workspace: run.WorkspaceID, Action: audit.ActionRunCancelAsk,
		ResourceType: audit.ResourceRun, ResourceID: run.ID,
		Metadata: map[string]any{"status_at_request": string(run.Status)},
	}); err != nil {
		return RunView{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return RunView{}, err
	}
	return runView(run), nil
}

func nowUTC() string { return time.Now().UTC().Format(time.RFC3339) }

type RunsOnDay struct {
	Day    time.Time
	Status string
	Runs   int64
}

func (s *Service) DailyRuns(ctx context.Context, since time.Time) ([]RunsOnDay, error) {
	rows, err := s.queries().CountRunsByDay(ctx, pgtype.Timestamptz{Time: since, Valid: true})
	if err != nil {
		return nil, err
	}
	out := make([]RunsOnDay, 0, len(rows))
	for _, r := range rows {
		out = append(out, RunsOnDay{Day: r.Day.Time, Status: r.Status, Runs: r.Runs})
	}
	return out, nil
}
