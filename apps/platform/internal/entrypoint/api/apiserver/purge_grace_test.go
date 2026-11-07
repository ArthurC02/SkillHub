package apiserver

import (
	"testing"
	"time"
)

func TestTheAccountHandlerStatesTheConfiguredPurgeGrace(t *testing.T) {
	for _, tc := range []struct {
		name       string
		configured time.Duration
		want       time.Duration
	}{
		{"unset keeps the promised thirty days", 0, 720 * time.Hour},
		{"a longer configured grace is the one stated", 1440 * time.Hour, 1440 * time.Hour},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := newAuthHandler(Config{AccountPurgeGrace: tc.configured}, nil).PurgeGrace; got != tc.want {
				t.Errorf("PurgeGrace = %s, want %s", got, tc.want)
			}
		})
	}
}
