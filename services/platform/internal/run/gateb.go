package run

// SEC-002 gate B: the checks a run must pass before it is allowed to start.
//
// The gate has four blocking conditions. Two of them landed with the features
// that own them and stay there:
//
//	permission summary not confirmed  -> preflight.go, requirePermissionConfirmation
//	capability beyond the provider    -> schedule.go, checkSchedulable
//
// The other two are here, because neither belongs to a feature - they are the
// gate itself:
//
//	blocking static scan result       -> requireScanNotBlocking (02:SEC-003)
//	workspace concurrency limit       -> requireRunSlot (PDM-005 §5.2)
//
// Both are fail-closed. SEC-002 says a check that cannot be performed counts as
// not passed, and neither of these has a "probably fine" answer: an unreadable
// package is not a clean one (DISC-004), and a concurrency count that could not
// be taken is not a count of zero.

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/services/platform/internal/ingest"
	"github.com/ArthurC02/skillhub/services/platform/internal/platform/db/gen"
	"github.com/ArthurC02/skillhub/services/platform/internal/platform/metrics"
	"github.com/ArthurC02/skillhub/services/platform/internal/skillpkg"
)

// MaxConcurrentRunsPerWorkspace is PDM-005 §5.2. It bounds what one workspace may
// have outstanding, which is a different quantity from Provider.ConcurrentRunSlots
// (how much a sandbox node can carry at once): the fleet could be idle and this
// limit would still apply.
const MaxConcurrentRunsPerWorkspace = 2

var (
	// ErrScanBlocked is a Skill Version whose static scan produced an error-level
	// finding, or whose package could not be scanned at all.
	ErrScanBlocked = errors.New("the skill version's static scan blocks it from running")
	// ErrRunLimitReached is the workspace concurrency ceiling.
	ErrRunLimitReached = errors.New("this workspace already has the maximum number of runs in progress")
	// ErrAccessRestricted is a skill whose materials are held back while a
	// licensing question about them is open (0023).
	ErrAccessRestricted = errors.New("this skill cannot be run while its source license is under review")
)

// requireNotAccessRestricted refuses a Run on materials under a licensing hold
// (0023, m2/anthropic-sa-license-memo.md 方案 C).
//
// Running is not display, but it is another copy: the package is unpacked into a
// sandbox and read there. The hold is on making copies, so it covers this too —
// and unlike display, a Run also produces output derived from the materials.
//
// Read from the run's own skill row rather than walked back through the fork
// lineage: the hold is copied onto a fork at fork time (registry.Fork), so the
// row in front of us already carries it, and a fork of a fork cannot shed it.
// A lineage walk would have to be written to terminate, and the version that
// forgot to would be the way around the gate.
func (s *Service) requireNotAccessRestricted(skill gen.Skill) error {
	if skill.AccessRestriction == nil || strings.TrimSpace(*skill.AccessRestriction) == "" {
		return nil
	}
	metrics.RunRefused.WithLabelValues("access_restricted").Inc()
	return fmt.Errorf("%w (%s)", ErrAccessRestricted, *skill.AccessRestriction)
}

// packageReport reads the stored package and validates it, without executing any
// of it (iron rule 1). ok is false when the scan could not be performed - no
// object store configured, bytes unreadable, or not a package - which every
// caller must treat as a distinct answer from "scanned and clean".
func (s *Service) packageReport(ctx context.Context, objectKey string) (skillpkg.Report, bool) {
	if s.store() == nil {
		return skillpkg.Report{}, false
	}
	data, err := s.store().Get(ctx, objectKey)
	if err != nil {
		return skillpkg.Report{}, false
	}
	fsys, err := ingest.PackageFS(data)
	if err != nil {
		return skillpkg.Report{}, false
	}
	return skillpkg.Validate(fsys), true
}

// requireScanNotBlocking is gate B's static scan condition (02:SEC-002, 02:SEC-003).
//
// The severity policy is not invented here: skillpkg already assigns every
// finding code a severity, and error-level is the level SKILL-002 defines as
// blocking. Import applies that same policy, so in the normal case this passes -
// which is the point. It catches the cases import cannot:
//
//   - the stored bytes changed, or stopped being readable, after import
//   - the version predates a finding code that is error-level today
//   - a version that entered the workspace by some path other than import
//
// Re-scanning rather than reading a stored verdict is deliberate. The only
// persisted scan projection is search_documents.scan, which is a nullable
// warning-count for the catalog UI and is explicitly documented as not a
// clearance; a gate reading it would be reading a cache of a decision, not the
// decision.
func (s *Service) requireScanNotBlocking(ctx context.Context, objectKey string) error {
	report, ok := s.packageReport(ctx, objectKey)
	if !ok {
		metrics.RunRefused.WithLabelValues("scan_unavailable").Inc()
		return fmt.Errorf("%w: the package could not be scanned, "+
			"and an unscanned package is not treated as a clean one", ErrScanBlocked)
	}
	if !report.Blocked {
		return nil
	}
	// The codes, not the messages: a message can quote package content, and this
	// string reaches an HTTP response.
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
	metrics.RunRefused.WithLabelValues("scan_blocked").Inc()
	return fmt.Errorf("%w: %s", ErrScanBlocked, strings.Join(list, ", "))
}

// requireRunSlot is gate B's concurrency condition (PDM-005 §5.2, limit 2).
//
// q must be the caller's transaction handle: the advisory lock it takes is held
// until that transaction commits, which is what stops two simultaneous requests
// from both counting one active run and both inserting a second.
//
// The workspace comes from the session (iron rule 3) - counting a workspace_id
// out of a request body would let a caller check somebody else's quota.
func (s *Service) requireRunSlot(ctx context.Context, q *gen.Queries, workspaceID pgtype.UUID) error {
	if err := q.LockWorkspaceRunSlots(ctx, uuidString(workspaceID)); err != nil {
		return err
	}
	active, err := q.CountActiveRuns(ctx, workspaceID)
	if err != nil {
		return err
	}
	if active >= MaxConcurrentRunsPerWorkspace {
		metrics.RunRefused.WithLabelValues("workspace_concurrency").Inc()
		return fmt.Errorf("%w: %d of %d in progress; wait for one to finish or cancel it",
			ErrRunLimitReached, active, MaxConcurrentRunsPerWorkspace)
	}
	return nil
}
