package catalog

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The three shelves have to be distinguishable and each has to say what it is
// and what it does not claim (設計 §2.11(c)). Mirrors TestTierDisplayDistinctPerTier
// one file over, and for the same reason: two shelves sharing a word is a filter
// control the reader cannot use.
func TestCategoryDisplayDistinctPerCategory(t *testing.T) {
	cats := []Category{CategoryDocuments, CategoryWriting, CategoryData, CategoryUnassigned}
	seen := map[string]Category{}
	for _, c := range cats {
		d := c.Display()
		if d.Label == "" || d.Note == "" {
			t.Fatalf("%s: empty display %+v", c, d)
		}
		if other, dup := seen[d.Label]; dup {
			t.Fatalf("%s and %s share label %q; shelves must be distinguishable", c, other, d.Label)
		}
		seen[d.Label] = c
	}
	// The one word 設計 §2.9 names for 「平台自己還沒決定」. Spelled out rather than
	// compared against a constant, because the constant is the thing under test.
	if got := CategoryUnassigned.Display().Label; got != "尚未定值" {
		t.Fatalf("the unassigned label is %q, want 尚未定值 (設計 §2.9 的固定表)", got)
	}
}

// A category is a shelf, not a verdict. NFR-001 forbids one axis being read as
// the other, so the copy must not reach for the safety vocabulary at all.
func TestCategoryNotesMakeNoSafetyClaim(t *testing.T) {
	for _, c := range []Category{CategoryDocuments, CategoryWriting, CategoryData, CategoryUnassigned} {
		note := c.Display().Note
		for _, forbidden := range []string{"安全保證", "已審查", "通過檢查", "背書"} {
			if strings.Contains(note, forbidden) {
				t.Errorf("%s note claims %q: a category says what a Skill is for, nothing else — %q", c, forbidden, note)
			}
		}
	}
}

// The inverse of the mistake tier_test.go records: an undefined value must not
// render blank. A shelf with no word on it reads as a row with nothing to say,
// which is the reading NFR-001 exists to prevent.
func TestCategoryDisplayUnknownValueShowsTheValueRatherThanNothing(t *testing.T) {
	d := Category("not-a-real-category").Display()
	if d.Label != "not-a-real-category" || d.Note == "" {
		t.Fatalf("an undefined category must still render something and keep its raw value: %+v", d)
	}
}

// 0053 stores NULL for anything nobody classified, and this is the branch that
// turns that into a word. Both shapes of "no value" land on the same one: a
// pointer that is nil, and a column that came back empty.
func TestCategoryLabelWordsTheAbsence(t *testing.T) {
	empty := ""
	data := "data"
	unknown := "cephalopods"

	for name, stored := range map[string]*string{
		"NULL":         nil,
		"empty string": &empty,
	} {
		got := categoryLabel(stored)
		if got.Value != string(CategoryUnassigned) {
			t.Errorf("%s: value = %q, want unassigned", name, got.Value)
		}
		if got.Label != "尚未定值" {
			t.Errorf("%s: label = %q, want 尚未定值 — a blank shelf is 設計 §2.9's FAIL", name, got.Label)
		}
		if got.Note == "" {
			t.Errorf("%s: the absence was rendered without saying why it is absent", name)
		}
	}

	if got := categoryLabel(&data); got.Value != "data" || got.Label != "資料" {
		t.Errorf("a stored shelf did not survive the read: %+v", got)
	}
	// Passed through rather than normalised to unassigned: a value nobody
	// planned for is a fact about the row, and hiding it behind 尚未定值 would
	// tell the reader the platform had not decided when in fact it had.
	if got := categoryLabel(&unknown); got.Value != "cephalopods" || got.Label != "cephalopods" {
		t.Errorf("an unrecognised stored value was rewritten instead of shown: %+v", got)
	}
}

// ?category= went live with 0053. The three shelves are accepted, anything else
// is a 400 — the same rule ?agent= and ?tier= follow, and for the reason
// unavailableFilters gives: a shared URL must never come back as a full page
// that looks filtered.
func TestParseFiltersCategory(t *testing.T) {
	for _, v := range []string{"documents", "writing", "data"} {
		f, err := parseFilters(httptest.NewRequest(http.MethodGet, "/?q=x&category="+v, nil))
		if err != nil {
			t.Fatalf("category=%s rejected: %v", v, err)
		}
		if f.Category == nil || *f.Category != v || !f.active() {
			t.Fatalf("category=%s did not reach the filter set: %+v", v, f)
		}
	}

	for _, v := range []string{"other", "", "unassigned"} {
		_, err := parseFilters(httptest.NewRequest(http.MethodGet, "/?q=x&category="+v, nil))
		if err == nil {
			t.Errorf("category=%q accepted; an unusable value must not be silently dropped", v)
		}
	}

	f, err := parseFilters(httptest.NewRequest(http.MethodGet, "/?q=x", nil))
	if err != nil || f.Category != nil || f.active() {
		t.Fatalf("absent category filter did not stay absent: %+v (%v)", f, err)
	}
}

// The other half of the same change: `category` left unavailableFilters when
// 0053 persisted the column, and `mcp` did not — nothing anywhere records
// whether a Skill needs MCP.
func TestOnlyMCPIsStillAnUnavailableDimension(t *testing.T) {
	if _, ok := unavailableFilters["category"]; ok {
		t.Error("category is still listed as unavailable; 0053 persisted it, so the note would now be untrue")
	}
	if note := unavailableFilters["mcp"]; note == "" {
		t.Error("mcp lost its refusal note; no MCP signal is captured anywhere, so the dimension must still be refused")
	}
	if _, err := parseFilters(httptest.NewRequest(http.MethodGet, "/?q=x&mcp=no", nil)); err == nil {
		t.Error("mcp=no accepted; the dimension has no source data and must be refused, not ignored")
	}
}
