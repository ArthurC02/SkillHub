# 草稿：可解釋、可修正的意圖搜尋

## 使用者結果

目錄訪客能看到輸入、輸出、工具、資料與環境五欄，保留原句並修正理解後再搜尋。分析失敗仍嘗試原句向量搜尋；向量也失敗時，誠實揭露詞法降級。完整允收以 [DISC-005](../../../plans/02-specifications-and-acceptance-criteria.md#disc-005意圖分析與查詢改寫) 為準，不以已完成的子集結案。

## 領域邊界

已審查的 Catalog Context 擁有搜尋投影與混合排名。本次沿用邊界，不新增 Aggregate、持久意圖紀錄或跨 Context 寫入；Go 驗證篩選值域、維持公開目錄 Scope 並決定降級，Python 只提供結構化分析。此處的新增行為來自需求與實作草稿，尚未提升為 Registry 的 reviewed rule 或 contract。

## 實作交接

模型輸出不可信，不能指定 Workspace 或擴大搜尋範圍。自動分析的關鍵詞只擴充 FTS 候選；原句仍決定向量、完整 bigram 覆蓋與名稱優先。使用者修正才成為新的需求，不能再讓模型覆寫。這些規則留在既有 Catalog Service，不另建搜尋子系統。

已否決把關鍵詞同時用作向量輸入與完整覆蓋優先權：固定語料分別顯示品質退步與投毒曝光增加。中文正規化 trigram 候選尚未採用；沒有以調低下界、移除既有能力、清空結果或依投毒檔名排除來通過驗收。

使用者已授權設計與實作版本綁定的內容審核／公開曝光資格；持久狀態、跨 Context 協作與驗收獨立記錄在[曝光審核提案](../catalog-exposure-review/draft-pr.md)，不把本提案的唯讀邊界擴張成跨 Context 寫入許可。投毒 Top-3 實測與並行成本仍未完成；授權本身不是通過證據或 Registry 正式核准。

## 提案與核准

`catalog-correctable-intent` 維持 `draft`，未提交、未核准、未套用。`registry_updates` 為空；不建立虛構的審查者、簽章或 SCM 證據。此目錄是可提交的提案來源，scratchpad 中的早期副本僅供歷史診斷，不再作為待維護的提案入口。

## 契約影響

公開 GET 回應新增 interpretation；相同搜尋資源新增接受使用者修正的 POST；內部契約新增意圖分析呼叫。契約先於下游型別，生成檔不手改。最新本機 `gen:check` 四組產物一致，不代表遠端相容性閘門已通過。

## 驗證與限制

七項允收對應 `test-obligations.json`，完整量測、突變及執行範圍見[意圖搜尋驗證](../../../plans/intent-search-validation.md)。已有本機完整 Platform 回歸、三引擎瀏覽器替身測試、Go／Python HTTP 整合，以及真實 Gateway 的耗盡拒絕證據；付費／部署跳過項與未知成本不得當成通過。逐義務、帶輸出雜湊的正式 attestation 尚未收齊，所以證據包仍未 verified。

Domain Memory 唯讀檢查結果：Registry 有效且 reviewed，來源選擇仍為 developer-confirmed，49 筆引用 current、零 stale／missing／invalid，55 筆稽核事件的雜湊鏈有效。兩份來源文件有引用範圍之外的變動，沒有因此重設來源確認或改寫 reviewed records。提案格式驗證只能證明結構完整，不能證明需求已完成。

## 尚未完成

中文詞法選型與固定下界、投毒紅線、未記帳並行費用、其他能力服務啟動入口、完整真實部署 E2E、文件最終對帳、正式提案證據及提交後的完整 SHA 遠端 CI 都仍有缺口。丙-76 保持開放。
