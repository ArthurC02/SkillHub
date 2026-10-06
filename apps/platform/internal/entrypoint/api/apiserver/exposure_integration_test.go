package apiserver_test

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/publishing"
)

type exposureWorld struct {
	a        *api
	pool     *pgxpool.Pool
	author   *client
	operator *client
	skillID  string
	name     string
	address  string
}

func newExposureWorld(t *testing.T, prefix string) exposureWorld {
	t.Helper()
	pool := requireDB(t)
	a := newAPI(t, pool)
	author := a.login(t, freshName(prefix+"-author"))
	operator := a.login(t, freshName(prefix+"-operator"))
	a.auth.Operators = map[string]bool{operator.userID: true}
	skillID, name, address := publishedSkill(t, author, prefix)
	w := exposureWorld{a: a, pool: pool, author: author, operator: operator, skillID: skillID, name: name, address: address}
	w.enrich(t)
	return w
}

func (w exposureWorld) enrich(t *testing.T) {
	t.Helper()
	if _, err := w.pool.Exec(context.Background(),
		"UPDATE search_documents SET enrichment_status = 'enriched', listable = true WHERE skill_id = $1",
		mustUUID(t, w.skillID)); err != nil {
		t.Fatal(err)
	}
}

func (w exposureWorld) casePath() string {
	return "/admin/publications" + strings.TrimPrefix(w.address, "/p") + "/exposure"
}

func (w exposureWorld) exposureCase(t *testing.T) map[string]any {
	t.Helper()
	code, body := getAdmin(t, w.operator, w.casePath())
	if code != http.StatusOK {
		t.Fatalf("GET %s: %d %v", w.casePath(), code, body)
	}
	return body
}

func (w exposureWorld) review(t *testing.T, releaseID string, sequence int, digest, decision, reason string) (int, map[string]any) {
	t.Helper()
	return postJSON(t, w.operator, w.casePath(), fmt.Sprintf(
		`{"release_id":%q,"expected_sequence":%d,"expected_snapshot_digest":%q,"decision":%q,"reason":%q}`,
		releaseID, sequence, digest, decision, reason))
}

func snapshotDigestOf(c map[string]any) string {
	snapshot, _ := c["snapshot"].(map[string]any)
	digest, _ := snapshot["digest"].(string)
	return digest
}

func (w exposureWorld) reviewCurrent(t *testing.T, decision, reason string) (int, map[string]any) {
	t.Helper()
	c := w.exposureCase(t)
	release, _ := c["release"].(map[string]any)
	sequence, _ := c["sequence"].(float64)
	return w.review(t, release["release_id"].(string), int(sequence), snapshotDigestOf(c), decision, reason)
}

func (w exposureWorld) allowRedistribution(t *testing.T) {
	t.Helper()
	setSkill(t, w.pool, w.skillID, "redistribution = 'allowed'")
}

func (w exposureWorld) searchFinds(t *testing.T) bool {
	t.Helper()
	var body struct {
		Results []struct {
			SkillID string `json:"skill_id"`
		} `json:"results"`
	}
	path := w.a.URL + "/api/skills/search?q=" + url.QueryEscape(w.name)
	if code := getJSON(t, http.DefaultClient, path, &body); code != http.StatusOK {
		t.Fatalf("anonymous search: %d", code)
	}
	for _, r := range body.Results {
		if r.SkillID == w.skillID {
			return true
		}
	}
	return false
}

func (w exposureWorld) anonymousDetail(t *testing.T) int {
	t.Helper()
	var body map[string]any
	return getJSON(t, http.DefaultClient, w.a.URL+"/api/skills/"+w.skillID, &body)
}

func (w exposureWorld) publicExposure(t *testing.T) map[string]any {
	t.Helper()
	_, public := publicRead(t, w.a, w.address)
	exposure, _ := public["exposure"].(map[string]any)
	return exposure
}

func (w exposureWorld) ownerExposureState(t *testing.T) string {
	t.Helper()
	publications := ownPublications(t, w.author)
	if len(publications) != 1 {
		t.Fatalf("owner publications = %v, want one", publications)
	}
	exposure, _ := publications[0]["catalog_exposure"].(map[string]any)
	state, _ := exposure["state"].(string)
	if state == "" {
		t.Fatalf("owner exposure = %v, want a named state", exposure)
	}
	return state
}

func (w exposureWorld) waitingForReview(t *testing.T) bool {
	t.Helper()
	_, body := getAdmin(t, w.operator, "/admin/exposure-reviews")
	for _, entry := range objects(t, body["publications"]) {
		if entry["address"] == w.address {
			return true
		}
	}
	return false
}

func (w exposureWorld) approveAndAssertExposedToAnyone(t *testing.T) {
	t.Helper()
	code, body := w.reviewCurrent(t, "approved", "checked the licence and the text")
	if code != http.StatusOK || body["exposed"] != true || body["sequence"] != float64(1) {
		t.Fatalf("approving: %d %v, want exposed at sequence 1", code, body)
	}
	if !w.searchFinds(t) {
		t.Errorf("an approved release is missing from anonymous search")
	}
	if code := w.anonymousDetail(t); code != http.StatusOK {
		t.Errorf("an approved release's detail page answers %d to anyone, want 200", code)
	}
	if exposure := w.publicExposure(t); exposure["available"] != true {
		t.Errorf("the public address still says it is not listed: %v", exposure)
	}
	if w.waitingForReview(t) {
		t.Errorf("an approved, current release is still waiting in the queue")
	}
	if n := countRow(t, w.pool, `SELECT count(*) FROM audit_events WHERE action = 'publication.exposure.review' AND resource_id = (SELECT id FROM publications WHERE skill_id = $1)`,
		mustUUID(t, w.skillID)); n != 1 {
		t.Errorf("the review wrote %d audit events, want 1", n)
	}
}

func TestExposureReviewAppearsInOperatorAuditLog(t *testing.T) {
	w := newExposureWorld(t, "exposure-audit")
	w.allowRedistribution(t)
	if code, body := w.reviewCurrent(t, "approved", "reviewed the current release"); code != http.StatusOK {
		t.Fatalf("review: %d %v", code, body)
	}
	events := allAuditEvents(t, w.operator)
	isReview := func(event map[string]any) bool {
		metadata, ok := event["metadata"].(map[string]any)
		return ok && event["action"] == "publication.exposure.review" &&
			event["actor_user_id"] == w.operator.userID && event["resource_type"] == "publication" &&
			event["workspace_id"] == w.author.workspaceID && metadata["reason"] == "reviewed the current release"
	}
	if !slices.ContainsFunc(events, isReview) {
		t.Errorf("the operator audit log omitted the completed exposure review: %v", events)
	}
	code, filtered := getAdmin(t, w.operator, "/admin/audit-log?workspace_id="+w.author.workspaceID)
	if code != http.StatusOK || !slices.ContainsFunc(objects(t, filtered["events"]), isReview) {
		t.Errorf("the owner's filtered audit log omitted the exposure review: %d %v", code, filtered)
	}
	code, unrelated := getAdmin(t, w.operator, "/admin/audit-log?workspace_id="+w.operator.workspaceID)
	if code != http.StatusOK || slices.ContainsFunc(objects(t, unrelated["events"]), isReview) {
		t.Errorf("an unrelated workspace's audit log included the exposure review: %d %v", code, unrelated)
	}
}

func TestAnApprovedReleaseEntersSearchAndTheDetailPageForAnyone(t *testing.T) {
	w := newExposureWorld(t, "exposed")
	if w.searchFinds(t) || w.anonymousDetail(t) != http.StatusNotFound {
		t.Fatal("an unreviewed publication is already searchable or readable by anyone; the rest proves nothing")
	}
	if !w.waitingForReview(t) {
		t.Errorf("a published release with no conclusion is missing from the review queue")
	}
	c := w.exposureCase(t)
	snapshot, _ := c["snapshot"].(map[string]any)
	if snapshot == nil || snapshot["name"] != w.name || snapshot["current"] != true || c["sequence"] != float64(0) {
		t.Fatalf("the case = %v, want the exact searchable text of this release at sequence 0", c)
	}

	w.allowRedistribution(t)
	w.approveAndAssertExposedToAnyone(t)

	code, body := w.reviewCurrent(t, "revoked", "a reader reported a problem")
	if code != http.StatusOK || body["exposed"] != false {
		t.Fatalf("revoking: %d %v", code, body)
	}
	if w.searchFinds(t) || w.anonymousDetail(t) != http.StatusNotFound {
		t.Errorf("a revoked release is still searchable or readable by anyone")
	}
	if history := objects(t, body["history"]); len(history) != 2 || history[0]["decision"] != "revoked" || history[1]["decision"] != "approved" {
		t.Errorf("history = %v, want the revocation on top of the approval, both kept", history)
	}
}

func TestOwnerCatalogExposureTracksTheReleaseAndSearchProjection(t *testing.T) {
	w := newExposureWorld(t, "owner-exposure")
	w.allowRedistribution(t)
	if got := w.ownerExposureState(t); got != "awaiting_review" {
		t.Fatalf("unreviewed owner exposure = %q, want awaiting_review", got)
	}
	if code, body := w.reviewCurrent(t, "approved", "current release and text checked"); code != http.StatusOK {
		t.Fatalf("approving: %d %v", code, body)
	}
	if got := w.ownerExposureState(t); got != "listed" {
		t.Fatalf("approved owner exposure = %q, want listed", got)
	}

	if _, err := w.pool.Exec(context.Background(),
		"UPDATE search_documents SET listable = false WHERE skill_id = $1", mustUUID(t, w.skillID)); err != nil {
		t.Fatal(err)
	}
	if w.searchFinds(t) {
		t.Fatal("a non-listable search document is still exposed")
	}
	if exposure := w.publicExposure(t); exposure["available"] != false {
		t.Fatalf("public exposure = %v, want unavailable while search is not listable", exposure)
	}
	if c := w.exposureCase(t); c["exposed"] != false {
		t.Fatalf("operator case = %v, want exposed false while search is not listable", c)
	}
	if got := w.ownerExposureState(t); got != "search_not_ready" {
		t.Fatalf("non-listable owner exposure = %q, want search_not_ready", got)
	}

	w.enrich(t)
	if _, err := w.pool.Exec(context.Background(),
		"UPDATE search_documents SET enriched_summary = 'changed after the review' WHERE skill_id = $1", mustUUID(t, w.skillID)); err != nil {
		t.Fatal(err)
	}
	if got := w.ownerExposureState(t); got != "review_outdated" {
		t.Fatalf("changed search text owner exposure = %q, want review_outdated", got)
	}
	if code, body := w.reviewCurrent(t, "revoked", "the current text must not be listed"); code != http.StatusOK {
		t.Fatalf("revoking: %d %v", code, body)
	}
	if got := w.ownerExposureState(t); got != "revoked" {
		t.Fatalf("revoked owner exposure = %q, want revoked", got)
	}
	setSkill(t, w.pool, w.skillID, "redistribution = 'self_supplied'")
	if got := w.ownerExposureState(t); got != "not_eligible" {
		t.Fatalf("ineligible owner exposure = %q, want not_eligible", got)
	}
}

func TestANewReleaseIsNotExposedUntilReviewedAndTakesTheOldOneDownWithIt(t *testing.T) {
	w := newExposureWorld(t, "republished")
	w.allowRedistribution(t)
	if code, body := w.reviewCurrent(t, "approved", "fine"); code != http.StatusOK {
		t.Fatalf("approving v1: %d %v", code, body)
	}
	if !w.searchFinds(t) {
		t.Fatal("v1 is not searchable after approval; the rest proves nothing")
	}

	uploadedSkill(t, w.author, w.name, "A second, unreviewed way.")
	w.enrich(t)
	if w.searchFinds(t) {
		t.Errorf("an uploaded but unreleased version rode on the old approval into search")
	}
	if code, body := publish(t, w.author, w.skillID, `{"rights_attested":true}`); code != http.StatusOK {
		t.Fatalf("publishing v2: %d %v", code, body)
	}
	if w.searchFinds(t) || w.anonymousDetail(t) != http.StatusNotFound {
		t.Errorf("v2 is exposed before anyone reviewed it, or v1 keeps its exposure with old content")
	}
	if exposure := w.publicExposure(t); exposure["available"] != false {
		t.Errorf("the public address claims v2 is listed: %v", exposure)
	}
	if code, body := w.reviewCurrent(t, "approved", "v2 read too"); code != http.StatusOK || body["exposed"] != true {
		t.Fatalf("approving v2: %d %v", code, body)
	}
	if !w.searchFinds(t) {
		t.Errorf("v2 is missing from search after its own approval")
	}
}

func TestCategoryTotalTracksOnlyTheCurrentlyApprovedRelease(t *testing.T) {
	w := newExposureWorld(t, "category-exposure")
	setCategory(t, w.pool, w.skillID, "data")
	w.allowRedistribution(t)
	anon := &client{Client: http.DefaultClient, base: w.a.URL}
	categoryTotal := func() int {
		return anon.search(t, "/api/skills/catalog?category=data").Total
	}
	before := categoryTotal()

	if code, body := w.reviewCurrent(t, "approved", "first release reviewed"); code != http.StatusOK {
		t.Fatalf("approving the first release: %d %v", code, body)
	}
	if got := categoryTotal(); got != before+1 {
		t.Fatalf("approved category total = %d, want %d", got, before+1)
	}

	uploadedSkill(t, w.author, w.name, "A second, unreviewed way.")
	w.enrich(t)
	if code, body := publish(t, w.author, w.skillID, `{"rights_attested":true}`); code != http.StatusOK {
		t.Fatalf("publishing the second release: %d %v", code, body)
	}
	if got := categoryTotal(); got != before {
		t.Fatalf("unreviewed category total = %d, want %d", got, before)
	}

	if code, body := w.reviewCurrent(t, "approved", "second release reviewed"); code != http.StatusOK {
		t.Fatalf("approving the second release: %d %v", code, body)
	}
	if got := categoryTotal(); got != before+1 {
		t.Fatalf("reapproved category total = %d, want %d", got, before+1)
	}
}

func TestAReviewThatSawStaleFactsIsRefused(t *testing.T) {
	w := newExposureWorld(t, "stale")
	w.allowRedistribution(t)
	c := w.exposureCase(t)
	releaseID := c["release"].(map[string]any)["release_id"].(string)
	digest := snapshotDigestOf(c)

	for _, tc := range []struct {
		name       string
		releaseID  string
		sequence   int
		decision   string
		reason     string
		wantCode   int
		wantReason string
	}{
		{"a sequence someone already moved past", releaseID, 1, "approved", "fine", http.StatusConflict, "review_stale"},
		{"a release that is not the newest", "00000000-0000-4000-8000-000000000001", 0, "approved", "fine", http.StatusConflict, "review_stale"},
		{"no reason", releaseID, 0, "approved", "   ", http.StatusUnprocessableEntity, "reason_missing"},
		{"an unknown decision", releaseID, 0, "maybe", "fine", http.StatusUnprocessableEntity, "decision_unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, body := w.review(t, tc.releaseID, tc.sequence, digest, tc.decision, tc.reason)
			if code != tc.wantCode || body["reason"] != tc.wantReason {
				t.Errorf("got %d %v, want %d %s", code, body, tc.wantCode, tc.wantReason)
			}
		})
	}
	if code, _ := w.review(t, releaseID, 0, digest, "approved", "first look"); code != http.StatusOK {
		t.Fatalf("the first review at sequence 0: %d", code)
	}
	if code, body := w.review(t, releaseID, 0, digest, "revoked", "a second reviewer who did not see the first"); code != http.StatusConflict || body["reason"] != "review_stale" {
		t.Errorf("a second review from the same premise: %d %v, want 409 review_stale", code, body)
	}
}

func TestAnApprovalNamesTheSnapshotTheReviewerSawAndARevocationNeedsNoSuchProof(t *testing.T) {
	w := newExposureWorld(t, "digest")
	w.allowRedistribution(t)
	c := w.exposureCase(t)
	releaseID := c["release"].(map[string]any)["release_id"].(string)
	seen := snapshotDigestOf(c)
	if seen == "" {
		t.Fatal("the case carries no snapshot digest; the rest proves nothing")
	}
	reviews := func() int {
		return countRow(t, w.pool, `SELECT count(*) FROM exposure_reviews WHERE publication_id = (SELECT id FROM publications WHERE skill_id = $1)`,
			mustUUID(t, w.skillID))
	}

	if _, err := w.pool.Exec(context.Background(),
		"UPDATE search_documents SET enriched_summary = enriched_summary || 'changed while the reviewer was reading' WHERE skill_id = $1",
		mustUUID(t, w.skillID)); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, digest string }{
		{"a digest from before the text changed", seen},
		{"no digest at all", ""},
	} {
		if code, body := w.review(t, releaseID, 0, tc.digest, "approved", "fine"); code != http.StatusConflict || body["reason"] != "review_stale" {
			t.Errorf("approving with %s: %d %v, want 409 review_stale", tc.name, code, body)
		}
	}
	if n := reviews(); n != 0 {
		t.Fatalf("a stale approval left %d review rows, want 0", n)
	}

	fresh := snapshotDigestOf(w.exposureCase(t))
	if fresh == seen {
		t.Fatal("the text change did not move the digest; the rest proves nothing")
	}
	code, body := w.review(t, releaseID, 0, fresh, "approved", "read the changed text")
	if code != http.StatusOK || body["exposed"] != true {
		t.Fatalf("approving the digest the reviewer now sees: %d %v", code, body)
	}
	history := objects(t, body["history"])
	if len(history) != 1 || history[0]["snapshot_digest"] != fresh {
		t.Errorf("history = %v, want the approval recorded against the digest %s", history, fresh)
	}

	if code, body := w.review(t, releaseID, 1, seen, "revoked", "revoking needs no proof of what was seen"); code != http.StatusOK || body["exposed"] != false {
		t.Errorf("revoking with an old digest: %d %v, want 200 and not exposed", code, body)
	}
}

func TestAnApprovalJudgesTheSkillAndSnapshotOnlyWhileThePublicationIsLocked(t *testing.T) {
	w := newExposureWorld(t, "locked")
	w.allowRedistribution(t)
	svc := w.a.app.Deps.Publishing.Svc
	readSkill, readSnapshot := svc.ReadSkill, svc.ReadSearchSnapshot
	var lockedAtRead []bool
	var armed bool
	probe := func() {
		if !armed {
			return
		}
		tx, err := w.pool.Begin(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tx.Rollback(context.Background()) }()
		var id string
		err = tx.QueryRow(context.Background(),
			"SELECT id::text FROM publications WHERE skill_id = $1 FOR UPDATE NOWAIT", mustUUID(t, w.skillID)).Scan(&id)
		lockedAtRead = append(lockedAtRead, err != nil)
	}
	svc.ReadSkill = func(ctx context.Context, ws, skill pgtype.UUID) (publishing.SkillFacts, bool, error) {
		probe()
		return readSkill(ctx, ws, skill)
	}
	svc.ReadSearchSnapshot = func(ctx context.Context, skill pgtype.UUID) (publishing.SearchSnapshot, bool, error) {
		probe()
		return readSnapshot(ctx, skill)
	}
	t.Cleanup(func() { svc.ReadSkill, svc.ReadSearchSnapshot = readSkill, readSnapshot })

	c := w.exposureCase(t)
	armed = true
	code, body := w.review(t, c["release"].(map[string]any)["release_id"].(string), 0, snapshotDigestOf(c), "approved", "fine")
	armed = false
	if code != http.StatusOK {
		t.Fatalf("approving: %d %v", code, body)
	}
	if len(lockedAtRead) < 2 || !lockedAtRead[0] || !lockedAtRead[1] {
		t.Errorf("publication locked at each fact read (snapshot, skill, then the case after commit) = %v, want the first two true", lockedAtRead)
	}
}

func TestOnlyAnAllowedVerdictOnCurrentFinishedTextCanBeApproved(t *testing.T) {
	w := newExposureWorld(t, "gate")
	for _, tc := range []struct {
		name, assignment, wantReason string
	}{
		{"self supplied", "redistribution = 'self_supplied'", "redistribution_not_allowed"},
		{"unknown", "redistribution = 'unknown'", "redistribution_not_allowed"},
		{"held", "redistribution = 'allowed', access_restriction = 'license-review'", "not_available"},
	} {
		setSkill(t, w.pool, w.skillID, tc.assignment)
		if code, body := w.reviewCurrent(t, "approved", "fine"); code != http.StatusUnprocessableEntity || body["reason"] != tc.wantReason {
			t.Errorf("%s: %d %v, want 422 %s", tc.name, code, body, tc.wantReason)
		}
	}
	setSkill(t, w.pool, w.skillID, "access_restriction = NULL")
	if _, err := w.pool.Exec(context.Background(),
		"UPDATE search_documents SET enrichment_status = 'pending' WHERE skill_id = $1", mustUUID(t, w.skillID)); err != nil {
		t.Fatal(err)
	}
	if code, body := w.reviewCurrent(t, "approved", "fine"); code != http.StatusUnprocessableEntity || body["reason"] != "snapshot_pending" {
		t.Errorf("text still being enriched: %d %v, want 422 snapshot_pending", code, body)
	}
	w.enrich(t)
	if code, body := w.reviewCurrent(t, "approved", "fine"); code != http.StatusOK {
		t.Fatalf("approving once allowed and finished: %d %v", code, body)
	}
	setSkill(t, w.pool, w.skillID, "redistribution = 'self_supplied'")
	if w.searchFinds(t) {
		t.Errorf("a verdict changed from allowed to self_supplied after approval still leaves the release in search")
	}
	if _, err := w.pool.Exec(context.Background(),
		"UPDATE search_documents SET enriched_summary = 'rewritten by a later enrichment' WHERE skill_id = $1", mustUUID(t, w.skillID)); err != nil {
		t.Fatal(err)
	}
	setSkill(t, w.pool, w.skillID, "redistribution = 'allowed'")
	if w.searchFinds(t) {
		t.Errorf("text that changed after the approval is exposed without a new review")
	}
}

func TestExposureReviewsAreForOperatorsOnly(t *testing.T) {
	w := newExposureWorld(t, "operators")
	for _, path := range []string{"/admin/exposure-reviews", w.casePath()} {
		if code, _ := getAdmin(t, w.author, path); code != http.StatusNotFound {
			t.Errorf("a member reading %s: %d, want the back office's 404", path, code)
		}
	}
	if code, _ := postJSON(t, w.author, w.casePath(), `{"release_id":"x","expected_sequence":0,"decision":"approved","reason":"mine"}`); code != http.StatusNotFound {
		t.Errorf("a member approving their own publication: %d, want 404", code)
	}
}

func TestAnExposedSkillsDetailNamesNoPrivateSkillFromTheSamePackage(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	author := a.login(t, freshName("exposed-siblings-author"))
	operator := a.login(t, freshName("exposed-siblings-operator"))
	a.auth.Operators = map[string]bool{operator.userID: true}
	sharedName, privateName := freshName("exposed-siblings-shared"), freshName("exposed-siblings-private")
	res := importSource(t, a, pool, author, map[string]string{
		"plugin.json":             conformingPlugin("exposed-siblings"),
		"skills/shared/SKILL.md":  skillNamed(sharedName),
		"skills/private/SKILL.md": skillNamed(privateName),
	}, nil)
	shared := uuidText(importedAt(t, res, "skills/shared").Skill.ID)
	registerPublisher(t, author, freshName("exposed-siblings"))
	code, body := publish(t, author, shared, `{"rights_attested":true}`)
	if code != http.StatusOK {
		t.Fatalf("publishing %s: %d %v", sharedName, code, body)
	}
	w := exposureWorld{a: a, pool: pool, author: author, operator: operator,
		skillID: shared, name: sharedName, address: body["address"].(string)}
	w.enrich(t)
	if !siblingNamed(t, author, a.URL+"/api/skills/"+shared, privateName) {
		t.Fatal("the owner does not see the skill that shares the package; the rest proves nothing")
	}

	w.allowRedistribution(t)
	w.approveAndAssertExposedToAnyone(t)

	stranger := a.login(t, freshName("exposed-siblings-stranger"))
	if siblingNamed(t, stranger, a.URL+"/api/skills/"+shared, privateName) {
		t.Errorf("a stranger reading the exposed skill sees %q, a private skill of its author", privateName)
	}
	if !siblingNamed(t, author, a.URL+"/api/skills/"+shared, privateName) {
		t.Errorf("the author no longer sees %q beside their own exposed skill", privateName)
	}
}

func siblingNamed(t *testing.T, c *client, path, name string) bool {
	t.Helper()
	var got detail
	if code := getJSON(t, c.Client, path, &got); code != http.StatusOK {
		t.Fatalf("GET %s: %d", path, code)
	}
	if got.Source == nil {
		t.Fatalf("GET %s carries no source", path)
	}
	for _, sibling := range got.Source.Siblings {
		if sibling.Name == name {
			return true
		}
	}
	return false
}

func TestAnExposedGeneratedSkillKeepsItsAuthorsPromptToItsAuthor(t *testing.T) {
	w := newExposureWorld(t, "exposed-prompt")
	if _, err := w.pool.Exec(context.Background(), `
		UPDATE skill_sources SET source_type = 'generated', task_description = 'my private brief',
		       generator_model = 'fixture-model', generator_prompt_version = 'fixture-prompt',
		       generation_inputs = '{"reference_skills":[]}'::jsonb
		WHERE id = (SELECT source_id FROM skill_versions WHERE skill_id = $1 ORDER BY version_number DESC LIMIT 1)`,
		mustUUID(t, w.skillID)); err != nil {
		t.Fatal(err)
	}
	source := func(c *client) map[string]any {
		var body struct {
			Source map[string]any `json:"source"`
		}
		if code := getJSON(t, c.Client, w.a.URL+"/api/skills/"+w.skillID, &body); code != http.StatusOK {
			t.Fatalf("GET detail: %d", code)
		}
		return body.Source
	}
	if got := source(w.author); got["task_description"] != "my private brief" {
		t.Fatalf("the author's own detail = %v, want the brief; the rest proves nothing", got)
	}

	w.allowRedistribution(t)
	w.approveAndAssertExposedToAnyone(t)

	stranger := w.a.login(t, freshName("exposed-prompt-stranger"))
	got := source(stranger)
	if _, ok := got["task_description"]; ok {
		t.Errorf("a stranger reads the author's brief: %v", got["task_description"])
	}
	if _, ok := got["generation_inputs"]; ok {
		t.Errorf("a stranger reads the author's generation inputs: %v", got["generation_inputs"])
	}
	if got["type"] != "generated" {
		t.Errorf("source type = %v, want generated still said", got["type"])
	}
	if trust, _ := got["trust"].(map[string]any); trust["label"] != "由平台生成" {
		t.Errorf("a stranger is told %v; the brief was the author's, not theirs", trust["label"])
	}
	if got := source(w.author); got["task_description"] != "my private brief" {
		t.Errorf("the author's own detail after exposure = %v, want the brief still there", got)
	}
}
