package registry

// Skill identity and lineage: fork, soft delete, takedown. These write the
// mutable `skills` row; the one place here that writes an immutable version row
// is Fork. See doc.go for the aggregate rules it has to respect.

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/audit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
)

var (
	// ErrNotFound: skill not visible in the caller's scope. Deliberately the
	// same answer for "does not exist" and "not yours" (WS-006).
	ErrNotFound = errors.New("skill not found")
	// ErrNameTaken: the caller already has a skill with this name.
	ErrNameTaken = errors.New("你的工作區已經有同名的 Skill")
)

// ObjectStore is the slice of object storage the registry needs (diff reads).
type ObjectStore interface {
	Get(ctx context.Context, key string) ([]byte, error)
}

// SkillProjection is the registry data catalog needs to index a fork.
type SkillProjection struct {
	SkillID     pgtype.UUID
	WorkspaceID pgtype.UUID
	Name        string
	Summary     string
}

type Service struct {
	Pool  *pgxpool.Pool
	Store ObjectStore
	// The search projection writes, both required. `search_documents` belongs to
	// catalog, and catalog has imported this package since DDD-020, so the
	// functions are injected by the composition root instead of imported
	// (ADR-034). Named for what they do rather than for the queries they call:
	// the ownership check matches call sites textually, and a field named after
	// the query would read as registry still calling it.
	IndexSkill      func(ctx context.Context, tx pgx.Tx, projection SkillProjection) error
	RemoveFromIndex func(ctx context.Context, tx pgx.Tx, workspaceID, skillID pgtype.UUID) error
	// SkillRisks reads back what those writes projected: the scan block for a
	// page of this workspace's skills, keyed by skill id, already serialised.
	// Injected for the same reason and by the same root as the two writes above.
	//
	// The blob is forwarded to the response untouched. Nothing here reads a field
	// of it, and nothing here should: the block is catalog's wording of catalog's
	// column, and the owner's list has to say what a search row says about the
	// same skill (02:NFR-007 第 3 條). Re-declaring the shape would be the drift.
	SkillRisks func(ctx context.Context, workspaceID pgtype.UUID, skillIDs []pgtype.UUID) (map[string]json.RawMessage, error)
	// CatalogSkillRisks is the same read against the public catalogue, for forks
	// whose bytes are identical to a catalogue ancestor (ADR-042 決策 6). It takes
	// no workspace argument because its scope is fixed inside catalog's SQL —
	// this side must not be able to name one (鐵律 3).
	CatalogSkillRisks func(ctx context.Context, skillIDs []pgtype.UUID) (map[string]json.RawMessage, error)
}

// requireProjection refuses to start any write that maintains the search
// projection when either function is missing. Skipping the projection write
// instead would be the worst available failure: the fork or the takedown
// commits, the caller is told it worked, and the index quietly disagrees with
// the registry — a taken-down skill still listed, a fork nobody can find — with
// nothing anywhere going red.
func (s *Service) requireProjection() error {
	if s.IndexSkill == nil || s.RemoveFromIndex == nil {
		return errors.New("registry: search projection writes not injected; refusing to write")
	}
	return nil
}

// Fork clones the latest version of a readable skill into ws. Provenance
// lives in forked_from_skill_id / forked_from_version_id; the package object
// is shared, not copied.
func (s *Service) Fork(ctx context.Context, ws identity.Workspace, skillID pgtype.UUID) (gen.Skill, gen.SkillVersion, error) {
	if err := s.requireProjection(); err != nil {
		return gen.Skill{}, gen.SkillVersion{}, err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return gen.Skill{}, gen.SkillVersion{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := gen.New(tx)

	// Readable scope is the caller's own workspace plus the public catalog:
	// forking a catalog entry into your own workspace is the whole point of
	// DISC/WS-001, and looking only at the caller's workspace made every catalog
	// fork a 404. Order matters — own workspace first, so a caller's private
	// skill is never shadowed by a catalog skill sharing an id (it cannot, but
	// the cheap read is also the correct one). Anything outside both scopes
	// still answers ErrNotFound, so another user's private skill is unchanged.
	src, err := q.GetSkill(ctx, gen.GetSkillParams{ID: skillID, WorkspaceID: ws.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		src, err = q.GetCatalogSkill(ctx, skillID)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return gen.Skill{}, gen.SkillVersion{}, ErrNotFound
	}
	if err != nil {
		return gen.Skill{}, gen.SkillVersion{}, err
	}
	// A taken-down skill is not a fork source (INGEST-010): the takedown exists
	// because the content should stop spreading, and a fork is a new copy.
	// Existing forks are untouched — theirs are already separate rows.
	if src.TakedownAt.Valid {
		return gen.Skill{}, gen.SkillVersion{}, ErrNotFound
	}
	srcVer, err := q.GetLatestSkillVersion(ctx, gen.GetLatestSkillVersionParams{
		SkillID: src.ID, WorkspaceID: src.WorkspaceID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return gen.Skill{}, gen.SkillVersion{}, ErrNotFound
	}
	if err != nil {
		return gen.Skill{}, gen.SkillVersion{}, err
	}

	// WS-001 says a signed-in user may fork a skill into their workspace. It
	// does not say once. Try-and-run (`x-fork`, then `x-fork-2`, `x-fork-3`)
	// rather than a fixed suffix, because the second fork is the ordinary shape
	// of the work — 試跑、改、再試一份 — and what it used to get was a 409 with
	// no suggested next step. Bounded: past the limit the caller gets the same
	// ErrNameTaken as before, which is the honest answer to 「你已經有很多份了」.
	// Still racy against a concurrent fork, and deliberately: the unique index is
	// what actually decides, and it is still the thing that returns ErrNameTaken.
	name, err := s.forkName(ctx, tx, ws.ID, src.Name)
	if err != nil {
		return gen.Skill{}, gen.SkillVersion{}, err
	}
	fork, err := q.CreateSkill(ctx, gen.CreateSkillParams{
		WorkspaceID:         ws.ID,
		Name:                name,
		Summary:             src.Summary,
		ForkedFromSkillID:   src.ID,
		ForkedFromVersionID: srcVer.ID,
		// A licensing hold travels with the copy (0023). Forking is how the
		// materials reach a workspace where they could be read in full or run, so
		// a fork that dropped the hold would be the way around it.
		AccessRestriction: src.AccessRestriction,
		// So does the redistribution verdict (0027, ADR-027 decision 4), for the
		// same reason plus one of its own: a fork that started again at the
		// column's conservative default would silently lose an `allowed` somebody
		// established, and letting the default answer for a copy is how a hold
		// becomes a formality.
		Redistribution: &src.Redistribution,
		// The PDM-001 category travels too (0053): it says what the bytes are
		// for, and a fork is the same bytes in another workspace. Unlike the
		// curation verdict, which is about who read them and stays behind.
		Category: src.Category,
	})
	if isUniqueViolation(err) {
		return gen.Skill{}, gen.SkillVersion{}, ErrNameTaken
	}
	if err != nil {
		return gen.Skill{}, gen.SkillVersion{}, err
	}

	// source_id stays NULL: skill_sources rows belong to the origin workspace;
	// fork provenance is carried by the lineage columns instead (INGEST-004).
	ver, err := q.CreateSkillVersion(ctx, gen.CreateSkillVersionParams{
		WorkspaceID:       ws.ID,
		SkillID:           fork.ID,
		ContentHash:       srcVer.ContentHash,
		PackageObjectKey:  srcVer.PackageObjectKey,
		Manifest:          srcVer.Manifest,
		LicenseExpression: srcVer.LicenseExpression,
		// WS-001 requires a fork to keep the License relationship, which is the
		// expression *and* the tier it was established at (ADR-021). Copying the
		// expression alone would silently upgrade a repo-level license into one
		// the fork appears to declare for itself.
		LicenseSource: srcVer.LicenseSource,
	})
	if err != nil {
		return gen.Skill{}, gen.SkillVersion{}, err
	}

	summary := ""
	if fork.Summary != nil {
		summary = *fork.Summary
	}
	if err := s.IndexSkill(ctx, tx, SkillProjection{
		SkillID:     fork.ID,
		WorkspaceID: ws.ID,
		Name:        fork.Name,
		Summary:     summary,
	}); err != nil {
		return gen.Skill{}, gen.SkillVersion{}, err
	}
	if err := audit.Log(ctx, tx, audit.Event{
		Actor:        ws.OwnerUserID,
		Workspace:    ws.ID,
		Action:       audit.ActionSkillFork,
		ResourceType: audit.ResourceSkill,
		ResourceID:   fork.ID,
		Metadata: map[string]any{
			"source_skill_id":   pgconv.UUIDString(src.ID),
			"source_version_id": pgconv.UUIDString(srcVer.ID),
		},
	}); err != nil {
		return gen.Skill{}, gen.SkillVersion{}, err
	}
	return fork, ver, tx.Commit(ctx)
}

// DeleteResult tells the user what the deletion covered (WS-005 狀態回饋).
type DeleteResult struct {
	VersionsRetained int64
}

// Delete soft-deletes a skill: it vanishes from lists, reads, and search now;
// version snapshots stay frozen (0005 trigger) until the 30-day grace purge
// (PDM-006). The content-addressed package objects are shared with forks, so
// they are never removed here.
// ponytail: the hard-purge background job lands with the retention work.
func (s *Service) Delete(ctx context.Context, ws identity.Workspace, skillID pgtype.UUID) (DeleteResult, error) {
	if err := s.requireProjection(); err != nil {
		return DeleteResult{}, err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return DeleteResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := gen.New(tx)

	skill, err := q.SoftDeleteSkill(ctx, gen.SoftDeleteSkillParams{ID: skillID, WorkspaceID: ws.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return DeleteResult{}, ErrNotFound
	}
	if err != nil {
		return DeleteResult{}, err
	}
	if err := s.RemoveFromIndex(ctx, tx, skill.WorkspaceID, skill.ID); err != nil {
		return DeleteResult{}, err
	}
	n, err := q.CountSkillVersions(ctx, skill.ID)
	if err != nil {
		return DeleteResult{}, err
	}
	if err := audit.Log(ctx, tx, audit.Event{
		Actor:        ws.OwnerUserID,
		Workspace:    ws.ID,
		Action:       audit.ActionSkillDelete,
		ResourceType: audit.ResourceSkill,
		ResourceID:   skill.ID,
		Metadata:     map[string]any{"versions_retained": n},
	}); err != nil {
		return DeleteResult{}, err
	}
	return DeleteResult{VersionsRetained: n}, tx.Commit(ctx)
}

// ErrAlreadyTakenDown: the skill is already off the catalog.
var ErrAlreadyTakenDown = errors.New("skill is already taken down")

// Takedown removes a skill from search and from the fork path while keeping
// every row it owns (INGEST-010 人工下架; PDM-006 §6 lists takedown as the only
// removal path for skills and versions, and it marks invisible rather than
// deletes). Existing forks, historical lineage and past runs are unaffected.
//
// Authorization is the ordinary workspace scope: the operator who curates the
// catalog owns the catalog workspace, so taking curated content down is the
// same statement as taking your own content down.
//
// ponytail: no operator role. A platform-wide takedown of content in *someone
// else's* workspace — abuse reports, DMCA — has no path here and needs the
// admin role SEC/CORE has not specified yet. Upgrade path is a second query
// without the workspace predicate behind that role, not a change to this one.
func (s *Service) Takedown(ctx context.Context, ws identity.Workspace, skillID pgtype.UUID, reason string) (gen.Skill, error) {
	if err := s.requireProjection(); err != nil {
		return gen.Skill{}, err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return gen.Skill{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := gen.New(tx)

	skill, err := q.TakedownSkill(ctx, gen.TakedownSkillParams{
		ID: skillID, WorkspaceID: ws.ID, Reason: &reason,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		// Either it is not visible here, or it is already down. Tell the two
		// apart with a scoped read; both answers are safe to give the owner.
		if _, getErr := q.GetSkill(ctx, gen.GetSkillParams{ID: skillID, WorkspaceID: ws.ID}); getErr == nil {
			return gen.Skill{}, ErrAlreadyTakenDown
		}
		return gen.Skill{}, ErrNotFound
	}
	if err != nil {
		return gen.Skill{}, err
	}
	// Same transaction as the flag, so the content can never be down in the
	// registry but still listed in search. The removal is catalog's write on
	// catalog's table; it stays inside this transaction because it is injected
	// rather than fetched from a service that would open its own (ADR-034).
	if err := s.RemoveFromIndex(ctx, tx, skill.WorkspaceID, skill.ID); err != nil {
		return gen.Skill{}, err
	}
	if err := audit.Log(ctx, tx, audit.Event{
		Actor:        ws.OwnerUserID,
		Workspace:    ws.ID,
		Action:       audit.ActionSkillTakedown,
		ResourceType: audit.ResourceSkill,
		ResourceID:   skill.ID,
		// The reason text lives on the skills row; the audit event stays
		// identifiers-only (PDM-006 §6 "不含內容").
	}); err != nil {
		return gen.Skill{}, err
	}
	return skill, tx.Commit(ctx)
}

func isUniqueViolation(err error) bool {
	pgErr, ok := errors.AsType[*pgconn.PgError](err)
	return ok && pgErr.Code == "23505"
}

// maxForkAttempts bounds the suffix search. Ten is not a rule about how many
// forks anybody may have; it is the point past which "pick the next free name"
// stops being a convenience and starts being a loop nobody watches.
const maxForkAttempts = 10

// forkName is the first free name in the series `x-fork`, `x-fork-2`, … in this
// workspace, or ErrNameTaken when the series is used up.
//
// A fork of a fork therefore becomes `x-fork-2` and not `x-fork-fork`: the
// suffix is appended to the source name only when the source is not already one
// of ours, so the series stays flat instead of growing a word per generation.
func (s *Service) forkName(
	ctx context.Context, tx pgx.Tx, workspaceID pgtype.UUID, sourceName string,
) (string, error) {
	base := strings.TrimSuffix(sourceName, "-fork")
	if trimmed, _, ok := cutForkOrdinal(base); ok {
		base = trimmed
	}
	for i := 1; i <= maxForkAttempts; i++ {
		name := base + "-fork"
		if i > 1 {
			name += "-" + strconv.Itoa(i)
		}
		if _, found, err := SkillByName(ctx, tx, workspaceID, name); err != nil {
			return "", err
		} else if !found {
			return name, nil
		}
	}
	return "", ErrNameTaken
}

// cutForkOrdinal splits `x-fork-3` into `x` and 3. It reports false for anything
// else, `x-fork` included — that one is handled by the plain suffix trim.
func cutForkOrdinal(name string) (string, int, bool) {
	base, ordinal, found := strings.Cut(name, "-fork-")
	if !found || base == "" {
		return name, 0, false
	}
	n, err := strconv.Atoi(ordinal)
	if err != nil || n < 2 {
		return name, 0, false
	}
	return base, n, true
}
