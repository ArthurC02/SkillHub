package registry

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

func TestAForkInheritsItsAncestorsScanOnlyForTheSameBytesOfALiveCatalogueSkill(t *testing.T) {
	id := func(b byte) pgtype.UUID {
		return pgtype.UUID{Bytes: [16]byte{15: b}, Valid: true}
	}
	catalogue, elsewhere, ancestorSkill, ancestorVersion := id(1), id(2), id(3), id(4)
	gone := pgtype.Timestamptz{Valid: true}
	fork := gen.ListSkillsRow{
		Skill:             gen.Skill{ForkedFromSkillID: ancestorSkill, ForkedFromVersionID: ancestorVersion},
		VerifiedAt:        pgtype.Timestamptz{Valid: true},
		NewestContentHash: "sha256:same",
	}
	ancestor := scanAncestor{
		VersionID: ancestorVersion, SkillID: ancestorSkill, WorkspaceID: catalogue, ContentHash: "sha256:same",
	}
	for _, tc := range []struct {
		name     string
		fork     func(*gen.ListSkillsRow)
		ancestor func(*scanAncestor)
		want     bool
	}{
		{"same bytes of a live catalogue skill", func(*gen.ListSkillsRow) {}, func(*scanAncestor) {}, true},
		{"the fork has no version", func(f *gen.ListSkillsRow) { f.VerifiedAt = pgtype.Timestamptz{} }, func(*scanAncestor) {}, false},
		{"the newest version was imported and scanned here", func(f *gen.ListSkillsRow) { f.VerifiedSourceID = id(9) }, func(*scanAncestor) {}, false},
		{"the newest version's bytes differ", func(f *gen.ListSkillsRow) { f.NewestContentHash = "sha256:edited" }, func(*scanAncestor) {}, false},
		{"the version belongs to another skill", func(*gen.ListSkillsRow) {}, func(a *scanAncestor) { a.SkillID = id(5) }, false},
		{"the version is not the one forked from", func(*gen.ListSkillsRow) {}, func(a *scanAncestor) { a.VersionID = id(6) }, false},
		{"the ancestor left the catalogue", func(*gen.ListSkillsRow) {}, func(a *scanAncestor) { a.WorkspaceID = elsewhere }, false},
		{"the ancestor was deleted", func(*gen.ListSkillsRow) {}, func(a *scanAncestor) { a.DeletedAt = gone }, false},
		{"the ancestor was taken down", func(*gen.ListSkillsRow) {}, func(a *scanAncestor) { a.TakedownAt = gone }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, a := fork, ancestor
			tc.fork(&f)
			tc.ancestor(&a)
			if got := inheritsScan(f, a, []pgtype.UUID{catalogue}); got != tc.want {
				t.Fatalf("inheritsScan = %v, want %v", got, tc.want)
			}
		})
	}
}
