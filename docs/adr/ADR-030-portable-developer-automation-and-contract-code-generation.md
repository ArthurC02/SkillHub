# ADR-030：可攜式開發自動化與契約程式碼生成

- 狀態：Accepted
- 日期：2026-08-18
- 決策者：產品負責人、架構規劃

## 背景

ADR-016 已要求跨語言介面先寫 OpenAPI，並由 codegen 產生 TypeScript client 與 Python stub；ADR-019（仍為 Proposed）提出的 Taskfile 薄入口與 CI drift check 已實際落地一部分，頂層目錄則以 ADR-024 的 Accepted 修訂為準。本 ADR 只承接這些已落地的接點，不把 ADR-019 其餘待決事項視為 Accepted。實作截至本 ADR 前只完成 `sqlc generate`：`Taskfile.yml` 的 `gen` 仍是 placeholder，CI 只 lint OpenAPI，前端與 Python 仍有手寫契約型別。

這不只是少一個 generator。專案由多個 Coding Agent 與不同實體電腦共同開發；目前工具版本散在 `go.mod`、CI 與各 package，Windows 上亦實測出以下差異：

- 開發機可能沒有 Task；
- 本機 Go／Node 版本可能與 repo／CI 不同；
- `uv` 的 hardlink 在部分 Windows／跨檔案系統暫存目錄失敗；
- 未指定 UTF-8 時，Python generator 會以 CP950 讀取含繁體中文的契約而失敗；
- 多 Agent 共用同一工作樹時，generator、formatter、lockfile 與 Git 寫入不可安全平行。

因此自動化必須同時解決可重現、可發現、共享工作樹協作與 generated output ownership，不能只新增一條本機命令。

## 評估選項

### 選項 A：只補 OpenAPI generator

- 優點：改動最小。
- 缺點：換電腦仍會因版本、Task、locale 或 link mode 失敗；Agent 也未必知道命令存在。只解決產物，不解決執行契約。

### 選項 B：重型 monorepo build system

採 Bazel、Nx 或同級工具統一依賴圖、快取與 codegen。

- 優點：集中式 task graph 與 cache。
- 缺點：推翻 ADR-019「原生工具＋薄共同入口」；Go／Python 會成為 JS 工具的外部黑盒，維護成本與目前規模不成比例。

### 選項 C：薄 Automation Contract＋可攜 devtools＋釘選 generator（採用）

- Taskfile 是人類、Agent 與 CI 的穩定入口。
- 小型 Go `devctl` 負責 doctor、generation orchestration、lock 與跨平台差異，不取代原生 package manager。
- Dev Container／devtools image 是換電腦時的最低依賴路徑；原生工具鏈仍保留。
- generated output 入 repo，由 CI 重生後檢查零 diff。
- 多 Agent 目前共用工作樹，不強制 worktree；SubAgent 預設唯讀，主 Agent 是單一 Writer。

此選項補齊 ADR-019 而不引入新的 build layer。

## 契約生成相容性實測

2026-08-18 對目前三份 OpenAPI 3.1 契約進行不落 repo 的暫存生成：

| 目標 | 候選與版本 | 結果 | 裁定 |
| --- | --- | --- | --- |
| Go | `oapi-codegen v2.5.1` | 失敗；明示尚不支援 3.1，遇到 `type: [string, "null"]` 即終止 | 不採用，不降級契約 |
| Go | `ogen v1.16.0` | 失敗；nullable union 仍被解析為只能是單一字串 type | 不採用舊版 |
| Go | `ogen v1.24.0` | 成功；完整 `public.yaml` 產生 20 個檔案；只有既有 enum coercion 與無 default response 警告 | 採用並釘選 |
| TypeScript | OpenAPI Generator `7.19.0`／`typescript-fetch` | 成功；`type: [string, "null"]` 產為 `Date \| null`，operationId 產為 typed client method | 採用並釘選 |
| Python | `datamodel-code-generator 0.35.0`／Pydantic v2 | 在 `UV_LINK_MODE=copy` 與 `--encoding utf-8` 下成功 | 採用並把兩個參數寫死在共同入口 |

測試也證明工具「能在某次命令產檔」仍不足以宣告可攜：Python 的兩個 Windows 失敗若沒有固定參數，在另一台機器會重現。

## 決策

### 1. Automation Contract

以下名稱是穩定的人機介面，底層實作可以演進：

- `task doctor`：只讀檢查工具版本、Docker、環境變數與可用 profile；秘密只顯示是否存在。
- `task bootstrap`：準備可自動取得的依賴；不可自動安裝者給出明確診斷。
- `task env:init`：由樣板建立本機設定，絕不覆蓋既有 `.env`。
- `task gen`／`task gen:check`：重生全部產物／檢查重生後零 drift。
- `task gen:sql`／`gen:openapi`：限定範圍的生成入口。
- `task dev:core`／`dev:model`／`dev:sandbox`：把無秘密的核心開發、需要模型憑證與付費的路徑分開。
- `task ci`：提交前的主要本機檢查。

Taskfile 維持薄索引；複雜但跨平台的協調放在 `tools/devctl`。

### 2. 工具版本與可攜執行

- 語言 runtime 的權威來源維持原生檔：Go 讀 `go.mod`，Node 讀 repo 的版本檔，Python 讀 `.python-version`。
- 無原生版本來源的 generator／linter 才寫入機器可讀的 `tools/toolchain.yaml`；CI、devctl 與 devtools image 必須讀同一值，不另抄一份浮動版本。
- Dev Container／devtools image 提供乾淨 clone 的建議路徑；本機只需 Git、Docker 與支援容器的執行端。
- 原生路徑仍受 `doctor` 驗證，不把「PATH 上剛好有某版本」視為通過。
- CI 的 codegen 一律在以 digest 指定的 canonical devtools image 內執行；原生與 Dev Container 路徑呼叫同一 devctl，並由 CI 反證其輸出與 canonical image 相同。

### 3. Generator 與輸出

- Go：`ogen v1.24.0`，先生成 transport types／介面並以一個低風險 domain pilot；`router.go` 的 AuthN/AuthZ matrix 仍是人工可審查的決策點。
- TypeScript：OpenAPI Generator `7.19.0` 的 `typescript-fetch` target，輸出至 `packages/api-client-ts/`。
- Python：`datamodel-code-generator 0.35.0` 產 Pydantic v2 models，輸出至 `packages/api-stub-py/`；FastAPI route 保持手寫薄 adapter，政策與狀態轉移不進 Python。
- SQL：維持 `sqlc 1.31.1` 與既有 Go output。
- 所有 generated output 入 repo；檔頭與目錄 README 必須指出 source 與重生命令，人工不得編輯。
- 生成先寫暫存目錄，成功才替換；同一工作樹有 generation lock；失敗不得留下半套產物。
- 輸出一律為 LF，且不得含時間戳、hostname、絕對路徑或秘密；CI 在 drift diff 前執行 content check，不靠 reviewer 目測。
- generator 升級只能用獨立 PR：先更新 manifest，在 Windows 與 canonical Linux image 跑 golden payload／compile tests，再由人工審查語意 diff；若回歸則回退 manifest 與產物，不在 consumer 端堆相容 workaround。
- 新 OpenAPI construct 若任何既定 generator 不支援，契約 PR 必須停在 CI；先評估 generator 升級或以等價、仍為 OpenAPI 3.1 的 schema 表達。不得維護 3.0 轉換副本、使用 `--skip-validate-spec` 掩蓋問題，或手改 generated output。若無等價表達，新增 ADR 變更 generator／邊界後才可合併。

### 4. 共享工作樹協作

- 不強制 SubAgent 使用 worktree。
- SubAgent 預設唯讀；主 Agent 是單一 Writer。
- 寫入 SubAgent 必須取得精確 path allowlist，且不得自行執行 repo-wide generator、formatter、package install、Compose stop、Git 寫入或 lockfile 更新。
- `contracts/`、`db/migrations/`、`db/queries/`、generated 目錄、lockfile、Taskfile 與 CI workflow 是高衝突區，由主 Agent序列化。
- 維持禁止 `git stash`；未知修改不得 reset／clean／checkout 還原；stage 只用明確 pathspec。
- Worktree 與多 Compose instance 保留為日後出現長期平行寫入事故時的重評選項，不是目前基線。

generation lock 的語意是 advisory file lock：devctl 取得 repo-local、gitignored 的 lock 後才可產生；第二個程序 fail-fast 並指出持有程序，程序結束即釋放，stale lock 以 PID 存活與建立時間判定。CI 雖只有一個 generator job，仍走同一路徑，避免本機與 CI 形成兩份實作。

### 5. 可發現性

自動化必須同時出現在：

1. `AGENTS.md` 的短版強制流程；
2. `task --list` 的完整 `desc`；
3. `docs/development/automation.md` 的詳細操作與排錯；
4. generated file header／目錄 README；
5. CI drift failure 的本機修復指令。

Agent-specific 指引只能引用上述權威來源，不複製一份會漂移的規則。若加入 repo-local Agent Skill，它只能呼叫同一 Task／devctl，不得成為唯一可用入口。

## 漸進導入與停止條件

1. 可攜性基線：doctor、toolchain、`.env.example`、profile 與 devtools image。
2. 生成基礎：deterministic `task gen`、lock、原子替換與 CI drift。
3. 先導入 TypeScript client，再導入 Python models。
4. 最後以低風險 Go domain 試行 ogen server interface。
5. 用新 Agent 黑箱任務驗證它能自行找到 automation。

若 Go pilot 需要繞過 generated interface 才能表達 session/workspace scope、讓 AuthZ 變得不透明，或 adapter 比原 handler 更長，Go 停在 models-only；TS、Python 與 SQL codegen 不回退。

## 驗收準則

- 乾淨 clone 在沒有模型金鑰時可完成 doctor、core bootstrap、unit test 與 generation check。
- Windows 與 Linux 對同一 commit 產生 byte-for-byte 相同 output。
- generator 第二次執行零 diff；任一 generated file 被手改，CI 必須失敗並顯示修復命令。
- Core／Model／Sandbox profile 明確分離，付費測試不會被一般 `task test` 意外觸發。
- 多個唯讀 SubAgent 可平行；同一時間最多一個 Writer／generator。
- 全新 Agent 未被口頭提示時，仍能從 repo 指引找到 `task doctor`、`task gen`、generated ownership 與共享工作樹規則。

## 影響

### 正面

- 新電腦不再依賴開發者記得散落於 CI 的版本與秘密。
- OpenAPI 修改會在同一 commit 自動傳到 TS／Python／Go 邊界，鐵律 12 成為真正的機器閘門。
- Coding Agent 能在修改前發現正確入口，且不會因共享工作樹誤清理他人變更。

### 成本與限制

- generated output 入 repo 會增加 diff；版本升級必須用獨立 PR 重生並審查。
- devtools image 與原生路徑都需維護，但兩者共用 devctl 與版本來源，不各自複製邏輯。
- `ogen` 對現有 enum 有 coercion 警告；進入 Go pilot 前必須以 golden test 固定實際語意。
- Dev Container 不能取代 SEC-009 的真實節點與 gVisor 驗收。

## 待決策

無。實作若推翻 generator 選型、共享工作樹模式或 OpenAPI 3.1 保留決策，新增 ADR，不原地改寫本文。

## 補記（2026-08-29）：Go pilot 的停止條件觸發了，Go 維持 models-only

**不改寫上方任何一段決策文字。**

本 ADR 為 `ogen` 的 Go pilot 訂了一條停止條件（enum coercion 語意必須先以 golden test 固定）。**該條件已觸發**：generated server 的 AuthZ 行為與 `router.go` 逐條套用的 `RequireSession`／`RequireOperator`／`OptionalSession` 不是同一份語意，而 `AGENTS.md` 開發自動化紅線第 6 條把它寫成硬邊界——ogen server 只在精確的 `GET /healthz` pattern 之後，**不得整批 mount**。

**結論：Go 側維持 models-only。** codegen 產 TS／Python stub 與 Go **model**，handler 由人寫並逐條對齊 `public.yaml`（`devctl` 的 route-table 檢查在守這件事）。**這不是失敗，是停止條件按設計運作了一次。**

**還沒做的那一件事，明確記為待辦而不是已完成**：把 generated server 的多餘部分裁掉、讓 `packages/api-*` 只輸出 models。**今天沒有做**，落點是 `04` 丙-89。在裁掉之前，repo 裡存在一份**沒有任何人 mount 的 generated server**——它不會壞事，但它會讓下一個人以為那條路是通的。

**同批訂正一句寫在 `AGENTS.md` 技術棧表裡的事實**：spec 的來源是 `contracts/openapi/public.yaml`，**Go 是下游不是來源**（原文寫「Go 為 spec 來源」）。鐵律 12「先寫 OpenAPI schema 再實作」講的一直是這件事，只有速覽表那一列把方向寫反了。
