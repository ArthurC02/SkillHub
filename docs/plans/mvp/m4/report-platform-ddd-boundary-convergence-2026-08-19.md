# Platform DDD 邊界收斂報告（2026-08-19）

- 類型：凍結報告
- 狀態：已收斂；後續架構決策與現行路徑以 ADR 為準，不以本報告新增工作。

## 結論

DDD-001～DDD-060 已完成。Platform 維持 modular monolith：11 個 Bounded Context 以 stable Boundary ID 治理，`shared/skillpkg` 是唯一 Shared Kernel；`foundation/*`、generated persistence 與 API entrypoint 是 Generic／組裝角色，不是隱藏的領域 Context。

現行的權威來源是：

- [Platform Bounded Context 與 Context Map](../../../adr/README.md#platform-bounded-context-與-context-map)：依賴方向與 composition 規則；Context Map 見 platform-context-map.md。
- [Query 與寫入所有權](../../../adr/README.md#query-與寫入所有權)：query ownership 與 read/write 強制。
- [Query 與寫入所有權](../../../adr/README.md#query-與寫入所有權)：跨 Context 寫入的依賴反轉。
- [Platform Bounded Context 與 Context Map](../../../adr/README.md#platform-bounded-context-與-context-map)：產品價值流、Shared Kernel、Foundation 與 Entrypoint 拓撲。

## 已收斂的範圍

- Context Map、depguard、query ownership 與 raw-SQL tripwire 均以 stable Boundary ID 而非目錄第一層推定。
- `allow:` 與 `read_allow:` 均為空；跨 Context 只交換 owner DTO、窄 callback 或 caller transaction，不傳 generated persistence carrier。
- Context 已依產品價值流遷至 `creator/`、`skill/`、`trial/`、`product/`；Shared Kernel、Foundation、Entrypoint 亦已完成其角色性收納。
- 交易、Workspace scope、不可變版本、Transactional Outbox 與 owner facts 的既有語意在搬遷中維持不變。

逐項 DDD-001～060 的當時證據與實作理由屬 Git history；不要重新建立可漂移的 active ledger。

## Test Lab 公開契約面（凍結）

- 不新增 `testlab` 的公開子 package；跨 Context 的 public face 是 owner Service 與 DTO，`run`、`eval`、`packaging` 不得直接讀取 persistence。
- Test Case draft 是可編輯的試跑情境；建立 Run 時由 `testlab` 產生不可變 snapshot，後續執行與評估只使用該快照。
- 對 UI 的現行 HTTP 契約以 [`contracts/openapi/public.yaml`](../../../../contracts/openapi/public.yaml) 為準；不得以過程設計稿作為契約來源。
- MCP Profile、Test Case 跨 Skill 複製與 criteria 批次確認均非 MVP 範圍。

## 驗證紀錄

收斂批次曾通過 Platform Go tests、`devctl automation-check`、OpenAPI/sqlc generated drift check、真實 PostgreSQL integration tests 與 `git diff --check`。日後變更須依[Platform Bounded Context 與 Context Map](../../../adr/README.md#platform-bounded-context-與-context-map)與[Query 與寫入所有權](../../../adr/README.md#query-與寫入所有權)的現行規則重跑相應檢查，而非將本報告當作一次性通行證。

## 尚未由本報告宣稱完成的事

本報告只證明 DDD 邊界收斂。部署期真實環境驗收、PDM／法務追認、封測與技術債殘項仍以 [04-backlog-and-handoffs.md](../../04-backlog-and-handoffs.md) 與 [release-checklist.md](release-checklist.md) 為準。
