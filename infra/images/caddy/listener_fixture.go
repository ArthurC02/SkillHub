package caddy

import (
	"net/http"
	"sync"
)

var fixturePendingListeners sync.Map

func FixtureRegisterListener(server *http.Server) {
	if server == nil {
		return
	}
	done := make(chan struct{})
	var once sync.Once
	fixturePendingListeners.Store(done, struct{}{})
	server.RegisterOnShutdown(func() {
		once.Do(func() {
			close(done)
			fixturePendingListeners.Delete(done)
		})
	})
}

func FixtureWaitListeners() {
	fixturePendingListeners.Range(func(done, _ any) bool {
		<-done.(chan struct{})
		return true
	})
}
