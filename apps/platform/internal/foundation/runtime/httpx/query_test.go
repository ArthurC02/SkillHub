package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestQueryInt32AcceptsOnlyOneWholeNumberWithinItsBounds(t *testing.T) {
	cases := []struct {
		name  string
		query string
		want  int32
		ok    bool
	}{
		{"absent takes the fallback", "", 50, true},
		{"lower bound", "?limit=1", 1, true},
		{"upper bound", "?limit=100", 100, true},
		{"below the lower bound", "?limit=0", 0, false},
		{"past the upper bound", "?limit=101", 0, false},
		{"present but empty", "?limit=", 0, false},
		{"not a number", "?limit=abc", 0, false},
		{"a fraction", "?limit=1.5", 0, false},
		{"repeated", "?limit=5&limit=6", 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := QueryInt32(httptest.NewRequest(http.MethodGet, "/x"+c.query, nil), "limit", 50, 1, 100)
			if (err == nil) != c.ok || got != c.want {
				t.Fatalf("QueryInt32(%q) = %d, %v; want %d, ok=%v", c.query, got, err, c.want, c.ok)
			}
		})
	}
}

func TestQueryValueRefusesARepeatedParameterInsteadOfPickingOne(t *testing.T) {
	cases := []struct {
		name        string
		query       string
		wantValue   string
		wantPresent bool
		wantErr     bool
	}{
		{"absent", "", "", false, false},
		{"once", "?cursor=a", "a", true, false},
		{"once and empty", "?cursor=", "", true, false},
		{"repeated", "?cursor=a&cursor=b", "", true, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			value, present, err := QueryValue(httptest.NewRequest(http.MethodGet, "/x"+c.query, nil), "cursor")
			if value != c.wantValue || present != c.wantPresent || (err != nil) != c.wantErr {
				t.Fatalf("QueryValue(%q) = %q, %v, %v; want %q, %v, err=%v",
					c.query, value, present, err, c.wantValue, c.wantPresent, c.wantErr)
			}
		})
	}
}

func TestQueryOffsetDefaultsToZeroAndRefusesANegative(t *testing.T) {
	cases := []struct {
		query string
		want  int32
		ok    bool
	}{
		{"", 0, true},
		{"?offset=0", 0, true},
		{"?offset=2147483647", 2147483647, true},
		{"?offset=-1", 0, false},
		{"?offset=2147483648", 0, false},
	}
	for _, c := range cases {
		got, err := QueryOffset(httptest.NewRequest(http.MethodGet, "/x"+c.query, nil))
		if (err == nil) != c.ok || got != c.want {
			t.Fatalf("QueryOffset(%q) = %d, %v; want %d, ok=%v", c.query, got, err, c.want, c.ok)
		}
	}
}
