package catalog

import (
	"reflect"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

func skill(b byte) pgtype.UUID { return pgtype.UUID{Bytes: [16]byte{15: b}, Valid: true} }

func TestHybridLegsFuseIntoOneCandidatePerSkill(t *testing.T) {
	fused, order := fuseHybridCandidates([]gen.ListHybridSearchCandidatesRow{
		{SkillID: skill(1), Distance: 0.4},
		{SkillID: skill(2), Unembedded: true},
		{SkillID: skill(1), Distance: 0.3, Lexical: true},
		{SkillID: skill(2), Unembedded: true, Lexical: true},
		{SkillID: skill(3), Distance: 0.2, Lexical: true},
		{SkillID: skill(3), Distance: 0.5},
	})

	if !reflect.DeepEqual(order, []pgtype.UUID{skill(1), skill(2), skill(3)}) {
		t.Fatalf("order = %v, want first appearance", order)
	}
	want := map[pgtype.UUID]hybridCandidate{
		skill(1): {ranked: true, distance: 0.3, covered: true},
		skill(2): {covered: true},
		skill(3): {ranked: true, distance: 0.2, covered: true},
	}
	if !reflect.DeepEqual(fused, want) {
		t.Fatalf("fused = %+v, want %+v", fused, want)
	}
}

func TestOnlyAnUncoveredRankedCandidateIsHeldToTheDistanceCutoff(t *testing.T) {
	const cutoff = 0.75
	for _, tc := range []struct {
		name string
		c    hybridCandidate
		want bool
	}{
		{"on the cutoff", hybridCandidate{ranked: true, distance: cutoff}, true},
		{"just past the cutoff", hybridCandidate{ranked: true, distance: 0.7500001}, false},
		{"past the cutoff but lexically covered", hybridCandidate{ranked: true, distance: 0.99, covered: true}, true},
		{"without a vector, whatever distance it carries", hybridCandidate{distance: 0.9}, true},
	} {
		if got := tc.c.admittedWithin(cutoff); got != tc.want {
			t.Errorf("%s: admitted = %v, want %v", tc.name, got, tc.want)
		}
	}
	fused := map[pgtype.UUID]hybridCandidate{
		skill(1): {ranked: true, distance: 0.9}, skill(2): {ranked: true, distance: 0.1},
	}
	if got := admittedCandidates(fused, []pgtype.UUID{skill(1), skill(2)}, cutoff); !reflect.DeepEqual(got, []pgtype.UUID{skill(2)}) {
		t.Fatalf("admitted = %v, want only the near one", got)
	}
}

func TestAPageKeepsAtMostTheLimitAndCountsEveryMatch(t *testing.T) {
	rows := []int{1, 2, 3}
	for _, tc := range []struct {
		limit int32
		want  []int
	}{
		{2, []int{1, 2}},
		{3, []int{1, 2, 3}},
		{4, []int{1, 2, 3}},
	} {
		page, total := firstPage(rows, tc.limit)
		if !reflect.DeepEqual(page, tc.want) || total != 3 {
			t.Errorf("limit %d: page %v total %d, want %v total 3", tc.limit, page, total, tc.want)
		}
	}
}

func TestHybridHitsPinTheExactNameThenCoveredThenNearestAndUnrankedLast(t *testing.T) {
	fused := map[pgtype.UUID]hybridCandidate{
		skill(1): {ranked: true, distance: 0.1},
		skill(2): {ranked: true, distance: 0.6, covered: true},
		skill(3): {ranked: true, distance: 0.7},
		skill(4): {},
		skill(5): {ranked: true, distance: 0.3},
		skill(6): {covered: true},
	}
	docs := []gen.ListHybridSearchDocumentsRow{
		{SkillID: skill(4), Name: "pending"},
		{SkillID: skill(1), Name: "nearest"},
		{SkillID: skill(6), Name: "covered-unranked"},
		{SkillID: skill(3), Name: "Excel-Freeze"},
		{SkillID: skill(5), Name: "middle"},
		{SkillID: skill(2), Name: "covered"},
	}

	rankHybridDocuments(docs, fused, "  excel-freeze ")

	var got []string
	for _, d := range docs {
		got = append(got, d.Name)
	}
	want := []string{"Excel-Freeze", "covered", "covered-unranked", "nearest", "middle", "pending"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
}
