package catalog

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTheOperatorHandlersRefuseARequestWithNoSession(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPut, "/admin/skills/x/redistribution", nil)

	if _, ok := sessionActor(w, r); ok {
		t.Fatal("a request with no session was accepted as an operator action")
	}

	if w.Code != http.StatusNotFound {
		t.Errorf("refusal answered %d, want 404", w.Code)
	}
}
