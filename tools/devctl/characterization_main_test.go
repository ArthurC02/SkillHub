package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const characterizationMainBlock = "if __name__ == \"__main__\":\n    for name, fn in sorted(globals().items()):\n        pass\n"

func writeCharacterizationScript(t *testing.T, root, body string) {
	t.Helper()
	dir := filepath.Join(root, "tools", "sample")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "test_sample_characterization.py"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCharacterizationMainProblems(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		body string
		want string
	}{
		{"a test above the main block is collected", "def test_before():\n    pass\n\n" + characterizationMainBlock, ""},
		{"a test below the main block is reported with both lines",
			"def test_before():\n    pass\n\n" + characterizationMainBlock + "\n\ndef test_after_main():\n    pass\n",
			"tools/sample/test_sample_characterization.py:9: test_after_main is defined after the __main__ block on line 4"},
		{"a script without a main block has nothing to report", "def test_only():\n    pass\n", ""},
		{"a single-quoted main block is recognised",
			"if __name__ == '__main__':\n    pass\n\ndef test_late():\n    pass\n",
			"test_sample_characterization.py:4: test_late is defined after the __main__ block on line 1"},
		{"an indented helper named like a test below the block is not a collected test",
			characterizationMainBlock + "    def test_inner():\n        pass\n", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			writeCharacterizationScript(t, root, tc.body)
			got := characterizationMainProblems(root)
			switch {
			case tc.want == "" && len(got) != 0:
				t.Fatalf("expected no problems, got %q", got)
			case tc.want != "" && (len(got) != 1 || !strings.Contains(got[0], tc.want)):
				t.Fatalf("expected one problem containing %q, got %q", tc.want, got)
			}
		})
	}
}

func TestCharacterizationMainProblemsIgnoresOtherPythonFiles(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	dir := filepath.Join(root, "tools", "sample")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := characterizationMainBlock + "\ndef test_after_main():\n    pass\n"
	if err := os.WriteFile(filepath.Join(dir, "helper.py"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := characterizationMainProblems(root); len(got) != 0 {
		t.Fatalf("only test_*_characterization.py scripts are scanned, got %q", got)
	}
}
