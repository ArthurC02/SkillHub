package apiserver_test

import (
	"net/http"
	"reflect"
	"testing"
)

func continuationRoomOf(t *testing.T, f fixture, runID string) map[string]any {
	t.Helper()
	code, body := f.doJSON(t, http.MethodGet, "/runs/"+runID, "")
	if code != http.StatusOK {
		t.Fatalf("GET /runs/%s: %d %v", runID, code, body)
	}
	room, _ := body["continuation"].(map[string]any)
	return room
}

func TestARunTellsWhetherItsQuestionsCanStillBeAnsweredBeforeAnyAnswerIsWritten(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	resetRunSettings(t, pool)
	f := newFixture(t, a, pool, freshName("alice-continue-room"))

	lone := seedAskingRun(t, pool, f, "要訂幾間房？")
	asking := seedAskingRun(t, pool, f, "小孩算不算人數？")
	first := seedAnsweredRound(t, pool, f, asking, "大人幾位？")
	second := seedAnsweredRound(t, pool, f, first, "要不要含稅？")
	third := seedAnsweredRound(t, pool, f, second, "幣別？")

	for _, tc := range []struct {
		name  string
		runID string
		want  map[string]any
	}{
		{"an unanswered run with every round left", lone, map[string]any{"answered": false, "rounds_left": float64(3)}},
		{"an answered run at the start of a conversation", asking, map[string]any{"answered": true, "rounds_left": float64(3)}},
		{"an answered run one round in", first, map[string]any{"answered": true, "rounds_left": float64(2)}},
		{"an answered run with one round left", second, map[string]any{"answered": true, "rounds_left": float64(1)}},
		{"an unanswered run at the round limit", third, map[string]any{"answered": false, "rounds_left": float64(0)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := continuationRoomOf(t, f, tc.runID); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("continuation = %v, want %v", got, tc.want)
			}
		})
	}

	operator := a.login(t, freshName("operator-continue-room"))
	a.auth.Operators = map[string]bool{operator.userID: true}
	if code, body := operatorCall(t, operator, http.MethodPut, "/admin/settings/runs/continuation-rounds",
		`{"rounds":2,"note":"fewer rounds than this conversation already took"}`); code != http.StatusOK {
		t.Fatalf("lower rounds: %d %v", code, body)
	}
	want := map[string]any{"answered": false, "rounds_left": float64(0)}
	if got := continuationRoomOf(t, f, third); !reflect.DeepEqual(got, want) {
		t.Errorf("a conversation deeper than a lowered limit: continuation = %v, want %v", got, want)
	}

	if got := continuationRoomOf(t, f, f.start(t).RunID); got != nil {
		t.Errorf("a queued run carries continuation %v, want none", got)
	}
}
