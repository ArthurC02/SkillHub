// Package ingest imports skill packages (SKILL-001, INGEST-002). Packages are
// analyzed statically only — nothing inside is executed (iron rule 1) — and
// accepted content lands as an immutable skill version plus the original
// archive in object storage (ADR-003).
package ingest

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/services/platform/internal/llmclient"
	"github.com/ArthurC02/skillhub/services/platform/internal/platform/db/gen"
	"github.com/ArthurC02/skillhub/services/platform/internal/skillpkg"
)

// MaxZipBytes caps the uploaded archive.
// ponytail: flat cap until PDM-005 fixes real limits.
const MaxZipBytes = 32 << 20 // 32 MiB

// maxUnpackedBytes caps total declared uncompressed size (zip bombs,
// ADR-007 壓縮炸彈). Var only so tests can lower it.
var maxUnpackedBytes = uint64(256 << 20) // 256 MiB

// ErrBadArchive marks input that is not an acceptable zip at all (as opposed
// to a well-formed package that fails validation).
var ErrBadArchive = errors.New("bad archive")

// ObjectStore is the slice of object storage ingest needs. Get serves the
// enrichment backfill, which recomputes from the stored package.
type ObjectStore interface {
	Put(ctx context.Context, key string, data []byte) error
	Get(ctx context.Context, key string) ([]byte, error)
}

type Service struct {
	Pool    *pgxpool.Pool
	Store   ObjectStore
	Fetcher *URLFetcher       // nil disables URL import
	LLM     *llmclient.Client // nil disables index-time enrichment (documents land pending)
}

// Result reports one upload. When Report.Blocked is true nothing was stored
// and the zero Skill/Version values are meaningless.
type Result struct {
	Report    skillpkg.Report
	Skill     gen.Skill
	Version   gen.SkillVersion
	Duplicate bool // same content already existed as a version of this skill
}

// sourceMeta records where a package came from (INGEST-004).
type sourceMeta struct {
	Type string  // 'upload' or 'git'
	URL  *string // original URL for git imports
	Ref  *string // branch, tag, or commit when known
}

// UploadZip validates data as an Agent Skills package and, if it passes,
// stores the archive and records source + version for ws.
func (s *Service) UploadZip(ctx context.Context, ws gen.Workspace, data []byte) (Result, error) {
	return s.importZip(ctx, ws, data, sourceMeta{Type: "upload"})
}

// ImportURL fetches a package from an allow-listed URL (INGEST-001) and runs
// it through the same import pipeline as uploads.
func (s *Service) ImportURL(ctx context.Context, ws gen.Workspace, rawURL string) (Result, error) {
	if s.Fetcher == nil {
		return Result{}, fmt.Errorf("%w: url import not configured", ErrFetch)
	}
	data, ref, err := s.Fetcher.Fetch(ctx, rawURL)
	if err != nil {
		return Result{}, err
	}
	meta := sourceMeta{Type: "git", URL: &rawURL}
	if ref != "" {
		meta.Ref = &ref
	}
	return s.importZip(ctx, ws, data, meta)
}

// preparedPackage is a validated, stored package awaiting database rows.
type preparedPackage struct {
	report      skillpkg.Report
	contentHash string
	objectKey   string
	manifest    []byte
	// skillMD and fileTree are the enrichment inputs (ADR-013 §1). They are
	// read here, not re-read later, because this is the only point that already
	// has the archive open. Both are untrusted package content and are only ever
	// passed to the LLM service as data (iron rule 1).
	skillMD  string
	fileTree []string
}

// Caps on what the enrichment call carries, matching llm-internal.yaml.
const (
	maxEnrichMDBytes = 200_000
	maxEnrichFiles   = 500
)

// readPackage validates data and collects the enrichment inputs. Shared by
// import (which then stores the archive) and by the backfill (which read the
// archive back out of storage).
func readPackage(data []byte) (preparedPackage, error) {
	fsys, err := packageFS(data)
	if err != nil {
		return preparedPackage{}, err
	}
	p := preparedPackage{report: skillpkg.Validate(fsys)}
	if p.report.Blocked {
		return p, nil
	}
	if md, err := fs.ReadFile(fsys, "SKILL.md"); err == nil {
		if len(md) > maxEnrichMDBytes {
			md = md[:maxEnrichMDBytes]
		}
		// Truncation can land mid-rune, and invalid UTF-8 does not survive JSON.
		p.skillMD = strings.ToValidUTF8(string(md), "")
	}
	_ = fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || len(p.fileTree) >= maxEnrichFiles {
			return nil //nolint:nilerr // unreadable entries are skipped, not fatal
		}
		p.fileTree = append(p.fileTree, path)
		return nil
	})
	return p, nil
}

// prepare validates data and stores the archive. A blocked report comes back
// with a nil error; the caller returns it to the client as findings.
func (s *Service) prepare(ctx context.Context, data []byte) (preparedPackage, error) {
	p, err := readPackage(data)
	if err != nil || p.report.Blocked {
		return p, err
	}

	sum := sha256.Sum256(data)
	p.contentHash = hex.EncodeToString(sum[:])
	p.objectKey = "packages/" + p.contentHash + ".zip"
	if p.manifest, err = json.Marshal(p.report.Manifest); err != nil {
		return preparedPackage{}, err
	}
	// Content-addressed put is idempotent, so storing before the DB commit
	// means a failed transaction leaves only a harmless orphan object.
	if err := s.Store.Put(ctx, p.objectKey, data); err != nil {
		return preparedPackage{}, err
	}
	return p, nil
}

func (s *Service) importZip(ctx context.Context, ws gen.Workspace, data []byte, src sourceMeta) (Result, error) {
	p, err := s.prepare(ctx, data)
	if err != nil || p.report.Blocked {
		return Result{Report: p.report}, err
	}
	res := Result{Report: p.report}

	// Index-time enrichment runs before the transaction opens (ADR-013 §1).
	// ponytail: re-importing byte-identical content pays for one enrichment call
	// that persistVersion then discards as a duplicate. Add a pre-transaction
	// content-hash lookup if duplicate imports ever show up in the model bill.
	e := s.enrichPackage(ctx, p)

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Result{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := gen.New(tx)

	skill, err := q.GetSkillByName(ctx, gen.GetSkillByNameParams{
		WorkspaceID: ws.ID, Name: p.report.Manifest.Name,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		skill, err = q.CreateSkill(ctx, gen.CreateSkillParams{
			WorkspaceID: ws.ID,
			Name:        p.report.Manifest.Name,
			Summary:     &p.report.Manifest.Description,
		})
	}
	if err != nil {
		return Result{}, err
	}
	res.Skill = skill

	res.Version, res.Duplicate, err = s.persistVersion(ctx, q, ws, skill, p, src, e)
	if err != nil {
		return Result{}, err
	}
	return res, tx.Commit(ctx)
}

// ErrSkillNotFound: target skill is not visible in the caller's workspace.
var ErrSkillNotFound = errors.New("skill not found")

// SaveVersion stores data as the next immutable version of an existing skill
// (WS-002). The skills row keeps its name; the manifest inside the version is
// the snapshot's truth. Existing versions are never touched (iron rule 4).
func (s *Service) SaveVersion(ctx context.Context, ws gen.Workspace, skillID pgtype.UUID, data []byte) (Result, error) {
	p, err := s.prepare(ctx, data)
	if err != nil || p.report.Blocked {
		return Result{Report: p.report}, err
	}
	res := Result{Report: p.report}

	e := s.enrichPackage(ctx, p) // outside the transaction; see importZip

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Result{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := gen.New(tx)

	skill, err := q.GetSkill(ctx, gen.GetSkillParams{ID: skillID, WorkspaceID: ws.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return Result{}, ErrSkillNotFound
	}
	if err != nil {
		return Result{}, err
	}
	res.Skill = skill

	res.Version, res.Duplicate, err = s.persistVersion(ctx, q, ws, skill, p, sourceMeta{Type: "upload"}, e)
	if err != nil {
		return Result{}, err
	}
	if !res.Duplicate {
		if err := q.UpdateSkillSummary(ctx, gen.UpdateSkillSummaryParams{
			ID: skill.ID, WorkspaceID: ws.ID, Summary: &p.report.Manifest.Description,
		}); err != nil {
			return Result{}, err
		}
	}
	return res, tx.Commit(ctx)
}

// persistVersion writes the dedupe-checked version row, its source row, and
// the search projection inside the caller's transaction.
func (s *Service) persistVersion(ctx context.Context, q *gen.Queries, ws gen.Workspace, skill gen.Skill, p preparedPackage, src sourceMeta, e enrichment) (gen.SkillVersion, bool, error) {
	if existing, err := q.GetVersionBySkillAndHash(ctx, gen.GetVersionBySkillAndHashParams{
		SkillID: skill.ID, ContentHash: p.contentHash,
	}); err == nil {
		// INGEST-005: identical content never overwrites or duplicates.
		return existing, true, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return gen.SkillVersion{}, false, err
	}

	source, err := q.CreateSkillSource(ctx, gen.CreateSkillSourceParams{
		WorkspaceID: ws.ID,
		SourceType:  src.Type,
		SourceUrl:   src.URL,
		SourceRef:   src.Ref,
		ContentHash: p.contentHash,
		FetchedAt:   pgtype.Timestamptz{Time: time.Now(), Valid: true},
	})
	if err != nil {
		return gen.SkillVersion{}, false, err
	}

	// The resolved license, not the manifest field: skillpkg falls back to a
	// LICENSE file in the package, then to a repository-level one carried in by
	// the packer, when the frontmatter declares nothing — and that fallback is
	// the only thing 37 of the 45 seed packages had (import-report.md §4 Top-1).
	// The tier travels with the expression (ADR-021): "MIT" from frontmatter and
	// "MIT" off a repo-root file are not the same claim, and DISC-003 has to show
	// the difference rather than flatten it into one string.
	var license, licenseSource *string
	if l := p.report.LicenseExpression; l != "" {
		license = &l
		s := p.report.LicenseSource
		licenseSource = &s
	}
	version, err := q.CreateSkillVersion(ctx, gen.CreateSkillVersionParams{
		WorkspaceID:       ws.ID,
		SkillID:           skill.ID,
		SourceID:          source.ID,
		ContentHash:       p.contentHash,
		PackageObjectKey:  p.objectKey,
		Manifest:          p.manifest,
		LicenseExpression: license,
		LicenseSource:     licenseSource,
	})
	if err != nil {
		return gen.SkillVersion{}, false, err
	}

	// Search projection updates in the same transaction (INGEST-009): same
	// database, so consistency is free; full rebuilds go through cmd/reindex.
	// The document keeps the skills row's name (fork names differ from their
	// manifest) but takes the newest description and enrichment.
	if err := upsertProjection(ctx, q, skill.ID, ws.ID, skill.Name, e); err != nil {
		return gen.SkillVersion{}, false, err
	}
	return version, false, nil
}

// upsertProjection writes one search document. Shared by import and by the
// enrichment backfill so both produce identical rows.
func upsertProjection(ctx context.Context, q *gen.Queries, skillID, workspaceID pgtype.UUID, name string, e enrichment) error {
	return q.UpsertSearchDocumentEnriched(ctx, gen.UpsertSearchDocumentEnrichedParams{
		SkillID:                 skillID,
		WorkspaceID:             workspaceID,
		Name:                    name,
		Summary:                 e.summary,
		EnrichedSummary:         e.enrichedSummary,
		TaskExamples:            e.taskExamples,
		Tags:                    e.tags,
		Embedding:               e.embedding,
		EnrichmentStatus:        e.status,
		EnrichmentModel:         e.model,
		EnrichmentPromptVersion: e.promptVersion,
	})
}

// ReindexPending re-runs index-time enrichment for documents the import path
// left pending (INGEST-009 重新索引, ADR-013 §1). Manual and bounded: this is
// the operator's catch-up tool, not a retry scheduler. Iron rule 6 puts the
// retry decision in Go, and for this batch that decision is "when someone runs
// reindex".
//
// Idempotent — a document that enriches successfully leaves the worklist, and
// one that fails again stays exactly as it was.
func (s *Service) ReindexPending(ctx context.Context, limit int32) (done, failed int, err error) {
	if s.LLM == nil {
		return 0, 0, errors.New("ingest: enrichment backfill needs an LLM service")
	}
	q := gen.New(s.Pool)
	rows, err := q.ListPendingEnrichment(ctx, limit)
	if err != nil {
		return 0, 0, err
	}
	for _, row := range rows {
		data, err := s.Store.Get(ctx, row.PackageObjectKey)
		if err != nil {
			slog.Warn("backfill: package unreadable", "skill", row.Name, "error", err)
			failed++
			continue
		}
		p, err := readPackage(data)
		if err != nil || p.report.Blocked {
			slog.Warn("backfill: stored package no longer parses", "skill", row.Name, "error", err)
			failed++
			continue
		}
		e := s.enrichPackage(ctx, p)
		if e.status != enrichmentEnriched {
			failed++ // enrichPackage already logged why; the row stays pending
			continue
		}
		if err := upsertProjection(ctx, q, row.SkillID, row.WorkspaceID, row.Name, e); err != nil {
			return done, failed, err
		}
		done++
	}
	return done, failed, nil
}

// packageFS opens the zip as a read-only fs.FS, rejecting bombs and locating
// the package root: SKILL.md at top level, or inside a single top-level
// directory (the shape GitHub archive downloads have).
func packageFS(data []byte) (fs.FS, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("%w: not a zip archive", ErrBadArchive)
	}
	var unpacked uint64
	for _, f := range zr.File {
		unpacked += f.UncompressedSize64
		if unpacked > maxUnpackedBytes {
			return nil, fmt.Errorf("%w: uncompressed content exceeds %d bytes", ErrBadArchive, maxUnpackedBytes)
		}
	}
	if _, err := fs.Stat(zr, "SKILL.md"); err == nil {
		return zr, nil
	}
	if dirs, err := fs.ReadDir(zr, "."); err == nil && len(dirs) == 1 && dirs[0].IsDir() {
		if sub, err := fs.Sub(zr, dirs[0].Name()); err == nil {
			return sub, nil
		}
	}
	return zr, nil // let Validate report skill-md-missing
}
