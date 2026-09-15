package registry

import "testing"

func TestARestrictionIsInEffectOnlyWhileAReasonIsRecorded(t *testing.T) {
	recorded := func(s string) *string { return &s }
	for _, tc := range []struct {
		name     string
		column   *string
		inEffect bool
		reason   string
	}{
		{"nothing recorded", nil, false, ""},
		{"an empty reason", recorded(""), false, ""},
		{"a blank reason", recorded("   "), false, ""},
		{"a recorded reason", recorded("license-review"), true, "license-review"},
		{"a reason with stray spaces", recorded(" license-review "), true, "license-review"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := RestrictionFrom(tc.column)
			if got.InEffect() != tc.inEffect || got.Reason() != tc.reason {
				t.Errorf("restriction = (%v, %q), want (%v, %q)", got.InEffect(), got.Reason(), tc.inEffect, tc.reason)
			}
		})
	}
}
