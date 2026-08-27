# Skill Hub M0：產品決策提案（PDM）

> **狀態：提案 v5；§9.1 的十四列已於 2026-08-27 全數處置——十一列追認、三列不追認（見下方定案紀錄）。**
> **三列未追認，各自在等不同的東西**：`PDM-007`（Local Runner OS，後 MVP，本文件不涵蓋）；`PDM-009`（產品面已於 2026-08-22 追認，**報酬金額與受測者簽署**仍等 [`05` R-2](../../05-pending-rulings.md)）；`SBX-002` 實測項（CLI 內建 Skill 與內建工具的裁減幅度**一次都沒有量過**，部分完成保持未勾）。
> 本文件由架構／規劃側起草，目的是把 `plans/03-work-items.md` 第 1 節的待決策項目，從「開放式問題」收斂成「可被否決或核准的具體方案」。
> **除下方已記錄的定案外，其餘仍只提案、不定案。** 定案後請由負責人：(1) 在本文件對應章節標註「已採納／已修改／已否決」與日期；(2) 依 §9 的回寫對照表，把數值寫進 `plans/mvp/02` 對應需求 ID 的允收準則；(3) 才把 `plans/mvp/03` 的 `- [ ]` 勾選為 `- [x]`。
> **已完成的落地動作（2026-08-14）**：`03` 的 PDM-001／002／003／011 已勾選；ADR-013／015 轉 Accepted、新增 ADR-018、ADR-014 轉 Superseded。~~**§9 的數值回寫至 `02` 尚未執行**，仍是開工前的待辦。~~<br>**2026-08-27 訂正**：§9.2 的回寫**早已陸續完成**（`02:TEST-002`／`RUN-003`／`TEST-005`／`NFR-002` 等處都已帶著這些值的可判定形式），這句「尚未執行」從某個時點起就是假的，只是沒有人回來改它。**真正一直缺的是反方向的那一半**——`02` 以「值已定案」的敘述引用這些數字，而 §9.1 的方塊一個都沒打，於是兩份文件對「是否已定案」永久不一致（`04` 乙-9）。**那一半在 2026-08-27 補上。**

> ## 負責人定案紀錄
>
> | 日期 | 決策 | 影響範圍 |
> | --- | --- | --- |
> | **2026-08-14** | **模型供應商採 OpenAI API**（經 LiteLLM 閘道）。ADR-017 架構與實作鐵律 8 不變——所有模型呼叫仍只走閘道、供應商金鑰只存在閘道，改變的只是閘道背後的後端。 | §3 PDM-003 模型分層改為 OpenAI 系列（型號與定價見該節）；§3 補測清單收斂；§5.2 Token 與預算編列；[cost-estimation.md](cost-estimation.md) §6.2；[pdm-003-litellm-spike-report.md](pdm-003-litellm-spike-report.md) §11 即為定案後在**正式後端**上完成的補測 |
> | **2026-08-14** | **PDM-001／002／003 三項全部依本文件 v5 提案定案**（首批三個 Skill 類別＝`documents`／`writing`／`data`；白名單來源與九項精選檢查表；Runtime＝Claude Agent SDK TS on Node.js 22 LTS ＋ OpenAI 模型分層）。負責人同時批准 M0 全部產出並指示開工。 | `plans/03-work-items.md` 的 PDM-001／002／003／011 已勾選；§4 的 `lxml`（必要）與 `matplotlib`（建議）白名單增補隨 PDM-002 一併採納；§9 回寫對照表的數值寫入 `02` 仍待執行 |
> | **2026-08-14** | **ADR 定案**：[ADR-013](../../../adr/ADR-013-intent-search-architecture.md) → **Accepted**（依 PDM-011 Spike，含四項實證調整）；[ADR-015](../../../adr/ADR-015-sandbox-isolation-technology.md) → **Accepted**（gVisor 基線與獨立 VM 池不變）；新增 [ADR-018](../../../adr/ADR-018-containerized-core-infrastructure.md) 核心基礎設施容器化自架 → **Accepted**，[ADR-014](../../../adr/ADR-014-core-infrastructure-selection.md) 隨之標為 **Superseded**。 | 本文件 §3 的 Embedding 與模型分層已回填至 ADR-013 待決策；§5.2 的 Sandbox 資源上限已回填至 ADR-015 容量池；[cost-estimation.md](cost-estimation.md) §5／§7 的容器化例外與 E1→E2 觸發條件已成為 ADR-018 決策內容 |
> | **2026-08-27** | **§9.1 的十一列整批追認，照提案值，行為零變動**（[`05` R-1b](../../05-pending-rulings.md)）：PDM-001／002／003／PDM-003 補測項／PDM-003×PDM-011／004／005／006／008（兩列）／010，外加「三份文件同步」與「是否構成新架構決策」兩列的核對。**這一批沒有改任何一個值**——追認補的是簽名不是行為，程式碼與測試一行未動。<br>**唯一一個真正被擇一的**：PDM-010 §8.1 自己點名不准實作推斷的那件事——**首月額度語意取 `min(20, 30) = 20`**，不是 20+30=50。也就是 `entitlements/quota.go` 現在就在強制的那一組：首窗 20／每窗 30／每日 5／窗長 30 天。<br>**追認之後解除的是一條禁令，不是一個開關**：ADR-028 決策 4 的「未追認的數字不得出現在畫面上」對這四個數不再成立；`RUN_QUOTA=off`（ADR-055）沒有變，所以本次封測畫面上仍然沒有額度可顯示。<br>**同批追認 M5 生成額度的四個數字**（每日 10／每窗 30／首窗 20／窗長 30 天，[`05` R-9](../../05-pending-rulings.md)），同樣是解除禁令而非打開 `GENERATE_QUOTA`（ADR-056）。<br>**未追認的三項，以及各自在等什麼**：**`PDM-007`**（Local Runner 首批 OS）等的是**需求訊號**——Local Runner 已移出 MVP 首發（`01` §7.3），本文件從一開始就不涵蓋它；**`PDM-009`**（封測人數與門檻）產品面已於 2026-08-22 追認（12 人、三層各 4、14 天、三條門檻），**等的是報酬金額與受測者簽署**（`04` 乙-15、[`05` R-2](../../05-pending-rulings.md)），而報酬是本次封測的最大單項支出且 [cost-estimation.md](cost-estimation.md) 沒有一行涵蓋它；**`SBX-002` 實測項**等的是**一次沒人做過的量測**——CLI 內建 Skill 與內建工具的裁減幅度與 Skill 相容性的權衡，那是 harness 19.4K 固定前綴的唯一槓桿。 | §9.1 全表；`03` §1 的 PDM-004／005／006／008／010 同批勾選；`01` §13 去向表三列訂正；[ADR-055](../../../adr/ADR-055-the-run-allowance-is-turned-off-and-that-took-an-action.md) 與 [ADR-056](../../../adr/ADR-056-the-generation-allowance-is-its-own-switch-and-it-is-off.md) 各加一節「後續」（**不改寫決策**）；`entitlements/quota.go`／`generate_quota.go` 與兩支測試的 `待追認` 註記改為已追認 |
>
> **此定案的一個直接紅利**：§11 的補測不再需要 Anthropic 憑證，且測到的就是生產模型本身，結論效力高於原先的「代打」設計。

- 起草日期：2026-08-13（v1）／2026-08-13 修訂（v2）／2026-08-14 同步（v3）／2026-08-14 修正（v4）／**2026-08-14 定案回填（v5）**。修正紀錄見文末第 10 節
- 涵蓋範圍：PDM-001、002、003、004、005、006、008、010
- 未涵蓋：PDM-007（Local Runner OS，後 MVP）、PDM-009（封測人數，建議與 PDM-010 一起定）
- **PDM-011 另行交付**：初步 Spike 已執行完成，見 [pdm-011-spike-report.md](pdm-011-spike-report.md)（結論「方向可行，但三段式作法的權重需調整」）。該報告 §6.2 的三項調整已回寫入本文件 PDM-003；**完整 golden query set（每類別 20 條）仍待 PDM-001 定案後補做**。

> **ID 引用慣例（v2 導入）**：`02-specifications-and-acceptance-criteria.md` 與 `03-work-items.md` 對 TEST／RUN／EVAL／PACK／TRACE／DISC 等前綴使用**相同前綴、不同編號**。本文件一律標註來源：`02:XXX-nnn` ＝ 規格與允收準則，`03:XXX-nnn` ＝ 工作項目。無衝突的前綴（PDM／SBX／CONTENT／INGEST／SEC／O11Y／BETA／CORE／WS／DESIGN／QA）不另標註，均指 `03`；NFR 與 SKILL 只存在於 `02`。

---

## 0. 決策相依關係

三個阻塞項（AGENTS.md）的實際依賴順序如下，建議照這個順序定案：

```text
PDM-001 (類別)  ──┬──► PDM-002 (來源與精選標準) ──► CONTENT-003/008
                  │                               └─► PDM-011 完整 golden query set ──► ADR-013 定案
                  └──► PDM-004 (Runtime 語言) ──► SBX-002 Runtime Image
PDM-003 (Runtime/模型) ──┬──► PDM-004、PDM-005（Token 上限）、PDM-008（打包 Profile）
                         ├──► ADR-017 待決策「Virtual Key 注入機制」（＝威脅模型 Q11）
                         └──► ADR-013 待決策「Embedding 與查詢改寫模型」
PDM-005 (資源上限) ──► PDM-010 (免費額度成本) ──► 部署平台成本試算 ──► ADR-014/015 定案
PDM-006 (保存期限) ──► NFR-002 可測試時間要求、SEC-006、威脅模型 Q9
```

**PDM-011 不在此鏈上游**：初步 Spike 已用 12 份公開 `anthropics/skills` 樣本在 PDM-001／002 定案前執行完畢，並已產出可回寫的結論（見前言）。PDM-001／002 只是**完整 golden query set** 的前置，不是 Spike 本身的前置。

**單點建議**：PDM-001 與 PDM-003 是真正的瓶頸，其餘六項都可以在這兩項定案後一週內跟進。

---

## 1. PDM-001：MVP 首批三個 Skill 類別

> **狀態（v5）：提案完整，可定案。** 唯一的阻塞項（`data` 類別零白名單供給）已由 [data-category-sourcing.md](data-category-sourcing.md) 解除。

### 背景

`plans/mvp/01` 第 8 節要求初期先選三個類別，每類整理少量高品質 Skill。這個選擇同時決定了三件事：M1 驗證閘門的測試素材、PDM-011 golden query set 的題目來源、以及 SBX-002 Runtime Image 要預裝什麼。

選擇必須同時滿足三個限制：

1. **是個人創作者真的會做的任務**（否則 M1 驗證閘門的使用者測試沒有意義）。
2. **公開生態已有可精選到高品質的 Skill**（否則 CONTENT-003 湊不出 golden query set 需要的量）。
3. **能在 ADR-005／015 的 Sandbox 限制下驗證效果**——沒有 GPU、預設封鎖外網、非 root、限時、只能存取使用者上傳資料與預裝資源。這一條淘汰掉大量看起來很吸引人的類別。

### 評估選項

| 選項 | 類別組合 | 優點 | 缺點 |
| --- | --- | --- | --- |
| A：產出導向 | ①文件與試算表產出 ②內容寫作與品牌一致性 ③資料整理與分析 | 三類都有「可下載的產物」，Sandbox 內可用解析器做確定性驗收；供給充足 | 對「精深者」persona 稍偏基礎 |
| B：開發者導向 | ①程式碼審查與測試 ②Web 應用自動化測試 ③MCP／Skill 建置 | 供給最大（VoltAgent 清單以開發類為主）；技術族群早期採用意願高 | Web 測試需要瀏覽器與網路，違反 ADR-005 egress 政策；程式碼審查需要 repo 上下文，Dataset 上傳難以模擬 |
| C：創意導向 | ①視覺／前端設計 ②簡報與敘事 ③演算法藝術 | 展示效果最好、最容易做 demo | 驗收幾乎只能靠人眼；`02:EVAL-001` 要求「附證據」在此類別退化為 LLM Judge 單一來源，M3 評估可信度不足 |

### 建議

**採選項 A，三個類別命名如下：**

| # | 類別 ID | 顯示名稱 | 典型任務句 | Sandbox 驗收方式 |
| --- | --- | --- | --- | --- |
| 1 | `documents` | 文件與試算表產出 | 「把這份會議逐字稿整理成一頁式決議摘要 docx」 | 產物用 `python-docx`／`openpyxl`／`pypdf` 解析，斷言結構、欄位、頁數、必要段落存在 |
| 2 | `writing` | 內容寫作與品牌一致性 | 「用我的品牌語氣把這篇技術更新改寫成給客戶的公告」 | 規則檢查（禁用詞、長度、必要區塊）＋ LLM Judge 對 rubric 逐項評分（`03:EVAL-005` 明確標示為模型判斷） |
| 3 | `data` | 資料整理與分析 | 「把這三個 CSV 合併、清掉重複列，輸出一份異常值報告」 | 對輸出 CSV／JSON 做 schema 與數值斷言，確定性最高 |

### 理由

- **驗收可解釋性排序**：`data` > `documents` > `writing`。三者形成一個梯度，正好讓 M3 的 `03:EVAL-005`「規則判斷／模型判斷／使用者判斷」三種來源都有真實案例，而不是三個類別都只能靠 LLM Judge（`02:EVAL-001` 要求「不得只提供無法解釋的分數」）。
- **三類供給全部已查核落地（v5 依實查更新）**：`anthropics/skills` 提供 `docx`／`xlsx`／`pptx`／`pdf`（文件類）與 `internal-comms`／`brand-guidelines`（寫作類）；**`data` 類別的供給缺口已解除**——[data-category-sourcing.md](data-category-sourcing.md) 走完 PDM-002 的回溯准入流程後查得 **25 個合格候選、橫跨 7 個獨立來源 repo**，超過 PDM-002「每類別 8–12 個索引項目、4–6 個精選」的目標，且來源多樣性高於另兩類（各僅 1 個 repo）。
  **兩項連帶修正**：(i) `anthropics/skills` 的 skill 目錄**實數清點完成＝17 個**（Apache-2.0 12／source-available 4／無 License 檔 1），PDM-002 選項 A「官方唯一」維持否決——17 個中屬 `data` 者只有 `xlsx` 一個。(ii) **`doc-coauthoring` 不再計入 `writing` 供給**：該 skill 目錄無 `LICENSE.txt`，且 repo 根目錄亦無可繼承的 License 檔，依精選標準第 1 項與「License 狀態預設未知」規則，**暫不可標為精選、不可打包**。`writing` 的官方精選供給實為 `brand-guidelines` ＋ `internal-comms` 兩個。
- **Sandbox 相容**：三類都只需要「上傳檔案進去、產出檔案出來」，不需要外網、不需要 MCP、不需要瀏覽器——完全落在 ADR-005 的 MVP 限制內，SBX-007 的 egress 允許清單可以維持 PDM-005 §5.2 列的三項（LiteLLM 閘道、物件儲存短效授權端點、Trace ingestion 端點），不需要為任何類別開放額外目的地。
- **Persona 覆蓋**：`documents` 服務學習者（立即有產物）、`writing` 服務改善者（rubric 反覆調整）、`data` 服務精深者（可自行改腳本）。

### 風險

| 風險 | 影響 | 緩解 |
| --- | --- | --- |
| `writing` 類別的評估過度依賴 LLM Judge | `02:EVAL-001`「不得只提供無法解釋的分數」難以達成 | 每個 `writing` 精選 Skill 必須附一份可編輯 rubric（CONTENT-007），Judge 逐項回傳證據引文；UI 明確標示為模型評估 |
| 三個類別都偏「內容加工」，未觸及自動化／工具串接 | 可能低估使用者對 MCP／外部工具的真實需求 | 這是刻意的 MVP 取捨（`02:TEST-003` 已移出首發）；M4 封測需明確蒐集「你最想串接什麼」的質性回饋，歸 BETA-005（範圍與優先級複審），不歸 BETA-003（該項專指評估報告與改善建議的回饋） |
| ~~**`data` 類別在白名單中無可直接匯入的來源**~~ **（v5 已解除）** | ~~三個類別中有一類湊不出 PDM-002 的 8–12 個索引目標~~ | **已由 [data-category-sourcing.md](data-category-sourcing.md) 解除：25 個合格候選 / 7 個 repo，換類別的觸發條件未成立。** 但查核附帶三項須併同定案的條件（`lxml` 必加、`matplotlib` 建議加、§2 候選方向表須重寫），全部已回填至 §2／§4 |
| **`data` 的來源品質梯度顯著低於另兩類**（v5 新增，取代上一列） | 25 個候選中 24 個來自 1–5★ 的個人社群 repo（`documents`／`writing` 靠 168,872★ 的 `anthropics/skills`）；上游刪除或改寫的機率高得多，且精選標準第 4／5 項的人工審查成本集中於此 | 匯入時保存 commit SHA 與內容雜湊（INGEST-004 既有要求）；INGEST-010／CONTENT-009 的失效流程對 `data` 需實際演練一次；CONTENT-003／008 排程時把 `data` 的精選工時**單獨估算**，並優先選 prompt-only 候選 |
| **`data` 候選的語意過度集中**（v5 新增） | 25 個中有 12 個是同一 repo 的 `excel-*`、另 12 個是同一作者的資料清洗系列，描述高度同質；PDM-011 的 recall@5 可能因語意叢集而失去鑑別力——成因由「數量不足」變成「語意過度集中」 | 出完整 golden query set 時**強制跨 repo 抽樣**，每個來源 repo 至多貢獻 20% 題目；必要時把條件候選納入 `indexed` 層（該層不需九項全過）以增加語意分散度 |
| `documents` 的高品質 Skill 集中在 `anthropics/skills`，且其中 4 個是 source-available 非 OSS | License 風險（見 PDM-002） | 索引與平台內試跑可以，但**不產出任何 Download Artifact**（PDM-002 風險表、PDM-008 共通規則已統一為此保守政策）；依 ADR-012「無 License 或不允許再散布時阻擋或限制用途」處理 |

---

## 2. PDM-002：首批 Skill 來源清單與精選標準

> **狀態（v5）：提案完整，可定案。** 白名單已補齊三類供給、`anthropics/skills` 實數清點完成、`data` 候選方向表已依實查重寫。定案時須併同採納 §4 的 `lxml`（必要）與 `matplotlib`（建議）白名單增補——否則 `data` 供給由 25 掉到 13。

### 背景

`plans/mvp/01` 第 12 節把「初期內容品質不足」列為第二大產品風險，對策是「精選優先、來源分級、限制全網內容」。同時 ADR-007 與實作鐵律第 1 條要求匯入與掃描階段不得執行套件內 Script，所以精選標準必須全部是**靜態可判定**或**平台試跑可判定**的，不能依賴「跑跑看再說」。

### 評估選項

| 選項 | 做法 | 優點 | 缺點 |
| --- | --- | --- | --- |
| A：官方唯一 | 只收 `anthropics/skills` | License 與品質最可控，人工檢視成本最低 | **實數已清點＝17 個（v5）**，其中屬 `data` 者僅 `xlsx` 一個且為 source-available（不可下載）。以 1 個項目無法支撐 8–12 個索引目標，recall@5 亦無鑑別力。**清點後此選項維持否決**，理由由「數量待查」升級為「已查證不足」 |
| B：白名單 repo + 目錄僅作探索 | 官方 + 少數高信任 repo 進入索引；社群「awesome 清單」只用來發現候選，實際匯入時回溯到原始 repo | 供給足夠、來源可追溯、三層來源分級（精選／已索引／外部）都有真實樣本 | 需要人工維護白名單與下架流程（CONTENT-009） |
| C：目錄大規模抓取 | 直接對接 skillsmp.com／awesomeskills.dev 之類的市集索引 | 供給最大 | 違反「不做大規模全網爬取」的 MVP 範圍；License 與內容品質不可控；靜態掃描量爆炸 |

### 建議

**採選項 B。首批來源清單（白名單）：**

| 來源 | URL | License | 角色 | 備註 |
| --- | --- | --- | --- | --- |
| Anthropic 官方 Skills | https://github.com/anthropics/skills | **逐目錄各自帶 `LICENSE.txt`，repo 根目錄無 License 檔**（GitHub metadata `license: null`）：Apache-2.0 12 個、**source-available 4 個**（`docx`／`pdf`／`pptx`／`xlsx`，官方 README 明文）、**無 License 檔 1 個**（`doc-coauthoring`） | 精選主力（`documents`、`writing`） | `spec/` 為 Agent Skills 規格，`template/` 可作 `02:SKILL-002` 驗證測試資料。**skill 目錄實數已清點＝17**（v5）。⚠️ **授權必須逐目錄判定，不可用 repo 層 License 一概而論**——`doc-coauthoring` 即為反例。README 另載明「provided for demonstration and educational purposes only」，屬使用免責、不改變 License 分類，但應納入 CONTENT-005 白話摘要的措辭考量 |
| YuYY2004/excel-skills | https://github.com/YuYY2004/excel-skills | MIT | 精選候選（`data`） | 12 個 `excel-*` 整理類 Skill。**依賴 `lxml`，需 PDM-004 白名單加入後才可用**（見 §4）。2★、單一作者，上游消失風險高 |
| danielrosehill/Claude-Data-Wrangler-plugin | https://github.com/danielrosehill/Claude-Data-Wrangler-plugin | MIT | 精選候選（`data`） | 12 個 prompt-only 清洗／轉換 Skill，零 Script、依賴僅 `pandas`／`numpy`。⚠️ **同 repo 另有 7 個需外網者必須逐一排除，不可整包匯入** |
| nqumich/data-analyst-skill | https://github.com/nqumich/data-analyst-skill | MIT | 精選候選（`data`） | 依賴零缺口、203 行落在 300 行上限內，是最乾淨的單一候選。⚠️ `SKILL.md` 為簡體中文，CONTENT-005 白話摘要需重寫 |
| obra/superpowers | https://github.com/obra/superpowers | MIT | **僅作發現／參考**，不列入首批精選 | **v2 降級**：實為「a complete software development methodology for your coding agents」，內容為 test-driven-development、systematic-debugging、code review、git workflow、subagent-driven-development 等**開發流程**技能，不是 `writing` 素材。其 git workflow 類 Skill 依賴 `git`，而 PDM-004 的 Runtime Image 明確不含 `git`，依精選標準第 6 項本就不通過。作為「Skill 如何組織方法論」的參考範例仍有價值 |
| VoltAgent awesome-agent-skills | https://github.com/VoltAgent/awesome-agent-skills | MIT（清單本身） | **僅作發現**，不直接匯入 | 約 1,500 條目（repo 自述 1,400+，badge 顯示 1,497+），自身不託管 Skill，全部連回原 repo；含官方團隊與社群。走下方「回溯准入流程」 |
| ComposioHQ awesome-claude-skills | https://github.com/ComposioHQ/awesome-claude-skills | 見 repo | **僅作發現** | 交叉比對用 |
| heilcheng awesome-agent-skills | https://github.com/heilcheng/awesome-agent-skills | 見 repo | **僅作發現** | 偏「實際工程團隊在用」的策展角度 |
| skillsmp.com / awesomeskills.dev / awesomeclaude.ai | https://skillsmp.com 、 https://www.awesomeskills.dev 、 https://awesomeclaude.ai/awesome-claude-skills | 市集／目錄，非來源 | **MVP 不接入** | 記錄於此供 M4 後評估「外部即時搜尋」第三層來源時參考 |

**回溯准入流程（v2 新增，補齊白名單供給的唯一合法路徑）：**

awesome 清單只是發現管道，任何條目要成為可匯入來源，必須走完以下四步；**清單本身的 License 不繼承給被列的 Skill**。

```text
1. 在 awesome 清單中發現候選條目
2. 回溯到原始 repo（不從清單頁面或市集抓取內容）
3. 對原始 repo 逐項過下方「九項精選檢查表」——License 與作者可辨識兩項先過，否則不進入後續成本
4. 全過 → 該 repo 加入白名單（記錄提名人、加入日期、涵蓋類別）；任一項不過 → 記錄否決原因，不重複評估
```

白名單是**資料不是程式碼**：新增走人工提名 + 檢查表，異動記入 CONTENT-009 的內容維護流程。

**`data` 類別的候選來源（v5 依實查重寫，取代 v2 的三個「待查核」方向）：**

v2 提名的三個方向已由 [data-category-sourcing.md](data-category-sourcing.md) 逐一走完回溯准入流程。**結果是三個方向有兩個證偽**，可用供給全部來自第三個方向：

| v2 候選方向 | 查核結果 | 判定 |
| --- | --- | --- |
| 1. `anthropics/skills` 的 `skills/xlsx` 及其周邊 | 全 repo 17 個 skill 中具 `data` 性質者**只有 `xlsx` 一個**，且為 source-available（依風險表統一政策不產出任何 Download Artifact） | **不足以獨立構成類別**，降為 §4.2 條件候選；歸 `documents` 或 `data` 待裁量 |
| 2. VoltAgent 清單「Data & Analytics」分類下的官方團隊條目 | **該分類不存在**——VoltAgent 依**廠商**分節（64 個），唯一的非廠商分節「Community Skills」的七個子分類中亦無資料處理分類。逐一回溯所有資料性質廠商（DuckDB／ClickHouse／Tinybird／Neon／MongoDB／Redis／Supabase／Google Workspace／Kaelio／Altimate）後**合格數 0** | **方向作廢。** v2 假設「官方團隊 License 最容易確認」在事實上成立，但**選錯了篩選維度**——真正的淘汰條件是依賴：資料類廠商的商業模式就是託管服務，其官方 Skill 必然要連回自家服務，與 egress default-deny ＋ PDM-004「明確不含資料庫 client」正面衝突，**無一例外** |
| 3. 純腳本型社群資料處理 Skill（pandas／CSV 清理／異常值報告類） | **唯一成立者**，經 GitHub 直接檢索補足候選池後得 **25 個合格候選 / 7 個 repo** | **採用**，見上方白名單三列 |

**兩項必須記住的查核副產物：**

1. **License 缺失是頭號殺手，且發生率高於 v2 預期。** 14 組候選中以「無 License」出局 4 組、以「依賴超出 Runtime」出局 8 組、以「Script 過大」出局 4 組（有重疊）。**最受歡迎的兩個純資料處理 Skill 全部倒在第 1 項**——`coffeefuelbump/csv-data-summarizer-claude-skill`（446★）與 `openai/skills`（24,904★）皆無 LICENSE 檔。v2 風險表「社群 repo 的 License 檔案缺失」的預測完全命中。
2. **回溯准入流程的成本被低估。** VoltAgent README 中 **582 個連結指向 `officialskills.sh`**（市集站）、僅 631 個指向 `github.com`——**過半條目無法從清單本身取得原始 repo URL**，第 2 步「回溯到原始 repo」需額外一次人工查找。流程本身仍成立，但 CONTENT-003 的工時估算應據此上修。

> **換類別的觸發條件未成立**（25 > 8–12 目標），`data` 維持在 PDM-001 選項 A 內。若審查者判定 12 個 `excel-*` 應歸 `documents`，供給降為 13——**仍高於下限，仍不觸發換類別**，但精選池變薄。定案時請明確寫下歸類原則，建議依「產出 vs 整理」切分：去重／篩選／驗證／合併／拆分屬**整理** → `data`；建立與格式化屬**產出** → `documents`。

**精選標準（`curated` 層的允收檢查表，九項全過才可標為精選）：**

1. **License 明確且允許再散布**（OSI 認可，或作者明確授權）。source-available 者可索引，但打包流程依 ADR-012 阻擋散布。
2. **來源可追溯**：保存 repo URL、commit SHA、擷取時間、內容雜湊（INGEST-004）。
3. **規格驗證無阻擋錯誤**：`SKILL.md` 存在、YAML frontmatter 合法、`name`／`description` 齊備、所有檔案引用可解析（`02:SKILL-002`）。
4. **Script 可審閱**：無 Script，或全部 Script 合計 ≤ 300 行、由人工逐行審過，且不含 `eval`／動態下載／`subprocess` 呼叫外部網路。
5. **無疑似 Secret**：靜態掃描通過，且人工確認無測試憑證、內部路徑。
6. **不需外網、不需 MCP**：所有依賴可在 PDM-004 的 Runtime Image 內滿足（首批不允許執行期 `pip install` / `npm install`）。
7. **有可理解的白話摘要**：對應 CONTENT-005，非技術使用者讀得懂它做什麼、需要什麼輸入。
8. **至少一次平台基準試跑通過**：附範例 Dataset、User Prompt 與驗收條件（CONTENT-007／008），Run 結果為「符合」。此項在隔離 Sandbox 內執行，不在匯入或掃描階段執行，符合鐵律 1。
9. **作者可辨識**：有 GitHub 帳號或組織可對應，供 CONTENT-009 下架與變更通知使用。

**首批數量目標：**

| 層級 | 每類別 | 三類合計 |
| --- | --- | --- |
| 精選（curated） | 4–6 | 12–18 |
| 已索引（indexed） | 8–12（含精選） | 24–36 |

理由：ADR-013 的 PDM-011 Spike 要求每類別 20 條 golden query 量測 recall@5。若候選池只有 4–5 個 Skill，recall@5 幾乎必然為 1.0，Spike 失去鑑別力。每類別 8–12 個索引項目是能讓 recall@5 產生訊號的最小規模。

### 理由

- 白名單制讓「來源狀態：未知／可追溯／已人工確認」這三態（`plans/mvp/02` 第 3 節）在 MVP 就有真實資料，不是空的欄位。
- 把 awesome 清單降級為「發現管道」而非「來源」，是關鍵取捨：清單本身的 License 不等於被列 Skill 的 License，直接匯入會製造無法追溯的授權債。上方的回溯准入流程是這個取捨的具體執行方式。
- 九項檢查表全部可在不執行程式碼的前提下判定（第 8 項在隔離 Sandbox 內執行，符合鐵律第 1 條），不會迫使匯入階段破壞信任邊界。
- 「外部結果」這第三層由使用者自行提交 URL 匯入（`03:INGEST-001`）產生，不由白名單供給；因此三層來源分級在 MVP 都有真實資料來源。**注意威脅模型 Q15：匯入 SSRF 防護在 MVP 期間目前無工作項目承接**，此缺口由負責人依該文件建議的兩條途徑處理，不在本提案範圍。

### 風險

| 風險 | 影響 | 緩解 |
| --- | --- | --- |
| `anthropics/skills` 的 docx/pdf/pptx/xlsx 為 source-available | 這四個正是 `documents` 類別最好的樣本，卻不能打包散布 | **統一政策（保守版）：索引與平台內試跑照常（在平台上執行不等於再散布），但一律不產出任何 Download Artifact——包含 `standard` 標準套件在內。** UI 顯示「可在平台試跑，不提供下載」。並額外找 2–3 個 OSI 授權的替代品作為可下載精選。此政策在 PDM-008 共通規則同步；**這是法遵保守預設，放寬（例如允許 `standard` 下載）需負責人與法務明確確認後才可改** |
| 社群 repo 的 License 檔案缺失或矛盾 | 誤判為可散布 | License 狀態預設「未知」，只有人工確認後才升級；未知一律不可打包（`02:DISC-003` 允收準則已要求） |
| 上游 repo 刪除或改寫 | 精選內容失效 | INGEST-010／CONTENT-009 的失效與下架流程；因 Skill Version 不可變（鐵律 4），既有 Run 仍可追溯 |
| 白名單被視為「Skill Hub 背書」 | 信任誤導（NFR-001「靜態掃描通過不等於安全保證」） | UI 用「已人工檢視來源與 License」措辭，不用「安全」「官方推薦」 |

---

## 3. PDM-003：主要 Agent Runtime 與模型

> **狀態（v5）：提案完整，可定案。技術前置與補測全部完結。** 三個定案前置（閘道相容性 7/7、Skill 載入路徑 6/6、真 Embedding 跨語言召回）於 v4 已解除；**四項補測於 2026-08-14 全數結案**——兩項已實測（自主觸發、prompt caching），兩項因供應商定案為 OpenAI 而不再適用（見下方補測狀態表）。

### 背景

`plans/mvp/01` 第 7.2 節允許 MVP「只設定一個主要 Agent Runtime 作為平台驗證基準」。這個選擇決定 SBX-002 的 Runtime Image、`03:TRACE-002`／`03:TRACE-003` 能拿到什麼粒度的事件、以及 PDM-008 的第一個打包 Profile。

硬性前提（ADR-017 與實作鐵律第 8 條）：**所有模型呼叫必須走 LiteLLM 閘道，不得直連供應商；供應商金鑰只存在閘道。** 因此 Runtime 必須支援「把 API base URL 與憑證換成閘道的」這件事，否則直接出局。

### 評估選項

| 選項 | 內容 | 優點 | 缺點 |
| --- | --- | --- | --- |
| A：Claude Code CLI（headless） | 在 Sandbox 內以 `claude -p` 非互動模式執行 | 與使用者本機體驗最接近；Skill 載入行為即 Claude Code 行為 | CLI 的結構化事件輸出契約較不穩定，`02:TRACE-001` 的 Schema 綁在 CLI 版本上；權限模型偏互動式，headless 下要靠旗標壓平 |
| B：Claude Agent SDK（函式庫） | `@anthropic-ai/claude-agent-sdk`（TS）或 `claude-agent-sdk`（Python），以 `query(prompt, options)` 驅動 | 就是 Claude Code 的 harness 打包成函式庫：內建 Read／Write／Edit／Bash／Glob／Grep、context 管理、hooks、permissions、subagents；hooks 正好是 `03:TRACE-002`／`03:TRACE-003` 的天然掛載點；permissions 對應 `02:TEST-005` 執行前權限摘要 | 仍是自建部署（harness only，不含部署）——但這正是 ADR-005 要求的 |
| C：自建最小 agent loop | 直接用 Anthropic Messages API + 自寫 tool loop | 事件粒度完全自己掌握 | 要自己實作 Skill 載入、檔案工具、context 管理；等於重寫 Claude Code，與「驗證 Skill 生態」的產品目標無關，是純成本 |

> 註：Anthropic Managed Agents（CMA）此處**不列入評估**——它同時提供 harness 與託管沙箱，與 ADR-005「自建 Sandbox、平台承擔隔離責任」的既定決策直接衝突，且會讓 ADR-004 的 Provider-neutral 邊界形同虛設。若未來要引入，應走「受管理第三方 Provider」路徑並另立 ADR。

### 建議

**Runtime：Claude Agent SDK（TypeScript），`@anthropic-ai/claude-agent-sdk`，執行於 Node.js 22 LTS。**

> **供應商定案為 OpenAI 後，Runtime 選擇不變（v5）。** 「Claude Agent SDK → LiteLLM 閘道 → OpenAI 後端」**正是實測 7/7 通過的那條路徑**，不是推論：Spike §4 的閘道相容性與 §10／§11 的 Skill 載入與行為測試全部在此組態下完成，`/v1/messages` 路由由閘道轉譯到 OpenAI 的 Responses API。
> **選 SDK 的核心理由在此定案後反而更強**：Agent Skills 的載入是 Claude Agent SDK 的**原生機制**（檔案系統發現 ＋ `Skill` 工具 ＋ harness 攔截），與後端模型無關——§11.4 已實測白名單過濾的攔截點在 harness 而非模型。因此「換後端供應商」不需要重做載入設計，這正是選項 C（自建 agent loop）買不到的東西。

- Runtime Image 名稱：`skillhub/runtime-agent-sdk:2026.08-1`（digest 與 SBOM 依 ADR-005 保存）。
- 選 TypeScript 而非 Python 版：TS 版是 Claude Code 的參考實作路線，工具行為與版本節奏最接近；且 ADR-016 守則 5 已明訂「Sandbox 內 Agent Runtime 的語言由 Runtime Image 決定，與平台語言選型無關」，因此不與「平台用 Go／Python」衝突。
- **Skill 載入路徑（v4 依實測修正）：`<workdir>/.claude/skills/<skill-name>/SKILL.md`。** Run 開始前由 Sandbox Worker 以短效物件授權下載展開（SBX-008）。
  v1–v3 假設的 `<workdir>/skills/<skill-name>/` **已被證偽**——[pdm-003-litellm-spike-report.md](pdm-003-litellm-spike-report.md) §10 測項 2 實測該佈局在任何設定下都不被發現。

  **四個啟用條件（缺一則 Skill 不被載入，全部須寫進 SBX-002／SBX-008 的實作與 PDM-008 的安裝說明）：**

  | # | 條件 | 說明 |
  | --- | --- | --- |
  | 1 | `cwd` | `query()` 的 `ClaudeAgentOptions.cwd` 必須指向含 `.claude/skills/` 的目錄或其子目錄 |
  | 2 | `setting_sources` | 省略即可（預設載入 user＋project）；**但一旦顯式指定就必須含 `"project"`**，否則專案 Skill 全部消失。**Sandbox 內建議顯式寫 `setting_sources=["project"]`**——排除 `"user"` 可避免映像或掛載中的 `~/.claude/` 意外污染 Run |
  | 3 | `skills` | 省略時已發現的 Skill 預設啟用；`"all"` 全開、`list[str]` 白名單、`[]` 全關 |
  | 4 | 工具清單 | 若傳了顯式工具清單，**必須包含 `"Skill"`** |

  **不需要程式化註冊 API**——Skill 只能是檔案系統產物，這簡化了 SBX-008：展開檔案即完成註冊，沒有額外的註冊呼叫需要處理失敗與重試。
  **`.claude` 是隱藏目錄**：SBX-008 的展開與 SBX-009 的清理邏輯都需明確涵蓋，不能只掃可見目錄。

**模型（v5 依負責人定案改為 OpenAI 系列；全部經 LiteLLM 閘道，Sandbox 只持有該 Run 的短效 Virtual Key）：**

型號與定價依 [pdm-003-litellm-spike-report.md](pdm-003-litellm-spike-report.md) §11.2 於 2026-08-14 的查核（每 MTok，輸入／輸出／**快取命中輸入＝輸入的 1/10**）：GPT-5.6 家族 2026-07-09 GA——Sol $5／$30、Terra $2／$12、Luna $0.20／$1.20；GPT-5.5 $5／$30；GPT-5.4 家族——Mini $0.75／$4.50、Nano $0.20／$1.25。

| 用途 | 模型 | 單價（輸入／輸出／快取輸入，每 MTok） | 理由 |
| --- | --- | --- | --- |
| **Sandbox 試跑（預設）** | **`gpt-5.4-mini`** | **$0.75 / $4.50 / $0.075** | **依 §11.3／§11.6(2) 實測選定，不是成本偏好**：(1) 自主觸發率旗艦**沒有任何優勢**（Sol 與 Mini 同為 **0/9**），「用更強的模型換自主觸發」的假設被證偽；(2) 明確點名時兩者皆 PASS——試跑要驗的「Skill 內容能否正確驅動模型」在 mini 級完全成立；(3) **旗艦的失敗模式對 Sandbox 更不利**：Sol 三次中兩次改用 `Glob`／`Read` 去翻檔案系統找 `SKILL.md`（其中一次因此耗盡 turn 上限而中止），額外燒 turn 與 input token 並引入非預期路徑；(4) 成本差 **6.7 倍** |
| Sandbox 試跑（進階，使用者可選） | `gpt-5.6-sol` | $5 / $30 / $0.50 | 給「精深者」驗證高難度 Skill；預設關閉，選用時 UI 顯示成本差異（`02:TEST-005` 成本摘要）。**UI 不得暗示旗艦會提高 Skill 觸發率**——實測為 0/9，該宣稱不成立 |
| LLM Judge（`02:EVAL-001`） | `gpt-5.6-terra` | $2 / $12 / $0.20 | Judge 品質直接決定 M3 可信度，不宜用最便宜的；但 Judge 是純文字評分、**不呼叫工具**，旗艦的檔案系統探索失敗模式在此不會發生，因此中階足夠。**與試跑預設不同型號，順帶降低自我偏袒風險**（見風險表） |
| 索引時增強（ADR-013 第 1 段） | `gpt-5.6-sol` | $5 / $30 / $0.50 | 每個 Skill Version 只跑一次、總量極小（24–36 個），**這是全表唯一「品質完全壓過成本」的用途**——其產出會成為主要檢索欄位（PDM-011 §6.2-2），錯一次會污染整條檢索鏈 |
| 查詢改寫（ADR-013 第 2 段） | `gpt-5.6-luna` | $0.20 / $1.20 / $0.02 | 在 NFR-004 p95 < 2 秒的延遲預算內，選最快最便宜的一檔。**定位（v3 依真 Embedding 重測下修）：Top-1 精準度的增益步驟，不是召回的必要條件**——跨語言召回由向量腿承載。**降級路徑不變：改寫失敗或逾時 → 降級為向量檢索，不得降級為原句 FTS** |
| 符合原因潤飾（ADR-013 第 3 段） | `gpt-5.6-luna` | 同上 | 可選強化，逾時降級為模板。**但模板路徑不足以覆蓋無詞彙交集的命中**（v3：Spike §9.4-4 已由 nice-to-have 升為必要項），需在索引時一併生成「適用任務範例句」供向量召回的結果引用 |
| Embedding（回答 ADR-013 待決策） | `text-embedding-3-small`（1536 維），經 LiteLLM 路由 | 約 $0.02 / MTok | **維持不變**，且供應商定案後**變成同一家的原生模型**——原本「Anthropic 無 embedding 端點，需另接一家」的權宜安排消失，多供應商面縮減為零。1536 維在 pgvector HNSW 上參數成熟、索引體積小；語料僅數十筆，換模型全量重建成本可忽略。**首要驗收條件（跨語言召回）已實測通過，見下。** 備選 `voyage-3` 已由定案前置降為選項 |

> ⚠️ **試跑預設 Prompt 的強制要求（v5，由 §11.3 實測直接導出）：預設 Prompt 必須明確指示呼叫被測 Skill。**
> 在 OpenAI 後端上，「Skill 被模型自主觸發」的基準率實測為 **0/9（旗艦與 mini 皆然）**，且對照組證明載入機制完好——這是**模型自主行為**，不是缺陷，換更強的模型也不會改善。三個直接後果：
>
> 1. **「Skill 是否被自主觸發」不可作為試跑成功與否的判準**（`02:EVAL-001`）。以它當判準，等於預設所有 Run 都失敗。
> 2. **CONTENT-007 的範例 Prompt 必須點名 Skill**，或把「明確點名」設為預設試跑模式。這同時是 PDM-008 兩個 Profile「安裝後驗證 Prompt」的寫法要求。
> 3. 若仍要把自主觸發率呈現為產品訊號，**必須另行標示為「探索性指標」並註明基準率為 0**，不得混入驗收結論。

> **Embedding 選型的首要驗收條件：繁體中文查詢 → 英文 `SKILL.md` 語料的 recall@5 —— v3：已實測通過。**
> [pdm-011-spike-report.md](pdm-011-spike-report.md) §9（v2 真 Embedding 重測，2026-08-13）以 `text-embedding-3-small` 對同一組 12 份樣本與 10 條查詢重跑：繁中原樣查詢對英文語料，**Top-3 召回 5/5（100%）、Top-1 4/5（80%）**（對照 BM25 的 Top-3 20%）。v1「唯一命中靠查詢夾帶英文詞」的假性命中不再是唯一來源——四條完全無詞彙交集的中文查詢全部被向量腿召回，其中三條直接是 Top-1。
> **結論**：`text-embedding-3-small` 在跨語言這一項**過關**，ADR-013 的降級路徑成立、不需重新設計。
> **`voyage-3` 對比降為選項**：Spike §9.6 認為 Top-3 已達 100%，沒有留下可供改進的空間；除非負責人判定原樣中文的 Top-1 80% 不足，否則不值得再引入一家供應商。**此項不再是定案前置。**
> 仍未涵蓋：Spike v2 直接呼叫供應商 API 而**未經 LiteLLM 閘道**（Spike 可接受，產品實作不可——鐵律 8）；索引時摘要與查詢改寫仍為模擬，未呼叫真 LLM。

**Virtual Key 注入機制（回答 ADR-017 待決策，＝威脅模型 Q11）：建議採環境變數。**
Go 控制平面在 `provisioning` 階段向 LiteLLM 管理 API 簽發帶預算與 TTL 的 Virtual Key，注入 Sandbox 為 `ANTHROPIC_BASE_URL`（指向閘道的 Anthropic 相容 `/v1/messages` 端點）與 `ANTHROPIC_AUTH_TOKEN`。Claude Agent SDK 原生讀取這兩個變數，因此不需要改寫 Skill、也不需要客製設定檔——這是選 B 而非 C 的一個實際好處。

> ⚠️ **已知殘餘風險（v2 補，回應威脅模型 TM-SEC-02 與 Q11）**：威脅模型明確指出「**環境變數對 Script 而言最容易被讀走**」。本提案仍選環境變數，是因為設定檔對 Sandbox 內的 Script 同樣可讀（兩者都在同一個檔案系統與程序環境內），設定檔換來的是實作複雜度而非實質隔離；真正的緩解在於**限制爆炸半徑**，不在於藏得比較深。已生效的緩解，全部是基線既有機制：
>
> - **每 Run 一把、帶預算上限與速率上限**（數值見 PDM-005 §5.2）。**v3 修正：預算是軟上限，不是硬性截斷。** [pdm-003-litellm-spike-report.md](pdm-003-litellm-spike-report.md) §4.2／§6.3 實測 LiteLLM 的 spend 記帳為非同步（先扣後檢，超出判定發生在**下一次**請求），即使把 flush 間隔設到每秒仍需重試迴圈才觀測到拒絕。因此爆炸半徑應寫為「**一次 Run 的預算金額，加上 flush 間隔內可發出的請求量**」——在 Sandbox 內腳本可高併發呼叫的前提下，這個尾巴不是零。
>   **v5 補強（§11.5.4 實測）**：預算金額與 token 上限**已脫鉤約 7～8 倍**（快取後同一筆錢買得到的 token 多 7–8 倍），因此 `max_budget` **不能再被當成 token 上限的代理**。三層併用才完整：`max_budget`（依快取後實價編列的**花費**煞車）＋ `tpm_limit`（即時速率，不依賴 spend flush）＋ **Go Worker 依閘道回報的 `input_tokens` 累計強制 token 上限**。**此為 PDM-003 定案時應併同寫入的條件。**
> - **短 TTL**：TTL 不長於 Run 的硬上限（15 分鐘）加清理餘裕，建議設 20 分鐘。
> - **終止即撤銷**：涵蓋成功、失敗、取消、逾時、crash 與節點失聯所有路徑（基線 D-06，且撤銷須併入冪等清理 RUN-007）。
> - **供應商金鑰永不進 Sandbox**（鐵律 8）：外洩的是短效 Virtual Key，不是供應商 Key。
>
> **仍未被緩解的部分**：憑證在 TTL 內可被濫用（威脅模型已列為 TM-SEC-02 的殘餘風險）；撤銷可靠性取決於清理實作。此殘餘風險由負責人在定案時確認接受。
>
> **兩項必須併同執行的配套：**
>
> 1. **Virtual Key 樣式須列入 Trace 遮罩規則（TRACE-005）。** 環境變數可被 Script 直接 `echo` 進 stdout，經 `03:TRACE-003`（Script Log）流入 Trace，構成鐵律 11 的直接違反路徑。遮罩規則需涵蓋 LiteLLM Virtual Key 的前綴樣式與 `ANTHROPIC_AUTH_TOKEN` 值本身，並列入 QA-005 的遮罩測試。
> 2. **Runtime Image 必須確保 `ANTHROPIC_API_KEY` 未設。** Anthropic SDK 的憑證解析順序為 `ANTHROPIC_API_KEY` → `ANTHROPIC_AUTH_TOKEN` → OAuth profile；兩者同時存在時 SDK 會同時送出兩個標頭而被 API 拒絕。SBX-002 的映像建置與 provisioning 階段都需斷言此變數為未設定（不是空字串——空值同樣佔用優先順位）。

> ✅ **前置 Spike 狀態（v4 更新）：三項全部解除，PDM-003 的技術前置完成。**
>
> | # | 驗證項 | 狀態 |
> | --- | --- | --- |
> | 1 | **LiteLLM 閘道相容性**（Anthropic 相容端點 ＋ tool use ＋ streaming ＋ 每 Run 短效 Virtual Key） | **通過，7/7 測項 PASS**。見 [pdm-003-litellm-spike-report.md](pdm-003-litellm-spike-report.md) §4。**退路（Claude Code CLI 或閘道前薄轉譯層）不需啟動** |
> | 2 | **Agent SDK 的 Skill 載入路徑** | **通過，6/6 測項 PASS**（同報告 §10）。**但通過的是機制、不是提案原本寫的那條路徑**——正確路徑為 `.claude/skills/`，已於上方修正 |
> | 3 | 真 Embedding 跨語言召回 | **通過**。見上方 Embedding 選型段 |
>
> **第 2 項的結論可直接轉移到 Anthropic 後端**：判定依據取自串流開頭 `subtype=init` 的 system message 的 `skills` 陣列，測項 1–5 讀完該事件即中止、**完全不呼叫模型**，因此與後端模型無關。
>
> ✅ **補測狀態（v5）：四項全部結案，PDM-003 的技術工作完結。**
>
> 供應商定案為 OpenAI 後，補測不再需要 Anthropic 憑證——[pdm-003-litellm-spike-report.md](pdm-003-litellm-spike-report.md) **§11** 直接在正式後端上完成（189 次模型呼叫，實際計費 $0.93）。
>
> | 補測項 | 狀態 | 結論 |
> | --- | --- | --- |
> | `thinking` 透傳 | **不再適用（緣由：供應商定案）** | 該項要驗的是「Anthropic 後端會不會原生透傳 `thinking`」。後端定為 OpenAI 後**沒有 Anthropic 路徑可測**，問題性質從「待驗證」變成「已知的固定組態」——見下方發現 (a) 的 v5 改寫：`MAX_THINKING_TOKENS=0` 由 Spike 變通升格為 **Runtime Image 的常設設定** |
> | prompt caching 下的 `cache_read_input_tokens` | **已完成**（§11.5） | 實測 LiteLLM 1.96.2 在 `/v1/messages` 上**完全不輸出 cache 用量欄位**（缺欄，不是 0），但 **spend log 的計費金額有正確套用 9.4× 快取折扣**。→ **計費準確、可觀測性缺欄**。校正結果全部回填 §5.2 |
> | 跨供應商 fallback 對 Agent SDK 路徑的可用性 | **不再適用（緣由：供應商定案）** | 單一供應商，MVP 無跨供應商 fallback 場景。**此限制仍需記入 ADR-017 的已知邊界**：日後若要新增第二家供應商作 fallback，這一項必須先實測，不可假設可用 |
> | 模型自主觸發 Skill 的能力 | **已完成**（§11.3） | **0/9，旗艦與 mini 皆然**，且對照組（明確點名）兩者都 PASS。答案是「**不能**」——自主觸發不可作為試跑判準。已回填至上方模型表的強制要求 |
>
> **連帶結案的 SBX-002 實測項**：`skills` 白名單的**行為性**過濾效果（§10.5 記錄項 a 的疑問）亦已由 **§11.4 實測回答＝有效**，見下方發現 (d) 的 v5 改寫。
>
> **第 1 項實測衍生的三項影響（全部需在定案時處理）：**
>
> **(a) `thinking` 參數在 `/v1/messages` 上不受 `drop_params` 管轄。（v5 由「待補測」改寫為「常設組態」）** 同一組設定下，`/v1/chat/completions` 送 `reasoning_effort` 會被正確丟棄（200 OK），但 `/v1/messages` 送 `thinking` 區塊會回 400——這不是模型能力問題，是 LiteLLM 1.96.2 的 Anthropic 相容路由未套用參數丟棄邏輯。**只在後端不是 Anthropic 模型時觸發。**
> **v5：供應商定案為 OpenAI，代表這個條件永遠成立，不是偶發情境。** 兩個後果隨之改變：**(i)** 客戶端設 `MAX_THINKING_TOKENS=0` **由 Spike 變通升格為 Runtime Image 的常設設定**，SBX-002 須將其與「`ANTHROPIC_API_KEY` 未設」並列為映像建置的斷言項——漏設不是效能問題，是每個 Run 的第一個請求就 400。**(ii)** ADR-017「模型抽換與 fallback 設定在閘道層」的限制仍然存在，但在 MVP 不會被觸發（單一供應商）；記入 ADR-017 的已知邊界，日後新增第二家供應商前必須實測。（此為 ADR 既有決策，本提案僅記錄，不修改 ADR。）
>
> **(b) Virtual Key 預算是軟上限**——已回寫至上方殘餘風險段。
>
> **(c) Claude Agent SDK 的 harness 固定開銷 ≈ 19.4K input tokens／**次 API 呼叫**。（v5 依 §11.5 修正單位）** v3／v4 記的「約 50K／輪」是**一輪含工具呼叫的合計值**，不是單次呼叫的固定開銷，兩者在本文件曾被當成同一件事引用。正確拆法：**每輪 input ≈ 19.4K ×（1 ＋ 該輪工具呼叫次數）**——因為每一次工具結果回填都要把整個 harness 前綴重送一次。實測 5 輪純對話每輪 19.2–19.4K，曲線幾乎持平（每輪只增加約 40 token ＝上一輪的一問一答），**前綴才是主體**。原「50K」落在 1–2 次工具呼叫之間，對工具密集輪次是對的，對純對話輪**高估了約 2.5 倍**。
> 對 300K input 上限的換算見 §5.2。**可行的成本槓桿：Runtime Image 若能裁減內建工具集，此數字會顯著下降——值得在 SBX-002 設計時評估。注意 `skills` 白名單不是這條槓桿**（見 (d)）。
>
> **(d) Runtime Image 會夾帶 CLI 內建 Skill（v4 新增，同時是成本項與試跑干擾源）。** Spike §10 測項 3 觀測到：**即使 `setting_sources=[]`，`init.skills` 仍列出 15 個非本機專案的 Skill**（`init.plugins` 為空）——它們是 Claude Code CLI 自帶的內建 Skill，**不受 `setting_sources` 管轄**。兩個後果：
> **(i) 成本**：這些 Skill 的 metadata 是上述固定前綴的一部分，PDM-004 的 Runtime Image 裁減應一併評估（見 §4 風險表）。
> **(ii) 試跑正確性**：試跑「使用者的 Skill」時，模型可能改用內建 Skill，構成試跑結果的干擾源——同一個 Run 可能因為內建 Skill 而「成功」，但使用者下載的 Skill 其實沒被用到，這會讓 `02:EVAL-001` 的驗收結論失真。
> **v5：緩解已由 §11.4 行為性實測證實有效，並採為 SBX-002 預設。** 設 `skills=["<被測 Skill 名>"]` 後，被排除的 Skill（**專案的與內建的都測過**）在模型嘗試呼叫時由 harness 直接回 `<tool_use_error>… is not in this session's skills allowlist</tool_use_error>`，內容不會進入對話。**攔截點在 SDK harness 而非模型，因此結論與後端模型無關，換模型不需重測。** 三點限制與具體寫法見 §4 PDM-004。
>
> **另：配套第 2 項（`ANTHROPIC_API_KEY` 須未設）的必要性已被實測證實**，Spike 觀測到 Claude Code 主動印出憑證來源優先順位的警告。該配套維持原樣。
>
> **部署註記**：Spike 記錄 `pip install` 的 proxy 在測試平台上只能跑無資料庫模式，任何需要 Virtual Key 的功能必須用官方 container image。這與 ADR-017「LiteLLM Proxy 作為獨立部署單元」一致，但應寫進未來的開發環境文件——開發者本機要跑帶 Virtual Key 的閘道，不能靠 `pip install`。

### 理由

- Agent Skills 的載入語意目前只有 Claude Code／Agent SDK 這條路徑是原生的；自己實作（選項 C）等於自訂一套「我們認為 Skill 該怎麼載入」，違反產品原則 5「可攜但不過度承諾」——我們會驗證出一個只在 Skill Hub 成立的結論。
- SDK 的 hooks 機制讓 `03:TRACE-002`（Skill 啟用、資源載入）與 `03:TRACE-003`（Tool Call、Script Log）可以在不 patch Runtime 的前提下取得結構化事件，直接支撐 `02:TRACE-001`「事件順序可被重建」的允收準則。
- permissions 機制對應 `02:TEST-005`「執行前權限摘要」與「權限有變更必須重新確認」，不必自己造一套。
- 模型分層讓 ADR-017 的「Run 級成本歸因」有意義：搜尋路徑用 `gpt-5.6-luna`、試跑路徑用 `gpt-5.4-mini`、索引與 Judge 用旗艦／中階，成本結構在 Usage Record 上一眼可分。**分層的成本槓桿在 v5 反而更大**——同一組 300K／60K 上限在 mini 與旗艦之間差 6.7 倍，分層失效（全用旗艦）會讓模型帳單直接翻約 6.6 倍（見 [cost-estimation.md](cost-estimation.md) §6.2.3）。
- **查詢改寫的定位與降級路徑（v3 依真 Embedding 重測定稿）**：[pdm-011-spike-report.md](pdm-011-spike-report.md) v1 §4.4(c) 量到繁中查詢對英文語料的純詞彙召回 Top-3 僅 20%，因而在 §6.2-1 建議把改寫升格為「召回的必要條件」。**v2 真 Embedding 重測推翻了這個升格**：向量腿在繁中原樣查詢下 Top-3 召回 100%、Top-1 80%，改寫只是把 Top-1 補到 100%。§9.4-1 因此下修為「**向量腿才是跨語言召回的承載者，改寫是 Top-1 精準度的增益步驟**」。
  兩個實務後果：**(i)** NFR-004 的延遲風險下降——改寫既非必要條件，逾時就砍掉是可接受的降級，不是品質懸崖；**(ii)** 但**降級目標仍必須是向量檢索**——降級後只走原句 FTS 才是真正的失效路徑（Top-3 20%），會直接摧毀繁中使用者的 `02:DISC-001` 體驗。這一條結論從 v2 到 v3 不變。
- **不要把 RRF 當成品質來源（v3 新增）**：Spike 兩次量測都顯示等權 RRF 相對最佳單腿**沒有增益**，兩種設定下還倒退一名。v1 把零增益歸因於兩腿同質（top-3 重疊 73%），**v2 推翻此解釋**——重疊已降到 37%～63%，兩腿確實送不同訊號，RRF 依然無增益；真正原因是**兩腿品質不對稱**（中文原樣查詢下 FTS 腿幾乎全滅，等權融合只會稀釋強腿）。
  因此本提案引用混合檢索時，其價值一律表述為**召回覆蓋**（某一腿失效時另一腿仍有結果），不是融合帶來的排序增益。另有一個必須落實的實作細節：**零命中的腿不得參與融合**（Spike §9.5，這正是 v1 量測缺陷的來源）。加權融合或依查詢語言動態調權超出 MVP 範圍，列為後續優化。

### 風險

| 風險 | 影響 | 緩解 |
| --- | --- | --- |
| ~~LiteLLM 的 Anthropic 相容端點與 Agent SDK 不完全相容~~ **（v3 降級）** | ~~PDM-003 整個方案失效~~ | **已由前置 Spike 第 1 項證偽，7/7 PASS，退路不需啟動。** 殘留的是範圍較窄的一項：`thinking` 透傳未驗證，且該路由的參數處理已知有缺陷（見上方發現 a）。補測未過則重新評估退路 |
| 綁定單一 Agent 生態，違反「不綁定單一執行環境」的產品目標 | 相容性結論過窄 | ADR-012 的三層相容性（格式／能力／行為）已把這件事說清楚：MVP 只宣稱「在 Claude Agent SDK 上行為相容」，其他 Agent 一律標「未驗證」（`02:PACK-002` 允收準則） |
| Agent SDK 版本升級改變工具行為，歷史 Run 不可重現 | 違反「所有執行可追溯到 Runtime」 | Runtime Image 版本化 + digest 記錄（基線 I-02 要求 pin by digest 而非 tag）；ADR-005 已要求「Image 更新不得改變歷史 Run 記錄中的 Runtime Version」 |
| ~~Judge 與試跑用同一模型，可能有自我偏袒~~ **（v5 已結構性緩解）** | `02:EVAL-001` 可信度 | v5 的分層讓試跑（`gpt-5.4-mini`）與 Judge（`gpt-5.6-terra`）**本來就不同型號**，最直接的自我偏袒路徑已消失。**殘留：兩者仍為同一供應商的同一模型家族**，家族層級的共同偏誤無法由分層排除——記錄為已知限制，M3 可加入跨家族 Judge 的 A/B，不列入 MVP 必要範圍 |
| ~~Agent SDK 從工作目錄載入 Skill 的實際路徑慣例未經查證~~ **（v4 已實現並修正）** | 原假設 `<workdir>/skills/<skill-name>/` **確實不成立** | **風險已兌現但代價僅為文件修正**：前置 Spike 第 2 項證偽該路徑，正確路徑為 `<workdir>/.claude/skills/<skill-name>/`，已於上方建議段與 §7 PDM-008 同步修正。載入**機制**本身 6/6 通過，PDM-003 的載入設計與 PDM-008 Profile 都不需重做，只換路徑與補上三個啟用條件 |
| 試跑時模型改用 CLI 內建 Skill 而非受測 Skill | `02:EVAL-001` 的驗收結論失真——Run 顯示成功，但使用者要下載的 Skill 其實沒被用到 | **v5：`skills` 白名單只放受測 Skill，過濾效果已行為性實測有效**（§11.4，對專案與內建 Skill 皆然）。Trace 仍需顯示「實際被啟用的是哪一個 Skill」（`03:TRACE-002`），並**把白名單拒絕（`tool_use_error`）獨立標示**，避免使用者誤判為 Skill 本身有問題 |
| **白名單被誤認為 token 成本槓桿**（v5 新增） | SBX-002 若把白名單當成裁減開銷的手段，會得到零效益並錯過真正的槓桿 | §11.4 實測：白名單擋的是**執行**、不是**曝光**——被排除的 Skill 仍在模型可見清單中（模型會先嘗試呼叫才被擋），因此**不會降低約 19.4K 的 harness 固定開銷**，反而**每次誤觸多一次無效工具往返（＋約 19.4K input）**。要削減 token 成本只有「裁減 Runtime Image 工具集」那條獨立路徑 |

---

## 4. PDM-004：SelfHostedProvider 首批 Runtime 語言與版本

> **現況覆寫（2026-08-19；本節仍保留為歷史提案）**：目前 Runtime Image 固定使用 Python 3.11，Claude Agent SDK 0.3.233 的實作 Profile 會省略 `setting_sources`；顯式 `setting_sources=["project"]` 在目前版本會造成 project Skill 為零。權威現況見 ADR-023、`contracts/packaging/profiles/claude-agent-sdk.json`、`apps/sandbox/README.md` 與 `UPGRADES.md`。Python 3.12 是否升級仍屬 PDM-004 待追認與真實 runsc 驗證項，以下 Python 3.12／`setting_sources=["project"]` 敘述不得當成已部署事實。

### 背景

ADR-005 要求「MVP 只允許預先建置、版本化與掃描的 Runtime Image」「不允許使用者直接指定任意基礎 Image」「限定少量 Runtime、檔案格式與最大資源」。PDM-001 的三個類別決定了 Skill 內 Script 實際會用到什麼。

### 評估選項

| 選項 | 內容 | 優點 | 缺點 |
| --- | --- | --- | --- |
| A：只有 Node | 僅 Agent SDK 執行所需 | Image 最小、攻擊面最小 | `data` 與 `documents` 類別的 Skill 幾乎全用 Python 腳本（openpyxl、pandas、python-docx），直接淘汰兩個類別 |
| B：Node + Python + POSIX 工具 | Agent SDK 跑在 Node，Skill Script 可用 Python 或 shell | 覆蓋 PDM-001 三類別的實際需求；套件白名單可控 | Image 較大（約 1.2–1.8 GB）；需要維護套件白名單 |
| C：B ＋ 執行期套件安裝 | 允許 `pip install` / `npm install` | Skill 相容性最高 | 需要對外網路（違反 SBX-007 預設封鎖）或維護內部 mirror；供應鏈風險（ADR-007）；MVP 不值得 |

### 建議

**採選項 B。**

| 項目 | 版本／內容 |
| --- | --- |
| Agent Runtime | Node.js **22 LTS** |
| Script 語言 | Python **3.12**（`/usr/bin/python3`），Bash **5.2**（POSIX 工具：coreutils、jq、ripgrep、unzip） |
| 預裝 Python 套件（白名單） | `openpyxl`、`python-docx`、`python-pptx`、`pypdf`、`pdfplumber`、`pillow`、`pandas`、`numpy`、`chardet`、`python-dateutil`、`markdown-it-py`、`pyyaml`、**`lxml`（v5 新增，必要）**、**`matplotlib`（v5 新增，建議）** |
| 預裝 Node 套件 | 僅 Agent SDK 及其相依；不預裝通用工具庫 |
| **`skills` 白名單（v5 新增，SBX-002 預設設定）** | 每個 Run 一律設 `skills=["<被測 Skill 名>"]`（只放被測 Skill），搭配 `cwd` 與 `setting_sources=["project"]`。過濾效果已由 [Spike §11.4](pdm-003-litellm-spike-report.md) 行為性實測證實有效，對**專案與內建 Skill 皆然**，且攔截點在 SDK harness 而非模型（換後端不需重測） |
| **內建工具與內建 Skill 的裁減（v4 新增，v5 更新數字）** | Agent SDK 夾帶 20+ 項內建工具與 **15 個 CLI 內建 Skill**（後者不受 `setting_sources` 管轄）。**兩者都應在 SBX-002 評估裁減**：它們合計構成約 **19.4K input tokens／次 API 呼叫**的固定前綴（v4 記的 50K 是「一輪含工具呼叫」的合計，單位不同）。裁減幅度需與 Skill 相容性權衡——裁掉 `Read`／`Write`／`Bash` 會讓多數 Skill 失效。**這是唯一真正削減該前綴的路徑，`skills` 白名單不是** |
| **常設環境變數斷言（v5）** | `ANTHROPIC_API_KEY` **未設定**（不是空字串）；`MAX_THINKING_TOKENS=0`。兩者都是漏設即在每個 Run 的第一個請求失敗的組態，須在映像建置與 provisioning 兩處斷言。理由見 §3 配套第 2 項與發現 (a) |
| **明確不含** | 編譯器工具鏈（gcc/make）、GPU 驅動與 CUDA、瀏覽器與 headless Chrome、資料庫 client、`git`、任何雲端 SDK |
| 套件安裝 | **執行期禁止**。`pip`／`npm` 的 registry 位址不在 egress 允許清單內（基線 N-01 default-deny、N-04 Proxy 固定 DNS），安裝必然失敗。**但失敗訊息不會是可理解的**——default-deny + 固定 DNS 下取得的是通用連線或 DNS 解析錯誤，因此可理解性必須由平台在**匯入階段**預先揭露（見下方風險表），不倚賴 Runtime 錯誤輸出（NFR-007「所有錯誤提供下一步行動」） |
| Image 命名 | `skillhub/runtime-agent-sdk:2026.08-1`，保存 content digest、SBOM、漏洞掃描報告（ADR-005） |
| 執行身分 | UID 10001 非 root、非特權、基礎檔案系統唯讀，可寫路徑僅 `/work`（暫存）與 `/out`（Artifact 輸出） |

> ⚠️ **採用 `skills` 白名單時必須一併寫入的三點限制（v5，Spike §11.4）——三點都是「別對它期待錯的東西」：**
>
> 1. **`init.skills` 不反映白名單。** 該陣列反映的是「發現」而非「過濾」，設白名單後內容不變。**驗證白名單是否生效只能用行為測試**（送一個點名被排除 Skill 的 Prompt，看是否回 `tool_use_error`），不能讀 `init.skills` 判定。QA 若寫成讀陣列，會得到一個永遠通過的假測試。
> 2. **擋的是「被執行」，不是「被看到」。** 被排除的 Skill 仍在模型可見清單中——實測模型會**先嘗試呼叫才被擋**。因此白名單不降低 harness 的固定前綴開銷，也不能完全消除注意力干擾；它保證的只有「干擾 Skill 的內容不會進入對話」。
> 3. **會多一次無效的工具往返。** 模型嘗試呼叫被擋的 Skill 時，該輪多一次 API 呼叫（**＋約 19.4K input**）。在 300K 上限下這不可忽略——約等於吃掉半輪的預算。`03:TRACE-003` 應把白名單拒絕獨立標示，避免使用者把它誤讀為 Skill 本身有問題。

### 理由

- 套件白名單是直接從 PDM-001 三個類別反推的，不是憑空列的：`documents` → docx/xlsx/pptx/pdf 四套 + pillow；`data` → pandas/numpy/chardet；`writing` → markdown-it-py/pyyaml。
- **v5 的兩項增補同樣是反推，不是擴充傾向**，依據 [data-category-sourcing.md](data-category-sourcing.md) §5：
  - **`lxml`（必要）**：`data` 合格候選中占近半的 12 個 `excel-*` 全部用它做 xlsx XML 層操作。**不加則 `data` 供給由 25 掉到 13**——仍在 8–12 目標之上，但精選池變薄且來源 repo 由 7 降為 6，PDM-011 的語意分散度受損。
  - **`matplotlib`（建議）**：PDM-001 給 `data` 的定位是「資料整理**與分析**」，現行白名單覆蓋得了「整理」但**完全無法產出圖表**，「分析」只剩文字。兩個高星等且長期維護的候選（`cohort-analysis`／`ab-test-analysis`）正是卡在此。
  - **`scipy`／`sklearn` 維持排除**：引入後重新進場的候選其 Script 規模仍不過精選標準第 4 項（2,860～6,197 行），收益有限而攻擊面與映像體積都上升。
- 排除 `git` 與雲端 SDK 是刻意的：這兩者是「Skill 想偷偷做網路 I/O」最常見的載體，移除後 egress 政策的執行成本大幅下降。
- 執行期禁止安裝，讓「依賴宣告」（`02:SKILL-002` 靜態檢查項）從一個提示性欄位變成一個**可強制執行的合約**：宣告了白名單外的依賴 = 精選標準第 6 項不通過。

### 風險

| 風險 | 影響 | 緩解 |
| --- | --- | --- |
| **Node.js 22 / Python 3.12 在 gVisor（runsc）下的 syscall 相容性未驗證** | ADR-015 明列「少數 syscall 不相容需驗證目標 Runtime」，且其**定案條件之一就是「PDM-004 選定的 Runtime 在 gVisor 下通過完整 Run 生命週期」**（威脅模型 §5.6 已列為獨立測試類型，對應基線 C-09）。未驗證即定案，等於 PDM-004 與 ADR-015 互相等待 | **在選定平台上實跑 runsc，跑通一次完整 Run 生命週期**（provisioning → preparing → running → 產出 Artifact → cleanup），Node 與 Python 兩條路徑各一次。已知需重點觀察：Node/V8 的執行緒與記憶體管理、非同步 I/O 路徑、Python 子程序與檔案描述符行為。此驗證列入 §9 檢查清單；**每次擴充 Runtime 需重跑**（ADR-015 明文） |
| 白名單太窄，好 Skill 被擋在門外 | 供給受限 | 白名單是**資料**不是程式碼；新增套件走 Image 版本升級流程（新 tag、新 digest），約定每兩週可增補一次 |
| 使用者不理解「為什麼裝不了套件」 | 體驗挫折（NFR-007「所有錯誤提供下一步行動」） | 匯入階段就在詳情頁顯示「此 Skill 需要 X，目前 Runtime 未提供」，不要等到 Run 失敗才說——**這是唯一的可理解性來源**，Runtime 端只會產生通用網路錯誤 |
| Image 體積導致 Sandbox 啟動時間過長 | NFR-004「Sandbox 建立時間需被量測」 | 節點預拉 Image；量測 provisioning p95 並列入 O11Y-001。**成本試算對此高度敏感**：[cost-estimation.md](cost-estimation.md) §6.1 把「單 Run 佔用時間」列為唯一應優先量測的變數，翻倍即 Sandbox 池 +60% |

---

## 5. PDM-005：Dataset 大小、檔案類型與單次 Run 資源上限

### 背景

`02:TEST-002` 要求「上傳前顯示大小限制」，`02:RUN-003` 要求「套用 CPU、記憶體、磁碟、程序數與執行時間限制」，但兩者都沒有數值。ADR-017 額外要求 Virtual Key 帶預算——這是 MVP 唯一能硬性止血模型成本的機制。

本節同時承接威脅模型的三個開放問題：**Q5**（資源上限具體數值，基線 C-10～C-15 有檢查但無值可驗）、**Q6**（Skill 套件解壓上限，見 §5.1b）、以及 TM-EXE-02 對「具體數值是 PDM-005 待決」的等待。

> **注意 ADR-005 沒有數值。** 下表「依據」欄標 ADR-005 者，指的是「ADR-005 要求必須設定此項限制」，不是「ADR-005 規定了這個數字」——ADR-005 只寫「限制 CPU、記憶體、磁碟、程序數、檔案描述符與最大執行時間」。所有具體數值都是本提案首次提出，需要負責人定案。

### 評估選項

| 選項 | 取向 | 適合 | 不適合 |
| --- | --- | --- | --- |
| A：寬鬆（單檔 500 MB、Run 60 分鐘） | 最大相容性 | 成熟產品、有付費機制 | MVP 免費額度會被單一使用者吃光；Sandbox 節點容量規劃困難 |
| B：中等（單檔 25 MB、Run 10 分鐘） | 覆蓋絕大多數真實個人創作者情境 | MVP | 少數大型資料集使用者受限 |
| C：嚴格（單檔 5 MB、Run 3 分鐘） | 成本最低 | 純 demo | 連一份 50 頁 PDF 或一份中型 CSV 都放不下，`data` 類別失去意義 |

### 建議

**採選項 B。具體數值如下：**

#### 5.1 Dataset

| 項目 | 限制 |
| --- | --- |
| 單一檔案大小 | ≤ 25 MB |
| 單一 Test Case 檔案總量 | ≤ 100 MB |
| 單一 Test Case 檔案數 | ≤ 20 |
| 允許的檔案類型（**以 magic bytes 判定，不信任副檔名**） | 文字：`.txt` `.md` `.csv` `.tsv` `.json` `.jsonl` `.xml` `.yaml` `.yml`<br>文件：`.pdf` `.docx` `.xlsx` `.pptx`<br>影像：`.png` `.jpg/.jpeg` `.webp`<br>封存：`.zip`（單層、解壓後仍受總量與檔數限制） |
| 明確拒絕 | 可執行檔（ELF／PE／Mach-O）、`.exe` `.dll` `.so` `.dylib`、巢狀壓縮、符號連結、路徑含 `..` 的封存項目 |
| 錯誤訊息 | 依 `02:TEST-002`「不洩漏系統資訊」：只回報「檔案類型不支援」「超過 25 MB 上限」，不回報偵測到的實際 magic bytes 或內部路徑 |

#### 5.1b Skill 套件匯入上限（v2 新增，承接威脅模型 Q6）

**與 Dataset 分開規範，因為爆炸半徑不同。** Dataset 在 Sandbox 內解壓（T0 區，逃逸才有影響）；**Skill 套件在控制平面側解壓**（TM-IMP-02），解壓炸彈直接打的是 T2/T3 區的匯入節點。威脅模型明列此上限屬 PDM-005 範圍，且 Q6 列於「阻擋 SEC-002 定案的問題」。

| 項目 | 限制 | 理由 |
| --- | --- | --- |
| 上傳套件壓縮檔大小 | ≤ 10 MB | Skill 是文字與腳本，非資料；`anthropics/skills` 最大的 skill 目錄仍遠低於此 |
| **解壓後總大小** | ≤ 100 MB | 壓縮比上限即 10:1；超過即判定為壓縮炸彈並中止 |
| **解壓過程的即時檢查** | 邊解壓邊累計，超過上限**立即中止**，不等解壓完成 | 事後檢查擋不住 zip bomb——磁碟在檢查前就滿了 |
| 檔案總數 | ≤ 2,000 | 涵蓋含資產檔的大型 Skill，同時擋住百萬小檔耗盡 inode |
| 單一檔案大小 | ≤ 10 MB | 與壓縮檔上限一致 |
| 目錄巢狀深度 | ≤ 10 層 | — |
| **壓縮巢狀層數** | **1 層（不允許壓縮檔內含壓縮檔）** | 巢狀壓縮是繞過解壓上限的標準手法 |
| **符號連結** | **一律拒絕**（不是解析後檢查，是直接拒絕含 symlink 的套件） | ADR-007 的「安全解壓」需求；解析後檢查有 TOCTOU 空間，直接拒絕沒有 |
| 路徑項目 | 拒絕絕對路徑、含 `..` 的相對路徑、Windows 磁碟機代號與 UNC 路徑 | 路徑穿越（TM-IMP-02） |
| 檔名 | 拒絕控制字元、NUL、保留裝置名（`CON`、`PRN`、`NUL` 等） | 跨平台解壓安全 |
| 失敗行為 | 匯入工作轉失敗並列出**原因分類**（超過大小／檔數／含 symlink／路徑穿越），不列出實際路徑內容 | `02:SKILL-001`「匯入結果顯示成功、警告或失敗，並列出原因」＋ 不洩漏系統資訊 |

適用對象：`03:INGEST-002`（上傳套件）與 `03:INGEST-001`（URL 匯入）取得的內容，兩條路徑共用同一組上限與同一份實作。

#### 5.2 單次 Run 資源上限

| 項目 | 上限 | 依據 |
| --- | --- | --- |
| vCPU | 2 | ADR-005 要求設 CPU 上限（無數值）；本提案定值，基線 C-10 據此可驗 |
| 記憶體 | **4 GiB** | pandas 處理 100 MB CSV 的合理上限。**此值已被 [cost-estimation.md](cost-estimation.md) v2 §2.1 採為 Sandbox slot 規格**（v1 誤用 2 GiB），並連帶決定各平台機型系列（該文件 §2.3：4 GiB/vCPU 淘汰所有運算最佳化機型）。**調整此值必須同步重算成本試算** |
| 暫存磁碟 | 8 GiB（`/work` 6 GiB + `/out` 2 GiB） | Dataset 100 MB + 解壓 + 產物。`/out` 2 GiB 相對 Artifact 100 MB 上限寬鬆，是為容納中間產物 |
| 程序數（PID） | 256 | ADR-005 要求設 PID 上限（無數值）；本提案定值 |
| 檔案描述符 | 1024 | ADR-005 要求設 FD 上限（無數值）；本提案定值 |
| Wall clock | **軟上限 10 分鐘**（達到即進入 `timed_out`）；**硬上限 15 分鐘**（強制銷毀） | `02:RUN-004`。成本試算 §2.1 以「中位 6 分鐘佔用」規劃容量，與此上限的關係見下方註記 |
| **同一 Workspace 並行 Run 上限** | **2**（v2 新增） | ADR-011 Policy 明列「每 Workspace 的並行 Run 上限」為 MVP 必備；威脅模型閘門 B 把「Workspace 超出並行 Run 上限或額度」列為阻擋啟動條件，但此前無值可驗。取 2 而非 1，是為讓使用者能在等待一個長 Run 時另開一個快速試跑；取 2 而非更高，是因為封測規模的尖峰併發 slot 本來就只有 1–2（成本試算 §2.2） |
| Artifact 輸出總量 | ≤ 100 MB，單檔 ≤ 25 MB | 超過即截斷並在 Trace 標記 |
| 模型 Token（**v5：改由 Go Worker 依 `input_tokens` 累計強制**，Virtual Key 預算不再代理此上限） | 每 Run ≤ **300K input / 60K output**<br>✅ **「未經驗證」標記已解除（v5）** | 見下方 §5.2a 的完整校正 |
| Egress | default-deny；允許清單僅三項：LiteLLM 閘道位址、物件儲存的短效授權端點、Trace ingestion 端點。三者皆須經受控 Egress Proxy，無旁路路徑 | ADR-005 執行節點拓撲（Egress Proxy／Scoped Object Transfer／Trace Ingestion 三條路徑）、基線 N-01／N-02／N-07 |

#### 5.2a Token 上限的校正與強制方式（v5 新增，依 [Spike §11.5／§11.6](pdm-003-litellm-spike-report.md)）

**(1) 300K input 維持不變，「未經驗證」解除。** v3 標註未經驗證的原因是「無 prompt caching 下只夠 5–6 輪」，並預期補測後可用輪數會顯著上升。**補測已完成，該預期不成立**——但 300K 這個數字本身經實測後**不需上調**。

**(2) 上限必須帶「每輪工具呼叫次數」這個參數才有意義。** harness 固定開銷實測為 **約 19.4K input tokens／次 API 呼叫**（不是「／輪」），每一次工具結果回填都要把整個前綴重送一次，因此**每輪 input ≈ 19.4K ×（1 ＋ 該輪工具呼叫次數）**：

| 每輪型態 | 每輪 input | **300K 可跑輪數** |
| --- | ---: | ---: |
| 純對話，0 次工具呼叫 | ~19.6K | **約 15 輪** |
| 每輪 1 次工具呼叫 | ~39.1K | **約 7.7 輪** |
| 每輪 2 次工具呼叫 | ~58.5K（外推） | **約 5 輪** |

回寫 `02:RUN-003` 時**必須帶這張表，不能只寫「300K」或只寫一個輪數**——單一輪數在此沒有意義。v3 原估的「5–6 輪」對工具密集輪次是對的，對純對話輪低估約 2.5 倍。

**(3) prompt caching 完全不增加 300K 能買的輪數。** 命中快取的 token **一樣計入 `input_tokens`**；快取省的是錢，不是 token 數。實測 87% 的成本折扣對應的輪數增益是 **0**。

**(4) 因此金額預算與 token 上限已脫鉤約 7～8 倍，`max_budget` 不能再代理 token 上限。** 同一筆金額在快取後買得到的 token 多 7–8 倍：v3 寫的「$1.80 ≈ 300K/60K」在快取生效後實際要跑到約 **2.4M** input token 才會被擋。**三層併用才完整：**

| 機制 | 職責 | 依據 |
| --- | --- | --- |
| **Go Worker 依閘道回報的 `input_tokens` 累計** | **token 上限（300K）的唯一強制點** | `input_tokens` 在 `/v1/messages` 上實測可用（缺的只有 cache 欄位）。**2026-08-25 落地回填（`04` 丙-69）：這一列在寫下之後有一段時間不成立。** 唯一的實作是 `run.mjs` 的 `ceilingBreach`——**在它要限制的那個不受信任行程裡面**，而 `apps/platform` 一個 token 都沒累加過（同一份程式裡兩處註解對此互相矛盾：`gateway.go` 說「that is the worker counting」，`service.go` 說「the counting happens in the sandbox harness」）。工作負載自己拿著 `ANTHROPIC_AUTH_TOKEN`，繞過 `run.mjs` 直接打閘道時，實際上界只剩 `max_budget` 與 `tpm_limit`——**也就是本節自己算過的那個 2.4M，約為畫面上告訴使用者、而使用者按下確認的數字的八倍**。現由 driver 在既有的 2 秒輪詢上讀 LiteLLM 管理 API 的 `GET /spend/logs/v2`（以 `key_alias` 定址，所以 RUN-008 重啟後重新接上的 attempt 也問得到，且該端點刻意不回 `messages`／`response`），超過即走與 wall clock 完全相同的取消路徑。**`run.mjs` 的計數器保留**：合作的工作負載由它早一步、便宜地停下，不合作的由 Worker 停；兩邊都用嚴格大於，剛好停在上限的 Run 兩邊都放行。**檢查刻意 fail-open**（管理 API 不在沙箱網路上，為一次維運抖動殺掉健康的 Run 比較糟），所以它另立了一個 counter——一個沒有序列的 fail-open 守門，與一個正在運作的守門長得一模一樣，那正是本列曾經的狀態 |
| `max_budget`（Virtual Key） | **花費**煞車，依**快取後實價**編列，非牌價反推 | 軟上限，非同步記帳 |
| `tpm_limit`（Virtual Key） | 即時速率煞車，不依賴 spend flush | §6.3 |

**(5) 依 OpenAI 定價編列的 max_budget（每 Run，300K in／60K out 打滿）：**

| 檔位 | 冷快取（首次 Run，無折扣） | 熱快取（實效 87% 折扣） | **建議 `max_budget`** |
| --- | ---: | ---: | ---: |
| **`gpt-5.4-mini`（預設）** | $0.225 ＋ $0.27 = **$0.50** | $0.03 ＋ $0.27 = **$0.30** | **$0.50** |
| `gpt-5.6-sol`（進階） | $1.50 ＋ $1.80 = **$3.30** | $0.20 ＋ $1.80 = **$2.00** | **$3.30** |

`max_budget` **取冷快取值**：取熱值會讓每個使用者的第一次 Run 被誤擋，而首次 Run 正是漏斗最不能斷的一步。

**(6) 成本結構已翻轉：快取後 input 幾乎免費，單 Run 成本由 output 主導。** mini 檔位下 output 佔上限成本的 **90%**（$0.27 / $0.30）。連帶的兩個提醒：
- **`02:TEST-005` 的「預估成本區間」必須是區間不是單值**——首次與後續 Run 的單位成本差約 8 倍（快取保留 24 小時且 harness 前綴跨 Run 完全相同，第二次以後的 Run 直接命中前一次留下的快取）。
- **60K output 上限的成本槓桿現在遠大於 300K input 上限**。若日後要收窄單 Run 成本，該動的是 output。

**(7) 可觀測性缺口（已知，不阻擋定案）：** LiteLLM 1.96.2 在 `/v1/messages` 路由上**不輸出任何 cache 用量欄位**（`cache_read_input_tokens` 與 `cache_creation_input_tokens` 是**缺欄，不是 0**），spend log 該欄位亦為 `null`。**計費金額是準的**（實測冷／熱請求折扣 9.4×，與 `/v1/chat/completions` 一致），**受損的只有可觀測性**：Trace 無法呈現快取命中率、無法反推 input 中有多少是重複前綴；ADR-017 若讓 Langfuse 成本歸因依賴 cache 欄位，該欄位在此路由上不可用。**應在 O11Y-001 記為已知缺口，不要為它設計繞道實作。**

> **與成本試算的時間關係**：成本試算以「單 Run 佔用 6 分鐘（4 分執行 + 1 分 provisioning/gVisor 冷啟 + 1 分清理）」規劃容量，介於 PDM-010 觀測的中位 2 分鐘與本表的 10 分鐘軟上限之間。該文件 §6.1 明示：若實際佔用時間翻倍到 12 分鐘（等於「使用者普遍跑到軟上限」），Sandbox 節點需求增加約 60%。**這不是極端假設**——軟上限 10 分鐘加 provisioning 與清理正好約 12 分鐘。此為 NFR-004 首要量測項。

#### 5.3 執行前必須顯示（`02:TEST-005` 權限摘要的具體欄位）

Dataset 清單與總大小、將掛載的路徑、預計使用的 Runtime 版本、資源上限、逾時秒數、**Token 預算與預估成本區間**、egress 允許清單摘要（「僅模型閘道，無一般網際網路」）。

### 理由

- 10 分鐘軟上限對齊 MVP 承諾「10 分鐘內完成第一次搜尋到下載」——單一 Run 不該吃掉整個承諾預算。
- Token 上限而非只有時間上限，是因為時間上限擋不住「短時間內燒掉大量 token」的模式。**v5 修正一個 v3 的簡化**：Virtual Key 預算原本被當成現成的強制機制，但實測後它只能當花費煞車（軟上限、且與 token 數脫鉤 7–8 倍），token 上限需 Go Worker 自行累計——**這仍然不必另造機制**（`input_tokens` 由閘道回報、Go 本來就擁有 Run 狀態機），只是強制點從閘道移到 Worker。成本試算 §6.2 把 Token 預算列為「唯一能硬性止血的機制」的結論不變。
- magic bytes 驗證而非副檔名，直接對應 ADR-007 供應鏈風險與 SEC-003 匯入前掃描政策。
- Skill 套件上限與 Dataset 上限分開（§5.1b），因為前者在控制平面側解壓，後者在 Sandbox 內——同一個數字套用到兩個信任區是錯的。

### 風險

| 風險 | 影響 | 緩解 |
| --- | --- | --- |
| 10 分鐘對某些 `data` 類 Skill 太短 | 使用者感受「跑不完」 | Trace 明確標示「因逾時停止，已產生的結果保留」（`02:RUN-004` 允收準則）；蒐集逾時率（O11Y-001），若 > 10% 則調整而非降級功能。**注意調整逾時上限會直接放大 Sandbox 容量需求**（成本試算 §6.1） |
| 100 MB 總量對真實資料分析偏小 | 精深者不滿 | 記錄為已知限制，M4 封測明確詢問（BETA-005）；提高上限是配置變更，不是架構變更 |
| 並行 Run 上限 2 對「改善 → 重新試跑 → 比較」循環造成等待 | 漏斗中段體驗 | 該循環本質是序列的（改完才能重跑），2 已足夠；若封測觀察到排隊感，提高上限同樣是配置變更 |
| Token 預算耗盡的失敗與模型錯誤難以區分 | `02:EVAL-002`「區分 Runtime 問題與 Skill 問題」 | LiteLLM 回傳的預算耗盡錯誤需映射為獨立的診斷碼（NFR-003），Trace 中標為 `budget_exhausted` 而非泛用失敗。**2026-08-25 回填**：`budget_exhausted` 一直都在 `sandbox-provider.yaml` 的 enum 裡，只是沒有人用——Worker 的 token 上限落地當天先報成 `execution`，**也就是本列逐字禁止的那個泛用失敗**，同日改為 `budget_exhausted` 並補上端對端斷言。`runs.failure_class` 維持 `workload_error` 不變，那一欄是重試決定（不得重試，重試只會再燒一次），`error_class` 才是診斷碼——兩欄各答一個問題 |
| Skill 套件上限太嚴，含大量資產檔的 Skill 被擋 | 供給受限 | 上限是配置值不是架構；匯入失敗訊息需明確指出是哪一項超限，讓作者可自行修正 |

---

## 6. PDM-006：Run、Dataset、Trace 與 Artifact 保存期限

### 背景

NFR-002 明確寫著「保存期限仍為待決策事項，確定後須轉換為可測試的時間要求」。ADR-014 把 Trace 設計為 Postgres 分割表——這意味著保存期限直接決定分割策略（按月分割 + `DROP PARTITION` 是最便宜的清理方式）。本節同時回答威脅模型 **Q9**（TM-DAT-03 無法轉成可測試要求）與部分 **Q10**（Langfuse 保存期限；遮罩範圍與使用者條款揭露仍待 SEC-006 與產品負責人處理）。

### 評估選項

| 選項 | 取向 | 優點 | 缺點 |
| --- | --- | --- | --- |
| A：一律短（全部 30 天） | 成本與隱私風險最低 | `02:EVAL-003`「比較結果不得因後續版本修改而改變歷史 Run 資料」在 30 天後失效；北極星指標無法做季度趨勢 |
| B：分級（metadata 長、內容短） | 指標與追溯保留、內容型資料快速淘汰 | 需要對每類資料分別實作清理 |
| C：一律長（全部 1 年以上） | 分析最完整 | 儲存成本、隱私暴露面、GDPR 類要求下的刪除負擔都最高 |

### 建議

**採選項 B。分級表如下（每一列都應轉為 `plans/mvp/02` NFR-002 下的可測試要求）：**

| 資料 | 保存期限 | 清理方式 | 理由 |
| --- | --- | --- | --- |
| Skill、Skill Source、Skill Version | **永久** | 僅人工下架（標記不可見，內容保留） | 鐵律 4 不可變；Fork 溯源需要 |
| Test Case 快照（Prompt、驗收條件；**不含上傳檔案**） | **永久** | — | `02:EVAL-003` 版本比較的必要前提 |
| Dataset 上傳檔案 | **90 天** | 到期自動刪除；使用者可隨時提前刪除（`02:WS-002`／NFR-002） | 使用者私有內容，暴露面最大，期限最短 |
| Run metadata、狀態機轉換紀錄、Evaluation 結果 | **永久** | — | 北極星指標與漏斗（11.2 節）需要跨季度比較；資料量小 |
| Trace 明細事件 | **90 天** | 按月分割，`DROP PARTITION` | 量最大；90 天覆蓋「改善建議 → 重新試跑 → 比較」的完整週期 |
| Run Artifact | **30 天** | 物件儲存 lifecycle rule | 使用者若需保留應主動下載；上傳前明示（`02:TEST-002`） |
| Download Artifact（打包產物） | **90 天** | 到期刪除，可依同一 Skill Version 重新打包 | 打包是冪等的，不必永久保存二進位 |
| Audit Event（CORE-008） | **400 天**，僅保留 actor ID、動作、資源 ID、時間戳，**不含內容** | 到期刪除 | 安全事件回溯需要跨年；不含內容故隱私風險低 |
| Langfuse LLM 明細（ADR-017） | **30 天** | Langfuse 保留設定 | 工程調優用途，非事實來源；含使用者 Prompt，故最短。**遮罩範圍仍待 SEC-006**（威脅模型 Q10 未被本節完全回答） |

#### 6.1 帳號刪除：兩類資料分開處理（v2 修正）

v1 寫「30 天寬限期後硬刪除所有上述使用者資料」，與同表的三個「永久」列直接衝突，也違反鐵律 4（Skill Version 不可變、Fork 溯源需要）。修正為分類處理：

| 類別 | 處理方式 | 理由 |
| --- | --- | --- |
| **私有內容硬刪除** | Dataset 上傳檔案、Run Artifact、Download Artifact、未被任何他人 Fork 且未被歷史 Run 引用的私有 Skill Version 與 Test Case 快照 → **實體刪除**（含物件儲存與備份的下一個週期） | 這是 NFR-002「使用者可主動刪除」承諾的實質內容 |
| **已被引用的不可變版本：去識別化保留** | 已被他人 Fork 的 Skill Version、被歷史 Run 引用的 Skill Version 與 Test Case 快照 → **內容保留，作者身分去識別化**（`user_id` 置換為墓碑識別、顯示為「已刪除的使用者」），標記為不可再被新 Run 引用 | 鐵律 4：Skill Version 不可變；`02:DISC-003`「任何 Skill Hub 修改後的版本都能追溯到原始來源及 Fork 關係」。硬刪除會讓下游 Fork 的溯源鏈斷裂，傷害的是第三方而非本人 |
| **Run metadata 與 Evaluation 結果** | 去識別化保留（保留狀態機轉換、成本、驗收結論；移除與帳號的關聯） | 北極星指標與漏斗需要跨季度比較；去識別化後隱私風險低 |
| **Audit Event** | 僅保留去識別化 actor ID | 安全事件回溯 |

- 寬限期 **30 天**（避免誤刪不可逆），排程工作具可追蹤狀態（NFR-002）。
- **刪除範圍必須在 UI 事前說明**（`02:WS-002`「系統應說明刪除範圍」）：明確告知「已被他人 Fork 的版本會保留內容但移除你的身分關聯」，不能讓使用者以為全部消失。
- 此分類需轉為 NFR-002 下的可測試要求，並列入 CORE-007（刪除流程）與 SEC-008（資料隔離）的測試範圍。

**UI 要求（對應 `02:TEST-002`／NFR-002 允收準則）**：上傳前與下載頁都必須顯示對應期限，且措辭為「Artifact 將於 30 天後自動刪除」而非「暫存」——後者會被誤讀為「隨時可能消失」或「永久保存」。

### 理由

- metadata 永久 / 內容短期，是唯一能同時滿足「所有 Run 都能追溯」（`01` 4.3 節平台成果）與「隱私暴露面最小化」（NFR-002）的組合。
- 90 天 Trace 對齊 ADR-014 的月分割：三個活躍分割 + 清理，運維最單純。成本試算 §5.3 也把 `DROP PARTITION` 列為容器化 Postgres 下磁碟水位的主要控制手段。
- Artifact 30 天短於 Trace 90 天是刻意的——Artifact 是 Run 產物中體積最大、隱私敏感度最高、且使用者最容易自行保存的一類。
- 本表的分級已被 [cost-estimation.md](cost-estimation.md) v2 §2.1 採用（「物件保存期：90 天（Trace）／30 天（Artifact）」），v1 的單一 90 天假設已修正，此處無待回寫項。

### 風險

| 風險 | 影響 | 緩解 |
| --- | --- | --- |
| 使用者以為 Artifact 永久保存，30 天後失望 | 信任損害 | 下載頁與 Run 詳情頁都顯示到期日（絕對日期，非相對天數）；到期前 3 天寄信（M4 後可選） |
| Trace 90 天但 Run metadata 永久 → 舊 Run 打得開但看不到細節 | 使用者困惑 | UI 對超過 90 天的 Run 顯示「Trace 明細已依保存政策清除，評估結果與驗收結論仍完整」 |
| 保存期限日後需縮短（法遵） | 已存資料需回溯清理 | 清理工作以「掃描 + 刪除超期資料」實作，而非「建立時排程」，縮短期限只需改配置 |

---

## 7. PDM-008：首批目標 Agent 打包 Profile

### 背景

ADR-012 的待決策第一條就是「MVP 首批支援的 Agent Packaging Profile」。`plans/mvp/01` 第 7.2 節允許「先支援標準套件與少量安裝 Profile」。`02:PACK-002` 明確要求「尚未驗證的 Agent 必須顯示未驗證，不得保證可正常運行」——所以 Profile 數量少不是缺陷，宣稱過頭才是。

### 評估選項

| 選項 | 內容 | 優點 | 缺點 |
| --- | --- | --- | --- |
| A：只有標準套件 | 一個符合 Agent Skills 規格的 zip + manifest | 實作最省；不會過度承諾 | `02:PACK-002`「提供至少一個安裝後驗證 Prompt 或檢查步驟」難以具體化；使用者仍要自己查「放哪裡」 |
| B：標準 + 2 個 Profile | 標準 + Claude Code + Claude Agent SDK | 兩個 Profile 都對應 PDM-003 已驗證的 Runtime，「行為相容」有真實證據 | 只覆蓋 Claude 生態 |
| C：標準 + 4 個以上 | 再加 Codex／Cursor／Gemini CLI | 覆蓋面最廣 | 這些 Agent 平台一次都沒在 Skill Hub 上跑過，全部只能標「未驗證」，Profile 淪為猜測性的路徑對照表，且每個都要維護（ADR-012 已列此成本） |

### 建議

**採選項 B：1 個標準套件 + 2 個已驗證安裝 Profile。**

計數說明（v2 統一）：`standard` 是**標準套件**，不是安裝 Profile——它不含任何平台特定路徑或設定。因此對外一律表述為「1 個標準套件 + 2 個已驗證安裝 Profile」，UI 顯示的「已驗證安裝 Profile 數量」為 **2**。

| # | 打包目標 ID | 類型 | 安裝位置 | 相容性層級（ADR-012） | 安裝後驗證 |
| --- | --- | --- | --- | --- | --- |
| 1 | `standard` | **標準套件** | — （原樣目錄 zip + `manifest.json`：來源 Skill Version、Profile 版本、打包器版本、內容雜湊、驗證結果、含／不含的 Test Case 清單） | 格式相容 | 提供 `SKILL.md` 規格驗證報告 |
| 2 | `claude-code` | 安裝 Profile | 使用者層 `~/.claude/skills/<name>/` 或專案層 `.claude/skills/<name>/` | 格式 + 能力 + **行為（若該 Skill 在平台試跑通過）** | 附一句驗證 Prompt（來自 CONTENT-007 的範例 Prompt） |
| 3 | `claude-agent-sdk` | 安裝 Profile | **Agent 工作目錄 `.claude/skills/<name>/`**（v4 依實測修正），附 `query()` options 設定片段——**片段必須示範 `cwd` 與 `setting_sources`** | 同上 | 附最小可執行範例片段 |

> **兩個 Profile 的安裝路徑實際相同（v4 釐清）。** 前置 Spike 第 2 項證實 Agent SDK 走的是與 Claude Code 相同的 `.claude/skills/` 慣例，因此專案層安裝路徑兩者一致，差別只在**使用者層 vs 工作目錄層**、以及隨附的驗證方式。
> 這降低了維護成本（路徑變更時兩個 Profile 同步修一處），但也意味著**文案必須把差異講清楚，否則使用者會以為是重複選項**。建議的區分說法：`claude-code` 是「裝到你的 Claude Code，之後每次都能用」；`claude-agent-sdk` 是「放進你這支程式的工作目錄，附上要怎麼呼叫 `query()`」。
> **`claude-agent-sdk` 的安裝說明只給路徑是不夠的**——`cwd` 與 `setting_sources` 是安裝步驟的一部分，缺任一項使用者的 Skill 就不會被載入（四個啟用條件見 §3）。`02:PACK-002` 要求「顯示安裝位置、依賴與環境變數需求」，這兩個選項應視為該要求下的「必要設定」欄位。

**M4 之後依需求訊號再評估**：Codex Skills、Cursor、Gemini CLI（社群目錄顯示這三個生態的 Skill 數量最多）。加入時每個都必須先通過至少一次實際安裝驗證，否則只能列為「未驗證」提示而非 Profile。

**共通規則（皆為 `02:PACK-001` 已有的允收準則，此處只是明確化）**：打包前重跑規格驗證；保留 License／作者／來源 URL／commit／Fork 關係；排除 Secrets、測試憑證、內部路徑；使用者可選是否包含可散布的 Test Case 與範例資料。

> **source-available Skill 的打包政策（v2 統一為保守版）：一律不產出任何 Download Artifact，包含 `standard` 標準套件在內。**
> v1 此處寫「在 `standard` 以外的 Profile 一律阻擋」（隱含 `standard` 可下載），與 PDM-002 風險表的「不提供下載」相反。兩處現統一為 **PDM-002 的保守版**：source-available 內容可索引、可在平台內試跑（在平台上執行不等於再散布），但不產生任何可下載產物。
> 這是**法遵保守預設**，不是對授權條款的法律判定。放寬（例如認定 `standard` 原樣重新散布屬允許範圍）**需負責人與法務明確確認後才可改**，屆時兩處必須同步。

### 理由

- 兩個 Profile 都指向 PDM-003 已選定的 Runtime，因此「行為相容」欄位有真實 Run 證據可填——這正是 ADR-012 三層相容性設計的目的。若加入從未驗證的 Agent，三層都只能填「未驗證」，Profile 就退化成一張猜測的路徑表，反而製造誤導（違反產品原則 5）。
- `standard` 必須存在且獨立於其他 Profile：它是「Skill Hub 不綁定單一 Agent」這個承諾的可驗證證據。

### 風險

| 風險 | 影響 | 緩解 |
| --- | --- | --- |
| 目標 Agent 的安裝路徑或 frontmatter 支援改變 | 既有 Profile 過期，使用者依說明安裝失敗 | ADR-012 已要求 Profile 版本化；Download Artifact 記錄 Profile 版本，可回溯是哪一版說明出錯 |
| 只支援 Claude 生態，被視為「這不是通用平台」 | 產品定位受質疑 | UI 明確區分「標準套件（任何支援 Agent Skills 的 Agent）」與「已驗證安裝 Profile（目前 2 個）」；M4 封測蒐集目標 Agent 需求，歸 BETA-005（範圍與優先級複審） |
| source-available 保守政策讓 `documents` 類最好的四個樣本完全不可下載 | 核心旅程最後一步（打包下載）在該類別走不完 | PDM-002 已要求「額外找 2–3 個 OSI 授權的替代品作為可下載精選」——**這是 `documents` 類別的必要條件，不是加分項**；MVP DoD 要求新使用者能完成「搜尋→…→打包下載」，至少一個 `documents` 精選 Skill 必須可下載 |
| 使用者對 `standard` 套件不知如何安裝 | 漏斗最後一步流失 | `standard` 下載頁附「什麼是 Agent Skills 規格 / 一般安裝原則」說明，並連結 https://github.com/anthropics/skills 的 `spec/` |

---

## 8. PDM-010：免費 Run 額度與自備 API Key

### 背景

`plans/mvp/01` 第 12 節把「Sandbox 成本不可持續」列為風險，對策包含「限制資源、顯示預估、保存用量並保留未來計費能力」。ADR-017 已明確寫下「未來使用者自備 API Key（PDM-010）以閘道的 BYO Key 機制實作，平台程式碼不變」——所以 BYO 不是架構問題，是產品時機問題。

### 評估選項

| 選項 | 內容 | 優點 | 缺點 |
| --- | --- | --- | --- |
| A：無免費額度，全部 BYO Key | 使用者自帶 Anthropic key 才能試跑 | 平台模型成本為零 | 直接摧毀 MVP 承諾——「10 分鐘內完成搜尋到下載」變成「先去申請 API key」；封測漏斗（BETA-002）第一步就斷 |
| B：免費額度，MVP 不做 BYO | 平台出錢，額度封頂 | 體驗最順；成本上限可由 Virtual Key 預算硬性保證 | 平台承擔全部模型成本 |
| C：免費額度 + 同時支援 BYO | 兩者都有 | 覆蓋最廣 | BYO 引入額外的 Secrets 生命週期（SEC-005）、成本歸屬（ADR-011 Usage Record 語意變更）與 UI 複雜度，對 MVP 驗證目標無增益 |

### 建議

**採選項 B：MVP 提供免費額度、不開放 BYO Key，但保持架構就緒。**

#### 8.1 免費額度

| 項目 | 數值 |
| --- | --- |
| **首個 30 天窗口上限** | **20 次 Run**（新帳號一次性額度**取代**該窗口的月上限，即 `min(20, 30) = 20`） |
| 第 31 天起：每月滾動上限 | 30 次 Run |
| 每日上限 | 5 次 Run（全期適用，含首月） |
| 同一 Workspace 並行 Run 上限 | 2（見 PDM-005 §5.2） |
| 計數方式 | 以「已進入 `preparing` 狀態的 Run」計 |
| 自動退還 | 因平台原因終止者退還額度：Provider 不可用、清理失敗、平台側 `failed`（含威脅模型閘門 B 的基線檢查未通過——該文件明訂此類「失敗分類為平台側錯誤，不計入使用者配額」）。**不退還**：Skill 本身執行失敗、驗收未通過、使用者主動取消（這些是有效的產品使用） |
| 額度耗盡行為 | 阻擋建立新 Run，顯示重置時間與「回報你的使用情境」入口（作為 M4 需求訊號蒐集管道） |
| 未登入 | 可搜尋、可看詳情、**不可 Run**（`02:DISC-001` 允收準則允許未登入搜尋；Run 需登入以便額度歸屬與濫用防護） |

> **首月語意（v2 補，標「待確認」）**：v1 同時列出「新帳號一次性 20 次」與「每月滾動 30 次」但未定義兩者關係，成本模型因此無法收斂。本提案採 **`min(20, 30) = 20`**：一次性額度是首個 30 天窗口的上限，不與月上限相加。
> 理由：新帳號尚未證明用途，是多帳號刷額度最主要的攻擊面（見風險表），首月給較低額度可壓縮濫用成本；20 次仍足以完成 5–6 輪完整的「試跑 → 改善 → 重新試跑 → 比較」循環（每輪約 3–4 次 Run）。
> **替代語意（若負責人偏好較寬鬆）**：20 + 30 = 50，代價是新帳號的單月成本上限由 **$9 升至 $15**（v5：以 `gpt-5.4-mini` 打滿的 $0.30/Run 計；v4 以 $1.80/Run 計為 $54 升至 $90），濫用防護需相應加強。**此項請負責人明確擇一，不要留給實作推斷。**
> **v5 附註**：換供應商後單 Run 成本下降約 6 倍，**寬鬆語意的成本代價已顯著變小**，此項的決策重心從成本移向濫用防護——負責人若原本因成本而傾向保守，值得重新衡量。

- **計數起點在 `preparing` 的一個推論**：`provisioning` 階段失敗（含 Provider 不可用、基線檢查未過）本來就還沒計數，因此「自動退還」實際只對 `preparing` 之後的平台側失敗生效。這是刻意的——把計數點放在資源真正被佔用之後。

#### 8.2 成本模型（v2 與成本試算對齊）

**用量前提改為與本節額度一致**：封測 30 人 × 每月 30 Run = **900 Run/月**。（v1 未定義用量前提，導致 [cost-estimation.md](cost-estimation.md) v1 以 75 Run/人/月 試算，與本節額度互斥；cost v2 §2.2 已加註此落差並提供 (a′)/(b′) 參照情境。）

**平台成本（引用 cost-estimation.md v2，E1 Hetzner 雲原生容器化推薦組合）：**

| 項目 | 值 | 出處 |
| --- | ---: | --- |
| 尖峰併發 slot | `30 Run/日 × 6min ÷ 480min × 2` = 0.75 → **1** | cost v2 §2.1 假設 |
| Sandbox 節點（含 N+1） | 1 + 1 = **2 台**（4 vCPU/16 GiB，CCX23） | cost v2 §2.2 |
| Sandbox 池月費 | **$200** | cost v2 §4.1 |
| **平台月費合計** | **$283** | cost v2 §4.4 情境 (a′)——900 Run 與 600 Run 的節點需求相同，成本一致 |
| **單 Run Sandbox 成本** | **≈ $0.22**（$200 ÷ 900） | 本次計算 |
| 單 Run 平台總成本 | ≈ $0.31（$283 ÷ 900） | 本次計算 |

> **v1 的 `$0.01–0.03` 已刪除，低估了約一個數量級。** 原因是 **N+1 冗餘的下限效應**：封測規模只需 1 個 slot，但 ADR-015 要求節點可滾動汰換，故最少 2 台節點——固定成本被少量 Run 攤提，單 Run 成本反而高。cost v2 §4.4 對此的結論是：**封測階段的 Sandbox 成本對 Run 量不敏感，想省錢只能省節點規格，不能靠減少 Run。** $0.01–0.03 要到 15,000 Run/月的規模才成立（cost v2 §4.2：$802 ÷ 15,000 = $0.053）。

**模型成本（v5 依 PDM-003 定案的 `gpt-5.4-mini` 與 §5.2a 的快取後實價重算）：**

| 項目 | 上限值（300K in / 60K out 打滿） | 預估中位數（假設用量為上限的 1/6） |
| --- | ---: | ---: |
| 單 Run 模型成本 | **$0.30**（熱快取）／$0.50（冷快取首次 Run） | **$0.05** |
| 單使用者月上限（30 Run 打滿） | **$9** | — |
| 單使用者月預估（30 Run 中位） | — | **$1.50** |
| **封測 30 人月模型費**（900 Run） | 最壞 **$270** | 預估 **$45** |

**合計（模型 + 平台）：**

| 情境 | 模型費 | 平台費 | 總計 | 模型費佔比 |
| --- | ---: | ---: | ---: | ---: |
| 封測 30 人，中位用量 | $45 | $283 | **$328/月** | **14%** |
| 封測 30 人，全部打滿 Token 上限 | $270 | $283 | **$553/月** | **49%** |
| （參照）若試跑改用旗艦 `gpt-5.6-sol`，中位用量 | $297 | $283 | $580/月 | 51% |

> ⚠️ **v5 的結論翻轉：模型費在封測規模已不是總帳單的多數項。** v4 的「中位 $553／最壞 $1,903、模型費佔 49%～85%」在 OpenAI mini 檔位下降為「中位 **$328**／最壞 **$553**、模型費佔 **14%～49%**」——**v4 的中位總額，現在正好是 v5 的最壞總額。** 三個結論：
>
> 1. **最壞與中位仍相差 6 倍，差距仍然全部來自 Token 用量、不是 Run 次數。** 這一條沒有因為換供應商而改變，只是絕對值變小。
> 2. **「每月滾動上限 30 次」仍是成本上限的執行機制，但槓桿變小了**——它現在控制的是一個佔比 14–49% 的項目，而非 61–97%。**平台選型的相對重要性回升**，見 [cost-estimation.md](cost-estimation.md) §6.2.3。
> 3. **成本控制的著力點已從 input 移到 output**：快取後 input 幾乎免費，60K output 上限佔單 Run 上限成本的 90%（§5.2a-6）。**要收窄成本應該動 output 上限與模型分層，不是 input 上限。**
>
> **已完成的交叉回寫**：本節的 $0.05/Run 中位數與 $0.30 上限已回寫至 [cost-estimation.md](cost-estimation.md) **§6.2.3（v3）**。
> **仍待交叉確認**：$0.05 中位數**仍是假設值（上限的 1/6），不是量測值**——換供應商只換掉了單價，沒有換掉這個假設。M1 的 20–30 次真實 Run 量測要求不變（見下方風險表最後一列）。
> **仍待交叉確認**：PDM-009（封測人數）——本節的 30 人是假設值，cost v2 用 20 人；兩者需一併定案。

#### 8.3 自備 API Key

**MVP 不開放，架構保持就緒。** 具體立場：

- 不在 MVP 實作 UI、不在 MVP 實作 Secrets 儲存流程。
- ADR-017 已定的實作路徑保持有效：開放時以 LiteLLM 的 BYO Key 機制實作，平台程式碼不變。
- **啟動條件**：M4 封測中出現任一訊號即評估開放——(a) 超過 30% 使用者觸及月額度上限；(b) 質性回饋明確要求；(c) 平台模型成本超出可承受區間。
- 開放時的最低要求（提前記錄，避免屆時倉促）：只接受 Anthropic API Key；金鑰只存在 LiteLLM 閘道、平台資料庫不存明文或密文（SEC-005）；Trace 與 Usage Record 明確標示該 Run 成本歸屬為使用者自付；Key 可隨時撤銷且撤銷後既有 Run 不受影響；金鑰不得出現在套件、Log、Trace、分析事件（鐵律 11）。

### 理由

- 免費額度是 MVP 承諾的前提條件，不是行銷手段——沒有它，核心旅程的第一次體驗就有一道外部依賴。
- Virtual Key 預算（PDM-005）讓「免費」有硬性天花板，這是選項 B 可行的唯一原因；若沒有 ADR-017 的閘道，免費額度會是不可控的成本黑洞。cost v2 §6.2 同樣把它列為「唯一能硬性止血的機制」。
- 不做 BYO 是為了保護 MVP 的驗證焦點：MVP 要驗證的是「試跑能否提高下載信心」，不是「計費模式」。BYO 會引入 Secrets 生命週期、成本歸屬語意、UI 分岔三個與驗證目標無關的複雜度。

### 風險

| 風險 | 影響 | 緩解 |
| --- | --- | --- |
| 多帳號刷免費額度 | 成本失控 | NFR-001 已要求速率限制與濫用防護；Run 需登入；同 IP／同裝置的新帳號額度可降級（M4 若出現實際濫用再啟用）。首月採 `min(20,30)` 而非相加，也是壓縮此攻擊面的一部分 |
| 額度太緊，使用者無法完成「改善 → 重新試跑 → 比較」循環 | 漏斗中段流失，`02:EVAL-003` 無法被驗證 | 一次完整循環約需 3–4 次 Run；每日 5 次剛好覆蓋一輪，首月 20 次可完成 5–6 輪。封測需監測「因額度耗盡而中斷的旅程」比例（BETA-004） |
| 不支援 BYO 導致精深者流失 | 目標 persona 之一未被服務 | 額度耗盡頁面直接提供「我想自備 API Key」回報入口，把流失轉成需求訊號 |
| **$0.05/Run 的 Token 中位數是假設值，不是量測值**（v5 更新數字，性質不變） | 最壞／中位仍差 6 倍，總成本估計在 $328 與 $553 之間擺盪；cost §8 已把此項列為「首次真實 Run 後應校準」的第二優先項。**換供應商只換掉單價，沒有換掉「用量為上限 1/6」這個假設** | **M1 期間用內部帳號跑 20–30 次真實 Run，量測實際 token 中位數**，把區間收窄後再定案免費額度。量測時**必須同時記錄每輪的工具呼叫次數**（§5.2a-2：input 用量幾乎完全由它決定），否則量到的中位數無法外推。在量測完成前，預算規劃應以**最壞值**而非中位值編列 |
| 平台級模型預算煞車尚未設計 | 單一失控迴圈可在 TTL 內燒掉整個 Run 預算，多個並發則放大 | 威脅模型 TM-MDL-02 已列此缺口，Q13 建議歸 ADR-011 Policy 模組。每 Run Virtual Key 預算限制的是**單 Run 爆炸半徑**，不是平台總量；本提案不解決此項，僅記錄 |

---

## 9. 定案檢查清單

### 9.1 逐項確認

- [x] **PDM-001**（v5：提案完整，可定案）：三個類別 ID 與顯示名稱確定；同步更新 `02:DISC-002` 篩選器的類別選項。**`data` 供給缺口已解除**（25 個候選 / 7 個 repo，換類別條件未成立）；**`excel-*` 歸 `data` 或 `documents` 的歸類原則已明確寫下**（影響精選池厚度）<br>**✅ 2026-08-27 追認（`05` R-1b）**：照提案值，行為零變動。類別 ID 與顯示名稱未動，`02:DISC-002` 的三個篩選值即本列。
- [x] **PDM-002**（v5：提案完整，可定案）：白名單 repo 清單確定（**含 v5 新增的三個 `data` 來源**）；九項精選標準逐條確認可執行；回溯准入流程確認可執行（**並知悉其成本被低估：過半 awesome 條目需額外一次人工查找原 repo**）；`anthropics/skills` 實數已清點＝17；**`doc-coauthoring` 無 License、暫不可精選一事已確認**；數量目標（12–18 精選 / 24–36 索引）與 PDM-011 完整 golden query set 的需求一致<br>**✅ 2026-08-27 追認（`05` R-1b）**：照提案值，行為零變動。白名單、四步回溯准入與九項精選檢查表全數照提案。
- [x] **PDM-003**（v5：提案完整，可定案。**技術前置與補測全部完結**）：定案時需確認：**模型供應商 OpenAI 已記錄於本文件標頭，模型分層六項型號確定**；**Skill 載入路徑 `.claude/skills/<skill-name>/` 與四個啟用條件已寫入 SBX-002／SBX-008**；**試跑預設 Prompt 必須點名被測 Skill，且「自主觸發」不作為成功判準**（基準率實測 0/9）；**Virtual Key 環境變數方案的殘餘風險已被明確接受，且定案文字含「`max_budget` ＋ `tpm_limit` ＋ Go Worker token 累計」三層**；回填 ADR-013 待決策「Embedding 與查詢改寫模型」與 ADR-017 待決策「Virtual Key 注入機制」（＝威脅模型 Q11）<br>**✅ 2026-08-27 追認（`05` R-1b）**：照提案值，行為零變動。模型分層六項型號、`.claude/skills/` 載入路徑與三層 token 煞車皆已在執行，追認補的是簽名。
- [x] ~~**PDM-003 補測項**~~ —— **v5：四項全部結案，此列已無待辦，僅供定案時核對。** 兩項已實測（自主觸發 0/9、prompt caching 缺欄但計費準確），兩項因 OpenAI 定案不再適用（`thinking` 透傳、跨供應商 fallback）。**兩項殘留動作已改列他處**：`MAX_THINKING_TOKENS=0` 進 PDM-004 常設斷言；跨供應商 fallback 未驗證一事進 ADR-017 已知邊界<br>**✅ 2026-08-27 追認（`05` R-1b）**：本列本來就無待辦，勾選只是讓「已核對」這件事在文件上留下痕跡。
- [ ] **SBX-002 實測項（v5 收斂）**：~~`skills` 白名單的行為性過濾效果~~ **已由 Spike §11.4 回答＝有效，改為「採用並寫入三點限制」**；**仍待實測**：CLI 內建 Skill 與內建工具的裁減幅度與 Skill 相容性的權衡（唯一真正的 token 槓桿）<br>**⛔ 2026-08-27 刻意不勾。** 本列自己寫著「**仍待實測**：CLI 內建 Skill 與內建工具的裁減幅度」，而那件事到今天一次都沒有做過——沒有量過裁減之後的 token 差，也沒有量過裁減之後的 Skill 相容性。**AGENTS.md 的規則是「部分完成保持未勾」**，而本列的兩半只成立一半（白名單過濾已由 §11.4 實測回答＝有效）。**勾它的代價很具體**：那 19.4K／次 API 呼叫的 harness 固定前綴是全表唯一真正的成本槓桿（§11.5 發現 (c)），勾掉之後不會再有人回頭找它。
- [x] **PDM-003 × PDM-011**：ADR-013 定案時一併補記 [pdm-011-spike-report.md](pdm-011-spike-report.md) §6.2 的三項調整——(1) 查詢改寫升格為召回必要條件、降級目標為向量檢索；(2) 索引時增強的產出應成為主要檢索欄位、原始 `SKILL.md` 內文降權；(3) 符合原因需涵蓋「無詞彙交集」情境（索引時一併生成適用任務範例句）<br>**✅ 2026-08-27 追認（`05` R-1b）**：照提案值，行為零變動。三項調整早已回寫 ADR-013 與 PDM-003，本列補的是核對紀錄。
- [x] **PDM-004**：套件白名單與 PDM-001 類別需求對齊——**`lxml`（必要）與 `matplotlib`（建議）的增補已擇一裁示，`scipy`／`sklearn` 維持排除**（v5）；Image tag 與版本升級節奏確定；**Node.js 22 與 Python 3.12 已在 runsc（gVisor）下跑通完整 Run 生命週期**（ADR-015 定案條件、威脅模型 §5.6 Runtime 相容性測試、基線 C-09）；**內建工具與 15 個 CLI 內建 Skill 的裁減方案已評估**（唯一真正的 token 槓桿 ＋ 試跑干擾源）；**`skills` 白名單採為預設且三點限制已寫入**（v5）；**`ANTHROPIC_API_KEY` 未設 與 `MAX_THINKING_TOKENS=0` 兩項常設斷言已納入映像建置與 provisioning**（v5）<br>**✅ 2026-08-27 追認（`05` R-1b）**：照提案值，行為零變動。**但兩件「還缺」不因追認而消失**：①`runsc` 上跑通完整 Run 生命週期仍是部署期實測（`SEC-009` T4）；②**內建工具與 15 個 CLI 內建 Skill 的裁減幅度從未量過**——那正是下一列 `SBX-002` 勾不下去的同一件事。**追認的是語言與版本，不是那次沒做的實測。**
- [x] **PDM-008**：`claude-agent-sdk` Profile 的安裝位置已改為 `.claude/skills/<name>/`，且安裝說明含 `cwd` 與 `setting_sources` 示範；兩個 Profile 路徑相同一事的文案區分已確認（v4）<br>**✅ 2026-08-27 追認（`05` R-1b）**：照提案值，行為零變動。安裝位置與兩個 Profile 的文案區分已落地。
- [x] **PDM-005**：所有數值轉為可測試允收準則；**§5.1b 的 Skill 套件解壓上限已納入**（回答威脅模型 Q6，該項列於「阻擋 SEC-002 定案的問題」）；並行 Run 上限已寫入 ADR-011 Policy 的首批配置值；**v5：300K input 已解除「未經驗證」可寫入 `02`，但必須連同 §5.2a 的輪數換算表一起寫（單一輪數無意義）；token 上限的強制點為 Go Worker 而非 Virtual Key 預算；`max_budget` 依冷快取實價編列（mini $0.50／旗艦 $3.30）**<br>**✅ 2026-08-27 追認（`05` R-1b）**：照提案值，行為零變動。§5.1b 的兩個較寬值已於 2026-08-27 另案追認為 10 MB／100 MB（`05` ~~R-13~~），**不是本列給的**。
- [x] **PDM-006**：保存期限表轉為 NFR-002 的可測試時間要求；**§6.1 的帳號刪除分類（私有內容硬刪除 vs 已被引用版本去識別化保留）已確認不違反鐵律 4**；確認與 SEC-006 一致<br>**✅ 追認發生在 2026-08-23，不是本批（`05` R-1a）**，本列 2026-08-27 才跟上打勾。**追認值有兩處偏離提案，兩處都要各自被讀**：下載產物 `DOWNLOAD_ARTIFACT_RETENTION=720h`（30 天，提案給 90 天）、分析事件 `ANALYTICS_RETENTION=8760h`（365 天，ADR-029 決策 5 提案 180 天，**方向是增加暴露**）；Trace `TRACE_RETENTION=2160h` 與提案一致。**三者程式端仍無預設值、仍 fail-closed。**<br>**本列的勾不涵蓋一件仍然開著的事**：提案表給 Run Artifact **30 天**，而 `02:NFR-002a` 第 2 條要求它 ≥ 可重評窗（＝Trace 的 90 天）——**那條下界今天是被違反的**，要拉齊還是維持 30 天並補完過期分支，是 [`05` R-11](../../05-pending-rulings.md) 要裁的，**不在本次追認範圍內**。同理，`SEC-006` 不勾的理由也是那個下界，不是缺實作。
- [x] **PDM-008**：計數統一為「1 標準套件 + 2 已驗證 Profile」；**source-available 打包政策已由負責人與法務確認**（目前為保守預設：一律不產出 Download Artifact）；回填 ADR-012 待決策「MVP 首批支援的 Agent Packaging Profile」<br>**✅ 2026-08-27 追認（`05` R-1b）**：「1 標準套件 + 2 已驗證 Profile」自 2026-08-23 `claude-code` 翻 `verified` 之後是真的。<br>**本列的措辭比它引用的來源鬆，追認時按來源讀**：§2 風險表逐字寫的是「這是法遵保守預設，**放寬**（例如允許 `standard` 下載）需負責人與法務明確確認後才可改」——**所以法務確認是「放寬」的前置，不是「維持保守預設」的前置。** 今天追認的是保守預設本身（一律不產出 Download Artifact），那只需要負責人。**anthropic-sa 的法務終判仍未回**（`04` 乙-10 仍開著），而它一旦回來且允許散布，動的是放寬那一側，不是本列。
- [x] **PDM-010**：**首月額度語意已擇一**（`min(20,30)=20` 或 20+30=50）；成本模型與 PDM-009 封測人數、[cost-estimation.md](cost-estimation.md) 交叉驗算<br>**✅ 2026-08-27 追認（`05` R-1b／R-1a 把四個值交給這一批）：首月語意取 `min(20,30)=20`**，也就是 `entitlements/quota.go` 現在就在強制的那一個（首窗 20／每窗 30／每日 5／窗長 30 天）。**追認補的是簽名不是行為——程式碼一個值都沒有改。**<br>**追認之後解除的是一條禁令，不是一個開關**：ADR-028 決策 4 的「未追認的數字不得出現在畫面上」對這四個數不再成立；但 [ADR-055](../../../adr/ADR-055-the-run-allowance-is-turned-off-and-that-took-an-action.md) 的 `RUN_QUOTA=off` 沒有變，所以本次封測畫面上仍然沒有額度可顯示。<br>**本列的第二半「與 PDM-009 封測人數交叉驗算」在追認時查證為過期**：§8.2 整段的用量前提是 **30 人**，而 PDM-009 的產品面已於 2026-08-22 追認為 **12 人**。**這不改變四個額度值**（它們是每工作區的，與人數無關），但它讓 §8.2 的 $328／$553 變成**上界而不是現況**——真正的封測規模是它的 0.4 倍。§8.2 的數字刻意不改寫（那是當時的推導），要用時請按 12 人重算。
- [x] 三份 MVP 文件（01／02／03）同步更新；`03` 的對應 `- [ ]` 才勾選為 `- [x]`<br>**✅ 2026-08-27 完成，順序照本文件前言要求**：先追認 → 再同步 → 最後才勾 `03`。`03` §1 的 PDM-004／005／006／008／010 五列同批勾選（PDM-001／002／003／011 早在 2026-08-14 已勾）；`01` §13 的去向表同批訂正三列（PDM-004 不再是「仍未定」；PDM-009 與 PDM-010 兩列的 ID **原本是對調的**，追認時查出並改正）。`02` 逐句查過**不需要改**——它本來就以「值已定案」的敘述引用這些數字，那些句子在今天之前是提前，今天起是對的。
- [x] 判斷是否有任一決策構成新的架構決策，需要從 **ADR-018** 起新增 ADR 並更新 `adr/README.md` 索引<br>**✅ 2026-08-27 判斷完成，結論是「本批不產生新的 ADR」**，理由是機械的：**這一批追認沒有改變任何行為**——十一列裡沒有一個值被改，程式碼與測試一行未動，所以沒有任何決策可以被推翻或取代。<br>**真正產生架構決策的是那些「改變了行為」的動作，而它們各自都已經有 ADR**：兩個額度開關是 [ADR-055](../../../adr/ADR-055-the-run-allowance-is-turned-off-and-that-took-an-action.md)（Run）與 [ADR-056](../../../adr/ADR-056-the-generation-allowance-is-its-own-switch-and-it-is-off.md)（生成），額度強制點的形狀是 [ADR-028](../../../adr/ADR-028-beta-admission-and-quota-enforcement-points.md)。**本批對這兩份 ADR 只各加一節「後續」，不改寫決策**（AGENTS.md：ADR 是決策歷史，不原地改寫）。

### 9.2 回寫 `02` 的對照表（v2 新增）

`02-specifications-and-acceptance-criteria.md` **沒有 PDM-* 需求 ID**，PDM 只以一行敘述存在於 `03` 第 1 節。因此「回寫 02」必須落到既有需求 ID，否則無處可寫：

| 決策 | 回寫目標（`02` 需求 ID） | 回寫內容 |
| --- | --- | --- |
| PDM-001 | `02:DISC-002` | 類別篩選器的三個選項值 |
| PDM-002 | `02:DISC-003`、`02:SKILL-002` | 來源／License 狀態三態的判定依據；精選標準第 3 項的規格驗證範圍 |
| PDM-003 | `02:TEST-005`、`02:TRACE-001`、`02:EVAL-001` | 權限摘要中的 Runtime 版本與模型欄位（**模型欄位值為 OpenAI 型號**，v5）；**成本摘要須為區間非單值**（首次／後續 Run 差約 8 倍，§5.2a-6）；Trace 遮罩須涵蓋 Virtual Key 樣式；**`02:EVAL-001` 須明訂「Skill 是否被自主觸發」不作為驗收判準**（v5，基準率實測 0/9） |
| PDM-004 | `02:RUN-003` | Runtime 語言版本與預裝套件白名單 |
| PDM-005 | `02:TEST-002`、`02:RUN-003`、`02:SKILL-001` | Dataset 上限與檔案類型；CPU／記憶體／磁碟／PID／FD／逾時／並行數值；Skill 套件匯入上限與失敗原因分類。**v5：300K input 的例外已解除，可寫入——但必須帶 §5.2a 的輪數換算表（15／7.7／5 輪對應每輪 0／1／2 次工具呼叫）**，只寫「300K」或只寫單一輪數都不構成可測試的允收準則 |
| PDM-006 | `NFR-002` | 各類資料的保存期限與帳號刪除範圍，全部轉為可測試時間要求 |
| PDM-008 | `02:PACK-001`、`02:PACK-002` | 打包目標清單、source-available 阻擋規則、安裝位置與驗證步驟 |
| PDM-010 | `NFR-001` | 額度數值與濫用防護要求 |

> 另有一個相關但不屬本提案的缺口：威脅模型 **Q19** 指出 `02` 同樣沒有 SEC-* 需求 ID 與允收準則，SEC-001／SEC-002 因此無正式判準。兩份 M0 文件的回寫需求宜一併處理。

---

## 附錄：引用來源

**外部來源（v2 於 2026-08-13 逐一查核）：**

- [anthropics/skills](https://github.com/anthropics/skills) — Anthropic 官方 Agent Skills 公開 repo。**已查核**：`skills/`、`spec/`、`template/` 三個目錄存在；README 明文「Many skills in this repo are open source (Apache 2.0)... the document creation & editing skills... in `skills/docx`, `skills/pdf`, `skills/pptx`, and `skills/xlsx`... **These are source-available, not open source**」。**skill 目錄實數未經核對**（v1 記為 17），定案前需清點。
- [obra/superpowers](https://github.com/obra/superpowers) — MIT。**已查核，v1 描述有誤**：自述為「a complete software development methodology for your coding agents, built on top of a set of composable skills」，`skills/` 內容為 test-driven-development、systematic-debugging、verification-before-completion、brainstorming、writing-plans、subagent-driven-development、code review、git workflow、writing-skills 等**軟體開發流程**技能。v1 稱其為「方法論類，可佐證 `writing`」屬錯配，v2 已降級為發現／參考。
- [VoltAgent/awesome-agent-skills](https://github.com/VoltAgent/awesome-agent-skills) — MIT。**已查核**：策展索引，**自身不託管 Skill**，全部連回原 repo（Anthropic、Google Labs、Vercel、Stripe 等官方團隊 + 社群）。條目數約 **1,500**（repo 自述 1,400+，badge 顯示 1,497+；v1 記為「1500+」略高）。
- [ComposioHQ/awesome-claude-skills](https://github.com/ComposioHQ/awesome-claude-skills) — 社群策展清單（未逐項查核）
- [heilcheng/awesome-agent-skills](https://github.com/heilcheng/awesome-agent-skills) — 社群策展清單（未逐項查核）
- [Awesome Agent Skills 目錄](https://www.awesomeskills.dev/en) 、 [SkillsMP](https://skillsmp.com/) 、 [Awesome Claude Skills](https://awesomeclaude.ai/awesome-claude-skills) — 市集／目錄，MVP 不接入，供 M4 後評估外部即時搜尋時參考

**模型 ID 與定價（v5 依負責人定案改為 OpenAI，2026-08-14 查核；與 PDM-003 §3 模型表一致）：**

| 模型 ID | 輸入 / 輸出 / 快取輸入（每 MTok） | 本提案用途 |
| --- | --- | --- |
| `gpt-5.6-sol` | $5 / $30 / $0.50 | 試跑（進階，選用）；索引時增強 |
| `gpt-5.6-terra` | $2 / $12 / $0.20 | LLM Judge |
| `gpt-5.6-luna` | $0.20 / $1.20 / $0.02 | 查詢改寫；符合原因潤飾 |
| `gpt-5.4-mini` | $0.75 / $4.50 / $0.075 | **試跑（預設）** |
| `gpt-5.4-nano` | $0.20 / $1.25 | 未採用（Luna 同輸入價、輸出更低且為較新家族） |
| `gpt-5.5` | $5 / $30 | 未採用（被 GPT-5.6 家族取代） |
| `text-embedding-3-small` | 約 $0.02 | Embedding（1536 維） |

- GPT-5.6 家族 2026-07-09 GA；**快取命中的 input 一律以 1/10 牌價計**，前綴 ≥ 1024 token 自動快取、2026-05-29 起預設保留 24 小時。
- **v4 之前的 `claude-sonnet-5` / `claude-opus-5` / `claude-haiku-4-5` 分層已由本表取代**，不再適用。

**v5 已失效、不可再引用的數字（避免舊值在下游文件復活）：**

| 舊值 | 出處 | 現值 |
| --- | --- | --- |
| 單 Run 模型成本上限 $1.80（導入價 $1.20） | v2–v4 §5.2／§8.2 | **$0.30 熱快取／$0.50 冷快取**（mini 檔位，§5.2a-5） |
| 單 Run 模型成本中位 $0.30 | v2–v4 §8.2 | **$0.05**（同一個「上限 1/6」假設，只換單價） |
| harness 固定開銷「約 50K／輪」 | v3–v4 §3(c)、§4、§5.2 | **約 19.4K／次 API 呼叫**（單位不同，§5.2a-2） |
| 300K input「未經驗證」 | v3 §5.2、§9.2 | **已驗證，解除標記**（§5.2a-1） |
| 「prompt caching 後可用輪數會顯著上升」 | v3 §5.2 | **不成立，輪數增益為 0**（§5.2a-3） |

**同目錄交叉引用文件：**

- [data-category-sourcing.md](data-category-sourcing.md)（2026-08-14）— **`data` 類別供給查核**。結論：供給充足（25 個合格候選 / 7 個 repo），但 v2 提名的三個候選方向有兩個證偽；附三項須併同定案的條件（`lxml` 必加、`matplotlib` 建議加、候選方向表重寫），全部已於 v5 回填
- [cost-estimation.md](cost-estimation.md)（v2 ＋ **§6.2.3 v3**）— 部署平台成本試算，推薦 **E1：雲原生容器化跑在 Hetzner 節點、Postgres 容器化、物件儲存與 Langfuse 維持受管、Sandbox 獨立 VM 池**
- [threat-model-and-sandbox-baseline.md](threat-model-and-sandbox-baseline.md)（v2）— 32 條威脅、45 項基線檢查、四個閘門、開放問題 Q1～Q19
- [pdm-011-spike-report.md](pdm-011-spike-report.md)（含 §9 v2 真 Embedding 重測）— 意圖搜尋品質 Spike。v1 結論「方向可行，但三段式作法的權重需調整」；v2 以 `text-embedding-3-small` 重測，跨語言召回過關、RRF 零增益、查詢改寫下修為精準度增益
- [pdm-003-litellm-spike-report.md](pdm-003-litellm-spike-report.md) — 涵蓋 PDM-003 的兩個前置項：**§1–§8 LiteLLM 閘道相容性**（7/7 通過，協定層可行、退路不需啟動；三個相容性坑見 §6）與 **§10 Skill 載入路徑**（6/6 通過，但證偽提案原本假設的路徑，正確路徑為 `.claude/skills/`）

---

## 10. 修正紀錄

### v5 定案回填（2026-08-14）

**觸發**：(1) 負責人定案模型供應商採 **OpenAI API**（經 LiteLLM 閘道，ADR-017 架構與鐵律 8 不變）；(2) 三份查核完成——[pdm-003-litellm-spike-report.md](pdm-003-litellm-spike-report.md) **§11 補測**、[data-category-sourcing.md](data-category-sourcing.md) **`data` 供給查核**、[cost-estimation.md](cost-estimation.md) **§6.2 模型成本敏感度**。
**未修改任何 ADR、`02`、`03`，未變更任何工作項目勾選狀態。**

| # | 位置 | 修正內容 | 依據 |
| --- | --- | --- | --- |
| v5-1 | 標頭 | 新增**負責人定案紀錄**表；狀態改為「PDM-001／002／003 三項提案完整、可定案」 | 負責人 2026-08-14 |
| v5-2 | §3 模型表（整表替換） | Claude 分層**全部改為 OpenAI 系列**：試跑預設 `gpt-5.4-mini`、進階 `gpt-5.6-sol`、Judge `gpt-5.6-terra`、索引時增強 `gpt-5.6-sol`、查詢改寫與符合原因潤飾 `gpt-5.6-luna`、Embedding 維持 `text-embedding-3-small`（**且成為同一家的原生模型，多供應商面歸零**）。預設選 mini 是**實測結論不是成本偏好**：旗艦自主觸發率無優勢（同 0/9）、有「改用 `Glob`／`Read` 翻檔案系統」的失敗模式（三次中兩次，一次因此耗盡 turn 上限）、成本差 6.7 倍 | Spike §11.2、§11.3、§11.6(2) |
| v5-3 | §3 建議段新增 blockquote | **Runtime 維持 Claude Agent SDK**：「SDK → LiteLLM → OpenAI」正是 7/7 實測通過的路徑；Agent Skills 載入是 SDK 的**原生機制**、與後端模型無關（攔截點在 harness），因此換供應商不需重做載入設計 | Spike §4、§10、§11.4 |
| v5-4 | §3 模型表下新增 blockquote | **試跑預設 Prompt 必須明確指示呼叫被測 Skill**；**自主觸發不可作為成功判準**（基準率實測 0/9，對照組證明機制完好）；若要呈現須標為「探索性指標」並註明基準率為 0。連帶要求 CONTENT-007 範例 Prompt 與 PDM-008 驗證 Prompt 都須點名 | Spike §11.3、§11.6(2) |
| v5-5 | §3 補測狀態表（取代原「待補測」表） | **四項全部結案**：`thinking` 透傳與跨供應商 fallback **因 OpenAI 定案不再適用**（緣由已標明，且各自留下一項殘留動作）；prompt caching 與自主觸發**已實測**。連帶結案 SBX-002 的白名單行為性驗證 | Spike §11 |
| v5-6 | §3 發現 (a)、(c)、(d) | **(a)** `thinking` 400 由「待補測」改寫為「常設組態」——`MAX_THINKING_TOKENS=0` 升格為 Runtime Image 斷言項；**(c)** harness 開銷單位由「約 50K／輪」修正為 **「約 19.4K／次 API 呼叫」**，每輪 input ≈ 19.4K ×（1 ＋ 工具呼叫次數）；**(d)** 白名單過濾**已實測有效**，對專案與內建 Skill 皆然 | Spike §11.4、§11.5、§11.6(1) |
| v5-7 | §3 風險表 | Judge 自我偏袒**已結構性緩解**（試跑與 Judge 不同型號），殘留家族層級共同偏誤；內建 Skill 干擾的緩解由「待驗證」改為「已實測有效」；**新增一列：白名單不是 token 成本槓桿**（擋執行不擋曝光，誤觸還多一次約 19.4K 的無效往返） | Spike §11.4 |
| v5-8 | **§5.2 Token 列 ＋ 新增 §5.2a** | 300K **維持並解除「未經驗證」**；新增輪數換算表（純對話 ~15 輪／每輪 1 工具 7.7 輪／每輪 2 工具 5 輪）；**prompt caching 不增加可用輪數**（v3 預期不成立，增益為 0）；**token 上限改由 Go Worker 依 `input_tokens` 累計強制**，`max_budget` ＋ `tpm_limit` ＋ Worker 三層併用；`max_budget` 依**冷快取**實價編列（mini $0.50／旗艦 $3.30）；記錄 **$1.80 與 300K 脫鉤 7～8 倍**；記錄 LiteLLM `/v1/messages` **不透傳 cache 欄位但計費準確**（可觀測性缺口，列 O11Y-001） | Spike §11.5、§11.6(1) |
| v5-9 | §4 PDM-004 | 白名單加 **`lxml`（必要）** 與 **`matplotlib`（建議）**，附反推理由與「不加則 `data` 供給 25→13」的量化後果；`scipy`／`sklearn` 維持排除的理由補上。新增 **`skills` 白名單為 SBX-002 預設**一列 ＋ **三點限制** blockquote（`init.skills` 不反映過濾／擋執行不擋看見／多一次無效往返）；新增**常設環境變數斷言**一列 | data-category-sourcing §5、Spike §11.4、§11.6(3) |
| v5-10 | §1 理由、§1 風險表 | `data` 供給阻塞**解除**（25 候選 / 7 repo）；`anthropics/skills` 實數清點＝17；**`doc-coauthoring` 無 LICENSE，依精選標準第 1 項暫不可精選**，`writing` 官方精選供給實為 2 個。風險表原「零供給」列標為已解除，**改列兩個新風險**：來源品質梯度低、候選語意過度集中 | data-category-sourcing §1、§5、§6 |
| v5-11 | §2 白名單表、選項 A 註記、候選方向表 | 白名單新增三個 `data` 來源 repo；`anthropics/skills` 的 License 欄改為**逐目錄判定**（12 Apache／4 source-available／1 無 License）；選項 A 由「數量待查」改為「已查證不足，維持否決」；**候選方向表依查核重寫**——VoltAgent **無 Data & Analytics 分類**、官方廠商條目**全數因依賴出局**（選錯篩選維度）、**License 缺失為頭號殺手**（446★ 與 24,904★ 的候選皆倒在第 1 項）；並記錄**回溯流程成本被低估**（582 個連結指向市集站而非原 repo） | data-category-sourcing §1～§5 |
| v5-12 | §8.1、§8.2 | 成本模型依 mini 檔位重算：單 Run 上限 **$0.30**、中位 **$0.05**、封測 30 人月模型費最壞 **$270**／預估 **$45**；合計中位 **$328**／最壞 **$553**，**模型費佔比由 49–85% 降為 14–49%**。首月額度替代語意的成本代價由 $54 vs $90 改為 **$9 vs $15** | §5.2a、cost §6.2.3 |
| v5-13 | §9.1、§9.2、附錄、§10.5 | 檢查清單三項標「提案完整可定案」並收斂補測項；`02` 回寫對照表解除 300K 的例外（但要求連輪數表一起寫）；附錄模型定價表整表換為 OpenAI，**新增「v5 已失效、不可再引用的數字」對照表**；§10.5 待辦由 9 項重整為「已結案 4 項 ＋ 待處理 9 項」，並註明**已無高嚴重度未解項** | 本次修正的連帶 |

**v5 的一句話結論**：供應商定案的價值不只在選了誰，而在**它讓四項補測從「等憑證」變成「在正式後端上直接測」**——結果是兩個被推翻的假設（旗艦不會提高 Skill 觸發率、prompt caching 不會買到更多輪數）與一個被證實的緩解（`skills` 白名單真的擋得住）。這三件事若留到 M1 才發現，代價分別是選錯預設模型、寫錯 `02` 的允收準則、以及為一個未驗證的緩解設計實作。

---

### v4 修正（2026-08-14）

前置項 2（Agent SDK 的 Skill 載入路徑）Spike 已完成，**PDM-003 三個技術前置全部解除**。**依據**：[pdm-003-litellm-spike-report.md](pdm-003-litellm-spike-report.md) **新增的 §10**（載入機制 6/6 PASS）。**未修改任何 ADR、`02`、`03`，未變更任何工作項目勾選狀態。**

| # | 位置 | 修正內容 | 依據 |
| --- | --- | --- | --- |
| v4-1 | §3 建議段「Skill 載入路徑」 | **路徑由 `<workdir>/skills/<skill-name>/` 改為 `<workdir>/.claude/skills/<skill-name>/SKILL.md`**（原假設實測不被發現，任何設定下皆然）。新增**四個啟用條件表**：`cwd` 指向含 `.claude/skills/` 的目錄；`setting_sources` 省略或顯式含 `"project"`（Sandbox 內建議顯式 `["project"]` 以排除 `~/.claude/` 污染）；`skills` 過濾語意；顯式工具清單須含 `"Skill"`。並註明**無程式化註冊 API**（Skill 只能是檔案系統產物，展開檔案即完成註冊）與 **`.claude` 為隱藏目錄**（SBX-008 展開與 SBX-009 清理須涵蓋） | Spike §10.2、§10.4、§10.6 |
| v4-2 | §3 前置項狀態表 | **三項全部解除**，PDM-003 技術前置完成。第 2 項標註「通過的是機制、不是提案原本寫的那條路徑」，並記錄**結論可直接轉移到 Anthropic 後端**的理由（判定依據取自 `init` 訊息的 `skills` 陣列，測項 1–5 完全不呼叫模型）。剩餘補測項改列為「不阻擋定案」，並**新增第四項：模型自主觸發 Skill 的能力**——測試用的 OpenAI 後端模型在「符合 description 但未點名」的任務下完全不呼叫 `Skill` 工具，此為模型行為層而非載入缺陷，但若 M1 要以「Skill 是否被觸發」作為試跑結果，自主觸發率就是產品訊號 | Spike §10.3、§10.5、§10.6 |
| v4-3 | §3 發現段新增 (d)、§3 風險表新增一列 | **Runtime Image 夾帶 CLI 內建 Skill**：`setting_sources=[]` 下 `init.skills` 仍列 15 個，**不受該參數管轄**。兩個後果：(i) 屬 50K harness 開銷的一部分；(ii) **試跑干擾源**——模型可能改用內建 Skill，使 `02:EVAL-001` 的驗收結論失真（Run 顯示成功但受測 Skill 未被用到）。緩解為 `skills` 白名單，**但其過濾效果尚未經行為性驗證**（`init.skills` 反映「發現」不反映「過濾」） | Spike §10.5 記錄項 a、§10.6-3 |
| v4-4 | §3 風險表第一列（Skill 路徑） | 原「路徑慣例未經查證」風險**標為已兌現並修正**：風險確實發生，但代價僅為文件修正——載入機制本身 6/6 通過，PDM-003 載入設計與 PDM-008 Profile 都不需重做，只換路徑與補啟用條件 | Spike §10.6 |
| v4-5 | §4 PDM-004 建議表 | 新增「**內建工具與內建 Skill 的裁減**」列：20+ 內建工具與 15 個內建 Skill 合計約 50K input tokens／輪，SBX-002 應一併評估裁減；並註明裁減幅度需與 Skill 相容性權衡（裁掉 `Read`／`Write`／`Bash` 會讓多數 Skill 失效） | Spike §6.2、§10.6-3 |
| v4-6 | §7 PDM-008 表格第 3 列 ＋ 新增 blockquote | `claude-agent-sdk` Profile 安裝位置改為 **`.claude/skills/<name>/`**，**移除「待確認」註記**；安裝說明的 `query()` options 片段**必須示範 `cwd` 與 `setting_sources`**（缺任一項 Skill 不會被載入，屬 `02:PACK-002` 的「必要設定」欄位）。新增說明：**兩個 Profile 的專案層安裝路徑實際相同**，差別只在使用者層 vs 工作目錄層與驗證方式；附建議的文案區分說法，避免使用者誤認為重複選項 | Spike §10.6 修正表、§10.6-2 |
| v4-7 | §9.1 檢查清單 | PDM-003 檢查項改為「技術前置已全部解除」並加入路徑與啟用條件的確認；補測項標「不阻擋定案」並加入自主觸發；PDM-004 補內建 Skill 裁減；**新增 PDM-008 獨立檢查項**；**新增 SBX-002 實測項**（`skills` 白名單的行為性過濾效果、裁減幅度權衡） | 本次修正的連帶 |

**v4 的一句話結論**：前置項 2 的價值不在「確認假設」而在「證偽假設」——若未做這個 Spike，SBX-008 會把 Skill 展開到一個永遠不會被載入的路徑，且問題只會在第一次真實試跑時以「Skill 沒有作用」的形式浮現，屆時很難與模型行為問題區分。

---

### v3 同步（2026-08-14）

兩個補跑的 Spike 已完成，依其結論小幅同步。**依據**：[pdm-011-spike-report.md](pdm-011-spike-report.md) **新增的 §9（v2 真 Embedding 重測）**與 [pdm-003-litellm-spike-report.md](pdm-003-litellm-spike-report.md)。**未修改任何 ADR、`02`、`03`，未變更任何工作項目勾選狀態。**

| # | 位置 | 同步內容 | 依據 |
| --- | --- | --- | --- |
| v3-1 | §3 模型表「查詢改寫」列、§3 理由 | 定位由「召回的必要條件」**下修為「Top-1 精準度增益」**——跨語言召回改由向量腿承載（繁中原樣 Top-3 100%、Top-1 80%，改寫把 Top-1 補到 100%）。**降級目標維持向量檢索、不得為原句 FTS 的結論不變。** 附帶：NFR-004 延遲風險下降，逾時砍掉是可接受降級而非品質懸崖 | PDM-011 §9.2、§9.3-A、§9.4-1 |
| v3-2 | §3 理由（新增一條） | **RRF 不得寫成品質來源。** 等權融合實測零增益，兩種設定下還倒退一名；v1「兩腿同質」的解釋已被推翻（重疊降到 37–63% 仍無增益），真因是兩腿品質不對稱。混合檢索的價值一律表述為**召回覆蓋**；並補上「零命中的腿不得參與融合」的實作要求 | PDM-011 §9.3-B、§9.4-3、§9.5 |
| v3-3 | §3 模型表 Embedding 列與其下方 blockquote | 「繁中→英文 recall@5」首要驗收條件**已實測通過**（`text-embedding-3-small`，Top-3 5/5）。**`voyage-3` 對比由定案前置降為選項**（Top-3 已 100%，無改進空間）。補記兩項未涵蓋：v2 未經 LiteLLM 閘道、索引時摘要與改寫仍為模擬 | PDM-011 §9.3-A、§9.4-5、§9.6 |
| v3-4 | §3 模型表「符合原因潤飾」列 | 「索引時生成適用任務範例句」由 nice-to-have **升為必要項**——四條中文查詢對正解的詞彙交集為零卻是向量腿的正常命中，模板路徑在這些案例完全產不出理由，而這在真實系統是主流情境 | PDM-011 §9.4-4 |
| v3-5 | §3 前置 Spike 段（改寫為狀態表） | 前置項 1（閘道相容性）**已完成、7/7 PASS、退路不需啟動**；前置項 3 已通過；**前置項 2（Agent SDK Skill 載入路徑）仍未驗證，PDM-003 維持不可定案**。並補三項實測衍生影響：**(a)** `thinking` 於 `/v1/messages` 不受 `drop_params` 管轄（僅非 Anthropic 後端觸發，變通 `MAX_THINKING_TOKENS=0`；補測時列第一優先，且使 ADR-017 的跨供應商 fallback 出現未驗證限制）、**(b)** Virtual Key 預算為軟上限（詳見 v3-6）、**(c)** Agent SDK harness 固定開銷實測 50K input tokens／輪，Runtime Image 裁減工具集是可行的成本槓桿 | PDM-003 Spike §4、§6.1、§6.2、§6.3、§7.1、§7.3 |
| v3-6 | §3 Virtual Key 殘餘風險段 | 「爆炸半徑上限＝一次 Run 的預算，約 $1.80」**修正為「約 $1.80 加上 flush 間隔內可發出的請求量」**——LiteLLM 的 spend 記帳為非同步（先扣後檢），預算是軟上限不是硬性截斷。**緩解改為「必須同時帶預算上限與即時的 `tpm_limit`」**，並寫入 §9 定案檢查項 | PDM-003 Spike §4.2、§6.3、§7.2-1 |
| v3-7 | §3 風險表第一列 | 「LiteLLM 相容端點與 Agent SDK 不完全相容 → 整個方案失效」**降級為已證偽**；殘留範圍窄化為 `thinking` 透傳待補測 | PDM-003 Spike §7.1 |
| v3-8 | §5.2 Token 上限列 | 300K input **標註為未經驗證的數值**：無 prompt caching 下僅夠 5–6 個 agent turn（harness 固定開銷 50K／輪）。**待補測 `cache_read_input_tokens` 後校正**；補測完成前不寫入 `02` 允收準則（§9.2 對照表同步加註） | PDM-003 Spike §6.2、§7.2-2 |
| v3-9 | §9.1 檢查清單、附錄 | PDM-003 檢查項改列三個前置項的實際狀態並補 `tpm_limit` 條件；新增「取得 Anthropic 憑證後的補測項」獨立檢查項（`thinking` 透傳、prompt caching、跨供應商 fallback）；附錄新增 PDM-003 Spike 報告並更新 PDM-011 報告的描述 | 兩份報告 |

**v3 未改動的結論**（避免誤讀）：降級目標必須是向量檢索、不得為原句 FTS；環境變數注入機制；`ANTHROPIC_API_KEY` 須未設的配套（其必要性反而被實測證實）；`text-embedding-3-small` 的選型；PDM-005 除 300K input 外的所有數值。

**v3 後 PDM-003 的剩餘定案阻塞：僅前置項 2（Agent SDK 的 Skill 載入路徑）。**

---

### v2 修正紀錄（2026-08-13）

依複核發現修正。**未修改任何 ADR、`02`、`03`，未變更任何工作項目勾選狀態。**

### 10.1 高嚴重度修正（7 項）

| # | 位置 | v1 | v2 | 原因 |
| --- | --- | --- | --- | --- |
| 1 | §5.2 Token 上限列 | 「約 $1.5（導入價約 $1.0）」 | **$1.80（導入價 $1.20）**，並列出算式 | **算術錯誤且自相矛盾**：300K × $3/MTok + 60K × $15/MTok = $1.80，導入價 $2/$10 得 $1.20。§8.2 原本就寫 $1.80，兩節不一致；§9 又要求把 §5.2 的值寫進 `02` 允收準則，錯值會外溢 |
| 2 | §2 白名單、§1 理由與風險 | `data` 類供給依據為「社群目錄量最大」 | 新增**回溯准入流程**（發現 → 回溯原 repo → 過九項檢查 → 入白名單）＋**三個候選來源方向（標待查核）**＋風險表新列 | 目錄依 PDM-002 自身規則只是發現管道不是來源，`data` 實際零白名單供給，8–12 個索引目標不可能達成 |
| 3 | §2 風險表、§7 共通規則 | PDM-002 寫「不提供下載」，PDM-008 寫「`standard` 以外一律阻擋」 | **統一為保守版：一律不產出 Download Artifact，含 `standard`**；兩處同步，並標註「法遵保守預設，放寬需負責人與法務確認」 | 兩節結論相反，且 PDM-008 還引用 PDM-002 作依據。授權判斷不能兩存 |
| 4 | §5.2 記憶體列 | 「4 GiB｜pandas 處理 100 MB CSV 的合理上限」 | 維持 **4 GiB**，依據欄補上「已被 cost v2 §2.1 採為 slot 規格，調整需同步重算」 | cost v1 誤用 2 GiB，**cost v2 已對齊為 4 GiB** 並連帶更換各平台機型（§2.3）。本文件補上反向引用，避免日後單方調整 |
| 5 | §8.2 成本模型 | 用量前提未定義；單 Run Sandbox 成本 $0.01–0.03；封測 30 人 $1,650／$150–300 | **改為 30 人 × 30 Run = 900 Run/月**；引用 cost v2 §4.4 (a′) 的 **$283/月平台費**；單 Run Sandbox **$0.22**；合計中位 **$553**／最壞 **$1,903**；並加註 $0.30 已回寫 cost v2 §6.2 | v1 未定義用量前提，導致 cost v1 以 75 Run/人/月 試算（與本節額度互斥）；$0.01–0.03 低估約一個數量級，忽略 **N+1 冗餘的下限效應** |
| 6 | §3 模型表查詢改寫列、§3 理由 | 「失敗即降級用原句」 | **「失敗或逾時 → 降級為跨語言向量檢索，不得降級為原句 FTS」**；理由段補 Spike 實測數據；§9 加對齊檢查項 | 直接牴觸 [pdm-011-spike-report.md](pdm-011-spike-report.md) §4.4(c)（繁中對英文語料 Top-3 僅 20%）與 §6.2-1 的明確建議 |
| 7 | 前言、§0 相依圖 | PDM-011「需先有 PDM-001／002 才能執行」，相依圖畫在 PDM-002 下游 | **初步 Spike 已交付並引用報告**；相依圖改為「PDM-001/002 → 完整 golden query set」 | Spike 已於同日以 12 份公開樣本執行完畢，`m0/README.md` 亦將其與本提案並列為平行交付物 |

### 10.2 中嚴重度修正（10 項）

| # | 位置 | 修正 |
| --- | --- | --- |
| 8 | **新增 §5.1b** | **Skill 套件匯入上限**（壓縮檔 10 MB／解壓後 100 MB 且邊解壓邊中止／檔數 2,000／深度 10／禁巢狀壓縮／**直接拒絕 symlink**／路徑穿越與檔名規則／失敗原因分類）。承接威脅模型 TM-IMP-02 與 **Q6**（列於「阻擋 SEC-002 定案的問題」）。與 Dataset 分開規範，因為前者在控制平面側解壓 |
| 9 | §5.2、§8.1 | 新增**同一 Workspace 並行 Run 上限 = 2**，標為 ADR-011 Policy 首批配置值。此前 ADR-011 要求、威脅模型閘門 B 阻擋條件、cost v2 併發推算三處都需要此值但無值可驗 |
| 10 | §3 Virtual Key 段 | 補**已知殘餘風險**（威脅模型 TM-SEC-02／Q11 明指「環境變數最容易被讀走」）＋四項緩解（每 Run 一把帶預算 $1.80 上限、TTL 建議 20 分鐘、終止即撤銷涵蓋所有路徑、供應商金鑰永不進 Sandbox）＋**兩項必須併同執行的配套**：Virtual Key 樣式列入 TRACE-005 遮罩（防 Script `echo` 經 `03:TRACE-003` 流入 Trace，違反鐵律 11）、Runtime Image 須斷言 `ANTHROPIC_API_KEY` **未設定**（不是空字串） |
| 11 | §4 風險表、§9 | 新增 **gVisor × Node 22 / Python 3.12 syscall 相容性**風險列與跑通 runsc 的檢查項。ADR-015 定案條件與威脅模型 §5.6 都明列此驗證，v1 完全未安排（PDM-003 有 Spike，PDM-004 沒有） |
| 12 | **新增 §6.1** | **帳號刪除分兩類處理**：私有內容硬刪除 vs 已被 Fork／歷史 Run 引用的版本**去識別化保留**。v1 的「硬刪除所有上述使用者資料」與同表三個「永久」列衝突，且硬刪除 Skill Version 會斷裂下游 Fork 溯源鏈（違反鐵律 4、`02:DISC-003`） |
| 13 | §2 白名單、附錄 | **obra/superpowers 降級為發現／參考**，不列入首批精選。查核後確認其為軟體開發方法論（TDD／debug／code review／git workflow），非 `writing` 素材，且 git 工作流依賴 `git`——PDM-004 明確不含，依精選標準第 6 項本就不通過。`writing` 供給敘述改為僅由 `anthropics/skills` 承擔 |
| 14 | 全文 | 導入 **`02:` / `03:` ID 來源前綴**（前言新增引用慣例）。兩份文件對 TEST／RUN／EVAL／PACK／TRACE／DISC 使用相同前綴、不同編號。**並修正 §1 的 `EVAL-001` → `03:EVAL-005`**——「規則判斷／模型判斷／使用者判斷」在 `02:EVAL-001`（結果評估）與 `03:EVAL-001`（驗收條件轉換）皆不成立 |
| 15 | §8.2、§8 風險表 | 單 Run Sandbox 成本改引 cost v2 實算值並解釋 N+1 下限效應；新增風險列說明 **$0.30/Run 是假設值不是量測值**，量測前預算應以最壞值編列 |
| 16 | §8.1 | **明寫首月額度語意**：採 `min(20, 30) = 20`（一次性額度取代首個 30 天窗口的月上限，不相加），附理由與替代語意（20+30=50，單月成本上限由 $36 升至 $90），標「請負責人明確擇一」 |
| 17 | §3 模型表 Embedding 列 | 補**首要驗收條件：繁中查詢 → 英文語料 recall@5**，並列為定案前置（真 Embedding 重測，Spike 因無 API key 以 TF-IDF 代用，所有向量結論皆未證實）。v1 的選型理由（維度成熟、索引體積）未涵蓋 Spike 認定的決定性條件 |

### 10.3 低嚴重度修正（7 項，全部採納）

| # | 位置 | 修正 |
| --- | --- | --- |
| 18 | §6 理由 | cost v2 §2.1 已將物件保存期改為「90 天（Trace）／30 天（Artifact）」並註明依據 PDM-006，**此項無待回寫**，於理由段註明 |
| 19 | §1 理由第三點 | egress 允許清單由「只有 LiteLLM 閘道與物件儲存」改為與 §5.2 一致的**三項**（含 Trace ingestion 端點），對齊 ADR-005 執行節點拓撲的三條路徑 |
| 20 | §4 套件安裝列、風險表 | 刪除「失敗訊息明確」的宣稱——default-deny + Proxy 固定 DNS 下只會得到通用網路錯誤；可理解性改由匯入階段預先揭露承擔（NFR-007） |
| 21 | §2 白名單、附錄 | VoltAgent 條目數由「1500+」改為「約 1,500（自述 1,400+，badge 1,497+）」；`anthropics/skills` 的「17 個 skill」改標**待清點**（該數字被用作否決「官方唯一」選項的依據） |
| 22 | §1 風險表、§7 風險表 | 「你最想串接什麼」與「目標 Agent 需求」由 BETA-003 改引 **BETA-005**（範圍與優先級複審）；BETA-003 專指評估報告與改善建議的質性回饋 |
| 23 | §7 建議表與風險表 | 計數統一為「**1 個標準套件 + 2 個已驗證安裝 Profile**」（`standard` 不計為安裝 Profile）；`claude-agent-sdk` 的 `<workdir>/skills/` 路徑標為**待 Spike 驗證**，並在 §3 風險表與前置 Spike 補對應項 |
| 24 | **新增 §9.2** | **回寫 `02` 的對照表**。`02` 沒有 PDM-* 需求 ID，v1 只為 PDM-005／006 指名回寫目標，其餘六項無處可寫。附註威脅模型 Q19（`02` 同樣缺 SEC-* 允收準則）宜一併處理 |

### 10.4 與 v2 交叉文件的對齊點

| 對齊項 | 本文件 | cost-estimation.md v2 | threat-model v2 |
| --- | --- | --- | --- |
| Sandbox slot 記憶體 | PDM-005 §5.2：4 GiB | §2.1 採 4 GiB（v1 的 2 GiB 已修正），§2.3 據此換機型 | 基線 C-11 要求設記憶體上限（Q5 待值） |
| 單 Run Token 上限與成本 | §5.2／§5.2a：300K/60K（**v5 已驗證**），mini 檔位上限 **$0.30 熱／$0.50 冷** | §6.2.3 採中位 **$0.05**（v5） | TM-EXE-02 等待 PDM-005 定值 |
| 模型供應商與分層 | 標頭定案紀錄 ＋ §3 模型表（OpenAI，六項用途分層） | §6.2.3 依 mini 檔位重算 | TM-MDL-02（分層失效即成本失控） |
| 免費額度與用量 | §8.1：30 Run/人/月；§8.2：30 人 × 30 = 900 Run/月 | §2.2 加註落差，§4.4 提供 (a′)/(b′)；§7.4 決策問題 3 | TM-MDL-02 等待 PDM-010 |
| 平台成本 | §8.2 引用 $283/月（E1 Hetzner） | §4.4 (a′) $283；推薦 E1 Hetzner 容器化 | — |
| 保存期限 | §6 分級表、§6.1 帳號刪除 | §2.1 採 90 天 Trace／30 天 Artifact | Q9（TM-DAT-03 待此定值） |
| 並行 Run 上限 | §5.2：2 | §2.2 尖峰併發推算 | 閘門 B 阻擋條件之一 |
| Skill 套件解壓上限 | **§5.1b（新增）** | — | **Q6**（阻擋 SEC-002 定案）、TM-IMP-02 |
| Virtual Key 注入 | §3：環境變數 + 殘餘風險 + 遮罩配套 | — | **Q11**、TM-SEC-02、基線 D-03／D-06 |
| Runtime × gVisor 相容性 | §4 風險表、§9 檢查項 | §8 定案前置之一 | §5.6 Runtime 相容性測試、基線 C-09 |
| egress 允許清單 | §1、§5.2：三項 | — | 基線 N-01／N-02／N-07 |
| Image pin 方式 | §3 風險表：pin by digest | — | 基線 I-02 |

### 10.5 本文件仍未解決、需負責人處理的事項（v5 更新）

**已結案（不再是待辦，保留供追溯）：**

1. ~~**PDM-003 前置項 2**~~ —— **v4 解除。** 三個技術前置全部通過。
2. ~~**`data` 類別供給**~~ —— **v5 解除。** [data-category-sourcing.md](data-category-sourcing.md) 查得 25 個合格候選 / 7 個 repo，超過 8–12 目標，換類別條件未成立。**這是 v4 標註的「唯一高嚴重度未解項」，現已無高嚴重度未解項。**
3. ~~**取得 Anthropic 憑證後的補測四項**~~ —— **v5 全數結案。** 兩項實測完成、兩項因供應商定案不再適用。詳見 §3 補測狀態表。
3b. ~~**`skills` 白名單的行為性過濾效果**~~ —— **v5 已實測有效**（Spike §11.4），並採為 SBX-002 預設。

**仍待處理：**

4. **PDM-009 封測人數**——本文件假設 30 人，cost 用 20 人，兩者需一併定案。
5. **source-available 打包政策**——目前為法遵保守預設，需法務確認。
6. **首月額度語意**——`min(20,30)` 或相加，請明確擇一。**v5：兩者的成本差已由 $54 vs $90 降為 $9 vs $15**，決策重心從成本移向濫用防護。
7. **Token 中位數 $0.05 未經量測**——M1 需以 20–30 次真實 Run 收窄，量測時須同時記錄每輪工具呼叫次數。**300K input 上限本身已於 v5 驗證，此項不再是未解事項。**
8. **平台級模型預算煞車**（威脅模型 Q13）——不在本提案範圍，建議歸 ADR-011 Policy 模組。**v5 補充**：Virtual Key 預算為軟上限、且與 token 上限脫鉤 7–8 倍，使此項的必要性再度提高。
9. **匯入 SSRF 的 MVP 歸屬**（威脅模型 Q15）——MVP 期間無工作項目承接，需依該文件建議的兩條途徑擇一。
10. **`excel-*` 的類別歸屬**（v5 新增）——12 個 `excel-*` 歸 `data` 或 `documents` 影響 `data` 精選池厚度（25 vs 13），定案時需明確寫下歸類原則。
11. **CLI 內建工具與內建 Skill 的裁減幅度**（v5，唯一仍待實測的 SBX-002 項）——這是削減約 19.4K 固定前綴的唯一路徑，`skills` 白名單不是。
12. **`doc-coauthoring` 的授權歸屬**（v5 新增）——目前無 License 檔故不可精選；可向 Anthropic 詢問，或接受 `writing` 官方精選供給只有 2 個。
