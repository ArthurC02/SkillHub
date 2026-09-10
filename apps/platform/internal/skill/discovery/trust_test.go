package catalog

import "testing"

func TestSourceTrustDisplayDistinct(t *testing.T) {

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

func TestRedistributionDisplayFallsBackToUnknownWording(t *testing.T) {
	d := Redistribution("value-added-next-year").Display()
	if d.Label == "" || d.Note == "" {
		t.Fatalf("an unrecognised redistribution value rendered blank: %+v", d)
	}
	if d != RedistributionUnknown.Display() {
		t.Errorf("an unrecognised value must read as 未確認, got %+v", d)
	}
}
