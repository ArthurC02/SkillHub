package run

import (
	"strings"
	"testing"
)

func TestArtifactFileNamesAcceptedAndRefused(t *testing.T) {
	for _, tc := range []struct {
		name  string
		file  string
		valid bool
	}{
		{"plain file", "out.txt", true},
		{"nested file", "dir/out.txt", true},
		{"inner space", "a b.txt", true},
		{"name at the byte ceiling", strings.Repeat("a", 1024), true},
		{"name one byte over the ceiling", strings.Repeat("a", 1025), false},
		{"device name con", "con", false},
		{"device name with extension", "CON.txt", false},
		{"device name in a subdirectory", "dir/nul", false},
		{"device name aux", "aux.log", false},
		{"device name prn", "prn", false},
		{"numbered device lpt1", "lpt1", false},
		{"numbered device COM9 with extension", "COM9.txt", false},
		{"com0 is not a device", "com0.txt", true},
		{"com10 is not a device", "com10", true},
		{"console is not a device", "console.log", true},
		{"unit separator control character", "bad\x1fname", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := validArtifactFileName(tc.file); got != tc.valid {
				t.Fatalf("validArtifactFileName(%q) = %v, want %v", tc.file, got, tc.valid)
			}
		})
	}
}
