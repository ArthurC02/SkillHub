package agentloop

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type memoryJournal struct {
	spent    int64
	steps    []StepRecord
	endings  []Ending
	recorded int
}

func (j *memoryJournal) StartRun(context.Context, string) (pgtype.UUID, error) {
	return pgtype.UUID{Bytes: [16]byte{1}, Valid: true}, nil
}

func (j *memoryJournal) BeforeStep(context.Context, pgtype.UUID) error { return nil }

func (j *memoryJournal) SpentSince(context.Context, string, time.Time) (int64, error) {
	return j.spent, nil
}

func (j *memoryJournal) SpendCapMicros(context.Context, string) (int64, error) { return 1_000_000, nil }

func (j *memoryJournal) RecordKeyBudget(context.Context, pgtype.UUID, float64) error { return nil }

func (j *memoryJournal) RecordStep(_ context.Context, _ pgtype.UUID, _ int, step StepRecord, _ ModelCall) error {
	j.steps = append(j.steps, step)
	return nil
}

func (j *memoryJournal) Finish(ctx context.Context, run pgtype.UUID, ending Ending) error {
	j.endings = append(j.endings, ending)
	if ending.Record != nil {
		return ending.Record(ctx, nil, run, ending.Now)
	}
	return nil
}

const finalResult = `{"ok":true}`

func finishing(journal *memoryJournal) *Runner {
	return &Runner{
		Journal: journal,
		Step: func(context.Context, StepRequest) (StepDecision, error) {
			return StepDecision{Result: json.RawMessage(finalResult)}, nil
		},
		IssueKey:   func(context.Context, string, float64, time.Duration) (string, error) { return "key", nil },
		RevokeKey:  func(context.Context, string) error { return nil },
		RecordCost: func(context.Context, pgtype.UUID, int, ModelCall) error { return nil },
		Now:        time.Now,
	}
}

var oneStep = Limits{MaxSteps: 1, MaxTokens: 1_000, Deadline: time.Minute, StepTimeout: time.Second}

func TestAConclusionsReasonAndRecordReachTheJournalWithTheRun(t *testing.T) {
	journal := &memoryJournal{}
	agent := Agent{Name: "a",
		Conclude: func(_ context.Context, result json.RawMessage, _ []StepRecord) (Conclusion, error) {
			if string(result) != finalResult {
				t.Errorf("Conclude saw %s, want the final result", result)
			}
			return Conclusion{Reason: "one action had no preview", Record: func(context.Context, pgx.Tx, pgtype.UUID, time.Time) error {
				journal.recorded++
				return nil
			}}, nil
		},
	}
	report, err := finishing(journal).Run(context.Background(), agent, nil, oneStep)
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != Completed || report.Reason != "one action had no preview" {
		t.Fatalf("report = %+v, want completed with the conclusion's reason", report)
	}
	if journal.recorded != 1 || len(journal.endings) != 1 || journal.endings[0].Now.IsZero() {
		t.Fatalf("recorded %d times, endings %+v; want the record run once in a dated ending", journal.recorded, journal.endings)
	}
}

func TestAConclusionThatFailsFailsTheRunAndKeepsTheResult(t *testing.T) {
	journal := &memoryJournal{}
	agent := Agent{Name: "a",
		Conclude: func(context.Context, json.RawMessage, []StepRecord) (Conclusion, error) {
			return Conclusion{}, errors.New("proposed an action it may not propose")
		},
	}
	report, err := finishing(journal).Run(context.Background(), agent, nil, oneStep)
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != Failed || report.Reason != "proposed an action it may not propose" || string(report.Result) != finalResult {
		t.Fatalf("report = %+v, want failed with the reason and the result kept", report)
	}
	if journal.endings[0].Record != nil {
		t.Fatal("a failed conclusion still handed the journal something to record")
	}
}

func TestAnAgentWithoutAConclusionCompletesWithNothingToRecord(t *testing.T) {
	journal := &memoryJournal{}
	report, err := finishing(journal).Run(context.Background(), Agent{Name: "a"}, nil, oneStep)
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != Completed || report.Reason != "" || journal.endings[0].Record != nil {
		t.Fatalf("report = %+v, ending = %+v; want a plain completion", report, journal.endings[0])
	}
}

func TestASpentCapEndsTheRunBeforeAnyStep(t *testing.T) {
	journal := &memoryJournal{spent: 1_000_000}
	runner := finishing(journal)
	runner.Step = func(context.Context, StepRequest) (StepDecision, error) {
		t.Fatal("a step was asked after the cap was spent")
		return StepDecision{}, nil
	}
	report, err := runner.Run(context.Background(), Agent{Name: "a"}, nil, oneStep)
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != Incomplete || report.Reason != reasonSpendCap {
		t.Fatalf("report = %+v, want incomplete for the spent cap", report)
	}
}
