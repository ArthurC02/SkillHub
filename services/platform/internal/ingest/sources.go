package ingest

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/ArthurC02/skillhub/services/platform/internal/audit"
	"github.com/ArthurC02/skillhub/services/platform/internal/platform/db/gen"
)

// CheckSources probes the recorded source URL of imported packages and records
// which ones no longer resolve (INGEST-010 外部內容失效／來源更新).
//
// It only marks. Nothing is unpublished automatically: an upstream repo that
// 404s today may be a rename, a rate limit or a transient outage, and the
// content we hold is a validated immutable snapshot that stays valid either way
// (iron rule 4). Deciding what to do about a source that has been gone for a
// while is the manual takedown path, on purpose.
//
// Callable, bounded and idempotent — a second run over the same rows writes the
// same marks. Scheduling belongs to deployment, not here.
func (s *Service) CheckSources(ctx context.Context, limit int32) (checked, unavailable int, err error) {
	if s.Fetcher == nil {
		return 0, 0, fmt.Errorf("%w: url import not configured", ErrFetch)
	}
	q := gen.New(s.Pool)
	rows, err := q.ListSourcesToCheck(ctx, limit)
	if err != nil {
		return 0, 0, err
	}
	for _, row := range rows {
		available := true
		if err := s.Fetcher.Probe(ctx, *row.SourceUrl); err != nil {
			slog.Info("import source unavailable", "url", *row.SourceUrl, "error", err)
			available = false
			unavailable++
		}
		if err := s.markChecked(ctx, q, row, available); err != nil {
			return checked, unavailable, err
		}
		checked++
	}
	return checked, unavailable, nil
}

// markChecked records this probe's answer, and audits it only when the answer
// changed (NFR-001, INGEST-010).
//
// The mark itself is idempotent and uninteresting: the sweep runs on a schedule
// and most rows answer the same thing they answered last time. What an audit
// trail wants is the two edges — the probe on which a source first stopped
// resolving, and the one on which it came back — because those are the moments a
// user's imported content changed availability without the user doing anything.
// A row per probe would be the same two facts buried under thousands of repeats,
// and the 400 day retention of PDM-006 §6 is affordable precisely because these
// rows are rare.
//
// The edge is read from unavailable_since, which is the column that already
// carries this state: it is set on the first failure and cleared on the first
// success (see MarkSourceChecked), so "was it failing before this probe" is
// exactly its validity, and no second source of truth is introduced.
func (s *Service) markChecked(
	ctx context.Context, q *gen.Queries, row gen.ListSourcesToCheckRow, available bool,
) error {
	wasUnavailable := row.UnavailableSince.Valid
	if wasUnavailable == !available {
		// Same answer as last time. One statement, no transaction to open.
		return q.MarkSourceChecked(ctx, gen.MarkSourceCheckedParams{ID: row.ID, Available: available})
	}

	// The state changed, so the mark and the event that announces it commit
	// together (iron rule 9): an audit row claiming a source went away while the
	// column still says it is fine would be worse than no row at all.
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := q.WithTx(tx)
	if err := qtx.MarkSourceChecked(ctx, gen.MarkSourceCheckedParams{
		ID: row.ID, Available: available,
	}); err != nil {
		return err
	}
	action := audit.ActionSourceRestored
	if !available {
		action = audit.ActionSourceUnavailable
	}
	// No URL in the metadata: it is on the skill_sources row this event points at,
	// and an audit event carries identifiers rather than content (iron rule 11).
	// Actor is zero — nobody asked for this, the sweep found it.
	if err := audit.Log(ctx, qtx, audit.Event{
		Workspace:    row.WorkspaceID,
		Action:       action,
		ResourceType: audit.ResourceImportSource,
		ResourceID:   row.ID,
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Probe reports whether rawURL still resolves, without downloading the package.
// Same allow list as Fetch: a source that is no longer on it counts as
// unavailable rather than being probed, because we could not re-import it
// anyway.
func (f *URLFetcher) Probe(ctx context.Context, rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("%w: invalid URL", ErrFetch)
	}
	if err := f.checkURL(u); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, u.String(), nil)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrFetch, err)
	}
	client := *f.client()
	client.CheckRedirect = func(req *http.Request, _ []*http.Request) error {
		return f.checkURL(req.URL) // same allow list as download (SSRF)
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrFetch, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("%w: %s returned status %d", ErrFetch, rawURL, resp.StatusCode)
	}
	return nil
}
