# Skill Hub — Coding Agent 導覽

Agent Skill 平台：Catalog、輕鬆創建、私人訂製與公開散布。架構與每條規則的理由在 [ADR 索引](docs/adr/README.md)，實作狀態在 [`01` §10](docs/plans/01-goals-and-plan.md)。

## 狀態與曝光邊界

- 狀態只看 [`01` §10](docs/plans/01-goals-and-plan.md) 與 [`04`](docs/plans/04-backlog-and-handoffs.md)，本檔不放日期與數字。**「程式面收斂」不等於「完成」**：M1 驗證閘門、M4 封測、M5 曝光都在等真人數字。
- **本專案永遠不凍結**：任何時候都可以修改程式、新增能力，不得以凍結、閘門期間、日期或「沒有放行」為由擋下或延後。品質靠測試、CI 與突變證明守住。文件裡殘留的「凍結例外」「第 N 次放行」是歷史紀錄。
- 三條 ⛔ 曝光邊界只寫在 `01` §10；最硬的一條：**M5 的生成入口不得對封測使用者出現**。

## 目錄地圖

| 目錄 | 內容 |
| --- | --- |
| `apps/` | `web`（React SPA）、`platform`（Go 控制平面）、`llm`（Python 能力提供者）、`sandbox`（Run 執行環境） |
| `contracts/` | 跨程序／跨語言介面的唯一來源（OpenAPI-first，程式是下游） |
| `packages/` | 供其他程式 import 的 library；generated 子目錄禁止手改 |
| `db/` | Migration、query、sqlc 設定與 `query-owners.yaml` |
| `infra/`、`tools/` | 部署與 runtime image；開發、CI 與維運命令（`tools/devctl` 是 repo 級機器檢查的家） |
| `docs/plans/` | `01` 目標與里程碑、`02` 規格允收（需求 ID）、`03` 工作清單、`04` 殘項與移交、`05` 待裁定；`mvp/mX/` 是里程碑當時的紀錄 |
| `docs/adr/` | 架構決策，一個主題一份、永遠是現行版本 |
| `docs/design/` | 前端兩把尺：[system.md](docs/design/system.md) 管一頁之內、[information-architecture.md](docs/design/information-architecture.md) 管頁與頁之間；兩份都有機器測試直接解析 |
| `docs/development/` | 開工前讀的手冊；[docs/runbooks/](docs/runbooks/) 是出事時讀的 |

## 實作鐵律（違反任何一條 = 架構回歸）

1. 不受信任的 Skill、Script、資料不得在 Web/API 程序內執行；匯入與掃描階段不得執行套件內 Script。
2. 執行平面不得直接存取核心資料庫；只透過任務契約、短效物件授權與事件互動。
3. 所有使用者資料查詢預設要求 Workspace Scope；不信任 UI 傳入的 `workspace_id`。
4. Skill Version、Test Case 快照、歷史 Run 不可變；採用改善建議＝建立新版本，不原地覆寫。
5. Run 狀態的唯一事實來源是 Go 擁有的 Postgres 狀態機；Python 側的程序內狀態只是暫存草稿，不得回寫成狀態。
6. Python 是能力提供者：收結構化請求、回結構化結果；政策、授權、狀態轉移、重試決策全在 Go。
7. 佇列消費者只有 Go Worker；Python 不消費佇列，由 Go 以內部 HTTP 呼叫（含逾時與取消傳遞）。
8. 所有模型呼叫走 LiteLLM 閘道，不得直連供應商；供應商金鑰只存在閘道。
9. 領域狀態變更與對外事件同交易（Transactional Outbox）；Consumer 必須冪等；`destroy`／清理可安全重複。
10. 平台 `run_id` 是永久識別；Provider 臨時 ID 不得當主鍵或永久 URL。
11. Secrets 不得出現在套件、Log、Trace 明文或分析事件；顯示前完成遮罩。
12. 跨語言介面先寫 OpenAPI schema 再實作；CI 以 codegen 檢查 drift。
13. `apps/web` 只對 `apps/platform` 的 API 說話；`apps/platform` 是唯一的控制平面。

## 慣例

- 文件用繁體中文（Run、Workspace、Provider 等術語不硬翻）；程式碼、識別字、commit message 用英文。
- **程式碼註解預設不寫**：意圖用命名說，看不懂就抽出有名字的函式、型別或常數。只有艱難、特殊的演算法才寫，說明它怎麼運作，一個區塊最多 3 行。
- **施工日誌與決策說明一律不寫進程式**：來龍去脈、理由、需求／裁定編號、日期、實測數字寫進 commit message 或 ADR；也不寫其他公司或產品的名稱。適用所有手寫程式與設定檔。寫完跑 `go -C tools/devctl run . comment-lint <路徑>`（`comment-budget` 會擋）。
- 只有 ADR 與 ADR 索引寫 ADR 編號或檔名；其他地方（含程式與測試）寫規則本身，要理由就連索引的主題段落（`adr-citations` 會擋）。
- **測試以 ISTQB 為準**：寫、改或審查測試前載入 `istqb-test-design` 技能，推出等價類、邊界、決策表、狀態轉移與負面案例；斷言具體結果，測試彼此獨立。
- **每次回覆的最後一段是給沒讀過程式碼的人看的白話摘要**：一段連貫敘述講做了什麼、發現了什麼、還缺什麼，不條列、不堆檔名。

## 開發自動化（Agent 開工先讀）

每條紅線的來歷與完整操作在 [docs/development/automation.md](docs/development/automation.md)。

1. **先診斷再修改**：`task --list`，再 `task doctor`（沒有 Task 時用 `go -C tools/devctl run . doctor`）。版本不符是診斷結果，不得靠跳過檢查偽裝成通過。
2. **預設不花錢**：`task dev` 只起 Postgres 與 SeaweedFS。`task dev:model` 會產生費用，**SubAgent 一律不得自行啟動**，需要就回報。
3. **共享工作樹、單一 Writer**：
   - SubAgent 預設唯讀；同一時間只有一個 Writer，而且要有精確的 path allowlist。
   - SubAgent 不跑 repo-wide formatter、套件安裝、Compose down、Git 寫入或 lockfile 更新。
   - 看到不屬於自己的未提交修改：保留並回報。不用 `git stash`、`git reset`、`git clean`、`git checkout -- <path>`、`git rm -f` 或強制切換分支，它們會收走或丟掉別人的工作。
   - 只以明確 pathspec stage 自己的檔案，push 前 `git pull --rebase`；不 `--amend`、不 `push --force`、不以 `--no-verify` 跳過 hook。暫存產物放 scratchpad。
   - 子代理的模型每次明確指定，**禁用旗艦級（主線自己用的那一級）**；從能做完的最低一級起，升級要有理由（分級見 automation.md〈共享工作樹與 SubAgent〉）。
   - 角色與技能只改 `.claude/agents/`、`.claude/skills/`；`.agents/`、`.codex/` 是生成快取，不手改、不提交，同步方式見 automation.md〈Harness〉。
4. **高衝突區由主 Agent 序列化**：`contracts/`、`db/migrations/`、`db/queries/`、`db/query-owners.yaml`、generated 目錄、`go.sum`／`package-lock.json`／`uv.lock`、`Taskfile.yml` 與 `.github/workflows/`。
5. **generated files 禁止手改**：來源改完由主 Agent 跑 `task gen:sql`／`task gen:openapi`，提交前一律 `task gen:check`。
6. **修好一個東西之後，把修法弄壞一次**：把修正那一行還原、跑對應測試、確認變紅、再改回來。綠燈只證明測試存在；commit 訊息裡「修好了 X」的證據就是那次紅。不適用純文案與純註解。

## 分區指標與攔阻

**動某個區域之前，沿 repo root 到目標父目錄讀完每一層 `AGENTS.md`**——它列出那一區的紅線、先讀哪一段、會被哪個閘門擋。工具不保證自動載入，這張表是唯一保證送達的入口。

| 你要動 | 先讀 |
| --- | --- |
| `apps/web/` | [`apps/web/AGENTS.md`](apps/web/AGENTS.md) |
| `apps/platform/` | [`apps/platform/AGENTS.md`](apps/platform/AGENTS.md)（route、Bounded Context、query 所有權的紅線）；動 `internal/` 再讀 [`internal/AGENTS.md`](apps/platform/internal/AGENTS.md)＋目標套件的 `doc.go` |
| `apps/llm/` | [`apps/llm/AGENTS.md`](apps/llm/AGENTS.md) |
| `apps/sandbox/` | [`apps/sandbox/AGENTS.md`](apps/sandbox/AGENTS.md) |
| `infra/images/` | [`infra/images/AGENTS.md`](infra/images/AGENTS.md)（改 runtime 映像要升版本、先問負責人） |
| `docs/` | [`docs/AGENTS.md`](docs/AGENTS.md)（文件維護規則） |

Claude Code 另有 `.claude/` 下的 rules、角色、技能、workflows 與 `skillhub-guards` 外掛，當場擋紅線；它們不取代文件，只是提早發現（見 automation.md〈Harness〉）。

## 去哪看

| 你要做的事 | 先看 |
| --- | --- |
| 狀態全貌、里程碑與 ⛔ 邊界 | [`01` §10](docs/plans/01-goals-and-plan.md) |
| 還缺什麼、誰在等誰（殘項總數唯一來源） | [`04`](docs/plans/04-backlog-and-handoffs.md) |
| 待裁定、我現在要簽什麼 | [`05`](docs/plans/05-pending-rulings.md) |
| 某功能的允收準則 | [`02`](docs/plans/02-specifications-and-acceptance-criteria.md)（按需求 ID） |
| 下一個工作項目 | [`03`](docs/plans/03-work-items.md) |
| 某條規則為什麼存在、系統邊界與資料模型 | [ADR 索引](docs/adr/README.md)，各主題的「待決策」是未決議題 |
| 派工流程、稽核 workflow、跨工具契約 | [agent-instructions.md](docs/development/agent-instructions.md) |
| 已審查的領域事實或已知空白 | [Domain Memory](docs/domain-memory/)；只把 reviewed records 當事實，`reviewed-empty` 代表尚無可依賴的模型 |
| 派送被停了怎麼判斷與解除 | [p1-dispatch-halt.md](docs/runbooks/p1-dispatch-halt.md) |
| 畫面該長什麼樣、新畫面放哪個網址 | [system.md](docs/design/system.md) §3、[information-architecture.md](docs/design/information-architecture.md) §0（先過這關再寫） |
| 封測上線前要做什麼 | [release-checklist](docs/plans/mvp/m4/release-checklist.md) |
| 同意書與資料保存 | [consent-and-data-policy.md](docs/plans/mvp/gate-test/consent-and-data-policy.md) |
| 新增、升級或移除一個依賴 | automation.md〈依賴的准入、更新與閘門〉 |
