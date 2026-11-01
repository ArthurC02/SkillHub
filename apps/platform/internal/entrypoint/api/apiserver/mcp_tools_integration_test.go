package apiserver_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/audit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/shared/skillpkg"
	ingest "github.com/ArthurC02/skillhub/apps/platform/internal/skill/admission"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
)

func uniqueServer() string {
	return fmt.Sprintf("srv-%d", time.Now().UnixNano())
}

func mcpToolBody(server, tool, url, note string) string {
	return fmt.Sprintf(`{"server":%q,"tool":%q,"purpose":"Creates an event.","parameters":"title, start","server_url":%q,"note":%q}`,
		server, tool, url, note)
}

func addMCPTool(t *testing.T, operator *client, server, tool string) map[string]any {
	t.Helper()
	code, added := operatorCall(t, operator, http.MethodPost, "/admin/mcp-tools", mcpToolBody(server, tool, "https://mcp.example.com/"+server, "sales follow-ups"))
	if code != http.StatusCreated {
		t.Fatalf("adding %s/%s: %d %v", server, tool, code, added)
	}
	return added
}

func TestAnOperatorAddsAnMCPToolThatSkillsReferenceByName(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	operator := a.login(t, freshName("operator-mcp-add"))
	a.auth.Operators = map[string]bool{operator.userID: true}
	server := uniqueServer()

	added := addMCPTool(t, operator, server, "create_event")
	if added["reference"] != "mcp__"+server+"__create_event" || added["server_url"] != "https://mcp.example.com/"+server ||
		added["reason"] != "sales follow-ups" || added["parameters"] != "title, start" || added["retired_at"] != nil {
		t.Fatalf("added = %v, want the listed tool under its mcp__ reference", added)
	}
	if n := countRow(t, pool, `SELECT count(*) FROM audit_events WHERE action = $1 AND resource_id = $2::uuid
		AND metadata->>'reference' = $3 AND metadata->>'reason' = 'sales follow-ups' AND metadata->>'scope' = 'operator'`,
		audit.ActionMCPToolAdd, added["id"], added["reference"]); n != 1 {
		t.Errorf("adding wrote %d matching audit events, want 1", n)
	}

	code, conflict := operatorCall(t, operator, http.MethodPost, "/admin/mcp-tools", mcpToolBody(server, "create_event", "https://other.example.com", "again"))
	if code != http.StatusConflict {
		t.Errorf("a second unretired %s/create_event answered %d %v, want 409", server, code, conflict)
	}
}

func TestAnMCPToolOutsideItsFormIsRefusedAndNothingIsListed(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	operator := a.login(t, freshName("operator-mcp-refuse"))
	a.auth.Operators = map[string]bool{operator.userID: true}
	server := uniqueServer()

	for name, body := range map[string]string{
		"an http address":     mcpToolBody(server, "send", "http://mcp.example.com", "n"),
		"an uppercase server": mcpToolBody(strings.ToUpper(server), "send", "https://mcp.example.com", "n"),
		"a double underscore": mcpToolBody(server, "send__mail", "https://mcp.example.com", "n"),
		"a blank note":        mcpToolBody(server, "send", "https://mcp.example.com", " "),
		"a server over 64":    mcpToolBody(strings.Repeat("a", 65), "send", "https://mcp.example.com", "n"),
		"not JSON":            `server`,
		"no purpose":          fmt.Sprintf(`{"server":%q,"tool":"send","server_url":"https://mcp.example.com","note":"n"}`, server),
	} {
		if code, answer := operatorCall(t, operator, http.MethodPost, "/admin/mcp-tools", body); code != http.StatusBadRequest {
			t.Errorf("%s: got %d %v, want 400", name, code, answer)
		}
	}
	if n := countRow(t, pool, `SELECT count(*) FROM mcp_tools WHERE server = $1`, server); n != 0 {
		t.Errorf("refused drafts listed %d tools, want 0", n)
	}
}

func TestARetiredMCPToolStaysListedWithItsReasonAndFreesItsName(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	operator := a.login(t, freshName("operator-mcp-retire"))
	a.auth.Operators = map[string]bool{operator.userID: true}
	server := uniqueServer()
	added := addMCPTool(t, operator, server, "send")
	path := fmt.Sprintf("/admin/mcp-tools/%s/retire", added["id"])

	if code, _ := operatorCall(t, operator, http.MethodPost, path, `{"note":" "}`); code != http.StatusBadRequest {
		t.Errorf("a blank retirement note answered %d, want 400", code)
	}
	code, retired := operatorCall(t, operator, http.MethodPost, path, `{"note":"the pilot ended"}`)
	if code != http.StatusOK || retired["retired_at"] == nil || retired["retire_reason"] != "the pilot ended" {
		t.Fatalf("retiring: %d %v, want it retired with its reason", code, retired)
	}
	if n := countRow(t, pool, `SELECT count(*) FROM audit_events WHERE action = $1 AND resource_id = $2::uuid AND metadata->>'reason' = 'the pilot ended'`,
		audit.ActionMCPToolRetire, added["id"]); n != 1 {
		t.Errorf("retiring wrote %d matching audit events, want 1", n)
	}
	for name, p := range map[string]string{
		"retired twice":  path,
		"an unknown id":  "/admin/mcp-tools/00000000-0000-4000-8000-000000000000/retire",
		"a malformed id": "/admin/mcp-tools/not-an-id/retire",
	} {
		if code, answer := operatorCall(t, operator, http.MethodPost, p, `{"note":"n"}`); code != http.StatusNotFound {
			t.Errorf("%s: got %d %v, want 404", name, code, answer)
		}
	}

	replacement := addMCPTool(t, operator, server, "send")
	expectUnretiredListedFirst(t, operator, replacement["id"], added["id"], "the pilot ended")
}

func expectUnretiredListedFirst(t *testing.T, operator *client, activeID, retiredID any, retireReason string) {
	t.Helper()
	code, list := operatorCall(t, operator, http.MethodGet, "/admin/mcp-tools", "")
	if code != http.StatusOK {
		t.Fatalf("listing: %d %v", code, list)
	}
	tools, _ := list["tools"].([]any)
	activeAt, retiredAt := -1, -1
	for i, raw := range tools {
		tool, _ := raw.(map[string]any)
		switch tool["id"] {
		case activeID:
			activeAt = i
		case retiredID:
			retiredAt = i
			if tool["retire_reason"] != retireReason {
				t.Errorf("the retired tool lists as %v, want its retirement reason %q", tool, retireReason)
			}
		}
	}
	if activeAt < 0 || retiredAt < 0 || activeAt > retiredAt {
		t.Errorf("the unretired tool is at %d and the retired one at %d, want both listed with the unretired one first", activeAt, retiredAt)
	}
}

func admissionWithTools(pool *pgxpool.Pool) *ingest.Service {
	return &ingest.Service{Pool: pool}
}

func draftDeclaring(tools string) ingest.GeneratedSkill {
	return ingest.GeneratedSkill{
		Name: "follow-up", Description: "Books a follow-up meeting after a sales call.", AllowedTools: tools,
		Body: "# Task\nRead the call notes and book the follow-up meeting the customer agreed to.",
	}
}

func TestACreatedDraftMayDeclareOnlyListedMCPTools(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	operator := a.login(t, freshName("operator-mcp-draft"))
	a.auth.Operators = map[string]bool{operator.userID: true}
	server := uniqueServer()
	listed := addMCPTool(t, operator, server, "create_event")["reference"].(string)
	svc := admissionWithTools(pool)

	check, err := svc.ValidateCreationDraft(context.Background(), draftDeclaring("Read "+listed))
	if err != nil || check.Blocked {
		t.Fatalf("a draft declaring the listed %s: blocked=%v err=%v report=%s", listed, check.Blocked, err, check.Report)
	}

	unlisted := "mcp__" + server + "__delete_event"
	check, err = svc.ValidateCreationDraft(context.Background(), draftDeclaring(listed+" "+unlisted))
	if err != nil || !check.Blocked {
		t.Fatalf("a draft declaring the unlisted %s: blocked=%v err=%v, want blocked", unlisted, check.Blocked, err)
	}
	if !strings.Contains(check.Report, skillpkg.CodeMCPToolNotOffered) || !strings.Contains(check.Report, unlisted) ||
		!strings.Contains(check.Report, listed) {
		t.Errorf("report = %s, want it to name %s as not offered and list %s as available", check.Report, unlisted, listed)
	}
}

func declaringVersion(t *testing.T, a *api, pool *pgxpool.Pool, f fixture, tools ...string) fixture {
	t.Helper()
	manifest, err := json.Marshal(map[string]any{"name": "clean-skill", "allowed_tools": tools})
	if err != nil {
		t.Fatal(err)
	}
	hash := fmt.Sprintf("hash-declaring-%d", time.Now().UnixNano())
	v, err := gen.New(pool).CreateSkillVersion(context.Background(), gen.CreateSkillVersionParams{
		WorkspaceID: mustUUID(t, f.workspaceID), SkillID: mustUUID(t, f.skillID), VersionNumber: nextVersionNumber(t, pool, f.skillID),
		ContentHash: hash, PackageObjectKey: "packages/" + hash + ".zip", Manifest: manifest,
	})
	if err != nil {
		t.Fatalf("a version declaring %v: %v", tools, err)
	}
	a.packages[v.PackageObjectKey] = cleanPackage(t)
	f.versionID = uuidText(v.ID)
	return f
}

func TestATrialRunOffersStandInsForTheListedToolsItsVersionDeclares(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	fake, svc := haltHarness(t, a, pool)
	operator := a.login(t, freshName("operator-mcp-stand-in"))
	a.auth.Operators = map[string]bool{operator.userID: true}
	server := uniqueServer()
	listed := addMCPTool(t, operator, server, "create_event")["reference"].(string)
	retired := addMCPTool(t, operator, server, "delete_event")
	if code, body := operatorCall(t, operator, http.MethodPost, fmt.Sprintf("/admin/mcp-tools/%s/retire", retired["id"]), `{"note":"withdrawn"}`); code != http.StatusOK {
		t.Fatalf("retiring: %d %v", code, body)
	}
	addMCPTool(t, operator, server, "list_events")
	f := declaringVersion(t, a, pool, newFixture(t, a, pool, "alice-mcp-stand-in"),
		"Read", listed, retired["reference"].(string), "mcp__"+server+"__send_mail")

	started := f.start(t)
	if err := driveThroughPolls(context.Background(), svc.Drive, mustUUID(t, f.workspaceID), mustUUID(t, started.RunID)); err != nil {
		t.Fatalf("driving the run: %v", err)
	}
	sent, ok := fake.Request(started.RunID, 1)
	want := []run.MCPToolStandIn{{Reference: listed, Purpose: "Creates an event.", Parameters: "title, start"}}
	if !ok || !reflect.DeepEqual(sent.MCPTools, want) {
		t.Errorf("the run was sent the stand-ins %+v (sent %v), want only the listed, unretired tool it declares: %+v", sent.MCPTools, ok, want)
	}
}

func TestATrialRunOfAVersionDeclaringNoMCPToolCarriesNoStandIns(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	fake, svc := haltHarness(t, a, pool)
	f := declaringVersion(t, a, pool, newFixture(t, a, pool, "alice-mcp-no-stand-in"), "Read", "Bash")

	started := f.start(t)
	if err := driveThroughPolls(context.Background(), svc.Drive, mustUUID(t, f.workspaceID), mustUUID(t, started.RunID)); err != nil {
		t.Fatalf("driving the run: %v", err)
	}
	if sent, ok := fake.Request(started.RunID, 1); !ok || sent.MCPTools != nil {
		t.Errorf("the run was sent the stand-ins %+v (sent %v), want none", sent.MCPTools, ok)
	}
}

func TestEndToEndRunAnswersADeclaredListedMCPToolWithAStandIn(t *testing.T) {
	realGatewayEnv(t)
	server := uniqueServer()
	reference := "mcp__" + server + "__create_event"
	if _, err := gen.New(requireDB(t)).InsertMCPTool(context.Background(), gen.InsertMCPToolParams{
		Server: server, Tool: "create_event", Purpose: "Creates an event in the team calendar.",
		Parameters: "title, start", ServerUrl: "https://mcp.example.com/" + server, Reason: "end to end stand-in check",
	}); err != nil {
		t.Fatalf("listing %s: %v", reference, err)
	}
	manifest, err := json.Marshal(map[string]any{"name": "book-follow-up", "allowed_tools": []string{reference}})
	if err != nil {
		t.Fatal(err)
	}
	o := finishRealGatewayRun(t, realGatewayRunSpec{
		user: "e2e-mcp-stand-in",
		pkg: zipOf(t, map[string]string{"SKILL.md": "---\nname: book-follow-up\n" +
			"description: Books a follow-up meeting in the team calendar.\nlicense: MIT\n" +
			"allowed-tools: " + reference + "\n---\n\n" +
			"Call the " + reference + " tool once, with the title \"Follow-up\" and the start \"2026-11-03T10:00\".\n" +
			"Then reply in one sentence saying whether the meeting was booked.\n"}),
		prompt:   "Use the book-follow-up skill to book the follow-up meeting.",
		manifest: manifest,
	})
	if o.final.Status != "succeeded" {
		t.Fatalf("run ended %s (%s / %s)", o.final.Status, o.final.FailureClass.Value, o.final.StatusReason)
	}
	var called map[string]any
	for _, e := range traceEvents(t, o.client, o.runID) {
		if e.Type == "tool_call" && e.Payload["tool_name"] == reference {
			called = e.Payload
		}
	}
	if called == nil {
		t.Fatalf("the trace records no call of %s: the agent was not offered the stand-in, or did not use it", reference)
	}
	if summary, _ := called["result_summary"].(string); !strings.Contains(summary, "this step needs the "+reference+" tool") {
		t.Errorf("calling %s answered %q, want the stand-in notice naming the tool the step needs", reference, summary)
	}
	t.Logf("the agent sent the stand-in %v", called["arguments"])
	cost, problems := gatewayUsageFindings(traceUsageEvent(t, o.client, o.runID))
	for _, problem := range problems {
		t.Error(problem)
	}
	t.Logf("gateway-reported cost for this run: $%.6f", cost)
}
