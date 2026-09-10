package eval

import "testing"

func TestACitationIsFoundWhicheverEncodingItWasQuotedIn(t *testing.T) {
	payload := []byte(`{"tool_name":"Read",` +
		`"result_summary":"0\t# Q2 Update\n1\t\n2\tSo basically, we leveraged our synergies."}`)

	if _, _, ok := locate(traceSearchText(payload), `0\t# Q2 Update`); !ok {
		t.Error("a quote carrying the payload's own escape sequences must still be found")
	}

	decoded := "0\t# Q2 Update\n1"
	if _, _, ok := locate(traceSearchText(payload), decoded); !ok {
		t.Error("a quote of the decoded field must be found")
	}

	if _, _, ok := locate(traceSearchText(payload), "So basically, we reduced our synergies."); ok {
		t.Error("a quote that is in no field must stay unverifiable")
	}

	if _, _, ok := locate(traceSearchText(payload), `Read0\t# Q2`); ok {
		t.Error("a quote must never match across two payload fields")
	}
}

func TestAnUnparseablePayloadIsStillSearchable(t *testing.T) {
	raw := []byte(`not json at all, but stored anyway`)
	if got := traceSearchText(raw); got != string(raw) {
		t.Errorf("unparseable payload must fall back to its raw form, got %q", got)
	}
}
