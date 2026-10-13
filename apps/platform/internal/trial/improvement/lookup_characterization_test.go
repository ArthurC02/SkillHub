package eval

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement/facts"
)

func TestASuggestionWhoseRunVersionIsGoneIsNotFound(t *testing.T) {
	svc := Service{
		ReadRunFacts: func(context.Context, pgtype.UUID, pgtype.UUID) (facts.Run, bool, error) {
			return facts.Run{}, true, nil
		},
		ReadVersion: func(context.Context, pgtype.UUID, pgtype.UUID) (facts.Version, bool, error) {
			return facts.Version{}, false, nil
		},
		ReadSkill: func(context.Context, pgtype.UUID, pgtype.UUID) (facts.Skill, bool, error) {
			t.Fatal("the skill was read although its version is gone")
			return facts.Skill{}, false, nil
		},
		ReadLatestVersion: func(context.Context, pgtype.UUID, pgtype.UUID) (facts.Version, bool, error) {
			t.Fatal("the latest version was read although the run's version is gone")
			return facts.Version{}, false, nil
		},
	}

	_, err := svc.loadVersions(context.Background(), pgtype.UUID{}, gen.Evaluation{}, suggestionCtx{})

	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestTheSkillEntryLeadsThePackageFilesAndTheRestKeepTheirOrder(t *testing.T) {
	got := skillEntryFirst([]string{"a.md", "dir/SKILL.md", "SKILL.md", "z.txt"})

	want := []string{"SKILL.md", "a.md", "dir/SKILL.md", "z.txt"}
	if !slices.Equal(got, want) {
		t.Fatalf("skillEntryFirst = %v, want %v", got, want)
	}
}
