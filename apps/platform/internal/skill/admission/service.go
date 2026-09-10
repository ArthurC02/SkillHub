package ingest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/audit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/ArthurC02/skillhub/apps/platform/internal/product/entitlements"
	"github.com/ArthurC02/skillhub/apps/platform/internal/shared/skillpkg"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/library"
)

type ObjectStore interface {
	Put(ctx context.Context, key string, data []byte) error
	Get(ctx context.Context, key string) ([]byte, error)
}

type Service struct {
	Pool    *pgxpool.Pool
	Store   ObjectStore
	Fetcher *URLFetcher
	LLM     *llmclient.Client

	IndexSkill func(ctx context.Context, tx pgx.Tx, projection SkillProjection) error

	PendingEnrichments func(ctx context.Context, limit int32) ([]PendingEnrichment, error)

	Credit CostRecorder

	CreditCanStart func(ctx context.Context, workspaceID pgtype.UUID) (bool, error)

	GenerateQuota policy.QuotaLimits

	generating sync.Map

	References ReferenceReader
}

type SkillProjection struct {
	SkillID                 pgtype.UUID
	WorkspaceID             pgtype.UUID
	Name                    string
	Summary                 string
	EnrichedSummary         string
	TaskExamples            string
	Tags                    []byte
	Limitations             string
	Scan                    []byte
	Embedding               *pgvector.Vector
	EnrichmentStatus        string
	EnrichmentModel         *string
	EnrichmentPromptVersion *string
}

type PendingEnrichment struct {
	SkillID          pgtype.UUID
	WorkspaceID      pgtype.UUID
	Name             string
	PackageObjectKey string
}

func (s *Service) requireProjection() error {
	if s.IndexSkill == nil {
		return errors.New("ingest: search projection write not injected; refusing to write")
	}
	return nil
}

type Result struct {
	Report    skillpkg.Report
	Skill     registry.Skill
	Version   registry.Version
	Duplicate bool
}

func redistributionFor(ws identity.Workspace, src sourceMeta) string {
	if ws.IsCatalog {
		return ""
	}

	if src.Type == sourceGenerated {
		return registry.RedistributionGenerated
	}
	return registry.RedistributionSelfSupplied
}

const sourceGenerated = "generated"

type sourceMeta struct {
	Type string
	URL  *string
	Ref  *string

	TaskDescription        *string
	GeneratorModel         *string
	GeneratorPromptVersion *string

	CostUSD          *float64
	PromptTokens     int64
	CompletionTokens int64

	GenerationInputs []byte
}

func (s *Service) UploadZip(ctx context.Context, ws identity.Workspace, data []byte) (Result, error) {
	return s.importZip(ctx, ws, data, sourceMeta{Type: "upload"})
}

func (s *Service) ImportURL(ctx context.Context, ws identity.Workspace, rawURL string) (Result, error) {
	if s.Fetcher == nil {
		return Result{}, fmt.Errorf("%w: 這個部署沒有啟用「從網址匯入」。", ErrFetch)
	}
	sourceURL, err := s.Fetcher.Normalize(rawURL)
	if err != nil {
		return Result{}, err
	}
	data, ref, err := s.Fetcher.Fetch(ctx, sourceURL)
	if err != nil {
		return Result{}, err
	}
	meta := sourceMeta{Type: "git", URL: &sourceURL}
	if ref != "" {
		meta.Ref = &ref
	}
	return s.importZip(ctx, ws, data, meta)
}

type preparedPackage struct {
	report      skillpkg.Report
	contentHash string
	objectKey   string

	skillMD  string
	fileTree []string
}

const (
	maxEnrichMDBytes = 200_000
	maxEnrichFiles   = 500
)

func readPackage(data []byte) (preparedPackage, error) {
	fsys, err := skillpkg.PackageFS(data)
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

func (s *Service) prepare(ctx context.Context, data []byte) (preparedPackage, error) {
	p, err := readPackage(data)
	if err != nil || p.report.Blocked {
		return p, err
	}

	sum := sha256.Sum256(data)
	p.contentHash = hex.EncodeToString(sum[:])
	p.objectKey = "packages/" + p.contentHash + ".zip"
	return p, nil
}

func (s *Service) beginPackageWrite(ctx context.Context, ws identity.Workspace, p preparedPackage, data []byte) (pgx.Tx, func(), error) {
	conn, err := s.Pool.Acquire(ctx)
	if err != nil {
		return nil, nil, err
	}
	workspaceLocked, objectLocked := false, false
	release := func() {
		unlockCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if objectLocked {
			if err := registry.UnlockPackageObject(unlockCtx, conn, p.objectKey); err != nil {
				slog.Error("package object lock could not be released; closing connection", "error", err)
				_ = conn.Hijack().Close(context.Background())
				return
			}
		}
		if workspaceLocked {
			if err := identity.UnlockObjectWrite(unlockCtx, conn, ws.ID); err != nil {
				slog.Error("workspace object write lock could not be released; closing connection", "error", err)
				_ = conn.Hijack().Close(context.Background())
				return
			}
		}
		conn.Release()
	}
	fail := func(err error) (pgx.Tx, func(), error) {
		release()
		return nil, nil, err
	}
	workspaceLocked, err = identity.LockObjectWrite(ctx, conn, ws.ID)
	if err != nil {
		return fail(err)
	}
	if err := registry.LockPackageObject(ctx, conn, p.objectKey); err != nil {
		return fail(err)
	}
	objectLocked = true

	if err := registry.TrackPackageObject(ctx, conn, p.objectKey); err != nil {
		return fail(err)
	}
	if err := s.Store.Put(ctx, p.objectKey, data); err != nil {
		return fail(err)
	}
	tx, err := conn.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	return tx, func() {
		_ = tx.Rollback(context.Background())
		release()
	}, nil
}

func (s *Service) importZip(ctx context.Context, ws identity.Workspace, data []byte, src sourceMeta) (Result, error) {
	return s.importZipWithCommit(ctx, ws, data, src, nil)
}

func (s *Service) importZipWithCommit(ctx context.Context, ws identity.Workspace, data []byte, src sourceMeta, after func(context.Context, pgx.Tx, Result) error) (Result, error) {
	p, err := s.prepare(ctx, data)
	if err != nil || p.report.Blocked {
		return Result{Report: p.report}, err
	}
	res := Result{Report: p.report}

	e := s.enrichPackage(ctx, p, ws.ID)

	tx, release, err := s.beginPackageWrite(ctx, ws, p, data)
	if err != nil {
		return Result{}, err
	}
	defer release()
	readSkill, found, err := registry.SkillByName(ctx, tx, ws.ID, p.report.Manifest.Name)
	var skill registry.Skill
	if !found && err == nil {
		skill, err = registry.CreateSkillFromPackage(ctx, tx, ws.ID, p.report, redistributionFor(ws, src))
	} else {
		skill = readSkill
	}
	if err != nil {
		return Result{}, err
	}

	if found && src.Type == sourceGenerated {
		return Result{}, fmt.Errorf("%w: %q", ErrGeneratedNameCollision, skill.Name)
	}
	res.Skill = skill

	res.Version, res.Duplicate, err = s.persistVersion(ctx, tx, ws, skill, p, src, e)
	if err != nil {
		return Result{}, err
	}
	importMeta := map[string]any{"source_type": src.Type}

	usageMeta(importMeta, src.CostUSD, src.PromptTokens, src.CompletionTokens)
	if err := auditVersion(ctx, tx, ws, audit.ActionSkillImport, res, importMeta); err != nil {
		return Result{}, err
	}
	if after != nil {
		if err := after(ctx, tx, res); err != nil {
			return Result{}, err
		}
	}
	return res, tx.Commit(ctx)
}

func auditVersion(ctx context.Context, tx pgx.Tx, ws identity.Workspace, action string, res Result, meta map[string]any) error {
	meta["skill_id"] = pgconv.UUIDString(res.Skill.ID)
	meta["duplicate"] = res.Duplicate
	meta["content_hash"] = res.Version.ContentHash
	return audit.Log(ctx, tx, audit.Event{
		Actor:        ws.OwnerUserID,
		Workspace:    ws.ID,
		Action:       action,
		ResourceType: audit.ResourceVersion,
		ResourceID:   res.Version.ID,
		Metadata:     meta,
	})
}

var ErrSkillNotFound = errors.New("skill not found")

func (s *Service) SaveVersion(ctx context.Context, ws identity.Workspace, skillID pgtype.UUID, data []byte) (Result, error) {
	p, err := s.prepare(ctx, data)
	if err != nil || p.report.Blocked {
		return Result{Report: p.report}, err
	}
	res := Result{Report: p.report}

	e := s.enrichPackage(ctx, p, ws.ID)

	tx, release, err := s.beginPackageWrite(ctx, ws, p, data)
	if err != nil {
		return Result{}, err
	}
	defer release()
	readSkill, found, err := registry.SkillByID(ctx, tx, ws.ID, skillID)
	if !found && err == nil {
		return Result{}, ErrSkillNotFound
	}
	if err != nil {
		return Result{}, err
	}
	skill := readSkill
	res.Skill = skill

	res.Version, res.Duplicate, err = s.persistVersion(ctx, tx, ws, skill, p, sourceMeta{Type: "upload"}, e)
	if err != nil {
		return Result{}, err
	}
	if !res.Duplicate {
		if err := registry.UpdateSummaryFromPackage(ctx, tx, skill.ID, ws.ID, p.report); err != nil {
			return Result{}, err
		}
	}
	if err := auditVersion(ctx, tx, ws, audit.ActionSkillVersionCreate, res, map[string]any{
		"version_number": res.Version.VersionNumber,
	}); err != nil {
		return Result{}, err
	}
	return res, tx.Commit(ctx)
}

func (s *Service) persistVersion(ctx context.Context, tx pgx.Tx, ws identity.Workspace, skill registry.Skill, p preparedPackage, src sourceMeta, e enrichment) (registry.Version, bool, error) {

	if err := s.requireProjection(); err != nil {
		return registry.Version{}, false, err
	}

	if skill.Redistribution == registry.RedistributionGenerated && src.Type != sourceGenerated {
		return registry.Version{}, false, fmt.Errorf("%w: %q", ErrGeneratedNameCollision, skill.Name)
	}
	q := gen.New(tx)
	if existing, found, err := registry.VersionByContent(ctx, tx, ws.ID, skill.ID, p.contentHash); err != nil {
		return registry.Version{}, false, err
	} else if found {

		return existing, true, nil
	}

	source, err := q.CreateSkillSource(ctx, gen.CreateSkillSourceParams{
		WorkspaceID: ws.ID,
		SourceType:  src.Type,
		SourceUrl:   src.URL,
		SourceRef:   src.Ref,
		ContentHash: p.contentHash,
		FetchedAt:   pgtype.Timestamptz{Time: time.Now(), Valid: true},

		TaskDescription:        src.TaskDescription,
		GeneratorModel:         src.GeneratorModel,
		GeneratorPromptVersion: src.GeneratorPromptVersion,
		GenerationInputs:       src.GenerationInputs,
	})
	if err != nil {
		return registry.Version{}, false, err
	}

	version, err := registry.CreateVersionFromPackage(ctx, tx, registry.NewVersion{
		WorkspaceID:      ws.ID,
		SkillID:          skill.ID,
		SourceID:         source.ID,
		ContentHash:      p.contentHash,
		PackageObjectKey: p.objectKey,
		Report:           p.report,
	})
	if err != nil {
		return registry.Version{}, false, err
	}

	if err := s.upsertProjection(ctx, tx, skill.ID, ws.ID, skill.Name, e); err != nil {
		return registry.Version{}, false, err
	}
	return version, false, nil
}

func (s *Service) upsertProjection(ctx context.Context, tx pgx.Tx, skillID, workspaceID pgtype.UUID, name string, e enrichment) error {
	return s.IndexSkill(ctx, tx, SkillProjection{
		SkillID:                 skillID,
		WorkspaceID:             workspaceID,
		Name:                    name,
		Summary:                 e.summary,
		EnrichedSummary:         e.enrichedSummary,
		TaskExamples:            e.taskExamples,
		Tags:                    e.tags,
		Limitations:             e.limitations,
		Scan:                    e.scan,
		Embedding:               e.embedding,
		EnrichmentStatus:        e.status,
		EnrichmentModel:         e.model,
		EnrichmentPromptVersion: e.promptVersion,
	})
}

func (s *Service) ReindexPending(ctx context.Context, limit int32) (done, failed int, err error) {
	if s.LLM == nil {
		return 0, 0, errors.New("ingest: enrichment backfill needs an LLM service")
	}

	if err := s.requireProjection(); err != nil {
		return 0, 0, err
	}
	if s.PendingEnrichments == nil {
		return 0, 0, errors.New("ingest: pending enrichment lister not injected; refusing backfill")
	}
	rows, err := s.PendingEnrichments(ctx, limit)
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
		e := s.enrichPackage(ctx, p, row.WorkspaceID)
		if e.status != enrichmentEnriched {
			failed++
			continue
		}
		tx, err := s.Pool.Begin(ctx)
		if err != nil {
			return done, failed, err
		}
		if err := s.upsertProjection(ctx, tx, row.SkillID, row.WorkspaceID, row.Name, e); err != nil {
			_ = tx.Rollback(ctx)
			return done, failed, err
		}
		if err := tx.Commit(ctx); err != nil {
			return done, failed, err
		}
		done++
	}
	return done, failed, nil
}
