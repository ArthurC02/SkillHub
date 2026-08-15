# Trace 事件樣本（TRACE-002～005）

這裡的 `*.jsonl` **不是手寫的範例**，是管線兩端的真實輸出，被記錄下來當作契約迴歸的證據。schema 自帶的 `examples` 是「契約寫得對」，這裡是「程式跑出來的東西真的符合契約」——後者才會在 harness 或遮罩器改動時破。

`python tools/contracts/validate_trace_events.py` 會逐行驗證本目錄的每個 `.jsonl`。

| 檔案 | 產生者 | 代表什麼 |
| --- | --- | --- |
| `run-trace-sample.jsonl` | `services/sandbox/internal/dockerdrv` 的 `TestTraceEventsReachTheCollectorFromARealContainer` | **生產端未遮罩的線上格式**：真實容器寫進自己 `/out` tmpfs、由 driver 讀出、collector 推送後在接收端錄到的原樣。`masked: false` 是正確的——沙箱不得自稱已遮罩（鐵律 11）。 |
| `stored-trace-sample.jsonl` | `services/platform/internal/identity` 的 `TestTraceIngestionMasksBeforeStorage...` | **入庫後的形態**：`trace_events` 的列重新組回 envelope。`masked: true`、`masked_fields` 指出被替換的位置、payload 內是 `[REDACTED]`。 |

重新產生（兩者都由環境變數開關，平常跑測試不會寫檔）：

```bash
# 生產端：需要可用的 Docker daemon
SKILLHUB_TRACE_SAMPLE_OUT=contracts/events/samples/run-trace-sample.jsonl \
  go test ./internal/dockerdrv/ -run TestTraceEventsReachTheCollector -count=1

# 入庫端：需要 SKILLHUB_TEST_DATABASE_URL
SKILLHUB_TRACE_SAMPLE_OUT=contracts/events/samples/stored-trace-sample.jsonl \
  go test ./internal/identity/ -run TestTraceIngestionMasksBeforeStorage -count=1
```

兩份樣本的 `run_id`、`event_id` 都是測試夾具，不是任何真實 Run；`stored-trace-sample.jsonl` 的 id 每次重產會變（來自即時建立的測試 Run），內容形狀則不變。
