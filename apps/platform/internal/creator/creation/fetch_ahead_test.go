package creation

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestAStepWhoseSessionCannotBeReadStopsInsteadOfPassingAsStale(t *testing.T) {
	pool, err := pgxpool.New(context.Background(), "postgres://nobody@127.0.0.1:1/nothing?sslmode=disable&connect_timeout=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	fetched := false
	s := &Service{Pool: pool, Fetch: func(context.Context, string) (Fetch, string) {
		fetched = true
		return Fetch{}, ""
	}}

	page, err := s.fetchAhead(context.Background(), JobArgs{SessionID: pgtype.UUID{Bytes: [16]byte{1}, Valid: true}})

	if err == nil || page != nil {
		t.Fatalf("page = %v, err = %v: a session that could not be read was taken as one with nothing to fetch, so its step would be dropped", page, err)
	}
	if fetched {
		t.Error("a page was fetched for a session that was never read")
	}
}
