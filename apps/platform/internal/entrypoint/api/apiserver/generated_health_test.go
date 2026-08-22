package apiserver

import (
	"encoding/json"
	"mime"
	"net/http"
	"net/http/httptest"
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

func TestGeneratedHealthHandlerFailsClosedForProtectedOperations(t *testing.T) {
	t.Parallel()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/me", nil)

	newGeneratedHealthHandler().ServeHTTP(recorder, request)

	if recorder.Code >= 200 && recorder.Code < 300 {
		t.Fatalf("protected generated operation returned success: %d", recorder.Code)
	}
}
