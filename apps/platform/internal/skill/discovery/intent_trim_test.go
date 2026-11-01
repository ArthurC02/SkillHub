package catalog

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestCorrectedSearchMeasuresTheQueryAfterTrimmingItLikeTheGetRoute(t *testing.T) {
	pool, err := pgxpool.New(context.Background(), "postgres://unused:unused@127.0.0.1:1/unused")
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	padded := "  " + strings.Repeat("x", maxQueryRunes) + "  "
	body := `{"query":"` + padded + `","intent":{"input":null,"output":null,"tools":null,"data":null,"environment":null},"keywords":[],"filters":{}}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/skills/search", strings.NewReader(body)).WithContext(ctx)
	(&Handler{Svc: &Service{Pool: pool}}).CorrectedSearch(w, req)

	if w.Code == http.StatusBadRequest {
		t.Fatalf("a %d-character query padded with spaces was refused as too long: %s", maxQueryRunes, w.Body.String())
	}
}
