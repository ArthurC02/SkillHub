package ingest

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/shared/skillpkg"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/admission/mcptools"
)

func reportDeclaring(tools ...string) skillpkg.Report {
	return skillpkg.Report{Manifest: &skillpkg.Manifest{Name: "notes", AllowedTools: tools}}
}

func TestAPackageWithoutMCPNamesIsNotCheckedAgainstTheList(t *testing.T) {
	for _, report := range []skillpkg.Report{
		reportDeclaring("Read", "Bash(git:*)"),
		reportDeclaring(),
		{},
		{Blocked: true, Manifest: &skillpkg.Manifest{AllowedTools: []string{"mcp__mail__send"}}},
	} {
		got := report
		if err := checkMCPTools(context.Background(), nil, &got, skillpkg.SeverityError); err != nil {
			t.Fatalf("checkMCPTools: %v", err)
		}
		if len(got.Findings) != len(report.Findings) || got.Blocked != report.Blocked {
			t.Errorf("report %+v became %+v, want it untouched", report, got)
		}
	}
}

func TestAnUnlistedMCPNameWarnsAnImportAndBlocksACreatedDraft(t *testing.T) {
	for _, tc := range []struct {
		severity    skillpkg.Severity
		wantBlocked bool
		wantText    string
	}{
		{skillpkg.SeverityWarning, false, "在這個平台上試跑時用不到這個工具"},
		{skillpkg.SeverityError, true, "目前清單是空的，請拿掉這個名稱"},
	} {
		t.Run(string(tc.severity), func(t *testing.T) {
			report := reportDeclaring("Read", "mcp__mail__send")
			markUnlisted(&report, nil, tc.severity)
			if len(report.Findings) != 1 || report.Blocked != tc.wantBlocked {
				t.Fatalf("findings %+v blocked %v, want one finding and blocked %v", report.Findings, report.Blocked, tc.wantBlocked)
			}
			f := report.Findings[0]
			if f.Severity != tc.severity || f.Code != "mcp-tool-not-offered" || f.Path != "SKILL.md" ||
				!strings.Contains(f.Message, "mcp__mail__send") || !strings.Contains(f.Message, tc.wantText) {
				t.Errorf("finding = %+v, want %s mcp-tool-not-offered naming mcp__mail__send with %q", f, tc.severity, tc.wantText)
			}
		})
	}
}

func TestAnUnreadableToolListFailsTheCheckInsteadOfPassingThePackage(t *testing.T) {
	pool, err := pgxpool.New(context.Background(), "postgres://nobody@127.0.0.1:1/none?connect_timeout=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	report := reportDeclaring("mcp__calendar__create_event")
	err = checkMCPTools(context.Background(), pool, &report, skillpkg.SeverityWarning)
	if err == nil || len(report.Findings) != 0 {
		t.Errorf("err = %v, findings %+v; want an error and no verdict on the name", err, report.Findings)
	}
}

func TestABlockedDraftIsToldWhichMCPToolsItMayDeclare(t *testing.T) {
	report := reportDeclaring("mcp__calendar__create_event", "mcp__mail__send")
	offered := []mcptools.Tool{{Server: "calendar", Tool: "create_event"}, {Server: "crm", Tool: "find"}}
	markUnlisted(&report, offered, skillpkg.SeverityError)
	if len(report.Findings) != 1 || !report.Blocked {
		t.Fatalf("findings %+v blocked %v, want only the unlisted name blocked", report.Findings, report.Blocked)
	}
	want := "allowed-tools 只能引用平台 MCP 工具清單上的名稱，mcp__mail__send 不在清單上；可用的有：mcp__calendar__create_event、mcp__crm__find。"
	if got := report.Findings[0].Message; got != want {
		t.Errorf("message = %q, want %q", got, want)
	}
}
