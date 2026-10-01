package apiserver_test

import (
	"strings"
	"testing"
)

func TestPublishingRequestsStopReadingABodyPastSixtyFourKilobytes(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	owner := a.login(t, "body-cap-owner")
	operator := a.login(t, "body-cap-operator")
	a.auth.Operators = map[string]bool{operator.userID: true}
	padding := strings.Repeat(" ", 64<<10)

	for _, tc := range []struct {
		name, path, body string
		as               *client
	}{
		{"register a publisher", "/me/publisher", `{"name":"body-cap-publisher"}`, owner},
		{"create a bundle version", "/me/bundles", `{"name":"cap","version":"1.0.0","member_version_ids":[]}`, owner},
		{"publish a bundle", "/me/bundles/cap/publication", `{"name":"cap","version":"1.0.0","rights_attested":true}`, owner},
		{"publish a skill", "/skills/00000000-0000-4000-8000-0000000000aa/publication", `{"name":"cap","version_id":"00000000-0000-4000-8000-0000000000ab"}`, owner},
		{"review an exposure", "/admin/publications/someone/cap/exposure", `{"release_id":"00000000-0000-4000-8000-0000000000ac","decision":"approve"}`, operator},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, out := postJSON(t, tc.as, tc.path, padding+tc.body)
			if message, _ := out["error"].(string); code != 400 || !strings.HasPrefix(message, "the body must be") {
				t.Errorf("got %d %v, want 400 and the malformed-body answer: nothing past 64 KiB may be read", code, out)
			}
		})
	}
}
