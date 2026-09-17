# ADR-003：Run 編排與非同步工作流程

- 狀態：Accepted
- 相關：[新 ADR-002 資料所有權與核心基礎設施](./ADR-002-data-ownership-and-core-infrastructure.md)、[新 ADR-004 Sandbox 隔離與執行安全](./ADR-004-sandbox-isolation-and-execution-security.md)、[新 ADR-005 模型閘道與可觀測性](./ADR-005-model-gateway-and-observability.md)、[新 ADR-009 評估判定與 Judge 信任邊界](./ADR-009-evaluation-verdicts-and-judge-trust.md)、[新 ADR-018 Aggregate 與領域事件](./ADR-018-aggregates-and-domain-events.md)、[新 ADR-023 帳號清除與 Credit](./ADR-023-account-purge-and-credit.md)、[ADR-024 外部系統的 Port 與 Adapter](./ADR-024-ports-and-adapters-for-external-systems.md)

## 背景

Skill Hub 需要在不綁死單一 Sandbox 供應商的前提下執行 Run，而 Skill 匯入、掃描、Run、評估、清理、打包、刪除這些流程都可能持續數秒到數分鐘、跨越資料庫、物件儲存、Provider 與外部服務——單一同步 HTTP Request 會造成逾時、不可恢復與難以追蹤的部分失敗。兩個問題共用同一個答案：一個不綁供應商的 Run 生命週期契約，加上以持久化狀態與事件驅動的非同步工作流程。

## 決策

### 決策 1：Run Orchestrator 只依賴 Provider Port，不依賴任一供應商的私有概念

Orchestrator 只依賴 Skill Hub 定義的 Provider Port 與標準 Run Contract；每個 Sandbox 實作以 Adapter 接入，Provider 專屬概念不提升為核心領域欄位。

Provider Port 是 Run 執行 context 裡的程式介面：查詢能力、建立 Attempt、查詢、取消、銷毀、列出仍存活的 sandbox。錯誤以領域分類回報（沒有空位、已不存在、拒絕、暫時不可用），Orchestrator 只依這些分類決定下一步。自建 Sandbox 的 HTTP 契約是其中一個 Adapter 的線路格式；不說這份契約的沙箱服務以另一個 Adapter 接入，Orchestrator 不變。外部系統 Port 與 Adapter 的通則見 ADR-024。

Provider 至少支援以下生命週期語意：

```text
validateCapabilities
→ provision
→ prepare
→ execute / streamEvents
→ cancel（選擇性發生）
→ collectArtifacts
→ destroy
```

`destroy` 必須具備冪等性，控制平面可以安全重複要求清理而不破壞其他 Run（鐵律 9）。

Run 狀態機：

```text
queued
→ provisioning
→ preparing
→ running
→ evaluating
→ succeeded | failed | cancelled | timed_out
→ cleaning_up
```

執行結果與清理結果分開記錄；即使使用者已取得結果，清理失敗仍是需要處理的系統事件。

Run Request 至少涵蓋：平台 `run_id` 與 Attempt、Skill Package Reference 與內容雜湊、Test Case Snapshot／Prompt／Dataset Reference、Agent／模型與 Runtime Profile、MCP／工具／Secret Reference、網路與資源（CPU、記憶體、磁碟、程序數、時間）政策、Trace Level、Artifact Policy 與資料保存政策。

標準輸出與事件至少涵蓋：Run 狀態與時間、Agent 最終輸出、Skill 啟用與資源載入事件、Tool Call／MCP Call／Script Log 與錯誤、Token／延遲／資源／成本計量、Artifact Manifest、安全與政策事件；Provider Diagnostics 可保存為擴充欄位，但核心評估不得只依賴某一家 Provider 的私有欄位。

Provider 需宣告 Runtime 類型與版本、Agent／模型整合模式、MCP／工具／Script／Artifact 能力、網路與 Private Network 能力、CPU／記憶體／磁碟／最大時間、地區與資料駐留與隔離強度、GPU 或特殊硬體、可用性與成本模型。隔離以強度比對而不是產品名：生產只接受強隔離（使用者態核心或硬體虛擬化），弱隔離（共用主機核心的容器）只在開發部署接受，無隔離只在淨測試模式接受；Orchestrator 以 Run Requirements 與 Capability Matching 選擇 Provider，無相容 Provider 時在排入執行前回報可理解的原因。

容量是選擇的一部分，因為平台是多人共用有限的沙箱：Orchestrator 只把 Run 派給相容且回報有空位的 Provider，空位多的優先。Provider 以「沒有空位」拒絕時，那次派送沒有開始，換下一個相容 Provider 是選擇，不是改派。所有相容 Provider 都滿時，Run 留在 `queued` 等候：這不是失敗，不佔用重試次數，也不佔住 Worker（工作延後再取）。排隊有自己的上限（`SlotWaitLimit`，30 分鐘），超過以 `timed_out` 結束並寫明是排隊逾時；Run 的硬性時間上限從第一次被 Provider 接受時起算，排隊的時間不算在裡面。沒有空位時被拒的那次 Attempt 仍會留下紀錄，但只在「看到有空位、送出時已被搶走」的競爭下才會出現。

識別策略區分三層，歷史資料、評估與 URL 一律使用平台 ID：

```text
run_id              Skill Hub 永久識別碼
run_attempt_id      平台的一次執行嘗試
provider_run_id     Provider 臨時識別碼
```

失敗與重試：Provision、Execution、Event Delivery、Artifact Upload、Evaluation 與 Cleanup 分別分類；只有已知冪等且符合政策的動作才能自動重試，不允許無限制重試；不確定 Provider 是否已開始時，以相同 Attempt 的 Idempotency Key 查詢或重送。

Provider 在 Attempt 執行中失聯（持續一段時間查不到，或回報不認得這個 Attempt）時，這次 Attempt 以「Provider 遺失」結束，那個 Provider 退出選擇，直到它再度回報健康；Run 改派一次到另一個相容的 Provider。改派只在下列四個條件都成立時才被允許，而這個系統的設計讓它們成立：

- 資料：新 Attempt 使用同一份不可變的 Skill Version 與 Test Case 快照，物件授權依新 Attempt 重新簽發，範圍不變。
- 權限：沿用使用者確認過的同一份權限摘要；Capability Matching 保證新 Provider 滿足同樣的隔離強度、出口政策與資源上限，並提供同一個 Runtime 版本。
- 成本：所有 Attempt 共用同一筆 Run 預算；新 Attempt 的模型預算是 Run 預算扣掉先前 Attempt 已記錄的花費，重跑不會讓一個 Run 花超過它的預算。
- 行為：沙箱唯一能對外產生的效果是經模型閘道的呼叫，重跑不會重複任何外部寫入；前一個 Attempt 事後才送回的 Artifact 落在它自己的授權範圍，不會混進新 Attempt。

每個 Run 最多改派一次，第二次遺失以 `provider_error` 結束。Run 記錄的 Provider 以最後被接受的 Attempt 為準，每個 Attempt 各自記錄自己的 Provider；遺失的 Provider 上殘留的 sandbox 由孤兒掃描回收。

驗證方式：Fake Provider 需通過完整生命週期契約測試；SelfHostedProvider 與其他 Provider 實作共用同一組核心測試；替換 Provider 時不修改 Skill、Test Case、Evaluation 的核心 Schema。

理由：Skill Hub 初期使用自建 Sandbox，但已確認未來必須可由使用者或平台選擇其他 Sandbox 實作。若 Run Model 直接使用第一個 Provider 的 API、狀態與檔案格式，後續更換 Provider 將需要重寫核心系統；統一的 Provider Port 讓 Run、Trace、Evaluation 與歷史資料不綁定供應商，也讓不同 Provider 之間可以建立共同契約測試與健康比較。最小公分母會隱藏 Provider 特有能力，需要可控 Extension 機制；Capability Matching 與狀態轉譯也增加設計成本，且不同 Provider 仍可能產生行為差異，不能宣稱完全一致。

### 決策 2：長時間或跨邊界流程一律走可持久化非同步工作流，核心狀態先落地再發事件

所有長時間或跨邊界流程（Skill 匯入、掃描、Run、評估、清理、打包）採可持久化的非同步工作流：核心狀態先寫入領域資料庫，再透過 Transactional Outbox 發出事件；Consumer 必須支援至少一次傳遞下的冪等處理（鐵律 9）。Outbox 與佇列共用同一個 PostgreSQL（見新 ADR-002），佇列以 River 實作，Go Worker 是唯一的佇列消費者（鐵律 7）。

典型的事件序列（現行實作以 [contracts/events/domain-events.md](../../contracts/events/domain-events.md) 的事件目錄為準）：

- Skill Ingestion：`ImportRequested → SourceFetched → PackageQuarantined → PackageValidated → TrustEvidenceRecorded → SkillVersionCreated → SearchProjectionUpdated`
- Run：`RunRequested → PolicyApproved → ProviderSelected → RunProvisioned → RunStarted → RunExecutionCompleted → EvaluationCompleted → CleanupCompleted`
- Packaging：`PackageRequested → VersionValidated → LicenseChecked → SecretsScanCompleted → PackageCreated → DownloadReady`

帳號刪除等合規性清除流程有自己的時序與一致性要求，不套用此處的事件鏈模型，見新 ADR-023。

每個領域事件至少包含 `event_id`、`event_type`、`event_version`、`occurred_at`、`correlation_id`、`causation_id`、`workspace_id`（適用時）、`aggregate_id`，以及不含 Secrets 的必要 Payload；Run 相關事件使用平台 `run_id` 作為主要 Correlation，不以 Provider ID 取代。

一致性與冪等規則：領域狀態與 Outbox Event 使用同一資料庫交易；Consumer 以 `event_id` 或業務 Idempotency Key 去重；狀態轉移以預期前置狀態或版本檢查防止倒退；外部呼叫保存 Request Key 與結果，避免不確定重試建立重複資源；Poison Message 進入隔離佇列並告警，不無限制重送。

事件的送達與消化：Outbox publisher 發布後由 `outbox.Dispatcher` 依事件類型路由到訂閱者所屬 context 自己的 Mailbox（一條 River 佇列，以事件識別去重）；Worker 從 Mailbox 取出事件、呼叫訂閱者的消化方法、存回。每一種事件都要有訂閱者或具名的忽略理由。Aggregate root 內部如何定義命令與記錄事件見新 ADR-018；本決策只規範跨 context 的傳遞機制。

命令與事件的差異：命令表示希望某個擁有者執行動作（例如 `StartRun`），事件表示已發生的事實（例如 `RunStarted`）；Event Consumer 不應依賴可變的隱含順序，需要順序時使用 Aggregate Version；使用者可見進度由持久化工作流狀態產生，不直接依賴暫時性 Queue 訊息。

即時更新：Web UI 可透過 Server-Sent Events、WebSocket 或輪詢接收進度，但即時通道只是傳遞方式；頁面重連後應能從控制平面查詢目前狀態與已保存事件，不依賴未保存的記憶體訊息。

理由：Skill 匯入、掃描、Run、評估、清理、打包、刪除都可能持續數秒到數分鐘且跨越多個系統邊界，同步請求模型無法承受逾時與部分失敗；持久化狀態加上 Outbox 事件讓長時間流程可恢復、可取消、可追蹤、可重試，也讓模組之間降低同步耦合，為未來拆分 Worker 或服務留出空間。代價是最終一致性下 UI 需呈現處理中狀態，且需要事件版本、去重、Outbox、Reconciler 與 Dead-letter 處理；事件不能取代清楚的領域 API 與資料所有權。

### 決策 3：Run 終態與 Evaluation 判定分屬兩個問題、兩個欄位、兩個表

`runs.status` 回答「這次執行發生了什麼」；`evaluations.overall` 回答「任務達成了嗎」。Evaluation 的判定不回寫 `runs.status`，也不回寫 `runs.failure_class`；Run 狀態機的 `evaluating → succeeded` 路徑不變，評估是這條路徑上的一個步驟，不是第二個狀態機。判定值域與 Judge 信任邊界見新 ADR-009。

落地要求：`evaluating → succeeded` 路徑不變，評估寫入 `evaluations`，不 UPDATE `runs` 的任何欄位；沒有評估的 Run 顯示「未評估」而非「通過」；評估未完成（`evaluations.status = failed`）與「未評估」分開顯示；Run 終態文案採執行語意（執行完成／執行失敗），任務判定另起一列顯示。

理由 1（失敗分類不被汙染）：`runs.failure_class` 區分 `provider_error`（平台問題，可重試）與 `workload_error`（Skill 問題，不重試），是重試決策的依據；若「輸出不符驗收條件」也變成 `failed`，等於把一個不該重試、也不是故障的結果塞進重試分類器，重試只會用同樣的 Skill 版本跑出同樣不符合的輸出。

理由 2（資料庫已經回答過一次）：Run 終態不可變（鐵律 4；`runs_terminal_immutable` trigger 禁止改寫已終態的 Run），而 Evaluation 的重評是 append-only、可在 rubric 或 Judge prompt 升版後對同一個 Run 產生新的判定；若終態由評估決定，該 Run 的終態就得跟著變，直接與不可變 trigger 衝突。

理由 3（執行事實與判斷分開）：Run Trace 是執行事實，Evaluation 是判斷；把判斷寫回執行事實等於抹除這條邊界，也讓「判斷來源」這個欄位失去落點（見新 ADR-005）。

## 影響

### 正面

- 可逐步加入第三方、自建、區域或高安全 Provider，Run、Trace、Evaluation 與歷史資料不綁定供應商。
- 長時間流程可恢復、取消、追蹤與重試；模組之間降低同步耦合，支援未來拆分 Worker 或服務。
- 重試分類器只處理它該處理的事，「執行成功但任務沒完成」變成一個說得出口的狀態，不必靠使用者自己讀 Trace 才發現落差。
- 重評（見新 ADR-009）與歷史 Run 的不可變性可以同時成立。

### 成本與限制

- 最小公分母的 Provider Port 可能隱藏特有能力，需要可控 Extension 機制；不同 Provider 仍可能有行為差異，不能宣稱完全一致。
- Provider 遺失後改派，使用者要多等一次派送；前一個 Attempt 已花掉的模型費用不會退回，只是總額仍受同一筆 Run 預算限制。
- 最終一致性下 UI 需呈現處理中狀態；需要事件版本、去重、Outbox、Reconciler 與 Dead-letter 處理。
- UI 需同時顯示執行結果與任務判定兩個狀態，資訊密度上升；對外部消費者而言，「Run 成功」不再是可單獨判斷結果的欄位，必須一併讀 evaluation。
