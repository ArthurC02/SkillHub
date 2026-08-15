# M1 工作項對帳（`03-work-items.md` 第 4～8 節）

- 日期：**2026-08-15**（第一次對帳）／**2026-08-15 第二次對帳，範圍僅 §7 DISC**
- 基準 commit：第一次 **`9ab422b`**（`Fix the four M1 import-report platform bugs and widen CONTENT-006 scanning`）；§7 DISC 的判定已於 **DISC-003 篩選器落地後重做**，見 §5 開頭的說明
- 第二次對帳只重驗 §7 DISC。**§2／§3／§4／§6（CONTENT／CORE／INGEST／WS）維持第一次對帳的判定**，其基準仍是 `9ab422b`，其中提到的 DISC 現況已被 §5 取代
- 範圍：`03-work-items.md` 的 **§4 CONTENT／§5 CORE／§6 INGEST／§7 DISC／§8 WS**，逐項對照 [`02-specifications-and-acceptance-criteria.md`](../02-specifications-and-acceptance-criteria.md) 的允收準則。
- 方法：**只採信 repo 內可指名的落地證據**——migration 檔、Go 檔與匯出符號、具名測試函式、CI 設定、已交付報告。沒有證據的項目一律記「未動」，不以「應該有」補位。
- 判定值：**完全符合**（勾 `[x]`）／**部分完成**（不勾，列缺口）／**進行中**（另一位協作者的未 commit 工作，不勾）／**未動**（不勾）。
- 依 AGENTS.md 文件維護規則：**部分完成一律保持 `[ ]`**。

> **對帳當下的工作樹狀態**：另一位協作者有未 commit 的變更於 `apps/web/`、`services/platform/internal/{ingest,registry,skillpkg}`、`db/queries/`、`tools/content/import_seed.py`，以及未追蹤的 `db/migrations/0012_license_provenance.sql`。**本對帳只認 `9ab422b` 已 commit 的內容**；上述進行中的工作在對應項目標「進行中」，不列為證據，也不勾選。

---

## 1. 對帳結論總表

| 節 | 項目數 | 完全符合（本次新勾） | 先前已勾 | 部分完成 | 進行中 | 未動 |
| --- | --- | --- | --- | --- | --- | --- |
| §4 CONTENT | 9 | 0 | 2 | 2 | 0 | 5 |
| §5 CORE | 8 | **2** | 4 | 0 | 0 | 2 |
| §6 INGEST | 13 | 0 | 12 | 0 | 0 | 1 |
| §7 DISC（第二次對帳後） | 10 | **9** | 0 | **1** | 0 | 0 |
| §8 WS | 6 | **1** | 4 | 0 | 0 | 1 |
| **合計** | **46** | **12** | **22** | **3** | **0** | **9** |

**§7 DISC 第二次對帳新勾九項：`DISC-001`、`002`、`004`、`005`、`006`、`007`、`008`、`009`、`010`；`DISC-003` 維持不勾**（六個篩選維度只有兩個有逐筆資料，見 §5.3）。**→ 2026-08-15 後續：`02:DISC-002` 的篩選維度已依 §5.3 建議改為分階段允收，`DISC-003` 據此勾選，`§7 DISC` 十項全數完成；見 [§5.3.1](#531-建議已採納2026-08-15-規格修訂與-disc-003-勾選)。上表數字為修訂前的對帳快照，不追改。**第一次對帳把整節判為「後端做得比帳面深，缺的是接線」，這次驗證的正是接線是否真的完成——順帶補上了三處第一次對帳沒有查到的實際缺口：搜尋結果列沒有渲染七欄、詳情頁沒有渲染「限制」、比較表把 `tier` 標成「類別」。

**第一次對帳新勾三項：`CORE-005`、`CORE-006`、`WS-006`。** 三項的共同特徵是——允收準則本身就是一條可測的行為斷言，而該斷言有具名測試，且該測試在 CI 裡**確實會執行**（見 §3.1 的 CI 佐證）。其餘所有「看起來已完成」的項目，卡點都不在後端程式，而在**UI 未接線**或**允收準則涵蓋的範圍比已落地的部分更寬**。

---

## 2. §4 Skill 內容與供應（CONTENT）

| 項目 | 判定 | 證據 | 缺口 |
| --- | --- | --- | --- |
| CONTENT-001 收錄政策 | 先前已勾 | [PDM-002 定案](../m0/pdm-proposals.md)（三層收錄政策＋九項精選檢查表） | — |
| CONTENT-002 來源可信度／License／衍生關係呈現規則 | 先前已勾 | 同上；`services/platform/internal/catalog/trust.go`（`SourceTrust`／`LicenseStatus`／`DerivationBadge`）＋ `trust_test.go` 三個測試 | ⚠️ `trust.go` **目前沒有任何 handler 引用**，是純顯示詞彙。規則已定義，尚未接線 |
| CONTENT-003 首批候選清單 | **部分完成** | [curated-skill-list.md](curated-skill-list.md)、`tools/content/seed-skills.json`（45 筆）。**2026-08-15 補足後三類數量目標全部達標**（`documents` 精選 3→4，見該文件 §9） | **不勾。** 九項檢查的 ⑤⑦⑧ 對全部 45 筆仍為 `pending`，⑤⑦⑧ 分別由 CONTENT-006／005／007＋008 承接；該文件 §3 已自訂「四者全數完成前不得勾選」。**數量缺口已關閉，品質檢查缺口未關閉** |
| CONTENT-004 來源及 License 檢查 | **部分完成** | curated-skill-list.md §5，11 個 repo 逐一實查 `LICENSE` 檔案內容 | **不勾。** §5.2 的法務判定未完成：`anthropics/skills` 的 "reproducing … outside the Services" 條款是否禁止平台保存內容快照。若禁止，`documents` 已索引 10→6 跌破下限 |
| CONTENT-005 白話摘要 | 未動 | — | 無任何摘要產出。⚠️ 注意 `services/llm` 的 `/v1/enrich-skill` 是**索引時**的機器增強，不等於本項要求的人可讀策展摘要 |
| CONTENT-006 規格及靜態掃描 | 未動 | — | [import-report.md §8](import-report.md) 已列出三個待決事項作為輸入（授權顯示的事實來源、是否延伸掃描 SKILL.md 內嵌程式、anthropic-sa 4 筆的法務判定） |
| CONTENT-007 範例資料／Prompt／驗收條件 | 未動 | — | 依賴 M2 的 Test Case 能力 |
| CONTENT-008 基準試跑 | 未動 | — | 依賴 M2 的 Sandbox。**M1 內結構性不可能完成** |
| CONTENT-009 更新／失效／下架／來源變更流程 | 未動 | — | 與 INGEST-010 是同一件事的兩面，見 §4 |

---

## 3. §5 核心領域與帳號（CORE）

### 3.1 本次新勾兩項

#### ✅ CORE-005 基本登入、登出與工作區存取控制 → 勾選

| 允收面 | 證據（絕對路徑） |
| --- | --- |
| 登入 | `services/platform/internal/identity/github.go` — `GitHubOAuth.AuthURL`／`.Exchange`／`.FetchUser`；`identity/http.go` — `startLogin`／`finishLogin` |
| 離線 dev provider | `identity/http.go` — `devLogin`，僅在 `DEV_LOGIN=1` 掛載；測試 `TestDevLoginNotMountedByDefault` |
| 登出（伺服器端撤銷） | `identity/service.go` — `.Logout`；測試 `TestLoginLogoutSessionLifecycle` **實測「登出後重放舊 token 必須 401」** |
| Session 生命週期 | `db/migrations/0006_auth.sql`（`sessions.token_hash` 只存雜湊）；`.UserForToken`、`.CleanupExpiredSessions`；測試 `TestExpiredSessionRejectedThenCleaned` 另驗證**清理可重複執行**（鐵律 9） |
| 工作區存取控制 | `identity/service.go` — `.PersonalWorkspace`；`identity/http.go` — `.RequireSession`；`cmd/api/main.go:92-105` 所有私有路由皆包 `RequireSession` |
| CSRF／state | 測試 `TestAuthURLCarriesState`、`TestCallbackRejectsStateMismatch` |

**為何算「完全符合」**：`.github/workflows/ci.yml` 掛了 `pgvector/pgvector:pg17` service 並設定 `SKILLHUB_TEST_DATABASE_URL`，**上述整合測試在 CI 每次都真的跑**，不是「無 DB 就跳過而綠燈」。該 workflow 的註解自己寫明了這個設計理由。

#### ✅ CORE-006 使用者私有內容的授權檢查 → 勾選

| 允收面 | 證據 |
| --- | --- |
| 私有內容跨使用者隔離 | 測試 `TestPrivateContentIsolatedAcrossWorkspaces`：Bob 持有 Alice 的 skill id，對 `DELETE /skills/{id}`、`POST /skills/{id}/fork`、`GET /skills/{id}/diff` **全部回 404 而非 403**（存在性本身是私有的），且事後確認 Alice 的資料未被更動 |
| 不信任 UI 傳入的 `workspace_id`（鐵律 3） | 測試 `TestClientSuppliedWorkspaceIDIsIgnored`：query string／header 傳入他人 workspace 一律無效 |
| 匿名呼叫 | 同測試：匿名 `DELETE` 回 401，連授權檢查都到不了 |
| 公開面的 scope 邊界 | `db/queries/search.sql` 的 `JOIN workspaces w ON w.id = s.workspace_id AND w.is_catalog`；`db/migrations/0010_public_catalog.sql`（公開性必須明確授予）；測試 `TestPublicSearchSeesOnlyCatalogWorkspaces` |
| Fork 路徑 | 測試 `TestForkOfAnotherUsersPrivateSkillStillNotFound` |

### 3.2 其餘

| 項目 | 判定 | 證據／缺口 |
| --- | --- | --- |
| CORE-001～004 資料模型與不可變規則 | 先前已勾 | `db/migrations/0002`～`0005`；`0005_immutability.sql` 的 `enforce_immutable()` trigger 掛在 `skill_versions`／`test_case_snapshots`／`run_status_transitions`／`trace_events`，`runs` 另有終態後只允許改 cleanup 欄位的條件式 trigger；`db/tests/immutability_test.sql` |
| CORE-007 使用者資料與 Artifact 的刪除流程 | **未動** | 只有 Skill 的軟刪除（WS-005）。**`artifacts` 表自 `0004` 起存在但無任何 Go 讀寫**；使用者資料刪除不存在；硬清除 job 在 `registry.go` 以 `ponytail:` 註解明示延後。⚠️ 這同時是 **NFR-002／SEC-006** 的缺口 |
| CORE-008 重要操作的 Audit Event | **未動** | 全 repo **無 audit 表、無查詢、無型別、無路由**。現有 `slog` 只是營運日誌，不是可稽核事件 |

---

## 4. §6 Skill 匯入、驗證與索引（INGEST）

> 本節在對帳期間由 10 項增為 13 項，新增的 INGEST-011／012／013 見 §4.1。

INGEST-001～009 **先前已勾，本次逐項複核維持有效**，並確認 [import-report.md §6.1](import-report.md) 記錄的四個缺陷已由 `9ab422b` 處理：

| 項目 | 複核結果 | 關鍵證據 |
| --- | --- | --- |
| INGEST-001 URL 匯入 | 維持 | `ingest/fetch.go` — `DefaultAllowedHosts`、`URLFetcher.Fetch`，redirect 經 `CheckRedirect` 重新檢查；測試 `TestFetchRejectsDisallowedHost`、`TestFetchRedirectOffAllowListBlocked`、`TestFetchSizeCap` |
| INGEST-002 套件上傳 | 維持 | `ingest/service.go` — `MaxZipBytes`、`UploadZip`、`packageFS`；測試含 `TestPackageFSZipBomb` |
| INGEST-003 解析 SKILL.md 與檔案樹 | 維持 | `skillpkg/skillpkg.go` — `parseFrontmatter`、`scanTree` |
| INGEST-004 來源可追溯 | 維持（缺陷已修） | `persistVersion` → `CreateSkillSource(SourceUrl, SourceRef, ContentHash, FetchedAt)`；`fetch.go` 的 `commitSHA` regex **只把 40 碼 SHA 當成不可變 ref**，回應 import-report Bug 4 |
| INGEST-005 重複偵測 | 維持 | `GetVersionBySkillAndHash` ＋ `0003` 的 `(skill_id, content_hash)` unique index 雙保險；import-report 實測第二輪 44 筆全數 `duplicate` |
| INGEST-006 規格驗證 | 維持（缺陷已修） | `checkManifest`、`nameRule`、`checkFileReferences`；測試 `TestManifestLimitsCountRunesNotBytes` **正是 import-report Bug 1（位元組 vs 字元）的修復證據** |
| INGEST-007 靜態檢查 | 維持（範圍已擴大） | `scanTree`、`checkEmbeddedCode`＋`runnableFences`（**回應 import-report 失敗模式 Top-2「看不見 SKILL.md 內嵌程式」**）、`addURLDisclosures` 依 host 聚合（**回應 Top-3「816 條 URL 淹沒」**）；測試 `TestEmbeddedCodeIsDisclosed`、`TestURLDisclosuresAggregateByHost`、`TestSecretsBlockWithoutEchoingValue` |
| INGEST-008 錯誤／警告／資訊分開呈現 | 維持 | `Report.Categorize`、`CategorizedFindings`；測試 `TestCategorizeSeparatesBySeverity`、`TestCategorizeNeverNil` |
| INGEST-009 索引與重新索引 | 維持 | `ingest/enrich.go`、`cmd/reindex/main.go` 兩階段（prune＋全量重建 → `ReindexPending` backfill）；測試 `TestEnrichPackageFallsBackToPendingWhenEnrichFails` 等 5 個 |
| **INGEST-010 失效／來源更新／人工下架** | **未動** | 全 repo **無實作**。唯一沾邊的是 `enrichment_status='pending'`（索引新鮮度）與 `PruneDeletedSearchDocuments`。與 CONTENT-009 同一件事 |

### 4.1 對帳期間新增的三項（INGEST-011／012／013）

本對帳進行期間，另一位協作者於 `03-work-items.md` **新增並自行勾選** INGEST-011（內嵌可執行程式碼揭露，`02:SKILL-003`）、INGEST-012（License 多層溯源，`02:SKILL-004`／[ADR-021](../../../adr/ADR-021-skill-license-provenance.md)）、INGEST-013（外部 URL 依主機聚合，`02:SKILL-005`）。

**本對帳不重複判定這三項**，理由是它們的實作與遷移在對帳基準 `9ab422b` 之後、且**部分仍在工作樹**（`db/migrations/0012_license_provenance.sql` 當時尚未追蹤，`skillpkg.go`／`service.go`／`registry.go`／`db/queries/skill_versions.sql` 有未 commit 變更）。判定應由該協作者或下一次對帳承接。

**只留一筆事實連結**：這三項正對應 [import-report.md §4](import-report.md) 的三個失敗模式（Top-1 授權在 frontmatter 缺席 37／45、Top-2 掃描看不見內嵌程式與相依、Top-3 external-url 揭露被 816 條淹沒），也回答了該報告 §8 給 CONTENT-006 的第 1、2 個待決事項。**若 INGEST-012 確已落地，CONTENT-004 的「授權顯示事實來源」缺口即告關閉，但 §5.2 的法務判定仍獨立存在，不因此解除。**

---

## 5. §7 Skill Explorer（DISC）— M1 驗證閘門所在

> **本節為第二次對帳（2026-08-15，DISC-003 篩選器落地後）。** 第一次對帳時本節 10 項全部不勾，卡點不在後端而在 §5.1 列的三個結構性缺口。**三個缺口目前全部關閉**，本次逐項重驗後 **勾選 9 項，DISC-003 維持不勾**。判定標準與前次相同：允收準則逐條對照，且每條要有 repo 內可指名的落地證據（檔案＋符號／具名測試），測試須在 CI 真的會跑。

### 5.1 第一次對帳三個結構性缺口的現況

| 第一次對帳的缺口 | 現況 | 證據 |
| --- | --- | --- |
| 1. 前端打錯路由（走需登入的 `GET /skills/search`） | **已關閉** | `apps/web/src/api/skills.ts` `searchSkills` 打 `/api/skills/search`；測試 `DISC-001: search hits the public endpoint, which needs no session` **同時斷言不得打到不含 `/api/` 的那條** |
| 2. 三個前端呼叫的後端路由不存在 | **已關閉（兩條）** | `cmd/api/main.go:102-103` 掛上 `GET /api/skills/{id}`、`GET /api/skills/{id}/files`（皆 `OptionalSession`）；`catalog/detail.go` 實作。`GET /skills/{id}/versions` 仍不存在，但無允收準則要求它 |
| 3. 結構化篩選完全不存在 | **部分關閉** | `db/queries/search.sql` 兩個公開查詢都帶 `has_script`／`spec_validated` 述詞；`catalog/http.go` `searchFilters`／`parseFilters`；UI `FilterBar`。**六維度只做到兩個，見 5.3** |

### 5.2 逐項判定

| 項目 | 判定 | 允收準則對照與證據 |
| --- | --- | --- |
| **DISC-001** 自然語言意圖搜尋 | **✅ 勾選** | `02:DISC-001` 四條全滿足。①候選或清楚無結果狀態：`catalog/http.go` `PublicSearch` ＋ `Home.tsx` 的 `no_results` 區塊。②保留原始查詢：`searchResponse.Query` 回傳並由 `Home.tsx` 的 `.echoed-query` 原樣顯示；符合原因逐筆顯示並標示來源，測試 `DISC-002: each candidate shows its match reason, labelled by provenance`、Go `TestMatchReasonsAreLabelledByProvenance`。③空白／不可理解不建立搜尋並提示：`isComprehensible`，Go `TestBlankQueryReturnsNoResults`、web `DISC-005: no_results shows the server's query_suggestion, not hardcoded copy`。④未登入可搜尋與看公開 Skill：`main.go:96` 裸掛載搜尋、`:102-103` 以 `OptionalSession` 掛詳情與檔案樹，Go `TestAnonymousReadsCatalogSkillDetail`（無 cookie jar 的 client）、`TestPublicSearchSeesOnlyCatalogWorkspaces` |
| **DISC-002** 候選 Skill 與符合原因 | **✅ 勾選** | `02:DISC-002` 第 1、4 條。七欄（名稱、白話摘要、來源層級、Agent 相容狀態、依賴、風險提示、最近驗證時間）API 側 Go `TestSearchResultsCarryTheDISC002Columns`，**UI 側本次補上** `Home.tsx` `ResultFacets`，測試 `DISC-002: a result row carries all seven columns, and infers none of them`。「尚未試跑」標記：`badge-untested`，同一測試斷言。不推定：未掃描列顯示「尚無掃描紀錄」、空依賴顯示「未擷取到依賴資訊」，同一測試反向斷言 |
| **DISC-003** 六維度篩選 | **❌ 不勾（部分完成）**<br>→ **2026-08-15 允收準則修訂後改為 ✅ 勾選**，見 [§5.3.1](#531-建議已採納2026-08-15-規格修訂與-disc-003-勾選) | 見 5.3 |
| **DISC-004** 可解釋的排序規則 | **✅ 勾選** | `02:DISC-002` 第 3 條。「不得只使用 Star」：排序管線**完全不含**人氣或時間訊號，唯一排序權威是餘弦距離（`search.sql` `PublicHybridSearchSkills`，Go `TestFTSWidensCandidatesWithoutTakingOverRanking`、`TestZeroHitLegDoesNotParticipateInFusion`）。「排序依據需可被簡要說明」：`Home.tsx` `RankingExplainer` 以繁中列出六條規則含兩個例外，且**當下生效的例外會被標出**；測試 `DISC-004: the ranking rule is explained on demand and matches the pipeline`、`DISC-004: a degraded answer marks the exception that is actually in force`。本次另補「篩選條件不影響名次」一條，並由 Go `TestFiltersOnTheHybridPathRemoveRowsWithoutReranking` 實測（篩選後存活列的 rank 與篩選前逐位元相同） |
| **DISC-005** 無結果、低信心及查詢補充流程 | **✅ 勾選** | `02:DISC-001` 第 3 條與門檻行為。門檻 `MaxCosineDistance = 0.75`，**已依 golden-query-set.md §10.5 以真實增強語料重新推導**（12/12 離題拒答、0/48 召回損失），第一次對帳指出的「0.79 由裸 frontmatter 推導、已逾期」缺口因此關閉。四個空狀態旗標分離（`no_results`／`degraded`／`partial_index`／本次新增的 `filtered_out`），UI 四者各有不同文案且不共用。測試：Go `TestOffTopicQueryIsRefusedWithASuggestion`、`TestRelevantQueryIsNotRefusedByTheCutOff`、`TestPartialIndexIsReportedSeparatelyFromDegradation`、`TestFilteredToEmptyIsNotTheNoResultsRefusal`；web `DISC-005: degraded and partial_index are separate, non-blocking notices`、`DISC-003: filtered-to-empty and the no-results refusal never share copy` |
| **DISC-006** Skill 一般詳情頁 | **✅ 勾選** | `02:DISC-003` 第 1 條要求九項：功能、限制、輸入、輸出、依賴、權限、來源、License、相容性摘要。API 九項齊備（`catalog/detail.go` `skillDetail`）。**UI 本次前為 8／9——`limitations` 有回傳但 `SkillDetail.tsx` 從未渲染**；本次補上 `Limitations` 元件（模型與掃描兩種來源分別標示，ADR-013），並把 `enrichment` pending／空 bucket 的輸入／輸出／依賴由「整列消失」改為顯示「未知」（消失會被讀成「沒有依賴」）。測試 `DISC-006: the general detail view answers all nine required facts`、`DISC-006: an unenriched skill reads as unknown, never as 'needs nothing'` |
| **DISC-007** `SKILL.md` 與檔案樹進階檢視 | **✅ 勾選** | `02:DISC-003` 第 2 條。`GET /api/skills/{id}/files` 回傳 `SKILL.md` 全文（含 `skill_md_truncated` 上限旗標）與含 `is_script` 的檔案樹；`SkillFiles.tsx` 渲染全文與樹，Script 以 `.script-tag` 明確標示，並重複揭露 SKILL.md 內嵌程式碼（`SKILL-003`，檔案樹本質上看不到它）。一般／進階為兩條路由並雙向可回。測試 Go `TestFileTreeMarksScriptsAndOmitsDirectories`、`TestSkillFilesServesSkillMDAndMarksScripts`；web **本次補上** `DISC-007: advanced mode shows SKILL.md in full and marks every script`（斷言標記只落在腳本那一列） |
| **DISC-008** 來源／License／風險／相容狀態展示 | **✅ 勾選** | `02:DISC-003` 第 3、4 條與 `02:DISC-004` 第 3 條。①來源四項（URL、版本/Commit、擷取時間、內容雜湊）由 `ingest/service.go` `CreateSkillSource` 保存、`detail.go` `sourceFrom` 回傳、`SkillDetail.tsx` `SourceBlock` 顯示，Go `TestAnonymousReadsCatalogSkillDetail` 斷言內容雜湊。②License 未知時顯示「License 未知」並附「未宣告 License，依規則不可下載」，**不顯示任何 License 名稱、不暗示可自由修改或再散布**，測試 `DISC-008: unknown license never shows a name or implies permissiveness`、Go `TestLicenseKeepsProvenanceTierAndNeverConfirms`。③風險不給單一「安全」標章、警告在前提示彙總（`DISC-008: warnings are up front...`），無法讀取套件時報「未知」而非「乾淨」（`DISC-004: an unreadable package is reported as unknown, never as a clean scan`、Go `TestUnreadablePackageIsReportedAsUnknownNotClean`）。④相容三軸分開且沙箱兩軸明示未驗證（`TestSpecValidationIsSeparateFromRuntimeCompatibility`） |
| **DISC-009** 至少兩個 Skill 的靜態比較 | **✅ 勾選** | `02:DISC-004` 三條。①可選 2～3 個候選：`Home.tsx` 勾選框＋`CompareBar`，`Compare.tsx` `MAX_COMPARE = 3`，測試 `DISC-009: comparison needs two candidates and accepts at most three`（含第四個勾選被拒而非默默頂掉前一個）。②比較內容涵蓋能力、輸入輸出、依賴、權限、相容性、來源、License、驗證證據八項（`Compare.tsx` `ROWS`，另含限制、來源層級、風險揭露、版本與時間）。③缺資料顯示未知且不推定為通過：`.compare-unknown`，且「未知」在差異比對中是獨立值，不會與「有值」比成相同，測試 `DISC-009: the table highlights differing rows and never invents a missing one`。**本次修正**：`ROWS` 中渲染 `tier` 的那一列原本標成「類別」，與平台實際沒有類別資料互相矛盾，已改回「來源層級」 |
| **DISC-010** 公開不要求登入／私有要求登入 | **✅ 勾選** | 允收準則兩半皆成立。公開側：`main.go:96` 搜尋裸掛載、`:102-103` 詳情與檔案樹 `OptionalSession`（匿名得公開目錄，帶 session 者額外得自己的私有內容），Go `TestAnonymousReadsCatalogSkillDetail`、`TestSkillFilesServesSkillMDAndMarksScripts`（皆以無 cookie 的 client 請求）、web `DISC-001: search hits the public endpoint...`。私有側：`main.go` 的 import／list／fork／versions／diff／delete／takedown 全部包 `RequireSession`，Go `TestPrivateContentIsolatedAcrossWorkspaces`（匿名 `DELETE` 回 401）、`TestAnonymousCannotReadPrivateSkillDetail`（詳情與檔案樹對他人私有內容皆回 404，不是 403） |

### 5.3 DISC-003 為何不勾：六維度只落地兩個

`02:DISC-002` 的字面要求是「使用者可依**類別、來源層級、Agent、是否包含 Script、是否需要 MCP 與驗證狀態**篩選」。逐維度盤點平台實際有沒有逐筆資料：

| 維度 | 有無資料 | 處置 | 解除條件 |
| --- | --- | --- | --- |
| **是否包含 Script** | ✅ 有 | **已實作**。述詞讀 `search_documents.scan` 的 `script-file`／`embedded-script` 兩個 code（後者是 `SKILL-003` 的 SKILL.md 內嵌程式碼，使用者問「有沒有腳本」時兩者都算）。**無掃描紀錄的列 `yes`／`no` 都不匹配**——沒人掃過就不是「沒有腳本」，這正是 `02:DISC-004`「不得自行推定為通過」 | — |
| **驗證狀態** | ✅ 有 | **已實作**。`passed` ＝ 有已保存版本（`skillpkg.Validate` 遇 error 級發現即擋下匯入且不保存，所以「被索引」本身就是通過的證據）；無版本 ＝ `unverified`，永不報 `failed` | — |
| **類別** | ❌ 無 | 停用＋說明；API 回 400 | 三個 PDM-001 類別目前**只存在於 `tools/content/seed-skills.json` 的策展判斷**，沒有任何欄位保存它，匯入端點收的是套件不是類別，使用者自行匯入的 Skill 根本不會有類別。落地它屬 **CONTENT-003 策展工作**（需 migration＋匯入契約欄位＋匯入器改動），不屬搜尋工作 |
| **來源層級** | ⚠️ 有欄位但只有一個值 | 停用＋說明；API 回 400 | `resultFacets` 對每一列無條件給 `TierIndexed`，因為**沒有任何東西記錄人工精選審查**（`tier.go` 說明；被收錄不等於被審查）。全目錄同值時篩選分不出東西，且 tier 不在 SQL 裡，硬做只會是 `WHERE true`／`WHERE false`。待 CONTENT-003 的九項檢查表產出實際 curated 判定後重開 |
| **Agent 相容** | ❌ 無 | 停用＋說明；API 回 400 | `capability`／`runtime` 兩軸在 M2 Sandbox 之前一律 `unverified`。提供篩選等於暗示有人做過判定 |
| **是否需要 MCP** | ❌ 無 | 停用＋說明；API 回 400 | 靜態掃描與 manifest **都沒有捕捉任何 MCP 訊號**，遠端 MCP 也已移出 MVP 首發（AGENTS.md 範圍注意）。這是四項裡唯一連資料來源都還沒定義的 |

**四個無資料維度的處置原則**：不隱藏、不假裝。UI 以停用的控制項＋逐項理由呈現（`Home.tsx` `UNAVAILABLE_FILTERS`），API 對這四個參數回 **400 並附理由**（`catalog/http.go` `unavailableFilters`）。理由是**默默忽略比拒絕更糟**——一個帶 `?category=documents` 的分享連結若回傳整個目錄，使用者會把整個目錄當成篩選後的子集。測試 `TestFilterDimensionsWithoutDataAreRejectedNotIgnored`（六個壞參數逐一斷言 400）、web `DISC-003: the filters the platform has no data for are disabled and say why`。

**已落地部分的測試**：Go `TestFiltersNarrowOnRealEvidenceIncludingTheDegradedPath`（單維度、組合、無掃描列兩邊都不匹配，且**跑在降級 FTS 路徑上**——降級已經是低召回，再默默不套使用者的篩選就是對頁面內容說謊）、`TestFiltersOnTheHybridPathRemoveRowsWithoutReranking`、`TestFilteredToEmptyIsNotTheNoResultsRefusal`；web `DISC-003: a chosen filter reaches the request and the shareable URL`（含 URL 可分享與清除維度時參數消失）。

**殘餘缺口（DISC-003 勾選前必須關閉）**：

1. 類別維度：需先由 CONTENT-003 決定類別是否成為平台欄位，以及使用者自行匯入的 Skill 如何取得類別。
2. 來源層級維度：需 CONTENT-003 的人工精選審查產出至少兩種 tier 值。
3. Agent 相容維度：需 M2 Sandbox 試跑結果。**M1 內結構性不可能完成。**
4. MCP 維度：需先定義訊號來源（掃描 manifest？SKILL.md？），且遠端 MCP 已移出 MVP 首發。

> **建議**：第 3、4 項在 M1 內不可能收斂。若要讓 DISC-003 有機會在 M1 結束前勾選，應**先修改 `02:DISC-002` 的允收準則**，把 Agent 與 MCP 兩個維度標成 M2／後 MVP，再由該修改驅動本項——而不是靠降低證據標準勾選。依 AGENTS.md 文件維護規則，改允收準則要三文件同步。

### 5.3.1 建議已採納：2026-08-15 規格修訂與 DISC-003 勾選

負責人授權採納 §5.3 與 §8 第三梯的兩項建議，規格修訂已於 **2026-08-15** 落地（只改 `plans/`，未動任何程式碼、ADR 或 CI）：

| 修訂 | 落點 | 內容 |
| --- | --- | --- |
| 篩選維度分階段允收 | `02:DISC-002` | 新增「篩選維度的允收階段」表：Script／驗證狀態＝**M1**；類別／來源層級＝**依 CONTENT-003 策展資料啟用**；Agent 相容＝**M2（依 Sandbox 實測）**；是否需要 MCP＝**後 MVP（隨 MCP 啟動）**。原「六維度」準則原文保留未刪 |
| 未達階段維度的處置入規格 | `02:DISC-002` | 新增一條允收準則：未達允收階段的維度不得隱藏或默默忽略，UI 停用＋理由、API 回 400 附理由（把 §5.3 已落地且已測的行為寫成準則，非新要求） |
| CONTENT-007／008 改列 M2 | `03` §4、`01` M1／M2 里程碑 | 依賴 M2 的 Test Case 與 Sandbox，M1 內結構性不可能完成 |
| CONTENT-003 勾選條件解綁 | `03` CONTENT-003、[curated-skill-list.md](curated-skill-list.md) §3 | ⑧ 不再是勾選前提，改為引用 CONTENT-007／008 並須記為「待 M2」；⑤⑦ 對 CONTENT-006／005 的綁定不變 |

**勾選變化：`DISC-003` → `[x]`。** 依修訂後的 `02:DISC-002` 逐條對照，M1 階段的兩個維度與新增的處置準則全部有 §5.3 已列的落地證據（Go `TestFiltersNarrowOnRealEvidenceIncludingTheDegradedPath`、`TestFiltersOnTheHybridPathRemoveRowsWithoutReranking`、`TestFilteredToEmptyIsNotTheNoResultsRefusal`、`TestFilterDimensionsWithoutDataAreRejectedNotIgnored`；web `DISC-003: a chosen filter reaches the request and the shareable URL`、`DISC-003: the filters the platform has no data for are disabled and say why`、`DISC-003: filtered-to-empty and the no-results refusal never share copy`），且測試在 CI 帶 Postgres service 實際執行。**證據標準未降低，改變的是允收範圍。** §5.2 表中 DISC-003 的原判定「❌ 不勾（部分完成）」是 2026-08-15 修訂前的判定，保留供回溯。

**未連動勾選：`CONTENT-003` 維持 `[ ]`。** 解綁 ⑧ 之後，⑤（CONTENT-006 未動）與 ⑦（CONTENT-005 未動）兩項檢查對全部 45 筆仍為 `pending`，另 §5.2 的 source-available 法務判定仍獨立存在。**CONTENT-004** 亦維持 `[ ]`（法務判定未完成），本次修訂未觸及其允收準則。

### 5.4 殘餘風險（不影響勾選，但應記錄）

1. ~~**整合測試的路由是抄的，不是 `main.go` 的。**~~ **已修復（commit `9cb1371`）。** 路由表抽成 `internal/apiserver.NewRouter(Deps)`；`cmd/api/main.go` 只組 deps 呼叫它，整合測試（`newAPI`）也呼叫同一個函式，`newGovernanceAPI` 這份重抄的 mux 已刪除。抽取前後方法＋路徑＋middleware 逐條相同（20 條，含 `auth.Mount` 的 7 條）。新增 `TestAnonymousCallersGetThePublicSurfaceAndNothingElse` 逐條走完整張表斷言匿名者的結果，補上先前零覆蓋的 `POST /skills/import/url`、`POST /skills/import/upload`、`POST /skills/{id}/versions`、`GET /healthz`、`GET /auth/github/*`、`POST /auth/logout`。
   附帶查明：原本擔心的「掉一條 `RequireSession` 測試不會紅」其實部分被 handler 自身接住——每個私有 handler 都會再驗一次 `SessionUser` 並同樣回 401（實測把 `import/url` 的 `RequireSession` 拿掉，邊界仍成立）。這是刻意的縱深防禦；測試斷言的是邊界結果，不是實施它的機制。
2. **Fork 血緣的詳情頁渲染沒有測試。** `detail.go` 回傳 `derivation`、`SkillDetail.tsx` 也渲染了原始 Skill 連結與分岔版本 ID，但沒有任何測試斷言**詳情回應**帶 derivation；`TestForkCatalogSkillIntoCallerWorkspace` 驗的是 fork 端點的回應。這是 `02:DISC-003` 第 5 條，行為正確但無回歸保護。
3. **篩選述詞跑在候選視窗之後。** 混合檢索兩腿各取 50 筆候選後才套篩選，目錄 45 筆時無人不可達；目錄長大後需把述詞下推到兩個 CTE（`search.sql` 已留 `ponytail:` 註記）。
4. **`02:DISC-003` 的字面用詞是「授權未知」，程式顯示的是「License 未知」。** 語意等價且更精確（同一畫面另有 License 名稱與溯源層級兩軸），但字面不同。若要求逐字一致，改的是 `trust.go` 的一個字串。

---

## 6. §8 Fork、版本與工作區（WS）

| 項目 | 判定 | 證據／缺口 |
| --- | --- | --- |
| WS-001 Fork 並保留來源與 License 關係 | 先前已勾 | `registry/registry.go` `.Fork`；`skills.forked_from_skill_id`／`forked_from_version_id`（`0003`）；測試 `TestForkCatalogSkillIntoCallerWorkspace`、`TestForkOfAnotherUsersPrivateSkillStillNotFound` |
| WS-002 不可變版本保存 | 先前已勾 | `ingest/service.go` `.SaveVersion`；**強制點在 DB 而非 Go**：`0005_immutability.sql` 的 `enforce_immutable()` trigger；`db/tests/immutability_test.sql` |
| WS-003 任兩版本差異比較 | 先前已勾 | `registry/diff.go` `.DiffVersions`、`FileDiff`；`GET /skills/{id}/diff?from=&to=`；兩個 version id 都被限制在 `ws.ID` 且必須屬於同一 `skillID`；測試 `TestDiffFS` |
| **WS-004 個人 Skill／Test Case／Run／下載紀錄列表** | **部分完成，不勾** | 只有 Skill：`registry/http.go` `.List` → `ListSkills`（且 Limit 100／Offset 0 寫死，無分頁）。**Test Case、Run、下載紀錄三者：無路由、無查詢、無 Go 程式**——`test_cases`／`runs`／`artifacts` 表自 `0004` 起存在、sqlc 也產了 model，但沒有任何讀取端。commit `51da7ba` 自己標的就是 "WS-004 partial" |
| WS-005 私有內容刪除與狀態回饋 | 先前已勾 | `registry.go` `.Delete`（軟刪除＋`DeleteSearchDocument`＋`CountSkillVersions` 同一交易）；回應含 `versions_retained` 與說明刪除範圍的 `note`（對應 `02:WS-002`「系統應說明刪除範圍」） |
| **✅ WS-006 驗證不同使用者無法存取彼此私有內容** | **完全符合 → 勾選** | 與 CORE-006 同一組證據（§3.1）。`02:WS-002`「使用者只能存取自己的私有內容」由四個具名測試覆蓋：`TestPrivateContentIsolatedAcrossWorkspaces`（列表／刪除／Fork／Diff 四條路徑皆 404）、`TestClientSuppliedWorkspaceIDIsIgnored`、`TestForkOfAnotherUsersPrivateSkillStillNotFound`、`TestPublicSearchSeesOnlyCatalogWorkspaces`；且 CI 帶 Postgres service，**測試確實執行** |

> **為何 WS-006 可勾而 WS-004 不可**：WS-006 的允收準則是一條**否定性的安全斷言**（「無法存取」），驗證它需要的就是測試本身，測試存在且會跑，項目即完成。WS-004 的允收準則是一份**清單**（Skill、Test Case、Run、下載紀錄），四項只到一項。

---

## 7. M1 驗證閘門狀態

M1 驗證閘門的敘述是「**使用者能以自然語言找到相關 Skill**」，其形式是**使用者測試**。

### 7.1 三個前置條件的現況

| 前置 | 狀態 | 依據 |
| --- | --- | --- |
| **量化前置（檢索品質）** | ✅ **已過** | [golden-query-set.md §5](golden-query-set.md)：60 條查詢／31 份語料／13 repo，向量腿 Top-3 96%、recall@5 96%，繁中跨語言 recall@5 93%；干擾查詢與正解的相似度分布**完全不重疊**，門檻 B 可做到 100% 拒答／0% 召回損失 |
| **索引管線就緒** | ✅ **已就緒** | [import-report.md §3](import-report.md)：44/45 匯入成功、`enrichment_status='enriched'` 44/44、embedding 44/44、`task_examples` 44/44；`cmd/reindex` backfill 冪等（`enriched=0 still_pending=0`）。§5.2 的 smoke 顯示中文文件類由「全滅」變 4/4 Top-1 |
| **使用者測試** | ❌ **未執行；UI 阻擋已解除，剩 CONTENT-005 一項前置** | 需真人。第一次對帳列的兩條 UI 阻擋（看不到符合原因與補充建議、點進詳情會壞掉）**已於第二次對帳確認關閉**，見 §5.1／§5.2；剩下的前置是 CONTENT-005 |

### 7.2 為什麼現在還不能開始使用者測試

golden-query-set.md §5 已經給過一條前提：**使用者測試應排在 CONTENT-005（白話摘要）與索引時增強管線就緒之後**，否則 `documents` 類別的結論不具代表性。索引時增強已就緒（import-report §3），但**CONTENT-005 未動**——目前目錄裡有 5 個簡體中文 Skill（`data-analyst` 與 YuYY2004 系列）尚未改寫為繁體中文。

第一次對帳在此之上列了兩條更硬的 UI 阻擋，**兩條在第二次對帳都已關閉**，此處保留紀錄：

1. ~~受測者在 UI 上看不到符合原因、也看不到查詢補充建議~~ → 已關閉：符合原因逐筆顯示並標示模型／規則來源，查詢補充建議直接取用伺服器文案（§5.2 的 DISC-001／002／005）。
2. ~~點進搜尋結果會壞掉——詳情頁呼叫的後端路由不存在~~ → 已關閉：`GET /api/skills/{id}` 與 `/files` 已掛載且免登入，「搜尋→詳情→進階檢視」整條旅程可走完（§5.1-2、DISC-006／007／010）。

**因此使用者測試目前唯一的實質前置是 CONTENT-005。** 附帶提醒：受測者現在會看到一列有四個停用篩選器的篩選列（DISC-003，§5.3），這是刻意的誠實呈現，但**它本身值得在使用者測試裡量一下**——受測者是否把「無法篩選」讀成「壞掉了」。

### 7.3 建議的使用者測試流程（待前置解除後執行）

| 項目 | 建議 | 理由 |
| --- | --- | --- |
| **人數** | **6～8 位**個人創作者；若要按三個類別分層則 **9 位**（每類 3 位） | 品質性可用性問題在 5～6 位受測者後即飽和；取 6～8 是為了容納「繁中／英文」與「明說格式／口語模糊」兩個維度。**這不取代 PDM-009 的封測人數決策**（該項屬 M4，仍未定） |
| **任務設計** | 直接取 [`tools/goldenset/queries.json`](../../../tools/goldenset/queries.json) 的**情境**改寫成任務卡，**但不給受測者原句**——讓他們自己打字 | golden set 量的是「原句直接進檢索」的降級路徑；使用者測試要量的是**真人會怎麼問**。兩者的落差本身就是最有價值的產出 |
| **必含題型** | 每人至少 1 條**口語模糊**（不含技術詞）與 1 條**干擾查詢**（離題） | 口語模糊是 golden set Top-1 最弱的一格（`documents`／繁中 30%）；干擾查詢驗的是 DISC-005 的門檻在真人面前是否可信 |
| **成功判準** | 主判準＝**受測者能在 Top-5 內自行指認出可用的 Skill 並說出理由**；次判準＝干擾查詢時受測者認同「沒有結果」是正確回應 | 對齊閘門敘述「能找到」。**不要用 recall 當使用者測試判準**——那已由 golden set 量過了 |
| **同時要量的** | 受測者是否需要看符合原因才敢點進去；排序理由是否需要解釋 | 直接回答 DISC-002／004 目前的兩個缺口 |
| **回歸把關** | 每次動語料或索引管線就跑 `python tools/goldenset/evaluate.py --selfcheck`（零成本、零網路） | golden-query-set.md §8.1 的建議；目前**尚未接進 CI** |

> ⚠️ **門檻會過期。** golden-query-set.md §4.4 明列三個必須重測的觸發條件。第一次對帳指出第 2 條「索引欄位換成真實的 LLM 增強產出」已經發生，而當時線上的 `MaxCosineDistance = 0.79` 是在裸 frontmatter 語料上推導的、嚴格說已逾期。
>
> **這個缺口已關閉**：目前線上值是 `0.75`，由 golden-query-set.md §10.5 在**真實增強語料**上重新推導（12/12 離題拒答、0/48 召回損失），`catalog/http.go` 的常數註解記載了推導過程與三個重測觸發條件。**其中第 1 條（語料成長一個數量級）與第 3 條（查詢改寫）仍然有效，仍需在觸發時重測。**

---

## 8. M1 未完成項的收斂清單與建議順序

依「解除下游阻擋的數量」排序，不依工作量。

### 第一梯：解除 M1 驗證閘門（做完才談閘門）

| 順序 | 項目 | 為什麼是這個順位 |
| --- | --- | --- |
| ~~1~~ | ~~補後端 `GET /skills/{id}` 與 `/skills/{id}/files`~~ | ✅ **已完成**（`main.go:102-103`、`catalog/detail.go`）。解除了 DISC-006／007／008／010 |
| ~~2~~ | ~~前端改打 `/api/skills/search` 並補齊回應欄位~~ | ✅ **已完成**（`api/skills.ts`、`api/types.ts`、`Home.tsx`）。解除了 DISC-001／002／005 的 UI 側 |
| **1** | **CONTENT-005 白話摘要**（含 5 個簡體中文 Skill 的繁中改寫） | golden-query-set.md §5 指名的使用者測試前置。**現在是使用者測試唯一剩下的實質前置** |
| ~~4~~ | ~~以增強後語料重跑門檻推導~~ | ✅ **已完成**：線上值 `MaxCosineDistance = 0.75`，由 golden-query-set.md §10.5 在真實增強語料上推導 |

### 第二梯：M1 節內仍缺的實作

| 順序 | 項目 | 備註 |
| --- | --- | --- |
| ~~2~~ | ~~DISC-003 的四個無資料維度~~ | ✅ **已依 §5.3.1 處置（2026-08-15）**：`02:DISC-002` 改為分階段允收，DISC-003 已勾選。四個維度本身仍未啟用，解除條件改由 CONTENT-003（類別／來源層級）、M2 Sandbox（Agent 相容）、後 MVP（MCP）承接，不再掛在 DISC-003 底下 |
| ~~6~~ | ~~DISC-004 的排序說明~~ | ✅ **已完成**：`Home.tsx` `RankingExplainer`，兩個具名 web 測試 |
| ~~7~~ | ~~DISC-009 兩個 Skill 的靜態比較~~ | ✅ **已完成**：`Compare.tsx`（比較在前端組合三次詳情查詢，未新增端點，因此 `registry/diff.go` 確實沒有被沿用） |
| 8 | **WS-004 補齊 Test Case／Run／下載紀錄列表** | ⚠️ 這三者的**寫入端屬 M2**。M1 內合理的收斂是「列表為空時的正確空狀態」，或**明確把本項改列 M2** |
| 9 | **INGEST-010 ＋ CONTENT-009**（失效／來源更新／人工下架） | 同一件事的兩面，應一起做。curated-skill-list.md §7 建議 4 已指名 `nqumich/data-analyst-skill` 或 `cabbagecachekid/neon-jetpack` 作為實際演練對象 |
| 10 | **CORE-007 刪除流程**、**CORE-008 Audit Event** | 兩者都零實作，且都是 **NFR-002／SEC-005／SEC-006 的前置**。`cmd/worker` 目前是 21 行空殼，沒有 River、沒有任何 job——刪除保留期、session 清理、增強重試三件事目前全靠手動 |

### 第三梯：M1 內結構性不可能完成，建議正式移出

| 項目 | 理由 | 建議 |
| --- | --- | --- |
| ~~**CONTENT-007／008**（範例資料、驗收條件、基準試跑）~~ | 依賴 M2 的 Test Case 與 Sandbox | ✅ **已處置（2026-08-15）**：`03` 兩項已附註 M2、`01` 里程碑已同步、CONTENT-003 的 ⑧ 綁定已解除，見 §5.3.1 |
| **CONTENT-003／004 的最終勾選** | 分別卡在 CONTENT-005／006（⑧ 已於 2026-08-15 解綁）與 §5.2 法務判定 | 法務判定是**唯一不需寫任何程式就能推進的阻塞項**，建議獨立追蹤、儘早發動 |

### 一句話總結

第一次對帳的結論是「後端做得比帳面深，前端與後端之間差一層接線」。**那層接線已經接完**：§7 DISC 十項中九項通過逐條允收準則驗證並勾選。

**M1 剩下的硬缺口只有三處，而且沒有一處是 Explorer 的程式問題：CONTENT-005 白話摘要（使用者測試的唯一實質前置）、DISC-003 四個無資料維度所依賴的 CONTENT-003 策展產出，以及完全沒動的稽核與刪除（CORE-007／008）。** 驗證閘門現在卡在「還沒找真人做使用者測試」，而不再是「UI 做不完整所以不能做測試」。
