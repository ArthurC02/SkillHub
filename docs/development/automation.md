# 開發自動化與 Coding Agent 協作

本文件是 ADR-030 的操作手冊，也是**開發自動化規則與操作的完整事實來源**：命令、檔案所有權、能力與成本分級、生成來源表與排錯都在這裡。`AGENTS.md` 只放紅線與入口連結，不複製本檔內容。兩者不另造第二套流程，所有入口最後都走 `Taskfile.yml` 與 `tools/devctl`。Platform 的 Bounded Context 日常判斷見 [DDD 實務指南](./platform-ddd-practices.md)。

## 新電腦的最短路徑

### 建議：Dev Container

1. clone repo；
2. 以 `.devcontainer/devcontainer.json` 開啟；
3. post-create 會安全建立 `.env`（已存在就不碰）並 bootstrap 依賴；
4. 執行 `task doctor` 與 `task gen:check`。

Dev Container 以 privileged mode 啟動獨立 DinD daemon，讓 Windows／macOS／Linux 的 nested generator 都看到同一個 `/workspace`；其 images/volumes 不借用 host daemon。Privileged container仍是高權限，只對可信任的 SkillHub repo 使用，細節見 `.devcontainer/README.md`。它不執行不受信任 Skill，也不取代 gVisor 部署驗收。

此處不用 host socket是實測結論：Docker-outside-of-Docker會讓內層 daemon把 container的 `/workspace` 當成物理主機路徑，第一次 clean-container `gen:check` 因此找不到 sqlc config。DinD修正後，以預設非 root `vscode` 使用者啟動、獨立 `/var/lib/docker` volume重跑 doctor與四個 generator，全部通過且零 drift。Python／Go generator image各自使用 `tools/codegen/<lang>` 作最小 build context；不得改回 repo root context，否則 builder會遍歷無關 Dataset／Trace sample與本機依賴目錄。

### Native fallback

新機器沒有 Task 時：

- `go -C tools/devctl run . doctor`
- `go -C tools/devctl run . env-init`
- `go -C tools/devctl run . bootstrap`

語言版本由 `go.mod`、`.node-version`、`.python-version` 擁有；其他工具與 generator image 由 `tools/toolchain.yaml` 擁有。不要從 README 文字或 Agent 記憶抄版本。

**釘選的版本要有人去啟用它，否則 doctor 的 FAIL 是唯一的提醒**：`.node-version` 只是一個寫著數字的檔案，不會讓任何 shell 換版本。2026-09-10 這台開發機上 `devctl doctor` 的 node 那一列長期是 FAIL——CI 跑 v22.14.0，本機跑 v25.0.0，而**版本釘選存在的理由就是不要有這條落差**。查下去發現 `fnm` 早就裝好、22.14.0 也早就是它的 default，**只是沒有任何一個 shell 在啟動時呼叫它**；Windows 上還多一層：機器層 `PATH` 排在使用者層前面，所以 `C:\Program Files\nodejs\` 一定贏過任何使用者層的 shim 目錄。修法是在 shell 啟動檔裡掛 `fnm env --use-on-cd`（`--use-on-cd` 讓它跟著目錄走，不會把整個帳號釘死在一個版本），**不是再裝一次 Node**。Git Bash 這邊還要注意：Agent 的每一次 `bash -c` 都是非互動 shell，`~/.bashrc` 只透過 `BASH_ENV` 才會被讀到，而那表示**帳號上每一個非互動 bash 都會源它**——所以那個檔案裡任何一行輸出都會混進別的腳本的 stdout，必須全程沉默。

**換過 Node 版本之後 `node_modules` 不算數**：舊的樹是在舊版本底下裝的，要重跑 `task bootstrap` 再驗一次，否則綠燈只證明「在另一個 runtime 上是綠的」。

## 能力與成本分級

| 入口 | Docker | Secret | 可能花錢 | 作用 |
| --- | --- | --- | --- | --- |
| `task doctor` | 只檢查 | 否 | 否 | 診斷 runtime、Task、uv、Docker、Compose 與 `.env` 是否存在；不讀值 |
| `task env:init` | 否 | 否 | 否 | `.env.example` → `.env`，已有檔案時不覆寫 |
| `task bootstrap` | 否 | 否 | 否 | Go download、npm ci/build、uv frozen sync |
| `task dev`／`dev:core` | 是 | 否 | 否 | Postgres＋SeaweedFS |
| `task dev:model` | 是 | **是** | 後續模型呼叫會 | 先 fail-closed 檢查 OPENAI/LiteLLM 變數，再啟動 gateway |
| `task dev:observability` | 是 | 否 | 否 | `docker compose --profile observability … up -d prometheus`：起一個**開發機**的 Prometheus，讓 `infra/observability/alerts.yml` 真的被求值（不是生產部署） |
| `task down` | 是 | 否 | 否 | `docker compose … down`：停掉本機基礎設施（不刪 volume）。**它停的是共享的那一組**，唯讀／寫入 SubAgent 一律不得自行執行 |
| `task gen*` | 是 | 否 | 否 | 固定版本 generator；apply 會改 generated files，check 只寫 `.devctl/` scratch |
| `task test`／`task ci` | 視測試 | 否 | 否 | 預設不執行 E2E 模型測試；需 secret 的測試維持 opt-in gate |

`.env.example` 內非空的 Postgres／SeaweedFS 值是公開的 local-only placeholder；production secret 一律留空。不得把 `.env`、key 值或 doctor 以外的環境 dump 放進回覆或 log。

「不需 secret／不花模型費」不等於 offline：第一次 bootstrap 會下載 Go/npm/uv 依賴，第一次 generation 會拉 digest-pinned image，Redocly 與 container build亦需 registry／package mirror。lockfile、version與digest確保內容可重現，不宣稱斷網仍能從空 cache 建置；隔離環境應提供核准的 mirror或事先填好的 cache。

## 生成來源與所有權

| 人工修改來源 | 生成目標（不得手改） | 入口 |
| --- | --- | --- |
| `db/migrations/**`、`db/queries/**`、`db/sqlc.yaml` | `apps/platform/internal/foundation/persistence/db/gen/**` | `task gen:sql` |
| `contracts/openapi/public.yaml` | `apps/platform/internal/entrypoint/api/gen/**`、`packages/api-client-ts/src/generated/**` | `task gen:openapi` |
| `contracts/openapi/llm-internal.yaml` | `packages/api-stub-py/src/skillhub_api_stub/generated/**` | `task gen:openapi` |

`.devctl/**` 是 gitignored lock／scratch，不是可引用、可提交或可手改的 generated API。Generator 失敗時只查看它作診斷；正式產物只以上表為準。

`task gen` 的順序：取得單一 Writer lock → 在 repo 同檔案系統 scratch 完整生成 → 檢查絕對路徑／timestamp → 比對完整 tree → apply 時原子替換。任何一步失敗都不應留下半套 tracked output。`task gen:check` 不修改 tracked files，CI 使用同一路徑。

Generator upgrade 必須獨立 commit／PR，同時更新 manifest、generator lock、generated output並審查語意 diff。不得使用 `--skip-validate-spec`、OpenAPI 3.0 shadow copy或 generated-file workaround。

## 新增 API 的安全順序

1. 先改 OpenAPI schema／operation；若需要資料，再改 migration/query。
2. 由主 Agent（單一 Writer）執行 `task gen`；SubAgent 不自行生成。
3. 實作 Go domain policy／service／adapter。Workspace 取自 session，不接受 UI 傳入 scope。
4. Web 保留 `apps/web/src/api/types.ts` 的 UI view model，逐 endpoint 寫 generated DTO adapter，不整檔替換。
5. Go 的 ogen server目前只在 `router.go` 精確 `GET /healthz` pattern後使用。新增 endpoint 仍必須在 `router.go` 明確保留原本 `RequireSession`／`RequireOperator`／`OptionalSession` 語意，不 mount整個 generated server。
6. 跑 scoped tests、`task gen:check`，最後跑 `task ci`。

## 共享工作樹與 SubAgent

- 目前不要求 worktree。
- 主 Agent 是唯一協調者與 Writer；SubAgent 預設唯讀。
- 寫入 SubAgent 需要精確 path allowlist。它不切 branch、不 stash、不 stage/commit/push、不安裝 package、不更新 lockfile、不執行 repo-wide formatter／generator，也不啟停共享 Compose。
- 唯讀 SubAgent 可以平行；寫入、generator、formatter、package manager、migration、contracts、CI/Taskfile 全部序列化。
- 未知 delta 視為他人工作：不得 reset、clean或 checkout還原。
- 只有負責整合的主 Agent執行明確 pathspec stage、commit、pull --rebase與 push。
- **子代理的模型按「這件事需要多少推論」選，不按名字選**（負責人 2026-09-04 明定：禁最高階之後反射性地全派次高階，不是正確做法）。每次派工明確指定；從能做完的最低一級起，升級要在 brief 裡寫理由。規則用特性描述，因為模型名稱會換、而且本 repo 同時有多個 coding agent 在協作；名稱只出現在「現行對應」一欄，換名只改那一欄：

  | 等級 | 特性 | 派給它的事 | 現行對應（Claude Code／Codex） |
  | --- | --- | --- | --- |
  | **旗艦** | 主線代理自己用的等級 | **子代理禁用**——它是派工者，不是被派的 | Fable／Sol |
  | **深度推論** | 善用推論思考，會停下來判斷 | 跨模組推理、規格含糊要判斷、安全或資料遺失路徑、對抗性審查 | Opus／Terra |
  | **快速執行** | 執行快、推論不多 | 規格與邊界都清楚的多檔實作、照分區卡與現成模式做、驗證回報、文件整理 | Sonnet／Luna |
  | **機械執行** | 推論極少，正因如此會像機械操作一樣照目標做完 | 規格完全明確、有測試判對錯：改一行跑一條測試、抄表、查值、突變稽核 | Haiku／Luna |

  Luna 介於快速執行與機械執行之間，Codex 側這兩級都派它。

  Claude Code 的三個角色檔（`.claude/agents/`）以機械執行／快速執行為 frontmatter 預設下限，派工時可指定更高；`automation-check` 的 `harness` 檢查擋角色檔出現旗艦級（名稱寫在檢查器裡，那是唯一需要名字的地方）。可攜角色與 skills 的唯一來源是 `.claude/agents/`、`.claude/skills/`；`task agents:sync` 在本機與 CI 產生 `.agents/skills` 與 `.codex/agents`，兩者是忽略的快取，永不手改、永不提交。新一輪 Agent 工作前由唯一 Writer 同步，已啟動的 Agent 必須重新派送才會讀到新版本。
- **Claude Code 另有一層攔阻**（`.claude/settings.json` 的 `permissions.deny` 把 `stash`／`add -A`／`commit -a`／`restore`／`checkout .`／`reset --hard`／`clean`／`push --force`／`commit --amend` 變成真的拒絕，對子代理同樣生效），**但它只是提早發現**：其他 coding agent 不受它管，本節的規則本體與 `automation-check`、測試、CI 才是保證。角色與技能的放置規則見下方〈Harness〉。

## 修好一個東西之後，把修法弄壞一次：三次前例

`AGENTS.md` 開發自動化第 9 條的來歷。改完跑一次測試是綠的，只證明測試存在，不證明它會紅。本專案已經三次「修好了但測試沒有牙齒」：

1. invalidate 用錯 query key——改回錯的，171 支全綠。
2. placeholder 六條規則有三條零正面測試——同時刪掉，全綠。
3. 兩條 import 路由的速率限制包裝——無聲刪掉，全套綠。

**三次都不是 code review 找到的**，是突變找到的。成本是一次 `sed` 加一次 `go test`。程序：把修正那一行還原、跑對應的那一條測試、確認變紅、改回來、`git diff` 確認為空。不適用於純文案、純註解與本來就沒有斷言可言的變更；適用於任何你在 commit 訊息裡寫「修好了 X」的東西。Claude Code 的 `mutation-check` 技能與 `skillhub-mutation` 角色是這條的可執行版本。

## Harness：角色、技能、rules、workflows 與 deny 的放置規則（2026-09-03 建立，09-04 重整）

`AGENTS.md`〈分區指標與攔阻〉只放入口；分層與派工契約見[開發者指示](./agent-instructions.md)；規則本體在這裡；**建立這套東西的歷程、洞見、工序與功法在 [harness/](./harness/README.md)**。**判準：換一個 repo 還成立嗎？** 成立才進 `.claude/skills/`；不成立留在 `docs/`、各層 `AGENTS.md`、`.claude/rules/` 或 deny／CI。`.claude/` 不是整體可攜來源：只有 roles／skills 的來源檔可攜，rules／workflows 是本 repo harness。每一層只放一種東西：

- **`.claude/rules/`**：按路徑觸發的指標，內容只有「先讀哪一份、會被哪個檢查擋」。實測會送達子代理，首次命中注入一次。
- **`.claude/agents/`**：按**風險**切的三個角色——`skillhub-writer`（寫，路徑範圍由簡報給）、`skillhub-verify`（唯讀驗證）、`skillhub-mutation`（證明測試會紅）。**角色不按目錄切、不新增**：某個區域該先讀什麼、哪些檔屬於 coordinator，寫在該區域的 `AGENTS.md`，由 rules 按路徑送達。禁令只有一份（`AGENTS.md` 開發自動化第 3 條），角色檔引用不複述。
- **`.claude/skills/`**：換一個 repo 還成立的程序。技能不得引用 `docs/`、ADR 編號或需求 ID——引用了就隨那份文件過期。
- **`.claude/workflows/`**（2026-09-04 起）：**有固定形狀的多代理程序**——哪些事平行、哪一步驗證、由流程決定是否有 gate——寫成一支 `<name>.js`，以 `/<name>` 呼叫；判斷本身仍在子代理，腳本只決定順序與扇出。`ux-text-audit` 與 `error-path-audit` 都是 Read → Refute → Critic 的唯讀稽核；`parallel-page-edit` 才是寫入後 Verify → Mutation → Gate。跨工具契約、無 runtime 時的原生派工映射與完成判定見[開發者指示](./agent-instructions.md)；原生腳本的扇出安全不是跨工具保證。**腳本裡的 `agent()` 不指定模型就繼承派工者的旗艦級**，所以每一個 `agent(` 呼叫都要在同一行寫 `model:`（慣例是一行 wrapper：`const run = (p, o = {}) => agent(p, { ...o, model: o.model ?? 'sonnet' })`），`harness` 檢查逐行看。
- **`permissions.deny`**（`.claude/settings.json`）：把慣例變成真的拒絕——`stash`、`reset --hard`、`clean`、`checkout -- `、`checkout .`、`restore`、`add -A`／`--all`／`.`、`commit -a`／`--all`、`push --force`／`-f`、`commit --amend`。對子代理同樣生效、不需 workspace trust。**每條加完要親自撞一次**（2026-09-03 就撞出一個誤擋 `git add .claude/` 的偽陽性）。陷阱：路徑型規則只認 `Edit(...)` 與 `Read(...)`，寫成 `Write(...)`／`Glob(...)` 會被接受但永不被查詢。

**送達與契約**：官方啟動載入規則見 [Agent configuration](https://learn.chatgpt.com/docs/agent-configuration/agents-md)；若有 `AGENTS.override.md` 必須先讀並檢查衝突。派工介面未提供 `cwd` 參數時（例如本次驗證的 `spawn_agent`），代理共享目前工作目錄；工具不保證按檔案自動載入，brief 必須列出 root → 目標父目錄的明確讀取路徑。Codex 指示總量上限 32 KiB（`project_doc_max_bytes`），所以根 `AGENTS.md` 有大小上限。Claude 的額外層是提早發現，真正的保證仍在 `automation-check`、測試與 CI。

**新增一個區域的三步配方**（不新增角色）：①在該目錄放 `AGENTS.md`（指標表：要做的事 → 先讀哪段 → 沒讀會被哪個閘門擋；加一行 `@AGENTS.md` 的 `CLAUDE.md`），遵循[開發者指示](./agent-instructions.md)的分層／派工流程；②在 `.claude/rules/` 加一條 `paths:` 指向它，規則提示改讀該區域 `AGENTS.md`；③在根 `AGENTS.md`〈分區指標與攔阻〉的表加一列。

**守它的機器**：名冊裡的 `harness`（技能不引用本地文件、角色必須指定 `model` 且不得 fable／sol／inherit、根 `AGENTS.md` 不超過上限、workflow 的每個 `agent(` 同一行有 `model:` 且 `meta.name` 等於檔名）與 `apps/platform/internal/shared/skillpkg/repo_skills_test.go`（技能過產品自己的 `skillpkg.Validate`）。

## 常見失敗

| 診斷 | 意義與處置 |
| --- | --- |
| Doctor 顯示 Go／Node 不符 | 切換到原生版本或 Dev Container；不跳過檢查 |
| Windows uv hardlink／CP950 error | 只能走 devctl；它固定 `UV_LINK_MODE=copy` 與 UTF-8 |
| `DRIFT ...` | 修改 source後未重生，執行 `task gen`；不得編輯產物讓 check 變綠 |
| generation already running | 共享工作樹已有 Writer；等對方完成。超過兩小時的 lock由 devctl回收 |
| generated conflict | 合併 OpenAPI／SQL來源後重生；不手動 merge generated code |
| `dev:model` 缺變數 | devctl只列變數名稱。把 secret放 ignored `.env`，不要放 `.env.example` |
| Python editable install access denied on OneDrive | 關閉仍占用 `.venv` 的程序後重跑 `uv sync --frozen`；不要刪他人工作或 lockfile |

## `automation-check` 跑了哪些檢查（名冊）

`go -C tools/devctl run . automation-check` 除了固定的文件字句、`Taskfile.yml` 的 `desc` 與 generated ownership marker 之外，還會跑一份**檢查名冊**：`tools/devctl/automation_check.go` 的 `documentCheckers()`。**那個函式就是名冊本身**（`TestAutomationCheckRunsEveryChecker` 逐項走過它），下表是 2026-09-03 逐項讀出來的 **24 條**（同日先讀到 23 條，`doc-links` 是當天稍晚加的第 24 條），加上 2026-09-04 的第 25 條 `harness` 、2026-09-11 的第 26 條 `comment-budget` 與 2026-09-12 的第 27 條 `dependency-policy`。**這個數字本身會過期**——以 `documentCheckers()` 的實際回傳為準。

**撞到紅燈時的用法**：`FAIL` 訊息開頭的名字對到下表，再去「規則寫在哪」那一欄讀該檔；它為什麼存在、抓到過什麼，看那個檔的 `git log`（程式裡不寫施工日誌，見根 `AGENTS.md`〈慣例〉）。**本節只給名字與落點；下面的散文只保留有故事的那五條**（`one-number`、`milestone-tally`、`backlog-tally`、`baseline-tally`、`doc-identifier`），其餘不在此重述。

| 名字 | 它比對什麼 | 規則寫在哪 |
| --- | --- | --- |
| `drift-marker` | ADR-032 附錄 A 與 `apps/platform/.golangci.yml` 的 `drift: DDD-n` 標記多重集必須一致 | `tools/devctl/automation_check.go` |
| `depguard-deny` | depguard deny 清單的**內容**與 ADR-032 附錄 A 相符——刪兩行就等於默默開一條跨 context 權限 | `tools/devctl/depguard_deny.go` |
| `service-construction` | 非 composition root 不得現場建構其他 Bounded Context 的 `Service`（ADR-032 §5） | `tools/devctl/service_construction.go` |
| `one-number` | 帶 `one-number:` 標記的各站點數值相同，且標記要在 `sharedNumberRoster` 名冊上（雙向） | `tools/devctl/shared_number.go` |
| `query-owner` | 每條 sqlc query 的呼叫方是 `db/query-owners.yaml` 宣告的 owner context（ADR-033／035） | `tools/devctl/query_owners.go` |
| `context-map` | ADR-032 §1 Context 對照表與 `.golangci.yml` 的 `files:` 清單逐套件對帳 | `tools/devctl/query_owners.go`（`contextMapProblems`） |
| `doc-identifier` | 活文件散文裡的識別字必須真的存在於程式樹 | `tools/devctl/doc_identifiers.go` |
| `milestone-tally` | M5 的勾選數只有 `03` §19 的 checkbox 能說，其餘四份文件不得出現這個數 | `tools/devctl/milestone_tally.go` |
| `backlog-tally` | `04` 每個帳目格的數字＝`<!-- open: … -->` 清單長度；清單上每個 id 都是真的列；沒有一列自稱已結案 | `tools/devctl/backlog_tally.go` |
| `baseline-tally` | SEC-002 基線在六處自述的規模等於逐列重數（含分區表的合計列） | `tools/devctl/baseline_tally.go` |
| `retention-floor` | `02:NFR-002a` 的三條保存期**下界**，逐條檢查而不是取平均 | `tools/devctl/retention_floor.go` |
| `sdk-version` | Agent SDK 版本字串在 Dockerfile `ARG`、`sandboxd/main.go` fallback、`apps/sandbox/README.md` 三處一致 | `tools/devctl/sdk_version.go` |
| `single-data-layer` | `db/gen` 之外不得長出第二個資料層（`02:PORT-008`） | `tools/devctl/second_data_layer.go` |
| `require-db-guard` | 會因缺 DB URL 自我停用的測試套件，都必須認 `SKILLHUB_REQUIRE_DB`（`02:PORT-004`） | `tools/devctl/require_db_guard.go` |
| `require-objstore-guard` | SBX-008 短效授權的那支測試還在，且認 `SKILLHUB_REQUIRE_OBJSTORE`（`02:PORT-009`） | `tools/devctl/require_objstore_guard.go` |
| `isolation-level` | 派送閘門接受的每個隔離等級都要寫在 `contracts/openapi/sandbox-provider.yaml` 的 enum 裡（單向） | `tools/devctl/isolation_levels.go` |
| `route-table` | `router.go` 掛上的 route 與 `contracts/openapi/public.yaml` 的 `paths:` **雙向**對帳（codegen 看不到 route） | `tools/devctl/route_table.go` |
| `requirement-refs` | `03`／`04`／`05` 引用的 `02:<ID>` 在 `02` 有且只有一個同名標題 | `tools/devctl/requirement_refs.go` |
| `purge-schedule` | `cmd/maintenance` 的每個清理子命令，在 release checklist 的部署段都要有一行 cron | `tools/devctl/purge_schedule.go` |
| `timeout-budget` | 成對的 `budget-over:`／`budget-ceiling:` 標記，Go 的 deadline 必須大於 Python 的 | `tools/devctl/timeout_budgets.go` |
| `image-version` | Dockerfile 的 `ARG IMAGE_VERSION` 每個版本，`UPGRADES.md` 都要有同名章節（ADR-023 §4；**只查章節在不在，查不出四項有沒有真的跑**） | `tools/devctl/image_version.go` |
| `embedding-dims` | `0007_search.sql` 的 `vector(1536)` 與 `apps/llm` 驗證的寬度一致（migration 為準） | `tools/devctl/embedding_dims.go` |
| `goldenset-mirror` | `tools/goldenset/evaluate.py` 的 `enriched_index_text` 與 Go 的 `embeddingText` 以 digest 綁在一起 | `tools/devctl/goldenset_mirror.go` |
| `capability-table` | `.env.example` 的每個變數都要說出它擋著什麼（`05` R-36），見下節 | `tools/devctl/capability_table.go` |
| `doc-links` | 每一條相對路徑的 markdown 連結都要指得到真實檔案（只驗路徑，不驗 `#` 錨點、不連外） | `tools/devctl/doc_links.go` |
| `dependency-policy` | Dockerfile 的 FROM、compose 與 workflow 的 `image:` 都釘 digest；`uses:` 釘 40 碼 SHA 並寫 `# vX`；每個 npm 專案有 `.npmrc` 的 `ignore-scripts=true`；每個 uv 專案有 `exclude-newer`；每個有 lockfile、Dockerfile、compose 或 composite action 的目錄都列在 `.github/dependabot.yml`；compose 與 workflow、`tools/ci/*.sh` 用到同一個映像時引用完全相同；node、go、python、uv、task、golangci-lint 在每個位置版本一致（ADR-078、079，見〈依賴的准入、更新與閘門〉） | `tools/devctl/dependency_policy.go` |
| `harness` | `.claude/skills/` 不得引用 `docs/`、ADR 編號或需求 ID；`.claude/agents/` 每個角色必須指定 `model`（不得 fable／sol／inherit；預設是各角色 frontmatter 的低階模型，簡報依任務難度升級）；根 `AGENTS.md` 不得超過 16 KiB（Codex 讀到 32 KiB 就靜默截斷；上限是棘輪，貼著現況而不是貼著懸崖）；`.claude/workflows/*.js` 以 `export const meta = { name }` 開頭、`name` 等於檔名，且每個 `agent(` 呼叫同一行要有 `model:`、字面值不得 fable／sol／inherit（裸 `agent()` 會繼承派工者的旗艦級）。**技能的 frontmatter 是否合 Agent Skills 規格，由產品自己的驗證器管**：`apps/platform/internal/shared/skillpkg/repo_skills_test.go` 把 `skillpkg.Validate` 跑在 `.claude/skills/` 上 | `tools/devctl/harness.go` |
| `comment-budget` | 手寫程式與設定檔（Go／TS／JS／Python／SQL／YAML／TOML／shell／Dockerfile／`.env.example`，含 `doc.go`；不含 generated 檔與 `go:`／`one-number:`／`-- name:` 等機器標記）的兩種註解：超過 3 行的區塊，以及帶需求／裁定編號、日期或 `§` 的施工日誌。`comment-lint <路徑>` 逐行列出。零容忍、沒有存量清單：2026-09-11 全 repo 清理後歸零。規則本體是根 `AGENTS.md`〈慣例〉 | `tools/devctl/comment_budget.go` |

### 新增一個 `.env.example` 變數，要同批說出它擋什麼

`capability-table`，2026-09-01 加入（`05` R-36）。促成它的那個變數是
`DOWNLOAD_ARTIFACT_RETENTION`：它在 `.env.example` 裡、程式讀它，而**沒有任何一個地方寫著沒有它就不能打包**。一個不說自己擋什麼的部署變數，在部署當天等於一個沒有人知道要不要填的欄位。

**所以規則是**：往 `.env.example` 加一個變數，同一批就要把它加進 `apps/platform/cmd/api/capabilities.go` 的能力表（說出這個能力少了它會怎樣），**或**加進 `tools/devctl/capability_table.go` 的 `capabilityLedger` 並寫下理由。兩個都沒做，`automation-check` 會 FAIL。

檢查跑三個方向，第二、三個才是讓它保持誠實的那一半：

- `.env.example` 有、能力表與帳目都沒有 → FAIL；
- 能力表宣告了某個變數，`.env.example` 卻沒有記載它 → FAIL（一個查不到的前提，維運人員無從發現）；
- 帳目豁免了一個已經不在 `.env.example` 的變數 → FAIL（帳目會看起來比實際短）。

**它問的是執行檔而不是解析 Go 原始碼**：能力表是 Go（R-36 決定「能力→前提」的宣告放 Go），而 devctl 是另一個 module，所以檢查跑 `go run ./cmd/api --capabilities`，讀那個程式**實際持有**的表——解析原始碼只會在「有人換一種寫法寫那個 literal」之前成立。

**`capabilityLedger` 是存量清單，不是擴充點**，形狀比照 `db/query-owners.yaml` 的 `allow:`：每一列都是 2026-09-01 當天就已經在 `.env.example` 裡的變數，而且**只准變短**。它自己的最後一組標著 ⛔——那十二個變數確實擋著東西，只是還沒有人寫下擋著什麼；那是這份帳目要讓人看見的債，不是拿來開脫的。

## 同一個數字散在好幾個檔案：`one-number:` 標記

有些值必須在**沒有任何編譯器會比對**的地方保持一致：一個 Go const、契約裡的 `maxLength`、`apps/llm` 的 Pydantic `max_length`、量測 harness 裡的常數。沒有東西把它們綁在一起，所以它們會漂，而且漂掉的時候是在最糟的時刻才被發現。

**它已經發生過一次**：`maxDigestEntry` 從 2000 調到 8000 **只改了 `judge.go`**（`04` 丙-47），於是**每一次評分都回 422**——因為 `apps/llm` 還在拒絕超過 2000 的字串。而那是四份副本裡**最大聲**的一份；`tools/eval-regression` 的那一份漂掉時，只會安靜地送出一個不一樣的請求，然後把結果當成沒事一樣報出來。

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
2. **標記要開啟那個註解**。`# one-number: x - 因為 y` 算；`# 因為 y; one-number: x` **不算**，而且它會是「安靜地看不見」而不是「報錯」——harness 那一份第一次就是這樣漏掉的，偏偏它正是漂掉時最安靜的那一份。
3. **只剩一個站點會 FAIL**。一個站點的不變量保護不了任何東西，而它變成一個站點的方式通常是有人刪掉了別人的標記而不是別人的副本。

**這不取代契約作為事實來源（鐵律 12）。** 每一份副本正確的修法都是「生成它或推導它」，能便宜做到就該做——`packages/api-stub-py` 的那一份就是從契約生出來的，所以它不帶標記。這個機制是給**生成器搆不到的那些副本**。

### 一個數字只能有一個作者

`milestone-tally`，2026-08-24 加入。M5 的「幾勾幾 ◐」曾經同時寫在 **五份文件**裡，六輪對抗式審查中有**三輪**抓到它們彼此不符——沒有任何一次單獨的編輯是錯的，錯的是一個**導出來的**數字有五個作者。

作法與 `one-number` 同一個家族，但方向多一邊：

- **`03` §19 的 checkbox 就是事實**，機器數 `- [x] GEN-` 與 `- [ ] GEN-`；
- **`03` 自己的節首必須說出那兩個數**（說錯就 FAIL）；
- **其餘四份文件（`AGENTS.md`、`01`、`mvp/README`、`m5/README`）不准出現這個數**——它們該說的是「◐ 的是哪兩項、為什麼」，那才是讀者要的，而且不會漂。

**只管 M5。** `01` 的 M4 那一行（「49 項中 16 勾／33 誠實不勾」）形狀一樣、風險一樣，但**那 49 項的集合只寫在 `m4/audit.md` 的散文裡**，機器既證實不了也否證不了它——**去標記一個查不出對錯的東西，是讓檢查失去讀者的方法**。所以 M4 那一行留著，並在此記明它未經驗證。

### 新增一個共用數字，要同批加進名冊

`one-number` 的保護本來是 opt-in：`found` 是從掃描結果建的，把某個不變量的**全部**標記一起拿掉 → 沒有這個 key → 迴圈不走 → 全綠。這不是理論風險：這些標記寄生在註解裡，`tools/eval-regression` 的一份副本就曾經因為一次註解整理而靜默地不再被計數。

**2026-08-25 起，`tools/devctl/shared_number.go` 持有一份 `sharedNumberRoster` 名冊，雙向強制**：名冊上找不到標記 → FAIL；有標記卻不在名冊上 → 也 FAIL（第二個方向才是讓名冊保持誠實的那一半）。**所以新增一個跨檔共用的數字時，同一批要把名字加進那份名冊，不然 CI 會 FAIL。**

### 殘項的數字必須對得上它自己的清單

`backlog-tally`，2026-08-25 加入。AGENTS.md 把 `04` 註明為**殘項總數的唯一來源**，而那份文件自述它的數字是「逐列重數、不是把歷次加減累積出來的」——**而沒有任何人可以驗證那句話**。全檔 67 列裡只有 13 列把結案寫在機器讀得到的格子裡，其餘寫在列內的散文裡。那個數字已經與自己不符過三次。

作法：每一個帳目格末尾帶一個 `<!-- open: 26,38,49,... -->`，機器對三件事：

- **粗體的數字等於清單長度**；
- **清單上的每一個 id 都真的是這份文件裡的一列**（一個程式註解曾經引用「`04` 丙-57」而那一列不存在）；
- **沒有一列被列為未結，卻在自己的狀態欄寫著已結案**。

**它故意不檢查另一個方向**：一個沒列到的列是不是真的結了。那需要逐列的狀態欄，而把「沒列到就算結案」當預設，等於一口氣斷言四十九件沒人查過的結案。**這與 `milestone-tally` 不標記 M4 那一行是同一個判斷：去檢查一個查不出對錯的東西，是讓檢查失去讀者的方法。**

### 安全基線的規模必須等於它自己的列數

`baseline-tally`，2026-08-26 加入。SEC-002 的基線在六個地方自述它有幾項，而唯一有權說話的是那些列。

`a7e1699` 在 2026-08-25 加了 **N-08**（`ip6` 鏈維持 `policy drop`、不得渲染 accept 規則），六個數字一個都沒動。於是威脅模型自相矛盾——§4.3 列了八列 N，分區表寫七，合計寫 45——而 `02:SEC-002` 把過時的數字引了四次。

**算錯是小的那一半。** ADR-022 §2 的覆蓋核對是**逐區**配測項的，所以一個數字過時的區，就是它最後幾列沒有測項的區；而 §3 的通過判準數到一個不包含它們的總數。**湊滿 45 永遠不需要碰 N-08。** 一個存在於基線、卻對驗收表隱形的檢查，比一個不存在的更糟——因為表看起來是完整的。

機器對四件事：

- **合計句**（`合計：N 項檢查（阻擋 N 項、告警 N 項）`）等於逐列重數的結果；
- **分區表的每一列**等於該區的列數與等級分佈，**連同它的 `**合計**` 那一列**——那一列不帶區代號，正好是逐區比對看不到的一列，也正好是讀者會相信的那一列；
- 沒有任何 ID 出現兩次；
- `02` **與 `03`** 裡任何一句自述基線規模的話，數字都要是現在這個。**`03` 是 2026-08-26 稍晚補進來的，補的時候它正好有兩句是舊的**——`§18` 開頭那句「允收準則來源為……45 項基線」與 `RELEASE-004` 那句「45 項全數 pass 且 0 unknown 才放行」。**兩句都不帶日期，所以兩句都是在講現在**，而那正是這個檢查存在的形狀：一個數字有好幾個作者，其中一個更新了，其他的安靜地留在原地。

**兩個刻意的範圍限制**，兩個都是第一次跑出來的：

- **帶日期的句子是在描述那個日期**，跳過。第一次跑的每一個誤報都是這一類：`v2 的 32 條威脅與 45 項基線檢查`、`2026-08-16 定案`、以及 N-08 那次自己補的「45 是 2026-08-15 到 2026-08-25 的數字」。**記下一個數字曾經是多少，正是文件對漂移誠實的方式**；一個禁止這件事的檢查，只會教人把歷史刪掉。
- **ADR-022 不在被檢查的名單上。** 它有九個地方寫 45，每一個都是它定案當日的數字，而 AGENTS.md 明文不原地改寫已定案 ADR 的決策內容；它的 2026-08-26 補記承接現行讀法。**把它列進來，等於要求人做出那條規則禁止的編輯**——與 `doc-identifier` 只掃活文件是同一個判斷。

### 活文件裡的識別字必須存在

同一個家族的第二條，2026-08-24 加入。**文件的散文檢查不了，但散文裡的識別字可以**——而一個死掉的識別字，通常是一句死掉的主張穿著它。

實際抓到的三個：`04` 拿一個叫 `PublicSearchHit` 的型別論證「我的 Skill 清單少了四項證據」（真名是 `PublicSearchResult`）；`04` 丙-38 的結案數字掛在 `ApplyPreview` 上（真名是 `improvement.Diff`／契約 `SuggestionDiff`，那個名字是 m3 報告發明的，被抄了三次）；`03` 在函式刪掉之後還說它「已備好」。**三個都是六輪對抗式審查裡人工抓到的，而它們是機器抓得到的那一類。**

**範圍只有活文件**（`AGENTS.md`、`docs/plans/01`～`05`、`docs/design/`、`m5/README`），而這個限制是整個設計：同一種檢查套在 ADR 與凍結的里程碑報告上，量到 **18 個命中，每一個都是「寫的當下是對的」的歷史**（DDD 搬檔、已刪除的 spike），而 AGENTS.md 明文要求不要順手修正那些。**一個會要求人違反明文規則的檢查，比沒有檢查更糟——人會學會忽略它。**

量測值（寫下時）：410 個引用、6 個命中、3 個是真的。誤報那三個由 `allowedDocWords` 記著理由，形狀比照 `db/query-owners.yaml` 的 `allow:`——**是存量清單，不是擴充點**。

兩條規則：

1. **死掉的名字不要穿反引號。** 反引號的意思是「這是一個真的符號」；訂正句裡提到一個從來不存在的名字時寫成純文字，檢查就不會命中，而讀者看到的資訊完全一樣。
2. **`declared` 是「這個字出現在任何一個程式檔裡」，不是「這個符號有宣告」。** 便宜、不需要 parser，代價是**一個被刪掉但名字還留在某段註解裡的函式會溜過去**。寫這個檢查的當天就踩到了：它自己的說明註解引用了三個要抓的名字，於是把它們全部漂白——所以掃描時跳過 `doc_identifiers.go` 自己。

## 跑一次真實的端到端 Run（2026-08-27 實測重寫）

**成本：一次約 $0.017（mini 級）。會真的花錢。**

`m2/README.md` 有一份「跑一個 Skill 的最短路徑」，那是里程碑時點的證據、已凍結，而**照它今天逐字做會得到一個沒有網路的 Run**——2026-08-26 之後 sandboxd 多了一個 fail-closed 的前置，那份文件寫的時候還不存在。下面是 2026-08-27 實際跑通的版本，`gateway-reported cost for this run: $0.016671`。

### 今天多出來的那一步：沒有渲染過的允許清單，就沒有網路

`SKILLHUB_SANDBOX_NETWORK` 一旦設了非 `none` 的值，`SKILLHUB_SANDBOX_EGRESS_ALLOW` 就**必填**，而且**committed 的 `infra/egress/rendered/egress-allow.json` 是空的**（`allowlist.yaml` 的 `pinned_ip` 仍是 `unset`，那是生產的正確預設）。空清單 ⇒ sandboxd 宣告 `none` ⇒ 沙箱被派到沒有網路的容器 ⇒ Agent SDK 對閘道空等到逾時。

所以 dev 要**另外渲一份**，不要動 committed 的那一份（動它會踩 `egress-allowlist.yml` 的「同 PR 必須改威脅模型」閘門）：

> ⚠️ **下面三行限 Linux／Dev Container 內執行**（2026-09-03 補記）。它們是 2026-08-27 那次實測當下的原樣，不是可攜的標準流程：`cp -r`、GNU `sed -i` 與 `python3` 三個都不是本專案的跨平台入口，而本專案主開發機是 Windows（`Taskfile.yml` 用的是 `python`，不是 `python3`）。在 Windows 主機上請進 Dev Container 再跑；**這裡刻意不改寫成一個沒有人實際驗過的跨平台版本**——一條沒跑過的指令比一條標明適用範圍的指令貴。

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
  -e SKILLHUB_SANDBOX_IMAGE=skillhub/runtime-agent-sdk:2026.08-10 \
  debian:12-slim /usr/local/bin/sandboxd
```

**映像版本（2026-09-03 訂正）**：上面原本寫 `2026.08-3`，那是 2026-08-27 實測當天的值，而 `-3` 之後映像有四次行為變更（`UPGRADES.md` 的 `-4`／`-5`／`-6`／`-7` 四節）。**這裡寫 `2026.08-5`，因為它就是部署預設**——`apps/sandbox/cmd/sandboxd/main.go` 的 `SKILLHUB_SANDBOX_IMAGE` fallback、`ci.yml` 的 `RUNTIME_IMAGE_FOR_PROBE`、`p02_docker_test.go` 的常數三處同值。**不是最新的 `2026.08-7`**：`-6` 與 `-7` 只動了 Dockerfile 的 `ARG IMAGE_VERSION`，ADR-023 §2 的四項實測還沒跑（`UPGRADES.md` 那兩節自陳、`04` 丙-125 開著），移動預設是四項通過之後的動作。要在這裡改成別的版本，先讀那兩節。

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
  -e SKILLHUB_MODEL_GATEWAY_URL=http://litellm:4000 -e SKILLHUB_MODEL_GATEWAY_KEY="$LITELLM_MASTER_KEY" \
  -e SKILLHUB_RUN_MODEL=gpt-5.4-mini -e SKILLHUB_E2E_PUBLIC_HOST=postgres \
  -e DEV_LOGIN=1 \
  debian:12-slim /usr/local/bin/e2e.test \
  -test.run TestEndToEndRunCallsTheModelThroughItsOwnVirtualKey -test.v -test.timeout 20m
```

**`-w` 那一行是必要的**：測試以相對路徑 `../../../../../../db/migrations` 找 migration。

**`DEV_LOGIN=1` 也是必要的（2026-09-06 補）**：`execution.Match` 對隔離等級是允許清單，這台 sandboxd 跑 runc、自報 `container`，只有標成開發部署的程序才接受它，否則每個 Run 都是 422「which this deployment does not accept」。這不是繞過——它就是那條規則為開發機留的門，生產部署不設它。同日的互動創作量測第一次跑 Run 階段就撞到這裡，14 場全 422 才發現配方缺這一行。

**Git Bash 跑上面這段時**：MSYS 會把以 `/` 開頭的參數與環境變數值改寫成 Windows 路徑（`-e X=/etc/foo` 進容器變成 `C:/Program Files/Git/etc/foo`）。整段前面加 `MSYS_NO_PATHCONV=1`，或把容器內路徑寫成 `//etc/foo`（Linux 把雙斜線當單斜線）。掛載來源用 `C:/...` 的寫法就不會被動。

### 它證明了什麼、沒證明什麼

**證明**：套件進物件儲存 → preflight → 確認 → 派送 → sandboxd → 容器跑 Agent SDK → 每 Run 短效 Virtual Key 經閘道呼叫模型 → trace 回推 → artifact 收集 → 金鑰撤銷，**整條在今天仍然是通的**。

**沒證明**：這一輪的 runtime 是 `runc` 不是 `runsc`，所以它不是 SEC-009 的任何一項；`--network skillhub_egress` 是 Docker 網路隔離，**不是** ADR-022 Q3 的 nftables 強制層（那條路的實驗室在 `tools/sec009/t5-network-egress.sh`）。

### 什麼時候要跑它（2026-09-10 裁定，`05` R-72）

**它不在 CI，而且刻意不排程。** 排程要一把能用的閘道金鑰放在 GitHub secret 裡，而鐵律 11 的形狀是「供應商金鑰只存在閘道」——為了一個每週一次的檢查，在閘道之外多開一個金鑰存放點。裁定取的是兩個**由人按、但綁死在事件上**的時刻：

- **(b) 發布前**：[release-checklist §2.7](../plans/mvp/m4/release-checklist.md) 有一列，勾選前必須附閘道回報的實際金額。
- **(c) 改到就跑**：`apps/sandbox`、`apps/llm`，或派送路徑（`apps/platform/internal/trial/` 的 Run 狀態機與 Worker）**有改動時**，用上面那段配方手動跑一次。這三個位置是這條線斷掉時唯一會動到的地方。

**為什麼不能像其他依賴那樣「跳過就是紅」**：`SKILLHUB_REQUIRE_DB`／`_OBJSTORE`／`_CREATION_PYTHON` 三個都做成了缺依賴即失敗，**唯獨這一支不行，因為它會花錢**。

**接受的殘留風險，明寫**：這條線的保證從「機器會檢查」降成「兩個明確的時刻有人記得」。它斷掉的樣子是**派送成功、Run 永遠不完成**——`web` 與 `platform` 兩個 job 全綠，沒有任何一盞燈會變色。若封測期間需要更硬的保證，正解是**在節點上跑**（節點本來就要有閘道金鑰），不是在 CI 裡多放一把。

## 推送前與推送後：pre-push hook、`task preflight`、`task ci:status`（2026-09-11）

**pre-push hook**：`task bootstrap` 會把 `core.hooksPath` 指到 `.githooks/`，`pre-push` 跑 `devctl preflight --hook`。它只檢查**這次要推的 commit**，而且讀的是 commit 裡的位元組（`git show <sha>:<path>`）而不是工作樹，所以共享工作樹裡別人未提交的修改擋不到你的推送。它查兩件 CI 會紅的事：

- 推送範圍內改到的 `apps/platform`／`apps/sandbox` Go 檔（gofmt）、`apps/web` 檔（prettier）、`apps/llm` Python 檔（ruff format）有沒有照 CI 的格式；
- `infra/images/runtime-agent-sdk/` 的 Dockerfile 或它 `COPY` 進映像的檔有改、`ARG IMAGE_VERSION` 卻沒動（I-05）。這段判斷與 `Runtime Image` workflow 呼叫的 `devctl image-gate` 是同一份程式，兩邊不會再各說各話；沒被複製進映像的檔（例如 `run.test.mjs`）不觸發。

格式工具不在這台機器上時只印 `WARN`、不擋，CI 仍會查。**不要用 `--no-verify` 跳過**（`.claude/settings.json` 的 deny 已擋）；hook 誤擋就修 `tools/devctl/preflight.go`，不要繞過它。

**`task preflight`**：手動版，範圍是 `@{upstream}..HEAD`，另外多跑一次 `automation-check`。後者讀的是工作樹，所以別人未提交的檔可能讓它紅——看 `FAIL` 的檔名判斷是不是你的。

**`task ci:status`**（`devctl ci-status [ref] [--wait]`）：列出該 commit 的**每一個** workflow run、失敗的 job 與 step、以及被 path filter 跳過而沒跑的 job。token 用 `git credential fill` 取得、只放在記憶體裡，所以不吃未驗證呼叫每小時 60 次的上限——幾個工作階段同時輪詢時，那個上限幾分鐘就會用完。結束碼：0 綠、1 紅（`cancelled` 也算，因為那個 commit 沒被驗到）、3 還在跑、4 還沒有 run。

**CI 的形狀**（`ci.yml`）：

- main 上的 push 以 commit SHA 分組、**不互相取消**，每個 commit 都會被驗到；只有 PR 會被同一個 PR 的下一次 push 取消。改之前的三天內 87 次 run 有 14 次被取消，那些 commit 從來沒被驗過。
- `images` 有自己的 path filter（ADR-019 §3 第 5 列本來就這樣寫），純文件 commit 不再建置、smoke、推送三個服務映像。它不再等語言 job，與它們並行；推送拆成 `images-push`，等所有 job 綠了才推，從同一次 run 的 GHA 快取重建，不重新編譯。
- 每週日一次 `schedule` 全量跑（所有 path filter 視為命中），`workflow_dispatch` 也是全量——手寫 path filter 漏掉的那一格由它兜底。
- 每個 job 都有 `timeout-minutes`，一個卡住的 job 不會再佔滿預設的 6 小時。
- 每次都跑、沒有 path filter 的有三個並行 job：`devctl`（devctl 自己的 vet／race 測試、`automation-check`、`agent-sync`）、`contracts-drift`（`gen --check` 與 `contracts/` 的各項檢查）與 `dependency-audit`（見下一條）。純文件 commit 的等待時間由三者中最慢的那個決定，不再是相加。
- **依賴漏洞**（[ADR-077](../adr/ADR-077-dependency-vulnerabilities-block-only-when-a-fix-exists.md)）：push／PR 跑 `devctl dep-audit`，只看會出貨的四個專案——`apps/web` 的 production 依賴（high 以上）、`apps/platform`／`apps/sandbox` 程式呼叫得到的漏洞、`apps/llm` 的非 dev 依賴——而且**只擋有修補版的**，沒有修補版的印成 `NOTE`。每週排程與 `workflow_dispatch` 改跑 `--full`：每個有 lockfile 的專案連 dev 依賴、npm 取 moderate 以上、Go 連沒呼叫到的模組也算，紅了就是那一週的報告。`images-push` 等它綠了才推。本機 `task deps:audit`（加 `-- --full` 全掃）；同一條命令也擋出貨依賴的授權與 workflow 的 zizmor 稽核（[ADR-078](../adr/ADR-078-dependency-governance-admission-updates-install-guards-licenses-and-pins.md)，用法見〈依賴的准入、更新與閘門〉）；job 帶 `GH_TOKEN`，zizmor 連網的稽核項目只在 CI 跑。工具版本在 `tools/toolchain.yaml` 的 `govulncheck`／`pip_audit`／`go_licenses`／`zizmor`。
- `golangci-lint` 由 [`.github/actions/golangci-lint`](../../.github/actions/golangci-lint/action.yml) 安裝：版本讀 `tools/toolchain.yaml` 的 `golangci_lint`，編好的 binary 以「版本＋OS＋Go 版本」為鍵另外快取。不能指望 `setup-go` 的快取帶著它——那份快取的鍵只有 `go.sum` 的雜湊，第一次存下之後內容就不再更新，`go.sum` 沒動過的模組會一直拿到那天的舊內容。
- `sandbox` 的 filter 只看 `infra/images/runtime-agent-sdk/**`：其他服務映像與 `infra/images/` 下的說明文件不影響 sandbox 的任何測試。

**Runtime Image**（`runtime-image.yml`）：發佈前先查 registry 有沒有這個版本的 tag，**有就只跑閘門、不推送、不移 tag**。版本 tag 一旦發佈就不再變，因為 ADR-023 決策 1 的事實來源是 digest，而 build 不是位元可重現的——同版重推會讓同一個版本字串悄悄指向另一份沒量過的映像。attestation 失敗會在同一個 run 裡自動重試一次；發佈中的 run 不會被下一次 push 取消。

## 依賴的准入、更新與閘門

決策與理由在 [ADR-077](../adr/ADR-077-dependency-vulnerabilities-block-only-when-a-fix-exists.md)（漏洞）與 [ADR-078](../adr/ADR-078-dependency-governance-admission-updates-install-guards-licenses-and-pins.md)（其餘）；這裡只放動手時要知道的事。

**新增一個直接依賴之前**，回答五個問題，答案寫進 commit message：標準庫、平台或已經裝的依賴做得到嗎（做得到就不加）；授權在允許清單上嗎；還有人維護嗎（最近一年有發佈、安全問題有回應）；會多拉進幾個傳遞依賴；需要 install script 嗎（`ignore-scripts=true` 會讓它失效，需要就寫 ADR）。`apps/web` 的執行期依賴另外要一份 ADR（system.md §4.8，前例 ADR-076）。

**這些版本要一起動**。Dependabot 會提出升級，但通常只改到其中一處；node、go、python、uv、task、golangci-lint 由 `dependency-policy` 比對每個位置，不一致就 FAIL 並列出各位置的值（[ADR-079](../adr/ADR-079-toolchain-versions-follow-upstream-and-move-together.md)）：

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
| Agent SDK | ADR-023 的四項重驗，`UPGRADES.md` 一節 |
| 稽核工具 | toolchain.yaml 的 `govulncheck`、`pip_audit`、`go_licenses`、`zizmor` |

**Dependabot 的 PR**：每週一（npm、Go、Python）與每月（Actions、映像、compose）各開一個群組 PR，major 另開。新版本要先存在 7 天才會被提（npm 的 major 14 天）。CI 綠了就能合；major 先讀 changelog。PR 只改到上表其中一處時，`dependency-policy` 會紅並列出其餘位置：在同一個 PR 補齊，不要關掉。

**`devctl dep-audit`**（本機 `task deps:audit`，全掃加 `-- --full`）一次跑三件事，任何一件出現 `FAIL` 都會紅：

- 有修補版的漏洞 → 升到訊息裡的版本。
- 出貨依賴的授權不在允許清單 → 換一個依賴。確認授權其實可以接受（例如分類器認不出的 MIT 變體）時，在 `tools/devctl/license_audit.go` 的 `acceptedLicenses` 加一筆，附上理由，並在 commit message 說明你讀過的授權原文。
- zizmor 的 medium 以上 → 照訊息裡的連結修 workflow。本機沒有 `GH_TOKEN` 時只跑離線稽核，CI 會多跑連網的幾項。

**automation-check 的 `dependency-policy`** 擋的是：FROM、compose 與 workflow 的 `image:` 沒釘 digest；`uses:` 沒釘 SHA 或少了 `# vX` 註解；npm 專案少了 `.npmrc` 的 `ignore-scripts=true`；uv 專案少了 `exclude-newer`；新目錄沒列進 `.github/dependabot.yml`；compose 與 workflow、`tools/ci/*.sh` 裡同一個映像的 tag 或 digest 不一樣；上表同一個工具在各位置的版本不一致（名冊在 `tools/devctl/toolchain_versions.go`）。

## 完成判準

一次 automation 變更至少通過：

- `go -C tools/devctl test ./...`
- `go -C tools/devctl run . automation-check`
- `task gen:check`
- **`task format:check`**（2026-09-10 補入）
- `task preflight`（2026-09-11 補入；推送時 pre-push hook 會自動跑它的 `--hook` 版）
- 受影響語言的 typecheck/test/build
- `git diff --check`

**為什麼把 `format:check` 單獨列出來**：上面那一列「typecheck/test/build」不涵蓋它——`go build` 對一個 `gofmt` 會改寫的檔案完全沒有意見，所以編得過、測得過、推上去，然後 CI 的 `golangci-lint fmt --diff` 才是第一個說話的人（2026-09-10 實際發生：`packaging.go` 多一個結構欄位改變了欄寬對齊，platform job 紅在那一步）。`git diff --check` 也抓不到，它只看行尾空白與衝突標記。

**怎麼裝 `golangci-lint`，以及為什麼不能照著它官網那一行裝**：

```bash
GOTOOLCHAIN=go1.27.1 go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.2
```

版本 `v2.13.2` 來自 [`tools/toolchain.yaml`](../../tools/toolchain.yaml) 的 `golangci_lint`，CI 的 [`.github/actions/golangci-lint`](../../.github/actions/golangci-lint/action.yml) 讀同一個欄位、跑同一個 `go install`；模組代理與 sumdb 會驗 checksum，所以上游再發版也不會讓一棵沒動過的樹變紅。**`GOTOOLCHAIN=go1.27.1` 那個前綴是必要的，不是保險**：`golangci-lint` 會拒絕載入一份「目標 Go 版本比它自己編譯時用的 Go 還新」的設定，而本 repo 四個模組的 `go` 指示都是 **1.27.1**。CI 為此付過一次代價，錯誤訊息與四天八個 commit 的損失逐字記在 `b255333c` 的 commit message 裡——**它當時的形狀不是 lint 紅了，是同一個 job 裡後面五個 `- run:` 全部被跳過**，所以那段時間每一句「套件全綠」的意思都是「在某人的筆電上是綠的」。本機的 `go version` 比 1.27 舊沒有關係（`GOTOOLCHAIN=auto` 會自己抓），**沒有寫這個前綴才有關係**。

裝完之後 `go env GOPATH`／`bin` 要在 `PATH` 上，`devctl doctor` 的 `golangci-lint` 那一列才會 PASS。**那一列從一開始就在 doctor 裡**——[開工守則第 1 條](../../AGENTS.md)「先診斷再修改」指的就是這件事，而 2026-09-10 那次格式紅燈的真正成因不是缺工具，是**沒有人先跑 doctor**。

**真的裝不起來時的退路是 `gofmt -l ./apps/ ./tools/`**——它隨 Go 工具鏈一起來，一定在；輸出**任何一個檔名就是未通過**（`golangci-lint fmt` 的 Go 部分預設就是 gofmt ＋ goimports，所以 `gofmt -l` 乾淨時剩下的差異只會是 import 分組）。**但它是退路不是等價物**，而且 `task format:check:platform` 在沒有那支指令時會失敗於「找不到指令」，**那看起來很像「檢查過了」**。

版本／generator／Task入口異動要同步更新**本文件**、`tools/toolchain.yaml`、相關 package README與 CI；**`AGENTS.md` 只在紅線本身增刪時才動**（它不複製版本、命令清單與生成來源表）。工具能跑但新 Agent找不到，視為未完成。

## Agent 黑箱驗收（2026-08-18）

給一個沒有前文、唯讀的低成本 Explore Agent 任務：「新增 authenticated GET API field、SQL query並顯示在 Web」，不提示任何 automation名稱。它自行找到並正確回報：

- 先讀 `AGENTS.md`，跑 doctor／bootstrap；
- OpenAPI-first 與 SQL-first 的來源位置；
- `task gen:openapi`、`gen:sql`、`gen:check`；
- 四類 generated target禁止手改；
- 共享工作樹單一 Writer、禁止 stash；
- `dev:model` 與 E2E 的 secret／費用邊界；
- Web view model不應被 generated DTO 整批取代。

黑箱同時抓到兩個可發現性缺口：它把 `.devctl/phase4-ogen` scratch誤列為正式產物，並建議一般 Agent自行切 branch。本文因此明列 `.devctl/**` 不是可提交 API，且只有整合主 Agent做 Git 寫入；SubAgent 不切 branch。`devctl automation-check` 現在把這些字句、所有 task的 `desc` 與 generated ownership marker設為 CI gate。

同一批 clean-machine 驗收另跑過 canonical Linux toolchain 的 `task check && task test && task build`：Web 117 tests、LLM 62 tests、Platform／Sandbox Go suites、兩個 TypeScript build與 platform build全數通過。這是開發環境證據，不取代部署期 SEC-009。
