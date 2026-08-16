# Run Trace 事件契約（TRACE-001）

- 檔案：[trace-event.schema.json](trace-event.schema.json)（JSON Schema 2020-12，英文）
- 驗證：`python tools/contracts/validate_trace_events.py`（驗 schema 內所有範例實例 + 三個反例）
- 樣本：[samples/](samples/) 是管線兩端的真實輸出（生產端未遮罩、入庫端已遮罩），validator 一併逐行驗證
- 狀態：契約為 M2 第一批產出；**收集管線於第三批（2026-08-16）落地**——事件產生見 `infra/images/runtime-agent-sdk/run.mjs`，收集與推送見 `services/sandbox/internal/sandbox/trace.go`，遮罩、入庫與兩種讀取模式見 `services/platform/internal/trace`。§8 的四個表欄位缺口已由 `db/migrations/0019_trace_ingestion.sql` 全數關閉。

## 1. 這份 schema 的位置（ADR-009 邊界）

ADR-009 把三種能力切開，共用 Correlation ID 但**資料模型與存取政策不同**：

| 平面 | 內容 | 本 schema 是否涵蓋 |
| --- | --- | --- |
| Platform Observability | 服務健康、延遲、錯誤率、Provider 可用性、Egress 阻擋統計 | ❌ 不涵蓋。走 O11Y-001~003，另有模型與保存政策 |
| **Run Trace** | 單次 Run 的使用者可見標準事件 | ✅ **就是這份** |
| Evaluation | 依 Skill／Test Case／Trace／輸出產生的判斷與建議 | ❌ 不涵蓋。落在 `evaluations` 表。1.2 的 `evaluation_started`／`evaluation_completed` 是**時間軸標記**（起訖、整體結果、計數），判定本身仍不在這裡，見 §4.1 |

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
| `evaluation_started` | 控制平面開始評估這個 Run | EVAL-001 | `emitted_by` 限 `orchestrator`，見 §4.1 |
| `evaluation_completed` | 評估結束（有沒有判定都算） | EVAL-001 | 同上；envelope `status` 分「評完了」與「評不動」 |

ADR-009 另有列 `Artifact Produced`、`Policy Decision`、`Security Event` 三類。本批**刻意未定義**：TRACE-002~004 的收集需求沒有用到，而新增事件型別在本契約下屬 additive（§7），要用時再加不必改版本主號。

每型都有一筆完整範例實例，放在 schema 頂層的 `examples` 陣列，validator 逐筆驗證。

## 3. `tool_call` 為何是單事件而非 start/end 配對

配對的成本是實的：分割表列數翻倍、UI 要自我 join 才算得出耗時、Sandbox 中途死掉就留下永遠等不到 end 的孤兒 start。TRACE-004 要的是延遲，`duration_ms` 就夠。

代價講清楚：工具執行期間沒有「進行中」事件。若 TRACE-006 的一般模式進度真的需要即時顯示「正在呼叫 X」，**新增 `tool_call_started` 型別**（additive），不要回頭在 `tool_call` 上補 phase 欄位——那會讓既有消費者的「一筆＝一次完成呼叫」假設失效。

## 4. `run_lifecycle` 不是事實來源（鐵律 5）

Run 狀態的唯一事實來源是 Go 擁有的 Postgres 狀態機（`runs.status` 與 `run_status_transitions`）。`run_lifecycle` 事件存在的理由只有一個：讓 Trace 時間軸是一條連續的故事，而不是一堆沒有上下文的工具呼叫。

因此：**不得**以重播這些事件來重建 Run 狀態；消費者對 Run 狀態的認知與 `runs` 表不一致時，以 `runs` 表為準。schema 內 `runLifecyclePayload` 的 `$comment` 已寫死這條。

### 4.1 兩個 `evaluation_*` 事件同理，而且限定 producer

`evaluation_started` / `evaluation_completed`（1.2）存在的理由與 `run_lifecycle` 一樣：**讓時間軸不要在 Run 結束的那一刻斷掉**。判定的事實來源是 `evaluations` 表（ADR-009 的 Evaluation 平面），不是這兩個事件；逐條判定、證據引用、改善建議都**不**放進 Trace。

多一條 `run_lifecycle` 沒有的限制：schema 把這兩型的 `emitted_by` 釘死為 `orchestrator`。評估發生在控制平面、發生時沙箱早已銷毀，所以一筆 `emitted_by: sandbox` 的 `evaluation_completed` 只可能是不受信任平面偽造的判定（validator 有對應反例）。

envelope 的 `status` 承擔「評完了」與「評不動」的區別：`ok` ＝評估跑完（`overall` 可能是 `undetermined`，那是**判不出來**這個判定本身），`error` ＝評估失敗，此時 `failure_reason` 有值。三者在 UI 上是三件事，不得合併為「沒通過」。

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
| `evaluation_completed` | `failure_reason` |

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
| `schema_version` | `schema_version`（`text`） | ✅ 缺口 3 已補（0019） |
| `masked` / `masked_fields` | `masked`（`boolean`）／`masked_fields`（`jsonb`） | ✅ 缺口 4 已補（0019） |
| — | `late`（`boolean`） | 平台指派：事件抵達時該 Run 已終態（TRACE-008） |
| — | `id`（`uuid`，DB 產生） | 保留為 PK 的一部分；冪等鍵是 `event_id` |

### 缺口清單（**2026-08-16 由 `db/migrations/0019_trace_ingestion.sql` 全數關閉**）

1. ~~**`event_id` 無欄位。**~~ **已補**：`event_id uuid NOT NULL` ＋ `UNIQUE (event_id, occurred_at)`（分割表的唯一索引必須含分割鍵），寫入用 `ON CONFLICT DO NOTHING`——重送更新零列，呼叫端據此計算重複數而不必再查一次。
2. ~~**`attempt` 無欄位。**~~ **已補**：`attempt integer NOT NULL DEFAULT 1 CHECK (attempt >= 1)`，並新增索引 `(run_id, attempt, source, seq)`。0004 的 `(run_id, seq)` 保留供範圍掃描。
3. ~~**`schema_version` 無欄位。**~~ **已補**：`schema_version text NOT NULL DEFAULT '1.0'`，存 producer 宣告的版本。
4. ~~**遮罩狀態無欄位。**~~ **已補**：`masked boolean NOT NULL DEFAULT false` ＋ `masked_fields jsonb NOT NULL DEFAULT '[]'`。**`CHECK (masked)` 已加**——db 負責人裁定如下：鐵律 11 沒有例外，「故意保留未遮罩以供事故調查」不是本系統存在的路徑，而那正是這條約束要擋的違規本身。因此「跳過遮罩」在資料庫層不可能，不只在程式碼層。

### 0019 另外處理的兩件事

- **`late boolean NOT NULL DEFAULT false`**（非原缺口）：TRACE-008 要求終態後仍收遲到事件。沙箱關機時推送的最後一批經常晚於平台判定終態，而那恰好是失敗 Run 最需要的部分（RUN-004），所以照收並標記，讓進階模式能說「這筆是遲到的」而不是默默重排時間軸。
- **DEFAULT 分割區**：0004 只建了 2026-08 一個月分割，並把後續分割稱為「維運工作」——但那個維運工作不存在，九月的第一筆事件會直接 INSERT 失敗、整條 Trace 消失。0019 加了 `trace_events_default`。代價寫在 migration 裡：日後要掛真正的月分割，必須先把 default 裡該月的資料清空（detach／搬移／re-attach）。

非缺口、但需注意：0004 的索引 `(run_id, seq)` 在 `seq` 改為 per-producer 範圍後不再具區別性（同一 Run 內 `seq=1` 會有三筆，分別來自三個 producer）。它仍能用於範圍掃描，但不應被當成唯一性保證——這是設計如此，不是缺陷。

## 9. 版本演進規則

`schema_version` 形如 `MAJOR.MINOR`，目前 `1.2`。

**版本紀錄**

| 版本 | 日期 | 變更 |
| --- | --- | --- |
| `1.0` | 2026-08-16 | 首版（TRACE-001）。 |
| `1.1` | 2026-08-16 | additive：`usagePayload.token_source`（`result`／`accumulated`／null）。成因見下段。 |
| `1.2` | 2026-08-17 | additive：新增 `evaluation_started`／`evaluation_completed` 兩個型別（M3 EVAL-001，[contract-deltas](../../docs/plans/mvp/m3/contract-deltas.md) §4）。兩者的 `emitted_by` 限 `orchestrator`，見 §4.1。 |

`token_source` 之所以需要：`usage` 事件原本只在 Agent SDK 的 `result` 分支發出，串流以任何其他方式結束（崩潰、牆鐘、token 上限中止，或實測到的「Run 成功但 SDK 沒給 result」）就**完全不發**——不是發 0，是不發，EVAL-012 的成本合計因此系統性低估（實測 45 個 Run：Trace $3.0879 vs 閘道實付 $3.3932）。harness 改為逐則訊息累計、結束時一律發一筆 run 級 usage，`token_source` 說明這筆的數字是 SDK 的 `result` 總計還是 harness 自己累計的。**`cost_source` 不承載這件事**：成本一律來自閘道，與 SDK 有沒有給 result 無關，把兩種來源塞進同一個欄位會讓「錢的來源」跟著「token 的來源」一起說謊。

**版本宣告的規則**：producer 宣告的是它「照哪一版契約寫」。只用 1.0 欄位的 producer（控制平面的 `error`／`run_lifecycle`）繼續宣告 `1.0` 是正確的，而 1.0 事件必然是合法的 1.1 事件；沙箱 harness 因為會寫 `token_source`，宣告 `1.1`；發 `evaluation_*` 的評估器宣告 `1.2`——那兩個型別在 1.0 不存在，**沿用 `internal/trace.SchemaVersion`（目前釘在 `"1.0"`）會宣告出一個當時還沒有這個型別的版本**，實作那一批必須讓宣告跟著事件型別走。消費端一律接受任何 `1.x`（`internal/trace.compatibleVersion`）。

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
