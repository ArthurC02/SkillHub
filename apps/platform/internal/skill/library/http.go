package registry

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/httpx"
)

// Handler exposes registry endpoints (contracts/openapi/public.yaml). All
// routes require a session; the workspace comes from it (iron rule 3).
type Handler struct {
	Svc      *Service
	Identity *identity.Service
}

func (h *Handler) workspace(w http.ResponseWriter, r *http.Request) (identity.Workspace, bool) {
	user, ok := identity.SessionUser(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "not authenticated")
		return identity.Workspace{}, false
	}
	ws, err := h.Identity.PersonalWorkspace(r.Context(), user)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "workspace lookup failed")
		return identity.Workspace{}, false
	}
	return ws, true
}

type skillResponse struct {
	SkillID string `json:"skill_id"`
	Name    string `json:"name"`
	Summary string `json:"summary"`
	// Both of these were already on the row `ListSkills` selects and were dropped
	// here. They are two of the four locks that refuse a download, and the owner's
	// own list is where they matter most: `unknown` is what a user's own import
	// carries by default (0027), so a skill they cannot take away looked exactly
	// like one they could until they reached the packaging screen.
	Redistribution      string  `json:"redistribution"`
	AccessRestriction   *string `json:"access_restriction"`
	ForkedFromSkillID   *string `json:"forked_from_skill_id,omitempty"`
	ForkedFromVersionID *string `json:"forked_from_version_id,omitempty"`
}

func toSkillResponse(s gen.Skill) skillResponse {
	out := skillResponse{
		SkillID:           pgconv.UUIDString(s.ID),
		Name:              s.Name,
		Redistribution:    s.Redistribution,
		AccessRestriction: s.AccessRestriction,
	}
	if s.Summary != nil {
		out.Summary = *s.Summary
	}
	if s.ForkedFromSkillID.Valid {
		v := pgconv.UUIDString(s.ForkedFromSkillID)
		out.ForkedFromSkillID = &v
	}
	if s.ForkedFromVersionID.Valid {
		v := pgconv.UUIDString(s.ForkedFromVersionID)
		out.ForkedFromVersionID = &v
	}
	return out
}

// Fork handles POST /skills/{id}/fork.
func (h *Handler) Fork(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.workspace(w, r)
	if !ok {
		return
	}
	var skillID pgtype.UUID
	if err := skillID.Scan(r.PathValue("id")); err != nil {
		httpx.WriteError(w, http.StatusNotFound, ErrNotFound.Error())
		return
	}

	fork, ver, err := h.Svc.Fork(r.Context(), ws, skillID)
	switch {
	case errors.Is(err, ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, err.Error())
		return
	case errors.Is(err, ErrNameTaken):
		httpx.WriteError(w, http.StatusConflict, err.Error())
		return
	case err != nil:
		httpx.WriteError(w, http.StatusInternalServerError, "fork failed")
		return
	}

	resp := struct {
		skillResponse
		VersionID     string `json:"version_id"`
		VersionNumber int32  `json:"version_number"`
	}{toSkillResponse(fork), pgconv.UUIDString(ver.ID), ver.VersionNumber}
	httpx.WriteJSON(w, http.StatusCreated, resp)
}

// Delete handles DELETE /skills/{id} (WS-005).
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.workspace(w, r)
	if !ok {
		return
	}
	var skillID pgtype.UUID
	if err := skillID.Scan(r.PathValue("id")); err != nil {
		httpx.WriteError(w, http.StatusNotFound, ErrNotFound.Error())
		return
	}

	res, err := h.Svc.Delete(r.Context(), ws, skillID)
	if errors.Is(err, ErrNotFound) {
		httpx.WriteError(w, http.StatusNotFound, err.Error())
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "delete failed")
		return
	}
	// 狀態回饋 (WS-005): say exactly what the deletion covers.
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"deleted":           true,
		"versions_retained": res.VersionsRetained,
		"note":              deletionNote,
	})
}

// deletionNote is what the user is told, verbatim, at the moment their skill
// goes away. It is a const so that a test can hold it to 02 §2.2 -- 不得顯示
// 沒被強制的承諾 -- because until 2026-08-25 it did not keep that rule.
//
// It used to end "version snapshots are retained for the 30-day grace period,
// then purged". Nothing purged them. The only hard delete of a skill in this
// repo is PurgeUnreferencedSkills (purge.go), which takes a workspace id, has no
// deleted_at predicate, and runs from account deletion alone; a skill deleted on
// its own keeps deleted_at set and its rows forever. So the one sentence that
// named a deadline was the one sentence with nothing behind it -- the same shape
// as the audit-event and run-output rows of the consent table (04 丙-63).
//
// The deadline is not restored by writing the purge job, and that is the point:
// the 30 days come from PDM-006 §6.1, which is still unratified, which is exactly
// why the numeral was struck from /policy, /workspace/account and the confirm
// text on /workspace/skills. Those three were scrubbed on the stated grounds
// that the server's note was "where a grace period gets stated by something
// that enforces it". It was not. Hard-deleting a user's content on a deadline
// nobody has signed is worse than keeping it, so the claim goes and the job
// waits for the signature.
const deletionNote = "已從你的工作區、清單與搜尋移除；版本快照維持凍結，這次刪除不會移除它們；" +
	"Fork 引用的共用套件物件不受影響"

// Takedown handles POST /skills/{id}/takedown (INGEST-010). The caller must own
// the workspace the skill lives in; see Service.Takedown for why that is the
// whole authorization story in the MVP.
func (h *Handler) Takedown(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.workspace(w, r)
	if !ok {
		return
	}
	var skillID pgtype.UUID
	if err := skillID.Scan(r.PathValue("id")); err != nil {
		httpx.WriteError(w, http.StatusNotFound, ErrNotFound.Error())
		return
	}
	var body struct {
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "body must be JSON with a reason")
		return
	}
	// A takedown nobody can explain later is not a moderation decision.
	if strings.TrimSpace(body.Reason) == "" {
		httpx.WriteError(w, http.StatusBadRequest, "reason is required")
		return
	}

	skill, err := h.Svc.Takedown(r.Context(), ws, skillID, strings.TrimSpace(body.Reason))
	switch {
	case errors.Is(err, ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, err.Error())
		return
	case errors.Is(err, ErrAlreadyTakenDown):
		httpx.WriteError(w, http.StatusConflict, err.Error())
		return
	case err != nil:
		httpx.WriteError(w, http.StatusInternalServerError, "takedown failed")
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"skill_id":    pgconv.UUIDString(skill.ID),
		"takedown_at": skill.TakedownAt.Time.UTC().Format(time.RFC3339),
		"reason":      body.Reason,
		"note": "removed from search and from the fork path; the skill, its versions " +
			"and their sources are retained, and existing forks and past runs are unaffected",
	})
}

// skillVersionResponse mirrors the inline `version` object the skill detail
// serves, so the version history and the detail view name one version the same
// way.
type skillVersionResponse struct {
	VersionID     string `json:"version_id"`
	VersionNumber int32  `json:"version_number"`
	ContentHash   string `json:"content_hash"`
	CreatedAt     string `json:"created_at"`
}

// Versions handles GET /skills/{id}/versions (WS-001) — the version history the
// pre-run and packaging screens pick from.
//
// A skill outside the caller's workspace answers 200 with an empty list rather
// than 404: the scope comes from the session (iron rule 3), so a caller cannot
// tell the two apart anyway, and the screens that read this treat "no versions"
// as "nothing to pick" either way.
func (h *Handler) Versions(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.workspace(w, r)
	if !ok {
		return
	}
	var skillID pgtype.UUID
	if err := skillID.Scan(r.PathValue("id")); err != nil {
		httpx.WriteError(w, http.StatusNotFound, ErrNotFound.Error())
		return
	}

	rows, err := gen.New(h.Svc.Pool).ListSkillVersions(r.Context(), gen.ListSkillVersionsParams{
		WorkspaceID: ws.ID, SkillID: skillID,
	})
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "list failed")
		return
	}
	out := make([]skillVersionResponse, 0, len(rows))
	for _, v := range rows {
		out = append(out, skillVersionResponse{
			VersionID:     pgconv.UUIDString(v.ID),
			VersionNumber: v.VersionNumber,
			ContentHash:   v.ContentHash,
			CreatedAt:     v.CreatedAt.Time.UTC().Format(time.RFC3339),
		})
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"versions": out})
}

// Diff handles GET /skills/{id}/diff?from=&to= (WS-003).
func (h *Handler) Diff(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.workspace(w, r)
	if !ok {
		return
	}
	var skillID, fromID, toID pgtype.UUID
	if skillID.Scan(r.PathValue("id")) != nil ||
		fromID.Scan(r.URL.Query().Get("from")) != nil ||
		toID.Scan(r.URL.Query().Get("to")) != nil {
		httpx.WriteError(w, http.StatusBadRequest, "id, from, and to must be version UUIDs")
		return
	}

	diffs, err := h.Svc.DiffVersions(r.Context(), ws, skillID, fromID, toID)
	if errors.Is(err, ErrNotFound) {
		httpx.WriteError(w, http.StatusNotFound, err.Error())
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "diff failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"files": diffs})
}

// List handles GET /skills — the caller's own skills (WS-004).
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.workspace(w, r)
	if !ok {
		return
	}
	// Ask for one more than the cap so the cap can be reported rather than
	// enforced silently. The limit was already here; what was missing is that a
	// workspace past it got a short list with nothing saying so, and a list that
	// is quietly incomplete reads as a complete answer.
	const listSkillsLimit = 100
	rows, err := gen.New(h.Svc.Pool).ListSkills(r.Context(), gen.ListSkillsParams{
		WorkspaceID: ws.ID, Limit: listSkillsLimit + 1, Offset: 0,
	})
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "list failed")
		return
	}
	truncated := len(rows) > listSkillsLimit
	// The count comes off the rows, so an empty list has none to read it from --
	// and zero is the right answer there, not a missing field. Every row carries
	// the same value (count(*) OVER () is constant across the window), so row 0
	// is as good as any.
	var total int64
	if len(rows) > 0 {
		total = rows[0].TotalMatches
	}
	if truncated {
		rows = rows[:listSkillsLimit]
	}

	// Not wired is a 500, not a list without the facet. This is a page of code
	// the caller owns and will run, and 02:DISC-004 forbids letting an absent
	// scan read as a clean one — a row that silently lost its risk block is that
	// exact failure, arriving through a deployment mistake instead of a scan.
	// app_test.go asserts the wiring, so this is the runtime half of a guard that
	// normally fails at startup.
	if h.Svc.SkillRisks == nil || h.Svc.CatalogSkillRisks == nil {
		httpx.WriteError(w, http.StatusInternalServerError, "list failed")
		return
	}
	ids := make([]pgtype.UUID, 0, len(rows))
	// Ancestors are asked for separately because they are a different scope, not
	// because they are a different question (ADR-042 決策 6). The SQL has already
	// refused every ancestor that fails a precondition, so anything in here is one
	// whose bytes still hash to the same value as the row's own.
	ancestors := make([]pgtype.UUID, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.Skill.ID)
		if row.InheritedFromSkillID.Valid {
			ancestors = append(ancestors, row.InheritedFromSkillID)
		}
	}
	risks, err := h.Svc.SkillRisks(r.Context(), ws.ID, ids)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "list failed")
		return
	}
	inherited, err := h.Svc.CatalogSkillRisks(r.Context(), ancestors)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "list failed")
		return
	}

	out := make([]ownSkillResponse, 0, len(rows))
	for _, row := range rows {
		risk := risks[pgconv.UUIDString(row.Skill.ID)]
		if row.InheritedFromSkillID.Valid {
			risk = inherited[pgconv.UUIDString(row.InheritedFromSkillID)]
		}
		out = append(out, ownSkillResponse{
			skillResponse: toSkillResponse(row.Skill),
			Risk:          risk,
			Verification:  verificationOf(row),
		})
	}
	// 設計系統 §4.3: a truncated list has to say 「共 N 筆，這裡顯示 M 筆，因為 X」.
	// The reason half was already here; `total` is the count half. Until now this
	// list could only say 「超過 100 個」, a lower bound a reader cannot tell 101
	// from 10100 by.
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"skills":    out,
		"limit":     listSkillsLimit,
		"truncated": truncated,
		"total":     total,
	})
}

// ownSkillResponse is a row of the caller's own list: the skill, plus the two
// facets that make the list decidable rather than merely enumerable (04 丙-31,
// 設計系統 §1.1). Not on skillResponse itself, because the fork reply shares
// that shape and a freshly created fork has neither.
type ownSkillResponse struct {
	skillResponse
	// Catalog's block, verbatim. See registry.Service.SkillRisks.
	Risk         json.RawMessage   `json:"risk"`
	Verification skillVerification `json:"verification"`
}

// skillVerification says whether anything was measured *here*, and when.
//
// It exists because the obvious answer is wrong. `verified_at` is defined as the
// newest version's creation time, which is exactly right for an import — that
// row was created by the scan — and a lie for a fork, whose version row was
// created the instant somebody pressed Fork with nothing scanned. Serving that
// timestamp would print something that reads as "scanned just now" on the one
// case where nothing was scanned at all, which is worse than a blank; serving a
// blank is what 設計系統 §2.9 forbids. So the state is named and the timestamp
// only appears in the state that has one.
//
// 未測量 for a fork is not the same claim as "unsafe": the bytes are identical to
// something that was scanned upstream, and attributing that measurement to the
// copy is arguable. It is a product decision that has not been made, and until
// it is, the one thing forbidden is making it silently.
// Labelled rather than a bare value, which is the 04 丙-29 ruling applied at
// birth rather than retrofitted: one field, one consumer, three values, and the
// alternative is a twenty-first enum→中文 map on the client for a state whose
// whole job is to be worded carefully.
type skillVerification struct {
	// scanned | not_measured | not_applicable — 設計系統 §2.9 的固定詞彙。
	Value     string  `json:"value"`
	Label     string  `json:"label"`
	Note      string  `json:"note"`
	ScannedAt *string `json:"scanned_at"`
}

func verificationOf(row gen.ListSkillsRow) skillVerification {
	switch {
	case !row.VerifiedAt.Valid:
		return skillVerification{
			Value: "not_applicable",
			Label: "不適用",
			Note:  "這個 Skill 還沒有任何版本,沒有可掃描的內容。",
		}
	case row.InheritedFromSkillID.Valid:
		// A copy inherits measurements that hold for bytes and never those that
		// hold for an environment (ADR-042 決策 6). All four preconditions were
		// enforced in ListSkills, including comparing the content hash now rather
		// than trusting it from fork time — so reaching here means these bytes and
		// the ancestor's bytes are the same bytes, and a deterministic scan of them
		// is the same answer wherever it ran.
		//
		// The state stays `scanned` because this is not an absence: it is a value
		// with an attribution, which is why it does not appear in 設計系統 §2.9's
		// table. The timestamp is the ancestor's import, so it is *older* than the
		// fork — honest, and it tells the reader how stale the scan is.
		at := row.InheritedVerifiedAt.Time.UTC().Format(time.RFC3339)
		return skillVerification{
			Value: "scanned",
			Label: "已掃描（來源）",
			Note: "這個版本是 Fork 進來的複本,內容雜湊與來源「" + row.InheritedFromName +
				"」相同,所以沿用來源匯入時的靜態掃描結果。時間是來源掃描的時間,不是 Fork 的時間;" +
				"相容性與試跑結果不沿用,那些量的是內容在某個環境下的行為。",
			ScannedAt: &at,
		}
	case !row.VerifiedSourceID.Valid:
		// Import sets source_id; Fork leaves it NULL because skill_sources rows
		// belong to the origin workspace (registry.Fork). So a version with no
		// source is one that arrived as a copy — and this branch is the copy whose
		// ancestor could *not* answer: taken down, deleted, outside the public
		// catalogue (無權檢視 rather than inherited), or no longer the same bytes.
		return skillVerification{
			Value: "not_measured",
			Label: "未測量",
			Note: "這個版本是 Fork 進來的複本,而它的來源現在無法提供掃描結果" +
				"(已下架、不在公開目錄,或內容已經不同),平台也沒有在你的工作區重跑。",
		}
	default:
		at := row.VerifiedAt.Time.UTC().Format(time.RFC3339)
		return skillVerification{
			Value:     "scanned",
			Label:     "已掃描",
			Note:      "匯入這個版本時做過靜態掃描,不執行套件內任何程式碼;逐項結果在 Skill 頁面。",
			ScannedAt: &at,
		}
	}
}
