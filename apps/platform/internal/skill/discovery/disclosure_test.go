package catalog

import (
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/shared/skillpkg"
)

// The catalogue is the only place a finding code becomes words, so a code the
// scanner emits and this list has no entry for is a fact the platform found and
// then said nothing about. That is not hypothetical: symlink-entry (04 丙-15
// D-3), undeclared-dependency, file-not-scanned, package-dependencies and
// entry-path-escape were all being emitted with no entry here, and the one that
// mattered most was file-not-scanned — a package whose second megabyte was never
// read for secrets showed no disclosure at all.
//
// The expected set is skillpkg.DisclosureCodes rather than a list written here,
// because a hand-written expectation drifts exactly the way the thing it is
// checking drifts, and then agrees with it.
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
	// And nothing here that the scanner cannot produce: an entry for a code no
	// scan writes is a promise about a check that does not exist.
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

// Order is part of the contract: both surfaces render the catalogue in this
// order, so a reader comparing two skills is comparing positions. The two
// blocking codes stay adjacent because they share the thing a reader has to be
// told about them — they only ever appear on an import preview.
func TestDisclosuresForKeepsCatalogueOrder(t *testing.T) {
	// Deliberately asked for in the reverse of catalogue order.
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
