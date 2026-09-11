package apiserver_test

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/creation"
)

func TestCreationStreamMeasureDeliveryLag(t *testing.T) {
	if os.Getenv("CREATION_STREAM_MEASURE") == "" {
		t.Skip("set CREATION_STREAM_MEASURE=1 to run the ADR-069 delivery-lag stopwatch")
	}
	const rounds = 24
	a, s, _ := creationFixture(t)
	c := a.login(t, "creation-stream-measure")

	gains := make([]float64, 0, rounds)
	for i := 0; i < rounds; i++ {
		v := creationPost(t, c, "/creation-sessions", map[string]any{"id": creationID(t), "message": "請建立資料摘要 Skill。", "budget_credits": 650}, 200)

		events, stop := readSSE(t, c, v.ID, fmt.Sprint(v.Revision))
		target := v.Revision + 1

		streamAt := make(chan time.Time, 1)
		go func() {
			for e := range events {
				if e.View.Revision >= target {
					streamAt <- time.Now()
					return
				}
			}
		}()

		pollAt := make(chan time.Time, 1)
		donePoll := make(chan struct{})
		phase := time.Duration(rand.Int63n(int64(time.Second)))
		go func() {
			tick := time.NewTicker(time.Second)
			defer tick.Stop()
			for {
				select {
				case <-donePoll:
					return
				case <-tick.C:
					res, err := c.Get(c.base + "/creation-sessions/" + v.ID)
					if err != nil {
						return
					}
					var got creation.View
					err = json.NewDecoder(res.Body).Decode(&got)
					res.Body.Close()
					if err == nil && got.Revision >= target {
						pollAt <- time.Now()
						return
					}
				}
			}
		}()

		time.Sleep(phase)
		creationStep(t, s, v)

		var sAt, pAt time.Time
		select {
		case sAt = <-streamAt:
		case <-time.After(20 * time.Second):
			t.Fatal("the stream never delivered the settled step")
		}
		select {
		case pAt = <-pollAt:
		case <-time.After(20 * time.Second):
			t.Fatal("the poller never saw the settled step")
		}
		close(donePoll)
		stop()
		gains = append(gains, pAt.Sub(sAt).Seconds())
	}

	sort.Float64s(gains)
	at := func(p float64) float64 { return gains[int(float64(len(gains)-1)*p)] }
	t.Logf("ADR-069 delivery lag removed, %d rounds: p50 %.3fs  p90 %.3fs  min %.3fs  max %.3fs",
		len(gains), at(0.5), at(0.9), gains[0], gains[len(gains)-1])

	if at(0.5) <= 0 {
		t.Errorf("the stream was not ahead of the poll at the median: %.3fs", at(0.5))
	}
}
