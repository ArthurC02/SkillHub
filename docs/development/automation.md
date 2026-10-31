# 開發自動化與 Coding Agent 協作

本文件是[開發自動化與依賴治理](../adr/README.md#開發自動化與依賴治理)決策的操作手冊，也是**開發自動化規則與操作的完整事實來源**：命令、檔案所有權、能力與成本分級、生成來源表與排錯都在這裡。`AGENTS.md` 只放紅線與入口連結，不複製本檔內容。兩者不另造第二套流程，所有入口最後都走 `Taskfile.yml` 與 `tools/devctl`。Platform 的 Bounded Context 日常判斷見 [DDD 實務指南](./platform-ddd-practices.md)。

## 新電腦的最短路徑

### 建議：Dev Container

1. clone repo；
2. 以 `.devcontainer/devcontainer.json` 開啟；
3. post-create 會安全建立 `.env`（已存在就不碰）並 bootstrap 依賴；
4. 執行 `task doctor` 與 `task gen:check`。

Dev Container 以 privileged mode 啟動獨立 DinD daemon，讓 Windows／macOS／Linux 的 nested generator 都看到同一個 `/workspace`；其 images/volumes 不借用 host daemon。Privileged container仍是高權限，只對可信任的 SkillHub repo 使用，細節見 `.devcontainer/README.md`。它不執行不受信任 Skill，也不取代 gVisor 部署驗收。

**不要改用 host socket**：Docker-outside-of-Docker 會讓內層 daemon 把 container 的 `/workspace` 當成物理主機路徑，generator 因此找不到 sqlc config。容器以預設非 root `vscode` 使用者啟動，Docker 資料放獨立 volume。Python／Go generator image 各自以 `tools/codegen/<lang>` 作最小 build context；**不得改回 repo root context**，否則 builder 會遍歷無關的 Dataset／Trace sample 與本機依賴目錄。

### Native fallback

新機器沒有 Task 時：

- `go -C tools/devctl run . doctor`
- `go -C tools/devctl run . env-init`
- `go -C tools/devctl run . bootstrap`

語言版本由 `go.mod`、`.node-version`、`.python-version` 擁有；其他工具與 generator image 由 `tools/toolchain.yaml` 擁有。不要從 README 文字或 Agent 記憶抄版本。

**釘選的版本要由 shell 啟用**：`.node-version` 只提供來源，不能自動切換 runtime；使用版本管理器時，確保非互動 shell 也能取得同一版本。Windows 若機器層 `PATH` 優先於使用者層 shim，先修正 `PATH` 順序，不要重複安裝。切換 Node 後重跑 `task bootstrap`，再以 `task doctor` 驗證。

## 能力與成本分級

| 入口 | Docker | Secret | 可能花錢 | 作用 |
| --- | --- | --- | --- | --- |
| `task doctor` | 只檢查 | 否 | 否 | 診斷 runtime、Task、uv、Docker、Compose、shell task 所需的 Bash 能力與 `.env` 是否存在；不讀值 |
| `task env:init` | 否 | 否 | 否 | `.env.example` → `.env`，已有檔案時不覆寫 |
| `task bootstrap` | 否 | 否 | 否 | Go download、npm ci/build、uv frozen sync |
| `task dev`／`dev:core` | 是 | 否 | 否 | Postgres＋SeaweedFS |
| `task dev:model` | 是 | **是** | 後續模型呼叫會 | 先 fail-closed 檢查 OPENAI/LiteLLM 變數，再啟動 gateway |
| `task dev:llm` | 否 | **是** | 視後續呼叫而定 | 新簽或保留服務 Virtual Key；啟動前向閘道查證有限、未耗盡的預算，無法確認便拒絕啟動；沒設閘道變數則以空金鑰啟動 |
| `task clean-mode` | 否 | 否 | 否 | 淨測試模式：內嵌資料庫、行程內物件儲存、本機行程 Driver，不碰閘道 |
| `task dev:observability` | 是 | 否 | 否 | `docker compose --profile observability … up -d prometheus`：起一個**開發機**的 Prometheus，讓 `infra/observability/alerts.yml` 真的被求值（不是生產部署） |
| `task down` | 是 | 否 | 否 | `docker compose … down`：停掉本機基礎設施（不刪 volume）。**它停的是共享的那一組**，唯讀／寫入 SubAgent 一律不得自行執行 |
| `task gen*` | 是 | 否 | 否 | 固定版本 generator；apply 會改 generated files，check 只寫 `.devctl/` scratch |
| `task test`／`task ci` | 視測試 | 否 | 否 | 預設不執行 E2E 模型測試；需 secret 的測試維持 opt-in gate |

Bash 能力探測會清空子行程的 `BASH_ENV`，不改動使用者環境；缺少或不可用時只顯示 `WARN`，不阻擋其餘原生命令。Windows 可將 Git Bash 放到 `PATH`，或在已設定的 WSL／Dev Container 內執行 shell task。

`.env.example` 內非空的 Postgres／SeaweedFS 值是公開的 local-only placeholder；production secret 一律留空。不得把 `.env`、key 值或 doctor 以外的環境 dump 放進回覆或 log。

「不需 secret／不花模型費」不等於 offline：第一次 bootstrap 會下載 Go/npm/uv 依賴，第一次 generation 會拉 digest-pinned image，Redocly 與 container build亦需 registry／package mirror。lockfile、version與digest確保內容可重現，不宣稱斷網仍能從空 cache 建置；隔離環境應提供核准的 mirror或事先填好的 cache。

## 生成來源與所有權

| 人工修改來源 | 生成目標（不得手改） | 入口 |
| --- | --- | --- |
| `db/migrations/**`、`db/queries/**`、`db/sqlc.yaml` | `apps/platform/internal/foundation/persistence/db/gen/**` | `task gen:sql` |
| `contracts/openapi/public.yaml` | `apps/platform/internal/entrypoint/api/gen/**`、`packages/api-client-ts/src/generated/**` | `task gen:openapi` |
| `contracts/openapi/llm-internal.yaml` | `packages/api-stub-py/src/skillhub_api_stub/generated/**` | `task gen:openapi` |

`.devctl/**` 是 gitignored lock／scratch，不是可引用、可提交或可手改的 generated API。Generator 失敗時只查看它作診斷；正式產物只以上表為準。`task gen:check` 不修改 tracked files，但可能寫入 `.devctl/` scratch；`task gen` 的 apply 才會更新 generated files。

`task gen` 的順序：取得單一 Writer lock → 在 repo 同檔案系統 scratch 完整生成 → 檢查絕對路徑／timestamp → 比對完整 tree → apply 時原子替換。任何一步失敗都不應留下半套 tracked output。`task gen:check` 不修改 tracked files，CI 使用同一路徑。

Generator upgrade 必須獨立 commit／PR，同時更新 manifest、generator lock、generated output並審查語意 diff。不得使用 `--skip-validate-spec`、OpenAPI 3.0 shadow copy或 generated-file workaround。

## 新增 API 的安全順序

1. 先改 OpenAPI schema／operation；若需要資料，再改 migration/query。
2. 由主 Agent（單一 Writer）執行 `task gen`；SubAgent 不自行生成。
3. 實作 Go domain policy／service／adapter。Workspace 取自 session，不接受 UI 傳入 scope。
4. Web 保留 `apps/web/src/core/api/types.ts` 的 UI view model，逐 endpoint 寫 generated DTO adapter，不整檔替換。
5. Go 的 contract 產出只有 models：`tools/codegen/go/ogen.yaml` 把 server 與 client 的生成都關掉，所以沒有 generated router 存在，也沒有東西可以整批 mount。新增 endpoint 一律在 `router.go` 明確掛上並保留 `RequireSession`／`RequireOperator`／`OptionalSession` 語意；`GET /healthz` 的回應形狀仍取自生成的型別。
6. 跑 scoped tests、`task gen:check`，最後跑 `task ci`。

## 共享工作樹與 SubAgent

- 目前不要求 worktree。
- 主 Agent 是唯一協調者與 Writer；SubAgent 預設唯讀。
- 寫入 SubAgent 需要精確 path allowlist。它不切 branch、不 stash、不 stage/commit/push、不安裝 package、不更新 lockfile、不執行 repo-wide formatter／generator，也不啟停共享 Compose。
- 唯讀 SubAgent 可以平行；寫入、generator、formatter、package manager、migration、contracts、CI/Taskfile 全部序列化。
- 未知 delta 視為他人工作：不得 reset、clean或 checkout還原。
- 只有負責整合的主 Agent執行明確 pathspec stage、commit、pull --rebase與 push。
- **子代理的模型按「這件事需要多少推論」選，不按名字選**——禁了最高階之後反射性地全派次高階，同樣不對。每次派工明確指定；從能做完的最低一級起，升級要在 brief 裡寫理由。規則用特性描述，因為模型名稱會換、而且本 repo 同時有多個 coding agent 在協作；名稱只出現在「現行對應」一欄，換名只改那一欄：

  | 等級 | 特性 | 派給它的事 | 現行對應（Claude Code／Codex） |
  | --- | --- | --- | --- |
  | **旗艦** | 主線代理自己用的等級 | **子代理禁用**——它是派工者，不是被派的 | Fable／Sol |
  | **深度推論** | 善用推論思考，會停下來判斷 | 跨模組推理、規格含糊要判斷、安全或資料遺失路徑、對抗性審查 | Opus，effort `medium`，對抗性審查 `high`／Terra |
  | **快速執行** | 執行快、推論不多 | 規格與邊界都清楚的多檔實作、照分區卡與現成模式做、驗證回報、文件整理 | Sonnet，effort `medium`／Luna |
  | **機械執行** | 推論極少，正因如此會像機械操作一樣照目標做完 | 規格完全明確、有測試判對錯：改一行跑一條測試、抄表、查值、突變稽核 | Sonnet，effort `low`／Luna |

  Luna 介於快速執行與機械執行之間，Codex 側這兩級都派它。

  **effort 是第二個旋鈕，而且先轉它**：同一個模型調低 effort，推論、成本與延遲降得比換小一號的模型或在 prompt 裡叫它「少想一點」都可靠；子代理沒寫 effort 就沿用派工那個 session 的 effort，等級表因此失效。`xhigh` 與 `max` 只給實測過確有收穫的工作——在較新的模型上它們會拉長每一輪，還可能自己再開審查子代理。Haiku 只在結果由機器判對錯、而且量大到成本是主要考量時才派：它對文字指示的遵循明顯不穩。

  Claude Code 的兩個角色檔（`.claude/agents/`）以機械執行／快速執行為 frontmatter 預設下限（`model` 與 `effort` 都寫），派工時可指定更高；`automation-check` 的 `harness` 檢查擋角色檔出現旗艦級或缺 `effort`（名稱寫在檢查器裡，那是唯一需要名字的地方）。可攜角色與 skills 的唯一來源是 `.claude/agents/`、`.claude/skills/`；`task agents:sync` 在本機與 CI 產生 `.agents/skills` 與 `.codex/agents`，兩者是忽略的快取，永不手改、永不提交。新一輪 Agent 工作前由唯一 Writer 同步，已啟動的 Agent 必須重新派送才會讀到新版本。
- **Claude Code 另有一層攔阻**（`.claude/settings.json` 的 `permissions.deny` 把 `stash`／`add -A`／`commit -a`／`restore`／`checkout .`／`reset --hard`／`clean`／`push --force`／`commit --amend` 變成真的拒絕，對子代理同樣生效），**但它只是提早發現**：其他 coding agent 不受它管，本節的規則本體與 `automation-check`、測試、CI 才是保證。角色與技能的放置規則見下方〈Harness〉。

## 修正必須證明測試會抓到回歸

行為修正不能只以綠色測試判定完成。把修正還原、執行對應測試確認它會失敗，再恢復修正並確認 diff；純文案、純註解與沒有可判定斷言的文件整理不做突變。`mutation-check` 技能是跨工具的操作契約；Claude Code 裡用 `skillhub-guards` 的 `mutation_probe` 工具做，它一定還原並逐位元組確認。

## Harness：角色、技能、rules、workflows 與 deny 的放置規則

`AGENTS.md`〈分區指標與攔阻〉只放入口；分層與派工契約見[開發者指示](./agent-instructions.md)，可用的檔案導覽見[Harness README](./harness/README.md)；規則本體在這裡；`.claude/` 是本 repo 的 harness 入口，角色與技能的來源檔在 `.claude/agents/` 與 `.claude/skills/`。**判準：換一個 repo 還成立嗎？** 成立才進 `.claude/skills/`；不成立留在 `docs/`、各層 `AGENTS.md`、`.claude/rules/` 或 deny／CI。`.claude/` 不是整體可攜來源：只有 roles／skills 的來源檔可攜，rules／workflows 是本 repo harness。每一層只放一種東西：

- **`.claude/rules/`**：按路徑觸發的指標，內容只有「先讀哪一份、會被哪個檢查擋」。實測會送達子代理，首次命中注入一次。
- **`.claude/agents/`**：按**風險**切的兩個角色——`skillhub-writer`（寫，路徑範圍由簡報給）、`skillhub-verify`（唯讀驗證）。突變證明不另設角色，用 `mutation_probe`。**角色不按目錄切、不新增**：某個區域該先讀什麼、哪些檔屬於 coordinator，寫在該區域的 `AGENTS.md`，由 rules 按路徑送達。禁令只有一份（`AGENTS.md` 開發自動化第 3 條），角色檔引用不複述。
- **`.claude/skills/`**：換一個 repo 還成立的程序。技能不得引用 ADR 編號或需求 ID——引用了就隨那個編號過期。技能本文不是專案產出物，出現 `docs/` 這類路徑字樣不算引用。**例外是只有 `.claude-plugin/plugin.json`、沒有 `SKILL.md` 的目錄**：那是 Claude Code 的 plugin，放這裡是因為 Claude Code 只從專案的 `.claude/skills/` 自動載入 repo 內的 plugin（接受 workspace trust 之後）。它不算技能：`agent-sync` 不同步給其他工具，也不過 `skillpkg.Validate`。`skillhub-guards` 是唯一一個，對 Bash 與 PowerShell 兩種殼一視同仁。它在操作當下擋下：任何人（主代理與子代理）碰 Git 紅線——`stash`、`reset --hard`、`clean -`、`checkout --`／`checkout .`、`restore`、`add -A`／`--all`／`.`、`commit -a`／`--all`／`--amend`、`push --force`／`-f`、`--no-verify`（引號與 heredoc 內的文字不算），不放行；子代理用旗艦級、沒指定模型或用 fork，以及子代理啟動付費模型堆疊、寫 Git、裝套件、Compose down、跑全 repo formatter、重生 generated 檔或跑 `agent-sync`；手改自稱 generated 的檔與 `.agents/`、`.codex/` 快取；用 heredoc 把含反斜線的內容寫進程式檔；未經負責人同意推送會發佈 runtime 映像的 commit、沒簽名就提交動到 `docs/domain-memory/` 的 commit、改動不是這個 session 留下的未提交檔——這三種由人輸入 `/guard-allow` 放行一次。它也省三件手工：`apps/platform` 的 Bash `go test` 自動帶上測試資料庫、`SKILLHUB_REQUIRE_DB=1` 與 `-count=1`；推送後盯 CI 並在狀態列回報；`mutation_probe`（弄壞、跑測試、一定還原）與 `ship`（comment-lint、逐檔 stage、需要時簽名、只 commit 列出的檔、快轉才推送並盯 CI）兩個工具，子代理不能用 `ship`。**它只在 Claude Code 信任這個 workspace 時載入**；Windows 上同一資料夾可能因磁碟代號大小寫或斜線不同登記成好幾筆，編輯器用的那一筆沒信任時 `claude plugin list` 會說它被 skipped，那時它一條也不會擋。
- **`.claude/workflows/`**：**有固定形狀的多代理程序**——哪些事平行、哪一步驗證、由流程決定是否有 gate——寫成一支 `<name>.js`，以 `/<name>` 呼叫；判斷本身仍在子代理，腳本只決定順序與扇出。`ux-text-audit` 與 `error-path-audit` 都是 Read → Refute → Critic 的唯讀稽核；寫入不做成 workflow，批次修補照[開發者指示](./agent-instructions.md)序列派工。跨工具契約、無 runtime 時的原生派工映射與完成判定見[開發者指示](./agent-instructions.md)；原生腳本的扇出安全不是跨工具保證。**腳本裡的 `agent()` 不指定模型就繼承派工者的旗艦級**，不指定 effort 就沿用派工者的 effort，所以每一個 `agent(` 呼叫都要在同一行寫 `model:` 與 `effort:`（慣例是一行 wrapper：`const run = (p, o = {}) => agent(p, { ...o, model: o.model ?? 'sonnet', effort: o.effort ?? 'medium' })`），`harness` 檢查逐行看。
- **`permissions.deny`**（`.claude/settings.json`）：把根 `AGENTS.md`〈開發自動化〉第 3 條的 Git 紅線變成真的拒絕，清單以該檔為準。對子代理同樣生效、不需 workspace trust，Bash 與 PowerShell 各一份，`harness` 檢查兩份逐條成對。它比對的是命令字串：`git -C <路徑> stash` 不含 `git stash`，擋不到；所以同一組紅線也寫在 `skillhub-guards` 裡，它認得 `-C`、`-c` 與 `rtk` 前綴。每條規則都必須以其目標命令與相鄰合法命令驗證，避免過度攔阻。陷阱：路徑型規則只認 `Edit(...)` 與 `Read(...)`，寫成 `Write(...)`／`Glob(...)` 會被接受但永不被查詢。

**送達與契約**：官方啟動載入規則見 [Agent configuration](https://learn.chatgpt.com/docs/agent-configuration/agents-md)；若有 `AGENTS.override.md` 必須先讀並檢查衝突。派工介面未提供 `cwd` 參數時（例如本次驗證的 `spawn_agent`），代理共享目前工作目錄；工具不保證按檔案自動載入，brief 必須列出 root → 目標父目錄的明確讀取路徑。Codex 指示總量上限 32 KiB（`project_doc_max_bytes`），所以根 `AGENTS.md` 有大小上限。Claude 的額外層是提早發現，真正的保證仍在 `automation-check`、測試與 CI。

**新增一個區域的三步配方**（不新增角色）：①在該目錄放 `AGENTS.md`（指標表：要做的事 → 先讀哪段 → 沒讀會被哪個閘門擋；加一行 `@AGENTS.md` 的 `CLAUDE.md`），遵循[開發者指示](./agent-instructions.md)的分層／派工流程；②在 `.claude/rules/` 加一條 `paths:` 指向它，規則提示改讀該區域 `AGENTS.md`；③在根 `AGENTS.md`〈分區指標與攔阻〉的表加一列。

**守它的機器**：名冊裡的 `harness`（技能不引用本地文件、角色必須指定 `model` 且不得 fable／sol／inherit、角色必須指定 `effort`、角色 `skills:` 預載的技能必須存在、根 `AGENTS.md` 不超過上限、workflow 的每個 `agent(` 同一行有 `model:` 與 `effort:` 且 `meta.name` 等於檔名）與 `apps/platform/internal/shared/skillpkg/repo_skills_test.go`（技能過產品自己的 `skillpkg.Validate`）。

## 常見失敗

| 診斷 | 意義與處置 |
| --- | --- |
| Doctor 顯示 Go／Node 不符 | 切換到原生版本或 Dev Container；不跳過檢查 |
| Windows uv hardlink／CP950 error | 只能走 devctl；它固定 `UV_LINK_MODE=copy` 與 UTF-8 |
| `DRIFT ...` | 修改 source後未重生，執行 `task gen`；不得編輯產物讓 check 變綠 |
| generation already running | 共享工作樹已有 Writer；等對方完成。超過兩小時的 lock由 devctl回收 |
| generated conflict | 合併 OpenAPI／SQL來源後重生；不手動 merge generated code |
| `dev:model` 缺變數 | devctl只列變數名稱。把 secret放 ignored `.env`，不要放 `.env.example` |
| CI 紅了，但 annotation 只有 `Process completed with exit code 1.` | 那一步沒包 [`tools/ci/report-failure.sh`](../../tools/ci/report-failure.sh)，不是查不到原因。診斷順序與會騙人的訊號見 [CI 看不見的失敗](../runbooks/ci-red.md) |
| `gofmt`／`prettier --check`／`golangci-lint fmt --diff` 對你沒碰過的檔案報格式錯，但 CI 的 Linux runner 全綠 | 工作樹是 CRLF checkout。行尾一律 LF，由 [`.gitattributes`](../../.gitattributes) 強制；保留工作樹中的未提交修改，在乾淨 clone 重新檢出或依作業系統的安全流程刷新行尾，再重跑檢查。 |
| postgres 容器起不來，日誌說 `there appears to be PostgreSQL data in: /var/lib/postgresql/data` | 先判斷資料是否要保留。要保留就以相容的舊版本匯出，再以現行映像還原；可拋資料則另起 disposable volume／資料庫後重跑初始化。不要直接刪除共享 volume。 |

### 本機同時跑 SPA 與 API：一個旗標加一個檔案

`cmd/api` 要 `DEV_CORS_ORIGIN=http://localhost:5173` 才會放行前端——**開發時兩者是不同來源，正式環境是同源**，所以這個放行逐行程選擇性開啟、其他地方一律不設（`httpx.DevCORS` 說明了為什麼不用 Vite dev-server proxy 解決）。

另一半是一個檔案而不是旗標：[`apps/web/.env.development`](../../apps/web/.env.development) 只在 `npm run dev` 時把 client 指向 `http://localhost:8080`。`vite build` 跑在 production 模式、不讀它，所以建置出來的 bundle 打的是同源路徑——正式部署與淨測試模式供應的都是那個形狀。**預設之所以是這一邊**：`API_BASE_URL` 在**建置期**解析，沒有人設值時它取到的那個值，會被每一份產物帶著走。`npm run build` 會拒絕一個對外指向絕對來源的 bundle（[`scripts/check-bundle-origins.mjs`](../../apps/web/scripts/check-bundle-origins.mjs)）；刪掉 `.env.development` 只會弄壞 `npm run dev`，其他什麼都不影響。

## `automation-check` 跑了哪些檢查

`go -C tools/devctl run . automation-check` 除了固定的文件字句、`Taskfile.yml` 的 `desc` 與 generated ownership marker 之外，還會跑 `tools/devctl/automation_check.go` 的 `documentCheckers()`：**那個函式就是名冊**，這裡不另抄一份。

**撞到紅燈時的用法**：每一行是 `FAIL <名字>: <問題>`。在 `documentCheckers()` 找到那個名字對應的函式，讀它的實作與測試；它為什麼存在、抓到過什麼，看那個檔的 `git log`。下面的散文只保留有故事的幾條（`capability-table`、`one-number`、`milestone-tally`、`backlog-tally`、`baseline-tally`、`doc-identifier`、`doc-prose`）。

### 新增一個 `.env.example` 變數，要同批說出它擋什麼

`capability-table`（`05` R-36）。促成它的那個變數是
`DOWNLOAD_ARTIFACT_RETENTION`：它在 `.env.example` 裡、程式讀它，而**沒有任何一個地方寫著沒有它就不能打包**。一個不說自己擋什麼的部署變數，在部署當天等於一個沒有人知道要不要填的欄位。

**所以規則是**：往 `.env.example` 加一個變數，同一批就要把它加進 `apps/platform/cmd/api/capabilities.go` 的能力表（說出這個能力少了它會怎樣），**或**加進 `tools/devctl/capability_table.go` 的 `capabilityLedger` 並寫下理由。兩個都沒做，`automation-check` 會 FAIL。

檢查跑三個方向，第二、三個才是讓它保持誠實的那一半：

- `.env.example` 有、能力表與帳目都沒有 → FAIL；
- 能力表宣告了某個變數，`.env.example` 卻沒有記載它 → FAIL（一個查不到的前提，維運人員無從發現）；
- 帳目豁免了一個已經不在 `.env.example` 的變數 → FAIL（帳目會看起來比實際短）。

**它問的是執行檔而不是解析 Go 原始碼**：能力表是 Go（R-36 決定「能力→前提」的宣告放 Go），而 devctl 是另一個 module，所以檢查跑 `go run ./cmd/api --capabilities`，讀那個程式**實際持有**的表——解析原始碼只會在「有人換一種寫法寫那個 literal」之前成立。

**`capabilityLedger` 是存量清單，不是擴充點**，形狀比照 `db/query-owners.yaml` 的 `allow:`：每一列都是既有 `.env.example` 變數，而且**只准變短**。最後一組標著 ⛔ 的變數仍須逐一補上能力說明；帳目只用來揭露尚未完成的歸屬，不能成為豁免理由。唯一能新增的一組，是只由 `cmd/maintenance` 某個 job 讀、缺了就讓那個 job 拒絕執行並點名的變數（例如物件備份的目的地）：API 刻意不持有它們，放進能力表只會讓 API 永遠回報一個它本來就不該有的能力缺席；這類變數另起一組，理由寫明是哪個 job 擁有。

## 同一個數字散在好幾個檔案：`one-number:` 標記

有些值必須在**沒有任何編譯器會比對**的地方保持一致：一個 Go const、契約裡的 `maxLength`、`apps/llm` 的 Pydantic `max_length`、量測 harness 裡的常數。沒有東西把它們綁在一起，所以它們會漂，而且漂掉的時候是在最糟的時刻才被發現。

跨檔副本若未同步，呼叫端與驗證端會對同一輸入套用不同上限，且量測工具可能無法直接辨識差異；因此每一處副本都必須納入同一個可檢查的名冊。

作法：每一個站點在同一行標一個名字，`devctl automation-check` 比對它們。

```go
maxDigestEntry  = 8000 // one-number: maxDigestEntry
```
```yaml
          maxLength: 8000  # one-number: maxDigestEntry
```
```python
    excerpt: str = Field(..., max_length=8000)  # one-number: maxDigestEntry
```

三條規則：

1. **數字與標記同一行**，值取標記之前的最後一個整數。這是它能在四種語法裡運作而不需要任何一種的 parser 的原因。
2. **標記要開啟那個註解**。`# one-number: x - 因為 y` 算；`# 因為 y; one-number: x` **不算**，因為檢查器必須能在所有支援語法中一致辨識標記。
3. **只剩一個站點會 FAIL**。一個站點的不變量保護不了任何東西，而它變成一個站點的方式通常是有人刪掉了別人的標記而不是別人的副本。

**這不取代契約作為事實來源（鐵律 12）。** 每一份副本正確的修法都是「生成它或推導它」，能便宜做到就該做——`packages/api-stub-py` 的那一份就是從契約生出來的，所以它不帶標記。這個機制是給**生成器搆不到的那些副本**。

### 一個數字只能有一個作者

`milestone-tally` 的事實來源與操作規則如下：

- **`03` §19 的 checkbox 就是事實**，機器數 `- [x] GEN-` 與 `- [ ] GEN-`；
- **`03` 自己的節首必須說出那兩個數**（說錯就 FAIL）；
- **其餘四份文件（`AGENTS.md`、`01`、`mvp/README`、`m5/README`）不准出現這個數**——它們該說的是「◐ 的是哪兩項、為什麼」，那才是讀者要的，而且不會漂。

**只管 M5。** `01` 的 M4 那一行（「49 項中 16 勾／33 誠實不勾」）形狀一樣、風險一樣，但**那 49 項的集合只寫在 `m4/audit.md` 的散文裡**，機器既證實不了也否證不了它——**去標記一個查不出對錯的東西，是讓檢查失去讀者的方法**。所以 M4 那一行留著，並在此記明它未經驗證。

### 新增一個共用數字，要同批加進名冊

`one-number` 的保護本來是 opt-in：`found` 是從掃描結果建的，把某個不變量的**全部**標記一起拿掉 → 沒有這個 key → 迴圈不走 → 全綠。這不是理論風險：這些標記寄生在註解裡，`tools/eval-regression` 的一份副本就曾經因為一次註解整理而靜默地不再被計數。

**`tools/devctl/shared_number.go` 持有一份 `sharedNumberRoster` 名冊，雙向強制**：名冊上找不到標記 → FAIL；有標記卻不在名冊上 → 也 FAIL（第二個方向才是讓名冊保持誠實的那一半）。**所以新增一個跨檔共用的數字時，同一批要把名字加進那份名冊，不然 CI 會 FAIL。**

### 殘項的數字必須對得上它自己的清單

`backlog-tally`：根 `AGENTS.md` 指定 `04` 為殘項總數唯一來源。每一個帳目格末尾帶一個 `<!-- open: 26,38,49,... -->`，機器對三件事：

- **粗體的數字等於清單長度**；
- **清單上的每一個 id 都真的是這份文件裡的一列**（一個程式註解曾經引用一列不存在的殘項）；
- **沒有一列被列為未結，卻在自己的狀態欄寫著已結案**。

**它不檢查另一個方向**：未列出的項目是否已結案仍由逐列狀態與人工審查判定；不要把「未列出」當成結案證據。

### 安全基線的規模必須等於它自己的列數

`baseline-tally`。SEC-002 的基線在六個地方自述它有幾項，而唯一有權說話的是那些列。

基線的列、分區表與計畫文件必須由同一份現行清單推導；任何新增、刪除或分區調整都要同步更新其唯一事實來源與機器對帳。

機器對四件事：

- **合計句**（`合計：N 項檢查（阻擋 N 項、告警 N 項）`）等於逐列重數的結果；
- **分區表的每一列**等於該區的列數與等級分佈，**連同它的 `**合計**` 那一列**——那一列不帶區代號，正好是逐區比對看不到的一列，也正好是讀者會相信的那一列；
- 沒有任何 ID 出現兩次；
- `02` **與 `03`** 裡任何一句自述基線規模的話，數字都要是現在這個；能連到清單就不要複寫導出數字。

**兩個刻意的範圍限制**，兩個都是第一次跑出來的：

- **帶日期的句子描述的是該日期**，不納入現行基線對帳；現行文件應連到唯一事實來源，不重述歷史數字。
- **[Sandbox 隔離與執行安全](../adr/README.md#sandbox-隔離與執行安全)那份 ADR 不在被檢查的名單上，而它也不需要在**：它的通過判準寫成「全部測項須全數通過、零項為未知」，一個數字都不重述，所以沒有可漂移的東西。**ADR 的正確寫法就是這一種**——不重述別人擁有的數字，就不必跟著它改。

### 活文件裡的識別字必須存在

同一個家族的第二條。**文件的散文檢查不了，但散文裡的識別字可以**——而一個死掉的識別字，通常是一句死掉的主張穿著它。

識別字檢查只確認活文件中的反引號名稱仍存在於程式樹；不存在的名稱應改成一般文字，外部工具與尚未實作的名稱則依檢查器的 allowlist 規則處理。

**範圍是活文件**（`AGENTS.md`、`docs/plans/01`～`05`、`docs/design/`、`m5/README`，加上 `docs/adr/`、`docs/development/` 與 `contracts/` 三棵樹）；凍結的里程碑報告不在內，避免把歷史證據當成現行識別字。

誤報由 `allowedDocWords` 記著理由，形狀比照 `db/query-owners.yaml` 的 `allow:`——**是存量清單，不是擴充點**。

三條規則：

1. **死掉的名字不要穿反引號。** 反引號的意思是「這是一個真的符號」；訂正句裡提到一個從來不存在的名字時寫成純文字，檢查就不會命中，而讀者看到的資訊完全一樣。
2. **未匯出的名字與欄位名也算。** 反引號裡第一個字母小寫、後面帶駝峰或底線的名字（函式、欄位、設定鍵、原因碼），與匯出名字一樣要存在。沒有駝峰也沒有底線的小寫字仍然是散文，不是符號。外部的 API 與工具名由 `allowedDocWords` 記著理由；尚未實作的欄位與原因碼也記在那裡，並寫出讓它成真的工作項目。
3. **`declared` 是「這個字出現在任何一個程式檔裡」，不是「這個符號有宣告」。** 掃描不使用 parser，並跳過檢查器自己的說明來源，避免自我命中。

### 一句話掉了一個指標之後，要留著它的文法

同一個家族的第三條，抓的是文件整理後留下的語法缺口。常見形狀如下：

```text
而那正是  修好的那個形狀
屬部署期硬化項，見 。
含必補回歸測試）。，該提案
```

**讀者看不出那裡原本有東西，只看得出這句話讀不通**。最糟的一種是連內容一起掉：一個工作項目結束在上面第三種形狀的半句話，後面的 provenance 規則、允收編號與五條測試義務全部不見，而檔案本身沒有任何地方說它不完整。

五個形狀：控制字元、CJK 字元之間的連續兩個以上空白、收尾標點前的空白、連續的標點、開括號後面直接接標點。都是**移除留下的痕跡**，不是排版偏好——`（`、`。`、`，` 在正常的句子裡不會這樣相鄰。

刻意不看三種東西：

1. **圍籬程式碼區塊**（含引言裡的），裡面的空白是對齊。
2. **有製表符號的行**（`─│┌└`），架構圖靠空白對齊，`README.zh-TW.md` 的方塊圖是唯一的實例。
3. **歸位字元**。工作樹的行尾由 `.gitattributes` 決定，Windows clone 的 CRLF 是那台機器的狀態、不是 repo 的內容；把它當成文件缺陷會讓整份檢查在某些機器上一開就紅。

範圍與 `doc-identifier` 不同：**所有活文件**（任何 `.md`），只排除 `docs/plans/mvp/` 與 `tools/goldenset/corpus/`。差別的理由是這條檢查跟內容對不對無關——一個句子在 2026 年是對的，也不會因為時間過去就允許它缺一半文法。

## 跑一次真實的端到端 Run

**成本：一次約 $0.017（mini 級）。會真的花錢。**

`m2/README.md` 也有一份「跑一個 Skill 的最短路徑」，但那是里程碑當時的證據、不回溯修正，**照它逐字做會得到一個沒有網路的 Run**——sandboxd 後來多了一個 fail-closed 的前置。要跑就照下面這份。

### 渲染允許清單後才可使用網路

`SKILLHUB_SANDBOX_NETWORK` 一旦設了非 `none` 的值，`SKILLHUB_SANDBOX_EGRESS_ALLOW` 就**必填**，而且**committed 的 `infra/egress/rendered/egress-allow.json` 是空的**（`allowlist.yaml` 的 `pinned_ip` 仍是 `unset`，那是生產的正確預設）。空清單 ⇒ sandboxd 宣告 `none` ⇒ 沙箱被派到沒有網路的容器 ⇒ Agent SDK 對閘道空等到逾時。

所以 dev 要**另外渲一份**，不要動 committed 的那一份（動它會踩 `egress-allowlist.yml` 的「同 PR 必須改威脅模型」閘門）：

> ⚠️ **下面三行限 Linux／Dev Container 內執行**：`cp -r`、GNU `sed -i` 與 `python3` 都不是本專案的跨平台入口（`Taskfile.yml` 用的是 `python`），而主開發機是 Windows。在 Windows 主機上請進 Dev Container 再跑；**這裡刻意不改寫成一個沒有人實際驗過的跨平台版本**——一條沒跑過的指令比一條標明適用範圍的指令貴。

```bash
# Linux / Dev Container only
cp -r tools infra /tmp/devsrc/            # 一份可改的副本
sed -i 's/pinned_ip: unset/pinned_ip: <litellm 在 skillhub_egress 上的 IP>/; \
        s/fqdn: litellm.internal/fqdn: litellm/' /tmp/devsrc/infra/egress/allowlist.yaml
python3 /tmp/devsrc/tools/egress/render.py --out /tmp/dev-egress
```

比對的規則是 **purpose ＋ port ＋（FQDN 或 pinned IP）**（`sandbox/egress.go` 的 `routes`），所以 `fqdn: litellm` 就夠——平台送的 `url` 是 `http://litellm:4000`。

### 三個程序

```bash
task dev:model                                   # postgres + seaweedfs + litellm
GOOS=linux GOARCH=amd64 go -C apps/sandbox build -o /tmp/sandboxd ./cmd/sandboxd
docker run -d --name skillhub-sandboxd --network skillhub_default --network-alias sandboxd \
  -v /tmp/sandboxd:/usr/local/bin/sandboxd:ro \
  -v /tmp/dev-egress/egress-allow.json:/etc/skillhub/egress-allow.json:ro \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -e SKILLHUB_SANDBOX_TOKEN=devsandboxtoken \
  -e SKILLHUB_SANDBOX_NETWORK=skillhub_egress \
  -e SKILLHUB_SANDBOX_EGRESS_ALLOW=/etc/skillhub/egress-allow.json \
  -e SKILLHUB_SANDBOX_IMAGE=skillhub/runtime-agent-sdk:2026.08-24 \
  debian:12-slim /usr/local/bin/sandboxd
```

**映像版本**：這裡寫的一律等於**部署預設**，不是 `UPGRADES.md` 裡最新的那一版——`apps/sandbox/cmd/sandboxd/main.go` 的 `SKILLHUB_SANDBOX_IMAGE` fallback、`ci.yml` 的 `RUNTIME_IMAGE_FOR_PROBE`（與它 `docker tag` 出來的本地 tag）、`p02_docker_test.go` 的常數與這一行，四處同值。**預設只在 [Sandbox 隔離與執行安全](../adr/README.md#sandbox-隔離與執行安全)裡的四項實測跑過那個 digest 之後才移動**，而那是人工關卡：沒有任何 job 會催它，`UPGRADES.md` 每一節自己寫著那一版跑了沒有。要改這裡的版本，先讀該版那一節。

**交叉編譯而不是在容器裡 `go run`**：這個 repo 的 module 目標版本比多數 `golang:` 映像新，而在容器裡下載 toolchain 只是為了跑一個已經編得出來的二進位。

### 測試程序要跑在容器裡，而且要與 postgres 共用網路命名空間

`TestEndToEndRunCallsTheModelThroughItsOwnVirtualKey` 有一道守門：**`SKILLHUB_TEST_DATABASE_URL` 必須指向 localhost**，因為它會 `DROP SCHEMA public`。而測試同時需要用服務名解析 `seaweedfs`／`litellm`／`sandboxd`——在 Windows 主機上兩者不能同時成立。

**`--network container:skillhub-postgres-1` 一次解決兩邊**：`localhost:5432` 就是那個 postgres（守門的用意完全成立，不是繞過），而 DNS 仍然是 `skillhub_default` 的，服務名照樣解析得到。sandboxd 要推 trace 回來，所以 `SKILLHUB_E2E_PUBLIC_HOST=postgres`——測試的 httptest server 綁在同一個命名空間裡。

```bash
GOOS=linux GOARCH=amd64 go -C apps/platform test -c -o /tmp/e2e.test \
  ./internal/entrypoint/api/apiserver
docker run --rm --network container:skillhub-postgres-1 \
  -v "$PWD:/src" -v /tmp/e2e.test:/usr/local/bin/e2e.test:ro \
  -w /src/apps/platform/internal/entrypoint/api/apiserver \
  -e SKILLHUB_TEST_DATABASE_URL="postgres://skillhub:skillhub@localhost:5432/skillhub_test?sslmode=disable" \
  -e OBJSTORE_ENDPOINT=seaweedfs:8333 -e OBJSTORE_ACCESS_KEY=skillhubdev \
  -e OBJSTORE_SECRET_KEY=skillhubdevsecret -e OBJSTORE_BUCKET=skillhub -e OBJSTORE_SSL=0 \
  -e SKILLHUB_E2E_SANDBOX_URL=http://sandboxd:9000 -e SKILLHUB_E2E_SANDBOX_TOKEN=devsandboxtoken \
  -e SKILLHUB_E2E_GATEWAY_URL=http://litellm:4000 \
  -e SKILLHUB_MODEL_GATEWAY_URL=http://litellm:4000 -e SKILLHUB_MODEL_GATEWAY_KEY="$LITELLM_MASTER_KEY" \
  -e SKILLHUB_RUN_MODEL=gpt-5.4-mini -e SKILLHUB_E2E_PUBLIC_HOST=postgres \
  -e DEV_LOGIN=1 \
  debian:12-slim /usr/local/bin/e2e.test \
  -test.run TestEndToEndRunCallsTheModelThroughItsOwnVirtualKey -test.v -test.timeout 20m
```

**`-w` 那一行是必要的**：測試以相對路徑 `../../../../../../db/migrations` 找 migration。

**`SKILLHUB_E2E_GATEWAY_URL` 與 `SKILLHUB_MODEL_GATEWAY_URL` 兩個都要設**，即使值相同：前者是交給沙箱的位址，後者是測試自己用來簽 Virtual Key 的管理位址；少了前者測試在派送之前就 `Fatal`。同一支測試以 `source_path` 參數化，帶值時跑的是「Plugin 內的一個 Skill」那條路徑。

**映像的四項行為實測要用同一段命令，但 `sandboxd` 的 `SKILLHUB_SANDBOX_IMAGE` 要指 CI 發佈的 digest**（不是本機建置——那等於白跑）；`usage` 事件那兩支在 `apps/sandbox/internal/dockerdrv`，交叉編譯後在容器裡跑，要 `SKILLHUB_E2E_GATEWAY_URL`／`_KEY`、`SKILLHUB_E2E_EGRESS_NETWORK` 與 `SKILLHUB_E2E_RUNTIME_IMAGE`。對帳讀閘道自己的 `LiteLLM_SpendLogs`：`/spend/logs` 的日期參數在現行閘道上會回空集合。

**`DEV_LOGIN=1` 也是必要的**：`execution.Match` 比對的是隔離強度，這台 sandboxd 跑 runc、自報 `weak`，只有標成開發部署的程序才接受低於強隔離的節點，否則每個 Run 都是 422「which this deployment does not accept」。這不是繞過——它就是那條規則為開發機留的門，生產部署不設它。

**Git Bash 跑上面這段時**：MSYS 會把以 `/` 開頭的參數與環境變數值改寫成 Windows 路徑（`-e X=/etc/foo` 進容器變成 `C:/Program Files/Git/etc/foo`）。整段前面加 `MSYS_NO_PATHCONV=1`，或把容器內路徑寫成 `//etc/foo`（Linux 把雙斜線當單斜線）。掛載來源用 `C:/...` 的寫法就不會被動。

### 它證明了什麼、沒證明什麼

**證明**：套件進物件儲存 → preflight → 確認 → 派送 → sandboxd → 容器跑 Agent SDK → 每 Run 短效 Virtual Key 經閘道呼叫模型 → trace 回推 → artifact 收集 → 金鑰撤銷，整條是通的。

**沒證明**：這一輪的 runtime 是 `runc` 不是 `runsc`，所以它不是 SEC-009 的任何一項；`--network skillhub_egress` 是 Docker 網路隔離，**不是** [Sandbox 隔離與執行安全](../adr/README.md#sandbox-隔離與執行安全)那項 nftables 強制層決策（那條路的實驗室在 `tools/sec009/t5-network-egress.sh`）。

### 什麼時候要跑它（`05` R-72）

**它不在 CI，而且刻意不排程。** 排程要一把能用的閘道金鑰放在 GitHub secret 裡，而鐵律 11 的形狀是「供應商金鑰只存在閘道」——為了一個每週一次的檢查，在閘道之外多開一個金鑰存放點。裁定取的是兩個**由人按、但綁死在事件上**的時刻：

- **(b) 發布前**：[release-checklist §2.7](../plans/mvp/m4/release-checklist.md) 有一列，勾選前必須附閘道回報的實際金額。
- **(c) 改到就跑**：`apps/sandbox`、`apps/llm`，或派送路徑（`apps/platform/internal/trial/` 的 Run 狀態機與 Worker）**有改動時**，用上面那段配方手動跑一次。這三個位置是這條線斷掉時唯一會動到的地方。

**為什麼不能像其他依賴那樣「跳過就是紅」**：`SKILLHUB_REQUIRE_DB`／`_OBJSTORE`／`_CREATION_PYTHON` 三個都做成了缺依賴即失敗，**唯獨這一支不行，因為它會花錢**。

**接受的殘留風險，明寫**：這條線的保證從「機器會檢查」降成「兩個明確的時刻有人記得」。它斷掉的樣子是**派送成功、Run 永遠不完成**——`web` 與 `platform` 兩個 job 全綠，沒有任何一盞燈會變色。若封測期間需要更硬的保證，正解是**在節點上跑**（節點本來就要有閘道金鑰），不是在 CI 裡多放一把。

## 推送前與推送後：pre-push hook、`task preflight`、`task ci:status`

**pre-push hook**：`task bootstrap` 會把 `core.hooksPath` 指到 `.githooks/`，`pre-push` 跑 `devctl preflight --hook`。它只檢查**這次要推的 commit**，而且讀的是 commit 裡的位元組（`git show <sha>:<path>`）而不是工作樹，所以共享工作樹裡別人未提交的修改擋不到你的推送。它查五件事，前四件 CI 也會紅：

- 推送範圍內改到的 `apps/platform`／`apps/sandbox` Go 檔（gofmt）、`apps/web` 檔（prettier）、`apps/llm` Python 檔（ruff format）有沒有照 CI 的格式；
- 推送範圍內改到、`comment-budget` 會掃的檔，註解有沒有超出預算（與 `automation-check` 同一份判斷）；`automation-check` 的其他項目讀工作樹，hook 不跑，CI 仍會查；
- 推送範圍內改到、又被 Domain Memory 引用的檔，引用以 commit 裡的內容看還是不是 current（`verify-evidence --path`，stale／missing 就擋）；跑不了這個工具時只印 `WARN`；
- `infra/images/runtime-agent-sdk/` 現在的映像內容，與**目前 `ARG IMAGE_VERSION` 這個版本當初發佈時的內容**不一樣（I-05）。判準是「HEAD 跟那個版本被寫進 Dockerfile 的那一次 commit 相比，有沒有任何會被 `COPY` 進映像的檔不同」，不是「這次推送改了什麼」——所以**把誤改還原就會自然變綠**，不必為了消紅而 bump 一個沒人量過的版本；反過來，分好幾次推送慢慢累積的變更也躲不掉。這段判斷與 `Runtime Image` workflow 呼叫的 `devctl image-gate` 是同一份程式，兩邊不會再各說各話；沒被複製進映像的檔（例如 `run.test.mjs`）不觸發。
- 推送範圍內改到 `.claude/skills/skillhub-guards/` 時跑 `claude plugin test`，失敗就擋。**CI 不跑這些測試**，這是它們唯一的自動關卡；這台機器沒有 `claude` 時只印 `WARN`，工作樹與推送的 commit 不同時也會提醒測試跑的是工作樹。

格式工具不在這台機器上時只印 `WARN`、不擋，CI 仍會查。**不要用 `--no-verify` 跳過**（`.claude/settings.json` 的 deny 已擋）；hook 誤擋就修 `tools/devctl/preflight.go`，不要繞過它。

**`task preflight`**：手動版，範圍是 `@{upstream}..HEAD`，另外多跑一次 `automation-check`。後者讀的是工作樹，所以別人未提交的檔可能讓它紅——看 `FAIL` 的檔名判斷是不是你的。

**`task ci:status`**（`devctl ci-status [ref] [--wait]`）：列出該 commit 的**每一個** workflow run、失敗的 job 與 step、以及被 path filter 跳過而沒跑的 job。token 用 `git credential fill` 取得、只放在記憶體裡，所以不吃未驗證呼叫每小時 60 次的上限——幾個工作階段同時輪詢時，那個上限幾分鐘就會用完。結束碼：0 綠、1 紅（`cancelled` 也算，因為那個 commit 沒被驗到）、3 還在跑、4 還沒有 run。

**CI 的形狀**（`ci.yml`）：

- main 上的 push 以 commit SHA 分組且不互相取消；只有 PR 會被同一個 PR 的下一次 push 取消。每個未被取消的 run 都必須完成自己的驗證。
- `images` 有自己的 path filter（見[Repo 結構、CI 與驗證層](../adr/README.md#repo-結構ci-與驗證層)），純文件 commit 不再建置、smoke、推送三個服務映像。它不再等語言 job，與它們並行；推送拆成 `images-push`，等待既有程式、工具、契約、依賴檢查與四個原生瀏覽器彙總閘門通過才推，從同一次 run 的 GHA 快取重建，不重新編譯。
- 每週日一次 `schedule` 全量跑（所有 path filter 視為命中），`workflow_dispatch` 也是全量——手寫 path filter 漏掉的那一格由它兜底。
- 每個 job 都有 `timeout-minutes`，一個卡住的 job 不會再佔滿預設的 6 小時。
- 瀏覽器測試使用 Playwright 原生分片：Linux WebKit 四片，Linux Chromium／Firefox 與 Windows Chromium 各兩片，保留完整套件與既有逾時；失敗產物按環境、分片與總片數命名。原有四個 `web-browser` 檢查名稱保留為彙總閘門，只有全部分片成功才通過，分片失敗、取消或缺席都不得放行。
- 瀏覽器的 baseURL、就緒探測網址與預覽監聽位址使用同一個明確的 IPv4 loopback，不依賴 localhost 的位址族解析順序。
- `web` 與瀏覽器分片的依賴安裝使用 `npm ci --no-audit`，避免每個安裝步驟重複查詢漏洞；獨立的 `dependency-audit` 仍執行完整的既定稽核並在失敗時擋下交付。
- 每次都跑、沒有 path filter 的有三個並行 job：`devctl`（devctl 自己的 vet／race 測試、`automation-check`、`agent-sync`）、`contracts-drift`（`gen --check` 與 `contracts/` 的各項檢查）與 `dependency-audit`（見下一條）。純文件 commit 的等待時間由三者中最慢的那個決定，不再是相加。
- **依賴漏洞**（見[開發自動化與依賴治理](../adr/README.md#開發自動化與依賴治理)）：push／PR 跑 `devctl dep-audit`，只看會出貨的四個專案——`apps/web` 的 production 依賴（high 以上）、`apps/platform`／`apps/sandbox` 程式呼叫得到的漏洞、`apps/llm` 的非 dev 依賴——而且**只擋有修補版的**，沒有修補版的印成 `NOTE`。每週排程與 `workflow_dispatch` 改跑 `--full`：每個有 lockfile 的專案連 dev 依賴、npm 取 moderate 以上、Go 連沒呼叫到的模組也算，紅了就是那一週的報告。`images-push` 等它綠了才推。本機 `task deps:audit`（加 `-- --full` 全掃）；同一條命令也擋出貨依賴的授權與 workflow 的 zizmor 稽核（用法見〈依賴的准入、更新與閘門〉）；job 帶 `GH_TOKEN`，zizmor 連網的稽核項目只在 CI 跑。工具版本在 `tools/toolchain.yaml` 的 `govulncheck`／`pip_audit`／`go_licenses`／`zizmor`。
- `golangci-lint` 由 [`.github/actions/golangci-lint`](../../.github/actions/golangci-lint/action.yml) 安裝：版本讀 `tools/toolchain.yaml` 的 `golangci_lint`，編好的 binary 以「版本＋OS＋Go 版本」為鍵另外快取。不能指望 `setup-go` 的快取帶著它——那份快取的鍵只有 `go.sum` 的雜湊，第一次存下之後內容就不再更新，`go.sum` 沒動過的模組會一直拿到那天的舊內容。
- `sandbox` 的 filter 只看 `infra/images/runtime-agent-sdk/**`：其他服務映像與 `infra/images/` 下的說明文件不影響 sandbox 的任何測試。`infra/nodes/gvisor-baseline.txt` 與 `infra/deploy/sandbox/daemon.json` 也在 filter 裡：Suite 1 安裝的就是基準檔指名的 release、註冊 runsc 用的就是那份 runtimeArgs，換基準不跑 sandbox 等於沒驗新版。

**Runtime Image**（`runtime-image.yml`）：發佈前先查 registry 有沒有這個版本的 tag，**有就只跑閘門、不推送、不移 tag**。版本 tag 一旦發佈就不再變，因為 [Sandbox 隔離與執行安全](../adr/README.md#sandbox-隔離與執行安全)那項決策的事實來源是 digest，而 build 不是位元可重現的——同版重推會讓同一個版本字串悄悄指向另一份沒量過的映像。attestation 失敗會在同一個 run 裡自動重試一次；發佈中的 run 不會被下一次 push 取消。

**Image Scan**（`image-scan.yml`）：compose 拉下來的映像與我們自己建的四顆各出一份 SBOM 與 grype 報告，判準與不對稱的閘門見〈依賴的准入、更新與閘門〉。`runtime-agent-sdk` 不在這一支裡，它有自己的 `runtime-image.yml`。

## 依賴的准入、更新與閘門

決策與理由在[開發自動化與依賴治理](../adr/README.md#開發自動化與依賴治理)（漏洞與其餘依賴治理規則）；這裡只放動手時要知道的事。

**新增一個直接依賴之前**，回答五個問題，答案寫進 commit message：標準庫、平台或已經裝的依賴做得到嗎（做得到就不加）；授權在允許清單上嗎；還有人維護嗎（最近一年有發佈、安全問題有回應）；會多拉進幾個傳遞依賴；需要 install script 嗎（`ignore-scripts=true` 會讓它失效，需要就寫 ADR）。`apps/web` 的執行期依賴另外要一份 ADR（system.md §4.8，前例見[營運後台](../adr/README.md#營運後台)）。

**這些版本要一起動**。Dependabot 會提出升級，但通常只改到其中一處；node、go、python、uv、task、golangci-lint 由 `dependency-policy` 比對每個位置，不一致就 FAIL 並列出各位置的值（見[開發自動化與依賴治理](../adr/README.md#開發自動化與依賴治理)）：

| 版本 | 同時要改的地方 |
| --- | --- |
| uv | `tools/toolchain.yaml` 的 `uv`、`tools/codegen/python/Dockerfile` 的 uv 映像、llm 與 devtools 映像的 `UV_VERSION` 與安裝腳本雜湊；`uv_build` 的上限跟著 uv 的 minor |
| Node | `.node-version`（CI 讀它）、web 映像的 node、devtools 映像的 `NODE_VERSION` |
| Go | 每個 `go.mod` 的 `go` 行（CI 的 setup-go 讀它）、各 Dockerfile 的 golang 映像 |
| Python | `apps/llm/.python-version`（CI 讀它）、三份 `pyproject.toml` 的 `requires-python`、llm 與 codegen 映像、devtools 映像的 `uv python install` |
| task、golangci-lint | toolchain.yaml 與 devtools 映像的 `ARG` |
| compose 與 CI 共用的映像（pgvector、seaweedfs） | Dependabot 只改 `infra/compose/docker-compose.yml`；同一個 PR 要把 `ci.yml` 與 `tools/ci/stack-smoke.sh` 裡的同一個映像改成一樣，否則 `dependency-policy` 會擋 |
| datamodel-code-generator | `tools/codegen/python/pyproject.toml`、`uv.lock`、toolchain.yaml 的版本與 `python_codegen` 映像標籤，然後 `task gen:openapi` |
| ogen | `tools/codegen/go/go.mod`、toolchain.yaml 的 `ogen` 與 `go_codegen` 映像標籤，然後 `task gen:openapi` |
| `@types/node` 的主版本 | 跟 `.node-version` 的 Node 主版本一致；Dependabot 只提 minor／patch |
| pglite | `tools/pglite/package.json`、toolchain.yaml 的三個 `pglite*` |
| Agent SDK | [Sandbox 隔離與執行安全](../adr/README.md#sandbox-隔離與執行安全)的四項重驗，`UPGRADES.md` 一節 |
| 稽核工具 | toolchain.yaml 的 `govulncheck`、`pip_audit`、`go_licenses`、`zizmor` |

**Dependabot 的 PR**：每週一（npm、Go、Python）與每月（Actions、映像、compose）各開一個群組 PR，major 另開。新版本要先存在 7 天才會被提（npm 的 major 14 天）。CI 綠了就能合；major 先讀 changelog。PR 只改到上表其中一處時，`dependency-policy` 會紅並列出其餘位置：在同一個 PR 補齊，不要關掉。

**`devctl dep-audit`**（本機 `task deps:audit`，全掃加 `-- --full`）一次跑三件事，任何一件出現 `FAIL` 都會紅：

- 有修補版的漏洞 → 升到訊息裡的版本。
- 出貨依賴的授權不在允許清單 → 換一個依賴。確認授權其實可以接受（例如分類器認不出的 MIT 變體）時，在 `tools/devctl/license_audit.go` 的 `acceptedLicenses` 加一筆，附上理由，並在 commit message 說明你讀過的授權原文。
- zizmor 的 medium 以上 → 照訊息裡的連結修 workflow。本機沒有 `GH_TOKEN` 時只跑離線稽核，CI 會多跑連網的幾項。

npm registry 對 `npm audit` 回 5xx 時，`dep-audit` 會再問，最多三次、間隔 15 秒與 30 秒；三次都是 5xx 就以「稽核沒有執行」失敗，不會當成沒有漏洞。看到這句就是 registry 故障，稍後重跑即可，不是你的 diff。

Go 全掃仍保留模組層級 advisory。有修補版仍是 `FAIL`；沒有修補版且已匯入受影響套件時是 `NOTE`。只有 source 掃描明確完成 package／symbol 分析、且 advisory 僅有模組層級證據時，才以 `INFO` 說明受影響套件未匯入；缺少分析能力資訊時不作這個判定。同一筆 advisory 有多份證據，以較高可達性為準。已停止維護的 OpenPGP 套件另外由 Go 出貨依賴檢查禁止引入，不以安裝另一個未使用的套件來取代。

**映像不在 `dep-audit` 裡，它讀的是 lockfile**（見[開發自動化與依賴治理](../adr/README.md#開發自動化與依賴治理)）。 映像由 [`tools/ci/scan-images.sh`](../../tools/ci/scan-images.sh) 掃，掛在 `image-scan.yml`（改到 compose 或四個應用映像時跑、每週排程、可手動觸發），`runtime-agent-sdk` 仍留在自己的 `runtime-image.yml`。**清單從 compose 讀出來，不另抄一份**——抄一份就會漂，而這個缺口本來就是漂出來的。

**閘門分三層，照「誰能修」與「會不會被部署」畫**：會被部署的 `platform`／`web`／`llm` 出現**有修補版的** Critical／High 就紅（修法在我們手上：換基底、升依賴，或像 `runtime-agent-sdk` 與 `llm` 那樣對單一套件 `--only-upgrade`）；compose 拉下來的上游四顆只報，**只有排程那一跑設 `FAIL_ON_UPSTREAM=1` 才紅**，所以每週有人被通知而沒有人的 PR 被擋，處置是**升那個 pin 或換 tag**，不是把閘門關掉；開發容器 `devtools` **永不紅**——它不部署，而它兩萬多列的發現絕大多數住在第三方工具的編譯產物裡，擋了也沒有人做得出對應的修正。

**automation-check 的 `dependency-policy`** 擋的是：FROM、compose 與 workflow 的 `image:` 沒釘 digest；`uses:` 沒釘 SHA 或少了 `# vX` 註解；npm 專案少了 `.npmrc` 的 `ignore-scripts=true`；uv 專案少了 `exclude-newer`；新目錄沒列進 `.github/dependabot.yml`；compose 與 workflow、`tools/ci/*.sh` 裡同一個映像的 tag 或 digest 不一樣；上表同一個工具在各位置的版本不一致（名冊在 `tools/devctl/toolchain_versions.go`，syft 與 grype 的 pin 也在上面，所以兩支 workflow 的掃描器版本不會各走各的）。

## 完成判準

一次 automation 變更至少通過：

- `go -C tools/devctl test ./...`
- `go -C tools/devctl run . automation-check`
- `task gen:check`
- **`task format:check`**
- `task preflight`（推送時 pre-push hook 會自動跑它的 `--hook` 版）
- 受影響語言的 typecheck/test/build
- `git diff --check`

**為什麼把 `format:check` 單獨列出來**：typecheck/test/build 不涵蓋格式檢查；`git diff --check` 也只看行尾空白與衝突標記，不能取代 formatter。

**怎麼裝 `golangci-lint`，以及為什麼不能照著它官網那一行裝**：

```bash
GOTOOLCHAIN=go1.27.3 go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.14.0
```

版本 `v2.14.0` 來自 [`tools/toolchain.yaml`](../../tools/toolchain.yaml) 的 `golangci_lint`，CI 的 [`.github/actions/golangci-lint`](../../.github/actions/golangci-lint/action.yml) 讀同一個欄位、跑同一個 `go install`；模組代理與 sumdb 會驗 checksum。**`GOTOOLCHAIN=go1.27.3` 前綴是必要的**，因為 lint 設定的目標 Go 版本必須不高於建置 lint 的 Go 版本；本機工具鏈則由 `GOTOOLCHAIN=auto` 取得相容版本。

裝完之後 `go env GOPATH`／`bin` 要在 `PATH` 上，`devctl doctor` 的 `golangci-lint` 那一列才會 PASS。**那一列從一開始就在 doctor 裡**——[開工守則第 1 條](../../AGENTS.md)「先診斷再修改」指的就是這件事：格式紅燈的成因通常不是缺工具，是沒有人先跑 doctor。

**真的裝不起來時的退路是 `gofmt -l ./apps/ ./tools/`**——它隨 Go 工具鏈一起來，一定在；輸出**任何一個檔名就是未通過**（`golangci-lint fmt` 的 Go 部分預設就是 gofmt ＋ goimports，所以 `gofmt -l` 乾淨時剩下的差異只會是 import 分組）。**但它是退路不是等價物**，而且 `task format:check:platform` 在沒有那支指令時會失敗於「找不到指令」，**那看起來很像「檢查過了」**。

版本／generator／Task入口異動要同步更新**本文件**、`tools/toolchain.yaml`、相關 package README與 CI；**`AGENTS.md` 只在紅線本身增刪時才動**（它不複製版本、命令清單與生成來源表）。工具能跑但新 Agent找不到，視為未完成。

## 一個新 Agent 找不到的東西，就是還沒完成的東西

這份手冊的驗收方式是黑箱：給一個沒有前文、唯讀的低成本 Agent 一個任務，不提示 task 或檔名，確認它能走到正確入口。找不到入口時，先補文件或 `automation-check` 的缺口，再宣稱完成。

`.devctl/**` 是暫存產物、不是可提交的 API；Git 寫入只由整合的主 Agent 做，SubAgent 不切 branch。
