package ingest

import (
	"context"
	"errors"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/library"
)

func TestAReferenceWhosePackageCannotBeOpenedIsUnavailable(t *testing.T) {
	const objectKey = "packages/unreadable-reference.zip"
	ws := identity.Workspace{ID: mustUUIDForTest(t, "10000000-0000-0000-0000-000000000041")}
	skillID := mustUUIDForTest(t, "20000000-0000-0000-0000-000000000042")
	versionID := mustUUIDForTest(t, "30000000-0000-0000-0000-000000000043")
	for _, tc := range []struct {
		name  string
		store fakeObjectStore
	}{
		{"object missing from the store", fakeObjectStore{}},
		{"object is not a zip", fakeObjectStore{objectKey: []byte("not a zip")}},
		{"package has no SKILL.md", fakeObjectStore{objectKey: zipBytes(t, map[string]string{"README.md": "# no skill here\n"})}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := &Service{
				Store: tc.store,
				References: fakeReferenceReader{
					workspace: map[string]registry.Skill{pgconv.UUIDString(skillID): {
						ID: skillID, WorkspaceID: ws.ID, Name: "reference-skill", Redistribution: string(registry.RedistributionSelfSupplied),
					}},
					versions: map[string]registry.Version{pgconv.UUIDString(skillID): {
						ID: versionID, SkillID: skillID, WorkspaceID: ws.ID, PackageObjectKey: objectKey,
					}},
				},
			}
			fixed, reference, err := svc.ReadCreationReference(context.Background(), ws, skillID, versionID)
			if !errors.Is(err, ErrReferenceUnavailable) {
				t.Fatalf("err = %v, want ErrReferenceUnavailable", err)
			}
			if fixed.VersionID.Valid || reference.SkillMD != "" {
				t.Fatalf("an unavailable reference still carried %+v / %q", fixed, reference.SkillMD)
			}
		})
	}
}

type unreachableObjectStore struct{ fakeObjectStore }

var errStoreDown = errors.New("object store unreachable")

func (unreachableObjectStore) Get(context.Context, string) ([]byte, error) { return nil, errStoreDown }

func TestAReferenceWhoseStoreIsDownIsAnOutageNotAnUnavailableReference(t *testing.T) {
	ws := identity.Workspace{ID: mustUUIDForTest(t, "10000000-0000-0000-0000-000000000051")}
	skillID := mustUUIDForTest(t, "20000000-0000-0000-0000-000000000052")
	versionID := mustUUIDForTest(t, "30000000-0000-0000-0000-000000000053")
	svc := &Service{
		Store: unreachableObjectStore{},
		References: fakeReferenceReader{
			workspace: map[string]registry.Skill{pgconv.UUIDString(skillID): {
				ID: skillID, WorkspaceID: ws.ID, Name: "reference-skill", Redistribution: string(registry.RedistributionSelfSupplied),
			}},
			versions: map[string]registry.Version{pgconv.UUIDString(skillID): {
				ID: versionID, SkillID: skillID, WorkspaceID: ws.ID, PackageObjectKey: "packages/reference.zip",
			}},
		},
	}
	_, _, err := svc.ReadCreationReference(context.Background(), ws, skillID, versionID)
	if !errors.Is(err, errStoreDown) || errors.Is(err, ErrReferenceUnavailable) {
		t.Fatalf("err = %v, want the store outage itself rather than ErrReferenceUnavailable", err)
	}
}
