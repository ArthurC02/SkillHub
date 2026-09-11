package credit

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

type fakeTx struct{}

func (fakeTx) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}
func (fakeTx) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return nil, errors.New("fakeTx: Query not implemented")
}
func (fakeTx) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row { return nil }

type fakeStore struct {
	balances map[string]int64
	events   map[string]string
	applied  map[string]bool
	stats    map[string]Statistics
	windows  map[string][]int64
	nextID   int
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		balances: map[string]int64{},
		events:   map[string]string{},
		applied:  map[string]bool{},
		stats:    map[string]Statistics{},
		windows:  map[string][]int64{},
	}
}

func idKey(id pgtype.UUID) string { return string(id.Bytes[:]) }

func (f *fakeStore) Balance(ctx context.Context, _ DBTX, userID pgtype.UUID) (int64, error) {
	return f.balances[idKey(userID)], nil
}

func (f *fakeStore) RecordCostEvent(ctx context.Context, tx DBTX, e CostEvent) (string, bool, error) {
	if id, ok := f.events[e.IdempotencyKey]; ok {
		return id, true, nil
	}
	f.nextID++
	id := fmt.Sprintf("evt-%d", f.nextID)
	f.events[e.IdempotencyKey] = id
	return id, false, nil
}

func (f *fakeStore) ApplyDebit(ctx context.Context, tx DBTX, d DebitEntry) (int64, bool, error) {
	key := idKey(d.UserID)
	if f.applied[d.IdempotencyKey] {
		return f.balances[key], true, nil
	}
	f.applied[d.IdempotencyKey] = true
	f.balances[key] -= d.Credits
	return f.balances[key], false, nil
}

func (f *fakeStore) ApplyGrant(ctx context.Context, tx DBTX, g GrantEntry) (int64, error) {
	key := idKey(g.UserID)
	f.balances[key] += g.Credits
	return f.balances[key], nil
}

func (f *fakeStore) RecentStatistics(ctx context.Context, kind string) (Statistics, error) {
	s, ok := f.stats[kind]
	if !ok {
		return Statistics{}, ErrNoStatistics
	}
	return s, nil
}

func (f *fakeStore) RecomputeStatistics(ctx context.Context, kind string, windowStart, windowEnd time.Time) (Statistics, error) {
	samples := f.windows[kind]
	var max int64
	for _, v := range samples {
		if v > max {
			max = v
		}
	}
	stats := Statistics{SampleCount: len(samples), MaxUsdMicros: max}
	f.stats[kind] = stats
	return stats, nil
}

func testConfig() Config {
	return Config{MicrosPerCredit: 1000, MarkupBps: 13000, DebtFloorCredits: -50, StartFallbackCredits: 70}
}

func testUser(b byte) pgtype.UUID {
	var u pgtype.UUID
	u.Bytes[0] = b
	u.Valid = true
	return u
}

func TestChargeIsIdempotentOnSessionRevision(t *testing.T) {
	store := newFakeStore()
	user := testUser(1)
	store.balances[idKey(user)] = 100
	s := &Service{Store: store, Config: testConfig()}
	usd := int64(1_000_000)
	in := ChargeInput{
		Kind: KindCreationStep, UserID: user, RefType: RefCreationSession, RefID: testUser(1),
		IdempotencyKey: "session-1:rev-3", UsdMicros: &usd, ReservedUsdMicros: usd,
	}
	r1, err := s.Charge(context.Background(), nil, in)
	if err != nil {
		t.Fatal(err)
	}
	if r1.Existed {
		t.Fatal("first charge must not report Existed")
	}

	r2, err := s.Charge(context.Background(), nil, in)
	if err != nil {
		t.Fatal(err)
	}
	if !r2.Existed {
		t.Fatal("replayed charge must report Existed")
	}
	if r1.Credits != r2.Credits || r1.NewBalance != r2.NewBalance {
		t.Fatalf("replay produced a different result: %+v vs %+v", r1, r2)
	}
	if got, want := store.balances[idKey(user)], int64(100)-r1.Credits; got != want {
		t.Fatalf("balance debited twice: got %d, want %d", got, want)
	}
}

func TestChargeUnknownUsageFallsBackToReservedAndMarksEstimated(t *testing.T) {
	store := newFakeStore()
	user := testUser(2)
	s := &Service{Store: store, Config: testConfig()}
	r, err := s.Charge(context.Background(), nil, ChargeInput{
		Kind: KindCreationStep, UserID: user,
		IdempotencyKey: "session-2:rev-1", UsdMicros: nil, ReservedUsdMicros: 5_000_000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !r.Estimated {
		t.Fatal("a charge with unknown actual usage must be marked Estimated")
	}
	if r.Credits == 0 {
		t.Fatal("unknown usage must never charge zero credits (ADR-068 decision 5)")
	}
}

func TestChargeKnownZeroUsageChargesNothing(t *testing.T) {
	store := newFakeStore()
	user := testUser(9)
	s := &Service{Store: store, Config: testConfig()}
	zero := int64(0)
	r, err := s.Charge(context.Background(), nil, ChargeInput{
		Kind: KindCreationStep, UserID: user,
		IdempotencyKey: "session-9:rev-1", UsdMicros: &zero, ReservedUsdMicros: 5_000_000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if r.Estimated {
		t.Fatal("a known cost of zero is not the same as an unknown cost")
	}
	if r.Credits != 0 {
		t.Fatalf("a known zero cost must charge zero credits, got %d", r.Credits)
	}
}

func TestChargeRequiresAUser(t *testing.T) {
	store := newFakeStore()
	s := &Service{Store: store, Config: testConfig()}
	usd := int64(1000)
	if _, err := s.Charge(context.Background(), nil, ChargeInput{
		Kind: KindCreationStep, IdempotencyKey: "x", UsdMicros: &usd,
	}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("Charge with no UserID must be refused with ErrInvalid, got %v", err)
	}
}

func TestChargeRejectsNegativeUsage(t *testing.T) {
	store := newFakeStore()
	s := &Service{Store: store, Config: testConfig()}
	negativeKnown := int64(-1)
	if _, err := s.Charge(context.Background(), nil, ChargeInput{
		Kind: KindCreationStep, UserID: testUser(14), IdempotencyKey: "session-14:rev-1", UsdMicros: &negativeKnown,
	}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("Charge with negative UsdMicros must be refused with ErrInvalid, got %v", err)
	}
	if _, err := s.Charge(context.Background(), nil, ChargeInput{
		Kind: KindCreationStep, UserID: testUser(15), IdempotencyKey: "session-15:rev-1", ReservedUsdMicros: -1,
	}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("Charge with negative ReservedUsdMicros must be refused with ErrInvalid, got %v", err)
	}
}

func TestRecordCostWritesNoDebit(t *testing.T) {
	store := newFakeStore()
	s := &Service{Store: store, Config: testConfig()}

	id, existed, err := s.RecordCost(context.Background(), nil, CostEvent{
		Kind: KindSearchEmbedding, UsdMicros: 10, IdempotencyKey: "search-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if existed {
		t.Fatal("first RecordCost must not report Existed")
	}
	if id == "" {
		t.Fatal("RecordCost must return the cost event id")
	}
	if len(store.balances) != 0 {
		t.Fatalf("RecordCost must never touch any balance, got %v", store.balances)
	}
}

func TestCanAffordStepEnforcesDebtFloor(t *testing.T) {
	store := newFakeStore()
	s := &Service{Store: store, Config: testConfig()}

	reserved := int64(10_000)

	within := testUser(3)
	store.balances[idKey(within)] = -30
	ok, err := s.CanAffordStep(context.Background(), within, reserved)
	if err != nil || !ok {
		t.Fatalf("balance -30 minus 13 credits must stay within the -50 floor: ok=%v err=%v", ok, err)
	}

	beyond := testUser(4)
	store.balances[idKey(beyond)] = -40
	ok, err = s.CanAffordStep(context.Background(), beyond, reserved)
	if err != nil || ok {
		t.Fatalf("balance -40 minus 13 credits must trip the -50 floor: ok=%v err=%v", ok, err)
	}
}

func TestCanStartUsesP95WithMarkupWhenEnoughSamples(t *testing.T) {
	store := newFakeStore()
	store.stats[KindGenerate] = Statistics{SampleCount: MinStatSamples, P95UsdMicros: 30_000_000, WindowEnd: time.Now()}
	s := &Service{Store: store, Config: testConfig()}
	user := testUser(5)
	store.balances[idKey(user)] = 1000
	check, err := s.CanStart(context.Background(), user, KindGenerate)
	if err != nil {
		t.Fatal(err)
	}
	billed, err := BilledMicros(30_000_000, 13000)
	if err != nil {
		t.Fatal(err)
	}
	want := CreditsForMicros(billed, 1000)
	if check.Threshold != want {
		t.Fatalf("Threshold = %d, want %d", check.Threshold, want)
	}
	if check.Estimated {
		t.Fatal("a threshold derived from MinStatSamples or more samples must not be marked Estimated")
	}
}

func TestCanStartFallsBackWhenNoStatisticsExist(t *testing.T) {
	store := newFakeStore()
	s := &Service{Store: store, Config: testConfig()}
	user := testUser(6)
	store.balances[idKey(user)] = 100
	check, err := s.CanStart(context.Background(), user, KindGenerate)
	if err != nil {
		t.Fatal(err)
	}
	if !check.Estimated || check.Threshold != 70 {
		t.Fatalf("expected the fallback constant 70 marked Estimated, got %+v", check)
	}
}

func TestCanStartAllowsAStartExactlyAtTheThreshold(t *testing.T) {
	store := newFakeStore()
	s := &Service{Store: store, Config: testConfig()}
	for _, tc := range []struct {
		balance int64
		want    bool
	}{{69, false}, {70, true}} {
		user := testUser(byte(30 + tc.balance%10))
		store.balances[idKey(user)] = tc.balance
		check, err := s.CanStart(context.Background(), user, KindGenerate)
		if err != nil {
			t.Fatal(err)
		}
		if check.OK != tc.want {
			t.Errorf("balance %d against the fallback threshold 70: OK = %v, want %v", tc.balance, check.OK, tc.want)
		}
	}
}

func TestCanStartFallsBackBelowMinSamples(t *testing.T) {
	store := newFakeStore()
	store.stats[KindGenerate] = Statistics{SampleCount: MinStatSamples - 1, P95UsdMicros: 30_000_000, WindowEnd: time.Now()}
	s := &Service{Store: store, Config: testConfig()}
	user := testUser(10)
	check, err := s.CanStart(context.Background(), user, KindGenerate)
	if err != nil {
		t.Fatal(err)
	}
	if !check.Estimated || check.Threshold != 70 {
		t.Fatalf("a sample count below MinStatSamples must fall back to the constant, got %+v", check)
	}
}

func TestGrantRequiresReasonAndOperator(t *testing.T) {
	store := newFakeStore()
	s := &Service{Store: store, Config: testConfig()}
	user := testUser(7)
	if _, err := s.Grant(context.Background(), nil, GrantInput{
		UserID: user, EntryKind: EntryGrant, Credits: 50, Reason: "", IdempotencyKey: "g1", OperatorID: testUser(99),
	}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("a grant without a reason must be refused with ErrInvalid, got %v", err)
	}
	if _, err := s.Grant(context.Background(), nil, GrantInput{
		UserID: user, EntryKind: EntryGrant, Credits: 50, Reason: "beta reward", IdempotencyKey: "g2",
	}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("a self-service grant with no operator id must be refused with ErrInvalid, got %v", err)
	}
}

func TestOnlyAnAdjustmentMayLowerABalance(t *testing.T) {
	s := &Service{Store: newFakeStore(), Config: testConfig()}
	base := GrantInput{UserID: testUser(21), Credits: -10, Reason: "corrects an over-grant", OperatorID: testUser(99)}

	lowering := base
	lowering.EntryKind, lowering.IdempotencyKey = EntryGrant, "g-lower-grant"
	if _, err := s.Grant(context.Background(), fakeTx{}, lowering); !errors.Is(err, ErrGrantLowersBalance) {
		t.Fatalf("a negative grant must be refused with ErrGrantLowersBalance, got %v", err)
	}
	adjustment := base
	adjustment.EntryKind, adjustment.IdempotencyKey = EntryAdjustment, "g-lower-adjustment"
	balance, err := s.Grant(context.Background(), fakeTx{}, adjustment)
	if err != nil {
		t.Fatalf("a negative adjustment must be accepted, got %v", err)
	}
	if balance != -10 {
		t.Fatalf("balance after a -10 adjustment = %d, want -10", balance)
	}
}

func TestAnOperatorEntryIsAnAdjustmentExactlyWhenItLowersTheBalance(t *testing.T) {
	for _, tc := range []struct {
		credits int64
		want    string
	}{{-1, EntryAdjustment}, {1, EntryGrant}} {
		if got := OperatorEntryKind(tc.credits); got != tc.want {
			t.Errorf("OperatorEntryKind(%d) = %q, want %q", tc.credits, got, tc.want)
		}
	}
}

func TestGrantAppliesToBalance(t *testing.T) {
	store := newFakeStore()
	s := &Service{Store: store, Config: testConfig()}
	user := testUser(8)
	balance, err := s.Grant(context.Background(), fakeTx{}, GrantInput{
		UserID: user, EntryKind: EntryGrant, Credits: 50, Reason: "beta reward",
		OperatorID: testUser(99), IdempotencyKey: "g3",
	})
	if err != nil {
		t.Fatal(err)
	}
	if balance != 50 {
		t.Fatalf("balance after a 50-credit grant = %d, want 50", balance)
	}
}

func TestFactsBlockPurgedAccount(t *testing.T) {
	store := newFakeStore()
	user := testUser(11)
	s := &Service{Store: store, Config: testConfig(), Facts: func(ctx context.Context, _ DBTX, id pgtype.UUID) (AccountFacts, error) {
		return AccountFacts{Exists: true, Purged: true}, nil
	}}
	if _, err := s.Grant(context.Background(), nil, GrantInput{
		UserID: user, EntryKind: EntryGrant, Credits: 10, Reason: "beta reward",
		OperatorID: testUser(99), IdempotencyKey: "g4",
	}); !errors.Is(err, ErrAccountGone) {
		t.Fatalf("a grant to a purged account must be refused with ErrAccountGone, got %v", err)
	}
}

func TestGrantRejectsUnrecognizedEntryKind(t *testing.T) {
	store := newFakeStore()
	s := &Service{Store: store, Config: testConfig()}
	if _, err := s.Grant(context.Background(), nil, GrantInput{
		UserID: testUser(13), EntryKind: "bogus", Credits: 50, Reason: "beta reward",
		OperatorID: testUser(99), IdempotencyKey: "g6",
	}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("Grant with an unrecognized EntryKind must be refused with ErrInvalid, got %v", err)
	}
}

func TestFactsBlockPurgedAccountOnCharge(t *testing.T) {
	store := newFakeStore()
	user := testUser(16)
	s := &Service{Store: store, Config: testConfig(), Facts: func(ctx context.Context, _ DBTX, id pgtype.UUID) (AccountFacts, error) {
		return AccountFacts{Exists: true, Purged: true}, nil
	}}
	usd := int64(1_000)
	if _, err := s.Charge(context.Background(), nil, ChargeInput{
		Kind: KindCreationStep, UserID: user, IdempotencyKey: "session-16:rev-1", UsdMicros: &usd,
	}); !errors.Is(err, ErrAccountGone) {
		t.Fatalf("a charge to a purged account must be refused with ErrAccountGone, got %v", err)
	}
	if len(store.events) != 0 {
		t.Fatalf("a refused charge must not record a cost event, got %v", store.events)
	}
	if len(store.applied) != 0 {
		t.Fatalf("a refused charge must not apply a debit, got %v", store.applied)
	}
}

func TestRecomputeStatisticsWritesAndReturnsTheResult(t *testing.T) {
	store := newFakeStore()
	store.windows[KindCreationStep] = []int64{10, 20, 30, 40, 50}
	s := &Service{Store: store, Config: testConfig()}
	got, err := s.RecomputeStatistics(context.Background(), KindCreationStep, 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if got.SampleCount != 5 || got.MaxUsdMicros != 50 {
		t.Fatalf("RecomputeStatistics = %+v, want SampleCount=5 MaxUsdMicros=50", got)
	}
	if stored := store.stats[KindCreationStep]; stored != got {
		t.Fatalf("the computed statistics were not written back: %+v", stored)
	}
}

func TestServiceMethodsFailClosedWithoutAStore(t *testing.T) {
	s := &Service{}
	if _, err := s.Charge(context.Background(), nil, ChargeInput{}); !errors.Is(err, ErrUnavailable) {
		t.Errorf("Charge without a Store = %v, want ErrUnavailable", err)
	}
	if _, _, err := s.RecordCost(context.Background(), nil, CostEvent{}); !errors.Is(err, ErrUnavailable) {
		t.Error("RecordCost without a Store must fail closed")
	}
	if _, err := s.CanAffordStep(context.Background(), testUser(1), 0); !errors.Is(err, ErrUnavailable) {
		t.Error("CanAffordStep without a Store must fail closed")
	}
	if _, err := s.CanStart(context.Background(), testUser(1), KindGenerate); !errors.Is(err, ErrUnavailable) {
		t.Error("CanStart without a Store must fail closed")
	}
	if _, err := s.Grant(context.Background(), nil, GrantInput{}); !errors.Is(err, ErrUnavailable) {
		t.Error("Grant without a Store must fail closed")
	}
}

func (f *fakeStore) SummarizeSession(context.Context, DBTX, pgtype.UUID) error { return nil }

func (f *fakeStore) SweepSessionSummaries(context.Context, time.Time, time.Time) (int64, error) {
	return 0, nil
}

func TestAccountChecksReadThroughTheCallersTransaction(t *testing.T) {
	store := newFakeStore()
	user := testUser(12)
	store.balances[idKey(user)] = 100
	var seen []DBTX
	s := &Service{Store: store, Config: testConfig(), Facts: func(_ context.Context, db DBTX, _ pgtype.UUID) (AccountFacts, error) {
		seen = append(seen, db)
		return AccountFacts{Exists: true}, nil
	}}
	if _, err := s.Grant(context.Background(), fakeTx{}, GrantInput{
		UserID: user, EntryKind: EntryGrant, Credits: 10, Reason: "beta reward",
		OperatorID: testUser(99), IdempotencyKey: "g5",
	}); err != nil {
		t.Fatal(err)
	}
	usd := int64(1_000)
	if _, err := s.Charge(context.Background(), fakeTx{}, ChargeInput{
		Kind: KindCreationStep, UserID: user, IdempotencyKey: "session-12:rev-1",
		UsdMicros: &usd, ReservedUsdMicros: usd,
	}); err != nil {
		t.Fatal(err)
	}
	if len(seen) != 2 {
		t.Fatalf("account checks = %d, want one for the grant and one for the charge", len(seen))
	}
	for i, db := range seen {
		if db == nil {
			t.Errorf("account check %d ran outside the caller's transaction; on a one-connection pool it waits forever", i+1)
		}
	}
}

func TestStatisticsPastTheirShelfLifeFallBackToTheConservativeThreshold(t *testing.T) {
	for _, tc := range []struct {
		name      string
		age       time.Duration
		estimated bool
		threshold int64
	}{
		{"an hour inside the shelf life", MaxStatisticsAge - time.Hour, false, 39_000},
		{"an hour past the shelf life", MaxStatisticsAge + time.Hour, true, 70},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := newFakeStore()
			store.stats[KindGenerate] = Statistics{SampleCount: MinStatSamples, P95UsdMicros: 30_000_000, WindowEnd: time.Now().Add(-tc.age)}
			s := &Service{Store: store, Config: testConfig()}
			check, err := s.CanStart(context.Background(), testUser(21), KindGenerate)
			if err != nil {
				t.Fatal(err)
			}
			if check.Estimated != tc.estimated || check.Threshold != tc.threshold {
				t.Errorf("threshold = %d (estimated %v), want %d (estimated %v)", check.Threshold, check.Estimated, tc.threshold, tc.estimated)
			}
		})
	}
}

func (f *fakeStore) RecentEntries(context.Context, DBTX, pgtype.UUID, int32) ([]LedgerEntry, error) {
	return nil, nil
}

func (f *fakeStore) LatestStatistics(context.Context) ([]KindStatistics, error) { return nil, nil }
