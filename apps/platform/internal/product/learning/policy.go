package analytics

// GET /policy/data-retention — the disclosure surface for the one data class a
// user produces without submitting anything (02:O11Y-004, ADR-029, 04 丙-25②).
//
// Same shape and the same reason as GET /test-cases/limits: a policy that only
// exists in a document is a policy the product cannot be held to, so the values
// are served from the constants the writer itself reads. `retention_days` comes
// from Service.Retention (ANALYTICS_RETENTION), not from a number typed here, so
// the page cannot promise a window the deployment does not apply.
//
// `collecting: false` is the honest answer for a deployment that has not set one,
// and it is the shipped default: NFR-002 forbids collection before a retention
// value exists, and ADR-029 決策 5's 180 days is still a proposal. The endpoint
// exists either way — "we collect nothing" is a disclosure, not an absence of one.
//
// Public, no session: a data policy a visitor has to log in to read is not a
// policy they can decide by. It reads nothing user-specific.

import (
	"net/http"
	"time"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/httpx"
)

// feedbackDays renders FeedbackRetention the way retention_days is rendered
// above, and -1 for "no window is set". A negative number rather than 0 because
// 0 is a real answer for a sub-day window, and the two must not collapse: one
// says "kept for less than a day", the other says nobody has decided.
func feedbackDays(retention time.Duration) int {
	if retention <= 0 {
		return -1
	}
	days := int(retention / (24 * time.Hour))
	if retention%(24*time.Hour) != 0 {
		days++
	}
	return days
}

// DataRetention handles GET /policy/data-retention.
func (h *Handler) DataRetention(w http.ResponseWriter, _ *http.Request) {
	days := 0
	if h.Svc != nil {
		// Rounded up, not truncated. A deployment with ANALYTICS_RETENTION=12h is
		// collecting — Enabled() only asks for a second — and reporting
		// `{"collecting": true, "retention_days": 0}` is two sentences that
		// contradict each other. Production's 8760h never reaches this; a staging
		// window shorter than a day does (M5 audit, 2026-08-25).
		days = int(h.Svc.Retention / (24 * time.Hour))
		if h.Svc.Retention%(24*time.Hour) != 0 {
			days++
		}
	}
	// BETA-003/004/005's reports, and the reason this endpoint needed a second
	// class at all. `note` below declares "there is no free-text column anywhere
	// in the table" — true of analytics_events, and a reader had no way to know
	// there is another table. What a participant types about where they got stuck
	// is the most sensitive thing this deployment holds, and it was the one
	// collected class this page did not mention.
	//
	// The window is FEEDBACK_RETENTION, the same value `maintenance
	// purge-feedback` refuses to run without, read from configuration rather than
	// written here — the rule the four events above already follow, and the
	// reason the number on this page cannot promise a sweep nobody performs.
	feedback := map[string]any{
		"what":                "由已登入的參與者在 POST /feedback 送出的回報（BETA-003/004/005）",
		"collected":           []string{"kind", "message", "page_path", "run_id", "build_id", "workspace_id", "user_id"},
		"free_text":           "message 是參與者自己寫的自由文字，最多 2000 字。它是這個部署唯一的自由文字欄位，不遮罩、不摘要、不截斷",
		"kind":                []string{"blocking_issue", "need_signal"},
		"page_path":           "他當時所在的路由，從不是完整網址：查詢字串可能帶個資，這個管道不收",
		"run_id":              "他當時看的 Run（若有），而且只在確認是他自己的 Run 之後",
		"on_account_deletion": "去識別而不是刪除：workspace_id 與 user_id 設為 NULL，文字保留（ADR-029 決策 5 的範圍複審建立在人們說了什麼之上，帳號刪除不能悄悄撤回已被計入的回報）",
	}
	if fd := feedbackDays(h.FeedbackRetention); fd >= 0 {
		feedback["retention_days"] = fd
	} else {
		// Said plainly rather than omitted. This is the honest state of a
		// deployment that has not set the variable, and it is the one a reader
		// most needs to see: PDM-006 has ratified no window for this class, so
		// nothing deletes these rows and they are kept until somebody decides.
		feedback["retention_days"] = nil
		feedback["note"] = "這個部署沒有設定回報的保存期限（FEEDBACK_RETENTION 未設），所以這些回報會一直保留，直到設定期限並執行 maintenance purge-feedback"
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"collecting":     h.Svc.Enabled(),
		"retention_days": days,
		"feedback":       feedback,
		// The closed set of four. Adding a fifth means answering first why no
		// domain table can (ADR-029 決策 1), and the CHECK in 0029 enforces it.
		//
		// Each `attributes` list is what that event adds *on top of* the five
		// fixed columns every row carries (ADR-029 決策 2). It is not the row and
		// it is not the constructor signature — the constructor's first field is
		// the session id, which belongs to all four and is disclosed once in
		// `note` rather than four times here (M5 audit, 2026-08-25).
		"events": []map[string]any{
			{
				"name":         EventSearchPerformed,
				"when":         "送出一次搜尋時",
				"attributes":   []string{"query_length", "query_language", "result_count", "has_results", "filters_applied"},
				"not_recorded": "查詢內容一個字都沒有記；query_language 是書寫系統的分桶（han／latin／mixed／other），不是語系設定",
			},
			{
				"name":         EventSkillDetailViewed,
				"when":         "開啟一個 Skill 的頁面時",
				"attributes":   []string{"skill_id"},
				"not_recorded": "你是怎麼到這一頁的——從搜尋結果來還是直接開連結，以及它在那份結果裡排第幾。這兩欄在 migration 0040 之前存在、之後刪除（04 丙-59），現在沒有任何地方記它們",
			},
			{
				"name":         EventSessionStarted,
				"when":         "一次造訪開始時",
				"attributes":   []string{},
				"not_recorded": "除了每一列都帶的那五個欄位以外什麼都沒有：這次造訪本身就是這個事件的全部。session_id 是一個不相干的隨機 cookie 值——不是登入用的 session token，也不是它的雜湊，而且反推不回去",
			},
			// No target attribute: the only caller passes an empty one
			// (feedback.go's route wrapper deliberately does not re-read the
			// artifact row for it) and DownloadStarted writes the column only when
			// it is non-empty, so it has never held a value. Naming it here told
			// the reader we collect more than we do, and this page is the one
			// ADR-029 決策 5 holds to a higher standard than any other.
			{
				"name":         EventDownloadStarted,
				"when":         "要求下載時",
				"attributes":   []string{"artifact_id"},
				"not_recorded": "檔案實際上有沒有送出去；那件事記在 download_records，那是領域事實，而兩者分開正是這個事件存在的理由",
			},
		},
		// 2026-09-08：這一段與上面每一個 `when`／`not_recorded` 原本是英文，而它們
		// 逐字印在 /policy 上給中文使用者看。那一頁的存在理由是揭露義務
		// （02:O11Y-004），也就是「被讀懂」本身就是它的允收準則——一份目標讀者讀不懂
		// 的揭露沒有滿足它。事實一件未改；欄位名與 cookie 名維持原樣（它們是識別字，
		// policy_test.go 也逐一比對它們）。
		"note": "這四個事件就是分析資料類別的全部——上面的 `feedback` 區塊是這個部署收集的另一個類別。" +
			"每一列另外都帶著 ADR-029 決策 2 為四個事件共同固定的五個欄位：event_id、" +
			"event_name、occurred_at、session_id 與 workspace_id，加在上面列出的屬性之外。" +
			"session_id 就是 sh_analytics 這個 cookie 的值：它把同一個訪客的搜尋與頁面瀏覽" +
			"串成一趟旅程，保存期限同上。workspace_id 在那個訪客登入之前是 null，而且兩者" +
			"永遠不會反向對接。這張表裡沒有任何自由文字欄位——不是「遮罩過的自由文字」，" +
			"是一個都沒有——所以 schema 沒有宣告的屬性由寫入端丟棄，不會被儲存。" +
			"在部署設定保存期限之前，包括 cookie 在內什麼都不會收集。",
	})
}
