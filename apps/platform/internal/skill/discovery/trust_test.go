package catalog

import "testing"

func TestSourceTrustDisplayDistinct(t *testing.T) {
	// SourceTrustGenerated was added 2026-08-23 and this list was not updated, so
	// the newest value was the one value with no test.
	levels := []SourceTrust{
		SourceTrustUnknown, SourceTrustTraceable, SourceTrustManuallyConfirmed, SourceTrustGenerated,
	}
	seen := map[string]bool{}
	for _, l := range levels {
		d := l.Display()
		if d.Label == "" {
			t.Fatalf("%s: empty label", l)
		}
		if seen[d.Label] {
			t.Fatalf("%s: duplicate label %q across source trust levels", l, d.Label)
		}
		seen[d.Label] = true
	}
}

func TestLicenseStatusDisplayDistinct(t *testing.T) {
	levels := []LicenseStatus{LicenseStatusUnknown, LicenseStatusDeclared, LicenseStatusConfirmed}
	seen := map[string]bool{}
	for _, l := range levels {
		d := l.Display()
		if d.Label == "" {
			t.Fatalf("%s: empty label", l)
		}
		if seen[d.Label] {
			t.Fatalf("%s: duplicate label %q across license statuses", l, d.Label)
		}
		seen[d.Label] = true
	}
}

func TestDerivationBadgeDiffersByForkStatus(t *testing.T) {
	fork := Derivation(true)
	original := Derivation(false)
	if fork.Label == original.Label {
		t.Fatalf("forked and original skills must show different badges, both got %q", fork.Label)
	}
	if fork.Label == "" || original.Label == "" {
		t.Fatalf("badge labels must not be empty: fork=%+v original=%+v", fork, original)
	}
}

// Redistribution is the axis that decides whether bytes leave the platform and
// the only one of the three with a fallback branch, and until now it was the
// only one with no test at all. Deleting the `if ok` — so an unrecognised value
// returns an empty TrustDisplay — left every test in this package green while
// the screen showed a blank label next to a skill nobody may copy.
func TestRedistributionDisplayDistinct(t *testing.T) {
	levels := []Redistribution{
		RedistributionAllowed, RedistributionBlocked, RedistributionUnknown,
		RedistributionSelfSupplied, RedistributionGenerated,
	}
	seen := map[string]bool{}
	for _, l := range levels {
		d := l.Display()
		if d.Label == "" || d.Note == "" {
			t.Fatalf("%s: incomplete display %+v", l, d)
		}
		if seen[d.Label] {
			t.Errorf("%s: duplicate label %q across redistribution values", l, d.Label)
		}
		seen[d.Label] = true
	}
}

// A value this build does not know must fall back to the unknown copy, which is
// the one that says 「未確認一律當成不可散布」. The packaging gate already refuses
// anything that is not exactly one of the releasing values, so the fallback is
// not what keeps the bytes in — it is what tells the reader of a refused skill
// why, instead of showing them an empty badge.
func TestRedistributionDisplayFallsBackToUnknownWording(t *testing.T) {
	d := Redistribution("value-added-next-year").Display()
	if d.Label == "" || d.Note == "" {
		t.Fatalf("an unrecognised redistribution value rendered blank: %+v", d)
	}
	if d != RedistributionUnknown.Display() {
		t.Errorf("an unrecognised value must read as 未確認, got %+v", d)
	}
}
