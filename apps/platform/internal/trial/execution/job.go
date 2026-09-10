package run

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"mime"
	"path"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/metrics"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
)

const (
	defaultMaxAttempts = 3

	defaultPollInterval = 2 * time.Second

	jobTimeout = 30 * time.Minute

	reasonLimit = 500
)

var liveJobStates = []rivertype.JobState{
	rivertype.JobStateAvailable,
	rivertype.JobStatePending,
	rivertype.JobStateRunning,
	rivertype.JobStateScheduled,
	rivertype.JobStateRetryable,
}

type JobArgs struct {
	RunID       string `json:"run_id"`
	WorkspaceID string `json:"workspace_id"`
}

func (JobArgs) Kind() string { return "run_execute" }

func executeInsertOpts() *river.InsertOpts {
	return &river.InsertOpts{
		UniqueOpts: river.UniqueOpts{ByArgs: true, ByState: liveJobStates},

		MaxAttempts: 3,
	}
}

type Worker struct {
	river.WorkerDefaults[JobArgs]
	Svc *Service
}

func (w *Worker) Timeout(*river.Job[JobArgs]) time.Duration { return jobTimeout }

var errSuperseded = errors.New("run was moved by something else")

func (w *Worker) Work(ctx context.Context, job *river.Job[JobArgs]) error {
	var runID, workspaceID pgtype.UUID
	if err := runID.Scan(job.Args.RunID); err != nil {
		return err
	}
	if err := workspaceID.Scan(job.Args.WorkspaceID); err != nil {
		return err
	}
	return w.Svc.Drive(ctx, workspaceID, runID)
}

func (s *Service) Drive(ctx context.Context, workspaceID, runID pgtype.UUID) error {
	current, err := s.Get(ctx, workspaceID, runID)
	if errors.Is(err, ErrNotFound) {
		slog.Warn("run job for unknown run", "run_id", pgconv.UUIDString(runID))
		return nil
	}
	if err != nil {
		return err
	}
	if IsTerminal(current.Status) {
		slog.Info("run job skipped, run already finished", "run_id", pgconv.UUIDString(runID), "status", current.Status)
		return nil
	}

	err = (&driver{svc: s, cur: current, deadline: hardDeadline(current)}).execute(ctx)
	if errors.Is(err, errSuperseded) {
		slog.Info("run driver superseded", "run_id", pgconv.UUIDString(runID))
		return nil
	}
	return err
}

type driver struct {
	svc      *Service
	cur      gen.Run
	provider *Provider
	deadline time.Time
}

func (d *driver) execute(ctx context.Context) error {

	if d.cur.CancelRequestedAt.Valid && d.cur.Status == gen.RunStatusQueued {
		return d.finish(ctx, pgtype.UUID{}, gen.RunStatusCancelled, failureCancelled, "派送之前就被取消")
	}
	if d.expired() {
		return d.finish(ctx, pgtype.UUID{}, gen.RunStatusTimedOut, failureTimeout, d.timeoutReason())
	}

	live, err := d.liveAttempt(ctx)
	if err != nil {
		return err
	}
	switch {
	case live != nil:

		return d.follow(ctx, *live)
	case d.cur.Status == gen.RunStatusQueued || d.cur.Status == gen.RunStatusProvisioning:
		return d.dispatch(ctx)
	case d.cur.Status == gen.RunStatusEvaluating:

		return d.resumeEvaluating(ctx)
	default:
		return d.terminateUnresumable(ctx)
	}
}

func (d *driver) resumeEvaluating(ctx context.Context) error {
	attempts, err := d.svc.queries().ListRunAttempts(ctx, gen.ListRunAttemptsParams{
		RunID: d.cur.ID, WorkspaceID: d.cur.WorkspaceID,
	})
	if err != nil {
		return err
	}
	if len(attempts) == 0 || attempts[len(attempts)-1].ErrorClass != nil {
		return d.terminateUnresumable(ctx)
	}
	return d.walkHappyPath(ctx, attempts[len(attempts)-1].ID)
}

func (d *driver) terminateUnresumable(ctx context.Context) error {
	return d.finish(ctx, pgtype.UUID{}, gen.RunStatusFailed, failurePlatform,
		"no live provider attempt to resume after a restart")
}

func (d *driver) dispatch(ctx context.Context) error {
	req, policy, err := requirementsFor(d.cur)
	if err != nil {
		return d.finish(ctx, pgtype.UUID{}, gen.RunStatusFailed, failurePlatform, err.Error())
	}

	halts := d.svc.haltsFailClosed(ctx)
	if halts.dispatchPaused(d.svc.providers()) {
		slog.Warn("dispatch paused; leaving the run queued",
			"run_id", pgconv.UUIDString(d.cur.ID), "status", d.cur.Status)
		return nil
	}

	if err := d.svc.requireCuratedContent(ctx, d.cur); err != nil {
		return d.finish(ctx, pgtype.UUID{}, gen.RunStatusFailed, failureNoProvider, err.Error())
	}

	provider, capability, profile, err := d.svc.providers().SelectExcluding(ctx, req, halts.byTarget)
	if err != nil {

		return d.finish(ctx, pgtype.UUID{}, gen.RunStatusFailed, failureNoProvider, err.Error())
	}
	d.provider = provider

	pinned, err := d.svc.pinProvider(ctx, d.cur, provider, capability, profile)
	if err != nil {
		return err
	}
	d.cur = pinned

	if d.cur.Status == gen.RunStatusQueued {
		if err := d.advance(ctx, pgtype.UUID{}, gen.RunStatusProvisioning, "已選定 Provider:"+provider.Name); err != nil {
			return err
		}
	}

	var (
		lastErr       error
		lastAttemptID pgtype.UUID
	)
	for i := 0; i < d.svc.maxAttempts(); i++ {
		if d.expired() {
			return d.finish(ctx, lastAttemptID, gen.RunStatusTimedOut, failureTimeout, d.timeoutReason())
		}
		if cancelled, err := d.cancelRequested(ctx); err != nil {
			return err
		} else if cancelled {
			return d.finish(ctx, lastAttemptID, gen.RunStatusCancelled, failureCancelled, "派送進行中被取消")
		}

		attempt, err := d.svc.queries().CreateRunAttempt(ctx, gen.CreateRunAttemptParams{
			ID: d.cur.ID, WorkspaceID: d.cur.WorkspaceID, Provider: provider.Name,
		})
		if err != nil {
			return err
		}
		lastAttemptID = attempt.ID

		request, err := d.svc.buildRunRequest(ctx, d.cur, attempt, profile, policy)
		if err != nil {

			if _, expiryErr := d.svc.queries().SetRunAttemptObjectGrantsExpiry(ctx, gen.SetRunAttemptObjectGrantsExpiryParams{
				ExpiresAt: pgtype.Timestamptz{Time: time.Now().UTC().Add(-2 * time.Minute), Valid: true}, ID: attempt.ID, WorkspaceID: d.cur.WorkspaceID,
			}); expiryErr != nil {
				slog.Error("could not close undispatched attempt object grants", "run_id", pgconv.UUIDString(d.cur.ID), "error", expiryErr)
			}
			d.failAttempt(ctx, attempt, errClassProvision, err.Error())
			return d.finish(ctx, attempt.ID, gen.RunStatusFailed, failurePlatform, err.Error())
		}
		pr, err := provider.CreateRun(ctx, request)
		if err != nil {
			lastErr = err
			d.failAttempt(ctx, attempt, dispatchErrorClass(err), err.Error())
			if !retryable(err) {
				break
			}
			slog.Warn("run dispatch failed, retrying with a new attempt",
				"run_id", pgconv.UUIDString(d.cur.ID), "attempt", attempt.AttemptNumber, "error", err)
			continue
		}

		attempt, err = d.svc.queries().SetAttemptProviderRunID(ctx, gen.SetAttemptProviderRunIDParams{
			ID: attempt.ID, WorkspaceID: d.cur.WorkspaceID, ProviderRunID: &pr.ProviderRunID,
		})
		if err != nil {

			if destroyErr := provider.Destroy(ctx, pr.ProviderRunID); destroyErr != nil {

				slog.Error("leaked a sandbox: its mapping could not be recorded and it could not be destroyed; the orphan scan will reclaim it",
					"run_id", pgconv.UUIDString(d.cur.ID), "error", destroyErr)
			}
			return err
		}

		if pr.State == ProviderStateFailed {
			lastErr = fmt.Errorf("provider failed during provisioning: %s", truncate(pr.StateReason))
			d.failAttempt(ctx, attempt, errClassProvision, lastErr.Error())
			continue
		}
		return d.follow(ctx, attempt)
	}

	message := "dispatch failed"
	if lastErr != nil {
		message = lastErr.Error()
	}
	return d.finish(ctx, lastAttemptID, gen.RunStatusFailed, failureProvider, message)
}

func (d *driver) follow(ctx context.Context, attempt gen.RunAttempt) error {
	provider := d.provider
	if provider == nil || provider.Name != attempt.Provider {
		provider = d.svc.providers().Lookup(attempt.Provider)
	}
	if provider == nil {
		return d.finish(ctx, attempt.ID, gen.RunStatusFailed, failurePlatform,
			"provider "+attempt.Provider+" is no longer configured")
	}
	d.provider = provider
	if attempt.ProviderRunID == nil {
		return d.finish(ctx, attempt.ID, gen.RunStatusFailed, failurePlatform, "這次嘗試沒有 Provider 的 handle")
	}
	handle := *attempt.ProviderRunID

	cancelSent := false
	for {
		pr, err := provider.GetRun(ctx, handle)
		switch {
		case err == nil:
			if err := d.mapState(ctx, attempt, pr); err != nil {
				return err
			}
			if pr.State.Terminal() {
				return d.settle(ctx, attempt, pr)
			}
		case !retryable(err):
			d.failAttempt(ctx, attempt, errClassExecution, err.Error())
			return d.finish(ctx, attempt.ID, gen.RunStatusFailed, failureProvider, err.Error())
		default:

			slog.Warn("provider poll failed", "run_id", pgconv.UUIDString(d.cur.ID), "error", err)
		}

		if !cancelSent {
			cancelled, err := d.cancelRequested(ctx)
			if err != nil {
				return err
			}
			if cancelled {

				if _, err := provider.Cancel(ctx, handle); err != nil {
					slog.Warn("provider cancel failed", "run_id", pgconv.UUIDString(d.cur.ID), "error", err)
				} else {
					cancelSent = true
				}
			}
		}

		if d.expired() {

			if _, err := provider.Cancel(ctx, handle); err != nil {
				slog.Warn("provider cancel on timeout failed", "run_id", pgconv.UUIDString(d.cur.ID), "error", err)
			}
			d.failAttempt(ctx, attempt, errClassTimeout, d.timeoutReason())
			return d.finish(ctx, attempt.ID, gen.RunStatusTimedOut, failureTimeout, d.timeoutReason())
		}

		if reason := d.tokenCeilingBreach(ctx, attempt); reason != "" {
			if _, err := provider.Cancel(ctx, handle); err != nil {
				slog.Warn("provider cancel on token ceiling failed", "run_id", pgconv.UUIDString(d.cur.ID), "error", err)
			}
			d.failAttempt(ctx, attempt, errClassBudgetExhausted, reason)
			return d.finish(ctx, attempt.ID, gen.RunStatusFailed, failureWorkload, reason)
		}

		select {
		case <-ctx.Done():

			return ctx.Err()
		case <-time.After(d.svc.pollInterval()):
		}
	}
}

func (d *driver) mapState(ctx context.Context, attempt gen.RunAttempt, pr ProviderRun) error {
	reason := truncate(pr.StateReason)
	switch pr.State {
	case ProviderStateCreating:
		if d.cur.Status == gen.RunStatusProvisioning {
			return d.advance(ctx, attempt.ID, gen.RunStatusPreparing, orDefault(reason, "Provider 正在建立沙箱"))
		}
	case ProviderStateRunning:
		if d.cur.Status == gen.RunStatusProvisioning {
			if err := d.advance(ctx, attempt.ID, gen.RunStatusPreparing, "Provider 已接下這次派送"); err != nil {
				return err
			}
		}
		if d.cur.Status == gen.RunStatusPreparing {
			return d.advance(ctx, attempt.ID, gen.RunStatusRunning, orDefault(reason, "工作負載已開始執行"))
		}
	}
	return nil
}

func (d *driver) settle(ctx context.Context, attempt gen.RunAttempt, pr ProviderRun) error {
	status, failureClass, errClass, message := classifyResult(pr)
	if err := d.recordArtifacts(ctx, attempt, pr); err != nil {
		message = err.Error()
		d.failAttempt(ctx, attempt, errClassProvision, message)
		return d.finish(ctx, attempt.ID, gen.RunStatusFailed, failureProvider, message)
	}
	d.failAttempt(ctx, attempt, errClass, message)

	if status != gen.RunStatusSucceeded {
		return d.finish(ctx, attempt.ID, status, failureClass, message)
	}
	return d.walkHappyPath(ctx, attempt.ID)
}

func (d *driver) walkHappyPath(ctx context.Context, attemptID pgtype.UUID) error {
	path, err := HappyPath(d.cur.Status)
	if err != nil {
		return err
	}
	for _, next := range path {
		if err := d.advance(ctx, attemptID, next, successReason(next)); err != nil {
			return err
		}
	}
	return nil
}

func successReason(to gen.RunStatus) string {
	switch to {
	case gen.RunStatusPreparing:
		return "Provider 已接下這次派送"
	case gen.RunStatusRunning:
		return "工作負載已開始執行"
	case gen.RunStatusEvaluating:
		return "工作負載已結束,正在收集結果"
	default:
		return "工作負載自己跑到結束並回報成功;任務究竟有沒有達成是另一個判斷" +
			"(見這次 Run 的評估)"
	}
}

func classifyResult(pr ProviderRun) (status gen.RunStatus, failureClass, errClass, message string) {
	if pr.Result == nil {
		return gen.RunStatusFailed, failureProvider, errClassProvision,
			"provider reported a terminal state with no result"
	}
	errClass, message = "", truncate(pr.StateReason)
	if pr.Result.Error != nil {
		errClass, message = pr.Result.Error.Class, truncate(pr.Result.Error.Message)
	}

	switch {
	case pr.State == ProviderStateCancelled || pr.Result.Status == "cancelled":
		return gen.RunStatusCancelled, failureCancelled,
			orDefault(errClass, errClassCancelled), orDefault(message, "stopped at the user's request")
	case pr.Result.Status == "timed_out":
		return gen.RunStatusTimedOut, failureTimeout,
			orDefault(errClass, errClassTimeout), orDefault(message, "the provider stopped the workload at its wall clock limit")
	case pr.State == ProviderStateCompleted && pr.Result.Status == "succeeded":
		return gen.RunStatusSucceeded, "", "", ""
	case pr.State == ProviderStateCompleted:
		return gen.RunStatusFailed, failureWorkload,
			orDefault(errClass, errClassExecution), orDefault(message, "the workload reported failure")
	default:
		return gen.RunStatusFailed, failureProvider,
			orDefault(errClass, errClassProvision), orDefault(message, "the provider could not carry the attempt")
	}
}

func dispatchErrorClass(err error) string {
	if pe, ok := errors.AsType[*providerError](err); ok {
		if pe.Class != "" {
			return pe.Class
		}
		if pe.Status == 422 {
			return errClassCapabilityMismatch
		}
	}
	return errClassProvision
}

func (d *driver) advance(ctx context.Context, attemptID pgtype.UUID, to gen.RunStatus, reason string) error {
	return d.transition(ctx, attemptID, to, "", reason)
}

func (d *driver) finish(ctx context.Context, attemptID pgtype.UUID, to gen.RunStatus, failureClass, reason string) error {
	if _, err := d.svc.queries().CloseUnissuedRunAttemptGrants(ctx, gen.CloseUnissuedRunAttemptGrantsParams{
		RunID: d.cur.ID, WorkspaceID: d.cur.WorkspaceID,
	}); err != nil {
		return err
	}
	return d.transition(ctx, attemptID, to, failureClass, reason)
}

func (d *driver) transition(ctx context.Context, attemptID pgtype.UUID, to gen.RunStatus, failureClass, reason string) error {
	run, err := d.svc.Transition(ctx, TransitionParams{
		WorkspaceID:  d.cur.WorkspaceID,
		RunID:        d.cur.ID,
		AttemptID:    attemptID,
		From:         d.cur.Status,
		To:           to,
		Reason:       truncate(reason),
		FailureClass: failureClass,
	})
	if errors.Is(err, ErrConflict) {
		return errSuperseded
	}
	if err != nil {
		return err
	}
	d.cur = run
	return nil
}

func (d *driver) recordArtifacts(ctx context.Context, attempt gen.RunAttempt, pr ProviderRun) error {
	if pr.Result == nil || (len(pr.Result.Artifacts) == 0 && !artifactCollectionTruncated(pr.Result)) {
		return nil
	}
	archiveKey := artifactObjectKey(pgconv.UUIDString(d.cur.ID), pgconv.UUIDString(attempt.ID))
	limits := runLimits(d.cur)
	truncated := artifactCollectionTruncated(pr.Result)
	if len(pr.Result.Artifacts) > 1000 {
		return errors.New("provider returned too many artifact manifest entries")
	}
	seen := make(map[string]struct{}, len(pr.Result.Artifacts))
	var total int64
	for _, artifact := range pr.Result.Artifacts {
		if artifact.ObjectKey != "" && artifact.ObjectKey != archiveKey {
			return errors.New("provider returned an artifact object key outside its write grant")
		}
		if !validArtifactFileName(artifact.FileName) {
			return fmt.Errorf("provider returned an invalid artifact file name %q", artifact.FileName)
		}
		nameKey := strings.ToLower(artifact.FileName)
		if _, duplicate := seen[nameKey]; duplicate {
			return fmt.Errorf("provider returned duplicate artifact file name %q", artifact.FileName)
		}
		seen[nameKey] = struct{}{}
		if artifact.SizeBytes < 0 || artifact.SizeBytes > limits.ArtifactFileBytes ||
			total > limits.ArtifactTotalBytes-artifact.SizeBytes {
			return fmt.Errorf("provider returned an invalid artifact size for %q", artifact.FileName)
		}
		total += artifact.SizeBytes
		decoded, err := hex.DecodeString(artifact.ContentHash)
		if err != nil || len(decoded) != 32 {
			return fmt.Errorf("provider returned an invalid artifact hash for %q", artifact.FileName)
		}
		if artifact.ContentType != "" {
			if len(artifact.ContentType) > 255 {
				return fmt.Errorf("provider returned an invalid artifact content type for %q", artifact.FileName)
			}
			if _, _, err := mime.ParseMediaType(artifact.ContentType); err != nil {
				return fmt.Errorf("provider returned an invalid artifact content type for %q", artifact.FileName)
			}
		}
	}
	if d.svc == nil || d.svc.Pool == nil {
		return errors.New("run artifact persistence is not configured")
	}
	tx, err := d.svc.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := gen.New(tx)
	if err := persistArtifactManifest(ctx, artifactManifestStore{q}, d.cur, archiveKey, pr.Result, truncated); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

type artifactManifestQueries interface {
	lock(context.Context, pgtype.UUID) error
	markTruncated(context.Context, gen.MarkRunArtifactsTruncatedParams) (int64, error)
	insert(context.Context, gen.InsertRunArtifactParams) (int64, error)
	retireIntent(context.Context, string) error
}

type artifactManifestStore struct{ q *gen.Queries }

func (s artifactManifestStore) lock(ctx context.Context, id pgtype.UUID) error {
	return s.q.LockRunArtifactManifest(ctx, id)
}
func (s artifactManifestStore) markTruncated(ctx context.Context, p gen.MarkRunArtifactsTruncatedParams) (int64, error) {
	return s.q.MarkRunArtifactsTruncated(ctx, p)
}
func (s artifactManifestStore) insert(ctx context.Context, p gen.InsertRunArtifactParams) (int64, error) {
	return s.q.InsertRunArtifact(ctx, p)
}
func (s artifactManifestStore) retireIntent(ctx context.Context, key string) error {
	return s.q.DeleteRunArtifactUploadIntentByObjectKey(ctx, key)
}

func persistArtifactManifest(
	ctx context.Context, q artifactManifestQueries, current gen.Run, archiveKey string,
	result *RunResult, truncated bool,
) error {
	if err := q.lock(ctx, current.ID); err != nil {
		return fmt.Errorf("lock artifact manifest: %w", err)
	}
	if truncated {
		if _, err := q.markTruncated(ctx, gen.MarkRunArtifactsTruncatedParams{
			ID: current.ID, WorkspaceID: current.WorkspaceID,
		}); err != nil {
			return fmt.Errorf("record truncated artifact collection: %w", err)
		}
	}
	for _, a := range result.Artifacts {
		contentType := a.ContentType
		if contentType == "" {

			contentType = "application/octet-stream"
		}
		if _, err := q.insert(ctx, gen.InsertRunArtifactParams{
			WorkspaceID: current.WorkspaceID, RunID: current.ID,
			FileName: a.FileName, ContentType: contentType, SizeBytes: a.SizeBytes,
			ContentHash: a.ContentHash, ObjectKey: archiveKey,
		}); err != nil {
			return fmt.Errorf("record artifact manifest row %q: %w", a.FileName, err)
		}
	}
	if len(result.Artifacts) > 0 {
		if err := q.retireIntent(ctx, archiveKey); err != nil {
			return fmt.Errorf("retire artifact upload intent: %w", err)
		}
	}
	return nil
}

func artifactCollectionTruncated(result *RunResult) bool {
	if result == nil {
		return false
	}
	if result.ArtifactsTruncated {
		return true
	}
	for _, artifact := range result.Artifacts {
		if artifact.Truncated {
			return true
		}
	}
	return false
}

func validArtifactFileName(name string) bool {
	if name == "" || len(name) > 1024 || !utf8.ValidString(name) || strings.Contains(name, `\`) ||
		strings.HasPrefix(name, "/") || strings.HasPrefix(name, "-") || path.Clean(name) != name {
		return false
	}
	for _, part := range strings.Split(name, "/") {
		if part == "" || part == "." || part == ".." || strings.TrimRight(part, " .") != part ||
			strings.ContainsAny(part, `<>:"|?*`) {
			return false
		}
		for _, r := range part {
			if r < 0x20 {
				return false
			}
		}
		base := strings.ToLower(strings.SplitN(part, ".", 2)[0])
		if base == "con" || base == "prn" || base == "aux" || base == "nul" ||
			(len(base) == 4 && (strings.HasPrefix(base, "com") || strings.HasPrefix(base, "lpt")) && base[3] >= '1' && base[3] <= '9') {
			return false
		}
	}
	return true
}

func (d *driver) failAttempt(ctx context.Context, attempt gen.RunAttempt, errClass, message string) {
	var classPtr, messagePtr *string
	if errClass != "" {
		classPtr = &errClass
	}
	if message != "" {
		truncated := truncate(message)
		messagePtr = &truncated
	}
	if _, err := d.svc.queries().FinishRunAttempt(ctx, gen.FinishRunAttemptParams{
		ID: attempt.ID, WorkspaceID: attempt.WorkspaceID,
		ErrorClass: classPtr, ErrorMessage: messagePtr,
	}); err != nil {
		slog.Warn("could not record attempt outcome", "run_attempt_id", pgconv.UUIDString(attempt.ID), "error", err)
	}
}

func (d *driver) liveAttempt(ctx context.Context) (*gen.RunAttempt, error) {
	attempts, err := d.svc.queries().ListRunAttempts(ctx, gen.ListRunAttemptsParams{
		RunID: d.cur.ID, WorkspaceID: d.cur.WorkspaceID,
	})
	if err != nil {
		return nil, err
	}
	for i := len(attempts) - 1; i >= 0; i-- {
		if attempts[i].ProviderRunID != nil && !attempts[i].FinishedAt.Valid {
			return &attempts[i], nil
		}
	}
	return nil, nil
}

func (d *driver) cancelRequested(ctx context.Context) (bool, error) {
	fresh, err := d.svc.Get(ctx, d.cur.WorkspaceID, d.cur.ID)
	if err != nil {
		return false, err
	}
	if IsTerminal(fresh.Status) {
		return false, errSuperseded
	}
	d.cur.CancelRequestedAt = fresh.CancelRequestedAt
	return fresh.CancelRequestedAt.Valid, nil
}

func hardDeadline(run gen.Run) time.Time {
	seconds := runLimits(run).WallClockHardSeconds
	if seconds <= 0 {
		seconds = DefaultResourceLimits().WallClockHardSeconds
	}
	if !run.CreatedAt.Valid {
		return time.Time{}
	}
	return run.CreatedAt.Time.Add(time.Duration(seconds) * time.Second)
}

func (d *driver) expired() bool {
	return !d.deadline.IsZero() && time.Now().After(d.deadline)
}

func (d *driver) timeoutReason() string {
	return fmt.Sprintf("超過硬性時間上限;期限是 %s", d.deadline.UTC().Format(time.RFC3339))
}

const tokenCeilingRoundsHint = "。此上限可跑的輪數取決於每輪的工具呼叫次數:純對話約 15 輪,每輪 1 次工具呼叫約 7.7 輪,每輪 2 次約 5 輪"

func (d *driver) tokenCeilingBreach(ctx context.Context, attempt gen.RunAttempt) string {
	limits := runLimits(d.cur).TokenBudget
	if d.svc.Gateway == nil || (limits.MaxInputTokens <= 0 && limits.MaxOutputTokens <= 0) {
		return ""
	}
	since := time.Now().UTC().Add(-time.Hour)
	if attempt.CreatedAt.Valid {
		since = attempt.CreatedAt.Time.UTC()
	}

	used, err := d.svc.Gateway.AttemptUsage(ctx, pgconv.UUIDString(attempt.ID), since)
	if err != nil {

		metrics.RunTokenUsageUnreadable.Inc()
		slog.Warn("could not read this attempt's token usage; the token ceiling is not being enforced for it",
			"run_id", pgconv.UUIDString(d.cur.ID), "run_attempt_id", pgconv.UUIDString(attempt.ID), "error", err)
		return ""
	}
	switch {
	case limits.MaxInputTokens > 0 && used.InputTokens > limits.MaxInputTokens:
		metrics.RunTokenCeilingBreached.Inc()
		return fmt.Sprintf("reached this run's token ceiling: %d input tokens used, limit %d%s",
			used.InputTokens, limits.MaxInputTokens, tokenCeilingRoundsHint)
	case limits.MaxOutputTokens > 0 && used.OutputTokens > limits.MaxOutputTokens:
		metrics.RunTokenCeilingBreached.Inc()
		return fmt.Sprintf("reached this run's token ceiling: %d output tokens used, limit %d%s",
			used.OutputTokens, limits.MaxOutputTokens, tokenCeilingRoundsHint)
	}
	return ""
}

func runLimits(run gen.Run) ResourceLimits {
	var policy policySnapshot
	if err := json.Unmarshal(run.PolicySnapshot, &policy); err != nil {
		return DefaultResourceLimits()
	}
	return policy.ResourceLimits
}

func orDefault(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

// Cutting by byte count can split a multi-byte rune; ToValidUTF8 drops the
// broken tail instead of writing invalid UTF-8 to a text column.
func truncate(s string) string {
	if len(s) <= reasonLimit {
		return s
	}
	return strings.ToValidUTF8(s[:reasonLimit], "") + "..."
}
