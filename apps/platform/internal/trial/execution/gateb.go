package run

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/metrics"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/ArthurC02/skillhub/apps/platform/internal/shared/skillpkg"
)

const MaxConcurrentRunsPerWorkspace = 2

var (
	ErrScanBlocked = errors.New("the skill version's static scan blocks it from running")

	ErrRunLimitReached = errors.New("this workspace already has the maximum number of runs in progress")

	ErrAccessRestricted = errors.New("this skill cannot be run while its source license is under review")
)

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
	if skill.AccessRestriction == nil || strings.TrimSpace(*skill.AccessRestriction) == "" {
		return nil
	}
	return refused("access_restricted", fmt.Errorf("%w (%s)", ErrAccessRestricted, *skill.AccessRestriction))
}

func (s *Service) packageReport(ctx context.Context, objectKey string) (skillpkg.Report, bool) {
	if s.store() == nil {
		return skillpkg.Report{}, false
	}
	data, err := s.store().Get(ctx, objectKey)
	if err != nil {
		return skillpkg.Report{}, false
	}
	fsys, err := skillpkg.PackageFS(data)
	if err != nil {
		return skillpkg.Report{}, false
	}
	return skillpkg.Validate(fsys), true
}

func (s *Service) requireScanNotBlocking(ctx context.Context, objectKey string) error {
	report, ok := s.packageReport(ctx, objectKey)
	if !ok {
		return refused("scan_unavailable", fmt.Errorf("%w: the package could not be scanned, "+
			"and an unscanned package is not treated as a clean one", ErrScanBlocked))
	}
	if !report.Blocked {
		return nil
	}

	codes := map[string]struct{}{}
	for _, f := range report.Findings {
		if f.Severity == skillpkg.SeverityError {
			codes[f.Code] = struct{}{}
		}
	}
	list := make([]string, 0, len(codes))
	for c := range codes {
		list = append(list, c)
	}
	sort.Strings(list)
	return refused("scan_blocked", fmt.Errorf("%w: %s", ErrScanBlocked, strings.Join(list, ", ")))
}

func (s *Service) requireRunSlot(ctx context.Context, q *gen.Queries, workspaceID pgtype.UUID) error {
	if err := q.LockWorkspaceRunSlots(ctx, pgconv.UUIDString(workspaceID)); err != nil {
		return err
	}
	active, err := q.CountActiveRuns(ctx, workspaceID)
	if err != nil {
		return err
	}
	if active >= MaxConcurrentRunsPerWorkspace {
		return refused("workspace_concurrency",
			fmt.Errorf("%w: %d of %d in progress; wait for one to finish or cancel it",
				ErrRunLimitReached, active, MaxConcurrentRunsPerWorkspace))
	}
	return nil
}
