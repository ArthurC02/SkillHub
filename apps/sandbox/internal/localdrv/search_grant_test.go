package localdrv

import (
	"testing"

	"github.com/ArthurC02/skillhub/apps/sandbox/internal/sandbox"
)

func TestSearchEnvironmentRequiresTheFixedGrantAndDoesNotInheritIt(t *testing.T) {
	t.Setenv("SKILLHUB_SEARCH_TOOL", "skillhub-web-search")
	for _, alias := range []string{"", "other-search", "skillhub-web-search"} {
		req := sandbox.RunRequest{ModelGateway: &sandbox.ModelGatewayGrant{SearchTool: alias}}
		set := namesSetBy(env(req, "work", "out"))
		if got, want := set["SKILLHUB_SEARCH_TOOL"], alias == "skillhub-web-search"; got != want {
			t.Fatalf("search environment for %q = %v, want %v", alias, got, want)
		}
	}
}
