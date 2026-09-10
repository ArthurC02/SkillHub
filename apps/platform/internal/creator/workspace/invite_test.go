package identity

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

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

func TestAnUnreadableInviteListIsAnErrorAndNotAnAdmission(t *testing.T) {
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
	user := User{ID: pgtype.UUID{Bytes: [16]byte{9}, Valid: true}, Email: "someone@example.com"}

	invited, err := h.invited(context.Background(), user)
	if err == nil {
		t.Fatal("a lookup that never answered returned no error, so both callers would read it as a decision")
	}
	if invited {
		t.Fatal("a lookup that never answered returned true")
	}
}
