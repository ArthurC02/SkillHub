package main

import (
	"strings"
	"testing"
)

const limitsImportLine = "import limits \"" + contractLimitsImport + "\"\n\n"

func lengthUnitFixture(t *testing.T, files map[string]string) []string {
	t.Helper()
	root := t.TempDir()
	for name, source := range files {
		writeAt(t, root, platformInternalDir+"/ctx/a/"+name, "package a\n\n"+source)
	}
	return contractLengthUnitProblems(root)
}

func TestAByteCountAgainstACharacterLimitIsFound(t *testing.T) {
	cases := []struct {
		name  string
		files map[string]string
		want  string
	}{
		{"the generated constant itself", map[string]string{
			"a.go": limitsImportLine + "func ok(s string) bool { return len(s) > limits.NoteMaxLength }\n",
		}, "ctx/a/a.go:5 compares len(), a byte count, with the contract's NoteMaxLength"},
		{"the limit on the left", map[string]string{
			"a.go": limitsImportLine + "func ok(s string) bool { return limits.NoteMaxLength < len(s) }\n",
		}, "ctx/a/a.go:5 compares len()"},
		{"a package constant declared in another file", map[string]string{
			"limits.go": limitsImportLine + "const maxNote = limits.NoteMaxLength\n",
			"check.go":  "func ok(s string) bool { return len(s) >= maxNote }\n",
		}, "ctx/a/check.go:3 compares len(), a byte count, with the contract's NoteMaxLength"},
	}
	for _, c := range cases {
		p := lengthUnitFixture(t, c.files)
		if len(p) != 1 || !strings.Contains(p[0], c.want) {
			t.Errorf("%s: %v, want one problem containing %q", c.name, p, c.want)
		}
	}
}

func TestCountsThatMatchTheLimitsUnitAreIgnored(t *testing.T) {
	cases := []struct {
		name, source string
	}{
		{"characters against a character limit", "import (\n\t\"unicode/utf8\"\n\n\tlimits \"" + contractLimitsImport + "\"\n)\n\n" +
			"func ok(s string) bool { return utf8.RuneCountInString(s) > limits.NoteMaxLength }\n"},
		{"a slice length against an item limit", limitsImportLine + "func ok(s []string) bool { return len(s) > limits.NotesMaxItems }\n"},
		{"bytes against a byte limit", limitsImportLine + "func ok(s string) bool { return len(s) > limits.NoteMaxBytes }\n"},
		{"a local limit not from the contract", "const maxNote = 1000\n\nfunc ok(s string) bool { return len(s) > maxNote }\n"},
	}
	for _, c := range cases {
		if p := lengthUnitFixture(t, map[string]string{"a.go": c.source}); len(p) != 0 {
			t.Errorf("%s: %v, want none", c.name, p)
		}
	}
}
