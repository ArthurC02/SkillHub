package worker

import (
	"testing"
	"time"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
	run "github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
)

func TestPlatformAgentsAreScheduledOnlyWithBothAModelServiceAndAGateway(t *testing.T) {
	cases := []struct {
		name      string
		llm       *llmclient.Client
		gateway   *run.Gateway
		scheduled bool
	}{
		{"model service and gateway", &llmclient.Client{BaseURL: "https://llm.invalid"}, &run.Gateway{}, true},
		{"no gateway", &llmclient.Client{BaseURL: "https://llm.invalid"}, nil, false},
		{"no model service", nil, &run.Gateway{}, false},
	}
	kind := PlatformAgentRunArgs{}.Kind()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pool, deps := testDeps(t)
			deps.LLM, deps.Gateway = tc.llm, tc.gateway
			set, err := BuildWorkers(pool, deps)
			if err != nil {
				t.Fatalf("BuildWorkers: %v", err)
			}
			runOnStart, scheduled := set.Scheduled[kind]
			if scheduled != tc.scheduled || set.WorkerKinds[kind] != tc.scheduled {
				t.Fatalf("scheduled=%v worker=%v, want both %v", scheduled, set.WorkerKinds[kind], tc.scheduled)
			}
			if runOnStart {
				t.Error("a platform agent runs on start; every worker restart would spend on a report nobody asked for")
			}
		})
	}
}

func TestDailyAtIsTheNextOccurrenceOfTheHourInUTC(t *testing.T) {
	at := dailyAt{hour: 2}
	cases := []struct {
		name    string
		current time.Time
		want    time.Time
	}{
		{"before the hour", time.Date(2026, 10, 7, 1, 59, 59, 0, time.UTC), time.Date(2026, 10, 7, 2, 0, 0, 0, time.UTC)},
		{"on the hour", time.Date(2026, 10, 7, 2, 0, 0, 0, time.UTC), time.Date(2026, 10, 8, 2, 0, 0, 0, time.UTC)},
		{"after the hour", time.Date(2026, 10, 7, 2, 0, 1, 0, time.UTC), time.Date(2026, 10, 8, 2, 0, 0, 0, time.UTC)},
		{"another zone", time.Date(2026, 10, 7, 9, 0, 0, 0, time.FixedZone("UTC+8", 8*3600)), time.Date(2026, 10, 7, 2, 0, 0, 0, time.UTC)},
		{"month end", time.Date(2026, 10, 31, 3, 0, 0, 0, time.UTC), time.Date(2026, 11, 1, 2, 0, 0, 0, time.UTC)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := at.Next(tc.current); !got.Equal(tc.want) {
				t.Errorf("Next(%v) = %v, want %v", tc.current, got, tc.want)
			}
		})
	}
}
