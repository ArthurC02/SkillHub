package creation

import (
	"errors"
	"strings"
	"testing"
)

// 一場失敗的會話在 2026-09-09 之前不帶任何可讀的原因：畫面一句「這一步未完成」，
// log 一個字都沒有。這支測試守的是那一句話**說得出是哪一邊壞了**。
func TestStepFailureMessageNamesTheSideThatBroke(t *testing.T) {
	for _, c := range []struct {
		name    string
		err     error
		callErr error
		want    string
	}{
		{"模型答了但不合規則", ErrInvalidCommand, nil, "模型的回覆不符合會話規則"},
		{"根本沒拿到答案（閘道、逾時、LLM 服務）", ErrUnavailable, errors.New("dial tcp [::1]:4000: refused"), "平台這一側沒能完成這次模型呼叫"},
		{"參考不可用：第二句話才是真正的原因", ErrUnavailable, ErrNotFound, "這一步未完成；"},
		{"餘額到底：第二句話才是真正的原因", ErrUnavailable, ErrCreditFloor, "這一步未完成；"},
		{"會話被停掉或超時：沒有第三方壞掉", ErrUnavailable, nil, "這一步未完成；"},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := stepFailureMessage(c.err, c.callErr)
			if !strings.Contains(got, c.want) {
				t.Fatalf("這一句沒有說出是哪一邊壞了\n want 含有: %s\n got:      %s", c.want, got)
			}
		})
	}
	// 反向：那兩句彼此不可以互相冒充，否則「說出是哪一邊」等於沒說。
	transport := stepFailureMessage(ErrUnavailable, errors.New("boom"))
	if strings.Contains(transport, "模型的回覆不符合會話規則") {
		t.Fatal("連不上模型時卻說模型的回覆不合規則")
	}
	if strings.Contains(stepFailureMessage(ErrInvalidCommand, nil), "平台這一側") {
		t.Fatal("模型答錯時卻說平台連不上")
	}
}
