package packaging

import (
	"encoding/json"
	"fmt"
	"net/url"
	"slices"
	"sort"
	"strings"

	"github.com/ArthurC02/skillhub/apps/platform/internal/shared/skillpkg"
)

const (
	pluginMCPFile       = "mcp.json"
	claudePluginMCPFile = ".mcp.json"
	mcpSchemaID         = "https://agent-plugins.org/schemas/1.0.0/mcp.schema.json"
	mcpReferencePrefix  = "mcp__"
)

type MCPTool struct {
	Reference string
	Server    string
	URL       string
}

type mcpServer struct {
	Name  string
	URL   string
	Tools []string
}

type mcpGap struct {
	Name   string
	Reason string
}

type mcpNeeds struct {
	Servers []mcpServer
	Gaps    []mcpGap
}

func (n mcpNeeds) empty() bool { return len(n.Servers) == 0 && len(n.Gaps) == 0 }

func mcpNeedsOf(declared []string, offered []MCPTool) mcpNeeds {
	references := slices.DeleteFunc(slices.Clone(declared), func(name string) bool {
		return !strings.HasPrefix(name, mcpReferencePrefix)
	})
	slices.Sort(references)
	return groupMCPTools(slices.Compact(references), offered)
}

func groupMCPTools(references []string, offered []MCPTool) mcpNeeds {
	var needs mcpNeeds
	byServer := map[string]*mcpServer{}
	conflicting := map[string]bool{}
	for _, t := range offered {
		if !slices.Contains(references, t.Reference) {
			continue
		}
		server, seen := byServer[t.Server]
		switch {
		case !seen:
			byServer[t.Server] = &mcpServer{Name: t.Server, URL: t.URL, Tools: []string{t.Reference}}
		case server.URL != t.URL:
			conflicting[t.Server] = true
		default:
			server.Tools = append(server.Tools, t.Reference)
		}
	}
	for _, ref := range references {
		if !slices.ContainsFunc(offered, func(t MCPTool) bool { return t.Reference == ref }) {
			needs.Gaps = append(needs.Gaps, mcpGap{Name: ref,
				Reason: "not on Skill Hub's MCP tool list, so Skill Hub does not know which server provides it"})
		}
	}
	for name, server := range byServer {
		switch {
		case conflicting[name]:
			needs.Gaps = append(needs.Gaps, mcpGap{Name: name,
				Reason: "Skill Hub's MCP tool list gives this server more than one address, so none is written"})
		case !declarableServerURL(server.URL):
			needs.Gaps = append(needs.Gaps, mcpGap{Name: name,
				Reason: "its address on Skill Hub's MCP tool list carries credentials or a fragment, so it is not written"})
		default:
			slices.Sort(server.Tools)
			server.Tools = slices.Compact(server.Tools)
			needs.Servers = append(needs.Servers, *server)
		}
	}
	sort.Slice(needs.Servers, func(i, j int) bool { return needs.Servers[i].Name < needs.Servers[j].Name })
	sort.SliceStable(needs.Gaps, func(i, j int) bool { return needs.Gaps[i].Name < needs.Gaps[j].Name })
	return needs
}

func declarableServerURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.Host != "" && u.User == nil && !strings.Contains(raw, "#")
}

func (n mcpNeeds) configFiles() ([]exportFile, error) {
	if len(n.Servers) == 0 {
		return nil, nil
	}
	standard := map[string]map[string]string{}
	claude := map[string]map[string]string{}
	for _, s := range n.Servers {
		standard[s.Name] = map[string]string{"type": "streamable-http", "url": s.URL}
		claude[s.Name] = map[string]string{"type": "http", "url": s.URL}
	}
	standardJSON, err := json.MarshalIndent(map[string]any{"$schema": mcpSchemaID, "mcpServers": standard}, "", "  ")
	if err != nil {
		return nil, err
	}
	claudeJSON, err := json.MarshalIndent(map[string]any{"mcpServers": claude}, "", "  ")
	if err != nil {
		return nil, err
	}
	return []exportFile{
		{path: pluginMCPFile, data: append(standardJSON, '\n')},
		{path: claudePluginMCPFile, data: append(claudeJSON, '\n')},
	}, nil
}

const (
	pluginMCPIntro = "These Skills call tools on the MCP servers below. `mcp.json` declares them for " +
		"Agent Plugins clients and `.mcp.json` for Claude Code, as Streamable HTTP servers; your client " +
		"connects to them and handles any sign-in they ask for.\n\n"
	skillMCPIntro = "This Skill calls tools on the MCP servers below. A Skill cannot carry server " +
		"settings, so connect your Agent to them yourself (Streamable HTTP); the Skill's " +
		"`allowed-tools` names each tool as `mcp__<server>__<tool>`.\n\n"
)

func declaredTools(r skillpkg.Report) []string {
	if r.Manifest == nil {
		return nil
	}
	return r.Manifest.AllowedTools
}

func writeMCPServers(b *strings.Builder, n mcpNeeds, intro string) {
	if n.empty() {
		return
	}
	b.WriteString("## MCP servers\n\n")
	if len(n.Servers) > 0 {
		b.WriteString(intro)
		for _, s := range n.Servers {
			fmt.Fprintf(b, "- `%s` — %s (tools: %s)\n", s.Name, s.URL, "`"+strings.Join(s.Tools, "`, `")+"`")
		}
		b.WriteString("\n")
	}
	if len(n.Gaps) > 0 {
		b.WriteString("Set these up yourself; Skill Hub writes no settings for them:\n\n")
		for _, g := range n.Gaps {
			fmt.Fprintf(b, "- `%s` — %s.\n", g.Name, g.Reason)
		}
		b.WriteString("\n")
	}
}
