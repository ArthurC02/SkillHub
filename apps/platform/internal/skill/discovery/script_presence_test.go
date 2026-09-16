package catalog

import "testing"

func TestAScriptIsPresentOnlyWhenTheScanRecordedAScriptCode(t *testing.T) {
	yes, no := true, false
	for _, tc := range []struct {
		name string
		scan string
		want *bool
	}{
		{"a script file", `{"warnings":0,"codes":["script-file"]}`, &yes},
		{"a script embedded in SKILL.md", `{"codes":["possible-secret","embedded-script"]}`, &yes},
		{"other codes only", `{"codes":["possible-secret"]}`, &no},
		{"no codes", `{"codes":[]}`, &no},
		{"codes recorded as null", `{"codes":null}`, &no},
		{"a scan without codes", `{"warnings":1}`, nil},
		{"no scan", ``, nil},
		{"a null scan", `null`, nil},
	} {
		got := scriptPresence([]byte(tc.scan))
		if (got == nil) != (tc.want == nil) || got != nil && *got != *tc.want {
			t.Errorf("%s: scriptPresence = %v, want %v", tc.name, deref(got), deref(tc.want))
		}
	}
}

func deref(b *bool) any {
	if b == nil {
		return nil
	}
	return *b
}
