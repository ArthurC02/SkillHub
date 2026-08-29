package run

import (
	"context"
	"strings"
	"testing"
)

// 02:276-284 (PDM-005 §5.2a-2) is not a suggestion: 「Token 上限必須連同輪數換算表
// 一起呈現,不得只寫「300K」…凡是對使用者呈現這個上限的地方(權限摘要、錯誤訊息、
// 文件),都必須讓讀者看得出這件事依賴每輪工具呼叫次數。」
//
// The reason is in the measurement it comes from: the harness costs ~19.4K input
// tokens per API call and every tool result resends the whole prefix, so one 300K
// ceiling is about 15 conversational rounds and about 5 tool-heavy ones. A bare
// "300000" is therefore not a smaller version of the truth, it is a different
// claim — that the ceiling is a fixed amount of work.
//
// Until this, nothing in apps/platform or apps/web contained the word 「輪」 at all.
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
	// The three-row table's numbers, because 「不得只寫 300K」 is not satisfied by a
	// vaguer sentence either: the point of the rule is that the reader can see the
	// spread.
	for _, want := range []string{"工具呼叫", "15", "7.7", "5"} {
		if !strings.Contains(note, want) {
			t.Errorf("the rounds note = %q, want it to carry %q from the conversion table", note, want)
		}
	}
}

// The second of the three places 02:276-284 names. The abort message used to be
// "used / limit" and nothing else, which reads as a fixed allowance the run
// overspent.
func TestTheTokenCeilingAbortMessageSaysTheRoundsDependOnToolCalls(t *testing.T) {
	d := driverWithCeiling(t, (&spendLogStub{calls: [][2]int{{300_001, 0}}}).start(t), 300_000, 60_000)
	reason := d.tokenCeilingBreach(context.Background(), anAttempt(t))
	if reason == "" {
		t.Fatal("a run past its input ceiling was allowed to continue")
	}
	if !strings.Contains(reason, "工具呼叫") {
		t.Errorf("abort message = %q, want it to say the ceiling's rounds depend on tool calls per round", reason)
	}
	// And it still names the two numbers, or the sentence has replaced the fact
	// rather than qualifying it.
	if !containsAll(reason, "300001", "300000") {
		t.Errorf("abort message = %q, want it to keep naming what was used and what the limit was", reason)
	}
}

// The output half carries the same clause: the same ceiling, the same reading.
func TestTheOutputCeilingAbortMessageCarriesTheSameClause(t *testing.T) {
	d := driverWithCeiling(t, (&spendLogStub{calls: [][2]int{{1_000, 60_001}}}).start(t), 300_000, 60_000)
	reason := d.tokenCeilingBreach(context.Background(), anAttempt(t))
	if !strings.Contains(reason, "工具呼叫") {
		t.Errorf("output-ceiling abort message = %q, want the same rounds clause as the input one", reason)
	}
}
