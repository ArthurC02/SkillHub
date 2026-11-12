package sandbox

import "testing"

func TestCgroupEventCountReadsOneNamedCounter(t *testing.T) {
	for _, tc := range []struct {
		name   string
		raw    string
		key    string
		want   int64
		wantOK bool
	}{
		{"never hit", "max 0\n", "max", 0, true},
		{"hit repeatedly", "low 0\nmax 17\n", "max", 17, true},
		{"without trailing newline", "max 3", "max", 3, true},
		{"empty", "", "max", 0, false},
		{"counter name only", "max\n", "max", 0, false},
		{"non numeric counter", "max many\n", "max", 0, false},
		{"negative counter", "max -1\n", "max", 0, false},
		{"other counter only", "other 5\n", "max", 0, false},
		{"key is not matched by prefix", "oom_kill_extra 3\n", "oom_kill", 0, false},
		{"counter among other lines", "other 5\nmax 2\n", "max", 2, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := CgroupEventCount(tc.raw, tc.key)
			if got != tc.want || ok != tc.wantOK {
				t.Errorf("CgroupEventCount(%q, %q) = %d, %v, want %d, %v", tc.raw, tc.key, got, ok, tc.want, tc.wantOK)
			}
		})
	}
}
