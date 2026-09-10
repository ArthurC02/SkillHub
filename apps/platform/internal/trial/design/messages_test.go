package testlab

import (
	"archive/zip"
	"bytes"
	"fmt"
	"regexp"
	"testing"
)

func TestWireMessagesAreTraditionalChinese(t *testing.T) {
	han := regexp.MustCompile(`\p{Han}`)

	englishSentence := regexp.MustCompile(`(?i)\b(must|failed|not found|required|invalid|cannot|could not)\b`)

	check := func(t *testing.T, label, msg string) {
		t.Helper()
		if !han.MatchString(msg) {
			t.Errorf("%s: %q has no Han characters", label, msg)
		}
		if englishSentence.MatchString(msg) {
			t.Errorf("%s: %q still reads as an English sentence", label, msg)
		}
	}

	criteria := []Criterion{{ID: "c1", Text: "existing criterion"}}

	cases := []struct {
		label string
		msg   string
	}{
		{"validateDraft blank name", mustErr(t, func() (string, string, error) {
			return validateDraft("", "prompt")
		})},
		{"validateDraft oversize name", mustErr(t, func() (string, string, error) {
			return validateDraft(bigString(MaxNameBytes+1), "prompt")
		})},
		{"validateDraft blank prompt", mustErr(t, func() (string, string, error) {
			return validateDraft("name", "")
		})},
		{"validateDraft oversize prompt", mustErr(t, func() (string, string, error) {
			return validateDraft("name", bigString(MaxPromptBytes+1))
		})},
		{"validateCriterion blank text", mustErr2(t, func() (string, error) {
			return validateCriterion("")
		})},
		{"validateCriterion oversize text", mustErr2(t, func() (string, error) {
			return validateCriterion(bigString(MaxCriterionBytes + 1))
		})},
		{"validateRubric blank version", mustErrRubric(t, func() (Rubric, error) {
			return validateRubric(Rubric{Version: "", Items: []RubricItem{{ID: "c1", Text: "x"}}}, criteria)
		})},
		{"validateRubric oversize version", mustErrRubric(t, func() (Rubric, error) {
			return validateRubric(Rubric{Version: bigString(MaxRubricVersionBytes + 1), Items: []RubricItem{{ID: "c1", Text: "x"}}}, criteria)
		})},
		{"validateRubric empty items", mustErrRubric(t, func() (Rubric, error) {
			return validateRubric(Rubric{Version: "v1", Items: nil}, criteria)
		})},
		{"validateRubric unknown item id", mustErrRubric(t, func() (Rubric, error) {
			return validateRubric(Rubric{Version: "v1", Items: []RubricItem{{ID: "no-such-criterion", Text: "x"}}}, criteria)
		})},
		{"validateRubric duplicate item", mustErrRubric(t, func() (Rubric, error) {
			return validateRubric(Rubric{Version: "v1", Items: []RubricItem{
				{ID: "c1", Text: "x"}, {ID: "c1", Text: "y"},
			}}, criteria)
		})},
		{"validateRubric blank item text", mustErrRubric(t, func() (Rubric, error) {
			return validateRubric(Rubric{Version: "v1", Items: []RubricItem{{ID: "c1", Text: ""}}}, criteria)
		})},
		{"ErrNotFound", ErrNotFound.Error()},
		{"ErrInvalid", ErrInvalid.Error()},
		{"ErrLimitExceeded", ErrLimitExceeded.Error()},
		{"ErrUnsupportedType", ErrUnsupportedType.Error()},
		{"deleteTestCaseNote", deleteTestCaseNote},
		{"deleteDatasetNote", deleteDatasetNote},
		{"limitsNote", limitsNote},
	}
	for _, kind := range allowedKindsWire {
		cases = append(cases, struct {
			label string
			msg   string
		}{"allowedKindsWire", kind})
	}

	inspectZipErr := zipTooManyFilesErr(t)
	cases = append(cases, struct {
		label string
		msg   string
	}{"inspectZip too many files", stripSentinelPrefix(inspectZipErr, ErrLimitExceeded)})

	for _, c := range cases {
		c := c
		t.Run(c.label, func(t *testing.T) {
			check(t, c.label, c.msg)
		})
	}
}

func bigString(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = 'a'
	}
	return string(b)
}

func mustErr(t *testing.T, fn func() (string, string, error)) string {
	t.Helper()
	_, _, err := fn()
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	return stripSentinelPrefix(err, ErrInvalid)
}

func mustErr2(t *testing.T, fn func() (string, error)) string {
	t.Helper()
	_, err := fn()
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	return stripSentinelPrefix(err, ErrInvalid)
}

func mustErrRubric(t *testing.T, fn func() (Rubric, error)) string {
	t.Helper()
	_, err := fn()
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	return stripSentinelPrefix(err, ErrInvalid)
}

func zipTooManyFilesErr(t *testing.T) error {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for i := 0; i <= MaxFilesPerTestCase; i++ {
		w, err := zw.Create(fmt.Sprintf("file-%d.txt", i))
		if err != nil {
			t.Fatalf("create zip entry: %v", err)
		}
		if _, err := w.Write([]byte("x")); err != nil {
			t.Fatalf("write zip entry: %v", err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip writer: %v", err)
	}
	err := inspectZip(buf.Bytes())
	if err == nil {
		t.Fatal("expected inspectZip to reject an archive over the file-count limit")
	}
	return err
}
