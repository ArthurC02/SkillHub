package apiserver

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/credit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/audit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/httpx"
	run "github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
)

const isoDate = "2006-01-02"

type trendsHandler struct {
	Credits   CreditLedger
	DailyRuns func(ctx context.Context, since time.Time) ([]run.RunsOnDay, error)
	Audit     audit.DBTX
	Now       func() time.Time
}

type dailyCountView struct {
	Day   string `json:"day"`
	Key   string `json:"key"`
	Count int64  `json:"count"`
}

type dailyAmountView struct {
	Day   string `json:"day"`
	Key   string `json:"key"`
	Count int64  `json:"count"`
	Total int64  `json:"total"`
}

func trendDays(r *http.Request) (int, error) {
	q := r.URL.Query()
	if !q.Has("days") {
		return 30, nil
	}
	switch q.Get("days") {
	case "7":
		return 7, nil
	case "30":
		return 30, nil
	case "90":
		return 90, nil
	}
	return 0, errors.New("query parameter days must be 7, 30 or 90")
}

func (h *trendsHandler) window(w http.ResponseWriter, r *http.Request) (from, to time.Time, ok bool) {
	days, err := trendDays(r)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return time.Time{}, time.Time{}, false
	}
	now := h.Now().UTC()
	to = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	return to.AddDate(0, 0, 1-days), to, true
}

func trendBody(from, to time.Time, buckets any) map[string]any {
	return map[string]any{"from": from.Format(isoDate), "to": to.Format(isoDate), "buckets": buckets}
}

func amountViews(days []credit.DailyAmount) []dailyAmountView {
	views := make([]dailyAmountView, 0, len(days))
	for _, d := range days {
		views = append(views, dailyAmountView{Day: d.Day.Format(isoDate), Key: d.Key, Count: d.Count, Total: d.Total})
	}
	return views
}

func (h *trendsHandler) Cost(w http.ResponseWriter, r *http.Request) {
	from, to, ok := h.window(w, r)
	if !ok {
		return
	}
	days, err := h.Credits.DailyCost(r.Context(), from)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "cost trend lookup failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, trendBody(from, to, amountViews(days)))
}

func (h *trendsHandler) CreditMovement(w http.ResponseWriter, r *http.Request) {
	from, to, ok := h.window(w, r)
	if !ok {
		return
	}
	days, balanceTotal, err := h.Credits.DailyCredits(r.Context(), from)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "credit trend lookup failed")
		return
	}
	body := trendBody(from, to, amountViews(days))
	body["balance_total"] = balanceTotal
	httpx.WriteJSON(w, http.StatusOK, body)
}

func (h *trendsHandler) Runs(w http.ResponseWriter, r *http.Request) {
	from, to, ok := h.window(w, r)
	if !ok {
		return
	}
	days, err := h.DailyRuns(r.Context(), from)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "run trend lookup failed")
		return
	}
	views := make([]dailyCountView, 0, len(days))
	for _, d := range days {
		views = append(views, dailyCountView{Day: d.Day.Format(isoDate), Key: d.Status, Count: d.Runs})
	}
	httpx.WriteJSON(w, http.StatusOK, trendBody(from, to, views))
}

func (h *trendsHandler) OperatorActions(w http.ResponseWriter, r *http.Request) {
	from, to, ok := h.window(w, r)
	if !ok {
		return
	}
	days, err := audit.DailyPlatform(r.Context(), h.Audit, operatorActions, from)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "operator action trend lookup failed")
		return
	}
	views := make([]dailyCountView, 0, len(days))
	for _, d := range days {
		views = append(views, dailyCountView{Day: d.Day.Format(isoDate), Key: d.Action, Count: d.Count})
	}
	httpx.WriteJSON(w, http.StatusOK, trendBody(from, to, views))
}
