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

// The inverse of what this test used to require. It asserted the zero value for
// an undefined tier, which locked in a blank badge — the one rendering NFR-001
// and axis()' own comment call worse than an unfamiliar word, because a row with
// no badge reads as a row with nothing to say about it.
func TestTierDisplayUnknownValueShowsTheValueRatherThanNothing(t *testing.T) {
	d := Tier("not-a-real-tier").Display()
	if d.Badge == "" || d.TrustIndicator == "" {
		t.Fatalf("an undefined tier must still render something, got %+v", d)
	}
	if d.Badge != "not-a-real-tier" {
		t.Errorf("the badge must be the raw value, not a guess at it: %+v", d)
	}
}
