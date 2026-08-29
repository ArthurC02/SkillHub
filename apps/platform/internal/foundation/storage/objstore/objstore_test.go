package objstore

import (
	"bytes"
	"strings"
	"testing"
)

// Get reads whole objects into memory and nine contexts call it on a request
// path, one of them in a loop. "The write side already caps it" is a transitive
// argument across several packages, and clean mode's in-process backend — a real
// deployment's object store, reachable by any local process — is a live
// counterexample to it. So the read side carries its own bound, the same move
// llmclient made for the same reason ("internal is not trusted").
//
// The ceiling itself is 128 MiB, which is not a size worth allocating in a test;
// what needs an assertion is the boundary behaviour, and that is this function.
func TestReadCappedRefusesRatherThanTruncates(t *testing.T) {
	for _, tc := range []struct {
		name    string
		size    int
		max     int
		wantErr bool
	}{
		{name: "under the ceiling", size: 3, max: 8},
		{name: "exactly at the ceiling", size: 8, max: 8},
		{name: "one byte over", size: 9, max: 8, wantErr: true},
		{name: "far over", size: 800, max: 8, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			want := bytes.Repeat([]byte{0x7f}, tc.size)
			got, err := readCapped(bytes.NewReader(want), tc.max)
			if tc.wantErr {
				if err == nil {
					// The mutation this catches: io.ReadAll(io.LimitReader(r, max))
					// with no length check, which returns a silently short object.
					t.Fatalf("readCapped returned %d bytes and no error for an object over the ceiling", len(got))
				}
				if !strings.Contains(err.Error(), "ceiling") {
					t.Errorf("error %q does not say a limit was hit", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("readCapped: %v", err)
			}
			if !bytes.Equal(got, want) {
				t.Errorf("readCapped returned %d bytes, want the whole %d", len(got), tc.size)
			}
		})
	}
}
