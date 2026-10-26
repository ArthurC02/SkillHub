# 資訊架構

本文件是 `apps/web` 的**資訊架構**：有哪些位址、它們屬於哪一段價值流、彼此怎麼到達、叫什麼名字、以及哪些狀態進得了網址。**活文件**，隨 [`apps/web/src/app/router.tsx`](../../apps/web/src/app/router.tsx) 一起改。

**與 [設計系統](./system.md) 的分工**：那份管**一頁之內**長什麼樣（字級、間距、狀態語彙、停用要說原因）；這份管**一頁與一頁之間**。兩份都遵守 system.md §0 的優先序——牴觸時安全與不誤導在前，一致與美觀在後。

**平台化目標與遷移順序見 [平台體驗模型](./platform-experience.md)。** 本檔仍只記已落地的規則與現況；`/library` 與 `/activity` 已落地，目標模型裡尚未實作的 `/releases` 等位址不進 §1～§4，直到同一批程式、測試與本檔一起落地。

**方向：§0 的規則走在程式前面，§1～§4 的表跟在程式後面。** 這兩半的權威方向相反，混在一起就沒有一半是真的：

| 章節 | 誰是事實來源 | 不一致時誰改 |
| --- | --- | --- |
| **§0 規則** | **本檔** | **改程式。** 一條新路由要先過 §0，過不了就先改 §0.2 的偏離帳並寫下理由——比照 `db/query-owners.yaml` 的 `allow:` 與 system.md §5 |
| §1～§4 的表（清單、導覽、連結、網址狀態） | `router.tsx` 與各頁 | 改本檔。它們是**盤點**，不是政策 |

**§1、§2.1、§2.3、§2.4、§4 與 §0.1 R2／R3 由 [`ia.test.ts`](../../apps/web/src/guards/ia.test.ts) 比對**（§4 的網址狀態表是**雙向**比對）——比照 `design-system.test.ts`：文件持有政策，程式持有事實，測試比對兩者。其餘章節沒有機器（見 §6），所以每一列都註明從哪裡讀來的，讓下一個人有辦法自己重數一次。

> **沒有機器守的章節會安靜地過期。** 條件入口不一定新增路由或連結，因此必須由 §2.4 與既有對帳明確列出；任何未被對帳的盤點都只能視為待人工複核。

產品領域語言與價值流的定義在 [Platform Bounded Context 與 Context Map](../adr/README.md#platform-bounded-context-與-context-map)；本檔用它的價值流當第一層分組，而不是用 `/workspace` 與 `/lab` 這兩個現行網址前綴——**那兩個前綴本身就是本檔第一個要記的問題**（§5 IA-2）。

---

## 0. 規則：下一個畫面要先過這一關

**這一節是本檔存在的理由，其餘章節是它的證據。** 只有盤點而沒有判準的資訊架構，不能約束下一條路由或入口；本節以可檢查的規則界定新增與遷移的條件。

七條規則沒有一條是新發明的：每一條都從已經定案的東西推出來，出處寫在該條裡。**推導不出來的東西不進這一節**——那是 §5 的問題清單或 `05` 的待裁定，不是規則。

### 0.1 七條

**R1. 一個位址回答一個問題。**
出處：[評估判定與 Judge 信任邊界](../adr/README.md#評估判定與-judge-信任邊界)（Run 終態與 Evaluation 判定是兩件事）＋ 命名規則（兩者各有產品名）。
一個畫面可以顯示很多東西，但**它的位址只能是一個答案的位址**。判準是使用者把網址貼給別人時，對方以為會看到什麼。
機器：**沒有**（判斷題）。現行違規：無。

**R2. 單筆位址由「它是什麼」決定，清單位址由「誰在問」決定。**
出處：WS-006（存在是私密的）＋鐵律 3（Workspace Scope 由伺服器決定，不信任 UI）。
- **單筆**：`/<複數名詞>/<id>`，**不掛在 `/workspace` 底下**。同一個 id 對擁有者與對訪客是同一份東西，差別在可見的欄位，不在位址。
- **清單**：掛在提問者的位置——`/workspace/*` 是「我的東西」，`/lab/*` 是「我要跑一次試跑時需要的東西」，`/admin/*` 是「我是 operator 時要處理的事」（[營運後台](../adr/README.md#營運後台)）。**這是「誰在問」第一次由角色決定、不是由帳號決定**：同一個帳號，`GET /me` 的 `operator` 翻了值，眼前就多一組位址。`/admin/*` 底下沒有單筆位址——找帳號、找 Skill 都是清單頁上的查詢，不是 `$id` 路徑段，所以單筆那一條不受影響。
- **根層**只留不屬於任何人的東西（`/`、`/policy`）與跨物件的操作（`/compare`）。

機器：**有**（`ia.test.ts`）。偏離見 §0.2。

**R3. 每一個畫面至少要有兩條進得去的路，否則要具名。**
出處：system.md §1.2（快到第一個判斷）——一條入邊的頁面，使用者按上一頁之後就回不去了。
新畫面只有一條入邊時，要嘛補第二條，要嘛寫進 §2.3 並說明為什麼它只該有一條。**「具名」那一支不是逃生門**：判準是這一頁要求的脈絡**是不是只有一個地方產得出來**——若是，第二條入邊得先發明一個脈絡（見 IA-7 的三個裁定）；若否，它就欠一條。
機器：**有**（0 入邊與 1 入邊兩份清單都雙向比對；具名與否是判斷題，機器只保證「有沒有出現在表上」）。

**R4. 「你在看哪一份東西」進網址，「你偏好怎麼看」不進。**
出處：現行 `router.tsx` 的 `validateSearch` 實際遵守的判準，見 §4。
機器：**沒有**。閱讀模式屬於「你偏好怎麼看」，不進網址（IA-4）。<br>（[設計系統、信任訊號與畫面用語](../adr/README.md#設計系統信任訊號與畫面用語)）：Tip 的開合與 `<details>` 的開合同屬「你偏好怎麼看」，**不進網址**；「記住已讀」若被裁定要做（`05` R-42 (a)），它的狀態也只能住在瀏覽器裡，不能住在網址裡——一個貼出去的連結對收件人不得少一段文字。

**R5. 標題要說出這一頁回答什麼，不用容器詞。**
出處：[Platform Bounded Context 與 Context Map](../adr/README.md#platform-bounded-context-與-context-map) 的命名規則。
**「詳情」「管理」「中心」這類容器詞不是名字**——它們是「我還沒決定這一頁在回答什麼」的另一種寫法，所以 R5 與 R1 總是一起被違反。

> **R5 不是要求把 `Run` 翻成「試跑」。** 命名規則定的是**概念的中文名**，沒有禁止英文術語，而 AGENTS.md 的慣例本來就保留 `Run`。**「Run 歷史」沒有問題，「Run 詳情」有問題**，而有問題的是後半不是前半。

機器：**沒有**。IA-8（Test Case 內的 `Dataset`）與 IA-3（`Run 詳情`）都已修正。

**R6. 一個入口是否出現，可以是旗標；但旗標本身必須寫在 §2.4。**
出處：[從描述生成 Skill](../adr/README.md#從描述生成-skill)（開工不等於曝光）。
**旗標後面的入口不新增路由、也可能不新增任何連結**，所以純看網址的檢查抓不到它——這是本檔學費最貴的一課（§7）。
機器：**有**（每一個 `features?.<name>` 的使用點都要在 §2.4 出現）。

**R7. 導覽列只放穩定的平台空間，不放一次性動作。**
出處：[平台體驗模型](./platform-experience.md) §2.1～§2.2 ＋ [從描述生成 Skill](../adr/README.md#從描述生成-skill)（生成入口仍不與搜尋等重）。
判準：導覽項目必須能承接多個物件與多次工作階段，並且離開再回來仍是同一個地方。探索、工作台、資產庫、Studio、活動與發佈符合；匯入 Skill、建立 Test Case、開始 Run 與打包是脈絡內動作，不符合。Studio 仍受 `generate_skill` 控制，規則是「旗標開啟時它是不是一個穩定空間」，不是「旗標能不能被導覽繞過」。
機器：**有**（`ia.test.ts` 雙向比對導覽目標與 §2.1，並拒絕已知的動作型入口）。

### 0.2 偏離帳

**形狀比照 [`db/query-owners.yaml`](../../db/query-owners.yaml) 的 `allow:` 與 system.md §5：具名、有理由、只能縮短不能增加。** 測試讀這張表；多一列就是 FAIL，除非有人先來改這張表——而那正是它要逼出來的那個可見決定。

| 位址 | 違反 | 理由與處置 |
| --- | --- | --- |
| `/lab/test-cases/$testCaseId` | R2（單筆掛在清單的前綴底下，而不是 `/test-cases/$id`） | **保留**。Test Case 沒有跨工作區的存在——它不像 Skill 或 Run 會被別人以 id 指涉，所以「單筆位址與擁有者無關」這個理由在它身上不成立。R2 的推導前提不適用，不是 R2 被打破 |
| `/lab/test-cases/$testCaseId/datasets` | R2（單筆後再掛 Dataset 子資源） | **保留**。Dataset 沒有脫離 Test Case 的獨立存在，owner API 也只提供 `/test-cases/{id}/datasets`；R2 對「一個可獨立指涉的單筆」所作的前提在依附子資源上不成立。把它移到根層反而會假裝平台有 Workspace Dataset 或獨立 Dataset 物件 |
| `/runs/$runId/compare` | R2（跨物件操作卻不在根層，`/compare` 在） | **保留**。它比較的是**這一次 Run 與另一次**，主詞是路徑上那個 run；`/compare` 比較的是兩個彼此無關的 Skill。兩者形狀不同不是不一致 |

**這張表現在有三列。它只能變短。** 新增一列要在同一批 commit 裡寫下理由，並且理由必須是「R 的推導前提在這裡不成立」，不能是「這樣比較方便」。舊 `/lab/run` 與 `/lab/datasets` 只保留為相容導向，不再是畫面；R2／R3 的頁面檢查排除只有 `beforeLoad` 導向、沒有元件的路由。

---

## 1. 現況：路由清單

`__outlines__/` 的快照份數多於位址數，而這不是矛盾（份數以 §6 為準）：`/` 有帶查詢與不帶查詢兩種、`/runs/$runId` 的兩種閱讀模式各存一份快照、還有一個不是路由的回報問題面板，**以及非成功態與目錄態各自存一份**（例如 `library-401`、`skills-skillId-test-cases-testCaseId-runs-new-loading`、`workspace-runs-empty`、`回報問題-驗證訊息`）。**快照認得的狀態比網址多**，這件事本身是 §5 IA-4。

| 位址 | 頁面元件 | 需求 ID（見表下說明） | 價值流／產品領域 |
| --- | --- | --- | --- |
| `/` | `Home` | DISC | Skill 生命週期／**Skill 探索** |
| `/compare` | `Compare` | DISC-009 | Skill 生命週期／Skill 探索 |
| `/skills/$skillId` | `SkillDetail` | DISC-003、SKILL | Skill 生命週期／Skill 資產與版本歷史 |
| `/skills/$skillId/files` | `SkillFiles` | SKILL | Skill 生命週期／Skill 資產與版本歷史 |
| `/skills/$skillId/versions/$versionId` | `SkillVersion` | SKILL、TEST、PACK-001／004 | Skill 生命週期／**精確版本、驗證與發佈**〔兩個 id 都是不可變脈絡；發佈仍不等於 Catalog 曝光〕 |
| `/skills/$skillId/package` | `Packaging` | 02:PACK-001／002 | Skill 生命週期／**Skill 交付與安裝** |
| `/p/$publisher/$name` | `PublicPublication` | 02:PACK-004 | Skill 生命週期／**Skill 發佈**〔不需登入；單筆的 id 是「發佈者／名稱」這一對，所以是兩段〕 |
| `/workspace` | `WorkspaceHome` | 02:WS-002／WS-004 | 創作者空間／**續作與跨物件注意事項** |
| `/workspace/import` | `ImportSkill` | SKILL、SEC | Skill 生命週期／**Skill 接納與信任** |
| `/library` | `WorkspaceSkills` | 02:WS-002 第 1 條／WS-004 | Skill 生命週期／**資產庫**〔擁有的 Skill 與新增入口；每個 Skill 續接版本、驗證、打包與發佈工作〕 |
| `/workspace/skills` | 相容導向 | 02:WS-002 第 1 條／WS-004 | 舊資產清單位址；保留 hash 後導向 `/library` |
| `/workspace/creations` | `CreateSkill` | 02:GEN-001（旗標 `generate_skill`）／[互動創作](../adr/README.md#互動創作)（旗標 `creation_skill`） | 創作者空間／**Skill 創作**〔負責人指示；旗標關著時這一頁只回一句「這一頁現在不存在」，⛔ `01` §10 邊界 1〕 |
| `/activity` | `Activity` | 02:WS-002／WS-004 | 創作者空間／**跨物件活動與續作**〔Run、Evaluation、Creation、Packaging、Publishing 五個 owner 全部成功才呈現；不完整時不顯示部分清單〕 |
| `/workspace/runs` | `WorkspaceRuns` | 02:WS-002 第 1 條／WS-004 | 創作者空間／**試跑活動**〔目前只投影 Run owner 清單；不冒充跨物件 Activity〕 |
| `/workspace/downloads` | `PublishingWorkspace` | 02:WS-002／WS-004、PACK-003～006 | Skill 生命週期／**發佈與交付**〔沿用舊網址；發佈者身分、跨 Skill Publication／最新 Release、精確版本 Bundle 與下載紀錄共用一個平台空間〕 |
| `/workspace/account` | `WorkspaceAccount` | CORE-007／02:SEC-006 | 創作者空間／創作者帳戶與工作區 |
| `/policy` | `DataPolicy` | 02:O11Y-004 | 產品營運／創作者使用權益與資料生命週期 |
| `/lab/test-cases` | `TestCaseList` | 03:TEST-012 | 試跑與改善／**試跑情境設計** |
| `/lab/test-cases/$testCaseId` | `TestCaseDetail` | TEST | 試跑與改善／試跑情境設計 |
| `/lab/test-cases/$testCaseId/datasets` | `DatasetUpload` | 02:TEST-002／03:TEST-004 | 試跑與改善／單一 Test Case 的 Dataset |
| `/lab/graders` | `EvaluationGraders` | 02:EVAL-014 | 試跑與改善／**成果判定**〔這個 Workspace 專用的 Prompt 評分器；平台通用的唯讀列出〕 |
| `/lab/datasets` | 相容導向 | 02:TEST-002／03:TEST-004 | 舊深連結；帶 Test Case 時改寫成上列 canonical URL，否則回到 Test Case 清單 |
| `/skills/$skillId/test-cases/$testCaseId/runs/new` | `RunPreflight` | 03:TEST-008／009（＋02:TEST-005 的同意綁定） | 試跑與改善／**指定 Skill 與 Test Case 的 Run 建立前確認** |
| `/lab/run` | 相容導向 | 03:TEST-008／009 | 舊連結帶齊 Skill 與 Test Case 時導向上列；缺 Test Case 時回到情境清單，缺 Skill 時若有 Test Case 則回單筆，不渲染另一份 preflight |
| `/runs/$runId` | `RunTrace` | 03:TRACE-006／007 **＋ EVAL-001／002** | 試跑與改善／**執行證據＋成果判定（兩個）** |
| `/runs/$runId/compare` | `RunCompare` | 02:EVAL-003 | 試跑與改善／成果判定與改善 |
| `/admin` | `AdminHome` | 02:OPS-001 | 產品營運／**營運後台** |
| `/admin/accounts` | `AdminAccounts` | 02:OPS-002／OPS-003 | 產品營運／**營運後台** |
| `/admin/skills` | `AdminSkills` | 02:OPS-004 | 產品營運／**營運後台** |
| `/admin/dispatch` | `AdminDispatch` | 02:OPS-005 | 產品營運／**營運後台** |
| `/admin/rosters` | `AdminRosters` | 02:OPS-005 | 產品營運／**營運後台** |
| `/admin/audit-log` | `AdminAuditLog` | 02:OPS-006 | 產品營運／**營運後台** |
| `/admin/model-budgets` | `AdminModelBudgets` | 02:OPS-009 | 產品營運／**營運後台** |
| `/admin/settings` | `AdminSettings` | 02:OPS-011／GEN-014 | 產品營運／**營運後台**〔平台政策：評估判定的評審團與對話回答輪數上限；每次改動要理由並進動作紀錄，可直達 Agent 花費上限與啟停〕 |
| `/admin/graders` | `AdminGraders` | 02:EVAL-014 | 產品營運／**營運後台**〔每個 Workspace 都能選用的平台評分器；每一版要理由並進動作紀錄〕 |
| `/admin/cost-statistics` | `AdminCostStatistics` | 02:OPS-007 | 產品營運／**營運後台** |
| `/admin/trends` | `AdminTrends` | 02:OPS-008 | 產品營運／**營運後台**〔圖表見 [營運後台](../adr/README.md#營運後台)〕 |
| `/admin/exposure` | `AdminExposure` | 02:DISC-007 | 產品營運／**營運後台**〔發佈物的曝光審核：待審清單與一筆的精確快照〕 |
| `/admin/agents` | `AdminAgents` | 02:OPS-011／OPS-012／OPS-013 | 產品營運／**營運後台**〔平台 Agent：待核准的提案（點開看會發生什麼、依據與決定）、日報回報的待辦收件匣（一件事一列，點開看依據、經過與處理）、日報與執行紀錄、每日花費上限、啟停與全域煞車〕 |

> **營運後台是組裝層，不是 Bounded Context**（[營運後台](../adr/README.md#營運後台)）——上表 `/admin/*` 那幾列的「價值流／Bounded Context」欄寫的是它服務的價值流。
>
> **這一欄只記在本表。** 全 repo 註解清理拿掉了 `router.tsx` 逐路由的需求 ID 註解，路由與需求 ID 的對照今天只剩這裡，**沒有第二個作者，也沒有機器**——改路由時要自己回來改這一格。
>
> **這一欄沒有機器**（§6：`ia.test.ts` 只比對位址那一欄）。

**沒有位址的區塊一個**：[`EvaluationPanel.tsx`](../../apps/web/src/features/runs/evaluation/EvaluationPanel.tsx)（全 app 最大的幾個檔案之一）。它長在 `/runs/$runId` 裡；沒有位址就不是頁面，所以它不是 `*.page.tsx`，而是 runs 這個 feature 裡的元件（[前端架構與樣式分層](../adr/README.md#前端架構與樣式分層)）。它原本兼供的 `RUN_STATUS_LABEL` 搬到了 `features/runs/runs.model.ts`。詳見 §5 IA-3。

**最深的工作脈絡是** `/skills/$skillId/test-cases/$testCaseId/runs/new`：它把這次建立 Run 所屬的 Skill 與 Test Case 固定在路徑，最多仍只記住兩個 id；可替換的 Version 留在 query，不把三個識別都塞進 search params。

---

## 2. 導覽：三層，第一層是穩定平台空間

### 2.1 全域（`RootLayout`，每一頁都有）

| 位置 | 項目 | 到哪裡 |
| --- | --- | --- |
| 標題 | `Skill Hub` | `/`；首頁就是可瀏覽的 Skill Catalog，不因登入狀態改變目的地 |
| 全域搜尋 | 離開 Catalog 後顯示「搜尋小工具或描述任務」；`/` 由 Catalog 頁自己的完整搜尋取代，不重複兩份表單 | `/` 加 `q` |
| 主要導覽（`<nav aria-label="主要導覽">`） | 工作台、探索、資產庫、Studio（旗標）、活動、發佈 | `/workspace`、`/`、`/library`、`/workspace/creations`、`/activity`、`/workspace/downloads` |
| 頁尾 | 回報問題（面板，非路由）、資料保存政策、帳號與刪除、**Build 識別碼**（`<details>`，非路由；IA-11） | `/policy`、`/workspace/account` |
| 右上 | `AuthControls`（未登入時是「使用 GitHub 登入」）；**`GET /me` 的 `operator` 為真時多一個「後台」**（`02:OPS-001`） | 外部 `/auth/github/login`；`/admin` |

「你在哪裡」不靠 `activeProps`：router 自己在相符的連結上標 `data-status="active"` 與 `aria-current="page"`，視覺與語意各一半，樣式在 `styles/layout.css` 的 `.app-nav`。

### 2.2 頁內連結圖（誰指向誰）

從 `pages/` 與 `components/` 的 `to="…"` 讀出，自我連結不計：

```
Home ───────────► /compare, /skills/$id, /workspace/import, /library#create
WorkspaceHome ──► /, /activity, /skills/$id, /skills/$id/versions/$id, /lab/test-cases/$id,
                  /runs/$id, /workspace/creations, /workspace/import,
                  /library
Compare ────────► /, /skills/$id
SkillDetail ────► /skills/$id/files, /skills/$id/versions/$id, /lab/test-cases
SkillFiles ─────► /skills/$id, /skills/$id/versions/$id, /lab/test-cases
SkillVersion ───► /skills/$id, /skills/$id/files, /skills/$id/package,
                  /skills/$id/versions/$id, /lab/test-cases, /workspace/downloads
ImportSkill ────► /skills/$id
Packaging ──────► /skills/$id, /skills/$id/files, /skills/$id/versions/$id,
                  /lab/test-cases, /workspace/downloads
PublishingWorkspace ► /p/$publisher/$name, /skills/$id, /library
WorkspaceSkills ► /skills/$id, /skills/$id/files, /skills/$id/package,
                  /lab/test-cases, /workspace/runs, /workspace/downloads,
                  /workspace/account, /workspace, /policy
Activity ────────► /runs/$id, /workspace/creations, /workspace/downloads, /library
WorkspaceRuns ──► /activity, /runs/$id, /skills/$id, /skills/$id/versions/$id,
                  /lab/test-cases/$id, /lab/test-cases, /workspace
TestCases ──────► /lab/test-cases/$id, /lab/test-cases/$id/datasets, /skills/$id/test-cases/$id/runs/new, /runs/$id,
                  /runs/$id/compare, /skills/$id, /skills/$id/files, /lab/graders
CheckEditor ────► /lab/graders                （渲染在 /lab/test-cases/$id 的驗收條件之內）
EvaluationGraders ► /lab/test-cases
DatasetUpload ──► /lab/test-cases/$id, /skills/$id, /skills/$id/files,
                  /skills/$id/versions/$id
RunPreflight ───► /lab/test-cases, /runs/$id, /runs/$id/compare, /skills/$id, /skills/$id/files
RunTrace ───────► /runs/$id/compare, /lab/test-cases, /skills/$id,
                  /skills/$id/files
RunCompare ─────► /skills/$id/test-cases/$id/runs/new, /runs/$id, /lab/test-cases, /skills/$id,
                  /skills/$id/files
EvaluationPanel ► /skills/$id/test-cases/$id/runs/new, /skills/$id
                                              （渲染在 /runs/$id 之內）
WorkspaceAccount► /policy, /library, /workspace/{runs,downloads}
DataPolicy ─────► /library, /workspace/{runs,downloads,account}
GenerateSkill ──► /skills/$id                 （旗標後面的元件，§2.4）
GeneratedNotice ► /lab/test-cases             （同上；尚無 Test Case 時先建立情境）
CreateHub ──────► /, /workspace/import        （渲染在 /library 之內）
CreationSession ► /lab/test-cases, /skills/$id/test-cases/$id/runs/new, /runs/$id,
                  /skills/$id/versions/$versionId, /library
                                              （渲染在 /workspace/creations 之內）
AdminSettings ──► /admin/agents
```

> **`/library` 的新增入口（`features/creation/components/CreateHub.tsx`）。** 它把三條接納路徑收在同一處：**匯入現成的套件** → `/workspace/import`（該頁唯一的 `.action`，system.md §4.6.3）、**從 Catalog Fork** → `/`（沒有封測邀請的帳號，入口旁直接說 Fork 需要邀請；有邀請或沒有名冊的部署不顯示）、**依任務描述創作** → 旗標後面的 Studio（§2.4），不新增邊。首頁的 hero 指著 `/library#create`。
>
> **形狀由資產庫空不空決定**：空的時候先說明這是空清單，再用完整卡片解釋每一條接納路徑；有內容時，Skill 卡片先回答「我擁有什麼」，新增方式收成清單後方的一條入口帶。讀取中或失敗不渲染新增入口，避免把未知冒充成空。兩種形狀的 DOM 順序都讓答案在動作之前。
>
> **旗標由入口所在畫面透過既有 hook 讀取**：`RootLayout` 只在 `generate_skill` 開啟時顯示 Studio；`WorkspaceSkills` 以 prop 把兩個創作旗標傳進建立區；`WorkspaceHome` 只有在兩個旗標都開啟時才掛載最近會話查詢。`ia.test.ts` 的 `FLAG_OFF_ASSERTED` 名冊以呼叫旗標 hook 的檔案為鍵。

> **這張圖是本節唯一沒有機器守的一格**（§6 上它一列都沒有，而 §2.1、§2.3、§2.4 都有），所以它會無聲過期。它要連 `components/` 的元件一起畫——§2.3 的反向連結數就是這樣數的（`ia.test.ts` 掃 `pages/` 與 `components/` 兩個目錄），只畫 `pages/` 會重現不出那些數字。

### 2.4 條件入口（旗標後面的入口）

**這一節是 §1 那張表看不到的一整條軸。** 一個藏在 feature flag 後面的入口**不新增路由，也可能不新增任何連結**——它只是在某些人的畫面上多出一塊。純粹看網址的檢查抓不到它。

| 旗標 | 來源 | 出現在 | 不出現在 |
| --- | --- | --- | --- |
| `generate_skill` | `GET /me` 的 `features`（[`useGenerateEntryPoint`](../../apps/web/src/features/creation/generate.service.ts)） | 搜尋的 `no_results` 空狀態、`/library` 新增區、全域 Studio 空間；與 `creation_skill` 同時開啟時，首頁可返回已存在且仍可操作的會話 | **搜尋框旁邊的等重動作**——Catalog 仍先搜尋；首頁不提供開始新創作的入口，沒有可續作會話時也不渲染空卡 |
| `creation_skill` | `GET /me` 的 `features`；`WorkspaceSkills`、`WorkspaceHome` 與全域殼層讀取 | `/library#create` 內的互動創作入口、首頁的既有會話續作，以及全域殼層授權後由 Activity 顯示的既有 Creation Session 續作連結；三處仍須 `generate_skill` 同時開啟，三種素材共用會話 | 未啟用部署與封測曝光限制中的使用者；預設關閉；首頁不把最近 50 筆清單冒充完整 Activity；Activity 可顯示 owner 狀態，但殼層未授權時不提供 Studio 入口 |
| `clean_mode` | `GET /me` 的 `features`（[`useCleanMode`](../../apps/web/src/core/session/me.service.ts)） | 每一頁 `<main>` 的第一個元素（[`CleanModeNotice`](../../apps/web/src/app/shell/CleanModeNotice.tsx)，掛在 `router.tsx` 的 `RootLayout`） | 匿名訪客的畫面——`GET /me` 要求 session，`/` 與 `/skills/$id` 未登入可見，PORT-003 今天只對已登入者成立（[淨測試模式](../adr/README.md#淨測試模式) 待決策 1 待敲定） |

**`clean_mode` 這一列是揭露不是入口**：它不帶使用者去任何新地方，只是在已經看得到的畫面上多說一句「這個部署沒有什麼」——上面 `generate_skill` 那一列的「出現在／不出現在」欄位問的是「使用者能不能從這裡走到一個新功能」，這一列的欄位問的是「使用者能不能看到這句話」，兩者是不同的問題，讀這張表時不要用入口的規矩讀這一列。

**`operator` 不是一個旗標**。`/admin/*` 的入口以 `GET /me` 的**頂層** `operator` 欄位分岔，不經過 `features`：它回答的是「你是誰」，不是「這個部署開了什麼」，所以它不在上面那張表，也不在 `ia.test.ts` 掃 `features?.` 的範圍裡；它的規則在 §0.1 R2。前端的判斷只決定畫不畫，**擋人的是每一條 `/admin/...` 端點各自的 `RequireOperator`**——member 直接打網址，看到的是與不存在的網址同一頁（`RouteNotFound`）。

**旗標從 `GET /me` 讀，不是 build 時的常數**，而這是[從描述生成 Skill](../adr/README.md#從描述生成-skill) 那條邊界唯一守得住的形狀：同一份 build 要同時服務看得到與看不到的兩群人。理由：**封測受測者一旦遇到「搜不到 → 生成一個」，`01` §11.2 漏斗第一段量到的就是另一件事，而那個數字只有一次機會、十二個人。**

---

### 2.3 反向連結數（進得來的路有幾條）

**§0.1 R3 的證據，逐列由機器算出來的**（`ia.test.ts` 掃 `pages/` 與 `components/` 的 `to="…"`，以不同來源檔計數，導覽列不算——一個**只**從導覽列進得去的頁正是這張表要找的東西，把導覽算成一條邊就會把它抹掉）。

| 入邊 | 位址 | R3 |
| ---: | --- | --- |
| **0** | （無） | ✅ 沒有孤兒頁 |
| **1** | `/compare`、`/lab/test-cases/$testCaseId/datasets` | ✅ 兩項都是 R3 的「具名」那一支（IA-7）：每一頁都要求一個**只有一個地方產得出來的脈絡**，第二條入邊得先發明一個脈絡才畫得出來，逐項理由見 §5 IA-7 |
| 2 | `/workspace`、`/workspace/account`、`/workspace/creations`、`/policy`、`/runs/$runId/compare`、`/admin`、`/admin/accounts`、`/admin/dispatch`、`/admin/rosters`、`/admin/model-budgets`、`/admin/settings`、`/admin/graders`、`/admin/cost-statistics`、`/lab/graders` | ✅ |
| 3 | `/skills/$skillId/files`、`/skills/$skillId/package`、`/p/$publisher/$name`、`/workspace/runs`、`/admin/skills`、`/admin/audit-log`、`/admin/trends`、`/admin/exposure` | ✅ |
| 4 | `/workspace/import`、`/skills/$skillId/test-cases/$testCaseId/runs/new` | ✅ |
| 5 | `/lab/test-cases/$testCaseId` | ✅ |
| 7 | `/`、`/library` | ✅ |
| 8 | `/runs/$runId` | ✅ |
| 9 | `/workspace/downloads`、`/admin/agents` | ✅ |
| 9 | `/lab/test-cases` | ✅ |
| 12 | `/skills/$skillId/versions/$versionId` | ✅ |
| 16 | `/skills/$skillId` | ✅ 全 app 的匯流點 |

> **「導覽列不算一條入邊」不是計數細節，是這張表的用途。** 外部審查要求刪掉 `features/workspace/skills/WorkspaceSkills.page.tsx` 頁尾的「這個工作區的其他清單」，理由是那四條連結與導覽列 100% 重複——**那是事實**。擋下它的正是這條計數規則：整塊刪掉會讓 `/workspace/runs`、`/workspace/account`、`/policy` 各從 2 掉到 1 條頁內入邊，一次生出三個只從導覽列進得去的頁。
>
> **其餘各列會無聲過期。** 機器只雙向比對 0 與 1；`/workspace/downloads` 與 `/workspace/runs` 都曾經在這張表上多掛一條，過期了好一陣子沒有人發現。改了連結就要回來重數這張表。

**0 與 1 這兩列由測試雙向比對**：多一個孤兒會 FAIL，把孤兒修好了而沒改這張表也會 FAIL。其餘各列是同一次計算的輸出，改了程式就會在這裡看到差異。

---

## 3. 命名

> **沒有第三套用語。** 命名規則定的是**概念的中文名**（「試跑」是一次 Run），它沒有禁止在畫面上用 `Run` 這個字，而 AGENTS.md 的慣例本來就保留它。把「UI 用 Run、文件用試跑」寫成用語衝突，是把**翻譯**當成了**分類**。

| 概念 | 受控中文名 | UI 用什麼 | 合規 |
| --- | --- | --- | --- |
| 一次 Run | 試跑 | `Run 結果`／`Run 比較` | ✅ `Run` 是保留術語，兩個標題都說出了自己回答什麼 |
| 跨物件工作投影 | 活動 | `活動`；Run-only 保存檢視為 `試跑活動` | ✅ `/activity` 只合併五個 owner 已分類的 facts；任何來源失敗都不呈現部分結果，來源物件的完整證據仍留在自己的頁面 |
| Trace | 執行證據 | `執行紀錄`（`/runs/$id` 的 h2） | ✅ 是那一頁的一個區塊，不是一個位址 |
| Evaluation | 成果判定 | `任務判定`（同頁 h2，排在最前） | ✅ 同上 |
| 不可變內容快照 | Skill 版本 | 版本選單、`?version=` | ✅ |
| 單一不可變內容快照的工作區 | 精確版本 | `版本與發佈` | ✅ 以 Skill 與 Version 兩個 id 形成可分享脈絡；不把它稱為草稿或 Release |
| 試跑情境 | 試跑情境設計 | `Test Case` | ✅ 保留術語 |
| 資料集 | — | `<Test Case 名稱> 的 Dataset`（`/lab/test-cases/$testCaseId/datasets` 的 h1） | ✅ **IA-8 已修正**；標題同時說出物件與所屬脈絡 |

**R5 的現行違規只剩一項**。`/compare` 與 `/skills/$skillId/package` 原本也是動詞當標題，已改為名詞（`Skill 比較`、`Skill 套件`）：

| 位址 | `<h1>` | 承載檔 | 處置 |
| --- | --- | --- | --- |
| `/workspace/import` | `匯入 Skill` | `features/creation/import/ImportSkill.page.tsx` | §5 IA-8 裁定留著、不排工——那一頁的**網址本身**也是動作，改它是路由遷移不是改標題 |

`/compare` 改後的 `Skill 比較` 與 `/runs/$runId/compare` 的 `Run 比較`（`features/runs/compare/RunCompare.page.tsx`）同形；兩者是不同的頁，比的東西也不同。

Catalog landing `/` 的 `<h1>` 是名詞化的產品承諾 `探索能直接採用的 Agent Skill`（`features/catalog/home/components/SearchHeroForm.tsx`）；搜尋是縮小同一座商品格線的工具，精選與已收錄由卡片標記區分，不另拆書架，匯入或建立也不與搜尋並列。`/` 是登入前後一致的產品首頁；`/workspace` 是「工作台」，只負責續作與跨物件注意事項，兩者不共用責任。

**R5 沒有機器**（見 §0.1），所以這張盤點表與程式的一致性只能靠人比，而這一格會無聲過期。

---

## 4. 網址承載的狀態

**判準（現行實作實際遵守的）**：一個狀態值得進網址，當它是**「你在看哪一份東西」**；不進網址，當它是**「你偏好怎麼看」**。

**這張表由 [`ia.test.ts`](../../apps/web/src/guards/ia.test.ts) 雙向比對 `router.tsx` 的 `validateSearch`**，所以第二欄要逐個列全、不能用刪節號。多一個參數沒補列會 FAIL，刪了參數沒改表也會。

| 位址 | search param | 進網址的理由（取自 `router.tsx`） |
| --- | --- | --- |
| `/` | `q`、`correction`、`script`、`validation`、`agent`、`tier`、`category`、`compare` | 搜尋條件即所看之物。`correction` 保存使用者確認的五欄意圖與關鍵詞 JSON，篩選仍由既有參數承載；重載與分享時直接使用修正版，不重新分析。修正版無效時明確報錯，不偷偷退回原句；不在列舉內的值直接丟掉，讓手改的網址落在未篩選清單而不是錯誤頁。<br>**`compare` 是後來加入的**，逗號分隔，與 `/compare?ids=` 同一個形狀。**它不是一條新規則，是既有那條規則的另一半**：`/compare?ids=` 把選擇放在網址上，讓一次比較可以被連結、撐得過重新整理（DISC-009），而產生那份選擇的上一步把它放在 `useState` 裡——同一份東西，晚一步撐得過重新整理，早一步撐不過，於是 DISC-009 自己的工作流「比較 → 上一頁 → 換掉一筆 → 再比較」每走一次都要重新勾兩個。<br>**原本提的是把 `q` 放到 `/compare` 上，那個修法被 R4 擋掉並且沒有做**：`q` 對 `/compare` 既不是「你在看哪一份東西」也不是偏好，是「你從哪來」，而 §0.2 只收「R 的推導前提在這裡不成立」。**而且它的前提本來就不成立**——`submitSearch` 是 `replace: true`，瀏覽器上一頁本來就回得到 `/?q=…`；遺失的一直是勾選。<br>**`category`**（PDM-001 的三個架位，自 migration 0053 起是平台欄位）。**進網址的理由是 R4 的「你在看哪一份東西」那一邊**：一個類別是目錄的一段——「文件類的 Skill 有哪些」是一個回答得完的問題，把網址貼給別人，他看到的就是那一段——而不是一種看法；反面同時成立，換一個類別換掉的是清單的內容，不是它的呈現方式。<br>**同一個參數由兩個控制項寫**：搜尋框下方那一列分類 chip（快捷，帶伺服器數出來的筆數）與篩選列裡的「類別」select（與其他五個維度並列）。**它們不是兩個狀態**——`?category=` 只有一份；兩個控制項對同一件事講不同的話，是這一頁已經出過事的形狀（「清除所有篩選」曾經漏掉 `agent`）。<br>**不收 `unassigned`**：那是一列在沒有人給它類別時的回答（畫面上是型別化的缺席詞，`02:DISC-004`），不是一個架位。`05` R-19 決定使用者自行匯入的 Skill 如何取得類別之前，那個狀態會一直在 |
| `/compare` | `ids` | DISC-009：比較要能被連結、能撐過重新整理 |
| `/skills/$id/package` | `version` | PACK-001／002：版本是路徑之外的另一個「哪一份」 |
| `/workspace/creations` | `session`（須為 UUID） | 目前正在續作哪一場可變創作會話；選擇歷程或建立成功都改寫同一個值，重載後向伺服器取回該 session 的最新 revision。它不把 revision 放進網址，也不把 session 當成正式 Skill Version |
| `/workspace/downloads` | `artifact`（須為 UUID）、`publication`（須為 `發佈者/名稱`）、`bundleVersion`（須為 UUID） | 發佈與交付空間裡正在續接哪一筆保存紀錄或哪一個 Bundle 成員版本。`artifact` 指向一個不可變打包成品，`publication` 指向一筆 Skill Publication，`bundleVersion` 指向使用者剛選定且 owner 版本清單確實包含的不可變 Skill Version；三者都只使用 owner API 已回傳的識別，不把 Workspace 識別或前一頁脈絡塞進網址。打包、公開取得或 Bundle 匯出一旦收到 `artifact_id`，產生結果的連結就必須帶回該值；版本內的 Publication 操作同樣使用回應中的 `publisher/name`；「加入 Bundle」則保留當下的精確 Version，不得只把人送到泛用清單，也不得改選最新版本。資料成功載入且找到精確項目後才定位；成功載入但找不到時說明目前工作區找不到這筆，讀取失敗仍是讀取失敗。兩個以上參數同時出現是含糊連結，頁面不任選一筆，也不移動焦點。Bundle Publication 不在 `/me/publications`，不由這一批參數假裝支援 |
| `/skills/$skillId/test-cases/$testCaseId/runs/new` | `version`、`against` | TEST-008／009：Skill 與 Test Case 是建立 Run 的固定工作脈絡；只有可替換的 Version 留在網址狀態並由頁面選擇。`against` 是改版前那次 Run：從改善建議建立新版本後重跑同一題時帶著它，試跑開始後頁面多給一條到 `/runs/$id/compare?against=` 的連結，讓使用者自己確認建議有沒有真的改善；它不改變確認流程，也不自動開跑 |
| `/lab/run` | `skill`、`version`、`test_case` | 舊深連結的相容輸入；帶齊 Skill 與 Test Case 後改寫成上列 canonical URL，沒有足夠脈絡時回到 Test Case 清單或單筆，不渲染第二份 preflight |
| `/lab/test-cases/$testCaseId/datasets` | `version`（須為 UUID） | Test Case 已固定在路徑；精確 Version 留在網址，讓 Dataset、Test Case 與後續 preflight 共用同一個不可變版本脈絡 |
| `/lab/datasets` | `test_case`、`version`（後者須為 UUID） | 舊深連結的相容輸入；有 Test Case 就改寫成上列 canonical URL，沒有就回到 Test Case 清單，不渲染第二份 Dataset 畫面 |
| `/lab/test-cases` | `skill`、`version`（皆須為 UUID） | 「此 Skill／Version 的 Test Case」那條連結要的東西；`version` 會跟著進單筆 Test Case 與 preflight，不改變清單本身的 owner scope |
| `/lab/test-cases/$testCaseId` | `version`（須為 UUID） | 從精確版本進來時保留不可變版本脈絡，讓「用這一題試跑」送往同一版 preflight |
| `/runs/$id/compare` | `against` | EVAL-003：對照的另一次 Run 在網址裡，比較才能被連結 |
| `/runs/$id` | `evaluation`、`events` | **這一格曾經寫「無」而且從來沒有更新過。** 一般／進階模式確實不在網址上（IA-4 的裁定，R4），但那不代表這一條路由沒有 search param——它有兩個，而且兩個都是 R4 的另一半「你在看哪一份東西」：`evaluation` 指名這次 Run 的某一份不可變判定（[資料所有權與核心基礎設施](../adr/README.md#資料所有權與核心基礎設施)／[評估判定與 Judge 信任邊界](../adr/README.md#評估判定與-judge-信任邊界)；沒有它，被取代的舊判定連不出去，而重新評估過的 Run 的「目前判定」是另一個判定），`events` 是進階 Trace 的游標堆疊，讓事件流的第 7 頁貼得出去也撐得過重新整理。<br>**這一格是本節補上機器的直接原因**：文件說「無」，程式說「兩個」，而在那之前沒有任何東西會 FAIL |
| `/admin/skills` | `q` | 你在治理哪一個 Skill（`02:OPS-004`）：一個 UUID 就是那一個，其他字串是名稱片段；清單上「處理這一個」把 `q` 換成那個 UUID，所以處理中的那一個可以連結、撐得過重新整理 |
| `/admin/exposure` | `publication` | 你在審哪一個發佈物（`02:DISC-007`）：`發佈者/名稱` 這一對就是那一筆；清單上「審這一筆」把它放進網址，所以審到一半的那一筆可以連結、撐得過重新整理；不是這個形狀的值丟掉、回到只有清單 |
| `/admin/trends` | `days` | 你在看哪一段資料（`02:OPS-008`）：7、30 或 90 天，其他值丟掉、回到預設的 30；分享出去的連結重現同一段 |
| `/admin/agents` | `status`（已解決、已忽略、已自行恢復三種）、`finding`（須為 UUID）、`proposal`（須為 UUID）、`proposal_view`（待核准、處理中、最近結案三種）、`proposal_offset`（非負整數）、`run`（須為 UUID） | 平台 Agent 的待辦收件匣與提案（`02:OPS-012`、`02:OPS-013`）：`status` 是你在看哪一疊已關閉的事，沒有它就是待辦（待處理與處理中）；`finding`、`proposal`、`run` 各是正在打開的待辦、提案與執行；`proposal_view` 與 `proposal_offset` 指明提案清單的工作位置，沒有時從待核准的第一頁開始。這些都是「正在看哪一份東西」，可連結且重整後不遺失；不合形狀的值丟掉，回到預設 |

**其餘十七條路由沒有 `validateSearch`**（33 條路由減去上表的 16 條）（`/skills/$id`、`/skills/$id/files`、`/skills/$id/versions/$versionId`、五條不含網址狀態的 `/workspace` 與 `/workspace/*`、`/policy` 等）：它們回答的問題完全由路徑決定，所以上表沒有它們的列——多列一條會 FAIL。

**永遠不進網址的一項**：Provider 的臨時 id。平台的 `run_id` 是唯一識別（鐵律 10）。

**第二項：別人的 email。** `/admin/accounts` 回答的是「你在查哪一個帳號」，照 R4 該進網址；但那個值是別人的 email，而網址會進瀏覽器歷史，也會隨連結被分享出去（`02:OPS-002`）。所以它刻意沒有 `validateSearch`，email 只留在輸入框裡。這是 R4 一個有名字的例外，不是遺漏；查到之後要分享的是 workspace id，那不是個人資料。

**外部對照**：另一個團隊的內部平台把「你在看哪一種版本庫」放在路徑段（`/version-repository/schedules`）而不是分頁狀態，貼出去的連結落在同一個分頁——R4 的「哪一份東西」那一半，做法與我們的 `?category=`／`?tier=` 同一個判準，只是用路徑段而不是 search param。**它同時把「開發／部署」這個全站環境切換放在側欄、不進網址**——那與我們 IA-4 的爭議是同一題（一個會改變整頁內容的切換，收件人打開連結看到的和寄件人不一樣），所以不是可借的做法，是同一個缺口的另一個實例。

---

## 5. 維護與驗證

本文件的現況清單、主要導覽、條件入口、反向連結數與網址狀態表，必須與 `router.tsx`、`RootLayout.tsx`、各頁連結及 `validateSearch` 同批更新。§1～§4 是盤點，不是把未落地目標提前寫成路由。

### 5.1 維護規則

- 新路由先確認它回答一個問題、屬於正確的物件脈絡，並決定是否需要偏離帳。
- 新的 `features?.<name>` 使用點必須在 §2.4 具名；旗標關閉時不得留下可誤解的入口。
- 新的 `validateSearch` 路由與每個參數必須在 §4 列出；閱讀偏好不進網址。
- 修改頁內連結後，重數 §2.3；只有 0 與 1 入邊由機器雙向比對，其餘列仍須更新。
- 相容導向不是新的畫面；它必須保留必要脈絡並導向 canonical URL。
- 目標模型中尚未實作的空間不得加入 §1～§4，直到程式、測試與本檔一起落地。

### 5.2 IA 結論與驗收

**IA-1 探索是穩定空間。** Catalog 維持登入前後一致的探索入口；搜尋、比較與採用不得被工作台、一次性建立動作或帳號狀態取代。

**IA-2 工作區前綴不定義領域。** `/workspace`、`/lab` 與 `/admin` 表示提問者與工作情境；路由表仍以價值流與物件脈絡分組，新增單筆位址依 R2 判定。

**IA-3 Run 頁同時回答執行與成果。** `/runs/$runId` 必須保留 Run、Trace 與 Evaluation 的分別；比較頁與重新試跑只能續接明確 Run、Skill、Version 與 Test Case。

**IA-4 URL 只保存可分享的脈絡。** 物件、精確 Version、比較對象與可重開的游標可進網址；閱讀模式、展開狀態與個人偏好不得進網址。新增參數必須通過 §4 雙向對帳。

**IA-5 導覽只列穩定空間。** 主要導覽可到工作台、探索、資產庫、旗標後的 Studio、活動與發佈；匯入、建立 Test Case、開始 Run、打包與下載維持脈絡內指令。

**IA-6 窄螢幕不移除資訊架構。** 主要導覽與其鍵盤順序仍可掃讀；必要時在其自身 rail 內捲動，不以折疊選單隱藏平台空間。

**IA-7 單一路徑必須具名。** 只有需要唯一產生脈絡的畫面可維持一條頁內入邊，並在 §2.3 說明；其餘畫面至少兩條頁內入邊，0 與 1 入邊由守衛對帳。

**IA-8 標題說明物件與脈絡。** 頁面標題不得以「詳情」「管理」「中心」等容器詞代替回答；依附資源必須說明其 owner，例如 Test Case 的 Dataset。

**IA-9 相容導向不創造第二個流程。** 舊網址保留必要識別後導向 canonical URL；資訊不足時回到能補足脈絡的清單或單筆，而不是渲染另一份頁面。

**IA-10 篩選與搜尋有明確作用範圍。** 一個控制項只能改變它具名的清單；同一狀態由多個控制項操作時共用同一份網址或頁面狀態，清除也必須涵蓋全部維度。

**IA-11 Build 與資料權益是全域揭露。** Build 識別碼、資料保存政策、帳號與刪除入口屬於殼層揭露，不新增路由責任，也不得在窄螢幕或工作台中消失。

**IA-12 條件入口是產品表面的一部分。** 每個 `features?.<name>` 的使用點都要在 §2.4 記錄其來源、可見處與不可見處；旗標關閉時不留下替代入口、空狀態或導覽繞行。

### 5.3 現行邊界

`/library` 與 `/activity` 是現行平台空間；`/workspace/creations` 仍受旗標控制。`/releases`、全域命令列與更完整的 Studio 旅程仍屬目標，不可在現行導覽或路由清單中宣稱已上線。

### 5.4 機器檢查

[`ia.test.ts`](../../apps/web/src/guards/ia.test.ts) 應維持以下對帳：§1 路由、§2.1 主要導覽、§2.3 的 0／1 入邊、§2.4 的 feature flags，以及 §4 的 search params。文件與程式任何一側先變更，都應讓另一側在同一批變更中恢復一致。
