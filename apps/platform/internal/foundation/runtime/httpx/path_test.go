package httpx

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPathUUIDAnswersAnUnreadableIDWithTheCallersNotFound(t *testing.T) {
	cases := []struct {
		name  string
		value string
		ok    bool
	}{
		{"a canonical uuid", "0f8fad5b-d9cb-469f-a165-70867728950e", true},
		{"an uppercase uuid", "0F8FAD5B-D9CB-469F-A165-70867728950E", true},
		{"empty", "", false},
		{"one character short", "0f8fad5b-d9cb-469f-a165-70867728950", false},
		{"not hex", "zf8fad5b-d9cb-469f-a165-70867728950e", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.SetPathValue("id", c.value)
			w := httptest.NewRecorder()

			id, ok := PathUUID(w, r, "id", "找不到這筆")

			if ok != c.ok || id.Valid != c.ok {
				t.Fatalf("ok = %v, id.Valid = %v, want both %v", ok, id.Valid, c.ok)
			}
			if c.ok {
				if w.Code != http.StatusOK || w.Body.Len() != 0 {
					t.Errorf("an accepted id wrote %d %q, want nothing", w.Code, w.Body.String())
				}
				return
			}
			var body struct{ Error string }
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if w.Code != http.StatusNotFound || body.Error != "找不到這筆" {
				t.Errorf("refusal = %d %q, want 404 找不到這筆", w.Code, body.Error)
			}
		})
	}
}
