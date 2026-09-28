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
		t.Skip("set CREATION_STREAM_MEASURE=1 to run the interactive-creation delivery-lag stopwatch")
	}
	const rounds = 24
	a, s, _ := creationFixture(t)
	c := a.login(t, "creation-stream-measure")

	gains := make([]float64, 0, rounds)
	for i := 0; i < rounds; i++ {
		gains = append(gains, measureDeliveryGain(t, c, s))
	}

	sort.Float64s(gains)
	at := func(p float64) float64 { return deliveryLagAt(gains, p) }
	t.Logf("interactive creation delivery lag removed, %d rounds: p50 %.3fs  p90 %.3fs  min %.3fs  max %.3fs",
		len(gains), at(0.5), at(0.9), gains[0], gains[len(gains)-1])

	if at(0.5) <= 0 {
		t.Errorf("the stream was not ahead of the poll at the median: %.3fs", at(0.5))
	}
}

func deliveryLagAt(sorted []float64, p float64) float64 {
	return sorted[int(float64(len(sorted)-1)*p)]
}

func measureDeliveryGain(t *testing.T, c *client, s *creation.Service) float64 {
	t.Helper()
	v := creationPost(t, c, "/creation-sessions", map[string]any{"id": creationID(t), "message": "請建立資料摘要 Skill。", "budget_credits": 650}, 200)

	events, stop := readSSE(t, c, v.ID, fmt.Sprint(v.Revision))
	target := v.Revision + 1

	streamAt := firstStreamedRevision(events, target)

	donePoll := make(chan struct{})
	phase := time.Duration(rand.Int63n(int64(time.Second)))
	pollAt := firstPolledRevision(c, v.ID, target, donePoll)

	time.Sleep(phase)
	creationStep(t, s, v)

	sAt := awaitDelivery(t, streamAt, "the stream never delivered the settled step")
	pAt := awaitDelivery(t, pollAt, "the poller never saw the settled step")
	close(donePoll)
	stop()
	return pAt.Sub(sAt).Seconds()
}

func firstStreamedRevision(events <-chan sseEvent, target int64) <-chan time.Time {
	streamAt := make(chan time.Time, 1)
	go func() {
		for e := range events {
			if e.View.Revision >= target {
				streamAt <- time.Now()
				return
			}
		}
	}()
	return streamAt
}

func firstPolledRevision(c *client, sessionID string, target int64, done <-chan struct{}) <-chan time.Time {
	pollAt := make(chan time.Time, 1)
	go func() {
		tick := time.NewTicker(time.Second)
		defer tick.Stop()
		for {
			select {
			case <-done:
				return
			case <-tick.C:
				res, err := c.Get(c.base + "/creation-sessions/" + sessionID)
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
	return pollAt
}

func awaitDelivery(t *testing.T, delivered <-chan time.Time, never string) time.Time {
	t.Helper()
	var at time.Time
	select {
	case at = <-delivered:
	case <-time.After(20 * time.Second):
		t.Fatal(never)
	}
	return at
}
