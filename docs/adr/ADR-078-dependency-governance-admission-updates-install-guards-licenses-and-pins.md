# ADR-078：依賴治理——准入、更新、安裝防護、授權與釘選

- 狀態：**Accepted**（2026-09-12，負責人指示：「繼續，完善整個套件管理，先分析現況，你可以參考網路資訊，思考最佳治理方案並且實現」）
- 日期：2026-09-12
- 相關：[ADR-077](./ADR-077-dependency-vulnerabilities-block-only-when-a-fix-exists.md)（漏洞閘門；本 ADR 在同一條 `devctl dep-audit` 加上授權與 workflow 稽核）、[ADR-022](./ADR-022-sandbox-deployment-topology-and-security-thresholds.md)（映像 SBOM 與 grype）、[ADR-023](./ADR-023-agent-sdk-version-pinning-and-behaviour-revalidation.md)（Agent SDK 的釘選與重驗）、[ADR-030](./ADR-030-portable-developer-automation-and-contract-code-generation.md)（產碼工具版本）、[ADR-076](./ADR-076-backoffice-charts-use-chartjs-and-show-only-aggregates.md)（圖表套件的授權逐層調查）、[system.md §4.8](../design/system.md)

## 背景

2026-09-12 以四個唯讀代理盤點 npm、Go、Python、映像與 Actions 的現況，另以三個代理調查更新工具、2025–2026 的供應鏈事件與對策、授權檢查工具。

**已經做對的**：每個專案都有 lockfile（npm 的 integrity、`go.sum`、帶 hash 的 `uv.lock`），CI 只用 `npm ci` 與 `uv sync --frozen`；七個 Dockerfile 的 FROM 都釘了 digest，workflow 的 `uses:` 都釘了 40 碼 SHA；兩個 Go 服務 `go mod verify` 通過；ADR-077 的漏洞閘門。

**缺口**：

1. **沒有任何自動更新**。`apps/web` 18 個直接依賴有 12 個落後；`openai` 落後一個主版本；`riverqueue/river` 從 0.43 落後到 0.47。升級只在出事時發生。
2. **沒有冷卻期**。2025 年 9 月 chalk/debug 維護者帳號被釣魚，以及同月開始的 Shai-Hulud 蠕蟲，惡意版本都在幾小時到幾天內被下架；在那個窗口裡跑 `npm install` 或 `uv lock` 的人就中了。
3. **npm 的 install script 預設會跑**，repo 裡沒有 `.npmrc`。Shai-Hulud 第二波就是放在 preinstall。
4. **沒有授權檢查**。ADR-076 的授權調查是一次性的人工作業，下一個依賴沒有人會做。盤點當下出貨的依賴都可接受（MPL-2.0 的 river、certifi、orjson、tqdm；runtime image 的 Agent SDK 是 Anthropic 的商業條款），但沒有東西保證下一個也是。
5. **新增依賴沒有准入規則**。只有 `apps/web` 有 system.md §4.8；Go 與 Python 加一個 module，不會碰到任何人或任何檢查。
6. **釘選不齊**。compose 的四個映像只有 tag，litellm 用的是會浮動的 `main-stable`；只有 runtime image 的 FROM 有機器檢查（image-gate）。
7. **GitHub Actions**。zizmor 對現有 workflow 報了 19 個 checkout 把憑證留在工作區（artipacked），以及 7 個 high 的 template injection（`${{ }}` 直接展開進 shell，其中三處是 `secrets.GITHUB_TOKEN`）。tj-actions/changed-files（2025 年 3 月）示範過：Action 本身就是依賴。

## 決策 1：更新交給 Dependabot，不用 Renovate

- Renovate 能做的比較多：可以用 regex 管 `tools/toolchain.yaml`，npm 的冷卻期在解析時就用 `--before` 強制。但它要安裝第三方 GitHub App，並給它改寫 `.github/workflows` 的寫入權——那個 App 本身就成了一個供應鏈節點，而且只能由負責人在瀏覽器上安裝。Dependabot 是 GitHub 內建的，推一個設定檔就生效。
- Renovate 多出來的兩件事，這裡用別的方式補：toolchain.yaml 綁住的版本本來就該人工連同 toolchain.yaml 一起升（下面「不自動升」的清單）；解析時的冷卻期，uv 由 `exclude-newer` 做（決策 2），npm 目前做不到（見限制）。
- `.github/dependabot.yml`：
  - npm（`apps/web`、`packages/api-client-ts`）、Go（`apps/platform`、`apps/sandbox`）、uv（`apps/llm`）每週一 06:00（台北）；GitHub Actions、Dockerfile、compose 每月一次。
  - 冷卻期 7 天，npm 的 major 14 天。minor 與 patch 合成一個 PR，major 各自一個；Actions、映像、compose 各合成一個。
  - **不自動升、只收安全更新**（`open-pull-requests-limit: 0`）：`tools/pglite`（版本在 toolchain.yaml 與 doctor 對帳）、`infra/images/runtime-agent-sdk`（ADR-023：升級要付費重驗）、`tools/codegen/*` 與 `tools/devctl`（產碼工具換版會改產出，ADR-030）。Dockerfile 的更新忽略 `astral-sh/uv`（跟 toolchain.yaml 的 `uv` 一起動）與 `node`（跟 `.node-version` 一起動）。
  - 安全更新不受冷卻期與上限影響。
- automation-check 的 `dependency-policy` 要求：每個有 lockfile、`go.mod`、Dockerfile、compose 或 composite action 的目錄，都要列在 dependabot.yml 裡。新專案不會默默漏掉。

## 決策 2：安裝時的防護

- **每個 npm 專案一份 `.npmrc`，`ignore-scripts=true`**。盤點當下只有 macOS 才會裝的 `fsevents` 帶 install script。明確呼叫的 `npm run`／`npm test` 照常執行（npm 擋的是依賴的 lifecycle script 與專案自己的 pre/post，repo 沒有 pre/post）。web 映像的 `npm ci` 加上 `--ignore-scripts`，因為它只複製 lockfile，讀不到 `.npmrc`。
- **runtime image 的 Dockerfile 這次不動**：它一改就要升 `IMAGE_VERSION` 並付費重驗（ADR-023），而它的依賴樹今天沒有任何 install script。下一次因為別的原因重建時一起加。
- **uv 專案設 `[tool.uv] exclude-newer = "7 days"`**：解析時看不到 7 天內發佈的版本，與 Dependabot 的冷卻期對齊。lockfile 只多一段 `[options]`，版本一個都沒變；`uv lock --check` 與 `uv sync --locked` 都照常通過。
- **CI 的 platform／sandbox job 在 setup-go 之後跑 `go mod verify`**：模組快取是從 `actions/setup-go` 的快取還原的，被下毒的快取在 `go build` 時不會重新比對 `go.sum`。

## 決策 3：出貨依賴的授權閘門

- `devctl dep-audit` 每次（push、PR、每週）檢查五個出貨專案：
  - `apps/web` 與 `infra/images/runtime-agent-sdk`：lockfile 裡非 dev 的套件，授權取自 lockfile 的 `license` 欄位；
  - `apps/platform` 與 `apps/sandbox`：go-licenses 2.0.1 讀每個套件的 LICENSE；
  - `apps/llm`：`uv export --no-dev` 列出的每個套件，授權取自 PyPI 的 `license_expression`，沒有就用 `license` 欄位，再沒有就用 classifier。
- 開發工具不檢查：不出貨，就沒有散布義務。
- **允許清單**：MIT、MIT-0、ISC、BSD-2-Clause、BSD-3-Clause、0BSD、Apache-2.0、Zlib、Unlicense、CC0-1.0、BlueOak-1.0.0、Python-2.0、PSF-2.0、MPL-2.0。MPL-2.0 是檔案層級的 copyleft：不修改那些檔案，就沒有公開其他程式碼的義務；改了，就要公開改過的那幾個檔案。GPL、LGPL、AGPL、SSPL、BUSL 與商業授權都不在清單上。
- **SPDX 表達式照規則判讀**：`A OR B` 有一邊允許就過，`A AND B` 要兩邊都允許；認不出來、空白、`SEE LICENSE IN` 一律不過。
- **具名接受**寫在 `tools/devctl/license_audit.go`，每一筆附理由，並印成 `NOTE`：
  - `@anthropic-ai/claude-agent-sdk` 家族：Anthropic 的商業條款，版本由 ADR-023 管理；
  - `github.com/segmentio/asm`：LICENSE 原文是 MIT No Attribution，go-licenses 的分類器認不出來，回報 `Unknown`。
- 接受只對那個生態系、那個名字家族、那個授權字串有效：同一個套件換了授權，就不再被接受。
- **沒有選的工具**：
  - Trivy：它的 uv 與 npm lockfile 模式不做授權偵測。
  - GitHub dependency review：repo 直接推 main、沒有 PR 流程，而且它不會對認不出來的授權失敗。
  - Anchore grant：讀的是 syft 的 SBOM，而 syft 在調查時還不支援 `uv.lock`。

## 決策 4：釘選交給機器檢查，不靠習慣

- automation-check 新增 `dependency-policy`，檢查：
  - 每個 tracked Dockerfile 的 FROM 都有 digest（指向前一個 stage 的除外）；
  - `infra/compose/` 與 workflow 裡的 `image:` 都有 digest；
  - 每個 `uses:` 都釘 40 碼 SHA，後面寫 `# vX` 版本註解。本地 `./` action 除外；`docker://` 要有 digest。
  - 決策 1、2 的三條（`.npmrc`、`exclude-newer`、dependabot.yml 的名冊）。
- image-gate 原本只看 runtime image；它判斷 FROM 的邏輯抽成了共用函式。
- **compose 的四個映像釘上 digest**：pgvector 與 seaweedfs 用和 CI 相同的 digest，本機與 CI 跑的是同一份；litellm 的 `main-stable` 與 prometheus 釘在 2026-09-12 的 digest，之後由 Dependabot 每月提案。
- **setup-python 的版本註解從 `# v5` 改成 `# v5.4.0`**：它釘的 SHA 是 v5.4.0，而 `v5` 標籤已經移走（zizmor 的 ref-version-mismatch）。

## 決策 5：GitHub Actions 由 zizmor 稽核

- `devctl dep-audit` 用 zizmor 1.30.1 稽核 `.github/workflows` 與 `.github/actions`，medium 以上就失敗。CI 帶 `GH_TOKEN`，需要連網的稽核（已知有漏洞的 Action、版本註解與 SHA 對不上等）一起跑；本機沒有 token 時用 `--offline`。
- **本批修掉的**：
  - 19 個 checkout 加上 `persist-credentials: false`。repo 是公開的，之後的 `git fetch` 不需要憑證。
  - 三處 `docker login` 改從環境變數讀 `GITHUB_TOKEN`。
  - 其餘在 `run:` 裡展開的 `${{ github.* }}`／`${{ steps.* }}`，都改成環境變數。
- 剩下 3 個 low 等級的 self-repository 建議不擋。

## 決策 6：新增依賴的准入

新增一個直接依賴（任何生態系）之前，先回答五個問題，答案寫進 commit message：

1. 標準庫、平台原生功能或已經裝的依賴做得到嗎？做得到就不加。
2. 授權在允許清單上嗎？`dep-audit` 會擋，但先查可以省一次來回。
3. 還有人維護嗎？最近一年有發佈、安全問題有回應（deps.dev 或 OpenSSF Scorecard 看得到）。
4. 會多拉進幾個傳遞依賴？
5. 需要 install script 嗎？`ignore-scripts` 會讓它失效；如果真的需要，就是一個要寫進 ADR 的例外。

`apps/web` 仍以 system.md §4.8 為準：新增執行期依賴要一份 ADR，前例是 ADR-076。`.claude/rules/dependencies.md` 會在代理動任何 manifest、lockfile、Dockerfile 或 workflow 時，把它指到這一段。

## 需要負責人做的一件事

在 GitHub repo 的 Settings → Code security 打開 **Dependabot alerts** 與 **Dependabot security updates**。盤點時兩者都是關的。`dependabot.yml` 管的是版本更新，安全更新要靠這兩個開關。本批嘗試用 API 開啟，被本機的自動權限規則擋下，留給負責人按。

## 成本與限制

- **npm 在解析時的冷卻期做不到**：`min-release-age` 要 npm 11.10 以上，本機是 11.6.2，CI 的 Node 22 帶的是 npm 10。人或代理在本機 `npm install` 新套件時，仍然拿得到剛發佈的版本；只有 Dependabot 的 PR 有冷卻期。npm 升到 11.10 之後，在 `.npmrc` 加一行就好。
- **每次 push 多約 40 秒**（本機量測；三個工具加上 PyPI 查詢）。PyPI、Go proxy 或 GitHub API 掛了會紅，看 `devctl:` 開頭的錯誤訊息分辨。
- **授權以中繼資料為準**：Python 看 PyPI 的欄位，不讀 LICENSE 原文；npm 看 lockfile 的 `license` 欄位（來自各套件自己的 package.json）。沒有做 ScanCode 那一層（掃描原始碼裡被複製進來的授權文字）。
- **冷卻期擋不住潛伏更久的惡意版本，也擋不住維護者本人作惡**；它擋的是「帳號被盜到被發現」之間的那個窗口。
- **自動更新會帶來 PR**：每週最多三個群組（npm、Go、Python 的 minor＋patch），加上各自的 major；每月最多三個（Actions、映像、compose）。

## 待決策

- 無。
