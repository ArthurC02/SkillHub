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
- **Claude Code 另有一層攔阻**（`.claude/settings.json` 的 `permissions.deny` 把 `stash`／`add -A`／`commit -a`／`restore`／`checkout .`／`reset --hard`／`clean`／`push --force`／`commit --amend` 變成真的拒絕，對子代理同樣生效），**但它只是提早發現**：其他 coding agent 不受它管，本節的規則本體與 `automation-check`、測試、CI 才是保證。角色與技能的放置規則見根 `AGENTS.md`〈分區指標與攔阻〉。

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

`go -C tools/devctl run . automation-check` 除了固定的文件字句、`Taskfile.yml` 的 `desc` 與 generated ownership marker 之外，還會跑一份**檢查名冊**：`tools/devctl/automation_check.go` 的 `documentCheckers()`。**那個函式就是名冊本身**（`TestAutomationCheckRunsEveryChecker` 逐項走過它），下表是 2026-09-03 逐項讀出來的 **24 條**（同日先讀到 23 條，`doc-links` 是當天稍晚加的第 24 條），加上 2026-09-04 的第 25 條 `harness`。**這個數字本身會過期**——以 `documentCheckers()` 的實際回傳為準。

**撞到紅燈時的用法**：`FAIL` 訊息開頭的名字對到下表，再去「規則寫在哪」那一欄讀該檔的檔頭——每一支的檔頭都寫著它為什麼存在、抓到過什麼，那是判斷「這次紅得有沒有道理」唯一夠用的材料。**本節只給名字與落點；下面的散文只保留有故事的那五條**（`one-number`、`milestone-tally`、`backlog-tally`、`baseline-tally`、`doc-identifier`），其餘不在此重述。

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
| `harness` | `.claude/skills/` 不得引用 `docs/`、ADR 編號或需求 ID；`.claude/agents/` 每個角色必須指定 `model`（不得 fable／inherit）；根 `AGENTS.md` 不得超過 28 KiB（Codex 讀到 32 KiB 就靜默截斷）。**技能的 frontmatter 是否合 Agent Skills 規格，由產品自己的驗證器管**：`apps/platform/internal/shared/skillpkg/repo_skills_test.go` 把 `skillpkg.Validate` 跑在 `.claude/skills/` 上 | `tools/devctl/harness.go` |

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
  -e SKILLHUB_SANDBOX_IMAGE=skillhub/runtime-agent-sdk:2026.08-5 \
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
  debian:12-slim /usr/local/bin/e2e.test \
  -test.run TestEndToEndRunCallsTheModelThroughItsOwnVirtualKey -test.v -test.timeout 20m
```

**`-w` 那一行是必要的**：測試以相對路徑 `../../../../../../db/migrations` 找 migration。

### 它證明了什麼、沒證明什麼

**證明**：套件進物件儲存 → preflight → 確認 → 派送 → sandboxd → 容器跑 Agent SDK → 每 Run 短效 Virtual Key 經閘道呼叫模型 → trace 回推 → artifact 收集 → 金鑰撤銷，**整條在今天仍然是通的**。

**沒證明**：這一輪的 runtime 是 `runc` 不是 `runsc`，所以它不是 SEC-009 的任何一項；`--network skillhub_egress` 是 Docker 網路隔離，**不是** ADR-022 Q3 的 nftables 強制層（那條路的實驗室在 `tools/sec009/t5-network-egress.sh`）。

## 完成判準

一次 automation 變更至少通過：

- `go -C tools/devctl test ./...`
- `go -C tools/devctl run . automation-check`
- `task gen:check`
- 受影響語言的 typecheck/test/build
- `git diff --check`

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
