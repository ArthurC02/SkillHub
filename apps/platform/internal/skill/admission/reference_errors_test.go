package ingest

import (
	"context"
	"errors"
	"io/fs"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/ArthurC02/skillhub/apps/platform/internal/shared/skillpkg"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/library"
)

func TestAReferenceWhosePackageCannotBeReadIsUnavailableAndKeepsTheCause(t *testing.T) {
	const objectKey = "packages/reference.zip"
	ws := identity.Workspace{ID: mustUUIDForTest(t, "10000000-0000-0000-0000-000000000001")}
	skillID := mustUUIDForTest(t, "20000000-0000-0000-0000-000000000002")
	versionID := mustUUIDForTest(t, "30000000-0000-0000-0000-000000000003")
	references := fakeReferenceReader{
		workspace: map[string]registry.Skill{
			pgconv.UUIDString(skillID): {ID: skillID, WorkspaceID: ws.ID, Name: "reference-skill", Redistribution: "self_supplied"},
		},
		versions: map[string]registry.Version{
			pgconv.UUIDString(skillID): {ID: versionID, SkillID: skillID, WorkspaceID: ws.ID, PackageObjectKey: objectKey},
		},
	}
	for _, tc := range []struct {
		name      string
		store     fakeObjectStore
		wantCause error
	}{
		{"the package is missing from the store", fakeObjectStore{}, fs.ErrNotExist},
		{"the stored bytes are not a package", fakeObjectStore{objectKey: []byte("not a zip")}, skillpkg.ErrBadArchive},
		{"the package holds no SKILL.md", fakeObjectStore{objectKey: zipBytes(t, map[string]string{"README.md": "hello"})}, fs.ErrNotExist},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := &Service{Store: tc.store, References: references}

			_, _, err := svc.resolveReference(context.Background(), ws, skillID)

			if !errors.Is(err, ErrReferenceUnavailable) {
				t.Fatalf("err = %v, want ErrReferenceUnavailable", err)
			}
			if !errors.Is(err, tc.wantCause) {
				t.Fatalf("err = %v, want it to still carry the cause %v", err, tc.wantCause)
			}
		})
	}
}

func TestAGeneratedFileNameTheZipCannotHoldIsUnpackageableAndSaysWhy(t *testing.T) {
	g := goodGeneratedSkill()
	g.Files = []GeneratedFile{{Path: strings.Repeat("a", 1<<16), Content: "x"}}

	_, err := buildGeneratedPackage(g)

	if !errors.Is(err, ErrGeneratedPackageInvalid) {
		t.Fatalf("err = %v, want ErrGeneratedPackageInvalid", err)
	}
	named := ErrGeneratedPackageInvalid.Error() + `: entry "` + g.Files[0].Path + `": `
	if reason, ok := strings.CutPrefix(err.Error(), named); !ok || reason == "" {
		t.Fatalf("err = %q, want the entry named and the zip writer's reason after it", err)
	}
}

type failingReferenceReader struct {
	fakeReferenceReader
	catalogErr, versionErr error
}

func (f failingReferenceReader) CatalogSkill(ctx context.Context, skillID pgtype.UUID) (registry.Skill, bool, error) {
	if f.catalogErr != nil {
		return registry.Skill{}, false, f.catalogErr
	}
	return f.fakeReferenceReader.CatalogSkill(ctx, skillID)
}

func (f failingReferenceReader) LatestVersion(ctx context.Context, workspaceID, skillID pgtype.UUID) (registry.Version, bool, error) {
	if f.versionErr != nil {
		return registry.Version{}, false, f.versionErr
	}
	return f.fakeReferenceReader.LatestVersion(ctx, workspaceID, skillID)
}

func TestACreationReferenceThatCannotBeLookedUpIsNotCalledUnavailable(t *testing.T) {
	ws := identity.Workspace{ID: mustUUIDForTest(t, "10000000-0000-0000-0000-000000000001")}
	skillID := mustUUIDForTest(t, "20000000-0000-0000-0000-000000000002")
	listed := fakeReferenceReader{catalog: map[string]registry.Skill{
		pgconv.UUIDString(skillID): {ID: skillID, WorkspaceID: ws.ID, Name: "reference-skill", Redistribution: "self_supplied"},
	}}
	outage := errors.New("connection reset")
	for _, tc := range []struct {
		name    string
		reader  failingReferenceReader
		wantErr error
	}{
		{"the catalog lookup fails", failingReferenceReader{catalogErr: outage}, outage},
		{"the version lookup fails", failingReferenceReader{fakeReferenceReader: listed, versionErr: outage}, outage},
		{"the skill is in neither", failingReferenceReader{}, ErrReferenceUnavailable},
		{"the skill has no version", failingReferenceReader{fakeReferenceReader: listed}, ErrReferenceUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := &Service{Store: fakeObjectStore{}, References: tc.reader}

			_, _, err := svc.ReadCreationReference(context.Background(), ws, skillID, pgtype.UUID{})

			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if errors.Is(tc.wantErr, outage) && errors.Is(err, ErrReferenceUnavailable) {
				t.Fatalf("err = %v: an outage was reported as the reference being unusable", err)
			}
		})
	}
}
