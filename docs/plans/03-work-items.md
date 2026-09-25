# Skill Hub MVP：工作項目列表

> 本文件較早完成紀錄中把 `symlink-entry` 寫成 warning／只在匯出時排除的敘述，已被目前較嚴格的入口契約取代。現在符號連結會以 `symlink-entry` error 拒絕，其他非普通檔案會以 `unsupported-entry-type` error 拒絕；反斜線路徑亦在入口直接拒絕。歷史段落保留原貌只作當時證據，不代表現行行為。

## 使用方式

- 本文件的完成紀錄會保留當時的 `internal/<flat-context>/...` 路徑，作為里程碑時點的歷史證據；它們不是現行程式導覽。現行 Context 地址以 [M4 DDD 邊界收斂報告](mvp/m4/report-platform-ddd-boundary-convergence-2026-08-19.md)、[Domain Memory](../domain-memory/) 的已審查 Context 與 `apps/platform/internal/README.md` 為準。

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

> 本節是 AGENTS.md 指向的「目前所有未決議題」入口之一，M2 期間有多項被後續 ADR 或實作回答，但**沒有一項取得負責人的逐項定案紀錄**（[m0/README.md](mvp/m0/README.md) 仍記「PDM-004~010 未逐項定案」，[m0/pdm-proposals.md](mvp/m0/pdm-proposals.md) §定案檢查清單各列仍為 `[ ]`）。因此下方**一個勾選都不改**，改為逐項標註「實質狀態」與「還缺什麼才能勾」——把「值已經定了」與「決策已經被追認」分開記，是這份清單唯一有用的地方。
>
> M2 殘項清單的「乙、待負責人決策」另有一份互補的視角（那份記的是阻擋實作的決策，本節記的是產品參數的決策），見 [04-backlog-and-handoffs.md](04-backlog-and-handoffs.md)。

- [x] PDM-001 選定 MVP 首批三個 Skill 類別。
- [x] PDM-002 確認首批 Skill 來源與精選標準。
- [x] PDM-003 選定主要 Agent Runtime 與模型。
- [x] PDM-004 決定 SelfHostedProvider 首批支援的 Runtime 語言與版本。（**實質已定，缺追認**：Runtime Image `2026.08-2` 為 Node 22 ＋ Agent SDK **0.3.233**（釘 digest ＋ lockfile）＋ `python3.11` 與 45 個目錄 Skill 宣告的 9 個 Python 依賴（[m2/README.md 乙-6](mvp/m2/README.md) 裁定「加」）。**還缺**：①上述組合寫成 PDM-004 的定案紀錄（現在只散在 Dockerfile、`infra/images/README.md` 與 M2 殘項清單裡）；②`runsc` 上跑通完整 Run 生命週期的實測——那是 m0 提案自訂的定案條件之一，屬部署期（`SEC-009` 測項 T4））
- [x] PDM-005 決定 Dataset 大小、檔案類型與單次 Run 資源上限。（**值大部分已強制，缺追認**：`02:TEST-002` 與 `02:RUN-003` 已把 §5.1／§5.2／§5.2a 的值回寫為可判定準則，六項資源上限、Artifact 上限、Workspace 並行 2、**Token 300K／60K**（`03` SBX-013）皆已有強制點。
- [x] PDM-006 決定 Run、Dataset、Trace 與 Artifact 的保存期限。（各類都有值：Dataset 由 `TEST-004` 的 `expires_at` 強制、Trace 分割表按月切、SEC-009 的證據保存另有下界（見[Sandbox 隔離與執行安全](../adr/README.md#sandbox-隔離與執行安全)），其餘由部署設定帶入（`TRACE_RETENTION`、`DOWNLOAD_ARTIFACT_RETENTION`、`ANALYTICS_RETENTION`）與 `trial/execution` 的 Run Artifact 常數。數字**之間**的下界約束是 `02:NFR-002a`，由 `automation-check` 的 `retention-floor` 逐條對帳）
- [ ] PDM-007 決定 Local Runner 首批支援的作業系統。（後 MVP）
- [x] PDM-008 決定首批目標 Agent 打包 Profile。
- [ ] PDM-009 決定封閉測試人數、招募方式與成功門檻。（→ [m4/pdm-009-beta-proposal.md](mvp/m4/pdm-009-beta-proposal.md)。此前**連提案都沒有**——`m0/pdm-proposals.md` 沒有 PDM-009 這一節，而兩份既有文件各自假設了一個數字（該檔 §8.2 用 30 人、`m0/cost-estimation.md` 用 20 人），**互斥且都沒有依據**。提案內容：12 人（三層各 4）、三條門檻（完整旅程完成率／漏斗無一段低於 50%／整層失效或同一問題撞到 4 人即否決）、招募與報酬全數重用 `gate-test/recruit.md`、測試期 14 天、**先閘門再封測**。**未追認前 `BETA-001`／`BETA-005`／`RELEASE-009` 全部不可判定**）
- [x] PDM-010 決定免費 Run 額度及是否支援使用者自備模型 API Key。
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

## 3. 資訊架構與體驗設計（M0–M4）

> **本節 13 項自 M0 起零勾選，根因不是進度**：**本專案沒有產出過設計交付物，UI 是直接依 `02` 的允收準則實作的。** 要改變這個狀態需要的是一個「這個專案要不要有設計交付物」的決定，不是某個里程碑的收尾動作。
>
> **裁定：`DESIGN-001`～`011` 標為「不再追蹤」並逐項指向已落地的畫面；`DESIGN-012`／`013` 保留為 M4 的實作項**（前者有真正的新畫面＝下載頁，後者有可判定的準則＝`QA-009` 已寫出的無障礙判準）。章節里程碑標記因此由 `M0–M3` 改為 **`M0–M4`**（[m4/README.md §7 差-1](mvp/m4/README.md)）。
>
> **三件必須一起被讀到的事**：
>
> 1. **「不再追蹤」不是「已完成」，所以一律維持 `- [ ]`。** 這些項目描述的設計交付物確實從未產出，勾選它們是說謊；刪掉整節則會讓 [評估判定與 Judge 信任邊界](../adr/README.md#評估判定與-judge-信任邊界)的待決策與差-1 的兩項失去承接者（m4/README §8.5 的選項 (b) 因此被否決）。**不刪文、不改判定、只加理由與落點**。
> 2. **不補做 13 份設計交付物**（選項 (c)）：M0～M3 的 UI 已經照允收準則落地，事後補一份沒有人會照著改程式的設計文件，只會產生第二份真相。
> 3. **殘留的缺口必須有具名落點，不得因為本節收斂而消失。** 兩處：`DESIGN-007` 的 preflight 版本選擇器；`DESIGN-010`／`011` 承接的[評估判定與 Judge 信任邊界](../adr/README.md#評估判定與-judge-信任邊界)待決策——實質已由既有畫面回答，剩下的是呈現。
>
> 代價寫明：本裁定等同承認 `01`／`02` 隱含的「先設計後實作」流程在本專案沒有發生過。這是紀錄真實發生的事，不是為了讓帳好看。

- [ ] DESIGN-001 建立全站資訊架構與導覽模型。（**不再追蹤**——落點：`apps/web` 的 `router.tsx` 與頁首導覽，路由與導覽模型隨各功能項逐次成形，沒有一份先行的架構文件）
- [ ] DESIGN-002 設計首頁與自然語言意圖搜尋流程。（**不再追蹤**——落點：`DISC-001`／`DISC-005` 已落地的首頁、搜尋與「無結果＋改寫建議」流程）
- [ ] DESIGN-003 設計搜尋結果、篩選與排序說明。（**不再追蹤**——落點：`DISC-002`／`DISC-003`／`DISC-004`，含刻意呈現的停用篩選控制項與逐維度理由、`RankingExplainer`）
- [ ] DESIGN-004 設計 Skill 一般詳情與進階檔案檢視。（**不再追蹤**——落點：`DISC-006`／`DISC-007`／`DISC-008`）
- [ ] DESIGN-005 設計 Skill 靜態比較介面。（**不再追蹤**——落點：`DISC-009`）
- [ ] DESIGN-006 設計 Fork、個人工作區與版本差異流程。（**不再追蹤**——落點：`WS-001`／`WS-002`／`WS-003`／`WS-005`）
- [ ] DESIGN-007 設計 Test Case、Dataset、MCP 與工具設定流程。（**不再追蹤**——落點：`TEST-012` 的 `apps/web/src/features/lab/test-cases/` 與 `/lab/datasets` 上傳頁。**兩項範圍註記**：①MCP 與工具設定屬後 MVP，本項那一半不會有落點；②**preflight 頁仍然沒有 Skill Version 選擇器**——`version` 只能從 URL query 進來，`EVAL-011` 也把它記在本項名下。本項收斂後該殘留不隨本節一起消失）
- [ ] DESIGN-008 設計執行前權限及成本摘要。（**不再追蹤**——落點：`TEST-008`／`TEST-009`／`TEST-011` 的 `RunPreflight`，含八項權限揭露、摘要 hash 重新確認與預估成本區間）
- [ ] DESIGN-009 設計 Run 進度的一般模式與進階 Trace 模式。（**不再追蹤**——落點：`TRACE-006`／`TRACE-007` 的 `/runs/$runId`，含一般／進階切換與 `complete: false` 的誠實標示）
- [ ] DESIGN-010 設計驗收條件、評估報告與改善建議流程。（**不再追蹤**——落點：`EVAL-003`～`EVAL-009` 的 `RunEvaluation.tsx`。**[評估判定與 Judge 信任邊界](../adr/README.md#評估判定與-judge-信任邊界)待決策（兩列狀態的文案與版面）的實質內容已由這個畫面回答**：執行狀態與任務判定永遠兩列、任務判定排在前、沒有評估的一邊顯示「未評估（不是通過）」，具名測試在 `eval.test.tsx`。剩餘缺口是 ——UI 分不出「引用回驗失敗」與「模型自己說不知道」，由 M4 的 UI 批承接）
- [ ] DESIGN-011 設計重新試跑與版本／結果比較流程。（**不再追蹤**——落點：`EVAL-011`／`EVAL-012` 的 `RunCompare.page.tsx` 與 `AppliedResult` 的 preflight 交接。[評估判定與 Judge 信任邊界](../adr/README.md#評估判定與-judge-信任邊界)的兩列狀態在比較畫面同樣成立，見 `DESIGN-010`）
- [x] DESIGN-012 設計打包下載、安裝及驗證說明。**（M4 實作項，保留追蹤）**（本項與 `DESIGN-001`～`011` 不同：M4 有真正的新畫面。範圍＝下載頁與安裝說明的呈現面，對應 `PACK-006`／`PACK-007`／`PACK-008` 與 `02:PACK-002` 的三條允收準則；三層相容性分開呈現、未驗證必須顯示未驗證、到期日顯示絕對日期不顯示相對天數。排入 M4 第 6 批）
- [x] DESIGN-013 完成鍵盤操作、文字標籤與錯誤訊息的無障礙檢查。**（M4 實作項，保留追蹤）**（判準取自 `02:NFR-007`：主要流程可鍵盤完成、表單具標籤與清楚驗證訊息、狀態不只依賴顏色。**與 `QA-009` 逐字重疊**（[m4/README.md §7 差-1](mvp/m4/README.md)）：兩者以**同一份檢查結果**判定，不做兩次也不各記一份帳——本項是檢查的執行，`QA-009` 是發佈檢查表上的同一格，勾選時互相引用。排入 M4 第 6 批）
- [ ] DESIGN-014 把畫面上的名詞換成日常用語。（**新增，`05` R-56 裁定 (a)、[設計系統、信任訊號與畫面用語](../adr/README.md#設計系統信任訊號與畫面用語)**。對照表在該 ADR 決策 1，已定死：`Skill`→小工具、`Run`→試跑／試跑紀錄、`Test Case`→測試題、`Fork`→複製一份，**`Workspace` 不改**。範圍：`apps/web/src` 的使用者可見字串 **382 處／31 個檔案**（無 i18n 集中層）＋ 2 個 Go 常數（`creator/workspace/http.go` 的 `deletionScope`、`entrypoint/api/apiserver/creation.go` 的 422 文字）＋ 10 份 `__outlines__` 標題快照。契約、資料庫、識別字、ADR 內文與需求 ID 一個字都不動。**不做括號並列、不做開關、不做逐頁分批**——三者都會讓同一個東西有兩個名字。**前置（寫死）：apps/web 的版面批合併之後**，理由是單一 Writer——兩個代理同時改同一批檔案是 AGENTS.md 直接禁止的。**必須單獨成一個 commit**，不與任何行為改動混在一起，否則 382 處的機械變動會把 review 淹掉。做完才結。允收：`02:NFR-001`、[設計系統、信任訊號與畫面用語](../adr/README.md#設計系統信任訊號與畫面用語)的相關決策）

## 4. Skill 內容與供應（M1）

> 本節的 **CONTENT-007／008 屬 M2**（依賴 M2 的 Test Case 與 Sandbox，M1 內結構性不可能完成），其餘項目屬 M1。
>
> 本節九項原本只有一行敘述、`02` 無對應需求 ID（[content/content-summaries.md §3](mvp/content/content-summaries.md) 交付時發現）。允收準則已補於 **`02` 第 4.7 節「內容供應與策展」**，各項行尾標註引用。準則內容取自已定案與已落地的實務（PDM-002 九項檢查與白名單制、[打包、授權溯源與散布](../adr/README.md#打包授權溯源與散布)的授權兩軸、[意圖搜尋](../adr/README.md#意圖搜尋)的索引時增強與人工抽查），未新增要求。

- [x] CONTENT-001 定義精選、已索引與外部結果的收錄政策。（允收：`02` §4.7）
- [x] CONTENT-002 定義來源可信度、License 與衍生關係的呈現規則。（允收：`02` §4.7）
- [x] CONTENT-003 建立首批 Skill 候選清單。（正式清單見 [content/curated-skill-list.md](mvp/content/curated-skill-list.md)、匯入資料 `tools/content/seed-skills.json`。
- [ ] CONTENT-004 對首批 Skill 完成來源及 License 檢查。（License 合規總表見 [curated-skill-list.md §5](mvp/content/curated-skill-list.md)；11 個入選 repo 已逐一實查 LICENSE 檔。未結案項：§5.2 `anthropics/skills` source-available 條款是否允許平台保存內容快照，待負責人與法務判定——[governance/anthropic-sa-license-memo.md](mvp/governance/anthropic-sa-license-memo.md)（索引／展示／沙箱試跑／打包下載四種行為逐項評估，另補 LLM 增強產出一項；建議方案 C：先關閉這 4 筆的全文展示、維持索引與下載封鎖、終判前不對外開放試跑，並與上游詢問書面澄清並行。**非法律意見，終判仍在負責人與法務**）（允收：`02` §4.7）
- [x] CONTENT-005 對首批 Skill 產生一般使用者可理解的摘要。（允收：`02` §4.7 修訂版逐條達成，見 [m1/content-review-report.md](mvp/m1/content-review-report.md) §8：45 筆全量自動化審校 **45/45 通過**、精選 15/15、主判準 45/45、忠實性 890 條宣稱 0 條未支持；審核紀錄見 [content/content-summaries.md](mvp/content/content-summaries.md)。
- [x] CONTENT-006 對首批 Skill 完成規格及靜態掃描。（允收：`02` §4.7）
- [x] CONTENT-007 對精選 Skill 建立範例資料、Prompt 與驗收條件。**（M2：依賴 Test Case 與 Sandbox）**。
- [x] CONTENT-008 對精選 Skill 完成至少一次基準試跑。**（M2：依賴 Test Case 與 Sandbox）**。
- [x] CONTENT-009 建立內容更新、失效、下架與來源變更流程。（允收：`02` §4.7）
- [ ] CONTENT-010 於 Runtime Image `2026.08-3` 重跑 45 筆基準，回填 `skill_runtime_compatibility` 的 (Skill Version × `2026.08-3`) 一軸。（見 [m2/content-baseline-report.md §14](mvp/m2/content-baseline-report.md)：45 筆全量發動，**41 筆完成量測**（符合 39、未產出 2）、**4 筆授權受限如實跳過**（`docx`／`pdf`／`pptx`／`xlsx`，`POST /runs` 回 422 `license-review`，未繞過）；41 筆判定與 `2026.08-2` **完全一致**，`ModuleNotFoundError` 由 4 筆歸零、輸入 token −16%。回填寫入 **41 列**（`activated`／`native`），舊映像 90 列 md5 前後相同。**四筆 restricted 在 `2026.08-3` 無列**，目錄仍顯示其 `2026.08-2` 舊值並附映像標籤，解除後須補跑——連同「office 三件套 validator 未被執行過」一併列為後續（§14.9）。閘道實付 $2.0038。**原始工作說明保留於下**：由 SBX-002 的 `2026.08-3` 升版產生：0022 以 (Skill Version × Runtime Image) 為鍵，`2026.08-3` 目前 **0 列**，目錄仍顯示 `2026.08-2` 的結論並附映像標籤。依賴集**只增不減**使結論極可能不變，但 0022 的鍵不接受「極可能」——[Sandbox 隔離與執行安全](../adr/README.md#sandbox-隔離與執行安全)的四項行為重驗已在新 digest 上實跑通過（[UPGRADES.md](../../infra/images/runtime-agent-sdk/UPGRADES.md)），那管**行為回歸**；本項管**目錄事實**，兩者不互相取代。成本以 [m2/content-baseline-report.md §13.6](mvp/m2/content-baseline-report.md) 實測推估約 **$2.2**。回填走 `tools/content/backfill-agent-compatibility.sql`，須帶 `-v image=` 與 `-v since=` 的時間窗，否則會把新量測貼上舊映像標籤）
- [ ] CONTENT-011 重跑 CONTENT-005 增強，使被拒收依賴造成的能力缺口進入「限制」欄。（**不得就地改寫**：`02:CONTENT-005` 規定 `需修改` 的處置是調整 prompt 或標記人工覆寫後**重跑增強與重新索引**，因此 `2026.08-3` 這批**只記錄建議、不執行**。對象三筆與應涵蓋內容：`pdf`——渲染／OCR 支路（`reportlab`／`pypdfium2`／`pdf2image`／`pytesseract`）與 Node 支路皆不可用，可用的是 `pypdf`＋`pdfplumber`＋`pillow` 的讀取／抽取／合併；`pii-flag`——`presidio-*` 偵測引擎不可用，可用的是模型判讀＋`phonenumbers`／`python-stdnum` 格式驗證，**NFR-001 的措辭紀律因此更吃重**；`document-format-skills`——`pywin32`／COM 的 `.doc`／`.wps` 轉檔在 Linux 沙箱上永久不可能，`.docx` 路徑可用。另 `pyarrow` 遭拒使 `add-iso3166`／`add-data-dictionary`／`data-comparability`／`data-cleanliness-scan` 四筆的 Parquet 格式支援不可用，亦應揭露。理由與逐項對照見 [infra/images/README.md](../../infra/images/README.md)「拒收」表。
- [x] CONTENT-012 同步 [content/curated-skill-list.md](mvp/content/curated-skill-list.md) §2 三張表的「依賴」欄與 `tools/content/seed-skills.json`，並依負責人裁定更正檢查 ⑥。
- [x] CONTENT-013 為已索引層補「依賴」欄或等價揭露。（由 CONTENT-012 發現：[content/curated-skill-list.md](mvp/content/curated-skill-list.md) §4.1／4.2／4.3 三張已索引表**只有 License 與 Tier，沒有依賴欄**，所以 `deps` 的修正在該層**沒有可同步的欄位**——12 筆已索引 Skill 的依賴更正只存在於 `seed-skills.json` 與 §3 腳註 7。已索引層依 PDM-002 不要求九項全過，故非阻擋，但「使用者看得到的依賴揭露」最終仍應由 DISC-003 的詳情頁承接而非本文件；先記錄缺口，實作歸屬待定）

## 5. 核心領域與帳號（M1）

- [x] CORE-001 建立 User 與個人工作區資料模型。
- [x] CORE-002 建立 Skill、Skill Source、Skill Version 與 Fork 資料模型。
- [x] CORE-003 建立 Test Case、Run、Trace、Evaluation 與 Artifact 資料模型。
- [x] CORE-004 定義不可變 Skill Version 與歷史 Run 快照規則。
- [x] CORE-005 建立基本登入、登出與工作區存取控制。（GitHub OAuth ＋ Postgres session ＋ `DEV_LOGIN` 離線 provider；登出為伺服器端撤銷。證據見 [m1-work-items-audit.md §3.1](mvp/m1/m1-work-items-audit.md)）
- [x] CORE-006 建立使用者私有內容的授權檢查。（Workspace scope 一律取自 session，不信任 UI 傳入值；非擁有者一律 404。證據見 [m1-work-items-audit.md §3.1](mvp/m1/m1-work-items-audit.md)）
- [x] CORE-007 建立使用者資料與 Artifact 的刪除流程。
- [x] CORE-008 建立重要操作的 Audit Event。

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
- [x] INGEST-010 建立外部內容失效、來源更新及人工下架流程。
- [x] INGEST-011 靜態檢查涵蓋 `SKILL.md` 內嵌可執行程式碼並揭露規模（`02:SKILL-003`）。
- [x] INGEST-012 實作 License 多層溯源：記錄來源層級、打包器搬運 repo 層授權、SPDX 正規化（`02:SKILL-004`、[打包、授權溯源與散布](../adr/README.md#打包授權溯源與散布)）。
- [x] INGEST-013 外部 URL 揭露依主機聚合並保留完整明細（`02:SKILL-005`）。
- [x] INGEST-015 修正索引增強補跑的兩個平台缺陷（[m2/README.md 乙-12](mvp/m2/README.md)）。
- [x] INGEST-016 大小拒絕要說得出數字，並且被計數。
- [x] INGEST-014 實作匯入抓取器的 SSRF 與內部網路防護：scheme 白名單、解析後逐一比對位址封鎖清單、DNS Rebinding 防護（解析與連線綁同一位址、每跳重驗）、redirect 上限 3 跳且跨主機不帶憑證、回應體 10 MB 邊讀邊中止、連線 10 秒／整體 60 秒逾時、fail-closed、錯誤訊息不洩漏內部位址。

- [ ] INGEST-017 匯入認得三種來源形狀並展開成多個 Skill Version：plugin manifest 的 `skills` 欄位、`skills/`、`.claude/skills/`、根目錄的固定探索順序；找不到任何 `SKILL.md` 時訊息列出找過的位置（`02:SKILL-006`）。今天的 `PackageRoot` 只認「根有 `SKILL.md`」或「單一頂層目錄」兩種，而 GitHub repo 的壓縮檔永遠是後者——指著一個真實 repo 的人一律收到「缺少 `SKILL.md`」。
- [ ] INGEST-018 Plugin 的非 Skill 元件只揭露不匯入：`commands/`、`agents/`、`workflows/`、`output-styles/`、`hooks/`、`.mcp.json`、`.lsp.json` 進排除揭露並可辨識為 Plugin 元件，不進任何會被複製進沙箱的位元組（`02:SKILL-006`、實作鐵律 1）。**hooks 與 MCP／LSP 的定義本身就是執行指令**，這一項守的是「匯入不得替使用者決定要執行什麼」。
- [ ] INGEST-019 plugin manifest 的驗證與來源事實：只有 `name` 缺少或不符 kebab-case 是阻擋錯誤、其餘未知欄位為 info；每個 Skill Version 記下 plugin `name`／`version`／`repository` 與相對路徑；單次匯入的 Skill 數量上限由部署設定，超過即整批拒絕並說出兩個數字（`02:SKILL-006`）。**上限值本身未定**，見[打包、授權溯源與散布](../adr/README.md#打包授權溯源與散布)的待決策。

## 7. Skill Explorer（M1，結束時通過驗證閘門才進 M2）

> 稽核指出「不通過不進 M2」三處原文未改而 M2～M6 已蓋了四層、九場一場沒跑，並建議二擇一：跑完它，或新增一份 ADR 明文取代它。**負責人選了第一條**（[`05` R-5](05-pending-rulings.md) 關閉，排程見 [gate-test/README](mvp/gate-test/README.md)）：**本節標題、`AGENTS.md` 與 `01` §10 的規則句都不動，也不需要新的 ADR**——這道門會真的被走一次。閘門與 M2～M6 並行是既成事實（`01` §10 有那句處置），所以本節標題讀作「這道門排定了」，不讀作「這道門擋住了後面」。

- [x] DISC-001 實作自然語言意圖搜尋。
- [x] DISC-002 產生候選 Skill 與符合原因。
- [x] DISC-003 實作類別、來源、Agent、Script、MCP 與驗證狀態篩選。**（依 `02:DISC-002`「篩選維度的允收階段」勾選：M1 階段的兩個維度「是否包含 Script」與「驗證狀態」已實作並有逐筆資料；其餘四個維度未達允收階段，依同節新增準則以停用控制項＋理由呈現，API 回 400 並附理由，皆有具名測試。類別／來源層級待 CONTENT-003 策展資料、Agent 相容待 M2 Sandbox、MCP 待後 MVP。證據與解除條件見 [m1/m1-work-items-audit.md §5.3](mvp/m1/m1-work-items-audit.md)。）**。
- [x] DISC-004 實作可解釋的排序規則。
- [x] DISC-005 實作無結果、低信心及查詢補充流程。
- [x] DISC-006 實作 Skill 一般詳情頁。
- [x] DISC-007 實作 `SKILL.md` 與檔案樹進階檢視。
- [x] DISC-008 實作來源、License、風險及相容狀態展示。
- [x] DISC-009 實作至少兩個 Skill 的靜態比較。
- [x] DISC-010 驗證公開搜尋與詳情不要求登入，私有操作要求登入。
- [ ] DISC-011 實作意圖分析與查詢改寫，並給中文查詢在 FTS 腿上的下界一個明文決定。（允收：`02:DISC-005`）<br>**這是十六步裡唯一一步連半邊都沒有的**（`04` 丙-76）：沒有端點、沒有畫面，查詢原文直接送 embedding，而 [意圖搜尋](../adr/README.md#意圖搜尋)指定的查詢改寫也沒有實作（`apps/llm` 只有 `/embed`、`/match-reasons`、`/suggest-criteria`）。<br>**四件要一起交，缺任何一件都不算**：①結構化意圖（輸入／輸出／工具／資料／環境五欄，缺席用缺席詞彙不用空字串）；②改寫成關鍵詞 ＋ 篩選條件，篩選值域限於 `DISC-002` 已實作的維度；③**降級不得繞過向量腿**——改寫失敗時降級為「原句直接做向量檢索」，**不得**降級為「原句直接做 FTS」（PDM-003／[意圖搜尋](../adr/README.md#意圖搜尋)明文禁止的那一種），**並要一支把改寫器換成永遠失敗的具名反證測試**，否則這條路徑會在某次重構裡安靜地變成 FTS-only 而**只在中文查詢上顯現**；④中文 FTS 的下界要有明文決定（`pg_bigm`／`pgroonga`／`pg_trgm` 擇一並記實測命中率，或明文寫「中文只走向量腿」）——**今天寫死 `websearch_to_tsquery('english', …)`，自家量到中文查詢只答對 20%，而這是一個介面全繁中的產品**。<br>**每一個新引入的數字都要有強制點或斷言**，並套 `one-number` 機械對帳（同 `GEN-001` 三個上限的處理）；沒有機器守著的數字不寫進畫面。<br>**時程上的意義**：M1 閘門量的是真人打的字找不找得到東西，**而那個數字只有一次機會**——這一項若在閘門之前沒有落地，不通過時**沒有人分得出來它敗在搜尋品質還是敗在一步從未實作**。
- [x] DISC-012 首頁在還沒有人搜尋時列出目錄。（允收：`02:DISC-006` 的八條）

## 8. Fork、版本與工作區（M1）

- [x] WS-001 實作 Fork 並保留來源與 License 關係。
- [x] WS-002 實作不可變版本保存。
- [x] WS-003 實作任兩版本差異比較。
- [x] WS-004 實作個人 Skill、Test Case、Run 與下載紀錄列表。
- [x] WS-005 實作私有內容刪除與狀態回饋。
- [x] WS-006 驗證不同使用者無法存取彼此私有內容。（列表／刪除／Fork／Diff 四條路徑與公開搜尋皆有具名整合測試，CI 帶 Postgres service 實際執行。證據見 [m1-work-items-audit.md §6](mvp/m1/m1-work-items-audit.md)）

## 9. Test Case 與執行設定（M2）

> **§9 的允收範圍**：本節的允收準則**含 UI**，但**含不含 UI 是逐條判定，不是逐節判定**——判準取自 `02` 各條允收準則自己的主詞與動詞：
>
> | 準則的主詞／動詞 | 要在哪個平面滿足 | 例 |
> | --- | --- | --- |
> | 「系統必須…」「每個 Run 必須…」 | 系統平面，API ＋ 契約 ＋ 強制點即成立 | `02:TEST-001` 第 1、4 條 |
> | 「系統依…自動建議…為可選強化」 | 同上 | `02:TEST-001` 第 2 條後半 |
> | 「**使用者可**…」「**顯示**…」 | 使用者碰得到的平面，**沒有介面就是沒有達成** | `02:TEST-001` 第 2 條前半、第 3 條；`02:TEST-002` 第 2 條 |
>
> 這把尺可以逐條判定，不需要先回答「§4.3 是在描述系統能力還是端到端可用性」——**條文自己已經用主詞回答了**。TEST-004 因「顯示」被退回、補上 `/lab/datasets` 後重新勾選，是同一把尺的第一次套用；本次套用結果為 **TEST-003 退回、TEST-001／002 維持**，逐條理由見 [m2/m2-work-items-audit.md §12](mvp/m2/m2-work-items-audit.md)。
>
> **TEST-012 完成、TEST-003 依同一把尺補回勾選**。退回時寫的解除條件（「缺的**只有**顯示與操作面，補上即可依允收重新勾選」）已經達成，介面在 `apps/web/src/features/lab/test-cases/`。對帳見 [m3/audit.md](mvp/m3/audit.md)——本節的帳因此在 M2 完結後被動了兩次，兩次都在本節就地記錄，`m2/` 目錄內的那份對帳維持凍結不改。
>
> 選 (a) 而不選 (b)（把 §9 界定在 API＋契約層）的理由：`02` §4.3 的條文是既有的產品承諾，把它改寫成「API 層即可」等於為了讓帳好看而降低承諾——而 MVP 的 Persona 明確是**非技術使用者**（`01` §2.1），一個只能用 `curl` 達成的「使用者可」不是這份文件在說的事。介面本身由 `DESIGN-007`（設計）與新增的 **TEST-012**（實作）承接。

- [x] TEST-001 實作 User Prompt 輸入與驗證。（Test Case CRUD 綁 Workspace 內 Skill；非空白與長度驗證在 Service 與 0004／0017 的 CHECK 雙重把關；使用者輸入與編輯畫面已由 `TEST-012` 的 `TestCases.page.tsx` 提供）
- [x] TEST-002 實作驗收條件自動建議。（`POST /test-cases/{id}/criteria/suggest`：Go 讀 Skill 名稱／摘要、User Prompt 與 Dataset **欄位名＋推斷型別**，經內部 HTTP 呼叫 `apps/llm` 的 `POST /suggest-criteria`（mini 級 `gpt-5.4-mini`、`json_schema` strict、經 LiteLLM 閘道）。**Dataset 的資料列不出境**：請求 schema 只有欄位名與型別兩個欄位，型別由第一列在 Go 程序內推斷後即丟棄（鐵律 11，具名整合測試以真實列值反證）。寫入時標 `source='suggested'` 且未確認；使用者改寫文字即轉為 `source='user'`。LLM 未設定或失敗回 503 並保留手動路徑，`02:TEST-001` 的「可選強化」語意成立。
- [x] TEST-003 實作驗收條件新增、修改、刪除與確認。（確認為明示同意欄位；改寫文字即撤銷既有確認。
- [x] TEST-004 實作 Dataset 上傳、限制、關聯與刪除。（PDM-005 §5.1 全數強制：單檔 25 MB、單 Test Case 100 MB／20 檔、magic bytes 判型不信副檔名、`expires_at` 90 天；刪除連同物件，皆有具名整合測試。`DatasetUpload.page.tsx` 在檔案控制項前顯示 API 提供的限制／保存／用途，讀不到規則時 fail-closed；`TestCases.page.tsx` 已提供 Dataset 列表與刪除）
- [ ] TEST-005 實作遠端 MCP 位址及短效憑證設定。（後 MVP）
- [ ] TEST-006 實作 MCP 工具發現與權限選擇。（後 MVP）
- [ ] TEST-007 實作 Local Runner 連線狀態與本機絕對路徑選擇。（後 MVP）
- [x] TEST-008 實作執行前 Dataset、Script、MCP、工具、網路與 Secrets 摘要。（`GET /skills/{id}/runs/preflight`，八項全數揭露：Dataset 名稱／型別／大小與合計、Script 由 `skillpkg.Validate` 重掃實際會執行的套件位元組（讀不到時標 `unavailable`，**不得呈現為 `none`**）、工具為 Sandbox 內建檔案與 Shell、**MCP 在 MVP 恆為空清單並明確顯示為「無」而非略過**、網路取 `policy_snapshot` 的 `default_deny` ＋空允許清單、Secrets 只列注入項名稱、Provider 取排程實際會選中的那一個、資源上限直接取 `DefaultResourceLimits()`／PDM-005 §5.2。摘要以 canonical JSON（固定欄位順序的 struct）取 sha256，慣例同 `testlab/snapshot.go`）
- [x] TEST-009 實作權限異動後重新確認。（migration **0020** `run_permission_confirmations` 記錄「誰、在何時、同意了哪個摘要 hash」；`POST /skills/{id}/runs/preflight/confirm` 寫入，`internal/run/service.go` 的 `Create` 入口重算當下摘要 hash 並要求兩件事同時成立：請求帶的 hash 等於重算值、且該 hash 有確認紀錄。任一不成立回 **422 且不建 Run**——這是 **SEC-002 閘門 B**「使用者未確認或未重新確認執行前權限摘要」那一項。舊確認因 hash 為查詢鍵而自然失效，不需另作撤銷掃描；換／刪 Dataset 皆有具名整合測試。使用者拒絕＝不呼叫確認端點，沒有紀錄就起不了 Run）
- [x] TEST-011 在執行前權限摘要加上**預估成本區間**（區間非單值）。（`PermissionSummary.EstimatedCost`、公開 OpenAPI 契約與 `RunPreflight` 均已落地；$0.01／常見 $0.06／$0.30 取自 M2 的 45 筆閘道實付分布，`basis` 明示為估計而非報價。成本不進權限 `summary_hash`，避免估值校準使既有權限確認失效；`TestCostEstimateIsOutsideTheConfirmedHash` 驗證此邊界）（允收：`02:TEST-005`）
- [x] TEST-012 實作 Test Lab 的 Test Case 與驗收條件介面：建立 Test Case、輸入與編輯 User Prompt、驗收條件的新增／編輯／刪除／確認、Dataset 列表與刪除。
- [ ] TEST-013 一個 Skill 可以有幾個 Test Case，沒有任何上限。**這不是有人忘了做，是三份東西宣稱它已經有了。** `testlab.go` 的 `page()` 當時的註解逐字寫著這份清單「already bounded by the per-skill test case count」，`packaging.go` 引用同一句，而**全 repo 沒有任何地方界過那個數**——PDM-005 §5.1 界的是**單一** Test Case 的檔案（20 檔／單檔 25 MiB／合計 100 MiB），§5.2 界的是 **Run 資源**，兩者都不是這件事。<br>**它是 `PACK-012` 推導停住的直接原因**：產出上限的算式是「匯入上限 ＋ 打包能加的量」，而打包能加的量 ＝ 每個 Test Case 的大小 × **數量**，數量無界則算式無界。`MaxProducedZipBytes` 因此只能寫成「約 48 個純文字 Test Case 打滿」，**而 48 是沒有來源的那一半**。<br>**兩條路徑各自受影響，程度不同**：①**讀取面**——`ListTestCasesForSkill` 沒有 `LIMIT`，`page()` 在 Go 裡切片，所以每一次列表都把全部列**先實體化**；今天沒事只是因為還沒有人建很多。**不在 query 上加 `LIMIT`**，因為同一條語句餵給打包，而打包合法地需要全部（`PACK-005`）——在那裡加 `LIMIT` 會**無聲截斷**一份下載的測試案例。②**打包面**——`excluded_test_cases` 已經是「這一筆不隨套件走，理由是 X」的既有機制，數量上限的排除**應該用它**，不另造。<br>**這一項要的是一個數字，而那個數字不是實作者能挑的**（同 PDM-005 其餘各項）。程式面能先做的是把界**放在建立當下**而不是列表當下——但沒有值就不該先寫強制點，否則就是另一種形狀的同一個問題（一個沒有人裁定過的上限）。<br>**同批記一個不擋本項的既有行為**：catalog 工作區的打包會**把資料集位元組本身**夾帶進去（`testcase.go` 的 `ws.IsCatalog` 分支），單筆上界是 `testlab.MaxTestCaseBytes` = 100 MiB，而 `PackageFS` 的單條目上限是 10 MB ⇒ 這種套件會在 `PACK-009` 的重開步驟被擋成「the produced package could not be re-opened」。**底層 error 其實說得出原因**（`%w` 包著），**但那一句沒有被翻成使用者看得懂的拒絕**。屬打包錯誤分類的問題，不在本項範圍。
- [x] TEST-010 保存實際執行使用的 Test Case 快照。（`testlab.CreateSnapshot` 為唯一實作，由 `internal/run` 於建立 Run 的同一交易呼叫；快照涵蓋 Prompt、驗收條件與 Dataset 參照並以單一 content hash 固定，不可變由 0005 trigger 保證；已刪除的 Test Case 不可起 Run）

## 10. Run Orchestrator 與 Provider 契約（M2）

- [x] RUN-001 定義 Provider-neutral Run Request 與 Run Result。（`contracts/openapi/sandbox-provider.yaml`）
- [x] RUN-002 定義 Provider Capability 描述格式。（同上檔案 `ProviderCapability`；能力相容檢查屬 RUN-005）
- [x] RUN-003 定義平台 `run_id` 與 `provider_run_id` 映射。（0016 `run_attempts`；解掉 0004「重試覆寫 `provider_run_id`」的已知債）
- [x] RUN-004 實作 queued 到 cleaning_up 的標準狀態機。
- [x] RUN-005 實作 Run 排程、Provider 選擇與能力相容檢查。（Provider 註冊表為部署靜態設定 `SKILLHUB_SANDBOX_PROVIDERS`／`SKILLHUB_SANDBOX_TOKEN_<NAME>`，不做動態註冊；`GET /capability` 以 30 秒 TTL 快取，worker 啟動時清空重讀；相容檢查涵蓋隔離強度（強／弱／無，部署設定可接受的最低值）、rootless、egress 模式、Runtime 家族與整合模式、六項資源上限，不相容者**在排入佇列前**回 422 並附逐一理由；派送時只選回報有空位的相容 Provider（空位多的優先），被以沒有空位拒絕就換下一個，全滿時 Run 留在 `queued` 等候、最多 30 分鐘，空位先給持有沙箱較少的 Workspace、同數時先建立的先，執行中的 Run 每次查詢後把工作延後再取、不佔住 Worker，硬性時間上限從 Provider 接受時起算；被接受時才把結果寫進 `runs.provider` 與 `runtime_snapshot`，`provider_run_id` 只寫 `run_attempts`；Provider 失聯滿 90 秒或回報不認得這個 Attempt 時改派一次到別的 Provider，第二次遺失以 provider_error 結束）
- [x] RUN-006 實作取消、逾時、有限重試與失敗分類。（取消：`cancel_requested_at` → 輪詢時呼叫 provider cancel → 待 provider 回終態才轉移，符合「取消不得謊報已停止」；逾時：provider 回報 `timed_out` 為軟逾時，平台側以 `created_at + wall_clock_hard_seconds` 為硬逾時，driver 與 supervisor 雙重把關；重試：新增一筆 `run_attempts` 且上限入設定（預設 3），**僅 provider 側失敗可重試，workload 自身失敗不重試**；分類寫入 0018 新增的 `runs.failure_class`。**限制註記**：重試窗僅涵蓋 run 仍在 `provisioning` 的派送階段——狀態機無回退邊，離開 `provisioning` 後的 provider 失敗只分類不重試，要放寬需新 ADR）
- [x] RUN-007 實作冪等清理與遺留 Sandbox 掃描。（終態轉移於同交易排入 cleanup job（River unique，重複排入為 no-op）；`DELETE` 依契約冪等且無 404，重跑安全；孤兒掃描以 `GET /runs?active=true` 比對平台狀態，僅在「平台已終態」或「平台不認得且 `observed_at - created_at` 超過 5 分鐘寬限」時 destroy，避免誤殺派送中的新 Run；清理失敗記 `cleanup_status='failed'` 並由 supervisor 重排）
- [x] RUN-008 實作服務重新啟動後的 Run 狀態恢復或安全終止。（supervisor 為 River periodic job（30 秒，`RunOnStart`，僅 leader 執行）：掃非終態 Run，逾期者判 `timed_out`＋清理，其餘以 unique job 重新入列——已有在途 job 時自動 no-op，故不需讀 river 表；有在途 attempt 者重新掛回輪詢，已離開派送階段卻無 attempt 可接者判 `platform_error` 安全終止）
- [x] RUN-009 建立 Provider 契約測試套件。（`internal/run/provider_contract_test.go`：冪等重送同資源／同鍵不同內容 409／cancel 已終態仍 202／DELETE 重複與未知 handle 皆 204／`active=false` 回 400／終態必帶 result／無 token 401；預設跑 in-repo fake（`internal/run/providertest`），設 `SKILLHUB_PROVIDER_CONTRACT_URL`＋`SKILLHUB_PROVIDER_CONTRACT_TOKEN` 即對真實服務跑同一套。狀態映射另以 `schedule_test.go` 的決策表驗證）

## 11. SelfHostedProvider（M2）

實作位於 `apps/sandbox/`（獨立 Go module，見[Repo 結構、CI 與驗證層](../adr/README.md#repo-結構ci-與驗證層)、鐵律 2），部署與 dev／prod 差異見 [apps/sandbox/README.md](../../apps/sandbox/README.md)。允收準則來源為 `02:RUN-003` 與威脅模型 SEC-002 的 46 項基線（本節以 `C-xx`／`N-xx`／`D-xx`／`I-xx`／`X-xx` 引用）。

- [x] SBX-001 決定自建 Sandbox 的隔離技術與執行節點拓撲。（[Sandbox 隔離與執行安全](../adr/README.md#sandbox-隔離與執行安全)已 Accepted：gVisor 基線＋專用 VM 池、不進 Kubernetes；DockerProvider 以 `SKILLHUB_SANDBOX_RUNTIME=runsc` 落實該選擇，宣告的隔離強度跟著實際設定走，跑 runc 的機器只會宣告 `weak`。**在部署平台上實跑 `runsc` 屬部署期驗收**，[Sandbox 隔離與執行安全](../adr/README.md#sandbox-隔離與執行安全)的定案紀錄已將其列為實作期前提，不影響本項的「決定」性質）
- [ ] SBX-002 建立經審核的 Runtime Image。**部分完成**：審核流水線已接上——[`.github/workflows/runtime-image.yml`](../../.github/workflows/runtime-image.yml)（path filter 只在 `infra/images/**` 變更時觸發）做 digest 斷言 → build → syft 產 SPDX SBOM → grype 掃描 → 門檻閘門，SBOM 與報告以 `if: always()` 上傳為 artifact，掃描失敗也留證據。I-02 已落地（`FROM` 帶 `@sha256:`，CI 以 grep 斷言）。掃描實跑結果與豁免清單見 [infra/images/README.md](../../infra/images/README.md)：可修的 Critical／High 原有 6 件、全在 base image 自帶的 npm 依賴，已於 Dockerfile 移除 npm／corepack（執行期不載入，沙箱亦無網路），現為 **0**；其餘 7 Critical／17 High 皆為 Debian bookworm 無上游修復項，逐項具名列入豁免清單並訂複審日，不靜默放行。**門檻已定案**：[Sandbox 隔離與執行安全](../adr/README.md#sandbox-隔離與執行安全)採納本流水線的兩項提案值並補上批准與時效——I-06 為「可修的 Critical／High 阻擋且**無豁免路徑**，不可修者具名豁免、複審日 ＝ **`first_exempted_at`（該 CVE 首次被豁免的日期，跨重掃保留）**＋90 天、逾期即依 I-04 判為過期」——錨定在首次而非最近一次掃描日是刻意的，否則每 30 天的例行重掃都會把複審日往後推 90 天，人工複審永遠不會到期，I-04 為「有效期 **30 天**，到期前 7 天告警，到期即不得被新 Run 引用」。 SBOM 與掃描結果（含機器可讀的 `scanned_at`）已成為隨 GHCR digest 的 attestation，`rescan` job 每週重掃已發佈的 digest，**流水線側四項檢查 I-02／I-03／I-04／I-06 已全部可自動化判定**。<br>**SBX-002 仍不勾的唯一原因**：閘門 A 的節點准入探針要**在真實節點上**查得到這兩份 attestation，屬部署批（SEC-009 前置條件①）。
- [x] SBX-003 實作每個 Run 的獨立環境與暫存空間。（每個 attempt 一個專屬容器，`/work`＋`/out` 為該容器獨有的 tmpfs，不與任何其他 Run 共用可寫路徑；C-01）
- [x] SBX-004 實作非 root、非特權及唯讀基礎檔案系統政策。（`User=65532:65532`、`no-new-privileges:true`、`CapDrop=ALL`、`Privileged=false`、`ReadonlyRootfs=true`；C-02／C-03／C-06／C-08。以真實容器驗證，非僅設定斷言）
- [ ] SBX-005 阻擋容器管理 Socket、主機敏感路徑與內部服務存取。`C-01` 的**執行期半邊**——「每個 Run 使用獨立環境與獨立暫存工作區，**不與其他 Run 共用可寫路徑**」的後半句——由本項的整合測試判定，不再由閘門 A 的探針判。理由是那半句是一個關於**兩個並行 Run** 的敘述，而閘門 A 拍的是一台閒置節點的照片；探針原本照 fail-closed 把它報成 `unknown`，於是**一台設定完全正確的節點也會 exit 2**，而一個永遠紅的閘門會被值班的人關掉——那比沒有閘門更糟，因為關掉之後沒有人記得它曾經該擋什麼。**後半句沒有被刪，它換了一個判得到它的地方**（探針現在印 `ELSEWHERE` 並在那一列點名本項）。**部分完成**：容器管理 Socket 與主機路徑已擋——`Binds`／`Mounts` 恆空，沙箱內 `/var/run/docker.sock` 不存在，namespace 全為 private（C-04／C-05／C-07，具名整合測試）。**「內部服務存取」未成立**：dev 已由網路面隔離——無出口需求時 `--network none`，有出口需求時只接上 `internal: true` 且只有模型閘道在上面的網路，核心資料庫與物件儲存都不在該網路上（SBX-007）。但生產的網路政策隔離與 P-02「Sandbox → 核心資料庫連線嘗試被實際阻擋」的**常駐探針**屬部署期，未實作——該探針的驗收形式見[Sandbox 隔離與執行安全](../adr/README.md#sandbox-隔離與執行安全)第三部分**測項 T10**（自 T8 的宣告式稽核獨立出來：其餘 P 區項目拍一次照即可，P-02 明文要求常駐，塞在 T8 裡會讓一次性稽核冒充常駐監控）；節點入池前跑一次且入池後常駐。<br>**不勾的理由有兩條，都不是接線**：①**探針從未在一台生產節點上跑過**，而它量的正是那台機器的網路政策——今天所有讀數都來自 fake 或 dev Docker；②**本項的另一半「生產的網路政策隔離」仍未成立**（見 SBX-007 與 `04` 甲-3）。**一個寫出來就會全面隱形的缺陷記在這裡**：第一版用 `d.tail` 讀探針輸出，而那個函式會把 handle 加上 `skillhub-run-` 前綴，於是它會去查一個不存在的容器、拿到錯誤、回空字串——而**空字串在判定裡等於「什麼都碰不到」**，所以那個版本會**每一輪都報 `pass`**。已改成以容器 ID 讀，且讀不到就是錯誤（→ `unknown`），不是 pass。具名測試：`TestAProbeThatCouldNotRunIsNeverAPass`、`TestP02ProbeDistinguishesAHoleFromTheAbsenceOfEvidence`、`TestABreachTerminatesEveryLiveRunAndRefusesNewOnes`、`TestCapabilityReportsTheProbeAndTakesTheNodeOutOfRotation`
- [x] SBX-006 實作 CPU、記憶體、磁碟、程序數與時間限制。（`NanoCPUs`／`Memory`＋`MemorySwap`（不給 swap）／tmpfs size 依 PDM-005 5.2 切 `/work` 3:1 `/out`／`PidsLimit`／`nofile` ulimit／soft 與 hard wall clock；C-10～C-15。逾時強停與 pids 上限以真實容器驗證）
- [ ] SBX-007 實作預設封鎖的網路出口政策及允許清單。**部分完成**：出口路徑已通——沙箱**僅在 `RunRequest.egress.allow` 含 `model_gateway` 時**接上 `internal: true` 的 Docker network，該網路上只有 LiteLLM 閘道可達（允許清單只有一項的最小強制形式）；`allow` 為空維持 `--network none`。方向只有一個：無出口路由的節點只宣告 `egress_modes: ["none"]`，且 `accept()` 會拒絕帶允許清單的請求，不以較弱模式頂替較強請求。物件儲存**刻意不在**該網路上（dev 的匿名／預簽語意會被破壞），位元組由 sandboxd 代搬。**設計已定案**：[Sandbox 隔離與執行安全](../adr/README.md#sandbox-隔離與執行安全)決定沙箱層採 **nftables default-deny ＋節點固定 DNS 解析器**（不部署 Squid／Envoy——MVP 的沙箱層允許清單在 SBX-008／TRACE-002 之後只剩 LiteLLM 閘道一項且為平台自有服務，L7 Proxy 買不到對應的政策需求，且看不見被拒絕的非 HTTP 嘗試），允許清單存於 `infra/egress/allowlist.yaml`（含變更 PR 流程、產品負責人核准、必須連帶更新威脅模型的 CI 斷言、90 天複審、N-07 供應商網域 deny-list），目的地記錄保存 90 天。**已隨[Sandbox 隔離與執行安全](../adr/README.md#sandbox-隔離與執行安全)落地的兩件**：①允許清單骨架 `infra/egress/allowlist.yaml`（兩層 `tier`）；②CI 斷言 `.github/workflows/egress-allowlist.yml`＋`check_egress_allowlist.py`——`tier: sandbox` 條目必須恰為一筆 `model_gateway`（否則 fail 並附上訊息指出對應決策的重評條件已觸發）、N-07 供應商網域 deny-list、`pinned_ip` 的 tier 規則、以及「動 `infra/egress/` 必須同 PR 更新威脅模型」。**仍未完成（故不勾）**：生產節點上的 nftables／dnsmasq 本體（強制點在**主機側 `forward`／`DOCKER-USER` 鏈或 Run netns 內**，不是容器內的 `output` 鏈——後者能被逃逸後改掉）、每 Run 網路命名空間與東西向阻擋、目的地記錄管線（N-01～N-07），以及 sandboxd 在 `accept()` 比對 `egress.allow` 與節點已渲染清單、不符即 `capability_mismatch` 拒絕（見[Sandbox 隔離與執行安全](../adr/README.md#sandbox-隔離與執行安全)），皆屬部署期。**另**：LiteLLM 閘道必須有沙箱面專屬位址、不得是控制平面節點（見[Sandbox 隔離與執行安全](../adr/README.md#sandbox-隔離與執行安全)的強制條件；成本 $10.38，由成本試算既有的 Egress Proxy 預算行承載）。**⚠️ [Sandbox 隔離與執行安全](../adr/README.md#sandbox-隔離與執行安全)另指出 dev 的既有缺口**：同一張 `skillhub_egress` 上的 sandbox 可互相連通，是不需逃逸的跨 Run 橫向路徑，生產形態必須關掉（SEC-009 測項 T5-4）。<br>**仍未完成（故仍不勾），逐條**：①**在節點上實際套用**——渲染器產出檔案，repo 裡沒有任何東西執行 `nft -f`；②**N-06 目的地記錄的收集端**——規則有 `log prefix` 與 drop 計數，沒有人把它們送到保存 90 天的地方；③**每 Run netns 與東西向阻擋**——dev 的 `skillhub_egress` 上 sandbox 仍可互連（T5-4）；④Q2 強制條件 6（LiteLLM 移到沙箱面專屬節點）；⑤`pinned_ip` 填實際位址。**①③⑤ 要那台節點，②④ 是部署期設定**<br>**①的「在節點上」那半仍然缺**，②④⑤ 一格未動，而**③被實驗室量得更清楚了**：東西向阻擋依賴 `net.bridge.bridge-nf-call-iptables=1`（repo 裡沒有任何地方寫著），而 `dockerdrv` 至今不指定 namespace——**所以 dev 的橫向路徑不是「規則還沒套用」，是沒有東西會套用它**。<br>**另一個要記的量測**：T5-5 的節點 loopback 那一項**在結構上留不下 nftables 紀錄**（封包在路由層就沒了，`prerouting` 的 raw 計數實測為 0），而 T5 的判準是「沒有記錄 ＝ N-06 未成立」——**節點版判定表要為它寫具名例外，否則它會永遠停在 `unknown`**
- [x] SBX-008 實作 Dataset、Skill、Secrets 與 Artifact 的短效傳遞。
- [x] SBX-009 實作完成、失敗、取消與逾時後清理。（四條終態路徑都會走到 `DELETE`；`DELETE` 冪等、無 404、不存在也回 204，釋放失敗回 500 供平台記錄 `cleanup_status` 並重試；X-01。以真實容器驗證重複 destroy 與容器確實消失）
- [ ] SBX-010 進行隔離、資源耗盡、網路與清理失敗測試。**屬部署期驗收，不在本批**：逃逸測試與 gVisor 相容性需要 Linux 與巢狀虛擬化（見[Repo 結構、CI 與驗證層](../adr/README.md#repo-結構ci-與驗證層)的待決策）。本批已有的是[Sandbox 隔離與執行安全](../adr/README.md#sandbox-隔離與執行安全)基線的真實容器驗證（非 root、唯讀 rootfs、無主機掛載、pids 上限、逾時強停、清理冪等），不等於逃逸測試通過。**SEC-009／SBX-010 未通過不得開放外部使用者提交 Skill 執行**（見[Sandbox 隔離與執行安全](../adr/README.md#sandbox-隔離與執行安全)的定案紀錄）。
- [x] SBX-011 把 Runtime Image 發佈流水線接上 **GHCR**，並把 SBOM 與掃描結果作為 attestation 隨 image digest 保存。
- [x] SBX-013 落地 PDM-005 §5.2a 的 token 硬上限（[m2/README.md 乙-2](mvp/m2/README.md) 的 **(a)**）。
- [x] SBX-012 實作 Reconciler 的 **in-flight orphan 表**（`provider_run_id → first_seen_round`）與**分項失敗計數器**（`gateway_revoke_failed`／`sandbox_destroy_failed`）。

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

> 事件流向為「容器內 harness 寫 JSONL 到 `/out` → sandboxd 讀出並 POST 到平台 ingestion → 平台遮罩後入庫」。沙箱無網路（`--network none`），執行平面不碰核心 DB（鐵律 2），ingestion 憑證是嵌在 `TracePolicy.ingestion_url` 內的每 attempt 短效簽章 token，未新增契約欄位。migration `0019_trace_ingestion.sql` 補齊 TRACE-001 §8 的四個缺口並加上 `late` 與 default partition。詳見 [m2/README.md 第三批交付摘要](mvp/m2/README.md)。

- [x] TRACE-001 定義標準 Trace Event Schema。（`contracts/openapi` 外的第一份事件契約：[contracts/events/trace-event.schema.json](../../contracts/events/trace-event.schema.json) ＋ 同目錄 README；9 種事件、`seq` 範圍為 `(run_id, attempt, emitted_by)`、`run_lifecycle` 非事實來源、遮罩規約與版本演進規則。
- [x] TRACE-002 收集 Skill 啟用與資源讀取事件。（`infra/images/runtime-agent-sdk/run.mjs` 由 Agent SDK 訊息流產生 `skill_activation`（`Skill` 工具呼叫）與 `resource_read`（讀取路徑落在 `SKILLHUB_SKILL_DIR` 下者，路徑相對化不外洩沙箱絕對路徑）；收集端見 `apps/sandbox/internal/sandbox/trace.go`。**限制註記**：`decision: skipped`「可用但未啟用」在 SDK 訊息流中不可觀測（沒被叫到就沒有事件），本批不臆造，留給 EVAL-002）
- [x] TRACE-003 收集 Tool Call、MCP Call 與 Script Log。（`tool_call` 為單事件自帶 `duration_ms`，以 `tool_use`／`tool_result` 配對計時；`Bash` 的結果另產 `script_log`。**MCP 不實作**——遠端 MCP 已移出 MVP 首發，契約只保留型別佔位，與 TEST-005/006 同步）
- [x] TRACE-004 收集 Agent 輸出、錯誤、延遲、Token 與成本。（`agent_output`（final／intermediate）、`error`（含控制平面自身的失敗）、延遲（`tool_call.duration_ms`、`usage.duration_ms`）、Token（SDK 回報的 `input_tokens`／`output_tokens`）；orchestrator 側在 `failed`／`timed_out` 轉移的同一交易內寫 `error` 事件，因此未進到沙箱就失敗的 Run 也有可看的時間軸（RUN-004）。
- [x] TRACE-005 實作 Secrets 與敏感欄位遮罩。（遮罩在平台 ingestion **入庫前**執行，明文不落 DB；`0019` 的 `CHECK (masked)` 讓「跳過遮罩」在資料庫層不可能。已知值（本 Run 的 ingestion token）＋ pattern（`sk-*`、`Authorization` 標頭、`ANTHROPIC_AUTH_TOKEN=`／`OPENAI_API_KEY=`、預簽 URL 的簽章參數、私鑰區塊）一律替換為字面量 `[REDACTED]`，不保留長度、不做部分遮罩；`masked_fields` 以 JSON Pointer 記錄實際被替換的位置。有具名單元測試與整合測試，後者直接查 `trace_events.payload` 確認明文不存在）
- [x] TRACE-006 實作一般模式進度摘要。（`GET /runs/{id}/trace`：Skill 啟用、資源讀取次數、工具呼叫統計與最慢者、錯誤、最終輸出、Token 與成本。**Run 狀態取自 `runs` 表、進度步驟取自 `run_status_transitions`**，不重播 `run_lifecycle` 事件重建狀態（鐵律 5）；`cost_usd` 為 `null` 時顯示「未回報」不顯示 0）
- [x] TRACE-007 實作進階模式 Trace 檢視。（`?mode=advanced`：經遮罩的原始事件以不漏 commit 的 ingestion cursor 分頁、頁內依 `(occurred_at, emitted_by, attempt, seq)` 排序；需要單一跨來源時間軸的 API consumer 抓完所有頁再依同一 tuple 重建。Web 提供前後頁與手動重新整理，讓終態後才抵達的 late event 仍可取回。逐串流列出 bounded `missing_seq` sample、exact `missing_count` 與遲到計數；`complete: false` 時 UI 明示「部分事件未送達」。payload 一律以 inert text 呈現，不解讀 HTML／ANSI／SVG，有具名測試以注入 `<img onerror>` 驗證。UI 為 `apps/web` 的 `/runs/$runId`，一般／進階切換）
- [x] TRACE-008 處理事件排序、重送、缺失與延遲。（去重鍵為 producer 產生的 `event_id`，`ON CONFLICT DO NOTHING`，重送回報為 `duplicate` 而非錯誤；順序以 per-producer 的 `seq` 重建，斷號＝該事件遺失且被逐一列出；終態後仍接受遲到事件並標記 `late`，因為沙箱關機時推送的最後一批正是失敗 Run 最需要的部分。sandboxd 側推送失敗不推進水位，下一輪重送）
- [x] TRACE-009 讓 `usage` 事件不再依賴 SDK 的 `result` 訊息（[m2/README.md 丙-3](mvp/m2/README.md)、[content-baseline-report.md §7.2 #4](mvp/m2/content-baseline-report.md)）。
- [x] O11Y-001 量測搜尋、Run 排隊、建立、成功、逾時與清理指標。（Prometheus 文字格式，`apps/platform` 與 `sandboxd` 各自曝露 `/metrics`；平台側走獨立 listener `METRICS_ADDR`，不掛在對外 API port 上。指標清單見 [infra/observability/README.md](../../infra/observability/README.md)）
- [x] O11Y-002 建立 Provider 健康度與錯誤監控。（`skillhub_provider_capability_total{provider,result}` 區分 ok／unhealthy／error；`skillhub_provider_request_total{provider,operation,class}` 以狀態碼分級記 429 與 5xx；另有每操作延遲 histogram。告警規則四條見 `alerts.yml`）
- [x] O11Y-003 建立遺留 Sandbox、資源異常及安全事件告警。（`skillhub_run_cleanup_backlog` gauge、`skillhub_orphan_scan_total`、`skillhub_orphan_sandbox_total{action}` 區分「殺掉的漏網 Sandbox」與「殺不掉的」，加上遮罩器靜默失效偵測（`TraceMaskingStopped`——NFR-002 沒有其他偵測器）。告警為 `infra/observability/alerts.yml` 的 rules 檔＋文件，**Alertmanager 部署、通知路由與 Grafana dashboard 屬部署期，明確未做**；門檻值為首發預設非實測校準值，已在文件標明需上線後回填）
- [x] O11Y-004 建立核心漏斗的產品分析事件：四個新事件（搜尋送出、Skill 詳情查看、Session 開始、下載發起）的產生、儲存與查詢，其餘漏斗段以既有領域表回答。**（M4）**（承接 [m4/README.md §7 差-4](mvp/m4/README.md)：`BETA-002` 要量 `01` §11.2 的七段漏斗，而**此前沒有任何工作項承接「漏斗事件的產生與儲存」**——`O11Y-001`～`003` 量的是平台健康（聚合計數、無使用者維度），`CORE-008` 的 audit event 是合規紀錄（不含內容、400 天），拿任一者當分析來源都會同時做壞兩件事。

## 14. 評估與改善（M3）

> 本節逐項對照 `02:EVAL-001`／`002`／`003`／`013` 的允收準則與實作現況，逐項證據見 [m3/audit.md](mvp/m3/audit.md)。判定尺沿用 §9 的既有裁定（準則的主詞是「使用者可」就要有使用者碰得到的平面），本次套用結果為 **EVAL-011 不勾、其餘十一項勾選**。

- [x] EVAL-001 將驗收條件轉換為可執行或可判斷的檢查。（**「可執行」的界線見 `02:EVAL-001` 的那條準則**：平台內建的確定性檢查，**不執行使用者提供的檢查腳本**）
- [x] EVAL-002 實作規格、啟用、執行、效果、相容與成本分類。
- [x] EVAL-003 對每項驗收條件產生通過、未通過或無法判斷及證據。
- [x] EVAL-004 產生符合、部分符合、未符合或無法判斷的整體結果。
- [x] EVAL-005 清楚標示規則判斷、模型判斷與使用者判斷。
- [x] EVAL-006 實作有幫助／無幫助及文字回饋。
- [x] EVAL-007 產生包含問題、證據、位置、修改與影響的改善建議。
- [x] EVAL-008 區分 Skill、Runtime、MCP、工具及測試資料問題。
- [x] EVAL-009 實作逐項接受／拒絕與修改差異預覽。
- [x] EVAL-010 套用改善時建立新 Skill Version。
- [x] EVAL-011 實作使用相同 Test Case 重新試跑。（退回時的理由與解除條件保留於下，因為它是判定尺被套用的紀錄）：`02:EVAL-003` 第 1 條是「**使用者可**使用同一 Test Case 對新 Skill Version 重新執行」，主詞是使用者，套用 §9 的同一把尺——**M3 最主要的那條路徑上沒有介面**。
- [x] EVAL-012 實作版本、驗收、輸出、錯誤、延遲與成本比較。
- [x] EVAL-013 建立 Judge 判準的回歸集與驗證流程：以 M2 的 45 筆基準 Run 為第一組標註資料，逐筆比對 Judge 判定與已記錄的「符合／未產出」答案，產出含符合率與逐筆差異歸因的報告；Judge prompt 或 rubric 升版即重跑。

## 15. 打包與下載（M4）

> 本節九項逐項對照 `02:PACK-001`／`02:PACK-002` 的允收準則與實作現況，逐項證據見 [m4/audit.md](mvp/m4/audit.md)。判定尺沿用 §9／§14 的既有裁定（準則的主詞是「使用者可」或「顯示」就要有使用者碰得到的平面；宣稱大於實作即不勾）。本次套用結果為 **六項勾選、三項誠實不勾**（`PACK-003`／`PACK-007`／`PACK-009`）。<br>`PACK-003` 的兩個缺口補上後改判勾選，本節現為 **七項勾選、兩項不勾**（`PACK-007`／`PACK-009`）。兩項各自關閉了哪一半、還缺哪一半，逐項寫在自己的行內。

- [x] PACK-001 建立標準 Agent Skill 打包流程。
- [x] PACK-002 打包前重新執行規格驗證。
- [x] PACK-003 保留 License、作者、原始來源與衍生關係。
- [x] PACK-004 排除 Secrets、測試憑證、內部路徑與受保護資料。
- [x] PACK-005 支援選擇是否包含可散布 Test Case 與範例資料。
- [x] PACK-006 建立首批目標 Agent 安裝 Profile。
- [x] PACK-007 產生安裝位置、依賴、環境變數與驗證步驟。
- [x] PACK-008 對未驗證 Agent 顯示清楚限制。
> **`PACK-010` 這個編號從未存在。** 本節是 `PACK-001`～`009`，§19 有 `PACK-011`／`PACK-012`，**中間跳過一號**。稽核指出「一個消失的工作項與一個從未存在的號碼在文件上長得一模一樣」，而本專案有過「一個工作項寫在別的檔案裡所以沒人找得到」的前例。**本次結論是跳號**：全 repo 對 `PACK-010` 零命中，`02` 的 `PACK-*` 允收也沒有對應條目；`PACK-011`／`012` 新增時編號從 011 起跳，沒有回填 010。**不補號、不改既有編號**——編號在這份文件裡是歷史，改它會讓所有既有引用失準；這一句就是那個號碼的墓碑。

- [x] PACK-009 驗證下載套件可被解壓、重新驗證與依說明安裝。

## 16. 安全、隱私與合規（M0 起跨階段）

> 本節十項原本只有一行敘述、`02` 無 SEC-* 需求 ID（威脅模型開放問題 **Q19**）。允收準則已補於 **`02` 第 6 節「安全需求」**，取自 [m0/threat-model-and-sandbox-baseline.md](mvp/m0/threat-model-and-sandbox-baseline.md) v2 已落地的 32 條威脅與 45 項基線檢查，未新增安全要求；該文件未定的門檻值在 `02` 標為「未涵蓋（待決策）」。

- [x] SEC-001 完成 Skill、Script、MCP、Dataset、Secrets 與 Local Runner 威脅模型。（允收：`02` §6）
- [ ] SEC-002 定義 Sandbox 最低安全基線與阻擋條件。（允收：`02` §6）**部分完成**： **46 項**基線、四閘門與 fail-closed 語意已定（威脅模型 v2 §4～§5）；**六項無值語句已由[Sandbox 隔離與執行安全](../adr/README.md#sandbox-隔離與執行安全)第二部分定值**（P-03 7 天／P-04 N·N−1 且 ≤90 天＋逃逸類 CVE 24 h／I-04 30 天／I-06 可修的 Critical·High 阻擋無豁免／X-02 每 5 分鐘／X-03 連 2 輪告警、X-04 單節點 ≥50% drain 與全池 ≥25% 下限 2 筆暫停），**勾選前提 Q1～Q3 亦已定案**（同 ADR 第一部分）。（`internal/trial/execution/specification.go`）——靜態掃描等級判斷在 Create 重新掃描套件並套用 `skillpkg` 既有的 severity 政策（`error` 級 ＝ `SKILL-002` 的阻擋級），掃不成即拒（fail-closed，SEC-002「檢查無法執行視同未通過」），因此不必等威脅模型 Q7；Workspace 並行上限 ＝ 2 以交易內的 advisory lock ＋ 非終態計數強制。**仍不勾的唯一原因**：45 項基線尚未經 SEC-009 全數驗證。

- [x] SEC-003 建立 Skill 匯入與執行前靜態掃描政策。（允收：`02` §6）
- [ ] SEC-004 建立遠端 MCP 的 SSRF、內部網路與資料外洩防護。（後 MVP，隨 MCP 啟動）（允收：`02` §6）
- [x] SEC-005 建立 Secrets 儲存、短效注入、遮罩與撤銷流程。（允收：`02` §6）
- [ ] SEC-006 建立 Dataset、Trace 與 Artifact 保存及刪除政策。（允收：`02` §6）<br>**已成立**：①使用者可主動刪除 Run 輸入與 Artifact 且刪除有可追蹤狀態（`CORE-007` 已勾，`Me.deletion_scope` 是契約必填欄位）；②`cleanup_status` 是 Run 列表的一等欄位，不以最終回應成功取代清理確認；③上傳授權限定目的地、大小與有效期（`SBX-008` 已勾）。**允收沒有要求一份獨立的政策文件**，對外揭露由 `GET /policy/data-retention` 與 [consent-and-data-policy.md](mvp/gate-test/consent-and-data-policy.md) 承擔。
- [ ] SEC-007 建立來源、License 與下架處理政策。（允收：`02` §6）<br>`CONTENT-002`／`PACK-001` 已勾；第 3 條（下架不刪改既有版本與歷史 Run）成立——不可變由 `0005` 的 trigger 擋著，`Takedown` 只翻可見性且與 `RemoveFromIndex` 同交易；第 4 條（授權未知或不可散布者不得進打包）成立（`0036` ＋ `TestForkCarriesTheRedistributionVerdict`）。<br>**兩個真的缺口，性質不同**：①**第 1 條卡在 `CONTENT-004`**，而 `CONTENT-004` 卡在 `anthropics/skills` 的法務判定內容（[`05` R-4](05-pending-rulings.md)）——**「法務看完了」與「法務判定是什麼」是兩件事**；②**第 2 條的偵測方式不是允收寫的那一種**：`02` 要的是「重抓並與保存的內容雜湊比對」，而 `skill/admission/sources.go` 的 `CheckSources` 只呼叫 `Probe`（一次 `HEAD`），**所以「刪除」偵測得到，「改寫」與「License 變更」偵測不到**——`ContentHash` 就存在同一個結構裡，沒有任何一行讀它去比對。**②不等任何人簽名**，與 `CONTENT-009`／`INGEST-010` 是同一個缺口。另 `02:534` 要求的白名單異動記錄（提名人／日期／原因／涵蓋類別）今天只是 [curated-skill-list.md §6](mvp/content/curated-skill-list.md) 的散文，**沒有提名人欄**。
- [ ] SEC-008 驗證使用者與 Run 之間的資料隔離。（允收：`02` §6）<br>**已成立**：①所有使用者資料查詢預設要求 Workspace Scope 且 Scope 取自 session（`CORE-006` 已勾，`TestClientSuppliedWorkspaceIDIsIgnored` 逐字押著「不信任呼叫端傳入的識別」）；②允收要求的五條路徑各有具名整合測試並在 CI 對真實資料庫執行——私有內容列表、刪除、Fork、版本差異、公開搜尋。<br>**要節點的兩條**：③憑證範圍（跨 Run 使用短效授權、TTL 過期後使用皆須失敗）是 `SEC-009` **T6**，**今天樹裡沒有任何測試斷言它**；④P-02 常駐探針——**程式已落地**（`apps/sandbox/internal/sandbox/p02.go` ＋ `dockerdrv/p02.go` ＋ `ProviderCapability.security.p02_probe` ＋ `detectP02Breach`），**但它量的是那台機器的網路政策，而它從未在生產節點上跑過**。**「探針存在」不是「連線嘗試被實際阻擋」**。
- [ ] SEC-009 進行 Sandbox 逃逸、資源濫用與權限提升測試。（允收：`02` §6）**驗收程序已定案、測試未執行**：[Sandbox 隔離與執行安全](../adr/README.md#sandbox-隔離與執行安全)第三部分是部署批實際照著跑的那份清單，內容為 **10 個測項**——T1 逃逸 PoC 集／T2 syscall 煙霧 fuzz／T3 資源耗盡／T4 Runtime 相容性（46 個精選 Skill 在 `runsc` 上重跑，約 $3.4）／**T5 網路外洩八子項**（DNS tunneling、內網掃描、Metadata Service、**東西向**、**節點面**、N-07 供應商網域、**T5-7 閘道 `pinned_ip` 的非允許 port**、**T5-8 自外部掃節點 port**）／T6 憑證範圍／**T7 清理失敗與遺留門檻（含人工注入假遺留資源——否則 X-04 的 drain 與暫停路徑沒有任何測項會執行到）**／T8 宣告式供應鏈稽核／**T9 Provider 契約（加 I-05 與 `egress.allow` 不符須回 `capability_mismatch` 兩項斷言）**／**T10 P-02 常駐探針**（入池前一次＋入池後常駐）。<br>**分兩個 suite**：Suite 1 可在一般 Linux runner 跑（**gVisor 的 `systrap` 平台不需巢狀虛擬化**，只有 `--platform=kvm` 才需要——[Repo 結構、CI 與驗證層](../adr/README.md#repo-結構ci-與驗證層)待決策 3 的範圍因此縮小，**待部署批第一台節點實測確認**）；Suite 2 需生產同規格節點。<br>**前置條件三條（缺一即判 `unknown` ＝ fail）**：①受測 Runtime Image 已發佈至 **GHCR 且附 SBOM 與掃描 attestation**（SBX-011）；②`infra/nodes/gvisor-baseline.txt` 已填實際版本（非 `unset`）；③`infra/egress/allowlist.yaml` 的 `tier: sandbox` 條目已填實際 `pinned_ip`，且該位址**不是控制平面節點**（Q2 強制條件 6）。<br>**通過判準**：46 項全數 pass、**0 項 unknown**，任一 fail 或 unknown 即不得開放外部使用者提交 Skill 執行，**無例外流程**。<br>**證據**：`plans/mvp/m4/sec-009-acceptance/<日期>-<節點>/`（判定表與 `versions.txt` 進 repo，原始輸出留 CI artifact 並附連結），保存 ≥ 1 年。
- [ ] SEC-010 完成安全事件回應與緊急停用 Provider 流程。（允收：`02` §6）未定的三項（嚴重度分級、誰接最高級告警、遮罩失敗補救程序）已寫成**提案**置於 `02:SEC-010` 的待決策區——P1／P2／P3 分級表（P1 的第一動作為自動停止派送，不等人）、GitHub issue ＋ email 的通知路徑（承認單人團隊、不採購值班輪替系統）、遮罩失敗 runbook（撤銷 → 清除 → 通知 → 事後，含必補回歸測試）。該提案為本需求的允收準則（威脅模型 **Q17** 一併結案；值班輪替待團隊有第二人時重開）。**本項仍不勾的原因**：核可的是分級與回應方式，**一鍵停用流程本體、runbook 文件與 P1 的自動第一動作都尚未產出**——自動動作由新增的 `SEC-012` 承接，runbook 與一鍵停用流程仍在本項。<br>①**一鍵停用流程本體存在**——`0030_dispatch_halts.sql`、`GET /admin/dispatch`、`PUT`／`DELETE /admin/dispatch/halt`，掛 `RequireOperator`，三個動作齊備（停派送兩個進入點、drain、保留現場），已在 `contracts/openapi/public.yaml` 宣告；②**runbook 存在且不是骨架**——[p1-dispatch-halt.md](../runbooks/p1-dispatch-halt.md) 有開關是什麼、怎麼確認現在停著、逐偵測器分辨真事故與誤觸（附可跑的 SQL）、解除前三個檢查、一次真實誤觸的原樣紀錄，以及明寫的「不涵蓋」清單；③**P1 自動第一動作**早已由本項自己改派給 `SEC-012`。<br>**真正剩下的兩件，而且只有第一件擋著勾選**：<br>**(a) 通知路徑一格都沒接。** 核可的 §(2) 要求每個 P1／P2 開一個帶 `sev/*` label 的 GitHub issue、並且 P1 要有一條非工作時間送得到的推播；`infra/observability/` 只有 Prometheus 規則，**全樹沒有 Alertmanager 設定，也沒有任何程式會開 issue**。**單人團隊裡送不到那個人的最高級告警等於沒有告警**——這正是 `RELEASE-006` 也不勾的同一個理由。屬部署期（要真實 endpoint 與憑證）。<br>**(b) 觸發條件那一句與核可的分級表衝突，已就地更正**：本文件原句把「清理長期失敗」列為一鍵停用的觸發條件，而分級表把「清理連續失敗」放在 **P2** 並逐字寫「不停整個平台」。runbook 的五條 P1 判準與程式都站在分級表那一邊，**三處一致、只有那一句不是**，所以更正的是那一句。
- [ ] SEC-011 實作平台 operator 角色與跨 Workspace 下架權限。（`02:SEC-011` 的追加小節）——負責人裁定對 `anthropics/skills` 的 4 筆（`docx`／`pdf`／`pptx`／`xlsx`）執行 [governance/anthropic-sa-license-memo.md](mvp/governance/anthropic-sa-license-memo.md) 方案 C：migration **`0023`** 的 `skills.access_restriction`（NULL ＝ 無限制，非 NULL ＝ 原因碼，現行只有 `license-review`），`/files` 回 403 附理由、Run 建立回 422、詳情頁顯示受限說明並收起進階連結、**搜尋與摘要照常**；旗標於 Fork 當下複製，未知原因碼 fail-closed 視為受限。逐筆設定見 `tools/content/restrict-anthropic-sa-display.sql`，測試見 `disc_integration_test.go` 的 `TestLicensingHoldClosesTheMaterialsAndKeepsTheListing`／`TestARunOnHeldMaterialsIsRefused` 與 `disc.test.tsx`。動作窮舉為三項——變更目錄項可見性、停用 Skill Version、白名單異動；**operator 不得讀取任何 Workspace 私有資料、不得代表使用者發起 Run、不得改寫既有 Skill Version 與歷史 Run**（最小權力原則）。每個動作與角色授予本身都寫 audit event，理由為必填；`member` 呼叫 operator 端點回 404。下架流程與 `CONTENT-009`／`INGEST-010` 共用同一組狀態，不另開第二套。<br>**(a) operator 身分**：部署設定 `OPERATOR_USER_IDS`（逗號分隔 user id）＋ `identity.RequireOperator` middleware；不在清單內（含未登入）一律 **404**，不是 401 也不是 403——照 `02:SEC-011`「不揭露端點存在」。**刻意不做 DB 角色表**：單人團隊裡，角色表要連帶做授予端點、該端點自己的授權、以及授予的稽核，三者存在的唯一目的是讓同一個帳號提拔自己；授予＝改設定重啟，不會部署的人本來就做不到。第二人出現時的升級路徑是「這裡改讀角色表」，路由與 handler 不動。<br>**(b) 授予入稽核（最小形式，限制寫明）**：`cmd/api` 每次啟動寫一筆 `operator.roster` audit event（生效清單＋筆數）。它答的是「誰現在是 operator」，**不是「誰在何時授予」**——後者的事實在部署設定的變更歷史裡。寫不成這筆事件時該次啟動**不承認任何 operator**（fail-closed）。<br>**(c) 端點**：`PUT`／`DELETE /admin/skills/{id}/restriction`。PUT body 需 `reason`（必須是平台認得的原因碼，目前只有 `license-review`——**未知碼在寫入端擋掉，在讀取端仍 fail-closed 視為受限**，兩條規則方向相反且都需要）＋ `note`（必填）；DELETE 需 `note`。兩者皆冪等（重設回 200 並回報前一狀態、解除不存在的限制回 204），且**兩種情況都照樣寫 audit event**——操作者做了這個動作本身就是要留的事實。audit metadata 為 `before`／`after`／`note`，actor 取自 session，與欄位變更**同一交易**（鐵律 9）。<br>**(d) 唯一的跨 Workspace 寫入**：`LockSkillForOperatorWrite`（原名 LockSkillForRestriction）／`SetSkillAccessRestriction` 是全樹僅有的兩條不帶 workspace 述詞的 skills 語句（授權事實屬於來源，必須同時涵蓋目錄項與其 fork）；範圍窄到只有一個欄位、一列、以主鍵定址，不回傳任何套件內容，且只有 operator 路由到得了。整合測試 `operator_integration_test.go` 覆蓋：非 operator（匿名與 member）全 404 且資料未變、operator 設定後 `/files` 立即 403 而搜尋與詳情照常、audit 事件的 before／after／note、解除後 `/files` 回復 200、兩個方向的冪等，以及**operator 身分不擴充 Workspace Scope**（讀他人私有 Skill 仍 404）。<br>**仍不勾的原因（三項）**：①窮舉的動作只落地 ①的較輕等級「授權受限展示」，②停用 Skill Version 與 ③白名單異動**未實作**（兩者都還沒有可驅動的機制，先開端點只是承諾）；②授予的稽核只到「清單」粒度，不到「誰在何時授予」；③`contracts/openapi/public.yaml` 尚未宣告這兩個路由（additive，待契約批，同 `TEST-011` 前例）。**`apps/web` 不做管理 UI**（裁定：單人以 curl 操作，範例見 `02:SEC-011` 追加小節）。
- [ ] SEC-012 實作 P1 安全事件的**自動第一動作**：偵測到 P1 判準即**立即停止派送新 Run**、drain 受影響節點、保留現場，不等人接告警。（允收：`02:SEC-010`）<br>**P1 判準取自 `02:SEC-010` 分級表，不在此另訂**：逃逸疑慮；P-02 探針偵測到 Sandbox → 核心資料庫連線；`TraceMaskingStopped`；隔離技術逃逸類 CVE 揭露；Reconciler 停擺 > 10 分鐘。升級規則同樣取自該表——**不確定是 P1 或 P2 時一律以 P1 處理**。<br>**與 X-04 的暫停機制同源（這是實作要求，不是註記）**：X-04 的 slot drain 門檻與 P1 的「停止派送」是**同一個開關的兩個觸發者**，必須共用同一份狀態與同一條解除路徑。做成兩套的後果可預期：一邊暫停、另一邊以為自己還在派送，而「現在到底有沒有在派送」會變成沒有單一答案的問題。<br>**已成立**：`dispatch_halts` 一張表承載兩個觸發者，唯一部分索引讓第二次宣告是 upsert 而非疊加；五條判準裡**自動接上三條**——`TraceMaskingStopped`（掛在既有 sweep 的尾巴，門檻照抄告警規則含 `for`）、Reconciler 停擺（掛在 API 而非 worker，因為停擺的 worker 會把自己的看門狗一起帶走）、P-02 探針（節點在既有的被呼叫端多報一個欄位，契約仍單向）。另有遮罩 canary：把平台自造的每一種 Secret 形狀餵過真正的 Masker，任何一個沒被遮就宣告 P1。**解除一律人工**——自動解除等於讓觸發條件自己決定何時恢復服務。<br>**不勾的理由**：①逃逸疑慮是人的判斷，**「疑慮」不是一個訊號**；④逃逸類 CVE 揭露的訊號在 CI 看得到而到不了生產資料庫。兩者維持人工宣告。**本項字面是「偵測到 P1 判準」，五條裡自動三條不等於做完**；且 P-02 探針**從未在生產節點上跑過**——它量的正是那台機器的網路政策。<br>**一個實跑發現**：`TraceMaskingStopped` 若省掉 `for` 只留 expr，低流量下會誤觸——**在告警裡只是吵人，接上停派送就會自己把自己停掉**。同一個閾值在兩種消費端下的代價差一個量級，照抄告警規則時 `for` 不是可以省的裝飾。
- [ ] SEC-013 **OWASP Top 10 for LLM（2025）對照與注入攻擊集**。負責人同日點名；對照表 [m0/owasp-llm-top10-mapping.md](mvp/m0/owasp-llm-top10-mapping.md) 與威脅模型 [§2.10 A10 互動創作](mvp/m0/threat-model-and-sandbox-baseline.md) 同批寫好——盤點結果：LLM01（創作迴圈沒有圍欄、沒有攻擊集）、LLM04（投毒 × Re-Use 一鍵採用、畫面缺層級與揭露）、LLM02（會話快照沒有遮罩證據）三項是真缺口；LLM03 模型未釘版本與 LLM10 平台級煞車要決策（`05` R-51）；LLM05／06 各一條小工作；LLM07／08／09 無新缺口。攻擊集、提示層圍欄、投毒題、會話遮罩與反證測試都已落地，注入紅線改由 Go 守門重放在 CI 裡判（見 `02` SEC-013）；**不勾**的是同意書互動創作那一列的法務確認（`04` 乙-16）。承接 `04` 丙-179。

## 17. 品質保證與封閉測試（M4）

> 本節十四項逐項對照實作現況，逐項證據見 [m4/audit.md](mvp/m4/audit.md)。**判定尺與 §15 不同，必須先說明**：`02` **沒有 QA-\* 需求 ID**（同 SEC 在 §6 補上之前的狀況），所以 `QA-001`～`009` 只能以**工作項自己的字面**判定——「建立……測試」＝ 該類測試存在且具名且跑得起來。若某一項的字面已被別的允收準則接住（`QA-009` ＝ `02:NFR-007`、`QA-007` ＝ `02:PACK-001`），以該準則為準。<br>**`BETA-001`～`005` 的判定尺是另一把**：它們量的是一次**真實發生過的封測**，程式面完成不等於這幾項完成——`BETA-001`／`BETA-005` 另有 PDM-009 未追認的硬前提。<br>本次套用結果為 **四項勾選（`QA-002`／`003`／`004`／`005`）、十項誠實不勾**。<br>`QA-001` 與 `QA-007` 的不勾理由各自關閉後改判勾選，本節現為 **六項勾選、八項不勾**。**上方那一行是對帳當下的帳，不改寫**。<br>`QA-009` 的第三個不勾理由關閉後改判勾選，本節現為 **七項勾選、七項不勾**（同批的 `DESIGN-013` 在 §3）。**`QA-008` 仍不勾**，但不勾的理由縮成一個——只剩 OS 矩陣，路線決策見[Repo 結構、CI 與驗證層](../adr/README.md#repo-結構ci-與驗證層)。**先前幾行的帳一律不改寫。**

- [x] QA-001 建立核心旅程端到端測試。
- [x] QA-002 建立 Agent Skills 規格驗證測試資料集。
- [x] QA-003 建立正常、失敗、逾時、取消與清理 Run 測試。
- [x] QA-004 建立 Dataset 與 Secrets 權限測試（MCP、Local Runner 部分隨後 MVP 啟動）。
- [x] QA-005 建立 Trace 完整性與敏感資訊遮罩測試。
- [x] QA-006 建立評估可解釋性與改善版本不可變測試。
- [x] QA-007 建立打包、License、來源及 Secrets 排除測試。
- [x] QA-008 完成主要瀏覽器與目標作業系統測試。
- [x] QA-009 完成鍵盤操作、文字標籤與色彩以外狀態提示檢查。
- [ ] BETA-001 招募目標個人創作者進行封閉測試。（<br>**程式面已完成**（[身分、Workspace、准入與額度](../adr/README.md#身分workspace准入與額度)決策 1）：`identity/http.go` 的 `RequireInvited` 疊在 `RequireSession` 之上，只掛在 Fork／建 Run／下載內容三條路由；未受邀回 **403 ＋ 說明 ＋ 指向 `POST /feedback`**（**刻意不是 404**，與 operator 路由的判準相反且寫明理由：operator 端點的存在該是秘密，封測的不是，而同一個 API 剛剛才把目錄服務給這個人）。`BETA_ALLOWLIST` 空 ＝ 閘門關閉；`LogInviteRoster` 每次啟動寫 `beta.roster`，**寫不成則誰都進不來**——與 operator roster 的 fail-closed **方向相反**（空的 operator 清單不授權任何人，空的邀請清單卻會放所有人進來）。`DEV_LOGIN` 沒有例外路徑。測試 `TestAdmissionListGatesForkRunAndDownloadOnly`／`TestNoAdmissionListMeansNoGate`。<br>**不勾的三個硬前提**：①**PDM-009 未追認**（人數、通路、報酬、門檻）——追認前本項不可判定；②**同意書未經法務確認**（[gate-test/consent-and-data-policy.md](mvp/gate-test/consent-and-data-policy.md) 為草稿），沒有東西可以隨確認信寄出；③**招募、篩選、名單填入部署設定並重啟**全部是人的動作（[m4/README.md §5.2 H-8](mvp/m4/README.md)）。<br>**招募材料本身可整套重用**（[gate-test/recruit.md](mvp/gate-test/recruit.md)），要改的只有框架與 Q5，見 [m4/pdm-009-beta-proposal.md §3](mvp/m4/pdm-009-beta-proposal.md)）
- [ ] BETA-002 量測搜尋到詳情、詳情到試跑、試跑到下載的漏斗。（<br>依賴 `O11Y-004`（同樣未勾）。**三件事各自缺**：①**跨表漏斗查詢不存在**——全 repo 沒有任何漏斗報表 SQL、腳本或 harness，而 `01` §11.2 的七段有五段要從既有領域表（`runs`、`evaluations.feedback_helpful`、`evaluation_suggestions.decision`、`download_records`）拼出來；②**`ANALYTICS_RETENTION` 未定值時平台一列都不收**（依 `NFR-002` 刻意如此），而該值隨 PDM-006 追認，**所以在追認之前這個漏斗結構上量不到任何東西**；③**沒有受測者**——封測未開始。<br>**精度限制在量之前就要寫進報表**（`02:O11Y-004` 最後一條）：前兩段以 session 為單位，同一個人跨裝置算兩個，第一段的分母系統性略高；百分比讀作量級）
> **Bounded Context 的邊界收斂已全面落地**：跨 context 的 read 一律走 Facts 介面或由呼叫者注入，裸 SQL 只剩具名技術豁免，`allow:`／`read_allow:` 均為空。現行規則見 [Platform Bounded Context 與 Context Map](../adr/README.md#platform-bounded-context-與-context-map) 與 [Query 與寫入所有權](../adr/README.md#query-與寫入所有權)。
>
>
>
>
>
>
>
>
>
>
>
>
>
>
>
>

>
>

- [ ] BETA-003 蒐集評估報告與改善建議的質性回饋。（<br>**已有**：逐次評估的有幫助／無幫助＋文字（`EVAL-006` 的 `PUT /runs/{id}/evaluation/feedback`，**已有 UI**）；`POST /feedback` 端點與 `feedback_reports` 表（`0029`，`kind` 兩值、2000 字上限與契約一致、`page_path` 帶 query string 一律丟棄不拒絕、他人的 `run_id` 丟棄不拒絕——**寧可少一個 context 欄位也不要弄丟整份回報**）。測試 `TestFeedbackIsRecordedWithWorkspaceScope`／`TestFeedbackRejectsWhatTheContractRejects`。<br>**缺的兩件**：①**`apps/web` 全樹沒有任何呼叫 `/feedback` 的程式碼**——[beta-design.md §5](mvp/m4/beta-design.md) 要求的「全站可及的入口」在畫面上不存在，端點目前只有 curl 到得了（見 `BETA-004`）；②**沒有任何一筆質性回饋被蒐集過**，本項的動詞是「蒐集」不是「建管道」）<br>入口是 `FeedbackEntry.tsx`，掛在 `router.tsx` 的版面**頁尾**、每一頁都在（`<details>` 收合）。**放頁尾不是版面偏好而是鍵盤順序**：它是一個裝著整張表單的收合區塊，放在 `<main>` 之前會卡在每一頁與該頁第一個控制項之間。**只送使用者看得到的東西**：`kind` 由使用者自己選（`blocking_issue`／`need_signal` 兩種問的是不同的事，平台從外面分不出來）、`page_path` **去掉 query string**（beta-design §4.2 界線 2；`feedbackRunID` 也先去掉再比對，否則帶 `?tab=` 的網址會安靜地丟掉 Run context）、`run_id` 只在網址本身指名某個 Run 時帶上，**三者都印在表單旁邊**——沒有截圖、沒有 console、沒有自動抓畫面。204 沒有 id，所以確認文案明講「沒有回覆機制、沒有查詢頁面」。送不出去時**內容留在框裡**並說下一步。測試 `feedback.test.tsx` 六支）
- [ ] BETA-004 記錄所有阻斷首次成功旅程的問題。（理由同 `BETA-003` 的兩點，另加一條本項專有的：**「阻斷」的判定尺需要漏斗事件與阻斷回報用同一個 session 識別串起來**（[beta-design.md §5](mvp/m4/beta-design.md)：一個問題算阻斷的條件是「使用者在該段停下且沒有自行繞過」）。`session_started` 事件與 `sh_analytics` cookie 已存在，**但把兩者對起來的查詢不存在**，與 `BETA-002` 的第①點同源）
- [ ] BETA-005 依封閉測試結果完成 MVP 範圍及優先級複審。（<br>前置為 `BETA-001`～`004` 全數完成 ＋ PDM-009 的三條門檻判定（[m4/pdm-009-beta-proposal.md §4](mvp/m4/pdm-009-beta-proposal.md)：B1 完整旅程完成率 ≥ 8／12、B2 漏斗前六段無一段低於 50%、B3 整層失效或同一問題撞到 ≥ 4 人即否決）。**不通過的處置決策樹已備妥**（該文件 §4.4，分類軸 ＝ 漏斗哪一段斷掉），**重測上限建議一次**——事後定門檻等於沒有門檻，事後才想「不通過怎麼辦」也一樣。<br>需求訊號的蒐集管道與 `BETA-003` 共用同一個表單（`kind = need_signal`），這是 `PDM-010` §8.1 的既有設計）

## 18. MVP 發佈檢查（M4）

> **`RELEASE-*` 的不勾理由逐項寫在各列。** 一個讀者看到一行「未完成」會以為它還沒開始，而實際上它在等的是別的東西。
>
> **逐項的「誰做什麼驗什麼」與執行順序見 [m4/release-checklist.md](mvp/m4/release-checklist.md)**；那份是說明與順序，**勾選狀態的唯一事實來源仍是本節與 [04-backlog-and-handoffs.md](04-backlog-and-handoffs.md)**。
>
> **共同的三個阻擋**（不逐項重複）：①**甲類四項未到期**（`SEC-009` **46 項**全 pass 0 unknown、`SBX-010`、`SBX-005`／`007` 生產網路面、`SBX-002` 閘門 A 探針）——依 [m4/README.md §4](mvp/m4/README.md) 的裁定它們在 M4 是封測的阻擋項；②**六項 PDM 未逐項追認**（`03` §1）；③**M1 驗證閘門的 D 日未宣告**——[m4/README.md §5.3](mvp/m4/README.md) 已寫明「**在一個未結案的閘門上宣告 MVP 完成，是把兩個相反的結論同時寫進同一份文件**」。

- [ ] RELEASE-001 所有 MVP 必要需求都有對應測試與結果。（**不勾**：（`QA-008` 於本日隨 `05` R-15 裁定勾選），**所以本項的第一個不勾理由已消失，第二個沒有**——且本項要的是一份**需求 ID × 測試**的對照表，目前不存在——`QA-003`／`004`／`005` 的測試分別掛在 `RUN-*`／`SBX-*`／`TRACE-*` 名下，沒有一份把它們對回需求。**那份對照表本身就是本項的交付物**，不是別項的副產品）
- [ ] RELEASE-002 完整核心旅程通過端到端驗證。（**不勾**：`QA-001` 未完成——旅程六段各有測試，沒有一支走完全程（接縫看不到）。另需真機部署上由**一位真人**走一次（`02` §7 DoD 第 1 條的主詞是「一位新使用者」），那屬封測）
- [ ] RELEASE-003 精選 Skill 內容、來源、License 與驗證狀態完成檢查。（**不勾**：`CONTENT-003`／`CONTENT-004` 仍未勾，卡在 `anthropics/skills` source-available 的**法務終判**。**打包上線讓這一項多了一個新的可驗證條件**：[beta-design.md §8](mvp/m4/beta-design.md) 第 11 項要求「至少一個 `documents` 類精選 Skill **可下載**」——那四筆 source-available 正是該類最好的樣本，而 PDM-002 早已要求補 2–3 個 OSI 授權的替代品，**那是 `documents` 類的必要條件不是加分項**）
- [ ] RELEASE-004 Sandbox 與 Secrets 安全檢查完成（MCP、Local Runner 隨後 MVP 啟動時補檢）。（**不勾**：`SEC-003` 與 `SEC-001` 都已勾（隨 [`05` R-13](05-pending-rulings.md)、R-14）；**仍未勾的是 `SEC-002`**（要節點），`SEC-009` 的十個測項沒有一個被記為通過（甲-1）。原文寫「十個測項一個都沒跑」，而 T1、T2 與 T8 的映像半都跑過了，證據在 [`m4/sec-009-acceptance/`](mvp/m4/sec-009-acceptance/)——**「跑過」與「記為通過」不是同一件事**，那份判定表逐列寫的是有沒有證據（17／4／25），不是 pass／fail。**無例外流程**——`02:SEC-009` 明文基線全數 pass 且 0 unknown 才放行，任一 fail 或 unknown 即不得開放外部使用者提交 Skill 執行；**基線是 46 項**)
- [ ] RELEASE-005 資料保存、使用者刪除與稽核流程通過驗證。（**不勾**：這一格是三個條件的 AND。稽核（`CORE-008`）與刪除流程（`CORE-007`）都已勾，保存期限也都有值且由 `retention-floor` 守住下界；缺的是保存與刪除政策這一項本身——`SEC-006` 仍未勾）
- [ ] RELEASE-006 Provider 故障、Run 逾時與清理失敗有可操作處理方式。（**不勾**：機制與測試面已相當完整（`QA-003` 已勾、`SEC-012` 的停派送開關已落地、`O11Y-002`／`003` 已勾），缺的是**「可操作」的兩半**——①`SEC-010` 的 runbook 文件與一鍵停用流程本體未產出；②**Alertmanager 部署與通知路由未接**（`O11Y-003` 自陳的部署期缺口），而單人團隊裡送不到人的最高級告警等於沒有告警）
- [ ] RELEASE-007 使用者可理解執行權限、評估依據與相容性限制。（**不勾**：權限（`TEST-005`／`008`／`009`／`011`）與相容性（[打包、授權溯源與散布](../adr/README.md#打包授權溯源與散布)三層、`0022` 的量測列）兩項已落地，**評估依據那一項有 `QA-006` 記的破口**——`artifact` 型引用的引文從未回驗（`QA-006` G7），使用者會看到一個看起來像依據的東西。**拍板前這一格不能勾**）
- [ ] RELEASE-008 下載套件可安裝且不包含受保護資料。（**不勾**：後半（不含受保護資料）已由 `PACK-004` 成立並對匯出位元組斷言；**前半（可安裝）正是 `PACK-009` 未勾的那一半**——`claude-code` 從來沒有人實際裝過，`standard` 的 round-trip 測試缺席。另 `QA-007` 的 License 檔隨包斷言缺席，`PACK-003` 同源）
- [ ] RELEASE-009 完成封閉測試成功門檻並處理阻斷問題。（**不勾**：封測未開始。**PDM-009 未追認前本項結構上不可判定**——三條門檻（B1／B2／B3）與否決條款是它定的。「處理阻斷問題」另需 `BETA-004` 的紀錄，而那需要漏斗事件與阻斷回報能用同一個 session 串起來）
- [ ] RELEASE-010 發佈決策、已知限制與下一階段範圍完成記錄。（**不勾**：純負責人動作，且前置是 `BETA-005` 的範圍複審。**「已知限制」的素材已經齊備且不必等封測**——[m4/audit.md](mvp/m4/audit.md) 的不勾清單、[`04`](04-backlog-and-handoffs.md) 的三類殘項、各 ADR 的「待決策」章節三者合起來就是它；缺的是把它們收斂成一份對外的限制清單，以及發佈與否的那個決定）

## 19. Skill 生成與互動創作（M5）

**目前範圍：單次生成基礎已完成，互動創作規劃已同意、尚未實作。** 完成數見 §19.1；下列帶日期的舊狀態只描述當時的單次生成範圍。實作放行與曝光邊界只依 `01` §10。

> 依據[從描述生成 Skill](../adr/README.md#從描述生成-skill)，允收準則見 [`02` §4.9](02-specifications-and-acceptance-criteria.md)（`GEN-001`～`004`）。本節**不在 MVP 的完成度計算內**——與 §12「Local Runner Beta（後 MVP）」同一種收納方式。<br>**本節另有兩個非 M5 的工作項**（`PACK-011`、`EVAL-014`，夾在 `GEN-007` 與 `GEN-008` 之間），它們是跨里程碑項目，**不計入本節的勾選數**。
>
> **啟動條件三項（封測結束、漏斗第一段有讀數、生成品質基線）已由[從描述生成 Skill](../adr/README.md#從描述生成-skill)暫時放行，`GEN-001`～`011` 全部可做。** **放行它的是授權不是證據**——[ask-5.md](mvp/m5/ask-5.md) 一次都沒執行，那三項的事實一件都沒有變。
>
>
>
> **⛔ 放行沒有授權曝光，這是本節的第二條硬邊界。** `GEN-008` 的生成入口**不得對封測使用者出現**，直到條件②的讀數存在為止；旗標預設為關。封測使用者一旦看得到「搜不到 → 生成一個」，`01` §11.2 第一段量到的就不再是搜尋好不好，而**那個數字只有一次機會、封測只有 12 個人**。
>
> **本節刻意不重複既有機制的工作項。** 生成物一旦寫成版本，`SKILL-002`／`SKILL-003`／`WS-001`／`WS-002`／`TEST-*`／`RUN-*`／`EVAL-*`／`PACK-*` 對它一體適用；本節只列「生成那一步」新增的東西。

- [x] GEN-001 在 `contracts/openapi/llm-internal.yaml` 定義 `POST /v1/generate-skill`。
- [x] GEN-002 在 `apps/llm` 實作該端點。
- [x] GEN-003 在 Go 側實作生成流程：呼叫 → 打包成 zip → 走 **admission 的同一條驗證路徑**寫版本。任一阻擋錯誤即整組拒絕、不建版本（與 `apply.go` 的 `validatePatched` 同一把尺）。**重試恰好一次**，同一個 prompt、同一個模型、不加修正提示；**平台不修改模型交出來的位元組**（[從描述生成 Skill](../adr/README.md#從描述生成-skill)決策 1）。截斷（`finish_reason == "length"`）是**另一個失敗類別，不套用這條重試**（決策 2）。**不引入 agent framework**——那個「for」現在有了上限，它就是 1。（允收：`02:GEN-003`）

**未涵蓋（留給後續工作項）**：對外的 HTTP 端點與 UI 在 `GEN-008`（受曝光旗標保護），配額強制點在 `GEN-004`。
- [x] GEN-004 生成前的成本預估與配額強制點：套用[身分、Workspace、准入與額度](../adr/README.md#身分workspace准入與額度)既有的強制點，額度不足時**在呼叫模型之前**拒絕。**額度以「一次生成」為單位扣抵不以呼叫次數**、**失敗不扣**、**與 Run 的額度分開計數**。（允收：`02:GEN-001`）
- [x] GEN-005 `skills.redistribution` 新增第五個值 `generated`。
- [x] GEN-006 生成物的來源紀錄與呈現：保存任務描述原文、生成時間、提示詞版本與模型識別，詳情頁顯示為「由平台依你的任務描述生成」並可展開。**不得顯示為未知來源。**（允收：`02:GEN-002` 第 1 條）
- [x] GEN-007 生成物不進公開目錄也不進搜尋索引，包括生成它的人自己搜尋時；工作區 Skill 列表是唯一入口。`tier` 不新增值域，`DISC-002` 的來源層級篩選不受影響。（允收：`02:GEN-002` 第 5 條）（**這一項的驗證方式是反證測試**：以生成物的關鍵字搜尋，斷言自己也搜不到——它屬於「壞掉的時候沒有任何症狀」那一類）
- [x] PACK-011 打包**之前**告訴使用者這包東西會保存多久。
- [x] PACK-012 產出的套件上限與匯入上限解耦。
- [ ] PACK-013 把平台對這個 Skill 的理解交給下游。（索引時的 LLM 增強每個版本花一次模型呼叫產出白話摘要與任務範例句，而**它的全部價值今天只兌現在平台內的搜尋列上**——使用者帶走的套件、以及他的 Agent 讀到的，仍然是作者原本那句 `description`。內容：(b) `skillhub-manifest.json` 加一個 `understanding` 區塊（摘要、任務範例句、生成時間、提示詞版本）；(c) `INSTALL.md` 加一句「這個 Skill 適合的任務」。**兩處都必須標明它是平台生成的、不是作者寫的**（缺席與來源詞彙照[設計系統、信任訊號與畫面用語](../adr/README.md#設計系統信任訊號與畫面用語)），且**不得進 frontmatter**——[打包、授權溯源與散布](../adr/README.md#打包授權溯源與散布)／PDM-008 禁止改變任務意圖，該禁令在 R-28 被逐條確認未被時間侵蝕。**前置（寫死，不是估時）：M1 驗證閘門的讀數。** 閘門量的正是「平台的理解對人有沒有用」，而如果那個讀數說沒有用，把它送到下游只是把沒有用的東西送得更遠。允收：`02:PACK-001`、`02:GEN-004` 的同一條紀律）
- [x] EVAL-014 重評時區分「Artifact 已過期」與「Artifact 從未被記錄」。
- [x] GEN-008 UI（**受曝光旗標保護，預設為關**）：無結果狀態的生成入口、生成中的進行中狀態（`InFlight` 既有元件，設計系統 §2.12）、失敗時的兩條出路、詳情與列表上「沒有經過任何人工檢視、沒有任何試跑證據」的說明（措辭適用[設計系統、信任訊號與畫面用語](../adr/README.md#設計系統信任訊號與畫面用語)的缺席詞彙）。**首頁不提供與搜尋對等的生成入口。**（允收：`02:GEN-004`）
- [x] GEN-009 生成品質基線：跑一批真實生成，記四個數——①第一次就通過 `skillpkg.Validate` 的比率、②被擋下來的錯誤分布、③生成物完成第一次試跑後的評估判定分布、④人看了會不會留著（這一項要人，前三項不用）。**◐ (甲)(乙) 已先跑掉** → [m5/report-generate-baseline.md](mvp/m5/report-generate-baseline.md)：**6／20 至少失敗一次、2／20 兩輪都失敗**（推翻[從描述生成 Skill](../adr/README.md#從描述生成-skill)「純隨機」理由，結論仍成立）；**mini 通過 19／20、便宜 21.4 倍**（依[從描述生成 Skill](../adr/README.md#從描述生成-skill)改預設）；**`possible-secret` 三輪 0／59**。
- [x] GEN-010 資料保存政策補上生成這一類資料：任務描述原文的保存期限與刪除行為。（**任務描述是使用者主動提交的自由文字**，與 `O11Y-004` 的分析事件「不記查詢原文」是兩件事，不得互相援引）（允收：`02:GEN-002`、`SEC-006`）
- [x] GEN-011 生成物可作為 `WS-001` Fork 的來源，Fork 後的版本逐字繼承 `redistribution = generated`（依[從描述生成 Skill](../adr/README.md#從描述生成-skill)）。**不寫阻擋規則**——Fork 對工作區自己的內容本來就通，擋它才要新增規則、錯誤碼與一句解釋。**要有一支具名測試**：繼承若失效，值退回 `unknown` 會把下載鎖回去，而那個失效沒有任何症狀（同[打包、授權溯源與散布](../adr/README.md#打包授權溯源與散布)的 `TestAnUploadIntoTheCatalogueIsNotSelfSupplied` 是同一類）。（允收：`02:GEN-002`）

- [x] GEN-012 契約與資料層：public／internal 兩份 OpenAPI schema 加 `diagram`（`GenerateDiagram`）與 `reference_skill_ids`；migration `0055` 的 `skill_sources.generation_inputs jsonb`（nullable）與 sqlc `CreateSkillSourceParams.GenerationInputs`；三個 one-number 常數（`generateMaxDiagramBytes` 4000000、`generateMaxReferences` 3、`generateMaxReferenceChars` 20000）逐處落地並被 `devctl automation-check` 機械比對。（允收：`02:GEN-005`、`GEN-006`）測試。
- [x] GEN-013 Go：`GenerateInput` 收三種可選欄位（至少一個 `task_description`／`diagram`）、`diagram` 的 base64／媒體類型／解碼大小驗證（400）、參考 Skill 的 scope／takedown／`access_restriction`／`redistribution=blocked` 拒絕規則（單一錯誤形狀 422，**不指名 id 也不說原因**，理由在 `02:GEN-006`）、`generation_inputs` 的 provenance 寫入（僅 diagram 或 references 有使用時才寫，圖片位元組本身不落地，只記 sha256／media_type／bytes 與參考的 skill_id／version_id／name）。（允收：`02:GEN-005`、`GEN-006`；依據[從描述生成 Skill](../adr/README.md#從描述生成-skill)）測試：至少一個輸入的 422、超過 3 個參考的 422、參考不可讀／已下架／受限／`blocked` 各自的拒絕與訊息、`generation_inputs` 只在使用時非 NULL、圖片位元組不進任何資料庫欄位或物件儲存的反證測試。
- [x] GEN-014。
- [x] GEN-015。

### 19.1 互動式創作規劃（尚未實作）

依賴順序：GEN-016 先定 public／internal 契約與 Go 資料 owner，並在建立任何新 package 前完成context map登記；GEN-017～020 依此接會話與 UI，GEN-021～022 接驗證及精確保存，GEN-023 收齊跨流程測量。預算／刪除機制須與各 producer 同批落地，不能等串接完成後才補。規劃 ID 不是已存在的程式符號。

- [ ] GEN-016 設計會話契約與 Go／Postgres 版本化快照、事件、CAS、outbox、idempotency。（對應 `02:GEN-007`、`GEN-011`；依[互動創作](../adr/README.md#互動創作)）
- [ ] GEN-017 實作 LangGraph 固定 workflow 與有界 ReAct 編排，從 Go 快照重建每個 Job。（對應 `02:GEN-007`、`GEN-011`）
- [ ] GEN-018 實作澄清、已確認 brief／驗收條件與草稿 revision 流程。（對應 `02:GEN-007`、`GEN-009`）
- [ ] GEN-019 實作流程圖理解確認、結構化結果與指紋保存／重新上傳語意。（對應 `02:GEN-008`）
- [ ] GEN-020 實作 scoped Catalog 參考檢索、缺席呈現、明確確認與 Web 會話 UI。（對應 `02:GEN-008`、`GEN-011`；依 GEN-016／019）
- [ ] GEN-021 串接 Go 驗證與經授權的私有候選試跑，回傳可修訂回饋。（對應 `02:GEN-009`、`GEN-012`）
- [ ] GEN-022 實作 revision／雜湊綁定的最終保存、取消恢復與重送冪等。（對應 `02:GEN-010`、`GEN-011`）
- [ ] GEN-023 完成會話預算、費用揭露、工具提權確認、帳號刪除／共享 write fence 的跨流程驗收與上線前量測（各強制點隨 GEN-016～022 的 producer 同批實作）。（對應 `02:GEN-011`、`GEN-012`；依 GEN-016～022）

**目前狀態：15 項已勾、8 項 ◐。** 本批已依 `01` §10 的實作授權接上 Python LangGraph、Go 會話／Worker／候選保存與 Web 三入口；免費證據見 [開發手冊](../development/interactive-creation.md)。上述八項仍待完整允收證據，M5 曝光與付費實測另行核准。各項完成需同時提交對應 `02` Given／When／Then 的成功與拒絕證據；GEN-016／022／023 必須驗證重啟、過期確認、重複工具結果、預算耗盡與刪除競態，GEN-018～021 須覆蓋三種輸入的使用者更正與草稿比較；mock、真實模型及人類採用結果分開列示。

## 20. 受限環境下的可攜執行（M6，**已收束：完成 10 項、撤回 2 項、剩 0 項**）

> 依據[淨測試模式](../adr/README.md#淨測試模式)，允收準則見 [`02` §4.10](02-specifications-and-acceptance-criteria.md)（`PORT-001`～`010`），計畫與邊界見 [m6/README.md](mvp/m6/README.md)，前期量測見 [m6/report-inmemory-postgres.md](mvp/m6/report-inmemory-postgres.md)。本節**不在 MVP 的完成度計算內**（同 M5 先例，`01` §7.3），惟該判定本身待裁定。
>
> **觸發它的是一個部署環境的事實，不是一個願望**：Pitch 在金融機構環境進行，那台機器只能執行白名單內的程式、不能安裝軟體、對外網路只保證得到模型供應商。今天的系統在那台機器上一行都跑不起來。
>
> **⛔ 一條邊界**：這個模式**要自己在畫面上說出它不是什麼**（`PORT-003`）——**三條軸各一句**：沙箱沒有隔離、物件儲存不驗 presigned URL、資料庫只有一條連線。形式與 `02:GEN-004` 對生成物的要求同源，理由也一樣——一個看起來像產品的東西，如果不說自己是什麼，就會被當成產品，而這一次的觀眾是金融機構。<br>**注意這不是「Adapter 不是產品」那句舊話**：淨測試模式跑的是**真的 Go 後端**，授權、政策、狀態機與評估判定全部都在，要揭露的只有上面那三項。
>
> **⛔ 判準一（不可協商）**：任何取代 PostgreSQL 的候選，**必須能讓 `UPDATE skill_versions` 失敗**。那是鐵律 4 由資料庫強制的形式，也是最便宜的真偽測試。手寫的假資料層與 SQLite 都過不了這一關，而它們會**照樣全綠**。
>
> **這一節的優先序取決於一個本節之外的東西**：`01` §11 的成功指標到今天一個數字都沒有，而 Pitch 一定會被問「有人要嗎」。[ask-5](mvp/m5/ask-5.md) 與 M1 閘門加起來不到兩天，**M6 讓 demo 跑得起來，但它不產生任何一個那樣的數字。**

- [x] PORT-001 建立乾淨測試模式的資料庫承載（PGlite）。（允收：`02` §4.10）**前期量測已完成**：42/42 乾淨套用，且逐項行為驗證通過——含**不可變性 trigger 真的擋下 UPDATE 與 DELETE**（`row in public.skill_versions is immutable and cannot be updated`）、advisory lock 三種形式、RANGE 分割表、pgvector 距離運算子。**本工作項要做的是把那份一次性腳本變成可重複的承載**，含版本釘選與 `devctl doctor` 對帳。
- [~] ~~PORT-002 SQL 抽取步驟~~ **已撤回**（依[淨測試模式](../adr/README.md#淨測試模式)）：淨測試模式下瀏覽器對話的是真的本機後端，**SQL 只有一個消費端，漂移的可能性隨形狀消失**。
- [x] PORT-003 淨測試模式的畫面揭露。（允收：`02` §4.10）以**共用元件**實作並掛在所有適用畫面，不得逐頁各寫一份。**先例見 `apps/web/src/features/creation/components/GeneratedNotice.tsx`**——那正是為了防止三份文案裡有一份日後變成安慰話而做成共用的。
- [x] PORT-004 讓跳過出聲：報出跳過筆數與原因，並提供令跳過改為失敗的開關（CI 啟用）。（允收：`02` §4.10）**今天的現況是 287 筆靜默消失、畫面全綠**（`apps/platform` 562 vs 889；`apps/sandbox` 亦同形）。**注意界線**：現行跳過訊息本身是誠實的，任何讓畫面更綠卻沒有真的執行斷言的改動都比現狀更糟。
- [x] PORT-005 **啟動形式**。
**用 cache 而不是打包 `node_modules`**：cache 還原時仍然照 lockfile 走，所以落在遠端機器上的是 lockfile 說的東西，不是某台開發機當時的樹。<br>
**bundle 會自己驗一次才敢交**：產完就地 wipe `node_modules` 再用 `--offline` 還原，還原不了就當場失敗而不是到那台沒有網路的機器上才發現。<br>
**`--offline` 是有牙的**：空 cache 直接以「快取裡沒有」失敗（實測），所以「離線裝成功」不是 npm 偷用預設 cache。還原後 `verify.mjs` 七條檢查照樣全過。<br>
<br>
**Go 半的 bundle 是 module proxy 版面（149 MB），修剪後才驗——被檢查的就是被帶走的**（未修剪是 634 MB，因為 `go mod download` 連解壓後的樹一起留）。遠端還原是 `GOPROXY=file:///<bundle>/go/cache/download go -C <module> mod download`，**它把那台機器自己的 module cache 填好，之後不用留任何環境變數**，啟動指令碼完全不需要知道 bundle 存在。<br>
**這裡查到一件非做不可、否則整包白帶的事**：`go.mod` 要 **1.27.0** 而本機 Go 是 **1.26.6**，所以 `go` 一直在**自己下載工具鏈**——而那個工具鏈本身就是一個 **246 MB 的模組**。離線機器若沒有它，**在編第一行程式之前就死**。突變驗過：對空 bundle 建置的失敗是 `toolchain not available`，連相依都還沒碰到。<br>
**兩半都自我驗證才敢交**：npm 半 wipe `node_modules` 後用 `--offline` 還原；Go 半用**空的 module cache ＋ 沒有網路後備的 file proxy** 把三個模組全部建一次。實測全過。<br>
**最後一條允收（前端要在 Chromium 系瀏覽器驗、不得只驗 Firefox）也關上了**：對真的跑起來的淨測試模式，用真的 Chromium、**零 cookie** 開首頁——三句揭露全部出現，禁用詞（完整／等同／與正式環境一致）一個都沒有。<br>
**這一項解掉的是網路，不是白名單**：Go 會把未簽章的執行檔丟進暫存目錄再執行，**那仍然是 environment-probe 的 Q2**，而 bundle 對它一點幫助都沒有。)（依[淨測試模式](../adr/README.md#淨測試模式)）：以一個指令碼啟動；**旗標未設時行為與今天完全相同，且要有測試在守**；相依必須能離線帶上機器；啟動失敗要說出缺什麼。（允收：`02` §4.10）<br>**原本的「以瀏覽器開啟本機檔案、不得依賴 localhost」屬於已被推翻的形狀**——本模式依賴 localhost 上的一個服務，那正是被換掉三個實作的系統本身。`file://` 下 Chromium 以 CORS 擋掉任何 module 的 `import`（Firefox 不擋），**而 Vite 產出的正是外部 module script，預設建置在 Edge 上開本機檔案會是一片空白**。可用的是 inline classic、外部 classic、無 import 的 inline module，**所以交付形式必須是全部內嵌的單檔**。<br>未驗證項：PGlite 的 WASM 與檔案系統映像在 `file://` 下能不能載入（若其載入路徑用 `fetch()` 會撞同一道牆，繞法是 base64 內嵌，+33% 體積）——**這一項是推導不是實測**。
- [~] ~~PORT-006 端點範圍表~~ **已撤回**（依[淨測試模式](../adr/README.md#淨測試模式)）：跑的是真的後端，**端點就是端點**。**本項的第二句仍然有效並已移交**：不受信任內容的執行仍然不行，由派送閘門強制（`PORT-010`）。
- [x] PORT-007 展示資料一律取自 repo 內可追溯的真實來源。（允收：`02` §4.10）**不得為展示另行編造。**。
- [x] PORT-009 物件儲存的 in-process 承載，以及它不涵蓋什麼的明文記載。（允收：`02` §4.10）**⚠️ 已退回未勾，理由寫在行內。**

hello in-process s3

0;chunk-signature=…`——**那截簽章原封不動在檔案裡**。<br>**第二半以測試形式落地**：`TestInProcessDoesNotAuthorize` **斷言這個承載不安全**——竄改簽章、拿掉簽章、TTL 一秒後隔三秒再用、拿 GET 的票發 PUT，四種全部斷言回 200，最後一種再讀一次確認物件真的被覆寫。**一句「這東西不驗簽」的註解沒有機器在背後，四條斷言有。**<br>**勘查沒查到而實測撞到的一件事**：minio-go 對**每一個**帶 bucket 的請求都會先發 `GET /bucket/?location` 做區域探測，不只 `MakeBucket` 會；沒處理的話 `EnsureBucket` 第一步就 405。<br>**還沒接線，而那不在本項允收裡**：`FromEnv()` 一個字沒改、四個 `cmd/*/main.go` 一個字沒改。旗標分支屬於[淨測試模式](../adr/README.md#淨測試模式)那顆待裁定的合併入口。
- [x] PORT-010a 派送閘門改為白名單，並開出 `clean` 等級。（允收：`02` §4.10）
- [x] PORT-010b 乾淨模式的執行 Driver 本體。（允收：`02` §4.10 `PORT-010`）**前期量測已完成**（[m6/report-local-driver.md](mvp/m6/report-local-driver.md)）：**沒有值得加的相依**——`golang.org/x/sys`（已在樹裡）＋約 120 行，參考實作是 Buildkite Agent 的 `internal/process`（MIT，可抄不可 import）。**實測**：只 kill 父行程會留下存活的孫行程（`leaked=1`），改用 Job Object 歸零。
- [x] PORT-010c 操作者具名放行一個未策展版本的開關。（允收：`02` §4.10 `PORT-010` 第 5 條的四條）
- [x] PORT-008 確認未引入第二份資料層。（允收：`02` §4.10）這是一條**禁令而非交付物**，勾選的形式是一次逐項核對：沒有第二個實作 `db/gen` 那批查詢方法的型別、沒有第二套 schema 定義。依據是 `coder/coder` 於 2024-11 移除 `dbmem` 的紀錄（PR #15291）。

## 21. Credit 計價與消費控制（M5，新功能凍結期間第十次放行）

> 依據[帳號清除與 Credit](../adr/README.md#帳號清除與-credit)（承接負責人本輪五點裁定：Credit 是這個系統唯一計價單位、餘額可能為負但下限 −50、餘額低於門檻可在開始前擋下、創作與搜尋的實際費用都要記錄並在特定時機與固定時間統計），允收準則見 [`02` §4.11](02-specifications-and-acceptance-criteria.md)（`CRED-001`～`008`）。**這是新功能**，放行依據記在 [`01` §10](01-goals-and-plan.md) 第十批；凍結期間三條 ⛔ 曝光邊界不變——Credit 的三道消費閘與生成入口的曝光旗標是兩件事，不互相取代。

> **狀態：schema、套件與掛勾已寫，端到端尚未接通，以下全部未勾。** `db/migrations/0060_credit_ledger.sql` 建了 `cost_events`／`credit_accounts`／`credit_entries`／`cost_statistics` 四表；`apps/platform/internal/creator/credit` 套件（`Service.Charge`／`CanStart`／`CanAffordStep`／`Grant`／`RecomputeStatistics`／`PurgeUser`，`money.go` 的無條件進位換算）對著一個假 `Store` 寫了完整單元測試；`creator/creation` 的 `CreationBilling.CanStart`／`Reserve`／`Settle` 三個 nil-able 掛勾已接進 `creation.go`／`job.go`（`credit_hooks_test.go` 守）；`apiserver/credits.go` 的 `GET /me/credits`／operator 授予 handler 已寫且有測試；Web `core/session/credits.service.ts`／`CreationSession.tsx` 已接畫面。**但沒有一段真正被組裝起來**——詳細缺口與逐項責任見 [`04` 丙-185](04-backlog-and-handoffs.md)，這裡不重複。**程式面收斂不等於完成**（AGENTS.md）：下列八項在對應缺口補齊、走完 `02` 的 Given／When／Then 之前一律不勾。

- [x] CRED-001 對 `credit.sql`／`cost.sql` 跑 `task gen:sql`（主 Agent 序列化），寫出 `credit.Store` 的真實 Postgres adapter，並在 `apiserver.NewApp`／`entrypoint/worker` 兩個組裝根建立 `credit.Service`。（對應 `02:CRED-001`）
- [x] CRED-002 把組裝出的 `credit.Service` 接進 `creation.Service` 的 `CreationBilling.CanStart`／`Reserve`／`Settle` 三個操作，使三道消費閘在生產環境真正生效（今天恆為 `nil`＝不檢查）。**接線時必須在組裝根明確做 Workspace → User 的轉換**——三個操作今天傳的是 `WorkspaceID`，`credit.Service` 的簽章收的是 `userID`；MVP 雖然一人一個工作區，但沒有任何程式碼保證兩個 id 相同，不得在呼叫端直接把 `WorkspaceID` 當 `userID` 傳入。（對應 `02:CRED-002`、`CRED-003`；依 CRED-001）
- [x] CRED-003 `contracts/openapi/public.yaml` 補 `GET /me/credits` 與 operator 授予端點的 operation（主 Agent 序列化），`router.go` 掛上 `credits.go` 已寫好的兩個 handler。（對應 `02:CRED-004`、`CRED-007`）
- [x] CRED-004 註冊 `credit.RecomputeWorker` 為 River 每日 periodic job；互動創作會話終結（保存／放棄／逾時）時寫入一列成本摘要，餵給滾動窗統計（事件驅動那一半今天還沒有程式碼）。（對應 `02:CRED-006`；依 CRED-001）
- [x] CRED-005 讓 `catalog`（搜尋 embedding、索引增強）、`eval`（評審／建議）、`ingest`（單次生成對照）三個既有 context 各自呼叫 `credit.RecordCost`（context map新增四列 Customer–Supplier 依賴）；MVP 期間這三類只寫 `cost_events` 餵統計，不對使用者扣點（待決策見[帳號清除與 Credit](../adr/README.md#帳號清除與-credit)）。（對應 `02:CRED-005`；依 CRED-001）
- [x] CRED-006 `cmd/maintenance` 帳號刪除步驟清單新增一步呼叫既有的 `credit.PurgeUser`，並補上 `PurgeExpiredCostEvents`／`PurgeExpiredCreditEntries` 的時間視窗保存掃描排程（同 `SEC-006` 形狀，值待負責人與既有保存清冊一併裁定）。（對應 `02:CRED-008`；依 CRED-001）
- [x] CRED-007 `apps/platform/.golangci.yml` 補 `creator/credit` 的 depguard 規則，收斂context map「先登記後建目錄」的過渡態（目錄已建，depguard 待補）。（依context map；鐵律 7）
- [x] CRED-008 CRED-001～003 落地後，對真後端跑一次端到端驗收（開始前拒絕、每步負債下限、餘額顯示、operator 授予），並把面額、加成、保守常數、滾動窗長度的最終值（[帳號清除與 Credit](../adr/README.md#帳號清除與-credit)「待決策」）回填部署設定，替換主線反推的預設值。（對應 `02:CRED-002`、`CRED-004`；依 CRED-001～003）
- [x] CRED-009 試跑扣點（`05` R-74）：`cost_events.kind` 加 `run`（migration 0062），建立 Run 時以閘道上界做開始前檢查，清理時依閘道實付結算、以 run id 冪等；讀不到花費只記不扣。兩個組裝根都接上。（對應 `02:CRED-003` 試跑那一條；依 CRED-001、CRED-002）

**目前狀態：9 項全勾**（CRED-008 的每步負債下限在真模型上跑過，最終值裁定維持現值）。各項完成須同時提交對應 `02` Given／When／Then 的成功與拒絕證據；依 AGENTS.md 鐵律 9，每條新規則要留一次「把修法還原、對應測試變紅、改回」的證據。

## 22. 營運後台（OPS）

> 依[營運後台](../adr/README.md#營運後台)（[`05` R-77](05-pending-rulings.md)）新增，允收準則見 [`02` §4.12](02-specifications-and-acceptance-criteria.md)。後台不是新的 Bounded Context：每一項工作各自落在擁有那項事實的 context，這裡只是把它們排成兩批。

- [x] OPS-001 契約 `GET /me` 加 `operator`；`apps/web` 新增 `/admin/*`（延遲載入、帳號選單入口、非 operator 看到「這一頁現在不存在」、淨測試模式的說明）；資訊架構 §0.1 R2 收下 `/admin/*`。（對應 `02:OPS-001`）
- [x] OPS-002 以 email 找帳號的端點（`identity`），找到時同一交易寫 audit；前端帳號頁。（對應 `02:OPS-002`）
- [x] OPS-003 點數餘額與分錄的端點（`credit`），同一交易寫 audit；前端帳號頁顯示並授予。（對應 `02:OPS-003`；依 OPS-002）
- [x] OPS-004 Skill 治理查詢端點（`catalog` 的 handler、`registry` 的 query）；前端治理頁接三個既有動作。（對應 `02:OPS-004`）
- [x] OPS-005 名冊唯讀端點（`identity`）；前端派送煞車頁與名冊頁。（對應 `02:OPS-005`）
- [x] OPS-006 operator 動作紀錄端點（`audit` 的 query，action 清單由 `apiserver` 提供）與前端頁。（對應 `02:OPS-006`；第二批）
- [x] OPS-007 成本統計端點（`credit`）與前端頁； 的觀察改看這一頁。（對應 `02:OPS-007`；第二批）
- [x] OPS-008 每一條新 `/admin/...` 端點列入 `authz_matrix_integration_test.go`；新 query 登記在 `db/query-owners.yaml`，不加 `allow:` 例外。（依[Platform Bounded Context 與 Context Map](../adr/README.md#platform-bounded-context-與-context-map)、[Query 與寫入所有權](../adr/README.md#query-與寫入所有權)；鐵律 7、8）
- [x] OPS-009 營運趨勢圖：`apps/web` 新增 `chart.js` 依賴（[營運後台](../adr/README.md#營運後台)，§4.8 的具名例外）與 `/admin/trends`；四條每日彙總端點（`credit` 兩條、`run`、`audit`），逐條 `RequireOperator` 並列入 authz 矩陣。（對應 `02:OPS-008`；第三批）
- [x] OPS-010 模型呼叫逾時：契約六個 request schema 加 `timeout_seconds` 並由 `apps/llm` 取 `min()`；migration `0082` 的 `model_call_budgets` 與新的 Generic 套件 `foundation/integration/modelbudget`；三條 `RequireOperator` 端點（理由必填、與 audit 同交易）；`apps/web` 的 `/admin/model-budgets`。（對應 `02:OPS-009`；[`05` R-84](05-pending-rulings.md)；第四批）

## 23. 測試的容器依賴

> 目標是讓 `apps/sandbox` 的測試不必有容器 daemon 也跑得完整，**但隔離證明留在 gVisor**：非 root、唯讀 rootfs、drop caps、無宿主路徑與 docker socket、網路 default-deny、資源上限真的攔得住、OOM 與逃逸，換到沒有隔離的後端上只會變成假綠。判準一句話：這條測試跑在完全沒有隔離的後端上，還在測同一件事嗎——是，就是驅動契約；會變得空洞或誤導，就是隔離證明，不准搬。

- [x] SBX-014 驅動契約寫一次、每個後端各跑一次：`internal/drivertest` 的 `Subject` 只提供各後端不同的部分（建構、handle、請求），斷言寫在契約裡；`localdrv` 永遠跑，`dockerdrv` 有 daemon 才跑。一條契約測試搬進去時必須從原本的檔案刪掉，否則是第二個會漂移的地方。
- [x] SBX-015 CI 有一條沒有容器執行期可用的工作（`sandbox-nodocker`，`DOCKER_HOST` 指向沒人在聽的位址），新長出來的 daemon 依賴在那裡是紅的，不是靜默跳過。既有的 `SKILLHUB_REQUIRE_DOCKER=1` 守的是相反方向：有 daemon 的工作不得整批跳過還報成功。
- [x] SBX-016 Linux 的 `localdrv` 以 cgroup v2 強制記憶體與行程數上限（`cgroup_linux.go`；非 Linux 的 Unix 沒有 cgroup，回報無強制）。**能力是探測出來的，不是宣告的**：取得不到可寫的委派子樹就回報 false，且 `attach` 只對回報得出的那幾項設上限——宣告得了才可以被派工。受 cgroup v2「父層有行程就不得委派 controller」所限，driver 啟動時會把自己移進自有 cgroup 的葉節點，移不動就整項放棄並歸位。`SKILLHUB_REQUIRE_CGROUP=1` 讓「環境不支援」由跳過變成失敗，守的方向與 `SKILLHUB_REQUIRE_DOCKER` 相同。**已在具委派子樹的 Linux 上以非 root 實證，四項突變各自會紅**；CI 帶著它跑上限那幾條，所以這項能力是被強制檢查的，不是靠宣告。
- [x] SBX-017 CI 上的上限檢查是閘門而非探測。hosted runner 的 job 位於 root 擁有、不可寫、`subtree_control` 為空的系統服務 cgroup，controller 都在、缺的只是委派；`tools/ci/with-delegated-cgroup.sh` 以 sudo 補上那三個寫入（建 slice、開 controller、chown、把 shell 移進去），**exec 之後全部非 root**，測試二進位先編譯再 exec 以獨佔該 cgroup。
