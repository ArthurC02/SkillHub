# 私人 Run 搜尋證據契約提案

工具結果可能裁切，目前事件契約沒有獨立的查詢、來源網址與費用欄位。提議在既有 `tool_call` 增加選填 `search` 物件，只能附於 `mcp__skillhub__web_search`，新生產者宣告 `1.3`；具體形狀見同目錄的 `proposed-search-evidence.schema.json`。

查詢和來源沿既有遮罩後保存，進階 Trace 以惰性文字閱讀。`cost_usd: null` 表示未知，零表示已回報免費；Sandbox 的數字不參與結算。既有事件、短效 token、Workspace 範圍、重送冪等及保留規則維持。

本提案只準備契約與測試義務，尚未改動正式契約或產品程式，亦未宣稱測試已執行。需要 developer 同意這個新增契約；runtime 映像、供應商開通及部署另行處理。
