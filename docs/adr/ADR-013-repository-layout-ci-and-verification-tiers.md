# ADR-013：Repo 結構、CI 與驗證層

- 狀態：Accepted
- 相關：[ADR-002](./ADR-002-data-ownership-and-core-infrastructure.md)（資料所有權與核心基礎設施）、[ADR-004](./ADR-004-sandbox-isolation-and-execution-security.md)（Sandbox 隔離與執行安全）、[ADR-014](./ADR-014-developer-automation-and-dependency-governance.md)（開發自動化與依賴治理）

## 背景

系統橫跨三種語言（TS 前端、Go 控制平面、Python 能力提供者）與一個資料庫，且跨語言介面必須先寫 OpenAPI schema 再實作、CI 需檢查 codegen drift。多語言、多產物的專案需要一個共同的頂層目錄語意、一個共同的建置入口，以及一條 CI 管線，讓任何人或 Agent 不必猜測「新東西該放哪裡」「該跑哪些檢查」。此外，前端測試長期只跑在 jsdom 裡，而部分判定（實際像素合成、真實版面溢位、真實 Tab 順序）在 jsdom 環境下結構上做不出來，需要一層額外的驗證。

## 決策

### 決策 1：單一 Monorepo

系統採單一 monorepo，不依語言拆成多個 repo。

理由：OpenAPI spec 與三語言的 codegen 產物落在同一個 commit，drift 檢查退化成「重跑 codegen 後 `git diff` 必須乾淨」，不需要跨 repo 版本協調；跨語言的原子性變更（改 spec、改 Go handler、改前端呼叫）是一個 PR；只需維護一套 CI 設定。拆 repo 換來的「獨立發版」在服務尚未因負載、組織或安全需求被迫拆分之前不值得預付成本，而拆 repo 是比拆服務更難回頭的動作。

### 決策 2：頂層目錄依產物角色劃分

頂層目錄依「產物在系統中的角色」判斷，不依語言、團隊或是否含程式碼判斷：

| 目錄 | 判準 | 現有內容 |
| --- | --- | --- |
| `apps/` | 可獨立啟動、建置或部署的產品程式；同一部署單元可有多個 `cmd/` 入口 | `web`、`platform`、`llm`、`sandbox` |
| `packages/` | 被其他程式 import／link 的可重用 library；主要是提交版 generated client／transport model | `api-client-ts`、`api-stub-py` |
| `contracts/` | 跨程序或跨語言介面的唯一來源，不是 generated output | OpenAPI、event schema、packaging schema |
| `db/` | 持久化結構與查詢的來源 | migration、sqlc query/config、資料庫測試 |
| `infra/` | 部署、runtime image、網路、節點與 observability 設定 | Compose、image、egress、alerts |
| `tools/` | 開發、CI、資料維護與維運命令，以及只能隨該命令演進的緊密 fixture | `devctl`、codegen、QA corpus、golden set |
| `docs/` | 敘事性文件與歷史紀錄，不被產品程式 import 或執行 | 計畫、ADR、手冊、spike 墓碑 |

`.github/`、`.devcontainer/` 與根層的 `Taskfile.yml`、語言版本檔、環境範本是 repository 控制面與入口，保留在工具預期的位置，不另包一層。判斷順序：可啟動產品程式進 `apps/`；只供 import 的 library 進 `packages/`；介面或資料庫來源分別進 `contracts/`、`db/`；部署與 runtime 設定進 `infra/`；操作者主動執行的 repo 命令與其不可分 fixture 進 `tools/`；純敘事進 `docs/`。「含程式碼」本身不是放進 `apps/` 的充分條件——四個可獨立部署的產品程式（`web`、`platform`、`llm`、`sandbox`）在同一個頂層，CI path filter、Task 與 module path 直接反映部署單元；不存在按語言或按「是否含程式碼」分出的第二層頂層目錄。`apps/sandbox` 用自己的 `go.mod`，不是 `apps/platform` 底下的另一個 `cmd/`；搭配 Go 的 `internal/` 語意，執行平面在編譯期就無法 import 控制平面的資料存取套件，不需要額外的 lint 設定即可擋住鐵律 2 的違反。

理由：先前把「會跑的」與「給人讀的」分成兩類（軟體目錄一組、`docs/` 一組）解決了文件與建置產物混雜的問題，但同時具有啟動入口與獨立部署生命週期的四個產品程式仍分散在兩個不同的頂層分組，新增元件時只能靠既有案例猜測歸屬。改依「角色」判斷後，「含程式碼」與「是否可獨立部署」被分開回答，四個產品程式因此收斂到同一個角色類別。

### 決策 3：建置入口是 Taskfile 加各語言原生工具

共同入口用 **Taskfile**（跨平台單一執行檔，不需要額外安裝 GNU make），職責只有「把常用指令記在一個地方」：`dev`（起本機依賴）、`test`、`lint`、`gen`（codegen 與 sqlc）、`build`，每個都只包一行原生指令。各語言維持原生工具鏈：Go 用 `go build`／`go test`／`golangci-lint`；TS 用 Vite／`tsc`／lint／格式化工具／測試框架；Python 用 `uv`／`ruff`／`pytest`；資料庫用 migration 工具加 `sqlc generate`；契約採 OpenAPI-first：spec 的來源是 `contracts/openapi/public.yaml`，Go 是下游，codegen 由它產生 TS client、Python stub 與 Go model，產物一律入 repo，讓「重跑 codegen 後 `git diff` 乾淨」成為不需要額外基礎設施的 drift 檢查，也讓程式碼審查看得到實際介面。

Taskfile 不得演變成依賴圖或建置快取層。重評條件（滿足任一即重開建置工具評估，考慮 Bazel／Nx 一類方案）：CI 單次 PR 的平均等待時間超過十分鐘；CI 分鐘數成為實質成本；或參與開發者達三人以上且開始出現「不知道該跑哪些測試」的實際事故。在重評條件成立前，單人／小團隊的建置時間不是瓶頸，買不到對應方案的價值。

### 決策 4：CI 管線與其邊界

CI（GitHub Actions）在對主線的 PR 與推送到主線時觸發，PR 上所有 job 必須綠燈：

| Job | Path filter | 內容 |
| --- | --- | --- |
| `web` | `apps/web/**`、`packages/api-client-ts/**` | install → lint → typecheck → test → build |
| `platform` | `apps/platform/**`、`db/**`、`contracts/**` | lint → `go test ./...` → `go build ./...` |
| `llm` | `apps/llm/**`、`packages/api-stub-py/**`、`contracts/**` | 依賴同步 → lint → 測試 |
| `contracts-drift` | 無 filter，每次都跑 | 重跑 codegen 與 `sqlc generate`，`git diff --exit-code`；有差異即失敗 |
| `images` | 同上三者 | 建置 `apps/web`、`apps/platform`、`apps/llm` 三個產品程式的容器 Image，並在推送前先跑一次 stack smoke test |
| `images-push` | 依賴 `images` 與其餘 required job 全綠 | 只在推送到主線時執行，把 `images` 建好的 Image 以 commit SHA 為 tag 推送至 registry |

`contracts-drift` 刻意不做 path filter：drift 也可能來自有人直接手改產物，那種變更不會觸碰 `contracts/`；這個 job 成本低，全跑最省事。`images` 在 PR 上照樣建置與 smoke test，讓 PR 就能發現映像建置壞掉；只有推送 registry 的 `images-push` 留到推送主線才跑。Image tag 用 commit SHA 而非浮動標籤，讓部署與回滾都指向明確版本。只改 `docs/**` 的 PR 只會觸發 `contracts-drift`。`apps/sandbox` 的 Runtime Image 不在這個 job 裡：它走 [ADR-004](./ADR-004-sandbox-isolation-and-execution-security.md) 的獨立映像掃描與發佈管線，發佈節奏與這裡的三個控制平面服務不同。Path filter 是手寫的，新增頂層目錄時必須同步更新，否則會有「該跑卻沒跑」的靜默漏洞；這是選擇原生工具而非跨語言依賴圖工具已知的代價。

**main 分支保護**：禁止直接 push；合併需通過上述所有 required checks；要求 linear history（squash merge）。

Container registry 採 **GHCR**：repo 與 CI 已在 GitHub 生態內，推送不需要管理額外憑證；OCI referrer／`cosign attach` 讓 SBOM 與掃描 attestation 能隨 image digest 一併保存，供 [ADR-004](./ADR-004-sandbox-isolation-and-execution-security.md) 的映像供應鏈檢查使用；digest 釘選與 commit SHA tag 的既有做法天然相容。

CI runner 使用 **GitHub-hosted runner**，不需要自架。gVisor 的預設平台 `systrap` 以 seccomp 與訊號攔截系統呼叫，不需要巢狀虛擬化（只有選用 `--platform=kvm` 才需要），這使得「多數 Sandbox 隔離測項可在一般 Linux runner 上執行、只有節點與網路面的測項需要真實節點」成為可行的分割方式；但這個分割必須先在真實節點上實測確認（`runsc --platform=systrap` 起一個容器並跑完一次 Run）才能視為定案，在那之前是提案而非事實。細節與驗收程序見 [ADR-004](./ADR-004-sandbox-isolation-and-execution-security.md)。

### 決策 5：前端真實瀏覽器驗證層是一層刻意窄的補充

jsdom 環境結構上判定不了三件事：合成後的實際像素（對比度計算需要真正的版面合成）、真實版面（欄位是否溢位、是否被擠爛）、真實的 Tab 鍵行為（jsdom 不實作瀏覽器的 Tab 焦點序列）。為此引入 Playwright，跑 chromium／firefox／webkit 三個引擎，作為 jsdom 層之外的補充，不是它的替代：

- **範圍刻意窄**：只測上述三件 jsdom 判定不了的事，不重跑 jsdom 層已覆蓋的路由與規則——那會用三個引擎的成本換同一個答案的第二份副本。
- **不需要任何後端**：所有 API 呼叫以請求攔截模擬；攔截規則依資源類型放行文件、樣式、腳本與字型，其餘一律視為需要模擬的資料請求。
- **驅動 production build，走同源**：以正式建置產物起服務並讓前端呼叫同源路徑，這既符合生產環境形態（SPA 與 API 同源），也讓跨來源資源共享設定不進入這一層——帶憑證的請求無法配合萬用來源設定，開發模式的跨來源設定會讓每個模擬回應在斷言執行前就被瀏覽器擋下。
- **反空轉檢查**：對比度斷言先確認規則本身已離開「未判定」狀態、真的做出了判定，才斷言零違規；否則「零違規」證明不了任何事——一個從不判定的環境本來就會回報零違規。
- **跨引擎差異如實記錄，不用單一引擎的行為當標準**：例如某引擎預設不將連結排入 Tab 序列，斷言因此檢查「焦點在文件中不會回跳」這類性質，而不是比對一份寫死的、只反映另一引擎行為的順序清單。
- **CI 獨立成一個 job，與既有前端測試 job 並列，不合併**，讓兩層各自回報、快的一層不被拖慢；引擎下載不加快取，維持每個外部 action 都經釘選版本驗證的既有紀律優先於節省少量時間。
- **不併入預設測試指令**：真實瀏覽器測試以獨立指令執行，預設的測試指令維持只跑 jsdom 層——jsdom 層不需要下載瀏覽器引擎、能快速執行，不應該讓每一次執行都承擔引擎下載的成本。

CI 的作業系統矩陣現況為 Linux 三引擎加 Windows 單引擎（chromium），不含 macOS。理由：對這個應用而言，會改變版面判定結果的作業系統變因只有兩個——系統字型的換行寬度、以及捲軸是否佔用版面空間——而 Linux 與 Windows 這兩格已分別落在「不佔版面空間」與「佔版面空間」兩端；macOS 在捲軸行為上與 Linux 同端，只多帶來第三種字型度量，不構成前兩格未覆蓋的新失敗模式。加入 macOS 也不會取得未經模擬的真實瀏覽器引擎——所有引擎皆為工具自行編譯的版本，這個限制與作業系統選擇無關。重評條件：出現第三種會改變版面判定的作業系統變因，或應用開始出現只在 macOS 才有的行為。

## 影響

### 正面

- 任何新元件都能以單一角色判準定位，不需要在「含程式碼」與「可獨立部署」之間猜測。
- 跨語言契約的一致性檢查是一個 `git diff`，不需要額外的基礎設施或多 repo 的版本協調。
- CI 依部署單元切分 job，只有實際變更的部分需要等待；容器映像只在推送到主線時建置，PR 不需要為此付出時間。
- 前端測試補上了 jsdom 結構上做不到的判定，且該判定被證明會在真的壞掉時失敗，而不是一次性的人工快照。

### 成本與限制

- Path filter 為手寫維護，新增頂層目錄或搬移產物位置時，若未同步更新，會出現「該跑卻沒跑」的靜默漏洞。
- codegen 產物入 repo 會讓部分 PR 的 diff 變大，但換得的是不需要額外基礎設施的 drift 檢查。
- 單一 repo 隨產物增加，全量 checkout 與編輯器索引成本會上升；在出現實際痛點前不特別處理。
- 真實瀏覽器驗證層引入的引擎並非使用者桌面上實際安裝的瀏覽器，不覆蓋各瀏覽器自身的 UI 層與擴充套件，也不涵蓋行動裝置真機；沒有快取的引擎下載讓相關 CI job 多出固定時間成本；多一套需要維護與升級的工具鏈。

## 待決策

- PR 是否強制至少一個 approval，或允許在保留 required checks 的前提下自行合併，尚未決定。
- 真實瀏覽器驗證層要不要加瀏覽器引擎快取，尚未決定；前提是先選定並驗證一個可釘選的 `actions/cache` 版本。
- 真實瀏覽器驗證層要不要涵蓋行動裝置（device emulation 或真機服務），目前只有一格桌面視窗寬度，尚未決定。
- 真實瀏覽器驗證層引擎版本的升級節奏與觸發條件，尚未決定。
