package checks

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/design"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement/graders"
)

type CheckValidator struct {
	Pool *pgxpool.Pool
}

func (v *CheckValidator) ValidateCheck(ctx context.Context, workspaceID pgtype.UUID, c *testlab.Check) (*testlab.Check, error) {
	normalized, err := ValidateCheck(c)
	if err != nil || normalized == nil {
		return normalized, err
	}
	ids := graderVersionIDs(*normalized, nil)
	if len(ids) == 0 {
		return normalized, nil
	}
	found, err := graders.Versions(ctx, v.Pool, workspaceID, ids)
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

func ReferencedGraderVersions(criteria []testlab.Criterion) []string {
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
