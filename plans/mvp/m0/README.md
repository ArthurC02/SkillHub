# M0:產品與安全基線 — 執行計畫

- 日期:2026-08-13(起)／**2026-08-14 更新**
- 狀態:**M0 完結:PDM-001~003 與 ADR-013／015／018 已定案(ADR-014 Superseded),進入 M1。** 負責人於 2026-08-14 批准 M0 全部產出並指示開工
- 定案內容:**PDM-001／002／003 依 [pdm-proposals.md](./pdm-proposals.md) v5 定案**(首批類別 `documents`／`writing`／`data`;白名單來源與九項精選檢查表;Runtime＝Claude Agent SDK TS on Node.js 22 LTS ＋ OpenAI 模型分層),`03-work-items.md` 的 PDM-001／002／003／011 已勾選;**ADR-013 → Accepted**(依 PDM-011 Spike,補記四項實證調整);**ADR-015 → Accepted**(gVisor 基線與獨立 VM 池不變,逃逸測試轉為實作期驗收關卡);**新增 ADR-018 核心基礎設施容器化自架 → Accepted**,**ADR-014 → Superseded**
- 產出背景:提案已至 v5;三個 Spike 均以真實 API 實測(意圖搜尋含真 embedding、LiteLLM 閘道 7/7、Skill 載入路徑 6/6),**並在模型供應商定案為 OpenAI 後於正式後端上完成四項補測**(自主觸發 0/9、`skills` 白名單過濾有效、prompt caching 缺欄但計費準確;`thinking` 透傳與跨供應商 fallback 因單一供應商不再適用)
- 進入 M1 前的剩餘待辦:(1) **§9 回寫對照表的數值寫入 `plans/mvp/02`**;(2) **PostgreSQL 備份還原演練**(E1 單實例架構可被接受的前提);(3) 提案 Monorepo 目錄結構與 CI/CD;(4) PDM-004~010 仍未逐項定案(提案完整,不阻擋 M1 起步但阻擋對應實作項)
- 供應商註記:負責人 2026-08-14 定案**模型供應商採 OpenAI API**(經 LiteLLM 閘道)。**ADR-017 架構與實作鐵律 8 不變**——改變的只是閘道背後的後端,不構成新的架構決策,故未新增 ADR
- 對應里程碑:M0(見 [01-goals-and-plan.md](../01-goals-and-plan.md) 第 10 節)

## 目的

M0 的產出是**文件、決策提案與 Spike**,不含產品程式碼。目標是解除 AGENTS.md 列出的三個開工阻塞項:

| 阻塞項 | 交付物 | 定案者 |
| --- | --- | --- |
| PDM-001~003 產品決策 | [pdm-proposals.md](./pdm-proposals.md)(提案 v5) | 負責人 |
| PDM-003 定案前置:LiteLLM 閘道相容性 ＋ Skill 載入路徑 ＋ **§11 補測** | [pdm-003-litellm-spike-report.md](./pdm-003-litellm-spike-report.md)＋`spikes/pdm-003-litellm-gateway/` | 負責人(依結果定案 PDM-003) |
| **PDM-001/002 定案前置:`data` 類別供給查核** | [data-category-sourcing.md](./data-category-sourcing.md) | 負責人(依結果定案 PDM-001／002 與 PDM-004 白名單增補) |
| 部署平台成本試算(ADR-014/015 定案條件) | [cost-estimation.md](./cost-estimation.md)(v2 ＋ §6.2.3 v3 模型成本重估) | 負責人(依試算定案 ADR) |
| PDM-011 意圖搜尋品質 Spike(ADR-013 定案條件) | [pdm-011-spike-report.md](./pdm-011-spike-report.md)＋`spikes/pdm-011-intent-search/` | 負責人(依結果定案 ADR-013) |

另含 M0 安全基線:

| 需求 ID | 交付物 |
| --- | --- |
| SEC-001 威脅模型、SEC-002 Sandbox 最低安全基線 | [threat-model-and-sandbox-baseline.md](./threat-model-and-sandbox-baseline.md) |

## 工作分解

1. **PDM 決策提案**:PDM-001~006、008、010 各給選項比較與明確推薦,標「待負責人定案」。
2. **成本試算**:依 ADR-014/015 候選平台,以封閉測試(20 人/日 50 Run)與早期成長(200 人/日 500 Run)兩情境估算月費。
3. **威脅模型與 Sandbox 基線**:STRIDE 式威脅模型(含後 MVP 的 Local Runner)＋可驗收的 Sandbox 阻擋條件清單。
4. **PDM-011 Spike**:以真實公開 Skill 樣本驗證 BM25＋向量＋RRF 混合檢索與「符合原因」生成的可行性,量測 Top-1/Top-3 命中率。

## 完成後的下一步(離開 M0 的條件)

1. ~~負責人審閱 pdm-proposals.md,定案 PDM-001~003(至少這三項)。~~ **已完成(2026-08-14)**
2. ~~負責人依 cost-estimation.md 將 ADR-014、ADR-015 由 Proposed 改 Accepted(或退回調整)。~~ **已完成(2026-08-14)**:ADR-015 → Accepted;ADR-014 的容器化方向以**新增 ADR-018 取代**處理(§7.4 決策問題 4 的答案),ADR-014 標 Superseded
3. ~~負責人依 spike 報告將 ADR-013 定案,結果回寫 M1 搜尋作法。~~ **已完成(2026-08-14)**:四項實證調整已補記於 ADR-013「定案紀錄」,待決策三項全部回填或移交
4. **待辦**:提案 Monorepo 目錄結構與 CI/CD(新 ADR),才進入 M1 實作。

## 檔案地圖

> **2026-08-16 加註**:本節是導覽加法,**不修訂任何結論**——上文各節的定案內容未動。

| 檔案 | 類型 | 一句話用途 | 狀態 |
| --- | --- | --- | --- |
| [`README.md`](README.md)(本檔) | 計畫 | M0 的計畫、定案紀錄與本目錄導覽 | 凍結 |
| [`pdm-proposals.md`](pdm-proposals.md) | 治理 | PDM-001~011 產品決策提案 v5,PDM-001／002／003 依此定案;§9 為回寫 `02` 的對照表 | 凍結 |
| [`data-category-sourcing.md`](data-category-sourcing.md) | 報告 | PDM-001／002 定案前置:`data` 類別的供給查核(來源、授權、九項精選檢查表逐一比對) | 凍結 |
| [`cost-estimation.md`](cost-estimation.md) | 報告 | ADR-014／015 定案前置:兩情境的部署平台月費試算(v2 ＋ §6.2.3 模型成本 v3 重估) | 凍結 |
| [`pdm-003-litellm-spike-report.md`](pdm-003-litellm-spike-report.md) | 報告 | PDM-003 定案前置:LiteLLM 閘道相容性與 Skill 載入路徑實測(spike code 在 `spikes/pdm-003-litellm-gateway/`) | 凍結 |
| [`pdm-011-spike-report.md`](pdm-011-spike-report.md) | 報告 | ADR-013 定案前置:混合檢索與「符合原因」生成的可行性實測(spike code 在 `spikes/pdm-011-intent-search/`) | 凍結 |
| [`threat-model-and-sandbox-baseline.md`](threat-model-and-sandbox-baseline.md) | 治理 | SEC-001 威脅模型(32 條)與 SEC-002 Sandbox 最低安全基線(45 項可驗收阻擋條件) | 凍結(結論)。**唯一例外**:§3 的出口目的地依 ADR-022 Q3 的 CI 斷言([`.github/workflows/egress-allowlist.yml`](../../../.github/workflows/egress-allowlist.yml))必須隨 `infra/egress/` 變更於同一個 PR 更新 |

## 備註

- 本目錄文件為 M0 工作產出;`plans/mvp/03-work-items.md` 的勾選僅在完全符合允收準則且決策定案後更新。
- **2026-08-14 起本目錄文件視為已定案的依據來源,不再修訂結論**;後續變更走新 ADR 或新版本文件。
- ADR 的定案動作已於 2026-08-14 依負責人授權執行(狀態變更、待決策回填、新增 ADR-018);既有決策內容未被原地改寫,依 AGENTS.md 文件維護規則保留為決策歷史。
