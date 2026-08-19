package catalog

import "testing"

func TestSourceTrustDisplayDistinct(t *testing.T) {
	levels := []SourceTrust{SourceTrustUnknown, SourceTrustTraceable, SourceTrustManuallyConfirmed}
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
