package apiserver_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/entrypoint/api/apiserver"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/envx"
)

func readinessRowFor(t *testing.T, cleanMode bool) envx.Status {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), "postgres://skillhub@127.0.0.1:1/skillhub")
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(pool.Close)
	reg := envx.NewRegistry([]envx.Capability{{
		ID: "readiness_probe", Name: "readiness probe",
		Needs:   []string{"SKILLHUB_READINESS_CHARACTERIZATION_NEVER_SET"},
		Without: "nothing works", Fix: "set it",
	}})
	app, err := apiserver.NewApp(apiserver.Config{Pool: pool, Secure: true, Readiness: reg, CleanMode: cleanMode})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /readyz = %d, want 200", rec.Code)
	}
	var body struct {
		Ready        bool          `json:"ready"`
		Capabilities []envx.Status `json:"capabilities"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode /readyz: %v", err)
	}
	if body.Ready || len(body.Capabilities) != 1 {
		t.Fatalf("/readyz = %+v, want one capability and not ready", body)
	}
	return body.Capabilities[0]
}

func TestReadinessHidesWhatIsMissingOutsideCleanMode(t *testing.T) {
	row := readinessRowFor(t, false)
	if row.Readiness != envx.Unavailable {
		t.Errorf("readiness = %q, want %q", row.Readiness, envx.Unavailable)
	}
	if row.Missing != nil || row.Without != "" || row.Fix != "" || row.Detail != "" {
		t.Errorf("a deployment outside clean mode disclosed its gaps: %+v", row)
	}
}

func TestReadinessNamesWhatIsMissingInCleanMode(t *testing.T) {
	row := readinessRowFor(t, true)
	if len(row.Missing) != 1 || row.Missing[0] != "SKILLHUB_READINESS_CHARACTERIZATION_NEVER_SET" {
		t.Errorf("missing = %v, want the one unset variable", row.Missing)
	}
	if row.Without != "nothing works" || row.Fix != "set it" {
		t.Errorf("clean mode dropped the explanation: %+v", row)
	}
}
