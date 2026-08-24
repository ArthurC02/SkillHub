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

### 活文件裡的識別字必須存在

同一個家族的第二條，2026-08-24 加入。**文件的散文檢查不了，但散文裡的識別字可以**——而一個死掉的識別字，通常是一句死掉的主張穿著它。

實際抓到的三個：`04` 拿一個叫 `PublicSearchHit` 的型別論證「我的 Skill 清單少了四項證據」（真名是 `PublicSearchResult`）；`04` 丙-38 的結案數字掛在 `ApplyPreview` 上（真名是 `improvement.Diff`／契約 `SuggestionDiff`，那個名字是 m3 報告發明的，被抄了三次）；`03` 在函式刪掉之後還說它「已備好」。**三個都是六輪對抗式審查裡人工抓到的，而它們是機器抓得到的那一類。**

**範圍只有活文件**（`AGENTS.md`、`docs/plans/01`～`05`、`docs/design/`、`m5/README`），而這個限制是整個設計：同一種檢查套在 ADR 與凍結的里程碑報告上，量到 **18 個命中，每一個都是「寫的當下是對的」的歷史**（DDD 搬檔、已刪除的 spike），而 AGENTS.md 明文要求不要順手修正那些。**一個會要求人違反明文規則的檢查，比沒有檢查更糟——人會學會忽略它。**

量測值（寫下時）：410 個引用、6 個命中、3 個是真的。誤報那三個由 `allowedDocWords` 記著理由，形狀比照 `db/query-owners.yaml` 的 `allow:`——**是存量清單，不是擴充點**。

兩條規則：

1. **死掉的名字不要穿反引號。** 反引號的意思是「這是一個真的符號」；訂正句裡提到一個從來不存在的名字時寫成純文字，檢查就不會命中，而讀者看到的資訊完全一樣。
2. **`declared` 是「這個字出現在任何一個程式檔裡」，不是「這個符號有宣告」。** 便宜、不需要 parser，代價是**一個被刪掉但名字還留在某段註解裡的函式會溜過去**。寫這個檢查的當天就踩到了：它自己的說明註解引用了三個要抓的名字，於是把它們全部漂白——所以掃描時跳過 `doc_identifiers.go` 自己。

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
