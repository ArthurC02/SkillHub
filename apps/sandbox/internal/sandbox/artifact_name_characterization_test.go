package sandbox

import (
	"strings"
	"testing"
)

func TestArtifactNameRefusesEachUnsafeShape(t *testing.T) {
	refused := []struct {
		name string
		raw  string
	}{
		{"nothing after the collection prefix", "artifacts/"},
		{"one byte past the length ceiling", "artifacts/" + strings.Repeat("a", 1025)},
		{"invalid UTF-8", "artifacts/\xff.txt"},
		{"absolute path", "/etc/passwd"},
		{"leading dash", "artifacts/-rf"},
		{"doubled separator", "artifacts/a//b.txt"},
		{"parent traversal", "artifacts/../x"},
		{"trailing dot", "artifacts/report."},
		{"trailing space", "artifacts/report "},
		{"control character below space", "artifacts/a\x1fb"},
		{"reserved con", "artifacts/con"},
		{"reserved prn with extension", "artifacts/PRN.txt"},
		{"reserved aux", "artifacts/sub/aux"},
		{"reserved nul", "artifacts/nul.log"},
		{"reserved com1", "artifacts/COM1"},
		{"reserved lpt9", "artifacts/lpt9.txt"},
	}
	for _, forbidden := range `<>:"|?*` {
		refused = append(refused, struct {
			name string
			raw  string
		}{"forbidden " + string(forbidden), "artifacts/a" + string(forbidden) + "b"})
	}
	for _, tc := range refused {
		t.Run(tc.name, func(t *testing.T) {
			if got := artifactName(tc.raw); got != "" {
				t.Fatalf("artifactName(%q) = %q, want it refused", tc.raw, got)
			}
		})
	}
}

func TestArtifactNameAcceptsTheNearestSafeShapes(t *testing.T) {
	atCeiling := strings.Repeat("a", 1024)
	for _, tc := range []struct {
		name string
		raw  string
		want string
	}{
		{"exactly the length ceiling", "artifacts/" + atCeiling, atCeiling},
		{"space inside a name", "artifacts/a b.txt", "a b.txt"},
		{"com0 is not a device", "artifacts/com0", "com0"},
		{"com10 is not a device", "artifacts/com10", "com10"},
		{"console is not con", "artifacts/console.txt", "console.txt"},
		{"backslashes become separators", `artifacts\sub\file.txt`, "sub/file.txt"},
		{"leading dot slash is dropped", "./report.txt", "report.txt"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := artifactName(tc.raw); got != tc.want {
				t.Fatalf("artifactName(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}
