# ADR-005：模型閘道與可觀測性

- 狀態：Accepted
- 相關：[ADR-002 資料所有權與核心基礎設施](./ADR-002-data-ownership-and-core-infrastructure.md)、[ADR-003 Run 編排與非同步工作流程](./ADR-003-run-orchestration-and-async-workflows.md)、[ADR-004 Sandbox 隔離與執行安全](./ADR-004-sandbox-isolation-and-execution-security.md)、[ADR-006 身分、Workspace、准入與額度](./ADR-006-identity-workspace-admission-and-allowances.md)、[ADR-008 意圖搜尋](./ADR-008-intent-search.md)、[ADR-009 評估判定與 Judge 信任邊界](./ADR-009-evaluation-verdicts-and-judge-trust.md)、[ADR-010 產品分析與稽核邊界](./ADR-010-product-analytics-and-audit-boundaries.md)、[ADR-025 Credit 計量與扣款](./ADR-025-credit-metering-and-charging.md)

## 背景

平台有兩類模型呼叫：Python LLM 服務的平台工作負載（搜尋增強、Judge、改善建議），以及 Sandbox 內 Agent Runtime 試跑 Skill 時的模型呼叫。兩者都需要供應商可抽換、Run 級成本歸因、短效憑證（Sandbox 不得持有長效金鑰）。

同時，一次 Run 會產生三種用途、敏感度與保存期限都不同的資料：服務健康與基礎設施狀態、使用者可見的執行過程、以及對輸出的判斷與改善建議。三者若混在一起，會讓使用者看到平台內部資訊，或讓維運告警依賴可被使用者輸入操控的資料；評估若讀取 Trace 與輸出，也需要明確劃清它能拿到多少權限。

## 決策

### 決策 1：LiteLLM Proxy 是唯一模型出口

供應商 API Key 只保存在閘道；Python 服務與 Sandbox 只持有 Virtual Key，不直連供應商。

- **每個 Run 一把短效 Virtual Key**：Go 在 provisioning 階段透過閘道管理 API 簽發（預算與 TTL 由 Policy 模組決定），以 `ANTHROPIC_BASE_URL`／`ANTHROPIC_AUTH_TOKEN` 環境變數注入 Sandbox；平台側 preflight 揭露與 Trace 遮罩都以同一組名稱為 pattern；Run 終止即撤銷。
- **成本歸因**：Go 依 Virtual Key／`run_id` 標籤拉取閘道用量寫入 Usage Record；閘道數據是計量來源之一，不是領域事實來源。
- 模型抽換與容錯（fallback、重試、路由）設定在閘道層，Python 程式碼不綁定供應商；未來使用者自備 API Key 以閘道的 BYO Key 機制實作，平台程式碼不變。
- 每次 Run 快照記錄實際使用的平台 Prompt（Judge、增強、改寫）版本，併入不可變快照清單。
- LiteLLM 與主 PostgreSQL 共用實例、獨立邏輯 database，不新增資料產品。

理由：Proxy 部署是唯一能讓 Sandbox 內任意 Runtime 與 Python 服務走同一閘道的整合模式（SDK 內嵌無法要求 Sandbox Runtime 引入函式庫，且會讓金鑰散落各呼叫端），也是實作短效憑證與 Run 級成本歸因最省的路徑。

### 決策 2：閘道邊界守則

1. Go 不做推理呼叫，只操作閘道管理 API（Key 生命週期、用量拉取）。
2. 閘道用量與觀測資料都不是領域事實來源；遺失或延遲不影響 Run 正確性。
3. Sandbox 對閘道的存取仍經 Egress Proxy 允許清單，閘道位址是少數預設允許目的地之一。
4. 閘道故障視為 Provider 級故障處理，呼叫端不得繞過閘道直連供應商。

### 決策 3：三類可觀測資料分離，共用 Correlation 但各自獨立資料模型與存取政策

1. **Platform Observability**：服務健康、效能、錯誤與基礎設施監控，至少涵蓋 API／搜尋／資料存取延遲與錯誤率、Run 各階段耗時、Run 終態與遺留資源比例、Provider 可用性、容量與錯誤分類、Egress 阻擋、安全事件與 Secrets 遮罩失敗、模型／Sandbox／儲存／網路用量。不作為使用者歷史 Run 的事實來源。
2. **Run Trace**：使用者可見、針對單次 Skill 執行的標準化事件，至少支援 Skill Activated／Skipped、Resource Loaded、Agent Step 與模型呼叫摘要、Tool Call／Result、MCP Call／Result、Script Start／Log／Exit、Artifact Produced、Policy Decision／Security Event、Error／Retry／Cancel／Timeout、Usage／Cost Estimate。每個事件帶 `run_id`、Attempt、時間、序號、來源與遮罩狀態；格式定義在 `contracts/events/trace-event.schema.json`，Sandbox 經唯一入口 `POST /internal/trace/{token}` 回報，落地為按月分割表並以 cursor 分頁查詢。
   - 一般模式將事件彙整為可理解的進度與問題，摘要由固定規則產生（讀取 Trace 事件與 Run 狀態的 SQL fold），不經過模型呼叫；一旦改由模型產生摘要，就必須遵守模型輸出需標示為模型產出的規則。
   - 進階模式顯示經安全處理的結構化事件與 Log；UI 不直接顯示未過濾的 ANSI、HTML、SVG 或可執行內容；Trace 缺失或順序不確定時明確標示，不假裝完整。
3. **Evaluation 邊界**：Evaluation 可使用確定性規則與格式檢查、工具與程序 Exit Status、使用者定義驗收條件、預期輸出或資料比對、LLM Judge、使用者人工判斷；每個 Criterion Result 必須標示判斷來源、證據與信心，LLM Judge 結果不得描述為確定事實。Evaluation 使用低權限讀取介面，不取得 Sandbox 控制、MCP 憑證或一般平台管理工具，且它讀取的內容仍視為可能包含 Prompt Injection。判定信任邊界與重評機制的細節由評估判定主題定義。

Correlation 體系：

```text
request_id    單次 API 或使用者互動
run_id        完整 Run 的永久關聯
attempt_id    一次 Provider 執行嘗試
trace_id      分散式服務呼叫關聯
event_id      單一領域或 Run Trace 事件
```

不同 ID 不互相取代，但應能安全關聯；Log 不記錄 Prompt、Dataset 或 Secret 全文。

這三分不涵蓋使用者行為的漏斗量測——那既不是平台健康、也不在任何一次 Run 之內，屬於明確從屬的第五類資料，由產品分析主題定義。

### 決策 4：LLM 呼叫的觀測不外接第三方服務

不引入外部 LLM 觀測服務做 SDK 埋點、閘道回呼或 Prompt Management。否決的理由是治理紅線而非成本：這類回呼通常需要一組獨立於閘道之外的金鑰，而供應商金鑰只存在閘道是不可退讓的邊界；為封測規模的一個工程儀表板在閘道之外多開一個金鑰存放點，不划算。

模型呼叫的事實來源改指系統裡已經存在的三樣：

| 要回答的問題 | 由誰回答 |
| --- | --- |
| 這次呼叫花了多少、打的是哪個模型 | `cost_events`（平台支出的唯一帳） |
| 這個 Run 裡發生了什麼、順序如何 | Run Trace 分割表 |
| 這次改動有沒有讓品質退步 | `tools/eval-regression` 腳本，人工讀報告 |

放棄的是互動式的探索介面：把同一個提示的多次呼叫並排比較、在網頁上標註、把樣本存成 Dataset 反覆重放。接受這個放棄的條件是規模——封測規模的資料量撐不起這類工作流要求的樣本數；重新評估的訊號是質性的：當「同一個提示的多次呼叫要並排比較」成為每週都會做的常態動作。

## 影響

### 正面

- 短效模型憑證、Run 級成本歸因、供應商抽換、BYO Key 四項需求由 LiteLLM 一個元件落地。
- 使用者體驗、評估證據與平台維運各自有清楚的資料邊界，可依敏感度與成本訂不同保存期限；評估模組不需要高權限執行能力。
- 閘道之外不多一份金鑰存放點，供應商金鑰只存在閘道的邊界維持完整。
- 查「模型呼叫有沒有被記下來」的人會落在 `cost_events` 與 Run Trace 上：前者是平台支出的唯一帳、每一筆都在，是產品功能而非觀測附加物；後者已有保存期限與遮罩規則。

### 成本與限制

- 閘道新增一個部署單元且位於推理關鍵路徑，需要健康監控與容量規劃；閘道目前是模型呼叫的單點，多副本部署是現成緩解。
- 同一次 Run 會產生 Run Trace、Evaluation 等多套關聯資料，需要一致的 Correlation 與 Schema 版本治理；詳細 Trace 的成本需要分級與保存限制。
- 模型品質的跨版本比較目前只能「跑一次腳本、人讀一份報告」，沒有互動式介面。

## 待決策

- Run Trace 的詳細度分級尚未訂定：哪些欄位永遠記全文、哪些只在取樣或除錯時記。保存期限本身已有定值（`TRACE_RETENTION`），分級沒有——今天所有 Run 記一樣多。
