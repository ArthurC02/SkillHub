package testlab

import (
	"bytes"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

func TestTestCaseOfPreservesThePersistentTestCase(t *testing.T) {
	id := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	workspaceID := pgtype.UUID{Bytes: [16]byte{2}, Valid: true}
	skillID := pgtype.UUID{Bytes: [16]byte{3}, Valid: true}
	createdAt := pgtype.Timestamptz{Time: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), Valid: true}
	updatedAt := pgtype.Timestamptz{Time: time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC), Valid: true}
	deletedAt := pgtype.Timestamptz{Time: time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC), Valid: true}
	row := gen.TestCase{
		ID: id, WorkspaceID: workspaceID, SkillID: skillID, Name: "name", UserPrompt: "prompt",
		AcceptanceCriteria: []byte(`[{"id":"criterion"}]`), Rubric: []byte(`{"version":"v1"}`),
		CreatedAt: createdAt, UpdatedAt: updatedAt, DeletedAt: deletedAt,
	}

	got := testCaseOf(row)
	if got.ID != id || got.WorkspaceID != workspaceID || got.SkillID != skillID ||
		got.Name != "name" || got.UserPrompt != "prompt" || got.CreatedAt != createdAt ||
		got.UpdatedAt != updatedAt || got.DeletedAt != deletedAt ||
		!bytes.Equal(got.AcceptanceCriteria, row.AcceptanceCriteria) || !bytes.Equal(got.Rubric, row.Rubric) {
		t.Fatalf("testCaseOf() = %+v, want all TestCase fields preserved", got)
	}
}

func TestSimulatedUserFactsStayWithinTheirCountAndSize(t *testing.T) {
	facts := func(n, size int) *[]string {
		out := make([]string, n)
		for i := range out {
			out[i] = strings.Repeat("x", size)
		}
		return &out
	}
	cases := []struct {
		name   string
		edited *[]string
		accept bool
	}{
		{"ten facts", facts(10, 1), true},
		{"eleven facts", facts(11, 1), false},
		{"exactly the byte budget", facts(1, MaxSimulatedUserFactsBytes), true},
		{"one byte over the budget", facts(1, MaxSimulatedUserFactsBytes+1), false},
		{"a blank fact", &[]string{"本期用水 45 度", "  "}, false},
		{"an empty list removes the person", &[]string{}, true},
	}
	for _, c := range cases {
		_, err := validSimulatedUserFacts(c.edited, nil)
		if (err == nil) != c.accept || (err != nil && !errors.Is(err, ErrSimulatedUserFacts)) {
			t.Errorf("%s: err = %v, want accepted %v", c.name, err, c.accept)
		}
	}
	if got, _ := validSimulatedUserFacts(&[]string{" 本期用水 45 度 "}, nil); !slices.Equal(got, []string{"本期用水 45 度"}) {
		t.Errorf("a fact is stored as %q, want it trimmed", got)
	}
	if got, _ := validSimulatedUserFacts(nil, []string{"stored"}); !slices.Equal(got, []string{"stored"}) {
		t.Errorf("an edit without facts keeps %q, want the stored facts", got)
	}
}
