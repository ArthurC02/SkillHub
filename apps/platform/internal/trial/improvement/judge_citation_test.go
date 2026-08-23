package eval

// Defence 3's matching, at the one place the B round proved it was wrong
// (04 丙-48, m3/report-judge-regression.md §14.4).

import "testing"

// The judge is shown the payload as raw JSON. A quote copied character for
// character therefore carries `\t` and `\n` as two characters each; a quote the
// model read and re-typed carries a real tab and a real newline. Only the first
// could ever be located, so five real rubric verdicts were thrown away for the
// encoding they happened to be written in - and the loss looked like the judge
// being conservative.
func TestACitationIsFoundWhicheverEncodingItWasQuotedIn(t *testing.T) {
	payload := []byte(`{"tool_name":"Read",` +
		`"result_summary":"0\t# Q2 Update\n1\t\n2\tSo basically, we leveraged our synergies."}`)

	// Copied off the screen, escapes intact. This always worked and must keep
	// working: it is the strongest form of the same claim.
	if _, _, ok := locate(traceSearchText(payload), `0\t# Q2 Update`); !ok {
		t.Error("a quote carrying the payload's own escape sequences must still be found")
	}

	// Read and re-typed. This is the one that failed, and it is the common one:
	// the escaping is the platform's, not the judge's.
	decoded := "0\t# Q2 Update\n1"
	if _, _, ok := locate(traceSearchText(payload), decoded); !ok {
		t.Error("a quote of the decoded field must be found")
	}

	// What must not become possible. The property is unchanged: the text has to be
	// in something this platform stored.
	if _, _, ok := locate(traceSearchText(payload), "So basically, we reduced our synergies."); ok {
		t.Error("a quote that is in no field must stay unverifiable")
	}

	// And no ordinary text may span two fields. The separator is NUL, which a JSON
	// string cannot contain (it encodes as \u0000) and which normalizeQuote does
	// not collapse - so bridging `tool_name` into `result_summary` would require a
	// quote containing a NUL, which is not something a judge can write.
	if _, _, ok := locate(traceSearchText(payload), `Read0\t# Q2`); ok {
		t.Error("a quote must never match across two payload fields")
	}
}

// A payload that is not JSON at all still gets checked against what was stored,
// rather than becoming unverifiable because the decoder gave up.
func TestAnUnparseablePayloadIsStillSearchable(t *testing.T) {
	raw := []byte(`not json at all, but stored anyway`)
	if got := traceSearchText(raw); got != string(raw) {
		t.Errorf("unparseable payload must fall back to its raw form, got %q", got)
	}
}
