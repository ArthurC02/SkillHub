# CONTENT-005：首批 Skill 白話摘要與審核紀錄

- 狀態：**審核已完成（自動化審校，2026-08-16）——45 筆全數通過。** 首輪 3 筆需修改（含 2 筆精選）已由 `enrich-skill/v4` 重跑修正，判定與理由見 [content-review-report.md](../m1/content-review-report.md)。
- ⚠️ **審核方式已變更**：負責人於 2026-08-16 決定不進行人工審核，改以 Script 與 KPI 審校取代（`02` §4.7 已加註修訂，原文以刪除線保留）。**審核對象是線上目錄的該版文字**，不是本文件引用的 2026-08-15 版文字——兩者不逐字相同是預期的（§2.4、`02` 非決定性上限）。
- 產出日：**2026-08-15**（第一輪）；**第二輪 8 筆重跑：2026-08-15**，見 §2.5
- 版本基線：**精選 15 筆全部為 `enrich-skill/v2`**；已索引 30 筆中 8 筆仍為 v1（§2.2）
- 機器可讀產出：[`tools/content/summaries.json`](../../../../tools/content/summaries.json)（45 筆，含 `model`、`prompt_version`、生成時間）
- 產生工具：[`tools/content/generate_summaries.py`](../../../../tools/content/generate_summaries.py)（`--selftest` 為離線自我檢查）
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

**不是**：摘要的產生器。摘要不是在這份文件裡「手寫」出來的——平台的匯入管線（[`services/platform/internal/ingest/enrich.go`](../../../../services/platform/internal/ingest/enrich.go)）在每次匯入時自動呼叫 `POST /v1/enrich-skill` 產生，並由詳情頁的 `enrichment` 區塊呈現（見 §3）。本工序補的是**精選內容的人工審核紀錄**，也就是 ADR-013 明列、但在此之前不存在的「人工抽查機制」。

> **摘要不在本文件就地改寫。** 判定「需修改」的處置是調整 prompt 或把該筆標為人工覆寫，再重跑增強與 reindex（ADR-013 §1「錯誤可由人工修正並觸發重建」）。直接在這裡改字，改到的只是紀錄，不是使用者看到的東西。

### 1.2 誰審、判準、狀態值

> **2026-08-16 執行方式變更（負責人授權）**：本節規定的人工審核未執行，改由 [`tools/content/review_summaries.py`](../../../../tools/content/review_summaries.py) 以 **6 項 KPI ＋ 獨立 Judge 模型（`gpt-5.6-terra`）** 對 **45 筆全量**審校，判定寫在各筆表格，方法與逐筆理由見 [content-review-report.md](../m1/content-review-report.md)。下表的判準（主判準、三條否決條件、狀態值）**未變**，變的是誰執行與覆蓋率（全量取代抽審）。

| 項目 | 規定 |
| --- | --- |
| **審核人** | ~~內容負責人，且**不得是本工序的產生者**。精選（curated）15 筆**必審**；已索引（indexed）30 筆抽審不低於 1/3，且必須涵蓋三個類別~~ → **`automated-review v1`**：Judge 模型 `gpt-5.6-terra`，與生成模型 `gpt-5.6-sol` 分離以滿足「非產生者」；45 筆全量，不抽審 |
| **唯一主判準** | 讀完該筆的摘要與範例句，**一個不懂技術的人能否說出「這個 Skill 能為我做什麼、我要給它什麼」**。答得出＝通過 |
| **否決條件（任一成立即不得記通過）** | (a) **幻覺**：宣稱套件文件沒有寫的能力；(b) **未繁中化**：直接沿用簡體中文原文或簡中在地慣例；(c) **超出 ADR-013 白名單**：出現信任、風險、安全、品質的判斷——這四類永遠不是模型可以產出的 |
| **狀態值** | `待審`／`通過`／`需修改`。**`需修改` 必須寫原因**；沒有原因的 `需修改` 視同 `待審`。本次填入的兩種寫法都落在這三個值內：**「通過（原判需修改，已處理）」**＝初判 `需修改`、重跑生成後重審為 `通過`；**「建議下架」**＝ `需修改` 且重跑 3 次未自癒，附下架建議（**建議而已，本工序不執行下架**） |
| **記錄方式** | 直接編輯本文件每筆下方的審核表格（狀態、審核人、日期、備註）。這份文件就是審核紀錄本身，不另設表單 |

### 1.3 簡體中文來源的處理

45 筆中有 **16 筆**的 `SKILL.md` 原文為簡體中文（`YuYY2004/excel-skills` 15 筆、`nqumich/data-analyst-skill` 1 筆），其中 **6 筆屬精選**。這 16 筆在下方各節以 ⚠️ 標註。

**處理方式：摘要即繁中化呈現層。** 平台**不改寫上游套件內容**（Skill Version 不可變，ADR-003／鐵律 4），也不維護一份翻譯後的 `SKILL.md`。使用者在搜尋結果與詳情頁讀到的繁體中文，是增強管線以 `language: zh-Hant` 產生的摘要與任務範例句；原始簡體 `SKILL.md` 只在進階模式的原文檢視中出現。**因此「改寫為繁中」在本專案的實作意義就是「產出繁中摘要」**，而不是翻譯原始檔。

**閘門測試會直接檢驗這批的三筆。** [`gate-test/task-cards.md` §3](../gate-test/task-cards.md) 的三張卡以簡中來源的 Skill 為 gold：

| 卡 | gold primary | 本文件對應 |
| --- | --- | --- |
| `DOC-4` | `excel-freeze` | §5 C-2 |
| `DAT-3` | `data-analyst` | §5 C-10 |
| `DAT-4` | `excel-deduplicate` | §5 C-14 |

若這三張卡在閘門中集中失敗，依 [`gate-test/analysis.md` §3](../gate-test/analysis.md)：先確認實際入庫的增強文字是否就是本文件審核過的版本，再判斷問題是否不在語言而在別處。

---

## 2. 生成方法與可追溯性

### 2.1 逐字重用生產路徑，不另寫一套

45 筆全部是 `POST /v1/enrich-skill`（[`services/llm`](../../../../services/llm/src/skillhub_llm/enrich.py)）的輸出，模型一律 **`gpt-5.6-sol`**。套件內容以 `import_seed.repack_skill` 在 pin 的 commit 上重組，`skill_md` 與 `file_tree` 與匯入時送出的完全一致。工具本身不含任何 prompt、schema 或模型參數。

### 2.2 計費：兩輪合計 38 次呼叫、8 筆重用

| 輪次／來源 | 筆數 | prompt 版本 | 說明 |
| --- | --- | --- | --- |
| 第一輪：重用 [`tools/goldenset/corpus_enriched/`](../../../../tools/goldenset/corpus_enriched/) | 15 → **剩 8** | `enrich-skill/v1` | 僅在 **repo、路徑、pin commit 三者全等**時重用，也就是被增強的是同一份位元組。原 15 筆中的 7 筆已於第二輪改為新呼叫 |
| 第一輪：新呼叫 | **30** | `enrich-skill/v2` | golden set 語料未涵蓋者。0 筆失敗 |
| **第二輪：重跑**（§2.5） | **8** | `enrich-skill/v2` | 7 筆補 `limitations`（原 v1 重用）＋ `excel-format` 1 筆的字型在地化複驗。0 筆失敗 |

`summaries.json` 現行 `counts`：`total=45`、`reused=8`、`new_calls=8`（`new_calls` 只計最近一次執行的呼叫數）。

> ⚠️ **prompt 版本分歧仍存在，但已不影響精選。** `enrich-skill/v2` 相對 v1 新增 `limitations` 欄位（`summaries.json` 中 `null`＝欄位不存在，與 `[]`＝模型未讀到限制不同）。
>
> - **精選 15 筆：15/15 為 v2，`limitations` 全數存在。** 02 §DISC-003 的「限制」允收在精選層已滿足。
> - **已索引 30 筆：8 筆仍為 v1、`limitations` 為 `null`** — `anthropic-sa/docx`、`pdf`、`pptx`、`xlsx`、`yuyy-excel/excel-filter`、`excel-merge`、`wrangler/date-wrangling`、`pii-flag`。
>
> **這 8 筆為什麼不重跑。** 它們是 `indexed` 層，不在必審範圍；而**線上目錄不受這裡影響**——重新匯入時 45 筆一律由現行 `services/llm` 以 v2 重新生成，包含這 8 筆（見 [`catalog-rebuild-report.md`](../m1/catalog-rebuild-report.md)）。也就是說 02 §DISC-003 的「限制」允收在**線上**是全數滿足的；這 8 筆的 `null` 只是**審核紀錄本身**的版本落差，重跑它們對閘門沒有任何影響，只會多花 8 次呼叫。保留 v1 反而有一個好處：它們的文字與 `tools/goldenset/corpus_enriched/` 逐字相同，是 `MaxCosineDistance = 0.75` 推導語料的可讀對照。
>
> **本節講的是 `summaries.json` 的版本分佈，不是線上目錄的。** 兩者的關係見 §2.4。

### 2.3 金鑰處理

`LITELLM_API_KEY` 只進 `services/llm` 的**行程環境變數**，由 repo 根 `.env` 的 `OPENAI_API_KEY` 匯出，不落任何檔案，也不在 `summaries.json` 或本文件中。

本次為離線內容工序，`LITELLM_BASE_URL` 直接指向 OpenAI，**未經 LiteLLM 閘道**——與 [`golden-query-set.md` §10 附註 1](../m1/golden-query-set.md) 及 [`import-report.md` §1.1](../m1/import-report.md) 同性質：**離線工具的既有例外，產品實作不得比照**（鐵律 8、ADR-017）。

### 2.4 ⚠️ 本次生成的文字不等於線上索引的文字

增強是**每次匯入重新生成**的，LLM 輸出非決定性。因此：

- 人工審核實際批准的是**「某個模型＋某個 prompt 版本在某份套件上的產出品質」**，而不是一串固定字串。這是 CONTENT-005 這個工作項的結構性上限，不是本次工序的疏漏。
- **第二輪重跑替這條上限提供了實測值**：8 筆在同模型、同 prompt 版本下重跑，8/8 的語意涵蓋一致、可理解性一致，漂移落在措辭與 `tags` 粒度（§2.5）。**同版本重跑的品質可複現，逐字內容不可複現**——這正是本節主張的形狀。
- **線上目錄已依 §7 第 5 項重建完成**，基線與統計見 [`catalog-rebuild-report.md`](../m1/catalog-rebuild-report.md)。線上 45 筆一律為 `enrich-skill/v2`，而本文件的 8 筆已索引項仍為 v1（§2.2）——**兩者不同是預期的**。
- **判定衝突時以線上為準。** 本文件是審核紀錄，閘門測試打的是線上目錄。審核人若對某筆的線上文字與本文件落差有疑慮，處置是實查該筆詳情頁後重審，而不是改本文件的字。

### 2.5 第二輪重跑（8 筆）

- 執行：2026-08-15，`python tools/content/generate_summaries.py --url http://127.0.0.1:8000 --only "<8 個 id>"`
- 結果：**8 次呼叫、8 筆成功、0 筆失敗**，模型 `gpt-5.6-sol`、prompt `enrich-skill/v2`
- 工具變更：`generate_summaries.py` 新增 `--only`（逗號分隔的 id 子字串），對指定筆強制重新呼叫、忽略既有列與重用；其餘 37 筆逐位元保持不動

| 重跑對象 | 筆數 | 理由 | 結果 |
| --- | --- | --- | --- |
| C-5、C-6、C-10、C-11、C-12、C-13、C-14 | 7 | 原為 v1 重用，缺 `limitations`（§4.2） | **7/7 補齊**，預判全數由「需修改」改為「預判通過」 |
| C-4 `excel-format` | 1 | 字型在地化複驗（§4.2） | 用語改善但**字型名稱仍為「微軟雅黑」**，改列為**審核裁定事項**，非生成錯誤（見 C-4） |

**同版本重跑的漂移形狀**（對 §2.4 的實測補充）：

| 觀察 | 例 |
| --- | --- |
| 語意涵蓋一致 | 8/8 的「做什麼、要給什麼」與第一輪相同 |
| 措辭改善 | C-11、C-13 第一輪的「缺主詞」問題自行消失；C-4 `字體`→`字型` |
| 新增具體上限 | C-14 多出「僅掃描前 49 列、前 9 欄」（已回查原文，屬轉述） |
| `tags` 粒度漂移 | C-6 的 `dependencies` 由四個檔名縮為一句概括 |
| 體例小幅不一致 | C-12 用「此 Skill」，其餘用「此技能」 |

**沒有出現的**：幻覺、簡體殘留、超出 ADR-013 白名單的判斷（§4.1 已就 45 筆重掃）。

---

## 3. CONTENT-005 允收對照

> **更新（2026-08-16）：`02` 已補上 CONTENT-005 的需求 ID 與允收準則**（commit `b2a7690`，§4.7）。本節原本記錄的是「02 完全沒有 CONTENT 系列允收準則」這個缺口，該缺口已關閉。下表保留原有的三份依據對照，並在最前面加上 `02` 的正式允收。
>
> **`02` 的兩條新準則對本工序有直接影響**：
>
> 1. 「一般模式顯示所需的『限制』欄位須有值；缺值者不得判定為通過」——第二輪重跑（§2.5）正是為此，精選 15/15 已有值。
> 2. 「**非決定性上限**：審核判定只對**已入庫的該版文字**生效；任何重跑增強後，須重新確認入庫文字與審核過的版本一致，否則該筆退回 `待審`」——**這條決定了審核要對著什麼看**，見 §2.4 與 [`catalog-rebuild-report.md` §5](../m1/catalog-rebuild-report.md)。**2026-08-16 更新**：45 筆已完成自動化審校，且審校一律讀線上文字，因此不存在「審過的版本與入庫版本不一致」的退回問題；重跑過的 10 筆是先重跑、再對重跑後的線上文字重審。

| 依據 | 要求 | 本工序的對應 | 判定 |
| --- | --- | --- | --- |
| **`02` §4.7 CONTENT-005**（2026-08-16 修訂版） | 見上（生產路徑產生、白名單、限制有值、審核人分離、主判準、否決條件、可追溯、非決定性上限） | 全部欄位與工序均已備齊；審核已由自動化審校完成（45/45 通過） | ✅ **允收達成**（逐條對照見 [審校報告 §8](../m1/content-review-report.md)） |
| `03-work-items.md` CONTENT-005 | 對首批 Skill 產生一般使用者可理解的摘要 | 45/45 筆皆有繁中白話摘要與 3–5 則雙語任務範例句 | ✅ 已產生；**可理解性主判準 45/45 通過**（審校 KPI2） |
| PDM-002 檢查 ⑦ | 非技術使用者讀得懂它做什麼、需要什麼輸入 | 已審校：主判準 45/45、精選 15/15 通過 | ✅ **⑦ 改記 `pass`** |
| ADR-013「需要人工抽查機制」 | 建立抽查機制 | 本文件即該機制：判定欄位、判準、否決條件、改法（重跑增強而非就地改字） | ✅ 已建立 |
| 02 §DISC-003 | 一般模式顯示功能、限制、輸入、輸出、依賴、權限、來源、License、相容性 | 「功能」＝`summary`；「輸入／輸出／依賴」＝`tags`；「限制」＝`limitations` | ✅ **精選 15/15 已有 `limitations`**（第二輪重跑補齊，§2.5）。線上目錄 45/45 為 v2，同樣全數有 `limitations` |
| 「呈現於平台」 | — | 已由 `ingest/enrich.go` 於匯入時自動產生（commit `b144bea`），由 `catalog/detail.go` 的 `enrichmentFrom` 供給 `enrichment` 區塊（commit `ebc4036`），契約見 `contracts/openapi/public.yaml`（`summary` 恆為套件自身 frontmatter，模型產出一律落在 `enrichment` 之下並標示）。測試：`services/platform/internal/ingest/enrich_test.go`、`services/llm/tests/test_enrich.py` | ✅ 已呈現；本工序補的是**人工審核紀錄** |

> **`anthropics/skills` 的免責條款怎麼處理。** `curated-skill-list.md` §3 腳註 3 要求把 README 的 "provided for demonstration and educational purposes only" 納入摘要措辭考量。**本工序的判定是：不納入摘要。** 該句是使用免責，屬於信任／品質陳述，而 ADR-013 白名單明令模型產出不得包含這一類判斷（`enrich.py` 的 system prompt 亦明文禁止）。它應由詳情頁的 License／來源區塊承接（ADR-021 的兩軸答案，`license.status` 目前最高只到 `declared`）。**此解讀需負責人確認**；若負責人要求摘要承載免責，則需改的是 ADR-013 白名單，不是這批摘要。涉及 6 筆（`brand-guidelines`、`internal-comms`、`docx`、`pdf`、`pptx`、`xlsx`）。

---

## 4. 抽查結果（精選 15 筆逐一目檢）

抽查者不是審核人，**下表是給審核人的預判，不是判定**。~~所有正式審核欄位仍為「待審」。~~

> **2026-08-16**：正式判定已由自動化審校產生（各筆表格）。本節預判保留供對照——**預判 14 通過／1 待裁定；首輪實際審校為精選 13 通過／2 需修改，經 v4 重跑後 15/15 通過**；差異出在預判只做目檢與關鍵詞掃描，沒有逐條事實宣稱比對原文（`internal-comms` 的「清楚／精簡」與 `excel-format` 的「預設值講成必填」都是逐條比對才抓得到）。

### 4.1 機械檢查

| 項目 | 結果 |
| --- | --- |
**第二輪後已就 45 筆重掃**，結果與第一輪相同：

| 項目 | 結果 |
| --- | --- |
| 簡體字殘留（掃 45 筆的 `summary` ＋ `task_examples.zh_hant` ＋ `limitations`） | **0 筆**。無任何簡體字元（含重跑的 8 筆） |
| 超出 ADR-013 白名單的宣稱（信任／風險／安全／品質） | **0 筆**。關鍵詞掃描命中 3 筆（`course-quiz-builder`「不具安全性」、`standardise-country-names`「低可信度」、`pii-flag`「高風險資訊」），逐筆回查後**皆為原文自述的轉述**（答案金鑰只做混淆、模糊比對的信心分數、個資類別分級），不是對 Skill 本身的信任／品質判斷，因此不計為違反。三筆均為第一輪產出，本輪未動 |
| 幻覺抽驗（對可疑數據回查 pin commit 的 `SKILL.md` 原文） | **0 筆**。`excel-find-duplicates` 的「168MB／33 萬列約 150 秒」實查原文第 3 條為 "pandas reading 168MB/330K rows takes ~150s"；第二輪 `excel-deduplicate` 新增的「前 49 列、前 9 欄」實查原文為 `range(1, min(50, ...))` × `range(1, min(10, ...))`。兩者皆屬轉述 |

### 4.2 目檢預判

**下表已依第二輪重跑（§2.5）更新。**

| 筆 | 預判 | 原因 |
| --- | --- | --- |
| C-4 `excel-format` | **審核裁定**（原「需修改」） | v2 重跑後用語改善（`字體`→`字型`、`12 點`→`12 號`），但字型名稱仍為「微軟雅黑」。這是 prompt「只轉述、不改寫」的必然結果，**不是生成錯誤**。請審核人在「轉述忠實性 vs 在地化」之間裁定；選擇在地化＝改 prompt 至 v3 並全量重跑，不是編輯本文件。判定依據見 C-4 |
| C-5 `brand-guidelines`、C-6 `internal-comms`、C-10 `data-analyst`、C-11 `data-cleanliness-scan`、C-12 `csv-to-json`、C-13 `text-to-numeric`、C-14 `excel-deduplicate` | **預判通過**（原「需修改」，共 7 筆） | `limitations` 已於第二輪以 v2 補齊（7/7），02 §DISC-003 的「限制」允收已可滿足。摘要文字第一輪即無問題，重跑後語意涵蓋一致 |
| C-11 `data-cleanliness-scan`、C-13 `text-to-numeric` | **已解決**（原附註） | 第一輪的「摘要句缺主詞」在重跑後自行消失，兩筆均已回到「此技能……」體例 |
| C-12 `csv-to-json` | 附註（不計為需修改） | 主詞用「此 Skill」而非其餘筆的「此技能」。體例小不一致，不影響可理解性 |
| C-6 `internal-comms` | 附註（不計為需修改） | `dependencies` 由第一輪的四個具體檔名縮為一句「對應的範例指南檔案」。兩者都不算錯，是 §2.4 所述非決定性的具體表現 |
| 其餘 7 筆 | **預判通過**（未重跑） | `excel-insert`、`excel-freeze`、`handoff`、`humanizer`、`line-edit`、`ai-written-check`、`excel-find-duplicates` |

**精選 15 筆的預判合計：14 筆預判通過、1 筆待審核人裁定（C-4）、0 筆需修改。**

> **一個正面發現：近義群已自行差異化。** `line-edit` 與 `ai-written-check` 的 `limitations` 明確寫出「這件事屬於 `cringe-check`／`full-review`」，正是 [`gate-test/analysis.md` §3 修正項 C3](../gate-test/analysis.md) 要求的「什麼時候該用我」。這是 v2 新增 `limitations` 的副產品——**第二輪把 7 筆補成 v2 後，這個效果已再現於 C-11**（其限制主動指向 `json-restructure`），對 WRI-3 這張近義群卡有直接好處。

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
| **通過** | automated-review v1（gpt-5.6-terra judge；負責人 2026-08-16 授權） | 2026-08-16 | KPI1–6 全數通過（KPI6 餘弦 0.9206） |

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
| **通過** | automated-review v1（gpt-5.6-terra judge；負責人 2026-08-16 授權） | 2026-08-16 | KPI1–6 全數通過（KPI6 餘弦 0.9607） |

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
| **通過** | automated-review v1（gpt-5.6-terra judge；負責人 2026-08-16 授權） | 2026-08-16 | KPI1–6 全數通過（KPI6 餘弦 0.9406） |

### C-4. `excel-format`（documents／精選）

- 來源：[https://github.com/YuYY2004/excel-skills](https://github.com/YuYY2004/excel-skills) @ `c15e51e20424`，`claude/skills/excel-format/SKILL.md`
- 生成：**第二輪重跑**｜模型 `gpt-5.6-sol`｜prompt `enrich-skill/v2`｜2026-08-15（§2.5）
- ⚠️ **來源 SKILL.md 為簡體中文**；下列摘要與範例句即繁中化呈現層（見 §1.3）。

> 此技能可統一調整 Excel 工作表的字型、字號、粗體、斜體、顏色、對齊方式、欄寬與列高，並可套用至整張表、表頭、資料區、指定欄列、範圍或符合條件的儲存格。使用者需要提供 Excel 檔案、想修改的格式參數及套用範圍；若只要求「美化」，則會使用內建的表頭與資料區格式預設。流程包含解析需求、檢查目前格式、執行修改、儲存備份及驗證結果。

任務範例句：

- 請把表頭設為粗體並置中，字型改成微軟雅黑 12 號。　／　*Make the header row bold and centered, using Microsoft YaHei at 12 pt.*
- 請將整張工作表改成 Arial 10 號，並把金額欄靠右對齊。　／　*Change the entire worksheet to Arial 10 pt and right-align the amount column.*
- 請自動調整所有欄寬，並讓資料區垂直置中。　／　*Automatically adjust all column widths and vertically center the data area.*
- 請將 B 欄與 E 欄的文字改成藍色斜體，其他格式保持不變。　／　*Set columns B and E to blue italic text without changing their other formatting.*
- 請使用預設格式方案美化這個 Excel 檔案。　／　*Beautify this Excel file using the default formatting preset.*

標籤 — 輸入：`excel workbook`、`format specifications`、`worksheet scope`、`row and column ranges`、`cell conditions`；輸出：`formatted excel workbook`、`format verification results`、`timestamped backups`；工具：`python`、`openpyxl`、`pandas`、`numpy`；依賴：`excel-safe-workflow`、`openpyxl`、`pandas`、`numpy`、`system fonts`

限制：

- 執行前必須完成需求解析與工作表勘察，執行後必須驗證結果。
- 操作前必須建立時間戳備份，成功後保留最新 3 份；若操作失誤，需刪除損壞檔案並由備份還原。
- 字型是否可用取決於開啟 Excel 檔案的系統。
- 合併儲存格的格式需要另外處理。
- 自動調整欄寬時，中文字符按 2 個寬度計算，且內容所述上限為 40 個字符。
- 使用自動欄寬功能時需要 pandas 與 numpy。

> **抽查預判：建議審核人裁定——轉述忠實性 vs 在地化。**
>
> 第二輪以 v2 重跑後，用語確有改善：`字體`→`字型`、`12 點`→`12 號`，是更自然的繁中習慣；但**字型名稱仍為「微軟雅黑」**，與第一輪一致。
>
> 這不是生成錯誤。來源 `SKILL.md` 原文寫的就是 `"微软雅黑"`，`enrich-skill` 的 system prompt 明令「Describe only what the content states. Do not invent capabilities.」——把它改寫成「微軟正黑體」等於讓模型**改動它所轉述的事實**，那才是違反 prompt 的行為。兩輪都照抄，是 prompt 設計的必然結果，不是抽樣波動。
>
> **`02` §4.7 CONTENT-005 的否決條件 (b) 讓這題更尖銳**：該條寫的是「未依文件語言慣例在地化」。字面上，「微軟雅黑」確實是簡中在地慣例的字型名；但它同時是**原文的事實**。允收準則沒有規定「轉述專有名詞時該不該在地化」，這個邊界只能由審核人裁定，本工序無權代行。
>
> 因此這一題不是「生成要不要重跑」，而是**審核人要在兩個都成立的價值之間裁定**：
>
> | 選項 | 主張 | 代價 |
> | --- | --- | --- |
> | **A. 維持忠實轉述**（現況） | 摘要是文件的轉述層，原文寫什麼就寫什麼；使用者看到的與套件實際會做的一致 | 繁中使用者讀到不熟悉、且在繁中系統上不必然存在的字型名 |
> | **B. 要求在地化** | 範例句是「使用者會怎麼打字」的示範，寫繁中使用者不會打的字型名，示範價值降低 | 需改 `enrich.py` 的 prompt（加入在地化指示），影響全部 45 筆的重跑；且模型改寫專有名詞的邊界難以界定 |
>
> **本工序不代行裁定，也不就地改字。** 若採 B，處置是改 prompt 版本（v3）後全量重跑，不是編輯本文件。

| 審核狀態 | 審核人 | 審核日期 | 備註 |
| --- | --- | --- | --- |
| **通過**（原判需修改，已處理） | automated-review v1（gpt-5.6-terra judge；負責人 2026-08-16 授權） | 2026-08-16 | 初判需修改（把預設值寫成必填＋字型未加註）。**`enrich-skill/v4` 重跑後通過**：摘要改為「未指定的格式維持不變，範圍未指定時預設為整張表」，字型為 `微软雅黑（繁中：微軟雅黑）`。另有一項判準範圍修正（KPI3 只掃 zh-Hant 欄位，英文範例句的原名引用改記為觀察值），**需負責人追認**，見 [審校報告 §9 第 2 條](../m1/content-review-report.md)。**線上文字已更新，本節引文為 2026-08-15 版**；判定對線上文字生效（KPI6 餘弦 0.9109） |

### C-5. `brand-guidelines`（writing／精選）

- 來源：[https://github.com/anthropics/skills](https://github.com/anthropics/skills) @ `f6656c1256d5`，`skills/brand-guidelines/SKILL.md`
- 生成：**第二輪重跑**｜模型 `gpt-5.6-sol`｜prompt `enrich-skill/v2`｜2026-08-15（原為 v1 重用，§2.5）

> 此技能會將 Anthropic 的官方品牌色彩與字體風格套用到需要品牌視覺、版面格式或公司設計規範的成品。它需要一份待調整的視覺成品，並依背景智慧選色、保留文字層級與格式，將 Poppins 用於 24pt 以上標題、Lora 用於內文，也會以橙、藍、綠色循環處理非文字圖形。若指定字型不可用，則自動改用 Arial 與 Georgia。

任務範例句：

- 請將 Anthropic 的官方色彩與字體套用到這份簡報。　／　*Apply Anthropic’s official colors and typography to this presentation.*
- 請用 Poppins 標題、Lora 內文及 Anthropic 品牌色重新設計這份成品。　／　*Restyle this artifact with Poppins headings, Lora body text, and Anthropic brand colors.*
- 請將所有 24pt 以上的標題套用品牌標題字體，並保留現有文字層級。　／　*Format all headings of 24pt or larger with the brand heading font while preserving the existing text hierarchy.*
- 請將橙色、藍色和綠色的品牌強調色套用到這份簡報的非文字圖形。　／　*Apply orange, blue, and green accent colors to the non-text shapes in this deck.*

標籤 — 輸入：`visual artifacts`、`text headings`、`body text`、`background colors`、`non-text shapes`；輸出：`anthropic-branded artifacts`、`styled typography`、`brand color formatting`、`accent-colored shapes`；工具：`python-pptx rgbcolor`；依賴：`python-pptx`、`system-installed fonts`、`poppins`、`lora`、`arial`、`georgia`

限制：

- 若要獲得最佳效果，環境中需預先安裝 Poppins 與 Lora 字型；若無法使用，標題與內文會分別改用 Arial 與 Georgia。
- 色彩是透過 python-pptx 的 RGBColor 類別套用。

> **抽查預判：預判通過** — `limitations` 已補齊（v2 重跑），02 §DISC-003 的「限制」允收已可滿足。摘要文字與第一輪同義，敘述更精確（補上「24pt 以上」的標題門檻）。

| 審核狀態 | 審核人 | 審核日期 | 備註 |
| --- | --- | --- | --- |
| **通過** | automated-review v1（gpt-5.6-terra judge；負責人 2026-08-16 授權） | 2026-08-16 | KPI1–6 全數通過（KPI6 餘弦 0.9572） |

### C-6. `internal-comms`（writing／精選）

- 來源：[https://github.com/anthropics/skills](https://github.com/anthropics/skills) @ `f6656c1256d5`，`skills/internal-comms/SKILL.md`
- 生成：**第二輪重跑**｜模型 `gpt-5.6-sol`｜prompt `enrich-skill/v2`｜2026-08-15（原為 v1 重用，§2.5）

> 此技能協助撰寫公司內部溝通內容，包括 3P 進度更新、公司電子報、FAQ 回覆、狀態報告、主管更新、專案更新與事件報告。使用者需提供要撰寫的溝通類型及相關內容或背景；技能會套用對應範例指南所指定的格式、語氣與內容蒐集方式。若沒有相符指南，會要求使用者補充期望格式或更多資訊。

任務範例句：

- 請根據這些進度、後續計畫與阻礙，撰寫本週的 3P 更新。　／　*Write a 3P update for this week using these progress items, next steps, and blockers.*
- 請將這些公司公告整理成一份給全體員工的內部電子報。　／　*Turn these company announcements into an internal newsletter for all employees.*
- 請針對新的遠距工作政策，草擬清楚的員工 FAQ 回覆。　／　*Draft clear FAQ answers for employees about the new remote-work policy.*
- 請整理專案目前狀態、里程碑與問題，撰寫一份主管更新。　／　*Create a leadership update summarizing the project's current status, milestones, and issues.*
- 請根據這份時間線、影響摘要與處理結果，撰寫內部事件報告。　／　*Write an internal incident report from this timeline, impact summary, and resolution information.*

標籤 — 輸入：`溝通類型`、`進度、計畫與問題`、`公司消息`、`常見問題`、`狀態資訊`、`專案更新`、`事件資訊`、`期望格式`、`背景資訊`；輸出：`3p 更新`、`公司電子報`、`faq 回覆`、`狀態報告`、`主管更新`、`專案更新`、`事件報告`、`內部溝通稿`；工具：（無）；依賴：`對應的範例指南檔案`

限制：

- 若溝通類型不符合現有指南，需先取得對期望格式的澄清或更多背景資訊。
- 需要存取 `examples/` 目錄中與溝通類型相符的指南檔案，才能依其格式、語氣及內容蒐集要求撰寫。

> **抽查預判：預判通過** — `limitations` 已補齊（v2 重跑）。摘要文字與第一輪同義。
>
> 附註：本輪的 `dependencies` 從第一輪的四個具體檔名（`examples/3p-updates.md` 等）縮成一句中文「對應的範例指南檔案」。**兩者都不算錯**——DISC-003 的「依賴」欄要顯示的是使用者要準備什麼，具體檔名對非技術使用者資訊量較低。但這說明 `tags` 的粒度在兩次呼叫之間會漂移，與 §2.4 的結構性上限同源。

| 審核狀態 | 審核人 | 審核日期 | 備註 |
| --- | --- | --- | --- |
| **通過**（原判需修改，已處理） | automated-review v1（gpt-5.6-terra judge；負責人 2026-08-16 授權） | 2026-08-16 | 初判需修改（為輸出加「清楚／精簡」等品質形容詞，屬 ADR-013 白名單外）。**`enrich-skill/v4` 重跑後通過**：形容詞全數消失，改為轉述範例指南的格式與語氣指示。**線上文字已更新，本節引文為 2026-08-15 版**；判定對線上文字生效（KPI6 餘弦 0.9521） |

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
| **通過** | automated-review v1（gpt-5.6-terra judge；負責人 2026-08-16 授權） | 2026-08-16 | KPI1–6 全數通過（KPI6 餘弦 0.9212） |

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
| **通過** | automated-review v1（gpt-5.6-terra judge；負責人 2026-08-16 授權） | 2026-08-16 | KPI1–6 全數通過（KPI6 餘弦 0.8922），低於 0.90：線上文字與本節引文非逐字相同，判定對線上文字生效 |

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
| **通過** | automated-review v1（gpt-5.6-terra judge；負責人 2026-08-16 授權） | 2026-08-16 | KPI1–6 全數通過（KPI6 餘弦 0.9393） |

### C-10. `data-analyst`（data／精選）

- 來源：[https://github.com/nqumich/data-analyst-skill](https://github.com/nqumich/data-analyst-skill) @ `0ba9d17ed275`，`SKILL.md`
- 生成：**第二輪重跑**｜模型 `gpt-5.6-sol`｜prompt `enrich-skill/v2`｜2026-08-15（原為 v1 重用，§2.5）
- ⚠️ **來源 SKILL.md 為簡體中文**；下列摘要與範例句即繁中化呈現層（見 §1.3）。

> 此技能可讀取、清理、驗證、篩選、彙總及連接 CSV、TSV、XLSX、JSON 與 JSON Lines 資料，並提供資料品質檢查和分析報告架構。它可將含合併儲存格、多層表頭、空白列欄、格式化數字或混合日期格式的 XLSX 標準化，也能合併多個工作表及分塊處理大型 CSV。使用者需提供資料檔案，以及所需的工作表、欄位、篩選條件、彙總方式、驗證規則或輸出路徑等設定。

任務範例句：

- 請將這份格式混亂的 Excel 活頁簿標準化，填入合併儲存格、合併兩列表頭、清理貨幣數值，並另存為 CSV。　／　*Standardize this messy Excel workbook by filling merged cells, combining its two header rows, cleaning currency values, and saving the result as CSV.*
- 請篩選 sales.csv，只保留 amount 大於 1,000 的資料列，並將結果寫入 filtered.csv。　／　*Filter sales.csv to rows where amount is greater than 1,000 and write the result to filtered.csv.*
- 請依 category 分組，計算 amount 的總和，並匯出彙總檔案。　／　*Group the data by category, calculate the sum of amount, and export a summary file.*
- 請使用 customer_id 對 orders.csv 與 customers.csv 執行左連接。　／　*Join orders.csv and customers.csv on customer_id using a left join.*
- 請驗證 amount、email 與 date 欄位，列出無效資料列，並產生資料品質報告。　／　*Validate the amount, email, and date columns, then list invalid rows and produce a data quality report.*

標籤 — 輸入：`csv files`、`tsv files`、`xlsx files`、`json files`、`jsonl files`、`sheet names`、`column names`、`filter expressions`、`aggregation rules`、`validation schemas`、`output paths`、`transform functions`；輸出：`pandas dataframes`、`cleaned datasets`、`csv files`、`xlsx files`、`quality reports`、`validation errors`、`aggregated summaries`、`joined datasets`、`analysis reports`；工具：`data_ops.py`、`python api`、`command line`；依賴：`python`、`pandas`、`openpyxl`

限制：

- 僅列明支援 CSV、TSV、XLSX、JSON 與 JSON Lines 格式。
- 需要安裝 python3 或 python；相關程式碼亦使用 pandas 與 openpyxl。
- 大型檔案的串流處理範例使用 CSV 輸入與輸出。

> 🎯 **閘門卡 `DAT-3` 的 gold primary**（`gate-test/task-cards.md` §3）。本筆的摘要品質會被閘門直接檢驗。

> **抽查預判：預判通過** — `limitations` 已補齊（v2 重跑）。摘要文字與第一輪同義，涵蓋範圍一致。

| 審核狀態 | 審核人 | 審核日期 | 備註 |
| --- | --- | --- | --- |
| **通過** | automated-review v1（gpt-5.6-terra judge；負責人 2026-08-16 授權） | 2026-08-16 | KPI1–6 全數通過（KPI6 餘弦 0.9363） |

### C-11. `data-cleanliness-scan`（data／精選）

- 來源：[https://github.com/danielrosehill/Claude-Data-Wrangler-plugin](https://github.com/danielrosehill/Claude-Data-Wrangler-plugin) @ `b12805a62307`，`skills/data-cleanliness-scan/SKILL.md`
- 生成：**第二輪重跑**｜模型 `gpt-5.6-sol`｜prompt `enrich-skill/v2`｜2026-08-15（原為 v1 重用，§2.5）

> 此技能會掃描一個或多個 CSV、Parquet、JSON、JSONL 或 Excel 平面資料檔，找出可能阻礙 SQL 匯入或造成分析錯誤的資料問題。它需要使用者提供檔案、確認格式與目標，並會檢查欄位型別、日期、空值、重複鍵、編碼、範圍、檔案結構及跨欄位邏輯一致性。輸出是按嚴重程度排序的 Markdown 報告，也可選擇產生 JSON 版本，包含受影響筆數、遮蔽個資後的樣本值與具體修復建議。

任務範例句：

- 請掃描這些 CSV 檔案，告訴我它們是否乾淨到足以匯入 PostgreSQL。　／　*Scan these CSV files and tell me whether they are clean enough to load into PostgreSQL.*
- 請檢查這份 Excel 活頁簿是否有混合資料型別、格式錯誤的日期、偽裝空值及重複鍵。　／　*Check this Excel workbook for mixed data types, malformed dates, disguised nulls, and duplicate keys.*
- 請稽核這些 JSONL 檔案，並產生按嚴重程度排序的 Markdown 與 JSON 資料潔淨度報告。　／　*Audit these JSONL files and produce a severity-ranked cleanliness report in Markdown and JSON.*
- 請找出這份 CSV 資料傾印中的分隔符漂移、欄位數不一致、編碼異常及混合換行格式。　／　*Find delimiter drift, ragged rows, encoding artefacts, and mixed line endings in this CSV data dump.*
- 請分析這個大型 Parquet 檔案是否有 SQL 匯入問題，並在報告中註明抽樣策略。　／　*Profile this large Parquet file for SQL ingestion issues and note the sampling strategy in the report.*

標籤 — 輸入：`csv files`、`parquet files`、`json files`、`jsonl files`、`excel files`、`target database`；輸出：`cleanliness_report.md`、`cleanliness_report.json`、`ranked issue report`、`remediation suggestions`；工具：`pandas`、`chardet`、`dateutil`；依賴：`pandas`、`pyarrow`、`openpyxl`、`chardet`、`python-dateutil`

限制：

- 對非常大的檔案會採策略性抽樣，並在報告中註明樣本大小。
- 對高度巢狀的 JSON，主要只檢查記錄間的結構與結構描述一致性；深入分析前建議先用 `json-restructure` 攤平。
- 離群值只會列為觀察結果，不會視為缺陷或建議移除。
- 修復技能只會依建議順序提供，不會自動執行。
- 需要安裝 pandas、pyarrow、openpyxl、chardet 與 python-dateutil。

> **抽查預判：預判通過** — `limitations` 已補齊（v2 重跑），且第一輪指出的「摘要句缺主詞」問題**一併消失**：本輪開頭已是「此技能會掃描……」，與其餘筆體例一致。第二條限制主動指向 `json-restructure`，是 §4.2 註記的近義群差異化效果再現。

| 審核狀態 | 審核人 | 審核日期 | 備註 |
| --- | --- | --- | --- |
| **通過** | automated-review v1（gpt-5.6-terra judge；負責人 2026-08-16 授權） | 2026-08-16 | KPI1–6 全數通過（KPI6 餘弦 0.9054） |

### C-12. `csv-to-json`（data／精選）

- 來源：[https://github.com/danielrosehill/Claude-Data-Wrangler-plugin](https://github.com/danielrosehill/Claude-Data-Wrangler-plugin) @ `b12805a62307`，`skills/csv-to-json/SKILL.md`
- 生成：**第二輪重跑**｜模型 `gpt-5.6-sol`｜prompt `enrich-skill/v2`｜2026-08-15（原為 v1 重用，§2.5）

> 此 Skill 可在 CSV、JSON 陣列與 JSONL 之間雙向轉換，並處理欄位對應、CSV 方言與編碼、空值、型別推斷及巢狀資料攤平。使用者需要提供來源與目標檔案路徑、轉換方向，並視需要確認型別推斷、空值標記及巢狀欄位的處理方式。它也會驗證輸出能否重新載入，並比對來源與輸出的資料筆數。

任務範例句：

- 請將 customers.csv 轉成 JSON 陣列，並將所有欄位保留為字串。　／　*Convert customers.csv into a JSON array while keeping every field as a string.*
- 請把這個以分號分隔的 CSV 轉成 JSONL，並推斷數字、布林值與空值。　／　*Convert this semicolon-delimited CSV file to JSONL and infer numbers, booleans, and null values.*
- 請將 orders.json 的巢狀物件用點號鍵名攤平，然後匯出為 CSV。　／　*Flatten the nested objects in orders.json with dotted keys and export the result as CSV.*
- 請逐行將 events.jsonl 轉成 CSV，並確認輸出筆數與來源一致。　／　*Convert events.jsonl to CSV line by line and verify that the output record count matches the source.*
- 請分塊將這個大型 CSV 檔案轉成 JSONL，並回報實際使用的編碼。　／　*Convert this large CSV file to JSONL in chunks and report which encoding was used.*

標籤 — 輸入：`csv files`、`json arrays`、`jsonl files`、`file paths`、`csv dialect settings`、`encoding settings`、`type inference options`、`null sentinels`、`nested field options`；輸出：`csv files`、`json arrays`、`jsonl files`、`row counts`、`object counts`、`encoding reports`；工具：`csv`、`json`、`csv.sniffer`、`pandas`、`pd.read_csv`；依賴：`pandas`、`python standard library`

限制：

- 必須提供來源與目標檔案路徑，以及要執行的轉換方向。
- CSV 方言若無法明確偵測，需要使用者指定分隔符、引號字元或標頭設定。
- CSV 型別推斷與空值標記清單需要使用者確認；預設會將值保留為字串。
- 若要使用 pandas 進行型別推斷或大型檔案分塊處理，需要先安裝 pandas。
- JSON 沒有原生日期時間型別，因此日期與時間會輸出為 ISO 8601 字串。
- 輸入超過 1 GB 時會提出警告，並建議使用 JSONL 而非 JSON 陣列。

> **抽查預判：預判通過** — `limitations` 已補齊（v2 重跑），且六條全部是可操作的前置條件（要準備什麼、什麼情況要人工確認），正是 PDM-002 檢查 ⑦「需要什麼輸入」要的資訊。
>
> 附註：本輪主詞改用「此 Skill」而非其餘筆的「此技能」，是體例上的小不一致，不影響可理解性。

| 審核狀態 | 審核人 | 審核日期 | 備註 |
| --- | --- | --- | --- |
| **通過** | automated-review v1（gpt-5.6-terra judge；負責人 2026-08-16 授權） | 2026-08-16 | KPI1–6 全數通過（KPI6 餘弦 0.8997），低於 0.90：線上文字與本節引文非逐字相同，判定對線上文字生效 |

### C-13. `text-to-numeric`（data／精選）

- 來源：[https://github.com/danielrosehill/Claude-Data-Wrangler-plugin](https://github.com/danielrosehill/Claude-Data-Wrangler-plugin) @ `b12805a62307`，`skills/text-to-numeric/SKILL.md`
- 生成：**第二輪重跑**｜模型 `gpt-5.6-sol`｜prompt `enrich-skill/v2`｜2026-08-15（原為 v1 重用，§2.5）

> 此技能會把含貨幣符號、千位分隔符、百分比、縮寫倍率或會計負號的文字欄位轉成可分析的數值欄位。它需要一份資料集及目標欄位；若未指定欄位，會找出大多數內容看似數字的文字欄位，並請使用者確認格式與百分比處理方式。它會保留或另存原始值、記錄格式至資料字典，並列出無法解析的值及其列索引。

任務範例句：

- 請把價格欄中像「$4.27」和「$1,234.56」的值轉成數字，並保留原始值。　／　*Convert the price column containing values such as "$4.27" and "$1,234.56" into numbers while preserving the original values.*
- 請找出這份資料集中大部分內容看似數字的文字欄位，並將它們轉成數值欄位。　／　*Find text columns in this dataset that are mostly numeric and convert them into numeric columns.*
- 請解析「€1.2M」、「2.5K」和「3.5%」等值，並在資料字典中記錄套用的倍率與百分比規則。　／　*Parse values such as "€1.2M", "2.5K", and "3.5%", and record the applied scale and percentage rules in the data dictionary.*
- 請轉換像「(500)」這類會計負數，並回報所有無法解析的列，但不要刪除它們。　／　*Convert accounting negatives such as "(500)" and report any rows that cannot be parsed without dropping them.*
- 這個欄位混合了美元、歐元與英鎊，請逐列提取貨幣並只轉換數值部分，不要進行匯率換算。　／　*This column mixes dollars, euros, and pounds; extract the currency for each row and convert only the numeric portion without FX conversion.*

標籤 — 輸入：`dataset`、`text-formatted numeric columns`、`currency symbols`、`thousands separators`、`decimal markers`、`scale suffixes`、`percentages`、`accounting negatives`、`format decisions`；輸出：`numeric columns`、`raw value columns`、`currency column`、`qualifier column`、`data dictionary metadata`、`unparseable row report`、`numeric output dataset`；工具：`pandas`、`babel.numbers.parse_decimal`；依賴：`pandas`、`babel`、`add-data-dictionary skill`

限制：

- 需要安裝 pandas 才能處理資料。
- 如要進行地區設定感知的解析，可選擇使用 babel.numbers.parse_decimal。
- 套用轉換前需要使用者確認偵測到的格式，尤其是小數點、千位分隔符與百分比處理方式。
- 不會對同一欄中的混合貨幣進行匯率換算，只會逐列提取貨幣並轉換數值部分。
- 遇到範圍值時，需要使用者決定拆成最小值與最大值、取中點或保留文字。

> **抽查預判：預判通過** — `limitations` 已補齊（v2 重跑），且第一輪指出的「摘要句缺主詞」問題**一併消失**：本輪開頭已是「此技能會把……」。

| 審核狀態 | 審核人 | 審核日期 | 備註 |
| --- | --- | --- | --- |
| **通過**（原判需修改，已處理） | automated-review v1（gpt-5.6-terra judge；負責人 2026-08-16 授權） | 2026-08-16 | 初判需修改，已以 `enrich-skill/v3` 重跑生成＋reindex 後重審通過。**線上文字已更新，本節引文為 2026-08-15 版**；判定對線上文字生效（KPI6 餘弦 0.9302） |

### C-14. `excel-deduplicate`（data／精選）

- 來源：[https://github.com/YuYY2004/excel-skills](https://github.com/YuYY2004/excel-skills) @ `c15e51e20424`，`claude/skills/excel-deduplicate/SKILL.md`
- 生成：**第二輪重跑**｜模型 `gpt-5.6-sol`｜prompt `enrich-skill/v2`｜2026-08-15（原為 v1 重用，§2.5）
- ⚠️ **來源 SKILL.md 為簡體中文**；下列摘要與範例句即繁中化呈現層（見 §1.3）。

> 此技能依指定的關鍵欄位為 Excel 資料去重，可選擇保留首次或末次出現的資料，並刪除其餘重複列。它先以唯讀方式掃描重複列並確認刪除範圍，再備份檔案、直接修改 .xlsx 內的 XML，以保留未刪除列的格式。完成後會重新讀取檔案，檢查殘留重複資料及部分公式中的 `#REF!` 錯誤。使用時需提供 Excel 檔案、關鍵欄位名稱及保留模式。

任務範例句：

- 請依客戶 ID 欄位替這個 Excel 檔案去重，每個 ID 保留首次出現的資料。　／　*Deduplicate this Excel file by the customer ID column, keeping the first occurrence of each ID.*
- 請依電子郵件欄刪除重複列，但保留最後一次出現的資料。　／　*Remove duplicate rows based on the email column, but keep the last occurrence.*
- 請掃描這份活頁簿中的重複訂單編號，顯示將刪除的列數，再執行去重。　／　*Scan this workbook for duplicate order numbers, show me how many rows will be removed, and then deduplicate it.*
- 請清理產品代碼欄中的重複值，建立備份，並驗證處理後沒有殘留重複資料。　／　*Clean duplicate values from the product code column, create a backup, and verify that no duplicates remain.*
- 請依帳號欄為這份試算表去重，並保留所有未刪除列的原有格式。　／　*Deduplicate this spreadsheet by the account column and preserve the formatting of all retained rows.*

標籤 — 輸入：`xlsx workbook`、`key column name`、`keep mode`、`deletion confirmation`；輸出：`deduplicated xlsx workbook`、`backup workbook`、`duplicate scan report`、`validation report`；工具：`pandas`、`lxml`、`openpyxl`、`zipfile`、`xml row deletion`；依賴：`python`、`excel-find-duplicates`、`excel-delete`、`excel-safe-workflow`

限制：

- 僅處理以關鍵欄位判定的重複資料，關鍵欄位名稱必須是 pandas 讀取後的欄名。
- 需要安裝 Python、pandas、lxml 與 openpyxl；lxml 可透過 `pip install lxml` 安裝。
- 流程使用 .xlsx 的 XML 結構，並要求在操作前建立備份。
- 關鍵欄位中的多個空值只會保留第一筆。
- 公式健康檢查僅掃描活頁簿作用中工作表前 49 列、前 9 欄中的 `#REF!`。

> 🎯 **閘門卡 `DAT-4` 的 gold primary**（`gate-test/task-cards.md` §3）。本筆的摘要品質會被閘門直接檢驗。

> **抽查預判：預判通過** — `limitations` 已補齊（v2 重跑）。最後一條「僅掃描前 49 列、前 9 欄」是第一輪完全沒有的具體上限，實查 pin commit 原文的公式健康檢查迴圈確為 `range(1, min(50, ws.max_row + 1))` × `range(1, min(10, ws.max_column + 1))`，即第 1～49 列、第 1～9 欄，屬轉述而非幻覺；對 `DAT-4` 這張卡而言是有價值的「這個工具不會替你做什麼」資訊。

| 審核狀態 | 審核人 | 審核日期 | 備註 |
| --- | --- | --- | --- |
| **通過**（原判需修改，已處理） | automated-review v1（gpt-5.6-terra judge；負責人 2026-08-16 授權） | 2026-08-16 | 初判需修改，已以 `enrich-skill/v3` 重跑生成＋reindex 後重審通過。**線上文字已更新，本節引文為 2026-08-15 版**；判定對線上文字生效（KPI6 餘弦 0.8849） |

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
| **通過** | automated-review v1（gpt-5.6-terra judge；負責人 2026-08-16 授權） | 2026-08-16 | KPI1–6 全數通過（KPI6 餘弦 0.9479） |

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
| **通過** | automated-review v1（gpt-5.6-terra judge；負責人 2026-08-16 授權） | 2026-08-16 | KPI1–6 全數通過（KPI6 餘弦 0.9491） |

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
| **通過** | automated-review v1（gpt-5.6-terra judge；負責人 2026-08-16 授權） | 2026-08-16 | KPI1–6 全數通過（KPI6 餘弦 0.9646） |

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
| **通過**（原判需修改，已處理） | automated-review v1（gpt-5.6-terra judge；負責人 2026-08-16 授權） | 2026-08-16 | 初判需修改，已以 `enrich-skill/v3` 重跑生成＋reindex 後重審通過。**線上文字已更新，本節引文為 2026-08-15 版**；判定對線上文字生效（KPI6 餘弦 0.9209） |

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
| **通過** | automated-review v1（gpt-5.6-terra judge；負責人 2026-08-16 授權） | 2026-08-16 | KPI1–6 全數通過（KPI6 餘弦 0.9739） |

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
| **通過**（原判需修改，已處理） | automated-review v1（gpt-5.6-terra judge；負責人 2026-08-16 授權） | 2026-08-16 | 初判需修改（把能力外推到文件未載的相鄰動作：擷取講者備註、移除重複投影片）。**`enrich-skill/v4` 重跑後通過**：能力敘述收回到文件實際記載的動作。**線上文字已更新，本節引文為 2026-08-15 版**；判定對線上文字生效（KPI6 餘弦 0.9019） |

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
| **通過**（原判需修改，已處理） | automated-review v1（gpt-5.6-terra judge；負責人 2026-08-16 授權） | 2026-08-16 | 初判需修改，已以 `enrich-skill/v3` 重跑生成＋reindex 後重審通過。**線上文字已更新，本節引文為 2026-08-15 版**；判定對線上文字生效（KPI6 餘弦 0.912） |

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
| **通過** | automated-review v1（gpt-5.6-terra judge；負責人 2026-08-16 授權） | 2026-08-16 | KPI1–6 全數通過（KPI6 餘弦 0.8882），低於 0.90：線上文字與本節引文非逐字相同，判定對線上文字生效 |

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
| **通過** | automated-review v1（gpt-5.6-terra judge；負責人 2026-08-16 授權） | 2026-08-16 | KPI1–6 全數通過（KPI6 餘弦 0.8862），低於 0.90：線上文字與本節引文非逐字相同，判定對線上文字生效 |

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
| **通過** | automated-review v1（gpt-5.6-terra judge；負責人 2026-08-16 授權） | 2026-08-16 | KPI1–6 全數通過（KPI6 餘弦 0.8632），低於 0.90：線上文字與本節引文非逐字相同，判定對線上文字生效 |

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
| **通過** | automated-review v1（gpt-5.6-terra judge；負責人 2026-08-16 授權） | 2026-08-16 | KPI1–6 全數通過（KPI6 餘弦 0.8919），低於 0.90：線上文字與本節引文非逐字相同，判定對線上文字生效 |

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
| **通過**（原判需修改，已處理） | automated-review v1（gpt-5.6-terra judge；負責人 2026-08-16 授權） | 2026-08-16 | 初判需修改，已以 `enrich-skill/v3` 重跑生成＋reindex 後重審通過。**線上文字已更新，本節引文為 2026-08-15 版**；判定對線上文字生效（KPI6 餘弦 0.8273） |

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
| **通過** | automated-review v1（gpt-5.6-terra judge；負責人 2026-08-16 授權） | 2026-08-16 | KPI1–6 全數通過（KPI6 餘弦 0.964） |

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
| **通過** | automated-review v1（gpt-5.6-terra judge；負責人 2026-08-16 授權） | 2026-08-16 | KPI1–6 全數通過（KPI6 餘弦 0.9694） |

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
| **通過** | automated-review v1（gpt-5.6-terra judge；負責人 2026-08-16 授權） | 2026-08-16 | KPI1–6 全數通過（KPI6 餘弦 0.8667），低於 0.90：線上文字與本節引文非逐字相同，判定對線上文字生效 |

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
| **通過** | automated-review v1（gpt-5.6-terra judge；負責人 2026-08-16 授權） | 2026-08-16 | KPI1–6 全數通過（KPI6 餘弦 0.964） |

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
| **通過** | automated-review v1（gpt-5.6-terra judge；負責人 2026-08-16 授權） | 2026-08-16 | KPI1–6 全數通過（KPI6 餘弦 0.8958），低於 0.90：線上文字與本節引文非逐字相同，判定對線上文字生效 |

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
| **通過** | automated-review v1（gpt-5.6-terra judge；負責人 2026-08-16 授權） | 2026-08-16 | KPI1–6 全數通過（KPI6 餘弦 0.9227） |

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
| **通過** | automated-review v1（gpt-5.6-terra judge；負責人 2026-08-16 授權） | 2026-08-16 | KPI1–6 全數通過（KPI6 餘弦 0.9372） |

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
| **通過** | automated-review v1（gpt-5.6-terra judge；負責人 2026-08-16 授權） | 2026-08-16 | KPI1–6 全數通過（KPI6 餘弦 0.8517），低於 0.90：線上文字與本節引文非逐字相同，判定對線上文字生效 |

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
| **通過**（原判需修改，已處理） | automated-review v1（gpt-5.6-terra judge；負責人 2026-08-16 授權） | 2026-08-16 | 初判需修改，已以 `enrich-skill/v3` 重跑生成＋reindex 後重審通過。**線上文字已更新，本節引文為 2026-08-15 版**；判定對線上文字生效（KPI6 餘弦 0.931） |

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
| **通過** | automated-review v1（gpt-5.6-terra judge；負責人 2026-08-16 授權） | 2026-08-16 | KPI1–6 全數通過（KPI6 餘弦 0.9485） |

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
| **通過**（原判需修改，已處理） | automated-review v1（gpt-5.6-terra judge；負責人 2026-08-16 授權） | 2026-08-16 | 初判需修改，已以 `enrich-skill/v3` 重跑生成＋reindex 後重審通過。**線上文字已更新，本節引文為 2026-08-15 版**；判定對線上文字生效（KPI6 餘弦 0.9254） |

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
| **通過** | automated-review v1（gpt-5.6-terra judge；負責人 2026-08-16 授權） | 2026-08-16 | KPI1–6 全數通過（KPI6 餘弦 0.9693） |

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
| **通過** | automated-review v1（gpt-5.6-terra judge；負責人 2026-08-16 授權） | 2026-08-16 | KPI1–6 全數通過（KPI6 餘弦 0.9418） |

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
| **通過** | automated-review v1（gpt-5.6-terra judge；負責人 2026-08-16 授權） | 2026-08-16 | KPI1–6 全數通過（KPI6 餘弦 0.9692） |

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
| **通過** | automated-review v1（gpt-5.6-terra judge；負責人 2026-08-16 授權） | 2026-08-16 | KPI1–6 全數通過（KPI6 餘弦 0.9752） |

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
| **通過** | automated-review v1（gpt-5.6-terra judge；負責人 2026-08-16 授權） | 2026-08-16 | KPI1–6 全數通過（KPI6 餘弦 0.9569） |

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
| **通過** | automated-review v1（gpt-5.6-terra judge；負責人 2026-08-16 授權） | 2026-08-16 | KPI1–6 全數通過（KPI6 餘弦 0.9226） |

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
| **通過** | automated-review v1（gpt-5.6-terra judge；負責人 2026-08-16 授權） | 2026-08-16 | KPI1–6 全數通過（KPI6 餘弦 0.9028） |

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
| **通過** | automated-review v1（gpt-5.6-terra judge；負責人 2026-08-16 授權） | 2026-08-16 | KPI1–6 全數通過（KPI6 餘弦 0.9325） |

---

## 7. 未完成事項

| # | 事項 | 狀態 | 阻擋誰 | 負責 |
| --- | --- | --- | --- | --- |
| 1 | ~~**45 筆的人工審核**（精選 15 必審、已索引 30 抽審 ≥1/3）~~ **改為自動化審校（45 筆全量）** | ✅ **已完成**（2026-08-16，42 通過／3 建議下架），見 [content-review-report.md](../m1/content-review-report.md) | — | — |
| 2 | 7 筆精選以 `enrich-skill/v2` 重跑，補齊 `limitations` | ✅ **已完成**（§2.5，7/7） | — | — |
| 3 | **`excel-format` 的字型在地化判定** | ✅ **已解決**——`enrich-skill/v3` 加入「簡中專有名詞附繁中對應」規則，線上現為 `微软雅黑（繁中：微軟雅黑）`：**加註而非替換**，事實不動（`微軟正黑體` 是另一套字型，替換等於改事實）。該筆的忠實性問題另由 v4 解決，最終通過 | — | — |
| 4 | **`anthropics/skills` 免責條款的承接層確認**（§3 註） | ⏳ 待確認 | 6 筆的審核判定 | 負責人 |
| 5 | 重新匯入 45 筆並 reindex | ✅ **已完成**（45/45 匯入、45/45 `enriched`），見 [`catalog-rebuild-report.md`](../m1/catalog-rebuild-report.md)。**2026-08-16 後線上為 v2 × 35 ＋ v3 × 7 ＋ v4 × 3**（審校重跑，未重新匯入） | — | — |
| 6 | `sokrati/sokrati` 匯入失敗（`description-too-long`） | ✅ **已解決**——`skillpkg` 改以 `utf8.RuneCountInString` 計長後通過，**線上目錄為 45 筆，不再是 44** | — | — |
| 7 | **審核對象改為線上文字**：`02` §4.7 CONTENT-005 的非決定性上限要求審核判定只對已入庫的該版文字生效 | ✅ **已落實**——本次審校 6 項 KPI 全部讀 `GET /api/skills/{id}`；KPI6 另量測線上與本文件引文的餘弦（45 筆中 12 筆 <0.90，量化了這條上限） | — | — |
| 8 | ~~**3 筆建議下架待修**~~ **改以 `enrich-skill/v4` 修正** | ✅ **已完成**——負責人採納審校報告 §7 建議，v4 加入三條轉述約束後重跑，**3 筆全數通過、未下架**，`documents` 精選維持 4 筆 | — | — |
| 9 | **KPI3 判準範圍修正的追認**（只掃 zh-Hant 欄位；英文範例句的簡中原名引用改記為觀察值） | ⏳ **待負責人追認**——影響 `excel-format` 一筆的判定，見 [審校報告 §9 第 2 條](../m1/content-review-report.md) | 該筆判定、⑦、CONTENT-005 勾選 | 負責人 |

> **§7 第 6 項對閘門文件的連帶影響。** [`gate-test/README.md`](../gate-test/README.md) §3.2 與 §4.2 多處以「44 筆索引」描述凍結標的，[`import-report.md`](../m1/import-report.md) §3 同。**實際線上為 45 筆**。本文件不代改閘門套件；請負責人在凍結生效（D 日）時一併更新該處數字，或於分析報告註明。
>
> ~~**02 沒有 CONTENT-005 的需求 ID。**~~ **已於 commit `b2a7690` 補上**（`02` §4.7，CONTENT-001～009 與 SEC 系列）。本文件 §3 已改為對照該節。
