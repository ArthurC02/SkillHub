# M0:產品與安全基線 — 執行計畫

- 日期:2026-08-13(起)／**2026-08-14 更新**
- 狀態:**產出完成,所有技術確認已完結,待負責人定案。** 提案已至 v5;三個 Spike 均以真實 API 實測(意圖搜尋含真 embedding、LiteLLM 閘道 7/7、Skill 載入路徑 6/6),**並在模型供應商定案為 OpenAI 後於正式後端上完成四項補測**(自主觸發 0/9、`skills` 白名單過濾有效、prompt caching 缺欄但計費準確;`thinking` 透傳與跨供應商 fallback 因單一供應商不再適用)
- 定案就緒摘要:**PDM-001、PDM-002、PDM-003 三項提案完整,可定案**(`data` 類別供給缺口已由查核解除,25 個候選 / 7 個 repo);**ADR-013 可定案**(意圖搜尋 Spike 結論已回寫);**ADR-014／ADR-015 待負責人依成本試算批示**(§7.4 四個決策問題,含容器化方向與 ADR-014 的處理方式)。**已無高嚴重度未解項**
- 方向註記:負責人已指示平台元件走雲原生容器化(Postgres 等以容器自架);此方向與 ADR-014 Proposed 內文的「最小受管組合」取向不同,定案時需決定修訂 ADR-014 或以新 ADR-018 取代
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

1. 負責人審閱 pdm-proposals.md,定案 PDM-001~003(至少這三項)。
2. 負責人依 cost-estimation.md 將 ADR-014、ADR-015 由 Proposed 改 Accepted(或退回調整)。
3. 負責人依 spike 報告將 ADR-013 定案,結果回寫 M1 搜尋作法。
4. 定案後:提案 Monorepo 目錄結構與 CI/CD(新 ADR,自 ADR-018 起編),才進入 M1 實作。

## 備註

- 本目錄文件為 M0 工作產出;`plans/mvp/03-work-items.md` 的勾選僅在完全符合允收準則且決策定案後更新。
- ADR 未在此階段被修改;定案動作由負責人執行或明確授權後執行。
