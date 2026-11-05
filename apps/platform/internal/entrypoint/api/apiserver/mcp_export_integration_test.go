package apiserver_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func uploadedSkillDeclaring(t *testing.T, c *client, name, tools string) string {
	t.Helper()
	code, reply := postSource(t, c, zipOf(t, map[string]string{
		"SKILL.md": "---\nname: " + name + "\ndescription: Books follow-up meetings.\nlicense: MIT\nallowed-tools: " + tools +
			"\n---\n# " + name + "\n\nBook the meeting.\n",
	}))
	if code != http.StatusCreated {
		t.Fatalf("upload %s: got %d (%v)", name, code, reply)
	}
	id, _ := skillsOf(t, reply)[0]["skill_id"].(string)
	return id
}

func TestExportsCarryTheListedMCPServersTheirSkillsDeclare(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	operator := a.login(t, freshName("operator-mcp-export"))
	a.auth.Operators = map[string]bool{operator.userID: true}
	server := uniqueServer()
	listed := addMCPTool(t, operator, server, "create_event")["reference"].(string)
	unlisted := "mcp__" + server + "__send_mail"

	alice := a.login(t, freshName("mcp-export-alice"))
	name := freshName("follow-up")
	skillID := uploadedSkillDeclaring(t, alice, name, "Read "+listed+" "+unlisted)
	versionID := newestVersionID(t, alice, skillID)
	serverURL := "https://mcp.example.com/" + server

	bundle := freshName("mcp-bundle")
	if code, body := createBundle(t, alice, bundle, "1.0.0", versionID); code != http.StatusCreated {
		t.Fatalf("creating bundle: %d %v", code, body)
	}
	code, exported := postJSON(t, alice, "/me/bundles/"+bundle+"/export", `{}`)
	if code != http.StatusCreated {
		t.Fatalf("exporting: %d %v", code, exported)
	}
	plugin := zipEntries(t, a, exported["content_hash"].(string))
	var mcp struct {
		MCPServers map[string]map[string]string `json:"mcpServers"`
	}
	if err := json.Unmarshal(plugin["mcp.json"], &mcp); err != nil {
		t.Fatalf("mcp.json = %q: %v", plugin["mcp.json"], err)
	}
	if got := mcp.MCPServers; len(got) != 1 || got[server]["url"] != serverURL || got[server]["type"] != "streamable-http" {
		t.Errorf("mcp.json servers = %v, want only %s at %s", got, server, serverURL)
	}
	if readme := string(plugin["README.md"]); !strings.Contains(readme, "`"+unlisted+"` — not on Skill Hub's MCP tool list") {
		t.Errorf("the plugin README does not name the unlisted tool:\n%s", readme)
	}

	code, packaged := postJSON(t, alice, packagingPath(skillID, versionID), `{"target":"standard"}`)
	if code != http.StatusCreated {
		t.Fatalf("packaging: %d %v", code, packaged)
	}
	install := string(zipEntries(t, a, packaged["content_hash"].(string))["INSTALL.md"])
	if !strings.Contains(install, "- `"+server+"` — "+serverURL+" (tools: `"+listed+"`)") {
		t.Errorf("INSTALL.md does not name the server the Skill calls:\n%s", install)
	}
}
