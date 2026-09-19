package run

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestTheRunNeverRepeatsWhatAnExternalSystemSaid(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	srv.Close()
	_, unreachable := NewProvider("test", srv.URL, "").Observe(context.Background(), "sbx-1")
	_, refused := answering(t, http.StatusUnprocessableEntity).Observe(context.Background(), "sbx-1")
	address := srv.Listener.Addr().String()

	d := &driver{}
	for _, tc := range []struct {
		name  string
		class FailureClass
		err   error
		want  statusReason
	}{
		{"a sandbox that cannot be reached", failureProvider, unreachable, "執行沙箱沒有回應"},
		{"a sandbox that refused the request", failureProvider, refused, "執行沙箱沒有接下這次試跑"},
		{
			"a model gateway that would not mint a key", failureProvider,
			&gatewayError{Status: http.StatusInternalServerError, Message: "budget exhausted for key sk-live-1"},
			"模型閘道沒有為這次試跑配發金鑰",
		},
		{
			"a deployment with no model gateway", failureNoProvider, ErrNoModelGateway,
			"這個部署沒有接上模型閘道,試跑沒有辦法連到模型",
		},
		{
			"material the clean test mode will not run", failureNoProvider, ErrContentNotCurated,
			"淨測試模式只跑已策展的內容,這個版本不在其中",
		},
		{
			"an error the platform has no sentence of its own for", failurePlatform,
			fmt.Errorf("dial tcp %s: connect: connection refused", address),
			"平台自己的錯誤",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reason := d.reasonFor(tc.class, tc.err)
			if reason != tc.want {
				t.Fatalf("reason = %q, want %q", reason, tc.want)
			}
			if strings.Contains(string(reason), address) {
				t.Errorf("reason = %q, and it carries this deployment's own address %q to whoever opens the run",
					reason, address)
			}
			for _, leaked := range []string{"dial", "gateway returned", "sk-live-1", "http://"} {
				if strings.Contains(string(reason), leaked) {
					t.Errorf("reason = %q, and it repeats %q from the external system", reason, leaked)
				}
			}
		})
	}
}
