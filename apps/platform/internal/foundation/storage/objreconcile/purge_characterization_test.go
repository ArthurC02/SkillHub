package objreconcile

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type guardTx struct{ pgx.Tx }

type purgeLedger struct {
	steps     []string
	removeErr map[string]error
	markErr   map[byte]error
	retain    map[string]bool
	marked    []byte
	markTxs   []pgx.Tx
}

func (l *purgeLedger) Exists(context.Context, string) (bool, error) { return true, nil }

func (l *purgeLedger) Remove(_ context.Context, key string) error {
	l.steps = append(l.steps, "remove "+key)
	return l.removeErr[key]
}

func (l *purgeLedger) mark(_ context.Context, tx pgx.Tx, id pgtype.UUID) error {
	if err := l.markErr[id.Bytes[0]]; err != nil {
		return err
	}
	l.steps = append(l.steps, "mark "+string('0'+id.Bytes[0]))
	l.marked = append(l.marked, id.Bytes[0])
	l.markTxs = append(l.markTxs, tx)
	return nil
}

func (l *purgeLedger) guard(_ context.Context, key string, action func(bool, pgx.Tx) error) error {
	l.steps = append(l.steps, "guard "+key)
	return action(l.retain[key], guardTx{})
}

func candidate(id byte, key string) Candidate {
	return Candidate{ID: pgtype.UUID{Bytes: [16]byte{id}, Valid: true}, ObjectKey: key}
}

func listing(rows ...Candidate) ListFunc {
	return func(context.Context, int32) ([]Candidate, error) { return rows, nil }
}

func unreachablePool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), "postgres://unused:unused@127.0.0.1:1/unused")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func TestPurgeExpiredRefusesWithoutItsCollaborators(t *testing.T) {
	pool, store, list := unreachablePool(t), &purgeLedger{}, listing(candidate(1, "a"))
	for name, call := range map[string]func() (int, error){
		"no pool":  func() (int, error) { return PurgeExpired(context.Background(), nil, store, list, noMark, nil, 1) },
		"no store": func() (int, error) { return PurgeExpired(context.Background(), pool, nil, list, noMark, nil, 1) },
		"no list":  func() (int, error) { return PurgeExpired(context.Background(), pool, store, nil, noMark, nil, 1) },
		"no mark":  func() (int, error) { return PurgeExpired(context.Background(), pool, store, list, nil, nil, 1) },
	} {
		if n, err := call(); err == nil || n != 0 {
			t.Errorf("%s: purged=%d err=%v, want a refusal", name, n, err)
		}
	}
	if len(store.steps) != 0 {
		t.Errorf("a refused sweep touched storage: %v", store.steps)
	}
}

func TestPurgeExpiredReportsAWorklistItCouldNotRead(t *testing.T) {
	want := errors.New("worklist unreadable")
	store := &purgeLedger{}
	n, err := PurgeExpired(context.Background(), unreachablePool(t), store,
		func(context.Context, int32) ([]Candidate, error) { return nil, want }, noMark, store.guard, 1)
	if !errors.Is(err, want) || n != 0 || len(store.steps) != 0 {
		t.Errorf("purged=%d err=%v steps=%v, want the read error and nothing done", n, err, store.steps)
	}
}

func TestPurgeExpiredPassesItsLimitToTheWorklist(t *testing.T) {
	var asked int32
	_, err := PurgeExpired(context.Background(), unreachablePool(t), &purgeLedger{},
		func(_ context.Context, limit int32) ([]Candidate, error) { asked = limit; return nil, nil }, noMark, nil, 37)
	if err != nil || asked != 37 {
		t.Errorf("limit asked=%d err=%v, want 37", asked, err)
	}
}

func TestPurgeExpiredRemovesEachSharedObjectOnceBeforeMarkingEveryRowOnIt(t *testing.T) {
	store := &purgeLedger{}
	n, err := PurgeExpired(context.Background(), unreachablePool(t), store,
		listing(candidate(1, "a"), candidate(2, "b"), candidate(3, "a")), store.mark, store.guard, 10)
	if err != nil || n != 3 {
		t.Fatalf("purged=%d err=%v, want 3", n, err)
	}
	want := []string{"guard a", "remove a", "mark 1", "mark 3", "guard b", "remove b", "mark 2"}
	if !slices.Equal(store.steps, want) {
		t.Errorf("steps = %v, want %v", store.steps, want)
	}
	for _, tx := range store.markTxs {
		if _, ok := tx.(guardTx); !ok {
			t.Errorf("a row was marked outside the guard's transaction: %T", tx)
		}
	}
}

func TestPurgeExpiredMarksARetainedObjectWithoutRemovingIt(t *testing.T) {
	store := &purgeLedger{retain: map[string]bool{"kept": true}}
	n, err := PurgeExpired(context.Background(), unreachablePool(t), store,
		listing(candidate(1, "kept"), candidate(2, "gone")), store.mark, store.guard, 10)
	want := []string{"guard kept", "mark 1", "guard gone", "remove gone", "mark 2"}
	if err != nil || n != 2 || !slices.Equal(store.steps, want) {
		t.Errorf("purged=%d err=%v steps=%v, want %v", n, err, store.steps, want)
	}
}

func TestPurgeExpiredLeavesRowsUnmarkedWhenTheirObjectCouldNotBeRemovedAndMovesOn(t *testing.T) {
	store := &purgeLedger{removeErr: map[string]error{"stuck": errors.New("storage unavailable")}}
	n, err := PurgeExpired(context.Background(), unreachablePool(t), store,
		listing(candidate(1, "stuck"), candidate(2, "stuck"), candidate(3, "free")), store.mark, store.guard, 10)
	if err != nil || n != 1 || !slices.Equal(store.marked, []byte{3}) {
		t.Errorf("purged=%d err=%v marked=%v, want only the row on the removed object", n, err, store.marked)
	}
}

func TestPurgeExpiredStopsAtAMarkItCouldNotWriteAndCountsWhatItDid(t *testing.T) {
	want := errors.New("mark failed")
	store := &purgeLedger{markErr: map[byte]error{2: want}}
	n, err := PurgeExpired(context.Background(), unreachablePool(t), store,
		listing(candidate(1, "a"), candidate(2, "b"), candidate(3, "c")), store.mark, store.guard, 10)
	if !errors.Is(err, want) || n != 1 || slices.Contains(store.steps, "guard c") {
		t.Errorf("purged=%d err=%v steps=%v, want the mark error after one row and no later object touched", n, err, store.steps)
	}
}

func TestPurgeExpiredReportsAGuardThatRefusedToRun(t *testing.T) {
	want := errors.New("lock unavailable")
	store := &purgeLedger{}
	n, err := PurgeExpired(context.Background(), unreachablePool(t), store,
		listing(candidate(1, "a"), candidate(2, "b")), store.mark,
		func(context.Context, string, func(bool, pgx.Tx) error) error { return want }, 10)
	if !errors.Is(err, want) || n != 0 || len(store.steps) != 0 {
		t.Errorf("purged=%d err=%v steps=%v, want the guard's error and nothing touched", n, err, store.steps)
	}
}
