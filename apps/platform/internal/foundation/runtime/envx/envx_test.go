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

func TestPositiveInt32TakesOnlyAPositiveValueThatFits(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want int32
	}{
		{"", 100},
		{"1", 1},
		{"2147483647", 2147483647},
		{"2147483648", 100},
		{"0", 100},
		{"-5", 100},
		{"1O0", 100},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			t.Setenv("ENVX_TEST_BATCH", tc.raw)
			if got := PositiveInt32("ENVX_TEST_BATCH", 100); got != tc.want {
				t.Errorf("PositiveInt32(%q) = %d, want %d", tc.raw, got, tc.want)
			}
		})
	}
}
