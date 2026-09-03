# 平台可觀測性（O11Y-001～003）

## 這一層是什麼、不是什麼

ADR-009 把三種能力切開，這裡只涵蓋第一種：

| 平面 | 這裡 | 在哪裡 |
| --- | --- | --- |
| **Platform Observability** | ✅ 服務健康、延遲、錯誤率、Provider 可用性、清理積壓 | 本目錄 ＋ 兩個服務的 `/metrics` |
| Run Trace | ❌ | `contracts/events/trace-event.schema.json`、`apps/platform/internal/trial/evidence` |
| Evaluation | ❌ | `evaluations` 表（M3） |

**告警不得依賴 Run Trace。** Trace payload 大量來自 Sandbox，是使用者可操控的輸入；讓告警讀它等於把觸發權交給不受信任的來源（ADR-009 背景段）。`alerts.yml` 的每一條規則都只讀服務自報的指標。

## 曝露方式

MVP 用 Prometheus 文字格式，各服務自己曝露，沒有 push gateway、沒有 agent：

| 服務 | 端點 | 開關 | 認證 |
| --- | --- | --- | --- |
| `apps/platform`（`cmd/api`、`cmd/worker` 各一份） | `GET /metrics` | `METRICS_ADDR`（例 `:9090`），未設＝不監聽 | 無，**綁內部位址**——這是維運面，不掛在對外的 API port 上 |
| `apps/sandbox`（`sandboxd`） | `GET /metrics` | 隨主服務 port | 與其他路由同一個 provider bearer token（該路由表沒有未認證路徑，這裡不開先例） |

`cmd/api` 與 `cmd/worker` 是兩個行程，各自有一份指標；Prometheus 分別 scrape，`sum()` 之後才是平台整體的數字。`alerts.yml` 的表達式都已經先 `sum` 過。

## 指標清單

實作在 `apps/platform/internal/foundation/observability/metrics/metrics.go` 與 `apps/sandbox/internal/sandbox/metrics.go`，Help 字串是權威說明，這裡只列覆蓋範圍與 NFR-005 的對照。

### O11Y-001 搜尋與 Run 漏斗

| 指標 | 型別 | 標籤 | NFR-005 要求 |
| --- | --- | --- | --- |
| `skillhub_search_duration_seconds` | histogram | `mode`（hybrid／fts） | 搜尋延遲 |
| `skillhub_run_created_total` | counter | — | 建立 |
| `skillhub_run_refused_total` | counter | `reason` | 建立（被拒） |
| `skillhub_run_queue_duration_seconds` | histogram | — | 排隊時間 |
| `skillhub_run_duration_seconds` | histogram | `status` | 建立到結束 |
| `skillhub_run_terminal_total` | counter | `status`、`failure_class` | 成功率、逾時率 |
| `skillhub_run_cleanup_total` | counter | `result` | 清理失敗率 |
| `skillhub_run_cleanup_duration_seconds` | histogram | — | — |

### O11Y-002 Provider 健康度

| 指標 | 型別 | 標籤 |
| --- | --- | --- |
| `skillhub_provider_capability_total` | counter | `provider`、`result`（ok／unhealthy／error） |
| `skillhub_provider_request_total` | counter | `provider`、`operation`、`class`（ok／throttled／client_error／server_error／error） |
| `skillhub_provider_request_duration_seconds` | histogram | `provider`、`operation` |

`class` 是狀態碼分級而非原始碼：429 與 5xx 是重試政策與告警唯一在乎的兩種，一碼一條 series 不會多買到任何東西。

### O11Y-003 遺留 Sandbox 與清理

| 指標 | 型別 | 標籤 |
| --- | --- | --- |
| `skillhub_run_cleanup_backlog` | gauge | — |
| `skillhub_orphan_scan_total` | counter | `provider`、`result` |
| `skillhub_orphan_sandbox_total` | counter | `provider`、`action`（destroyed／failed） |
| `skillhub_orphan_sandbox_persistent` | gauge | `provider` |
| `skillhub_gateway_revoke_failed_total` | counter | — |
| `skillhub_sandbox_destroy_failed_total` | counter | `provider` |
| `skillhub_dispatch_halted` | gauge | `target`（provider 名或 `pool`）、`source`（`p1_incident`／`orphan_threshold`） |

後三個是 **SBX-012**（ADR-022 X-03／X-04 的量測前提）：

- `skillhub_orphan_sandbox_persistent` 是「同一筆連續 ≥2 輪仍在」的當下筆數，事實來源是 Reconciler 的 in-flight orphan 表（`reconciler_orphan_sightings`，migration 0021）。**連續性由該表保證**：某一輪沒看到就刪列，重新出現從第 1 輪起算。累加計數器做不到這件事——`increase(...) > 0` 只說得出「這段時間內動過手」，兩筆不同資源各失敗一次也會滿足。
- `gateway_revoke_failed` 與 `sandbox_destroy_failed` 分開計，因為**正確動作相反**：沙箱殺不掉要 drain 節點，金鑰撤不掉 drain 一點用都沒有（要人到閘道側處理）。合併在 `skillhub_run_cleanup_total{result="failed"}` 裡的告警指不出該做哪一件。

`skillhub_dispatch_halted` 是 **SEC-012** 的開關本身（migration `0030` 的 `dispatch_halts`），**X-04 的暫停與 P1 的停止派送共用它**（`03:SEC-012`「必須共用同一份狀態與同一條解除路徑」）。1 ＝ 該 target 不再被派送新 Run。`source` 是唯一要看的欄位：`orphan_threshold` 會自己解除（連 2 輪門檻以下），`p1_incident` 不會，且**保留現場期間清理與孤兒回收都停手**。

> **已知延遲**：這個 gauge 由 Reconciler 每 5 分鐘從 `dispatch_halts` 重新發佈——撥開關的可能是 API 程序，發佈指標的是 worker。**行為沒有延遲**（Create 與派送都直接讀表），延遲只在這個顯示上，上界一個掃描週期。要即時的答案用 `GET /admin/dispatch`（operator 端點）。

### Trace 管線

| 指標 | 型別 | 標籤 |
| --- | --- | --- |
| `skillhub_trace_events_total` | counter | `source`、`result`（stored／duplicate／rejected） |
| `skillhub_trace_masked_fields_total` | counter | — |
| `skillhub_trace_ingest_lag_seconds` | histogram | — |
| `skillhub_trace_ingest_rejected_total` | counter | `reason` |

### sandboxd

`skillhub_sandbox_dispatched_total`、`skillhub_sandbox_finished_total{status}`、`skillhub_sandbox_active`、`skillhub_sandbox_trace_push_total{result}`、`skillhub_sandbox_trace_events_total`，加上 Go runtime 與 process collector。

## 標籤規約（違反＝指標爆炸或洩漏）

1. **不放使用者資料**：`run_id`、`workspace_id`、`user_id`、Skill 名稱、Prompt 一律不進 label。要定位個別 Run 走 log，以 `run_id` 關聯（NFR-005 第三條）。
2. **不放來自 Sandbox 的值**：`provider` 來自部署設定（`SKILLHUB_SANDBOX_PROVIDERS`），`source` 先過契約的 producer enum 才當 label——不受信任的輸入不得憑空造出 series。

## 告警

`alerts.yml`（Prometheus alerting rules，繁中註解說明每條的判準與處置）。

**掛法（2026-08-22 起不再是一句說明）**：`task dev:observability` 起一個 Prometheus（`infra/compose/docker-compose.yml` 的 `observability` profile），設定在 [`prometheus.yml`](prometheus.yml)，`rule_files` 指向本目錄的 `alerts.yml`，UI 在 `127.0.0.1:9095`。

**在這之前，這 20 條規則從來沒有被任何東西載入過。** 指標寫了、規則寫了、門檻由 ADR-022 定值了，而中間沒有 evaluator——**X-02～X-04 因此沒有觸發路徑，缺的不是門檻是求值**。首次載入即通過（`promtool check rules` 與實際 `/api/v1/rules` 各驗一次，4 群組 20 條），所以這一段補的是路徑不是錯誤。**「4 群組 20 條」是 2026-08-22 那次的數字，不是現況——現為 5 群組 26 條**（2026-09-03 逐列重數 `alerts.yml`：`skillhub-run-funnel`、`skillhub-provider-health`、`skillhub-cleanup-and-leaks`、`skillhub-trace-pipeline`、`skillhub-foundation-signals`）。條數會再長，**以 `alerts.yml` 的列為準，本檔的數字只是當下的快照**。

**scrape 目標是宿主機不是別的容器**：`cmd/api` 與 `cmd/worker` 跑在開發者機器上（compose 裡還沒有它們，見 CORE-001 的 TODO），而兩者讀**同一個** `METRICS_ADDR`——同一台機器上要給兩個值（`:9090` 與 `:9091`），否則後起的那個綁不上。`sandboxd` 故意不列為 target：它的 `/metrics` 要 bearer，沒有 token 檔的 scrape 只會拿到 401，而**一個永遠 down 的 target 讀起來跟服務掛了一模一樣**。

**未做的部分，明說**：沒有 Alertmanager、沒有通知路由、沒有 silence 政策、沒有 Grafana dashboard，**上面那個 Prometheus 是開發機的，不是生產部署**。那些是部署期的事（`04` 甲類），本批交付的是「什麼算異常、依據哪個指標、要人做什麼」，加上一個會真的去算它的東西。

**門檻值多數是首發預設，不是實測校準值**。NFR-004 自陳效能目標「需在確認基礎設施後校準」，上線後第一個月應以實際分佈回填並註明校準日期。

**例外：`skillhub-cleanup-and-leaks` 群組的門檻已定值**——SEC-002 的六項門檻（威脅模型 Q18）於 2026-08-16 由 [ADR-022](../../docs/adr/ADR-022-sandbox-deployment-topology-and-security-thresholds.md) 第二部分定案（X-02 每 5 分鐘、X-03 同一筆連 2 輪、X-04 單節點 50%／全池 25% 下限 2 筆、6b 連 3 輪撤銷失敗）。**該群組的規則已全部是正式形式（2026-08-16，SBX-012 落地後）**：`LeakedSandboxStillPresent` 改讀 `skillhub_orphan_sandbox_persistent`、`CredentialRevokeFailing` 改讀 `skillhub_gateway_revoke_failed_total`，原本以累加計數器近似「同一筆連續 N 輪」與「哪一種資源撤不掉」的兩條過渡規則已移除近似。**ADR-022 定的動作（drain 節點、暫停整池派送、暫停 P-03 例行重建）屬平台實作，不是 Alertmanager 的職責。**（**2026-08-18 更新**：前兩個動作已由 `SEC-012` 落地——`dispatch_halts` 與 `internal/trial/execution/halt.go`，Reconciler 直接撥開關；本檔因此新增 `DispatchHalted`／`DispatchHaltedForIncident` 兩條，用途是**讓人知道平台已經自己停下來了**，仍然不是由告警去執行動作。**P-03 例行重建的暫停仍未做**：節點重建是部署期的動作，平台這一側沒有它的控制點。）

`CleanupBacklogGrowing` 的門檻不在 ADR-022 的六項之內，但已於 **2026-08-16 依封測容量校準**：`> 5` → `> 2`。舊值大於整池 4 個 slot，代表整池沙箱全數洩漏都還低於門檻；新值取封測 4 slot 的 50%，與 X-04 單節點 drain 用同一個比例。池容量改變時要重推。

## 最該先看的一條

`TraceMaskingStopped`。遮罩器壞掉時系統一切正常運作，只是 Secrets 開始落庫——NFR-002 沒有其他偵測器，而 0019 的 `CHECK (masked)` 擋得住「沒跑遮罩」，擋不住「跑了但規則失效」。
