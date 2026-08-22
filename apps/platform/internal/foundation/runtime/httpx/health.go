// Package httpx holds shared HTTP infrastructure for the control plane.
// It carries no business rules; domain packages live under internal/<domain>.
package httpx

import (
	"encoding/json"
	"net/http"
)

// Health reports process liveness. It must not touch the database or any
// downstream service: it answers "is this process up", not "is the system well".
func Health(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
