# Runbook：從零 Provision 整個系統

這份手冊處理「沒有任何既存部署可依賴」與「既存設定遺失，需要安全重建」兩種情況。目標不是最快讓一個程序聽起來像在跑，而是讓 Web、控制平面、Worker、能力提供者、模型閘道、Sandbox、資料庫與物件儲存用同一組已知事實重新接起來。

它是整體順序與驗收表；每個節點的主機指令、網路規則與復原細節仍以各節點 runbook 為準。任何密鑰只放在部署主機的秘密檔或忽略的 `.env`，不寫入本檔、commit、終端輸出或 issue。

## 1. 先選 Provision 類型

| 類型 | 適用情況 | 不包含 |
| --- | --- | --- |
| 淨測試模式 | 示範、無網路或沒有 Docker 的開發機 | 真實 PostgreSQL、物件儲存、隔離 Sandbox、模型呼叫 |
| 本機完整開發 | 開發與整合驗證 | 對外 TLS、強隔離節點、正式秘密管理 |
| 正式部署 | 讓不受信任的 Skill 接受試跑或對外服務 | 開發登入、弱隔離、主機 Docker 的便利設定 |

不要把三種模式混在同一份設定中。尤其不要把 `DEV_LOGIN=1`、`COOKIE_INSECURE=1` 或本機 Sandbox 設定帶進正式部署；API 與 Worker 會拒絕部分危險組合，但拒絕不是 Provision 策略。

## 2. 所有模式共有的起點

1. 取得要部署的 commit，確認工作樹沒有未知變更。正式部署只能使用已通過驗證、已產出映像的 commit。
2. 執行 `task doctor`；沒有 Task 時執行 `go -C tools/devctl run . doctor`。依它指出的實際版本修正 Go、Node、uv、Docker 與 Compose，不要從本文猜版本。
3. 執行 `task env:init` 與 `task bootstrap`。前者只在 `.env` 不存在時由 `.env.example` 建立檔案；後者取得各語言依賴。
4. 執行 `task gen:check`。跨程序契約、SQL query 與 generated output 必須在服務啟動前對齊。

驗：`task doctor` 與 `task gen:check` 成功；`.env` 仍未加入 Git；任何讀取環境的診斷都只報變數名稱、不回顯值。

## 3. 淨測試模式：最低成本的產品驗證

這條路不需要 Docker 或模型密鑰，適合先確認 Web 到 API 的基本旅程：

```bash
task bootstrap
npm ci --prefix tools/pglite
npm --prefix apps/web run build
task clean-mode
```

它以嵌入式資料庫、記憶體物件儲存與本機程序 Driver 取代正式基礎設施。它不能執行不受信任 Skill，也不能作為 Sandbox、網路隔離、物件 URL 或併發行為的證據。

驗：啟動器印出 URL，開啟後能讀到健康檢查與示範介面。若要載入可搜尋的示範內容，還需要本機模型閘道和 `apps/llm`；這會進入下一節的成本邊界。

## 4. 本機完整開發：由下往上建立

### 4.1 基礎資料層

```bash
task dev
```

這會啟動 PostgreSQL 與 SeaweedFS，沒有模型費用。以 `.env.example` 的 local-only placeholder 作為開發起點；真實環境的資料庫與物件儲存值必須改放進忽略的 `.env` 或部署秘密檔。

驗：兩個容器健康、API 所用的 `DATABASE_URL` 與 `OBJSTORE_*` 指向同一組本機基礎設施。

### 4.2 模型能力（可選且可能付費）

```bash
task dev:model
task dev:llm
```

`task dev:model` 先檢查必要密鑰再啟動 LiteLLM；任何後續送到模型供應商的請求都可能產生費用。`task dev:llm` 會替 Python 服務取得受預算限制的 Virtual Key，Python 服務不得持有閘道的 master key。

驗：LiteLLM 健康端點有回應，`apps/llm` 使用 Virtual Key；沒有這一層時，搜尋會退回 FTS，評估判定會是 `undetermined`，但核心 API 不應假裝模型能力可用。

### 4.2.1 模型能力的可用條件

模型 profile 的密鑰檢查只回答 Gateway 能否啟動，不能證明 Go 或 Python 已能使用模型。要讓本機能力可用，`.env` 還必須把下列三個層次接起來：

| 層次 | 必要設定 | 驗證方式 |
| --- | --- | --- |
| Gateway 位址與管理 | `LITELLM_BASE_URL`、`SKILLHUB_MODEL_GATEWAY_URL`、`SKILLHUB_MODEL_GATEWAY_ADMIN_URL`、`SKILLHUB_MODEL_GATEWAY_KEY` | Gateway 可簽發受限 Virtual Key；不要把 master key 傳給 Python 或 Sandbox |
| Python 服務 | `LLM_SERVICE_TOKEN` | 帶同一個 Bearer token 呼叫 `/readyz` 得到 `200` 且 `gateway_configured=true` |
| 控制平面 | `LLM_SERVICE_URL`、相同的 `LLM_SERVICE_TOKEN` | API／Worker 以 `/readyz` 驗證服務，而不是只看行程或容器仍在執行 |

`task dev:llm` 需要簽發 Virtual Key 時會使用新的 alias；Gateway 的 key alias 不能重複。啟動器會保留可辨識的前綴並加上唯一尾碼，避免前一次行程留下的 alias 使下一次 Provision 在簽發階段失敗。

### 4.2.2 分層驗收與成本邊界

先完成不花費模型費用的契約與本機服務檢查，再選擇需要的實際驗收層。`task dev:model` 只使 Gateway 可用，不等於已付費；只有送到模型的測試才會花費費用。

| 目的 | 驗收層 | 必要前提 |
| --- | --- | --- |
| 確認每個已設定模型 tier 接受服務採樣參數 | `apps/llm/tests/test_gateway_live.py` | 明確設定 `SKILLHUB_LIVE_GATEWAY=1`；以短效 Virtual Key 執行 |
| 確認 Run 的產物、Gateway 計費 trace 與 key cleanup | `TestEndToEndRunCallsTheModelThroughItsOwnVirtualKey` | PostgreSQL、物件儲存、Sandbox、Gateway 與可由 Sandbox 存取的 trace 位址 |
| 確認生成結果寫入 Gateway 實際成本 | `TestARealGatewayGenerationRecordsWhatItActuallyCost` | 正在執行且 `/readyz` 成功的 `apps/llm`，以及測試資料庫 |
| 基準量測 | GEN-009、模式批次、創作量測 | 使用其受版本控制的 corpus／圖檔與獨立輸出目錄；它們是多次付費工作，不能以單次 E2E 取代或自動宣稱完成 |

驗收結束時，確認 Run 的 `cleanup_status` 為 `cleaned`、Gateway 回報的成本來源為 `gateway`，並停止這次才啟動的 Python 行程。暫存輸出可以刪除；不要將 Virtual Key、master key 或服務 token 寫入輸出檔、文件或 shell history。

### 4.3 啟動應用程式

在不同終端啟動下列五個程序：

```bash
go -C apps/platform run ./cmd/api
go -C apps/platform run ./cmd/worker
(cd apps/llm && uv run uvicorn skillhub_llm.app:app)
go -C apps/sandbox run ./cmd/sandboxd
npm --prefix apps/web run dev
```

本機 SPA 對 API 是跨來源，API 必須顯式設定 `DEV_CORS_ORIGIN=http://localhost:5173`。Worker 不是可省略的背景便利程序：Run 派送、清理與分割表預建都依賴它。

本機要跑真實 Run 時，API 與 Worker 必須持有完全相同的 Run 相關設定：

| 設定群組 | 必須一致的原因 |
| --- | --- |
| `DATABASE_URL`、`OBJSTORE_*` | API 建立快照，Worker 讀取、寫回與收集產物 |
| `SKILLHUB_SANDBOX_PROVIDERS`、對應的 `SKILLHUB_SANDBOX_TOKEN_<NAME>` | API 預檢與 Worker 派送必須指向同一個 Provider |
| `SKILLHUB_MODEL_GATEWAY_URL`、`SKILLHUB_MODEL_GATEWAY_ADMIN_URL`、`SKILLHUB_MODEL_GATEWAY_KEY`、`SKILLHUB_RUN_MODEL` | Worker 簽發、Sandbox 使用、完成後撤銷 Virtual Key |
| `SKILLHUB_TRACE_INGEST_URL`、`SKILLHUB_TRACE_INGEST_SECRET` | Sandbox 回推 Trace，平台才有完整執行證據 |

驗：Web 可以登入、建立 Skill 與 Test Case；從 Run 頁開始執行後，Run 到終態，產物可讀，`cleanup_status` 最終為 `cleaned`。只看到 API 回 201 不算完成；必須讀回終態與清理狀態。

### 4.4 本機常見的跨網路失敗

Sandbox 是另一個網路觀點。它需要同時能存取物件儲存的預簽 URL、模型閘道與 Trace ingestion URL；主機上的 `localhost` 或 `127.0.0.1` 對 Sandbox 通常不是同一台主機。先以 Sandbox 內的連線驗證每個 URL，再開始排查程式。

模型閘道的主機名稱、Sandbox egress allowlist 與其 pinned IP 也必須相符。只改其中一處會造成「Sandbox 已建立、Run 最後失敗」的延遲錯誤。正式節點的正確做法在[沙箱節點](sandbox-node.md) §1 與 §5；不要把本機的寬鬆網路設定複製過去。

同一個差異也適用於容器化的驗收 runner：它與主機上的 `apps/llm` 不是同一個 loopback。若 runner 要呼叫主機暫時啟動的能力服務，服務須監聽主機可路由的位址，runner 則以該 Docker 環境提供的主機名稱（例如 `host.docker.internal`）連線；兩端仍以服務 token 驗證。先從 runner 網路命名空間呼叫 `/readyz`，成功後才啟動會付費的測試。這只是本機驗收的接線方式，不能取代正式部署中服務對服務的私有網路與 egress 規則。

## 5. 正式部署：固定的依賴順序

正式部署由三種主機組成，順序不可顛倒：

```text
控制平面資料庫與物件儲存
        ↓
模型閘道（使用控制平面的 litellm 資料庫）
        ↓
沙箱節點（egress allowlist 指向模型閘道）
        ↓
控制平面服務設定 Sandbox 與 Gateway
        ↓
Web 對外提供同源介面
```

1. 依[控制平面節點](control-plane.md) §1 建置控制平面，放入秘密、套 migration、啟動服務與 timer。
2. 在控制平面建立 LiteLLM 專用資料庫，再依[模型閘道節點](gateway.md)建置閘道。先驗證私有網路上的健康檢查與 4000 埠來源限制。
3. 先在 `infra/egress/allowlist.yaml` 釘住閘道私有 IP，重新產生並驗證 egress 規則；再依[沙箱節點](sandbox-node.md)建立、准入並啟動每個節點。
4. 將 Sandbox provider、token、Gateway 位址與 Trace ingestion 設回控制平面 `platform.env`，依控制平面換版程序套用。Gateway 位址與 egress allowlist 的 pinned IP 必須相同。
5. 依控制平面 runbook 做對外健康檢查、備份、還原演練與告警送達測試，才把 DNS 導向新系統。

驗：每個 compose service 都在執行、公開 `/healthz` 回 200、Gateway probe 成功、Sandbox 節點是 `serving`、一個真實 Run 完成且可在控制平面讀到 egress 記錄。資料庫備份與還原演練也必須成功；沒有可還原的備份不算 Provision 完成。

## 6. 設定遺失時的重建程序

1. 停止把猜測的環境變數塞進正在執行的程序。先確認最後已知良好的 commit、資料庫版本、物件 bucket 與各節點私有位址。
2. 以 `.env.example`、服務的 capability 輸出與各 runbook 的秘密表重建**變數名稱清單**，再從秘密管理系統或受控人工交接填值。不要從 `docker inspect`、shell history 或 log 複製密鑰。
3. 先對每個獨立元件做健康檢查：資料庫／物件儲存、Gateway、Sandbox capability、API，再啟動 Worker 與 Web。
4. 用新建的非正式測試 Workspace 跑一個最小 Run，讀回終態、Trace、產物、成本事件與 `cleanup_status`。任何一步失敗都只修那個邊界，不以跳過 Worker、關閉 egress 或改成長效 master key 來讓它看似成功。
5. 完成後立即把已知的「設定群組與來源」更新到受控部署文件；密鑰本身仍留在秘密管理系統。

## 7. 完成定義

Provision 完成不是容器都在 `Up`。至少要同時證明：

- API 與 Web 可以健康回應，且登入與 Workspace scope 正常；
- Worker 實際接走 Run，而不是讓它停在 `queued`；
- Sandbox 以期望的隔離強度執行，並只可連到允許的出口；
- 模型呼叫使用短效 Virtual Key，而非 master key；
- Run 的終態、Trace、產物、成本與清理狀態都能由控制平面讀回；
- 備份、還原演練與告警送達已被驗證。

任一項缺席時，系統是部分啟動，不是可交付的整體系統。
