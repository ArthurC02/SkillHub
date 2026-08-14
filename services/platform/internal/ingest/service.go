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
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

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

// ObjectStore is the slice of object storage ingest needs.
type ObjectStore interface {
	Put(ctx context.Context, key string, data []byte) error
}

type Service struct {
	Pool    *pgxpool.Pool
	Store   ObjectStore
	Fetcher *URLFetcher // nil disables URL import
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
}

// prepare validates data and stores the archive. A blocked report comes back
// with a nil error; the caller returns it to the client as findings.
func (s *Service) prepare(ctx context.Context, data []byte) (preparedPackage, error) {
	fsys, err := packageFS(data)
	if err != nil {
		return preparedPackage{}, err
	}
	p := preparedPackage{report: skillpkg.Validate(fsys)}
	if p.report.Blocked {
		return p, nil
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

	res.Version, res.Duplicate, err = s.persistVersion(ctx, q, ws, skill, p, src)
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

	res.Version, res.Duplicate, err = s.persistVersion(ctx, q, ws, skill, p, sourceMeta{Type: "upload"})
	if err != nil {
		return Result{}, err
	}
	if !res.Duplicate {
		if err := q.UpdateSkillSummary(ctx, gen.UpdateSkillSummaryParams{
			ID: skill.ID, Summary: &p.report.Manifest.Description,
		}); err != nil {
			return Result{}, err
		}
	}
	return res, tx.Commit(ctx)
}

// persistVersion writes the dedupe-checked version row, its source row, and
// the search projection inside the caller's transaction.
func (s *Service) persistVersion(ctx context.Context, q *gen.Queries, ws gen.Workspace, skill gen.Skill, p preparedPackage, src sourceMeta) (gen.SkillVersion, bool, error) {
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

	var license *string
	if l := p.report.Manifest.License; l != "" {
		license = &l
	}
	version, err := q.CreateSkillVersion(ctx, gen.CreateSkillVersionParams{
		WorkspaceID:       ws.ID,
		SkillID:           skill.ID,
		SourceID:          source.ID,
		ContentHash:       p.contentHash,
		PackageObjectKey:  p.objectKey,
		Manifest:          p.manifest,
		LicenseExpression: license,
	})
	if err != nil {
		return gen.SkillVersion{}, false, err
	}

	// Search projection updates in the same transaction (INGEST-009): same
	// database, so consistency is free; full rebuilds go through cmd/reindex.
	// The document keeps the skills row's name (fork names differ from their
	// manifest) but takes the newest description.
	if err := q.UpsertSearchDocument(ctx, gen.UpsertSearchDocumentParams{
		SkillID:     skill.ID,
		WorkspaceID: ws.ID,
		Name:        skill.Name,
		Summary:     p.report.Manifest.Description,
	}); err != nil {
		return gen.SkillVersion{}, false, err
	}
	return version, false, nil
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
