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
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/audit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

type SourceType string

const (
	SourceGit       SourceType = "git"
	SourceUpload    SourceType = "upload"
	SourceGenerated SourceType = "generated"
)

func AllSourceTypes() []SourceType {
	return []SourceType{SourceGit, SourceUpload, SourceGenerated}
}

type Source struct {
	SourceType       string
	SourceURL        *string
	SourceRef        *string
	ContentHash      string
	FetchedAt        pgtype.Timestamptz
	LastCheckedAt    pgtype.Timestamptz
	UnavailableSince pgtype.Timestamptz

	TaskDescription        *string
	GeneratorModel         *string
	GeneratorPromptVersion *string

	GenerationInputs []byte
}

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

type LineageSource struct {
	SourceType  string
	SourceURL   *string
	SourceRef   *string
	ContentHash string
	FetchedAt   pgtype.Timestamptz
}

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

func (s *Service) OldestSourceCheck(ctx context.Context) (pgtype.Timestamptz, error) {
	return gen.New(s.Pool).OldestSourceCheck(ctx, string(SourceGit))
}

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

	if changed > 1 && changed == checked {
		slog.Warn("every source in this sweep hashed differently; suspect the archive generator, not the content",
			"checked", checked, "changed", changed)
	}
	return checked, unavailable, changed, nil
}

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

func checkedSource(row gen.ListSourcesToCheckRow, available, contentChanged bool, now time.Time) gen.MarkSourceCheckedParams {
	checked := gen.MarkSourceCheckedParams{
		ID: row.ID, UnavailableSince: row.UnavailableSince, ContentChangedAt: row.ContentChangedAt,
	}
	switch {
	case available:
		checked.UnavailableSince = pgtype.Timestamptz{}
	case !row.UnavailableSince.Valid:
		checked.UnavailableSince = pgtype.Timestamptz{Time: now, Valid: true}
	}
	if contentChanged && !row.ContentChangedAt.Valid {
		checked.ContentChangedAt = pgtype.Timestamptz{Time: now, Valid: true}
	}
	return checked
}

func (s *Service) markChecked(
	ctx context.Context, q *gen.Queries, row gen.ListSourcesToCheckRow, available, contentChanged bool,
) error {
	wasUnavailable := row.UnavailableSince.Valid
	if wasUnavailable == !available && !contentChanged {

		return q.MarkSourceChecked(ctx, checkedSource(row, available, false, time.Now()))
	}

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := q.WithTx(tx)
	if err := qtx.MarkSourceChecked(ctx, checkedSource(row, available, contentChanged, time.Now())); err != nil {
		return err
	}

	logEdge := func(action string) error {
		return audit.Log(ctx, tx, audit.Event{
			Workspace:    row.WorkspaceID,
			Action:       action,
			ResourceType: audit.ResourceImportSource,
			ResourceID:   row.ID,
		})
	}

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
	resp, err := f.do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("%w: source returned status %d", ErrFetch, resp.StatusCode)
	}
	return nil
}
