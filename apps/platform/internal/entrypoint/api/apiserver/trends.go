package apiserver

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/credit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/audit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/httpx"
	analytics "github.com/ArthurC02/skillhub/apps/platform/internal/product/learning"
	run "github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
)

const isoDate = "2006-01-02"

type trendsHandler struct {
	Credits            CreditLedger
	DailyRuns          func(ctx context.Context, since time.Time) ([]run.RunsOnDay, error)
	DailyRunWorkspaces func(ctx context.Context, since time.Time) ([]run.RunWorkspacesOnDay, error)
	DailyFunnelReach   func(ctx context.Context, since time.Time) ([]analytics.FunnelReach, error)
	Audit              audit.DBTX
	Now                func() time.Time
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

type trendView[T any] struct {
	From         string `json:"from"`
	To           string `json:"to"`
	Buckets      []T    `json:"buckets"`
	BalanceTotal *int64 `json:"balance_total,omitempty"`
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

func newTrendView[T any](from, to time.Time, buckets []T) trendView[T] {
	return trendView[T]{From: from.Format(isoDate), To: to.Format(isoDate), Buckets: buckets}
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
	httpx.WriteJSON(w, http.StatusOK, newTrendView(from, to, amountViews(days)))
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
	body := newTrendView(from, to, amountViews(days))
	body.BalanceTotal = &balanceTotal
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
	httpx.WriteJSON(w, http.StatusOK, newTrendView(from, to, views))
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
	httpx.WriteJSON(w, http.StatusOK, newTrendView(from, to, views))
}

const funnelRunStarted = "run_started"

type funnelStage struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Grain string `json:"grain"`
}

var funnelStages = []funnelStage{
	{Key: analytics.EventSearchPerformed, Label: "搜尋",
		Grain: "每個瀏覽工作階段一天算一次。同一個人換裝置或清掉 cookie 會算成兩個，所以這一段系統性偏高，只能讀量級。"},
	{Key: analytics.EventSkillDetailViewed, Label: "看 Skill 詳情",
		Grain: "粒度同搜尋，每個瀏覽工作階段一天算一次；不要求先搜尋過，從連結直接進來的也算。"},
	{Key: funnelRunStarted, Label: "開始試跑",
		Grain: "每個工作區一天算一次，來自執行紀錄而不是分析事件；和前兩段的工作階段不是同一種單位，不能相除成轉換率。"},
	{Key: analytics.EventDownloadStarted, Label: "按下下載",
		Grain: "每個工作區一天算一次；記的是按下按鈕，打包仍可能被拒，實際下載以下載紀錄為準。"},
}

type funnelTrendView struct {
	trendView[dailyCountView]
	Stages []funnelStage `json:"stages"`
}

func (h *trendsHandler) Funnel(w http.ResponseWriter, r *http.Request) {
	from, to, ok := h.window(w, r)
	if !ok {
		return
	}
	reach, err := h.DailyFunnelReach(r.Context(), from)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "funnel trend lookup failed")
		return
	}
	runs, err := h.DailyRunWorkspaces(r.Context(), from)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "funnel trend lookup failed")
		return
	}
	views := make([]dailyCountView, 0, len(reach)+len(runs))
	for _, d := range reach {
		views = append(views, dailyCountView{Day: d.Day.Format(isoDate), Key: d.Event, Count: d.Reached})
	}
	for _, d := range runs {
		views = append(views, dailyCountView{Day: d.Day.Format(isoDate), Key: funnelRunStarted, Count: d.Workspaces})
	}
	httpx.WriteJSON(w, http.StatusOK, funnelTrendView{trendView: newTrendView(from, to, views), Stages: funnelStages})
}
