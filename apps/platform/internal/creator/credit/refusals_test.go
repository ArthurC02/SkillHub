package credit

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestACostEventWithoutAKindOrKeyIsRefused(t *testing.T) {
	for _, tc := range []struct {
		name  string
		event CostEvent
	}{
		{"no kind", CostEvent{IdempotencyKey: "evt-1"}},
		{"no idempotency key", CostEvent{Kind: KindCreationStep}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := newFakeStore()
			s := &Service{Store: store, Config: testConfig()}
			if _, _, err := s.RecordCost(context.Background(), nil, tc.event); !errors.Is(err, ErrInvalid) {
				t.Fatalf("RecordCost = %v, want ErrInvalid", err)
			}
			if len(store.events) != 0 {
				t.Fatalf("a refused cost event was recorded: %v", store.events)
			}
		})
	}
}

func TestAChargeWithoutAKindOrKeyIsRefused(t *testing.T) {
	usd := int64(1_000)
	for _, tc := range []struct {
		name string
		in   ChargeInput
	}{
		{"no kind", ChargeInput{UserID: testUser(21), IdempotencyKey: "c-1", UsdMicros: &usd}},
		{"no idempotency key", ChargeInput{Kind: KindCreationStep, UserID: testUser(21), UsdMicros: &usd}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := newFakeStore()
			s := &Service{Store: store, Config: testConfig()}
			if _, err := s.Charge(context.Background(), nil, tc.in); !errors.Is(err, ErrInvalid) {
				t.Fatalf("Charge = %v, want ErrInvalid", err)
			}
			if len(store.events) != 0 || len(store.applied) != 0 {
				t.Fatalf("a refused charge left events %v and debits %v", store.events, store.applied)
			}
		})
	}
}

func TestAPurgedAccountCannotPassTheStartCheck(t *testing.T) {
	for _, tc := range []struct {
		name  string
		facts AccountFacts
	}{
		{"purged", AccountFacts{Exists: true, Purged: true}},
		{"missing", AccountFacts{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := newFakeStore()
			user := testUser(22)
			store.balances[idKey(user)] = 1_000
			s := &Service{Store: store, Config: testConfig(), Facts: func(context.Context, DBTX, pgtype.UUID) (AccountFacts, error) {
				return tc.facts, nil
			}}
			check, err := s.CanStart(context.Background(), user, KindCreationStep)
			if !errors.Is(err, ErrAccountGone) || check.OK {
				t.Fatalf("CanStart = %+v, %v; want refused with ErrAccountGone", check, err)
			}
		})
	}
}

type pagedStore struct {
	*fakeStore
	stored    int
	requested int32
}

func (p *pagedStore) OwnEntries(_ context.Context, _ pgtype.UUID, page EntryPage) ([]StatementEntry, error) {
	p.requested = page.Limit
	return make([]StatementEntry, min(p.stored, int(page.Limit))), nil
}

func TestAStatementPageHoldsAtMostThePageSizeAndSaysWhenMoreRemain(t *testing.T) {
	for _, tc := range []struct {
		stored   int
		wantLen  int
		wantMore bool
	}{
		{StatementPageSize, StatementPageSize, false},
		{StatementPageSize + 1, StatementPageSize, true},
	} {
		store := &pagedStore{fakeStore: newFakeStore(), stored: tc.stored}
		s := &Service{Store: store, Config: testConfig()}
		entries, more, err := s.Statement(context.Background(), testUser(23), time.Now(), pgtype.UUID{})
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != tc.wantLen || more != tc.wantMore {
			t.Errorf("%d stored: %d entries, more=%t; want %d, more=%t", tc.stored, len(entries), more, tc.wantLen, tc.wantMore)
		}
		if store.requested != StatementPageSize+1 {
			t.Errorf("asked the store for %d entries, want one past the page", store.requested)
		}
	}
}
