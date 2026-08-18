# 開發自動化與 Coding Agent 協作

本文件是 ADR-030 的操作手冊。`AGENTS.md` 放開工前的強制短版規則；本檔解釋命令、檔案所有權、環境分級與排錯。兩者不另造第二套流程，所有入口最後都走 `Taskfile.yml` 與 `tools/devctl`。

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

## 能力與成本分級

| 入口 | Docker | Secret | 可能花錢 | 作用 |
| --- | --- | --- | --- | --- |
| `task doctor` | 只檢查 | 否 | 否 | 診斷 runtime、Task、uv、Docker、Compose 與 `.env` 是否存在；不讀值 |
| `task env:init` | 否 | 否 | 否 | `.env.example` → `.env`，已有檔案時不覆寫 |
| `task bootstrap` | 否 | 否 | 否 | Go download、npm ci/build、uv frozen sync |
| `task dev`／`dev:core` | 是 | 否 | 否 | Postgres＋SeaweedFS |
| `task dev:model` | 是 | **是** | 後續模型呼叫會 | 先 fail-closed 檢查 OPENAI/LiteLLM 變數，再啟動 gateway |
| `task gen*` | 是 | 否 | 否 | 固定版本 generator；apply 會改 generated files，check 只寫 `.devctl/` scratch |
| `task test`／`task ci` | 視測試 | 否 | 否 | 預設不執行 E2E 模型測試；需 secret 的測試維持 opt-in gate |

`.env.example` 內非空的 Postgres／SeaweedFS 值是公開的 local-only placeholder；production secret 一律留空。不得把 `.env`、key 值或 doctor 以外的環境 dump 放進回覆或 log。

「不需 secret／不花模型費」不等於 offline：第一次 bootstrap 會下載 Go/npm/uv 依賴，第一次 generation 會拉 digest-pinned image，Redocly 與 container build亦需 registry／package mirror。lockfile、version與digest確保內容可重現，不宣稱斷網仍能從空 cache 建置；隔離環境應提供核准的 mirror或事先填好的 cache。

## 生成來源與所有權

| 人工修改來源 | 生成目標（不得手改） | 入口 |
| --- | --- | --- |
| `db/migrations/**`、`db/queries/**`、`db/sqlc.yaml` | `services/platform/internal/platform/db/gen/**` | `task gen:sql` |
| `contracts/openapi/public.yaml` | `services/platform/internal/api/gen/**`、`packages/api-client-ts/src/generated/**` | `task gen:openapi` |
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

## 完成判準

一次 automation 變更至少通過：

- `go -C tools/devctl test ./...`
- `go -C tools/devctl run . automation-check`
- `task gen:check`
- 受影響語言的 typecheck/test/build
- `git diff --check`

版本／generator／Task入口異動要同步更新 `AGENTS.md`、本文件、相關 package README與 CI。工具能跑但新 Agent找不到，視為未完成。

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
