package ingest

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
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
		wantCause string
	}{
		{"the package is missing from the store", fakeObjectStore{}, `fakeObjectStore: no object "packages/reference.zip"`},
		{"the stored bytes are not a package", fakeObjectStore{objectKey: []byte("not a zip")}, "bad archive: not a zip archive"},
		{"the package holds no SKILL.md", fakeObjectStore{objectKey: zipBytes(t, map[string]string{"README.md": "hello"})}, "SKILL.md"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := &Service{Store: tc.store, References: references}

			_, _, err := svc.resolveReference(context.Background(), ws, skillID)

			if !errors.Is(err, ErrReferenceUnavailable) {
				t.Fatalf("err = %v, want ErrReferenceUnavailable", err)
			}
			if !strings.HasPrefix(err.Error(), ErrReferenceUnavailable.Error()+": ") || !strings.Contains(err.Error(), tc.wantCause) {
				t.Fatalf("err = %q, want the refusal followed by the cause %q", err, tc.wantCause)
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
	if !strings.HasPrefix(err.Error(), ErrGeneratedPackageInvalid.Error()+`: entry "aaaa`) ||
		!strings.HasSuffix(err.Error(), ": zip: FileHeader.Name too long") {
		t.Fatalf("err = %q, want the entry named and the zip writer's reason", err)
	}
}
