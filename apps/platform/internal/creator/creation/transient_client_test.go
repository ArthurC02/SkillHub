package creation

import (
	"context"
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
