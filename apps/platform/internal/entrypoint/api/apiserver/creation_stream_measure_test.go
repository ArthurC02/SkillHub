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

// The delivery-lag half of ADR-069's measurement (05 R-71 signature 3).
//
// # Why this one costs nothing
//
// The ruling authorised a paid run for 「how long does a step take」. That
// number turned out to be on disk already: 23 recorded paid runs under
// docs/plans/mvp/m5/creation-measure/ carry `seconds_per_call` for 1183 real
// model calls (p50 4.13s, p90 6.74s, p95 8.13s). Nothing needs re-buying to
// know how long a person waits.
//
// What was NOT on disk is the other term — how much of that wait is the
// platform noticing late — and that one needs no model at all: it is the
// distance between the state changing and a client seeing it, which the fake
// LLM produces exactly as well as a real one. So this harness measures the
// thing the money would not have bought.
//
// # What it measures
//
// Two observers on one session: the SSE stream, and a poller on the same 1s
// interval apps/web uses. Both watch for the same revision, and the reported
// number is how much sooner the stream saw it. Comparing the two observers to
// each other rather than to a start time is deliberate — it removes the step's
// own duration from the measurement, which is the term this change cannot
// affect.
//
// Skipped unless CREATION_STREAM_MEASURE is set: it is a stopwatch, not an
// assertion, and a latency assertion in CI is a flake waiting for a slow
// machine.
func TestCreationStreamMeasureDeliveryLag(t *testing.T) {
	if os.Getenv("CREATION_STREAM_MEASURE") == "" {
		t.Skip("set CREATION_STREAM_MEASURE=1 to run the ADR-069 delivery-lag stopwatch")
	}
	const rounds = 24
	a, s, _ := creationFixture(t)
	c := a.login(t, "creation-stream-measure")

	gains := make([]float64, 0, rounds)
	for i := 0; i < rounds; i++ {
		v := creationPost(t, c, "/creation-sessions", map[string]any{"id": creationID(t), "message": "請建立資料摘要 Skill。", "budget_usd": .5}, 200)

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

		// The poller apps/web falls back to: one GET a second. Its phase is
		// randomised inside the second, and that detail decides whether this
		// measurement means anything: a page's poll is not aligned to the moment
		// the worker finishes, so a fixed-phase harness would report the WORST
		// case (nearly a whole second) as if it were the average. With the phase
		// spread, the median lands where a real page's does.
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

		// The phase belongs on the EVENT, not on the poller. A page's poll has
		// been running for a while by the time the worker finishes, so the wait
		// that follows is spread uniformly across one interval. Delaying the
		// poller instead — which this harness did in its first version — measures
		// the poll's first tick from a standing start and reports the worst case
		// as the median.
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
	// Printed, not asserted — except for the one direction that would mean the
	// endpoint is pointless.
	if at(0.5) <= 0 {
		t.Errorf("the stream was not ahead of the poll at the median: %.3fs", at(0.5))
	}
}
