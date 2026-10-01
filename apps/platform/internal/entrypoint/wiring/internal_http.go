package wiring

import (
	"net/http"
	"sync"
	"time"
)

const (
	internalIdleConnsPerHost = 64
	internalIdleConns        = 256
)

var internalTransport = sync.OnceValue(func() *http.Transport {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConnsPerHost = internalIdleConnsPerHost
	transport.MaxIdleConns = internalIdleConns
	return transport
})

func internalHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout, Transport: internalTransport()}
}
