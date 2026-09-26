package apiserver_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/entrypoint/api/apiserver"
)

func freshName(prefix string) string {
	return prefix + "-" + strings.ReplaceAll(uuid.NewString(), "-", "")[:10]
}

func registerPublisher(t *testing.T, c *client, name string) {
	t.Helper()
	if code, body := postJSON(t, c, "/me/publisher", `{"name":"`+name+`"}`); code != http.StatusCreated {
		t.Fatalf("POST /me/publisher %q: got %d (%v)", name, code, body)
	}
}

func uploadedSkill(t *testing.T, c *client, name, body string) string {
	t.Helper()
	code, reply := postSource(t, c, zipOf(t, map[string]string{
		"SKILL.md": "---\nname: " + name + "\ndescription: Turn one thing into another.\nlicense: MIT\n---\n# " + name + "\n\n" + body + "\n",
	}))
	if code != http.StatusCreated {
		t.Fatalf("upload %s: got %d (%v)", name, code, reply)
	}
	skills := skillsOf(t, reply)
	id, _ := skills[0]["skill_id"].(string)
	if id == "" {
		t.Fatalf("upload %s answered no skill id: %v", name, reply)
	}
	return id
}

func publish(t *testing.T, c *client, skillID, body string) (int, map[string]any) {
	t.Helper()
	return postJSON(t, c, "/skills/"+skillID+"/publication", body)
}

func publicRead(t *testing.T, a *api, address string) (int, map[string]any) {
	t.Helper()
	var body map[string]any
	code := getJSON(t, http.DefaultClient, a.URL+"/publications"+strings.TrimPrefix(address, "/p"), &body)
	return code, body
}

func setSkill(t *testing.T, pool *pgxpool.Pool, skillID, assignment string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), "UPDATE skills SET "+assignment+" WHERE id = $1", mustUUID(t, skillID)); err != nil {
		t.Fatal(err)
	}
}

func TestAnAccountRegistersOnePublisherAndANameIsTakenOnceSiteWide(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	alice := a.login(t, freshName("publisher-alice"))
	bob := a.login(t, freshName("publisher-bob"))
	name := freshName("alice")

	registerPublisher(t, alice, name)
	var own map[string]any
	if code := getJSON(t, alice.Client, alice.base+"/me/publisher", &own); code != http.StatusOK || own["name"] != name {
		t.Fatalf("GET /me/publisher: %d %v, want 200 and %q", code, own, name)
	}
	if code, body := postJSON(t, alice, "/me/publisher", `{"name":"`+freshName("second")+`"}`); code != http.StatusConflict || body["reason"] != "already_registered" {
		t.Errorf("a second publisher for one account: %d %v, want 409 already_registered", code, body)
	}
	if code, body := postJSON(t, bob, "/me/publisher", `{"name":"`+name+`"}`); code != http.StatusConflict || body["reason"] != "name_taken" {
		t.Errorf("another account taking the same name: %d %v, want 409 name_taken", code, body)
	}
	for _, tc := range []struct{ name, reason string }{
		{"skill-hub", "name_reserved"},
		{"Bad_Name", "name_shape"},
	} {
		if code, body := postJSON(t, bob, "/me/publisher", `{"name":"`+tc.name+`"}`); code != http.StatusUnprocessableEntity || body["reason"] != tc.reason {
			t.Errorf("registering %q: %d %v, want 422 %s", tc.name, code, body, tc.reason)
		}
	}
	if code := getJSON(t, bob.Client, bob.base+"/me/publisher", nil); code != http.StatusNotFound {
		t.Errorf("a refused registration left bob with a publisher: GET /me/publisher = %d", code)
	}
}

func TestPublishingNeedsAPublisherAndTheAuthorsStatementForTheirOwnContent(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	alice := a.login(t, freshName("publish-alice"))
	skillName := freshName("summarise")
	skillID := uploadedSkill(t, alice, skillName, "Summarise it.")

	if code, body := publish(t, alice, skillID, `{"rights_attested":true}`); code != http.StatusConflict || body["reason"] != "no_publisher" {
		t.Fatalf("publishing before registering: %d %v, want 409 no_publisher", code, body)
	}
	publisher := freshName("alice")
	registerPublisher(t, alice, publisher)

	if code, body := publish(t, alice, skillID, `{}`); code != http.StatusUnprocessableEntity || body["reason"] != "rights_not_attested" {
		t.Fatalf("publishing self-supplied content without the statement: %d %v, want 422 rights_not_attested", code, body)
	}
	if n := countRow(t, pool, `SELECT count(*) FROM publications WHERE skill_id = $1`, mustUUID(t, skillID)); n != 0 {
		t.Fatalf("a refused publish left %d publication rows", n)
	}

	code, body := publish(t, alice, skillID, `{"rights_attested":true}`)
	if code != http.StatusOK {
		t.Fatalf("publishing with the statement: %d %v", code, body)
	}
	if want := "/p/" + publisher + "/" + skillName; body["address"] != want || body["status"] != "published" {
		t.Errorf("publication = %v, want address %s and status published", body, want)
	}
	releases := objects(t, body["releases"])
	if len(releases) != 1 || releases[0]["version_number"] != float64(1) || releases[0]["rights_attested"] != true {
		t.Errorf("releases = %v, want one release of version 1 carrying the statement", releases)
	}
	if n := countRow(t, pool, `SELECT count(*) FROM audit_events WHERE action = 'publication.release' AND workspace_id = $1`,
		mustUUID(t, alice.workspaceID)); n != 1 {
		t.Errorf("publication.release audit rows = %d, want 1", n)
	}
}

func TestTheReleaseGateRefusesAHoldABlockedAndAnUnknownLicence(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	alice := a.login(t, freshName("gate-alice"))
	registerPublisher(t, alice, freshName("gate"))

	for _, tc := range []struct {
		name, assignment, reason string
	}{
		{"unknown", "redistribution = 'unknown'", "license_unknown"},
		{"blocked", "redistribution = 'blocked'", "not_redistributable"},
		{"held", "redistribution = 'allowed', access_restriction = 'license_review'", "license_hold"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			skillID := uploadedSkill(t, alice, freshName("gate-"+tc.name), "Do it.")
			setSkill(t, pool, skillID, tc.assignment)
			if code, body := publish(t, alice, skillID, `{"rights_attested":true}`); code != http.StatusUnprocessableEntity || body["reason"] != tc.reason {
				t.Errorf("%s: %d %v, want 422 %s", tc.name, code, body, tc.reason)
			}
		})
	}
}

func TestThePublicAddressIsReadableWithoutSigningInAndFollowsATakedownAtOnce(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	alice := a.login(t, freshName("public-alice"))
	registerPublisher(t, alice, freshName("public"))
	skillID := uploadedSkill(t, alice, freshName("public-skill"), "Explain it.")
	_, published := publish(t, alice, skillID, `{"rights_attested":true}`)
	address, _ := published["address"].(string)

	code, body := publicRead(t, a, address)
	if code != http.StatusOK {
		t.Fatalf("anonymous GET %s: %d %v", address, code, body)
	}
	if availability, _ := body["availability"].(map[string]any); availability["value"] != "available" {
		t.Errorf("availability = %v, want available", body["availability"])
	}
	release, _ := body["release"].(map[string]any)
	if release == nil || release["content_hash"] == "" || release["redistribution"] == nil {
		t.Errorf("the public address did not disclose the release: %v", body)
	}
	if exposure, _ := body["exposure"].(map[string]any); exposure["available"] != false || exposure["note"] == "" {
		t.Errorf("exposure = %v, want not listed and a sentence saying so", body["exposure"])
	}

	setSkill(t, pool, skillID, "takedown_at = now(), takedown_reason = 'fixture'")
	code, body = publicRead(t, a, address)
	if code != http.StatusOK {
		t.Fatalf("anonymous GET after the takedown: %d %v", code, body)
	}
	if availability, _ := body["availability"].(map[string]any); availability["value"] != "taken_down" {
		t.Errorf("after the takedown availability = %v, want taken_down", body["availability"])
	}
	if body["skill"] != nil || body["release"] != nil || len(body["releases"].([]any)) != 0 {
		t.Errorf("a taken-down Skill's public address still carries its content: %v", body)
	}
	if n := countRow(t, pool, `SELECT count(*) FROM publications WHERE skill_id = $1 AND status = 'published'`, mustUUID(t, skillID)); n != 1 {
		t.Errorf("the takedown was expected to need no second switch, yet the publication changed state (%d published rows)", n)
	}

	if code, _ := publicRead(t, a, "/p/no-such-publisher/"+freshName("nothing")); code != http.StatusNotFound {
		t.Errorf("an address nobody published: %d, want 404", code)
	}
}

func TestRepublishingAddsAReleaseAndThePublicationKeepsItsName(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	alice := a.login(t, freshName("republish-alice"))
	registerPublisher(t, alice, freshName("republish"))
	skillName := freshName("tidy")
	skillID := uploadedSkill(t, alice, skillName, "First way.")
	if code, body := publish(t, alice, skillID, `{"rights_attested":true}`); code != http.StatusOK {
		t.Fatalf("first publish: %d %v", code, body)
	}
	if again := uploadedSkill(t, alice, skillName, "Second way."); again != skillID {
		t.Fatalf("the second upload became skill %s, want a new version of %s", again, skillID)
	}

	code, body := publish(t, alice, skillID, `{"rights_attested":true}`)
	if code != http.StatusOK {
		t.Fatalf("second publish: %d %v", code, body)
	}
	releases := objects(t, body["releases"])
	if len(releases) != 2 || releases[0]["version_number"] != float64(2) || releases[1]["version_number"] != float64(1) {
		t.Errorf("releases = %v, want version 2 then version 1", releases)
	}
	if code, body := publish(t, alice, skillID, `{"name":"another-name","rights_attested":true}`); code != http.StatusConflict || body["reason"] != "name_is_permanent" {
		t.Errorf("renaming through a publish: %d %v, want 409 name_is_permanent", code, body)
	}

	code, body = deleteJSON(t, alice, "/skills/"+skillID+"/publication")
	if code != http.StatusOK || body["status"] != "delisted" {
		t.Fatalf("delist: %d %v, want 200 delisted", code, body)
	}
	_, public := publicRead(t, a, body["address"].(string))
	if availability, _ := public["availability"].(map[string]any); availability["value"] != "delisted" || public["release"] != nil || public["delisted_at"] == nil || len(public["releases"].([]any)) != 0 {
		t.Errorf("a delisted address = %v, want delisted with the time it was withdrawn and without content", public)
	}
	if code, body := publish(t, alice, skillID, `{"rights_attested":true}`); code != http.StatusOK || body["status"] != "published" {
		t.Errorf("publishing again after a delist: %d %v, want 200 published", code, body)
	}
}

func TestTwoSkillsCannotShareOnePublicationName(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	alice := a.login(t, freshName("clash-alice"))
	registerPublisher(t, alice, freshName("clash"))
	first := uploadedSkill(t, alice, freshName("first"), "One.")
	second := uploadedSkill(t, alice, freshName("second"), "Two.")
	shared := freshName("shared")

	if code, body := publish(t, alice, first, `{"name":"`+shared+`","rights_attested":true}`); code != http.StatusOK {
		t.Fatalf("first skill: %d %v", code, body)
	}
	if code, body := publish(t, alice, second, `{"name":"`+shared+`","rights_attested":true}`); code != http.StatusConflict || body["reason"] != "name_taken" {
		t.Errorf("second skill under the same name: %d %v, want 409 name_taken", code, body)
	}
}

func TestAnotherAccountCannotPublishOrDelistYourSkill(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	alice := a.login(t, freshName("owner-alice"))
	mallory := a.login(t, freshName("owner-mallory"))
	registerPublisher(t, alice, freshName("owner"))
	registerPublisher(t, mallory, freshName("mallory"))
	skillID := uploadedSkill(t, alice, freshName("owned"), "Mine.")
	if code, body := publish(t, alice, skillID, `{"rights_attested":true}`); code != http.StatusOK {
		t.Fatalf("owner publish: %d %v", code, body)
	}

	if code, _ := publish(t, mallory, skillID, `{"rights_attested":true}`); code != http.StatusNotFound {
		t.Errorf("another account publishing alice's skill: %d, want 404", code)
	}
	if code, _ := deleteJSON(t, mallory, "/skills/"+skillID+"/publication"); code != http.StatusNotFound {
		t.Errorf("another account delisting alice's skill: %d, want 404", code)
	}
	if code := getJSON(t, mallory.Client, mallory.base+"/skills/"+skillID+"/publication", nil); code != http.StatusNotFound {
		t.Errorf("another account reading alice's owner view: %d, want 404", code)
	}
	if n := countRow(t, pool, `SELECT count(*) FROM publications WHERE skill_id = $1 AND status = 'published'`, mustUUID(t, skillID)); n != 1 {
		t.Errorf("alice's publication is no longer published (%d rows)", n)
	}
}

func TestAccountPurgeRemovesThePublicationsAndKeepsTheNameReserved(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	alice := a.login(t, freshName("purge-alice"))
	publisher := freshName("purge")
	registerPublisher(t, alice, publisher)
	skillID := uploadedSkill(t, alice, freshName("purged"), "Gone soon.")
	if code, body := publish(t, alice, skillID, `{"rights_attested":true}`); code != http.StatusOK {
		t.Fatalf("publish: %d %v", code, body)
	}
	if code, _ := deleteJSON(t, alice, "/me"); code != http.StatusOK {
		t.Fatalf("DELETE /me: %d", code)
	}
	if _, err := a.auth.Service.PurgeExpiredAccounts(context.Background(), &recordingStore{}, 0, 100); err != nil {
		t.Fatal(err)
	}

	if n := countRow(t, pool, `SELECT count(*) FROM publications p JOIN publishers pb ON pb.id = p.publisher_id
		WHERE pb.workspace_id = $1`, mustUUID(t, alice.workspaceID)); n != 0 {
		t.Errorf("%d publications survived the account purge", n)
	}
	if n := countRow(t, pool, `SELECT count(*) FROM publishers WHERE name = $1`, publisher); n != 1 {
		t.Errorf("the purged account's publisher name is free for someone else to take (%d rows)", n)
	}
}

func acquire(t *testing.T, c *client, address string) (int, map[string]any) {
	t.Helper()
	return postJSON(t, c, "/publications"+strings.TrimPrefix(address, "/p")+"/acquisitions", `{}`)
}

func publishedSkill(t *testing.T, author *client, prefix string) (skillID, skillName, address string) {
	t.Helper()
	registerPublisher(t, author, freshName(prefix))
	skillName = freshName(prefix + "-skill")
	skillID = uploadedSkill(t, author, skillName, "Do the thing carefully.")
	code, body := publish(t, author, skillID, `{"rights_attested":true}`)
	if code != http.StatusOK {
		t.Fatalf("publishing %s: %d %v", skillName, code, body)
	}
	return skillID, skillName, body["address"].(string)
}

func TestAcquiringAPublicationRecordsADownloadInTheAcquirersOwnWorkspace(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	alice := a.login(t, freshName("acquire-alice"))
	bob := a.login(t, freshName("acquire-bob"))
	_, skillName, address := publishedSkill(t, alice, "acquire")

	code, body := acquire(t, bob, address)
	if code != http.StatusCreated {
		t.Fatalf("bob acquiring %s: %d %v", address, code, body)
	}
	artifactID, _ := body["artifact_id"].(string)
	if body["content_url"] != "/downloads/"+artifactID+"/content" || body["duplicate"] != false {
		t.Errorf("acquisition = %v, want a new artifact and the address of its bytes", body)
	}
	if n := countRow(t, pool, `SELECT count(*) FROM download_artifacts WHERE artifact_id = $1 AND workspace_id = $2`,
		mustUUID(t, artifactID), mustUUID(t, bob.workspaceID)); n != 1 {
		t.Errorf("the acquired artifact is not recorded in bob's workspace (%d rows)", n)
	}
	if downloads := alice.listDownloads(t); len(downloads) != 0 {
		t.Errorf("the author's downloads gained %d rows from someone else's acquisition", len(downloads))
	}
	downloads := bob.listDownloads(t)
	if len(downloads) != 1 || downloads[0].ArtifactID != artifactID || downloads[0].IncludesTestCases ||
		downloads[0].FileName != skillName+"-v1-standard.zip" {
		t.Fatalf("bob's downloads = %+v, want the author's version 1 as a standard package without test cases", downloads)
	}

	resp, data := bob.fetchContent(t, artifactID)
	if resp.StatusCode != http.StatusOK || !strings.HasPrefix(string(data), "PK") {
		t.Errorf("bob fetching the acquired bytes: %d (%d bytes), want 200 and a zip", resp.StatusCode, len(data))
	}
	if resp, _ := alice.fetchContent(t, artifactID); resp.StatusCode != http.StatusNotFound {
		t.Errorf("the author fetching bob's artifact: %d, want 404", resp.StatusCode)
	}

	if code, again := acquire(t, bob, address); code != http.StatusCreated || again["artifact_id"] != artifactID || again["duplicate"] != true {
		t.Errorf("acquiring the same release again: %d %v, want the kept artifact %s", code, again, artifactID)
	}
}

func TestAnUnofferedPublicationCannotBeAcquiredAndSaysWhyAsItsAddressDoes(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	alice := a.login(t, freshName("unoffered-alice"))
	bob := a.login(t, freshName("unoffered-bob"))
	skillID, _, address := publishedSkill(t, alice, "unoffered")

	for _, tc := range []struct {
		name, assignment, want string
	}{
		{"held", "access_restriction = 'license-review'", "held"},
		{"taken down", "access_restriction = NULL, takedown_at = now(), takedown_reason = 'fixture'", "taken_down"},
	} {
		setSkill(t, pool, skillID, tc.assignment)
		_, public := publicRead(t, a, address)
		availability, _ := public["availability"].(map[string]any)
		code, body := acquire(t, bob, address)
		if code != http.StatusConflict || body["reason"] != tc.want || body["error"] != availability["note"] {
			t.Errorf("%s: acquiring gave %d %v, want 409 %s saying what the address says (%v)", tc.name, code, body, tc.want, availability)
		}
		if acquisition, _ := public["acquisition"].(map[string]any); acquisition["available"] != false {
			t.Errorf("%s: the address still offers a download: %v", tc.name, acquisition)
		}
	}
	setSkill(t, pool, skillID, "takedown_at = NULL, takedown_reason = NULL")
	if code, body := deleteJSON(t, alice, "/skills/"+skillID+"/publication"); code != http.StatusOK {
		t.Fatalf("delist: %d %v", code, body)
	}
	if code, body := acquire(t, bob, address); code != http.StatusConflict || body["reason"] != "delisted" {
		t.Errorf("acquiring a delisted publication: %d %v, want 409 delisted", code, body)
	}
	if code, _ := acquire(t, bob, "/p/no-such-publisher/"+freshName("nothing")); code != http.StatusNotFound {
		t.Errorf("acquiring an address nobody published: %d, want 404", code)
	}
	if n := countRow(t, pool, `SELECT count(*) FROM download_artifacts WHERE workspace_id = $1`, mustUUID(t, bob.workspaceID)); n != 0 {
		t.Errorf("refused acquisitions left %d artifacts in bob's workspace", n)
	}
}

func TestUninvitedAccountsAcquireOnlyWhenTheDeploymentOpensDownloads(t *testing.T) {
	pool := requireDB(t)
	for _, tc := range []struct {
		name        string
		open        bool
		want        int
		saysInvited bool
	}{
		{"downloads kept to the invited", false, http.StatusForbidden, true},
		{"downloads opened to everyone", true, http.StatusCreated, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			authorName, invitedName := freshName("gate-author"), freshName("gate-invited")
			a := newAPITuned(t, pool, "", func(d *apiserver.Deps) {
				d.Auth.Invited = map[string]bool{authorName: true, invitedName: true}
				d.Publishing.DownloadsOpenToUninvited = tc.open
			})
			author := a.login(t, authorName)
			invited := a.login(t, invitedName)
			stranger := a.login(t, freshName("gate-uninvited"))
			_, _, address := publishedSkill(t, author, "gate")

			code, body := acquire(t, stranger, address)
			if code != tc.want {
				t.Fatalf("an uninvited account acquiring: %d %v, want %d", code, body, tc.want)
			}
			if tc.open {
				if resp, _ := stranger.fetchContent(t, body["artifact_id"].(string)); resp.StatusCode != http.StatusOK {
					t.Errorf("an uninvited account fetching what it acquired: %d, want 200", resp.StatusCode)
				}
			}
			if code, body := acquire(t, invited, address); code != http.StatusCreated {
				t.Errorf("an invited account acquiring: %d %v, want 201", code, body)
			}
			_, public := publicRead(t, a, address)
			acquisition, _ := public["acquisition"].(map[string]any)
			note, _ := acquisition["note"].(string)
			if strings.Contains(note, "這個部署目前只開放受邀者下載") != tc.saysInvited || acquisition["available"] != true {
				t.Errorf("the address says %v, want it to say invite-only = %v before any button", acquisition, tc.saysInvited)
			}
		})
	}
}
