package creation

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	identity "github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestStartingASessionChecksTheRequestBeforeAnyDatabaseWork(t *testing.T) {
	id := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	l := testLimits()
	for _, c := range []struct {
		name    string
		limits  Limits
		id      pgtype.UUID
		message string
		budget  float64
		want    error
	}{
		{"limits not configured", Limits{}, id, "hi", .5, ErrUnavailable},
		{"no id", l, pgtype.UUID{}, "hi", .5, ErrInvalidCommand},
		{"budget not a number", l, id, "hi", math.NaN(), ErrInvalidCommand},
		{"infinite budget", l, id, "hi", math.Inf(1), ErrInvalidCommand},
		{"message one rune over", l, id, strings.Repeat("字", maxPersonMessageRunes+1), .5, ErrInvalidCommand},
		{"message at the rune limit", l, id, strings.Repeat("字", maxPersonMessageRunes), .5, nil},
		{"budget below one call", l, id, "hi", l.MaxCallCostUSD - .001, ErrBudgetOutOfBand},
		{"budget of exactly one call", l, id, "hi", l.MaxCallCostUSD, nil},
		{"budget at the ceiling", l, id, "hi", l.MaxCostUSD, nil},
		{"budget over the ceiling", l, id, "hi", l.MaxCostUSD + .001, ErrBudgetOutOfBand},
	} {
		t.Run(c.name, func(t *testing.T) {
			err := (&Service{Limits: c.limits}).admitStart(context.Background(), identity.Workspace{}, c.id, c.message, c.budget)
			if !errors.Is(err, c.want) {
				t.Fatalf("err = %v, want %v", err, c.want)
			}
		})
	}
}

func TestTheCreditGateComesFirst(t *testing.T) {
	refused := errors.New("ledger down")
	for _, c := range []struct {
		name string
		ok   bool
		err  error
		want error
	}{{"below the threshold", false, nil, ErrCreditThreshold}, {"the gate failed", false, refused, refused}} {
		s := &Service{CreditCanStart: func(context.Context, pgtype.UUID) (bool, error) { return c.ok, c.err }}
		if err := s.admitStart(context.Background(), identity.Workspace{}, pgtype.UUID{}, "hi", math.NaN()); !errors.Is(err, c.want) {
			t.Errorf("%s: err = %v, want %v", c.name, err, c.want)
		}
	}
}

func startedRow(t *testing.T, key string, expires time.Time) gen.CreationSession {
	t.Helper()
	row := storedSession(t, StateWaitingInput, 1, envelope{StartHash: key})
	row.ExpiresAt = pgtype.Timestamptz{Time: expires, Valid: true}
	return row
}

func TestResumingAStartReturnsTheSameSessionOnlyForTheSameRequest(t *testing.T) {
	if _, err := resumeStart(startedRow(t, "other", time.Now().Add(time.Hour)), "key"); !errors.Is(err, ErrReplayMismatch) {
		t.Errorf("another request: err = %v, want ErrReplayMismatch", err)
	}
	if _, err := resumeStart(startedRow(t, "key", time.Now().Add(-time.Second)), "key"); !errors.Is(err, ErrNotFound) {
		t.Errorf("expired: err = %v, want ErrNotFound", err)
	}
	if v, err := resumeStart(startedRow(t, "key", time.Now().Add(time.Hour)), "key"); err != nil || v.State != string(StateWaitingInput) || v.Revision != 1 {
		t.Errorf("the same request: view = %+v, err = %v", v, err)
	}
	broken := gen.CreationSession{Snapshot: []byte(`{"snapshot": 5}`)}
	if _, err := resumeStart(broken, "key"); err == nil || errors.Is(err, ErrReplayMismatch) || errors.Is(err, ErrNotFound) {
		t.Errorf("undecodable: err = %v, want the decoding error itself", err)
	}
}

func TestAnEmptyFirstMessageWaitsForThePerson(t *testing.T) {
	s := &Service{Limits: testLimits(), CatalogCheck: func(context.Context, identity.Workspace, string) ([]Reference, float64, error) {
		t.Fatal("nothing to check")
		return nil, 0, nil
	}}
	e, state := s.openingEnvelope(context.Background(), identity.Workspace{}, "  ", .5, "key")
	if state != StateWaitingInput || len(e.Snapshot.Messages) != 0 || e.Snapshot.References == nil || e.StartHash != "key" || e.Snapshot.BudgetUSD != .5 || *e.Snapshot.SpentUSD != 0 {
		t.Fatalf("state = %s, envelope = %+v", state, e)
	}
	if d := time.Until(e.Deadline); d <= 0 || d > testLimits().SessionTimeout {
		t.Fatalf("deadline in %v, want within the session timeout", d)
	}
}

func TestAFirstMessageIsMaskedAndQueued(t *testing.T) {
	s := &Service{Limits: testLimits(), Mask: func(v string) string { return "masked:" + v }}
	e, state := s.openingEnvelope(context.Background(), identity.Workspace{}, "hi", .5, "key")
	if state != StateQueued || len(e.Snapshot.Messages) != 1 || e.Snapshot.Messages[0].Content != "masked:hi" || e.Snapshot.CatalogChecked {
		t.Fatalf("state = %s, snapshot = %+v", state, e.Snapshot)
	}
}

func TestTheCatalogCheckOffersAtMostThreeSkillsBeforeTheModelRuns(t *testing.T) {
	var asked string
	s := &Service{Limits: testLimits(), CatalogCheck: func(_ context.Context, _ identity.Workspace, q string) ([]Reference, float64, error) {
		asked = q
		return []Reference{{SkillID: "1", Confirmed: true}, {SkillID: "2"}, {SkillID: "3"}, {SkillID: "4"}}, .01, nil
	}}
	e, state := s.openingEnvelope(context.Background(), identity.Workspace{}, "summarise", .5, "key")
	p := e.Snapshot
	if state != StateWaitingConfirmation || asked != "summarise" || !p.CatalogChecked || len(p.References) != 3 || p.References[0].Confirmed || p.PendingAction != "confirm_references" || *p.SpentUSD != .01 {
		t.Fatalf("state = %s, snapshot = %+v", state, p)
	}
}

func TestAFailedCatalogCheckStillChargesAndCarriesOn(t *testing.T) {
	s := &Service{Limits: testLimits(), CatalogCheck: func(context.Context, identity.Workspace, string) ([]Reference, float64, error) {
		return nil, .01, errors.New("search down")
	}}
	e, state := s.openingEnvelope(context.Background(), identity.Workspace{}, "summarise", .5, "key")
	if state != StateQueued || e.Snapshot.CatalogChecked || *e.Snapshot.SpentUSD != .01 {
		t.Fatalf("state = %s, snapshot = %+v", state, e.Snapshot)
	}
}

func TestSpendIsAddedOnlyForAPositiveCostOnAKnownTotal(t *testing.T) {
	for name, cost := range map[string]float64{"zero": 0, "negative": -.01} {
		spent := .1
		p := &Snapshot{SpentUSD: &spent}
		addSpend(p, cost)
		if *p.SpentUSD != .1 {
			t.Errorf("%s: spent = %v", name, *p.SpentUSD)
		}
	}
	p := &Snapshot{}
	addSpend(p, .01)
	if p.SpentUSD != nil {
		t.Errorf("an unknown total became %v", *p.SpentUSD)
	}
}
