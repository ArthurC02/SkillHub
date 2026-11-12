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

func TestEllipsizeMarksOnlyATextThatWasCut(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"exactly at the limit", "驗收條", "驗收條"},
		{"one past the limit", "驗收條件", "驗收條…"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Ellipsize(c.in, 3); got != c.want {
				t.Errorf("Ellipsize(%q, 3) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestSHA256HexIsTheLowercaseHexDigest(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"empty", "", "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"},
		{"abc", "abc", "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := SHA256Hex([]byte(c.in)); got != c.want {
				t.Errorf("SHA256Hex(%q) = %s, want %s", c.in, got, c.want)
			}
		})
	}
}

func TestCutBytesKeepsAValidPrefixOfAtMostTheLimitInBytes(t *testing.T) {
	cases := []struct {
		name, in      string
		limit         int
		want          string
		wantTruncated bool
	}{
		{"exactly at the limit", "abc", 3, "abc", false},
		{"one past the limit", "abcd", 3, "abc", true},
		{"a cut inside a character drops its partial bytes", "驗收", 4, "驗", true},
		{"a cut on a character boundary keeps it", "驗收", 3, "驗", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, truncated := CutBytes(c.in, c.limit)
			if got != c.want || truncated != c.wantTruncated {
				t.Errorf("CutBytes(%q, %d) = %q, %v; want %q, %v", c.in, c.limit, got, truncated, c.want, c.wantTruncated)
			}
		})
	}
}
