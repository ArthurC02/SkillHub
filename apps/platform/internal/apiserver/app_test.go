package apiserver

import "testing"

// The one invariant BetaGateClosed exists for: an unaudited cohort must not
// produce an empty invite list, because an empty one admits everybody (ADR-028
// 決策 1). A "fail closed" that returns nothing fails open.
func TestBetaGateClosedIsNotEmpty(t *testing.T) {
	if len(BetaGateClosed()) == 0 {
		t.Fatal("BetaGateClosed() is empty; an empty invite list admits every signed-in user")
	}
}
