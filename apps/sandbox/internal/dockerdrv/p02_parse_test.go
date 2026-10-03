package dockerdrv

import (
	"context"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestParseProbeOutputFiltersToRequestedAndDeDupes(t *testing.T) {
	targets := []string{"db.internal:5432", "api.internal:8080"}
	out := "REACHED db.internal:5432\n" +
		"REACHED elsewhere.internal:9999\n" +
		"not a probe line\n" +
		"\n" +
		"REACHED db.internal:5432\n"

	got := parseProbeOutput(out, targets)
	want := []string{"db.internal:5432"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseProbeOutput = %v, want %v", got, want)
	}
}

func TestParseProbeOutputWithNoReachedLineReturnsNone(t *testing.T) {
	got := parseProbeOutput("garbage\n", []string{"db.internal:5432"})
	if len(got) != 0 {
		t.Fatalf("parseProbeOutput = %v, want none reached", got)
	}
}

func TestATargetTheProbeCannotDialIsAnErrorNotASkippedLine(t *testing.T) {
	d := &Driver{cfg: Config{Network: "none"}}
	for _, tc := range []struct {
		target string
		dials  bool
	}{
		{"db.internal:5432", true},
		{"db.internal:1", true},
		{"db.internal:65535", true},
		{"::1:5432", true},
		{"db.internal", false},
		{":5432", false},
		{"db.internal:", false},
		{"db.internal:0", false},
		{"db.internal:65536", false},
		{"db.internal:pg", false},
	} {
		t.Run(tc.target, func(t *testing.T) {
			_, err := d.ProbeEgress(context.Background(), []string{"api.internal:8080", tc.target})
			if tc.dials && err != nil {
				t.Fatalf("ProbeEgress refused %q: %v", tc.target, err)
			}
			if !tc.dials && (err == nil || !strings.Contains(err.Error(), strconv.Quote(tc.target))) {
				t.Fatalf("ProbeEgress with %q = %v, want an error naming it: a skipped target "+
					"is reported as unreachable", tc.target, err)
			}
		})
	}
}
