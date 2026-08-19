# Test Lab 公開契約面設計（UI/UX 導向，2026-08-19）

- 定位：[ADR-032](../adr/ADR-032-ddd-bounded-context-governance-for-platform.md) 待決策「testlab 公開契約面」的設計答案；執行項為 [DDD-007](platform-ddd-realignment-2026-08-19.md)。
- 負責人裁示：以 UI/UX 為導向設計，參考 docs 既有內容。
- 設計依據：`02` §4.3（TEST-001～005）、`03-work-items.md` §裁定尺（「條文主詞是使用者可／顯示者，沒有介面就是沒有達成」）、`01` §M2 自評（「編輯 Prompt 與驗收條件仍無 UI」的承接脈絡）、現有頁面（`TestCases.tsx`、`RunPreflight.tsx`）與 2026-08-19 的契約面盤點。

## 1. 設計原則（全部來自 repo 既有裁定，非新發明）

1. **旅程為綱**：契約面依使用者旅程六步組織（建立 → 編輯 → 掛 Dataset →（MCP 缺席，明示「無」）→ 權限確認發起 → 回看結果）。每條 endpoint 必須對應得到一個使用者動作；「有契約沒使用者面」與「有使用者需求沒契約」都是缺陷。
2. **fail-closed 的顯示面**：規則讀不到就不給控制項（`/lab/datasets` 前例）；absent 渲染為「未回報」不渲染為 0。
3. **確認的完整性**：`summary_hash` 涵蓋的每一個欄位都必須在摘要畫面上看得到——**不顯示的欄位不得進 hash**，否則使用者在對自己看不見的東西重新確認（TEST-005 的精神）。
4. **凍結語意上畫面**：快照不可變的事實必須在編輯處預告（現有三處文案的慣例延續）。

## 2. 契約面 A：HTTP／OpenAPI（testlab ↔ UI）

依旅程排列。標 **[新增]**／**[調整]** 者是 DDD-007 的工作內容；未標者維持現況。

### 步驟 1｜建立與瀏覽 Test Case

| 項目 | 設計 |
| --- | --- |
| `GET /test-cases` **[調整]** | 加 `?skill_id=` 篩選（後端 `ListTestCasesForSkill` 已存在，只缺路由參數）；list item 加三個彙總欄位：`skill_name`（免裸 UUID）、`criteria_confirmed`／`criteria_total`（現由前端自己數）、`has_rubric`。 |
| `GET /skills/{id}` 詳情頁 **[新增入口]** | Skill 詳情頁提供「此 Skill 的 Test Case」入口（消費上述篩選）。回答「這個 Skill 我寫過哪些 Test Case」這條目前無路可走的問句。 |
| `DELETE /test-cases/{id}` **[補使用者面]** | 契約已存在且 `DeleteResult.datasets_deleted` 專為 WS-002 的「說明刪除範圍」而設，但前端零呼叫——正是裁定尺會退回的形狀。前端補 `deleteTestCase`＋既有 `ConfirmDelete` 元件；回應的刪除範圍逐項顯示。 |

### 步驟 2｜編輯 Prompt 與驗收條件

| 項目 | 設計 |
| --- | --- |
| criteria CRUD | 維持逐條操作；不加批次確認（TEST-001 允收沒有要求，先不做）。 |
| `POST .../criteria/suggest` **[調整]** | 現況直接把建議寫進清單，使用者只能事後逐刪。改為**回傳建議、不落庫**；採納＝前端以既有 `POST criteria`（`source: suggested`）逐條寫入。TEST-001 明訂自動建議是「可選強化」、確認權在使用者——先寫後刪的形狀與此相反。503 視為「不可用」的語意不變。 |
| 「開始試跑」文案 **[修正]** | `TestCases.tsx` 仍寫「還需要填入 Skill Version id」——已被 `SkillVersionPicker`（丙-14）過時化，隨本批更新。 |

### 步驟 3｜Dataset

| 項目 | 設計 |
| --- | --- |
| `expires_at` **[補使用者面]** | 後端 `datasetResponse` 已回傳，前端型別沒接。TEST-002 的「保存政策」目前只講規則（90 天），沒有畫面講**這一個檔案哪天到期**——前端 `Dataset` 型別補 `expires_at` 並在 `DatasetSection` 顯示。 |
| 建立時同步上傳 | **不做**。兩步流程（先建 Test Case 再上傳）已被 M2 對帳接受為 TEST-002 的滿足形；合併成單請求要動 multipart＋交易邊界，UX 收益不成比例。 |
| `content_hash` | 維持在 wire 上（快照可追溯性需要），UI 繼續不渲染——它是機器欄位，不是使用者資訊。 |

### 步驟 5｜權限確認與發起

| 項目 | 設計 |
| --- | --- |
| `summary_hash` 涵蓋範圍 **[調整顯示面]** | 現況 `resource_limits` 十個欄位進 hash、畫面只渲染六個——改內部參數會作廢所有既有確認，而使用者根本沒看過那些欄位。裁定：**hash 涵蓋不縮**（縮 hash 是往不安全方向走），改為摘要頁加「進階限制」收合區補渲染其餘欄位（`max_pids`、`max_open_files`、artifact 上限、soft timeout），原則 3 成立。`provider` 五欄同理進收合區。 |
| 版本預設 **[新增]** | preflight 入口帶「上次執行版本」預設：由步驟 6 的 run 篩選免費取得（同一 Test Case 最近一筆 Run 的 `skill_version_id`），`SkillVersionPicker` 預選之，仍可改。testlab 本身不記「上次版本」——那是 run context 的事實，不複製。 |

### 步驟 6｜回看結果（旅程目前斷掉的回程）

| 項目 | 設計 |
| --- | --- |
| `GET /runs?test_case_id=` **[新增]** | run context 的 list 加此篩選（`RunListItem` 已帶 `test_case_id`，只缺查詢參數）。 |
| Test Case 詳情頁「執行歷史」區 **[新增]** | 消費上述篩選，顯示此 Test Case 的近期 Run（狀態、版本、時間、連往 `/runs/{id}`）。這補上「建立 → 試跑 → **回來看**」的閉環，也是 M3 主路徑「採納建議 → 新版本 → 同一個 Test Case 重跑」的 UI 支點。 |

### 明確不做（記錄理由，防止回鍋）

- **MCP Profile 面**：TEST-003 後 MVP；preflight 維持明示「無」。
- **Test Case 跨 Skill 複製**：`skill_id` 綁死是快照與評估語意的簡化前提；fork 後 Test Case 跟不過去是真需求，但屬 M5 範圍，等封測 `need_signal` 回饋佐證再設計（BETA-005 複審的輸入）。
- **criteria 批次確認／整體確認布林**：允收未要求；UI 以彙總欄位（`criteria_confirmed/total`）滿足引導需求即可。

## 3. 契約面 B：Go API（testlab ↔ 其他 context）

回答 ADR-032 待決策的原問題：**不抽子包**，以「單一讀寫門面」明確化——套件不拆，公開面收窄並補齊缺的那一半。

1. **公開面定義**（寫進 `testlab` 套件文件，並對應 ADR-032 附錄 A）：
   - 寫入面：`CreateSnapshot`（唯一寫入點，run 建立交易內呼叫——現況已正確）。
   - **讀取面 [新增]**：`ReadSnapshot(ctx, q, snapshotID)` 之類的讀取 API，回傳凍結的 prompt／criteria／rubric／dataset refs。現況 testlab 只給了寫入面，`run/preflight.go` 因此完全繞過 testlab 直讀 `gen.GetTestCase`／`gen.ListDatasets`，並自建與 `DatasetRef` 近乎相同的 `DatasetSummary` 型別、連排序穩定性註解都重複兩份——這是 bounded context 邊界上最實質的裂縫，讀取面補上後 run 改走門面。
   - 解碼函式：`DecodeCriteria`／`DecodeRubric`／`DecodeDatasetRefs` 是「與寫入同法讀取」的保證；**`packaging/testcase.go` 自行 `json.Unmarshal` criteria 的繞道改回 `DecodeCriteria`**。
   - 型別：`Criterion`、`Rubric`／`RubricItem`、`DatasetRef`、sentinel errors。
2. **上限常數維持單一強制點**：`MaxFileBytes` 等常數留在 testlab（註解已明言「唯一強制點」），HTTP `GET /test-cases/limits` 是它的顯示投影——這已是正確形狀。
3. **preflight hash 與 snapshot hash 維持兩個雜湊**：前者涵蓋「能碰什麼」（權限、草稿）、後者涵蓋「執行了什麼」（凍結輸入），語意不同不得合併（`run/preflight.go` 註解已載明，本設計確認之）。

## 4. 隨批驗證項（設計時發現的既有疑點）

- `02` §TEST-011 現況註記已過時（成本區間實際已渲染）——修 `02` 的註記。
- `m2/README.md` 記載 `public.yaml` 的 `RunPermissionSummary` 未宣告 `estimated_cost`，而前端在讀該欄位——若屬實即鐵律 12 違規（schema 先行）。DDD-007 開工時先驗證 `public.yaml` 現況，缺就補宣告。

## 5. 執行順序（併入 DDD-007）

1. 契約先行（鐵律 12）：`public.yaml` 補 §2 的新參數／欄位與 §4 的缺漏宣告 → `task gen:openapi`。
2. 契約面 B（Go 門面）：補讀取面、收 packaging 繞道、run/preflight 改走門面——這步在 depguard 白名單（DDD-002）標註 testlab 合法公開面之後做，籬笆先立。
3. 契約面 A 的前後端實作與文案修正。
4. 驗收：旅程六步每步至少一條 UI 測試（沿用 route-覆蓋自我強制的既有測試機制）；`GET /runs?test_case_id=` 有 route 測試與 AuthZ 矩陣列。
