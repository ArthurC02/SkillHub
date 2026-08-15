# Run Trace 事件契約（TRACE-001）

- 檔案：[trace-event.schema.json](trace-event.schema.json)（JSON Schema 2020-12，英文）
- 驗證：`python tools/contracts/validate_trace_events.py`（驗 schema 內所有範例實例 + 三個反例）
- 狀態：M2 第一批產出，**只定契約**。事件的產生與收集屬 TRACE-002~004、遮罩執行屬 TRACE-005、UI 呈現屬 TRACE-006/007、排序與重送處理屬 TRACE-008。

## 1. 這份 schema 的位置（ADR-009 邊界）

ADR-009 把三種能力切開，共用 Correlation ID 但**資料模型與存取政策不同**：

| 平面 | 內容 | 本 schema 是否涵蓋 |
| --- | --- | --- |
| Platform Observability | 服務健康、延遲、錯誤率、Provider 可用性、Egress 阻擋統計 | ❌ 不涵蓋。走 O11Y-001~003，另有模型與保存政策 |
| **Run Trace** | 單次 Run 的使用者可見標準事件 | ✅ **就是這份** |
| Evaluation | 依 Skill／Test Case／Trace／輸出產生的判斷與建議 | ❌ 不涵蓋。落在 `evaluations` 表 |

兩條由此而來的硬性規則：

1. **不要把平台內部資訊塞進 Run Trace 事件**。使用者看得到這裡的每一個欄位；主機名稱、內部 IP、佇列深度、其他 Workspace 的任何線索都不屬於這裡。
2. **維運告警不得讀這份資料**。Trace payload 大量來自 Sandbox，屬於使用者可操控的輸入；讓告警依賴它等於把告警交給不受信任的來源（ADR-009 背景段的原始理由）。

`emitted_by: sandbox` 的事件跨越信任邊界（ADR-001）：控制平面收到後視為**不受信任輸入**，先驗 schema、再遮罩、再落庫；UI 一律以 inert text 呈現，不解讀 ANSI／HTML／SVG（ADR-009）。

## 2. 事件型別

| `type` | 用途 | 需求 | 備註 |
| --- | --- | --- | --- |
| `skill_activation` | Skill 被啟用或被略過（含理由） | TRACE-002 | `decision: activated \| skipped` |
| `resource_read` | 套件內檔案／資料集被讀取 | TRACE-002 | 路徑相對於套件或掛載根目錄，不外洩 Sandbox 絕對路徑 |
| `tool_call` | 一次完成的工具呼叫 | TRACE-003 | **單事件**，自帶 `duration_ms`（理由見 §3） |
| `mcp_call` | MCP 呼叫 | TRACE-003 | **佔位**。遠端 MCP 已移出 MVP 首發，只保留型別讓 `event_type` 值域穩定；payload 形狀未定案，不得依賴 |
| `script_log` | Sandbox 內腳本輸出 | TRACE-003 | `stream: stdout \| stderr`、`truncated`、`dropped_bytes` |
| `agent_output` | Agent 產出的文字（`final` 為最終回答） | TRACE-004 | |
| `error` | 值得對使用者顯示的失敗 | TRACE-004 | `category` 對齊 ADR-004 失敗分類 |
| `usage` | Token 與成本計量 | TRACE-004 | cache 欄位可為 `null`，見 §5 |
| `run_lifecycle` | Run 狀態轉移的**鏡像** | RUN-002 | **非事實來源**，見 §4 |

ADR-009 另有列 `Artifact Produced`、`Policy Decision`、`Security Event` 三類。本批**刻意未定義**：TRACE-002~004 的收集需求沒有用到，而新增事件型別在本契約下屬 additive（§7），要用時再加不必改版本主號。

每型都有一筆完整範例實例，放在 schema 頂層的 `examples` 陣列，validator 逐筆驗證。

## 3. `tool_call` 為何是單事件而非 start/end 配對

配對的成本是實的：分割表列數翻倍、UI 要自我 join 才算得出耗時、Sandbox 中途死掉就留下永遠等不到 end 的孤兒 start。TRACE-004 要的是延遲，`duration_ms` 就夠。

代價講清楚：工具執行期間沒有「進行中」事件。若 TRACE-006 的一般模式進度真的需要即時顯示「正在呼叫 X」，**新增 `tool_call_started` 型別**（additive），不要回頭在 `tool_call` 上補 phase 欄位——那會讓既有消費者的「一筆＝一次完成呼叫」假設失效。

## 4. `run_lifecycle` 不是事實來源（鐵律 5）

Run 狀態的唯一事實來源是 Go 擁有的 Postgres 狀態機（`runs.status` 與 `run_status_transitions`）。`run_lifecycle` 事件存在的理由只有一個：讓 Trace 時間軸是一條連續的故事，而不是一堆沒有上下文的工具呼叫。

因此：**不得**以重播這些事件來重建 Run 狀態；消費者對 Run 狀態的認知與 `runs` 表不一致時，以 `runs` 表為準。schema 內 `runLifecyclePayload` 的 `$comment` 已寫死這條。

## 5. Cache 用量欄位為何允許 `null`

`pdm-003-litellm-spike-report.md` §11.5.2 實測：LiteLLM 1.96.2 在 `/v1/messages` 路由上**完全不輸出** `cache_read_input_tokens` 與 `cache_creation_input_tokens`（是缺欄，不是 0），`/v1/chat/completions` 則正常透傳。計費不受影響（LiteLLM 內部有正確套用快取折扣），**受損的是可觀測性**。

所以 schema 把 `cache_read_input_tokens` / `cache_write_input_tokens` / `cost_usd` 都設為 nullable，並在 `$comment` 註明成因與出處。消費端規約：

- `null` 一律呈現為「未回報」，**不得**顯示為 `0`——那會讓使用者以為快取沒命中。
- `cost_source: estimated` 的金額在 UI 必須標示為估算值，不得與閘道回報值混為一談。

## 6. 遮罩規約（schema 層；執行屬 TRACE-005）

鐵律 11 與 NFR-002：Secrets 不得出現在 Log、Trace 明文或分析事件。schema 能做的是**把哪些欄位危險標記出來，並強制記錄遮罩是否跑過**。

**(a) 敏感欄位標記。** 任何可能承載 Secret 的欄位，在 schema 定義上帶 `$comment` 且以固定前綴開頭：

```
"$comment": "sensitivity: secret_bearing - <為什麼這個欄位危險>"
```

遮罩器（TRACE-005）以此清單為工作範圍，不靠字串猜測欄位名。目前被標記的欄位：

| 事件型別 | 欄位 |
| --- | --- |
| `skill_activation` | `reason` |
| `tool_call` | `arguments`（整個物件）、`result_summary` |
| `mcp_call` | 整個 payload |
| `script_log` | `message` |
| `agent_output` | `text` |
| `error` | `message`、`provider_diagnostics`（整個物件） |

**(b) 命名規約。** 未標記 `secret_bearing` 的欄位一律是**平台自產的結構化值**（ID、enum、計數、時間），新增欄位時二選一：不是平台自產，就必須帶標記。不允許出現「平台自產但其實會夾帶使用者內容」的第三類——那正是遮罩漏網的來源。

**(c) 遮罩狀態隨事件走。** envelope 兩個欄位：

- `masked`（必填 boolean）：遮罩器是否已處理過。**`masked: false` 的事件不得落庫、不得顯示。**
- `masked_fields`（JSON Pointer 陣列，相對於 `payload`）：實際被替換掉的位置。空陣列表示「跑過但沒東西要遮」，與「沒跑過」語意不同。

**(d) 佔位字串。** 被遮罩的值一律替換為字面量 `[REDACTED]`，不保留長度、不保留前後綴、不做部分遮罩（部分遮罩會洩漏熵）。

## 7. 排序、重送與缺失（TRACE-008 預留）

- **`seq` 單調且無洞，範圍是 `(run_id, attempt, emitted_by)` 三元組**，從 1 起算。序號出現斷點＝有事件遺失，這是 TRACE-001「能識別缺失事件」的實作基礎。
- **不用全 Run 單一計數器**。理由：跨 producer 的全域計數器需要一個序列化點；而若改由 ingest 端在寫入時發號，遺失就在定義上永遠偵測不到——等於把可觀測性換掉效能。
- **跨 producer 排序用 `(occurred_at, emitted_by, seq)`**，不可只用 `seq`。不同 producer 的時鐘會漂移，`occurred_at` 只負責合併三條串流，串流內部的權威順序是 `seq`。
- **傳遞語意 at-least-once**（ADR-008 Outbox）。消費者必須冪等，**冪等鍵是 `event_id`**（producer 產生的 UUID）；已存在的 `event_id` 再次到達時視為 no-op，不是更新。
- **延遲事件不重排既有時間軸**：晚到的事件按其 `seq` 插入所屬串流；UI 在偵測到斷點或串流未收尾時必須明示「可能不完整」，不得假裝完整（ADR-009）。

## 8. 與 `trace_events` 表的映射

表定義：`db/migrations/0004_test_lab_and_runs.sql`（按 `occurred_at` 月份 range 分割）。

| schema 欄位 | `trace_events` 欄位 | 說明 |
| --- | --- | --- |
| `event_id` | *（無對應欄位）* | ⚠️ 缺口 1 |
| `run_id` | `run_id` | 直接對應 |
| — | `workspace_id` | 由 `run_id` 在寫入端查出後填入（鐵律 3，不信任外部傳入） |
| `attempt` | *（無對應欄位）* | ⚠️ 缺口 2 |
| `seq` | `seq`（`bigint`） | 直接對應 |
| `occurred_at` | `occurred_at` | 分割鍵 |
| `type` | `event_type`（`text`） | 直接對應 |
| `emitted_by` | `source`（`text`） | 直接對應 |
| `status` | `status`（`text`，nullable） | 直接對應 |
| `payload` | `payload`（`jsonb`） | 直接對應 |
| `payload_object_key` | `payload_object_key`（`text`） | 直接對應 |
| `schema_version` | *（無對應欄位）* | ⚠️ 缺口 3 |
| `masked` / `masked_fields` | *（無對應欄位）* | ⚠️ 缺口 4 |
| — | `id`（`uuid`，DB 產生） | 見缺口 1 |

### 缺口清單（回報 db 負責人，本批不改 migration）

1. **`event_id` 無欄位。** 表的主鍵 `id` 是 `gen_random_uuid()` 產生的，同一事件重送兩次會得到兩個不同 `id`、變成兩列。at-least-once 傳遞下這是必然發生的重複。需要 producer 提供的 `event_id` 欄位加上 `UNIQUE (event_id, occurred_at)`（分割表的唯一索引必須含分割鍵），寫入用 `ON CONFLICT DO NOTHING`。
2. **`attempt` 無欄位。** ADR-004 明確區分 `run_id` 與 run attempt，`runs.attempt` 也已存在；Trace 少了這欄，重試後的事件與首次嘗試的事件會混在同一條時間軸上，而且 `seq` 的三元組範圍在 DB 端無法表達。建議加 `attempt integer NOT NULL DEFAULT 1`，索引改 `(run_id, attempt, source, seq)`。
3. **`schema_version` 無欄位。** ADR-009「影響／成本與限制」已列出「Trace Schema 版本與事件轉譯需治理」。版本若只藏在 `payload` 裡，跨版本查詢與轉譯要全表掃 jsonb。建議加 `schema_version text NOT NULL`。
4. **遮罩狀態無欄位。** ADR-009 要求每個事件包含「遮罩狀態」。目前只能塞進 `payload` jsonb，無法用 DB 約束擋下未遮罩事件，也無法便宜地稽核「有多少事件是被遮罩過的」（O11y 的「Secrets 遮罩失敗」指標會需要）。建議加 `masked boolean NOT NULL` 與 `masked_fields jsonb NOT NULL DEFAULT '[]'`；`CHECK (masked)` 可考慮，但會擋掉「故意保留未遮罩以供事故調查」的路徑，由 db 負責人裁定。

非缺口、但需注意：現有索引 `(run_id, seq)` 在 `seq` 改為 per-producer 範圍後不再具區別性（同一 Run 內 `seq=1` 會有三筆，分別來自三個 producer）。它仍能用於範圍掃描，但不應被當成唯一性保證——這是設計如此，不是缺陷。

## 9. 版本演進規則

`schema_version` 形如 `MAJOR.MINOR`，目前 `1.0`。

**Minor bump（additive，消費者不需改）——允許：**

- 新增 optional envelope 欄位。
- 新增事件型別（`type` enum 新值 + 對應 payload 定義）。消費者遇到不認得的 `type` 必須忽略該事件而非報錯。
- 新增 optional payload 欄位。
- 放寬既有限制（如 `maxLength` 調大）。

**Major bump（breaking，需新 `$id` 與轉譯層）——以下任一即是：**

- 移除或改名任何欄位。
- 把 optional 改成 required。
- 收緊既有欄位的值域（enum 刪值、`maxLength` 調小、nullable 改 non-null）。
- 改變既有欄位的語意（含把 `null` 的意義從「未回報」改成別的）。

`$defs/runStatus` 是 `db/migrations/0004` 的 `run_status` enum 的手動鏡像。**DB enum 是定義，這裡是複本**；改 DB enum 時必須同步這裡，並依上述規則判定是 minor 還是 major。

新增或修改任何事件型別後，schema 的 `examples` 必須補上對應範例——validator 會檢查每個 `type` 至少有一筆範例，缺了就失敗。
