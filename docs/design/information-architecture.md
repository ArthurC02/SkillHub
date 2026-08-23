# 資訊架構

本文件是 `apps/web` 的**資訊架構**：有哪些位址、它們屬於哪一段價值流、彼此怎麼到達、叫什麼名字、以及哪些狀態進得了網址。**活文件**，隨 [`apps/web/src/router.tsx`](../../apps/web/src/router.tsx) 一起改。

**與 [設計系統](./system.md) 的分工**：那份管**一頁之內**長什麼樣（字級、間距、狀態語彙、停用要說原因）；這份管**一頁與一頁之間**。兩份都遵守 system.md §0 的優先序——牴觸時安全與不誤導在前，一致與美觀在後。

**事實來源是 `router.tsx`，不是本檔。** 第 1 節的表是從它與各頁的 `<Link to=…>` 讀出來的快照；**沒有任何機制比對兩者**（見 §6），所以每一列都註明從哪裡讀來的，讓下一個人有辦法自己重數一次。

產品領域語言與價值流的定義在 [ADR-038](../adr/ADR-038-platform-product-domain-language-and-value-stream-navigation.md) §2；本檔用它的價值流當第一層分組，而不是用 `/workspace` 與 `/lab` 這兩個現行網址前綴——**那兩個前綴本身就是本檔第一個要記的問題**（§5 IA-2）。

---

## 1. 現況：17 條路由

`__outlines__/` 是 20 個檔，`rendered.spec.ts` 說「18 個位址」——差額不是矛盾：`/` 有帶查詢與不帶查詢兩種、`/runs/$runId` 的兩種閱讀模式各存一份快照、還有一個不是路由的回報問題面板。**快照認得的狀態比網址多**，這件事本身是 §5 IA-4。

| 位址 | 頁面元件 | 需求 ID（取自 `router.tsx` 檔頭） | ADR-038 價值流／產品領域 |
| --- | --- | --- | --- |
| `/` | `Home` | DISC | Skill 生命週期／**Skill 探索** |
| `/compare` | `Compare` | DISC-009 | Skill 生命週期／Skill 探索 |
| `/skills/$skillId` | `SkillDetail` | DISC-003、SKILL | Skill 生命週期／Skill 資產與版本歷史 |
| `/skills/$skillId/files` | `SkillFiles` | SKILL | Skill 生命週期／Skill 資產與版本歷史 |
| `/skills/$skillId/package` | `Packaging` | 02:PACK-001／002 | Skill 生命週期／**Skill 交付與安裝** |
| `/workspace/import` | `ImportSkill` | SKILL、SEC | Skill 生命週期／**Skill 接納與信任** |
| `/workspace/skills` | `WorkspaceSkills` | 02:WS-002 第 1 條／WS-004 | 創作者空間／創作者帳戶與工作區 |
| `/workspace/runs` | `WorkspaceRuns` | 02:WS-002 第 1 條／WS-004 | 試跑與改善／Skill 試跑執行 |
| `/workspace/downloads` | `Downloads` | 02:WS-002／WS-004 | Skill 生命週期／Skill 交付與安裝 |
| `/workspace/account` | `WorkspaceAccount` | WS、NFR | 創作者空間／創作者帳戶與工作區 |
| `/policy` | `DataPolicy` | 02:O11Y-004／`04` 丙-25② | 產品營運／創作者使用權益與資料生命週期 |
| `/lab/test-cases` | `TestCaseList` | 03:TEST-012 | 試跑與改善／**試跑情境設計** |
| `/lab/test-cases/$testCaseId` | `TestCaseDetail` | TEST | 試跑與改善／試跑情境設計 |
| `/lab/datasets` | `DatasetUpload` | 02:TEST-002／03:TEST-004 | 試跑與改善／試跑情境設計 |
| `/lab/run` | `RunPreflight` | 03:TEST-008／009（＋02:TEST-005 的同意綁定） | 試跑與改善／Skill 試跑執行 |
| `/runs/$runId` | `RunTrace` | 03:TRACE-006／007 **＋ EVAL-001／002** | 試跑與改善／**執行證據＋成果判定（兩個）** |
| `/runs/$runId/compare` | `RunCompare` | 02:EVAL-003 | 試跑與改善／成果判定與改善 |

**沒有位址的頁面一個**：[`RunEvaluation.tsx`](../../apps/web/src/pages/RunEvaluation.tsx)（31 KB，全 app 第二大）。它以 `EvaluationPanel` 的形式長在 `/runs/$runId` 裡，並且把 `RUN_STATUS_LABEL` 供給另外四個檔。詳見 §5 IA-3。

**深度最多三層**（`/skills/$id/package`），沒有一條路由需要記住兩個以上的 id。

---

## 2. 導覽：三層，而且第一層只服務一種人

### 2.1 全域（`RootLayout`，每一頁都有）

| 位置 | 項目 | 到哪裡 |
| --- | --- | --- |
| 標題 | `Skill Hub` | `/` |
| 主要導覽（`<nav aria-label="主要導覽">`） | 我的 Skill、Run 歷史、匯入 Skill、Test Case、下載紀錄 | `/workspace/skills`、`/workspace/runs`、`/workspace/import`、`/lab/test-cases`、`/workspace/downloads` |
| 頁尾 | 回報問題（面板，非路由）、資料保存政策、帳號與刪除 | `/policy`、`/workspace/account` |
| 右上 | `AuthControls`（未登入時是「使用 GitHub 登入」） | 外部 `/auth/github/login` |

「你在哪裡」不靠 `activeProps`：router 自己在相符的連結上標 `data-status="active"` 與 `aria-current="page"`，視覺與語意各一半，樣式在 `index.css` 的 `.app-nav`。

### 2.2 頁內連結圖（誰指向誰）

從各頁的 `to="…"` 讀出，自我連結不計：

```
Home ───────────► /compare, /skills/$id
Compare ────────► /, /skills/$id
SkillDetail ────► /skills/$id/files, /skills/$id/package, /lab/test-cases
SkillFiles ─────► /skills/$id
ImportSkill ────► /skills/$id
Packaging ──────► /skills/$id, /workspace/downloads
Downloads ──────► /skills/$id
WorkspaceSkills ► /, /skills/$id, /skills/$id/files, /skills/$id/package,
                  /lab/test-cases, /workspace/runs, /workspace/downloads,
                  /workspace/account, /policy
WorkspaceRuns ──► /runs/$id, /lab/test-cases
TestCases ──────► /lab/test-cases/$id, /lab/datasets, /lab/run, /runs/$id
DatasetUpload ──► /lab/test-cases, /lab/test-cases/$id
RunPreflight ───► /lab/test-cases, /runs/$id
RunTrace ───────► /runs/$id/compare
RunCompare ─────► /lab/run, /runs/$id
RunEvaluation ──► /lab/run, /skills/$id       （渲染在 /runs/$id 之內）
WorkspaceAccount► /policy, /workspace/{skills,runs,downloads}
DataPolicy ─────► /workspace/{skills,runs,downloads,account}
```

### 2.3 反向連結數（進得來的路有幾條）

**這一欄是 IA 的體檢指標**：只有一條入邊的頁面，使用者一旦用網址直接抵達或按了上一頁，就沒有第二條路回到它。

| 入邊 | 位址 |
| --- | --- |
| **0（只有導覽列）** | `/workspace/import` |
| **1** | `/compare`（只有 `Home`）、`/lab/datasets`（只有 `TestCases`）、`/runs/$runId/compare`（只有 `RunTrace`） |
| 2 | `/skills/$id/files`、`/skills/$id/package`、`/lab/test-cases/$id` |
| 3 以上 | 其餘 |
| 8 | `/skills/$skillId`（全 app 的匯流點） |

---

## 3. 命名：三套用語同時在跑

| 概念 | ADR-038 §3 規則 2 的受控用語 | UI 的 `<h1>` | 網址 |
| --- | --- | --- | --- |
| 一次 Run | **試跑** | Run 歷史／Run 詳情／Run 比較 | `/runs`、`/lab/run` |
| Trace | **執行證據** | （併入「Run 詳情」） | `/runs/$id` |
| Evaluation | **成果判定** | （併入「Run 詳情」） | 無 |
| 不可變內容快照 | **Skill 版本** | — | `?version=` |
| 試跑情境 | 試跑情境設計 | Test Case | `/lab/test-cases` |
| 資料集 | — | 上傳 Dataset（動詞） | `/lab/datasets`（名詞複數） |

AGENTS.md 的慣例「保留 Run、Workspace、Provider 等英文術語不硬翻」是對**文件**說的；**沒有任何決策說過 UI 該用哪一套**。於是 UI 事實上成了第三套用語——而它與 ADR-038 的差別不是翻譯，是**分類**：「Run 詳情」把兩個 ADR-025 明文分開的概念合成一個容器詞（§5 IA-3）。

---

## 4. 網址承載的狀態

**判準（現行實作實際遵守的）**：一個狀態值得進網址，當它是**「你在看哪一份東西」**；不進網址，當它是**「你偏好怎麼看」**。

| 位址 | search param | 進網址的理由（取自 `router.tsx`） |
| --- | --- | --- |
| `/` | `q`、`script`、`validation`… | 搜尋條件即所看之物；不在列舉內的值直接丟掉，讓手改的網址落在未篩選清單而不是錯誤頁 |
| `/compare` | `ids` | DISC-009：比較要能被連結、能撐過重新整理 |
| `/skills/$id/package` | `version` | PACK-001／002：版本是路徑之外的另一個「哪一份」 |
| `/lab/run` | `skill`、`version`、`test_case` | TEST-008／009：三個 id 都可從網址帶入；只有 `version` 另有選單，另外兩個由擁有它們的畫面選 |
| `/lab/datasets` | `test_case` | 同上；目前沒有選單（DESIGN-007） |
| `/lab/test-cases` | `skill`（須為 UUID） | 「此 Skill 的 Test Case」那條連結要的東西 |
| `/runs/$id/compare` | `against` | EVAL-003：對照的另一次 Run 在網址裡，比較才能被連結 |
| `/runs/$id` | **無** | 檔頭寫明：一般／進階模式是 component state，「閱讀偏好，不值得一個可分享的位址」 |

**永遠不進網址的一項**：Provider 的臨時 id。平台的 `run_id` 是唯一識別（鐵律 10）。

---

## 5. 現況的問題

逐項附證據。**這一節不做裁定**——需要產品決定的標明「待裁定」，其餘標明可以直接做。

### IA-1 探索不在導覽列，而它是第一段價值流

主要導覽的五項全部是「我的東西」。`01` 列的第一個使用者問題是「找不到、判斷不了」，ADR-038 的 Skill 探索是 Skill 生命週期的第一個領域，而它在導覽上的**唯一入口是左上角的產品標題**——一個約定俗成、但沒有任何標籤說它是「搜尋」的連結。

一個沒登入的訪客打開站，導覽列上**五項沒有一項對他有意義**。

### IA-2 同一個工作區有四個網址前綴，判準沒有寫在任何一處

`/workspace/*`（4 條）、`/lab/*`（3 條）、`/runs/*`（2 條）、`/skills/*`（3 條，且同時服務目錄與自己的 Skill）。

理由是逐檔寫在 `router.tsx` 檔頭註解裡的——例如下載紀錄「在 `/workspace` 底下而不在 `/downloads`，因為 API 是這樣擁有它的」。**那是一次一次的理由，不是一條規則**，所以下一條路由該放哪裡，只能靠讀完全部註解再類推。

### IA-3 `/runs/$runId` 一個位址回答兩個問題

[ADR-025](../adr/ADR-025-run-terminal-state-and-evaluation-verdict-separation.md) 明文把 **Run 終態**與 **Evaluation 判定**分離，ADR-038 §3 規則 2 也給了它們各自的產品名（執行證據／成果判定）。但：

- `RunTrace` 同時渲染 `EvaluationPanel`；
- `<h1>` 是「Run 詳情」——**兩個名字一個都沒用上**，用的是容器詞；
- `RunEvaluation.tsx` 沒有自己的位址，卻是全 app 第二大的頁面檔，並供給另外四個檔的標籤常數。

也就是說**領域模型分開了，網址與標題沒有**。使用者要把「這次跑出來的判定」貼給別人時，貼出去的位址叫做 Trace。

### IA-4 快照認得兩個閱讀模式，網址不認得——而且兩份文件對此各說各話

`__outlines__/` 有 `runs-runId-一般模式.txt` 與 `runs-runId-進階模式.txt` 兩份，也就是**測試已經把它們當成兩個畫面在守**。網址只有一個。

而兩處的立場相反：

- `router.tsx` 檔頭：模式是 component state，「閱讀偏好，不值得一個可分享的位址」——**一項決定**。
- [system.md §7.1](./system.md) 第 1 項：「進階 Trace 視圖無法被連結……把它提到 URL 上是一個小改動，所以自成一批」——**一項待辦**。
- `03` 的 `TRACE-007` 已定 `?mode=advanced` 為 **API 參數**（已勾選）。

三個位置、兩種立場。**待裁定**：閱讀模式該不該可連結。無論答案是什麼，輸的那一份要改，而不是繼續各自留著。

### IA-5 搜尋沒有結果時，沒有出口

`Home` 只連向 `/compare` 與 `/skills/$id`。`no_results` 與 `filtered_out` 兩個空狀態的文案分別要求使用者「換個說法」與「放寬篩選」——**兩條都是要他再試一次搜尋**。通往「自己匯入一個」的路只有導覽列（§2.3：`/workspace/import` 的頁內入邊是 0）。

這一格正是 M5 要落腳的地方（§7）。

### IA-6 沒有登出狀態的資訊架構

`router.tsx` 的 `beforeLoad` 與 `redirect` 各 **0 處**（可自行 `grep -c` 複驗）。所有 `/workspace/*` 與 `/lab/*` 在未登入時仍然可到達，導覽列也不隨登入狀態改變。`/policy` 是**唯一**明文決定「不放在 `/workspace` 底下、也不放在 session 後面」的頁——理由寫在它的檔頭：平台記錄了訪客什麼，是登入之前就會被問的問題。

**未查核**：各頁在 API 回 401 時各自畫什麼。本檔只斷言 router 層沒有守衛這一件事。

### IA-7 三個單一入口的頁面

`/compare`、`/lab/datasets`、`/runs/$runId/compare` 各只有一條入邊（§2.3）。其中 `/runs/$runId/compare` 的唯一入口 `RunTrace` 本身就是 IA-3 那個名字不對的頁。

### IA-8 一個地方，兩個心智模型

`/lab/datasets` 的 `<h1>` 是「上傳 Dataset」。網址是名詞複數（一個地方），標題是動詞（一個動作）。`/workspace/import` 同型：網址與導覽標籤都是動作，但它在 IA 上是 ADR-038「Skill 接納與信任」這一整段的入口。

---

## 6. 強制對照表：誰在守 IA

比照 system.md §6，**每一列都要寫覆蓋範圍**。這張表的誠實之處在於它大部分是空的。

| 規則 | 把關者 | 覆蓋範圍 |
| --- | --- | --- |
| 每條路由的標題階層不跳級 | [`a11y.test.tsx`](../../apps/web/src/a11y.test.tsx)（axe `heading-order`） | 全部路由；新路由沒加案例會 FAIL |
| 標題階層變了要被看到 | [`__outlines__/`](../../apps/web/src/__outlines__/) 快照 | 20 個檔。**不判斷對錯，只讓變更變成必須核可的 diff** |
| 導覽 landmark 唯一且具名 | `a11y.test.tsx`（axe `landmark-unique`） | 全部路由。`SkillDetail` 與 `SkillFiles` 各自帶一個未命名的 `<nav>`，主導覽因此必須具名 |
| 「你在哪裡」有語意 | TanStack Router 自動加的 `aria-current="page"` | 主要導覽五項 |
| 375px 不橫向溢出 | [`e2e/rendered.spec.ts`](../../apps/web/e2e/rendered.spec.ts) | 18 個位址，三引擎 |
| 網址參數不在列舉內就丟掉（不落在錯誤頁） | `validateSearch`（逐路由手寫） | 有 `validateSearch` 的 7 條 |
| **反向連結／可達性** | **沒有** | §2.3 是手數的 |
| **單一入口偵測** | **沒有** | 同上 |
| **命名與 ADR-038 受控用語一致** | **沒有** | §3 是手比的 |
| **一個位址只回答一個問題** | **沒有** | IA-3 就是這樣長出來的 |
| **登出狀態可達性** | **沒有** | IA-6 |
| **新頁面該放哪個前綴** | **沒有** | IA-2 |

---

## 7. 還沒進來的：M5（GEN）

[ADR-046](../adr/ADR-046-generating-a-skill-from-a-task-description.md) 起的「從任務描述生成 Skill」在 IA 上**目前不存在**：`router.tsx` 沒有任何生成入口，也不應該有。

**[ADR-052](../adr/ADR-052-m5-starts-in-parallel-with-an-unfinished-mvp.md) 的硬邊界是 IA 的約束，不只是排程的**：開工不等於曝光——生成入口**不得對封測使用者出現**，直到漏斗第一段有讀數為止。因此本節寫的是「它將來落在哪裡」，不是「它現在在哪裡」。

已經確定的兩件事：

1. **它的產出物是一次匯入**（GEN-003：generation is an import），所以生成物落在 Skill 生命週期的**接納與信任 → Skill 資產與版本歷史**，和 `/workspace/import` 同一段；`redistribution` 的第五個值 `generated` 就是這件事在資料上的形狀。
2. **它的輸入與探索的輸入是同一句話**。`Home` 的 `<h1>` 已經是「用一句話描述你的任務」。

**待裁定（本檔不決定）**：生成是**搜尋結果頁的一個出口**（找不到就幫你生一個，直接補上 IA-5 那個沒有出口的空狀態），還是**一個獨立入口**（導覽列第六項）？

兩者的 IA 後果不同：前者把生成綁在「探索失敗」這個時機上，量得到漏斗；後者讓生成變成與探索並列的第一級動作，也就更難維持 ADR-052 那條「不得曝光」的邊界——**一個放在導覽列上的東西，沒辦法只給一部分人看**。

---

## 8. 修這些的先後

依 system.md §0 的優先序推出來，**不是依工作量**：

| 順位 | 項目 | 為什麼排這裡 |
| --- | --- | --- |
| 1 | **IA-3**（一個位址兩個答案） | 最接近「不誤導」：把成果判定貼給別人時，位址叫 Trace。而且它與 ADR-025 的明文分離相牴觸 |
| 2 | **IA-4**（三處兩種立場） | 不必先決定對錯就能先讓三處說同一句話；拖著的代價是下一個人照著錯的那一份做 |
| 3 | **IA-1／IA-5**（探索不在導覽、空狀態沒出口） | 影響的是「快到第一個判斷」（system.md §1.2），而且 IA-5 是 M5 落腳處的前置 |
| 4 | **IA-6**（登出狀態） | 未查核的部分要先查核再排 |
| 5 | **IA-2／IA-8**（前綴與命名） | 一致性層級；先做不會錯，但它讓步不影響安全 |
| — | **IA-7** | 三個單一入口裡有兩個是 IA-3 的下游，修完上面再重數 |
