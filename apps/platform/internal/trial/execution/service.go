package run

// The application side of the Run lifecycle: the Service every other file in this
// package hangs its methods off, the create path, and the workspace-scoped reads.
// The aggregate it drives is statemachine.go; see doc.go for the file-group map.

import (
	"context"
	"encoding/json"
	"errors"
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
	// ErrNotFound covers "no such run" and "not yours" alike: existence is itself
	// private (WS-006), so callers turn this into a 404 either way.
	ErrNotFound = errors.New("run not found")
	// ErrRunFinished is a cancel arriving after the run reached a terminal state.
	ErrRunFinished               = errors.New("run has already finished")
	errRegistryReadNotConfigured = errors.New("run: registry owner read is not configured")
)

// SkillFacts and VersionFacts are the Registry facts consumed by Run.
type SkillFacts struct {
	AccessRestriction *string
}

type VersionFacts struct {
	ID               pgtype.UUID
	SkillID          pgtype.UUID
	ContentHash      string
	PackageObjectKey string
}

// ContentSource is where one Skill Version's material came from, in the only
// terms 02:PORT-007 recognises as curated: the public catalogue, or a PDM-002
// curation verdict that examined these exact bytes.
//
// Facts only. Whether they add up to "may run here" is a deployment policy and
// stays in this context (requireCuratedContent in schedule.go) - a composition
// root that decided it would be a second place the rule is written down, and
// the two would drift.
type ContentSource struct {
	// WorkspaceIsCatalog: the skill lives in a public catalogue workspace.
	WorkspaceIsCatalog bool
	// CurationTier is the skill's PDM-002 verdict (`curated` or `indexed`, 0042).
	CurationTier string
	// CuratedVersionIsThisOne: the verdict above examined the version about to
	// run. `curated` alone cannot say that - the column it travels with,
	// skills.curated_version_id, is what says it, and a newer version pushed on
	// top of a curated one inherits the tier without inheriting the review.
	CuratedVersionIsThisOne bool
}

// providerUnassigned is the provider of a run that has not been scheduled yet.
// The scheduler overwrites it the moment it picks one (RUN-005); runs.provider is
// NOT NULL, and naming the gap is better than defaulting to whichever provider
// happens to exist first.
const providerUnassigned = "unassigned"

// Service owns run state. Every method that changes a run does so in one
// transaction that also writes the status history, the audit event and the outbox
// event (iron rule 9).
type Service struct {
	Pool *pgxpool.Pool
	// TestLab is the published owner face for drafts, snapshots and datasets.
	TestLab *testlab.Service
	// ReadSkill and ReadVersion are Registry owner reads adapted by each root.
	ReadSkill   func(context.Context, pgtype.UUID, pgtype.UUID) (SkillFacts, bool, error)
	ReadVersion func(context.Context, pgtype.UUID, pgtype.UUID) (VersionFacts, bool, error)
	// ReadContentSource answers 02:PORT-010's content-source question for one
	// Skill Version (workspace, version). It is a Registry owner read like the
	// two above and is injected the same way: `skills.curation_tier` and
	// `workspaces.is_catalog` belong to other contexts, and reaching into their
	// queries from here is what ADR-033 forbids.
	//
	// Nil is not "no opinion". The one gate that reads it refuses when it cannot
	// ask, because the deployment it guards has no isolation boundary at all -
	// see requireCuratedContent.
	ReadContentSource func(context.Context, pgtype.UUID, pgtype.UUID) (ContentSource, bool, error)
	// RunVerdicts is eval's owner read of the standing verdict for a page of
	// runs, injected by the composition root (ADR-034, 04 丙-32). A JOIN to
	// `evaluations` from a query in this context would pass CI — the ownership
	// checker sees which context calls which query, not which tables a query
	// touches — and that blind spot is exactly what ADR-033 was written to close.
	//
	// The block crosses as bytes because nothing here reads a field of it: the
	// wording folds the evaluation's status together with its verdict, and only
	// eval can tell 「評估中」 from 「無法判斷」.
	RunVerdicts func(context.Context, pgtype.UUID, []pgtype.UUID) (map[string]json.RawMessage, error)
	// WorkspaceCreatedAt is identity's pool-backed owner read for quota display.
	WorkspaceCreatedAt func(context.Context, pgtype.UUID) (time.Time, error)
	// ActiveArtifactReferences is packaging's owner read, injected by each
	// composition root before this service may delete shared object bytes.
	ActiveArtifactReferences func(ctx context.Context, objectKey string) (int64, error)
	// Queue enqueues the execution job in the same transaction as the run row, so
	// a committed run always has a job and a rolled-back one never does. Nil is
	// allowed: the run is created and stays queued forever, which is what a
	// deployment without a worker actually does.
	Queue *river.Client[pgx.Tx]
	// Providers is the configured sandbox fleet (RUN-005). Nil or empty is a
	// deployment with no sandbox: runs are still accepted and then fail saying so,
	// rather than being rejected as if the user had asked for something impossible.
	Providers *Registry
	// Store reads the stored skill package, for the one question the pre-run
	// permission summary cannot answer from the database: does this version carry
	// a script (02:TEST-005). Nil means the scan reports itself unavailable rather
	// than reporting a clean package.
	Store ObjectStore
	// MaxAttempts bounds automatic retries of one run (ADR-004: no unbounded
	// retries). Zero means defaultMaxAttempts.
	MaxAttempts int
	// PollInterval is how often a dispatched attempt is read back from the
	// provider. Zero means defaultPollInterval.
	PollInterval time.Duration
	// TraceSigner mints the per-attempt trace ingestion credential (TRACE-002).
	// Nil, or one with no secret, means no ingestion URL is handed to the
	// provider and no events are collected - which is honest for a deployment
	// that has not configured the secret, and better than an open endpoint.
	TraceSigner *trace.Signer
	// Trace owns deployment-wide masking activity. It is injected by each root;
	// a missing service returns an error instead of pretending the counts are zero.
	Trace *trace.Service
	// MaskerCanary overrides the masker liveness probe run at the end of every
	// supervisor sweep (02:SEC-010's TraceMaskingStopped, asked directly rather
	// than inferred from traffic). Nil means trace.MaskerCanary, the real one;
	// only a test ever sets it, because a pure function over compiled-in rules has
	// no other seam a test can break.
	MaskerCanary func() []string
	// TraceIngestBaseURL is the origin an execution node reaches the control
	// plane on (SKILLHUB_TRACE_INGEST_URL). Empty has the same effect as a
	// disabled signer.
	TraceIngestBaseURL string
	// Quota is the PDM-010 free run allowance this deployment applies (ADR-028).
	// The zero value enforces nothing and displays nothing, which is the honest
	// state of a build with no allowance — see internal/policy for why enforcement
	// had to exist before the display did.
	Quota policy.QuotaLimits
	// Gateway mints and revokes the per-run model credential (SBX-008, ADR-017).
	// Nil is a deployment with no model gateway: no grant is minted, the egress
	// allow list defaultPolicy writes stays empty, and the sandbox gets no route
	// out. Only the worker sets it - the API never mints a key.
	Gateway *Gateway
}

func (s *Service) requireTestLab() error {
	if s.TestLab == nil || s.TestLab.Pool == nil {
		return errors.New("run: test lab service not injected")
	}
	return nil
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

// ResourceLimits mirrors ResourceLimits in contracts/openapi/sandbox-provider.yaml.
// Values are PDM-005 5.2. Changing one means changing both - the contract is the
// spec, this is the snapshot written into policy_snapshot so TEST-005 can show the
// user what their run will be held to, and so a past run stays explainable after
// the defaults change.
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
		MemoryBytes:          4 << 30, // 4 GiB
		DiskBytes:            8 << 30, // 8 GiB: /work 6 + /out 2
		MaxPIDs:              256,
		MaxOpenFiles:         1024,
		WallClockSoftSeconds: 600, // reaching this marks the run timed_out
		WallClockHardSeconds: 900, // forced destroy
		ArtifactTotalBytes:   100 << 20,
		ArtifactFileBytes:    25 << 20,
	}
	// Enforced, and not by the Virtual Key's max_budget - prompt caching decoupled
	// spend from token count by 7-8x (PDM-005 5.2a). Two parties count, on either
	// side of the sandbox boundary: the harness stops a cooperating workload from
	// inside, and the worker reads the gateway's own spend log and terminates one
	// that ignored it (job.go's tokenCeilingBreach). RunUsage still carries no
	// number back here - it arrives at settlement, which is after the point of
	// stopping anything. These limits travel to both counters inside this same
	// snapshot, so the ceiling that stops a run is the one the pre-run permission
	// summary showed the user and they confirmed (02:TEST-005).
	l.TokenBudget.MaxInputTokens = 300_000
	l.TokenBudget.MaxOutputTokens = 60_000
	return l
}

// policySnapshot is runs.policy_snapshot: what the run is held to, frozen at
// creation (ADR-003) and read back by the scheduler so capability matching happens
// against what the user was shown, not against today's defaults.
type policySnapshot struct {
	ResourceLimits ResourceLimits `json:"resource_limits"`
	Egress         EgressPolicy   `json:"egress"`
}

// defaultPolicy is the policy a new run gets, and the ONLY place it is written.
//
// Egress is default-deny. The allow list names the model gateway when one is
// configured (SBX-007: the sandbox is placed on a network where that address,
// and nothing else, is reachable) and is otherwise empty, which the dev provider
// serves as `--network none` - no egress at all, strictly stronger. Object
// storage and trace ingestion are deliberately *not* on it: the execution node
// moves those bytes on the sandbox's behalf, so the sandbox itself never holds a
// route to the platform's storage (SBX-008).
//
// The URL here is not an authorization - it is the destination the user is shown
// and agrees to before the run starts (02:TEST-005). The credential for it is
// the Virtual Key, minted per attempt at dispatch and never part of this policy.
//
// Every reader goes through here: Create freezes it onto the run, the scheduler
// matches providers against it, and the pre-run summary shows it. A second copy
// would let the screen the user confirms drift away from what the run is
// actually held to, and the hash would keep saying they agreed (02:TEST-005).
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

// CreateParams is one run request. The workspace comes from the session, never
// from the request body (iron rule 3).
type CreateParams struct {
	WorkspaceID pgtype.UUID
	Actor       pgtype.UUID
	SkillID     pgtype.UUID
	VersionID   pgtype.UUID
	TestCaseID  pgtype.UUID
	// ConfirmedSummaryHash is the pre-run permission summary the user agreed to
	// (02:TEST-005). It is not trusted as an authorization by itself — the summary
	// is rebuilt and the agreement looked up in requirePermissionConfirmation.
	ConfirmedSummaryHash string
}

// Create records a queued run and enqueues its execution job, in one transaction.
//
// The test case is snapshotted here rather than referenced: a run points at frozen
// content (iron rule 4), so later edits to the test case cannot rewrite what a past
// run was asked to do.
//
// This wrapper is only here to give the refusals one exit. create below has a
// dozen error returns spread over four files, and gate B's whole point is that any
// one of them stops the run — so "a run was refused" had a dozen places it could
// have been recorded and, until now, was recorded in none of them. Audited here
// instead of at each check for the same reason requirePermissionConfirmation lives
// in Create rather than in the handler: one choke point cannot be forgotten by the
// next check somebody adds.
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

// auditRefusal records a gate B refusal (NFR-001). Best effort, deliberately:
// nothing was written by the path that just failed, so there is no transaction to
// join (iron rule 9 has nothing to hold this to) and no state for a missing row to
// contradict. Failing the request because the note about the refusal could not be
// filed would turn a 422 into a 500 and tell the user even less.
//
// Only gate B refusals are recorded. A lookup that came back ErrNotFound is the
// caller asking about something that is not theirs — WS-006 makes that answer
// deliberately indistinguishable from "does not exist", and a row per probe would
// be an enumeration log. Infrastructure errors are not refusals at all; those are
// what slog and the error counters are for.
func (s *Service) auditRefusal(ctx context.Context, p CreateParams, err error) {
	var reason string
	r, isRefusal := errors.AsType[refusal](err)
	switch {
	case isRefusal:
		reason = r.reason
	// The two gate B conditions that live with their features (gateb.go's header
	// lists them). Neither goes through refused(), so they are matched on their
	// sentinels; each has exactly one refusal, so nothing is lost by the coarser
	// match.
	case errors.Is(err, ErrPermissionsNotConfirmed):
		reason = "permissions_unconfirmed"
	case errors.Is(err, ErrNoCompatibleProvider):
		reason = "capability_mismatch"
	default:
		return
	}
	// Identifiers and the reason code only — never the error string, which quotes
	// scan findings and package content (iron rule 11). There is no run to point
	// at, so the resource is the version that was refused.
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
	// SEC-012 action ①, first of the two entry points (halt.go): while the fleet is
	// held for a P1, no new run is accepted. Before every other request check,
	// including the workspace-scoped reads below, because a stopped platform is not going
	// to become able to run this by the caller fixing anything about the request —
	// and because a stopped platform should not be doing lookups on its way to
	// saying no.
	if err := s.requireDispatchable(ctx); err != nil {
		return gen.Run{}, err
	}
	// Both reads are workspace scoped, so another workspace's version or test case
	// is "not found" rather than "forbidden" (WS-006).
	version, found, err := s.ReadVersion(ctx, p.WorkspaceID, p.VersionID)
	if !found && err == nil {
		return gen.Run{}, ErrNotFound
	}
	if err != nil {
		return gen.Run{}, err
	}
	// The version must belong to the skill in the URL, or /skills/A/runs could run
	// a version of skill B that the caller also happens to own.
	if version.SkillID != p.SkillID {
		return gen.Run{}, ErrNotFound
	}
	// 0023: materials under a licensing hold are not copied into a sandbox.
	// Checked from the skill row before anything else is read, because it is the
	// one refusal that no amount of the caller fixing their request can clear.
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
	// RUN-005 / ADR-004: work no configured provider can run is refused here,
	// before it is queued, rather than after a user has watched it sit in a queue.
	var decoded policySnapshot
	if err := json.Unmarshal(policy, &decoded); err != nil {
		return gen.Run{}, err
	}
	if err := s.checkSchedulable(ctx, decoded); err != nil {
		return gen.Run{}, err
	}
	// SEC-002 gate B: a version whose static scan is blocking, or that cannot be
	// scanned at all, does not start (02:SEC-003). Before the permission summary,
	// because a run that may not start at all should not ask the user to agree to
	// its permissions first.
	if err := s.requireScanNotBlocking(ctx, version.PackageObjectKey); err != nil {
		return gen.Run{}, err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return gen.Run{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.queries().WithTx(tx)

	// SEC-002 gate B: the workspace concurrency ceiling (PDM-005 §5.2). Inside the
	// transaction, unlike the three checks above, because it is the only one whose
	// answer another request can invalidate while this one is deciding - it takes a
	// per-workspace lock that is released by this commit.
	if err := s.requireRunSlot(ctx, q, p.WorkspaceID); err != nil {
		return gen.Run{}, err
	}
	// PDM-010's free allowance (ADR-028 決策 2). Immediately after the concurrency
	// check and deliberately not anywhere else: this is the only point where a run
	// is about to exist and nothing has been spent on it, and it reuses the lock
	// that call just took, so two simultaneous requests cannot both see the last
	// remaining run. Nothing on the model gateway answers this question — see
	// quota.go for why max_budget, tpm_limit and the concurrency limit are three
	// different brakes and none of them is a monthly allowance.
	if err := s.requireQuota(ctx, tx, p.WorkspaceID); err != nil {
		return gen.Run{}, err
	}

	// The permission check and snapshot are one critical section. Dataset
	// upload/delete and test-case edits all take this parent row lock, so the
	// confirmed hash cannot describe one input set while the snapshot freezes
	// another (SEC-002 gate B). The lock has to be taken here rather than left to
	// CreateSnapshot below, which also takes it: the section has to be open
	// before the permission check reads the summary it compares.
	//
	// Asked of the test lab rather than taken with its query (DDD-031): the row
	// belongs to that context and so does the decision about what locking it
	// protects. This package still owns everything it owned - which skill the
	// case must belong to, the permission check, the run row.
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

	// The test lab owns the snapshot's shape and its hash; this passes it the
	// transaction handle so the snapshot and the run below commit together
	// (iron rule 9). A test case that is missing, in another workspace, or
	// soft-deleted comes back as not found - a deleted draft is not runnable.
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
		// Empty until a provider is chosen and its runtime pinned (RUN-005).
		RuntimeSnapshot: []byte("{}"),
		PolicySnapshot:  policy,
	})
	if err != nil {
		return gen.Run{}, err
	}

	if err := s.record(ctx, q, tx, run, nil, pgtype.UUID{}, "run requested", p.Actor, audit.ActionRunCreate); err != nil {
		return gen.Run{}, err
	}

	if s.Queue != nil {
		// Same unique options the supervisor uses, so a re-enqueue after a restart
		// lands on this job instead of starting a second driver (RUN-008).
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

// Get returns one run, workspace scoped.
func (s *Service) Get(ctx context.Context, workspaceID, runID pgtype.UUID) (gen.Run, error) {
	run, err := s.queries().GetRun(ctx, gen.GetRunParams{ID: runID, WorkspaceID: workspaceID})
	if errors.Is(err, pgx.ErrNoRows) {
		return gen.Run{}, ErrNotFound
	}
	return run, err
}

// List returns the workspace's Run history, newest first (WS-004). A valid
// testCaseID narrows it to the runs of that one test case, which is what turns
// 建立 → 試跑 into 建立 → 試跑 → 回來看.
//
// The page size is clamped rather than trusted: it comes from a query string and
// it decides how much work one request does. The filter cannot widen the scope —
// the statement is workspace scoped either way (iron rule 3), and another
// workspace's test case simply matches none of the rows it returns.
func (s *Service) List(
	ctx context.Context, workspaceID, testCaseID pgtype.UUID, limit, offset int32,
) ([]gen.ListWorkspaceRunsRow, error) {
	if limit <= 0 || limit > maxRunPageSize {
		limit = defaultRunPageSize
	}
	if offset < 0 {
		offset = 0
	}
	if !testCaseID.Valid {
		return s.queries().ListWorkspaceRuns(ctx, gen.ListWorkspaceRunsParams{
			WorkspaceID: workspaceID, PageSize: limit, PageOffset: offset,
		})
	}

	// ponytail: the predicate runs here over the newest runFilterScan runs rather
	// than in ListWorkspaceRuns. Ceiling: a workspace with more runs than that
	// will not see the older ones in a filtered list. Upgrade path is a nullable
	// test_case_id argument on the statement itself, which is where it belongs the
	// moment a real workspace approaches the cap.
	rows, err := s.queries().ListWorkspaceRuns(ctx, gen.ListWorkspaceRunsParams{
		WorkspaceID: workspaceID, PageSize: runFilterScan, PageOffset: 0,
	})
	if err != nil {
		return nil, err
	}
	matched := make([]gen.ListWorkspaceRunsRow, 0, limit)
	skipped := int32(0)
	for _, row := range rows {
		if row.TestCaseID != testCaseID {
			continue
		}
		if skipped < offset {
			skipped++
			continue
		}
		if matched = append(matched, row); int32(len(matched)) == limit {
			break
		}
	}
	return matched, nil
}

const (
	defaultRunPageSize = 50
	maxRunPageSize     = 200
	// runFilterScan bounds the in-Go filter above. Well past maxRunPageSize so a
	// filtered page is full whenever an unfiltered one would be.
	runFilterScan = 500
)

// Artifacts returns one run's output manifest (WS-004's read side, and what makes
// DeleteArtifact reachable — a delete for something nobody can list is a delete
// nobody can press).
func (s *Service) Artifacts(
	ctx context.Context, workspaceID, runID pgtype.UUID,
) ([]gen.Artifact, error) {
	// The run itself is read first so a caller learns nothing about another
	// workspace's run id: an unknown run and somebody else's run answer alike.
	if _, err := s.Get(ctx, workspaceID, runID); err != nil {
		return nil, err
	}
	return s.queries().ListRunArtifacts(ctx, gen.ListRunArtifactsParams{
		RunID: runID, WorkspaceID: workspaceID,
	})
}

// DeleteArtifact removes one Run output the owner asked to be gone
// (02:WS-002 3, 02:SEC-006 1). Until now the only way to reach a run artifact was
// to delete the whole account.
//
// Idempotent, exactly as the download package's delete is: an id that is not
// there, is already deleted, or belongs to somebody else all reach the same
// answer. The caller's intent — this file must not exist — holds in every one of
// those cases.
//
// The row is soft-deleted and the object removed after the commit, the order
// DeleteDownload uses and for the same reason: object storage has no rollback, so
// removing bytes a live row still points at is the failure that cannot be
// repaired, while an orphan object costs storage and nothing else.
//
// This path owns the object of a deleted row outright. The retention sweep
// (ExpiredArtifactCandidates) skips `deleted_at IS NOT NULL` on purpose: the
// shared-key count below is what stops a delete taking a sibling row's bytes,
// and the sweep does not have it. So a Remove that fails here leaves bytes
// behind until the account purge lists the key — logged, not retried.
func (s *Service) DeleteArtifact(
	ctx context.Context, ws identity.Workspace, runID, artifactID pgtype.UUID,
) error {
	if s.ActiveArtifactReferences == nil {
		return errors.New("run: artifact reference counter not injected; refusing delete")
	}
	tx, err := s.Pool.Begin(ctx)
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
		return nil // the bytes are already gone, or there is no store to ask
	}
	// A run's outputs are one archive per attempt, so several rows can name one
	// object key. Removing it while another row still points at it would delete a
	// file the owner did not ask about; the count is asked after the soft delete,
	// so it cannot count the row just removed.
	shared, err := s.ActiveArtifactReferences(ctx, row.ObjectKey)
	if err != nil || shared > 0 {
		if err != nil {
			slog.Warn("could not check whether a run artifact object is shared; leaving the bytes",
				"object_key", row.ObjectKey, "error", err)
		}
		return nil
	}
	// Best effort: the row is gone either way and Remove is idempotent. Nothing
	// retries it — the retention sweep does not read deleted rows — so the bytes
	// wait for the account purge, which lists every key regardless of deleted_at.
	if err := s.Store.Remove(ctx, row.ObjectKey); err != nil {
		slog.Warn("run artifact object not removed; the row is deleted and the bytes remain until the account purge",
			"object_key", row.ObjectKey, "error", err)
	}
	return nil
}

// Linkage returns the skill the run's version belongs to and the editable test
// case its snapshot was frozen from — the two ids the runs row does not carry and
// the read surface has to serve (RUN-002).
//
// Neither is an authorization. Whether the run may be repeated is preflight's
// answer, and whether its inputs still exist is a separate question again.
func (s *Service) Linkage(ctx context.Context, workspaceID, runID pgtype.UUID) (gen.GetRunLinkageRow, error) {
	row, err := s.queries().GetRunLinkage(ctx, gen.GetRunLinkageParams{
		RunID: runID, WorkspaceID: workspaceID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return gen.GetRunLinkageRow{}, ErrNotFound
	}
	return row, err
}

// History returns the status transitions of one run, oldest first (RUN-002: the
// user can see the current status and when it last changed).
func (s *Service) History(ctx context.Context, workspaceID, runID pgtype.UUID) ([]gen.RunStatusTransition, error) {
	return s.queries().ListRunStatusTransitions(ctx, gen.ListRunStatusTransitionsParams{
		RunID: runID, WorkspaceID: workspaceID,
	})
}

// Attempts returns the run's attempts with their provider ids (RUN-003).
func (s *Service) Attempts(ctx context.Context, workspaceID, runID pgtype.UUID) ([]gen.RunAttempt, error) {
	return s.queries().ListRunAttempts(ctx, gen.ListRunAttemptsParams{
		RunID: runID, WorkspaceID: workspaceID,
	})
}

// RequestCancel records the user's intent to cancel (RUN-004). It does not change
// the run's status: the workload is still running until something stops it, and
// reporting `cancelled` before that would be a lie about a live sandbox.
//
// Propagating the intent - signalling the provider, transitioning to cancelled
// once the workload is actually down, and the wall-clock timeout that shares this
// machinery - is RUN-006.
func (s *Service) RequestCancel(ctx context.Context, workspaceID, runID, actor pgtype.UUID) (gen.Run, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return gen.Run{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.queries().WithTx(tx)

	run, err := q.RequestRunCancel(ctx, gen.RequestRunCancelParams{ID: runID, WorkspaceID: workspaceID})
	if errors.Is(err, pgx.ErrNoRows) {
		// Either it does not exist here, or it is already finished. Distinguish,
		// so a user who cancelled a second too late is told which.
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
