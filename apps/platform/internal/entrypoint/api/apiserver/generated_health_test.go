package apiserver

import (
	"encoding/json"
	"mime"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGeneratedHealthHandlerMatchesThePublicContract(t *testing.T) {
	t.Parallel()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)

	newGeneratedHealthHandler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", recorder.Code)
	}
	mediaType, _, err := mime.ParseMediaType(recorder.Header().Get("Content-Type"))
	if err != nil {
		t.Fatal(err)
	}
	if mediaType != "application/json" {
		t.Fatalf("content-type = %q; want application/json", mediaType)
	}
	var body struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Status != "ok" {
		t.Fatalf("status body = %q; want ok", body.Status)
	}
}

func TestTheGeneratedGoPackageCarriesModelsAndNoServer(t *testing.T) {
	t.Parallel()
	entries, err := os.ReadDir(filepath.Join("..", "gen"))
	if err != nil {
		t.Fatal(err)
	}
	server := []string{"func NewServer(", "UnimplementedHandler", "func (s *Server) ServeHTTP"}
	models := false
	for _, entry := range entries {
		data, err := os.ReadFile(filepath.Join("..", "gen", entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		text := string(data)
		if strings.Contains(text, "type Health struct") {
			models = true
		}
		for _, declaration := range server {
			if strings.Contains(text, declaration) {
				t.Errorf("%s declares %q; the generated server is not an authorization boundary and nothing "+
					"mounts it, so a route it appears to serve is a route nobody guards", entry.Name(), declaration)
			}
		}
	}
	if !models {
		t.Error("the generated package carries no models either, so this check would pass on an empty directory")
	}
}
