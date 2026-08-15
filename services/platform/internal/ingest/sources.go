package ingest

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"

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
		if err := q.MarkSourceChecked(ctx, gen.MarkSourceCheckedParams{
			ID: row.ID, Available: available,
		}); err != nil {
			return checked, unavailable, err
		}
		checked++
	}
	return checked, unavailable, nil
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
