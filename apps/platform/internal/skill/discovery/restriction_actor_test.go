package catalog

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// The operator handlers' own check, and the whole of what it does.
//
// It was called operatorUser and its comment called it 「the second line of
// defence the authz matrix says every private handler has」, which was not true:
// the roster (OPERATOR_USER_IDS) lives on identity's HTTP Handler and this
// package is handed identity's Service, which cannot see it. What the function
// does — refuse a request that reached an operator route with no session, with
// the same 404 RequireOperator gives everybody else — is worth keeping and worth
// testing. What it does not do is now written where the next reader will see it.
func TestTheOperatorHandlersRefuseARequestWithNoSession(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPut, "/admin/skills/x/redistribution", nil)

	if _, ok := sessionActor(w, r); ok {
		t.Fatal("a request with no session was accepted as an operator action")
	}
	// 404 and not 401: SEC-011 requires the endpoint's existence not to be
	// knowable, so the second check must not disclose what the first one hides.
	if w.Code != http.StatusNotFound {
		t.Errorf("refusal answered %d, want 404", w.Code)
	}
}
