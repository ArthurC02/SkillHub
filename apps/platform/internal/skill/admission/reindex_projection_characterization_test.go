package ingest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

func TestAPreparedPackageIsAddressedByTheHashOfItsWholeArchive(t *testing.T) {
	archive := zipBytes(t, map[string]string{"SKILL.md": skillMD})
	sum := sha256.Sum256(archive)
	want := hex.EncodeToString(sum[:])

	p, err := (&Service{}).prepare(archive)

	if err != nil || p.contentHash != want || p.objectKey != "packages/"+want+".zip" {
		t.Fatalf("hash=%q key=%q err=%v, want %q and packages/<hash>.zip", p.contentHash, p.objectKey, err, want)
	}
}

func TestABlockedPackageIsNeverGivenAnObjectKey(t *testing.T) {
	p, err := (&Service{}).prepare(zipBytes(t, map[string]string{"README.md": "no skill here"}))

	if err != nil || !p.report.Blocked || p.contentHash != "" || p.objectKey != "" {
		t.Fatalf("blocked=%v hash=%q key=%q err=%v, want a blocked report with no address", p.report.Blocked, p.contentHash, p.objectKey, err)
	}
}

func TestAReindexedSkillIsProjectedUnderItsOwnNameAndWorkspace(t *testing.T) {
	pool := requireCreationDB(t)
	ctx := context.Background()
	ws := seedCreationWorkspace(t, pool, "reindex-projection-identity")
	removeAdmissionWorkspace(t, pool, ws)
	q := gen.New(pool)
	skill, err := q.CreateSkill(ctx, gen.CreateSkillParams{WorkspaceID: ws.ID, Name: "pdf-tools", Redistribution: "unknown"})
	if err != nil {
		t.Fatal(err)
	}
	archive := zipBytes(t, map[string]string{"SKILL.md": skillMD})
	pkg, err := (&Service{}).prepare(archive)
	if err != nil {
		t.Fatal(err)
	}
	version, err := q.CreateSkillVersion(ctx, gen.CreateSkillVersionParams{
		WorkspaceID: ws.ID, SkillID: skill.ID, VersionNumber: 1,
		ContentHash: pkg.contentHash, PackageObjectKey: pkg.objectKey, Manifest: []byte(`{}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	var indexed []SkillProjection
	svc := &Service{
		Pool:  pool,
		Store: &creationTestStore{data: map[string][]byte{pkg.objectKey: archive}},
		LLM:   (&stubEnricher{enrichStatus: http.StatusOK, embedStatus: http.StatusOK}).start(t),
		PendingEnrichments: func(context.Context, int32) ([]PendingEnrichment, error) {
			return []PendingEnrichment{{SkillID: skill.ID, VersionID: version.ID, WorkspaceID: ws.ID, Name: skill.Name, PackageObjectKey: pkg.objectKey}}, nil
		},
		IndexSkill: func(_ context.Context, _ pgx.Tx, p SkillProjection) error {
			indexed = append(indexed, p)
			return nil
		},
	}

	if done, failed, err := svc.ReindexPending(ctx, 1); err != nil || done != 1 || failed != 0 {
		t.Fatalf("done=%d failed=%d err=%v, want 1/0/nil", done, failed, err)
	}

	if len(indexed) != 1 || indexed[0].Name != "pdf-tools" || indexed[0].WorkspaceID != ws.ID || indexed[0].SkillID != skill.ID {
		t.Fatalf("projections = %+v, want one for pdf-tools in its workspace", indexed)
	}
}
