# Skill Hub MVP：工作項目列表

## 使用方式

- `[ ]`：尚未完成。
- `[x]`：已完成並符合對應允收準則。
- 每個工作項目應能對應到需求 ID、設計決策或可驗證成果。
- 部分完成的項目保持 `[ ]`，並在子項目或工作系統中追蹤進度。
- 章節標題後的括號為主要對應里程碑；標記「後 MVP」的章節或項目不在 MVP 首發範圍。

## 0. 已確認的產品基線

- [x] 確認主要使用者為個人創作者。
- [x] 確認使用通用 Agent Skills 規格作為核心格式。
- [x] 確認 MVP 以探索既有 Skill 為第一入口。
- [x] 確認「試跑、評估、改善、打包下載」為核心價值閉環。
- [x] 確認初期採自建 Cloud Sandbox，並保留可抽換 Sandbox Provider 架構。
- [x] 確認本機絕對路徑透過 Local Runner 處理，不由 Cloud Sandbox 直接存取。
- [x] 建立 MVP 目標、規格允收及工作清單文件。

## 1. 待完成的產品決策（M0）

> **2026-08-16 刷新**：本節是 AGENTS.md 指向的「目前所有未決議題」入口之一，M2 期間有多項被後續 ADR 或實作回答，但**沒有一項取得負責人的逐項定案紀錄**（[m0/README.md](m0/README.md) 仍記「PDM-004~010 未逐項定案」，[m0/pdm-proposals.md](m0/pdm-proposals.md) §定案檢查清單各列仍為 `[ ]`）。因此下方**一個勾選都不改**，改為逐項標註「實質狀態」與「還缺什麼才能勾」——把「值已經定了」與「決策已經被追認」分開記，是這份清單唯一有用的地方。
>
> M2 殘項清單的「乙、待負責人決策」另有一份互補的視角（那份記的是阻擋實作的決策，本節記的是產品參數的決策），見 [04-backlog-and-handoffs.md](04-backlog-and-handoffs.md)。

- [x] PDM-001 選定 MVP 首批三個 Skill 類別。
- [x] PDM-002 確認首批 Skill 來源與精選標準。
- [x] PDM-003 選定主要 Agent Runtime 與模型。
- [ ] PDM-004 決定 SelfHostedProvider 首批支援的 Runtime 語言與版本。（**實質已定，缺追認**：Runtime Image `2026.08-2` 為 Node 22 ＋ Agent SDK **0.3.233**（[ADR-023](../../adr/ADR-023-agent-sdk-version-pinning-and-behaviour-revalidation.md) 釘 digest ＋ lockfile）＋ `python3.11` 與 45 個目錄 Skill 宣告的 9 個 Python 依賴（[m2/README.md 乙-6](m2/README.md) 裁定「加」）。**還缺**：①上述組合寫成 PDM-004 的定案紀錄（現在只散在 Dockerfile、`infra/images/README.md` 與 M2 殘項清單裡）；②`runsc` 上跑通完整 Run 生命週期的實測——那是 m0 提案自訂的定案條件之一，屬部署期（`SEC-009` 測項 T4））
- [ ] PDM-005 決定 Dataset 大小、檔案類型與單次 Run 資源上限。（**值已全數強制，缺追認**：`02:TEST-002` 與 `02:RUN-003` 已把 §5.1／§5.2／§5.2a 的值回寫為可判定準則，六項資源上限、Artifact 上限、Workspace 並行 2、**Token 300K／60K**（`03` SBX-013）皆已有強制點。**還缺**：m0 提案的定案檢查清單未打勾，`02` 以「值其實已於 PDM-005 定案」的敘述引用它——**兩份文件對「是否已定案」的說法不一致，需負責人裁定其一**）
- [ ] PDM-006 決定 Run、Dataset、Trace 與 Artifact 的保存期限。（**部分**：Dataset 90 天已定值並強制（`TEST-004` 的 `expires_at`）、Trace 分割表已按月切、SEC-009 證據保存 ≥ 1 年已定（ADR-022）。**還缺**：Run、Trace、Artifact 的保存期限本身仍無值，`SEC-006` 因此不可判定——這是 M2 之後第一個會被使用者問到的保存政策缺口）
- [ ] PDM-007 決定 Local Runner 首批支援的作業系統。（後 MVP）
- [ ] PDM-008 決定首批目標 Agent 打包 Profile。
- [ ] PDM-009 決定封閉測試人數、招募方式與成功門檻。
- [ ] PDM-010 決定免費 Run 額度及是否支援使用者自備模型 API Key。
- [x] PDM-011 完成意圖搜尋品質 Spike：以首批精選 Skill 驗證意圖比對與符合原因生成的可行性，結果回寫 M1 搜尋作法。

## 2. 使用者研究與產品驗證（M0–M4 跨階段）

- [ ] UX-001 訪談目標個人創作者，驗證搜尋、試跑與下載需求。
- [ ] UX-002 整理學習者、改善者與精深者的主要任務與障礙。
- [ ] UX-003 以低擬真流程驗證首頁意圖輸入是否容易理解。
- [ ] UX-004 驗證 Skill 卡片上的來源、權限、相容與測試資訊是否足夠決策。
- [ ] UX-005 驗證快速 Demo 與自有資料兩種試跑入口。
- [ ] UX-006 驗證一般模式的評估報告是否不需閱讀 Trace 即可理解。
- [ ] UX-007 驗證進階使用者是否能從 Trace 找到實際問題。
- [ ] UX-008 驗證下載與安裝說明能否讓使用者在目標 Agent 完成使用。

## 3. 資訊架構與體驗設計（M0–M3）

- [ ] DESIGN-001 建立全站資訊架構與導覽模型。
- [ ] DESIGN-002 設計首頁與自然語言意圖搜尋流程。
- [ ] DESIGN-003 設計搜尋結果、篩選與排序說明。
- [ ] DESIGN-004 設計 Skill 一般詳情與進階檔案檢視。
- [ ] DESIGN-005 設計 Skill 靜態比較介面。
- [ ] DESIGN-006 設計 Fork、個人工作區與版本差異流程。
- [ ] DESIGN-007 設計 Test Case、Dataset、MCP 與工具設定流程。
- [ ] DESIGN-008 設計執行前權限及成本摘要。
- [ ] DESIGN-009 設計 Run 進度的一般模式與進階 Trace 模式。
- [ ] DESIGN-010 設計驗收條件、評估報告與改善建議流程。
- [ ] DESIGN-011 設計重新試跑與版本／結果比較流程。
- [ ] DESIGN-012 設計打包下載、安裝及驗證說明。
- [ ] DESIGN-013 完成鍵盤操作、文字標籤與錯誤訊息的無障礙檢查。

## 4. Skill 內容與供應（M1）

> **2026-08-15 依 [m1/m1-work-items-audit.md §8 第三梯](m1/m1-work-items-audit.md) 調整**：本節的 **CONTENT-007／008 里程碑改為 M2**（依賴 M2 的 Test Case 與 Sandbox，M1 內結構性不可能完成），其餘項目維持 M1。
>
> **2026-08-15 補齊允收準則**：本節九項原本只有一行敘述、`02` 無對應需求 ID（[m1/content-summaries.md §3](m1/content-summaries.md) 交付時發現）。允收準則已補於 **`02` 第 4.7 節「內容供應與策展」**，各項行尾標註引用。準則內容取自已定案與已落地的實務（PDM-002 九項檢查與白名單制、ADR-021 授權兩軸、ADR-013 索引時增強與人工抽查），未新增要求。

- [x] CONTENT-001 定義精選、已索引與外部結果的收錄政策。（允收：`02` §4.7）
- [x] CONTENT-002 定義來源可信度、License 與衍生關係的呈現規則。（允收：`02` §4.7）
- [ ] CONTENT-003 建立首批 Skill 候選清單。（正式清單見 [plans/mvp/m1/curated-skill-list.md](m1/curated-skill-list.md)、匯入資料 `tools/content/seed-skills.json`。**2026-08-15 起三類的已索引與精選數量目標全部達標**——`documents` 精選由 3 補足為 4（`excel-format`，見該文件 §9）。仍不勾的原因有二：§5.2 的 source-available 法務判定未完成前，`documents` 已索引有 10→6 的塌陷風險；九項精選檢查的 ⑤ 需 CONTENT-006 才能判定（**⑦ 已於 2026-08-16 由自動化審校判為 `pass`，15/15**）。**2026-08-15 修訂**：⑧「平台基準試跑」由已改列 M2 的 CONTENT-007／008 承接，**不再是 CONTENT-003 的勾選前提**，改以引用方式處理——CONTENT-003 勾選時須在清單中把 ⑧ 明確記為「待 M2 基準試跑，見 CONTENT-007／008」，不得記為已通過。依據 [m1/m1-work-items-audit.md §8 第三梯](m1/m1-work-items-audit.md)）（允收：`02` §4.7）<br>**2026-08-16 允收準則新增一條「依賴欄位須涵蓋套件實際需要的第三方模組（靜態掃描可驗）」**，並依此重新推導 `tools/content/seed-skills.json` 的 `deps`：**13 個 Skill 補上共 8 個原本漏抄的套件**（`pycountry`×2、`defusedxml`×3、`lxml`×3、`openpyxl`×3、`pyarrow`×4、`chardet`、`ftfy`、`confusable-homoglyphs`、`pytz`、`phonenumbers`、`python-stdnum`、`pillow`×2、`reportlab`／`pypdfium2`／`pdf2image`／`pytesseract`、`presidio-*`、`pywin32`、`numpy`、`pdf-lib`／`pdfjs-dist`）。`deps` 的語意同時釐清為「**套件宣告什麼**，不是 Runtime Image 提供什麼」，映像側的准入與拒收見 SBX-002 與 [infra/images/README.md](../../infra/images/README.md)。**同步狀態**：[m1/curated-skill-list.md](m1/curated-skill-list.md) §2 的「依賴」欄已於同日由 **CONTENT-012 同步完成**（三列更正 ＋ 檢查 ⑥ 的更正紀錄，見該檔 §3 腳註 7）；**已索引層無「依賴」欄可同步**，缺口記於 CONTENT-013。兩處不一致時**以 `seed-skills.json` 為準**。
- [ ] CONTENT-004 對首批 Skill 完成來源及 License 檢查。（License 合規總表見 [curated-skill-list.md §5](m1/curated-skill-list.md)；11 個入選 repo 已逐一實查 LICENSE 檔。未結案項：§5.2 `anthropics/skills` source-available 條款是否允許平台保存內容快照，待負責人與法務判定——**2026-08-16 已備妥分析與處置提案** [m2/anthropic-sa-license-memo.md](m2/anthropic-sa-license-memo.md)（索引／展示／沙箱試跑／打包下載四種行為逐項評估，另補 LLM 增強產出一項；建議方案 C：先關閉這 4 筆的全文展示、維持索引與下載封鎖、終判前不對外開放試跑，並與上游詢問書面澄清並行。**非法律意見，終判仍在負責人與法務**）（允收：`02` §4.7）
- [x] CONTENT-005 對首批 Skill 產生一般使用者可理解的摘要。（允收：`02` §4.7 修訂版逐條達成，見 [m1/content-review-report.md](m1/content-review-report.md) §8：45 筆全量自動化審校 **45/45 通過**、精選 15/15、主判準 45/45、忠實性 890 條宣稱 0 條未支持；審核紀錄見 [m1/content-summaries.md](m1/content-summaries.md)。**2026-08-16 兩次修正輪後仍為 45/45**：①§11 的 11 筆 Python 揭露缺口以 `enrich-skill/v5` 全數關閉；②§13 的 `docx` 依負責人路徑 ② 裁定升 `enrich-skill/v6`（新增「不得把原文分別陳述的兩件事串成一件」的通用第 4 條），A 輪修掉 KPI1 的串接外推、B 輪 KPI5 一致性自癒後**通過**，成本 $0.194／上限 $0.5）
- [ ] CONTENT-006 對首批 Skill 完成規格及靜態掃描。（允收：`02` §4.7）<br>**2026-08-16 新增依賴掃描**：`services/platform/internal/skillpkg/deps.go` 出兩個 finding——資訊級 `package-dependencies`（套件證據所及的全部第三方依賴，`Details` 為結構化清單，即 CONTENT-003 依賴欄位應填的值）與警告級 `undeclared-dependency`（程式碼 import 但套件從未宣告者）。**對 45 個 pin commit 套件實跑驗證**：修正後的 `deps` 與掃描結果一致，僅 3 筆差異且皆為真實發現——`pdf` 的 `pdf-lib`／`pdfjs-dist`（Node 支路）、`excel-validate` 的 `numpy`（已補入 `deps`）、`excel-delete` 的 `compact_rows`（**SKILL.md 指示 `from compact_rows import compact_xlsx` 而套件未附該模組**，屬本項「指向套件外檔案的跨 Skill 引用」準則，pin commit 上未解決，已記於該筆 `notes`）。抽取為純文字比對、不執行任何東西（鐵律 1）；動態構造的 import 明確不涵蓋，故為佐證非保證。<br>**開發期兩個誤判已修並留測試**：行首 `from` 未要求 `import` 關鍵字時會把英文散文（"from the model API"）算成依賴；`npm install` 未錨定行首時會把句中反引號引用（"do not run `npm install` first. The model knows the API"）整句吃成套件名。
- [ ] CONTENT-007 對精選 Skill 建立範例資料、Prompt 與驗收條件。**（M2：依賴 Test Case 與 Sandbox；2026-08-15 依審計調整）**（**2026-08-16 部分完成**，見 [m2/content-baseline-report.md §10](m2/content-baseline-report.md)：15 個精選各有一組範例 Dataset、點名該 Skill 的 User Prompt 與三條驗收條件，並以 `test_case_snapshots` 不可變保存；範例資料為合成內容、無 Secrets 與個資。**不勾的唯一原因**：`writing` 類精選的「可編輯 rubric，供 LLM Judge 逐項回傳證據引文」未做——EVAL-001／002 的 Judge 介面尚未實作，rubric 沒有消費端）（允收：`02` §4.7）
- [x] CONTENT-008 對精選 Skill 完成至少一次基準試跑。**（M2：依賴 Test Case 與 Sandbox；2026-08-15 依審計調整）**（**2026-08-16 完成**：45 個 Skill 全數經完整平台路徑在隔離 Sandbox 實跑，**精選 15/15 結果為「符合」**（Run `succeeded` ＋ trace 有 `skill_activation` ＋ artifact 確有檔案），逐 Skill 結果、可追溯性與允收對照見 [m2/content-baseline-report.md](m2/content-baseline-report.md)。九項精選檢查的 ⑧ 可由 `pending` 改記 `pass`。已索引 30 個中 20 個符合、9 個被閘道預算計數缺陷中止（非 Skill 問題，§6.2）、1 個 Run 成功但未產出。**同日補跑**：預算保留根因修復（`3906fe5`）後，該 9 筆於 Runtime Image `2026.08-2` 重測，**8/9 符合、幽靈 429 歸零**，餘 1 筆（`pptx`）改以 PDM-005 5.2a 的輸入 token 上限失敗。**全量重測（負責人裁定）**：其餘 36 筆亦於 `2026.08-2` 重跑，36/36 Run 成功、符合 34，33 筆 `transpiled` 全數轉 `native`，同組 Skill 成本 −28%、輸入 token −50%；全 45 筆最新量測合併後**符合 42／45，精選在兩個映像上皆 15/15**，見 §12、§13）（允收：`02` §4.7）
- [ ] CONTENT-009 建立內容更新、失效、下架與來源變更流程。（允收：`02` §4.7）
- [x] CONTENT-010 於 Runtime Image `2026.08-3` 重跑 45 筆基準，回填 `skill_runtime_compatibility` 的 (Skill Version × `2026.08-3`) 一軸。（**2026-08-16 完成**，見 [m2/content-baseline-report.md §14](m2/content-baseline-report.md)：45 筆全量發動，**41 筆完成量測**（符合 39、未產出 2）、**4 筆授權受限如實跳過**（`docx`／`pdf`／`pptx`／`xlsx`，`POST /runs` 回 422 `license-review`，未繞過）；41 筆判定與 `2026.08-2` **完全一致**，`ModuleNotFoundError` 由 4 筆歸零、輸入 token −16%。回填寫入 **41 列**（`activated`／`native`），舊映像 90 列 md5 前後相同。**四筆 restricted 在 `2026.08-3` 無列**，目錄仍顯示其 `2026.08-2` 舊值並附映像標籤，解除後須補跑——連同「office 三件套 validator 未被執行過」一併列為後續（§14.9）。閘道實付 $2.0038。**原始工作說明保留於下**：由 SBX-002 的 `2026.08-3` 升版產生：0022 以 (Skill Version × Runtime Image) 為鍵，`2026.08-3` 目前 **0 列**，目錄仍顯示 `2026.08-2` 的結論並附映像標籤。依賴集**只增不減**使結論極可能不變，但 0022 的鍵不接受「極可能」——ADR-023 的四項行為重驗已在新 digest 上實跑通過（[UPGRADES.md](../../infra/images/runtime-agent-sdk/UPGRADES.md)），那管**行為回歸**；本項管**目錄事實**，兩者不互相取代。成本以 [m2/content-baseline-report.md §13.6](m2/content-baseline-report.md) 實測推估約 **$2.2**。回填走 `tools/content/backfill-agent-compatibility.sql`，須帶 `-v image=` 與 `-v since=` 的時間窗，否則會把新量測貼上舊映像標籤）
- [ ] CONTENT-011 重跑 CONTENT-005 增強，使被拒收依賴造成的能力缺口進入「限制」欄。（**不得就地改寫**：`02:CONTENT-005` 規定 `需修改` 的處置是調整 prompt 或標記人工覆寫後**重跑增強與重新索引**，因此 `2026.08-3` 這批**只記錄建議、不執行**。對象三筆與應涵蓋內容：`pdf`——渲染／OCR 支路（`reportlab`／`pypdfium2`／`pdf2image`／`pytesseract`）與 Node 支路皆不可用，可用的是 `pypdf`＋`pdfplumber`＋`pillow` 的讀取／抽取／合併；`pii-flag`——`presidio-*` 偵測引擎不可用，可用的是模型判讀＋`phonenumbers`／`python-stdnum` 格式驗證，**NFR-001 的措辭紀律因此更吃重**；`document-format-skills`——`pywin32`／COM 的 `.doc`／`.wps` 轉檔在 Linux 沙箱上永久不可能，`.docx` 路徑可用。另 `pyarrow` 遭拒使 `add-iso3166`／`add-data-dictionary`／`data-comparability`／`data-cleanliness-scan` 四筆的 Parquet 格式支援不可用，亦應揭露。理由與逐項對照見 [infra/images/README.md](../../infra/images/README.md)「拒收」表。<br>**2026-08-16 負責人確認：本項建議已記錄在案即可，D 日前不執行增強重跑**（凍結標的不動）。因此本項**保持未勾**，且不因 `2026.08-3` 已發佈而視為過期——待解凍後照 CONTENT-005 流程重跑）
- [x] CONTENT-012 同步 [m1/curated-skill-list.md](m1/curated-skill-list.md) §2 三張表的「依賴」欄與 `tools/content/seed-skills.json`，並依負責人裁定補記檢查 ⑥ 的更正。（**2026-08-16 完成**。<br>**① 依賴欄**：需修正的只有 §2.3 `data` 三列，因為 §4 已索引各表**沒有「依賴」欄**（本項原估「13 列跨三張表」是高估）——`data-cleanliness-scan`（~~prompt-only（`pandas`／`numpy`）~~→ `pandas`／`openpyxl`／`chardet`／`python-dateutil` ✅＋`pyarrow` ⛔）、`csv-to-json` 與 `text-to-numeric`（~~prompt-only~~→ `pandas`）。三者是**同一個錯誤**：把「無 Script」寫進「依賴」欄當成「無依賴」，`numpy` 更是憑空多出來的。已改用刪除線＋更正註記（repo 慣例），並在「Script 規模」欄註明**無 Script ≠ 無依賴**。<br>**② 檢查 ⑥ 更正（負責人 2026-08-16 裁定：兩個讀法都不採，記更正）**：判定值**維持 `pass`**，但補記「原判定於記錄當時無依據」——原值是拿抄漏的 `deps` 去比對得出的，**是沒有輸入的判斷，不是錯的判斷**；依當時正確資料，於映像 `2026.08-2` 應為 `fail`。**歷史不改寫**，更正紀錄見 curated-skill-list.md §3 腳註 7 與 `seed-skills.json` 的 `checks` schema。<br>**③ 判準一併明確化**（原措辭無法回答「刻意不裝」）：**⑥ pass ⟺ 不需外網／MCP，且每個宣告依賴要麼在 Runtime Image 內、要麼在該映像的具名拒收清單內——落差已揭露即通過，落差未知才不通過。** 兩個輸入皆為機器產出可重跑（`skillpkg` 的 `package-dependencies` finding ＋ `infra/images/README.md` 的納入／拒收表，映像 `2026.08-3`）。<br>**④ 精選數維持 6**（`data` 類，PDM-002 [4, 6] 內），因現況判定成立；CONTENT-003 的達標敘述無需更動。<br>**⑤ 一項與裁定文字不符、已按事實記錄**：裁定稱「七筆…2026.08-3 已補齊依賴」，但實測**受影響的是 13 筆而非 7 筆**，且該 7 筆正是 `2026.08-3` **未**補齊的那些（其依賴遭拒收）；真正被補齊的是另外 6 筆（`docx`／`pptx`／`xlsx` 的 `defusedxml`、`standardise-country-names` 的 `pycountry`、`unicode-consistency` 的 `ftfy`＋`confusable-homoglyphs`、`date-wrangling` 的 `pytz`）。裁定的**結論**（維持 pass、精選 6、記更正）不受影響，因為新判準對兩組都成立；只有理由要分開寫，已如實照辦）
- [ ] CONTENT-013 為已索引層補「依賴」欄或等價揭露。（由 CONTENT-012 發現：[m1/curated-skill-list.md](m1/curated-skill-list.md) §4.1／4.2／4.3 三張已索引表**只有 License 與 Tier，沒有依賴欄**，所以 2026-08-16 的 `deps` 修正在該層**沒有可同步的欄位**——12 筆已索引 Skill 的依賴更正只存在於 `seed-skills.json` 與 §3 腳註 7。已索引層依 PDM-002 不要求九項全過，故非阻擋，但「使用者看得到的依賴揭露」最終仍應由 DISC-003 的詳情頁承接而非本文件；先記錄缺口，實作歸屬待定）

## 5. 核心領域與帳號（M1）

- [x] CORE-001 建立 User 與個人工作區資料模型。
- [x] CORE-002 建立 Skill、Skill Source、Skill Version 與 Fork 資料模型。
- [x] CORE-003 建立 Test Case、Run、Trace、Evaluation 與 Artifact 資料模型。
- [x] CORE-004 定義不可變 Skill Version 與歷史 Run 快照規則。
- [x] CORE-005 建立基本登入、登出與工作區存取控制。（GitHub OAuth ＋ Postgres session ＋ `DEV_LOGIN` 離線 provider；登出為伺服器端撤銷。證據見 [m1-work-items-audit.md §3.1](m1/m1-work-items-audit.md)）
- [x] CORE-006 建立使用者私有內容的授權檢查。（Workspace scope 一律取自 session，不信任 UI 傳入值；非擁有者一律 404。證據見 [m1-work-items-audit.md §3.1](m1/m1-work-items-audit.md)）
- [ ] CORE-007 建立使用者資料與 Artifact 的刪除流程。
- [ ] CORE-008 建立重要操作的 Audit Event。

## 6. Skill 匯入、驗證與索引（M1）

- [x] INGEST-001 支援從允許的 URL 匯入 Skill。
- [x] INGEST-002 支援上傳 Skill 套件。
- [x] INGEST-003 解析 `SKILL.md` 與套件檔案樹。
- [x] INGEST-004 保存來源 URL、版本／Commit、擷取時間與內容雜湊。
- [x] INGEST-005 偵測重複內容並避免覆蓋既有版本。
- [x] INGEST-006 實作 Agent Skills 規格驗證。
- [x] INGEST-007 實作檔案引用、依賴、Script、外部 URL 與疑似 Secret 靜態檢查。
- [x] INGEST-008 分開呈現錯誤、警告與資訊訊息。
- [x] INGEST-009 建立可搜尋索引與重新索引流程。
- [ ] INGEST-010 建立外部內容失效、來源更新及人工下架流程。
- [x] INGEST-011 靜態檢查涵蓋 `SKILL.md` 內嵌可執行程式碼並揭露規模（`02:SKILL-003`）。
- [x] INGEST-012 實作 License 多層溯源：記錄來源層級、打包器搬運 repo 層授權、SPDX 正規化（`02:SKILL-004`、[ADR-021](../../adr/ADR-021-skill-license-provenance.md)）。
- [x] INGEST-013 外部 URL 揭露依主機聚合並保留完整明細（`02:SKILL-005`）。
- [x] INGEST-015 修正索引增強補跑的兩個平台缺陷（[m2/README.md 乙-12](m2/README.md)）。（**2026-08-16 新增並完成**。**歸屬裁定：不重開 INGEST-009，以本項承接**——INGEST-009 的允收準則在當時完全成立且有證據，兩個缺陷都是 M2 才出現的新資料形態誘發的（基準試跑在臨時 Workspace 留下 45 筆 fork 文件；修正輪首次做大批量增強），把一個已結案的工作項改回未完成會讓「某一時點的帳」失去意義，而缺陷本身仍必須被修、被記錄——所以記在這裡而不是那裡。<br>**(a) 逾時**：`internal/llmclient` 的預設 client 不再自帶 `Timeout: 30s`，逾時完全交給呼叫端的 ctx。原本那 30 秒比每一個呼叫端的預算都短——`ingest/enrich.go` 給 75 秒是為了讓 LLM 服務自己的 60 秒上限先報錯，那個預算從來沒被用到；修正輪 14 次成功增強裡有 3 次因此逾時、重跑即過，而 client 端放棄不會取消上游，每次逾時都已在閘道產生費用。五個呼叫端（enrich 75s／embed 20s／catalog embed 10s／match reasons 8s／testlab suggest）全部已帶 ctx deadline，已逐一確認；`client_test.go` 以一個永不回應的 server 斷言呼叫者的 100ms deadline 是唯一結束它的東西。<br>**(b) 時間戳**：`db/queries/search.sql` 的 `ReindexAll` 在 `ON CONFLICT` 路徑不再寫 `updated_at = now()`（新列仍寫）。`search_documents.updated_at` 全庫唯一的讀者是 `ListPendingEnrichment` 的 oldest-first 排序，重建投影不會讓一份文件變新；無條件蓋掉它會讓整個投影共用同一瞬間，排序失去意義，`REINDEX_BATCH` 也就擋不住那 45 筆 fork（每次補跑約 $2 的旗艦增強）。保留時間戳後新 fork 自然排在最後，有界的 `REINDEX_BATCH` 先取到真正舊的 pending 列，**每次補跑前手動暫標 `enriched`、跑完還原的人工步驟不再需要**。<br>**誠實界線**：這不會讓 fork 文件永遠不被增強——夠大的批次仍會取到它們。「私有 fork 的增強是否應複用來源版本的既有增強（內容雜湊相同）」是另一個問題，不在本項）
- [ ] INGEST-014 實作匯入抓取器的 SSRF 與內部網路防護：scheme 白名單、解析後逐一比對位址封鎖清單、DNS Rebinding 防護（解析與連線綁同一位址、每跳重驗）、redirect 上限 3 跳且跨主機不帶憑證、回應體 10 MB 邊讀邊中止、連線 10 秒／整體 60 秒逾時、fail-closed、錯誤訊息不洩漏內部位址。（**2026-08-16 新增**：威脅模型 **Q15** 已裁定歸屬為擴充 `02:SEC-003` 而非另立需求 ID，理由見該節；封鎖清單與實作須與後 MVP 的 `02:SEC-004` 共用同一份。**未實作**）（允收：`02:SEC-003`）

## 7. Skill Explorer（M1，結束時通過驗證閘門才進 M2）

- [x] DISC-001 實作自然語言意圖搜尋。
- [x] DISC-002 產生候選 Skill 與符合原因。
- [x] DISC-003 實作類別、來源、Agent、Script、MCP 與驗證狀態篩選。**（2026-08-15 依修訂後的 `02:DISC-002`「篩選維度的允收階段」勾選：M1 階段的兩個維度「是否包含 Script」與「驗證狀態」已實作並有逐筆資料；其餘四個維度未達允收階段，依同節新增準則以停用控制項＋理由呈現，API 回 400 並附理由，皆有具名測試。類別／來源層級待 CONTENT-003 策展資料、Agent 相容待 M2 Sandbox、MCP 待後 MVP。證據與解除條件見 [m1/m1-work-items-audit.md §5.3](m1/m1-work-items-audit.md)。）** **2026-08-16 追加：「Agent 相容」維度已啟用**——migration `0022` 建 `skill_runtime_compatibility`，回填 45 筆 M2 基準實測（`capability` 45/45 `activated`；`runtime` 12 `native`／33 `transpiled`），`?agent=native|transpiled|failed|unverified` 精確比對而非布林（`transpiled` 是第三種答案，布林會讓「非 native」悄悄含進沒人測過的列），搜尋與詳情一併回傳 `runtime_image` 與 `measured_at`。**capability 一軸顯示但不可篩**，理由同「來源層級」：全目錄同值，篩了分不出東西。`apps/web` 的停用控制項由四個減為三個，並新增可用的「Agent 相容（實測）」選單，`disc.test.tsx` 有具名測試。詳見 [m2/README.md 殘項乙-4](m2/README.md)。
- [x] DISC-004 實作可解釋的排序規則。
- [x] DISC-005 實作無結果、低信心及查詢補充流程。
- [x] DISC-006 實作 Skill 一般詳情頁。
- [x] DISC-007 實作 `SKILL.md` 與檔案樹進階檢視。
- [x] DISC-008 實作來源、License、風險及相容狀態展示。
- [x] DISC-009 實作至少兩個 Skill 的靜態比較。
- [x] DISC-010 驗證公開搜尋與詳情不要求登入，私有操作要求登入。

## 8. Fork、版本與工作區（M1）

- [x] WS-001 實作 Fork 並保留來源與 License 關係。
- [x] WS-002 實作不可變版本保存。
- [x] WS-003 實作任兩版本差異比較。
- [ ] WS-004 實作個人 Skill、Test Case、Run 與下載紀錄列表。
- [x] WS-005 實作私有內容刪除與狀態回饋。
- [x] WS-006 驗證不同使用者無法存取彼此私有內容。（列表／刪除／Fork／Diff 四條路徑與公開搜尋皆有具名整合測試，CI 帶 Postgres service 實際執行。證據見 [m1-work-items-audit.md §6](m1/m1-work-items-audit.md)）

## 9. Test Case 與執行設定（M2）

> **2026-08-16 §9 允收範圍裁定（乙-7 結案，取 `m2` 對帳 §3.2 的 (a) 但只退回一項）**：本節的允收準則**含 UI**，但**含不含 UI 是逐條判定，不是逐節判定**——判準取自 `02` 各條允收準則自己的主詞與動詞：
>
> | 準則的主詞／動詞 | 要在哪個平面滿足 | 例 |
> | --- | --- | --- |
> | 「系統必須…」「每個 Run 必須…」 | 系統平面，API ＋ 契約 ＋ 強制點即成立 | `02:TEST-001` 第 1、4 條 |
> | 「系統依…自動建議…為可選強化」 | 同上 | `02:TEST-001` 第 2 條後半 |
> | 「**使用者可**…」「**顯示**…」 | 使用者碰得到的平面，**沒有介面就是沒有達成** | `02:TEST-001` 第 2 條前半、第 3 條；`02:TEST-002` 第 2 條 |
>
> 這把尺可以逐條判定，不需要先回答「§4.3 是在描述系統能力還是端到端可用性」——**條文自己已經用主詞回答了**。TEST-004 因「顯示」被退回、補上 `/lab/datasets` 後重新勾選，是同一把尺的第一次套用；本次套用結果為 **TEST-003 退回、TEST-001／002 維持**，逐條理由見 [m2/m2-work-items-audit.md §12](m2/m2-work-items-audit.md)。
>
> 選 (a) 而不選 (b)（把 §9 界定在 API＋契約層）的理由：`02` §4.3 的條文是既有的產品承諾，把它改寫成「API 層即可」等於為了讓帳好看而降低承諾——而 MVP 的 Persona 明確是**非技術使用者**（`01` §2.1），一個只能用 `curl` 達成的「使用者可」不是這份文件在說的事。介面本身由 `DESIGN-007`（設計）與新增的 **TEST-012**（實作）承接。

- [x] TEST-001 實作 User Prompt 輸入與驗證。（Test Case CRUD 綁 Workspace 內 Skill；非空白與長度驗證在 Service 與 0004／0017 的 CHECK 雙重把關。**2026-08-16 同尺審查後維持勾選**：本項對應的 `02:TEST-001` 第 1 條為「每個 Run 必須包含非空白 User Prompt」，主詞是 Run／系統，強制點在 Service 與 DB CHECK，**條文沒有要求任何顯示或使用者操作**。誠實記錄：`apps/web` 目前**沒有**輸入 Prompt 的畫面，Prompt 只能經 API 寫入——那不是本條的字面要求，而是 `DESIGN-007`／`TEST-012` 的缺口）
- [x] TEST-002 實作驗收條件自動建議。（`POST /test-cases/{id}/criteria/suggest`：Go 讀 Skill 名稱／摘要、User Prompt 與 Dataset **欄位名＋推斷型別**，經內部 HTTP 呼叫 `services/llm` 的 `POST /suggest-criteria`（mini 級 `gpt-5.4-mini`、`json_schema` strict、經 LiteLLM 閘道）。**Dataset 的資料列不出境**：請求 schema 只有欄位名與型別兩個欄位，型別由第一列在 Go 程序內推斷後即丟棄（鐵律 11，具名整合測試以真實列值反證）。寫入時標 `source='suggested'` 且未確認；使用者改寫文字即轉為 `source='user'`。LLM 未設定或失敗回 503 並保留手動路徑，`02:TEST-001` 的「可選強化」語意成立。**2026-08-16 同尺審查後維持勾選**：本項對應的是 `02:TEST-001` 第 2 條的**後半**「系統依 Prompt 與 Skill 自動建議驗收條件為**可選強化**」，主詞是系統、且條文明文為可選；前半的「使用者可純手動建立驗收條件」是**必要**的那一半，主詞是使用者，由 TEST-003 承接——它退回，本項不退回，因為一個可選強化的實作與否不因必要路徑缺介面而改變判定）
- [ ] TEST-003 實作驗收條件新增、修改、刪除與確認。（確認為明示同意欄位；改寫文字即撤銷既有確認。**2026-08-16 同尺審查後退回 `[x]` → `[ ]`（部分完成）**：`02:TEST-001` 第 3 條是「**使用者可**新增、編輯、刪除及確認驗收條件」，四個動詞的主詞都是使用者，而 `apps/web` **沒有任何驗收條件的介面**——沒有列表、沒有編輯器、沒有確認控制項（`src/api/lab.ts` 只包了 limits／upload／preflight／confirm／startRun 五支，沒有 criteria 端點的呼叫端）。`RunPreflight` 上的「確認」是**權限摘要**的確認（`02:TEST-005`／TEST-009），與驗收條件的確認是兩件事，不能互相頂替。<br>**後端已落地的部分完全站得住**：`testlab.go:249/267/301` 的新增／修改／刪除、`mutateCriteria:313` 的 `LockTestCase` 行鎖、`:279-286` 的「改寫文字即撤銷確認且寫回相同文字不撤銷」，測試 `TestAcceptanceCriteriaLifecycle`。缺的**只有**顯示與操作面，補上即可依允收重新勾選——與 TEST-004 的退回與補回同型。<br>實作由 **TEST-012** 承接；退回依據見 [m2/m2-work-items-audit.md §12](m2/m2-work-items-audit.md)）
- [x] TEST-004 實作 Dataset 上傳、限制、關聯與刪除。（PDM-005 §5.1 全數強制：單檔 25 MB、單 Test Case 100 MB／20 檔、magic bytes 判型不信副檔名、`expires_at` 90 天；刪除連同物件，皆有具名整合測試。**2026-08-16 對帳退回後重新勾選**：退回的唯一理由是 `02:TEST-002` 第 2 條「上傳前**顯示**大小限制、保存政策及資料使用範圍」沒有顯示的地方；`apps/web` 已補上 `/lab/datasets` 上傳頁（`src/pages/DatasetUpload.tsx`），三項規則**在檔案選擇控制項之前**渲染，資料來源是 `GET /test-cases/limits`，UI 內不另寫一份數字。讀不到規則時**不提供上傳控制項**（fail-closed，規則沒顯示就等於「上傳前顯示」沒發生）。兩個具名 vitest（`src/dataset.test.tsx`）分別斷言「規則在畫面上且此時尚未呼叫過 `/datasets`」與「讀不到規則時沒有 file input」。**範圍誠實記錄**：仍只有上傳這一步，Test Case 建立、Prompt／驗收條件編輯、檔案列表與刪除的 UI 仍屬 `DESIGN-007`，故 test_case id 由 URL 帶入，與 preflight 頁同一個做法。**2026-08-16 更新**：TEST-001／002／003 的同尺審查已裁定——TEST-003 退回、TEST-001／002 維持，判準見本節開頭的裁定說明與 [m2/m2-work-items-audit.md §12](m2/m2-work-items-audit.md)）
- [ ] TEST-005 實作遠端 MCP 位址及短效憑證設定。（後 MVP）
- [ ] TEST-006 實作 MCP 工具發現與權限選擇。（後 MVP）
- [ ] TEST-007 實作 Local Runner 連線狀態與本機絕對路徑選擇。（後 MVP）
- [x] TEST-008 實作執行前 Dataset、Script、MCP、工具、網路與 Secrets 摘要。（`GET /skills/{id}/runs/preflight`，八項全數揭露：Dataset 名稱／型別／大小與合計、Script 由 `skillpkg.Validate` 重掃實際會執行的套件位元組（讀不到時標 `unavailable`，**不得呈現為 `none`**）、工具為 Sandbox 內建檔案與 Shell、**MCP 在 MVP 恆為空清單並明確顯示為「無」而非略過**、網路取 `policy_snapshot` 的 `default_deny` ＋空允許清單、Secrets 只列注入項名稱、Provider 取排程實際會選中的那一個、資源上限直接取 `DefaultResourceLimits()`／PDM-005 §5.2。摘要以 canonical JSON（固定欄位順序的 struct）取 sha256，慣例同 `testlab/snapshot.go`）
- [x] TEST-009 實作權限異動後重新確認。（migration **0020** `run_permission_confirmations` 記錄「誰、在何時、同意了哪個摘要 hash」；`POST /skills/{id}/runs/preflight/confirm` 寫入，`internal/run/service.go` 的 `Create` 入口重算當下摘要 hash 並要求兩件事同時成立：請求帶的 hash 等於重算值、且該 hash 有確認紀錄。任一不成立回 **422 且不建 Run**——這是 **SEC-002 閘門 B**「使用者未確認或未重新確認執行前權限摘要」那一項。舊確認因 hash 為查詢鍵而自然失效，不需另作撤銷掃描；換／刪 Dataset 皆有具名整合測試。使用者拒絕＝不呼叫確認端點，沒有紀錄就起不了 Run）
- [x] TEST-011 在執行前權限摘要加上**預估成本區間**（區間非單值）。（**2026-08-16 新增並實作**：PDM-005 §5.3 指定要回寫 `02:TEST-005` 的欄位清單當時沒有回寫，[m2 對帳 §9.3](m2/m2-work-items-audit.md) 查出 `PermissionSummaryContent` 完全沒有此欄位。實作為 `PermissionSummary.EstimatedCost`（`internal/run/preflight.go`）與前端 `RunPreflight` 的「預估成本（估計值）」列，值為 $0.01／常見 $0.06／$0.30，來源是 M2 基準試跑 45 個 Skill 的閘道實付分布（中位數 $0.0566、平均 $0.0702、最大 $0.2367），`basis` 欄把來源與「非報價」寫在畫面上。**必須是區間**——首次與後續 Run 因 prompt caching 差約 8 倍（PDM-005 §5.2a-6），單值必然對其中一種情境是錯的。**刻意不進 `summary_hash`**：hash 涵蓋的是「這個 Run 能碰什麼」，成本是預測不是權限，拿更大的樣本重新校準估計值不該把使用者手上每一份確認一起作廢——與 User Prompt 不入 hash 同一條規則。整合測試 `TestCostEstimateIsOutsideTheConfirmedHash` 直接對回應位元組重算 sha256 反證這件事。此欄位不在 TEST-008 的字面範圍內，故不退回 TEST-008。**契約缺口（記錄，未改 spec）**：`contracts/openapi/public.yaml` 的 `RunPermissionSummary` 尚未宣告 `estimated_cost`，屬 additive 變更，待契約批補上）（允收：`02:TEST-005`）
- [ ] TEST-012 實作 Test Lab 的 Test Case 與驗收條件介面：建立 Test Case、輸入與編輯 User Prompt、驗收條件的新增／編輯／刪除／確認、Dataset 列表與刪除。（**2026-08-16 新增**，承接本節開頭裁定退回的 **TEST-003**，並補齊 `02:TEST-001` 第 2 條前半「使用者可純手動建立驗收條件」。**範圍界線**：本項只做已存在端點的消費端，不新增 API、不改契約——五支端點（Test Case CRUD、criteria 增改刪確認、`POST /test-cases/{id}/criteria/suggest`、`/datasets`、`/test-cases/limits`）全部已落地且有整合測試。設計由 `DESIGN-007` 承接；兩者拆開是因為 `DESIGN-007` 涵蓋的流程比本項寬（含 MCP 與工具設定，兩者皆為後 MVP）。<br>**做完之後才能勾 TEST-003**：判準是「一個沒有 API 知識的使用者能不能新增、改、刪、確認一條驗收條件」，不是「畫面上有沒有這些欄位」。<br>**順帶收掉的既有權宜**：`/lab/run` 與 `/lab/datasets` 目前用 URL query 帶 `test_case` id（`router.tsx` 兩處註解都指名「沒有 picker，等 DESIGN-007」）——本項落地後應改由本頁連過去，把 id 從使用者要自己知道的東西變回內部識別）（允收：`02:TEST-001` 第 2 條前半、第 3 條）
- [x] TEST-010 保存實際執行使用的 Test Case 快照。（`testlab.CreateSnapshot` 為唯一實作，由 `internal/run` 於建立 Run 的同一交易呼叫；快照涵蓋 Prompt、驗收條件與 Dataset 參照並以單一 content hash 固定，不可變由 0005 trigger 保證；已刪除的 Test Case 不可起 Run）

## 10. Run Orchestrator 與 Provider 契約（M2）

- [x] RUN-001 定義 Provider-neutral Run Request 與 Run Result。（`contracts/openapi/sandbox-provider.yaml`）
- [x] RUN-002 定義 Provider Capability 描述格式。（同上檔案 `ProviderCapability`；能力相容檢查屬 RUN-005）
- [x] RUN-003 定義平台 `run_id` 與 `provider_run_id` 映射。（0016 `run_attempts`；解掉 0004「重試覆寫 `provider_run_id`」的已知債）
- [x] RUN-004 實作 queued 到 cleaning_up 的標準狀態機。（**2026-08-16 由 RUN-007 一併結案**：終態轉移在同一交易內排入 cleanup job，清理結果寫 `runs.cleanup_status`／`cleanup_at`。`cleaning_up` 依 ADR-004「執行結果與清理結果分開記錄」不進 `run_status` enum，而是 0004 既有的 `run_cleanup_status` 欄位——「Run 結束後必須進入清理流程」與「重複清理不造成錯誤」兩條允收由 `internal/run/cleanup.go` 與整合測試覆蓋）
- [x] RUN-005 實作 Run 排程、Provider 選擇與能力相容檢查。（Provider 註冊表為部署靜態設定 `SKILLHUB_SANDBOX_PROVIDERS`／`SKILLHUB_SANDBOX_TOKEN_<NAME>`，不做動態註冊；`GET /capability` 以 30 秒 TTL 快取，worker 啟動時清空重讀；相容檢查涵蓋隔離等級、rootless、egress 模式、Runtime 家族與整合模式、六項資源上限，不相容者**在排入佇列前**回 422 並附逐一理由；選定結果寫 `runs.provider` 與 `runtime_snapshot`，`provider_run_id` 只寫 `run_attempts`）
- [x] RUN-006 實作取消、逾時、有限重試與失敗分類。（取消：`cancel_requested_at` → 輪詢時呼叫 provider cancel → 待 provider 回終態才轉移，符合「取消不得謊報已停止」；逾時：provider 回報 `timed_out` 為軟逾時，平台側以 `created_at + wall_clock_hard_seconds` 為硬逾時，driver 與 supervisor 雙重把關；重試：新增 `run_attempt` 且上限入設定（預設 3），**僅 provider 側失敗可重試，workload 自身失敗不重試**；分類寫入 0018 新增的 `runs.failure_class`。**限制註記**：重試窗僅涵蓋 run 仍在 `provisioning` 的派送階段——狀態機無回退邊，離開 `provisioning` 後的 provider 失敗只分類不重試，要放寬需新 ADR）
- [x] RUN-007 實作冪等清理與遺留 Sandbox 掃描。（終態轉移於同交易排入 cleanup job（River unique，重複排入為 no-op）；`DELETE` 依契約冪等且無 404，重跑安全；孤兒掃描以 `GET /runs?active=true` 比對平台狀態，僅在「平台已終態」或「平台不認得且 `observed_at - created_at` 超過 5 分鐘寬限」時 destroy，避免誤殺派送中的新 Run；清理失敗記 `cleanup_status='failed'` 並由 supervisor 重排）
- [x] RUN-008 實作服務重新啟動後的 Run 狀態恢復或安全終止。（supervisor 為 River periodic job（30 秒，`RunOnStart`，僅 leader 執行）：掃非終態 Run，逾期者判 `timed_out`＋清理，其餘以 unique job 重新入列——已有在途 job 時自動 no-op，故不需讀 river 表；有在途 attempt 者重新掛回輪詢，已離開派送階段卻無 attempt 可接者判 `platform_error` 安全終止）
- [x] RUN-009 建立 Provider 契約測試套件。（`internal/run/provider_contract_test.go`：冪等重送同資源／同鍵不同內容 409／cancel 已終態仍 202／DELETE 重複與未知 handle 皆 204／`active=false` 回 400／終態必帶 result／無 token 401；預設跑 in-repo fake（`internal/run/providertest`），設 `SKILLHUB_PROVIDER_CONTRACT_URL`＋`SKILLHUB_PROVIDER_CONTRACT_TOKEN` 即對真實服務跑同一套。狀態映射另以 `schedule_test.go` 的決策表驗證）

## 11. SelfHostedProvider（M2）

實作位於 `services/sandbox/`（獨立 Go module，ADR-019、鐵律 2），部署與 dev／prod 差異見 [services/sandbox/README.md](../../services/sandbox/README.md)。允收準則來源為 `02:RUN-003` 與威脅模型 SEC-002 的 45 項基線（本節以 `C-xx`／`N-xx`／`D-xx`／`I-xx`／`X-xx` 引用）。

- [x] SBX-001 決定自建 Sandbox 的隔離技術與執行節點拓撲。（[ADR-015](../../adr/ADR-015-sandbox-isolation-technology.md) 已 Accepted：gVisor 基線＋專用 VM 池、不進 Kubernetes；DockerProvider 以 `SKILLHUB_SANDBOX_RUNTIME=runsc` 落實該選擇，宣告的 `isolation.level` 跟著實際設定走，跑 runc 的機器只會宣告 `container`。**在部署平台上實跑 `runsc` 屬部署期驗收**，ADR-015 定案紀錄已將其列為實作期前提，不影響本項的「決定」性質）
- [ ] SBX-002 建立經審核的 Runtime Image。**部分完成**：審核流水線已接上——[`.github/workflows/runtime-image.yml`](../../.github/workflows/runtime-image.yml)（path filter 只在 `infra/images/**` 變更時觸發）做 digest 斷言 → build → syft 產 SPDX SBOM → grype 掃描 → 門檻閘門，SBOM 與報告以 `if: always()` 上傳為 artifact，掃描失敗也留證據。I-02 已落地（`FROM` 帶 `@sha256:`，CI 以 grep 斷言）。掃描實跑結果與豁免清單見 [infra/images/README.md](../../infra/images/README.md)：可修的 Critical／High 原有 6 件、全在 base image 自帶的 npm 依賴，已於 Dockerfile 移除 npm／corepack（執行期不載入，沙箱亦無網路），現為 **0**；其餘 7 Critical／17 High 皆為 Debian bookworm 無上游修復項，逐項具名列入豁免清單並訂複審日，不靜默放行。**門檻已定案（2026-08-16）**：[ADR-022](../../adr/ADR-022-sandbox-deployment-topology-and-security-thresholds.md) 第二部分採納本流水線的兩項提案值並補上批准與時效——I-06 為「可修的 Critical／High 阻擋且**無豁免路徑**，不可修者具名豁免、複審日 ＝ **`first_exempted_at`（該 CVE 首次被豁免的日期，跨重掃保留）**＋90 天、逾期即依 I-04 判為過期」——錨定在首次而非最近一次掃描日是刻意的，否則每 30 天的例行重掃都會把複審日往後推 90 天，人工複審永遠不會到期，I-04 為「有效期 **30 天**，到期前 7 天告警，到期即不得被新 Run 引用」。~~**未勾原因（只剩一項）**：I-03 要求 SBOM 隨 Image 保存以供閘門 A 查詢、I-04 的 `scanned_at` 同理，兩者現落在 90 天 CI artifact 與人工維護的日期。~~ **2026-08-16 已由 SBX-011 解除**：SBOM 與掃描結果（含機器可讀的 `scanned_at`）已成為隨 GHCR digest 的 attestation，`rescan` job 每週重掃已發佈的 digest，**流水線側四項檢查 I-02／I-03／I-04／I-06 已全部可自動化判定**。<br>**2026-08-16 另一項變更**：映像升 **`2026.08-2`**——加 `python3` 與 45 個目錄 Skill 宣告的 9 個 Python 依賴（版本 pin 取「通得過 I-06 的最新版」，第一次嘗試的保守舊版在 `lxml`／`pypdf`／`pdfminer.six` 上被抓到可修的 High，依 ADR-022 無豁免路徑故往上釘）。掃描重跑：**可修的 Critical／High 仍為 0**；新增的 1 Critical ＋ 31 High 全部來自 python3 帶進的 Debian 套件（`python3.11` 系列、`libexpat1`、`libsqlite3-0`、`libncursesw6`），皆無上游修復，已逐項具名列入 [infra/images/README.md](../../infra/images/README.md) 的豁免清單，`first_exempted_at` **2026-08-16**、複審日 **2026-11-14**。pip 安裝的 9 個套件在釘選版本上 0 件 Critical／High。<br>**2026-08-16 第三次變更**：映像升 **`2026.08-3`**——修補依賴集缺口。M2 全量重測（[m2/content-baseline-report.md §13.4](m2/content-baseline-report.md)）發現腳本 import 了目錄 `deps` 未宣告的套件；**逐套件靜態掃描 45 個 pin commit 套件樹後，實際短少的是 13 個 Skill、8 個套件**（§13.4 只看到 4 個，因為只有那 4 個在該批 Dataset 上真的走到 `ModuleNotFoundError`）。根因是 `deps` 欄位人工抄漏，映像忠實照抄。處置三段：①`tools/content/seed-skills.json` 的 `deps` 依靜態掃描重新推導；②映像加**准入門檻「純 Python wheel、無編譯擴充、不需系統二進位檔」**，納入 8 個（`pycountry`／`chardet`／`defusedxml`／`ftfy`／`confusable-homoglyphs`／`pytz`／`phonenumbers`／`python-stdnum`），**拒收 `pyarrow`／`reportlab`／`pypdfium2`／`pdf2image`／`pytesseract`／`presidio-*`／`pywin32` 及全部 npm 套件**，理由與失去的能力逐項具名於 [infra/images/README.md](../../infra/images/README.md)「依賴集」節——把 native parser 堆疊放進一個專門解析不受信任資料的沙箱，等於用掉隔離本身的價值；③CONTENT-003／006 補可機械驗證的準則與掃描 finding（見該兩項）。**掃描重跑：各等級件數與 `2026.08-2` 逐格相同、豁免清單無新增列、可修的 Critical／High 仍為 0**——純 Python wheel 沒有 native CVE 面。映像 1.24 GB → 1.35 GB。行為重驗證據見 [infra/images/runtime-agent-sdk/UPGRADES.md](../../infra/images/runtime-agent-sdk/UPGRADES.md)（ADR-023 四項全數在新 digest 上實跑，$0.0202）。<br>**SBX-002 仍不勾的唯一原因**：閘門 A 的節點准入探針要**在真實節點上**查得到這兩份 attestation，屬部署批（SEC-009 前置條件①）。
- [x] SBX-003 實作每個 Run 的獨立環境與暫存空間。（每個 attempt 一個專屬容器，`/work`＋`/out` 為該容器獨有的 tmpfs，不與任何其他 Run 共用可寫路徑；C-01）
- [x] SBX-004 實作非 root、非特權及唯讀基礎檔案系統政策。（`User=65532:65532`、`no-new-privileges:true`、`CapDrop=ALL`、`Privileged=false`、`ReadonlyRootfs=true`；C-02／C-03／C-06／C-08。以真實容器驗證，非僅設定斷言）
- [ ] SBX-005 阻擋容器管理 Socket、主機敏感路徑與內部服務存取。**部分完成**：容器管理 Socket 與主機路徑已擋——`Binds`／`Mounts` 恆空，沙箱內 `/var/run/docker.sock` 不存在，namespace 全為 private（C-04／C-05／C-07，具名整合測試）。**「內部服務存取」未成立**：dev 已由網路面隔離——無出口需求時 `--network none`，有出口需求時只接上 `internal: true` 且只有模型閘道在上面的網路，核心資料庫與物件儲存都不在該網路上（2026-08-16 第四批，SBX-007）。但生產的網路政策隔離與 P-02「Sandbox → 核心資料庫連線嘗試被實際阻擋」的**常駐探針**屬部署期，未實作——該探針的驗收形式見 [ADR-022](../../adr/ADR-022-sandbox-deployment-topology-and-security-thresholds.md) 第三部分**測項 T10**（自 T8 的宣告式稽核獨立出來：其餘 P 區項目拍一次照即可，P-02 明文要求常駐，塞在 T8 裡會讓一次性稽核冒充常駐監控）；節點入池前跑一次且入池後常駐。
- [x] SBX-006 實作 CPU、記憶體、磁碟、程序數與時間限制。（`NanoCPUs`／`Memory`＋`MemorySwap`（不給 swap）／tmpfs size 依 PDM-005 5.2 切 `/work` 3:1 `/out`／`PidsLimit`／`nofile` ulimit／soft 與 hard wall clock；C-10～C-15。逾時強停與 pids 上限以真實容器驗證）
- [ ] SBX-007 實作預設封鎖的網路出口政策及允許清單。**部分完成（2026-08-16 第四批）**：出口路徑已通——沙箱**僅在 `RunRequest.egress.allow` 含 `model_gateway` 時**接上 `internal: true` 的 Docker network，該網路上只有 LiteLLM 閘道可達（允許清單只有一項的最小強制形式）；`allow` 為空維持 `--network none`。方向只有一個：無出口路由的節點只宣告 `egress_modes: ["none"]`，且 `accept()` 會拒絕帶允許清單的請求，不以較弱模式頂替較強請求。物件儲存**刻意不在**該網路上（dev 的匿名／預簽語意會被破壞），位元組由 sandboxd 代搬。**設計已定案（2026-08-16）**：[ADR-022](../../adr/ADR-022-sandbox-deployment-topology-and-security-thresholds.md) Q3 決定沙箱層採 **nftables default-deny ＋節點固定 DNS 解析器**（不部署 Squid／Envoy——MVP 的沙箱層允許清單在 SBX-008／TRACE-002 之後只剩 LiteLLM 閘道一項且為平台自有服務，L7 Proxy 買不到對應的政策需求，且看不見被拒絕的非 HTTP 嘗試），允許清單存於 `infra/egress/allowlist.yaml`（含變更 PR 流程、產品負責人核准、必須連帶更新威脅模型的 CI 斷言、90 天複審、N-07 供應商網域 deny-list），目的地記錄保存 90 天。**已隨 ADR-022 落地的兩件**：①允許清單骨架 `infra/egress/allowlist.yaml`（兩層 `tier`）；②CI 斷言 `.github/workflows/egress-allowlist.yml`＋`check_egress_allowlist.py`——`tier: sandbox` 條目必須恰為一筆 `model_gateway`（否則 fail 並明寫「ADR-022 Q3 重評條件已觸發」）、N-07 供應商網域 deny-list、`pinned_ip` 的 tier 規則、以及「動 `infra/egress/` 必須同 PR 更新威脅模型」。**仍未完成（故不勾）**：生產節點上的 nftables／dnsmasq 本體（強制點在**主機側 `forward`／`DOCKER-USER` 鏈或 Run netns 內**，不是容器內的 `output` 鏈——後者能被逃逸後改掉）、每 Run 網路命名空間與東西向阻擋、目的地記錄管線（N-01～N-07），以及 sandboxd 在 `accept()` 比對 `egress.allow` 與節點已渲染清單、不符即 `capability_mismatch` 拒絕（ADR-022 A1-e），皆屬部署期。**另**：LiteLLM 閘道必須有沙箱面專屬位址、不得是控制平面節點（ADR-022 Q2 強制條件 6；成本 $10.38，由成本試算既有的 Egress Proxy 預算行承載）。**⚠️ ADR-022 另指出 dev 的既有缺口**：同一張 `skillhub_egress` 上的 sandbox 可互相連通，是不需逃逸的跨 Run 橫向路徑，生產形態必須關掉（SEC-009 測項 T5-4）。
- [x] SBX-008 實作 Dataset、Skill、Secrets 與 Artifact 的短效傳遞。（**2026-08-16 第四批結案**。簽發：worker 於 dispatch 以 S3 預簽鑄 `skill_package`／`dataset`（read）與 `artifact_upload`（write）三類 `ObjectGrant`，TTL＝該 Run 硬牆鐘＋5 分鐘；Virtual Key 經 LiteLLM `/key/generate` 鑄出，帶 `max_budget`＋`tpm_limit` 雙限並限定模型層（PDM-003 v5），以 `ANTHROPIC_BASE_URL`／`ANTHROPIC_AUTH_TOKEN` 注入。**簽發失敗即不派送**（fail-closed）。撤銷：cleanup 以 alias（`skillhub-attempt-<run_attempt_id>`）呼叫 `/key/delete`，冪等、不需保存金鑰本身、重啟後仍撤得掉；撤銷失敗記 `cleanup_status='failed'` 並重試（D-03／D-06、SEC-005）。傳遞：**沙箱不再持有任何預簽 URL**——sandboxd 用 grant 取得位元組後以 `docker exec` 寫進容器（`docker cp` 對唯讀 rootfs 被 daemon 拒絕，實測），Artifact 由 sandboxd 讀出 `/out/artifacts`、套用 PDM-005 5.2 的單檔／總量上限後以寫入授權上傳，manifest 回填 `RunResult.artifacts`。收集在 workload 退出**前**完成（`/out` 是 tmpfs，行程一結束即消失），以 `.workload-done`／`.collected` 交接。全鏈已由真實端到端 Run 驗證。**已知簡化**：一個 attempt 一個 tar 封存（預簽是逐物件的，平台無法預知檔名）；逐檔物件需 POST policy 前綴授權，留給 PACK-001。）
- [x] SBX-009 實作完成、失敗、取消與逾時後清理。（四條終態路徑都會走到 `DELETE`；`DELETE` 冪等、無 404、不存在也回 204，釋放失敗回 500 供平台記錄 `cleanup_status` 並重試；X-01。以真實容器驗證重複 destroy 與容器確實消失）
- [ ] SBX-010 進行隔離、資源耗盡、網路與清理失敗測試。**屬部署期驗收，不在本批**：逃逸測試與 gVisor 相容性需要 Linux 與巢狀虛擬化（ADR-019 待決策 3）。本批已有的是 ADR-005 基線的真實容器驗證（非 root、唯讀 rootfs、無主機掛載、pids 上限、逾時強停、清理冪等），不等於逃逸測試通過。**SEC-009／SBX-010 未通過不得開放外部使用者提交 Skill 執行**（ADR-015 定案紀錄）。
- [x] SBX-011 把 Runtime Image 發佈流水線接上 **GHCR**，並把 SBOM 與掃描結果作為 attestation 隨 image digest 保存。（**2026-08-16 完成**。[`.github/workflows/runtime-image.yml`](../../.github/workflows/runtime-image.yml)：main 上 `infra/images/**` 變更時，在既有四道閘門（digest 斷言 → build → syft SBOM → grype → I-06 門檻）**全部通過之後**才 push 至 `ghcr.io/arthurc02/skillhub-runtime-agent-sdk`（tag 取自 Dockerfile 的 `ARG IMAGE_VERSION` 與 commit SHA，避免版本寫在兩個地方漂移），再以 `actions/attest-sbom` 與 `actions/attest` 把 **SPDX SBOM（I-03）** 與 **in-toto vulns predicate（I-04，含 `scanned_at` 與 `fixable_critical_high`）** 掛到**推上去的 digest**，`push-to-registry: true` 使其成為 OCI referrer。順序即政策：過不了 I-06 的 image 到不了 registry。<br>**選 GitHub artifact attestations 而非 cosign**（ADR-022 兩者皆點名）：兩者產出的都是 Sigstore 簽章的 in-toto statement、都以 referrer 存在 digest 旁，差別在維護成本——Actions OIDC 的 keyless 簽章沒有金鑰要保管輪替，驗證端 `gh attestation verify` 不必先裝東西；cosign 的價值在「簽章發生在 Actions 以外」或「registry 不是 GHCR」，而 ADR-022 選 GHCR 正因 CI 本來就在這裡。理由記於 workflow 註解與 [infra/images/README.md](../../infra/images/README.md)。<br>**權限最小化**：workflow 預設 `contents: read`；`review` job 才加 `packages`／`id-token`／`attestations`（三者只有底部的發佈步驟用得到，且該步驟 `if: github.event_name == 'push'`，fork PR 的 token 本來就是唯讀）；`rescan` job 只要 `packages: read`。<br>**另含定期重掃**：`rescan` job（每週 cron ＋ `workflow_dispatch`）拉 **GHCR 上已發佈的那個 digest** 重掃並重新 attest `scanned_at`——不是當場重建一個 image，那會是另一組位元組、回答不了「生產上跑的那個還乾淨嗎」。這補上 `infra/images/README.md` 原記為「目前做不到」的那一項，也讓 I-04 的 30 天有效期首次可被自動判定。<br>**未做且明說**：到期前 7 天告警的**發送端**未接（判定材料已存在於 attestation，讀它並叫人是閘門 A 探針的工作，屬部署批）。<br>**已發佈並驗證**（2026-08-16）：首發後修了兩件事——①`scan_predicate.sh` 的執行位元在 Windows 上沒進 git index，runner 以 exit 126 死在 I-04 那步（修法：index `+x` ＋ 呼叫端加 `bash` 前綴，兩者並用，因為任一單獨都可能被另一個 OS 的 re-add 靜默還原）；②path filter 排除 `infra/images/**/*.md` 並納入 `scan_predicate.sh`——**build 不是位元可重現的，所以每一次跑到發佈步驟都會產生新 digest 並把版本 tag 移過去**，一次純文件編輯就會孤立前一個 digest。孤兒清單與刪除方式記於 [infra/images/README.md](../../infra/images/README.md)（**刪除需 `delete:packages` scope，本機 token 沒有，留給負責人**）。）
- [x] SBX-013 落地 PDM-005 §5.2a 的 token 硬上限（[m2/README.md 乙-2](m2/README.md) 的 **(a)**）。（**2026-08-16 新增並完成**。乙-2 的兩個選項裡取 (a)「強制」而非 (b)「從權限摘要移除」：那個數字已經被使用者確認過，拿掉它等於承認確認畫面在演戲。<br>**機制裁定：強制點在 harness，不在 Go worker。** PDM-005 §5.2a 原本指定 worker 依閘道回報的 `input_tokens` 累加，那按字面做不出來——`RunResult.usage` 不回傳 token（契約 `eab9d26` 已誠實記載），從 Trace 事後重建則發生在錢花完之後。**唯一看得到每次回應 token 數的地方是沙箱內的 harness**，所以由它累計並在跨越上限時優雅中止：`break` 出 SDK 訊息迴圈（不再有下一次模型呼叫）→ 發 `error` 事件（`code: token_budget_exceeded`，NFR-003 的穩定診斷碼）→ 發終態 `usage` 事件 → 以 exit code **9** 結束。sandboxd 認得該 code，回報 `completed`＋`status: failed` 並附可讀的 `RunError.message`（不是「workload exited with code 9」），因此 artifact 與 trace 尾巴照常收集。<br>**值域同源**：`SKILLHUB_MAX_INPUT_TOKENS`／`_OUTPUT_TOKENS` 由 `dockerdrv.env()` 從 `RunRequest.ResourceLimits.TokenBudget` 產生，而該欄位來自該 Run 凍結的 `policy_snapshot`——與執行前權限摘要讀的是同一份物件（`internal/run.defaultPolicy`），所以中止用的上限就是使用者確認的上限（`02:TEST-005`）。Provider 現在也**宣告**這項能力（`DefaultLimits.TokenBudget`）並在 `accept()` 拒絕超過它能強制的 token 上限——宣告了才可以被派工，這正是契約要求「不能強制就不要宣告」的另一半。<br>**failure_class 裁定：不擴充 `0018` 的 CHECK，不新增 migration。** 理由三條：①該值域是為 retry policy 定義的（`0018` 註解明寫），而 token 上限中止的 retry 語意與 `workload_error` 完全相同（重試只會燒同樣的 token 得到同樣結果）；②要讓平台分辨它，必須讓沙箱回報一個新的 `RunError.class`，那是 `contracts/openapi/sandbox-provider.yaml` 的 enum，屬契約批範圍，在那之前新增一個沒有任何寫入者的值域等於加一個永遠是 NULL 的值；③精確原因已有位置——trace 的 `error` 事件 `code` 與終態 `usage` 的 token 數，schema 明文「Detail lives in the payload」。要在 `runs.failure_class` 層級分辨，先做契約側的 additive 變更再談 migration。<br>**誠實界線**：這是**合作式**停止，與 `wall_clock_soft_seconds` 同一類，擋的是失控的 agent 迴圈；非合作式的煞車仍是沙箱外的 Virtual Key `max_budget`／`tpm_limit` 與硬牆鐘。已跨越上限的那一次回應已經付過錢，中止擋的是下一次。<br>**證據**：真容器＋真 Agent SDK＋真閘道呼叫，`services/sandbox/internal/dockerdrv/harness_token_e2e_test.go`（環境變數開關，CI 不跑）。實跑：上限設 1 token → 第一次回應（16,408 input）即中止，`RunError` 為 token ceiling 訊息、trace 依序為 `agent_output` → `error(token_budget_exceeded)` → `usage(token_source: accumulated, cost_source: gateway, cost_usd 0.00205215)`，樣本存 `contracts/events/samples/harness-usage-sample.jsonl`）
- [x] SBX-012 實作 Reconciler 的 **in-flight orphan 表**（`provider_run_id → first_seen_round`）與**分項失敗計數器**（`gateway_revoke_failed`／`sandbox_destroy_failed`）。（**2026-08-16 完成**：migration `0021_reconciler_orphan_sightings`（`(provider, provider_run_id)` 主鍵、`first_seen_at`／`last_seen_at`／`rounds`）＋ `internal/run/cleanup.go` 的每輪 upsert 與剪除（本輪沒看到就刪列，所以 `rounds >= 2` 的語意是**連續**兩輪而不是「總共見過兩次」），新指標 `skillhub_orphan_sandbox_persistent{provider}`（gauge）、`skillhub_gateway_revoke_failed_total`、`skillhub_sandbox_destroy_failed_total{provider}`。用資料表而非行程內 map，是因為 Reconciler 是 River 的 leader-only periodic job：leader 會換行程、行程會重啟，而「洩漏活得比 worker 久」正是這個門檻要抓的情況。另修一個既有行為：`scanProvider` 原本一筆 destroy 失敗就中止整輪，導致第二筆洩漏連被看見都沒有——現在逐筆記錄後續掃完，錯誤合併回傳。整合測試 `TestOrphanSightingsCountConsecutiveRoundsNotTotalFailures` 斷言「一輪不算、同一筆連兩輪才算、另一筆新出現不加總、清掉後自動歸零」。`infra/observability/alerts.yml` 的 `LeakedSandboxStillPresent` 與 `CredentialRevokeFailing` **已升為正式形式**（改讀新指標，移除過渡註解），`LeakedSandboxDestroyFailed` 併入清理路徑的分項計數器，observability README 同步。順帶依封測容量校準 `CleanupBacklogGrowing`：`> 5` → `> 2`——舊值大於整池 4 個 slot，代表整池沙箱全洩漏都還低於門檻，新值取 4 slot 的 50%，與 ADR-022 X-04 單節點 drain 同一個比例；`for: 15m` 不變）

## 12. Local Runner Beta（後 MVP，依 M2 後需求訊號啟動）

- [ ] LOCAL-001 定義 Local Runner 與 Skill Hub 的信任及配對流程。
- [ ] LOCAL-002 實作 Runner 能力、作業系統與版本回報。
- [ ] LOCAL-003 實作本機工具路徑、參數、工作目錄與存取範圍預覽。
- [ ] LOCAL-004 實作每次執行的明確使用者確認。
- [ ] LOCAL-005 實作執行、事件串流、取消、逾時與結果回傳。
- [ ] LOCAL-006 確保未授權的本機檔案與工具不被上傳。
- [ ] LOCAL-007 遮罩 Runner Log、Trace 與錯誤中的 Secrets。
- [ ] LOCAL-008 進行路徑穿越、命令參數、符號連結及權限邊界測試。

## 13. Trace 與平台 O11y（M2）

> **2026-08-16 第三批（Trace 收集管線與 O11y）交付摘要**：事件流向為「容器內 harness 寫 JSONL 到 `/out` → sandboxd 讀出並 POST 到平台 ingestion → 平台遮罩後入庫」。沙箱無網路（`--network none`），執行平面不碰核心 DB（鐵律 2），ingestion 憑證是嵌在 `TracePolicy.ingestion_url` 內的每 attempt 短效簽章 token，未新增契約欄位。migration `0019_trace_ingestion.sql` 補齊 TRACE-001 §8 的四個缺口並加上 `late` 與 default partition。詳見 [m2/README.md 第三批交付摘要](m2/README.md)。

- [x] TRACE-001 定義標準 Trace Event Schema。（`contracts/openapi` 外的第一份事件契約：[contracts/events/trace-event.schema.json](../../contracts/events/trace-event.schema.json) ＋ 同目錄 README；9 種事件、`seq` 範圍為 `(run_id, attempt, emitted_by)`、`run_lifecycle` 非事實來源、遮罩規約與版本演進規則。**2026-08-16 由第三批勾選**：schema 本身在第一批完成，但當時記錄的四個 `trace_events` 欄位缺口（`event_id`／`attempt`／`schema_version`／遮罩狀態）使契約無法落地，`0019` 全數關閉後才符合「Trace 至少包含時間、事件類型、來源、狀態與關聯 Run」與「順序可被重建、能識別缺失」的允收）
- [x] TRACE-002 收集 Skill 啟用與資源讀取事件。（`infra/images/runtime-agent-sdk/run.mjs` 由 Agent SDK 訊息流產生 `skill_activation`（`Skill` 工具呼叫）與 `resource_read`（讀取路徑落在 `SKILLHUB_SKILL_DIR` 下者，路徑相對化不外洩沙箱絕對路徑）；收集端見 `services/sandbox/internal/sandbox/trace.go`。**限制註記**：`decision: skipped`「可用但未啟用」在 SDK 訊息流中不可觀測（沒被叫到就沒有事件），本批不臆造，留給 EVAL-002）
- [x] TRACE-003 收集 Tool Call、MCP Call 與 Script Log。（`tool_call` 為單事件自帶 `duration_ms`，以 `tool_use`／`tool_result` 配對計時；`Bash` 的結果另產 `script_log`。**MCP 不實作**——遠端 MCP 已移出 MVP 首發，契約只保留型別佔位，與 TEST-005/006 同步）
- [x] TRACE-004 收集 Agent 輸出、錯誤、延遲、Token 與成本。（`agent_output`（final／intermediate）、`error`（含控制平面自身的失敗）、延遲（`tool_call.duration_ms`、`usage.duration_ms`）、Token（SDK 回報的 `input_tokens`／`output_tokens`）；orchestrator 側在 `failed`／`timed_out` 轉移的同一交易內寫 `error` 事件，因此未進到沙箱就失敗的 Run 也有可看的時間軸（RUN-004）。**2026-08-16 第四批補上成本**：`model_gateway` 授權簽發後，harness 以**自己的 Virtual Key 讀閘道 `/key/info` 的 per-key spend**填 `cost_usd`＋`cost_source: gateway`——刻意不採 SDK 的 `total_cost_usd`，那是本地價目表對閘道解析出的模型名所做的估算，掛 `gateway` 會是假的。閘道 spend 為非同步 flush，故先等 2.5 秒再讀並讀到數值穩定；實測與閘道 `LiteLLM_SpendLogs` 逐筆合計一致（`0.00571125`，見 [m2/README.md 第四批](m2/README.md)）。讀不到時仍誠實留 `null` 並顯示「未回報」。**限制註記**：這是讀數不是帳本，最後一次 flush 若落在最後一次輪詢之後會少算，權威對帳來源是閘道的 per-key spend（ADR-017）。**2026-08-16 補正**：當時 `usage` 只掛在 SDK 的 `result` 分支，沒有 `result` 的那一輪整筆消失——見 **TRACE-009**，已修，本項的允收不受影響（成本欄位仍在，只是現在每個 Run 都有）。）
- [x] TRACE-005 實作 Secrets 與敏感欄位遮罩。（遮罩在平台 ingestion **入庫前**執行，明文不落 DB；`0019` 的 `CHECK (masked)` 讓「跳過遮罩」在資料庫層不可能。已知值（本 Run 的 ingestion token）＋ pattern（`sk-*`、`Authorization` 標頭、`ANTHROPIC_AUTH_TOKEN=`／`OPENAI_API_KEY=`、預簽 URL 的簽章參數、私鑰區塊）一律替換為字面量 `[REDACTED]`，不保留長度、不做部分遮罩；`masked_fields` 以 JSON Pointer 記錄實際被替換的位置。有具名單元測試與整合測試，後者直接查 `trace_events.payload` 確認明文不存在）
- [x] TRACE-006 實作一般模式進度摘要。（`GET /runs/{id}/trace`：Skill 啟用、資源讀取次數、工具呼叫統計與最慢者、錯誤、最終輸出、Token 與成本。**Run 狀態取自 `runs` 表、進度步驟取自 `run_status_transitions`**，不重播 `run_lifecycle` 事件重建狀態（鐵律 5）；`cost_usd` 為 `null` 時顯示「未回報」不顯示 0）
- [x] TRACE-007 實作進階模式 Trace 檢視。（`?mode=advanced`：經遮罩的原始事件依 `(occurred_at, emitted_by, seq)` 重建順序，並逐串流列出 `missing_seq` 與遲到計數；`complete: false` 時 UI 明示「部分事件未送達」。payload 一律以 inert text 呈現，不解讀 HTML／ANSI／SVG，有具名測試以注入 `<img onerror>` 驗證。UI 為 `apps/web` 的 `/runs/$runId`，一般／進階切換）
- [x] TRACE-008 處理事件排序、重送、缺失與延遲。（去重鍵為 producer 產生的 `event_id`，`ON CONFLICT DO NOTHING`，重送回報為 `duplicate` 而非錯誤；順序以 per-producer 的 `seq` 重建，斷號＝該事件遺失且被逐一列出；終態後仍接受遲到事件並標記 `late`，因為沙箱關機時推送的最後一批正是失敗 Run 最需要的部分。sandboxd 側推送失敗不推進水位，下一輪重送）
- [x] TRACE-009 讓 `usage` 事件不再依賴 SDK 的 `result` 訊息（[m2/README.md 丙-3](m2/README.md)、[content-baseline-report.md §7.2 #4](m2/content-baseline-report.md)）。（**2026-08-16 新增並完成**。原本 `usage` 只在 `result` 分支發出，串流以任何其他方式結束就**完全不發**——不是發 0，是不發。實例 `add-iso3166`：Run 成功、產出 artifact、13 個事件無斷號，token 與成本在平台端不存在；45 個 Run 合計 Trace $3.0879 對閘道實付 $3.3932，EVAL-012 直接加總會系統性低估。<br>**修法**：harness 逐則訊息累計每次模型回應的 token，run 結束時**一律**發一筆 run 級 `usage`——有 `result` 就以其為準（`token_source: result`），沒有就發累計值（`token_source: accumulated`），崩潰路徑與 token 上限中止路徑都會發。`cost_usd` 兩條路徑都照舊向閘道要（成本來源與 SDK 有沒有給 result 無關），所以 `cost_source` 維持 `gateway`。<br>**契約**：`token_source` 為 additive optional 欄位，事件 schema 升 **1.1**（`contracts/events/trace-event.schema.json` ＋ README 版本紀錄）；1.0 事件仍是合法的 1.1 事件，只寫 1.0 欄位的 producer（控制平面）繼續宣告 1.0。**刻意不塞進 `cost_source`**：那個欄位答的是「錢的來源」，讓它兼答「token 的來源」會在兩件事上同時說謊。<br>**實測推翻的假設**（pinned SDK 0.3.233，與 `settingSources` 同型的靜默失效）：assistant 訊息的 `message.usage` **四個欄位恆為 0**，且同一個 `message.id` 會出現多次；唯一有真實數字的是 `stream_event` 的 `message_start`／`message_delta`，因此 query 必須開 `includePartialMessages: true`——關掉它所有計數都讀 0 且不會報錯。<br>**證據**：`harness_token_e2e_test.go` 兩個真容器測試；累計值與 SDK `result` 總計在同一組實跑中逐數字相同（16,408 input）。留在原位的限制：累計值是**下界**（串流結束時仍在途中的回應不在內），權威對帳來源仍是閘道 per-key spend（ADR-017））
- [x] O11Y-001 量測搜尋、Run 排隊、建立、成功、逾時與清理指標。（Prometheus 文字格式，`services/platform` 與 `sandboxd` 各自曝露 `/metrics`；平台側走獨立 listener `METRICS_ADDR`，不掛在對外 API port 上。指標清單見 [infra/observability/README.md](../../infra/observability/README.md)）
- [x] O11Y-002 建立 Provider 健康度與錯誤監控。（`skillhub_provider_capability_total{provider,result}` 區分 ok／unhealthy／error；`skillhub_provider_request_total{provider,operation,class}` 以狀態碼分級記 429 與 5xx；另有每操作延遲 histogram。告警規則四條見 `alerts.yml`）
- [x] O11Y-003 建立遺留 Sandbox、資源異常及安全事件告警。（`skillhub_run_cleanup_backlog` gauge、`skillhub_orphan_scan_total`、`skillhub_orphan_sandbox_total{action}` 區分「殺掉的漏網 Sandbox」與「殺不掉的」，加上遮罩器靜默失效偵測（`TraceMaskingStopped`——NFR-002 沒有其他偵測器）。告警為 `infra/observability/alerts.yml` 的 rules 檔＋文件，**Alertmanager 部署、通知路由與 Grafana dashboard 屬部署期，明確未做**；門檻值為首發預設非實測校準值，已在文件標明需上線後回填）

## 14. 評估與改善（M3）

- [ ] EVAL-001 將驗收條件轉換為可執行或可判斷的檢查。
- [ ] EVAL-002 實作規格、啟用、執行、效果、相容與成本分類。
- [ ] EVAL-003 對每項驗收條件產生通過、未通過或無法判斷及證據。
- [ ] EVAL-004 產生符合、部分符合、未符合或無法判斷的整體結果。
- [ ] EVAL-005 清楚標示規則判斷、模型判斷與使用者判斷。
- [ ] EVAL-006 實作有幫助／無幫助及文字回饋。
- [ ] EVAL-007 產生包含問題、證據、位置、修改與影響的改善建議。
- [ ] EVAL-008 區分 Skill、Runtime、MCP、工具及測試資料問題。
- [ ] EVAL-009 實作逐項接受／拒絕與修改差異預覽。
- [ ] EVAL-010 套用改善時建立新 Skill Version。
- [ ] EVAL-011 實作使用相同 Test Case 重新試跑。
- [ ] EVAL-012 實作版本、驗收、輸出、錯誤、延遲與成本比較。

## 15. 打包與下載（M4）

- [ ] PACK-001 建立標準 Agent Skill 打包流程。
- [ ] PACK-002 打包前重新執行規格驗證。
- [ ] PACK-003 保留 License、作者、原始來源與衍生關係。
- [ ] PACK-004 排除 Secrets、測試憑證、內部路徑與受保護資料。
- [ ] PACK-005 支援選擇是否包含可散布 Test Case 與範例資料。
- [ ] PACK-006 建立首批目標 Agent 安裝 Profile。
- [ ] PACK-007 產生安裝位置、依賴、環境變數與驗證步驟。
- [ ] PACK-008 對未驗證 Agent 顯示清楚限制。
- [ ] PACK-009 驗證下載套件可被解壓、重新驗證與依說明安裝。

## 16. 安全、隱私與合規（M0 起跨階段）

> **2026-08-15 補齊允收準則**：本節十項原本只有一行敘述、`02` 無 SEC-* 需求 ID（威脅模型開放問題 **Q19**）。允收準則已補於 **`02` 第 6 節「安全需求」**，取自 [m0/threat-model-and-sandbox-baseline.md](m0/threat-model-and-sandbox-baseline.md) v2 已落地的 32 條威脅與 45 項基線檢查，未新增安全要求；該文件未定的門檻值在 `02` 標為「未涵蓋（待決策）」。

- [ ] SEC-001 完成 Skill、Script、MCP、Dataset、Secrets 與 Local Runner 威脅模型。（允收：`02` §6）
- [ ] SEC-002 定義 Sandbox 最低安全基線與阻擋條件。（允收：`02` §6）**部分完成**：45 項基線、四閘門與 fail-closed 語意已定（威脅模型 v2 §4～§5）；**六項無值語句已於 2026-08-16 由 [ADR-022](../../adr/ADR-022-sandbox-deployment-topology-and-security-thresholds.md) 第二部分定值**（P-03 7 天／P-04 N·N−1 且 ≤90 天＋逃逸類 CVE 24 h／I-04 30 天／I-06 可修的 Critical·High 阻擋無豁免／X-02 每 5 分鐘／X-03 連 2 輪告警、X-04 單節點 ≥50% drain 與全池 ≥25% 下限 2 筆暫停），**勾選前提 Q1～Q3 亦已定案**（同 ADR 第一部分）。**2026-08-16 更新：閘門 B 的四項額外阻擋已全數落地**（`internal/run/gateb.go`）——靜態掃描等級判斷改為在 Create 重新掃描套件並套用 `skillpkg` 既有的 severity 政策（`error` 級 ＝ `SKILL-002` 的阻擋級），掃不成即拒（fail-closed，SEC-002「檢查無法執行視同未通過」），因此不必等威脅模型 Q7；Workspace 並行上限 ＝ 2 以交易內的 advisory lock ＋ 非終態計數強制。**仍不勾的唯一原因**：45 項基線尚未經 SEC-009 全數驗證。

- [ ] SEC-003 建立 Skill 匯入與執行前靜態掃描政策。（允收：`02` §6）**2026-08-16**：威脅模型 **Q15**（匯入 SSRF 的歸屬）已裁定為**擴充本需求**，`02:SEC-003` 已補上抓取器防護的允收準則；實作由 `INGEST-014` 承接。**本項仍不勾的原因不變**：每類掃描發現對應的等級（阻擋／警告／資訊）尚未定案（威脅模型 **Q7**），`SEC-002` 閘門 B 的「靜態掃描結果為阻擋」因此仍不可判定。
- [ ] SEC-004 建立遠端 MCP 的 SSRF、內部網路與資料外洩防護。（後 MVP，隨 MCP 啟動）（允收：`02` §6）
- [ ] SEC-005 建立 Secrets 儲存、短效注入、遮罩與撤銷流程。（允收：`02` §6）
- [ ] SEC-006 建立 Dataset、Trace 與 Artifact 保存及刪除政策。（允收：`02` §6）
- [ ] SEC-007 建立來源、License 與下架處理政策。（允收：`02` §6）
- [ ] SEC-008 驗證使用者與 Run 之間的資料隔離。（允收：`02` §6）
- [ ] SEC-009 進行 Sandbox 逃逸、資源濫用與權限提升測試。（允收：`02` §6）**驗收程序已定案、測試未執行**：[ADR-022](../../adr/ADR-022-sandbox-deployment-topology-and-security-thresholds.md) 第三部分是部署批實際照著跑的那份清單，內容為 **10 個測項**——T1 逃逸 PoC 集／T2 syscall 煙霧 fuzz／T3 資源耗盡／T4 Runtime 相容性（45 個精選 Skill 在 `runsc` 上重跑，約 $3.4）／**T5 網路外洩八子項**（DNS tunneling、內網掃描、Metadata Service、**東西向**、**節點面**、N-07 供應商網域、**T5-7 閘道 `pinned_ip` 的非允許 port**、**T5-8 自外部掃節點 port**）／T6 憑證範圍／**T7 清理失敗與遺留門檻（含人工注入假遺留資源——否則 X-04 的 drain 與暫停路徑沒有任何測項會執行到）**／T8 宣告式供應鏈稽核／**T9 Provider 契約（加 I-05 與 `egress.allow` 不符須回 `capability_mismatch` 兩項斷言）**／**T10 P-02 常駐探針**（入池前一次＋入池後常駐）。<br>**分兩個 suite**：Suite 1 可在一般 Linux runner 跑（**gVisor 的 `systrap` 平台不需巢狀虛擬化**，只有 `--platform=kvm` 才需要——ADR-019 待決策 3 的範圍因此縮小，**待部署批第一台節點實測確認**）；Suite 2 需生產同規格節點。<br>**前置條件三條（缺一即判 `unknown` ＝ fail）**：①受測 Runtime Image 已發佈至 **GHCR 且附 SBOM 與掃描 attestation**（SBX-011）；②`infra/nodes/gvisor-baseline.txt` 已填實際版本（非 `unset`）；③`infra/egress/allowlist.yaml` 的 `tier: sandbox` 條目已填實際 `pinned_ip`，且該位址**不是控制平面節點**（Q2 強制條件 6）。<br>**通過判準**：45 項全數 pass、**0 項 unknown**，任一 fail 或 unknown 即不得開放外部使用者提交 Skill 執行，**無例外流程**。<br>**證據**：`plans/mvp/m4/sec-009-acceptance/<日期>-<節點>/`（判定表與 `versions.txt` 進 repo，原始輸出留 CI artifact 並附連結），保存 ≥ 1 年。
- [ ] SEC-010 完成安全事件回應與緊急停用 Provider 流程。（允收：`02` §6）**2026-08-16**：未定的三項（嚴重度分級、誰接最高級告警、遮罩失敗補救程序）已寫成**提案**置於 `02:SEC-010` 的待決策區——P1／P2／P3 分級表（P1 的第一動作為自動停止派送，不等人）、GitHub issue ＋ email 的通知路徑（承認單人團隊、不採購值班輪替系統）、遮罩失敗 runbook（撤銷 → 清除 → 通知 → 事後，含必補回歸測試）。**2026-08-16 同日經負責人核可（`934db2b`）**，該提案自即日起為本需求的允收準則（威脅模型 **Q17** 一併結案；值班輪替待團隊有第二人時重開）。**本項仍不勾的原因**：核可的是分級與回應方式，**一鍵停用流程本體、runbook 文件與 P1 的自動第一動作都尚未產出**——自動動作由新增的 `SEC-012` 承接，runbook 與一鍵停用流程仍在本項。
- [ ] SEC-011 實作平台 operator 角色與跨 Workspace 下架權限。（**2026-08-16 新增**，允收：`02:SEC-011`）**2026-08-16 追加：授權受限展示已落地**（`02:SEC-011` 的追加小節）——負責人裁定對 `anthropics/skills` 的 4 筆（`docx`／`pdf`／`pptx`／`xlsx`）執行 [m2/anthropic-sa-license-memo.md](m2/anthropic-sa-license-memo.md) 方案 C：migration **`0023`** 的 `skills.access_restriction`（NULL ＝ 無限制，非 NULL ＝ 原因碼，現行只有 `license-review`），`/files` 回 403 附理由、Run 建立回 422、詳情頁顯示受限說明並收起進階連結、**搜尋與摘要照常**；旗標於 Fork 當下複製，未知原因碼 fail-closed 視為受限。逐筆設定見 `tools/content/restrict-anthropic-sa-display.sql`，測試見 `disc_integration_test.go` 的 `TestLicensingHoldClosesTheMaterialsAndKeepsTheListing`／`TestARunOnHeldMaterialsIsRefused` 與 `disc.test.tsx`。動作窮舉為三項——變更目錄項可見性、停用 Skill Version、白名單異動；**operator 不得讀取任何 Workspace 私有資料、不得代表使用者發起 Run、不得改寫既有 Skill Version 與歷史 Run**（最小權力原則）。每個動作與角色授予本身都寫 audit event，理由為必填；`member` 呼叫 operator 端點回 404。下架流程與 `CONTENT-009`／`INGEST-010` 共用同一組狀態，不另開第二套。<br>**2026-08-16 部分完成——最小操作流程已落地**：受限旗標從「只能跑 SQL」升級為有端點、有授權、有稽核、有測試的操作。<br>**(a) operator 身分**：部署設定 `OPERATOR_USER_IDS`（逗號分隔 user id）＋ `identity.RequireOperator` middleware；不在清單內（含未登入）一律 **404**，不是 401 也不是 403——照 `02:SEC-011`「不揭露端點存在」。**刻意不做 DB 角色表**：單人團隊裡，角色表要連帶做授予端點、該端點自己的授權、以及授予的稽核，三者存在的唯一目的是讓同一個帳號提拔自己；授予＝改設定重啟，不會部署的人本來就做不到。第二人出現時的升級路徑是「這裡改讀角色表」，路由與 handler 不動。<br>**(b) 授予入稽核（最小形式，限制寫明）**：`cmd/api` 每次啟動寫一筆 `operator.roster` audit event（生效清單＋筆數）。它答的是「誰現在是 operator」，**不是「誰在何時授予」**——後者的事實在部署設定的變更歷史裡。寫不成這筆事件時該次啟動**不承認任何 operator**（fail-closed）。<br>**(c) 端點**：`PUT`／`DELETE /admin/skills/{id}/restriction`。PUT body 需 `reason`（必須是平台認得的原因碼，目前只有 `license-review`——**未知碼在寫入端擋掉，在讀取端仍 fail-closed 視為受限**，兩條規則方向相反且都需要）＋ `note`（必填）；DELETE 需 `note`。兩者皆冪等（重設回 200 並回報前一狀態、解除不存在的限制回 204），且**兩種情況都照樣寫 audit event**——操作者做了這個動作本身就是要留的事實。audit metadata 為 `before`／`after`／`note`，actor 取自 session，與欄位變更**同一交易**（鐵律 9）。<br>**(d) 唯一的跨 Workspace 寫入**：`LockSkillForRestriction`／`SetSkillAccessRestriction` 是全樹僅有的兩條不帶 workspace 述詞的 skills 語句（授權事實屬於來源，必須同時涵蓋目錄項與其 fork）；範圍窄到只有一個欄位、一列、以主鍵定址，不回傳任何套件內容，且只有 operator 路由到得了。整合測試 `operator_integration_test.go` 覆蓋：非 operator（匿名與 member）全 404 且資料未變、operator 設定後 `/files` 立即 403 而搜尋與詳情照常、audit 事件的 before／after／note、解除後 `/files` 回復 200、兩個方向的冪等，以及**operator 身分不擴充 Workspace Scope**（讀他人私有 Skill 仍 404）。<br>**仍不勾的原因（三項）**：①窮舉的動作只落地 ①的較輕等級「授權受限展示」，②停用 Skill Version 與 ③白名單異動**未實作**（兩者都還沒有可驅動的機制，先開端點只是承諾）；②授予的稽核只到「清單」粒度，不到「誰在何時授予」；③`contracts/openapi/public.yaml` 尚未宣告這兩個路由（additive，待契約批，同 `TEST-011` 前例）。**`apps/web` 不做管理 UI**（裁定：單人以 curl 操作，範例見 `02:SEC-011` 追加小節）。
- [ ] SEC-012 實作 P1 安全事件的**自動第一動作**：偵測到 P1 判準即**立即停止派送新 Run**（一鍵停用 `SelfHostedProvider`）、drain 受影響節點、保留現場，不等人接告警。（**2026-08-16 新增**，承接 `02:SEC-010` 分級表**經負責人核可後**（`934db2b`）成為允收準則的「自動動作（不等人）」欄——核可前它只是提案，核可後它是一條沒有任何工作項承接的準則，因此在此立項。<br>**P1 判準（取自 `02:SEC-010` 分級表，不在此另訂）**：逃逸疑慮；P-02 探針偵測到 Sandbox → 核心資料庫連線；`TraceMaskingStopped`；隔離技術逃逸類 CVE 揭露；Reconciler 停擺 > 10 分鐘。分級的唯一升級規則同樣取自該表——**不確定是 P1 或 P2 時一律以 P1 處理**。<br>**與 X-04 暫停機制同源（這是本項的實作要求，不是註記）**：ADR-022 X-04 的「單節點 ≥50% slot drain／全池 ≥25%（下限 2 筆）暫停整池派送」與 P1 的「停止派送」是**同一個開關的兩個觸發者**，必須共用同一份狀態與同一條解除路徑。做成兩套的後果是可預期的：一邊暫停、另一邊以為自己還在派送，而「現在到底有沒有在派送」會變成沒有單一答案的問題。<br>**落地時機：部署批之前。** 理由是這條準則的價值全在「不等人」——`SEC-009` 通過、開放外部使用者提交 Skill 執行的那一刻，P1 若仍需要人手動停派送，`02:SEC-010` 的設計前提（團隊是一個人、等人接的最高級告警等於沒有 SLA）就直接落空。<br>**必須同時可測**：暫停與解除各要有一個具名測試，且解除**不得**是自動的（自動解除等於讓觸發條件自己決定何時恢復服務）；`SEC-009` 測項 **T7** 已要求人工注入假遺留資源來執行 X-04 的 drain 與暫停路徑，本項的開關應由同一個測項覆蓋，不另立驗收。<br>**未實作**）（允收：`02:SEC-010`）

## 17. 品質保證與封閉測試（M4）

- [ ] QA-001 建立核心旅程端到端測試。
- [ ] QA-002 建立 Agent Skills 規格驗證測試資料集。
- [ ] QA-003 建立正常、失敗、逾時、取消與清理 Run 測試。
- [ ] QA-004 建立 Dataset 與 Secrets 權限測試（MCP、Local Runner 部分隨後 MVP 啟動）。
- [ ] QA-005 建立 Trace 完整性與敏感資訊遮罩測試。
- [ ] QA-006 建立評估可解釋性與改善版本不可變測試。
- [ ] QA-007 建立打包、License、來源及 Secrets 排除測試。
- [ ] QA-008 完成主要瀏覽器與目標作業系統測試。
- [ ] QA-009 完成鍵盤操作、文字標籤與色彩以外狀態提示檢查。
- [ ] BETA-001 招募目標個人創作者進行封閉測試。
- [ ] BETA-002 量測搜尋到詳情、詳情到試跑、試跑到下載的漏斗。
- [ ] BETA-003 蒐集評估報告與改善建議的質性回饋。
- [ ] BETA-004 記錄所有阻斷首次成功旅程的問題。
- [ ] BETA-005 依封閉測試結果完成 MVP 範圍及優先級複審。

## 18. MVP 發佈檢查（M4）

- [ ] RELEASE-001 所有 MVP 必要需求都有對應測試與結果。
- [ ] RELEASE-002 完整核心旅程通過端到端驗證。
- [ ] RELEASE-003 精選 Skill 內容、來源、License 與驗證狀態完成檢查。
- [ ] RELEASE-004 Sandbox 與 Secrets 安全檢查完成（MCP、Local Runner 隨後 MVP 啟動時補檢）。
- [ ] RELEASE-005 資料保存、使用者刪除與稽核流程通過驗證。
- [ ] RELEASE-006 Provider 故障、Run 逾時與清理失敗有可操作處理方式。
- [ ] RELEASE-007 使用者可理解執行權限、評估依據與相容性限制。
- [ ] RELEASE-008 下載套件可安裝且不包含受保護資料。
- [ ] RELEASE-009 完成封閉測試成功門檻並處理阻斷問題。
- [ ] RELEASE-010 發佈決策、已知限制與下一階段範圍完成記錄。

