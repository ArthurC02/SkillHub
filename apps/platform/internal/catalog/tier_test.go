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

func TestTierDisplayUnknownValueIsZero(t *testing.T) {
	d := Tier("not-a-real-tier").Display()
	if d != (TierDisplay{}) {
		t.Fatalf("want zero value for an undefined tier, got %+v", d)
	}
}
