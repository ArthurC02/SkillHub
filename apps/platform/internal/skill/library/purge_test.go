package registry

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

func TestPurgesRefuseWithoutTheReferenceReads(t *testing.T) {
	ctx := context.Background()
	if err := (&Service{}).PurgeWorkspace(ctx, nil, pgtype.UUID{}); err == nil {
		t.Error("PurgeWorkspace ran without knowing what still references a skill")
	}
	if _, err := (&Service{}).PurgeDeletedSkills(ctx, time.Hour, 10); err == nil {
		t.Error("PurgeDeletedSkills ran without knowing what still references a skill")
	}
}

func TestAPurgeKeepsASkillWhileAnythingStillHoldsItOrOneOfItsVersions(t *testing.T) {
	skill := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	first := pgtype.UUID{Bytes: [16]byte{2}, Valid: true}
	second := pgtype.UUID{Bytes: [16]byte{3}, Valid: true}
	candidate := purgeCandidate{skillID: skill, versionIDs: []pgtype.UUID{first, second}}
	for _, tc := range []struct {
		what     string
		skills   []pgtype.UUID
		versions []pgtype.UUID
		keep     bool
	}{
		{"nothing refers to it", nil, nil, false},
		{"the skill itself is held", []pgtype.UUID{skill}, nil, true},
		{"its newest version is held", nil, []pgtype.UUID{second}, true},
		{"its first version is held", nil, []pgtype.UUID{first}, true},
		{"only another skill's version is held", nil, []pgtype.UUID{{Bytes: [16]byte{9}, Valid: true}}, false},
	} {
		holds := purgeHolds{skills: map[pgtype.UUID]bool{}, versions: map[pgtype.UUID]bool{}}
		for _, id := range tc.skills {
			holds.skills[id] = true
		}
		for _, id := range tc.versions {
			holds.versions[id] = true
		}
		if got := holds.keep(candidate); got != tc.keep {
			t.Errorf("%s: keep = %v, want %v", tc.what, got, tc.keep)
		}
	}
}

func TestAForkKeepsItsSourceOnlyWhileTheForkItselfStays(t *testing.T) {
	id := func(b byte) pgtype.UUID { return pgtype.UUID{Bytes: [16]byte{b}, Valid: true} }
	source, fork, grandchild, outsider := id(1), id(2), id(3), id(4)
	edge := func(from, by pgtype.UUID) gen.ListSkillForksRow {
		return gen.ListSkillForksRow{SourceID: from, ForkID: by}
	}
	for _, tc := range []struct {
		what       string
		candidates []pgtype.UUID
		held       []pgtype.UUID
		forks      []gen.ListSkillForksRow
		retained   []pgtype.UUID
	}{
		{"a fork purged in the same pass", []pgtype.UUID{source, fork}, nil,
			[]gen.ListSkillForksRow{edge(source, fork)}, nil},
		{"a fork in the same pass that is itself held", []pgtype.UUID{source, fork}, []pgtype.UUID{fork},
			[]gen.ListSkillForksRow{edge(source, fork)}, []pgtype.UUID{source, fork}},
		{"a fork outside the purge", []pgtype.UUID{source}, nil,
			[]gen.ListSkillForksRow{edge(source, outsider)}, []pgtype.UUID{source}},
		{"a chain whose last fork is outside", []pgtype.UUID{source, fork}, nil,
			[]gen.ListSkillForksRow{edge(source, fork), edge(fork, outsider)}, []pgtype.UUID{source, fork}},
		{"a chain whose last fork is held", []pgtype.UUID{source, fork, grandchild}, []pgtype.UUID{grandchild},
			[]gen.ListSkillForksRow{edge(source, fork), edge(fork, grandchild)}, []pgtype.UUID{source, fork, grandchild}},
	} {
		candidates := make([]purgeCandidate, len(tc.candidates))
		for i, c := range tc.candidates {
			candidates[i] = purgeCandidate{skillID: c}
		}
		holds := purgeHolds{skills: map[pgtype.UUID]bool{}, versions: map[pgtype.UUID]bool{}}
		for _, h := range tc.held {
			holds.skills[h] = true
		}
		got := retainedSkills(candidates, holds, tc.forks)
		if len(got) != len(tc.retained) {
			t.Errorf("%s: retained %d skills, want %d (%v)", tc.what, len(got), len(tc.retained), got)
		}
		for _, want := range tc.retained {
			if !got[want] {
				t.Errorf("%s: skill %d was not retained", tc.what, want.Bytes[0])
			}
		}
	}
}
