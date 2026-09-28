package analytics

import (
	"strings"
	"testing"
)

func TestReportedPagePathKeepsOnlyABarePathWithinTheLimit(t *testing.T) {
	atLimit := "/" + strings.Repeat("p", 511)
	cases := []struct {
		name string
		raw  string
		want *string
	}{
		{"a path of exactly 512 bytes is kept", atLimit, &atLimit},
		{"a path of 513 bytes is dropped", atLimit + "p", nil},
		{"surrounding space is trimmed", "  /skills  ", ptr("/skills")},
		{"a relative path is dropped", "skills", nil},
		{"a query string is dropped", "/skills?q=x", nil},
		{"a fragment is dropped", "/skills#top", nil},
		{"an empty path is dropped", "", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assertSameOptional(t, reportedPagePath(c.raw), c.want)
		})
	}
}

func TestReportedBuildIDKeepsOnlyANonEmptyIDWithinTheLimit(t *testing.T) {
	atLimit := strings.Repeat("b", 64)
	cases := []struct {
		name string
		raw  string
		want *string
	}{
		{"an id of exactly 64 bytes is kept", atLimit, &atLimit},
		{"an id of 65 bytes is dropped", atLimit + "b", nil},
		{"surrounding space is trimmed", " abc123 ", ptr("abc123")},
		{"a blank id is dropped", "   ", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assertSameOptional(t, reportedBuildID(c.raw), c.want)
		})
	}
}

func ptr(s string) *string { return &s }

func assertSameOptional(t *testing.T, got, want *string) {
	t.Helper()
	switch {
	case want == nil && got != nil:
		t.Fatalf("got %q, want nothing recorded", *got)
	case want != nil && got == nil:
		t.Fatalf("got nothing recorded, want %q", *want)
	case want != nil && *got != *want:
		t.Fatalf("got %q, want %q", *got, *want)
	}
}
