package ingest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/audit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/ArthurC02/skillhub/apps/platform/internal/product/entitlements"
	"github.com/ArthurC02/skillhub/apps/platform/internal/shared/skillpkg"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/library"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/modelbudget"
)

type ObjectStore interface {
	Put(ctx context.Context, key string, data []byte) error
	Get(ctx context.Context, key string) ([]byte, error)
}

type SourceFetcher interface {
	Normalize(rawURL string) (string, error)
	Fetch(ctx context.Context, rawURL string) (data []byte, ref string, err error)
	Probe(ctx context.Context, rawURL string) error
}

// A nil *URLFetcher inside a non-nil interface passes every `Fetcher == nil`
// guard and panics on the first call.
func FetcherOrNone(f *URLFetcher) SourceFetcher {
	if f == nil {
		return nil
	}
	return f
}

type Service struct {
	Pool    *pgxpool.Pool
	Store   ObjectStore
	Fetcher SourceFetcher
	LLM     Model

	Budgets *modelbudget.Service

	IndexSkill func(ctx context.Context, tx pgx.Tx, projection SkillProjection) error

	PendingEnrichments func(ctx context.Context, limit int32) ([]PendingEnrichment, error)

	Credit CostRecorder

	CreditCanStart func(ctx context.Context, workspaceID pgtype.UUID) (bool, error)

	GenerateQuota policy.QuotaLimits

	References ReferenceReader

	SourcesInVersions func(ctx context.Context, db gen.DBTX, sourceIDs []pgtype.UUID) ([]pgtype.UUID, error)
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
	VersionID        pgtype.UUID
	WorkspaceID      pgtype.UUID
	Name             string
	PackageObjectKey string
	SourcePath       string
}

func (s *Service) GenerateFailures(ctx context.Context, workspaceID pgtype.UUID, limit int32) ([]audit.Record, error) {
	if s == nil || s.Pool == nil {
		return nil, errors.New("generate failure history requires a database pool")
	}
	return audit.ListForWorkspace(ctx, s.Pool, workspaceID,
		[]string{audit.ActionSkillGenerateFailed}, limit)
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

func redistributionFor(ws identity.Workspace, src sourceMeta) registry.Redistribution {
	if ws.IsCatalog {
		return ""
	}

	if src.Type == SourceGenerated {
		return registry.RedistributionGenerated
	}
	return registry.RedistributionSelfSupplied
}

type sourceMeta struct {
	Type SourceType
	URL  *string
	Ref  *string

	TaskDescription        *string
	GeneratorModel         *string
	GeneratorPromptVersion *string

	CostUSD          *float64
	PromptTokens     int64
	CompletionTokens int64

	GenerationInputs []byte
	Interactive      bool

	Plugin *skillpkg.PluginFacts

	ImprovedBy *registry.Improvement
}

func pluginFact(facts *skillpkg.PluginFacts, read func(skillpkg.PluginFacts) string) *string {
	if facts == nil {
		return nil
	}
	if value := read(*facts); value != "" {
		return &value
	}
	return nil
}

func (m sourceMeta) countsTowardGenerateQuota() bool {
	return m.Type == SourceGenerated && !m.Interactive
}

var ErrIncompleteProvenance = errors.New("ingest: the source does not record where its content came from")

func (m sourceMeta) provenanceComplete() error {
	generatorFields := []*string{m.TaskDescription, m.GeneratorModel, m.GeneratorPromptVersion}
	switch m.Type {
	case SourceGit:
		if !present(m.URL) || slices.ContainsFunc(generatorFields, present) {
			return fmt.Errorf("%w: a git source needs its URL and no generator", ErrIncompleteProvenance)
		}
	case SourceUpload:
		if present(m.URL) || slices.ContainsFunc(generatorFields, present) {
			return fmt.Errorf("%w: an upload has no URL or generator", ErrIncompleteProvenance)
		}
	case SourceGenerated:
		if present(m.URL) || m.TaskDescription == nil || !present(m.GeneratorModel) || !present(m.GeneratorPromptVersion) {
			return fmt.Errorf("%w: a generated source records its task and needs its model and prompt version", ErrIncompleteProvenance)
		}
	default:
		return fmt.Errorf("%w: unknown source type %q", ErrIncompleteProvenance, m.Type)
	}
	return nil
}

func present(s *string) bool {
	return s != nil && strings.TrimSpace(*s) != ""
}

func (s *Service) UploadZip(ctx context.Context, ws identity.Workspace, data []byte) (SourceResult, error) {
	return s.importSource(ctx, ws, data, sourceMeta{Type: SourceUpload})
}

func (s *Service) ImportURL(ctx context.Context, ws identity.Workspace, rawURL string) (SourceResult, error) {
	if s.Fetcher == nil {
		return SourceResult{}, fmt.Errorf("%w: 這個部署沒有啟用「從網址匯入」。", ErrFetch)
	}
	sourceURL, err := s.Fetcher.Normalize(rawURL)
	if err != nil {
		return SourceResult{}, err
	}
	data, ref, err := s.Fetcher.Fetch(ctx, sourceURL)
	if err != nil {
		return SourceResult{}, err
	}
	meta := sourceMeta{Type: SourceGit, URL: &sourceURL}
	if ref != "" {
		meta.Ref = &ref
	}
	return s.importSource(ctx, ws, data, meta)
}

type preparedPackage struct {
	report      skillpkg.Report
	contentHash string
	objectKey   string
	sourcePath  string

	skillMD  string
	fileTree []string
}

const (
	maxEnrichMDBytes = 200_000
	maxEnrichFiles   = 500
)

func readPackage(data []byte, sourcePath string) (preparedPackage, error) {
	fsys, err := skillpkg.SkillFS(data, sourcePath)
	if err != nil {
		return preparedPackage{}, err
	}
	p := preparedPackage{report: skillpkg.Validate(fsys), sourcePath: sourcePath}
	if p.report.Blocked {
		return p, nil
	}
	if md, err := fs.ReadFile(fsys, "SKILL.md"); err == nil {
		if len(md) > maxEnrichMDBytes {
			md = md[:maxEnrichMDBytes]
		}

		p.skillMD = strings.ToValidUTF8(string(md), "")
	}
	p.fileTree = enrichFileTree(fsys)
	return p, nil
}

func (s *Service) prepare(data []byte) (preparedPackage, error) {
	p, err := readPackage(data, "")
	if err != nil || p.report.Blocked {
		return p, err
	}

	sum := sha256.Sum256(data)
	p.contentHash = hex.EncodeToString(sum[:])
	p.objectKey = "packages/" + p.contentHash + ".zip"
	return p, nil
}

func (s *Service) beginPackageWrite(ctx context.Context, ws identity.Workspace, objectKey string, data []byte) (pgx.Tx, func(), error) {
	conn, err := s.Pool.Acquire(ctx)
	if err != nil {
		return nil, nil, err
	}
	workspaceLocked, objectLocked := false, false
	release := func() {
		unlockCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if objectLocked {
			if err := registry.UnlockPackageObject(unlockCtx, conn, objectKey); err != nil {
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
	if err := registry.LockPackageObject(ctx, conn, objectKey); err != nil {
		return fail(err)
	}
	objectLocked = true

	if err := registry.TrackPackageObject(ctx, conn, objectKey); err != nil {
		return fail(err)
	}
	if err := s.Store.Put(ctx, objectKey, data); err != nil {
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
	p, err := s.prepare(data)
	if err != nil || p.report.Blocked {
		return Result{Report: p.report}, err
	}
	e := s.enrichPackage(ctx, p, ws.ID)

	tx, release, err := s.beginPackageWrite(ctx, ws, p.objectKey, data)
	if err != nil {
		return Result{}, err
	}
	defer release()
	res, err := s.importOne(ctx, tx, ws, incomingVersion{pkg: p, source: src, enrichment: e})
	if err != nil {
		return Result{}, err
	}
	if after != nil {
		if err := after(ctx, tx, res); err != nil {
			return Result{}, err
		}
	}
	return res, tx.Commit(ctx)
}

type versionAudit struct {
	action string
	meta   map[string]any
}

func auditVersion(ctx context.Context, tx pgx.Tx, ws identity.Workspace, res Result, entry versionAudit) error {
	meta := entry.meta
	meta["skill_id"] = pgconv.UUIDString(res.Skill.ID)
	meta["duplicate"] = res.Duplicate
	meta["content_hash"] = res.Version.ContentHash
	return audit.Log(ctx, tx, audit.Event{
		Actor:        ws.OwnerUserID,
		Workspace:    ws.ID,
		Action:       entry.action,
		ResourceType: audit.ResourceVersion,
		ResourceID:   res.Version.ID,
		Metadata:     meta,
	})
}

var ErrSkillNotFound = errors.New("skill not found")

func (s *Service) SaveVersion(ctx context.Context, ws identity.Workspace, skillID pgtype.UUID, data []byte) (Result, error) {
	return s.saveVersion(ctx, ws, skillID, data, sourceMeta{Type: SourceUpload})
}

type Improvement struct {
	EvaluationID  pgtype.UUID
	SuggestionIDs []pgtype.UUID
}

func (s *Service) SaveImprovedVersion(
	ctx context.Context, ws identity.Workspace, skillID pgtype.UUID, data []byte, by Improvement,
) (Result, error) {
	return s.saveVersion(ctx, ws, skillID, data, sourceMeta{
		Type: SourceUpload, ImprovedBy: &registry.Improvement{EvaluationID: by.EvaluationID, SuggestionIDs: by.SuggestionIDs},
	})
}

func (s *Service) saveVersion(ctx context.Context, ws identity.Workspace, skillID pgtype.UUID, data []byte, src sourceMeta) (Result, error) {
	p, err := s.prepare(data)
	if err != nil || p.report.Blocked {
		return Result{Report: p.report}, err
	}
	res := Result{Report: p.report}

	e := s.enrichPackage(ctx, p, ws.ID)

	tx, release, err := s.beginPackageWrite(ctx, ws, p.objectKey, data)
	if err != nil {
		return Result{}, err
	}
	defer release()
	root, err := registry.LoadSkill(ctx, tx, ws.ID, skillID)
	if errors.Is(err, registry.ErrNotFound) {
		return Result{}, ErrSkillNotFound
	}
	if err != nil {
		return Result{}, err
	}
	res.Skill = root.Skill()

	res.Version, res.Duplicate, err = s.persistVersion(ctx, tx, ws, root, incomingVersion{pkg: p, source: src, enrichment: e})
	if err != nil {
		return Result{}, err
	}
	if !res.Duplicate {
		root.AdoptNewestSummary()
		if err := registry.SaveSkill(ctx, tx, root); err != nil {
			return Result{}, err
		}
	}
	if err := auditVersion(ctx, tx, ws, res, versionAudit{audit.ActionSkillVersionCreate, map[string]any{
		"version_number": res.Version.VersionNumber,
	}}); err != nil {
		return Result{}, err
	}
	return res, tx.Commit(ctx)
}

type incomingVersion struct {
	pkg        preparedPackage
	source     sourceMeta
	enrichment enrichment
}

func (s *Service) persistVersion(ctx context.Context, tx pgx.Tx, ws identity.Workspace, root *registry.SkillRoot, in incomingVersion) (registry.Version, bool, error) {
	p, src, e := in.pkg, in.source, in.enrichment
	if err := s.requireProjection(); err != nil {
		return registry.Version{}, false, err
	}
	if err := src.provenanceComplete(); err != nil {
		return registry.Version{}, false, err
	}

	skill, generated := root.Skill(), src.Type == SourceGenerated
	if !root.AcceptsContent(generated) {
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
		SourceType:  string(src.Type),
		SourceUrl:   src.URL,
		SourceRef:   src.Ref,
		ContentHash: p.contentHash,
		FetchedAt:   pgtype.Timestamptz{Time: time.Now(), Valid: true},

		TaskDescription:        src.TaskDescription,
		GeneratorModel:         src.GeneratorModel,
		GeneratorPromptVersion: src.GeneratorPromptVersion,
		GenerationInputs:       src.GenerationInputs,

		CountsTowardGenerateQuota: src.countsTowardGenerateQuota(),

		PluginName:       pluginFact(src.Plugin, func(p skillpkg.PluginFacts) string { return p.Name }),
		PluginVersion:    pluginFact(src.Plugin, func(p skillpkg.PluginFacts) string { return p.Version }),
		PluginRepository: pluginFact(src.Plugin, func(p skillpkg.PluginFacts) string { return p.Repository }),
	})
	if err != nil {
		return registry.Version{}, false, err
	}

	content, err := registry.ContentFromPackage(registry.NewVersion{
		SourceID:         source.ID,
		ContentHash:      p.contentHash,
		PackageObjectKey: p.objectKey,
		SourcePath:       p.sourcePath,
		Report:           p.report,
	}, generated)
	if err != nil {
		return registry.Version{}, false, err
	}
	if src.ImprovedBy != nil {
		content = content.ImprovedBy(*src.ImprovedBy)
	}
	root.AddVersion(content)
	if err := registry.SaveSkill(ctx, tx, root); err != nil {
		return registry.Version{}, false, err
	}

	if err := s.IndexSkill(ctx, tx, e.projection(ws.ID, skill.ID, skill.Name)); err != nil {
		return registry.Version{}, false, err
	}
	return root.AddedVersion(), false, nil
}

func (e enrichment) projection(workspaceID, skillID pgtype.UUID, name string) SkillProjection {
	return SkillProjection{
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
		EnrichmentStatus:        string(e.status),
		EnrichmentModel:         e.model,
		EnrichmentPromptVersion: e.promptVersion,
	}
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
		indexed, err := s.reindexOne(ctx, row)
		if err != nil {
			return done, failed, err
		}
		if !indexed {
			failed++
			continue
		}
		done++
	}
	return done, failed, nil
}

func (s *Service) reindexOne(ctx context.Context, row PendingEnrichment) (bool, error) {
	data, err := s.Store.Get(ctx, row.PackageObjectKey)
	if err != nil {
		slog.Warn("backfill: package unreadable", "skill", row.Name, "error", err)
		return false, nil
	}
	p, err := readPackage(data, row.SourcePath)
	if err != nil || p.report.Blocked {
		slog.Warn("backfill: stored package no longer parses", "skill", row.Name, "error", err)
		return false, nil
	}
	e := s.enrichPackage(ctx, p, row.WorkspaceID)
	if e.status != enrichmentEnriched {
		return false, nil
	}
	return s.storeReindexedProjection(ctx, row, e)
}

func (s *Service) storeReindexedProjection(ctx context.Context, row PendingEnrichment, e enrichment) (bool, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	current, err := registry.LockCurrentPackage(ctx, tx, registry.Version{
		ID: row.VersionID, WorkspaceID: row.WorkspaceID, SkillID: row.SkillID, PackageObjectKey: row.PackageObjectKey,
	})
	if err != nil || !current {
		_ = tx.Rollback(ctx)
		return false, err
	}
	if err := s.IndexSkill(ctx, tx, e.projection(row.WorkspaceID, row.SkillID, row.Name)); err != nil {
		_ = tx.Rollback(ctx)
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return true, nil
}
