package trace

import (
	"encoding/json"
	"testing"
)

func TestMaskKeepsIntegersExactAndSettlesFractionsAsFloats(t *testing.T) {
	in := `{"big":9007199254740993,"f":5.0,"e":1e3,"neg":-9007199254740993,"nested":[9223372036854775807,0]}`
	want := `{"big":9007199254740993,"e":1000,"f":5,"neg":-9007199254740993,"nested":[9223372036854775807,0]}`
	result, err := (&Masker{}).Mask(json.RawMessage(in))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(result.Payload); got != want {
		t.Fatalf("masked payload = %s, want %s", got, want)
	}
}

func TestMaskRefusesWhatIsNotExactlyOneJSONValueWithRepresentableNumbers(t *testing.T) {
	for _, tc := range []struct{ name, in string }{
		{"a second value after the first", `{"a":1} {"b":2}`},
		{"stray text after the value", `{"a":1}x`},
		{"a fraction beyond float range", `{"a":1e999}`},
		{"an empty payload", ``},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := (&Masker{}).Mask(json.RawMessage(tc.in)); err == nil {
				t.Fatalf("%q was accepted", tc.in)
			}
		})
	}
}
