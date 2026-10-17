package packaging

import (
	"io/fs"
	"testing"
	"testing/fstest"
)

func TestAPathThatOnlyWindowsWouldReadAsNestedOrRootedNeverTravels(t *testing.T) {
	regular, err := fs.Stat(fstest.MapFS{"f": {Data: []byte("x")}}, "f")
	if err != nil {
		t.Fatal(err)
	}
	entry := fs.FileInfoToDirEntry(regular)
	for _, tc := range []struct {
		path       string
		wantTravel bool
	}{
		{"scripts/run.sh", true},
		{`scripts\..\..\escape.sh`, false},
		{`a\b`, false},
		{"C:escape.sh", false},
	} {
		t.Run(tc.path, func(t *testing.T) {
			ok, reason := travels(tc.path, entry)
			if ok != tc.wantTravel || (!tc.wantTravel && reason != ReasonUnsafePath) {
				t.Errorf("travels(%q) = %v, %q; want travel %v, or %q when refused", tc.path, ok, reason, tc.wantTravel, ReasonUnsafePath)
			}
		})
	}
}
