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
	Pool  *pgxpool.Pool
	Store ObjectStore
}

// Result reports one upload. When Report.Blocked is true nothing was stored
// and the zero Skill/Version values are meaningless.
type Result struct {
	Report    skillpkg.Report
	Skill     gen.Skill
	Version   gen.SkillVersion
	Duplicate bool // same content already existed as a version of this skill
}

// UploadZip validates data as an Agent Skills package and, if it passes,
// stores the archive and records source + version for ws.
func (s *Service) UploadZip(ctx context.Context, ws gen.Workspace, data []byte) (Result, error) {
	fsys, err := packageFS(data)
	if err != nil {
		return Result{}, err
	}

	res := Result{Report: skillpkg.Validate(fsys)}
	if res.Report.Blocked {
		return res, nil
	}

	sum := sha256.Sum256(data)
	contentHash := hex.EncodeToString(sum[:])
	objectKey := "packages/" + contentHash + ".zip"
	manifestJSON, err := json.Marshal(res.Report.Manifest)
	if err != nil {
		return Result{}, err
	}

	// Content-addressed put is idempotent, so storing before the DB commit
	// means a failed transaction leaves only a harmless orphan object.
	if err := s.Store.Put(ctx, objectKey, data); err != nil {
		return Result{}, err
	}

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Result{}, err
	}
	defer tx.Rollback(ctx)
	q := gen.New(tx)

	skill, err := q.GetSkillByName(ctx, gen.GetSkillByNameParams{
		WorkspaceID: ws.ID, Name: res.Report.Manifest.Name,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		skill, err = q.CreateSkill(ctx, gen.CreateSkillParams{
			WorkspaceID: ws.ID,
			Name:        res.Report.Manifest.Name,
			Summary:     &res.Report.Manifest.Description,
		})
	}
	if err != nil {
		return Result{}, err
	}
	res.Skill = skill

	if existing, err := q.GetVersionBySkillAndHash(ctx, gen.GetVersionBySkillAndHashParams{
		SkillID: skill.ID, ContentHash: contentHash,
	}); err == nil {
		// INGEST-005: identical content never overwrites or duplicates.
		res.Version, res.Duplicate = existing, true
		return res, tx.Commit(ctx)
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return Result{}, err
	}

	source, err := q.CreateSkillSource(ctx, gen.CreateSkillSourceParams{
		WorkspaceID: ws.ID,
		SourceType:  "upload",
		ContentHash: contentHash,
		FetchedAt:   pgtype.Timestamptz{Time: time.Now(), Valid: true},
	})
	if err != nil {
		return Result{}, err
	}

	var license *string
	if l := res.Report.Manifest.License; l != "" {
		license = &l
	}
	res.Version, err = q.CreateSkillVersion(ctx, gen.CreateSkillVersionParams{
		WorkspaceID:       ws.ID,
		SkillID:           skill.ID,
		SourceID:          source.ID,
		ContentHash:       contentHash,
		PackageObjectKey:  objectKey,
		Manifest:          manifestJSON,
		LicenseExpression: license,
	})
	if err != nil {
		return Result{}, err
	}
	return res, tx.Commit(ctx)
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
