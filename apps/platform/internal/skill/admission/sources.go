package ingest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/audit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

// Source is the workspace-scoped provenance Catalog may display.
type Source struct {
	SourceType       string
	SourceURL        *string
	SourceRef        *string
	ContentHash      string
	FetchedAt        pgtype.Timestamptz
	LastCheckedAt    pgtype.Timestamptz
	UnavailableSince pgtype.Timestamptz
	// The generation record (GEN-006). All three are nil for git and upload, and
	// all three are set together for a generated package — 0037 has a CHECK
	// saying so, because a half-filled record does not reproduce anything.
	TaskDescription        *string
	GeneratorModel         *string
	GeneratorPromptVersion *string
	// GenerationInputs is 0055's `generation_inputs` jsonb as stored (ADR-066):
	// nil for every source that had no diagram and no reference skills. Bytes,
	// not a struct — the shape is declared once, by the writer in generate.go,
	// and the read side hands it on without re-declaring it (04 丙-159).
	GenerationInputs []byte
}

// ReadSource keeps the generated skill_sources row inside its owner.
func (s *Service) ReadSource(ctx context.Context, workspaceID, sourceID pgtype.UUID) (Source, bool, error) {
	row, err := gen.New(s.Pool).GetSkillSource(ctx, gen.GetSkillSourceParams{
		ID: sourceID, WorkspaceID: workspaceID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Source{}, false, nil
	}
	if err != nil {
		return Source{}, false, err
	}
	return Source{
		SourceType: row.SourceType, SourceURL: row.SourceUrl, SourceRef: row.SourceRef,
		ContentHash: row.ContentHash, FetchedAt: row.FetchedAt,
		LastCheckedAt: row.LastCheckedAt, UnavailableSince: row.UnavailableSince,
		TaskDescription: row.TaskDescription, GeneratorModel: row.GeneratorModel,
		GeneratorPromptVersion: row.GeneratorPromptVersion,
		GenerationInputs:       row.GenerationInputs,
	}, true, nil
}

// LineageSource is the ingest-owned source provenance view packaging may publish.
type LineageSource struct {
	SourceType  string
	SourceURL   *string
	SourceRef   *string
	ContentHash string
	FetchedAt   pgtype.Timestamptz
}

// SourceLineage keeps the generated skill_sources row inside its owner.
func (s *Service) SourceLineage(ctx context.Context, sourceID pgtype.UUID) (LineageSource, error) {
	row, err := gen.New(s.Pool).GetLineageSource(ctx, sourceID)
	if err != nil {
		return LineageSource{}, err
	}
	return LineageSource{
		SourceType:  row.SourceType,
		SourceURL:   row.SourceUrl,
		SourceRef:   row.SourceRef,
		ContentHash: row.ContentHash,
		FetchedAt:   row.FetchedAt,
	}, nil
}

// CheckSources asks two questions about every imported source: is the URL still
// there, and does it still hold what we imported (INGEST-010 外部內容失效／來源更新,
// 02:CONTENT-009, 02:SEC-007 第 2 條).
//
// The second question is the one the acceptance criteria actually name -- 「以重抓
// 並與保存的內容雜湊比對進行」 -- and until 2026-08-26 this function only asked the
// first, with one HEAD. A HEAD answers "is this URL still there". It does not
// answer "is it still the same thing", so deletion was caught and REWRITE and
// LICENCE CHANGE were not, and those last two are what SEC-007 exists for. The
// value to compare against was already on the row: content_hash, written by every
// import and read by nothing.
//
// It only marks. Nothing is unpublished automatically: an upstream repo that
// 404s today may be a rename, a rate limit or a transient outage, and the content
// we hold is a validated immutable snapshot that stays valid either way (iron
// rule 4). The same goes double for a changed hash -- upstream moving on is
// normal, and what a rewrite means for OUR copy is a licence question, not a
// sweep's. Deciding what to do about either is the manual takedown path, on
// purpose.
//
// Callable, bounded and idempotent -- a second run over the same rows writes the
// same marks. Scheduling belongs to deployment, not here.
func (s *Service) CheckSources(ctx context.Context, limit int32) (checked, unavailable, changed int, err error) {
	if s.Fetcher == nil {
		return 0, 0, 0, fmt.Errorf("%w: url import not configured", ErrFetch)
	}
	q := gen.New(s.Pool)
	rows, err := q.ListSourcesToCheck(ctx, limit)
	if err != nil {
		return 0, 0, 0, err
	}
	for _, row := range rows {
		available, contentChanged := true, false
		if err := s.Fetcher.Probe(ctx, *row.SourceUrl); err != nil {
			slog.Info("import source unavailable", "url", *row.SourceUrl, "error", err)
			available = false
			unavailable++
		} else {
			contentChanged = s.contentDiffers(ctx, row)
			if contentChanged {
				changed++
			}
		}
		if err := s.markChecked(ctx, q, row, available, contentChanged); err != nil {
			return checked, unavailable, changed, err
		}
		checked++
	}
	// One upstream changing is content news. EVERY upstream changing in the same
	// sweep is not: it is the archive generator. GitHub has altered how it builds
	// repo zips before, and the day it does so again every pinned hash in this
	// table stops matching at once with nothing upstream having been edited.
	// Saying so here costs a line, and it is the difference between reading the
	// next morning's audit rows as a mass licence event or as what it is.
	if changed > 1 && changed == checked {
		slog.Warn("every source in this sweep hashed differently; suspect the archive generator, not the content",
			"checked", checked, "changed", changed)
	}
	return checked, unavailable, changed, nil
}

// contentDiffers re-downloads the source and compares the hash with the one this
// workspace holds. Failure to answer is not a difference: a fetch that errors
// leaves the row alone, because "we could not look" must not enter the record as
// "it changed" -- that edge is written once and never cleared.
//
// Already-marked rows are skipped rather than re-fetched. The mark is an edge and
// the edge has been taken, so the download would buy nothing and would cost a
// full archive on every sweep forever.
//
// ponytail: re-downloads the whole archive to hash it, because that is what
// import hashed (prepare() in service.go: sha256 over the raw bytes) and
// comparing against a differently-derived hash compares nothing. Bounded by the
// fetcher's own size ceiling and by `limit`. If this ever costs real bandwidth,
// the move is a cheap pre-check -- ETag or Last-Modified on the HEAD that already
// happens -- not a second definition of the hash.
func (s *Service) contentDiffers(ctx context.Context, row gen.ListSourcesToCheckRow) bool {
	if row.ContentChangedAt.Valid || row.ContentHash == "" {
		return false
	}
	data, _, err := s.Fetcher.Fetch(ctx, *row.SourceUrl)
	if err != nil {
		slog.Info("import source could not be re-fetched for comparison", "url", *row.SourceUrl, "error", err)
		return false
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]) != row.ContentHash
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
	ctx context.Context, q *gen.Queries, row gen.ListSourcesToCheckRow, available, contentChanged bool,
) error {
	wasUnavailable := row.UnavailableSince.Valid
	if wasUnavailable == !available && !contentChanged {
		// Same answer as last time. One statement, no transaction to open.
		return q.MarkSourceChecked(ctx, gen.MarkSourceCheckedParams{
			ID: row.ID, Available: available, ContentChanged: false,
		})
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
		ID: row.ID, Available: available, ContentChanged: contentChanged,
	}); err != nil {
		return err
	}
	// No URL in the metadata: it is on the skill_sources row these events point at,
	// and an audit event carries identifiers rather than content (iron rule 11).
	// Actor is zero — nobody asked for this, the sweep found it.
	logEdge := func(action string) error {
		return audit.Log(ctx, tx, audit.Event{
			Workspace:    row.WorkspaceID,
			Action:       action,
			ResourceType: audit.ResourceImportSource,
			ResourceID:   row.ID,
		})
	}
	// Availability and content are separate findings and both can land in the same
	// sweep: a source that came back is allowed to have come back different. Two
	// events rather than one with a field, for the reason the pair above is a pair
	// -- "this source no longer holds what we imported" is what somebody searches
	// for, not a filter on something else.
	if wasUnavailable != !available {
		action := audit.ActionSourceRestored
		if !available {
			action = audit.ActionSourceUnavailable
		}
		if err := logEdge(action); err != nil {
			return err
		}
	}
	if contentChanged {
		if err := logEdge(audit.ActionSourceChanged); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// Probe reports whether rawURL still resolves, without downloading the package.
// Same allow list as Fetch: a source that is no longer on it counts as
// unavailable rather than being probed, because we could not re-import it
// anyway.
func (f *URLFetcher) Probe(ctx context.Context, rawURL string) error {
	normalized, err := f.Normalize(rawURL)
	if err != nil {
		return err
	}
	u, _ := url.Parse(normalized)
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, u.String(), nil)
	if err != nil {
		return fmt.Errorf("%w: invalid URL", ErrFetch)
	}
	resp, err := f.do(req) // same allow list, redirect budget and dial guard as download
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("%w: source returned status %d", ErrFetch, resp.StatusCode)
	}
	return nil
}
