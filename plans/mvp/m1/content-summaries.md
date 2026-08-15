# CONTENT-005：首批 Skill 白話摘要與人工審核紀錄

- 狀態：**工序已建立，人工審核未完成。** 本文件 45 筆的審核狀態全部為「待審」。
- 產出日：**2026-08-15**
- 機器可讀產出：[`tools/content/summaries.json`](../../../tools/content/summaries.json)（45 筆，含 `model`、`prompt_version`、生成時間）
- 產生工具：[`tools/content/generate_summaries.py`](../../../tools/content/generate_summaries.py)（`--selftest` 為離線自我檢查）
- 承接工作項：[`03-work-items.md` CONTENT-005](../03-work-items.md)；[`curated-skill-list.md` §3 檢查 ⑦](curated-skill-list.md)
- 依據：
  - [ADR-013 §1](../../../adr/ADR-013-intent-search-architecture.md)（索引時增強每個 Skill Version 執行一次；**生成內容標示為模型產出，錯誤可由人工修正並觸發重建**）
  - ADR-013「成本與限制」第 1 條（**索引時增強的品質決定搜尋上限，需要人工抽查機制**）——本文件即該機制的落地
  - [PDM-002 精選檢查 ⑦](../m0/pdm-proposals.md)（有可理解的白話摘要：非技術使用者讀得懂它做什麼、需要什麼輸入）
  - [02 §DISC-003](../02-specifications-and-acceptance-criteria.md)（一般模式顯示功能、限制、輸入、輸出、依賴……）

---

## 1. 審核工序

### 1.1 這份文件是什麼、不是什麼

**是**：對 45 筆種子 Skill 的模型生成摘要，建立一份可審核、可追溯、有判定欄位的紀錄。

**不是**：摘要的產生器。摘要不是在這份文件裡「手寫」出來的——平台的匯入管線（[`services/platform/internal/ingest/enrich.go`](../../../services/platform/internal/ingest/enrich.go)）在每次匯入時自動呼叫 `POST /v1/enrich-skill` 產生，並由詳情頁的 `enrichment` 區塊呈現（見 §3）。本工序補的是**精選內容的人工審核紀錄**，也就是 ADR-013 明列、但在此之前不存在的「人工抽查機制」。

> **摘要不在本文件就地改寫。** 判定「需修改」的處置是調整 prompt 或把該筆標為人工覆寫，再重跑增強與 reindex（ADR-013 §1「錯誤可由人工修正並觸發重建」）。直接在這裡改字，改到的只是紀錄，不是使用者看到的東西。

### 1.2 誰審、判準、狀態值

| 項目 | 規定 |
| --- | --- |
| **審核人** | 內容負責人，且**不得是本工序的產生者**。精選（curated）15 筆**必審**；已索引（indexed）30 筆抽審不低於 1/3，且必須涵蓋三個類別 |
| **唯一主判準** | 讀完該筆的摘要與範例句，**一個不懂技術的人能否說出「這個 Skill 能為我做什麼、我要給它什麼」**。答得出＝通過 |
| **否決條件（任一成立即不得記通過）** | (a) **幻覺**：宣稱套件文件沒有寫的能力；(b) **未繁中化**：直接沿用簡體中文原文或簡中在地慣例；(c) **超出 ADR-013 白名單**：出現信任、風險、安全、品質的判斷——這四類永遠不是模型可以產出的 |
| **狀態值** | `待審`／`通過`／`需修改`。**`需修改` 必須寫原因**；沒有原因的 `需修改` 視同 `待審` |
| **記錄方式** | 直接編輯本文件每筆下方的審核表格（狀態、審核人、日期、備註）。這份文件就是審核紀錄本身，不另設表單 |

### 1.3 簡體中文來源的處理

45 筆中有 **16 筆**的 `SKILL.md` 原文為簡體中文（`YuYY2004/excel-skills` 15 筆、`nqumich/data-analyst-skill` 1 筆），其中 **6 筆屬精選**。這 16 筆在下方各節以 ⚠️ 標註。

**處理方式：摘要即繁中化呈現層。** 平台**不改寫上游套件內容**（Skill Version 不可變，ADR-003／鐵律 4），也不維護一份翻譯後的 `SKILL.md`。使用者在搜尋結果與詳情頁讀到的繁體中文，是增強管線以 `language: zh-Hant` 產生的摘要與任務範例句；原始簡體 `SKILL.md` 只在進階模式的原文檢視中出現。**因此「改寫為繁中」在本專案的實作意義就是「產出繁中摘要」**，而不是翻譯原始檔。

**閘門測試會直接檢驗這批的三筆。** [`gate-test/task-cards.md` §3](gate-test/task-cards.md) 的三張卡以簡中來源的 Skill 為 gold：

| 卡 | gold primary | 本文件對應 |
| --- | --- | --- |
| `DOC-4` | `excel-freeze` | §5 C-2 |
| `DAT-3` | `data-analyst` | §5 C-10 |
| `DAT-4` | `excel-deduplicate` | §5 C-14 |

若這三張卡在閘門中集中失敗，依 [`gate-test/analysis.md` §3](gate-test/analysis.md)：先確認實際入庫的增強文字是否就是本文件審核過的版本，再判斷問題是否不在語言而在別處。

---

## 2. 生成方法與可追溯性

### 2.1 逐字重用生產路徑，不另寫一套

45 筆全部是 `POST /v1/enrich-skill`（[`services/llm`](../../../services/llm/src/skillhub_llm/enrich.py)）的輸出，模型一律 **`gpt-5.6-sol`**。套件內容以 `import_seed.repack_skill` 在 pin 的 commit 上重組，`skill_md` 與 `file_tree` 與匯入時送出的完全一致。工具本身不含任何 prompt、schema 或模型參數。

### 2.2 計費：15 筆重用、30 筆新呼叫

| 來源 | 筆數 | prompt 版本 | 說明 |
| --- | --- | --- | --- |
| 重用 [`tools/goldenset/corpus_enriched/`](../../../tools/goldenset/corpus_enriched/) | **15** | `enrich-skill/v1` | 僅在 **repo、路徑、pin commit 三者全等**時重用，也就是被增強的是同一份位元組。同時保證這 15 筆的文字與 golden set 推導 `MaxCosineDistance = 0.75` 時所用的完全相同 |
| 本次新呼叫 | **30** | `enrich-skill/v2` | golden set 語料未涵蓋者。0 筆失敗 |

> ⚠️ **prompt 版本分歧，且是實質差異。** `enrich-skill/v2` 相對 v1 新增 `limitations` 欄位。**重用的 15 筆沒有 `limitations`**（在 `summaries.json` 中記為 `null`＝欄位不存在，與 `[]`＝模型未讀到限制不同），其中 **7 筆是精選**。02 §DISC-003 的允收準則要求一般模式顯示「限制」，**這 7 筆目前無法滿足**（見 §4 與 §7）。

### 2.3 金鑰處理

`LITELLM_API_KEY` 只進 `services/llm` 的**行程環境變數**，由 repo 根 `.env` 的 `OPENAI_API_KEY` 匯出，不落任何檔案，也不在 `summaries.json` 或本文件中。

本次為離線內容工序，`LITELLM_BASE_URL` 直接指向 OpenAI，**未經 LiteLLM 閘道**——與 [`golden-query-set.md` §10 附註 1](golden-query-set.md) 及 [`import-report.md` §1.1](import-report.md) 同性質：**離線工具的既有例外，產品實作不得比照**（鐵律 8、ADR-017）。

### 2.4 ⚠️ 本次生成的文字不等於線上索引的文字

增強是**每次匯入重新生成**的，LLM 輸出非決定性。因此：

- 人工審核實際批准的是**「某個模型＋某個 prompt 版本在某份套件上的產出品質」**，而不是一串固定字串。這是 CONTENT-005 這個工作項的結構性上限，不是本次工序的疏漏。
- **`import-report.md` 記錄的 44 筆線上目錄目前不存在。** 實查 `skillhub-postgres-1`：`skills` 2 筆（整合測試殘留）、`skill_versions` 0 筆、`search_documents` 2 筆且皆 `pending`。閘門測試的「環境凍結（44 筆索引）」必須先重新匯入才談得上。
- 重新匯入時若 `services/llm` 為現行版本，增強會以 **v2** 產出，與 `import-report.md` 記錄的 v1 不同。**建議：人工審核完成後再重新匯入，並核對線上 `enriched_summary` 與本文件的落差；落差大到改變「非技術使用者讀不讀得懂」的判定時，重審該筆。**

---

## 3. CONTENT-005 允收對照

**先講結論：`02-specifications-and-acceptance-criteria.md` 沒有 CONTENT-005 的需求 ID，也沒有任何 CONTENT 系列的允收準則。** 該檔只在 DISC-002 的欄位表中兩次提及 CONTENT-003。CONTENT-005 的文字只存在於 `03-work-items.md` 的一行工作項。因此逐條對照的對象取三份現有依據。

| 依據 | 要求 | 本工序的對應 | 判定 |
| --- | --- | --- | --- |
| `03-work-items.md` CONTENT-005 | 對首批 Skill 產生一般使用者可理解的摘要 | 45/45 筆皆有繁中白話摘要與 3–5 則雙語任務範例句 | ✅ 已產生（**品質待人工判定**） |
| PDM-002 檢查 ⑦ | 非技術使用者讀得懂它做什麼、需要什麼輸入 | 判準已定為唯一主判準（§1.2），欄位已備妥 | ⏳ **待人工審核**，⑦ 仍為 `pending` |
| ADR-013「需要人工抽查機制」 | 建立抽查機制 | 本文件即該機制：判定欄位、判準、否決條件、改法（重跑增強而非就地改字） | ✅ 已建立 |
| 02 §DISC-003 | 一般模式顯示功能、限制、輸入、輸出、依賴、權限、來源、License、相容性 | 「功能」＝`summary`；「輸入／輸出／依賴」＝`tags`；「限制」＝`limitations` | ⚠️ **7 筆精選缺 `limitations`**（§2.2） |
| 「呈現於平台」 | — | 已由 `ingest/enrich.go` 於匯入時自動產生（commit `b144bea`），由 `catalog/detail.go` 的 `enrichmentFrom` 供給 `enrichment` 區塊（commit `ebc4036`），契約見 `contracts/openapi/public.yaml`（`summary` 恆為套件自身 frontmatter，模型產出一律落在 `enrichment` 之下並標示）。測試：`services/platform/internal/ingest/enrich_test.go`、`services/llm/tests/test_enrich.py` | ✅ 已呈現；本工序補的是**人工審核紀錄** |

> **`anthropics/skills` 的免責條款怎麼處理。** `curated-skill-list.md` §3 腳註 3 要求把 README 的 "provided for demonstration and educational purposes only" 納入摘要措辭考量。**本工序的判定是：不納入摘要。** 該句是使用免責，屬於信任／品質陳述，而 ADR-013 白名單明令模型產出不得包含這一類判斷（`enrich.py` 的 system prompt 亦明文禁止）。它應由詳情頁的 License／來源區塊承接（ADR-021 的兩軸答案，`license.status` 目前最高只到 `declared`）。**此解讀需負責人確認**；若負責人要求摘要承載免責，則需改的是 ADR-013 白名單，不是這批摘要。涉及 6 筆（`brand-guidelines`、`internal-comms`、`docx`、`pdf`、`pptx`、`xlsx`）。

---

## 4. 抽查結果（精選 15 筆逐一目檢）

抽查者不是審核人，**下表是給審核人的預判，不是判定**——所有正式審核欄位仍為「待審」。

### 4.1 機械檢查

| 項目 | 結果 |
| --- | --- |
| 簡體字殘留（掃 45 筆的 `summary` ＋ `task_examples.zh_hant` ＋ `limitations`） | **0 筆**。無任何簡體字元 |
| 超出 ADR-013 白名單的宣稱（信任／風險／安全／品質） | **0 筆** |
| 幻覺抽驗（對可疑數據回查 pin commit 的 `SKILL.md` 原文） | **0 筆**。`excel-find-duplicates` 的「168MB／33 萬列約 150 秒」實查原文第 3 條為 "pandas reading 168MB/330K rows takes ~150s"，屬轉述 |

### 4.2 目檢預判

| 筆 | 預判 | 原因 |
| --- | --- | --- |
| C-4 `excel-format` | **需修改** | 範例句「字體改成微軟雅黑 12 點」忠實轉述簡中原文（原文 `"微软雅黑"`），**不是幻覺**；但「微軟雅黑」是簡中在地字型，繁中環境的對應是「微軟正黑體」。對繁中使用者不自然，且該字型不必然存在。屬 §1.2 否決條件 (b) 的邊界情形 |
| C-5 `brand-guidelines`、C-6 `internal-comms`、C-10 `data-analyst`、C-11 `data-cleanliness-scan`、C-12 `csv-to-json`、C-13 `text-to-numeric`、C-14 `excel-deduplicate` | **需修改**（共 **7 筆**） | **缺 `limitations` 欄位**（`enrich-skill/v1` 產出）。02 §DISC-003 允收要求一般模式顯示「限制」。摘要文字本身無問題。處置＝以 v2 重跑這 7 筆（7 次呼叫），非改寫文字 |
| C-11 `data-cleanliness-scan`、C-13 `text-to-numeric` | 附註（不單獨計為需修改） | 摘要句缺主詞（「掃描一個或多個 CSV……」「將含貨幣符號……」），與其餘 13 筆的「此技能……」體例不一致。不影響可理解性 |
| 其餘 7 筆 | **預判通過** | `excel-insert`、`excel-freeze`、`handoff`、`humanizer`、`line-edit`、`ai-written-check`、`excel-find-duplicates` |

> **一個正面發現：近義群已自行差異化。** `line-edit` 與 `ai-written-check` 的 `limitations` 明確寫出「這件事屬於 `cringe-check`／`full-review`」，正是 [`gate-test/analysis.md` §3 修正項 C3](gate-test/analysis.md) 要求的「什麼時候該用我」。這是 v2 新增 `limitations` 的副產品——**也就是說，把 v1 的 7 筆補成 v2 對 WRI-3 這張近義群卡有直接好處**。

---

## 5. 精選（curated）15 筆

### C-1. `excel-insert`（documents／精選）

- 來源：[https://github.com/YuYY2004/excel-skills](https://github.com/YuYY2004/excel-skills) @ `c15e51e20424`，`claude/skills/excel-insert/SKILL.md`
- 生成：本次新呼叫｜模型 `gpt-5.6-sol`｜prompt `enrich-skill/v2`｜2026-08-15
- ⚠️ **來源 SKILL.md 為簡體中文**；下列摘要與範例句即繁中化呈現層（見 §1.3）。

> 此技能可在 Excel 檔案的指定位置插入一列或多列，或插入一行或多行，並支援設定新列表頭及填充值。使用者需要提供 Excel 檔案、明確的目標行或列，以及插入方向；也可指定插入數量、表頭名稱和要填入的值或公式。它會先檢查工作表結構與附近公式，規劃插入位置，完成修改後再核對新增行列及內容。

任務範例句：

- 請在「申請日」欄右側插入一欄，並命名為「格式化日期」。　／　*Insert a column to the right of the “Application Date” column and name it “Formatted Date”.*
- 請在這個 Excel 檔案的第 5 行下方新增三個空白行。　／　*Add three blank rows below row 5 in this Excel file.*
- 請在 E 欄左側新增一欄，並將所有資料列填入「待處理」。　／　*Insert a new column to the left of column E and fill every data row with “Pending”.*
- 請在第 20 行上方新增兩行，並將新行的所有儲存格填入 0。　／　*Add two rows above row 20 and fill all their cells with 0.*
- 請在「金額」欄後新增一欄，命名為「稅額」，並填入指定公式。　／　*Insert a column after the “Amount” header, name it “Tax”, and fill it with the specified formula.*

標籤 — 輸入：`excel 檔案`、`目標行`、`目標列`、`插入方向`、`插入數量`、`表頭名稱`、`填充值`、`公式`；輸出：`修改後的 excel 檔案`、`新增行`、`新增列`、`驗證結果`、`備份檔案`；工具：`python`、`openpyxl`；依賴：`python`、`openpyxl`、`excel-safe-workflow`

限制：

- 必須明確指定目標列或目標行。
- 執行前必須完成需求解析、勘察與規劃，執行後必須驗證。
- 操作前必須建立時間戳備份，成功後保留最新 3 份；操作失誤時須刪除損壞檔案並從備份復原。
- openpyxl 插入操作不會更新 INDIRECT 或 OFFSET 公式的引用。

| 審核狀態 | 審核人 | 審核日期 | 備註 |
| --- | --- | --- | --- |
| **待審** | | | |

### C-2. `excel-freeze`（documents／精選）

- 來源：[https://github.com/YuYY2004/excel-skills](https://github.com/YuYY2004/excel-skills) @ `c15e51e20424`，`claude/skills/excel-freeze/SKILL.md`
- 生成：本次新呼叫｜模型 `gpt-5.6-sol`｜prompt `enrich-skill/v2`｜2026-08-15
- ⚠️ **來源 SKILL.md 為簡體中文**；下列摘要與範例句即繁中化呈現層（見 §1.3）。

> 此技能可凍結或取消凍結 Excel 活動工作表的窗格，讓指定的列或欄在捲動時保持可見，而不更動資料。它支援凍結首列、首欄、首列加首欄、前幾列或指定位置，並會先檢查目前狀態與表頭、執行設定，再驗證結果。使用者需提供目標 .xlsx 檔案及想要的凍結方式或位置。

任務範例句：

- 請凍結這份 Excel 活頁簿的首列，讓表頭在捲動時保持可見。　／　*Freeze the top row in this Excel workbook so the headers remain visible while scrolling.*
- 請在目前的工作表中同時凍結首列與首欄。　／　*Keep both the first row and first column fixed in the active worksheet.*
- 請凍結這個 .xlsx 檔案的前三列。　／　*Freeze the first three rows of this .xlsx file.*
- 請取消目前工作表既有的凍結窗格設定。　／　*Remove the existing freeze panes setting from the active worksheet.*
- 請先檢查目前的凍結窗格設定，再將工作表凍結於 C4。　／　*Check the current freeze panes setting and then freeze the worksheet at cell C4.*

標籤 — 輸入：`excel .xlsx file`、`freeze position`、`active worksheet`；輸出：`updated excel workbook`、`freeze panes status`；工具：`python`、`microsoft excel`；依賴：`openpyxl`、`excel-safe-workflow`

限制：

- 僅影響活動工作表；若活頁簿有多個工作表，需逐一設定。
- 凍結線穿過合併儲存格時，合併會被拆分。
- 凍結效果需在 Excel 中開啟檔案才能查看；openpyxl 只會寫入相關屬性。
- 操作前必須依時間戳命名方式備份檔案。
- 需要安裝 openpyxl。

> 🎯 **閘門卡 `DOC-4` 的 gold primary**（`gate-test/task-cards.md` §3）。本筆的摘要品質會被閘門直接檢驗。

| 審核狀態 | 審核人 | 審核日期 | 備註 |
| --- | --- | --- | --- |
| **待審** | | | |

### C-3. `handoff`（documents／精選）

- 來源：[https://github.com/ToolMonsters/handoff-skill](https://github.com/ToolMonsters/handoff-skill) @ `fa70c91e44a5`，`SKILL.md`
- 生成：本次新呼叫｜模型 `gpt-5.6-sol`｜prompt `enrich-skill/v2`｜2026-08-15

> 此 Skill 會把目前對話整理成完整的交接 Markdown 文件，讓另一個大型語言模型能直接接續工作。它需要目前對話中的背景、已定決策、工作進度、完整進行中內容、已放棄方案、使用者偏好與下一步，並要求內容只採用對話裡明確出現的資訊。適合在使用者輸入「handoff」、工作階段快滿、接近上下文或用量限制，或想改用另一個 AI 繼續對話時使用。

任務範例句：

- 請建立交接文件，讓我能在新的 ChatGPT 工作階段繼續這段對話。　／　*Create a handoff document so I can continue this conversation in a fresh ChatGPT session.*
- handoff——這個工作階段快滿了，我需要另一個 AI 從目前進度無縫接手。　／　*Handoff—this session is almost full, and I need another AI to pick up the work exactly where we left off.*
- 請把目前完整草稿、所有已定決策、已放棄方案和立即下一步整理成交接檔案。　／　*Please capture the complete current draft, all settled decisions, rejected approaches, and the immediate next step in a handoff file.*
- 我快達到上下文限制了，請為新的 Claude 工作階段產生 Markdown 交接文件。　／　*I’m hitting the context limit; generate a Markdown handoff for a fresh Claude session.*

標籤 — 輸入：`目前對話`、`對話背景與目標`、`已定決策`、`工作進度`、`進行中內容`、`已放棄方案`、`使用者偏好`、`下一步`；輸出：`markdown 交接文件`、`handoff-[topic].md`；工具：（無）；依賴：（無）

限制：

- 只能使用目前對話中明確出現的事實、決定與文字；不清楚之處須標示為「unclear」。
- 進行中的草稿、提示詞、程式碼、清單或計畫必須完整逐字收錄，不能只做摘要。
- 所有數字、名稱、檔案路徑、URL 與日期必須保持原樣。
- 輸出必須是單一 Markdown 文件；若平台支援檔案或 artifact，檔名須為 `handoff-[topic].md`，否則須放在單一 Markdown 程式碼區塊中。
- 文件必須包含規定的所有章節，且不得加入前言或結語。

| 審核狀態 | 審核人 | 審核日期 | 備註 |
| --- | --- | --- | --- |
| **待審** | | | |

### C-4. `excel-format`（documents／精選）

- 來源：[https://github.com/YuYY2004/excel-skills](https://github.com/YuYY2004/excel-skills) @ `c15e51e20424`，`claude/skills/excel-format/SKILL.md`
- 生成：本次新呼叫｜模型 `gpt-5.6-sol`｜prompt `enrich-skill/v2`｜2026-08-15
- ⚠️ **來源 SKILL.md 為簡體中文**；下列摘要與範例句即繁中化呈現層（見 §1.3）。

> 此技能可統一調整 Excel 工作表的字體、字號、粗體、斜體、顏色、水平與垂直對齊、欄寬及列高。使用者需要提供 Excel 檔案、想套用的格式，以及整張工作表、表頭、資料區、指定欄列或條件相符儲存格等套用範圍；若只要求美化，則使用內建的表頭、資料區與自動欄寬預設。流程會先檢查目前格式，再修改檔案並驗證表頭、資料區與欄寬等結果。

任務範例句：

- 請把表頭設為粗體並置中，字體改成微軟雅黑 12 點。　／　*Make the header row bold and centered, using Microsoft YaHei at 12 pt.*
- 請將整張工作表改成 Arial 10 點，並把金額欄靠右對齊。　／　*Change the entire worksheet to Arial 10 pt and right-align the amount column.*
- 請自動調整所有欄寬，並將資料區垂直置中。　／　*Auto-fit all column widths and vertically center the data area.*
- 請只將符合我指定條件的儲存格設為紅色斜體文字。　／　*Apply red italic text only to the cells matching my specified condition.*
- 請使用預設格式方案美化這個 Excel 檔案。　／　*Beautify this Excel file using the default formatting preset.*

標籤 — 輸入：`xlsx workbook`、`format specification`、`scope selection`、`font settings`、`alignment settings`、`column width`、`row height`；輸出：`formatted xlsx workbook`、`format verification`、`timestamped backups`；工具：`python`、`openpyxl`、`pandas`；依賴：`python`、`openpyxl`、`pandas`、`numpy`、`excel-safe-workflow`、`system fonts`

限制：

- 執行前必須完成需求解析與格式勘察，執行後必須驗證結果。
- 操作前必須建立時間戳備份，成功後保留最新 3 份；若操作失誤，需刪除損壞檔案並從備份恢復。
- 字體是否可用取決於開啟 Excel 檔案的系統。
- 合併儲存格的格式需要另外處理。
- 未指定的格式參數不會變更，會保留原有格式。

> **抽查預判：需修改** — 範例句「字體改成微軟雅黑 12 點」忠實轉述簡中原文，非幻覺；但「微軟雅黑」是簡中在地字型，繁中對應為「微軟正黑體」（§4.2）。

| 審核狀態 | 審核人 | 審核日期 | 備註 |
| --- | --- | --- | --- |
| **待審** | | | |

### C-5. `brand-guidelines`（writing／精選）

- 來源：[https://github.com/anthropics/skills](https://github.com/anthropics/skills) @ `f6656c1256d5`，`skills/brand-guidelines/SKILL.md`
- 生成：重用 golden set 既有增強產出｜模型 `gpt-5.6-sol`｜prompt `enrich-skill/v1`｜2026-08-15

> 此技能會將 Anthropic 的官方品牌視覺套用到簡報等適合進行視覺格式化的成品，包括品牌色彩、字體、文字層級與圖形強調色。使用者需要提供要套用樣式的成品或其內容；標題會優先使用 Poppins、內文使用 Lora，若系統沒有這些字型，則分別改用 Arial 與 Georgia。非文字圖形會輪替使用橙、藍、綠色，並依背景選擇易讀的文字顏色。

任務範例句：

- 請將 Anthropic 的官方色彩與字體套用到這份簡報。　／　*Apply Anthropic’s official colors and typography to this presentation.*
- 請用 Poppins 標題、Lora 內文及適當的備用字型重新設計這些投影片。　／　*Restyle these slides with Poppins headings, Lora body text, and suitable fallback fonts.*
- 請使用 Anthropic 的深色、淺色、灰色與強調色調色盤來格式化這份成品。　／　*Format this artifact using Anthropic’s dark, light, gray, and accent color palette.*
- 請將非文字圖形依序套用 Anthropic 的橙色、藍色與綠色強調色。　／　*Update the non-text shapes to cycle through Anthropic’s orange, blue, and green accent colors.*

標籤 — 輸入：`presentation artifact`、`text hierarchy`、`background colors`、`shapes`；輸出：`brand-styled artifact`、`formatted typography`、`brand color palette`、`styled shapes`；工具：`python-pptx rgbcolor`；依賴：`python-pptx`、`system-installed fonts`、`poppins`、`lora`、`arial`、`georgia`

限制：**欄位不存在**（本筆為 `enrich-skill/v1` 產出，該版本尚無 `limitations`；見 §2.2）

> **抽查預判：需修改** — 缺 `limitations` 欄位（`enrich-skill/v1` 產出），不滿足 02 §DISC-003 的「限制」允收。摘要文字本身未見問題。處置＝以 v2 重跑本筆（§4.2）。

| 審核狀態 | 審核人 | 審核日期 | 備註 |
| --- | --- | --- | --- |
| **待審** | | | |

### C-6. `internal-comms`（writing／精選）

- 來源：[https://github.com/anthropics/skills](https://github.com/anthropics/skills) @ `f6656c1256d5`，`skills/internal-comms/SKILL.md`
- 生成：重用 golden set 既有增強產出｜模型 `gpt-5.6-sol`｜prompt `enrich-skill/v1`｜2026-08-15

> 此技能協助撰寫符合公司慣用格式的內部溝通內容，包括 3P（進度、計畫、問題）更新、公司電子報、FAQ、狀態報告、主管簡報、專案更新與事件報告。使用者需提供溝通類型、相關內容，以及期望的格式或背景；技能會依類型套用 examples 目錄中的格式、語氣與內容蒐集指引，若無相符指引則要求進一步說明。

任務範例句：

- 請撰寫一份每週 3P 更新，說明我們的進度、計畫與目前問題。　／　*Write a weekly 3P update covering our progress, plans, and current problems.*
- 請將這些公司公告整理成一份提供給全體員工的內部電子報。　／　*Turn these company announcements into an internal newsletter for all employees.*
- 請針對新的遠距工作政策草擬清楚的 FAQ 回覆。　／　*Draft clear FAQ answers about the new remote-work policy.*
- 請根據這些專案筆記與里程碑，整理一份精簡的主管更新。　／　*Create a concise leadership update from these project notes and milestones.*
- 請撰寫一份內部事件報告，說明事件經過、影響與後續行動。　／　*Write an internal incident report describing what happened, the impact, and the follow-up actions.*

標籤 — 輸入：`溝通類型`、`內部溝通內容`、`格式需求`、`背景資訊`；輸出：`3p 更新`、`公司電子報`、`faq 回覆`、`狀態報告`、`主管更新`、`專案更新`、`事件報告`；工具：（無）；依賴：`examples/3p-updates.md`、`examples/company-newsletter.md`、`examples/faq-answers.md`、`examples/general-comms.md`

限制：**欄位不存在**（本筆為 `enrich-skill/v1` 產出，該版本尚無 `limitations`；見 §2.2）

> **抽查預判：需修改** — 缺 `limitations` 欄位（`enrich-skill/v1` 產出），不滿足 02 §DISC-003 的「限制」允收。摘要文字本身未見問題。處置＝以 v2 重跑本筆（§4.2）。

| 審核狀態 | 審核人 | 審核日期 | 備註 |
| --- | --- | --- | --- |
| **待審** | | | |

### C-7. `humanizer`（writing／精選）

- 來源：[https://github.com/blader/humanizer](https://github.com/blader/humanizer) @ `523374dee72d`，`SKILL.md`
- 生成：本次新呼叫｜模型 `gpt-5.6-sol`｜prompt `enrich-skill/v2`｜2026-08-15

> 此 Skill 會檢查文字中的常見 AI 寫作痕跡，例如浮誇宣傳語氣、空泛歸因、過度使用特定詞彙、破折號、三項列舉、被動語態、填充語句及公式化結尾，並將內容改得更自然。使用者可貼上文字、指定要就地改寫的檔案，或將它嵌入其他寫作工作；也可提供寫作樣本與期望語氣，讓改寫貼近原作者。它會保留原有資訊而不新增事實，並依使用模式輸出草稿、檢查摘要與定稿，或直接更新檔案、只回傳最終文字。

任務範例句：

- 請改寫這份產品公告，讓語氣更自然、少一點宣傳感，但不要更動任何事實。　／　*Rewrite this product announcement so it sounds natural and less promotional without changing any facts.*
- 請讓這篇文章讀起來更像真人寫作，並模仿我提供的寫作樣本風格。　／　*Humanize this essay and match the style of the writing sample I provided.*
- 請檢查這份技術文件中的 AI 寫作痕跡，同時維持中性且精確的語氣。　／　*Review this technical documentation for AI writing patterns while keeping the tone neutral and precise.*
- 請直接改寫這個檔案中的散文內容，但不要變更程式碼區塊、frontmatter、資料及連結目標。　／　*Humanize the prose in this file in place, but leave its code blocks, frontmatter, data, and link targets unchanged.*
- 請移除這份草稿中的贅詞、空泛歸因、過多破折號及公式化結尾。　／　*Remove filler, vague attributions, excessive em dashes, and formulaic conclusions from this draft.*

標籤 — 輸入：`source text`、`file path`、`writing sample`、`intended tone`；輸出：`draft rewrite`、`audit bullets`、`final rewrite`、`edited file`、`change summary`；工具：`file reader`、`file editor`；依賴：（無）

限制：

- 改寫必須保留原文中的所有主張，不得加入來源文字或使用者未提供的事實、姓名、數字、日期、引文或引用資料。
- 只有部落格、散文、評論及個人寫作等適合展現作者聲音的內容才會加入個性；百科、技術、法律及參考文字維持中性平實。
- 檔案模式只改寫散文，不變更程式碼區塊、frontmatter、資料及連結目標。
- 最終改寫不使用 em dash 或 en dash；若使用者提供的寫作樣本本身使用 em dash，則依樣本的頻率保留。
- 要模仿特定作者的寫作習慣，使用者必須提供其既有寫作樣本。

| 審核狀態 | 審核人 | 審核日期 | 備註 |
| --- | --- | --- | --- |
| **待審** | | | |

### C-8. `line-edit`（writing／精選）

- 來源：[https://github.com/cabbagecachekid/neon-jetpack](https://github.com/cabbagecachekid/neon-jetpack) @ `4c0f62e17c00`，`skills/line-edit/SKILL.md`
- 生成：本次新呼叫｜模型 `gpt-5.6-sol`｜prompt `enrich-skill/v2`｜2026-08-15

> 此技能會輕度潤飾作者已有的草稿，修正文法、拼字、標點、冗詞、彆扭句子與不清楚的表達，同時盡量保留原意和個人語氣。它預設適用於商務電子郵件或專業訊息，也會配合明顯的休閒文本風格。使用者需提供現有草稿；輸出會先給可直接複製的完整修訂版，再列出涉及語意取捨的修改與原因，並把無法確認的意思標為問題。

任務範例句：

- 請潤飾這封客戶電子郵件的文法與清晰度，但不要改變我的原意或語氣。　／　*Please polish this client email for grammar and clarity without changing my meaning or voice.*
- 請整理這段 Slack 訊息，並保留輕鬆自然的風格。　／　*Clean up this Slack message and keep it casual.*
- 請逐句編修這份草稿、刪除贅詞，並標出任何意思不清楚之處。　／　*Line-edit this draft, remove wordiness, and flag anything whose meaning is unclear.*
- 請校對這則商務訊息，並提供可直接複製貼上的完整版本。　／　*Proofread this business message and give me a clean version I can copy and paste.*
- 請精簡這封電子郵件，但凡是原本已通順的措辭都盡量保留。　／　*Tighten this email while preserving my phrasing wherever it already works.*

標籤 — 輸入：`現有草稿`、`商務電子郵件`、`專業訊息`、`休閒訊息`、`收件對象`；輸出：`潤飾後全文`、`判斷性修改清單`、`待確認問題`；工具：（無）；依賴：`cringe-check`、`ai-written-check`、`full-review`

限制：

- 只處理作者已寫好的草稿，不會根據筆記起草內容或代筆。
- 不進行語氣、定位、是否自大或是否像仿寫等評估；這些屬於 `cringe-check`。
- 不檢查更深層的 AI 寫作模式；這些屬於 `ai-written-check`，但破折號與空泛贅詞仍在處理範圍內。
- 不提供多輪完整審閱、前後差異比較或確認關卡；這些屬於 `full-review`。
- 需要提供現有草稿；若無法判斷用途或對象，可能需要說明內容將傳給誰。

| 審核狀態 | 審核人 | 審核日期 | 備註 |
| --- | --- | --- | --- |
| **待審** | | | |

### C-9. `ai-written-check`（writing／精選）

- 來源：[https://github.com/cabbagecachekid/neon-jetpack](https://github.com/cabbagecachekid/neon-jetpack) @ `4c0f62e17c00`，`skills/ai-written-check/SKILL.md`
- 生成：本次新呼叫｜模型 `gpt-5.6-sol`｜prompt `enrich-skill/v2`｜2026-08-15

> 此技能會檢查履歷、求職信、作品集、案例研究、提案、個人簡介等文案，找出容易讓文字顯得由 AI 生成的機械式句法與用詞模式。使用者需要提供待檢查的散文式文案；技能會逐項列出規則名稱、問題原文與具體改寫，最後給出「乾淨」、「輕微修整」或「需大幅調整」的結論。它會依模式的重複次數與既定門檻判斷，而不是一律刪除所有類似表達。

任務範例句：

- 請檢查我的求職信是否有 AI 寫作痕跡，並為每個問題提出具體改寫。　／　*Check my cover letter for AI-written tells and suggest a concrete rewrite for every issue.*
- 請檢查這個作品集頁面是否過度使用破折號、通用範本文案或新創圈俚語。　／　*Review this portfolio page for em-dash overuse, generic template copy, and startup slang.*
- 這篇案例研究讀起來像 AI 生成的嗎？請引用每個機械式寫作痕跡，並示範如何修改。　／　*Does this case study sound AI-generated? Quote each mechanical tell and show me how to fix it.*
- 請掃描我的專業簡介，找出重複的「不是 X，而是 Y」句型與無法驗證的普遍性主張。　／　*Scan my professional bio for repeated “not X, but Y” constructions and unsupported universal claims.*
- 請對這份提案進行 AI 寫作檢查，最後判定為乾淨、輕微修整或需大幅調整。　／　*Give this proposal an AI-written check and finish with a clean, light touch-up, or heavy verdict.*

標籤 — 輸入：`human-facing prose`、`resume`、`cover letter`、`portfolio page`、`case study`、`proposal`、`bio`；輸出：`pattern findings`、`quoted text`、`specific rewrites`、`review verdict`；工具：（無）；依賴：（無）

限制：

- 不判斷語氣、自大感或定位；這類檢查應使用 `cringe-check`。
- 不驗證事實或主張；這部分屬於 `full-review`。
- 不會全面重寫文章，只會標記問題並提出具體改寫，由作者決定是否採用。
- 此技能適用於面向讀者的散文式文案，並要求依各模式的門檻與出現次數判定。

| 審核狀態 | 審核人 | 審核日期 | 備註 |
| --- | --- | --- | --- |
| **待審** | | | |

### C-10. `data-analyst`（data／精選）

- 來源：[https://github.com/nqumich/data-analyst-skill](https://github.com/nqumich/data-analyst-skill) @ `0ba9d17ed275`，`SKILL.md`
- 生成：重用 golden set 既有增強產出｜模型 `gpt-5.6-sol`｜prompt `enrich-skill/v1`｜2026-08-15
- ⚠️ **來源 SKILL.md 為簡體中文**；下列摘要與範例句即繁中化呈現層（見 §1.3）。

> 此技能可處理 CSV、TSV、XLSX、JSON 與 JSONL 資料，執行讀取、清理、篩選、彙總、連接、驗證及基本統計分析。它特別支援整理複雜 Excel 活頁簿，包括合併儲存格、多層表頭、空白列欄、格式化數字、混合日期及多工作表，也能以分塊方式處理大型 CSV。使用者需提供資料檔案、分析問題，以及視需要指定欄位、篩選條件、驗證結構、工作表或輸出路徑；結果可輸出為整理後的資料、品質檢查、錯誤清單、彙總結果或分析報告。

任務範例句：

- 請清理這份 Excel 活頁簿：合併兩層表頭、填補合併儲存格、將貨幣與百分比欄位轉成數值，並另存為 CSV。　／　*Clean this Excel workbook by flattening its two-row header, filling merged cells, converting currency and percentage fields to numbers, and saving the result as CSV.*
- 請篩選 sales.csv 中金額大於 1,000 的資料，再依類別彙總金額總和。　／　*Filter sales.csv to rows where amount is greater than 1,000, then summarize the total amount by category.*
- 請用 customer_id 對 orders.csv 與 customers.csv 執行左連接，並將合併結果儲存為 merged.csv。　／　*Join orders.csv and customers.csv on customer_id using a left join, and save the merged data to merged.csv.*
- 請檢查這份資料集的重複列、空值、資料型別與數值欄位摘要統計。　／　*Check this dataset for duplicate rows, missing values, data types, and numeric summary statistics.*
- 請分塊處理這個大型 CSV，保留金額高於 100 的紀錄、新增 amount_usd 欄位，並將結果寫入新檔案。　／　*Process this large CSV in chunks, keep records with amount above 100, add an amount_usd column, and write the results to a new file.*

標籤 — 輸入：`csv files`、`tsv files`、`xlsx workbooks`、`json files`、`jsonl files`、`filter expressions`、`column names`、`validation schemas`、`sheet names`、`analysis questions`、`output paths`；輸出：`pandas dataframes`、`cleaned csv files`、`cleaned xlsx files`、`aggregated data`、`joined data`、`data quality reports`、`validation errors`、`descriptive statistics`、`analysis reports`；工具：`python scripts`、`data_ops.py`、`command line`；依賴：`python`、`pandas`、`openpyxl`

限制：**欄位不存在**（本筆為 `enrich-skill/v1` 產出，該版本尚無 `limitations`；見 §2.2）

> 🎯 **閘門卡 `DAT-3` 的 gold primary**（`gate-test/task-cards.md` §3）。本筆的摘要品質會被閘門直接檢驗。

> **抽查預判：需修改** — 缺 `limitations` 欄位（`enrich-skill/v1` 產出），不滿足 02 §DISC-003 的「限制」允收。摘要文字本身未見問題。處置＝以 v2 重跑本筆（§4.2）。

| 審核狀態 | 審核人 | 審核日期 | 備註 |
| --- | --- | --- | --- |
| **待審** | | | |

### C-11. `data-cleanliness-scan`（data／精選）

- 來源：[https://github.com/danielrosehill/Claude-Data-Wrangler-plugin](https://github.com/danielrosehill/Claude-Data-Wrangler-plugin) @ `b12805a62307`，`skills/data-cleanliness-scan/SKILL.md`
- 生成：重用 golden set 既有增強產出｜模型 `gpt-5.6-sol`｜prompt `enrich-skill/v1`｜2026-08-15

> 掃描一個或多個 CSV、Parquet、JSON、JSONL 或 Excel 檔案，評估資料整潔度及找出可能導致 SQL 匯入失敗或分析錯誤的問題。它會檢查欄位型別、日期、空值、重複鍵、編碼、分隔符、列結構、值域及跨欄邏輯等，並按嚴重程度排序。使用者需提供資料檔案、確認格式及目標系統；輸出為含問題範例與修正建議的 Markdown 報告，亦可選擇產生 JSON 版本。

任務範例句：

- 在我把這些 CSV 檔案匯入 PostgreSQL 前，請先掃描資料品質問題。　／　*Scan these CSV files for data quality issues before I load them into PostgreSQL.*
- 請檢查這份 Excel 活頁簿是否有混合資料型別、錯誤日期、偽裝空值及重複鍵。　／　*Check this Excel workbook for mixed data types, malformed dates, disguised nulls, and duplicate keys.*
- 這份 JSONL 資料能順利匯入 SQL 嗎？請提供按嚴重程度排序的報告及修正建議。　／　*Will this JSONL data load cleanly into SQL? Give me a severity-ranked report with remediation suggestions.*
- 請用策略性抽樣稽核這些大型 Parquet 檔案，並在報告中註明樣本大小。　／　*Audit these large Parquet files using strategic sampling and note the sample size in the report.*
- 請找出這批資料中的分隔符漂移、欄數不齊、編碼異常及混合換行格式。　／　*Find delimiter drift, ragged rows, encoding artefacts, and mixed line endings in this data dump.*

標籤 — 輸入：`csv files`、`parquet files`、`json files`、`jsonl files`、`excel files`、`target sql system`；輸出：`cleanliness_report.md`、`cleanliness_report.json`、`ranked issue report`、`remediation suggestions`；工具：`pandas`、`chardet`、`dateutil`；依賴：`pandas`、`pyarrow`、`openpyxl`、`chardet`、`python-dateutil`

限制：**欄位不存在**（本筆為 `enrich-skill/v1` 產出，該版本尚無 `limitations`；見 §2.2）

> **抽查預判：需修改** — 缺 `limitations` 欄位（`enrich-skill/v1` 產出），不滿足 02 §DISC-003 的「限制」允收。摘要文字本身未見問題。處置＝以 v2 重跑本筆（§4.2）。

| 審核狀態 | 審核人 | 審核日期 | 備註 |
| --- | --- | --- | --- |
| **待審** | | | |

### C-12. `csv-to-json`（data／精選）

- 來源：[https://github.com/danielrosehill/Claude-Data-Wrangler-plugin](https://github.com/danielrosehill/Claude-Data-Wrangler-plugin) @ `b12805a62307`，`skills/csv-to-json/SKILL.md`
- 生成：重用 golden set 既有增強產出｜模型 `gpt-5.6-sol`｜prompt `enrich-skill/v1`｜2026-08-15

> 此技能可在 CSV、JSON 陣列與 JSONL 格式之間進行雙向轉換，並處理分隔符、引號、標頭、文字編碼、空值及資料型別。使用者需提供來源檔案、目標格式與輸出位置，並可選擇是否推斷型別，以及如何處理巢狀物件或陣列。它也會驗證輸出筆數，並針對大型檔案採用串流或分塊方式。

任務範例句：

- 請將 customers.csv 轉成 JSON 陣列，並把所有值保留為字串。　／　*Convert customers.csv to a JSON array while preserving every value as a string.*
- 請將這個以分號分隔的 CSV 轉成 JSONL，推斷數字與布林型別，並將 NA 和空白欄位視為 null。　／　*Convert this semicolon-delimited CSV file to JSONL, infer numeric and Boolean types, and treat NA and empty fields as null.*
- 請用點號鍵名攤平 orders.json 中的巢狀物件，並匯出成 CSV。　／　*Flatten the nested objects in orders.json using dotted keys and export them as CSV.*
- 請逐行將 events.jsonl 轉成 CSV，並確認輸出筆數與來源一致。　／　*Convert events.jsonl to CSV line by line and verify that the output record count matches the source.*
- 請分塊將這個大型 CSV 檔案轉成 JSONL，並回報偵測到的文字編碼。　／　*Convert this large CSV file to JSONL in chunks and report which text encoding was detected.*

標籤 — 輸入：`csv files`、`json arrays`、`jsonl files`、`file paths`、`csv dialect`、`text encoding`、`type inference options`、`null sentinels`、`nested data handling`；輸出：`csv files`、`json arrays`、`jsonl files`、`row counts`、`object counts`、`encoding reports`、`validation reports`；工具：`csv.Sniffer`、`csv`、`json`、`pd.read_csv`；依賴：`python standard library`、`pandas`

限制：**欄位不存在**（本筆為 `enrich-skill/v1` 產出，該版本尚無 `limitations`；見 §2.2）

> **抽查預判：需修改** — 缺 `limitations` 欄位（`enrich-skill/v1` 產出），不滿足 02 §DISC-003 的「限制」允收。摘要文字本身未見問題。處置＝以 v2 重跑本筆（§4.2）。

| 審核狀態 | 審核人 | 審核日期 | 備註 |
| --- | --- | --- | --- |
| **待審** | | | |

### C-13. `text-to-numeric`（data／精選）

- 來源：[https://github.com/danielrosehill/Claude-Data-Wrangler-plugin](https://github.com/danielrosehill/Claude-Data-Wrangler-plugin) @ `b12805a62307`，`skills/text-to-numeric/SKILL.md`
- 生成：重用 golden set 既有增強產出｜模型 `gpt-5.6-sol`｜prompt `enrich-skill/v1`｜2026-08-15

> 將含貨幣符號、千位分隔符、百分比、縮寫倍率或會計負號等格式的文字數值，轉換成可分析的整數或浮點數欄位。它需要資料集與目標欄位；若未指定，也可找出大多數內容看似數字的文字欄位，並在小數點、千位格式或百分比處理不明確時請使用者確認。原始格式、幣別、倍率與百分比決策會記錄到資料字典，無法解析的值則保留為空值並列出列索引。

任務範例句：

- 請將營收欄位中像「$4.27」、「$1.2M」和「(500)」的值轉成數值欄位，並保留原始值。　／　*Convert the revenue column containing values like "$4.27", "$1.2M", and "(500)" into a numeric column while preserving the original values.*
- 請找出這份資料集中內容大多可視為數字的文字欄位，並建議如何解析其千位分隔符與小數點。　／　*Find text columns in this dataset that are mostly numeric and suggest how to parse their separators and decimal markers.*
- 請將百分比欄位解析為小數比例，讓「3.5%」變成 0.035，並把這項處理方式記錄在資料字典中。　／　*Parse the percentage column as decimal fractions, so "3.5%" becomes 0.035, and record that decision in the data dictionary.*
- 請將混合幣別符號擷取到獨立的幣別欄位，並把其餘金額轉成數字，但不要進行匯率換算。　／　*Extract mixed currency symbols into a separate currency column and convert the remaining amounts to numbers without performing exchange-rate conversion.*
- 請將「~500」和「>1M」等近似或界限值轉成數字、保留其限定符，並回報所有無法解析的資料列。　／　*Convert approximations such as "~500" and ">1M" into numeric values, preserve their qualifiers, and report any rows that cannot be parsed.*

標籤 — 輸入：`dataset`、`text-formatted numeric columns`、`format preferences`、`percentage handling`；輸出：`numeric columns`、`raw value columns`、`data dictionary metadata`、`currency column`、`qualifier column`、`unparseable row report`、`numeric output dataset`；工具：`add-data-dictionary`；依賴：`pandas`、`babel`

限制：**欄位不存在**（本筆為 `enrich-skill/v1` 產出，該版本尚無 `limitations`；見 §2.2）

> **抽查預判：需修改** — 缺 `limitations` 欄位（`enrich-skill/v1` 產出），不滿足 02 §DISC-003 的「限制」允收。摘要文字本身未見問題。處置＝以 v2 重跑本筆（§4.2）。

| 審核狀態 | 審核人 | 審核日期 | 備註 |
| --- | --- | --- | --- |
| **待審** | | | |

### C-14. `excel-deduplicate`（data／精選）

- 來源：[https://github.com/YuYY2004/excel-skills](https://github.com/YuYY2004/excel-skills) @ `c15e51e20424`，`claude/skills/excel-deduplicate/SKILL.md`
- 生成：重用 golden set 既有增強產出｜模型 `gpt-5.6-sol`｜prompt `enrich-skill/v1`｜2026-08-15
- ⚠️ **來源 SKILL.md 為簡體中文**；下列摘要與範例句即繁中化呈現層（見 §1.3）。

> 此技能依指定關鍵欄位為 Excel 資料去重，可選擇保留首次或末次出現的資料列，並刪除其餘重複列。它需要提供 .xlsx 檔案、關鍵欄位名稱及保留規則；流程會先唯讀掃描並列出重複情況，再備份檔案、直接修改工作表 XML、重新封裝，最後驗證殘留重複與公式中的 #REF! 錯誤。此方式只移除資料列，保留其餘內容與格式不變。

任務範例句：

- 請依「客戶編號」欄位移除這份 Excel 的重複資料列，保留第一次出現的資料。　／　*Remove duplicate rows from this Excel file by the customer ID column, keeping the first occurrence.*
- 請按電子郵件地址為試算表去重，每個地址保留最後一筆紀錄。　／　*Deduplicate the spreadsheet by email address and keep the last record for each address.*
- 請掃描這份活頁簿中的重複訂單編號，顯示預計刪除的列數，然後執行去重。　／　*Scan this workbook for duplicate order numbers, show me how many rows will be removed, and then deduplicate it.*
- 請清理這份 XLSX 檔案中的重複產品代碼、建立備份，並確認沒有殘留重複資料或 #REF! 錯誤。　／　*Clean duplicate product codes from this XLSX file, create a backup, and verify that no duplicates or #REF! errors remain.*

標籤 — 輸入：`xlsx file`、`key column`、`keep rule`；輸出：`deduplicated xlsx file`、`backup xlsx file`、`duplicate scan report`、`validation report`；工具：`pandas read_excel`、`xml row deletion`、`zipfile`、`formula health check`；依賴：`python`、`pandas`、`lxml`、`openpyxl`、`excel-find-duplicates`、`excel-delete`、`excel-safe-workflow`

限制：**欄位不存在**（本筆為 `enrich-skill/v1` 產出，該版本尚無 `limitations`；見 §2.2）

> 🎯 **閘門卡 `DAT-4` 的 gold primary**（`gate-test/task-cards.md` §3）。本筆的摘要品質會被閘門直接檢驗。

> **抽查預判：需修改** — 缺 `limitations` 欄位（`enrich-skill/v1` 產出），不滿足 02 §DISC-003 的「限制」允收。摘要文字本身未見問題。處置＝以 v2 重跑本筆（§4.2）。

| 審核狀態 | 審核人 | 審核日期 | 備註 |
| --- | --- | --- | --- |
| **待審** | | | |

### C-15. `excel-find-duplicates`（data／精選）

- 來源：[https://github.com/YuYY2004/excel-skills](https://github.com/YuYY2004/excel-skills) @ `c15e51e20424`，`claude/skills/excel-find-duplicates/SKILL.md`
- 生成：本次新呼叫｜模型 `gpt-5.6-sol`｜prompt `enrich-skill/v2`｜2026-08-15
- ⚠️ **來源 SKILL.md 為簡體中文**；下列摘要與範例句即繁中化呈現層（見 §1.3）。

> 此技能以唯讀方式掃描 Excel 檔案，依指定的單一欄位或多欄組合找出重複資料，且不會修改原始檔案。使用者需要提供 Excel 檔案與明確的關鍵欄位，並可指定保留第一筆或最後一筆；結果包含重複統計與 Excel 列號清單。列號清單可交給 excel-delete，由下往上刪除重複列。

任務範例句：

- 請依專利號找出這個 Excel 檔案中的重複列，並保留第一次出現的資料。　／　*Find duplicate rows in this Excel file by patent number and keep the first occurrence.*
- 請以客戶編號和訂單日期的組合作為條件查重，並回傳重複資料的 Excel 列號。　／　*Check for duplicates using the combination of customer ID and order date, then return the Excel row numbers.*
- 請掃描 E 欄的重複值，並保留每個值最後一次出現的資料。　／　*Scan column E for duplicate values and keep the last occurrence of each value.*
- 請顯示這份活頁簿的重複資料統計，以及前 20 個重複列號。　／　*Show me the duplicate statistics and the first 20 duplicate row numbers in this workbook.*
- 請依電子郵件地址找出重複資料，並提供可傳給 excel-delete 的列號清單。　／　*Find duplicates by email address and provide a row list that I can pass to excel-delete.*

標籤 — 輸入：`excel file`、`key columns`、`keep strategy`；輸出：`duplicate statistics`、`excel row numbers`、`duplicate row list`；工具：`python`、`pandas read_excel`；依賴：`pandas`、`excel-delete`、`excel-safe-workflow`

限制：

- 必須明確指定用於查重的關鍵欄位，可為單欄或多欄組合。
- 此技能僅進行唯讀掃描，不會修改或刪除原始 Excel 檔案；如需刪除重複列，需另行搭配 excel-delete。
- 輸出的列號以第 1 列為標題列、第 2 列為第一筆資料。
- 關鍵欄位中有多列空值時，這些列會被視為重複；預設只保留第一筆。
- 大型檔案可能需要較長讀取時間；文件舉例指出，168MB、33 萬列的檔案約需 150 秒。

| 審核狀態 | 審核人 | 審核日期 | 備註 |
| --- | --- | --- | --- |
| **待審** | | | |

---

## 6. 已索引（indexed）30 筆

### I-1. `document-format-skills`（documents／已索引）

- 來源：[https://github.com/KaguraNanaga/document-format-skills](https://github.com/KaguraNanaga/document-format-skills) @ `cbdd11249b6d`，`SKILL.md`
- 生成：本次新呼叫｜模型 `gpt-5.6-sol`｜prompt `enrich-skill/v2`｜2026-08-15

> 此技能透過命令列診斷、清理及格式化中文 Word 文件，可修正中英文標點與間距、套用公文／學術／法律／自訂格式、整理表格與頁碼，並處理修訂標記。它需要輸入 `.docx` 文件，或在符合環境要求時輸入 `.doc`／`.wps`；也能將純文字或 Markdown 轉成格式化的 DOCX，並支援 JSON 自訂設定與批次腳本流程。

任務範例句：

- 請完整清理這份中文 DOCX，並套用公文格式預設。　／　*Clean up this Chinese DOCX end to end and apply the official-document preset.*
- 請分析這份 Word 文件的格式問題，並以 JSON 回傳診斷結果。　／　*Analyze this Word document’s formatting problems and return the diagnostic results as JSON.*
- 請修正中英文混排的標點與間距，並在中英文或數字交界處保留一個空格。　／　*Fix mixed Chinese and English punctuation and spacing while keeping one space at language boundaries.*
- 請將這份 Markdown 工作計畫轉成格式化 DOCX，標題設為「工作方案」。　／　*Convert this Markdown work plan into a formatted DOCX with the title “工作方案”.*
- 請將這些法律文件套用法律格式、整理表格、加入置中頁碼，並保留修訂標記。　／　*Format these legal documents, normalize their tables, add centered page numbers, and preserve revision marks.*

標籤 — 輸入：`docx documents`、`doc documents`、`wps documents`、`plain text`、`markdown`、`custom settings json`、`desktop schema v2 configs`、`exported preset files`；輸出：`formatted docx documents`、`formatted doc documents`、`formatted wps documents`、`diagnostic reports`、`json diagnostics`、`revision marks`、`page numbers`、`normalized tables`；工具：`command line`、`scripts/process.py`、`scripts/formatter.py`、`scripts/punctuation.py`、`scripts/from_text.py`、`scripts/analyzer.py`、`scripts/converter.py`、`uv`；依賴：`python-docx`、`pywin32`、`windows`、`wps office`、`microsoft word`、`macos system fonts`

限制：

- 處理 `.docx` 需要安裝 `python-docx`。
- 轉換 `.doc` 或 `.wps` 僅支援 Windows，並需要安裝 WPS Office 或 Microsoft Word，以及 `pywin32`。
- `custom` 預設需在可取得桌面版自訂預設時使用；也可透過 `--custom-settings` 提供相容的 JSON 設定。
- Markdown 模式僅明確支援標題、粗體、排序與非排序清單、引用及圍欄程式碼區塊。

| 審核狀態 | 審核人 | 審核日期 | 備註 |
| --- | --- | --- | --- |
| **待審** | | | |

### I-2. `course-quiz-builder`（documents／已索引）

- 來源：[https://github.com/the3ma/course-quiz-builder](https://github.com/the3ma/course-quiz-builder) @ `e2ac75e52a9a`，`skills/course-quiz-builder/SKILL.md`
- 生成：本次新呼叫｜模型 `gpt-5.6-sol`｜prompt `enrich-skill/v2`｜2026-08-15

> 此技能會根據課程名稱與 Markdown 課程內容設計題目，產生並驗證一個可直接開啟的自包含測驗 HTML 頁面。測驗可隨機排列題目與選項、讓學員提交前檢查答案，並提供通過／未通過結果、逐題解析、主題得分、重試、計時與進度保存等功能。它也能選擇性地將測驗發佈至 GitHub Pages，或把作答結果複製成 JSON、提交至指定端點。

任務範例句：

- 請根據這份新人訓練 Markdown 檔建立一份 20 題、及格分數為 80% 的知識測驗。　／　*Create a 20-question knowledge check from this onboarding Markdown file with an 80% passing score.*
- 請把這五堂課轉成會隨機排列題目、附答案解析與各主題得分的測驗。　／　*Turn these five course lessons into a randomized quiz with explanations and scores by topic.*
- 請根據這份課程教材建立自包含的 quiz.html，並確認它通過必要的自我測試。　／　*Build a self-contained quiz.html from this course material and verify that it passes the required self-test.*
- 請將這份已完成的測驗發佈到 GitHub Pages，並提供上線網址。　／　*Publish this completed quiz to GitHub Pages and give me the live URL.*
- 請加入 30 分鐘計時器，並設定測驗在評分後將結果資料 POST 到這個端點。　／　*Add a 30-minute timer and configure the quiz to POST result data to this endpoint after grading.*

標籤 — 輸入：`course name`、`markdown course content`、`questions.json`、`quiz configuration`、`submit endpoint`；輸出：`self-contained quiz html`、`answer review`、`pass/fail verdict`、`topic scores`、`quiz result json`、`github pages url`；工具：`build-quiz.mjs`、`selftest.mjs`、`browser-check.mjs`、`publish-pages.mjs`、`publish-selftest.mjs`、`github pages`、`localstorage`、`clipboard`、`http post`；依賴：`node.js`、`github account`、`github token`、`github pro`、`headless chromium`

限制：

- 不適用於撰寫課程內容，也不適用於建立單元測試或端對端程式測試。
- 題目必須完全依據提供的 Markdown 內容；若內容不足以支援指定題數，必須減少題數。
- 建置與驗證需要使用 Node.js 執行套件腳本，且交付前必須讓 selftest.mjs 對產生的 HTML 輸出 PASS。
- 答案金鑰僅經過混淆，並不具安全性或防竄改能力；公開儲存庫也會以純文字暴露 questions.json。
- 發佈至 GitHub Pages 需要有效的 GitHub 權杖及足夠權限；私人儲存庫的 Pages 功能需要 GitHub Pro。
- 真實瀏覽器檢查需要可用的無頭 Chromium；若沒有瀏覽器，該檢查會略過。

| 審核狀態 | 審核人 | 審核日期 | 備註 |
| --- | --- | --- | --- |
| **待審** | | | |

### I-3. `docx`（documents／已索引）

- 來源：[https://github.com/anthropics/skills](https://github.com/anthropics/skills) @ `f6656c1256d5`，`skills/docx/SKILL.md`
- 生成：重用 golden set 既有增強產出｜模型 `gpt-5.6-sol`｜prompt `enrich-skill/v1`｜2026-08-15

> 此技能用於建立、讀取、編輯及整理 Word 文件（.docx）與範本（.dotx），可處理標題、目錄、頁碼、表格、圖片、清單及版面格式。它也能從既有文件擷取或取代內容、加入註解與追蹤修訂、接受修訂，以及將舊式 .doc 轉換後再處理。使用時需提供要製作的內容與格式需求，或提供既有的 Word 文件及具體修改要求；完成後可驗證文件結構並轉成 PDF 與頁面圖片檢查外觀。

任務範例句：

- 請建立一份專業的 Word 報告，包含目錄、編號標題、頁碼及格式化表格。　／　*Create a polished Word report with a table of contents, numbered headings, page numbers, and formatted tables.*
- 請在這份 DOCX 文件中全面替換公司名稱並插入新標誌，但不要改變原有版面。　／　*Replace the company name throughout this DOCX file and insert the new logo without changing the existing layout.*
- 請將這份 Word 文件的內容擷取成 Markdown，並依標題重新整理。　／　*Extract the content from this Word document into Markdown and reorganize it by heading.*
- 請以我的姓名追蹤修訂這份合約，在費用條款加入註解，並另提供一份接受所有修訂的乾淨版本。　／　*Redline this contract under my name, add comments to the fee clauses, and provide a clean copy with all changes accepted.*
- 請將這份舊式 DOC 文件轉成 DOCX、修正格式，並以頁面預覽檢查最終外觀。　／　*Convert this legacy DOC file to DOCX, fix the formatting, and verify the final pages visually.*

標籤 — 輸入：`docx files`、`dotx files`、`doc files`、`document content`、`formatting requirements`、`images`、`replacement text`、`comments`、`tracked changes`；輸出：`docx documents`、`edited word documents`、`word templates`、`markdown content`、`redlined documents`、`annotated documents`、`accepted-changes copies`、`pdf previews`、`jpeg page images`；工具：`docx-js`、`pandoc`、`soffice.py`、`pdftoppm`、`unzip`、`zip`、`merge_runs.py`、`validate.py`、`comment.py`、`accept_changes.py`；依賴：`docx npm package`、`pandoc`、`libreoffice`、`poppler`

限制：**欄位不存在**（本筆為 `enrich-skill/v1` 產出，該版本尚無 `limitations`；見 §2.2）

| 審核狀態 | 審核人 | 審核日期 | 備註 |
| --- | --- | --- | --- |
| **待審** | | | |

### I-4. `pdf`（documents／已索引）

- 來源：[https://github.com/anthropics/skills](https://github.com/anthropics/skills) @ `f6656c1256d5`，`skills/pdf/SKILL.md`
- 生成：重用 golden set 既有增強產出｜模型 `gpt-5.6-sol`｜prompt `enrich-skill/v1`｜2026-08-15

> 此技能可讀取、建立及處理 PDF，包括擷取文字、表格、圖片與中繼資料，以及合併、分割、旋轉、加浮水印、填寫表單和密碼保護。它也能以 OCR 將掃描式 PDF 轉成可搜尋文字，並可把擷取的表格匯出為 Excel。使用者需提供一個或多個 PDF，並視任務提供頁碼範圍、密碼、浮水印檔案、表單資料或要寫入新 PDF 的內容。

任務範例句：

- 請從這份 PDF 擷取文字與表格，並將表格儲存為 Excel 檔案。　／　*Extract the text and tables from this PDF and save the tables as an Excel file.*
- 請依照提供的順序，將這三個 PDF 合併成一份文件。　／　*Merge these three PDF files into one document in the order provided.*
- 請對這份掃描式 PDF 執行 OCR，讓其中的文字可以搜尋及擷取。　／　*Run OCR on this scanned PDF so its text can be searched and extracted.*
- 請將第 1 頁順時針旋轉 90 度，並在每一頁加入這個浮水印。　／　*Rotate page 1 clockwise by 90 degrees and add this watermark to every page.*
- 請使用不同的使用者密碼與擁有者密碼保護這份 PDF。　／　*Password-protect this PDF using separate user and owner passwords.*

標籤 — 輸入：`pdf files`、`scanned pdf`、`page ranges`、`passwords`、`watermark pdf`、`form data`、`document content`；輸出：`extracted text`、`extracted tables`、`excel workbook`、`pdf metadata`、`merged pdf`、`split pdfs`、`rotated pdf`、`watermarked pdf`、`encrypted pdf`、`decrypted pdf`、`searchable text`、`extracted images`、`filled pdf form`、`new pdf`；工具：`pdftotext`、`qpdf`、`pdftk`、`pdfimages`；依賴：`python`、`pypdf`、`pdfplumber`、`pandas`、`reportlab`、`pytesseract`、`pdf2image`、`poppler-utils`、`pdf-lib`、`pypdfium2`

限制：**欄位不存在**（本筆為 `enrich-skill/v1` 產出，該版本尚無 `limitations`；見 §2.2）

| 審核狀態 | 審核人 | 審核日期 | 備註 |
| --- | --- | --- | --- |
| **待審** | | | |

### I-5. `pptx`（documents／已索引）

- 來源：[https://github.com/anthropics/skills](https://github.com/anthropics/skills) @ `f6656c1256d5`，`skills/pptx/SKILL.md`
- 生成：重用 golden set 既有增強產出｜模型 `gpt-5.6-sol`｜prompt `enrich-skill/v1`｜2026-08-15

> 此技能可建立、讀取、解析、編輯、合併、拆分及套用範本處理 PowerPoint 簡報，並支援投影片版面、圖表、講者備註與註解。它需要現有的 .pptx／.potx 檔案，或製作新簡報所需的內容、資料與設計要求。處理流程也涵蓋文字擷取、結構驗證、內容檢查，以及將投影片轉成圖片進行版面與溢位檢查。

任務範例句：

- 請根據這份大綱與銷售資料製作一份精緻的 PowerPoint 提案簡報，並加入原生圖表與講者備註。　／　*Create a polished PowerPoint pitch deck from this outline and sales data, including native charts and speaker notes.*
- 請以新數據更新這份 PPTX，保留原有格式，並移除未使用的投影片與範例佔位文字。　／　*Update this PPTX with the new figures, preserve its formatting, and remove any unused slides and placeholder text.*
- 請擷取這份簡報中的文字與講者備註，並依投影片順序逐頁摘要。　／　*Extract the text and speaker notes from this presentation and summarize each slide in order.*
- 請使用這個 POTX 範本與我提供的內容製作簡報，在維持範本風格的同時變化各頁版面。　／　*Use this POTX template to build a presentation from my content, varying the layouts while keeping the template style.*
- 請合併這些 PowerPoint 檔案、驗證合併結果，並檢查每張投影片是否有文字溢位、物件重疊或殘留佔位內容。　／　*Combine these PowerPoint files, validate the result, and check every slide for overflow, overlap, and leftover placeholders.*

標籤 — 輸入：`pptx files`、`potx templates`、`ppt files`、`slide content`、`chart data`、`design requirements`；輸出：`pptx presentations`、`potx templates`、`extracted text`、`pdf files`、`slide images`、`validation reports`、`thumbnail grids`；工具：`node`、`python`、`shell`、`markitdown`、`thumbnail.py`、`add_slide.py`、`clean.py`、`validate.py`、`soffice.py`、`pdftoppm`；依賴：`pptxgenjs`、`markitdown[pptx]`、`pillow`、`defusedxml`、`lxml`、`libreoffice`、`poppler`、`react-icons`、`react`、`react-dom`、`sharp`

限制：**欄位不存在**（本筆為 `enrich-skill/v1` 產出，該版本尚無 `limitations`；見 §2.2）

| 審核狀態 | 審核人 | 審核日期 | 備註 |
| --- | --- | --- | --- |
| **待審** | | | |

### I-6. `xlsx`（documents／已索引）

- 來源：[https://github.com/anthropics/skills](https://github.com/anthropics/skills) @ `f6656c1256d5`，`skills/xlsx/SKILL.md`
- 生成：重用 golden set 既有增強產出｜模型 `gpt-5.6-sol`｜prompt `enrich-skill/v1`｜2026-08-15

> 此技能用於建立、讀取、編輯、修復、清理、分析或轉換試算表，支援 Excel、CSV 與 TSV 等表格檔案，也能加入公式、格式與圖表。使用者需提供現有試算表或資料來源，並說明所需欄位、工作表名稱、計算公式、格式或轉換要求；最終交付成果必須是試算表檔案。它也會重新計算並檢查公式錯誤、保留既有檔案慣例，並可依指定規則製作財務模型。

任務範例句：

- 請清理這個 CSV 中格式錯亂的資料列，把欄位標題移到正確位置，並將結果交付為 XLSX 檔案。　／　*Please clean the malformed rows in this CSV, move the headers to the correct row, and deliver the result as an XLSX file.*
- 請在附件活頁簿中以公式新增「總計」欄，沿用現有格式，並保留所有既有公式不變。　／　*Add a Total column to the attached workbook using formulas, match the existing formatting, and keep all current formulas unchanged.*
- 請根據這些假設建立財務預測活頁簿，將各項輸入分別放在獨立儲存格，套用百分比格式，並為每個預測年度加入公式。　／　*Create a financial projection workbook from these assumptions, with separate input cells, percentage formatting, and formulas for each forecast year.*
- 請將這個 TSV 檔轉換成具專業格式的 Excel 活頁簿，並新增一張彙總每月銷售額的圖表。　／　*Convert this TSV file to a professionally formatted Excel workbook and add a chart summarizing monthly sales.*
- 請開啟我下載資料夾中的 XLSM 檔，將這些數值填入指定的輸入儲存格、保留巨集，並重新計算所有公式。　／　*Open the XLSM file in my downloads, update the designated input cells with these values, preserve its macros, and recalculate all formulas.*

標籤 — 輸入：`xlsx files`、`xlsm files`、`xltx files`、`csv files`、`tsv files`、`tabular data`、`spreadsheet specifications`、`formulas`、`formatting requirements`；輸出：`spreadsheet files`、`formatted workbooks`、`calculated formulas`、`charts`、`cleaned tabular data`、`financial models`；工具：`openpyxl`、`pandas read_excel`、`pandas to_excel`、`markitdown`、`recalc.py`、`libreoffice`；依賴：`openpyxl`、`pandas`、`markitdown`、`libreoffice`、`soffice`

限制：**欄位不存在**（本筆為 `enrich-skill/v1` 產出，該版本尚無 `limitations`；見 §2.2）

| 審核狀態 | 審核人 | 審核日期 | 備註 |
| --- | --- | --- | --- |
| **待審** | | | |

### I-7. `cringe-check`（writing／已索引）

- 來源：[https://github.com/cabbagecachekid/neon-jetpack](https://github.com/cabbagecachekid/neon-jetpack) @ `4c0f62e17c00`，`skills/cringe-check/SKILL.md`
- 生成：本次新呼叫｜模型 `gpt-5.6-sol`｜prompt `enrich-skill/v2`｜2026-08-15

> 此技能會審查履歷、求職信、作品集、提案或推銷文案的語氣與定位，找出自大、刻意討好、鸚鵡式照抄、單打獨鬥式敘事、貶低他人、越權替客戶下結論，以及誇大權責或規模等問題。使用者需提供要檢查的文案；技能會按問題面向列出原句，並提供更具合作感的改寫。它也會檢查是否適當呈現好奇心、未解問題、共同成果，以及是否不當削弱客製化交付成果。

任務範例句：

- 可以幫我的求職信做一次尷尬感檢查，並改寫聽起來自大或過度討好的句子嗎？　／　*Can you give my cover letter a cringe check and rewrite any lines that sound arrogant or overly eager to please?*
- 請檢查這份履歷是否把我寫成單打獨鬥的英雄，並確保產品與工程夥伴得到適當肯定。　／　*Review this resume for solo-hero framing and make sure I give proper credit to my product and engineering partners.*
- 請檢查我的應徵文案是否照抄了太多職缺描述中的標誌性用語。　／　*Check whether my application copies too many distinctive phrases from the job description.*
- 請審查這份客戶提案是否武斷判定對方的業務狀況，並改寫成聚焦於研究可以釐清的問題。　／　*Audit this client proposal for presumptuous claims about their business and rewrite them to focus on what the research can uncover.*
- 這篇作品集案例是否誇大了我的權責，或把週末專案寫得像與企業級工作同等規模？　／　*Does this portfolio case study overstate my authority or make a weekend project sound equivalent to enterprise work?*

標籤 — 輸入：`履歷`、`求職信`、`作品集頁面`、`提案`、`推銷文案`、`職缺描述`、`專業文案`；輸出：`語氣與定位審查`、`問題面向`、`冒犯原句`、`合作語氣改寫`、`規模問題說明`；工具：（無）；依賴：`references/POSITIONING.md`、`ai-written-check`、`full-review`

限制：

- 不檢查句子結構、文字機械感或 AI 寫作痕跡；這類檢查應使用 `ai-written-check`。
- 不驗證內容事實；事實查核應使用 `full-review`。
- 不會為了放大成果而膨脹描述；若實際規模較小，會如實指出。

| 審核狀態 | 審核人 | 審核日期 | 備註 |
| --- | --- | --- | --- |
| **待審** | | | |

### I-8. `full-review`（writing／已索引）

- 來源：[https://github.com/cabbagecachekid/neon-jetpack](https://github.com/cabbagecachekid/neon-jetpack) @ `4c0f62e17c00`，`skills/full-review/SKILL.md`
- 生成：本次新呼叫｜模型 `gpt-5.6-sol`｜prompt `enrich-skill/v2`｜2026-08-15

> 此 Skill 依序用六個面向審查履歷、求職信、作品集頁面、案例研究、提案與個人簡介等專業文案，包括 AI 寫作痕跡、實作者定位、誠實與誇大、語氣、語域及事實查證。使用者需提供待審文案、原始資料與可核實事實的來源；結果會以修改前後對照表說明每項建議，並列出刻意未納入的內容及選項建議。它支援反覆討論修改，但在作者明確批准前不會更動正式檔案。

任務範例句：

- 請完整審查我的履歷，並標出任何看起來超出我實際職責範圍的說法。　／　*Please run a full review of my resume and flag any claims that sound broader than my actual responsibilities.*
- 請審查這封求職信的 AI 寫作痕跡、語氣、語域一致性，以及需要查證的事實。　／　*Review this cover letter for AI-written tells, tone, voice consistency, and facts that need verification.*
- 請協助修改這篇作品集案例研究，並用附理由的修改前後對照表呈現每項建議。　／　*Workshop this portfolio case study and show every proposed edit in a before-and-after table with reasons.*
- 請完整審查我的客戶提案，推薦最合適的措辭，並列出你刻意不納入的內容。　／　*Do a full pass on my client proposal, recommend the strongest wording, and list what you deliberately leave out.*
- 請審查我的專業簡介，但在我明確批准修改前不要變更正式檔案。　／　*Review my professional bio without changing the live file until I explicitly approve the edits.*

標籤 — 輸入：`professional copy`、`resumes`、`cover letters`、`portfolio pages`、`case studies`、`proposals`、`bios`、`source material`、`fact sources`、`author feedback`；輸出：`six-pass review`、`before-after diff table`、`change reasoning`、`omitted content list`、`recommended options`、`fact verification flags`；工具：`ai-written-check`、`cringe-check`；依賴：`neon-jetpack`、`references/passes.md`

限制：

- 在作者明確表示「land it」或同等確認前，不會變更正式檔案。
- 完整審查需搭配 neon-jetpack 套件中的 ai-written-check 與 cringe-check；若未安裝，僅使用 references/PASSES.md 提供的簡短替代檢查。
- 可驗證的日期、名稱、職稱、指標與技術術語需要來源；沒有來源的事實會標示為「傳送前驗證」。
- 不會在未經明確確認的情況下擴大原始陳述的權責、規模、團隊範圍或創始地位。

| 審核狀態 | 審核人 | 審核日期 | 備註 |
| --- | --- | --- | --- |
| **待審** | | | |

### I-9. `copyright-creative-work`（writing／已索引）

- 來源：[https://github.com/cabbagecachekid/neon-jetpack](https://github.com/cabbagecachekid/neon-jetpack) @ `4c0f62e17c00`，`skills/copyright-creative-work/SKILL.md`
- 生成：本次新呼叫｜模型 `gpt-5.6-sol`｜prompt `enrich-skill/v2`｜2026-08-15

> 此技能協助使用者依照美國著作權制度，記錄、註冊及管理自己的歌曲、歌詞、文章、照片等原創作品。它會根據作品類型、發表時間、人類與 AI 的創作部分、共同作者分配，以及是否涉及取樣、翻唱或授權，提供下一步行動、申請準備清單與分潤表範本。內容也說明音樂作品與錄音著作的區別、團體註冊條件、版稅機構，以及何時應尋求智慧財產權律師協助。

任務範例句：

- 請幫我在發行原創歌曲前，準備向美國著作權局申請註冊。　／　*Help me prepare to register my original song with the US Copyright Office before I release it.*
- 歌詞是我寫的，但旋律與音訊由 Suno 生成；申請時我應主張哪些部分，又該揭露什麼？　／　*I wrote the lyrics, but Suno generated the melody and audio. What should I claim and disclose in the application?*
- 請為我和兩位共同創作者合寫的歌曲整理分潤表填寫清單。　／　*Create a split sheet checklist for a song I co-wrote with two collaborators.*
- 我想發行一首附有音樂影片的翻唱歌曲，需要取得哪些授權？　／　*I want to release a cover song with a music video. Which licenses do I need?*
- 我可以把多張未發表照片或多首專輯曲目一起註冊嗎？申請前需要準備哪些資料？　／　*Can I register several unpublished photographs or album tracks together, and what information should I gather first?*

標籤 — 輸入：`own creative work`、`work type`、`publication date`、`authorship details`、`ai-generated material`、`human-authored elements`、`collaborator shares`、`sample details`、`cover song details`、`licensing status`；輸出：`copyright action guidance`、`registration preparation checklist`、`split sheet`、`ai disclosure guidance`、`license requirements`、`escalation guidance`；工具：`copyright.gov`、`eco`、`split-sheet template`、`filing-prep checklist`、`easy song`、`harry fox songfile`、`distrokid`、`tunecore`、`cd baby`、`ascap`、`bmi`、`sesac`、`gmr`、`the mlc`、`copyright claims board`；依賴：`us copyright law`、`copyright.gov`、`eco electronic system`、`standard application`、`registration fee`、`work deposit`、`human authorship`、`sample clearances`、`mechanical license`、`sync license`

限制：

- 內容僅供資訊參考，不構成法律意見，也不建立律師與客戶關係。
- 內容以美國著作權制度為主；其他司法管轄區的規則與結果可能不同。
- 僅用於保護與管理使用者自己的創作，不用於重製、檢查或評估他人的受著作權保護素材。
- 著作權不能保護想法、事實、標題、名稱、短語、口號、方法、系統、流程或程序，只能保護具體表達。
- 若要取得註冊效力，須透過 copyright.gov 的 eCO 系統提交申請、費用及作品副本。
- 含 AI 生成素材的作品須使用 Standard Application 逐件申請，不能使用 GRUW、GRAM 或其他團體註冊方式。
- AI 提示詞本身不會讓生成內容取得著作權；只能主張人類創作的部分，並須揭露及排除 AI 生成素材。
- 取樣須先取得錄音母帶與詞曲作品兩項授權；純音訊翻唱須取得機械授權，搭配影片則另須同步授權。
- 合理使用須依個案判斷，且只能由法院最終確定。
- 涉及重大爭議、高商業價值、方法或框架保護、重要 AI 請求或合理使用計畫時，內容要求諮詢智慧財產權律師。

| 審核狀態 | 審核人 | 審核日期 | 備註 |
| --- | --- | --- | --- |
| **待審** | | | |

### I-10. `sokrati`（writing／已索引）

- 來源：[https://github.com/iamursky/sokrati](https://github.com/iamursky/sokrati) @ `6255b82d5f25`，`skills/sokrati/SKILL.md`
- 生成：本次新呼叫｜模型 `gpt-5.6-sol`｜prompt `enrich-skill/v2`｜2026-08-15

> 此技能依據俄文資訊風格原則，編輯、重寫或評析商業與專業文本，例如信件、履歷、新聞稿、報告、簡報、社群貼文與登陸頁。它會改善文字的實用性、清晰度、連貫性與簡潔度，並依文類調整結構、語句及行動呼籲。使用者需提供原文，最好也說明目標讀者、文本類型、閱讀目的，以及希望完整改寫或只取得回饋。

任務範例句：

- 請用資訊風格重寫這封俄文陌生開發信，並說明主要修改。　／　*Please rewrite this Russian cold email in information style and explain the main changes.*
- 請評析我的俄文履歷，但不要重寫，並列出最重要的三項改進。　／　*Review my Russian résumé without rewriting it, and list the three most important improvements.*
- 請刪除這篇俄文新聞稿中的官僚用語與含糊主張。　／　*Remove bureaucratic language and vague claims from this Russian press release.*
- 請改善這個面向潛在客戶的俄文登陸頁，讓產品效益更清楚。　／　*Improve this Russian landing page for prospective customers and make its benefits clearer.*
- 請將伊利亞霍夫的方法套用到這篇英文公司介紹，並建議應補充哪些事實。　／　*Apply Ilyakhov’s approach to this English company description and recommend facts that should be added.*

標籤 — 輸入：`russian professional text`、`english text`、`target audience`、`text type`、`reader goal`、`editing mode`；輸出：`edited text`、`change list`、`recommendations`、`text review`、`priority actions`、`writing guidance`；工具：`read`、`write`、`edit`、`grep`、`glob`、`askuserquestion`；依賴：`references/knowledge.md`、`information style principles`

限制：

- 主要處理俄文商業與專業文本；英文文本須由使用者明確要求採用伊利亞霍夫的方法。
- 若讀者、文本類型或閱讀目的不明確，需先向使用者詢問背景。
- 文字層級的修改不保證能解決受眾不明、目標不清或研究不足等更高層次問題。

| 審核狀態 | 審核人 | 審核日期 | 備註 |
| --- | --- | --- | --- |
| **待審** | | | |

### I-11. `shorten`（writing／已索引）

- 來源：[https://github.com/iamursky/sokrati](https://github.com/iamursky/sokrati) @ `6255b82d5f25`，`skills/shorten/SKILL.md`
- 生成：本次新呼叫｜模型 `gpt-5.6-sol`｜prompt `enrich-skill/v2`｜2026-08-15

> 此技能依資訊風格原則編輯、精簡或評析英文商務與專業文本，例如電子郵件、履歷、報告、新聞稿、簡報、登陸頁與社群貼文。它會檢查贅詞、模糊表述、官僚語言、句子清晰度、內容結構與文類規則，並可提供改寫版本、修改說明、改善建議或不重寫的評析。使用者需提供原文，最好同時說明目標讀者、文本類型與希望讀者採取的行動；若明確提出，也可處理俄文文本。

任務範例句：

- 請精簡這封寄給潛在客戶的銷售信，並說明最重要的修改。　／　*Please tighten this sales email for prospective customers and explain the most important changes.*
- 請評析我的履歷是否清楚有力，但不要直接重寫，並列出最重要的五項改善建議。　／　*Review my resume for clarity and impact without rewriting it, then list the top five improvements.*
- 請用淺白英文重寫這個登陸頁，刪除陳腔濫調，並具體呈現客戶可獲得的好處。　／　*Rewrite this landing page in plain English, remove clichés, and make the customer benefits specific.*
- 請重新整理這份內部報告，將摘要與需要讀者採取的行動放在最前面。　／　*Reorganize this internal report so the summary and required actions appear first.*
- 請依資訊風格原則編輯這篇俄文新聞稿，並加強第一段。　／　*Edit this Russian press release using information-style principles and strengthen the opening paragraph.*

標籤 — 輸入：`business text`、`professional copy`、`audience context`、`text genre`、`reader action`、`english text`、`russian text`；輸出：`edited text`、`change log`、`writing review`、`recommendations`、`priority actions`；工具：`read`、`write`、`edit`、`grep`、`glob`、`askuserquestion`；依賴：`references/knowledge.md`

限制：

- 主要用於英文商務與專業寫作；只有使用者明確提出要求時，才可套用於俄文文本。
- 編輯前需要了解讀者、文本類型與讀者閱讀後可採取的實際行動；若情境不明顯且未提供，會先詢問。
- 文字層級的修改不一定能解決受眾不清、目標不明或研究不足等問題。

| 審核狀態 | 審核人 | 審核日期 | 備註 |
| --- | --- | --- | --- |
| **待審** | | | |

### I-12. `excel-filter`（data／已索引）

- 來源：[https://github.com/YuYY2004/excel-skills](https://github.com/YuYY2004/excel-skills) @ `c15e51e20424`，`claude/skills/excel-filter/SKILL.md`
- 生成：重用 golden set 既有增強產出｜模型 `gpt-5.6-sol`｜prompt `enrich-skill/v1`｜2026-08-15
- ⚠️ **來源 SKILL.md 為簡體中文**；下列摘要與範例句即繁中化呈現層（見 §1.3）。

> 此技能依照指定條件篩選 Excel 資料列，可選擇只保留符合條件的列，或刪除符合條件的列；支援等於、包含、大小比較、日期範圍、空值及多條件組合。使用者需提供 Excel 檔案、篩選欄位與條件、表頭位置，以及保留或刪除模式。它會先以 pandas 找出目標列，再直接修改工作表 XML 以保留格式，並進行備份與結果驗證；刪除後還可在使用者確認後壓實不連續的空白列。

任務範例句：

- 請只保留這份 Excel 中申請日在 2020 年 1 月 1 日之後（含當日）的資料列。　／　*Keep only the rows in this Excel file where the application date is on or after January 1, 2020.*
- 請刪除申請人欄位為空白的資料列，保留其他資料。　／　*Delete rows where the applicant field is blank, and keep all other data.*
- 請提取標題包含「石墨烯」且法律狀態為「已授權」的資料列。　／　*Extract rows where the title contains “graphene” and the legal status is “granted.”*
- 請保留申請日在 2020 至 2023 年之間的紀錄，並在刪除後壓實留下的空白列。　／　*Keep records with application dates between 2020 and 2023, then compact the blank rows left after deletion.*
- 請刪除申請人欄位包含「華為」或「騰訊」的所有資料列。　／　*Remove every row where the applicant contains either Huawei or Tencent.*

標籤 — 輸入：`xlsx file`、`filter column`、`filter condition`、`filter mode`、`header row`；輸出：`filtered xlsx file`、`backup xlsx file`、`filter verification`、`row deletion report`；工具：`pandas`、`openpyxl`、`xlsx xml`、`zipfile`；依賴：`pandas`、`openpyxl`、`lxml`、`excel-safe-workflow`、`excel-delete`

限制：**欄位不存在**（本筆為 `enrich-skill/v1` 產出，該版本尚無 `limitations`；見 §2.2）

| 審核狀態 | 審核人 | 審核日期 | 備註 |
| --- | --- | --- | --- |
| **待審** | | | |

### I-13. `excel-validate`（data／已索引）

- 來源：[https://github.com/YuYY2004/excel-skills](https://github.com/YuYY2004/excel-skills) @ `c15e51e20424`，`claude/skills/excel-validate/SKILL.md`
- 生成：本次新呼叫｜模型 `gpt-5.6-sol`｜prompt `enrich-skill/v2`｜2026-08-15
- ⚠️ **來源 SKILL.md 為簡體中文**；下列摘要與範例句即繁中化呈現層（見 §1.3）。

> 此技能以唯讀方式掃描 Excel 檔案，檢查空值率、唯一值、欄位類型一致性、數值異常值、完全重複列、全空欄位、未命名欄位及疑似以文字儲存的日期。它會依使用者指定的範圍執行單項或完整檢查，並輸出包含嚴重程度與處理建議的資料品質報告，不會變更原始檔案。使用時需要提供目標 Excel 檔案，並可說明想檢查的資料品質問題。

任務範例句：

- 請掃描這個 Excel 檔案的資料品質問題，並給我一份報告。　／　*Scan this Excel file for data quality issues and give me a report.*
- 請檢查這份試算表哪些欄位有空值，並列出各欄位的空值率。　／　*Check which columns in this spreadsheet contain null values and show their null rates.*
- 請找出這個 Excel 檔案中是否有完全重複的資料列。　／　*Find any fully duplicated rows in this Excel file.*
- 請檢查數值欄位是否有極端異常值，並回報最小值與最大值。　／　*Check the numeric columns for extreme outliers and report their minimum and maximum values.*
- 請檢查這份活頁簿是否有混合資料類型、全空欄位，以及以文字格式儲存的日期。　／　*Validate this workbook for mixed data types, all-null columns, and dates stored as text.*

標籤 — 輸入：`excel file`、`validation scope`、`worksheet data`；輸出：`data quality report`、`null rate report`、`outlier report`、`duplicate row count`、`severity levels`、`handling suggestions`；工具：`python`、`pd.read_excel`、`filesystem`；依賴：`pandas`、`numpy`、`excel reader`

限制：

- 僅輸出檢查報告，不會修改原始 Excel 檔案。
- 異常值檢查只針對數值欄位，並以四分位距規則辨識極端值。
- 重複資料檢查只計算整列完全相同的資料。
- 空值報告最多先列出 20 個有空值的欄位，其餘僅顯示欄位數量。
- 需要 pandas、NumPy，以及可供讀取的 Excel 檔案。
- 在 Windows 輸出中文時，可能需要將標準輸出編碼設為 UTF-8。

| 審核狀態 | 審核人 | 審核日期 | 備註 |
| --- | --- | --- | --- |
| **待審** | | | |

### I-14. `excel-merge`（data／已索引）

- 來源：[https://github.com/YuYY2004/excel-skills](https://github.com/YuYY2004/excel-skills) @ `c15e51e20424`，`claude/skills/excel-merge/SKILL.md`
- 生成：重用 golden set 既有增強產出｜模型 `gpt-5.6-sol`｜prompt `enrich-skill/v1`｜2026-08-15
- ⚠️ **來源 SKILL.md 為簡體中文**；下列摘要與範例句即繁中化呈現層（見 §1.3）。

> 此技能可將多個結構相同的 Excel 檔案按列合併為單一活頁簿，先檢查欄位標題是否一致，並只保留一次表頭。使用者需提供要合併的檔案清單或資料夾，亦可指定輸出檔名；輸出會繼承第一個檔案的格式。小型檔案透過 pandas 與 openpyxl 處理，大型檔案則以 XML 追加資料列，並處理共享字串、公式列號及結果驗證。

任務範例句：

- 請把這三個 Excel 檔案合併成 merged.xlsx，表頭只保留一次。　／　*Merge these three Excel files into merged.xlsx and keep the header only once.*
- 請合併這個資料夾內所有 XLSX 檔案，並沿用第一個活頁簿的格式。　／　*Combine all XLSX files in this folder and use the first workbook's formatting.*
- 請先檢查這些 Excel 檔案的表頭是否一致，再彙總所有資料列。　／　*Check whether these Excel files have matching headers before consolidating their rows.*
- 請合併這些大型 Excel 檔案，並調整新增資料列中的公式列號引用。　／　*Merge these large Excel files and adjust formula row references for the appended rows.*

標籤 — 輸入：`excel files`、`file list`、`input folder`、`output filename`、`header row`；輸出：`merged excel workbook`、`row count validation`、`header validation`；工具：`python`、`xml row append`、`zipfile`；依賴：`pandas`、`openpyxl`、`lxml`

限制：**欄位不存在**（本筆為 `enrich-skill/v1` 產出，該版本尚無 `limitations`；見 §2.2）

| 審核狀態 | 審核人 | 審核日期 | 備註 |
| --- | --- | --- | --- |
| **待審** | | | |

### I-15. `excel-split`（data／已索引）

- 來源：[https://github.com/YuYY2004/excel-skills](https://github.com/YuYY2004/excel-skills) @ `c15e51e20424`，`claude/skills/excel-split/SKILL.md`
- 生成：本次新呼叫｜模型 `gpt-5.6-sol`｜prompt `enrich-skill/v2`｜2026-08-15
- ⚠️ **來源 SKILL.md 為簡體中文**；下列摘要與範例句即繁中化呈現層（見 §1.3）。

> 此技能可依指定欄位的不同值，將大型 Excel 表格拆成多個獨立的 .xlsx 檔案，並保留原有格式。使用者需要提供來源檔案與拆分欄位，也可指定 Top N 數量及輸出目錄；超出 Top N 的資料可合併為「其他.xlsx」。流程會先統計與規劃分組，再以串流方式分流、產生檔案，最後驗證輸出總行數與資料是否錯配。

任務範例句：

- 請按「申請人」欄拆分這份 Excel，只為前 10 名申請人各建立一個檔案，其餘資料放入「其他」檔案。　／　*Split this Excel workbook by the Applicant column and create one file for each of the top 10 applicants, with all remaining rows placed in an Others file.*
- 請依「年份」欄將試算表拆成多個檔案，並儲存到 year_split 資料夾。　／　*Separate the spreadsheet into individual files by year and save them in the year_split folder.*
- 請依「部門」欄拆分這個大型 .xlsx 檔案，使用預設的 Top 20 限制，並驗證輸出總行數與來源一致。　／　*Split this large .xlsx file by department, using the default Top 20 limit, and verify that the output row count matches the source.*
- 請先統計「地區」欄各值的筆數，再拆出數量最多的 5 個地區，並將剩餘資料合併為一個「其他」檔案。　／　*Show me the value counts for the Region column first, then split the workbook into files for the five most common regions and one Others file.*

標籤 — 輸入：`xlsx 活頁簿`、`拆分欄位`、`top n`、`輸出目錄`；輸出：`分組 xlsx 檔案`、`其他 xlsx 檔案`、`輸出資料夾`、`行數驗證結果`；工具：`pandas grouping`、`lxml iterparse`、`xml streaming`、`zipfile`；依賴：`python`、`pandas`、`lxml`、`excel-safe-workflow`、`excel-delete`

限制：

- 必須明確指定拆分欄位。
- 內容所示流程以 .xlsx 檔案為處理對象。
- 若唯一值過多（超過 50 個），流程只拆分 Top N，其餘資料合併至「其他.xlsx」；Top N 預設為 20。
- 若活頁簿曾經去重或篩選而留下行號空隙，拆分前需要先壓實。
- 執行環境需要 Python、pandas 與 lxml。
- 每個輸出檔都會繼承完整的 sharedStrings.xml，因此輸出檔越多，打包時間與總檔案大小越大。
- 拆分前需要先備份原始檔案；流程說明要求保留最新 3 份時間戳備份。

| 審核狀態 | 審核人 | 審核日期 | 備註 |
| --- | --- | --- | --- |
| **待審** | | | |

### I-16. `excel-sort`（data／已索引）

- 來源：[https://github.com/YuYY2004/excel-skills](https://github.com/YuYY2004/excel-skills) @ `c15e51e20424`，`claude/skills/excel-sort/SKILL.md`
- 生成：本次新呼叫｜模型 `gpt-5.6-sol`｜prompt `enrich-skill/v2`｜2026-08-15
- ⚠️ **來源 SKILL.md 為簡體中文**；下列摘要與範例句即繁中化呈現層（見 §1.3）。

> 此技能依指定欄位整理 Excel 資料，支援單欄或多欄優先排序，以及文字、數值與日期的升冪或降冪排列。使用者需提供 Excel 檔案並明確指出排序欄，可另外指定排序方向、欄位優先次序與資料範圍。流程會先檢查表頭、資料型別、公式與範圍，再以 pandas 排序、用 openpyxl 恢復關鍵格式，最後抽查結果是否依指定順序排列。

任務範例句：

- 請將這份 Excel 按「公開日」欄由舊到新升冪排序。　／　*Sort this Excel file by the Publication Date column in ascending order.*
- 請將「金額」欄從大到小排序，並保持表頭在原位。　／　*Sort the Amount column from largest to smallest while keeping the header in place.*
- 請先依「類別」升冪排序，再依「日期」由新到舊排序。　／　*Sort first by Category in ascending order, then by Date with the newest records first.*
- 請將 E 欄由小到大排序，並保留活頁簿格式與凍結窗格。　／　*Sort column E in ascending order and preserve the workbook's formatting and frozen panes.*
- 請先檢查排序欄是否含公式並備份檔案，再將資料依該欄降冪排列。　／　*Check whether the sort column contains formulas, back up the file, and then sort the data in descending order.*

標籤 — 輸入：`excel 活頁簿`、`排序欄位`、`排序方向`、`欄位優先次序`、`資料範圍`、`表頭列`；輸出：`已排序 excel 活頁簿`、`時間戳備份`、`排序驗證結果`、`保留的關鍵格式`；工具：`pandas sort_values`、`openpyxl load_workbook`、`pandas read_excel`、`pandas to_excel`；依賴：`python`、`pandas`、`openpyxl`、`excel-safe-workflow`

限制：

- 執行前必須確認排序欄與資料範圍，完成後必須驗證排序順序。
- 排序欄必須明確；未指定方向時預設為升冪。
- 表頭不參與排序，並固定保留在原位。
- 空值無論升冪或降冪都排在最後。
- 公式欄排序可能造成引用錯亂；若排序欄含公式，需先轉換為值。
- 超過 5 萬列時，必須先詢問使用者是否要保留原格式。
- 資料格式以首筆資料列為範本套用；若同一欄內格式不一致，原有差異會遺失。
- 操作前必須依照 excel-safe-workflow 建立時間戳備份。

| 審核狀態 | 審核人 | 審核日期 | 備註 |
| --- | --- | --- | --- |
| **待審** | | | |

### I-17. `excel-regex-clean`（data／已索引）

- 來源：[https://github.com/YuYY2004/excel-skills](https://github.com/YuYY2004/excel-skills) @ `c15e51e20424`，`claude/skills/excel-regex-clean/SKILL.md`
- 生成：本次新呼叫｜模型 `gpt-5.6-sol`｜prompt `enrich-skill/v2`｜2026-08-15
- ⚠️ **來源 SKILL.md 為簡體中文**；下列摘要與範例句即繁中化呈現層（見 §1.3）。

> 此技能使用正則表達式清理 Excel 指定欄位，可提取符合內容、刪除符合片段，或將其替換成其他文字。使用者需提供 Excel 檔案、目標欄位，以及自然語言描述或正則規則；替換模式還需提供替換文字。它會先預覽變更，再於副本中處理並驗證結果；大型檔案使用工作表 XML 路徑，小型檔案使用 openpyxl。

任務範例句：

- 請從這個 Excel 檔案的「產業」欄中，只提取括號內的中文文字。　／　*Extract only the Chinese text inside parentheses from the “Industry” column in this Excel file.*
- 請刪除「類別」欄中的所有數字與句點，其他欄位不要變更。　／　*Remove all numbers and periods from the “Category” column while leaving the other columns unchanged.*
- 請把「產品名稱」欄中的所有空格替換成底線。　／　*Replace every space in the “Product Name” column with an underscore.*
- 請移除「申請日」欄中的連字號，並將結果另存成新的 Excel 檔案。　／　*Remove the hyphens from the “Application Date” column and save the result as a new Excel file.*
- 請先預覽「代碼」欄只保留英文字母的結果，確認無誤後再執行。　／　*Preview the changes from keeping only English letters in the “Code” column, then apply them if the preview is correct.*

標籤 — 輸入：`xlsx file`、`target column`、`regex pattern`、`cleaning mode`、`replacement text`、`natural-language rule`；輸出：`cleaned xlsx file`、`transformation preview`、`cell change count`、`value distribution`；工具：`regular expressions`、`sheet xml`、`openpyxl`、`zipfile`；依賴：`python`、`pandas`、`openpyxl`、`lxml`、`python re`、`excel-safe-workflow`

限制：

- 需要提供 Excel 檔案、目標欄位，以及清理需求或正則表達式。
- 正則中的「.」、「(」、「)」和「\」等特殊字元需要跳脫。
- extract 模式若找不到符合項目，會保留原值。
- 只處理指定的目標欄位，不變更其他欄位。
- 執行環境需要 Python，以及 pandas、openpyxl 和 lxml 等套件。
- 建議先預覽轉換結果再執行，並依 excel-safe-workflow 在操作前備份。

| 審核狀態 | 審核人 | 審核日期 | 備註 |
| --- | --- | --- | --- |
| **待審** | | | |

### I-18. `excel-scout`（data／已索引）

- 來源：[https://github.com/YuYY2004/excel-skills](https://github.com/YuYY2004/excel-skills) @ `c15e51e20424`，`claude/skills/excel-scout/SKILL.md`
- 生成：本次新呼叫｜模型 `gpt-5.6-sol`｜prompt `enrich-skill/v2`｜2026-08-15
- ⚠️ **來源 SKILL.md 為簡體中文**；下列摘要與範例句即繁中化呈現層（見 §1.3）。

> 此技能會在執行 Excel 操作前，以唯讀方式掃描活頁簿，列出表頭、辨識日期或公式欄位，並依使用者的業務需求定位可能要處理的欄位及顯示目前值的樣本。它需要明確的 Excel 檔案路徑與操作意圖，例如日期格式轉換、國別代碼轉換或序號重排，最後產生結構化勘察報告供使用者確認。

任務範例句：

- 請勘察 reports/merged.xlsx，找出所有可能需要轉成 yyyymmdd 的日期欄位。　／　*Inspect reports/merged.xlsx and find every date column that may need conversion to yyyymmdd.*
- 請查看 data/patents.xlsx 的結構，找出公開國別代碼所在欄位並顯示值的樣本。　／　*Check the structure of data/patents.xlsx and identify the column containing publication country codes, with sample values.*
- 請勘察 sales.xlsx，確認哪一欄是序號，以及目前編號是否連續。　／　*Scout sales.xlsx to determine which column is the sequence number and whether its numbering is continuous.*
- 請列出 archive.xlsx 的所有表頭，並透過比較公式與計算值辨識公式欄位。　／　*List all headers in archive.xlsx and identify formula columns by comparing formulas with calculated values.*
- 請檢查 merged_records.xlsx，包括中間與末尾資料列，以確認資料是否由多段內容拼接而成。　／　*Inspect merged_records.xlsx, including middle and final rows, to check whether the data contains concatenated segments.*

標籤 — 輸入：`excel file path`、`operation intent`、`column characteristics`；輸出：`scout report`、`header inventory`、`target columns`、`value samples`；工具：`openpyxl read_only`、`openpyxl data_only`、`pandas read_excel`；依賴：`python`、`openpyxl`、`pandas`

限制：

- 必須提供明確的 Excel 文件路徑。
- 僅進行唯讀掃描，不會修改檔案。
- 資料值以 pandas 限量取樣，不會載入完整大型資料集。
- 使用者確認勘察結果後，實際修改須交由後續操作技能執行。

| 審核狀態 | 審核人 | 審核日期 | 備註 |
| --- | --- | --- | --- |
| **待審** | | | |

### I-19. `excel-delete`（data／已索引）

- 來源：[https://github.com/YuYY2004/excel-skills](https://github.com/YuYY2004/excel-skills) @ `c15e51e20424`，`claude/skills/excel-delete/SKILL.md`
- 生成：本次新呼叫｜模型 `gpt-5.6-sol`｜prompt `enrich-skill/v2`｜2026-08-15
- ⚠️ **來源 SKILL.md 為簡體中文**；下列摘要與範例句即繁中化呈現層（見 §1.3）。

> 此技能可從 Excel 活頁簿中刪除指定列或欄，也能掃描並移除完全空白的列或欄。使用者需提供 .xlsx 檔案，以及要刪除的列號、欄號、欄名或空白項目條件。它會先備份檔案並檢查公式依賴，發現可能造成 #REF! 的引用時會要求確認，完成後再驗證刪除結果與公式狀態；XML 刪列後也可經確認壓實列號。

任務範例句：

- 請刪除這個 Excel 檔案的 E 欄，但先檢查是否有公式依賴它。　／　*Delete column E from this Excel file, but check whether any formulas depend on it first.*
- 請移除活頁簿的第 5 至第 10 列，並確認沒有產生 #REF! 錯誤。　／　*Remove rows 5 through 10 from the workbook and verify that no #REF! errors are created.*
- 請找出並刪除這份試算表中所有完全空白的列。　／　*Find and delete every completely empty row in this spreadsheet.*
- 請刪除名為「申請日」的欄，並在變更前建立備份。　／　*Delete the column named "Application Date" and create a backup before making changes.*
- 請刪除所有空白欄，並在需要時壓實剩餘的列號。　／　*Delete all empty columns, then compact the remaining row numbers if needed.*

標籤 — 輸入：`xlsx file`、`row indices`、`column indices`、`column names`、`empty rows`、`empty columns`；輸出：`modified xlsx file`、`backup xlsx file`、`formula dependency report`、`deletion verification`、`compacted row numbers`；工具：`openpyxl`、`lxml etree`、`zipfile`、`xml operations`；依賴：`python`、`openpyxl`、`lxml`、`excel-safe-workflow`、`compact_rows`

限制：

- 刪除前必須完成需求解析、勘察與規劃，刪除後必須驗證。
- 刪除前必須備份檔案並檢查公式依賴；若發現依賴，需先取得使用者確認。
- INDIRECT 與 OFFSET 產生的間接引用不會被自動偵測。
- 大型檔案需要安裝 lxml，並使用 huge_tree=True。
- 以 XML 刪除列後，列號不會自動連續；必須詢問使用者是否要壓實列號。
- 列刪除使用 openpyxl，列刪除則直接操作工作表 XML。

| 審核狀態 | 審核人 | 審核日期 | 備註 |
| --- | --- | --- | --- |
| **待審** | | | |

### I-20. `excel-mapping-replace`（data／已索引）

- 來源：[https://github.com/YuYY2004/excel-skills](https://github.com/YuYY2004/excel-skills) @ `c15e51e20424`，`claude/skills/excel-mapping-replace/SKILL.md`
- 生成：本次新呼叫｜模型 `gpt-5.6-sol`｜prompt `enrich-skill/v2`｜2026-08-15
- ⚠️ **來源 SKILL.md 為簡體中文**；下列摘要與範例句即繁中化呈現層（見 §1.3）。

> 此技能依照映射表，批次替換 Excel 指定欄位中的對應值，未出現在映射表中的內容會保留原樣。使用者需要提供目標 Excel 檔案、目標欄名稱，以及透過對話、貼上清單或 xlsx／csv 檔案提供的映射關係。技能會先統計受影響與未匹配的資料，再依檔案大小使用 openpyxl 或工作表 XML 進行替換，並備份及驗證結果。

任務範例句：

- 請依照我的映射清單，把這份 Excel「公開國別」欄中的國家名稱替換成 CN、JP、US。　／　*Replace the country names in the “Public Country” column of this Excel file with CN, JP, and US according to my mapping list.*
- 請使用「國家代碼表.xlsx」的 A 欄到 B 欄作為映射，更新目標活頁簿中的國家欄。　／　*Use columns A and B from Country Codes.xlsx as the mapping table and update the country column in the target workbook.*
- 請用以下映射批次替換狀態欄：待處理 → P、已核准 → A、已拒絕 → R。　／　*Batch replace values in the status column using these mappings: Pending → P, Approved → A, Rejected → R.*
- 請把 mapping.csv 的映射套用到代碼欄，沒有匹配的值請維持不變。　／　*Apply the mappings from mapping.csv to the code column, and keep any unmatched values unchanged.*
- 在替換分類欄的值之前，請先列出每個映射會影響多少列。　／　*Show me how many rows each mapping will affect before replacing the values in the category column.*

標籤 — 輸入：`xlsx file`、`target column`、`mapping pairs`、`mapping xlsx file`、`mapping csv file`、`pasted mapping list`；輸出：`modified xlsx file`、`backup xlsx file`、`replacement count`、`unmatched value report`、`verification report`；工具：`pandas`、`openpyxl`、`worksheet xml`、`lxml`、`zipfile`；依賴：`python`、`pandas`、`openpyxl`、`lxml`、`excel-safe-workflow`

限制：

- 必須明確指定目標欄與映射關係。
- 僅進行精確匹配，不支援包含匹配；例如「中國」不會匹配「中國北京」。
- 匹配區分大小寫；如需忽略大小寫，必須先行預處理。
- 映射表中不存在的值不會被替換，而是保留原值。
- 若目標檔案正在 Excel 中開啟，可能無法儲存，必須關閉檔案後重試。

| 審核狀態 | 審核人 | 審核日期 | 備註 |
| --- | --- | --- | --- |
| **待審** | | | |

### I-21. `excel-date-to-text`（data／已索引）

- 來源：[https://github.com/YuYY2004/excel-skills](https://github.com/YuYY2004/excel-skills) @ `c15e51e20424`，`claude/skills/excel-date-to-text/SKILL.md`
- 生成：本次新呼叫｜模型 `gpt-5.6-sol`｜prompt `enrich-skill/v2`｜2026-08-15
- ⚠️ **來源 SKILL.md 為簡體中文**；下列摘要與範例句即繁中化呈現層（見 §1.3）。

> 此技能會掃描 Excel 中的日期欄，將 datetime 值轉成指定的文字格式，例如 yyyymmdd、yyyy-mm-dd、中文年月日或自訂年月日組合。結果可插入原日期欄左側或右側，也可直接取代原欄，並能自訂新欄表頭。使用時需要提供 Excel 檔案；日期格式、輸出位置與表頭若未指定，預設分別為 yyyymmdd、左側插入及「原欄名(文本)」。

任務範例句：

- 請把這個 Excel 檔案中的所有日期欄轉成 yyyymmdd 文字，並在原欄左側插入新欄。　／　*Convert every date column in this Excel file to yyyymmdd text and insert the new columns on the left.*
- 請將日期格式化為 yyyy/mm/dd，並直接取代原本的日期欄。　／　*Format the dates as yyyy/mm/dd and replace the original date columns directly.*
- 請在每個日期欄右側新增名為「格式化日期」的文字欄，格式使用 yyyy年mm月dd日。　／　*Add a text column named “Formatted Date” to the right of each date column using the yyyy年mm月dd日 format.*
- 請將日期欄轉成不補零的 yyyy/m/d 格式，空白日期維持空白。　／　*Convert the date columns to yyyy/m/d without leading zeros, keeping empty dates blank.*
- 請先掃描這個活頁簿中的日期欄，告訴我會轉換哪些欄位，再進行修改。　／　*Scan this workbook for date columns and tell me which columns would be converted before making changes.*

標籤 — 輸入：`xlsx file`、`datetime columns`、`date format`、`output position`、`column header`；輸出：`modified xlsx file`、`formatted date text`、`text columns`；工具：`openpyxl`、`pandas`；依賴：`python`、`openpyxl`、`datetime`、`pandas`、`excel-safe-workflow`、`excel-insert`、`excel-replace`

限制：

- 執行前必須先備份檔案，並完成需求解析、勘察與規劃；執行後必須驗證結果。
- 日期值必須透過 openpyxl 的 datetime 型別或 pandas 的日期型別判定，不能依 Excel XML 數值範圍猜測。
- 空白日期不會轉換；新增的文字欄會保持空白。
- 替換模式只轉換 datetime 值，非日期值維持不變。
- 超過 100MB 的大型檔案在載入與儲存時可能各需數分鐘，因此需要足夠的執行逾時時間。

| 審核狀態 | 審核人 | 審核日期 | 備註 |
| --- | --- | --- | --- |
| **待審** | | | |

### I-22. `standardise-country-names`（data／已索引）

- 來源：[https://github.com/danielrosehill/Claude-Data-Wrangler-plugin](https://github.com/danielrosehill/Claude-Data-Wrangler-plugin) @ `b12805a62307`，`skills/standardise-country-names/SKILL.md`
- 生成：本次新呼叫｜模型 `gpt-5.6-sol`｜prompt `enrich-skill/v2`｜2026-08-15

> 此技能會整理資料集中的國家欄位，將不同拼法、別名、大小寫或代碼統一成使用者選定的標準形式。它需要一份含有國家欄位的資料集，並會先列出各個唯一值及出現次數，再詢問要採用 ISO 官方短名、常用名稱、ISO alpha-2 或 alpha-3 代碼。處理後會回報前後差異、標記無法解析的值、以 `_standardised` 後綴寫出結果，並在資料字典存在時更新它。

任務範例句：

- 請將這份資料集的國家欄位標準化，讓 USA、U.S.A. 和 United States of America 統一使用 ISO 3166 官方短名。　／　*Standardize the country column in this dataset so that USA, U.S.A., and United States of America all use ISO 3166 official short names.*
- 請先列出國家欄位的唯一值及出現次數，再將確認過的結果轉成 ISO alpha-2 代碼並放在新欄位。　／　*Profile the unique country values and counts first, then convert confirmed matches to ISO alpha-2 codes in a new column.*
- 請統一大小寫不一致與別名形式的國家名稱，但遇到歷史名稱或爭議地區時先詢問我。　／　*Normalize mixed-case and alias country names, but ask me before resolving historical names or disputed territories.*
- 請清理國家名稱並覆寫原欄位，標記無法解析的項目，然後以 `_standardised` 後綴儲存輸出。　／　*Clean up the country names, overwrite the existing column, flag unresolved entries, and save the output with a _standardised suffix.*

標籤 — 輸入：`dataset`、`country column`、`canonical standard`、`data dictionary`；輸出：`standardised dataset`、`canonical country names`、`country_standardisation_status column`、`unresolved values report`、`updated data dictionary`；工具：`exact matching`、`alias table`、`fuzzy matching`、`case normalisation`；依賴：`pandas`、`pycountry`

限制：

- 需要安裝 pandas 與 pycountry。
- 套用前需要使用者選擇標準名稱形式，以及要新增欄位或覆寫原欄位。
- 低可信度的模糊比對需由使用者確認後才會套用。
- 歷史名稱需由使用者決定要映射至目前的繼承國家，或保留原值。
- 臺灣、科索沃、西撒哈拉與巴勒斯坦等爭議地區不會自動解析，需詢問使用者。
- 無法解析的值會保留原文，並在 country_standardisation_status 欄位中標記。

| 審核狀態 | 審核人 | 審核日期 | 備註 |
| --- | --- | --- | --- |
| **待審** | | | |

### I-23. `unicode-consistency`（data／已索引）

- 來源：[https://github.com/danielrosehill/Claude-Data-Wrangler-plugin](https://github.com/danielrosehill/Claude-Data-Wrangler-plugin) @ `b12805a62307`，`skills/unicode-consistency/SKILL.md`
- 生成：本次新呼叫｜模型 `gpt-5.6-sol`｜prompt `enrich-skill/v2`｜2026-08-15

> 此技能稽核資料集文字欄位的 Unicode 一致性，包括編碼與正規化形式、混合文字系統、同形異義字、不可見字元、空白與標點變體、亂碼、表情符號序列、大小寫折疊及雙向文字問題。它需要含文字欄位的資料集，以及使用者偏好的正規化形式與領域規則；完成後會產生 `unicode_report.md`、逐欄修復指令碼、清理前後預覽，並在確認後預設寫入新欄位、重新稽核及更新資料字典。

任務範例句：

- 請稽核這份資料集的文字欄位是否混用 Unicode 正規化形式、含有不可見字元或亂碼，並產生報告。　／　*Audit the text columns in this dataset for mixed Unicode normalization, invisible characters, and mojibake, then create a report.*
- 請找出 `customer_name` 欄位中的名稱看起來相同、卻無法正確關聯的原因。　／　*Find out why joins on the customer_name field fail even though the names look identical.*
- 請檢查這份多語產品資料是否誤用了拉丁字母與西里爾字母的同形字元，但允許日文與拉丁產品代碼混用。　／　*Check this multilingual product dataset for accidental Latin-Cyrillic homoglyphs while allowing Japanese text mixed with Latin product codes.*
- 請用 10 筆範例預覽 NFC 正規化與不可見字元移除結果，再準備將結果寫入新清理欄位的指令碼。　／　*Preview NFC normalization and invisible-character removal on 10 examples, then prepare a script that writes the results to new clean columns.*
- 請偵測這些匯入文字欄位中的 UTF-8 雙重編碼與破損表情符號序列，並提出修復方式，但不要修改原始欄位。　／　*Detect double-encoded UTF-8 and broken emoji sequences in these imported text fields, and propose fixes without changing the original columns.*

標籤 — 輸入：`dataset`、`text columns`、`preferred unicode normalization form`、`allowed script combinations`、`domain cleaning rules`；輸出：`unicode_report.md`、`remediation script`、`before-and-after preview`、`cleaned text columns`、`residual audit`、`updated data dictionary`；工具：`chardet`、`charset-normalizer`、`unicodedata`、`confusable_homoglyphs`、`uniseg`、`ftfy`；依賴：`pandas`、`charset-normalizer`、`ftfy`、`confusable-homoglyphs`、`python unicodedata`、`unicode confusables table`、`CONVENTIONS.md`

限制：

- 套用清理前須先在樣本上顯示 10 組前後對照並取得使用者確認。
- 預設會將結果寫入新的 `<col>_nfc` 或 `<col>_clean` 欄位；只有在使用者明確要求並遵循 `CONVENTIONS.md` 的備份政策時才覆寫原值。
- 多語資料可能有刻意的文字系統混用，需設定允許的文字系統組合。
- 識別碼等領域特定欄位可能合理使用非 ASCII 字元，未經使用者確認不得自動正規化。
- 密碼雜湊、簽章及含百分比編碼的 URL 等需保留往返一致性的欄位不得正規化，只能標記為「請勿變更」。
- `ftfy.fix_text` 採啟發式修復，偶爾可能過度更正，因此大規模套用前必須預覽。
- 選擇 Unicode 正規化形式前，需先確認下游需求；HF Datasets、JSON-LD 與網頁 API 通常使用 NFC，部分語言工具則需要 NFD。
- 需安裝 `pandas`、`charset-normalizer` 與 `ftfy`；同形異義字檢查可選裝 `confusable-homoglyphs`。

| 審核狀態 | 審核人 | 審核日期 | 備註 |
| --- | --- | --- | --- |
| **待審** | | | |

### I-24. `date-wrangling`（data／已索引）

- 來源：[https://github.com/danielrosehill/Claude-Data-Wrangler-plugin](https://github.com/danielrosehill/Claude-Data-Wrangler-plugin) @ `b12805a62307`，`skills/date-wrangling/SKILL.md`
- 生成：重用 golden set 既有增強產出｜模型 `gpt-5.6-sol`｜prompt `enrich-skill/v1`｜2026-08-15

> 此技能將資料集中的日期與時間欄位轉換成下游系統需要的格式，例如 ISO 8601、Unix epoch、時區日期時間、會計年度、ISO 週日期或儒略日。使用者需提供資料集、來源欄位、目標格式，並在日期順序、來源時區或會計年度起始月份不明時補充相關資訊。它預設保留原始欄位、建立具描述性的輸出欄位，並驗證精度及記錄格式、時區與有損轉換資訊。

任務範例句：

- 請將 created_at 欄位轉成 UTC 的 ISO 8601 格式，並保留原始欄位。　／　*Convert the created_at column to ISO 8601 in UTC while preserving the original column.*
- 請從 event_time 新增 epoch 毫秒欄位，供 Java 應用程式使用。　／　*Add an epoch-milliseconds column from event_time for use by a Java application.*
- 請將 event_date 與 event_clock 合併為一個含時區的日期時間欄位，來源時區假設為 Asia/Taipei。　／　*Combine the event_date and event_clock columns into one timezone-aware datetime column, assuming Asia/Taipei.*
- 請將 order_timestamp 拆成日期與時間欄位，並新增 ISO 週次及星期欄位。　／　*Split order_timestamp into separate date and time columns and add ISO week and day-of-week fields.*
- 請依 invoice_date 建立會計年度與季度欄位，會計年度從四月開始。　／　*Create fiscal year and quarter columns from invoice_date, with the fiscal year starting in April.*

標籤 — 輸入：`dataset`、`date columns`、`time columns`、`source format`、`target format`、`source timezone`、`fiscal start month`；輸出：`transformed dataset`、`iso 8601 timestamps`、`unix timestamps`、`timezone-aware datetimes`、`derived date columns`、`updated data dictionary`；工具：`pandas.to_datetime`；依賴：`pandas`、`python-dateutil`、`pytz`、`zoneinfo`、`data-enrichment skill`、`conventions.md`

限制：**欄位不存在**（本筆為 `enrich-skill/v1` 產出，該版本尚無 `limitations`；見 §2.2）

| 審核狀態 | 審核人 | 審核日期 | 備註 |
| --- | --- | --- | --- |
| **待審** | | | |

### I-25. `json-restructure`（data／已索引）

- 來源：[https://github.com/danielrosehill/Claude-Data-Wrangler-plugin](https://github.com/danielrosehill/Claude-Data-Wrangler-plugin) @ `b12805a62307`，`skills/json-restructure/SKILL.md`
- 生成：本次新呼叫｜模型 `gpt-5.6-sol`｜prompt `enrich-skill/v2`｜2026-08-15

> 此技能可重新塑造 JSON 或 JSONL 資料，例如展平或還原巢狀結構、依鍵分組或解除分組、移動欄位層級，以及展開或合併陣列。使用者需提供資料或樣本，並說明期望的頂層型別、鍵結構與巢狀層次；技能會先以樣本示範，再轉換完整資料並驗證筆數、鍵覆蓋與可逆性。它也會用描述性後綴寫出結果，並更新資料字典中的結構與來源紀錄。

任務範例句：

- 請依國家分組這些 JSON 資料列，讓每個國家包含一個城市陣列。　／　*Group these JSON rows by country, with each country containing an array of its cities.*
- 請將這個巢狀 API 回應展平為點號鍵，讓我能當成表格使用。　／　*Flatten this nested API response into dotted keys so I can use it as a table.*
- 請將這些使用點號表示的 JSON 鍵還原成巢狀物件。　／　*Convert these dotted JSON keys back into nested objects.*
- 請將 tags 陣列展開成多筆獨立紀錄，並在每筆紀錄中保留 ID。　／　*Explode the tags array into separate records while keeping the ID on every record.*
- 請將這份交易清單改成每個帳戶一個物件，並把交易存放在巢狀陣列中。　／　*Turn this transaction list into one object per account, with the transactions stored in a nested array.*

標籤 — 輸入：`json`、`jsonl`、`target json shape`、`grouping keys`、`field paths`、`data dictionary`；輸出：`restructured json`、`restructured jsonl`、`flattened records`、`nested objects`、`grouped hierarchy`、`exploded records`、`updated data dictionary`、`provenance entry`；工具：`json.load`、`pandas.json_normalize`、`collections.defaultdict`、`itertools.groupby`；依賴：`python`、`pandas`、`python standard library`

限制：

- 需要先確認目標 JSON 結構，包括頂層型別、鍵結構與巢狀深度。
- 大型檔案或 JSONL 需逐行串流處理；小型檔案可完整載入。
- 使用 pandas.json_normalize 進行扁平化時需要安裝 pandas。
- 扁平化時若不同巢狀路徑產生相同鍵名，需要使用者指定加後綴、報錯或保留第一個值等處理規則。
- 轉換後的日期時間、NaN、位元組等不可直接序列化為 JSON 的值，需分別轉為 ISO 字串、null 或 base64。

| 審核狀態 | 審核人 | 審核日期 | 備註 |
| --- | --- | --- | --- |
| **待審** | | | |

### I-26. `data-shape`（data／已索引）

- 來源：[https://github.com/danielrosehill/Claude-Data-Wrangler-plugin](https://github.com/danielrosehill/Claude-Data-Wrangler-plugin) @ `b12805a62307`，`skills/data-shape/SKILL.md`
- 生成：本次新呼叫｜模型 `gpt-5.6-sol`｜prompt `enrich-skill/v2`｜2026-08-15

> 此技能分析尚未載入資料庫或需要重新整理的平面檔案、試算表與 API 匯出資料，找出重複欄組、封裝欄位、冗餘參照資料、混合粒度及其他結構問題。它會依資料欄位、型別、基數、空值率、範例資料列與預期查詢方式，提出 SQL 或其他結構化系統的資料表、鍵、關聯、索引及正規化方案。輸出可包含架構說明、Mermaid ER 圖、可調整執行的建表 SQL，以及來源欄位至目標欄位的對應。

任務範例句：

- 在載入 SQL 前，我該如何把這份客戶訂單寬表拆成正規化資料表？　／　*How should I split this wide customer-orders spreadsheet into normalized tables before loading it into SQL?*
- 請檢視這些欄位與範例資料列，並建議資料表、主鍵、外鍵及索引。　／　*Review these columns and sample rows, then propose tables, primary keys, foreign keys, and indexes.*
- 我的 CSV 有 item_1、item_2 與逗號分隔的標籤欄位；請設計資料架構並對應來源欄位。　／　*My CSV has item_1, item_2, and comma-separated tag fields; design a schema and map the source columns to it.*
- 請為這份事件資料提出星型架構，包含事實表、維度表與 Mermaid ER 圖。　／　*Create a star-schema proposal for this event dataset, including fact and dimension tables and a Mermaid ER diagram.*
- 這份 API 載荷應保留在 JSONB，還是拆成可關聯查詢的資料表？　／　*Should this API payload remain in JSONB or be promoted into related tables for querying?*

標籤 — 輸入：`flat files`、`api dumps`、`spreadsheets`、`source columns`、`data types`、`cardinalities`、`null rates`、`sample rows`、`access patterns`；輸出：`schema proposal`、`database schema`、`tables`、`keys`、`relationships`、`indexes`、`mermaid er diagram`、`create table sql`、`column mapping`；工具：`sql`、`jsonb`、`json`、`enum`、`check constraints`、`materialized views`、`sql-load`、`graph-database`、`vector-upsert`；依賴：`pandas`

限制：

- 需要先取得來源資料的欄位、型別、基數、空值率與範例資料列，才能進行剖析與提出結構建議。
- 若資料是刻意反正規化的分析型寬表，需先確認使用者的工作負載再建議是否正規化。
- 原始 API 載荷等應保留的半結構化欄位不會被強制攤平，而是建議使用 JSONB 或 JSON 欄位。
- 代理鍵與自然鍵各有取捨，需由使用者選擇。
- 結構完成後，仍需先把來源轉換成建議的各資料表形狀，才能交由 `sql-load` 載入。

| 審核狀態 | 審核人 | 審核日期 | 備註 |
| --- | --- | --- | --- |
| **待審** | | | |

### I-27. `data-comparability`（data／已索引）

- 來源：[https://github.com/danielrosehill/Claude-Data-Wrangler-plugin](https://github.com/danielrosehill/Claude-Data-Wrangler-plugin) @ `b12805a62307`，`skills/data-comparability/SKILL.md`
- 生成：本次新呼叫｜模型 `gpt-5.6-sol`｜prompt `enrich-skill/v2`｜2026-08-15

> 此技能會分析兩個或以上的表格式資料集，找出欄名、資料型別、類別值、單位及聯結鍵之間的差異，並提出合併、聯集或交叉分析前的具體整理方案。使用者需提供 CSV、Parquet、Excel 或 JSON 資料集；技能會盤點欄位與缺值、建立欄位對齊矩陣、檢查鍵值重疊及可能造成資料損失的型別轉換。最後會把依序執行的重新命名、型別轉換、值映射、單位換算與異常資料列處理建議寫入輸入檔案旁的 `comparability_plan.md`，但不會自動執行這些操作。

任務範例句：

- 比較這三個 CSV 檔案，並在我合併前建立欄名、資料型別與類別值的對齊計畫。　／　*Compare these three CSV files and create a plan to align their column names, data types, and category values before I combine them.*
- 檢查這份 Excel 與 Parquet 資料能否聯結，並報告候選鍵、唯一性及匹配率。　／　*Check whether these Excel and Parquet datasets can be joined, and report candidate keys, uniqueness, and match rates.*
- 找出這些銷售資料集之間的單位與詞彙差異，並提出標準值及換算方式。　／　*Identify unit and vocabulary differences across these sales datasets and propose canonical values and conversions.*
- 為這些 JSON 資料集建立欄位對齊矩陣，並標示重新命名、拆分、合併及缺少的欄位。　／　*Build a column alignment matrix for these JSON datasets and flag renames, splits, merges, and missing fields.*
- 檢視這份交易層級資料與每月彙總資料，告訴我合併前需要進行哪些彙總。　／　*Review these transaction-level and monthly aggregate datasets and tell me what aggregation is needed before merging them.*

標籤 — 輸入：`csv datasets`、`parquet datasets`、`excel datasets`、`json datasets`、`data dictionaries`、`sample values`、`dataset paths`；輸出：`comparability plan`、`column alignment matrix`、`type reconciliation plan`、`categorical value mappings`、`unit conversion plan`、`join key analysis`、`comparability_plan.md`；工具：`python`、`plugin skills`；依賴：`pandas`、`pyarrow`

限制：

- 需要兩個或以上的表格式資料集才能進行分析。
- 支援的資料格式為 CSV、Parquet、Excel 與 JSON。
- 需要安裝 pandas 與 pyarrow。
- 此技能只產生可比性與清理計畫，不會自動執行清理或轉換；由使用者選擇要執行的外掛技能。
- 若資料集的粒度差異很大，會建議先彙總，不會直接提出資料列層級的聯結。
- 若類別值涉及不同語言或地區設定，會要求使用者提供翻譯對照，而不會自行猜測。

| 審核狀態 | 審核人 | 審核日期 | 備註 |
| --- | --- | --- | --- |
| **待審** | | | |

### I-28. `add-data-dictionary`（data／已索引）

- 來源：[https://github.com/danielrosehill/Claude-Data-Wrangler-plugin](https://github.com/danielrosehill/Claude-Data-Wrangler-plugin) @ `b12805a62307`，`skills/add-data-dictionary/SKILL.md`
- 生成：本次新呼叫｜模型 `gpt-5.6-sol`｜prompt `enrich-skill/v2`｜2026-08-15

> 此 Skill 會為 CSV、JSON、JSONL、Parquet 或 Excel 資料集建立資料字典，記錄每個欄位的名稱、型別、說明、單位、範例值、是否允許空值、來源及轉換歷程。使用者需提供資料集路徑與格式，並可逐欄補充欄位含義，或先產生含待填說明的草稿。結果預設寫成資料集同一資料夾內的 `data_dictionary.md`，也可輸出 YAML、JSON 或 CSV。

任務範例句：

- 請為這個 CSV 檔建立 Markdown 資料字典，並儲存在資料集旁邊。　／　*Create a Markdown data dictionary for this CSV file and save it beside the dataset.*
- 請分析這個 Parquet 資料集的每個欄位，並產生以 TODO 佔位的資料字典。　／　*Profile every field in this Parquet dataset and generate a data dictionary with TODO placeholders for descriptions.*
- 請以互動方式記錄這個 JSONL 資料集，逐欄詢問我每個欄位的含義。　／　*Document this JSONL dataset interactively by asking me for each field's meaning.*
- 請為這個 Excel 活頁簿產生 JSON 資料字典，包含可否為空、範例值及轉換歷程。　／　*Generate a JSON data dictionary for this Excel workbook, including nullability, sample values, and transformation history.*
- 請為這個巢狀 JSON 檔加入資料字典，並將欄位清單展平一層。　／　*Add a data dictionary for this nested JSON file and flatten one level for the field listing.*

標籤 — 輸入：`csv dataset`、`json dataset`、`jsonl dataset`、`parquet dataset`、`excel dataset`、`dataset path`、`column descriptions`；輸出：`data dictionary`、`markdown file`、`yaml file`、`json file`、`csv file`、`column profile`、`provenance log`；工具：`python`、`pandas`；依賴：`pandas`、`pyarrow`、`openpyxl`

限制：

- 需要先確認資料集的路徑與格式。
- 欄位含義通常需要使用者提供；若未提供，可改用 `<TODO: describe>` 佔位符。
- 巢狀 JSON 僅展平一層，並在描述中註明巢狀結構。
- 超過 50 個欄位的資料集預設使用佔位模式。
- 二進位或 blob 欄位只記錄為 `bytes`，不進行內容分析。
- 需要安裝 pandas、pyarrow 與 openpyxl。

| 審核狀態 | 審核人 | 審核日期 | 備註 |
| --- | --- | --- | --- |
| **待審** | | | |

### I-29. `pii-flag`（data／已索引）

- 來源：[https://github.com/danielrosehill/Claude-Data-Wrangler-plugin](https://github.com/danielrosehill/Claude-Data-Wrangler-plugin) @ `b12805a62307`，`skills/pii-flag/SKILL.md`
- 生成：重用 golden set 既有增強產出｜模型 `gpt-5.6-sol`｜prompt `enrich-skill/v1`｜2026-08-15

> 此技能會掃描資料集的欄位與儲存格，偵測姓名、電子郵件、電話、地址、政府證件號碼、信用卡、IP 位址、出生日期、精確地理座標及健康資料等個人識別資訊，也可選擇使用機器學習檢查自由文字。使用者需提供要檢查的資料集，並決定是否啟用選用的文字模型；輸出包括遮罩後的逐格 JSONL 報告、欄位摘要、信心分數與刪除、雜湊、泛化或合成替換等修復建議。若發現健康資料、政府證件號碼或信用卡等高風險資訊，會要求在後續公開發布前取得明確確認。

任務範例句：

- 在我把這個資料集發布到 Hugging Face 前，請先掃描其中的個人識別資訊。　／　*Scan this dataset for PII before I publish it to Hugging Face.*
- 請檢查 notes 欄位是否含有姓名、電子郵件、電話號碼或醫療資訊，並在報告中遮罩偵測到的值。　／　*Check whether the notes column contains names, email addresses, phone numbers, or medical information, and mask detected values in the report.*
- 請建立逐儲存格的個資偵測報告，以及包含信心分數與修復建議的逐欄摘要。　／　*Create a cell-level PII report and a per-column summary with confidence scores and remediation recommendations.*
- 請使用選用的機器學習偵測器，找出自由文字欄位中夾帶的個人資料。　／　*Use the optional ML-based detector to find personal data embedded in free-text fields.*
- 在公開分享這個資料集前，請辨識政府證件號碼、信用卡號或健康資料等高風險欄位。　／　*Identify high-risk fields such as government IDs, credit card numbers, or health data before this dataset is shared publicly.*

標籤 — 輸入：`dataset`、`column names`、`column data types`、`cell values`、`free text`、`locale hints`、`ml detection preference`；輸出：`pii_report.jsonl`、`masked cell-level findings`、`column-level summary`、`confidence scores`、`remediation recommendations`、`high-risk pii warning`、`.gitignore entry`；工具：`regular expressions`、`luhn validation`、`header-name heuristics`、`value-pattern heuristics`、`presidio analyzer`、`named entity recognition`；依賴：`pandas`、`phonenumbers`、`python-stdnum`、`presidio-analyzer`、`presidio-anonymizer`、`local ner model`、`synthetic-data-overlay`

限制：**欄位不存在**（本筆為 `enrich-skill/v1` 產出，該版本尚無 `limitations`；見 §2.2）

| 審核狀態 | 審核人 | 審核日期 | 備註 |
| --- | --- | --- | --- |
| **待審** | | | |

### I-30. `add-iso3166`（data／已索引）

- 來源：[https://github.com/danielrosehill/Claude-Data-Wrangler-plugin](https://github.com/danielrosehill/Claude-Data-Wrangler-plugin) @ `b12805a62307`，`skills/add-iso3166/SKILL.md`
- 生成：本次新呼叫｜模型 `gpt-5.6-sol`｜prompt `enrich-skill/v2`｜2026-08-15

> 此技能會為以國家名稱表示的資料集新增 ISO 3166-1 alpha-2、alpha-3 與數字代碼欄位，並預設保留原始檔案格式輸出。使用者需提供 CSV、JSON、JSONL、Parquet 或 XLSX 檔案及國家名稱欄位；技能也會處理常見別名、回報無法配對與空值的資料列，並在適用時更新或提議建立資料字典。遇到既有代碼、次國家地區、歷史實體或單列多國等情況時，會詢問使用者如何處理。

任務範例句：

- 請為 countries.csv 的 country 欄位新增 ISO 3166 alpha-2、alpha-3 與數字國家代碼。　／　*Add ISO 3166 alpha-2, alpha-3, and numeric country codes to the country column in countries.csv.*
- 請使用 nation 欄位為這份 Excel 檔補上 ISO 國家代碼，並以相同格式儲存。　／　*Enrich this Excel file with ISO country codes using the nation column, and save it in the same format.*
- 請只為我的 Parquet 資料集新增 ISO alpha-2 代碼，並回報所有無法配對的國家名稱。　／　*Add only ISO alpha-2 codes to my Parquet dataset and report every country name that cannot be matched.*
- 請將這份 JSONL 檔中的 USA、UK、South Korea 等常見別名對應為 ISO 3166 代碼。　／　*Map common aliases such as USA, UK, and South Korea to ISO 3166 codes in this JSONL file.*
- 請為這份資料集新增國家代碼，國家空值維持空白，並更新同一資料夾中的資料字典。　／　*Add country codes to this dataset, leave null country values blank, and update the data dictionary in the same folder.*

標籤 — 輸入：`country-name dataset`、`csv`、`json`、`jsonl`、`parquet`、`xlsx`、`country column`、`file path`、`data dictionary`；輸出：`iso 3166-1 alpha-2 codes`、`iso 3166-1 alpha-3 codes`、`iso 3166-1 numeric codes`、`enriched dataset`、`unresolved-country report`、`updated data dictionary`；工具：`pandas`、`pycountry`；依賴：`pandas`、`pycountry`、`openpyxl`、`pyarrow`、`add-data-dictionary`、`update-data-dictionary`

限制：

- 需要提供資料集的檔案路徑與格式，支援 CSV、JSON、JSONL、Parquet及 XLSX。
- 若國家欄位有多個候選或含義不明，需要使用者指定正確欄位。
- 若已存在 ISO 代碼欄位，需要使用者決定覆寫或略過。
- 模糊或部分名稱須透過信心門檻配對；無法對應的國家值會列出並統計，不會在未回報的情況下略過。
- 次國家地區需由使用者決定要對應至母國或留空；歷史實體可選擇使用 ISO 3166-3。
- 包含多個國家的資料列需要使用者決定拆分、略過或保留第一個國家；空值會維持為空並回報數量。
- 需要安裝 pandas、pycountry、openpyxl 與 pyarrow。

| 審核狀態 | 審核人 | 審核日期 | 備註 |
| --- | --- | --- | --- |
| **待審** | | | |

---

## 7. 未完成事項

| # | 事項 | 阻擋誰 | 負責 |
| --- | --- | --- | --- |
| 1 | **45 筆的人工審核**（精選 15 必審、已索引 30 抽審 ≥1/3） | CONTENT-005 勾選、`curated-skill-list.md` 檢查 ⑦、閘門測試 D 日 | 內容負責人 |
| 2 | **7 筆精選以 `enrich-skill/v2` 重跑**，補齊 `limitations`（§4.2） | 02 §DISC-003 的「限制」允收 | 內容工序 |
| 3 | **`excel-format` 的字型範例句在地化判定**（§4.2 C-4） | 審核判定 | 內容負責人 |
| 4 | **`anthropics/skills` 免責條款的承接層確認**（§3 註） | 6 筆的審核判定 | 負責人 |
| 5 | **重新匯入 45 筆並 reindex**，讓線上索引文字與審核過的版本對齊（§2.4） | 閘門測試「環境凍結（44 筆索引）」 | 平台工序 |
| 6 | `sokrati/sokrati` 匯入失敗（`description-too-long`，[`import-report.md` §3.4](import-report.md)）。本文件仍為其保留摘要紀錄，但**它沒有線上對應**，目錄實數為 44 | 目錄筆數對帳 | — |

> **02 沒有 CONTENT-005 的需求 ID。** 依 AGENTS.md 文件維護規則「規格新功能先補需求 ID 與允收準則」，CONTENT 系列在 `02` 中完全缺席是既有缺口。本文件不代行補寫規格；**建議補一組 `CONTENT-xxx` 允收準則到 `02`，否則此工作項的勾選永遠只能對照工作項的一句話**。
