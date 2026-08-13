# PDM-001／002:`data` 類別供給查核報告

- 查核日期:**2026-08-14**(表中所有 URL 與 License 狀態均以此日觀測為準)
- 查核對象:[pdm-proposals.md](./pdm-proposals.md) §1(PDM-001)風險表最後一列、§2(PDM-002)「`data` 類別的候選來源」三個待查核方向
- 查核方法:實地存取 GitHub REST API、`raw.githubusercontent.com` 原始檔與各 repo 頁面;逐一比對 PDM-002「九項精選檢查表」與 PDM-004 的 Runtime 套件白名單
- 本文件**只查核與建議,不定案**;**未修改**任何 ADR、`02`、`03`、`pdm-proposals.md`,也未變動任何工作項目勾選狀態

> **結論(一句話):採選項 (a)——`data` 類別供給充足。** 查得 **25 個候選 Skill、橫跨 7 個來源 repo**,超過 PDM-002「每類別 8–12 個索引項目、4–6 個精選」的目標;但 **PDM-002 原本提名的三個候選方向有兩個已證偽**,可用供給全部來自「補充搜尋」路徑,且附帶三項必須併同處理的條件(§5)。

---

## 1. 查核項目一:`anthropics/skills` 實數清點

**結論:實數為 17 個 skill 目錄,PDM-002 §2 選項 A 註記的「v1 記為 17,未經核對」數字正確,此待辦可結案。**

repo 根目錄僅有 `.claude-plugin/`、`skills/`、`spec/`、`template/` 四個目錄與 `README.md`、`THIRD_PARTY_NOTICES.md`、`.gitignore`。所有 skill 都在 `skills/` 之下。

| # | Skill 目錄 | License(實地核對 `LICENSE.txt` 首行) | 類別歸屬 | 資料處理性質 |
| --- | --- | --- | --- | --- |
| 1 | `algorithmic-art` | Apache-2.0 | 創意 | — |
| 2 | `brand-guidelines` | Apache-2.0 | `writing` | — |
| 3 | `canvas-design` | Apache-2.0 | 創意 | — |
| 4 | `claude-api` | Apache-2.0 | 開發 | — |
| 5 | `doc-coauthoring` | **無 `LICENSE.txt`**(見下方發現 A) | `writing` | — |
| 6 | `docx` | **Source-available**(Anthropic 服務條款) | `documents` | — |
| 7 | `frontend-design` | Apache-2.0 | 創意 | — |
| 8 | `internal-comms` | Apache-2.0 | `writing` | — |
| 9 | `mcp-builder` | Apache-2.0 | 開發 | — |
| 10 | `pdf` | **Source-available** | `documents` | 部分(表格／文字抽取) |
| 11 | `pptx` | **Source-available** | `documents` | — |
| 12 | `skill-creator` | Apache-2.0 | 開發 | — |
| 13 | `slack-gif-creator` | Apache-2.0 | 創意 | — |
| 14 | `theme-factory` | Apache-2.0 | 創意 | — |
| 15 | `web-artifacts-builder` | Apache-2.0 | 創意 | — |
| 16 | `webapp-testing` | Apache-2.0 | 開發 | — |
| 17 | `xlsx` | **Source-available** | `documents` | **是**——唯一具備 `data` 性質者 |

合計:**Apache-2.0 12 個、Source-available 4 個、無 License 檔 1 個**。

**與 PDM-002 的差異(需回寫):**

### 發現 A:`anthropics/skills` **repo 根目錄沒有 LICENSE 檔**,`doc-coauthoring` 也沒有

- GitHub API 的 repo metadata `license` 欄位回傳 `null`;授權完全由**各 skill 目錄自帶的 `LICENSE.txt`** 決定。
- `skills/doc-coauthoring/` 只有一個 `SKILL.md`,**沒有 `LICENSE.txt`**,也沒有根目錄 License 可繼承。
- **影響 `writing` 類別,不是 `data`**:PDM-001 §「理由」把 `doc-coauthoring` 列為 `writing` 的既有供給,但依 PDM-002 精選標準第 1 項(License 明確且允許再散布)與風險表「License 狀態預設『未知』」,**`doc-coauthoring` 目前不可標為精選、不可打包**。`writing` 的官方供給實為 `brand-guidelines` ＋ `internal-comms` 兩個(皆 Apache-2.0)。
- 建議動作:定案時將此列入 `writing` 的供給重算,或向 Anthropic 詢問 `doc-coauthoring` 的授權歸屬。

### 發現 B:`README.md` 的免責條款適用全 repo

README 載明「These skills are provided for demonstration and educational purposes only.」——此為使用免責,不改變上表的 License 分類,但應納入 CONTENT-005 白話摘要的措辭考量(呼應 NFR-001「靜態掃描通過不等於安全保證」的既有措辭紀律)。

### 對 PDM-002 選項 A(官方唯一)的重新評估

PDM-002 §2 註記「若清點後數量足夠,此選項需重新評估」。**清點結果:不足,選項 A 維持否決。** 17 個中真正屬於 `data` 者只有 `xlsx` 一個,且為 source-available(依 PDM-002 風險表統一政策,**不產出任何 Download Artifact**)。以 1 個項目無法支撐 8–12 個索引目標,recall@5 亦無鑑別力。**選項 B(白名單 + 目錄僅作探索)的原始理由成立。**

---

## 2. 查核項目二:VoltAgent 清單回溯

**結論:PDM-002 提名的候選方向 2「VoltAgent 清單中『Data & Analytics』分類」——該分類不存在,方向作廢。**

### 發現 C:VoltAgent 清單依**廠商**分節,不依功能領域分類

實地取得 `VoltAgent/awesome-agent-skills` 的 `README.md`(1,977 行、243KB,MIT,badge 自述 1,000+ 條目)。其目錄結構為:

- 主結構是 **「Official Skills by <廠商>」**,共 64 個廠商／個人分節(Claude、VoltAgent、TestMu AI、Supabase、Stripe、ClickHouse、DuckDB、Neon、MongoDB、Redis、Microsoft、OpenAI、NVIDIA、Google Cloud…)。
- 唯一的非廠商分節是 **「Community Skills」**,其下七個子分類為:Vector Databases、Marketing、Productivity and Collaboration、Development and Testing、Context Engineering、Specialized Domains、n8n Automation。
- **全文無「Data & Analytics」或近義的功能分類。**

### 發現 D:VoltAgent 已改為連向 `officialskills.sh`,而非原始 repo

README 中 **582 個連結指向 `officialskills.sh`(市集／目錄站)**,631 個指向 `github.com`。廠商分節幾乎全部走 `officialskills.sh`。

這對 PDM-002 §「回溯准入流程」第 2 步(**回溯到原始 repo,不從清單頁面或市集抓取內容**)有直接影響:**過半條目無法從清單本身取得原始 repo URL**,回溯需額外一次人工查找。流程本身仍成立,但成本被低估了。

### 資料性質廠商條目的逐一回溯結果

依 PDM-002 候選方向 2 的原意(「資料庫／BI／CSV 處理類廠商發布的 Skill」),把所有資料性質廠商分節逐一回溯:

| 廠商／條目 | 原始 repo | License | 是否需外網／DB client | 九項檢查判定 |
| --- | --- | --- | --- | --- |
| DuckDB(`attach-db`、`query`、`read-file`、`convert-file`、`s3-explore`、`install-duckdb`、`duckdb-docs`、`read-memories`,9 個 SKILL.md) | [duckdb/duckdb-skills](https://github.com/duckdb/duckdb-skills) | MIT ✅ | **需 DuckDB CLI**;`install-duckdb` 明示執行期安裝、`duckdb-docs` 走 HTTPS 全文檢索、`s3-explore` 需遠端物件儲存 | **第 6 項不過**——PDM-004 白名單無 duckdb,且執行期禁止安裝 |
| ClickHouse(6 個) | ClickHouse 官方 | 未進一步查(第 6 項先擋) | **需 chdb／clickhousectl ＋ ClickHouse Cloud** | **第 6 項不過** |
| Tinybird(4 個) | Tinybird 官方 | 同上 | **需 Tinybird CLI ＋ 雲端 API** | **第 6 項不過** |
| Neon(3 個) | Neon 官方 | 同上 | **需 Postgres client ＋ Neon 雲端** | **第 6 項不過** |
| MongoDB、Redis、Supabase(`postgres-best-practices`) | 各官方 | 同上 | **需各自 DB client** | **第 6 項不過** |
| Google Workspace `gws-sheets` | Google 官方 | 同上 | **需 Google Sheets API ＋ OAuth** | **第 6 項不過** |
| OpenAI `spreadsheet`(建立／編輯／分析／視覺化試算表) | [openai/skills](https://github.com/openai/skills)(24,904★) | **無 LICENSE 檔**(repo metadata `license: null`,根目錄僅 `.gitignore`／`README.md`／`contributing.md`／`skills/`) | 待查(第 1 項先擋) | **第 1 項不過**——License 未知,依 PDM-002 風險表一律不可打包 |
| MiniMax `minimax-xlsx` | [MiniMax-AI/skills](https://github.com/MiniMax-AI/skills) | MIT ✅ | 待逐項查 | 未完成查核(見 §4 備援池) |
| Kaelio `ktx`(17 個,含 `analytics`、`live_database_ingest`、`looker_ingest`) | [Kaelio/ktx](https://github.com/Kaelio/ktx)(1,540★) | Apache-2.0 ✅ | **live database ingest／Looker／GDrive,全需外連** | **第 6 項不過** |
| Altimate `data-engineering-skills`(11 個,dbt 為主) | [AltimateAI/data-engineering-skills](https://github.com/AltimateAI/data-engineering-skills)(118★) | MIT ✅ | **需 dbt ＋ warehouse 連線** | **第 6 項不過** |

**PDM-002 對這個方向的假設「官方團隊發布者的 License 與作者可辨識性最容易確認,優先評估」在事實上成立(License 確實乾淨),但選錯了篩選維度。** 真正的淘汰條件不是 License 而是**依賴**:資料類廠商的商業模式就是託管服務,其官方 Skill 必然要連回自家服務,**與 ADR-005 egress default-deny ＋ PDM-004「明確不含資料庫 client」正面衝突,無一例外**。PDM-002 自己在該列的「待查核項目」已預示了這點(「需外連者直接淘汰」),查核結果是**全數淘汰**。

清單回溯**在窮盡所有資料性質廠商分節後停止,合格候選數 0**。Community Skills 的七個子分類中亦無資料處理分類,其 Specialized Domains 下唯一相關者 `takechanman1228/claude-ecom` 另見 §4 淘汰表。

---

## 3. 查核項目三:補充搜尋(GitHub 直接檢索)

PDM-002 候選方向 3「純腳本型社群資料處理 Skill(pandas／CSV 清理／異常值報告類)」是**三個方向中唯一成立者**。以 GitHub repository search(`topic:claude-skills`、`topic:agent-skills` ＋ data／CSV／excel／cleaning／analysis 關鍵詞)補足候選池,逐一查 License 與依賴。

PDM-002 對此方向的風險預判——「作者可辨識性與 License 明確性是最大風險;社群 repo 常缺 LICENSE 檔」——**完全命中**。最受歡迎的三個純 CSV 處理 Skill 全部因缺 License 出局(見 §4)。

---

## 4. 候選白名單(建議)

判定基準為 PDM-002 九項精選檢查表。**第 8 項(平台基準試跑通過)對所有候選一律標「未執行」**——平台尚未存在,此項對 `documents`／`writing` 同樣未執行,不構成 `data` 的特有差距。**第 4 項的「人工逐行審過」亦未執行**,下表只機械量測行數是否落在 300 行上限內。

### 4.1 建議白名單(合格候選,共 25 個 / 7 個來源 repo)

依賴欄的判定基準為 PDM-004 §「預裝 Python 套件(白名單)」:`openpyxl`、`python-docx`、`python-pptx`、`pypdf`、`pdfplumber`、`pillow`、`pandas`、`numpy`、`chardet`、`python-dateutil`、`markdown-it-py`、`pyyaml`。

| # | Skill | 來源 repo | License | 依賴 | Script 規模 | 風險註記 |
| --- | --- | --- | --- | --- | --- | --- |
| 1–12 | `excel-find-duplicates`、`excel-deduplicate`、`excel-filter`、`excel-validate`、`excel-merge`、`excel-split`、`excel-sort`、`excel-regex-clean`、`excel-scout`、`excel-delete`、`excel-mapping-replace`、`excel-date-to-text` | [YuYY2004/excel-skills](https://github.com/YuYY2004/excel-skills)(2★) | **MIT** ✅ | `pandas`、`openpyxl` ✅／**`lxml` ✗ 不在白名單** | 12 個為 prompt-only;6 個共用 `scripts/` 的腳本,單檔 63–253 行(合計 883 行,但**單一 Skill 引用者均 ≤ 253 行,落在 300 行上限內**) | ⚠️ **需 PDM-004 白名單加 `lxml`**;⚠️ 2★、單一作者,上游消失風險高(INGEST-010);⚠️ 全數為 xlsx 操作,**與 `documents` 類別邊界重疊**(見 §6 風險 3) |
| 13–24 | `data-cleanliness-scan`、`standardise-country-names`、`text-to-numeric`、`unicode-consistency`、`date-wrangling`、`csv-to-json`、`json-restructure`、`data-shape`、`data-comparability`、`add-data-dictionary`、`pii-flag`、`add-iso3166` | [danielrosehill/Claude-Data-Wrangler-plugin](https://github.com/danielrosehill/Claude-Data-Wrangler-plugin)(3★) | **MIT** ✅ | prompt-only,僅需 `pandas`／`numpy` ✅ | **0 行**(全 repo 無 `.py`) | ✅ 作者 Daniel Rosehill 可辨識;⚠️ 同 repo 另有 7 個需外網者(`hf-dataset-push`、`vector-upsert`、`sql-load`、`api-loader`、`graph-database`、`database-guide`、`enrich-with-currency`)**必須逐一排除,不可整包匯入** |
| 25 | `data-analyst`(CSV／TSV/XLSX／JSON／JSONL 處理、清洗、統計) | [nqumich/data-analyst-skill](https://github.com/nqumich/data-analyst-skill)(1★) | **MIT** ✅ | `pandas`、`openpyxl` ✅ **全部在白名單內** | **203 行**,✅ 落在 300 行上限內 | ✅ 依賴零缺口,是最乾淨的單一候選;⚠️ 1★、`SKILL.md` 為簡體中文(CONTENT-005 白話摘要需重寫) |

### 4.2 條件候選(需負責人裁量,不計入上表 25)

| Skill | 來源 repo | License | 依賴 | 阻礙項 |
| --- | --- | --- | --- | --- |
| `data-analysis-skills`(CSV/Excel/JSON/TSV → 敘事型分析報告) | [cabbage2000-lab/data-analysis-skills](https://github.com/cabbage2000-lab/data-analysis-skills)(5★) | MIT ✅ | prompt-only | ⚠️ **`SKILL.md` 明文指示「if a dependency is missing, try pip install first」**——直接牴觸精選標準第 6 項與 PDM-004「執行期禁止安裝」。實際依賴(pandas)在白名單內,該分支不會觸發,但**Skill 文字本身教模型做被禁止的事**,需人工確認是否可接受 |
| `cohort-analysis`、`ab-test-analysis` | [phuryn/pm-skills](https://github.com/phuryn/pm-skills)(25,234★) | MIT ✅ | prompt-only | ⚠️ 兩者的分析步驟均要求產生圖表／統計檢定,**`matplotlib`／`scipy` 不在 PDM-004 白名單**。同 repo 的 `sql-queries` 需 warehouse,應排除。**優點:唯一一個高星等、長期維護的來源** |
| `business-analysis-skill`(Excel/CSV → HTML 經營報告) | [AbdoBasyioni/business-analysis-skill](https://github.com/AbdoBasyioni/business-analysis-skill)(15★) | MIT ✅ | `pandas`、`openpyxl` ✅ | ⚠️ **Python 約 500 行,超過精選標準第 4 項的 300 行上限**;⚠️ 報告輸出為阿拉伯文 RTL |
| `xlsx` | [anthropics/skills](https://github.com/anthropics/skills) | **Source-available** | 已知 | ⚠️ 依 PDM-002 風險表統一政策**不產出任何 Download Artifact**;⚠️ 歸屬 `documents` 或 `data` 需裁量 |

### 4.3 已否決候選(記錄否決原因,依 PDM-002 回溯流程第 4 步「不重複評估」)

| 候選 | 星等 | 否決原因 | 不過的檢查項 |
| --- | --- | --- | --- |
| [coffeefuelbump/csv-data-summarizer-claude-skill](https://github.com/coffeefuelbump/csv-data-summarizer-claude-skill) | 446★ | **repo 完全無 LICENSE 檔**;另用 `matplotlib`／`seaborn`／`requests` | 1、6 |
| [openai/skills](https://github.com/openai/skills)(`spreadsheet` 等) | 24,904★ | **repo 完全無 LICENSE 檔** | 1 |
| [lytssaa/data-cleaning-skill](https://github.com/lytssaa/data-cleaning-skill) | 2★ | 無 License | 1 |
| [yogaibhh/claude-clean-data-skill](https://github.com/yogaibhh/claude-clean-data-skill) | 1★ | 無 License | 1 |
| [takechanman1228/claude-ecom](https://github.com/takechanman1228/claude-ecom) | 47★ | MIT ✅,但 **Python 3,967 行**,遠超 300 行上限;另用 `click`／`pytest`／`tomli` | 4、6 |
| [pablodiegoo/Data-Pro-Skill](https://github.com/pablodiegoo/Data-Pro-Skill) | 7★ | **2,860 行** ＋ `scipy`／`sklearn` | 4、6 |
| [lancegui/causal-powers](https://github.com/lancegui/causal-powers) | 2★ | **6,197 行** ＋ `scipy`／`sklearn`／`requests` | 4、6 |
| [soupandpsy/amazing-psycoder-skills](https://github.com/soupandpsy/amazing-psycoder-skills) | 31★ | Script 合計 **3.3 MB** | 4 |
| [MannLabs/proteomics-agent-skills](https://github.com/MannLabs/proteomics-agent-skills) | 14★ | Apache-2.0 ✅,但需質譜蛋白體專用套件;且與「個人創作者」persona 不符 | 6(＋PDM-001 限制 1) |
| [Xcaffrey13/lead-listing-agent](https://github.com/Xcaffrey13/lead-listing-agent) | 5★ | enrichment 需 `requests`／`anthropic` 外連 | 6 |
| [duckdb/duckdb-skills](https://github.com/duckdb/duckdb-skills) | 527★ | MIT ✅,但需 DuckDB CLI ＋ 執行期安裝 ＋ HTTPS 檢索 | 6 |
| [AltimateAI/data-engineering-skills](https://github.com/AltimateAI/data-engineering-skills) | 118★ | MIT ✅,但需 dbt ＋ warehouse | 6 |
| [Kaelio/ktx](https://github.com/Kaelio/ktx) | 1,540★ | Apache-2.0 ✅,但需 live DB／Looker／GDrive | 6 |
| ClickHouse／Tinybird／Neon／MongoDB／Redis／Supabase／Google Workspace(VoltAgent 廠商分節) | — | 全數需 DB client 或雲端 API | 6 |

**否決統計:14 組候選中,以「無 License」出局 4 組、以「依賴超出 Runtime」出局 8 組、以「Script 過大」出局 4 組(有重疊)。** 最受歡迎的兩個純資料處理 Skill(446★ 與 24,904★)全部倒在第 1 項——這正是 PDM-002 風險表「社群 repo 的 License 檔案缺失」預測的情形,且發生率高於預期。

---

## 5. 結論與建議

### 選項 (a):`data` 供給充足,建議維持 PDM-001 選項 A 的三類別不變

**數量核對:**

| PDM-002 目標 | 目標值 | `data` 實查供給 | 判定 |
| --- | --- | --- | --- |
| 已索引(indexed),每類別 | 8–12 | **25**(§4.1) | ✅ 超額 |
| 精選(curated),每類別 | 4–6 | 25 個中可挑 4–6(建議自 §4.1 #13–25 起,依賴零缺口) | ✅ 可達成 |
| 來源多樣性(recall@5 鑑別力所需) | 未明定 | **7 個獨立 repo** | ✅ 高於 `documents`／`writing`(各 1 個 repo) |

因此 **PDM-001 §「風險」表最後一列的觸發條件(「若定案前候選 repo 未通過九項精選檢查,應改選類別而非降低數量目標」)未被觸發**,不需啟動換類別。

### 但有三項必須併同定案的條件

| # | 條件 | 原因 | 影響的文件 |
| --- | --- | --- | --- |
| 1 | **PDM-004 預裝 Python 套件白名單須加 `lxml`** | §4.1 #1–12 這一組(12 個候選,占供給近半)全部依賴 `lxml` 作 xlsx XML 層操作。不加則 `data` 供給從 25 掉到 13 | `pdm-proposals.md` §4 白名單表 |
| 2 | **決定是否加入 `matplotlib`(建議加)、`scipy`(建議不加)** | PDM-001 給 `data` 的定位是「資料**整理與分析**」。目前白名單能覆蓋「整理」,但**完全無法產出圖表**,「分析」只剩文字。§4.2 的 `cohort-analysis`／`ab-test-analysis` 兩個高星等候選正是卡在此。`scipy`／`sklearn` 則建議維持排除——引入後 §4.3 的多個大型候選會重新進場,但其 Script 規模仍不過第 4 項,收益有限 | `pdm-proposals.md` §4 白名單表;連帶影響 `02:EVAL-001` 對 `data` 的驗收設計 |
| 3 | **PDM-002 §「`data` 類別的候選來源」三行候選方向表需重寫** | 方向 1(`anthropics/skills/xlsx`)查核結果=僅 1 個且不可下載;方向 2(VoltAgent Data & Analytics)**該分類不存在,且所有資料廠商條目全數因依賴出局**;方向 3(純腳本社群)是唯一成立者。現行表格會誤導後續執行者 | `pdm-proposals.md` §2(**本次未修改,依指示留待收尾處理**) |

### 建議的精選(curated)首批 4–6 個

以「依賴零缺口 ＋ 作者可辨識 ＋ 驗收確定性最高」排序,建議:

1. `data-analyst`([nqumich/data-analyst-skill](https://github.com/nqumich/data-analyst-skill))——依賴全在白名單、203 行、涵蓋 CSV/TSV/XLSX/JSON
2. `data-cleanliness-scan`(danielrosehill)——prompt-only,零依賴風險
3. `csv-to-json`(danielrosehill)——輸出可做 schema 斷言,確定性最高
4. `text-to-numeric`(danielrosehill)——同上
5. `excel-deduplicate`(YuYY2004)——**條件 1 通過後才可入選**,直接對應 PDM-001 的典型任務句「清掉重複列」
6. `excel-find-duplicates`(YuYY2004)——同上

前四個**不需要動 PDM-004 白名單即可成立**,構成 `data` 類別的無條件下限。

---

## 6. 殘餘風險

| # | 風險 | 影響 | 建議緩解 |
| --- | --- | --- | --- |
| 1 | **來源品質梯度顯著低於另兩類**:`data` 的 25 個候選中 24 個來自 1–5★ 的個人社群 repo;`documents`／`writing` 則靠 168,872★ 的 `anthropics/skills` | 上游刪除或改寫的機率高得多(INGEST-010／CONTENT-009 的實際觸發率會集中在此類別) | 匯入時保存內容雜湊與 commit SHA(INGEST-004 既有要求);M1 驗證閘門前對 `data` 的白名單做一次重新確認 |
| 2 | **人工審查成本集中在 `data`**:精選標準第 4 項(逐行審 Script)與第 5 項(無疑似 Secret)對社群 repo 的成本遠高於官方 repo | CONTENT-003／008 的工時估算若按類別均攤會低估 | 排程時把 `data` 的精選工時單獨估算;優先選 prompt-only 候選(§4.1 #13–25 全部 0 行 Script) |
| 3 | **`data` 與 `documents` 的類別邊界重疊**:§4.1 #1–12 全為 xlsx 操作。若審查者判定 `excel-*` 應歸 `documents`,`data` 供給從 25 降為 13(來源 repo 從 7 降為 6) | 13 仍 > 8–12 目標,**不會觸發換類別**,但精選池變薄 | 定案時明確寫下歸類原則。建議依「產出 vs 整理」切分:`excel-deduplicate`／`filter`／`validate`／`merge`/`split` 屬**整理**→ `data`;`xlsx` 的建立與格式化屬**產出**→ `documents` |
| 4 | **PDM-011 完整 golden query set 的多樣性**:§4.1 #1–12 的 12 個 `excel-*` 描述高度同質,#13–24 的 12 個 danielrosehill 描述亦同質 | 若 golden query 全部命中同一叢集,recall@5 仍可能失去鑑別力——這正是 PDM-002 設定 8–12 下限想避免的問題,只是成因從「數量不足」變成「語意過度集中」 | 出 golden query 時**強制跨 repo 抽樣**,每個來源 repo 至多貢獻 20 % 的題目;必要時把 §4.2 的條件候選納入索引層(indexed 不需九項全過)以增加語意分散度 |
| 5 | `doc-coauthoring` 無 License(發現 A) | 影響的是 **`writing`** 類別的既有供給認定,不影響 `data` | 見 §1 發現 A |

---

## 7. 查核未涵蓋

- **精選標準第 8 項(平台基準試跑)**:平台不存在,全類別皆未執行。
- **精選標準第 4 項的人工逐行審查**:本次只機械量測行數與 import,未逐行閱讀任何 Script。
- **精選標準第 5 項(無疑似 Secret)靜態掃描**:未執行。
- `MiniMax-AI/skills` 的 `minimax-xlsx`:License 已確認 MIT,依賴與 Script 規模未完成查核(GitHub API 未認證額度用盡)。若條件 1／2 未獲採納導致供給吃緊,這是第一個應補查的備援。
- **VoltAgent Community Skills 的完整逐條回溯**:僅回溯了七個子分類中資料相關者;因該清單本就無資料分類,且已在補充搜尋取得超額供給,依任務規則「查到 12 個合格候選即停」停止。

---

## 8. 對外部檔案的影響(僅記錄,本次未執行)

| 檔案 | 需要的動作 | 觸發條件 |
| --- | --- | --- |
| `plans/mvp/m0/pdm-proposals.md` §1 風險表 | 「`data` 類別在白名單中無可直接匯入的來源」一列可標為**已解除**,改引用本文件 | 負責人採納 §5 選項 (a) |
| `plans/mvp/m0/pdm-proposals.md` §1 理由段 | 「`data` 類別在白名單中目前沒有可直接匯入的來源」需改寫;`doc-coauthoring` 的 `writing` 供給認定需修正(發現 A) | 同上 |
| `plans/mvp/m0/pdm-proposals.md` §2 候選來源表 | 三行候選方向全部重寫為本文件 §4.1 的白名單(見 §5 條件 3) | 同上 |
| `plans/mvp/m0/pdm-proposals.md` §2 選項 A 註記 | 「skill 目錄實數待清點(v1 記為 17,未經核對)」可結案:**實數確為 17**,選項 A 維持否決 | 同上 |
| `plans/mvp/m0/pdm-proposals.md` §4 白名單表 | 加 `lxml`(必要)、`matplotlib`(建議) | §5 條件 1／2 定案 |
| `plans/mvp/m0/README.md` | 「PDM-001/002 阻塞於 `data` 類別零白名單供給(唯一高嚴重度未解項)」需更新 | 負責人採納後 |
| `plans/mvp/03-work-items.md` | CONTENT-003／008 的勾選;**本次未動,依 AGENTS.md 須完全符合允收準則才勾** | 精選標準第 8 項可執行後 |
