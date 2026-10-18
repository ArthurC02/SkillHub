package operations

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAnOperatorNoteIsAtMostTheCapCountedInCharacters(t *testing.T) {
	cases := []struct {
		name   string
		runes  int
		wantOK bool
	}{
		{"exactly the cap", maxNoteChars, true},
		{"one character over", maxNoteChars + 1, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			_, ok := requireNote(w, strings.Repeat("字", c.runes))
			if ok != c.wantOK {
				t.Fatalf("a note of %d characters: accepted = %v, want %v", c.runes, ok, c.wantOK)
			}
			if !ok && w.Code != http.StatusBadRequest {
				t.Fatalf("a refused note answered %d, want 400", w.Code)
			}
		})
	}
}
