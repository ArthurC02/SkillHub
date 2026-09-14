# ADR-082：前端以 feature 收納，頁面、子元件與 Service 各有位置

- 狀態：**Accepted**（2026-09-14，負責人指示：「不論是 pages 或是 components 你都是單一 Component 的概念；但是 Sub-Component 以及 Service 的概念並不具備……重新規劃」；核准計畫時裁定「子元件可以呼叫」service，並說明目的：「讓架構會說話，Coding Agent 才能有更低的摩擦」）
- 日期：2026-09-14
- 取代：[ADR-081](./ADR-081-frontend-components-in-three-layers-and-server-state-lives-in-api.md) 的決策 1（三層目錄）與決策 6 的路徑；ADR-081 的決策 2–5 延續，只換位置（見本文決策 3）
- 相關：[ADR-031](./ADR-031-artifact-role-repository-layout.md)（收納語意）、[ADR-039](./ADR-039-frontend-design-system-and-ui-evaluation-criteria.md)（前端設計系統）

## 背景

ADR-081 把 `apps/web/src` 分成 `pages/`、`components/`、`api/` 三層，管住了 import 的方向，卻沒有表達「誰屬於誰」：

- **`components/` 的 38 個檔混了四種東西。**
  - 真正跨功能共用的：`LoginRequired` 被 23 個檔使用。
  - 只有一頁在用的子元件：`AdminNav`、`BarChart` 只給 Admin，`InFlight` 只給 RunTrace，`CreationSession` 只給 CreateSkill。
  - App 外殼：`AuthControls`、`FeedbackEntry`、`CleanModeNotice` 只有 router 在用。
  - 根本不是元件的純邏輯：`runStatus.ts`、`packagingGate.ts`、`format.ts`。
- **真正的子元件反而擠在頁面檔裡。** Admin 一個檔裡有 19 個元件，EvaluationPanel 有 13 個，TestCases 與 Home 各 11 個；CreationSession 單檔 1478 行。
- **`api/` 是 20 個平放的檔**，看不出哪一份伺服器狀態屬於哪一個功能。
- **34 個測試檔平放在 `src/` 根層**，與被測物分開。

Coding Agent 要回答「這個子元件還有誰在用」「這一頁的資料從哪裡來」，只能全 repo 搜尋。目錄本身沒有說出任何事。

## 決策 1：四個區，import 只往內或往下

| 區 | 放什麼 | 可以 import |
| --- | --- | --- |
| `app/` | 組裝：`App.tsx`、`router.tsx`、`shell/`（頁首身分、回報入口、淨模式橫幅） | 自己、`core/`、`shared/`、各 feature 的 `*.page.tsx` 與 `index.ts` |
| `features/<name>/` | 一個功能的頁面、子元件、service 與 model | 自己、`shared/`、`core/`；別的 feature **只能經過它的 `index.ts`**；`app/router.tsx` 只能 `import type`（路由 search 的形狀） |
| `shared/` | 跨功能重用、自己不擁有伺服器資料的東西：`ui/`（`Loading`、`Timestamp`、`LoginRequired`……）、`format.ts` | `shared/`、`core/` |
| `core/` | 沒有畫面的單例：`api/`（`client`、`queryClient`、`queryKeys`、與契約對照的 `types.ts`）、`session/`（登入身分、點數） | 只有 `core/` |

另外兩個區不是產品程式：`guards/` 放量整個 app 的尺，`testing/` 放 fixtures。`src/` 根層只剩 `main.tsx` 與 `index.css`。

八個 feature 各自對到使用者的一段旅程：

| feature | 內容 |
| --- | --- |
| `catalog` | 首頁、Skill 比較 |
| `skill` | Skill 詳情、檔案 |
| `creation` | 建立、匯入、生成 |
| `lab` | Test Case、Dataset、執行前確認 |
| `runs` | Trace、Run 比較、Run 歷史、評估 |
| `packaging` | 打包、下載 |
| `workspace` | 我的 Skill、帳號、資料政策 |
| `admin` | 營運後台 |

- **feature 之間為什麼要經過 `index.ts`：** 一個 feature 對外提供什麼，都寫在同一個檔裡，一眼就能看完；改動內部時也不會意外弄壞別人。`index.ts` 裡只准出現 `export { … } from "./…";` 這一種句子。
- **匯入結果的 `ImportFinding`、`CategorizedFindings`、`ImportResult` 移進 `core/api/types.ts`。** 因為 core 的型別與 shared 的 `Findings` 都在用它們，它們是平台共用的詞彙，不是匯入功能私有的東西。

## 決策 2：feature 裡的角色，看檔名就知道

| 位置與檔名 | 角色 | 規則 |
| --- | --- | --- |
| `*.page.tsx` | 頁面：有位址的畫面，持有這一頁的 UI 狀態，組合子元件 | 只有 `app/` 可以 import；**每一個都必須被路由掛載**，沒有位址的就不是頁面 |
| `<頁面資料夾>/components/*.tsx` | 子元件：只屬於那一頁 | 只能被同一個頁面資料夾裡的檔使用 |
| feature 根層的 `components/*.tsx` | feature 內多頁共用，或透過 `index.ts` 提供給別的 feature 的元件 | 只能被這個 feature 使用，對外一律經過 `index.ts` |
| `*.service.ts` | Service：查詢、寫入，以及寫入後讓哪些資料失效 | 見決策 3 |
| `*.model.ts` | 型別與純函式：標籤表、閘門判斷 | 不需要 React 就能測 |
| `index.ts` | 這個 feature 對外提供什麼 | 只有 export 清單 |

- **子元件可以直接呼叫自己 feature 的 service**（負責人裁定）。寫入的「處理中」與錯誤狀態，只有顯示它的那個元件用得到，一路用 props 往下傳只會讓大頁面更難讀。TanStack Query 以快取鍵去重，所以子元件自己讀同一份資料不會多發請求。
- **本批只搬家，不拆檔。** 把頁面檔裡的區塊拆進各自的 `components/` 是下一批（見後續工作），拆的時候照這一節放。

## 決策 3：伺服器狀態的規則不變，只換位置

ADR-081 的決策 2–5 全部延續：

- react-query 的 hook 只出現在 `*.service.ts`，以及建立 QueryClient 的 `core/api/`。
- 每一個寫入都是 service 裡的 hook，並且自己宣告讓哪些資料失效。
- 快取鍵只由 `core/api/queryKeys.ts` 產生。
- 伺服器回應不複製進 `useState`。
- `retry: false` 只寫在 `core/api/queryClient.ts`。

新增一條：**service 不畫畫面**。沒有 `.service.tsx` 這種檔，service 也不 import 任何 `.tsx`。

快取鍵表仍集中成一份，不拆給各 feature，原因有兩個：跨 feature 的失效需要看得到別人的鍵（例如由改善建議建出新版本時，要讓 skill 的版本清單失效）；而且既有測試是用鍵的字面值預先填快取。

## 決策 4：測試跟著被測物走

- 測某個功能的測試放在該 feature 的根層，檔名不變（例如 `lab.test.tsx` 放在 `features/lab/`）。
- 測整個 app 流程的測試（App、critical-flows、clean-mode）放在 `app/`；回報入口的測試放在 `app/shell/`。
- 量所有頁面的尺放在 `guards/`：architecture、ia、design-system、a11y、contrast、contract、untrusted-text，以及 session（IA-6 的 401 規則橫跨所有頁面）。a11y 的輪廓快照 `__outlines__/` 跟著 a11y 一起放。
- fixtures 放在 `testing/fixtures/`。

## 決策 5：守它們的機器

`apps/web/src/guards/architecture.test.ts` 逐檔檢查以下各條。每一條都植入過一次違規、看它變紅，再還原：

- 決策 1 的 import 方向，包括 feature 之間只能經過 `index.ts`。
- 決策 2：
  - 子元件只被擁有它的資料夾使用。
  - 頁面只被 `app/` import，而且每一頁都有路由掛載。
  - `index.ts` 只有 export 清單。
- 決策 3：
  - react-query 的位置。
  - service 不含畫面。
  - 快取鍵。
  - retry。

「這個 state 是不是伺服器資料的副本」這一條仍然只能靠審查（ADR-081 決策 4）。

## 搬家對照

ADR 與 `docs/plans/mvp/` 裡的舊路徑是當時的紀錄，照舊不改；要找它們現在的位置，查下表。

| 舊位置 | 新位置 |
| --- | --- |
| `api/client.ts`、`queryClient.ts`、`queryKeys.ts`、`types.ts` | `core/api/` 同名 |
| `api/me.ts`、`api/credits.ts` | `core/session/me.service.ts`、`credits.service.ts` |
| `App.tsx`、`router.tsx` | `app/` 同名 |
| `components/AuthControls`、`FeedbackEntry`、`CleanModeNotice`，`api/feedback.ts` | `app/shell/`（後者改名 `feedback.service.ts`） |
| `components/Loading`、`Timestamp`、`ConfirmDelete`、`Reveal`、`StateIcon`、`LabelledBadge`、`LicenseBadge`、`RiskIndicator`、`CompatibilityStatus`、`Findings`、`ListFreshness`、`Tip`、`LoginRequired`、`SignIn`、`RouteNotFound`、`spotlight` | `shared/ui/` 同名 |
| `components/format.ts` | `shared/format.ts` |
| `pages/Home`、`pages/Compare`，`components/FacetNotes` | `features/catalog/home/Home.page.tsx`（及其 `components/FacetNotes`）、`compare/Compare.page.tsx` |
| `pages/SkillDetail`、`pages/SkillFiles`，`components/VersionUpload`、`SkillVersionPicker`，`api/skills`、`api/versions` | `features/skill/detail/`（及其 `components/VersionUpload`）、`files/`、`components/SkillVersionPicker`、`skills.service.ts`、`versions.service.ts` |
| `pages/CreateSkill`、`pages/ImportSkill`，`components/CreationSession`、`ModelMarkdown`、`GenerateSkill`、`CreateHub`、`GeneratedNotice`、`generateFailureSentence`，`api/creation`、`generate`、`import` | `features/creation/create/`（及其 `components/CreationSession`、`ModelMarkdown`）、`import/`、`components/`、`generate.model.ts`、`creation.service.ts`、`generate.service.ts`、`import.service.ts` |
| `pages/RunPreflight`、`TestCases`、`DatasetUpload`，`api/lab`、`api/testcases` | `features/lab/preflight/`、`test-cases/`、`dataset-upload/`、`lab.service.ts`、`testcases.service.ts` |
| `pages/RunTrace`、`RunCompare`、`WorkspaceRuns`，`components/InFlight`、`EvaluationPanel`、`RunVerdict`、`VersionDiff`、`runStatus`，`api/runs`、`trace`、`evaluation` | `features/runs/trace/`（及其 `components/InFlight`）、`compare/`、`list/`、`components/`、`runs.model.ts`、`runs.service.ts`、`trace.service.ts`、`evaluation.service.ts` |
| `pages/Packaging`、`Downloads`，`components/DownloadArtifactFacts`、`packagingGate`，`api/packaging` | `features/packaging/build/`、`downloads/`、`components/`、`packaging.model.ts`、`packaging.service.ts` |
| `pages/WorkspaceSkills`、`WorkspaceAccount`、`DataPolicy`，`api/policy` | `features/workspace/skills/`、`account/`、`policy/`、`policy.service.ts` |
| `pages/Admin`，`components/AdminNav`、`BarChart`，`api/admin` | `features/admin/Admin.page.tsx`、`components/`、`admin.service.ts` |
| `fixtures/`、`__outlines__/` | `testing/fixtures/`、`guards/__outlines__/` |
| `*.test.*` | 見決策 4 |

## 影響

- **行為不變。** 本批只搬檔、改寫 import，並把三個型別換到另一個檔。全部測試通過；ADR-081 的 6 條架構測試換成本 ADR 的 9 條。
- **路徑的改寫範圍。** 活文件（`docs/design/`、`docs/development/`、`docs/plans/` 的 01–05、`apps/web/AGENTS.md`、`.claude/`）裡的路徑已同步改寫。所有 markdown 連結都改指新位置，連結目標必須存在（`doc-links`）。

## 後續工作

- **拆大檔。** 把以下各檔裡的區塊拆進各自的 `components/`：
  - `Admin.page.tsx`（19 個元件）
  - `EvaluationPanel.tsx`（13 個）
  - `TestCases.page.tsx`、`Home.page.tsx`、`SkillDetail.page.tsx`、`Packaging.page.tsx`（各 11 個）
  - `RunTrace.page.tsx`（10 個）
  - `CreationSession.tsx`（1478 行）

  頁面檔裡的純邏輯拆成 `*.model.ts`，例如 RunPreflight 的 `SCRIPT_LABEL`、`limit`、`startFailureSentence`。
- **沿用 ADR-081 的後續工作。** 送出按鈕與確認動作的收斂，以及 `Findings` 與 `DownloadArtifactFacts` 的合併，仍要先決定文案。
