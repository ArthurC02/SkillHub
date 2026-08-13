# ADR-008：長時間流程採非同步工作流與領域事件

- 狀態：Accepted
- 日期：2026-08-13
- 決策者：產品負責人、架構規劃

## 背景

Skill 匯入、掃描、Run、評估、清理、打包與刪除皆可能持續數秒到數分鐘，且跨越資料庫、物件儲存、Provider 與外部服務。使用單一同步 HTTP Request 會造成逾時、不可恢復與難以追蹤的部分失敗。

## 決策

所有長時間或跨邊界流程採可持久化的非同步工作流。核心狀態先寫入領域資料庫，再透過 Transactional Outbox 發出事件。Consumer 必須支援至少一次傳遞下的冪等處理。

## 工作流類型

### Skill Ingestion Workflow

```text
ImportRequested
→ SourceFetched
→ PackageQuarantined
→ PackageValidated
→ TrustEvidenceRecorded
→ SkillVersionCreated
→ SearchProjectionUpdated
```

### Run Workflow

```text
RunRequested
→ PolicyApproved
→ ProviderSelected
→ RunProvisioned
→ RunStarted
→ RunExecutionCompleted
→ EvaluationCompleted
→ CleanupCompleted
```

### Packaging Workflow

```text
PackageRequested
→ VersionValidated
→ LicenseChecked
→ SecretsScanCompleted
→ PackageCreated
→ DownloadReady
```

### Deletion Workflow

```text
DeletionRequested
→ AccessRevoked
→ ObjectsDeleted
→ SearchProjectionRemoved
→ TracePolicyApplied
→ DeletionCompleted
```

## 事件信封

每個領域事件至少包含：

- `event_id`
- `event_type`
- `event_version`
- `occurred_at`
- `correlation_id`
- `causation_id`
- `workspace_id`（適用時）
- `aggregate_id`
- 不含 Secrets 的必要 Payload

Run 相關事件使用平台 `run_id` 作為主要 Correlation，不以 Provider ID 取代。

## 一致性與冪等

- 領域狀態與 Outbox Event 使用同一資料庫交易。
- Consumer 以 `event_id` 或業務 Idempotency Key 去重。
- 狀態轉移以預期前置狀態或版本檢查防止倒退。
- 外部呼叫保存 Request Key 與結果，避免不確定重試建立重複資源。
- Poison Message 進入隔離佇列並告警，不無限制重送。

## 事件與命令的差異

- 命令表示希望某個擁有者執行動作，例如 `StartRun`。
- 事件表示已發生的事實，例如 `RunStarted`。
- Event Consumer 不應依賴可變的隱含順序；需要順序時使用 Aggregate Version。
- 使用者可見進度由持久化工作流狀態產生，不直接依賴暫時性 Queue 訊息。

## 即時更新

Web UI 可透過 Server-Sent Events、WebSocket 或輪詢接收進度，但即時通道只是傳遞方式。頁面重連後應能從控制平面查詢目前狀態與已保存事件，不依賴未保存的記憶體訊息。

## 影響

### 正面

- 長時間流程可恢復、取消、追蹤與重試。
- 模組之間降低同步耦合。
- 支援未來拆分 Worker 或服務。

### 成本與限制

- 採最終一致性後，UI 需呈現處理中狀態。
- 需要事件版本、去重、Outbox、Reconciler 與 Dead-letter 處理。
- 事件不能取代清楚的領域 API 與資料所有權。

## 待決策

- MVP Queue／Event Transport 的產品選型。→ [ADR-014](./ADR-014-core-infrastructure-selection.md)（Postgres 佇列，Outbox 同庫）
- Workflow 採自建狀態機或 Durable Workflow Engine。→ [ADR-014](./ADR-014-core-infrastructure-selection.md)（自建狀態機起步）
- Trace 高頻事件是否使用與領域事件不同的傳輸通道。→ [ADR-014](./ADR-014-core-infrastructure-selection.md)（批次寫入分割表，與領域事件分離）

