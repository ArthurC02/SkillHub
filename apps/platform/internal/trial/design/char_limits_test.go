package testlab

import (
	"errors"
	"strings"
	"testing"
)

func TestTextLimitsCountCharactersNotBytes(t *testing.T) {
	han := func(n int) string { return strings.Repeat("長", n) }
	criteria := []Criterion{{ID: "c1", Text: "x"}}
	cases := []struct {
		name  string
		limit int
		check func(text string) error
	}{
		{"name", MaxNameChars, func(text string) error { _, _, err := validateDraft(text, "prompt"); return err }},
		{"prompt", MaxPromptChars, func(text string) error { _, _, err := validateDraft("name", text); return err }},
		{"criterion", MaxCriterionChars, func(text string) error { _, err := validateCriterion(text); return err }},
		{"rubric version", MaxRubricVersionChars, func(text string) error {
			_, err := validateRubric(Rubric{Version: text, Items: []RubricItem{{ID: "c1", Text: "x"}}}, criteria)
			return err
		}},
		{"rubric item", MaxCriterionChars, func(text string) error {
			_, err := validateRubric(Rubric{Version: "v1", Items: []RubricItem{{ID: "c1", Text: text}}}, criteria)
			return err
		}},
	}
	for _, c := range cases {
		if err := c.check(han(c.limit)); err != nil {
			t.Errorf("%s of %d characters (%d bytes) was refused: %v", c.name, c.limit, len(han(c.limit)), err)
		}
		if err := c.check(han(c.limit + 1)); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s of %d characters: %v, want it refused", c.name, c.limit+1, err)
		}
	}
}
