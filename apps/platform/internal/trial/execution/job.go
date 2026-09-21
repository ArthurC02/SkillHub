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
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/metrics"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
)

const (
	defaultMaxAttempts = 3

	defaultPollInterval = 2 * time.Second

	defaultSlotWaitInterval = 15 * time.Second

	SlotWaitLimit = 30 * time.Minute

	reasonLimit = 500
)

var errSuperseded = errors.New("run was moved by something else")

var ErrTryAgainLater = errors.New("the run is not ready to move on")

type tryAgainError struct{ after time.Duration }

func (e *tryAgainError) Error() string {
	return fmt.Sprintf("%s; look again in %s", ErrTryAgainLater, e.after)
}

func (e *tryAgainError) Unwrap() error { return ErrTryAgainLater }

func tryAgainIn(after time.Duration) error { return &tryAgainError{after: after} }

func RetryAfter(err error) (time.Duration, bool) {
	var retry *tryAgainError
	if !errors.As(err, &retry) {
		return 0, false
	}
	return retry.after, true
}

func (s *Service) Drive(ctx context.Context, workspaceID, runID pgtype.UUID) error {
	current, err := s.load(ctx, workspaceID, runID)
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

	attempts, err := s.attempts(ctx, current.WorkspaceID, current.ID)
	if err != nil {
		return err
	}
	err = (&driver{svc: s, cur: current, clock: clockFor(current, attempts)}).execute(ctx)
	if errors.Is(err, errSuperseded) {
		slog.Info("run driver superseded", "run_id", pgconv.UUIDString(runID))
		return nil
	}
	return err
}

type driver struct {
	svc      *Service
	cur      gen.Run
	provider SandboxProvider
	clock    runClock
}

func (d *driver) execute(ctx context.Context) error {

	if d.cur.CancelRequestedAt.Valid && d.cur.Status == gen.RunStatusQueued {
		return d.finish(ctx, pgtype.UUID{}, gen.RunStatusCancelled, failureCancelled, "派送之前就被取消")
	}
	if d.expired() {
		return d.finish(ctx, pgtype.UUID{}, gen.RunStatusTimedOut, failureTimeout, d.timeoutReason())
	}

	attempts, err := d.svc.attempts(ctx, d.cur.WorkspaceID, d.cur.ID)
	if err != nil {
		return err
	}
	switch live := liveAttempt(attempts); {
	case live != nil:

		return d.follow(ctx, attempts, *live)
	case d.cur.Status == gen.RunStatusQueued || d.cur.Status == gen.RunStatusProvisioning:
		return d.dispatch(ctx)
	case reassignableAfter(attempts):
		return d.dispatch(ctx)
	case d.cur.Status == gen.RunStatusEvaluating:

		return d.resumeEvaluating(ctx)
	default:
		return d.terminateUnresumable(ctx)
	}
}

func (d *driver) resumeEvaluating(ctx context.Context) error {
	attempts, err := d.svc.attempts(ctx, d.cur.WorkspaceID, d.cur.ID)
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
		"重啟之後沒有任何還活著的 Provider 嘗試可以接回去")
}

func (d *driver) dispatch(ctx context.Context) error {
	req, policy, err := requirementsFor(d.cur)
	if err != nil {
		return d.finish(ctx, pgtype.UUID{}, gen.RunStatusFailed, failurePlatform,
			d.reasonFor(failurePlatform, err))
	}

	attempts, err := d.svc.attempts(ctx, d.cur.WorkspaceID, d.cur.ID)
	if err != nil {
		return err
	}

	halts := d.svc.haltsFailClosed(ctx)
	if halts.dispatchPaused(d.svc.providers()) {
		slog.Warn("dispatch paused; leaving the run queued",
			"run_id", pgconv.UUIDString(d.cur.ID), "status", d.cur.Status)
		return nil
	}

	if err := d.svc.requireCuratedContent(ctx, d.cur); err != nil {
		return d.finish(ctx, pgtype.UUID{}, gen.RunStatusFailed, failurePolicy,
			d.reasonFor(failurePolicy, err))
	}

	if err := d.svc.requireModelGateway(); err != nil {
		return d.finish(ctx, pgtype.UUID{}, gen.RunStatusFailed, failureNoProvider,
			d.reasonFor(failureNoProvider, err))
	}

	budget, err := d.budgetForNextAttempt(ctx, attempts)
	if err != nil {
		return d.finish(ctx, pgtype.UUID{}, gen.RunStatusFailed, failureProvider,
			d.reasonFor(failureProvider, err))
	}

	avoid := lostProviders(attempts, halts.byTarget)
	placements, err := d.svc.providers().Place(ctx, req, avoid)
	switch {
	case errors.Is(err, ErrNoFreeSlot):
		return d.waitForSlot()
	case errors.Is(err, ErrNoSandboxAvailableYet):
		return d.waitForSandbox(err)
	case err != nil:
		return d.finish(ctx, pgtype.UUID{}, gen.RunStatusFailed, failureNoProvider,
			d.reasonFor(failureNoProvider, err))
	}
	if d.cur.Status == gen.RunStatusQueued {
		yield, err := d.svc.turnBelongsToAnother(ctx, d.cur, placements, avoid)
		if err != nil {
			return err
		}
		if yield {
			return d.waitForTurn()
		}
	}

	var (
		lastReason    statusReason
		lastAttemptID pgtype.UUID
		failures      int
	)
dispatching:
	for failures < d.svc.maxAttempts() {
		if len(placements) == 0 {
			return d.waitForSlot()
		}
		placement := placements[0]
		provider := placement.Provider
		d.provider = provider
		if d.expired() {
			return d.finish(ctx, lastAttemptID, gen.RunStatusTimedOut, failureTimeout, d.timeoutReason())
		}
		if cancelled, err := d.cancelRequested(ctx); err != nil {
			return err
		} else if cancelled {
			return d.finish(ctx, lastAttemptID, gen.RunStatusCancelled, failureCancelled, "派送進行中被取消")
		}

		started, err := d.command(ctx, func(r *Run) { r.StartAttempt(provider.Name()) })
		if err != nil {
			return err
		}
		attempt := started.LatestAttempt()
		lastAttemptID = attempt.ID

		request, err := d.svc.buildRunRequest(ctx, d.cur, attempt, placement.Profile, policy, budget)
		if err != nil {

			if expiryErr := d.svc.recordObjectGrantExpiry(ctx, attempt, objectGrantsExpiredOnArrival()); expiryErr != nil {
				slog.Error("could not close undispatched attempt object grants", "run_id", pgconv.UUIDString(d.cur.ID), "error", expiryErr)
			}
			reason := d.reasonFor(failurePlatform, err)
			return d.finishAttemptAndRun(ctx, attempt, errClassProvision, string(reason), gen.RunStatusFailed, failurePlatform, reason)
		}
		pr, err := provider.Start(ctx, request)
		if err != nil {
			lastReason = d.reasonFor(failureProvider, err)
			if err := d.finishAttempt(ctx, attempt, dispatchErrorClass(err), string(lastReason)); err != nil {
				return err
			}
			switch {
			case refusedForCapacity(err):
				d.svc.providers().forget(provider.Name())
				placements = placements[1:]
				continue
			case !retryable(err):
				break dispatching
			}
			failures++
			slog.Warn("run dispatch failed, retrying with a new attempt",
				"run_id", pgconv.UUIDString(d.cur.ID), "attempt", attempt.AttemptNumber, "error", err)
			continue
		}

		snapshot, err := pinnedRuntime(provider, placement.Capability, placement.Profile)
		if err != nil {
			return err
		}
		dispatched, err := d.command(ctx, func(r *Run) {
			r.RecordDispatch(attempt.ID, pr.ProviderRunID)
			r.AssignProvider(provider.Name(), snapshot)
		})
		if err == nil {
			attempt = dispatched.Attempt(attempt.ID)
			d.cur = dispatched.Row()
			d.clock = d.clock.dispatchedAt(attempt.StartedAt.Time)
		}
		if err != nil {

			if destroyErr := provider.Destroy(ctx, pr.ProviderRunID); destroyErr != nil {

				slog.Error("leaked a sandbox: its mapping could not be recorded and it could not be destroyed; the orphan scan will reclaim it",
					"run_id", pgconv.UUIDString(d.cur.ID), "error", destroyErr)
			}
			return err
		}
		if d.cur.Status == gen.RunStatusQueued {
			if err := d.advance(ctx, pgtype.UUID{}, gen.RunStatusProvisioning, "已選定 Provider:"+statusReason(provider.Name())); err != nil {
				return err
			}
		}

		if pr.State == ProviderStateFailed {
			slog.Error("a run failed on an external system; the run carries the platform's own wording instead",
				"run_id", pgconv.UUIDString(d.cur.ID),
				"error", fmt.Errorf("provider failed during provisioning: %s", truncate(pr.StateReason)))
			lastReason = "執行沙箱在準備階段就失敗了"
			if err := d.finishAttempt(ctx, attempt, errClassProvision, string(lastReason)); err != nil {
				return err
			}
			failures++
			continue
		}
		return d.follow(ctx, append(slices.Clone(attempts), attempt), attempt)
	}

	return d.finish(ctx, lastAttemptID, gen.RunStatusFailed, failureProvider,
		orDefault(lastReason, "派送沒有成功"))
}

func (d *driver) follow(ctx context.Context, attempts []gen.RunAttempt, attempt gen.RunAttempt) error {
	provider := d.provider
	if provider == nil || provider.Name() != attempt.Provider {
		provider = d.svc.providers().Lookup(attempt.Provider)
	}
	if provider == nil {
		return d.finish(ctx, attempt.ID, gen.RunStatusFailed, failurePlatform,
			"這次試跑用的 Provider "+statusReason(attempt.Provider)+" 已經不在這個部署的設定裡")
	}
	d.provider = provider
	if attempt.ProviderRunID == nil {
		return d.finish(ctx, attempt.ID, gen.RunStatusFailed, failurePlatform, "這次嘗試沒有 Provider 的 handle")
	}
	handle := *attempt.ProviderRunID

	pr, err := provider.Observe(ctx, handle)
	switch {
	case err == nil:
		d.providerAnswered(ctx, attempt)
		if err := d.mapState(ctx, attempt, pr); err != nil {
			return err
		}
		if pr.State.Terminal() {
			return d.settle(ctx, attempt, pr)
		}
	case providerForgotAttempt(err):
		return d.providerLost(ctx, attempt, "執行沙箱 "+statusReason(provider.Name())+" 已經不認得這次嘗試")
	case !retryable(err):
		reason := d.reasonFor(failureProvider, err)
		return d.finishAttemptAndRun(ctx, attempt, errClassExecution, string(reason), gen.RunStatusFailed, failureProvider, reason)
	default:
		silentSince, markErr := d.providerSilentSince(ctx, attempt)
		if markErr != nil {
			return markErr
		}
		silent := d.providerSilentFor(silentSince)
		if silent >= ProviderLostAfter {
			slog.Error("a provider stopped answering; the run carries the platform's own wording instead",
				"run_id", pgconv.UUIDString(d.cur.ID), "provider", provider.Name(), "error", err)
			return d.providerLost(ctx, attempt,
				"執行沙箱 "+statusReason(provider.Name())+" 已經 "+
					statusReason(silent.Round(time.Second).String())+" 沒有回應")
		}
		slog.Warn("provider poll failed", "run_id", pgconv.UUIDString(d.cur.ID), "error", err)
	}

	cancelled, err := d.cancelRequested(ctx)
	if err != nil {
		return err
	}
	if cancelled {
		if _, err := provider.Cancel(ctx, handle); err != nil {
			slog.Warn("provider cancel failed", "run_id", pgconv.UUIDString(d.cur.ID), "error", err)
		}
	}

	if d.expired() {
		if _, err := provider.Cancel(ctx, handle); err != nil {
			slog.Warn("provider cancel on timeout failed", "run_id", pgconv.UUIDString(d.cur.ID), "error", err)
		}
		return d.finishAttemptAndRun(ctx, attempt, errClassTimeout, string(d.timeoutReason()), gen.RunStatusTimedOut, failureTimeout, d.timeoutReason())
	}

	if reason := d.tokenCeilingBreach(ctx, attempts); reason != "" {
		if _, err := provider.Cancel(ctx, handle); err != nil {
			slog.Warn("provider cancel on token ceiling failed", "run_id", pgconv.UUIDString(d.cur.ID), "error", err)
		}
		return d.finishAttemptAndRun(ctx, attempt, errClassBudgetExhausted, string(reason), gen.RunStatusFailed, failureWorkload, reason)
	}

	return tryAgainIn(d.svc.pollInterval())
}

func (d *driver) mapState(ctx context.Context, attempt gen.RunAttempt, pr ProviderRun) error {
	reason := relayed(truncate(pr.StateReason))
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
		reason := d.reasonFor(failureProvider, err)
		return d.finishAttemptAndRun(ctx, attempt, errClassProvision, string(reason), gen.RunStatusFailed, failureProvider, reason)
	}
	if status != gen.RunStatusSucceeded {
		d.keepWorkloadOutput(ctx, attempt, pr)
		return d.finishAttemptAndRun(ctx, attempt, errClass, string(message), status, failureClass, message)
	}
	if err := d.finishAttempt(ctx, attempt, errClass, string(message)); err != nil {
		return err
	}

	return d.walkHappyPath(ctx, attempt.ID)
}

func (d *driver) keepWorkloadOutput(ctx context.Context, attempt gen.RunAttempt, pr ProviderRun) {
	if pr.Result == nil || pr.Result.AgentOutput == "" {
		return
	}
	err := d.svc.captureWorkloadOutput(ctx, attempt, pr.Result.AgentOutput)
	if err != nil {
		slog.Warn("the workload's own output could not be kept; this run's failure may have no explanation",
			"run_id", pgconv.UUIDString(attempt.RunID), "error", err)
	}
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

func successReason(to gen.RunStatus) statusReason {
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

func classifyResult(pr ProviderRun) (
	status gen.RunStatus, failureClass FailureClass, errClass string, message statusReason,
) {
	if pr.Result == nil {
		return gen.RunStatusFailed, failureProvider, errClassProvision,
			"執行沙箱回報這次嘗試已經結束,卻沒有附上結果"
	}
	errClass, message = "", relayed(truncate(pr.StateReason))
	if pr.Result.Error != nil {
		errClass, message = pr.Result.Error.Class, relayed(truncate(pr.Result.Error.Message))
	}

	switch {
	case pr.State == ProviderStateCancelled || pr.Result.Status == "cancelled":
		return gen.RunStatusCancelled, failureCancelled,
			orDefault(errClass, errClassCancelled), orDefault(message, "是使用者要求停止的")
	case pr.Result.Status == "timed_out":
		return gen.RunStatusTimedOut, failureTimeout,
			orDefault(errClass, errClassTimeout), orDefault(message, "執行沙箱在它自己的時間上限把工作負載停掉了")
	case pr.State == ProviderStateCompleted && pr.Result.Status == "succeeded":
		return gen.RunStatusSucceeded, "", "", ""
	case pr.State == ProviderStateCompleted:
		return gen.RunStatusFailed, failureWorkload,
			orDefault(errClass, errClassExecution), orDefault(message, "工作負載跑起來了,而且自己回報失敗")
	default:
		return gen.RunStatusFailed, failureProvider,
			orDefault(errClass, errClassProvision), orDefault(message, "執行沙箱沒能承載這次嘗試")
	}
}

type statusReason string

func relayed(fromProvider string) statusReason { return statusReason(fromProvider) }

func platformWordingFor(class FailureClass) statusReason {
	return statusReason(failureClassWords[class][0])
}

func namedRefusal(err error) statusReason {
	if _, ok := errors.AsType[*gatewayError](err); ok {
		return "模型閘道沒有為這次試跑配發金鑰"
	}
	if _, ok := errors.AsType[*providerError](err); ok {
		return "執行沙箱沒有接下這次試跑"
	}
	switch {
	case errors.Is(err, ErrProviderUnavailable):
		return "執行沙箱沒有回應"
	case errors.Is(err, ErrNoModelGateway):
		return "這個部署沒有接上模型閘道,試跑沒有辦法連到模型"
	case errors.Is(err, ErrContentNotCurated):
		return "淨測試模式只跑已策展的內容,這個版本不在其中"
	case errors.Is(err, ErrNoProvider):
		return "這個部署沒有設定任何執行沙箱,試跑沒有地方可以跑"
	case errors.Is(err, ErrNoCompatibleProvider):
		return "沒有任何已設定的執行沙箱能承接這個請求;是哪一項要求對不上,見這次 Run 的嘗試紀錄"
	}
	return ""
}

func (d *driver) reasonFor(class FailureClass, err error) statusReason {
	slog.Error("a run failed; the run carries the platform's own wording instead of this error",
		"run_id", pgconv.UUIDString(d.cur.ID), "failure_class", string(class), "error", err)
	if refused := namedRefusal(err); refused != "" {
		return refused
	}
	return platformWordingFor(class)
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

func (d *driver) advance(ctx context.Context, attemptID pgtype.UUID, to gen.RunStatus, reason statusReason) error {
	return d.transition(ctx, attemptID, to, "", reason)
}

func (d *driver) finish(
	ctx context.Context, attemptID pgtype.UUID, to gen.RunStatus, failureClass FailureClass, reason statusReason,
) error {
	return d.transition(ctx, attemptID, to, failureClass, reason)
}

func (d *driver) command(ctx context.Context, command func(*Run)) (*Run, error) {
	return d.svc.commandRun(ctx, d.cur.WorkspaceID, d.cur.ID, pgtype.UUID{}, func(r *Run) error {
		command(r)
		return nil
	})
}

func (d *driver) transition(
	ctx context.Context, attemptID pgtype.UUID, to gen.RunStatus, failureClass FailureClass, reason statusReason,
) error {
	run, err := d.svc.transition(ctx, transitionParams{
		workspaceID: d.cur.WorkspaceID,
		runID:       d.cur.ID,
		attemptID:   attemptID,
		from:        d.cur.Status,
		to:          to,
		reason:      truncate(reason),
		failure:     failureClass,
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
		nameKey := artifactNameKey(artifact.FileName)
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
	return d.svc.saveArtifactManifest(ctx, d.cur, archiveKey, pr.Result, truncated)
}

const runArtifactRetention = 90 * 24 * time.Hour

func artifactNameKey(fileName string) string { return strings.ToLower(fileName) }

type artifactManifestQueries interface {
	lock(context.Context, pgtype.UUID) error
	markTruncated(context.Context, gen.MarkRunArtifactsTruncatedParams) (int64, error)
	recordedNames(context.Context, gen.ListRunArtifactFileNamesParams) ([]string, error)
	insert(context.Context, gen.InsertRunArtifactParams) error
	retireIntent(context.Context, string) error
}

type artifactManifestStore struct{ q *gen.Queries }

func (s artifactManifestStore) lock(ctx context.Context, id pgtype.UUID) error {
	return s.q.LockRunArtifactManifest(ctx, id)
}
func (s artifactManifestStore) markTruncated(ctx context.Context, p gen.MarkRunArtifactsTruncatedParams) (int64, error) {
	return s.q.MarkRunArtifactsTruncated(ctx, p)
}
func (s artifactManifestStore) recordedNames(ctx context.Context, p gen.ListRunArtifactFileNamesParams) ([]string, error) {
	return s.q.ListRunArtifactFileNames(ctx, p)
}
func (s artifactManifestStore) insert(ctx context.Context, p gen.InsertRunArtifactParams) error {
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
	recorded, err := q.recordedNames(ctx, gen.ListRunArtifactFileNamesParams{RunID: current.ID, WorkspaceID: current.WorkspaceID})
	if err != nil {
		return fmt.Errorf("read recorded artifact names: %w", err)
	}
	taken := make(map[string]struct{}, len(recorded))
	for _, name := range recorded {
		taken[artifactNameKey(name)] = struct{}{}
	}
	expires := pgtype.Timestamptz{Time: time.Now().UTC().Add(runArtifactRetention), Valid: true}
	for _, a := range result.Artifacts {
		if _, redelivered := taken[artifactNameKey(a.FileName)]; redelivered {
			continue
		}
		contentType := a.ContentType
		if contentType == "" {

			contentType = "application/octet-stream"
		}
		if err := q.insert(ctx, gen.InsertRunArtifactParams{
			WorkspaceID: current.WorkspaceID, RunID: current.ID,
			FileName: a.FileName, ContentType: contentType, SizeBytes: a.SizeBytes,
			ContentHash: a.ContentHash, ObjectKey: archiveKey, ExpiresAt: expires,
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

func (d *driver) finishAttempt(ctx context.Context, attempt gen.RunAttempt, errClass, message string) error {
	_, err := d.svc.commandRun(ctx, attempt.WorkspaceID, attempt.RunID, pgtype.UUID{}, func(r *Run) error {
		r.FinishAttempt(attempt.ID, errClass, truncate(message))
		return nil
	})
	if errors.Is(err, errAttemptFinished) {
		return nil
	}
	return err
}

func (d *driver) finishAttemptAndRun(
	ctx context.Context, attempt gen.RunAttempt, errClass, message string,
	to gen.RunStatus, failureClass FailureClass, reason statusReason,
) error {
	from := d.cur.Status
	r, err := d.svc.commandRun(ctx, attempt.WorkspaceID, attempt.RunID, pgtype.UUID{}, func(r *Run) error {
		r.FinishAttemptAndTransition(attempt.ID, errClass, truncate(message), to, truncate(reason), failureClass)
		return nil
	})
	if errors.Is(err, ErrConflict) {
		return errSuperseded
	}
	if err != nil {
		return err
	}
	d.cur = r.Row()
	observeTransition(d.cur, transitionParams{
		workspaceID: attempt.WorkspaceID, runID: attempt.RunID, attemptID: attempt.ID,
		from: from, to: to, reason: truncate(reason), failure: failureClass,
	})
	return nil
}

func liveAttempt(attempts []gen.RunAttempt) *gen.RunAttempt {
	for i := len(attempts) - 1; i >= 0; i-- {
		if attempts[i].ProviderRunID != nil && !attempts[i].FinishedAt.Valid {
			return &attempts[i]
		}
	}
	return nil
}

func (d *driver) cancelRequested(ctx context.Context) (bool, error) {
	fresh, err := d.svc.load(ctx, d.cur.WorkspaceID, d.cur.ID)
	if err != nil {
		return false, err
	}
	if IsTerminal(fresh.Status) {
		return false, errSuperseded
	}
	d.cur.CancelRequestedAt = fresh.CancelRequestedAt
	return fresh.CancelRequestedAt.Valid, nil
}

type runClock struct {
	createdAt  time.Time
	dispatched time.Time
	wallClock  time.Duration
}

func clockFor(run gen.Run, attempts []gen.RunAttempt) runClock {
	seconds := runLimits(run).WallClockHardSeconds
	if seconds <= 0 {
		seconds = DefaultResourceLimits().WallClockHardSeconds
	}
	clock := runClock{wallClock: time.Duration(seconds) * time.Second}
	if run.CreatedAt.Valid {
		clock.createdAt = run.CreatedAt.Time
	}
	for _, a := range attempts {
		if a.ProviderRunID != nil && a.StartedAt.Valid {
			clock = clock.dispatchedAt(a.StartedAt.Time)
		}
	}
	return clock
}

func (c runClock) dispatchedAt(at time.Time) runClock {
	if c.dispatched.IsZero() || at.Before(c.dispatched) {
		c.dispatched = at
	}
	return c
}

func (c runClock) waiting() bool { return c.dispatched.IsZero() }

func (c runClock) deadline() time.Time {
	switch {
	case !c.waiting():
		return c.dispatched.Add(c.wallClock)
	case c.createdAt.IsZero():
		return time.Time{}
	default:
		return c.createdAt.Add(SlotWaitLimit)
	}
}

func (c runClock) expired(now time.Time) bool {
	deadline := c.deadline()
	return !deadline.IsZero() && now.After(deadline)
}

func (c runClock) timeoutReason() statusReason {
	deadline := c.deadline().UTC().Format(time.RFC3339)
	if c.waiting() {
		return statusReason(fmt.Sprintf("排隊等空的沙箱超過時間上限 %d 分鐘;期限是 %s", int(SlotWaitLimit.Minutes()), deadline))
	}
	return statusReason(fmt.Sprintf("超過硬性時間上限;期限是 %s", deadline))
}

func (d *driver) expired() bool { return d.clock.expired(d.svc.now()) }

func (d *driver) providerSilentFor(since time.Time) time.Duration { return d.svc.now().Sub(since) }

func (d *driver) timeoutReason() statusReason { return d.clock.timeoutReason() }

func (d *driver) waitForSlot() error {
	slog.Info("every sandbox provider that can run this is full; the run keeps its place in the queue",
		"run_id", pgconv.UUIDString(d.cur.ID), "status", d.cur.Status)
	return tryAgainIn(d.svc.slotWaitInterval())
}

func (d *driver) waitForSandbox(err error) error {
	slog.Warn("no sandbox provider is available right now; the run keeps its place in the queue "+
		"instead of failing, because waiting can fix this",
		"run_id", pgconv.UUIDString(d.cur.ID), "status", d.cur.Status, "error", err)
	return tryAgainIn(d.svc.slotWaitInterval())
}

func (d *driver) waitForTurn() error {
	slog.Info("a run that waited longer, or whose workspace holds fewer sandboxes, takes the free slot first",
		"run_id", pgconv.UUIDString(d.cur.ID))
	return tryAgainIn(d.svc.slotWaitInterval())
}

const tokenCeilingRoundsHint = "。此上限可跑的輪數取決於每輪的工具呼叫次數:純對話約 15 輪,每輪 1 次工具呼叫約 7.7 輪,每輪 2 次約 5 輪"

func (d *driver) tokenCeilingBreach(ctx context.Context, attempts []gen.RunAttempt) statusReason {
	limits := runLimits(d.cur).TokenBudget
	if d.svc.Gateway == nil || (limits.MaxInputTokens <= 0 && limits.MaxOutputTokens <= 0) {
		return ""
	}
	used, err := d.svc.usageOf(ctx, attempts)
	if err != nil {

		metrics.RunTokenUsageUnreadable.Inc()
		slog.Warn("could not read this run's token usage; the token ceiling is not being enforced for it",
			"run_id", pgconv.UUIDString(d.cur.ID), "error", err)
		return ""
	}
	switch {
	case limits.MaxInputTokens > 0 && used.InputTokens > limits.MaxInputTokens:
		metrics.RunTokenCeilingBreached.Inc()
		return statusReason(fmt.Sprintf("這次試跑用掉的輸入 token 超過上限:已用 %d,上限 %d%s",
			used.InputTokens, limits.MaxInputTokens, tokenCeilingRoundsHint))
	case limits.MaxOutputTokens > 0 && used.OutputTokens > limits.MaxOutputTokens:
		metrics.RunTokenCeilingBreached.Inc()
		return statusReason(fmt.Sprintf("這次試跑產出的 token 超過上限:已用 %d,上限 %d%s",
			used.OutputTokens, limits.MaxOutputTokens, tokenCeilingRoundsHint))
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

func orDefault[T ~string](v, fallback T) T {
	if v == "" {
		return fallback
	}
	return v
}

// Cutting by byte count can split a multi-byte rune; ToValidUTF8 drops the
// broken tail instead of writing invalid UTF-8 to a text column.
func truncate[T ~string](s T) T {
	if len(s) <= reasonLimit {
		return s
	}
	return T(strings.ToValidUTF8(string(s)[:reasonLimit], "") + "...")
}
