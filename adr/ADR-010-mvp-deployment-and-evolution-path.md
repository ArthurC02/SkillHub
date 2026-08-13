# ADR-010：MVP 採少量部署單元，依壓力逐步拆分

- 狀態：Accepted
- 日期：2026-08-13
- 決策者：產品負責人、架構規劃

## 背景

Skill Hub 未來可能需要多區域、企業治理、多 Provider 與大量 Run，但 MVP 首先要驗證搜尋、試跑與下載價值。提前建立大量微服務會增加 CI/CD、網路、監控、資料一致性與開發協調成本。

另一方面，不受信任執行平面不能因 MVP 簡化而與控制平面混合。

## 決策

MVP 控制平面採模組化單體與少量 Worker；執行平面維持獨立部署及安全區域。服務拆分必須由明確壓力觸發，而非為了預測未來。

## MVP 建議部署單元

1. Web UI。
2. Control Plane API（模組化單體）。
3. Background Worker（匯入、索引、評估、打包與刪除）。
4. Run Orchestrator Worker。
5. Self-hosted Sandbox Worker／Execution Nodes。
6. Local Runner Client（後 MVP，依需求訊號啟動）。
7. 關聯式資料庫。
8. 物件儲存。
9. Queue／Event Transport。
10. Search Capability。
11. Secrets Store。
12. Observability Stack。
13. Python LLM 服務（[ADR-016](./ADR-016-language-and-framework-selection.md)）。
14. LiteLLM Model Gateway（[ADR-017](./ADR-017-model-gateway-and-llm-observability.md)）。

部署單元不等同領域模組；多個模組可以存在同一程序，但依賴與資料所有權仍需受約束。

## 環境

- Local Development：可用 Fake Provider 或受限本機 Sandbox，不降低生產安全假設。
- Integration：驗證資料庫、物件、Queue、Provider Contract 與外部整合。
- Staging：與生產相同安全區域及政策，使用非生產 Secrets 和資料。
- Production：控制與執行區隔離，具備用量限制、告警及緊急停用能力。

## 可用性與擴展

- Web/API 以無狀態方式水平擴展，狀態保存於受管資料層。
- Worker 依 Queue 深度與工作類型分別擴展。
- Sandbox Worker 依 Runtime、區域、安全等級與容量池分組。
- 搜尋投影可重建，故不作核心交易單點。
- Provider 不可用時以清楚狀態阻擋或依政策路由，不靜默改變環境。

## 服務拆分觸發條件

| 壓力 | 優先拆分候選 |
| --- | --- |
| Run 數量、部署頻率或安全責任獨立 | Run Orchestrator |
| 外部 Skill 數量與匯入負載增長 | Ingestion／Trust Worker |
| 搜尋規模與查詢演進速度增長 | Catalog & Search |
| 模型評估成本與併發增長 | Evaluation Worker／Service |
| 多區域與資料駐留 | Regional Execution Plane |
| 企業政策、稽核與計費複雜 | Policy、Usage、Billing |
| 多 Provider 路由與採購治理 | Provider Router |

拆分前需確認：獨立擴展、獨立部署、安全邊界、資料所有權或團隊責任至少一項有實際需求。

## 災難恢復與備份

- 關聯式資料庫與物件儲存需有獨立備份／版本政策。
- 搜尋索引及部分讀取投影可由事實來源重建。
- Queue 不是永久事實來源；工作流狀態需持久化。
- Provider 執行環境為暫時性，不納入備份。
- Secrets 需要獨立備份、輪替與撤銷程序。

RPO、RTO 與多區域策略在 MVP 容量與商業要求確認後另立 ADR。

## 影響

### 正面

- 控制 MVP 複雜度與交付成本。
- 保持明確安全邊界與未來拆分接縫。
- 技術和組織可依真實壓力演進。

### 成本與限制

- 模組化單體需要自動化依賴規則，否則容易退化成大泥球。
- Background Worker 初期共用部署時，需避免單一工作類型耗盡全部資源。
- 部分企業能力要等實際需求後才完整設計。

## 待決策

- MVP 的部署平台、網路拓撲與容量估算。
- CI/CD、環境晉升、資料 Migration 與 Rollback 策略。
- SLO、RPO、RTO 與緊急變更流程。

