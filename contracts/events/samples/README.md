# Trace 事件樣本（TRACE-002～005）

這裡的 `*.jsonl` **不是手寫的範例**，是管線兩端的真實輸出，被記錄下來當作契約迴歸的證據。schema 自帶的 `examples` 是「契約寫得對」，這裡是「程式跑出來的東西真的符合契約」——後者才會在 harness 或遮罩器改動時破。

`python tools/contracts/validate_trace_events.py` 會逐行驗證本目錄的每個 `.jsonl`。

| 檔案 | 產生者 | 代表什麼 |
| --- | --- | --- |
| `run-trace-sample.jsonl` | `apps/sandbox/internal/dockerdrv` 的 `TestTraceEventsReachTheCollectorFromARealContainer` | **生產端未遮罩的線上格式**：真實容器寫進自己 `/out` tmpfs、由 driver 讀出、collector 推送後在接收端錄到的原樣。`masked: false` 是正確的——沙箱不得自稱已遮罩（鐵律 11）。 |
| `stored-trace-sample.jsonl` | `apps/platform/internal/identity` 的 `TestTraceIngestionMasksBeforeStorage...` | **入庫後的形態**：`trace_events` 的列重新組回 envelope。`masked: true`、`masked_fields` 指出被替換的位置、payload 內是 `[REDACTED]`。 |
| `harness-usage-sample.jsonl` | `apps/sandbox/internal/dockerdrv` 的 `TestHarnessStopsAtTheTokenCeilingAndStillReportsUsage` | **schema 1.1、且是「沒有 `result` 訊息」那條路徑的真實輸出**：真容器、真 Agent SDK、真閘道呼叫，harness 在 token 上限處自行中止，因此那一輪沒有 `result`。三個事件依序是 `agent_output`(intermediate) → `error`(`token_budget_exceeded`) → `usage`(`token_source: accumulated`，`cost_source: gateway`)。這正是修正前**完全不會出現 `usage`** 的情形，所以它是這條路徑的迴歸證據，不只是格式範例。 |

重新產生（兩者都由環境變數開關，平常跑測試不會寫檔）：

```bash
# 生產端：需要可用的 Docker daemon
SKILLHUB_TRACE_SAMPLE_OUT=contracts/events/samples/run-trace-sample.jsonl \
  go test ./internal/dockerdrv/ -run TestTraceEventsReachTheCollector -count=1

# 入庫端：需要 SKILLHUB_TEST_DATABASE_URL
SKILLHUB_TRACE_SAMPLE_OUT=contracts/events/samples/stored-trace-sample.jsonl \
  go test ./internal/identity/ -run TestTraceIngestionMasksBeforeStorage -count=1

# harness 的 usage 路徑：需要 Docker、Runtime Image，以及一把可用的閘道金鑰（會花錢，約 $0.002）
SKILLHUB_E2E_GATEWAY_URL=http://litellm:4000 SKILLHUB_E2E_GATEWAY_KEY=sk-... \
SKILLHUB_TRACE_SAMPLE_OUT=<absolute path>/contracts/events/samples/harness-usage-sample.jsonl \
  go test ./internal/dockerdrv/ -run TestHarnessStopsAtTheTokenCeiling -count=1
```

兩份樣本的 `run_id`、`event_id` 都是測試夾具，不是任何真實 Run；`stored-trace-sample.jsonl` 的 id 每次重產會變（來自即時建立的測試 Run），內容形狀則不變。
