//go:build linux

package localdrv

import (
	"os"
	"path/filepath"
	"testing"
)

func TestACgroupReportsTheLimitsItsEventFilesRecord(t *testing.T) {
	const (
		killed   = "low 0\nhigh 0\nmax 9\noom 1\noom_kill 1\n"
		unkilled = "low 0\nhigh 0\nmax 9\noom 0\noom_kill 0\n"
	)
	for _, tc := range []struct {
		name   string
		memory *string
		pids   *string
		want   limitHits
	}{
		{"neither limit hit", ptr(unkilled), ptr("max 0\n"), limitHits{}},
		{"a process was oom killed", ptr(killed), ptr("max 0\n"), limitHits{OOMKilled: true}},
		{"the pids limit was hit", ptr(unkilled), ptr("max 3\n"), limitHits{PidsLimitHit: true}},
		{"both limits hit", ptr(killed), ptr("max 3\n"), limitHits{OOMKilled: true, PidsLimitHit: true}},
		{"memory max events alone are not an oom kill", ptr("max 9\noom_kill 0\n"), ptr("max 0\n"), limitHits{}},
		{"event files absent", nil, nil, limitHits{}},
		{"only the pids file present", nil, ptr("max 2\n"), limitHits{PidsLimitHit: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			write := func(name string, content *string) {
				if content == nil {
					return
				}
				if err := os.WriteFile(filepath.Join(dir, name), []byte(*content), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			write("memory.events", tc.memory)
			write("pids.events", tc.pids)

			if got := (&cgroup{dir: dir}).hits(); got != tc.want {
				t.Errorf("hits() = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func ptr(s string) *string { return &s }
