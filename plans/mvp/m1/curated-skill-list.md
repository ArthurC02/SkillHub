# CONTENT-003／004：首批 Skill 正式候選清單與 License 合規總表

- 狀態：**正式交付（CONTENT-003 候選清單、CONTENT-004 來源與 License 檢查）**。工作項目勾選留待負責人。
- 查核日：**2026-08-15**（本文件所有 URL、commit SHA、License 判定均以此日觀測為準）
- **2026-08-15 修訂（`documents` 精選補足）**：依 §7 缺口 1 的補足路徑實地量測後
  - **路徑 2（`the3ma/course-quiz-builder`）不通過**，④⑥ 兩項皆 fail，詳見 §6.2；
  - **路徑 1（`YuYY2004/excel-skills` 的 `excel-format`）通過**，原記錄的「全文約 1,600–1,800 行」為**量測錯誤**，實測 262 行／168 行內嵌 Python，**升為精選 D-4**（§2.1、§9）。
  - `documents` 精選由 3 補足為 **4**，達標。
- 查核方法：實地存取 GitHub REST API、`github.com` 頁面與 `raw.githubusercontent.com` 原始檔。**License 一律以實查 `LICENSE`／`LICENSE.txt` 檔案內容判定，不採信 README 或 repo metadata 的宣稱**——本次即因此揪出兩個「宣稱 MIT、實為 Anthropic source-available 衍生物」的 repo（見 §4.1）。
- 依據：
  - [PDM-001](../m0/pdm-proposals.md#1-pdm-001mvp-首批三個-skill-類別)（三類別 `documents`／`writing`／`data`，2026-08-14 定案）
  - [PDM-002](../m0/pdm-proposals.md#2-pdm-002首批-skill-來源清單與精選標準)（白名單制、回溯准入流程、九項精選檢查表、數量目標，2026-08-14 定案）
  - [PDM-004 §4](../m0/pdm-proposals.md)（Runtime 預裝套件白名單；`lxml` 必要、`matplotlib` 建議，**兩者已隨 PDM-002 於 2026-08-14 一併採納**）
  - [data-category-sourcing.md](../m0/data-category-sourcing.md)（`data` 類 25 候選與否決紀錄，2026-08-14）
- 前身文件：[content-candidates.md](./content-candidates.md)（M1 起步的候選盤點初稿）。**本文件為其正式版**，沿用其類別邊界原則與 `data` 類歸屬說明；初稿保留不刪，作為歸屬判斷的推導過程。

> **類別邊界原則（承自 content-candidates.md，PDM-002 定案時的建議切法）**
> 去重／篩選／驗證／合併／拆分／取代屬**整理** → `data`；**建立與格式化**屬**產出** → `documents`。
> 本次依此原則將 `YuYY2004/excel-skills` 新發現的 `excel-insert`（插入列欄＝建立）、`excel-format`、`excel-freeze`（格式化）歸入 `documents`——這是 `documents` 首次取得 **OSI 授權**的候選，直接緩解 PDM-001 風險表「`documents` 高品質 Skill 全部 source-available」的缺口。

---

## 1. 達標總覽

| 類別 | 精選目標 4–6 | 實際精選候選 | 判定 | 已索引目標 8–12 | 實際已索引 | 判定 |
| --- | --- | --- | --- | --- | --- | --- |
| `documents` | 4–6 | **4** | ✅（2026-08-15 補足，見 §9） | 8–12 | **10** | ✅ |
| `writing` | 4–6 | **5** | ✅ | 8–12 | **10** | ✅ |
| `data` | 4–6 | **6** | ✅ | 8–12 | **25** | ✅ 超額 |
| **合計** | 12–18 | **15** | ✅ 落在區間內 | 24–36 | **45** | ✅ |

**一句話結論：三類的「已索引」與「精選」數量目標全部達標。** `documents` 精選原短少 1 個，已由 `excel-format` 於 2026-08-15 補足（§9）；**剩下的實質風險不是數量而是 §5.2 的法務判定**——若 `anthropics/skills` 的 4 個 source-available Skill 連索引都不成立，`documents` 已索引由 10 掉到 6，跌破下限（§7 缺口 2）。

來源多樣性（PDM-011 golden query set 的跨 repo 抽樣前提）：**9 個獨立來源 repo**，較 M0 的 7 個增加 2 個，且 `documents`／`writing` 首次各自擁有 2 個以上獨立來源，不再單押 `anthropics/skills`。

---

## 2. 精選（curated）候選清單

### 2.1 `documents`

| # | Skill | 來源 repo | pin commit | SKILL.md 路徑 | License | 依賴 | 外網 | Script 規模 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| D-1 | `excel-insert` | [YuYY2004/excel-skills](https://github.com/YuYY2004/excel-skills) | `c15e51e20424284f98359e8d3512f8aaa771c62f` | `claude/skills/excel-insert/SKILL.md` | MIT（實查 `LICENSE`） | `openpyxl` ✅ | 不需 | 全文約 280–300 行，內嵌 Python 約 180 行 ✅ ≤300 |
| D-2 | `excel-freeze` | YuYY2004/excel-skills | 同上 | `claude/skills/excel-freeze/SKILL.md` | MIT | `openpyxl` ✅ | 不需 | 全文約 120 行，內嵌 Python 約 45 行 ✅ |
| D-3 | `handoff` | [ToolMonsters/handoff-skill](https://github.com/ToolMonsters/handoff-skill) | `fa70c91e44a5f36b374f3600ddca3c98814e6451` | `SKILL.md`（repo 根） | MIT（實查 `LICENSE`） | 無（prompt-only） | 不需 | **0 行** ✅ |
| **D-4** | `excel-format` | YuYY2004/excel-skills | 同 D-1 | `claude/skills/excel-format/SKILL.md` | MIT | `openpyxl`、`pandas`、`numpy` ✅ | 不需 | **全文 262 行，內嵌 Python 168 行** ✅ ≤300（2026-08-15 精確量測，見 §9） |

### 2.2 `writing`

| # | Skill | 來源 repo | pin commit | SKILL.md 路徑 | License | 依賴 | 外網 | Script 規模 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| W-1 | `brand-guidelines` | [anthropics/skills](https://github.com/anthropics/skills) | `f6656c1256d5a8adfa37db9110046ef20bac644c` | `skills/brand-guidelines/SKILL.md` | **Apache-2.0**（實查 `skills/brand-guidelines/LICENSE.txt` 首行為 Apache License 2.0） | 無 | 不需 | 0 行 |
| W-2 | `internal-comms` | anthropics/skills | 同上 | `skills/internal-comms/SKILL.md` | Apache-2.0 | 無 | 不需 | 0 行 |
| W-3 | `humanizer` | [blader/humanizer](https://github.com/blader/humanizer) | `523374dee72d67c7b2b5f858ea0094ffda49c3ac` | `SKILL.md`（repo 根） | MIT（實查 `LICENSE`；frontmatter 亦自帶 `license: MIT`） | 無 | 不需 | 1 個 `scripts/validate-package.py`，1,952 bytes，**打包驗證用、非執行路徑** |
| W-4 | `line-edit` | [cabbagecachekid/neon-jetpack](https://github.com/cabbagecachekid/neon-jetpack) | `4c0f62e17c00681705c481d9be600f5d4d1a660a` | `skills/line-edit/SKILL.md` | MIT（實查 `LICENSE`） | 無 | 不需 | 0 行 |
| W-5 | `ai-written-check` | cabbagecachekid/neon-jetpack | 同上 | `skills/ai-written-check/SKILL.md` | MIT | 無 | 不需 | 0 行 |

> `humanizer` 是三類精選中唯一的高星等來源（35,674★，2026-08-15 觀測），對沖了 PDM-001 風險表「社群 repo 上游消失風險」的一部分。

### 2.3 `data`

沿用 [data-category-sourcing.md §5](../m0/data-category-sourcing.md) 的排序（依賴零缺口 → 作者可辨識 → 驗收確定性），本次逐條複核仍有效：

| # | Skill | 來源 repo | pin commit | SKILL.md 路徑 | License | 依賴 | 外網 | Script 規模 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| A-1 | `data-analyst` | [nqumich/data-analyst-skill](https://github.com/nqumich/data-analyst-skill) | `0ba9d17ed275b341df713db6a10b44eb32bf6eb1` | `SKILL.md`（repo 根） | MIT（實查 `LICENSE`） | `pandas`、`openpyxl` ✅ | 不需 | `scripts/data_ops.py` 203 行 ✅ |
| A-2 | `data-cleanliness-scan` | [danielrosehill/Claude-Data-Wrangler-plugin](https://github.com/danielrosehill/Claude-Data-Wrangler-plugin) | `b12805a62307021e024626616119b505e015f5a9` | `skills/data-cleanliness-scan/SKILL.md` | MIT | prompt-only（`pandas`／`numpy`）✅ | 不需 | 0 行 |
| A-3 | `csv-to-json` | danielrosehill/Claude-Data-Wrangler-plugin | 同上 | `skills/csv-to-json/SKILL.md` | MIT | prompt-only ✅ | 不需 | 0 行 |
| A-4 | `text-to-numeric` | danielrosehill/Claude-Data-Wrangler-plugin | 同上 | `skills/text-to-numeric/SKILL.md` | MIT | prompt-only ✅ | 不需 | 0 行 |
| A-5 | `excel-deduplicate` | YuYY2004/excel-skills | `c15e51e2...` | `claude/skills/excel-deduplicate/SKILL.md` | MIT | `pandas`、`openpyxl`、`lxml` ✅（`lxml` **已於 2026-08-14 隨 PDM-002 納入白名單**） | 不需 | 全文約 180–190 行，內嵌 Python 約 130–140 行 ✅ |
| A-6 | `excel-find-duplicates` | YuYY2004/excel-skills | 同上 | `claude/skills/excel-find-duplicates/SKILL.md` | MIT | 同上 ✅ | 不需 | 單檔 ≤253 行 ✅ |

> **`lxml` 條件已解除。** content-candidates.md §3.1 把 A-5／A-6 標為「待 PDM-004 定案 `lxml` 後才可入選」；[pdm-proposals.md 定案紀錄](../m0/pdm-proposals.md)（2026-08-14）已載明「§4 的 `lxml`（必要）與 `matplotlib`（建議）白名單增補隨 PDM-002 一併採納」。**兩者為無條件精選候選，該註記作廢。**

---

## 3. 九項精選檢查矩陣

判定值：`pass`／`fail`／`pending`（需平台或後續工作項才能判定，標明承接的工作項）。

檢查項編號依 PDM-002：①License 明確且允許再散布 ②來源可追溯 ③規格驗證無阻擋錯誤 ④Script 可審閱（≤300 行、人工逐行、無 `eval`／動態下載／外連 `subprocess`）⑤無疑似 Secret ⑥不需外網／MCP、依賴在 Runtime Image 內 ⑦白話摘要 ⑧平台基準試跑通過 ⑨作者可辨識

| Skill | ① | ② | ③ | ④ | ⑤ | ⑥ | ⑦ | ⑧ | ⑨ |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| D-1 `excel-insert` | pass | pass | pass | **pending**¹ | pending² | pass | **pass**³ | pending⁴ | pass |
| D-2 `excel-freeze` | pass | pass | pass | **pending**¹ | pending² | pass | **pass**³ | pending⁴ | pass |
| D-3 `handoff` | pass | pass | pass | **pass**（無 Script） | pending² | pass | **pass**³ | pending⁴ | pass |
| **D-4** `excel-format` | pass | pass | pass | **pending**¹⁺⁶ | pending² | pass | **pass**³ | pending⁴ | pass |
| W-1 `brand-guidelines` | pass | pass | pass | pass（無 Script） | pending² | pass | **pass**³ | pending⁴ | pass |
| W-2 `internal-comms` | pass | pass | pass | pass（無 Script） | pending² | pass | **pass**³ | pending⁴ | pass |
| W-3 `humanizer` | pass | pass | pass | **pending**¹ | pending² | pass | **pass**³ | pending⁴ | pass |
| W-4 `line-edit` | pass | pass | pass | pass（無 Script） | pending² | pass | **pass**³ | pending⁴ | pass |
| W-5 `ai-written-check` | pass | pass | pass | pass（無 Script） | pending² | pass | **pass**³ | pending⁴ | pass |
| A-1 `data-analyst` | pass | pass | pass | **pending**¹ | pending² | pass | **pass**³ | pending⁴ | pass |
| A-2 `data-cleanliness-scan` | pass | pass | pass | pass（無 Script） | pending² | pass | **pass**³ | pending⁴ | pass |
| A-3 `csv-to-json` | pass | pass | pass | pass（無 Script） | pending² | pass | **pass**³ | pending⁴ | pass |
| A-4 `text-to-numeric` | pass | pass | pass | pass（無 Script） | pending² | pass | **pass**³ | pending⁴ | pass |
| A-5 `excel-deduplicate` | pass | pass | pass | **pending**¹⁺⁵ | pending² | pass | **pass**³ | pending⁴ | pass |
| A-6 `excel-find-duplicates` | pass | pass | pass | **pending**¹ | pending² | pass | **pass**³ | pending⁴ | pass |

腳註：

1. **④ 的行數已機械量測且全部落在 300 行上限內**，但**「人工逐行審過」一項尚未執行**（沿用 M0 的既有限制）。承接工作項：**CONTENT-006**。
2. **⑤ 靜態掃描與人工 Secret 確認尚未執行。** 承接工作項：**CONTENT-006**。
3. ~~**⑦ 白話摘要：工序已建立，人工審核未完成，故仍記 `pending`。**~~ **⑦ 於 2026-08-16 改記 `pass`。** 承接工作項：**CONTENT-005**（已完成）。
   **2026-08-16 審校結果**（[content-review-report.md](content-review-report.md)）：45 筆全量自動化審校，**45/45 通過**；精選 **15/15**。唯一主判準（非技術讀者可理解性）45/45，忠實性 890 條事實宣稱 0 條未支持，語言慣例與白名單皆 0 命中。首輪有 3 筆（含精選 `excel-format`、`internal-comms`）因把預設值寫成必填、為輸出加品質形容詞、能力外推而未通過，已由 `enrich-skill/v4` 加入三條轉述約束後重跑修正，**未下架任何一筆**。
   ⚠️ 一項判準範圍修正（KPI3 只掃 zh-Hant 欄位）影響 `excel-format` 的判定，**待負責人追認**；若不接受，該筆退回需修改、⑦ 一併退回 `pending`（報告 §9 第 2 條）。
   **2026-08-15 進度**：45 筆全部已有繁中白話摘要與雙語任務範例句（`gpt-5.6-sol`，全數走生產端點 `POST /v1/enrich-skill`；15 筆重用 golden set 既有產出、30 筆新呼叫）。產出見 [`tools/content/summaries.json`](../../../tools/content/summaries.json)，**可審核紀錄與審核工序見 [content-summaries.md](content-summaries.md)**（誰審、判準、否決條件、狀態欄位；45 筆現皆為「待審」）。
   - ⚠️ 原註「`data-analyst` 與 YuYY2004 系列需重寫為繁體中文」的處置已定案：**平台不改寫上游套件，摘要即繁中化呈現層**（content-summaries.md §1.3）。16 筆簡中來源的摘要與範例句已全數為繁體中文，機械掃描無簡體字殘留。
   - ⚠️ 原註「`anthropics/skills` 免責條款需納入措辭考量」的處置：**不納入摘要**，改由詳情頁 License／來源區塊承接——該句屬信任／品質陳述，ADR-013 白名單明令模型產出不得包含。**此解讀待負責人確認**（content-summaries.md §3）。
   - ⑦ 改記 `pass` 的條件：精選 15 筆全數審核為「通過」，且 content-summaries.md §7 的待辦 2（7 筆補 `limitations`）與待辦 3 結案。**現況：三項條件皆已滿足（2026-08-16）。**
4. **⑧ 平台基準試跑需要平台存在。** 承接工作項：**CONTENT-007（範例資料／Prompt／驗收條件）→ CONTENT-008（基準試跑）**。此項在隔離 Sandbox 內執行，符合鐵律 1。
5. **`excel-deduplicate` 的 SKILL.md 於依賴段落出現 `pip install` 字樣。** 三個套件（`pandas`／`lxml`／`openpyxl`）全部在 PDM-004 白名單內，該分支在 Runtime Image 中不會觸發；但**「SKILL.md 文字教模型執行被禁止的動作」與 M0 否決 `cabbage2000-lab/data-analysis-skills` 的理由同型**。CONTENT-006 需人工確認措辭是否可接受，或於匯入時標註。
6. **D-4 `excel-format` 的 ④ 機械項於 2026-08-15 全部通過**：262 行全文／168 行內嵌 Python（≤300）、無 `eval`／`exec`、無動態下載、無 `subprocess`、無 `pip install` 字樣（不同於腳註 5 的 A-5）。**與 D-1／D-2 同樣只差「人工逐行審過」**，故仍記 pending 而非 pass。另 SKILL.md 以 `[[excel-safe-workflow]]` wiki-link 引用同 repo 的另一個 Skill，該檔不在套件內；匯入實測未觸發阻擋錯誤（③ pass），但 CONTENT-006 應決定此類跨 Skill 引用的呈現方式。

> **九項全過者目前為 0**——這是預期狀態，不是缺陷：~~⑤⑦⑧~~ **⑤⑧** 兩項在平台建成前對**所有**候選（含官方來源）一律無法判定。**⑦ 已於 2026-08-16 由自動化審校判為 `pass`（15/15）**，故 CONTENT-003 的勾選現在只等 ⑤（CONTENT-006 的靜態掃描與人工 Secret 確認）。~~**在 CONTENT-005／006／007／008 全數完成前，`03-work-items.md` 的 CONTENT-003 不得勾選為完成**。~~
>
> **2026-08-15 修訂**（依據 [m1-work-items-audit.md §8 第三梯](m1-work-items-audit.md)，CONTENT-007／008 已改列 M2）：勾選條件改為 **CONTENT-005／006 全數完成**。⑧「平台基準試跑」不再是 CONTENT-003 的勾選前提，改以引用處理——勾選當下 ⑧ 須明確記為「待 M2 基準試跑，見 CONTENT-007／008」，**不得記為 pass**。⑤⑦ 的綁定不變（原文保留於刪除線，供回溯）。

---

## 4. 已索引（indexed）清單

已索引層**不要求九項全過**（PDM-002），只要求 License 狀態可判定、來源可追溯、不含明確阻擋項。精選候選同時計入已索引。

### 4.1 `documents`（10）

| # | Skill | 來源 repo | pin commit | License | Tier | 備註 |
| --- | --- | --- | --- | --- | --- | --- |
| 1 | `excel-insert` | YuYY2004/excel-skills | `c15e51e2` | MIT | **curated** | 見 §2.1 |
| 2 | `excel-freeze` | YuYY2004/excel-skills | `c15e51e2` | MIT | **curated** | 見 §2.1 |
| 3 | `handoff` | ToolMonsters/handoff-skill | `fa70c91e` | MIT | **curated** | 見 §2.1 |
| 4 | `excel-format` | YuYY2004/excel-skills | `c15e51e2` | MIT | **curated** | 見 §2.1 D-4。~~原記「全文約 1,600–1,800 行、④不過」~~ **該量測有誤，2026-08-15 實測 262 行／168 行內嵌 Python，④ 機械項通過**（§9） |
| 5 | `document-format-skills` | [KaguraNanaga/document-format-skills](https://github.com/KaguraNanaga/document-format-skills) | `cbdd11249b6d8925a2ca6ff83f3a82201023962c` | MIT（實查 `LICENSE`） | indexed | ❌ ④不過：`scripts/` 下 9 個 Python 檔，僅 `process.py` 即約 290 行，合計遠超上限。⚠️ 選用依賴 `pywin32`（Windows-only，`.doc`／`.wps` 轉檔）不在白名單，該路徑在 Runtime Image 內不可用；核心 `.docx` 路徑只需 `python-docx` ✅。中國國標 GB/T 9704-2012 導向 |
| 6 | `course-quiz-builder` | [the3ma/course-quiz-builder](https://github.com/the3ma/course-quiz-builder) | `e2ac75e52a9a8214b656d9da1a505a2c74116225` | MIT（實查 `LICENSE`） | indexed | Node.js、**零 npm 依賴**（僅 `node:` 內建），Node ≥18 ✅（Image 為 Node 22）。**2026-08-15 完成量測：④⑥ 兩項確定 fail，維持 indexed、不列精選**，詳見 §6.2 與 §9.2 |
| 7 | `docx` | anthropics/skills | `f6656c12` | **Source-available**（Anthropic 服務條款） | indexed | ❌ ①不過，見 §5.2 的升級告警 |
| 8 | `pdf` | anthropics/skills | `f6656c12` | Source-available | indexed | 同上 |
| 9 | `pptx` | anthropics/skills | `f6656c12` | Source-available | indexed | 同上 |
| 10 | `xlsx` | anthropics/skills | `f6656c12` | Source-available | indexed | 同上；依類別邊界原則歸 `documents`（建立／格式化），非 `data` |

### 4.2 `writing`（10）

| # | Skill | 來源 repo | pin commit | License | Tier |
| --- | --- | --- | --- | --- | --- |
| 1–2 | `brand-guidelines`、`internal-comms` | anthropics/skills | `f6656c12` | Apache-2.0 | **curated** |
| 3 | `humanizer` | blader/humanizer | `523374de` | MIT | **curated** |
| 4–5 | `line-edit`、`ai-written-check` | cabbagecachekid/neon-jetpack | `4c0f62e1` | MIT | **curated** |
| 6 | `cringe-check` | cabbagecachekid/neon-jetpack | `4c0f62e1` | MIT | indexed |
| 7 | `full-review` | cabbagecachekid/neon-jetpack | `4c0f62e1` | MIT | indexed |
| 8 | `copyright-creative-work` | cabbagecachekid/neon-jetpack | `4c0f62e1` | MIT | indexed；主題為著作權常識，**非法律意見**，UI 需比照 NFR-001 措辭紀律標註 |
| 9 | `sokrati` | [iamursky/sokrati](https://github.com/iamursky/sokrati) | `6255b82d5f25de5e8ef8ebd4bbe1fbeded0a1865` | MIT（實查根目錄 `license` 小寫檔，`Copyright (c) 2026 Ilya Evseev`） | indexed |
| 10 | `shorten` | iamursky/sokrati | `6255b82d` | MIT | indexed |

> **`sokrati` 的語意叢集警告**：該 repo 的 7 個 skill 中有 6 個（`abbrevia`／`abrege`／`abrevia`／`escurca`／`kuerzen`／`shorten`）是同一個「精簡文字」能力的多語版本。**本清單只納入 `sokrati` 與 `shorten` 兩個**，其餘 5 個刻意不索引——否則 PDM-011 的 golden query set 會因語意重複而失去 recall@5 鑑別力（PDM-001 風險表「`data` 候選語意過度集中」的同型風險，在 `writing` 也存在）。
>
> `neon-jetpack` 的 `ux-web-design-review` 與 `journey-map` 屬設計評審，不屬 `writing`，未納入；`using-neon-jetpack` 為 meta 說明，未納入。

### 4.3 `data`（25）

沿用 [data-category-sourcing.md §4.1](../m0/data-category-sourcing.md) 的 25 個合格候選，**本次逐條複核：3 個來源 repo 全部存在、未封存、License 未變更**（見 §5.1）。

| 群組 | Skill | 來源 repo | pin commit | License | Tier |
| --- | --- | --- | --- | --- | --- |
| 1–10 | `excel-deduplicate`※、`excel-find-duplicates`※、`excel-filter`、`excel-validate`、`excel-merge`、`excel-split`、`excel-sort`、`excel-regex-clean`、`excel-scout`、`excel-delete` | YuYY2004/excel-skills | `c15e51e2` | MIT | ※為 curated，其餘 indexed |
| 11–12 | `excel-mapping-replace`、`excel-date-to-text` | YuYY2004/excel-skills | `c15e51e2` | MIT | indexed |
| 13–24 | `data-cleanliness-scan`※、`csv-to-json`※、`text-to-numeric`※、`standardise-country-names`、`unicode-consistency`、`date-wrangling`、`json-restructure`、`data-shape`、`data-comparability`、`add-data-dictionary`、`pii-flag`、`add-iso3166` | danielrosehill/Claude-Data-Wrangler-plugin | `b12805a6` | MIT | ※為 curated，其餘 indexed |
| 25 | `data-analyst`※ | nqumich/data-analyst-skill | `0ba9d17e` | MIT | curated |

**同 repo 必須逐一排除、不可整包匯入者（danielrosehill，7 個需外網）**：`hf-dataset-push`、`vector-upsert`、`sql-load`、`api-loader`、`graph-database`、`database-guide`、`enrich-with-currency`。

---

## 5. License 合規總表（CONTENT-004）

### 5.1 入選 repo 的 License 判定

「可否再散布」＝ 是否允許 Skill Hub 產出 Download Artifact（`02:PACK-001`）。「衍生關係」欄依 `02:DISC-003`「任何 Skill Hub 修改後的版本都能追溯到原始來源及 Fork 關係」與 CONTENT-002 的呈現規則填寫。

| # | 來源 repo | pin commit | License（實查檔案） | 判定依據 | 可否再散布 | source-available 標記 | 衍生關係 | 涵蓋類別 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 1 | [anthropics/skills](https://github.com/anthropics/skills) — `brand-guidelines`、`internal-comms` | `f6656c1256d5a8adfa37db9110046ef20bac644c` | **Apache-2.0** | `skills/<name>/LICENSE.txt` 首行 `Apache License Version 2.0` | ✅ 可 | 否 | 原創（Anthropic, PBC） | `writing` |
| 2 | anthropics/skills — `docx`／`pdf`／`pptx`／`xlsx` | 同上 | **Source-available（Anthropic 服務條款）** | `skills/docx/LICENSE.txt`：「© 2025 Anthropic, PBC. All rights reserved.」＋ 使用受 Consumer／Commercial Terms 管轄 | ❌ **不可** | ✅ **是** | 原創（Anthropic, PBC） | `documents` |
| 3 | [YuYY2004/excel-skills](https://github.com/YuYY2004/excel-skills) | `c15e51e20424284f98359e8d3512f8aaa771c62f` | **MIT** | 根目錄 `LICENSE`；repo metadata `spdx_id: MIT` 一致 | ✅ 可 | 否 | 原創（單一作者） | `data`、`documents` |
| 4 | [danielrosehill/Claude-Data-Wrangler-plugin](https://github.com/danielrosehill/Claude-Data-Wrangler-plugin) | `b12805a62307021e024626616119b505e015f5a9` | **MIT** | 根目錄 `LICENSE`；metadata 一致 | ✅ 可 | 否 | 原創（Daniel Rosehill） | `data` |
| 5 | [nqumich/data-analyst-skill](https://github.com/nqumich/data-analyst-skill) | `0ba9d17ed275b341df713db6a10b44eb32bf6eb1` | **MIT** | 根目錄 `LICENSE`；metadata 一致 | ✅ 可 | 否 | 原創 | `data` |
| 6 | [blader/humanizer](https://github.com/blader/humanizer) | `523374dee72d67c7b2b5f858ea0094ffda49c3ac` | **MIT** | 根目錄 `LICENSE`；`SKILL.md` frontmatter 亦宣告 `license: MIT` | ✅ 可 | 否 | 內容方法論引用 Wikipedia〈Signs of AI writing〉（CC BY-SA 4.0 條目）作為**參考來源**，非程式碼衍生；**CONTENT-005 摘要應標註此引用** | `writing` |
| 7 | [cabbagecachekid/neon-jetpack](https://github.com/cabbagecachekid/neon-jetpack) | `4c0f62e17c00681705c481d9be600f5d4d1a660a` | **MIT** | 根目錄 `LICENSE`；metadata 一致 | ✅ 可 | 否 | 原創 | `writing` |
| 8 | [iamursky/sokrati](https://github.com/iamursky/sokrati) | `6255b82d5f25de5e8ef8ebd4bbe1fbeded0a1865` | **MIT** | 根目錄 **小寫 `license`** 檔，`Copyright (c) 2026 Ilya Evseev`。⚠️ 檔名非標準大寫，匯入腳本的 License 偵測需容忍大小寫 | ✅ 可 | 否 | 原創 | `writing` |
| 9 | [ToolMonsters/handoff-skill](https://github.com/ToolMonsters/handoff-skill) | `fa70c91e44a5f36b374f3600ddca3c98814e6451` | **MIT** | 根目錄 `LICENSE` | ✅ 可 | 否 | 原創 | `documents` |
| 10 | [KaguraNanaga/document-format-skills](https://github.com/KaguraNanaga/document-format-skills) | `cbdd11249b6d8925a2ca6ff83f3a82201023962c` | **MIT** | 根目錄 `LICENSE` | ✅ 可 | 否 | 原創 | `documents` |
| 11 | [the3ma/course-quiz-builder](https://github.com/the3ma/course-quiz-builder) | `e2ac75e52a9a8214b656d9da1a505a2c74116225` | **MIT** | 根目錄 `LICENSE`；`marketplace.json` 一致 | ✅ 可 | 否 | repo 內含 `docs/superpowers/`，疑似參考 [obra/superpowers](https://github.com/obra/superpowers)（MIT，授權相容）；**CONTENT-006 需確認歸屬標註是否完整** | `documents` |

**統計：9 個 repo 可再散布（MIT 8、Apache-2.0 1），1 個 repo 的 4 個 Skill 為 source-available、不可再散布。**

### 5.2 ⚠️ 升級告警：`anthropics/skills` 的 source-available 條款比 PDM-002 假設的更嚴格

PDM-002 風險表的統一政策為「**索引與平台內試跑照常（在平台上執行不等於再散布），但一律不產出任何 Download Artifact**」。

本次實查 `skills/docx/LICENSE.txt` 的**完整條款**後發現，該授權除禁止「distributing, sublicensing, or transferring these materials to any third party」外，**同時禁止「extracting, copying, creating derivative works, or reproducing the materials outside the Services」**。

**「reproducing … outside the Services」的字面範圍可能涵蓋「Skill Hub 在自己的資料庫與物件儲存中保存一份內容快照」**——而 INGEST-004／精選標準第 2 項（保存內容雜湊與快照）正是平台匯入的必要動作。若此解讀成立，**這 4 個 Skill 連「已索引」都不成立，只能停在「外部結果」層（僅存 URL 與 metadata，不存內容）**。

- **這是法律解釋問題，不是架構問題，本文件不作判定。**
- **2026-08-16 回填**：四種平台行為（索引／展示／沙箱試跑／打包下載，另加 LLM 增強產出）的逐項風險評估與處置提案已寫成 [m2/anthropic-sa-license-memo.md](../m2/anthropic-sa-license-memo.md)。**該備忘同樣不是法律意見、不作終判**，只把選項與代價攤開；終判仍在負責人與法務。
- **建議動作**：負責人與法務就以下兩點確認後回填 PDM-002 風險表：(a) 平台保存內容快照是否構成 "reproducing outside the Services"；(b) 若構成，`documents` 類別的 4 個官方 Skill 應降級為第三層「外部結果」（僅索引 metadata 與連結）。
- **對數量目標的影響**：若降級，`documents` 的已索引從 10 掉到 **6**，**低於 8–12 下限**，必須另補 2–6 個 OSI 候選。**精選 4 個（D-1～D-4）全部是 MIT，不受此判定影響**——2026-08-15 補足缺口 1 後，這條風險只剩在「已索引」層。§7 的補足路徑 3（`w95` 的 39 個自製 skill）因此保留為此判定的備援池。

### 5.3 衍生關係的頭號地雷：**「宣稱 MIT 的 Anthropic source-available 衍生物」**

本次查核最重要的單一發現。詳見 §6.1 的兩個 repo。**匯入腳本不可只讀 repo 根目錄的 `LICENSE` 就判定授權**——這兩個 repo 的根 `LICENSE` 都是合法的 MIT 檔案，錯的是「被 MIT 涵蓋的內容並非該作者所有」。

**可執行的偵測啟發式（供 CONTENT-006／INGEST 實作參考）**：README 或 INDEX 出現 "based on Anthropic's official skills"／"Anthropic Official"／"Anthropic ... Plugins" 等來源字樣，且 skill 目錄名與 `anthropics/skills` 的 `docx`／`pdf`／`pptx`／`xlsx` 重合時，**一律標為 License 狀態「未知」並轉人工**，不得因 repo 有 MIT 檔而自動升級。

---

## 6. 淘汰／否決清單

依 PDM-002 回溯准入流程第 4 步「記錄否決原因，不重複評估」。M0 已記錄的 14 組見 [data-category-sourcing.md §4.3](../m0/data-category-sourcing.md)，**不在此重複**；以下為本次（2026-08-15）新增。

### 6.1 授權衍生關係不成立（最高嚴重度）

| 候選 | 星等 | 宣稱 License | 實際問題 | 不過的檢查項 |
| --- | --- | --- | --- | --- |
| [appautomaton/document-SKILLs](https://github.com/appautomaton/document-SKILLs) | 145★ | 根目錄 `LICENSE` 為 MIT | README 明文：「MIT license, **based on Anthropic's official skills**」。其 `docx`／`pdf`／`pptx`／`xlsx` 四個目錄即 `anthropics/skills` 的 source-available 內容，**該授權明文禁止建立衍生作品與再散布**，作者無權以 MIT 轉授。**另**：系統依賴 `pandoc`／`poppler-utils`／`tesseract-ocr`／`qpdf`／`libreoffice`，且以 `uv` + PEP 723 在執行期解析依賴 | **①**（授權來源不成立）、**⑥**（系統依賴全不在 PDM-004 Image 內，且執行期安裝被禁止） |
| [w95/awesome-claude-corporate-skills](https://github.com/w95/awesome-claude-corporate-skills)（`78dbc7c7...`） | 156★ | 根目錄 `LICENSE` 為 MIT | `INDEX.md` 自述 166 個 skill 來自五個來源：Anthropic Financial Services Plugins 51、Anthropic Knowledge Work Plugins 41、自製 39、社群 27、**Anthropic Official 8**。其 `13-document-processing` 的 `docx`／`pdf`／`pptx`／`xlsx` 明確標為 "Anthropic Official"。**100/166 為 Anthropic 來源，整包以 MIT 再授權的基礎不成立** | **①** |

> **對 `w95` 的保留意見**：其中 39 個「自製」skill 的 MIT 宣稱本身可能成立，`07-Operations`（`sop-builder`、`project-status-report`、`incident-postmortem`）與 `03-HR`（`job-description-writer`、`employee-handbook-builder`）等與 `documents`／`writing` 高度相關。**但每一個都需逐一確認來源歸屬**，成本高且結果不確定。列為**條件候選，不計入本次數量**；若 §5.2 的法務判定導致 `documents` 供給塌陷，這是第一個應回頭挖掘的池子。

### 6.2 依賴或外網不過

| 候選 | 星等 | License | 否決原因 | 不過的檢查項 |
| --- | --- | --- | --- | --- |
| [inhouseseo/superseo-skills](https://github.com/inhouseseo/superseo-skills)（11 個 skill） | 254★ | Apache-2.0 ✅ | 實查 `write-content` 與 `improve-content` 的 `SKILL.md`：兩者的工作流第一步即「Google 主關鍵字並讀取前 5 名 SERP 結果」、`improve-content` 另需抓取目標 URL。**11 個 skill 全部建立在即時 SERP 研究之上**，與 ADR-005 egress default-deny 正面衝突 | **⑥** |
| [umarmsharif/ai-presentation-builder](https://github.com/umarmsharif/ai-presentation-builder) | 1★ | MIT ✅ | 依賴 npm 套件 `pptxgenjs`；PDM-004 白名單只涵蓋 Python 套件，Runtime Image 未預裝任何第三方 npm 套件，且執行期禁止 `npm install`。選用依賴另含 LibreOffice 與 8 種商業字型 | **⑥**（若負責人決定為 Node 建立套件白名單，可回頭複審——**這是本清單唯一因「白名單語言缺口」而非品質問題出局的候選**） |
| `data-to-document`（danielrosehill 同 repo，M0 未涵蓋） | — | MIT ✅ | 實查 `SKILL.md`：需 **Typst CLI** 在 PATH 上，另需 `pyyaml` ✅、選用 `babel` ✗ | **⑥** |
| [nexu-io/html-anything](https://github.com/nexu-io/html-anything) | 8,291★ | Apache-2.0 ✅ | 實查根目錄：pnpm monorepo（`cli`／`next`／`e2e`），**無 `SKILL.md`、無 `skills/` 目錄**，不是 Agent Skills 套件而是一套本機應用程式 | **③**（非 Agent Skills 規格產物） |
| `course-quiz-builder`（the3ma，**精選否決；indexed 維持**） | — | MIT ✅ | **⑥**：`publish-pages.mjs` 不是可選附件而是 SKILL.md「Pipeline」的**第 7 步**（"Skip only if the user asked for a local page"），需 GitHub API／Pages 外網（`fetch()`）＋ `execFileSync` 呼叫 `git`／`gh auth token` 與一枚 repo 寫入 Token，與 ADR-005 egress default-deny 正面衝突，**不可被排除**。（`browser-check.mjs` 反倒可排除——SKILL.md 標為 "Optionally"、無瀏覽器時 skips cleanly。）**④**：`publish-pages.mjs` **768 行**、`publish-selftest.mjs` 356 行、`selftest.mjs` **341 行**，三檔皆超 300 行上限；且 `selftest.mjs` 用 `new Function()` 動態求值從 HTML 模板切下的 CORE 區段，等同 `eval`，為 ④ 明文禁止項。**`selftest.mjs` 又是 SKILL.md 硬規則 2 的強制品質閘門**（"Never ship without selftest.mjs printing PASS"），同樣不可排除 | **④**、**⑥** |

### 6.3 License 缺失

| 候選 | 星等 | 否決原因 | 不過的檢查項 |
| --- | --- | --- | --- |
| `doc-coauthoring`（anthropics/skills） | — | 實查 `skills/doc-coauthoring/` 目錄**只有 `SKILL.md`，無 `LICENSE.txt`**；repo 根目錄亦無可繼承的 License 檔（metadata `license: null`）。**2026-08-15 複查結果與 M0 一致，狀況未改善** | **①**——不可精選、不可打包；僅可標為「外部」供發現用 | 
| [PaodingAI/skills](https://github.com/PaodingAI/skills) | 16★ | repo metadata `license: null` | ① |
| [0x-man/mindmap-skill](https://github.com/0x-man/mindmap-skill) | 10★ | `license: null` | ① |
| [7abushahla/ieee-paper-skills](https://github.com/7abushahla/ieee-paper-skills) | 0★ | License 為 `NOASSERTION`（非標準授權，需人工判讀）；另需 LaTeX 工具鏈 | ①、⑥ |

### 6.4 僅作發現，不直接匯入

[karanb192/awesome-claude-skills](https://github.com/karanb192/awesome-claude-skills)（MIT，487★）本次作為**發現管道**使用，符合 PDM-002 對 awesome 清單的定位。**其 `documents` 分區的 4 個條目全部指回 `anthropics/skills`，`writing` 分區只有 `brand-guidelines`／`internal-comms`／一個 0★ 條目與一個「社群待補」佔位**——這獨立驗證了「`documents`／`writing` 的公開 OSI 供給確實稀薄」，不是本次搜尋不力。

---

## 7. 缺口與建議

### ~~缺口 1：`documents` 精選短少 1 個（3 / 4–6）~~ → **已於 2026-08-15 關閉（4 / 4–6）**

**成因不是搜尋不足，是結構性的**：`documents` 的高品質供給集中在 `anthropics/skills`，而其 4 個核心 Skill 為 source-available（①不過）；市面上最像替代品的兩個高星等 repo（`appautomaton/document-SKILLs` 145★、`w95/...` 156★）**恰好都是那 4 個 Skill 的非法 MIT 再授權**（§6.1）。真正原創且 OSI 授權的文件產出 Skill 極少。

原列三條補足路徑，依成本排序，**執行結果如下（詳見 §9）**：

1. **（最低成本）複審 `YuYY2004/excel-skills` 剩餘的格式化類 Skill。** → **✅ 缺口由此關閉，且不需要上游 PR。** 原記錄的「`excel-format` SKILL.md 全文約 1,600–1,800 行」是**量測錯誤**；在 pin 的 commit 上實測為 **262 行／9,839 bytes**，內嵌 Python 168 行，④ 的機械項全數通過。`excel-format` 直接升為精選 **D-4**。
2. **量測 `the3ma/course-quiz-builder` 的 `.mjs` 行數，並確認 `publish-pages.mjs`／`browser-check.mjs` 可被排除。** → **❌ 不通過。** `browser-check.mjs` 確實可排除，但 `publish-pages.mjs`（768 行、需 GitHub API 外網與 Token）是 SKILL.md Pipeline 的第 7 步、`selftest.mjs`（341 行、用 `new Function()`）是硬規則 2 的強制閘門，**兩者都不可排除**，④⑥ 皆 fail。維持 indexed，記入 §6.2。
3. **（最高成本）挖掘 `w95/...` 的 39 個自製 skill**，逐一確認來源歸屬（§6.1 保留意見）。→ **未執行，也不需要執行**（路徑 1 已達標）。**保留為 §5.2 法務判定的備援池**。

### 缺口 2：§5.2 的法務判定未完成，`documents` 的已索引數有塌陷風險

若 `anthropics/skills` 的 4 個 source-available Skill 因「reproducing outside the Services」條款被判定連索引都不可，`documents` 已索引由 10 → **6**，跌破下限。**缺口 1 關閉後，這是本文件唯一剩下的高優先未決事項**；補足路徑 3（`w95` 的 39 個自製 skill）是其備援池，另一條低成本方向是複查 §8 列出的 `danielrosehill` 13 個與 `YuYY2004` 2 個未複查 Skill 中是否有可歸 `documents` 者。

### 建議 3：PDM-004 是否為 Node 建立第三方套件白名單

目前白名單只有 Python 套件。Runtime 本身是 Node（Claude Agent SDK TS），但沒有任何第三方 npm 套件可用。`ai-presentation-builder`（§6.2）是第一個因此出局的候選，且 `documents` 類的 Node 生態（`pptxgenjs`、`docx`、`pdf-lib`）相當成熟。**建議把「是否開 npm 白名單」列為 PDM-004 的後續議題**——但注意這會擴大 SBX-002 的 SBOM 與供應鏈審查面，不是零成本。

### 建議 4：golden query set 的跨 repo 抽樣配額（PDM-011）

45 個已索引項目分布極不平均：`YuYY2004/excel-skills` 貢獻 14 個、`danielrosehill` 12 個，兩者合計 58%。**PDM-001 風險表「語意過度集中」的緩解（每 repo 至多 20% 題目）在本清單下仍然必要**。本次已對 `writing` 主動執行同型緩解（`sokrati` 的 6 個多語變體只取 1 個，見 §4.2）。

### 建議 5：INGEST-010／CONTENT-009 的失效演練對象

`nqumich/data-analyst-skill`（1★，最後推送 2026-03-02）與 `cabbagecachekid/neon-jetpack`（1★）是全清單上游消失風險最高的兩個，且各自承載 1 個與 2 個精選候選。**建議以其中之一作為 CONTENT-009 失效流程的實際演練對象**（PDM-001 風險表要求「對 `data` 需實際演練一次」，本建議把對象擴及 `writing`）。

---

## 8. 本次查核未涵蓋

- **精選標準 ④ 的人工逐行審查**：僅機械量測行數與 import，未逐行閱讀任何 Script（同 M0 限制）。承接 CONTENT-006。
- **精選標準 ⑤ 靜態 Secret 掃描**：未執行。承接 CONTENT-006。
- **`02:SKILL-002` 的完整規格驗證**：僅確認 `SKILL.md` 存在且 frontmatter 具備 `name`／`description`，**未驗證所有檔案引用可解析**。承接 CONTENT-006。
- **內容雜湊**：本文件只 pin 到 commit SHA，未計算內容雜湊（INGEST-004 要求，匯入時產生）。
- **`danielrosehill` repo 於 2026-08-15 觀測到 32 個 skill 目錄**（M0 記錄的是 12 合格 ＋ 7 需外網 ＝ 19）。新出現的 13 個（`add-changelog`、`data-dictionary-export`、`data-enrichment`、`data-to-document`、`divergent-data-pipe`、`geodata-formatter`、`header-standardisation`、`iso-review`、`localization-headers`、`numeric-rounding`、`parquet-jsonl-package`、`synthetic-data-overlay`、`update-data-dictionary`）**本次只抽查了 `data-to-document`（已否決）**，其餘未逐一複查，未計入數量。`data` 已超額達標，補查非急件。
- **`YuYY2004/excel-skills` 於 2026-08-15 觀測到 18 個 skill**（M0 記錄 12），路徑為 `claude/skills/<name>/SKILL.md`（另有 `codex/` 版本整併為單一 `CODEX.md`）。新增 6 個中，`excel-insert`／`excel-format`／`excel-freeze` 已完成複查並歸入 `documents`，`excel-replace`／`excel-orchestrate` 未複查、未計入；`excel-safe-workflow` 已複查（約 420 行全文／約 85 行 Python，`openpyxl`，不需外網）但屬流程性 meta skill，未計入數量。
- **GitHub API 於查核後段觸發未認證速率上限**，部分項目改以 `github.com` 頁面與 `raw.githubusercontent.com` 取得；所有已記錄的 SHA 與 License 判定均取自實際回應，無推測值。**注意：§9 已證實本次查核的行數量測至少有一筆嚴重錯誤（`excel-format` 記 1,600–1,800 行，實為 262 行），推測與此降級取徑有關。** §8 其餘「約 XXX 行」的估值同樣未經精確量測，CONTENT-006 應一併複驗。

---

## 9. 補選量測附錄（2026-08-15，`documents` 精選第 4 席）

量測方法：以 `codeload.github.com/<owner>/<repo>/zip/<pinned commit>` 取得 pin 的完整快照，於本機解壓後逐檔計算實際行數與位元組數；`excel-format` 另以 `raw.githubusercontent.com` 的同 commit 路徑二次取得交叉驗證。**全程只讀取、不執行任何套件內 Script（鐵律 1）。**

### 9.1 路徑 1：`YuYY2004/excel-skills` @ `c15e51e2...` — ✅ 通過

`claude/skills/<name>/SKILL.md` 全 18 個 Skill 的精確量測（行數／內嵌 ```` ```python ```` 區塊行數）：

| Skill | 全文行數 | 內嵌 Python | ④ ≤300 |
| --- | --- | --- | --- |
| **`excel-format`** | **262** | **168** | **✅** |
| `excel-freeze`（D-2） | 83 | 34 | ✅ |
| `excel-find-duplicates`（A-6） | 98 | 36 | ✅ |
| `excel-orchestrate` | 136 | 0 | ✅（meta skill，未計入數量） |
| `excel-validate` | 157 | 105 | ✅ |
| `excel-scout` | 164 | 58 | ✅ |
| `excel-deduplicate`（A-5） | 176 | 138 | ✅ |
| `excel-sort` | 181 | 122 | ✅ |
| `excel-split` | 200 | 123 | ✅ |
| `excel-insert`（D-1） | 222 | 140 | ✅ |
| `excel-safe-workflow` | 234 | 83 | ✅（meta skill，未計入數量） |
| `excel-mapping-replace` | 257 | 166 | ✅ |
| `excel-regex-clean` | 257 | 181 | ✅ |
| `excel-merge` | 303 | 233 | ⚠️ 全文 303 超 3 行（Python 233 行仍在限內） |
| `excel-filter` | 306 | 218 | ⚠️ 同上 |
| `excel-replace` | 336 | 229 | ⚠️（未複查、未計入數量） |
| `excel-delete` | 374 | 283 | ⚠️ |
| `excel-date-to-text` | 395 | 267 | ⚠️ |

**`excel-format` 的驗證憑據**：`sha256 = 68dc7ce82d6b14bb967e691356c30fe148624cb011ecd7b7f67287c0f8ff85e0`、9,839 bytes、262 行；zip 與 raw 兩條取徑**位元組完全相同**。原記錄的 1,600–1,800 行與此差 6 倍以上，判定為量測錯誤而非上游變更（pin 未變）。

**④ 其餘機械項**：無 `eval`／`exec`／`compile`、無動態下載、無 `subprocess`、無 `pip install` 字樣。依賴為 `openpyxl`（主路徑）＋ `pandas`／`numpy`（僅 `COL_WIDTH='auto'` 分支），三者皆在 [PDM-004 §4 白名單](../m0/pdm-proposals.md)內。⑥ 不需外網。②⑨ 沿用 §5.1 第 3 列（MIT、單一可辨識作者）。

> **順帶更正一筆既有記錄**：`excel-safe-workflow` 在 §8 記為「約 420 行／約 85 行 Python」，實測為 **234 行／83 行**。其 meta skill 定位與「不計入數量」的判斷不變。

### 9.2 路徑 2：`the3ma/course-quiz-builder` @ `e2ac75e5...` — ❌ 不通過

套件內 `skills/course-quiz-builder/scripts/` 全部 5 個 `.mjs` 的精確行數：

| 檔案 | 行數 | 外網／子行程 | SKILL.md 引用位置 | 可排除？ |
| --- | --- | --- | --- | --- |
| `build-quiz.mjs` | 266 ✅ | 無（僅 `node:fs`／`path`／`url`） | Pipeline 第 4 步 | 核心，不需排除 |
| `selftest.mjs` | **341 ❌** | 無網路，但用 **`new Function()`** 動態求值模板 CORE 區段 | Pipeline 第 6 步＋**硬規則 2「Never ship without selftest.mjs printing PASS」** | **否**（強制閘門） |
| `publish-pages.mjs` | **768 ❌** | **`fetch()` 打 GitHub API ＋ `execFileSync` 呼叫 `git`／`gh auth token` ＋ repo 寫入 Token** | Pipeline 第 7 步（"Skip only if the user asked for a local page"） | **否**（主流程步驟） |
| `publish-selftest.mjs` | **356 ❌** | `execFileSync`（無網路、無 Token） | 「Quick reference」與 publish 段落 | 隨 publish 路徑連帶 |
| `browser-check.mjs` | 257 ✅ | `execFileSync` 啟動 headless Chromium（不在 Runtime Image 內） | 第 6 步標為 "Optionally"，無瀏覽器時 skips cleanly | **是** |

**結論**：唯一可乾淨排除的是 `browser-check.mjs`；`publish-pages.mjs` 與 `selftest.mjs` 都被 SKILL.md 正文指定為必經步驟，排除等於改寫 Skill 語意。**④（3 檔超 300 行 ＋ `new Function()` 動態求值）與 ⑥（GitHub API egress ＋ Token）雙重不過**，維持 indexed。

> 附帶觀察：`publish-pages.mjs` 的 Token 解析鏈（`$GITHUB_REPOSITORY_TOKEN` → `--env-file` → `$GH_TOKEN` → `~/.config/course-quiz/token.env` → `gh auth token`）與其 exit 4 的 secret guard，正是 **SEC-005／NFR-002 會關切的形態**。即使日後 ⑥ 政策放寬，此 Skill 也應被視為高風險樣本，不宜作為首批精選。
