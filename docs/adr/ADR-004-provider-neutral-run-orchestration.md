# ADR-004：採用 Provider-neutral Run Orchestration

- 狀態：Accepted
- 日期：2026-08-13
- 決策者：產品負責人、架構規劃

## 背景

Skill Hub 初期使用自建 Cloud Sandbox，但已確認未來必須可由使用者或平台選擇其他 Sandbox 實作。若 Run Model 直接使用第一個 Provider 的 API、狀態與檔案格式，後續更換 Provider 將需要重寫核心系統。

## 決策

Run Orchestrator 只依賴 Skill Hub 定義的 Provider Port 與標準 Run Contract。每個 Sandbox 實作以 Adapter 接入，不將 Provider 專屬概念提升為核心領域欄位。

## 標準生命週期

Provider 至少支援以下語意：

```text
validateCapabilities
→ provision
→ prepare
→ execute / streamEvents
→ cancel（選擇性發生）
→ collectArtifacts
→ destroy
```

`destroy` 必須具備冪等性。控制平面可以安全重複要求清理，而不破壞其他 Run。

## Run 狀態

```text
queued
→ provisioning
→ preparing
→ running
→ evaluating
→ succeeded | failed | cancelled | timed_out
→ cleaning_up
```

執行結果與清理結果分開記錄。即使使用者已取得結果，清理失敗仍是需要處理的系統事件。

## Run Request 核心內容

- 平台 `run_id` 與 Attempt。
- Skill Package Reference 與內容雜湊。
- Test Case Snapshot、Prompt 與 Dataset Reference。
- Agent／模型與 Runtime Profile。
- MCP、工具與 Secret Reference。
- 網路、CPU、記憶體、磁碟、程序數與時間政策。
- Trace Level、Artifact Policy 與資料保存政策。

## 標準輸出與事件

- Run 狀態與時間。
- Agent 最終輸出。
- Skill 啟用與資源載入事件。
- Tool Call、MCP Call、Script Log 與錯誤。
- Token、延遲、資源與成本計量。
- Artifact Manifest。
- 安全與政策事件。
- Provider Diagnostics 擴充欄位。

Provider Diagnostics 可以保存，但核心評估不得只依賴某一家 Provider 的私有欄位。

## Provider Capability

Provider 需宣告：

- Runtime 類型與版本。
- Agent／模型整合模式。
- MCP、工具、Script 與 Artifact 能力。
- 網路及 Private Network 能力。
- CPU、記憶體、磁碟與最大時間。
- 地區、資料駐留與隔離等級。
- GPU 或特殊硬體。
- 可用性與成本模型。

Orchestrator 使用 Run Requirements 與 Capability Matching 選擇 Provider。若無相容 Provider，應在排入執行前回報可理解的原因。

## 識別策略

```text
run_id              Skill Hub 永久識別碼
run_attempt_id      平台的一次執行嘗試
provider_run_id     Provider 臨時識別碼
```

歷史資料、評估與 URL 使用平台 ID，不使用 Provider ID 作為永久主鍵。

## 失敗與重試

- Provision、Execution、Event Delivery、Artifact Upload、Evaluation 與 Cleanup 分別分類。
- 只有已知冪等且符合政策的動作才能自動重試。
- 不允許無限制重試。
- 不確定 Provider 是否已開始時，以相同 Attempt 的 Idempotency Key 查詢或重送。
- Provider 不可用不應自動改派另一 Provider，除非資料、權限、成本與行為差異已被政策允許。

## 影響

### 正面

- 可逐步加入第三方、自建、區域或高安全 Provider。
- Run、Trace、Evaluation 與歷史資料不綁定供應商。
- 可以建立共同契約測試與 Provider 健康比較。

### 成本與限制

- 最小公分母可能隱藏 Provider 特有能力，需要可控 Extension 機制。
- Capability Matching 與狀態轉譯增加設計成本。
- 不同 Provider 仍可能產生行為差異，不能宣稱完全一致。

## 驗證方式

- 使用 Fake Provider 通過完整生命週期契約測試。
- SelfHostedProvider 與 LocalRunnerProvider 使用同一組核心測試。
- 替換 Provider 時不修改 Skill、Test Case、Evaluation 的核心 Schema。

