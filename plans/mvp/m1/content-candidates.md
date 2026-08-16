# CONTENT-003：首批 Skill 候選清單

> **本文件已由 [curated-skill-list.md](./curated-skill-list.md)（2026-08-15）升級為正式清單。**
> 本文件保留為推導過程（類別邊界原則、`data` 類歸屬說明、缺口盤點的原始論證），**不再更新數量與 Tier**。三處已被正式清單推翻的內容：
> 1. §2「`writing` 只有 2 個確認候選」——正式清單已補到 10 個已索引／5 個精選。
> 2. §3.1 註記「A-5／A-6 待 PDM-004 定案 `lxml`」——`lxml` 已於 2026-08-14 隨 PDM-002 一併採納，條件解除。
> 3. §4「`documents` 可達成精選數 0」——正式清單依類別邊界原則自 `YuYY2004/excel-skills` 取得 2 個 OSI 授權精選候選，另加 1 個，合計 3（仍短少 1）。

- 狀態：候選盤點（M1 起步產出）。**尚未執行** CONTENT-004（來源／License 逐一人工確認）、CONTENT-006（規格與靜態掃描）、CONTENT-007／008（範例資料、Prompt 與基準試跑）——依 AGENTS.md 文件維護規則，下表的 Tier 欄一律是**建議值**，`03-work-items.md` 的 CONTENT-003 只在對應 `plans/mvp/m0` 三個來源文件的候選數量與 PDM-002 目標核對一致後才可勾選。
- 依據：[PDM-001](../m0/pdm-proposals.md#1-pdm-001mvp-首批三個-skill-類別)（首批類別 `documents`／`writing`／`data`）、[PDM-002](../m0/pdm-proposals.md#2-pdm-002首批-skill-來源清單與精選標準)（白名單來源、九項精選檢查表、回溯准入流程）、[data-category-sourcing.md](../m0/data-category-sourcing.md)（`data` 類別實查結果，25 候選／7 來源 repo）。
- 目標數量（PDM-002）：每類別 4–6 精選（curated）、8–12 已索引（indexed，含精選）。
- 類別邊界原則（PDM-001 §「data 的來源品質梯度」風險緩解）：去重／篩選／驗證／合併／拆分屬**整理** → `data`；建立與格式化屬**產出** → `documents`。因此 `anthropics/skills` 的 `xlsx`（建立試算表）歸 `documents`，`YuYY2004/excel-skills` 的 12 個 `excel-*`（去重／篩選／合併等）歸 `data`。

---

## 1. `documents`（文件與試算表產出）

**現況：只有 4 個確認候選，全部 source-available，未達 8–12 已索引目標。** 這是 PDM-001 風險表已記錄的已知缺口（見下方「待補足」），不是本次盤點遺漏。

| # | Skill | 來源 repo | License 狀態 | 建議 Tier | 備註 |
| --- | --- | --- | --- | --- | --- |
| 1 | `docx` | [anthropics/skills](https://github.com/anthropics/skills) | Source-available（Anthropic 服務條款） | indexed | 依 PDM-002 風險表統一政策：可索引、可平台試跑，**不產出 Download Artifact** |
| 2 | `pdf` | anthropics/skills | Source-available | indexed | 同上；部分具資料抽取性質，但依「建立與格式化屬產出」歸 `documents` |
| 3 | `pptx` | anthropics/skills | Source-available | indexed | 同上 |
| 4 | `xlsx` | anthropics/skills | Source-available | indexed | 依類別邊界原則歸 `documents`（建立／格式化），非 `data` |

**待補足**：PDM-001 風險表要求「額外找 2–3 個 OSI 授權的替代品作為可下載精選」——本次盤點沿用 M0 既有查核結果，尚未執行針對 `documents` 的回溯准入流程（PDM-002 步驟）去找這 2–3 個候選。這是 CONTENT-003 收尾前必須補做的動作，不在本文件範圍內完成。

---

## 2. `writing`（內容寫作與品牌一致性）

**現況：只有 2 個確認候選，全部 Apache-2.0，未達 8–12 已索引目標。**

| # | Skill | 來源 repo | License 狀態 | 建議 Tier | 備註 |
| --- | --- | --- | --- | --- | --- |
| 1 | `brand-guidelines` | [anthropics/skills](https://github.com/anthropics/skills) | Apache-2.0（`LICENSE.txt` 存在） | curated 候選 | 需先過 CONTENT-004／006／007／008 才能真正標 curated |
| 2 | `internal-comms` | anthropics/skills | Apache-2.0 | curated 候選 | 同上 |

**已排除**：`doc-coauthoring`（anthropics/skills）——目錄無 `LICENSE.txt`，repo 根目錄亦無可繼承的 License 檔（[data-category-sourcing.md §1 發現 A](../m0/data-category-sourcing.md#發現-areadmemd-的免責條款適用全-repo)）。依精選標準第 1 項與「License 狀態預設未知」規則，暫不可索引為可下載/可精選候選，僅可標 `external` 供發現用。

**待補足**：同 `documents`，需另外執行回溯准入流程找足供給，本文件未執行。

---

## 3. `data`（資料整理與分析）

**現況：25 個確認候選、橫跨 7 個來源 repo，超過 8–12 目標。** 完整清單、License、依賴與否決紀錄見 [data-category-sourcing.md §4](../m0/data-category-sourcing.md#4-候選白名單建議)；本節只摘錄可直接動作的部分。

### 3.1 建議首批精選（curated，6 個）

依「依賴零缺口 → 作者可辨識 → 驗收確定性最高」排序（data-category-sourcing.md §5）：

| # | Skill | 來源 repo | License | 依賴 | 備註 |
| --- | --- | --- | --- | --- | --- |
| 1 | `data-analyst` | [nqumich/data-analyst-skill](https://github.com/nqumich/data-analyst-skill) | MIT | `pandas`、`openpyxl`（全在白名單） | 203 行 Script，落在 300 行上限內；`SKILL.md` 為簡體中文，CONTENT-005 白話摘要需重寫 |
| 2 | `data-cleanliness-scan` | [danielrosehill/Claude-Data-Wrangler-plugin](https://github.com/danielrosehill/Claude-Data-Wrangler-plugin) | MIT | prompt-only | 零依賴風險 |
| 3 | `csv-to-json` | danielrosehill/Claude-Data-Wrangler-plugin | MIT | prompt-only | 輸出可做 schema 斷言，驗收確定性最高 |
| 4 | `text-to-numeric` | danielrosehill/Claude-Data-Wrangler-plugin | MIT | prompt-only | 同上 |
| 5 | `excel-deduplicate` | [YuYY2004/excel-skills](https://github.com/YuYY2004/excel-skills) | MIT | `pandas`、`openpyxl`、**`lxml`（需 PDM-004 白名單先加入）** | 直接對應 PDM-001 典型任務句「清掉重複列」；**條件候選，待 PDM-004 定案 `lxml` 後才可入選** |
| 6 | `excel-find-duplicates` | YuYY2004/excel-skills | MIT | 同上，需 `lxml` | 同上，條件候選 |

前 4 個不需要任何 PDM-004 白名單異動即可成立，是 `data` 類別的無條件下限；後 2 個依賴 `lxml` 加入白名單（PDM-002 定案時的併同條件之一，尚待負責人在 `pdm-proposals.md` 正式回填）。

### 3.2 其餘已索引候選（19 個，indexed）

- `YuYY2004/excel-skills`（MIT，需 `lxml`）另有 10 個：`excel-filter`、`excel-validate`、`excel-merge`、`excel-split`、`excel-sort`、`excel-regex-clean`、`excel-scout`、`excel-delete`、`excel-mapping-replace`、`excel-date-to-text`。
- `danielrosehill/Claude-Data-Wrangler-plugin`（MIT，prompt-only）另有 9 個：`standardise-country-names`、`unicode-consistency`、`date-wrangling`、`json-restructure`、`data-shape`、`data-comparability`、`add-data-dictionary`、`pii-flag`、`add-iso3166`。⚠️ 同 repo 另有 7 個需外網的 Skill（`hf-dataset-push`、`vector-upsert`、`sql-load`、`api-loader`、`graph-database`、`database-guide`、`enrich-with-currency`），**不列入本清單，匯入時需逐一排除**。

### 3.3 條件候選（需負責人裁量是否納入白名單，見 data-category-sourcing.md §4.2）

`data-analysis-skills`（cabbage2000-lab，`SKILL.md` 教模型在依賴缺失時自行 `pip install`，牴觸精選標準第 6 項）、`cohort-analysis`／`ab-test-analysis`（phuryn/pm-skills，25,234★，需 `matplotlib`／`scipy` 不在白名單）、`business-analysis-skill`（AbdoBasyioni，Script 約 500 行超過上限）。**不計入 25 個確認候選數。**

---

## 4. 彙總與下一步

| 類別 | 已索引目標 | 確認候選數 | 精選目標 | 可達成精選數 | 缺口 |
| --- | --- | --- | --- | --- | --- |
| `documents` | 8–12 | 4 | 4–6 | 0（全數 source-available，不可打包，仍可 curated=索引+試跑通過但不可下載） | 需再找 2–3 個 OSI 授權替代品（PDM-001 風險表已記錄） |
| `writing` | 8–12 | 2 | 4–6 | 2（待 CONTENT-004/006/007/008） | 需擴大回溯准入流程 |
| `data` | 8–12 | 25 | 4–6 | 6（見 §3.1） | 供給充足；`lxml` 白名單異動未定案會使 2 個精選候選卡住 |

**CONTENT-003 收尾前必須完成的動作（不在本文件範圍內）：**

1. 對 `documents`／`writing` 執行 PDM-002 回溯准入流程，補足 8–12 已索引目標（目前分別短少至少 4／6 個）。
2. 負責人在 `pdm-proposals.md` 正式定案 PDM-004 白名單加入 `lxml`（必要）與 `matplotlib`（建議），否則 §3.1 的 #5、#6 無法真正匯入。
3. 交給 CONTENT-004（來源／License 人工確認）與 CONTENT-006（規格／靜態掃描）逐一過檢查表，通過後才能把本文件的「建議 Tier」轉為 [`catalog.Tier`](../../../services/platform/internal/catalog/tier.go) 的實際值。
