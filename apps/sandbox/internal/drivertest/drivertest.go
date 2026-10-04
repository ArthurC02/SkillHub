package drivertest

import (
	"context"
	"errors"
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
	t.Run("a skill package whose bytes are not the ones named fails the dispatch", func(t *testing.T) {
		if err := startWithPackage(t, subject, otherPackageSHA256); !errors.Is(err, sandbox.ErrInputDigestMismatch) {
			t.Fatalf("Start = %v, want ErrInputDigestMismatch: the sandbox would run a package nobody admitted", err)
		}
	})
	t.Run("a skill package whose bytes are the ones named is delivered", func(t *testing.T) {
		if err := startWithPackage(t, subject, servedPackageSHA256); err != nil {
			t.Fatalf("Start = %v, want nil", err)
		}
	})
}

const (
	servedPackage       = "hello"
	servedPackageSHA256 = "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	otherPackageSHA256  = "0000000000000000000000000000000000000000000000000000000000000000"
)

func startWithPackage(t *testing.T, subject Subject, packageSHA256 string) error {
	t.Helper()
	driver := subject.New(t)
	served := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(servedPackage))
	}))
	t.Cleanup(served.Close)

	req := subject.Request(t)
	req.SkillVersion.PackageSHA256 = packageSHA256
	req.ObjectGrants = []sandbox.ObjectGrant{{
		Purpose:   "skill_package",
		ObjectKey: "packages/test.zip",
		Access:    "read",
		URL:       served.URL + "/packages/test.zip",
		ExpiresAt: time.Now().Add(time.Hour),
	}}
	id := subject.Handle(t)
	t.Cleanup(func() { _ = driver.Remove(context.Background(), id) })
	return driver.Start(context.Background(), id, req)
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
