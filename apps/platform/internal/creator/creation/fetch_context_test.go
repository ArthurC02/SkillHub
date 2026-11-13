package creation

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/creation/webfetch"
)

func TestOnlyTheMatchingSuccessfulPageReachesTheCurrentModelInput(t *testing.T) {
	record := Fetch{URL: "https://example.test/rules", Status: "ok", SHA256: "page-hash", Bytes: 50}
	svc := &Service{Mask: func(text string) string { return strings.ReplaceAll(text, "secret", "***") }}
	for _, tc := range []struct {
		name     string
		messages []Message
		page     *fetchedPage
		wantText bool
	}{
		{"current successful fetch", []Message{{Role: "tool", Content: webfetch.Observation(record, "")}}, &fetchedPage{record: record, text: "secret rules"}, true},
		{"no fetched page", []Message{{Role: "tool", Content: webfetch.Observation(record, "")}}, nil, false},
		{"no messages", nil, &fetchedPage{record: record, text: "secret rules"}, false},
		{"different metadata", []Message{{Role: "tool", Content: webfetch.Observation(Fetch{URL: record.URL, Status: "blocked"}, "")}}, &fetchedPage{record: record, text: "secret rules"}, false},
		{"author text is not a tool result", []Message{{Role: "user", Content: webfetch.Observation(record, "")}}, &fetchedPage{record: record, text: "secret rules"}, false},
		{"unsuccessful fetch", []Message{{Role: "tool", Content: webfetch.Observation(record, "")}}, &fetchedPage{record: Fetch{Status: "blocked"}, text: "secret rules"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			original, _ := json.Marshal(tc.messages)
			got := fetchedMessages(tc.messages, tc.page, svc.masked)
			encoded, _ := json.Marshal(got)
			after, _ := json.Marshal(tc.messages)
			if string(original) != string(after) || strings.Contains(string(encoded), "secret") || strings.Contains(string(encoded), "*** rules") != tc.wantText {
				t.Fatalf("transient page handling changed the stored messages or exposed the wrong page: %s => %s", original, encoded)
			}
		})
	}
}

func TestLegacyFetchTextIsRemovedWithoutChangingAuthoredOrOtherToolMessages(t *testing.T) {
	record := Fetch{URL: "https://example.test/rules", Status: "ok", SHA256: "hash", Bytes: 10}
	text := webfetch.Observation(record, "private page")
	messages := []Message{
		{Role: "user", Content: text}, {Role: "assistant", Content: text},
		{Role: "tool", Content: "not JSON"}, {Role: "tool", Content: `{"search":[]}`},
		{Role: "tool", Content: `{"fetch":null}`}, {Role: "tool", Content: text},
	}
	clean := withoutFetchedText(messages)
	if !reflect.DeepEqual(clean[:5], messages[:5]) || clean[5].Content != webfetch.Observation(record, "") || messages[5].Content != text {
		t.Fatalf("legacy sanitization altered authored data, metadata or its input: %+v", clean)
	}
	encoded, _ := json.Marshal(envelope{Snapshot: Snapshot{Messages: clean[5:]}, sourceMessages: messages[5:]})
	if strings.Contains(string(encoded), "private page") {
		t.Fatal("transient source evidence was serialized into the envelope")
	}
}

func TestTheCurrentPageCanProveASourceWithoutBeingStoredInTheDraftSession(t *testing.T) {
	p, r := sourcedDraft()
	record := Fetch{URL: "https://example.test/rules", Status: "ok", SHA256: "hash", Bytes: 100}
	p.Fetches = []Fetch{record}
	p.Messages = []Message{{Role: "tool", Content: webfetch.Observation(record, "")}}
	e := fundedSession(p)
	e.sourceMessages = []Message{{Role: "tool", Content: webfetch.Observation(record, *r.SourceAssessment.Rules[0].Quote)}}
	calls := 0
	state, _, err := countingValidator(&calls, false).proposal(t.Context(), 2, &e, r, nil)
	if err != nil || state != StateDraftReady || calls != 1 || e.SourceCheckedHash == "" || e.Snapshot.Messages[0].Content != p.Messages[0].Content {
		t.Fatalf("transient sources were lost or stored: state=%s calls=%d err=%v snapshot=%+v", state, calls, err, e.Snapshot)
	}
}

func TestLegacyFetchRedactionPreservesAllOtherMetadataAndPayloads(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input string
		want  string
	}{
		{"extra metadata", `{"extra":{"id":9007199254740993},"fetch":{"bytes":10,"note":"custom note","text":"private page","unknown":{"id":9007199254740993},"url":"https://example.test/path\\u0000"}}`, `{"extra":{"id":9007199254740993},"fetch":{"bytes":10,"note":"custom note","text":"","unknown":{"id":9007199254740993},"url":"https://example.test/path\\u0000"}}`},
		{"no body", `{"fetch":{"bytes":9007199254740993},"other":"value"}`, `{"fetch":{"bytes":9007199254740993},"other":"value"}`},
		{"other fetch type", `{"fetch":"another tool value","other":9007199254740993}`, `{"fetch":"another tool value","other":9007199254740993}`},
		{"missing URL", `{"fetch":{"text":"private page"}}`, `{"fetch":{"text":""}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			messages := []Message{{Role: "tool", Content: tc.input}}
			clean := withoutFetchedText(messages)
			var got, want any
			observed := json.NewDecoder(strings.NewReader(clean[0].Content))
			observed.UseNumber()
			if err := observed.Decode(&got); err != nil {
				t.Fatal(err)
			}
			expected := json.NewDecoder(strings.NewReader(tc.want))
			expected.UseNumber()
			if err := expected.Decode(&want); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) || messages[0].Content != tc.input {
				t.Fatalf("fetch redaction changed unrelated metadata: got=%s want=%s original=%s", clean[0].Content, tc.want, messages[0].Content)
			}
		})
	}
}
