package analytics

import (
	"net/http"
	"time"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/httpx"
)

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

func (h *Handler) DataRetention(w http.ResponseWriter, _ *http.Request) {
	days := 0
	if h.Svc != nil {

		days = int(h.Svc.Retention / (24 * time.Hour))
		if h.Svc.Retention%(24*time.Hour) != 0 {
			days++
		}
	}

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

		feedback["retention_days"] = nil
		feedback["note"] = "這個部署沒有設定回報的保存期限（FEEDBACK_RETENTION 未設），所以這些回報會一直保留，直到設定期限並執行 maintenance purge-feedback"
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"collecting":     h.Svc.Enabled(),
		"retention_days": days,
		"feedback":       feedback,

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

			{
				"name":         EventDownloadStarted,
				"when":         "要求下載時",
				"attributes":   []string{"artifact_id"},
				"not_recorded": "檔案實際上有沒有送出去；那件事記在 download_records，那是領域事實，而兩者分開正是這個事件存在的理由",
			},
		},

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
