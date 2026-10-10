package testlab

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

func values(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = "v"
	}
	return out
}

func TestACheckIsAcceptedOnlyWithTheValuesItsKindTakes(t *testing.T) {
	cases := []struct {
		name  string
		check Check
		ok    bool
	}{
		{"contains_values needs at least one value", Check{Kind: CheckContainsValues}, false},
		{"contains_values with one value", Check{Kind: CheckContainsValues, Values: values(1)}, true},
		{"contains_values at the value limit", Check{Kind: CheckContainsValues, Values: values(MaxCheckValues)}, true},
		{"contains_values one past the value limit", Check{Kind: CheckContainsValues, Values: values(MaxCheckValues + 1)}, false},
		{"asks with no values", Check{Kind: CheckAsks}, true},
		{"asks one past the value limit", Check{Kind: CheckAsks, Values: values(MaxCheckValues + 1)}, false},
		{"does_not_ask with no values", Check{Kind: CheckDoesNotAsk}, true},
		{"does_not_ask with a value", Check{Kind: CheckDoesNotAsk, Values: values(1)}, false},
		{"an unknown kind", Check{Kind: "regex", Values: values(1)}, false},
		{"a blank value", Check{Kind: CheckContainsValues, Values: []string{"  "}}, false},
		{"a value at the byte limit", Check{Kind: CheckContainsValues, Values: []string{strings.Repeat("a", MaxCheckValueBytes)}}, true},
		{"a value one byte past the limit", Check{Kind: CheckContainsValues, Values: []string{strings.Repeat("a", MaxCheckValueBytes+1)}}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := validateCheck(&c.check)
			if c.ok && (err != nil || got == nil) {
				t.Fatalf("want accepted, got %v", err)
			}
			if !c.ok && !errors.Is(err, ErrInvalid) {
				t.Fatalf("want ErrInvalid, got %v", err)
			}
		})
	}
}

func TestNoCheckStaysNoCheckAndValuesAreTrimmed(t *testing.T) {
	if got, err := validateCheck(nil); got != nil || err != nil {
		t.Errorf("nil check = %+v, %v", got, err)
	}
	got, err := validateCheck(&Check{Kind: CheckContainsValues, Values: []string{" 1,200 ", "2026-10-11"}})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got.Values, []string{"1,200", "2026-10-11"}) {
		t.Errorf("values = %q", got.Values)
	}
	if got, _ := validateCheck(&Check{Kind: CheckAsks, Values: []string{}}); got.Values != nil {
		t.Errorf("an empty value list is stored as none, got %q", got.Values)
	}
}
