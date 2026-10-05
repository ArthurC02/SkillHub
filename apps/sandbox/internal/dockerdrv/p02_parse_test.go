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

func TestProbeVerdict(t *testing.T) {
	targets := []string{"db.internal:5432", "api.internal:8080"}
	cases := []struct {
		name     string
		exitCode int64
		out      string
		want     []string
		wantErr  string
	}{
		{"nonzero exit with a REACHED line is an error", 2, "REACHED db.internal:5432\nP02-DONE 2\n", nil, "exited with code 2"},
		{"nonzero exit with empty output is an error", 137, "", nil, "exited with code 137"},
		{"zero exit with empty output is an error", 0, "", nil, "reports -1 of 2"},
		{"zero exit with garbage output and no marker is an error", 0, "fatal error: newosproc\n", nil, "reports -1 of 2"},
		{"marker below the target count is an error", 0, "P02-DONE 1\n", nil, "reports 1 of 2"},
		{"marker above the target count is an error", 0, "P02-DONE 3\n", nil, "reports 3 of 2"},
		{"marker without a number is an error", 0, "P02-DONE x\n", nil, "reports -1 of 2"},
		{"matching marker and no REACHED line is a clean result", 0, "P02-DONE 2\n", nil, ""},
		{"matching marker and a REACHED line reports that target", 0, "REACHED api.internal:8080\nP02-DONE 2\n", []string{"api.internal:8080"}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := probeVerdict(c.exitCode, c.out, targets)
			if c.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), "did not finish") || !strings.Contains(err.Error(), c.wantErr) {
					t.Fatalf("probeVerdict(%d, %q) error = %v, want one containing %q", c.exitCode, c.out, err, c.wantErr)
				}
				if got != nil {
					t.Fatalf("probeVerdict returned %v alongside an error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("probeVerdict(%d, %q) error = %v", c.exitCode, c.out, err)
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("probeVerdict = %v, want %v", got, c.want)
			}
		})
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
