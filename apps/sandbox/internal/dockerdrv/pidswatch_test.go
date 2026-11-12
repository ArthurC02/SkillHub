package dockerdrv

import "testing"

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
