# ADR-077：依賴漏洞只擋有修補版的——push 擋會出貨的程式，每週掃全部

- 狀態：**Accepted**（2026-09-12，負責人指示：「優先處理和思考npm audit」，看完兩層方案後：「執行待辦」）
- 日期：2026-09-12
- 相關：[ADR-019](./ADR-019-monorepo-structure-and-cicd.md)（CI/CD 基線）、[ADR-022](./ADR-022-sandbox-deployment-topology-and-security-thresholds.md)（Runtime Image 的 grype 門檻 I-04／I-06，本 ADR 沿用它的 `--only-fixed` 判準）、[ADR-023](./ADR-023-agent-sdk-version-pinning-and-behaviour-revalidation.md)（Runtime Image 的 npm 樹由 grype 管）、[ADR-030](./ADR-030-portable-developer-automation-and-contract-code-generation.md)（產碼工具的版本；本批把 datamodel-code-generator 從 0.35.0 升到 0.64.0）

## 背景

- 2026-09-12 手動跑 `npm audit` 才看到 `apps/web` 的 vitest GHSA-82fw-gwwq-j7x9：moderate、dev 依賴，而且我們的用法（jsdom 跑 `vitest run`，沒有 browser mode、沒有 `mockerPlugin`）碰不到它。**CI 對應用程式的依賴沒有任何漏洞掃描**，npm、Go、Python 三邊都沒有；唯一有門檻的是 Runtime Image（`runtime-image.yml` 的 grype `--only-fixed --fail-on high`）。
- 同日第一次三邊全掃：
  - npm 四個專案：vitest 升到 4.1.11 之後都是 0。
  - Go：`apps/platform` 程式呼叫到的 0；模組層 4 筆，全在 `golang.org/x/crypto` v0.54.0（3 筆由 v0.55.0／v0.56.0 修補、1 筆沒有修補版）。`apps/sandbox` 程式呼叫到 2 筆、模組層另 3 筆，全在 `github.com/docker/docker` v28.5.2+incompatible，**govulncheck 對 5 筆都報沒有修補版**；其中 GO-2026-4887 是 daemon 端 AuthZ plugin 的繞過，sandbox 只用 client。`tools/devctl` 0。
  - Python：`apps/llm` 0；`tools/codegen/python` 的 datamodel-code-generator 0.35.0 有 8 筆（程式碼注入、SSRF、跨來源轉址洩漏標頭），攻擊面都是「不受信任的 schema」，而它只讀我們自己的 `contracts/`；0.64.0 全部修補。
- 三個工具都沒有 grype 的 `--only-fixed`：govulncheck、`npm audit`、pip-audit 一有發現就非零結束。照原樣當閘門，sandbox 那 2 筆會讓每一次 push 都紅，而且沒有任何可以做的修正。
- 漏洞公告與 commit 無關，任何一天都可能出現新的；擋在 push 上的門檻每紅一次，都會卡住與它無關的工作。

## 決策 1：只擋有修補版的

- 判準沿用 ADR-022：**有沒有可以升級的版本**。npm 的 `fixAvailable` 不是 `false`、govulncheck 的 `fixed_version` 非空、pip-audit 的 `fix_versions` 非空。沒有修補版的印成 `NOTE`、不擋——擋了也沒有人能做什麼，只會讓人習慣紅燈。
- 過濾寫在 `devctl dep-audit`（讀三個工具的 JSON），不寫成 workflow 裡的 shell：它有單元測試與突變證明，本機與 CI 跑同一條命令（`task deps:audit`）。輸出若不是一份真的報告（npm 的錯誤物件、沒有 `config` 訊息的 govulncheck、沒有 `dependencies` 的 pip-audit）一律當錯誤，不當成「沒有漏洞」。

## 決策 2：push／PR 只擋會出貨的程式

| 專案 | 掃什麼 | 門檻 |
| --- | --- | --- |
| `apps/web` | `npm audit --omit=dev` | high 以上 |
| `apps/platform`、`apps/sandbox` | govulncheck 預設的 symbol 掃描 | 程式真的呼叫得到的 |
| `apps/llm` | `uv export --no-dev` 交給 pip-audit | 全部（PyPI 的公告沒有嚴重度欄位） |

- 新 job `dependency-audit` 不設 path filter，每次都跑；`images-push` 等它綠了才推。
- Runtime Image 的 npm 樹不在這裡：`runtime-image.yml` 的 grype 已經對整個映像擋門（ADR-023）。
- **為什麼不是「任何警告都擋」**：這次的 vitest 就是反例——moderate、dev、碰不到。擋下它，等於要每個人先處理一件與自己的 commit 無關、也不影響使用者的事。

## 決策 3：每週掃全部，紅了就是那一週的報告

- 既有的週日 `schedule` 與 `workflow_dispatch` 改跑 `dep-audit --full`：repo 內每個有 lockfile 的專案（npm 4 個、Go 3 個、Python 2 個），連 dev 依賴一起掃；npm 取 moderate 以上，Go 連程式沒呼叫到的模組層也算。仍然只擋有修補版的。
- 它不在 push 上跑，紅了不擋任何人；GitHub 會對失敗的排程 run 發通知，那就是報告。
- 名冊由測試守住：`TestEveryLockedProjectIsAudited` 用 `git ls-files` 找出每個有 `package-lock.json`、`uv.lock` 或含 Go 檔的 `go.mod` 的目錄，新專案沒登記就紅；登記了卻沒有 lockfile 也紅。`tools/codegen/go` 只有 `go.mod`（釘住產碼映像裡的 ogen 版本），沒有程式可掃；產出的程式碼用到的依賴在 `apps/platform/go.mod`，在那裡掃到。

## 決策 4：本批把可修的全部修掉，閘門第一天就是綠的

- `apps/web`：vitest 4.1.10 → 4.1.11（commit `e580c4f`）。
- `apps/platform`：`golang.org/x/crypto` v0.54.0 → v0.56.0，連帶 `golang.org/x/text` v0.40.0 → v0.41.0；`go mod tidy` 同時移除一個不再被使用的 indirect 依賴。
- `tools/codegen/python`：datamodel-code-generator 0.35.0 → 0.64.0（修掉 8 筆的最低版本；最新的 0.79.0 不需要）。重新產生的 `models.py` 多數是寫法（`List` → `list`、`Optional[X]` → `X | None`），**一處是語意**：`format: byte` 的欄位從 `str` 變成 `Base64Str`，驗證時會做 base64 解碼。這份 stub 只被 `apps/llm` 的契約測試使用，247 條通過。
- 仍列為 `NOTE` 的六筆（sandbox 的 5 筆 docker/docker、platform 的 1 筆 x/crypto）都沒有修補版。

## 版本

`tools/toolchain.yaml` 新增 `govulncheck: "1.8.0"` 與 `pip_audit: "2.10.1"`。devctl 以 `go run golang.org/x/vuln/cmd/govulncheck@v…` 與 `uvx pip-audit==…` 執行，不需要預先安裝，所以 doctor 不檢查它們。

## 成本與限制

- 每次 push 多一個並行 job。本機量測：閘門 19 秒、全掃 22 秒；CI 另要下載工具與模組。
- 公告資料庫是外部服務（npm registry、Go vulndb、PyPI）：它們掛了這個 job 會紅，但那一次的紅不代表有漏洞——看 `devctl:` 開頭的錯誤訊息分辨。
- govulncheck 的「呼叫得到」是靜態分析；PyPI 的公告沒有嚴重度，所以 Python 的閘門比 npm 嚴。
- **`github.com/docker/docker` 舊路徑對閘門等於永遠放行**：govulncheck 對它的公告都報沒有修補版（Moby 已把 client 移到新的模組路徑），以後這個模組的公告也只會是 `NOTE`。要讓它重新受閘門管，就得遷移到新的 client 模組，那是一次 API 遷移，不是升版。

## 待決策

- sandbox 從 `github.com/docker/docker` 遷移到 Moby 新的 client 模組：目前 5 筆不是 daemon 端就是沒被呼叫，沒有排入；每週報告會持續列出它們。
