package mcptools

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
)

func calendar() Tool { return Tool{Server: "calendar", Tool: "create_event"} }

func TestAToolIsReferencedAsMcpServerTool(t *testing.T) {
	if got := calendar().Reference(); got != "mcp__calendar__create_event" {
		t.Errorf("Reference() = %q, want mcp__calendar__create_event", got)
	}
}

func TestOnlyDeclaredMCPNamesMissingFromTheListAreUnlisted(t *testing.T) {
	for _, tc := range []struct {
		name     string
		declared []string
		want     []string
	}{
		{"a listed tool", []string{"mcp__calendar__create_event"}, nil},
		{"an unlisted tool", []string{"mcp__mail__send"}, []string{"mcp__mail__send"}},
		{"a built-in tool is not an MCP name", []string{"Read", "Bash(git:*)"}, nil},
		{"a name that only looks alike", []string{"mcp__calendar__create_events"}, []string{"mcp__calendar__create_events"}},
		{"each unlisted name once per mention, in order", []string{"mcp__b__x", "Read", "mcp__calendar__create_event", "mcp__a__y"},
			[]string{"mcp__b__x", "mcp__a__y"}},
		{"nothing declared", nil, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Unlisted(tc.declared, []Tool{calendar()}); !slices.Equal(got, tc.want) {
				t.Errorf("Unlisted(%v) = %v, want %v", tc.declared, got, tc.want)
			}
		})
	}
}

func TestEveryMCPNameIsUnlistedWhenNothingIsOffered(t *testing.T) {
	if got := Unlisted([]string{"mcp__calendar__create_event"}, nil); !slices.Equal(got, []string{"mcp__calendar__create_event"}) {
		t.Errorf("Unlisted with no active tools = %v, want the declared MCP name", got)
	}
}

func draft() Draft {
	return Draft{
		Server: "calendar", Tool: "create_event", Purpose: "Creates a calendar event.",
		Parameters: "title, start, end", ServerURL: "https://mcp.example.com/calendar", Note: "requested by the sales team",
	}
}

func TestADraftWithinEveryLimitIsAcceptedTrimmed(t *testing.T) {
	d := draft()
	d.Server, d.Note = "  calendar ", " needed "
	got, err := validDraft(d)
	if err != nil {
		t.Fatalf("validDraft: %v", err)
	}
	if got.Server != "calendar" || got.Note != "needed" {
		t.Errorf("validDraft kept %q and %q, want both trimmed", got.Server, got.Note)
	}
}

func TestEachFieldIsAcceptedAtItsLimitAndRefusedOneOver(t *testing.T) {
	for _, tc := range []struct {
		field string
		max   int
		set   func(*Draft, string)
		fill  string
	}{
		{"server", 64, func(d *Draft, v string) { d.Server = v }, "a"},
		{"tool", 64, func(d *Draft, v string) { d.Tool = v }, "a"},
		{"purpose", 500, func(d *Draft, v string) { d.Purpose = v }, "字"},
		{"parameters", 1000, func(d *Draft, v string) { d.Parameters = v }, "字"},
		{"note", 500, func(d *Draft, v string) { d.Note = v }, "字"},
	} {
		t.Run(tc.field, func(t *testing.T) {
			at := draft()
			tc.set(&at, strings.Repeat(tc.fill, tc.max))
			if _, err := validDraft(at); err != nil {
				t.Errorf("%s of %d characters refused: %v", tc.field, tc.max, err)
			}
			over := draft()
			tc.set(&over, strings.Repeat(tc.fill, tc.max+1))
			if _, err := validDraft(over); !errors.Is(err, ErrShape) {
				t.Errorf("%s of %d characters: err = %v, want ErrShape", tc.field, tc.max+1, err)
			}
		})
	}
}

func TestTheServerAddressIsAcceptedAtItsLimitAndRefusedOneOver(t *testing.T) {
	prefix := "https://mcp.example.com/"
	at := draft()
	at.ServerURL = prefix + strings.Repeat("a", 500-len(prefix))
	if _, err := validDraft(at); err != nil {
		t.Errorf("a 500-character address refused: %v", err)
	}
	at.ServerURL += "a"
	if _, err := validDraft(at); !errors.Is(err, ErrShape) {
		t.Errorf("a 501-character address: err = %v, want ErrShape", err)
	}
}

func TestADraftOutsideItsFormIsRefused(t *testing.T) {
	for _, tc := range []struct {
		name string
		set  func(*Draft)
	}{
		{"blank server", func(d *Draft) { d.Server = " " }},
		{"uppercase server", func(d *Draft) { d.Server = "Calendar" }},
		{"server with a double hyphen", func(d *Draft) { d.Server = "my--calendar" }},
		{"server with an underscore", func(d *Draft) { d.Server = "my_calendar" }},
		{"server ending in a hyphen", func(d *Draft) { d.Server = "calendar-" }},
		{"blank tool", func(d *Draft) { d.Tool = "" }},
		{"tool with a double underscore", func(d *Draft) { d.Tool = "create__event" }},
		{"tool with a space", func(d *Draft) { d.Tool = "create event" }},
		{"blank purpose", func(d *Draft) { d.Purpose = "  " }},
		{"plain http address", func(d *Draft) { d.ServerURL = "http://mcp.example.com" }},
		{"address with no host", func(d *Draft) { d.ServerURL = "https:///calendar" }},
		{"blank address", func(d *Draft) { d.ServerURL = "" }},
		{"blank note", func(d *Draft) { d.Note = " " }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := draft()
			tc.set(&d)
			if _, err := validDraft(d); !errors.Is(err, ErrShape) {
				t.Errorf("err = %v, want ErrShape", err)
			}
		})
	}
}

func TestAToolWithoutParametersIsAccepted(t *testing.T) {
	d := draft()
	d.Parameters = ""
	if _, err := validDraft(d); err != nil {
		t.Errorf("a tool with no parameters refused: %v", err)
	}
}

func TestHyphenatedNamesAreAccepted(t *testing.T) {
	d := draft()
	d.Server, d.Tool = "team-calendar", "create-event"
	if _, err := validDraft(d); err != nil {
		t.Errorf("hyphenated server and tool refused: %v", err)
	}
}

func TestOnlyDeclaredToolsOnTheListAreOffered(t *testing.T) {
	listed := []Tool{calendar(), {Server: "mail", Tool: "send"}}
	for _, tc := range []struct {
		name     string
		declared []string
		want     []string
	}{
		{"a declared listed tool", []string{"Read", "mcp__calendar__create_event"}, []string{"mcp__calendar__create_event"}},
		{"a listed tool the version does not declare", []string{"mcp__mail__send"}, []string{"mcp__mail__send"}},
		{"a declared name that is not listed", []string{"mcp__crm__lookup"}, nil},
		{"only built-in tools", []string{"Read", "Bash"}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got []string
			for _, tool := range declaredAmong(tc.declared, slices.Clone(listed)) {
				got = append(got, tool.Reference())
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("declaredAmong(%v) = %v, want %v", tc.declared, got, tc.want)
			}
		})
	}
}

func TestTheListIsNotReadWhenTheVersionDeclaresNoMCPTool(t *testing.T) {
	got, err := Offered(context.Background(), nil, []string{"Read", "Bash(git:*)"})
	if err != nil || got != nil {
		t.Errorf("Offered = %v, %v; want nothing offered and no read of the list", got, err)
	}
}

func TestTheRuntimesOwnServerNameIsRefused(t *testing.T) {
	d := draft()
	d.Server = "skillhub"
	if _, err := validDraft(d); !errors.Is(err, ErrShape) {
		t.Errorf("server skillhub: err = %v, want ErrShape", err)
	}
	d.Server = "skillhub-crm"
	if _, err := validDraft(d); err != nil {
		t.Errorf("server skillhub-crm refused: %v", err)
	}
}
