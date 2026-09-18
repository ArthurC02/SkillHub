package run

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func answering(t *testing.T, status int) SandboxProvider {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"error":"no"}`))
	}))
	t.Cleanup(srv.Close)
	return NewProvider("test", srv.URL, "")
}

func TestTheAdapterTranslatesEveryRefusalIntoTheDomainsOwnWords(t *testing.T) {
	for _, tc := range []struct {
		status int
		want   error
	}{
		{http.StatusTooManyRequests, ErrProviderFull},
		{http.StatusNotFound, ErrAttemptUnknown},
		{http.StatusInternalServerError, ErrProviderUnavailable},
		{http.StatusServiceUnavailable, ErrProviderUnavailable},
		{http.StatusUnprocessableEntity, ErrProviderRefused},
		{http.StatusUnauthorized, ErrProviderRefused},
	} {
		t.Run(http.StatusText(tc.status), func(t *testing.T) {
			_, err := answering(t, tc.status).Observe(context.Background(), "sbx-1")
			if !errors.Is(err, tc.want) {
				t.Errorf("a %d answer became %v, want %v", tc.status, err, tc.want)
			}
		})
	}
}

func TestAProviderThatCannotBeReachedIsUnavailableRatherThanRefusing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	srv.Close()
	p := NewProvider("test", srv.URL, "")

	_, err := p.Observe(context.Background(), "sbx-1")
	if !errors.Is(err, ErrProviderUnavailable) || !retryable(err) {
		t.Errorf("an unreachable provider gave %v (retryable %v), want an unavailable provider worth another poll",
			err, retryable(err))
	}
}

func TestACancelledCallIsNotBlamedOnTheProvider(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := answering(t, http.StatusOK).Observe(ctx, "sbx-1")
	if errors.Is(err, ErrProviderUnavailable) || retryable(err) {
		t.Errorf("a cancelled call gave %v (retryable %v), want the cancellation itself", err, retryable(err))
	}
}

func TestOnlyAFullOrSilentProviderIsWorthAnotherTry(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want bool
	}{
		{"full", ErrProviderFull, true},
		{"unavailable", ErrProviderUnavailable, true},
		{"refused", ErrProviderRefused, false},
		{"unknown attempt", ErrAttemptUnknown, false},
		{"cancelled", context.Canceled, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := retryable(tc.err); got != tc.want {
				t.Errorf("retryable(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestARunThatCannotMoveOnYetAsksToBeLookedAtAgainInsteadOfFailing(t *testing.T) {
	d := &driver{svc: &Service{}}
	for _, tc := range []struct {
		name string
		err  error
		want time.Duration
	}{
		{"no free slot", d.waitForSlot(), defaultSlotWaitInterval},
		{"another run holds the turn", d.waitForTurn(), defaultSlotWaitInterval},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if !errors.Is(tc.err, ErrTryAgainLater) {
				t.Fatalf("waiting gave %v, want the run to be told to try again later", tc.err)
			}
			var again *tryAgainError
			if !errors.As(tc.err, &again) || again.after != tc.want {
				t.Errorf("the wait carries %v, want %v so the queue knows when to look again", again, tc.want)
			}
		})
	}
}
