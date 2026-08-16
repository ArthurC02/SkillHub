# ADR-003：依資料特性劃分所有權、儲存與生命週期

- 狀態：Accepted
- 日期：2026-08-13
- 決策者：產品負責人、架構規劃

## 背景

Skill Hub 同時包含關聯式領域資料、不可變套件、使用者 Dataset、大量 Trace、搜尋投影與 Secrets。若全部放在同一資料庫，會造成容量、查詢、安全與生命週期衝突；若過早使用多種資料產品，又會增加 MVP 維運成本。

## 決策

採多模型儲存，但以最少必要產品起步。不同資料依用途由明確模組擁有，派生資料可重建，Secrets 與一般資料分離。

## 資料分類與建議儲存

| 資料 | 事實來源 | 主要擁有者 | 特性 |
| --- | --- | --- | --- |
| User、Workspace、Skill metadata、Run 狀態 | 關聯式資料庫 | 對應領域模組 | 交易、一致性、查詢 |
| Skill Package、Dataset、Artifact | 物件儲存 | Registry／Test Lab／Packaging | 大型 Binary、不可變、生命週期政策 |
| Skill Search Document | 搜尋索引或資料庫搜尋投影 | Catalog | 可重建、讀取最佳化 |
| Run Trace | 結構化事件儲存 | Run／Trace | 追加寫入、大量、時間排序 |
| API Key、MCP Credential | Secrets Store | Policy／Integration | 加密、短效授權、輪替 |
| 平台 Logs／Metrics／Traces | O11y 平台 | Platform | 維運用途，不作領域事實來源 |
| Usage Record | 關聯式資料庫或分析投影 | Policy & Usage | 不可變計量事件 |

## 事實來源原則

- 原始 Skill Package、內容雜湊與不可變 Skill Version 是 Skill 的事實來源。
- 搜尋摘要、Embedding、分類與排序特徵是可重建投影。
- Run 定義與狀態由控制平面保存；Provider 的狀態不是唯一事實來源。
- Trace 與 Artifact 不得反向修改歷史 Test Case 或 Skill Version。
- Secrets Store 只保存密鑰材料；領域資料庫只保存 Secret Reference 與必要 metadata。

## 不可變快照

每次 Run 必須固定引用：

- `skill_version_id`
- `test_case_snapshot_id`
- Agent／模型與版本
- Runtime Profile
- Provider 與 Capability Snapshot
- 權限與網路政策 Snapshot
- Dataset 內容雜湊

後續修改 Skill、Test Case 或 Provider 設定不得改變歷史 Run。

## 物件存取

- Sandbox 使用短效、單一物件或單一 Run 範圍的存取權。
- 執行平面不得取得列舉整個 Bucket 的權限。
- Artifact 上傳先進入隔離區，通過大小、類型與安全檢查後才可供下載。
- 下載使用短效授權，不公開永久物件 URL。

## Workspace 邊界

所有使用者擁有的資料至少包含 `workspace_id` 或能由父層不可歧義地導出 Workspace。Repository 與查詢預設要求 Workspace 範圍，不接受由 UI 單獨保證隔離。

## 資料生命週期

需分別定義：

- 原始 Skill 與版本保存政策。
- Dataset 預設保存期限與立即刪除行為。
- Run Trace 的明細及彙總保存期限。
- Artifact 的到期與下載政策。
- Secrets 撤銷與刪除。
- Audit Event 與 Usage Record 的法規或營運保存需求。

MVP 尚未確定具體天數；實作前必須將保存期限轉成 Policy 設定與可測試需求。

### 刪除與可追溯性

使用者刪除 Dataset 或 Run 輸入後：

- 歷史 Run 保留內容雜湊、metadata 與 Trace 引用，並標示「輸入已刪除」。
- Run 維持可追溯（能證明當時用了什麼），但不再保證可重現（無法重新執行）。
- UI 與評估報告不得在輸入已刪除時暗示仍可重跑比較。

## 一致性策略

- 單一模組內的重要狀態使用資料庫交易。
- 資料變更與對外事件使用 Transactional Outbox。
- 搜尋索引與讀取投影接受最終一致性，UI 顯示必要處理狀態。
- 刪除跨越物件儲存、索引與 Trace 時使用可追蹤工作流程，不宣稱瞬間完成。

## 影響

### 正面

- 大型物件、搜尋、Secrets 與交易資料使用合適儲存模型。
- 可重建投影降低供應商綁定。
- 支援歷史可重現與使用者資料刪除。

### 成本與限制

- 需要處理跨儲存刪除、重建與一致性狀態。
- 資料庫備份不再等同完整系統備份。
- 必須定義 Trace 與 Artifact 成本治理政策。

## 待決策

- 關聯式資料庫、物件儲存與 Secrets 的實際產品選型。→ [ADR-014](./ADR-014-core-infrastructure-selection.md)
- Trace 初期使用關聯式分割表、物件事件檔或專用事件儲存。→ [ADR-014](./ADR-014-core-infrastructure-selection.md)（分割表＋大型 Payload 進物件儲存）
- 搜尋初期使用資料庫全文／向量能力或獨立搜尋引擎。→ [ADR-013](./ADR-013-intent-search-architecture.md)（Postgres FTS＋pgvector）

