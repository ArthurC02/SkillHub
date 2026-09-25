# ADR-014：開發自動化與依賴治理

- 狀態：Accepted
- 相關：[ADR-004 Sandbox 隔離與執行安全](./ADR-004-sandbox-isolation-and-execution-security.md)（Runtime Image 的漏洞門檻與 Agent SDK 釘選）、[ADR-013 Repo 結構、CI 與驗證層](./ADR-013-repository-layout-ci-and-verification-tiers.md)（CI/CD 基線與 automation-check）

## 背景

專案由多個 Coding Agent 與不同實體電腦共同開發，且跨語言介面（OpenAPI、SQL）需要在同一次修改內把 Go、TypeScript、Python 三邊的型別同步好。開發自動化必須同時解決四件事：換一台電腦也能重現同一份工具鏈、命令要讓人與 Agent 都能自行發現、多個 Agent 共用同一份工作樹時的寫入安全，以及生成產物的所有權。另一方面，依賴（npm、Go module、Python package）、容器映像與 GitHub Actions 都是供應鏈風險：漏洞、授權、未釘選版本與未受管控的安裝腳本都可能在與目前工作無關的時間點造成事故；治理規則必須是機器閘門，不能只靠人記得。

## 決策

### 決策 1：Automation Contract 是唯一入口

以下命令名稱是人類、Agent 與 CI 共用的穩定介面，底層實作可以演進，但名稱與語意不得漂移：

- `task doctor`：唯讀診斷 runtime、Task、uv、Docker、Compose 與 `.env` 是否存在，不讀值、不需秘密。
- `task env:init`：由 `.env.example` 建立 `.env`，已存在時不覆寫。
- `task bootstrap`：取得可自動安裝的依賴（Go download、npm ci/build、uv frozen sync）；不可自動安裝者給出明確診斷。
- `task dev`／`dev:core`：只啟動 Postgres 與 SeaweedFS，不需秘密。
- `task dev:model`／`dev:sandbox`：把需要模型憑證、會產生費用的路徑與需要 Sandbox 的路徑分開；不得由唯讀 SubAgent 自行啟動。
- `task gen`／`task gen:check`：重生全部生成產物／檢查重生後零 drift；`task gen:sql`、`task gen:openapi` 是限定範圍的入口。
- `task ci`：提交前的主要本機檢查；Model／Sandbox profile 各自分離，付費測試不會被一般 `task test` 意外觸發。

Taskfile 維持薄索引，跨平台的協調邏輯集中在 `tools/devctl`；語言 runtime 版本一律讀原生檔（Go 讀 `go.mod`，Node 讀 `.node-version`，Python 讀 `.python-version`），沒有原生來源的 generator／linter／掃描器版本才寫進機器可讀的 `tools/toolchain.yaml`，CI、devctl 與 devtools image 讀同一份值。乾淨 clone 在沒有模型金鑰時，必須能完成 doctor、core bootstrap、unit test 與 generation check。

自動化必須同時出現在 `AGENTS.md` 的強制流程、`task --list` 的 `desc`、`docs/development/automation.md` 的操作細節、生成檔案的檔頭／目錄 README，以及 CI drift 失敗訊息裡的本機修復指令；Agent 專用指引只能引用這些來源，不得複製一份會漂移的規則。

### 決策 2：契約程式碼生成——釘選版本、原子替換、Go 只出 model

跨語言契約（`contracts/openapi/*.yaml`、`db/queries/`）先修改來源，再由生成器產出下游型別，不手改 generated output：

- SQL：`sqlc` 產生 Go 型別。
- TypeScript：OpenAPI Generator 的 `typescript-fetch` target，輸出至 `packages/api-client-ts/`。
- Python：`datamodel-code-generator` 產 Pydantic v2 models，輸出至 `packages/api-stub-py/`；FastAPI route 保持手寫薄 adapter。
- Go：`ogen` 只生成 transport model，**不生成、不掛載 server interface**；HTTP handler 與逐路由的 `RequireSession`／`RequireOperator`／`OptionalSession` 由人工撰寫並對齊 `public.yaml`。

CI 的 codegen 一律在以 digest 指定的 canonical devtools image 內執行；原生與 Dev Container 路徑呼叫同一份 `devctl`，並由 CI 反證兩者輸出與 canonical image 相同——Windows 與 Linux 對同一個 commit 必須產生 byte-for-byte 相同的 output。所有 generated output 入 repo，檔頭與目錄 README 指出來源與重生命令。生成先寫暫存目錄，成功才原子替換；同一時間只有一個生成程序持有 repo-local、gitignored 的 advisory lock，第二個程序 fail-fast 並指出持有者；CI 走同一條路徑，不另有一份實作。輸出一律 LF，不得含時間戳、hostname、絕對路徑或秘密，CI 在 drift diff 前先做內容檢查。generator 升級只用獨立 PR：先更新版本清單，在 Windows 與 canonical Linux image 跑 golden payload／compile test，人工審查語意差異後才合併；若某個新的 OpenAPI 語法任何既定 generator 都不支援，契約 PR 停在 CI，不得手改 generated output 或以 `--skip-validate-spec` 之類的旗標掩蓋問題。

### 決策 3：共享工作樹下的協作邊界

多個 Agent 共用同一份工作樹，不強制使用獨立 worktree：

- SubAgent 預設唯讀；同一時間只能有一個 Writer（通常是主 Agent）。
- 寫入 SubAgent 必須有精確的 path allowlist，且不得自行執行 repo-wide 的 generator、formatter、package install、Compose down、Git 寫入或 lockfile 更新。
- `contracts/`、`db/migrations/`、`db/queries/`、generated 目錄、lockfile、`Taskfile.yml` 與 CI workflow 是高衝突區，一律由主 Agent 序列化執行。
- 禁止 `git stash`；對不屬於自己的修改禁止 `git reset`、`git clean`、`git checkout -- <path>`；stage 只用明確 pathspec。

### 決策 4：決策理由與估算依據只寫在 ADR、文件與 commit message

程式碼註解不承載決策理由、估算依據、需求或裁定編號、日期、實測數字；這些只寫在 ADR、`docs/` 與 commit message，程式碼只用命名表達意圖。`automation-check` 的 `comment-budget` 以零容忍守住這條規則；一個艱難演算法區塊最多容許三行說明「怎麼運作」，不得說明「為什麼這樣做」。任何測試或程式的估算依據需要被複查時，複查者讀的是對應主題的 ADR 與 `docs/plans/`，不是程式碼。

### 決策 5：依賴漏洞閘門——只擋有修補版的，只擋會出貨的程式

判準是「有沒有可以升級的版本」（npm 的 `fixAvailable`、govulncheck 的 `fixed_version`、pip-audit 的 `fix_versions`）；沒有修補版的印成 `NOTE`、不擋——擋了也無法修正，只會讓人習慣紅燈。過濾邏輯集中在 `devctl dep-audit`，本機與 CI 跑同一條命令；工具輸出不是一份完整報告時一律當錯誤，不當成「沒有漏洞」。

push／PR 只擋會出貨的程式：

| 專案 | 掃什麼 | 門檻 |
| --- | --- | --- |
| `apps/web` | `npm audit --omit=dev` | high 以上 |
| `apps/platform`、`apps/sandbox` | govulncheck 的 symbol 掃描 | 程式真的呼叫得到的 |
| `apps/llm` | 非 dev 依賴（pip-audit） | 全部 |

`dependency-audit` job 不設 path filter，每次都跑，映像推送等它綠了才進行。每週排程額外跑 `--full`：repo 內每個有 lockfile 的專案（含 dev 依賴、含 Go 模組層未被呼叫到的部分）都掃一次，紅了不擋任何人，是那一週的報告；名冊由測試守住，新專案沒登記或登記了卻沒有 lockfile 都會失敗。Runtime Image 的映像層漏洞不在這裡管，見決策 9。

### 決策 6：依賴治理——更新節奏與安裝防護

新版本更新交給 GitHub 內建的 Dependabot：npm、Go、uv 每週一次，Actions／Dockerfile／compose 每月一次；冷卻期 7 天（npm major 14 天）；minor 與 patch 合成一個 PR，major 各自一個。以下項目換版會改變生成產物或需要付費重驗，不自動升，只收安全更新：產碼工具（`tools/codegen/*`、`tools/devctl`）、`tools/pglite`、Runtime Image 的 Agent SDK。安全更新不受冷卻期與這份「只收安全更新」清單的上限影響，照樣立刻放行。`automation-check` 的 `dependency-policy` 要求每個有 lockfile、`go.mod`、Dockerfile、compose 或 composite action 的目錄都登記在 `dependabot.yml`。

安裝時的防護：

- 每個 npm 專案的 `.npmrc` 設 `ignore-scripts=true`；映像建置的 `npm ci` 加 `--ignore-scripts`。
- 每個 uv 專案設 `exclude-newer = "7 days"`，與 Dependabot 的冷卻期對齊；npm 側在四份 `.npmrc` 設 `min-release-age=7`，同樣在解析時套用冷卻期。
- CI 的 Go job 在 `setup-go` 之後跑 `go mod verify`，避免被下毒的 module 快取繞過 `go.sum` 比對。

### 決策 7：依賴治理——授權閘門與新增依賴准入

出貨依賴（`apps/web`、`apps/platform`、`apps/sandbox`、`apps/llm`、Runtime Image）的授權由 `devctl dep-audit` 檢查，允許清單為 MIT、MIT-0、ISC、BSD-2-Clause、BSD-3-Clause、0BSD、Apache-2.0、Zlib、Unlicense、CC0-1.0、BlueOak-1.0.0、Python-2.0、PSF-2.0、MPL-2.0；GPL、LGPL、AGPL、SSPL、BUSL 與商業授權不在清單上。MPL-2.0 是檔案層級 copyleft：不修改對應檔案就沒有公開義務。SPDX 表達式的 `OR` 有一邊允許即通過，`AND` 要兩邊都允許；認不出來、空白或 `SEE LICENSE IN` 一律不過。個別套件的具名例外（例如商業條款、分類器誤判）連同理由寫在授權檢查程式裡，且只對該生態系、名字家族與授權字串本身有效。開發用、不出貨的工具不受此閘門管。

GitHub Actions 由 zizmor 稽核 `.github/workflows` 與 `.github/actions`，medium 以上即失敗；CI 帶 token 做連網稽核（涵蓋已知有漏洞的 Action、版本註解與該 SHA 實際版本不一致等），本機用離線模式。`dependency-policy` 額外離線檢查釘選格式是否存在：每個 tracked Dockerfile 的 FROM 都要 digest、compose 與 workflow 裡的 `image:` 都要 digest、每個 `uses:` 都要 40 碼 SHA 並附版本註解。

新增一個直接依賴（任何生態系）之前，先確認：標準庫、平台原生功能或已裝依賴做不做得到；授權在允許清單上；是否仍有人維護；會多拉進幾個傳遞依賴；是否需要 install script（`ignore-scripts` 會讓它失效，若確實需要即是一個要寫成 ADR 的例外）。`apps/web` 新增執行期依賴仍需一份 ADR（見 [設計系統、信任訊號與畫面用語](./ADR-020-design-system-trust-signals-and-screen-words.md)）。

### 決策 8：工具鏈版本跟著上游走，機器要求所有位置一致

工具鏈版本只保留一條忽略——`@types/node` 的 major 跟著 `.node-version` 走，Node 本身升主版本時同一批一起升；Dependabot 對 Node、Python、Go、uv 基底映像與相關建置後端照常提出升級，審查時依各工具的 LTS／穩定節奏決定是否採用。`automation-check` 的 `dependency-policy` 對同一個工具（node、go、python、uv、task、golangci-lint、syft、grype 等）在每個出現位置（版本檔、toolchain.yaml、各 Dockerfile、CI 設定）做一致性比對：任一位置的版本對不上，或某個位置讀不到版本，一律失敗並列出所有位置目前的值，避免升級只改到一處而在不同檔案間分岔。CI 一律從專案的版本檔（如 `.node-version`、`.python-version`、`go.mod`）讀取版本，不在 CI 設定裡另外寫一份版本號。Runtime Image 的基底映像版本因升級需連動 Agent SDK 付費重驗，維持獨立節奏，不受此一致性比對約束。

### 決策 9：映像漏洞閘門——依「誰能修」與「會不會被部署」分三層

映像掃描的清單直接從 `infra/compose/docker-compose.yml` 解析，不在腳本裡另抄一份；每顆映像留存 SBOM、完整掃描報告與「只看有修補版」報告。閘門分三層：

| 層 | 映像 | 有可修的 Critical／High 時 |
| --- | --- | --- |
| 會被部署 | 我們自建且會部署的應用映像（`platform`、`web`、`llm`） | job 失敗；修法是換基底、升依賴，或對單一套件做定點升級而不動整體釘選 |
| 上游 | compose 拉下來的第三方映像 | 只報告；只有每週排程才失敗，觸發後決定要升的目標版本或改換來源，不放寬閘門 |
| 開發專用 | 開發容器（不部署、不處理使用者資料） | 永不失敗，只留存報告 |

掃描器（syft、grype）版本進同一份工具鏈名冊，任何一處單獨升級都會被決策 8 的一致性比對擋下。Runtime Image 不在這張表裡：它的掃描與發布流程的 attestation 綁在一起，走獨立節奏、不併入這裡的每週排程，門檻也比「會被部署」那一層嚴——可修的 Critical／High 沒有豁免路徑，不可修的要逐項具名並附複審日，掃描結論本身還有有效期。規則在 [ADR-004](./ADR-004-sandbox-isolation-and-execution-security.md) 決策 12。

## 影響

### 正面

- 新電腦或新 Agent 不必依賴散落於 CI 的版本與秘密即可自行找到入口；契約修改能在同一次生成中傳到 TypeScript／Python／Go 邊界。
- 多個唯讀 SubAgent 可平行工作，且不會因共用工作樹誤清理他人尚未提交的修改。
- 依賴、授權與映像的供應鏈風險由機器閘門持續守住，不依賴人記得在每次新增依賴或升級版本時手動檢查。

### 成本與限制

- generated output 入 repo 會增加 diff；版本升級一律要獨立 PR 重生並人工審查語意差異。
- 每次 push 增加漏洞、授權與 workflow 稽核的並行檢查時間；外部公告資料庫（npm registry、Go vulndb、PyPI、GitHub API）不可用時該次檢查會失敗，需以錯誤訊息判斷是否為外部服務問題而非真有漏洞。
- 授權判定以套件中繼資料（lockfile 欄位、PyPI 分類器）為準，不掃描原始碼裡被複製貼上的授權文字。
- 冷卻期只防得住「帳號被盜用到被發現」之間的窗口，防不了維護者本人作惡或潛伏更久的惡意版本。
- 一次工具鏈升級通常要改動多個檔案；一致性檢查能指出還缺哪些位置，但不會代為修改。
