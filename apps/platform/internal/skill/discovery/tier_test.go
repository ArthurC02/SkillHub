package catalog

import "testing"

func TestTierDisplayDistinctPerTier(t *testing.T) {
	tiers := []Tier{TierCurated, TierIndexed, TierExternal}
	seen := map[string]Tier{}
	for _, tier := range tiers {
		d := tier.Display()
		if d.Badge == "" || d.TrustIndicator == "" {
			t.Fatalf("%s: empty display %+v", tier, d)
		}
		if other, dup := seen[d.Badge]; dup {
			t.Fatalf("%s and %s share badge %q; tiers must be visually distinguishable", tier, other, d.Badge)
		}
		seen[d.Badge] = tier
	}
}

func TestTierDisplayUnknownValueShowsTheValueRatherThanNothing(t *testing.T) {
	d := Tier("not-a-real-tier").Display()
	if d.Badge == "" || d.TrustIndicator == "" {
		t.Fatalf("an undefined tier must still render something, got %+v", d)
	}
	if d.Badge != "not-a-real-tier" {
		t.Errorf("the badge must be the raw value, not a guess at it: %+v", d)
	}
}

func TestTheTierFilterAsksForCuratedOrNotAndNothingWhenAbsent(t *testing.T) {
	curated, indexed := string(TierCurated), string(TierIndexed)
	if curatedFilter(nil) != nil {
		t.Fatal("no tier filter still filtered on curation")
	}
	if got := curatedFilter(&curated); got == nil || !*got {
		t.Fatalf("tier=curated filter = %v, want curated only", got)
	}
	if got := curatedFilter(&indexed); got == nil || *got {
		t.Fatalf("tier=indexed filter = %v, want uncurated only", got)
	}
	if tierOf(true) != TierCurated || tierOf(false) != TierIndexed {
		t.Fatalf("tierOf(true)=%q tierOf(false)=%q", tierOf(true), tierOf(false))
	}
}

func TestASearchRowShowsTheModelSummaryOnlyWhenOneWasWritten(t *testing.T) {
	if got, source := summaryText("from the package", ""), summarySource(""); got != "from the package" || source != "package" {
		t.Fatalf("without an enriched summary: %q from %q, want the package summary", got, source)
	}
	if got, source := summaryText("from the package", "from the model"), summarySource("from the model"); got != "from the model" || source != "model" {
		t.Fatalf("with an enriched summary: %q from %q, want the model summary", got, source)
	}
}
