# ADR-009：分離平台 O11y、Run Trace 與 Evaluation

- 狀態：Accepted
- 日期：2026-08-13
- 決策者：產品負責人、架構規劃

## 背景

原始需求包含 O11y，而 Skill Lab 也需要向使用者顯示執行過程。兩者雖然都包含 Log、Metric 與 Trace，但用途、資料敏感度、保存期限和使用者權限不同。

評估又會讀取 Run Trace 與輸出，若三者混在一起，可能讓使用者看到平台內部資訊，或讓維運告警依賴可被使用者輸入操控的資料。

## 決策

將下列三類能力分離，使用共同 Correlation ID 但不同資料模型與存取政策：

1. **Platform Observability**：服務健康、效能、錯誤與基礎設施監控。
2. **Run Trace**：使用者可見、針對單次 Skill 執行的標準化事件。
3. **Evaluation**：根據 Skill、Test Case、Trace、輸出與規則產生的判斷及改善建議。

## Platform Observability

至少量測：

- API、搜尋與資料存取延遲及錯誤率。
- Run 排隊、Provision、執行、評估及清理時間。
- Run 成功、失敗、取消、逾時與遺留資源比例。
- Provider 可用性、容量與錯誤分類。
- Egress 阻擋、安全事件與 Secrets 遮罩失敗。
- 模型、Sandbox、儲存與網路用量。

平台 O11y 不作為使用者歷史 Run 的事實來源。

## Run Trace

標準事件至少支援：

- Skill Activated／Skipped。
- Resource Loaded。
- Agent Step 與模型呼叫摘要。
- Tool Call 與 Tool Result。
- MCP Call 與 MCP Result。
- Script Start、Log、Exit。
- Artifact Produced。
- Policy Decision 與 Security Event。
- Error、Retry、Cancel、Timeout。
- Usage 與 Cost Estimate。

每個事件包含 `run_id`、Attempt、時間、序號或排序資訊、來源與遮罩狀態。

## 一般與進階檢視

- 一般模式將事件彙整為可理解的進度與問題。
- 進階模式顯示經安全處理的結構化事件及 Log。
- UI 不直接顯示未過濾的 ANSI、HTML、SVG 或可執行內容。
- Trace 缺失或順序不確定時明確標示，不假裝完整。

## Evaluation 邊界

Evaluation 可使用：

- 確定性規則與格式檢查。
- 工具及程序 Exit Status。
- 使用者定義驗收條件。
- 預期輸出或資料比對。
- LLM Judge。
- 使用者人工判斷。

每個 Criterion Result 必須標示判斷來源、證據與信心。LLM Judge 結果不得描述為確定事實。

Evaluation 使用低權限讀取介面，不取得 Sandbox 控制、MCP 憑證或一般平台管理工具。它讀取的內容仍視為可能包含 Prompt Injection。

## Correlation

```text
request_id    單次 API 或使用者互動
run_id        完整 Run 的永久關聯
attempt_id    一次 Provider 執行嘗試
trace_id      分散式服務呼叫關聯
event_id      單一領域或 Run Trace 事件
```

不同 ID 不互相取代，但應能安全關聯。Log 不記錄 Prompt、Dataset 或 Secret 全文。

## 影響

### 正面

- 使用者體驗、評估證據與平台維運各自有清楚資料邊界。
- 可依敏感度與成本使用不同保存期限。
- 評估模型無需取得高權限執行能力。

### 成本與限制

- 同一次 Run 可能產生三套關聯資料，需要一致 Correlation。
- Trace Schema 版本與事件轉譯需治理。
- 詳細 Trace 可能昂貴，需設等級、大小與保存限制。

## 待決策

- Trace 的保存格式、即時傳輸與查詢技術。
- 一般模式摘要由規則、模型或混合方式產生。
- 每個方案允許的 Trace 詳細度與保存期限。→ 保存期限本身仍未定值（PDM-006）；**Trace 被清掉後評估證據怎麼辦**已由 [ADR-026](./ADR-026-evaluation-reassessment-evidence-lifetime-and-judge-trust-boundary.md) 決策 2 回答（引用 ＋ 判定當下的可讀摘要雙存，過期時誠實標明）。
- 本節「Evaluation 邊界」只寫了「讀取的內容仍視為可能包含 Prompt Injection」，未給防線。→ [ADR-026](./ADR-026-evaluation-reassessment-evidence-lifetime-and-judge-trust-boundary.md) 決策 3（四條防線）。Evaluation 的判定與 Run 終態的關係另見 [ADR-025](./ADR-025-run-terminal-state-and-evaluation-verdict-separation.md)。
- 本 ADR 的三分（外加 [ADR-017](./ADR-017-model-gateway-and-llm-observability.md) 的 Langfuse 第四軌）**不涵蓋使用者行為的漏斗量測**——那既不是平台健康、也不在任何一次 Run 之內。→ [ADR-029](./ADR-029-product-analytics-events-and-audit-trace-boundaries.md) 把它定為**第五類且明確從屬**的資料（不是事實來源），並劃清它與 audit event、Run Trace 的邊界；鐵律 11 的「分析事件」自該 ADR 起取得定義。

