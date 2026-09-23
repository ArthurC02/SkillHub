package dockerdrv_test

import (
	"testing"

	"github.com/ArthurC02/skillhub/apps/sandbox/internal/drivertest"
	"github.com/ArthurC02/skillhub/apps/sandbox/internal/sandbox"
)

func TestTheDockerDriverMeetsTheDriverContract(t *testing.T) {
	drivertest.RunContract(t, drivertest.Subject{
		New: func(t *testing.T) sandbox.Driver {
			d, _ := newDriver(t)
			return d
		},
		Handle: handle,
		Request: func(t *testing.T) sandbox.RunRequest {
			t.Helper()
			return testRequest("sleep 30")
		},
	})
}
