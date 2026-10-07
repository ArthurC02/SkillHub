package run

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/metrics"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/ArthurC02/skillhub/apps/platform/internal/product/entitlements"
	"github.com/ArthurC02/skillhub/apps/platform/internal/shared/skillpkg"
)

const MaxConcurrentRunsPerWorkspace = 1

var (
	ErrScanBlocked = errors.New("the skill version's static scan blocks it from running")

	ErrRunLimitReached = errors.New("this workspace already has the maximum number of runs in progress")

	ErrAccessRestricted = errors.New("this skill cannot be run while its source license is under review")
)

const (
	ReasonAccessRestricted       = "access_restricted"
	ReasonVersionDisabled        = "version_disabled"
	ReasonScanUnavailable        = "scan_unavailable"
	ReasonScanBlocked            = "scan_blocked"
	ReasonWorkspaceConcurrency   = "workspace_concurrency"
	ReasonCreditBalance          = "credit_balance"
	ReasonPermissionsUnconfirmed = "permissions_unconfirmed"
	ReasonCapabilityMismatch     = "capability_mismatch"
)

func RefusalReasons() []string {
	reasons := []string{
		ReasonAccessRestricted,
		ReasonVersionDisabled,
		ReasonScanUnavailable,
		ReasonScanBlocked,
		ReasonWorkspaceConcurrency,
		ReasonCreditBalance,
		ReasonPermissionsUnconfirmed,
		ReasonCapabilityMismatch,
	}
	return append(reasons, policy.AllowanceRefusalReasons(policy.RunQuotaPrefix)...)
}

type refusal struct {
	reason string
	err    error
}

func (r refusal) Error() string { return r.err.Error() }
func (r refusal) Unwrap() error { return r.err }

func refused(reason string, err error) error {
	metrics.RunRefused.WithLabelValues(reason).Inc()
	return refusal{reason: reason, err: err}
}

func (s *Service) requireNotAccessRestricted(skill SkillFacts) error {
	reason, err := accessVerdict(skill)
	if err != nil {
		return refused(reason, err)
	}
	return nil
}

func accessVerdict(skill SkillFacts) (string, error) {
	if !skill.AccessRestricted {
		return "", nil
	}
	return ReasonAccessRestricted,
		fmt.Errorf("%w (%s)", ErrAccessRestricted, skill.AccessRestrictionReason)
}

type packageScan struct {
	report  skillpkg.Report
	scanned bool
}

func (s *Service) packageReport(ctx context.Context, stored skillpkg.StoredSkill) packageScan {
	if s.store() == nil {
		return packageScan{}
	}
	data, err := s.store().Get(ctx, stored.ObjectKey)
	if err != nil {
		return packageScan{}
	}
	fsys, err := stored.Open(data)
	if err != nil {
		return packageScan{}
	}
	return packageScan{report: skillpkg.Validate(fsys), scanned: true}
}

type scanRefusal struct {
	codes     []string
	unscanned bool
}

func (s scanRefusal) Unwrap() error { return ErrScanBlocked }

func (s scanRefusal) Error() string {
	if s.unscanned {
		return fmt.Sprintf("%s: the package could not be scanned, and an unscanned package "+
			"is not treated as a clean one", ErrScanBlocked)
	}
	return fmt.Sprintf("%s: %s", ErrScanBlocked, strings.Join(s.codes, ", "))
}

func (s scanRefusal) inInterfaceLanguage() string {
	if s.unscanned {
		return "這個版本的套件這次讀不到，所以沒有掃描結果；沒掃過的套件不會被當成乾淨的。"
	}
	return "擋下它的掃描項目：" + strings.Join(s.codes, "、") + "。"
}

func scanVerdict(scan packageScan) (string, error) {
	if !scan.scanned {
		return ReasonScanUnavailable, scanRefusal{unscanned: true}
	}
	if !scan.report.Blocked {
		return "", nil
	}
	return ReasonScanBlocked, scanRefusal{codes: blockingCodes(scan.report)}
}

func blockingCodes(report skillpkg.Report) []string {
	var codes []string
	for _, f := range report.Findings {
		if f.Severity == skillpkg.SeverityError && !slices.Contains(codes, f.Code) {
			codes = append(codes, f.Code)
		}
	}
	sort.Strings(codes)
	return codes
}

func requireScanNotBlocking(scan packageScan) error {
	reason, err := scanVerdict(scan)
	if err != nil {
		return refused(reason, err)
	}
	return nil
}

func runSlotVerdict(active int64) error {
	if active < MaxConcurrentRunsPerWorkspace {
		return nil
	}
	return refused(ReasonWorkspaceConcurrency,
		fmt.Errorf("%w: %d of %d in progress; wait for one to finish or cancel it",
			ErrRunLimitReached, active, MaxConcurrentRunsPerWorkspace))
}

func (s *Service) requireRunSlot(ctx context.Context, q *gen.Queries, workspaceID pgtype.UUID) error {
	if err := q.LockWorkspaceRunSlots(ctx, pgconv.UUIDString(workspaceID)); err != nil {
		return err
	}
	active, err := q.CountActiveRuns(ctx, workspaceID)
	if err != nil {
		return err
	}
	return runSlotVerdict(active)
}
