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
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/audit"
"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/ArthurC02/skillhub/apps/platform/internal/product/entitlements"
"github.com/ArthurC02/skillhub/apps/platform/internal/skill/library"
	"github.com/ArthurC02/skillhub/apps/platform/internal/shared/skillpkg"
)

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
	// IndexSkill writes the search projection. `search_documents` belongs to
	// catalog, so the write is catalog's and arrives injected from the
	// composition root rather than imported (ADR-034) — the same shape registry
	// uses for the same table, even though ingest could import catalog without a
	// cycle. Named for what it does, not for the query it calls: the ownership
	// check matches call sites textually.
	IndexSkill func(ctx context.Context, tx pgx.Tx, projection SkillProjection) error
	// PendingEnrichments is catalog's owner read, adapted by the composition root.
	PendingEnrichments func(ctx context.Context, limit int32) ([]PendingEnrichment, error)
	// GenerateQuota is the generation allowance (GEN-004, ADR-047 決策 5). The
	// zero value enforces nothing and displays nothing, which is what a build
	// with no generation allowance is. Deliberately NOT the run allowance: one
	// pool would let a generation quietly eat a trial run's balance, and the
	// trial run is what the MVP funnel measures.
	GenerateQuota policy.QuotaLimits
}

// SkillProjection is the complete search data produced by ingest.
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

// PendingEnrichment is the only catalog projection data the backfill consumes.
type PendingEnrichment struct {
	SkillID          pgtype.UUID
	WorkspaceID      pgtype.UUID
	Name             string
	PackageObjectKey string
}

// requireProjection refuses to run any path that maintains the search
// projection when the write is missing. Carrying on without it would let an
// import commit and report success while the document never appears — a skill
// that exists and cannot be found, with nothing going red anywhere.
func (s *Service) requireProjection() error {
	if s.IndexSkill == nil {
		return errors.New("ingest: search projection write not injected; refusing to write")
	}
	return nil
}

// Result reports one upload. When Report.Blocked is true nothing was stored
// and the zero Skill/Version values are meaningless.
type Result struct {
	Report    skillpkg.Report
	Skill     registry.Skill
	Version   registry.Version
	Duplicate bool // same content already existed as a version of this skill
}

// redistributionFor is the one place import answers "may this leave again".
//
// It answers about the SUPPLIER, not about the licence. A workspace that brought
// its own bytes in gets them back: that is retrieval, not redistribution, and
// there is no second party for a licence to protect. A licence verdict is still
// nobody's here, which is why the value is `self_supplied` and not `allowed`
// (0036).
//
// The catalogue is excluded because it is loaded through this same upload
// endpoint (tools/content/import_seed.py). A rule keyed on "was this uploaded"
// would mark all 45 curated skills self-supplied and let every fork of them out
// — the failure ADR-021 §5.3 is written about. is_catalog is the same
// discriminator PACK-001 第二層 already uses for "is this the platform's own".
//
// Both source types are included. The question the gate exists to ask is
// whether the platform is adding a distribution step, and for a fetch the user
// asked for, into the user's own workspace, readable only by them, it is not.
func redistributionFor(ws identity.Workspace, src sourceMeta) string {
	if ws.IsCatalog {
		return ""
	}
	// A generated package was never supplied by anybody, so `self_supplied`
	// would answer a question nobody asked. The two release the same download
	// and differ in what a future publishing path has to ask about: whether the
	// user had the right to redistribute someone else's bytes, or who owns what
	// a model wrote (ADR-047 決策 4).
	if src.Type == sourceGenerated {
		return registry.RedistributionGenerated
	}
	return registry.RedistributionSelfSupplied
}

// sourceGenerated is the third source_type, added by 0037. A package the
// platform wrote is neither a fetch nor an upload, and recording it as an
// upload would take `self_supplied` above without anybody deciding to.
const sourceGenerated = "generated"

// sourceMeta records where a package came from (INGEST-004).
type sourceMeta struct {
	Type string  // 'upload', 'git' or 'generated'
	URL  *string // original URL for git imports
	Ref  *string // branch, tag, or commit when known
	// The three below are set together or not at all, and only for
	// sourceGenerated: they are what lets someone re-derive the package sitting
	// in the workspace (02:GEN-002, ADR-047 決策 1). 0037 has a CHECK saying the
	// same thing, so a half-filled generated row cannot commit.
	TaskDescription        *string
	GeneratorModel         *string
	GeneratorPromptVersion *string
}

// UploadZip validates data as an Agent Skills package and, if it passes,
// stores the archive and records source + version for ws.
func (s *Service) UploadZip(ctx context.Context, ws identity.Workspace, data []byte) (Result, error) {
	return s.importZip(ctx, ws, data, sourceMeta{Type: "upload"})
}

// ImportURL fetches a package from an allow-listed URL (INGEST-001) and runs
// it through the same import pipeline as uploads.
func (s *Service) ImportURL(ctx context.Context, ws identity.Workspace, rawURL string) (Result, error) {
	if s.Fetcher == nil {
		return Result{}, fmt.Errorf("%w: url import not configured", ErrFetch)
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

// preparedPackage is a validated, stored package awaiting database rows.
type preparedPackage struct {
	report      skillpkg.Report
	contentHash string
	objectKey   string
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
	// Content-addressed put is idempotent, so storing before the DB commit
	// means a failed transaction leaves only a harmless orphan object.
	if err := s.Store.Put(ctx, p.objectKey, data); err != nil {
		return preparedPackage{}, err
	}
	return p, nil
}

func (s *Service) importZip(ctx context.Context, ws identity.Workspace, data []byte, src sourceMeta) (Result, error) {
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
	res.Skill = skill

	res.Version, res.Duplicate, err = s.persistVersion(ctx, tx, ws, skill, p, src, e)
	if err != nil {
		return Result{}, err
	}
	if err := auditVersion(ctx, tx, ws, audit.ActionSkillImport, res, map[string]any{
		"source_type": src.Type,
	}); err != nil {
		return Result{}, err
	}
	return res, tx.Commit(ctx)
}

// auditVersion records one accepted package against the version it produced
// (CORE-008, NFR-001 "匯入"). Written inside the import transaction, so the
// trail and the version appear together or not at all (iron rule 9).
// Duplicates are recorded too and marked as such: "someone uploaded this again"
// is exactly the kind of thing an audit trail is asked about afterwards.
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

// ErrSkillNotFound: target skill is not visible in the caller's workspace.
var ErrSkillNotFound = errors.New("skill not found")

// SaveVersion stores data as the next immutable version of an existing skill
// (WS-002). The skills row keeps its name; the manifest inside the version is
// the snapshot's truth. Existing versions are never touched (iron rule 4).
func (s *Service) SaveVersion(ctx context.Context, ws identity.Workspace, skillID pgtype.UUID, data []byte) (Result, error) {
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

// persistVersion writes the dedupe-checked version row, its source row, and
// the search projection inside the caller's transaction.
func (s *Service) persistVersion(ctx context.Context, tx pgx.Tx, ws identity.Workspace, skill registry.Skill, p preparedPackage, src sourceMeta, e enrichment) (registry.Version, bool, error) {
	// Before the first row, not at the projection write: both import entry points
	// funnel through here, and a version row that committed without its document
	// would be exactly the outcome INGEST-009 exists to prevent.
	if err := s.requireProjection(); err != nil {
		return registry.Version{}, false, err
	}
	q := gen.New(tx)
	if existing, found, err := registry.VersionByContent(ctx, tx, skill.ID, p.contentHash); err != nil {
		return registry.Version{}, false, err
	} else if found {
		// INGEST-005: identical content never overwrites or duplicates.
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
	})
	if err != nil {
		return registry.Version{}, false, err
	}

	// registry owns skills/skill_versions, so the row goes in through its API
	// rather than through the query (ADR-033 clearance path 2). The whole
	// validation report travels with it: the license columns are resolved from
	// what skillpkg actually evidenced, and that resolution is the version row's
	// business, not the importer's.
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

	// Search projection updates in the same transaction (INGEST-009): same
	// database, so consistency is free; full rebuilds go through cmd/reindex.
	// The document keeps the skills row's name (fork names differ from their
	// manifest) but takes the newest description and enrichment.
	if err := s.upsertProjection(ctx, tx, skill.ID, ws.ID, skill.Name, e); err != nil {
		return registry.Version{}, false, err
	}
	return version, false, nil
}

// upsertProjection writes one search document. Shared by import and by the
// enrichment backfill so both produce identical rows.
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
	// Checked here as well as in persistVersion, and before the worklist read:
	// this path never goes through persistVersion, and failing only after a batch
	// of enrichment calls would bill for work it cannot store.
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
		e := s.enrichPackage(ctx, p)
		if e.status != enrichmentEnriched {
			failed++ // enrichPackage already logged why; the row stays pending
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
