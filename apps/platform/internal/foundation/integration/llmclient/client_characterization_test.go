package llmclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestATruncatedGenerationStillCarriesTheUpstreamStatusAndDetail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(map[string]string{"detail": truncationMarker})
	}))
	defer srv.Close()

	_, err := (&Client{BaseURL: srv.URL}).GenerateSkill(context.Background(),
		GenerateSkillRequest{TaskDescription: "任何任務"})
	if err == nil {
		t.Fatal("a truncated generation came back as success")
	}
	want := ErrGenerateTruncated.Error() + ": llmclient: /v1/generate-skill returned 502: "
	if !strings.HasPrefix(err.Error(), want) || !strings.Contains(err.Error(), truncationMarker) {
		t.Errorf("error = %q, want it to start with %q and keep the upstream detail", err.Error(), want)
	}
}
