package run

import (
	"errors"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	testlab "github.com/ArthurC02/skillhub/apps/platform/internal/trial/design"
)

func TestARunThatDoesNotWithholdTheSkillIsRefusedWithoutItsPackage(t *testing.T) {
	cases := []struct {
		name  string
		store ObjectStore
		key   string
		want  error
	}{
		{"no package key, with an object store", packageShelf{}, "", errSkillPackageMissing},
		{"no package key, without an object store", nil, "", errSkillPackageMissing},
		{"a package key, without an object store", nil, "skills/v1.zip", errRunRequestUnbuildable},
	}
	for _, c := range cases {
		s := &Service{TestLab: &testlab.Service{}, Store: c.store}
		inputs := runInputs{version: VersionFacts{PackageObjectKey: c.key}}
		_, _, err := s.grantsFor(t.Context(), gen.Run{}, gen.RunAttempt{}, inputs, 0)
		if !errors.Is(err, c.want) || !errors.Is(err, errRunRequestUnbuildable) {
			t.Errorf("%s: %v, want an unbuildable request because %v", c.name, err, c.want)
		}
		if c.key != "" && errors.Is(err, errSkillPackageMissing) {
			t.Errorf("%s: a stored package was reported missing: %v", c.name, err)
		}
	}
}
