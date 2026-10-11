package eval

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/audit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	limits "github.com/ArthurC02/skillhub/apps/platform/internal/shared/contractlimits/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/design"
)

const (
	MaxGraderNameChars         = limits.EvaluationGraderDraftNameMaxLength
	MaxGraderInstructionsChars = limits.EvaluationGraderRevisionInstructionsMaxLength
)

var (
	ErrGraderShape    = errors.New("eval: invalid grader")
	ErrGraderConflict = errors.New("eval: grader conflict")
)

type graderVersion struct {
	name         string
	version      int32
	instructions string
}

type Grader struct {
	ID           string
	Name         string
	Platform     bool
	VersionID    string
	Version      int32
	Instructions string
	RevisedAt    time.Time
}

type GraderDraft struct {
	Name         string
	Instructions string
	Reason       string
}

type GraderService struct {
	Pool *pgxpool.Pool
}

func (g *GraderService) ValidateCheck(ctx context.Context, workspaceID pgtype.UUID, c *testlab.Check) (*testlab.Check, error) {
	normalized, err := ValidateCheck(c)
	if err != nil || normalized == nil {
		return normalized, err
	}
	ids := graderVersionIDs(*normalized, nil)
	if len(ids) == 0 {
		return normalized, nil
	}
	found, err := loadGraderVersions(ctx, gen.New(g.Pool), workspaceID, ids)
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		if _, ok := found[id]; !ok {
			return nil, fmt.Errorf("%w: 這個 Workspace 找不到評分器版本 %s", ErrInvalidCheck, id)
		}
	}
	return normalized, nil
}

func (g *GraderService) List(ctx context.Context, workspaceID pgtype.UUID) ([]Grader, error) {
	rows, err := gen.New(g.Pool).ListVisibleEvaluationGraders(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	out := make([]Grader, len(rows))
	for i, r := range rows {
		out[i] = listedGrader(r)
	}
	return out, nil
}

func (g *GraderService) ListPlatform(ctx context.Context) ([]Grader, error) {
	rows, err := gen.New(g.Pool).ListPlatformEvaluationGraders(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Grader, len(rows))
	for i, r := range rows {
		out[i] = listedGrader(gen.ListVisibleEvaluationGradersRow(r))
	}
	return out, nil
}

func (g *GraderService) Create(ctx context.Context, workspaceID, actor pgtype.UUID, d GraderDraft) (Grader, error) {
	name := strings.TrimSpace(d.Name)
	if name == "" || utf8.RuneCountInString(name) > MaxGraderNameChars {
		return Grader{}, fmt.Errorf("%w: 名稱要 1 到 %d 個字", ErrGraderShape, MaxGraderNameChars)
	}
	rev, err := revision(d)
	if err != nil {
		return Grader{}, err
	}
	var created Grader
	err = pgx.BeginFunc(ctx, g.Pool, func(tx pgx.Tx) error {
		grader, err := gen.New(tx).InsertEvaluationGrader(ctx, gen.InsertEvaluationGraderParams{
			WorkspaceID: workspaceID, Name: name, CreatedBy: actor,
		})
		if err != nil {
			return err
		}
		created, err = recordGraderVersion(ctx, tx, grader, actor, rev)
		return err
	})
	if isUniqueViolation(err) {
		return Grader{}, fmt.Errorf("%w: 已經有同名的評分器", ErrGraderConflict)
	}
	return created, err
}

func (g *GraderService) Revise(ctx context.Context, workspaceID, graderID, actor pgtype.UUID, d GraderDraft) (Grader, error) {
	rev, err := revision(d)
	if err != nil {
		return Grader{}, err
	}
	var revised Grader
	err = pgx.BeginFunc(ctx, g.Pool, func(tx pgx.Tx) error {
		grader, err := gen.New(tx).GetEvaluationGrader(ctx, gen.GetEvaluationGraderParams{ID: graderID, WorkspaceID: workspaceID})
		if err != nil {
			return err
		}
		revised, err = recordGraderVersion(ctx, tx, grader, actor, rev)
		return err
	})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return Grader{}, ErrNotFound
	case isUniqueViolation(err):
		return Grader{}, fmt.Errorf("%w: 另一個新版本剛存進來，請重新整理後再改", ErrGraderConflict)
	}
	return revised, err
}

type graderRevision struct {
	instructions string
	reason       string
}

func recordGraderVersion(
	ctx context.Context, tx pgx.Tx, grader gen.EvaluationGrader, actor pgtype.UUID, rev graderRevision,
) (Grader, error) {
	v, err := gen.New(tx).InsertEvaluationGraderVersion(ctx, gen.InsertEvaluationGraderVersionParams{
		GraderID: grader.ID, WorkspaceID: grader.WorkspaceID, Instructions: rev.instructions, Reason: rev.reason, CreatedBy: actor,
	})
	if err != nil {
		return Grader{}, err
	}
	err = audit.Log(ctx, tx, audit.Event{
		Actor: actor, Workspace: grader.WorkspaceID, Action: audit.ActionGraderVersionCreate,
		ResourceType: audit.ResourceEvaluationGrader, ResourceID: grader.ID,
		Metadata: map[string]any{"name": grader.Name, "version": v.Version, "reason": rev.reason},
	})
	return listedGrader(gen.ListVisibleEvaluationGradersRow{
		ID: grader.ID, WorkspaceID: grader.WorkspaceID, Name: grader.Name,
		VersionID: v.ID, Version: v.Version, Instructions: v.Instructions, VersionCreatedAt: v.CreatedAt,
	}), err
}

func revision(d GraderDraft) (graderRevision, error) {
	rev := graderRevision{instructions: strings.TrimSpace(d.Instructions), reason: strings.TrimSpace(d.Reason)}
	if rev.instructions == "" || utf8.RuneCountInString(rev.instructions) > MaxGraderInstructionsChars {
		return rev, fmt.Errorf("%w: 評分指示要 1 到 %d 個字", ErrGraderShape, MaxGraderInstructionsChars)
	}
	if rev.reason == "" {
		return rev, fmt.Errorf("%w: 要說明這一版改了什麼", ErrGraderShape)
	}
	return rev, nil
}

func listedGrader(r gen.ListVisibleEvaluationGradersRow) Grader {
	return Grader{
		ID: pgconv.UUIDString(r.ID), Name: r.Name, Platform: !r.WorkspaceID.Valid,
		VersionID: pgconv.UUIDString(r.VersionID), Version: r.Version, Instructions: r.Instructions, RevisedAt: r.VersionCreatedAt.Time,
	}
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func referencedGraderVersions(criteria []testlab.Criterion) []string {
	var ids []string
	for _, c := range criteria {
		if c.Check != nil {
			ids = graderVersionIDs(*c.Check, ids)
		}
	}
	return ids
}

func graderVersionIDs(c testlab.Check, into []string) []string {
	if c.Kind == promptGraderKind && len(c.Values) == 1 {
		into = append(into, c.Values[0])
	}
	for _, child := range c.Children {
		into = graderVersionIDs(child, into)
	}
	return into
}

func loadGraderVersions(ctx context.Context, q *gen.Queries, workspaceID pgtype.UUID, ids []string) (map[string]graderVersion, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	parsed := make([]pgtype.UUID, 0, len(ids))
	for _, id := range ids {
		var u pgtype.UUID
		if u.Scan(id) == nil {
			parsed = append(parsed, u)
		}
	}
	rows, err := q.GetVisibleGraderVersions(ctx, gen.GetVisibleGraderVersionsParams{Ids: parsed, WorkspaceID: workspaceID})
	if err != nil {
		return nil, err
	}
	out := make(map[string]graderVersion, len(rows))
	for _, r := range rows {
		out[pgconv.UUIDString(r.ID)] = graderVersion{name: r.Name, version: r.Version, instructions: r.Instructions}
	}
	return out, nil
}
