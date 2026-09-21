package envx

import "testing"

func TestOrReturnsTheProvidedValueOrFallback(t *testing.T) {
	if got := Or("configured", "fallback"); got != "configured" {
		t.Fatalf("Or(configured, fallback) = %q", got)
	}
	if got := Or("", "fallback"); got != "fallback" {
		t.Fatalf("Or(empty, fallback) = %q", got)
	}
}
