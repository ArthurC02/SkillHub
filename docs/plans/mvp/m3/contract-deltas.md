# M3 契約增量清單（第 1 批）

- 日期：2026-08-16
- 狀態：**清單，未實作**。依鐵律 12「跨語言介面先寫 OpenAPI schema 再實作；CI 以 codegen 檢查 drift」。
- 用法：本文件只列**要動哪個檔、加哪些 path／schema、每個 schema 的形狀與存在理由**。**不寫 YAML 實體**——實體由第 1 批的四個 agent 各自在自己的檔案內產出。
- 四個檔案互不重疊，可四個 agent 平行：

  | # | 檔案 | 節 |
  | --- | --- | --- |
  | 1 | `contracts/openapi/public.yaml` | §1、§4 |
  | 2 | `contracts/openapi/llm-internal.yaml` | §2 |
  | 3 | `contracts/events/trace-event.schema.json` | §3 |
  | 4 | `db/migrations/0024_evaluation.sql` ＋ `db/queries/` | 非契約，但同批；形狀見 [evaluation-design.md](evaluation-design.md) §3.2 |

---

## 1. `public.yaml`：新增 path

所有端點皆為 **workspace-scoped，scope 取自 session**（鐵律 3）；非擁有者一律 **404**（沿用 `CORE-006` 既有慣例，不新增 403 語意）。

### 1.1 評估讀取與回饋

| Path | Method | 形狀摘要 | 需求 |
| --- | --- | --- | --- |
| `/runs/{id}/evaluation` | GET | 回 `Evaluation`。**Run 沒有評估時回 404 而非空物件**——「未評估」是一個狀態，不是一份空評估（設計 §4.3）。`?revision=` 可選，省略即當前版（`superseded_at IS NULL`） | `02:EVAL-001` |
| `/runs/{id}/evaluation/revisions` | GET | 回 `EvaluationRevision[]`（id、`judge_prompt_version`、`rubric_version`、`overall`、`evaluated_at`、`superseded_at`）。存在理由：重評是 append-only，使用者要看得出「這份判定是哪個 rubric 下的」 | `02:EVAL-001`、設計 §3.2b |
| `/runs/{id}/evaluation/feedback` | PUT | body `{helpful: boolean, comment?: string}`。**PUT 不是 POST**：使用者可以改主意，這是既有列的可變欄位（設計 §3.2c） | `02:EVAL-001` 第 4 條 |

### 1.2 改善建議

| Path | Method | 形狀摘要 | 需求 |
| --- | --- | --- | --- |
| `/runs/{id}/suggestions` | GET | 回 `ImprovementSuggestion[]`，掛在**當前評估**上 | `02:EVAL-002` |
| `/suggestions/{id}/decision` | PUT | body `{decision: accepted\|rejected}`。**只記決定，不套用**——套用是下一個端點，兩者分開是因為「接受五條、一次建一個版本」（設計 §5.3） | 第 3 條 |
| `/suggestions/{id}/diff` | GET | 回 `SuggestionDiff`：`target_path` ＋ unified diff 文字 ＋ `applicable: boolean` ＋ `blocked_reason?`。**套用前可查看差異**是允收準則，所以 diff 是端點不是前端自行計算 | 第 3 條 |
| `/skills/{id}/versions/from-suggestions` | POST | body `{evaluation_id, suggestion_ids[]}` → 建**一個**新 Skill Version。回 `SkillVersion` ＋ `applied_suggestion_ids`。失敗時逐條回拒絕理由（路徑越界／雜湊不符／驗證阻擋／授權受限，設計 §5.2） | 第 4 條、鐵律 4 |

### 1.3 重跑與比較

| Path | Method | 形狀摘要 | 需求 |
| --- | --- | --- | --- |
| `/runs/{id}/comparison` | GET | query `against=<run_id>`。回 `RunComparison` | `02:EVAL-003` 第 2 條 |

**不新增「重跑」端點。** 重跑就是既有的 `POST /skills/{id}/runs`，帶新的 `skill_version_id` 與同一個 `test_case_id`——**而且照樣要先過 preflight 與 `confirmed_summary_hash`**（設計 §5.4）。多開一個端點等於多開一條繞過權限確認的路。

---

## 2. `public.yaml`：新增 schema

| Schema | 形狀要點 | 為什麼是這個形狀 |
| --- | --- | --- |
| `Evaluation` | `evaluation_id`、`run_id`、`status`(`pending\|completed\|failed`)、`overall`(`met\|partially_met\|not_met\|undetermined`)、`summary`、`criterion_results[]`、`deterministic_findings[]`、`judge_model`、`judge_prompt_version`、`rubric_version?`、`evidence_complete`、`cost`、`feedback?`、`evaluated_at` | `status` 與 `overall` **是兩件事**：評估跑不動 vs 判定不出來。少了 `status`，UI 分不出「評估失敗」與「無法判斷」 |
| `CriterionResult` | `criterion_id`、`text`、`result`(`passed\|failed\|undetermined`)、**`source`(`rule\|model\|user`)**、`evidence[]`、`reason` | `source` 是 `02:EVAL-001` 第 5 條「LLM Judge 的判斷必須標示為模型評估」的落點，**required 不可選** |
| `EvidenceRef` | `kind`(`trace_event\|artifact\|agent_output`)、`trace_event_id?`、`occurred_at?`、`artifact_path?`、`byte_range?`、`char_range?`、**`excerpt`**、`excerpt_truncated`、**`available: boolean`** | `occurred_at` 是必要的：`trace_events` 按時間分割，少了它定位不到分割區。`excerpt` ＋ `available` 是「證據會過期」的形狀（設計 §3.4）——分割區被清掉後仍讀得到摘要，且**誠實標明原始事件已不在** |
| `DeterministicFinding` | `category`(`spec\|activation\|execution\|effect\|compatibility\|cost`)、`severity`、`message`、`evidence[]` | `02:EVAL-001` 第 1 條的六類分開呈現。`effect` 一律由 Judge 產出，其餘由規則 |
| `EvaluationCost` | `evaluation_usd`、`source`、`note` | **只放評估自身的成本**。Run 成本在 `RunComparison` 那邊且標明是下界（丙-3），兩者不合併為一個數字 |
| `EvaluationRevision` | 見 §1.1 | — |
| `ImprovementSuggestion` | `suggestion_id`、`category`(`skill\|runtime\|mcp\|tool\|dataset`)、`problem`、`evidence[]`、`target_path`、`expected_impact`、`decision`、`decided_at?`、`applied_skill_version_id?` | 五個必要欄位直接對應 `02:EVAL-002` 第 1 條。`mcp` 在 MVP 恆不產生，型別佔位（同 `TRACE-003` 前例） |
| `SuggestionDiff` | `target_path`、`unified_diff`、`applicable`、`blocked_reason?` | `blocked_reason` 的值域即設計 §5.2 的五項拒絕條件 |
| `RunComparison` | `runs[2]`（各含 `run_id`、`skill_version_id`、`status`、`evaluation` 摘要、`final_output`、`errors[]`、`duration_ms`、**`cost: {usd, is_lower_bound: true, authoritative_source}`**）、`criterion_matrix[]`（逐條 × 兩次）、`version_diff_url` | `is_lower_bound` **寫死為 true 且 required**：丙-3 要求標明它是下界，把這件事放進 schema 就不可能在某個畫面被忘記 |

**`is_lower_bound` 值得單獨說一句**：它不是一個「有時 true 有時 false」的旗標。Trace 的成本合計結構上就是下界（串流結束時在途的回應不計、最後一次 flush 可能落在讀取之後），把它做成常數 required 欄位，是讓契約自己承擔誠實義務而不是靠每個 UI 記得。

---

## 3. `llm-internal.yaml`：新增 path 與 schema

沿用既有 `/suggest-criteria` 的形式（strict `json_schema`、無狀態、不知道 workspace）。

| Path | Method | 請求 | 回應 |
| --- | --- | --- | --- |
| `/judge-run` | POST | `JudgeRunRequest` | `JudgeRunResponse` |
| `/suggest-improvements` | POST | `SuggestImprovementsRequest` | `SuggestImprovementsResponse` |

| Schema | 形狀要點 |
| --- | --- |
| `JudgeRunRequest` | `skill: {name, summary}`、`user_prompt`、`criteria[]`（`{id, text}`）、`rubric?`、`final_output`、`artifacts[]`（`{path, size_bytes, content_type, text_excerpt?}`）、`trace_digest`（**已聚合的摘要，不是原始事件流**）、`truncation[]`（哪些欄位被截斷） |
| `JudgeRunResponse` | `criterion_results[]`（`{criterion_id, result, reason, evidence_refs[]}`）、`overall`、`summary`。**沒有 `source` 欄位**——Python 回什麼，Go 一律標 `source: model`；讓模型自稱是規則判定是不能給的權力 |
| `SuggestImprovementsRequest` | `evaluation_digest`、`file_tree[]`、`target_files[]`（`{path, content}`，有大小上限） |
| `SuggestImprovementsResponse` | `suggestions[]`（`{category, problem, evidence, target_path, proposed_content, expected_impact}`） |
| `Rubric` | `items[]`（`{id, text, weight?, evidence_required: boolean}`）。`CONTENT-007` 要求「逐項回傳證據引文」，所以 `evidence_required` 是 rubric 自己的欄位而不是全域開關 |

**三個必須寫進 schema description 的界線**（不是註解，是契約文字）：

1. `evidence_refs` 由 **Go 逐條回驗**；驗不過即降 `undetermined`（設計 §2.4 第 3 條）。契約要寫明 Python 側**無法**保證引用有效。
2. `truncation` 非空時，受影響的條目**允許**回 `undetermined`；契約要說「看不到全文而判 `passed` 是不可接受的」。
3. `trace_digest` 是聚合摘要不是事件流：**Python 服務不得取得完整 trace**（ADR-009「Evaluation 使用低權限讀取介面」）。

---

## 4. `trace-event.schema.json`：升 1.2（additive）

`TRACE-009` 已把事件 schema 升到 1.1。M3 需要兩個由**控制平面**產生的新事件（丙-2，沿用 `RecordOrchestratorEvent`）：

| 事件型別 | payload 要點 |
| --- | --- |
| `evaluation_started` | `evaluation_id`、`judge_model`、`judge_prompt_version`、`rubric_version?` |
| `evaluation_completed` | `evaluation_id`、`overall`、`criteria_total`／`passed`／`failed`／`undetermined`、`evidence_complete`、`cost_usd?`、`failure_reason?`（`status = failed` 時） |

規則同既有版本演進慣例：additive、1.1 事件仍是合法的 1.2 事件、只寫 1.1 欄位的 producer 繼續宣告 1.1。**沙箱 harness 不產這兩個事件**——評估發生在控制平面，harness 那時早就沒了。

---

## 5. 順手補上的兩筆既有欠帳

第 1 批既然要動 `public.yaml`，把 M2 記錄在案的兩個契約缺口一併補掉（皆為 additive，鐵律 12 已欠帳）：

| # | 缺口 | 出處 |
| --- | --- | --- |
| 欠-1 | `RunPermissionSummary` 未宣告 `estimated_cost`（實作已上線） | `03` TEST-011、`04` 乙-9 |
| 欠-2 | `PUT`／`DELETE /admin/skills/{id}/restriction` 未進 spec（實作已上線） | `03` SEC-011 |

補這兩筆**不改變任何行為**，只是讓 spec 追上實作。若第 1 批因故不補，必須在該批交付摘要裡重新記錄，不得靜默略過。

---

## 6. 明確不動的契約

| 檔案／schema | 為什麼不動 |
| --- | --- |
| `sandbox-provider.yaml` 的 `RunResult.usage` | 丙-7：token 與成本走 Trace 已足夠。要 provider 側 usage 是 additive 變更，等真的需要時再議 |
| `sandbox-provider.yaml` 的 `TracePolicy` | M3 不改 Trace 收集路徑 |
| `AcceptanceCriterion` | 形狀已經正確（`id`／`text`／`source`／`confirmed_at`），且 spec 註解早就預告了 EVAL-001 會用 `source` 這個欄位。**不加 `weight`**——rubric 已經承擔權重，加在兩個地方會有兩份真相 |
| `run_status` enum | 設計 §4 的決策本身：評估不新增也不改變 Run 狀態 |
