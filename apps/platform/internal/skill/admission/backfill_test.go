package ingest

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

func TestBackfillDiscardsPackagesThatAreNoLongerCurrent(t *testing.T) {
	pool := requireCreationDB(t)
	for _, scenario := range []string{"current", "replaced", "same-package-new-version", "deleted", "taken-down", "no-version"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			ws := seedCreationWorkspace(t, pool, "backfill-"+scenario)
			q := gen.New(pool)
			skill, err := q.CreateSkill(ctx, gen.CreateSkillParams{
				WorkspaceID: ws.ID, Name: "pdf-tools", Redistribution: "unknown",
			})
			if err != nil {
				t.Fatal(err)
			}
			archive := zipBytes(t, map[string]string{"SKILL.md": skillMD})
			pkg, err := (&Service{}).prepare(ctx, archive)
			if err != nil {
				t.Fatal(err)
			}
			addVersion := func(number int32, key string) pgtype.UUID {
				t.Helper()
				version, err := q.CreateSkillVersion(ctx, gen.CreateSkillVersionParams{
					WorkspaceID: ws.ID, SkillID: skill.ID, VersionNumber: number,
					ContentHash: fmt.Sprintf("%s-%d", key, number), PackageObjectKey: key, Manifest: []byte(`{}`),
				})
				if err != nil {
					t.Fatal(err)
				}
				return version.ID
			}
			var versionID pgtype.UUID
			if scenario != "no-version" {
				versionID = addVersion(1, pkg.objectKey)
			}
			pending := PendingEnrichment{SkillID: skill.ID, VersionID: versionID, WorkspaceID: ws.ID, Name: skill.Name, PackageObjectKey: pkg.objectKey}
			writes := 0
			stub := &stubEnricher{enrichStatus: http.StatusOK, embedStatus: http.StatusOK}
			svc := &Service{
				Pool: pool, Store: &creationTestStore{data: map[string][]byte{pkg.objectKey: archive}},
				LLM: stub.start(t),
				PendingEnrichments: func(context.Context, int32) ([]PendingEnrichment, error) {
					switch scenario {
					case "replaced":
						addVersion(2, "packages/replacement.zip")
					case "same-package-new-version":
						addVersion(2, pkg.objectKey)
					case "deleted":
						_, err = q.SoftDeleteSkill(ctx, gen.SoftDeleteSkillParams{ID: skill.ID, WorkspaceID: ws.ID})
					case "taken-down":
						reason := "withdrawn package"
						_, err = q.SetSkillTakedown(ctx, gen.SetSkillTakedownParams{ID: skill.ID, TakedownReason: &reason})
					}
					return []PendingEnrichment{pending}, err
				},
				IndexSkill: func(_ context.Context, _ pgx.Tx, projection SkillProjection) error {
					writes++
					if projection.SkillID != skill.ID || projection.EnrichedSummary != testEnrichedSummary {
						t.Errorf("unexpected projection: %+v", projection)
					}
					return nil
				},
			}
			done, failed, err := svc.ReindexPending(ctx, 1)
			wantDone, wantFailed := 0, 1
			if scenario == "current" {
				wantDone, wantFailed = 1, 0
			}
			if err != nil || done != wantDone || failed != wantFailed || writes != wantDone {
				t.Fatalf("done=%d failed=%d writes=%d err=%v; want %d/%d/%d/nil", done, failed, writes, err, wantDone, wantFailed, wantDone)
			}
			if len(stub.embedded) != 1 {
				t.Fatalf("embedding calls=%d, want one completed enrichment before the write guard", len(stub.embedded))
			}
		})
	}
}

func TestBackfillDiscardsAnUnidentifiedSkill(t *testing.T) {
	pool := requireCreationDB(t)
	ctx := context.Background()
	ws := seedCreationWorkspace(t, pool, "backfill-read-error")
	archive := zipBytes(t, map[string]string{"SKILL.md": skillMD})
	pkg, err := (&Service{}).prepare(ctx, archive)
	if err != nil {
		t.Fatal(err)
	}
	stub := &stubEnricher{enrichStatus: http.StatusOK, embedStatus: http.StatusOK}
	svc := &Service{
		Pool: pool, Store: &creationTestStore{data: map[string][]byte{pkg.objectKey: archive}},
		LLM: stub.start(t),
		PendingEnrichments: func(context.Context, int32) ([]PendingEnrichment, error) {
			return []PendingEnrichment{{WorkspaceID: ws.ID, SkillID: pgtype.UUID{}, VersionID: ws.ID, PackageObjectKey: pkg.objectKey}}, nil
		},
		IndexSkill: func(context.Context, pgx.Tx, SkillProjection) error {
			t.Error("unidentified Skill reached projection writer")
			return nil
		},
	}
	done, failed, err := svc.ReindexPending(ctx, 1)
	if err != nil || done != 0 || failed != 1 {
		t.Fatalf("done=%d failed=%d err=%v; want 0/1/nil for missing Skill", done, failed, err)
	}
}
