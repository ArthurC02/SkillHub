# M1 工作項對帳（`03-work-items.md` 第 4～8 節）

- 日期：**2026-08-15**
- 基準 commit：**`9ab422b`**（`Fix the four M1 import-report platform bugs and widen CONTENT-006 scanning`）
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
| §7 DISC | 10 | 0 | 0 | 3 | 4 | 3 |
| §8 WS | 6 | **1** | 4 | 0 | 0 | 1 |
| **合計** | **46** | **3** | **22** | **5** | **4** | **12** |

**本次新勾三項：`CORE-005`、`CORE-006`、`WS-006`。** 三項的共同特徵是——允收準則本身就是一條可測的行為斷言，而該斷言有具名測試，且該測試在 CI 裡**確實會執行**（見 §3.1 的 CI 佐證）。其餘所有「看起來已完成」的項目，卡點都不在後端程式，而在**UI 未接線**或**允收準則涵蓋的範圍比已落地的部分更寬**。

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

**本節 10 項全部不勾。** 這不是後端沒做，而是**允收準則寫在使用者可見的行為上**，而 UI 與後端之間有一道明確的接線缺口。

### 5.1 全節共通的三個結構性缺口

1. **前端打錯路由。** `apps/web/src/pages/Home.tsx` 走 `useSkillSearch` → `GET /skills/search`（`cmd/api/main.go:98`，包 `RequireSession`），而公開搜尋是 `GET /api/skills/search`（第 96 行，**刻意不包 session**）。因此 **DISC-001 的「使用者可在未登入狀態完成搜尋」在 UI 上不成立**，儘管後端已備妥。
2. **三個前端呼叫的後端路由不存在**：`GET /skills/{id}`（詳情）、`/skills/{id}/files`（檔案樹）、`/skills/{id}/versions`。`apps/web/src/api/skills.ts` 的檔頭註解自己寫明了這件事，`contracts/openapi/public.yaml` 的 `/skills/{id}` 也只有 DELETE。
3. **結構化篩選完全不存在。** `PublicSearch` 沒有任何 filter 參數，SQL 沒有對應述詞。

### 5.2 逐項

| 項目 | 判定 | 已落地的部分 | 未達允收準則之處 |
| --- | --- | --- | --- |
| DISC-001 自然語言意圖搜尋 | **部分完成** | 後端 `catalog/http.go` `PublicSearch` 完整：向量＋FTS、保留原始查詢（`searchResponse.Query`）、空白／不可理解查詢不建立搜尋（`isComprehensible`，測試 `TestBlankQueryReturnsNoResults`）。前端有搜尋框與結果列表 | AC「未登入可完成搜尋」**UI 未達成**（§5.1-1）。AC「顯示每個候選與需求相符的原因」未達成：`api/types.ts` 的 `SearchHit` **沒有 `match_reason` 欄位**，Home.tsx 也沒渲染 |
| DISC-002 候選 Skill 與符合原因 | **部分完成** | `applyMatchReasons`、`templateMatchReason`、`overlapTerms`；供應來源**逐候選**標記 `model`／`template`（測試 `TestMatchReasonsAreLabelledByProvenance`，回應 import-report Bug 3） | `02:DISC-002` 要求每個結果至少顯示**名稱、白話摘要、來源層級、Agent 相容狀態、依賴、風險提示、最近驗證時間**七項；Home.tsx 只渲染名稱與摘要兩項。「尚未試跑」標記未出現在結果列 |
| DISC-003 篩選（類別／來源／Agent／Script／MCP／驗證狀態） | **未動** | — | 六種篩選**一種都沒有**。`enrich.go` 的註解已載明結構化篩選將讀 manifest 而非索引投影，方向有了，程式沒有 |
| DISC-004 可解釋的排序規則 | **部分完成** | 排序規則本身已定案並落地：向量餘弦距離為唯一排序權威，`rank = 1 - COALESCE(distance,1)`，FTS 腿只擴大候選集不搶排序（`db/queries/search.sql` 的註解引 golden-query-set.md §3.7；測試 `TestFTSWidensCandidatesWithoutTakingOverRanking`、`TestZeroHitLegDoesNotParticipateInFusion`）。**AC「不得只使用 Star」確定滿足** | AC「排序依據需**可被簡要說明**」是對使用者的說明義務；目前排序理由只存在於 SQL 註解與本 repo 文件，**API 回應與 UI 都沒有任何排序說明** |
| DISC-005 無結果、低信心及查詢補充流程 | **部分完成** | **後端完整落地，且門檻確實來自 golden set**：`MaxCosineDistance = 0.79`（`catalog/http.go:73`）＝ golden-query-set.md §4.3 建議值 B「餘弦相似度 ≥ 0.21」的等價距離。`NoResults`／`QuerySuggestion`／`PartialIndex` 三個欄位分離；測試 `TestOffTopicQueryIsRefusedWithASuggestion`、`TestRelevantQueryIsNotRefusedByTheCutOff`、`TestPartialIndexIsReportedSeparatelyFromDegradation`、`TestBlankQueryReturnsNoResults` | **UI 未接線**：Home.tsx 第 31-33 行以 `results.length === 0` 自行判斷，渲染一句寫死的中文，**完全沒有讀 `no_results` 與 `query_suggestion`**。`SearchHit` 型別也沒有這些欄位。使用者因此看不到後端算出的補充建議，「低信心」與「查無結果」在畫面上無法區分 |
| DISC-006 Skill 一般詳情頁 | **進行中，不勾** | `apps/web/src/pages/SkillDetail.tsx` 已寫（badges、fork 血緣、功能／限制／輸入／輸出／依賴／權限／來源＋License／相容性／風險各區塊） | 後端 `GET /skills/{id}` **不存在**，`api/types.ts` 分隔線以下的型別全是推測。且 `apps/web` 有另一位協作者的未 commit 變更 |
| DISC-007 `SKILL.md` 與檔案樹進階檢視 | **進行中，不勾** | `apps/web/src/pages/SkillFiles.tsx` 已寫（一般／進階切換、`SKILL.md` 全文、檔案樹標示 `is_script`） | 後端 `GET /skills/{id}/files` **不存在**。同上 |
| DISC-008 來源／License／風險／相容狀態展示 | **進行中，不勾** | 四個元件齊備：`TrustBadge`（來源未知／可追溯／已人工確認）、`LicenseBadge`（**未知時渲染「授權未知」並隱藏名稱**，正面回應 `02:DISC-003`）、`RiskIndicator`（腳本／外部 URL／疑似 Secret 三項分列，**刻意不給單一「安全」標章**）、`CompatibilityStatus` | 後端 `catalog/trust.go`、`catalog/tier.go` **沒有任何 handler 引用**，沒有 API 供應這些資料。元件目前無真實資料來源 |
| DISC-009 至少兩個 Skill 的靜態比較 | **未動** | — | **確認未落地。** 唯一的比較設施是 `registry/diff.go`，而它 `diff.go:37` 明確拒絕跨 Skill：`v.SkillID != skillID` 即回 `ErrNotFound`——它比的是**同一個 Skill 的兩個版本**（WS-003），與 `02:DISC-004`「選擇至少兩個候選 Skill 進行靜態比較」是不同的東西。`02:DISC-004` 另要求缺資料欄位顯示未知、不得推定為通過，亦無實作 |
| DISC-010 公開不要求登入／私有要求登入 | **部分完成** | 公開搜尋確實免登入（`cmd/api/main.go:96` 裸掛載），scope 純由 SQL 的 `is_catalog` 把關，測試 `TestPublicSearchSeesOnlyCatalogWorkspaces`；私有側全部包 `RequireSession`，測試 `TestPrivateContentIsolatedAcrossWorkspaces` 驗證匿名 401 | AC 是「公開搜尋**與詳情**不要求登入」。**公開詳情端點根本不存在**，無從驗證。另 `identity.OptionalSession` 已實作且有單元測試，卻**未掛到任何路由** |

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
| **使用者測試** | ❌ **未執行，且目前不具備執行條件** | 需真人；且 §5 列出的 UI 接線缺口會讓測試量到「缺工序的中間態」 |

### 7.2 為什麼現在還不能開始使用者測試

golden-query-set.md §5 已經給過一條前提：**使用者測試應排在 CONTENT-005（白話摘要）與索引時增強管線就緒之後**，否則 `documents` 類別的結論不具代表性。索引時增強已就緒（import-report §3），但**CONTENT-005 未動**——目前目錄裡有 5 個簡體中文 Skill（`data-analyst` 與 YuYY2004 系列）尚未改寫為繁體中文。

在此之上，本次對帳新增兩條更硬的阻擋：

1. **受測者在 UI 上看不到符合原因、也看不到查詢補充建議**（§5.2 的 DISC-001／002／005）。閘門要驗的正是「找得到」，而使用者判斷「找到的對不對」靠的就是符合原因。
2. **點進搜尋結果會壞掉**——詳情頁呼叫的後端路由不存在（§5.1-2）。受測者無法完成「搜尋→詳情」這段旅程。

### 7.3 建議的使用者測試流程（待前置解除後執行）

| 項目 | 建議 | 理由 |
| --- | --- | --- |
| **人數** | **6～8 位**個人創作者；若要按三個類別分層則 **9 位**（每類 3 位） | 品質性可用性問題在 5～6 位受測者後即飽和；取 6～8 是為了容納「繁中／英文」與「明說格式／口語模糊」兩個維度。**這不取代 PDM-009 的封測人數決策**（該項屬 M4，仍未定） |
| **任務設計** | 直接取 [`tools/goldenset/queries.json`](../../../tools/goldenset/queries.json) 的**情境**改寫成任務卡，**但不給受測者原句**——讓他們自己打字 | golden set 量的是「原句直接進檢索」的降級路徑；使用者測試要量的是**真人會怎麼問**。兩者的落差本身就是最有價值的產出 |
| **必含題型** | 每人至少 1 條**口語模糊**（不含技術詞）與 1 條**干擾查詢**（離題） | 口語模糊是 golden set Top-1 最弱的一格（`documents`／繁中 30%）；干擾查詢驗的是 DISC-005 的門檻在真人面前是否可信 |
| **成功判準** | 主判準＝**受測者能在 Top-5 內自行指認出可用的 Skill 並說出理由**；次判準＝干擾查詢時受測者認同「沒有結果」是正確回應 | 對齊閘門敘述「能找到」。**不要用 recall 當使用者測試判準**——那已由 golden set 量過了 |
| **同時要量的** | 受測者是否需要看符合原因才敢點進去；排序理由是否需要解釋 | 直接回答 DISC-002／004 目前的兩個缺口 |
| **回歸把關** | 每次動語料或索引管線就跑 `python tools/goldenset/evaluate.py --selfcheck`（零成本、零網路） | golden-query-set.md §8.1 的建議；目前**尚未接進 CI** |

> ⚠️ **門檻會過期。** golden-query-set.md §4.4 明列三個必須重測的觸發條件，其中第 2 條「索引欄位換成真實的 LLM 增強產出」**已經發生**（import-report §3 證實 44/44 已 enriched）。目前線上的 `MaxCosineDistance = 0.79` 是在**裸 frontmatter** 語料上推導的，**嚴格說已經逾期，應在使用者測試前以增強後的語料重跑一次門檻推導**。這是本次對帳發現的、尚未被任何工作項承接的缺口。

---

## 8. M1 未完成項的收斂清單與建議順序

依「解除下游阻擋的數量」排序，不依工作量。

### 第一梯：解除 M1 驗證閘門（做完才談閘門）

| 順序 | 項目 | 為什麼是這個順位 |
| --- | --- | --- |
| 1 | **補後端 `GET /skills/{id}` 與 `/skills/{id}/files`** | 一次解除 **DISC-006／007／008／010** 四項。前端三個頁面與四個元件都已寫好，卡的只是沒有端點。這是全 M1 投報比最高的一步 |
| 2 | **前端改打 `/api/skills/search`，並在 `SearchHit` 補 `match_reason`／`no_results`／`query_suggestion`／`degraded`／`partial_index`** | 一次解除 **DISC-001／002／005** 的 UI 側。後端全部備妥，純接線 |
| 3 | **CONTENT-005 白話摘要**（含 5 個簡體中文 Skill 的繁中改寫） | golden-query-set.md §5 指名的使用者測試前置 |
| 4 | **以增強後語料重跑門檻推導** | §7.3 的逾期問題。做在使用者測試前，否則測到的門檻不是上線的門檻 |

### 第二梯：M1 節內仍缺的實作

| 順序 | 項目 | 備註 |
| --- | --- | --- |
| 5 | **DISC-003 結構化篩選** | 六種篩選零實作。需先決定資料來源（manifest vs 索引投影），`enrich.go` 註解已指向前者 |
| 6 | **DISC-004 的排序說明** | 規則已定案且已落地，缺的只是把理由送到 API 回應與 UI。可與第 2 項合併做 |
| 7 | **DISC-009 兩個 Skill 的靜態比較** | 需新端點；`registry/diff.go` **不可沿用**（它按設計拒絕跨 Skill） |
| 8 | **WS-004 補齊 Test Case／Run／下載紀錄列表** | ⚠️ 這三者的**寫入端屬 M2**。M1 內合理的收斂是「列表為空時的正確空狀態」，或**明確把本項改列 M2** |
| 9 | **INGEST-010 ＋ CONTENT-009**（失效／來源更新／人工下架） | 同一件事的兩面，應一起做。curated-skill-list.md §7 建議 4 已指名 `nqumich/data-analyst-skill` 或 `cabbagecachekid/neon-jetpack` 作為實際演練對象 |
| 10 | **CORE-007 刪除流程**、**CORE-008 Audit Event** | 兩者都零實作，且都是 **NFR-002／SEC-005／SEC-006 的前置**。`cmd/worker` 目前是 21 行空殼，沒有 River、沒有任何 job——刪除保留期、session 清理、增強重試三件事目前全靠手動 |

### 第三梯：M1 內結構性不可能完成，建議正式移出

| 項目 | 理由 | 建議 |
| --- | --- | --- |
| **CONTENT-007／008**（範例資料、驗收條件、基準試跑） | 依賴 M2 的 Test Case 與 Sandbox | **建議在 `03-work-items.md` 標註里程碑為 M2**，否則 CONTENT-003 的「九項全過」在 M1 內永遠無法收斂 |
| **CONTENT-003／004 的最終勾選** | 分別卡在 CONTENT-005/006/007/008 與 §5.2 法務判定 | 法務判定是**唯一不需寫任何程式就能推進的阻塞項**，建議獨立追蹤、儘早發動 |

### 一句話總結

**M1 的後端做得比帳面深，前端與後端之間差一層接線；真正的硬缺口只有三處：結構化篩選（DISC-003）、跨 Skill 比較（DISC-009），以及完全沒動的稽核與刪除（CORE-007／008）。** 驗證閘門本身卡在「使用者測試不能在半接線的 UI 上做」，而非檢索品質不足——檢索的量化前置早已通過，且有大幅餘裕。
