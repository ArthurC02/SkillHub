# services/sandbox — Sandbox Provider

執行平面的 Provider 服務：實作 [`contracts/openapi/sandbox-provider.yaml`](../../contracts/openapi/sandbox-provider.yaml)（37f1918 凍結），為每個 Run attempt 建立一個隔離沙箱，並把生命週期回報給 Go 控制平面。

> ⛔ **上線硬性關卡（ADR-015 定案紀錄）**：**SEC-009 逃逸測試與 SBX-010 隔離測試未通過前，不得開放外部使用者提交 Skill 執行。** 本目錄實作的是 ADR-005 基線與 gVisor 的**配置開關**，不代表隔離強度已被驗證——那需要在部署平台上實跑 `runsc` 與逃逸測試，屬部署期驗收。

## 邊界（為什麼是獨立 module）

- **沒有任何 Postgres 連線，也不會有**（鐵律 2）。`go.mod` 是獨立的，編譯期就 import 不到 `services/platform` 的 `internal/`。
- **不受信任的程式碼只在沙箱容器內執行**（鐵律 1）。本服務程序不解壓套件、不讀 Dataset 內容、不執行 Skill Script；它傳遞路徑與短效 URL，由沙箱內的 Runtime 自己去取。
- **不主動回呼平台**。控制平面是唯一的 client，以輪詢互動；Trace 走 Outbox 事件通道，不走這支 API。
- **Secrets 不進 Log**（鐵律 11）。Log 只有 `run_id`／`attempt`／`provider_run_id`／狀態；Virtual Key 與 pre-signed URL 從不寫入 Log，且在工作負載輸出寫進 `RunResult.agent_output` 前先被遮罩。

## 端點

| 方法與路徑 | 行為 |
| --- | --- |
| `GET /capability` | 目前可承接的 Runtime、資源上限、隔離等級與空 slot 數（RUN-002） |
| `POST /runs` | 派送一次 attempt。冪等鍵＝body 的 `(run_id, attempt)`：首次 201、重送 200 回同一資源且**不開第二個沙箱**、同鍵不同內容 409、能力不符 422（`RunError`）、無 slot 429 |
| `GET /runs/{provider_run_id}` | 輪詢單一 attempt；`result` 只在終態出現 |
| `POST /runs/{provider_run_id}/cancel` | 記錄意圖回 202；**已終態也回 202**，不回 409 |
| `DELETE /runs/{provider_run_id}` | 釋放資源，冪等，**無 404**；不存在也回 204。釋放失敗回 500 讓平台記錄 `cleanup_status` 並重試 |
| `GET /runs?active=true` | 目前仍持有資源的 Run 快照（含 `observed_at`），供遺留 Sandbox 掃描；`active=false` 回 400 |
| `GET /metrics` | Prometheus 文字格式（O11Y-001/003）。**不在契約內**，是本節點的維運面；同樣要 provider token——這張路由表沒有未認證路徑，不從 scrape 端點開先例 |

驗證：所有端點一律要 `Authorization: Bearer <token>`，靜態 token 由環境變數注入，比較採常數時間。此 token 只證明「呼叫方是控制平面」，不帶 Workspace／使用者／Run 範圍（鐵律 3）。

## 設定

| 環境變數 | 預設 | 說明 |
| --- | --- | --- |
| `SKILLHUB_SANDBOX_TOKEN` | 無（**必填**） | Provider token；未設定則拒絕啟動（fail closed） |
| `SKILLHUB_SANDBOX_ADDR` | `:9000` | 監聽位址 |
| `SKILLHUB_SANDBOX_IMAGE` | `skillhub/runtime-agent-sdk:2026.08-1` | Runtime Image。**生產須填 digest**（I-02），tag 是移動標的 |
| `SKILLHUB_SANDBOX_RUNTIME` | 空（＝主機預設 runtime） | 生產填 `runsc`（gVisor，ADR-015） |
| `SKILLHUB_SANDBOX_NETWORK` | `none` | **出口網路**的名稱。`none`／空＝本節點無出口，所有沙箱一律 `--network none`。設了名字，沙箱**仍只在 `RunRequest.egress.allow` 含 `model_gateway` 時**才接上去；dev 填 `skillhub_egress`（`internal: true`，上面只有 LiteLLM 閘道），生產填 Egress Proxy 的網路名 |
| `SKILLHUB_SANDBOX_SLOTS` | `2` | 併發上限；滿了回 429 |
| `SKILLHUB_SANDBOX_UID`／`GID` | `65532` | 工作負載身分，不得為 0 |
| `SKILLHUB_SANDBOX_RUNTIME_VERSION` | `0.3.233` | 宣告在 Capability 的 Agent SDK 版本，須與 Image 內一致 |
| `SKILLHUB_SANDBOX_STORAGE_QUOTA` | 關 | 開啟 `--storage-opt size=`；需要 xfs pquota 或 btrfs，否則 Docker 直接拒絕建立 |
| `SKILLHUB_SANDBOX_DEV_CMD` | 關 | 允許 `provider_extensions.dev_cmd` 覆寫映像 command。**只給開發與隔離測試用**，生產不得開啟 |

## dev 與 prod 的差異

本機是 Windows，`runsc` 需要 Linux（見 [plans/mvp/m2/README.md](../../plans/mvp/m2/README.md)），所以開發用 DockerProvider、生產用 gVisor。兩者**走同一段程式碼**，差的只有下面三格：

| 項目 | dev（本機） | prod（執行節點池） |
| --- | --- | --- |
| 容器 runtime | 主機預設（runc） | `runsc`（`SKILLHUB_SANDBOX_RUNTIME=runsc`） |
| Capability 宣告的隔離等級 | `container` | `gvisor` |
| 網路 | 無出口需求＝`--network none`；有出口需求＝`internal: true` 的 Docker network，上面只有 LiteLLM 閘道 | Egress Proxy 專用網路，default-deny＋允許清單、DNS 固定解析、目的地記錄 |

宣告等級跟著實際設定走：在跑 runc 的機器上宣告 `gvisor` 會讓 RUN-005 依錯誤的前提派工。ADR-005 的其餘基線（非 root、唯讀 rootfs、drop 全部 capability、無管理 Socket、無主機掛載、資源上限）**兩邊完全相同**——gVisor 是多加一層，不替代其中任何一項。

### 每個沙箱的實際隔離參數

| 設定 | 值 | 基線 |
| --- | --- | --- |
| `User` | `65532:65532` | C-02 |
| `SecurityOpt` | `no-new-privileges:true` | C-02 |
| `Privileged` | `false` | C-03 |
| PID／IPC／UTS／Network namespace | 全部 private，不接 host、不接其他容器 | C-04 |
| `Binds`／`Mounts` | **空**（無 docker.sock、無任何主機路徑） | C-05、C-07 |
| `ReadonlyRootfs` | `true` | C-06 |
| `CapDrop` | `ALL` | C-08 |
| 可寫路徑 | tmpfs `/work`（disk 的 3/4）、`/out`（1/4）、`/tmp` 64 MiB `noexec`；皆 `nosuid,nodev`、`mode=0700`、屬 65532 | C-01、C-12 |
| `NanoCPUs` | `vcpu × 1e9` | C-10 |
| `Memory`／`MemorySwap` | 皆＝`memory_bytes`（不給 swap，天花板就是天花板） | C-11 |
| `PidsLimit` | `max_pids` | C-13 |
| `Ulimits` | `nofile` soft＝hard＝`max_open_files`；`core` 0 | C-14、C-16 |
| Wall clock | soft 到 → 送停止訊號、寬限期＝hard−soft；hard 到 → 強制 kill。結果為 `state=failed` ＋ `result.status=timed_out` | C-15、RUN-004 |
| `LogConfig` | `json-file`，16 MB 上限、不輪替 | 避免工作負載灌爆節點磁碟 |
| `AutoRemove` | `false` | 清理是 DELETE 的職責，容器不得自己消失，否則平台無法對帳 |

`/work` 與 `/out` 用 tmpfs 是刻意的：tmpfs 能真的擋住 size，而它的頁面算在 memory cgroup 上，所以想灌滿磁碟的工作負載會先撞到記憶體上限——比磁碟上限更嚴格，不會更鬆。`--storage-opt` 只在檔案系統支援時才是更貼切的作法，因此以開關提供。

### Egress（SBX-007，部分完成）

沙箱接哪個網路是**逐 Run 決定**的：`RunRequest.egress.allow` 含 `model_gateway` 才接上 `SKILLHUB_SANDBOX_NETWORK` 指的出口網路，否則一律 `--network none`。方向只有一個——沒有出口網路的節點只宣告 `egress_modes: ["none"]`，而 `accept()` 會以 422 拒絕帶允許清單的請求；較弱的模式永遠不會頂替較強的請求（契約 `EgressPolicy` 的階序）。

dev 的出口網路是 `skillhub_egress`，`internal: true`：上面的容器沒有對外路由，而網路上只有 LiteLLM 閘道。「允許清單只有一項」因此由**線路本身**強制，不需要 Proxy。**物件儲存刻意不在上面**——dev 的 SeaweedFS 若沙箱直接連得到，預簽 URL 就形同虛設（沙箱能讀整個 bucket）；位元組由本服務代搬（見下）。

**未完成**：生產的 Egress Proxy 本體、域名允許清單、DNS 固定解析與目的地記錄（N-01～N-07）都屬部署期，允許清單管理流程仍是 ADR-015 待決策，所以 SBX-007 不勾。

### 短效授權與位元組搬運（SBX-008）

沙箱**不持有任何預簽 URL**。它拿得到的秘密只有模型閘道的 Virtual Key，也就是它唯一到得了的目的地的憑證。其餘位元組由 sandboxd 代搬：

| 方向 | 作法 |
| --- | --- |
| Skill 套件、Dataset 進沙箱 | sandboxd 以 `object_grants` 的預簽 URL 下載 → `docker exec /bin/tee` 寫進 `/work/.skillhub/skill.zip`、`/work/data/<檔名>` → 最後寫 `/work/.skillhub/ready` |
| Artifact 出沙箱 | `docker exec /bin/tar -cf - -C /out artifacts` 讀出 → 套用 PDM-005 5.2 的單檔／總量上限 → 以寫入授權 `PUT` 上傳單一封存 → manifest 回填 `RunResult.artifacts` |

用 `docker exec` 是被迫的，不是偏好：`docker cp` 對唯讀 rootfs 的容器一律被 daemon 拒絕（實測 `container rootfs is marked read-only`），而且 copy API 看不到掛載點下的檔案；bind mount 會把主機路徑放進沙箱（C-05 禁止）。風險界線同 `ReadTrace`：映像自帶二進位、絕對路徑、無 shell、無呼叫端參數、與工作負載同一個非特權 uid、讀取有上限。內容一律不解析——本服務不解壓套件、不讀 Dataset（鐵律 1）。

**收集交接（為什麼工作負載會等）**：`/out` 是 tmpfs，工作負載的行程一結束，核心就把裡面的東西丟掉；而 `docker exec` 需要**執行中**的容器。因此沒有任何「事後讀取」存在。工作負載做完事後寫 `/out/.workload-done` 並等待，sandboxd 排完 trace、收完 artifact 才寫 `/out/.collected` 放它走。這就是 PDM-005 說的合作式停止窗口。**限制**：崩潰、被殺、逾時的工作負載走不到這一步，只剩 2 秒 ticker 已經推出去的部分。

**Artifact 只有一個封存**：預簽是逐物件的，而平台在工作負載產出前不可能知道檔名，所以一張寫入授權只能授權一個 key。Manifest 逐檔列出名稱、大小與雜湊，位元組在該封存裡。

## Trace 收集（TRACE-002）

沙箱到得了的位址只有模型閘道，**它不能自己送事件**。所以流程是：容器內的 harness 把事件寫成 JSONL（`/out/trace/events.jsonl`，一行一個 JSON）→ sandboxd 讀出來 → sandboxd POST 到 `RunRequest.trace.ingestion_url`。目的地與其憑證都在該 URL 裡，由平台每個 attempt 簽發；沒給 URL 就不收集，也不推送。

讀檔的方式是**在容器內執行 `/bin/cat`**，不是 `docker cp`：`/out` 是 tmpfs，而 Docker 的 copy API 對照容器 rootfs 層解析路徑，對掛載點下的檔案一律回「找不到」（已對 daemon 實測）。bind mount 會把主機路徑放進沙箱（C-05 禁止），走 stdout 會讓 trace 與工作負載自己的輸出混在一起。exec 的風險界線寫在 `dockerdrv.ReadTrace` 的註解：唯讀 rootfs 上的映像檔自帶二進位、絕對路徑、無 shell、無呼叫端參數、與工作負載同一個非特權 uid、讀取有上限。

推送是**邊跑邊送**（2 秒一次）而非結束後一次送，因為 NFR-004 要求事件產生後 3 秒內出現在畫面；容器結束後還有一次收尾推送（含重試），因為那時容器還在（`DELETE` 才移除它），而那批正是失敗 Run 最需要的尾巴。推送失敗不推進水位，下一輪整批重送——平台以 `event_id` 去重，重送是安全的（TRACE-008）。

事件本身一律帶 `masked: false`：遮罩是控制平面的事，沙箱替自己背書正是信任邊界禁止的（鐵律 11）。

## Skill 載入路徑

沙箱內的展開點是 `<workdir>/.claude/skills/<name>/`，不是 `skills/`——這是 PDM-003 Spike §10 的實測結果，原提案的假設已被證偽。`.claude` 是隱藏目錄，展開與清理都必須涵蓋它。SDK 以「一個目錄一個 skill」發現，所以 `<name>/` 那層不能省：套件直接倒進 skills 根目錄會被發現為零個 skill。

啟用條件全部寫在 [`infra/images/runtime-agent-sdk/run.mjs`](../../infra/images/runtime-agent-sdk/run.mjs)，缺一則 Skill **靜默**不載入（不報錯、不警告，只是一個都沒有）：

| 條件 | 值 | 註 |
| --- | --- | --- |
| `cwd` | 指向持有 `.claude/skills/` 的目錄 | 即 `/work` |
| `settingSources` | **省略** | 見下 |
| `skills` | `"all"` | 0.3.x 起 skill 啟用的唯一開關 |
| `allowedTools` | 工作負載可用的工具清單 | 傳 `'Skill'` 已 **deprecated**，改由 `skills` 負責 |

> **`settingSources` 的行為在 SDK 0.3.233 上與 PDM-003 Spike（claude-agent-sdk 0.2.137）相反。** 2026-08-16 實測：`settingSources: ["project"]` 在 0.3.233 **完全發現不到**專案 skill（`init.skills` 只剩內建 plugin），**省略**該選項才發現得到。Spike 當初列出 `["project"]` 是為了排除 `user` 以免映像裡的 `~/.claude` 滲進 Run；`HOME` 指向每個 Run 專屬的 `/work` tmpfs，沒有別的家目錄可滲，所以省略它不損失什麼。
>
> **SDK 版本升級時，以上每一條都必須重新實測，不得用推理帶過**——它們全是靜默失效，而這一條已經反轉過一次（`run.mjs` 檔頭有同樣的警語）。

## 開發與驗證

本機無 Go 工具鏈，一律用容器：

```bash
# build / vet / test（Docker 整合測試會自動 skip）
docker run --rm -v "$PWD:/src" -w /src/services/sandbox golang:1.25 \
  sh -c "go build ./... && go vet ./... && go test ./..."

# 含 Docker 整合測試：把 socket 掛進測試容器
# 這是「測試基礎設施」而非產品路徑——產品程式碼永遠不會把 docker.sock 掛進沙箱
docker run --rm -v "$PWD:/src" -v /var/run/docker.sock:/var/run/docker.sock \
  -w /src/services/sandbox golang:1.25 go test -count=1 ./...

# lint（與 services/platform 同一版本與設定）
docker run --rm -v "$PWD:/src" -w /src/services/sandbox \
  golangci/golangci-lint:v2.12.2 sh -c "golangci-lint fmt --diff && golangci-lint run ./..."

# Runtime Image
docker build -t skillhub/runtime-agent-sdk:2026.08-1 infra/images/runtime-agent-sdk
```

整合測試偵測不到 Docker daemon 就 skip 而非 fail，並且每個測試容器都帶 `skillhub.sandbox.test=1` label、用完即刪。

## 已知簡化（第三批或部署期解掉）

- **Run 狀態存記憶體，重啟靠容器 label 重建**（`Adopt`）。label 只夠回答 `GET /runs`、銷毀沙箱、以雜湊認出重送的派工，**不足以重建 RunRequest**——那裡面有 Secrets，而 label 在節點上是公開可讀的（D-05）。「建立容器」與「記錄 entry」之間崩潰仍會漏一個沙箱，由平台的遺留掃描（RUN-007）收拾。要更強就在執行節點放本地持久化，**不是**去連核心資料庫。
- **Artifact 是單一封存**：一個 attempt 上傳一個 tar，`RunResult.artifacts` 逐檔列出名稱、大小與雜湊但不逐檔給 `object_key`（位元組都在那一個授權 key 裡）。逐檔獨立物件需要前綴授權（S3 POST policy），留給 PACK-001。
- **崩潰的工作負載會掉 trace 尾巴與 artifact**：收集靠合作式交接，而 `/out` 是 tmpfs、`docker exec` 需要執行中的容器，所以被殺或逾時的工作負載只剩已推出去的部分。這是 tmpfs 暫存空間的真實限制，不是交接可以補的。
- **Usage 只有 wall clock**：`RunResult.usage` 的 token 與成本仍為空——它們走 Trace 的 `usage` 事件（TRACE-004，`cost_source: gateway`，已接）。PDM-005 5.2a 的「Go worker 累加 input_tokens 當硬上限」因此尚未接上：需要 provider 側回報或平台讀 trace。
- **`agent_output` 只取工作負載輸出尾端 32 KB**，完整內容屬 Trace。
- **Runtime Image 尚未可發佈**：SBOM（I-03）與漏洞掃描（I-04）是發佈時閘門，尚未接上流水線。
