package skillpkg

import (
	"slices"
	"testing"
)

func TestWindowsCOMImportsAreReportedAsTheirOneDistribution(t *testing.T) {
	r := Validate(pkg(goodMD, map[string]string{
		"scripts/office.py": "import win32com.client\nimport pythoncom\nimport pywintypes\n",
	}))
	listed, ok := details(r, CodePackageDependencies)
	if !ok {
		t.Fatalf("want a %s finding, got %+v", CodePackageDependencies, r.Findings)
	}
	if !slices.Equal(listed, []string{"pywin32"}) {
		t.Fatalf("listed dependencies = %v, want exactly [pywin32]", listed)
	}
}
