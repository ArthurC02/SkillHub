package identity

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ADR-028's beta gate is a cost ceiling on a publicly reachable deployment, so
// its unreadable case has to refuse. Nothing covered that: the integration tests
// reach the empty-list path and the not-on-the-list path, and the whole err
// branch could be deleted — or invited() could `return true, nil` outright —
// with every test still green (M4 audit, 2026-08-24).
//
// The unreachable pool is the entire fixture. pgxpool.New does not dial, so the
// lookup fails with a connection error rather than pgx.ErrNoRows, which is
// exactly the shape the branch is about: "we could not read who you are" is not
// "you are on the list".
func TestAnUnreadableInviteListRefusesRatherThanAdmitting(t *testing.T) {
	pool, err := pgxpool.New(context.Background(),
		"postgres://nobody@127.0.0.1:1/nothing?sslmode=disable&connect_timeout=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	h := &Handler{
		Service: &Service{Pool: pool},
		Invited: map[string]bool{"github-user-on-the-list": true},
	}

	admitted := false
	guarded := h.RequireInvited(func(http.ResponseWriter, *http.Request) { admitted = true })

	r := httptest.NewRequest(http.MethodGet, "/api/skills", nil)
	r = r.WithContext(context.WithValue(r.Context(), ctxKey{},
		User{ID: pgtype.UUID{Bytes: [16]byte{9}, Valid: true}, Email: "someone@example.com"}))
	w := httptest.NewRecorder()
	guarded(w, r)

	if admitted {
		t.Fatal("a user was admitted to the closed beta on a lookup that never answered")
	}
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status %d, want 503: an unanswerable check is not a 403 either — "+
			"403 tells the user they are not invited, which is a statement nobody made",
			w.Code)
	}
}
