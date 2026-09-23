package localdrv

import (
	"crypto/rand"
	"encoding/hex"
	"testing"

	"github.com/ArthurC02/skillhub/apps/sandbox/internal/drivertest"
	"github.com/ArthurC02/skillhub/apps/sandbox/internal/sandbox"
)

func TestTheLocalDriverMeetsTheDriverContract(t *testing.T) {
	drivertest.RunContract(t, drivertest.Subject{
		New: func(t *testing.T) sandbox.Driver {
			nodeBin := requireNode(t)
			d, err := New(Config{
				NodeBin:      nodeBin,
				RunnerScript: testdataScript(t, "workload.mjs"),
				BaseDir:      t.TempDir(),
			})
			if err != nil {
				t.Fatalf("driver: %v", err)
			}
			t.Cleanup(func() { _ = d.Close() })
			return d
		},
		Handle: func(t *testing.T) string {
			t.Helper()
			var b [8]byte
			if _, err := rand.Read(b[:]); err != nil {
				t.Fatalf("handle: %v", err)
			}
			return "contract" + hex.EncodeToString(b[:])
		},
		Request: func(t *testing.T) sandbox.RunRequest {
			t.Helper()
			return minimalRequest("contract-run")
		},
	})
}
