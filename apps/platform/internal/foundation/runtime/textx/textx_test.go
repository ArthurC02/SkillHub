package textx

import "testing"

func TestCutRunesKeepsAtMostTheLimitInCharactersNotBytes(t *testing.T) {
	cases := []struct {
		name, in      string
		limit         int
		want          string
		wantTruncated bool
	}{
		{"one under the limit", "驗收", 3, "驗收", false},
		{"exactly at the limit", "驗收條", 3, "驗收條", false},
		{"one past the limit", "驗收條件", 3, "驗收條", true},
		{"empty", "", 0, "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, truncated := CutRunes(c.in, c.limit)
			if got != c.want || truncated != c.wantTruncated {
				t.Errorf("CutRunes(%q, %d) = %q, %v; want %q, %v", c.in, c.limit, got, truncated, c.want, c.wantTruncated)
			}
		})
	}
}
