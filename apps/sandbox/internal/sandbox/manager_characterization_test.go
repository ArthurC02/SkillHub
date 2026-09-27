package sandbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestAStartThatFailsAfterAP02BreachIsRefusedNotRecorded(t *testing.T) {
	drv := newP02Driver()
	drv.startEntered = make(chan struct{})
	drv.startBlock = make(chan struct{})
	drv.startCanceled = make(chan struct{})
	m := p02Manager(drv)
	probe := NewP02Probe([]string{"db.internal:5432"}, 0, 0)
	probe.Check(context.Background(), &fakeProber{}, probeAt)
	m.p02 = probe

	type createResult struct {
		created bool
		err     error
	}
	done := make(chan createResult, 1)
	go func() {
		_, created, err := m.Create(context.Background(), p02Request())
		done <- createResult{created: created, err: err}
	}()
	<-drv.startEntered
	probe.Check(context.Background(), &fakeProber{reached: []string{"db.internal:5432"}}, probeAt)
	if _, err := m.Cancel(context.Background(), m.List().Runs[0].ProviderRunID); err != nil {
		t.Fatal(err)
	}

	var result createResult
	select {
	case result = <-done:
	case <-time.After(time.Second):
		t.Fatal("Create did not return after its Start failed")
	}
	var re *RunError
	if result.created || !errors.As(result.err, &re) || !strings.Contains(re.Message, "P-02") {
		t.Fatalf("Create after a breach: created=%v err=%v, want the P-02 refusal", result.created, result.err)
	}
	if len(m.List().Runs) != 0 {
		t.Fatal("a start that failed during a breach remained in the manager")
	}
}

type windowTraceDriver struct {
	Driver
	trace []byte
}

func (d *windowTraceDriver) ReadTrace(_ context.Context, _ string, offset int64) ([]byte, bool, error) {
	if offset >= int64(len(d.trace)) {
		return nil, false, nil
	}
	return d.trace[offset:], false, nil
}

type failNthSink struct {
	pushes int
	failOn int
	events []string
}

func (s *failNthSink) Push(_ context.Context, _ string, events []json.RawMessage) error {
	s.pushes++
	if s.pushes == s.failOn {
		return errors.New("ingestion unavailable")
	}
	for _, e := range events {
		s.events = append(s.events, string(e))
	}
	return nil
}

func traceEvent(seq int) string { return fmt.Sprintf(`{"event_id":"e-%04d"}`, seq) }

func TestAFailedPushLaterInTheTraceDoesNotResendWhatWasAccepted(t *testing.T) {
	oversized := `{"padding":"` + strings.Repeat("x", traceBatchBytes) + `"}`
	for _, tc := range []struct {
		name           string
		first, later   []string
		acceptedPushes int
		want           []string
	}{
		{
			name:           "a full batch then a failed one",
			first:          seqEvents(0, 0),
			later:          seqEvents(1, traceBatch+1),
			acceptedPushes: 1,
			want:           seqEvents(0, traceBatch+1),
		},
		{
			name:  "an oversized event then a failed batch",
			first: []string{oversized, traceEvent(0)},
			later: []string{oversized, traceEvent(1)},
			want:  seqEvents(0, 1),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			drv := &windowTraceDriver{trace: []byte(strings.Join(tc.first, "\n") + "\n")}
			sink := &failNthSink{}
			m := NewManager(drv, Config{Provider: "test", Slots: 1}, slog.New(slog.DiscardHandler)).WithTrace(sink, nil)
			m.runs["run-1"] = &entry{}

			if !m.flushTrace(context.Background(), "run-1", "http://platform/trace") {
				t.Fatal("the first flush did not finish")
			}
			drv.trace = append(drv.trace, []byte(strings.Join(tc.later, "\n")+"\n")...)
			sink.failOn = sink.pushes + tc.acceptedPushes + 1
			if m.flushTrace(context.Background(), "run-1", "http://platform/trace") {
				t.Fatal("a flush with a failed push reported success")
			}
			if !m.flushTrace(context.Background(), "run-1", "http://platform/trace") {
				t.Fatal("the retried flush did not finish")
			}
			if !slices.Equal(sink.events, tc.want) {
				t.Fatalf("received %d events, want each of the %d exactly once in order", len(sink.events), len(tc.want))
			}
		})
	}
}

func seqEvents(from, to int) []string {
	out := make([]string, 0, to-from+1)
	for seq := from; seq <= to; seq++ {
		out = append(out, traceEvent(seq))
	}
	return out
}
