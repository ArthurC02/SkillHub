# Skill Hub 架構決策紀錄

這裡保存 Skill Hub 的架構決策。**一個主題一份，內容永遠是現行版本**：決策變了就改寫那一份，歷史由 git 保存，不留補記、日期與刪除線。出現新主題才新增一份，編號取最大號 + 1，刪除過的編號不再使用。

**只有這些 ADR 與本索引寫 ADR 編號或檔名。** 其他文件、程式、測試、設定與契約寫規則本身；需要讓讀者看見決策理由時，連到本頁的主題標題（`docs/adr/README.md#<主題>`）。里程碑當時的機器輸出、量測結果與第三方語料照原樣保存。這條由 `automation-check` 的 `adr-citations` 強制。

每份 ADR 的形狀是背景、決策、影響，以及仍未回答的待決策。

## 系統情境、平面與部署路徑

[ADR-001](./ADR-001-system-context-planes-and-deployment-path.md)｜系統邊界、控制平面與執行平面怎麼分，以及部署形態與演進路徑。

## 資料所有權與核心基礎設施

[ADR-002](./ADR-002-data-ownership-and-core-infrastructure.md)｜哪些資料歸誰、存在哪裡，以及資料庫、物件儲存與佇列的選擇。

## Run 編排與非同步工作流程

[ADR-003](./ADR-003-run-orchestration-and-async-workflows.md)｜Run 的狀態機、佇列，以及 Provider 中立的編排方式。

## Sandbox 隔離與執行安全

[ADR-004](./ADR-004-sandbox-isolation-and-execution-security.md)｜不受信任的內容在哪裡執行、用什麼隔離，以及安全驗收的定值。

## 模型閘道與可觀測性

[ADR-005](./ADR-005-model-gateway-and-observability.md)｜所有模型呼叫怎麼走閘道，以及 Trace 與 LLM 可觀測性的範圍。

## 身分、Workspace、准入與額度

[ADR-006](./ADR-006-identity-workspace-admission-and-allowances.md)｜登入與 Session、Workspace Scope、封測准入與配額的強制點。

## 打包、授權溯源與散布

[ADR-007](./ADR-007-packaging-license-provenance-and-redistribution.md)｜可攜套件的形狀與完整性、License 溯源，以及能不能再散布。

## 意圖搜尋

[ADR-008](./ADR-008-intent-search.md)｜Catalog 的混合檢索與 LLM 增強怎麼組合。

## 評估判定與 Judge 信任邊界

[ADR-009](./ADR-009-evaluation-verdicts-and-judge-trust.md)｜Evaluation 的判定與重評、證據壽命，以及 LLM Judge 能決定什麼。

## 產品分析與稽核邊界

[ADR-010](./ADR-010-product-analytics-and-audit-boundaries.md)｜分析事件、audit 與 Trace 各自記什麼、不記什麼。

## 從描述生成 Skill

[ADR-011](./ADR-011-generating-a-skill-from-a-description.md)｜從一段描述生成 Skill 的輸入、拒絕條件、成本與產出歸屬。

## 互動創作

[ADR-012](./ADR-012-interactive-creation.md)｜互動創作旅程的形狀，以及它與一次性生成的關係。

## Repo 結構、CI 與驗證層

[ADR-013](./ADR-013-repository-layout-ci-and-verification-tiers.md)｜頂層目錄的收納語意、CI 基線與驗證分層。

## 開發自動化與依賴治理

[ADR-014](./ADR-014-developer-automation-and-dependency-governance.md)｜自動化契約、共享工作樹的單一 Writer，以及依賴的准入與更新。

## ADR 管理

[ADR-015](./ADR-015-managing-adrs.md)｜這個目錄自己的規則：一個主題一份，誰可以寫 ADR 編號。

## Platform Bounded Context 與 Context Map

[ADR-016](./ADR-016-platform-bounded-contexts-and-context-map.md)｜context 怎麼劃分、跨 context 的協作方向，以及機械強制。

## Query 與寫入所有權

[ADR-017](./ADR-017-query-and-write-ownership.md)｜query 屬於哪個 context，跨 context 怎麼拿事實、怎麼寫入。

## Aggregate 與領域事件

[ADR-018](./ADR-018-aggregates-and-domain-events.md)｜aggregate 的邊界，以及它們之間用領域事件溝通的規則。

## 前端架構與樣式分層

[ADR-019](./ADR-019-frontend-architecture-and-styles.md)｜前端的架構分層、樣式分層與守衛。

## 設計系統、信任訊號與畫面用語

[ADR-020](./ADR-020-design-system-trust-signals-and-screen-words.md)｜兩把設計尺的定位、信任訊號的呈現，以及畫面用語的規範。

## 營運後台

[ADR-021](./ADR-021-operator-backoffice.md)｜Operator 看得到什麼、做得了什麼，以及它的邊界。

## 淨測試模式

[ADR-022](./ADR-022-clean-test-mode.md)｜在裝不了東西的機器上跑同一套產品程式的條件與代價。

## 帳號清除與 Credit

[ADR-023](./ADR-023-account-purge-and-credit.md)｜帳號清除涵蓋的範圍，以及 Credit 的計量與扣款。

## 外部系統的 Port 與 Adapter

[ADR-024](./ADR-024-ports-and-adapters-for-external-systems.md)｜外部系統怎麼接進來、領域擁有什麼，以及換掉一個外部系統時要動哪些地方。
