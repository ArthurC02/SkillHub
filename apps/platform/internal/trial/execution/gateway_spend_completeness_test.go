package run

import (
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

func TestGatewayReportsCostOnlyWhenEveryCallIsPriced(t *testing.T) {
	for _, tc := range []struct {
		name     string
		pages    []string
		want     float64
		reported bool
	}{
		{"all priced", []string{`[{"prompt_tokens":42,"completion_tokens":3,"spend":0.03},{"spend":0.005}]`}, .035, true},
		{"known then null", []string{`[{"prompt_tokens":42,"completion_tokens":3,"spend":0.03},{"spend":null}]`}, .03, false},
		{"missing then known", []string{`[{"prompt_tokens":42,"completion_tokens":3},{"spend":0.005}]`}, .005, false},
		{"null on earlier page", []string{`[{"prompt_tokens":42,"completion_tokens":3,"spend":null}]`, `[{"spend":0.005}]`}, .005, false},
		{"missing on later page", []string{`[{"prompt_tokens":42,"completion_tokens":3,"spend":0.03}]`, `[{}]`}, .03, false},
		{"negative only", []string{`[{"prompt_tokens":42,"completion_tokens":3,"spend":-0.005}]`}, 0, false},
		{"known then negative", []string{`[{"prompt_tokens":42,"completion_tokens":3,"spend":0.03},{"spend":-0.005}]`}, .03, false},
		{"negative then known", []string{`[{"prompt_tokens":42,"completion_tokens":3,"spend":-0.03},{"spend":0.005}]`}, .005, false},
		{"explicit zero", []string{`[{"prompt_tokens":42,"completion_tokens":3,"spend":0}]`}, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				page := 0
				if r.URL.Query().Get("page") == "2" {
					page = 1
				}
				if len(tc.pages) == 2 {
					_, _ = w.Write([]byte(`{"data":` + tc.pages[page] + `,"total_pages":2}`))
					return
				}
				_, _ = w.Write([]byte(`{"data":` + tc.pages[0] + `,"total_pages":1}`))
			}))
			defer server.Close()
			gateway := &Gateway{adminBaseURL: server.URL, client: server.Client()}
			got, err := gateway.Usage(t.Context(), "attempt", time.Time{})
			wantIncomplete := !tc.reported
			if err != nil || got.CostReported != tc.reported || got.CostIncomplete != wantIncomplete || math.Abs(got.ModelCostUSD-tc.want) > 1e-12 || got.InputTokens != 42 || got.OutputTokens != 3 || got.Incomplete {
				t.Fatalf("usage misrepresented returned call costs or tokens: got=%+v err=%v", got, err)
			}
		})
	}
}

func TestTruncatedSpendPagesCannotReportACompletePrice(t *testing.T) {
	for _, tc := range []struct {
		name     string
		calls    int
		complete bool
	}{
		{"all five pages", 1000, true},
		{"one more call than five pages", 1001, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stub := &spendLogStub{calls: make([][2]int, tc.calls), spends: make([]float64, tc.calls)}
			for i := range stub.calls {
				stub.calls[i], stub.spends[i] = [2]int{10, 1}, .001
			}
			got, err := stub.start(t).Usage(t.Context(), "attempt", time.Time{})
			if err != nil || stub.requests != 5 || got.InputTokens != 10000 || got.OutputTokens != 1000 || math.Abs(got.ModelCostUSD-1) > 1e-12 {
				t.Fatalf("bounded usage read changed known observations: got=%+v requests=%d err=%v", got, stub.requests, err)
			}
			wantIncomplete := !tc.complete
			if got.CostReported != tc.complete || got.CostIncomplete != wantIncomplete || got.Incomplete != wantIncomplete {
				t.Fatalf("truncated cost was reported as complete: got=%+v want complete=%v", got, tc.complete)
			}
		})
	}
}

func TestIncompleteAttemptSpendIsNotSettledEvenWhenTheBackendReportsAPrice(t *testing.T) {
	for _, tc := range []struct {
		name       string
		incomplete bool
	}{
		{"complete backend prices", false},
		{"priced but incomplete backend", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gateway := gatewaySpending(.1)
			gateway.perAttempt.Incomplete = tc.incomplete
			service := &Service{Gateway: gateway}
			attempts := []gen.RunAttempt{{ID: pgtype.UUID{Bytes: [16]byte{1}, Valid: true}}, {ID: pgtype.UUID{Bytes: [16]byte{2}, Valid: true}}}
			got := service.reportedSpend(t.Context(), gen.Run{}, attempts)
			if tc.incomplete && got != nil || !tc.incomplete && (got == nil || math.Abs(*got-.2) > 1e-12) {
				t.Fatalf("incomplete attempt produced a final cost: incomplete=%v got=%v", tc.incomplete, spendOrNone(got))
			}
		})
	}
}

func TestUnknownCallCostPreventsSettlementAndBudgetReuseAcrossAttempts(t *testing.T) {
	for _, tc := range []struct {
		name     string
		rows     string
		want     *float64
		wantLeft float64
	}{
		{"empty attempt", `[]`, usd(.03), .47},
		{"free attempt", `[{"spend":0}]`, usd(.03), .47},
		{"known attempt", `[{"spend":0.005}]`, usd(.035), .465},
		{"unpriced attempt", `[{}]`, nil, 0},
		{"partly priced attempt", `[{"spend":0.005},{"spend":null}]`, nil, 0},
		{"negative priced attempt", `[{"spend":-0.005}]`, nil, 0},
	} {
		for _, unpricedFirst := range []bool{true, false} {
			name := "last"
			if unpricedFirst {
				name = "first"
			}
			t.Run(tc.name+"/"+name, func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					rows := tc.rows
					if strings.HasSuffix(r.URL.Query().Get("key_alias"), "02000000-0000-0000-0000-000000000000") {
						rows = `[{"spend":0.03}]`
					}
					_, _ = w.Write([]byte(`{"data":` + rows + `,"total_pages":1}`))
				}))
				defer server.Close()
				service := &Service{Gateway: &Gateway{adminBaseURL: server.URL, client: server.Client()}}
				attempts := []gen.RunAttempt{{ID: pgtype.UUID{Bytes: [16]byte{1}, Valid: true}}, {ID: pgtype.UUID{Bytes: [16]byte{2}, Valid: true}}}
				if !unpricedFirst {
					attempts[0], attempts[1] = attempts[1], attempts[0]
				}
				assertRunSpendCompleteness(t, service, attempts, tc.want, tc.wantLeft)
			})
		}
	}
}

func assertRunSpendCompleteness(t *testing.T, service *Service, attempts []gen.RunAttempt, want *float64, wantLeft float64) {
	t.Helper()
	usage, err := service.usageOf(t.Context(), attempts)
	if err != nil || usage.CostIncomplete != (want == nil) || usage.CostReported != (want != nil) {
		t.Fatalf("run usage lost unknown cost across attempts: usage=%+v err=%v", usage, err)
	}
	got := service.reportedSpend(t.Context(), gen.Run{}, attempts)
	if (got == nil) != (want == nil) || got != nil && math.Abs(*got-*want) > 1e-12 {
		t.Fatalf("partial run spend was settled: got=%v want=%v", spendOrNone(got), spendOrNone(want))
	}
	left, err := service.budgetLeft(t.Context(), attempts, .5)
	if want == nil {
		if left != 0 || !errors.Is(err, errSpendUnreadable) {
			t.Fatalf("unknown spend was reused as a new budget: left=%v err=%v", left, err)
		}
	} else if err != nil || math.Abs(left-wantLeft) > 1e-12 {
		t.Fatalf("known or empty attempt blocked the remaining budget: left=%v err=%v", left, err)
	}
}
