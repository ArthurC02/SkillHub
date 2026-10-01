package literalgate

import (
	"regexp"
	"testing"
)

func TestAPatternIsSkippedOnlyWhenTheTextLacksALiteralEveryMatchNeeds(t *testing.T) {
	for _, c := range []struct {
		name, pattern, text string
		canMatch            bool
	}{
		{"a case-insensitive literal folded outside ASCII", `(?i)token=x`, "toKen=x", true},
		{"a case-insensitive literal in another ASCII case", `(?i)aws_secret\s*=x`, "AWS_Secret =x", true},
		{"a case-insensitive literal absent from ASCII text", `(?i)aws_secret\s*=x`, "region=x", false},
		{"a literal without case under (?i)", `(?i)\d+=\d+`, "1=2", true},
		{"either side of an alternation", `a|b`, "b", true},
		{"an optional part", `x?yz`, "yz", true},
		{"a repeated part", `a*b`, "b", true},
		{"literals inside a group", `(ab)c`, "abc", true},
		{"a missing required literal", `abc`, "xyz", false},
		{"a required literal after a group", `(ab)c`, "abd", false},
	} {
		re := regexp.MustCompile(c.pattern)
		if got := CanMatch(re, c.text); got != c.canMatch {
			t.Errorf("%s: CanMatch(%s, %q) = %v, want %v", c.name, c.pattern, c.text, got, c.canMatch)
		}
		if got := CanMatchBytes(re, []byte(c.text)); got != c.canMatch {
			t.Errorf("%s: CanMatchBytes(%s, %q) = %v, want %v", c.name, c.pattern, c.text, got, c.canMatch)
		}
		if c.canMatch && !re.MatchString(c.text) {
			t.Errorf("%s: the case itself is wrong, %s does not match %q", c.name, c.pattern, c.text)
		}
	}
}

func TestNonASCIITextAlwaysRunsACaseInsensitivePattern(t *testing.T) {
	re := regexp.MustCompile(`(?i)aws_secret_access_key\s*=\s*\S{20}`)
	text := "aws_ſecret_access_Key = abcdefghijklmnopqrstuvwxyz"
	if !re.MatchString(text) {
		t.Fatalf("%s does not match %q; the case is wrong", re, text)
	}
	if !CanMatch(re, text) || !CanMatchBytes(re, []byte(text)) {
		t.Errorf("%q matches %s but would be skipped", text, re)
	}
}
