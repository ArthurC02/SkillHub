package catalog

import (
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/shared/skillpkg"
)

func TestEveryScannerDisclosureCodeHasWords(t *testing.T) {
	have := make(map[string]bool, len(disclosureCatalogue))
	for _, d := range disclosureCatalogue {
		if d.Label == "" || d.Note == "" {
			t.Errorf("%s: an entry with no words is the same as no entry: %+v", d.Code, d)
		}
		if have[d.Code] {
			t.Errorf("%s appears twice; disclosuresFor would render it twice", d.Code)
		}
		have[d.Code] = true
	}
	for _, code := range skillpkg.DisclosureCodes {
		if !have[code] {
			t.Errorf("skillpkg emits %q and the disclosure catalogue has no words for it", code)
		}
	}

	emits := make(map[string]bool, len(skillpkg.DisclosureCodes))
	for _, code := range skillpkg.DisclosureCodes {
		emits[code] = true
	}
	for _, d := range disclosureCatalogue {
		if !emits[d.Code] {
			t.Errorf("the catalogue has words for %q, which skillpkg never emits", d.Code)
		}
	}
}

func TestDisclosuresForKeepsCatalogueOrder(t *testing.T) {

	got := disclosuresFor(map[string]bool{
		skillpkg.CodeFileNotScanned: true,
		skillpkg.CodeScriptFile:     true,
	})
	if len(got) != 2 {
		t.Fatalf("got %d disclosures, want 2: %+v", len(got), got)
	}
	if got[0].Code != skillpkg.CodeScriptFile || got[1].Code != skillpkg.CodeFileNotScanned {
		t.Errorf("disclosuresFor must render in catalogue order, got %s then %s", got[0].Code, got[1].Code)
	}
	if d := disclosuresFor(nil); d == nil || len(d) != 0 {
		t.Errorf("no codes must serialise as [] rather than null, got %#v", d)
	}
}
