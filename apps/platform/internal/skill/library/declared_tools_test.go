package registry

import (
	"slices"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

func TestAVersionCarriesTheToolsItsManifestDeclares(t *testing.T) {
	for _, tc := range []struct {
		name     string
		manifest string
		want     []string
	}{
		{"declared tools", `{"name":"report","allowed_tools":["Read","mcp__calendar__create_event"]}`, []string{"Read", "mcp__calendar__create_event"}},
		{"no allowed-tools", `{"name":"report"}`, nil},
		{"an erased version", `{}`, nil},
		{"an unreadable manifest", `not json`, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := versionDTO(gen.SkillVersion{Manifest: []byte(tc.manifest)}).AllowedTools; !slices.Equal(got, tc.want) {
				t.Errorf("AllowedTools = %v, want %v", got, tc.want)
			}
		})
	}
}
