package drivertest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ArthurC02/skillhub/apps/sandbox/internal/sandbox"
)

type Subject struct {
	New     func(t *testing.T) sandbox.Driver
	Handle  func(t *testing.T) string
	Request func(t *testing.T) sandbox.RunRequest
}

func RunContract(t *testing.T, subject Subject) {
	t.Helper()
	t.Run("a granted object that cannot be fetched fails the dispatch", func(t *testing.T) {
		grantThatCannotBeFetchedFailsTheDispatch(t, subject)
	})
}

func grantThatCannotBeFetchedFailsTheDispatch(t *testing.T, subject Subject) {
	driver := subject.New(t)
	refused := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer refused.Close()

	req := subject.Request(t)
	req.ObjectGrants = []sandbox.ObjectGrant{{
		Purpose:   "skill_package",
		ObjectKey: "packages/test.zip",
		Access:    "read",
		URL:       refused.URL + "/packages/test.zip",
		ExpiresAt: time.Now().Add(time.Hour),
	}}

	id := subject.Handle(t)
	t.Cleanup(func() { _ = driver.Remove(context.Background(), id) })

	err := driver.Start(context.Background(), id, req)
	if err == nil {
		t.Fatal("a skill package that could not be fetched was treated as delivered")
	}
	if strings.Contains(err.Error(), refused.URL) {
		t.Errorf("the grant URL leaked into the error: %v", err)
	}
	if !strings.Contains(err.Error(), "skill_package") {
		t.Errorf("error does not name what could not be placed: %v", err)
	}
}
