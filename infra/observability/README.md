# 平台可觀測性（O11Y-001～003）

## 這一層是什麼、不是什麼

ADR-009 把三種能力切開，這裡只涵蓋第一種：

| 平面 | 這裡 | 在哪裡 |
| --- | --- | --- |
| **Platform Observability** | ✅ 服務健康、延遲、錯誤率、Provider 可用性、清理積壓 | 本目錄 ＋ 兩個服務的 `/metrics` |
| Run Trace | ❌ | `contracts/events/trace-event.schema.json`、`services/platform/internal/trace` |
| Evaluation | ❌ | `evaluations` 表（M3） |

**告警不得依賴 Run Trace。** Trace payload 大量來自 Sandbox，是使用者可操控的輸入；讓告警讀它等於把觸發權交給不受信任的來源（ADR-009 背景段）。`alerts.yml` 的每一條規則都只讀服務自報的指標。

## 曝露方式

MVP 用 Prometheus 文字格式，各服務自己曝露，沒有 push gateway、沒有 agent：

| 服務 | 端點 | 開關 | 認證 |
| --- | --- | --- | --- |
| `services/platform`（`cmd/api`、`cmd/worker` 各一份） | `GET /metrics` | `METRICS_ADDR`（例 `:9090`），未設＝不監聽 | 無，**綁內部位址**——這是維運面，不掛在對外的 API port 上 |
| `services/sandbox`（`sandboxd`） | `GET /metrics` | 隨主服務 port | 與其他路由同一個 provider bearer token（該路由表沒有未認證路徑，這裡不開先例） |

`cmd/api` 與 `cmd/worker` 是兩個行程，各自有一份指標；Prometheus 分別 scrape，`sum()` 之後才是平台整體的數字。`alerts.yml` 的表達式都已經先 `sum` 過。

## 指標清單

實作在 `services/platform/internal/platform/metrics/metrics.go` 與 `services/sandbox/internal/sandbox/metrics.go`，Help 字串是權威說明，這裡只列覆蓋範圍與 NFR-005 的對照。

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

**未做的部分，明說**：沒有 Alertmanager 部署、沒有通知路由、沒有 silence 政策、沒有 Grafana dashboard。那些是部署期的事，本批交付的是「什麼算異常、依據哪個指標、要人做什麼」。掛法：Prometheus 的 `rule_files` 指向本檔。

**門檻值多數是首發預設，不是實測校準值**。NFR-004 自陳效能目標「需在確認基礎設施後校準」，上線後第一個月應以實際分佈回填並註明校準日期。

**例外：`skillhub-cleanup-and-leaks` 群組的門檻已定值**——SEC-002 的六項門檻（威脅模型 Q18）於 2026-08-16 由 [ADR-022](../../adr/ADR-022-sandbox-deployment-topology-and-security-thresholds.md) 第二部分定案（X-02 每 5 分鐘、X-03 同一筆連 2 輪、X-04 單節點 50%／全池 25% 下限 2 筆、6b 連 3 輪撤銷失敗）。該群組的規則已依定值改寫，但 `LeakedSandboxStillPresent` 與 `CredentialRevokeFailing` **目前是過渡形式**：現有累加計數器表達不了「同一筆連續 N 輪」，也區分不出「金鑰撤不掉」與「沙箱殺不掉」，需要 `03` 的 **SBX-012**（in-flight orphan 表 ＋ `gateway_revoke_failed`／`sandbox_destroy_failed` 分項計數器）。規則註解已逐條標明。**ADR-022 定的動作（drain 節點、暫停整池派送、暫停 P-03 例行重建）屬平台實作，不是 Alertmanager 的職責。**

## 最該先看的一條

`TraceMaskingStopped`。遮罩器壞掉時系統一切正常運作，只是 Secrets 開始落庫——NFR-002 沒有其他偵測器，而 0019 的 `CHECK (masked)` 擋得住「沒跑遮罩」，擋不住「跑了但規則失效」。
