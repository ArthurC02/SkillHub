package run

import (
	"context"
	"strings"
	"testing"
)

func TestThePermissionSummarySaysWhatTheTokenCeilingDependsOn(t *testing.T) {
	var note string
	for _, n := range permissionSummaryNotes {
		if strings.Contains(n, "輪") {
			note = n
			break
		}
	}
	if note == "" {
		t.Fatal("no pre-run note explains that the token ceiling's rounds depend on tool calls per round (02:276-284)")
	}

	for _, want := range []string{"工具呼叫", "15", "7.7", "5"} {
		if !strings.Contains(note, want) {
			t.Errorf("the rounds note = %q, want it to carry %q from the conversion table", note, want)
		}
	}
}

func TestTheTokenCeilingAbortMessageSaysTheRoundsDependOnToolCalls(t *testing.T) {
	d := driverWithCeiling(t, (&spendLogStub{calls: [][2]int{{300_001, 0}}}).start(t), 300_000, 60_000)
	reason := d.tokenCeilingBreach(context.Background(), anAttempt(t))
	if reason == "" {
		t.Fatal("a run past its input ceiling was allowed to continue")
	}
	if !strings.Contains(reason, "工具呼叫") {
		t.Errorf("abort message = %q, want it to say the ceiling's rounds depend on tool calls per round", reason)
	}

	if !containsAll(reason, "300001", "300000") {
		t.Errorf("abort message = %q, want it to keep naming what was used and what the limit was", reason)
	}
}

func TestTheOutputCeilingAbortMessageCarriesTheSameClause(t *testing.T) {
	d := driverWithCeiling(t, (&spendLogStub{calls: [][2]int{{1_000, 60_001}}}).start(t), 300_000, 60_000)
	reason := d.tokenCeilingBreach(context.Background(), anAttempt(t))
	if !strings.Contains(reason, "工具呼叫") {
		t.Errorf("output-ceiling abort message = %q, want the same rounds clause as the input one", reason)
	}
}
