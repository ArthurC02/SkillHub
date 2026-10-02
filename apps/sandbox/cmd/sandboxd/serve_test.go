package main

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestServingEndsOnlyAfterTheRequestsInFlightAreAnswered(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	srv := &http.Server{
		ReadHeaderTimeout: time.Second,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			close(entered)
			<-release
			w.WriteHeader(http.StatusCreated)
		}),
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, signalled := context.WithCancel(context.Background())
	returned := make(chan error, 1)
	go func() { returned <- serveUntil(ctx, srv, func() error { return srv.Serve(ln) }) }()
	answered := make(chan int, 1)
	go func() {
		resp, err := http.Post("http://"+ln.Addr().String()+"/runs", "application/json", nil)
		if err != nil {
			answered <- 0
			return
		}
		resp.Body.Close()
		answered <- resp.StatusCode
	}()
	<-entered

	signalled()
	select {
	case <-returned:
		t.Fatal("serving returned while a request was still being handled; main would close the driver under it")
	case <-time.After(300 * time.Millisecond):
	}
	close(release)

	if code := <-answered; code != http.StatusCreated {
		t.Errorf("the in-flight request got %d, want its own answer 201", code)
	}
	if err := <-returned; err != nil {
		t.Errorf("serveUntil = %v, want nil after a signalled shutdown", err)
	}
}
