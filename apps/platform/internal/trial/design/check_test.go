package testlab

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestACheckIsValidatedByTheInjectedValidatorAndRefusedWithoutOne(t *testing.T) {
	ctx, ws := t.Context(), pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	if got, err := (&Service{}).validateCheck(ctx, ws, nil); got != nil || err != nil {
		t.Errorf("no check: got %+v %v, want none", got, err)
	}
	if _, err := (&Service{}).validateCheck(ctx, ws, &Check{Kind: "asks"}); !errors.Is(err, errChecksUnavailable) {
		t.Errorf("no validator wired: got %v, want the check refused", err)
	}
	refusal := errors.New("bad shape")
	var sawWorkspace pgtype.UUID
	refusing := &Service{ValidateCheck: func(_ context.Context, w pgtype.UUID, _ *Check) (*Check, error) {
		sawWorkspace = w
		return nil, refusal
	}}
	if _, err := refusing.validateCheck(ctx, ws, &Check{Kind: "asks"}); !errors.Is(err, refusal) {
		t.Errorf("a refused check: got %v, want the validator's own error", err)
	}
	if sawWorkspace != ws {
		t.Errorf("the validator saw workspace %v, want %v", sawWorkspace, ws)
	}
	normalized := &Check{Kind: "asks", Values: []string{"trimmed"}}
	accepting := &Service{ValidateCheck: func(context.Context, pgtype.UUID, *Check) (*Check, error) { return normalized, nil }}
	if got, err := accepting.validateCheck(ctx, ws, &Check{Kind: "asks"}); err != nil || got != normalized {
		t.Errorf("an accepted check: got %+v %v, want the validator's normalized check", got, err)
	}
}

func TestAContinuationPromptIsHeldToThePromptLimit(t *testing.T) {
	svc := &Service{}
	if _, err := svc.ContinueSnapshot(t.Context(), nil, pgtype.UUID{}, pgtype.UUID{}, strings.Repeat("a", MaxPromptBytes+1)); !errors.Is(err, ErrPromptTooLong) {
		t.Errorf("one byte past the limit: got %v, want ErrPromptTooLong", err)
	}
	if _, err := svc.ContinueSnapshot(t.Context(), nil, pgtype.UUID{}, pgtype.UUID{}, strings.Repeat("a", MaxPromptBytes)); errors.Is(err, ErrPromptTooLong) {
		t.Errorf("a prompt at the limit was refused as too long")
	}
}
