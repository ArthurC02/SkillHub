package apiserver

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestTrendViewOnlyShowsBalanceWhenTheTrendHasOne(t *testing.T) {
	from := time.Date(2026, time.January, 2, 0, 0, 0, 0, time.UTC)
	to := from.AddDate(0, 0, 6)
	withoutBalance, err := json.Marshal(newTrendView(from, to, []dailyCountView{}))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(withoutBalance), "balance_total") {
		t.Errorf("ordinary trend response exposed balance_total: %s", withoutBalance)
	}

	total := int64(42)
	withBalance := newTrendView(from, to, []dailyAmountView{})
	withBalance.BalanceTotal = &total
	encoded, err := json.Marshal(withBalance)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"balance_total":42`) {
		t.Errorf("credit trend response omitted balance_total: %s", encoded)
	}
}
