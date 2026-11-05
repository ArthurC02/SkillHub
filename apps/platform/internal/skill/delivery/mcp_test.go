package packaging

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestDeclaredMCPToolsAreGroupedByTheServerThatProvidesThem(t *testing.T) {
	search := MCPTool{Reference: "mcp__docs__search", Server: "docs", URL: "https://docs.example.com/mcp"}
	fetch := MCPTool{Reference: "mcp__docs__fetch", Server: "docs", URL: "https://docs.example.com/mcp"}
	cases := []struct {
		name        string
		references  []string
		offered     []MCPTool
		wantServers []mcpServer
		wantGaps    []string
	}{
		{"nothing declared", nil, nil, nil, nil},
		{"one listed tool", []string{"mcp__docs__search"}, []MCPTool{search},
			[]mcpServer{{Name: "docs", URL: "https://docs.example.com/mcp", Tools: []string{"mcp__docs__search"}}}, nil},
		{"two tools on one server", []string{"mcp__docs__search", "mcp__docs__fetch"}, []MCPTool{search, fetch},
			[]mcpServer{{Name: "docs", URL: "https://docs.example.com/mcp", Tools: []string{"mcp__docs__fetch", "mcp__docs__search"}}}, nil},
		{"a tool not on the list", []string{"mcp__crm__lookup"}, nil, nil, []string{"mcp__crm__lookup"}},
		{"an offered tool the Skill did not declare", []string{"mcp__docs__search"}, []MCPTool{search, fetch},
			[]mcpServer{{Name: "docs", URL: "https://docs.example.com/mcp", Tools: []string{"mcp__docs__search"}}}, nil},
		{"one server under two addresses", []string{"mcp__docs__search", "mcp__docs__fetch"},
			[]MCPTool{search, {Reference: "mcp__docs__fetch", Server: "docs", URL: "https://mirror.example.com/mcp"}},
			nil, []string{"docs"}},
		{"an address with credentials", []string{"mcp__docs__search"},
			[]MCPTool{{Reference: "mcp__docs__search", Server: "docs", URL: "https://user:pw@docs.example.com/mcp"}},
			nil, []string{"docs"}},
		{"an address with a fragment", []string{"mcp__docs__search"},
			[]MCPTool{{Reference: "mcp__docs__search", Server: "docs", URL: "https://docs.example.com/mcp#x"}},
			nil, []string{"docs"}},
		{"an address with an empty fragment", []string{"mcp__docs__search"},
			[]MCPTool{{Reference: "mcp__docs__search", Server: "docs", URL: "https://docs.example.com/mcp#"}},
			nil, []string{"docs"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := groupMCPTools(tc.references, tc.offered)
			if !reflect.DeepEqual(got.Servers, tc.wantServers) {
				t.Errorf("servers = %+v, want %+v", got.Servers, tc.wantServers)
			}
			var gaps []string
			for _, g := range got.Gaps {
				gaps = append(gaps, g.Name)
			}
			if !reflect.DeepEqual(gaps, tc.wantGaps) {
				t.Errorf("gaps = %v, want %v", gaps, tc.wantGaps)
			}
		})
	}
}

func TestOnlyMCPReferencesCountAndEachOnce(t *testing.T) {
	docs := MCPTool{Reference: "mcp__docs__search", Server: "docs", URL: "https://docs.example.com/mcp"}
	got := mcpNeedsOf([]string{"Read", "mcp__docs__search", "Bash(git:*)", "mcp__docs__search", "mcp__crm__lookup", "mcp__crm__lookup"},
		[]MCPTool{docs, docs})
	want := mcpNeeds{
		Servers: []mcpServer{{Name: "docs", URL: "https://docs.example.com/mcp", Tools: []string{"mcp__docs__search"}}},
		Gaps: []mcpGap{{Name: "mcp__crm__lookup",
			Reason: "not on Skill Hub's MCP tool list, so Skill Hub does not know which server provides it"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("needs = %+v, want %+v", got, want)
	}
	if !mcpNeedsOf([]string{"Read", "Bash(git:*)"}, nil).empty() {
		t.Fatal("a Skill that declares no MCP tool needs no server")
	}
}

const mcpSkillMD = "---\nname: tidy-csv\ndescription: Tidies CSV files.\nlicense: MIT\nallowed-tools: Read mcp__docs__search\n---\n\nTidy it.\n"

var offeringDocs = []MCPTool{{Reference: "mcp__docs__search", Server: "docs", URL: "https://docs.example.com/mcp"}}

func TestAPluginDeclaresTheMCPServersItsMembersCall(t *testing.T) {
	p, err := planTestPlugin(pluginFixture{skillMD: mcpSkillMD, offered: offeringDocs})
	if err != nil || !p.Allowed {
		t.Fatalf("plan=%+v err=%v", p, err)
	}
	entries := zipEntries(t, p.Zip)

	var standard map[string]any
	if err := json.Unmarshal(entries["mcp.json"], &standard); err != nil {
		t.Fatalf("mcp.json = %q: %v", entries["mcp.json"], err)
	}
	wantStandard := map[string]any{
		"$schema": "https://agent-plugins.org/schemas/1.0.0/mcp.schema.json",
		"mcpServers": map[string]any{
			"docs": map[string]any{"type": "streamable-http", "url": "https://docs.example.com/mcp"},
		},
	}
	if !reflect.DeepEqual(standard, wantStandard) {
		t.Errorf("mcp.json = %v, want %v", standard, wantStandard)
	}

	var claude map[string]any
	if err := json.Unmarshal(entries[".mcp.json"], &claude); err != nil {
		t.Fatalf(".mcp.json = %q: %v", entries[".mcp.json"], err)
	}
	wantClaude := map[string]any{
		"mcpServers": map[string]any{"docs": map[string]any{"type": "http", "url": "https://docs.example.com/mcp"}},
	}
	if !reflect.DeepEqual(claude, wantClaude) {
		t.Errorf(".mcp.json = %v, want %v", claude, wantClaude)
	}

	if readme := string(entries["README.md"]); !strings.Contains(readme, "`docs` — https://docs.example.com/mcp (tools: `mcp__docs__search`)") {
		t.Errorf("README does not name the server:\n%s", readme)
	}
}

func TestAPluginWhoseMembersCallNoListedServerCarriesNoMCPConfig(t *testing.T) {
	p, err := planTestPlugin(pluginFixture{skillMD: mcpSkillMD})
	if err != nil || !p.Allowed {
		t.Fatalf("plan=%+v err=%v", p, err)
	}
	entries := zipEntries(t, p.Zip)
	if _, ok := entries["mcp.json"]; ok {
		t.Error("mcp.json written with no server to declare")
	}
	if _, ok := entries[".mcp.json"]; ok {
		t.Error(".mcp.json written with no server to declare")
	}
	if readme := string(entries["README.md"]); !strings.Contains(readme, "`mcp__docs__search` — not on Skill Hub's MCP tool list") {
		t.Errorf("README does not name the tool left for the reader to set up:\n%s", readme)
	}
}

func TestAPluginIsNotBuiltWhenTheMCPToolListCannotBeRead(t *testing.T) {
	p, err := planTestPlugin(pluginFixture{skillMD: mcpSkillMD, readErr: errors.New("list down")})
	if err == nil || !strings.Contains(err.Error(), "list down") {
		t.Fatalf("plan=%+v err=%v, want the read failure, not a plugin without its servers", p, err)
	}
}

func TestTheInstallGuideNamesTheServersASkillNeeds(t *testing.T) {
	needs := groupMCPTools([]string{"mcp__crm__lookup", "mcp__docs__search"}, []MCPTool{
		{Reference: "mcp__docs__search", Server: "docs", URL: "https://docs.example.com/mcp"},
	})
	guide := renderInstall(Profile{DisplayName: "Any", Version: "1"}, "demo", nil, needs)
	section, at := installSection(guide, "MCP servers")
	if at < 0 {
		t.Fatalf("no MCP servers section:\n%s", guide)
	}
	for _, want := range []string{
		"connect your Agent to them yourself",
		"- `docs` — https://docs.example.com/mcp (tools: `mcp__docs__search`)",
		"- `mcp__crm__lookup` — not on Skill Hub's MCP tool list",
	} {
		if !strings.Contains(section, want) {
			t.Errorf("MCP section lacks %q:\n%s", want, section)
		}
	}

	if _, at := installSection(renderInstall(Profile{DisplayName: "Any", Version: "1"}, "demo", nil, mcpNeeds{}), "MCP servers"); at >= 0 {
		t.Error("a Skill that calls no MCP tool got an MCP servers section")
	}
}
