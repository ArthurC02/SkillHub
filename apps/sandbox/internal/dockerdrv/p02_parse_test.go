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

func TestTheDockerProbeRefusesAListWithATargetItCannotDial(t *testing.T) {
	d := &Driver{cfg: Config{Network: "none"}}
	if _, err := d.ProbeEgress(context.Background(), []string{"db.internal:5432"}); err != nil {
		t.Fatalf("ProbeEgress refused a dialable target: %v", err)
	}
	_, err := d.ProbeEgress(context.Background(), []string{"db.internal:5432", "db.internal"})
	if err == nil || !strings.Contains(err.Error(), strconv.Quote("db.internal")) {
		t.Fatalf("ProbeEgress with db.internal = %v, want an error naming it", err)
	}
}
