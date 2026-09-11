package dockerdrv

import (
	"reflect"
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
