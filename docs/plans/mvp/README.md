# Skill Hub MVP 規劃

## 文件目的

本目錄是 Skill Hub MVP 的規劃基準，記錄目前已確認的產品方向、功能規格、允收準則與可追蹤工作項目。開發、設計與測試若對範圍有不同理解，應先回到這組文件確認，並透過文件變更保留決策紀錄。

## 產品定位

> Skill Hub 是 Agent Skill 的搜尋引擎與試驗室，協助個人創作者找到、試過、改善並下載可攜的 Skill。

核心產品流程：

```text
描述意圖
→ 探索候選 Skill
→ 查看、比較與 Fork
→ 使用 Prompt 與測試資料試跑
→ 查看 Trace 與評估報告
→ 改善並重新試跑
→ 打包下載
```

## 已確認的產品決策

- 主要使用者為個人創作者。
- Skill 以開放、通用的 Agent Skills 規格為核心，不綁定單一 Agent。
- MVP 的第一入口是探索既有 Skill，而不是從零撰寫。
- Skill 來源初期可包含人工精選內容、已索引來源與外部網路結果。
- MVP 試跑支援 User Prompt 與測試資料；遠端 MCP 與 Local Runner 為既定架構方向，但移出 MVP 首發，依封閉測試需求訊號啟動。
- 初期 Cloud Sandbox 可以自建，但 Run Orchestrator 必須透過可抽換的 Sandbox Provider 介面執行。
- 執行結果必須包含可觀察的 Run Trace、驗收結果與具體改善建議。
- 最終輸出是符合 Agent Skills 規格、保留來源與授權資訊的可下載套件。

## 文件索引

1. [目標與計畫內容](01-goals-and-plan.md)
2. [規格與允收準則](02-specifications-and-acceptance-criteria.md)
3. [工作項目列表](03-work-items.md)
4. [待辦與移交：目前還缺什麼、誰在等誰](04-backlog-and-handoffs.md)（**活文件**；殘項三類清單＋跨里程碑待辦）
5. [架構決策紀錄](../../adr/README.md)
6. [M0 執行計畫與產出](m0/README.md)（決策提案、成本試算、威脅模型、Spike 報告）
7. [M1 執行計畫與產出](m1/README.md)（內容策展、目錄重建、審校報告、閘門測試材料）
8. [M2 執行計畫與產出](m2/README.md)（Skill Lab 交付摘要、基準試跑、工作項對帳）
9. [M3 執行計畫與產出](m3/README.md)（評估與改善：範圍、批次分解與裁定、評估管線設計、契約增量清單、[逐項對帳](m3/audit.md)、[Judge 判準回歸報告](m3/report-judge-regression.md)）
10. [M4 執行計畫與產出](m4/README.md)（打包與封閉測試：範圍、六條接點與甲類對應、**程式能完成 vs 負責人動作的分界**、批次分解、未決點；[打包管線設計](m4/packaging-design.md)、[封閉測試設計](m4/beta-design.md)、[契約增量清單](m4/contract-deltas.md)、[PDM-009 封測提案](m4/pdm-009-beta-proposal.md)（待追認）、[逐項對帳](m4/audit.md)、[封測上線前檢查表](m4/release-checklist.md)）——**程式面已收斂；封測待部署期與負責人動作**

跨里程碑仍在被引用的主題目錄（依 [ADR-024](../../adr/ADR-024-top-level-repository-layout.md)，不屬於任何 `mX/`）：[`content/`](content/)（策展資料與 writing rubric）、[`governance/`](governance/)（授權備忘與上游詢問信草稿）、[`gate-test/`](gate-test/)（M1 驗證閘門材料，**另含閘門與封測共用的[受測者同意書與資料保存政策](gate-test/consent-and-data-policy.md)——草稿，待法務確認**）。

**目錄骨架（M3 起適用，既有檔名不回溯改）**：每個 `mX/` 固定為 `README.md`（計畫＋狀態＋檔案地圖）、`audit.md`（逐項對帳）、`report-*`（報告）；目錄內檔名不重複 `mX` 前綴。

## 文件維護規則

- 已定案內容直接寫入「已確認決策」或正式規格。
- 尚未定案內容使用「待決策」或「規劃假設」標記。
- 新增功能前，先補上需求 ID 與允收準則，再加入工作項目。
- 工作完成時，將 `- [ ]` 改為 `- [x]`；不得只因已開始或部分完成就標記完成。
- 若 MVP 範圍改變，應同步更新三份文件，避免規格與工作清單失去一致性。
