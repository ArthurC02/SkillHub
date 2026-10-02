package trace

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestMaskRedactsAStringValueWhoseKeyNamesASecret(t *testing.T) {
	for _, key := range []string{
		"password", "PASSWD", "client_secret", "token", "access-token", "OPENAI_API_KEY", "Api-Key", "apikey",
		"access_key", "private_key", "Authorization", "credential", "credentials", "Set-Cookie", "session",
	} {
		t.Run(key, func(t *testing.T) {
			in, _ := json.Marshal(map[string]any{"arguments": map[string]any{key: "opaque-value-123"}})
			result, err := (&Masker{}).Mask(in)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(result.Payload), "opaque-value-123") {
				t.Fatalf("value under %q survived: %s", key, result.Payload)
			}
			want := []string{"/arguments/" + escapePointer(key)}
			if !reflect.DeepEqual(result.Fields, want) {
				t.Fatalf("masked_fields = %v, want %v", result.Fields, want)
			}
		})
	}
}

func TestMaskKeepsValuesWhoseKeyOnlyResemblesASecretName(t *testing.T) {
	in := `{"token_count":"12","tokens":"many","max_tokens":"4096","token_source":"result","tokenizer":"bpe","secretary":"Ann","session_id":"s-1","password_hint_text":"x"}`
	result, err := (&Masker{}).Mask(json.RawMessage(in))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(result.Payload); !reflect.DeepEqual(canonicalJSON(t, got), canonicalJSON(t, in)) || len(result.Fields) != 0 {
		t.Fatalf("payload = %s, masked_fields = %v, want the input unchanged", got, result.Fields)
	}
}

func TestMaskLeavesValuesThatCarryNoSecretUnderASecretKey(t *testing.T) {
	in := `{"token":true,"secret":null,"cookie":"","session":"[REDACTED]"}`
	result, err := (&Masker{}).Mask(json.RawMessage(in))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(result.Payload); !reflect.DeepEqual(canonicalJSON(t, got), canonicalJSON(t, in)) || len(result.Fields) != 0 {
		t.Fatalf("payload = %s, masked_fields = %v, want the input unchanged", got, result.Fields)
	}
}

func canonicalJSON(t *testing.T, raw string) any {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestMaskConcealsEveryNumberAndStringNestedUnderASecretKey(t *testing.T) {
	in := `{"password":12345,"credentials":{"user":"ann","pin":4321,"enabled":true},"api_key":["k-1","k-2"],"note":"plain"}`
	result, err := (&Masker{}).Mask(json.RawMessage(in))
	if err != nil {
		t.Fatal(err)
	}
	want := `{"password":"[REDACTED]","credentials":{"user":"[REDACTED]","pin":"[REDACTED]","enabled":true},"api_key":["[REDACTED]","[REDACTED]"],"note":"plain"}`
	if got := string(result.Payload); !reflect.DeepEqual(canonicalJSON(t, got), canonicalJSON(t, want)) {
		t.Fatalf("payload = %s, want %s", got, want)
	}
	if len(result.Fields) != 5 {
		t.Errorf("masked_fields = %v, want the five concealed values", result.Fields)
	}
}
