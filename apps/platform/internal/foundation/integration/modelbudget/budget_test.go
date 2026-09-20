package modelbudget

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

var judge = Endpoint{Kind: "judge-run", Deadline: 135 * time.Second}

func TestTheCeilingLeavesTheMarginBelowTheCallersDeadline(t *testing.T) {
	for _, tc := range []struct {
		name     string
		deadline time.Duration
		want     int
	}{
		{name: "a long call", deadline: 135 * time.Second, want: 130},
		{name: "exactly the margin", deadline: Margin, want: 0},
		{name: "one second above the margin", deadline: Margin + time.Second, want: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := Endpoint{Kind: "k", Deadline: tc.deadline}
			if got := e.Ceiling(); got != tc.want {
				t.Fatalf("Ceiling() = %d, want %d", got, tc.want)
			}
			if ceiling := time.Duration(e.Ceiling()) * time.Second; ceiling >= e.Deadline {
				t.Errorf("Ceiling() = %s is not below the %s deadline; the caller would stop waiting "+
					"while the gateway call kept running and kept billing", ceiling, e.Deadline)
			}
		})
	}
}

func TestOnlyASecondsValueTheCallerCanStillWaitOutIsAccepted(t *testing.T) {
	for _, tc := range []struct {
		name    string
		seconds int
		want    bool
	}{
		{name: "below the floor", seconds: MinSeconds - 1, want: false},
		{name: "on the floor", seconds: MinSeconds, want: true},
		{name: "on the ceiling", seconds: judge.Ceiling(), want: true},
		{name: "one past the ceiling", seconds: judge.Ceiling() + 1, want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := judge.valid(tc.seconds); got != tc.want {
				t.Fatalf("valid(%d) = %v, want %v", tc.seconds, got, tc.want)
			}
		})
	}
}

func TestAnUnreachableServiceStillHandsOutTheCompiledCeiling(t *testing.T) {
	want := time.Duration(judge.Ceiling()) * time.Second

	var missing *Service
	if got := missing.Within(context.Background(), judge); got != want {
		t.Errorf("a nil service gave %s, want the compiled %s: a model call must not fail because "+
			"nobody wired the settings", got, want)
	}
	if got := (&Service{}).Within(context.Background(), judge); got != want {
		t.Errorf("a service with no database gave %s, want the compiled %s", got, want)
	}
}

func TestSettingRefusesWhatTheOperatorMayNotDecide(t *testing.T) {
	svc := &Service{Endpoints: []Endpoint{judge}}
	ctx, actor := context.Background(), pgtype.UUID{}

	for _, tc := range []struct {
		name    string
		kind    string
		seconds int
		reason  string
		want    error
	}{
		{name: "an endpoint the platform does not call", kind: "no-such-call", seconds: 10,
			reason: "why", want: ErrUnknownKind},
		{name: "no reason at all", kind: judge.Kind, seconds: 10, reason: "", want: ErrReasonRequired},
		{name: "a reason of only spaces", kind: judge.Kind, seconds: 10, reason: "   ",
			want: ErrReasonRequired},
		{name: "a reason past the cap", kind: judge.Kind, seconds: 10,
			reason: strings.Repeat("x", maxReason+1), want: ErrReasonRequired},
		{name: "zero seconds", kind: judge.Kind, seconds: 0, reason: "why", want: ErrOutOfRange},
		{name: "one second past the ceiling", kind: judge.Kind, seconds: judge.Ceiling() + 1,
			reason: "why", want: ErrOutOfRange},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.Set(ctx, tc.kind, tc.seconds, tc.reason, actor)
			if !errors.Is(err, tc.want) {
				t.Fatalf("Set = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestClearingRefusesAnEndpointOrAReasonItCannotAccept(t *testing.T) {
	svc := &Service{Endpoints: []Endpoint{judge}}

	if err := svc.Clear(context.Background(), "no-such-call", "why", pgtype.UUID{}); !errors.Is(err, ErrUnknownKind) {
		t.Errorf("Clear of an unknown endpoint = %v, want %v", err, ErrUnknownKind)
	}
	if err := svc.Clear(context.Background(), judge.Kind, "  ", pgtype.UUID{}); !errors.Is(err, ErrReasonRequired) {
		t.Errorf("Clear without a reason = %v, want %v; returning an endpoint to its default is an "+
			"operator action nobody can explain later otherwise", err, ErrReasonRequired)
	}
}
