package localdrv

import "testing"

func TestEventOccurredReadsOneNamedCounter(t *testing.T) {
	const memoryEvents = "low 0\nhigh 0\nmax 4\noom 2\noom_kill 1\noom_group_kill 0\n"
	for _, tc := range []struct {
		name string
		raw  string
		key  string
		want bool
	}{
		{"oom_kill counted once", memoryEvents, "oom_kill", true},
		{"oom counted but oom_kill zero is a different key", "oom 2\noom_kill 0\n", "oom_kill", false},
		{"key is not matched by prefix", "oom_kill_extra 3\n", "oom_kill", false},
		{"pids max never hit", "max 0\n", "max", false},
		{"pids max hit once", "max 1\n", "max", true},
		{"pids max hit repeatedly without trailing newline", "max 17", "max", true},
		{"counter name only", "max\n", "max", false},
		{"non numeric counter", "max many\n", "max", false},
		{"negative counter", "max -1\n", "max", false},
		{"empty", "", "max", false},
		{"counter among other lines", "other 5\nmax 2\n", "max", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := eventOccurred(tc.raw, tc.key); got != tc.want {
				t.Errorf("eventOccurred(%q, %q) = %v, want %v", tc.raw, tc.key, got, tc.want)
			}
		})
	}
}

func TestEventCountReturnsTheCounterValue(t *testing.T) {
	got, ok := eventCount("low 0\nmax 17\n", "max")
	if got != 17 || !ok {
		t.Errorf("eventCount = %d, %v, want 17, true", got, ok)
	}
}
