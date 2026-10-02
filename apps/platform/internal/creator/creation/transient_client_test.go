package creation

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type transientRoundTripper func(*http.Request) (*http.Response, error)

func (f transientRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestTransientClientUsesItsComposedHTTPClient(t *testing.T) {
	called := false
	client := &http.Client{Transport: transientRoundTripper(func(request *http.Request) (*http.Response, error) {
		called = request.URL.String() == "http://worker.test/v1/creation/transient-step"
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
	})}

	call := TransientClientWithHTTP("http://worker.test", "worker-token", time.Second, client)
	err := call(context.Background(), JobArgs{}, &Diagram{})
	if err != nil || !called {
		t.Fatalf("call error = %v, used composed client = %v", err, called)
	}
}

func TestATransientStepTheWorkerRefusesKeepsItsCause(t *testing.T) {
	refused := errors.New("connection refused")
	for _, tc := range []struct {
		name  string
		trip  transientRoundTripper
		cause string
	}{
		{"unreachable", func(*http.Request) (*http.Response, error) { return nil, refused }, refused.Error()},
		{"refused", func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusBadGateway, Status: "502 Bad Gateway", Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
		}, "502 Bad Gateway"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			call := TransientClientWithHTTP("http://worker.test", "worker-token", time.Second, &http.Client{Transport: tc.trip})
			err := call(context.Background(), JobArgs{}, &Diagram{})
			if !errors.Is(err, ErrUnavailable) || !strings.Contains(err.Error(), tc.cause) {
				t.Fatalf("err = %v, want ErrUnavailable carrying %q", err, tc.cause)
			}

		})
	}
}
