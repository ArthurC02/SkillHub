package testlab

import (
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestACheckIsValidatedByTheInjectedValidatorAndRefusedWithoutOne(t *testing.T) {
	if got, err := (&Service{}).validateCheck(nil); got != nil || err != nil {
		t.Errorf("no check: got %+v %v, want none", got, err)
	}
	if _, err := (&Service{}).validateCheck(&Check{Kind: "asks"}); !errors.Is(err, errChecksUnavailable) {
		t.Errorf("no validator wired: got %v, want the check refused", err)
	}
	refusing := &Service{ValidateCheck: func(*Check) (*Check, error) { return nil, errors.New("bad shape") }}
	if _, err := refusing.validateCheck(&Check{Kind: "asks"}); !errors.Is(err, ErrInvalid) {
		t.Errorf("a refused check: got %v, want ErrInvalid so the caller answers 400", err)
	}
	normalized := &Check{Kind: "asks", Values: []string{"trimmed"}}
	accepting := &Service{ValidateCheck: func(*Check) (*Check, error) { return normalized, nil }}
	if got, err := accepting.validateCheck(&Check{Kind: "asks"}); err != nil || got != normalized {
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
