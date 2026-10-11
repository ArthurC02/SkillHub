package graders

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/audit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	limits "github.com/ArthurC02/skillhub/apps/platform/internal/shared/contractlimits/gen"
)

const (
	MaxNameChars         = limits.EvaluationGraderDraftNameMaxLength
	MaxInstructionsChars = limits.EvaluationGraderRevisionInstructionsMaxLength
)

var (
	ErrShape    = errors.New("eval: invalid grader")
	ErrConflict = errors.New("eval: grader conflict")
	ErrNotFound = errors.New("eval: grader not found")
)

type Version struct {
	Name         string
	Number       int32
	Instructions string
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

type Draft struct {
	Name         string
	Instructions string
	Reason       string
}

type Service struct {
	Pool *pgxpool.Pool
}

func (g *Service) List(ctx context.Context, workspaceID pgtype.UUID) ([]Grader, error) {
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

func (g *Service) ListPlatform(ctx context.Context) ([]Grader, error) {
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

func (g *Service) Create(ctx context.Context, workspaceID, actor pgtype.UUID, d Draft) (Grader, error) {
	name := strings.TrimSpace(d.Name)
	if name == "" || utf8.RuneCountInString(name) > MaxNameChars {
		return Grader{}, fmt.Errorf("%w: 名稱要 1 到 %d 個字", ErrShape, MaxNameChars)
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
		created, err = recordVersion(ctx, tx, grader, actor, rev)
		return err
	})
	if pgconv.IsUniqueViolation(err) {
		return Grader{}, fmt.Errorf("%w: 已經有同名的評分器", ErrConflict)
	}
	return created, err
}

func (g *Service) Revise(ctx context.Context, workspaceID, graderID, actor pgtype.UUID, d Draft) (Grader, error) {
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
		revised, err = recordVersion(ctx, tx, grader, actor, rev)
		return err
	})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return Grader{}, ErrNotFound
	case pgconv.IsUniqueViolation(err):
		return Grader{}, fmt.Errorf("%w: 另一個新版本剛存進來，請重新整理後再改", ErrConflict)
	}
	return revised, err
}

func Versions(ctx context.Context, db gen.DBTX, workspaceID pgtype.UUID, ids []string) (map[string]Version, error) {
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
	rows, err := gen.New(db).GetVisibleGraderVersions(ctx, gen.GetVisibleGraderVersionsParams{Ids: parsed, WorkspaceID: workspaceID})
	if err != nil {
		return nil, err
	}
	out := make(map[string]Version, len(rows))
	for _, r := range rows {
		out[pgconv.UUIDString(r.ID)] = Version{Name: r.Name, Number: r.Version, Instructions: r.Instructions}
	}
	return out, nil
}

type pendingRevision struct {
	instructions string
	reason       string
}

func recordVersion(
	ctx context.Context, tx pgx.Tx, grader gen.EvaluationGrader, actor pgtype.UUID, rev pendingRevision,
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

func revision(d Draft) (pendingRevision, error) {
	rev := pendingRevision{instructions: strings.TrimSpace(d.Instructions), reason: strings.TrimSpace(d.Reason)}
	if rev.instructions == "" || utf8.RuneCountInString(rev.instructions) > MaxInstructionsChars {
		return rev, fmt.Errorf("%w: 評分指示要 1 到 %d 個字", ErrShape, MaxInstructionsChars)
	}
	if rev.reason == "" {
		return rev, fmt.Errorf("%w: 要說明這一版改了什麼", ErrShape)
	}
	return rev, nil
}

func listedGrader(r gen.ListVisibleEvaluationGradersRow) Grader {
	return Grader{
		ID: pgconv.UUIDString(r.ID), Name: r.Name, Platform: !r.WorkspaceID.Valid,
		VersionID: pgconv.UUIDString(r.VersionID), Version: r.Version, Instructions: r.Instructions, RevisedAt: r.VersionCreatedAt.Time,
	}
}
