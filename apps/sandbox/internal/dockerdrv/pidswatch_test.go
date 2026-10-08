package dockerdrv

import "testing"

func TestPidsLimitEventsReadsTheMaxCounter(t *testing.T) {
	for _, tc := range []struct {
		name   string
		raw    string
		want   int64
		wantOK bool
	}{
		{"never hit", "max 0\n", 0, true},
		{"hit once", "max 1\n", 1, true},
		{"hit repeatedly", "max 17\n", 17, true},
		{"without trailing newline", "max 3", 3, true},
		{"empty", "", 0, false},
		{"counter name only", "max\n", 0, false},
		{"non numeric counter", "max many\n", 0, false},
		{"negative counter", "max -1\n", 0, false},
		{"other counter only", "other 5\n", 0, false},
		{"max among other lines", "other 5\nmax 2\n", 2, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := pidsLimitEvents(tc.raw)
			if got != tc.want || ok != tc.wantOK {
				t.Errorf("pidsLimitEvents(%q) = %d, %v, want %d, %v", tc.raw, got, ok, tc.want, tc.wantOK)
			}
		})
	}
}

func TestCgroupV2PathTakesTheUnifiedHierarchyLine(t *testing.T) {
	for _, tc := range []struct {
		name   string
		raw    string
		want   string
		wantOK bool
	}{
		{"unified line", "0::/system.slice/docker-abc.scope\n", "/system.slice/docker-abc.scope", true},
		{"unified line after v1 lines", "12:pids:/x\n0::/docker/abc\n", "/docker/abc", true},
		{"unified line of another container", "0::/system.slice/docker-def.scope\n", "", false},
		{"v1 only", "12:pids:/abc\n", "", false},
		{"empty", "", "", false},
		{"unified line without a path", "0::\n", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := cgroupV2Path(tc.raw, "abc")
			if got != tc.want || ok != tc.wantOK {
				t.Errorf("cgroupV2Path(%q) = %q, %v, want %q, %v", tc.raw, got, ok, tc.want, tc.wantOK)
			}
		})
	}
}
