package objstore

import (
	"bytes"
	"strings"
	"testing"
)

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
