# 任務情境卡

- 12 張情境卡 ＋ 2 張干擾卡，每位受測者做 **5 張情境卡 ＋ 2 張干擾卡**
- 對應：[README.md §4](README.md) 與 golden set 的關係、[moderator-guide.md §3](moderator-guide.md) 給卡方式

> ## ⚠️ 分版紀律
>
> - **§2 受測者版**：只有情境文字。可以印出來、可以貼進聊天視窗。
> - **§3 主持人版對照表**：含 golden query family 與預期 gold skill。**不得出現在受測者視線內。**
> - **§4 分派表**：主持人用。
>
> 印製時**只印 §2**。遠端模式只分享單一視窗，不分享桌面。

---

## 1. 卡片設計說明

### 1.1 三個維度怎麼分

| 維度 | 值 | 卡片 |
| --- | --- | --- |
| **類別** | documents / writing / data | 各 4 張 |
| **題型** | 直述（明說動作與資料）／口語模糊（不含任何技術詞） | 直述 6、口語模糊 3、英文卡 3 |
| **語言與跨語言方向** | 見下 | — |

**「跨語言」在本測試有三種方向，不是一個開關**：

| 方向 | 說明 | 卡片 |
| --- | --- | --- |
| **正向跨語言**（繁中查詢 → 英文 `SKILL.md`） | 線上目錄多數是英文文件，繁中查詢天然跨語言。golden set 的 30 條繁中查詢全屬此類 | DOC-1、DOC-2、DOC-3、WRI-1～3、DAT-1～3（9 張） |
| **反向跨語言**（英文查詢 → 簡中 `SKILL.md`） | 較少被量到的方向。線上目錄的簡中文件集中在 YuYY excel 系列與 `data-analyst` | **DOC-4、DAT-4**（2 張） |
| **同語言對照組**（英文查詢 → 英文 `SKILL.md`） | 沒有跨語言負擔的基準線。與上一列對比才知道跨語言到底扣了多少 | **WRI-4**（1 張） |

> **`writing` 類為何沒有反向跨語言卡**：線上目錄的 writing 類 10 筆**全部是英文文件**，結構上做不出「英文查詢 → 中文文件」的案例。這不是設計疏漏，是目錄現況；`WRI-4` 因此設為同語言對照組，反而提供了 `DOC-4`／`DAT-4` 的比較基準。

### 1.2 卡片寫法的三條規則

1. **不是查詢句。** 每張卡是一段情境敘述，**不能直接複製貼上當查詢**。沒有任何一張卡以動詞開頭的祈使句結尾。
2. **不出現 Skill 名稱與檔案格式關鍵字**（口語模糊卡）。直述卡可以出現格式（CSV、Word），因為那正是「直述」的定義。
3. **英文卡以英文書寫**，讓受測者自然以英文輸入。中文卡以繁中書寫。

### 1.3 刻意保留的三個難點

| 卡片 | 難點 | 為什麼保留 |
| --- | --- | --- |
| **DOC-2** | 情境提到「掃描」與「表格」，但**不點名 PDF** | 這是 golden set v1 唯一的 recall@5 miss（D02，第 10 名），v2 增強索引後修復到第 3 名。它是**索引時任務範例句是否真的有效**的直接檢驗 |
| **WRI-3** | 線上有 4～5 個語意重疊的候選（`humanizer`／`ai-written-check`／`cringe-check`／`full-review`／`line-edit`） | 近義群是真實目錄的常態，也是 golden set §10.6 記錄的「增強的代價」。同時是 DISC-009 比較功能的自然觸發題 |
| **DOC-4／DAT-4** | gold 是簡體中文文件，**且是 CONTENT-005 待改寫的那批** | 若 CONTENT-005 已完成而這兩張仍集中失敗，問題就不在語言（見 `analysis.md` §3） |

---

## 2. 受測者版（可印出／可分享）

> 給受測者的指示（每張卡一致）：**「這是一個情境。你就是這個情境裡的人。請用你自己的話，在這個網站上把你要的東西找出來。」**

---

### 卡片 DOC-1

你在一個小團隊裡負責整理會議。今天的會議你已經在筆記軟體裡列好一份分層的大綱：三個主題，每個主題下面有幾條決議和待辦事項。

主管要的是一份可以直接寄出去的 Word 檔——有標題階層、有目錄。你手上只有那份大綱。

---

### 卡片 DOC-2

廠商每個月會寄一疊單據給你。那些是掃描器掃出來的檔案，不是可以直接複製文字的那種，每一份裡面都有一張表格。

月底你要把這些表格的內容整理成一份東西交出去。你想先把表格裡的內容取出來，再把好幾份合成一份。

---

### 卡片 DOC-3

下週一你要跟老闆和幾個部門主管交代一個做了三個月的專案。

你手上有一堆過程記錄，但你不想站在那邊照著唸「我們做了什麼、然後又做了什麼」。你希望講完之後，他們記得住重點。時間 15 分鐘。

---

### 卡片 DOC-4（English）

You keep a stock sheet with around 900 rows. Every time you scroll down, the header row disappears and you lose track of which column is which.

The columns are also all over the place — different widths, some numbers left-aligned, some right. You want the sheet to stay readable while you scroll, and to look consistent throughout.

---

### 卡片 WRI-1

你們公司剛做完一次品牌調整。設計師給了你一份新的用色、字型和語氣說明。

接下來每個人寫出去的東西——電子報、社群貼文、提案——都要照這份走。你希望以後不用每次都人工對一遍。

---

### 卡片 WRI-2

公司下個月要換一套請假系統，你被指派寫一封給全體同仁的公告。

要說清楚為什麼換、什麼時候換、他們自己要做什麼，而且不能寫得像法律條文讓大家直接跳過。

---

### 卡片 WRI-3

你上週交了一篇稿子，編輯回你一句「這讀起來怪怪的，很平」。

你自己再看一次，覺得每一句都沒錯，但整篇就是沒有人味。你想找東西幫你看看問題在哪裡，順一順。

---

### 卡片 WRI-4（English）

Someone sent you a guest post for your site. Before you publish it, you want to know whether it was actually written by a person or generated.

You are not trying to rewrite it — you just want a read on it.

---

### 卡片 DAT-1

你手上有一份匯出的 CSV。同事那邊的程式只吃「一行一筆、每一筆是一個物件」的那種檔案。

你要把這份 CSV 換成他要的樣子，欄位名稱不能跑掉。

---

### 卡片 DAT-2

你要把一份客戶名單寄給外部的合作廠商。

寄出去之前，你想先確認裡面有沒有不該外流的東西——身分證字號、手機、地址那類的。你希望有人幫你標出來在哪幾欄。

---

### 卡片 DAT-3

有人丟給你一份檔案，只說了一句「你先看一下」。

你打開，幾千列、二十幾欄。你完全不知道要從哪裡開始，也不知道這份東西到底在講什麼。你想先弄清楚它長什麼樣子。

---

### 卡片 DAT-4（English）

Your mailing list spreadsheet grew by merging three separate exports. The same person now shows up two or three times.

You want one row per person, and you do not want the formatting of the surviving rows to change.

---

### 卡片 X-1

下個月你要去日本七天，想把機票和租車一起處理掉。預算有限，時間排得很滿。

---

### 卡片 X-2（English）

Your car has started making a noise once you get up to highway speed. Someone told you it might be the timing belt.

You want to know what is actually involved in changing it.

---

## 3. 主持人版對照表 ⚠️ 不得給受測者

判定 `gold_in_top5` 的規則：Top-5 內出現 **primary 或 acceptable 任一項**即記 `Y`。判定 `picked_is_gold` 同理。

| 卡 | 類別 | 題型 | 語言／跨語言方向 | golden query family | **gold primary** | **gold acceptable** | 備註 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| **DOC-1** | documents | 直述 | 繁中／正向 | **D01**（從大綱產出 Word 文件） | `docx` | `document-format-skills` | golden set 原 acceptable `minimax-docx` 不在線上目錄 |
| **DOC-2** | documents | 直述（**未點名格式**） | 繁中／正向 | **D02**（掃描檔抽表格再合併） | `pdf` | — | v1 唯一 recall@5 miss（第 10 名）→ v2 第 3 名。**診斷題**，見 §1.3 |
| **DOC-3** | documents | **口語模糊** | 繁中／正向 | **D09**（要向上匯報，不提檔案格式）＋ D03 | `pptx` | `handoff` | 原 gold `pitchcraft`／`deck-publisher` 皆不在線上目錄，本卡只有單一 primary，**判定最嚴的一張** |
| **DOC-4** | documents | 直述 | **英文／反向**（→ 簡中文件） | **D04／A14**（試算表要看得懂、格式一致） | `excel-freeze` | `excel-format`、`excel-insert` | YuYY 系列為簡體中文，屬 CONTENT-005 待改寫批次 |
| **WRI-1** | writing | 直述 | 繁中／正向 | **W01**（品牌語氣與字型規範） | `brand-guidelines` | — | |
| **WRI-2** | writing | 直述 | 繁中／正向 | **W02**（公司內部公告） | `internal-comms` | — | |
| **WRI-3** | writing | **口語模糊** | 繁中／正向 | **W05／W13**（讀起來像 AI 寫的，要人味） | `humanizer` | `line-edit`、`cringe-check`、`full-review`、`ai-written-check` | **近義群題**，DISC-009 比較功能的觸發點。原 gold `humanize`／`prose-revision` 線上對應為 `humanizer`／`line-edit` |
| **WRI-4** | writing | 直述 | **英文／同語言對照組** | **W06／W14**（判斷是否 AI 生成） | `ai-written-check` | `humanizer`、`full-review`、`cringe-check` | 原 gold `ai-check` |
| **DAT-1** | data | 直述 | 繁中／正向 | **A05**（CSV 轉 JSONL） | `csv-to-json` | `json-restructure` | |
| **DAT-2** | data | 直述 | 繁中／正向 | **A07**（標出個資欄位） | `pii-flag` | — | UI 文案不得暗示這是合規保證（NFR-001）；受測者若如此解讀，**記進 `analysis.md` §2.5 的風險訊號** |
| **DAT-3** | data | **口語模糊** | 繁中／正向 | **A08／A09**（一堆資料看不出所以然） | `data-analyst` | `data-shape`、`data-cleanliness-scan`、`excel-scout` | `data-analyst` 為簡體中文，屬 CONTENT-005 待改寫批次 |
| **DAT-4** | data | 直述 | **英文／反向**（→ 簡中文件） | **A01／A13**（Excel 去除重複列） | `excel-deduplicate` | `excel-find-duplicates` | 近義對（移除 vs 偵測）。YuYY 系列，CONTENT-005 待改寫批次 |
| **X-1** | 干擾 | — | 繁中 | **D11／A11 家族**（生活題） | **無結果** | — | 預期：`no_results` ＋ `query_suggestion` |
| **X-2** | 干擾 | — | 英文 | **A20**（機械維修） | **無結果** | — | 預期同上。golden set 中 A20 的最高相似度落在干擾分布內，門檻 0.75 應拒答 |

### 3.1 干擾卡的判定（G3）

系統應回 `no_results` ＋ 改寫建議。記錄三件事：

| 欄 | 記什麼 |
| --- | --- |
| `system_refused` | 系統是否真的回無結果（**若系統硬給了結果，該題次 G3 不通過，且是 DISC-005 的重大訊號**） |
| `distractor_reaction` | 受測者第一反應：`rewrote`（照建議改寫）／`gave_up`（放棄）／`thinks_broken`（認為壞了）／`accepts`（直接接受） |
| `g3_pass` | 受測者是否**認同「沒有結果」是正確且誠實的回應**。判定標準見 `moderator-guide.md` §3.D |

> **若某位受測者的兩張干擾卡系統都回了結果**：停止該場後續，立刻檢查 `MaxCosineDistance` 是否被改動與目錄是否被異動（README §3.2 環境凍結）。這是測試作廢的條件之一。

---

## 4. 分派表

每位受測者 **5 張情境卡 ＋ 2 張干擾卡**。分派滿足四個條件：

- 每人涵蓋**三個類別**
- 每人**至少 1 張口語模糊卡**、**至少 1 張英文卡**
- 每個類別在全體共 **15 題次**（對應 G4 的「類別通過率 < 8/15 即否決」）
- 直述卡各用 4～5 次，口語模糊與英文卡各用 3 次

| 受測者 | 層 | 第 1 題 | 第 2 題 | 第 3 題 | 干擾① | 第 4 題 | 第 5 題 | 干擾② |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| P1 | 學習者 | DOC-1 | WRI-1 | DAT-2 | **X-1** | DAT-3 | DOC-4 | **X-2** |
| P2 | 學習者 | DOC-2 | WRI-2 | DAT-1 | **X-2** | DOC-3 | WRI-4 | **X-1** |
| P3 | 學習者 | WRI-1 | DAT-2 | DOC-1 | **X-1** | WRI-3 | DAT-4 | **X-2** |
| P4 | 改善者 | DOC-2 | WRI-2 | DAT-1 | **X-2** | DAT-3 | DOC-4 | **X-1** |
| P5 | 改善者 | DOC-1 | WRI-1 | DAT-2 | **X-1** | DOC-3 | WRI-4 | **X-2** |
| P6 | 改善者 | WRI-2 | DAT-1 | DOC-2 | **X-2** | WRI-3 | DAT-4 | **X-1** |
| P7 | 精深者 | DOC-1 | WRI-2 | DAT-2 | **X-1** | DAT-3 | WRI-4 | **X-2** |
| P8 | 精深者 | DOC-2 | WRI-1 | DAT-1 | **X-2** | DOC-3 | DAT-4 | **X-1** |
| P9 | 精深者 | WRI-2 | DAT-2 | DOC-2 | **X-1** | WRI-3 | DOC-4 | **X-2** |

**干擾卡的位置刻意錯開**：奇數場先給 X-1、偶數場先給 X-2，避免所有受測者在同一個位置遇到同一張干擾卡而讓第二張失去效力。

### 4.1 使用次數檢核

| 卡 | 次數 | | 卡 | 次數 |
| --- | --- | --- | --- | --- |
| DOC-1 | 4 | | WRI-3 | 3（模糊） |
| DOC-2 | 5 | | WRI-4 | 3（英文） |
| DOC-3 | 3（模糊） | | DAT-1 | 4 |
| DOC-4 | 3（英文） | | DAT-2 | 5 |
| WRI-1 | 4 | | DAT-3 | 3（模糊） |
| WRI-2 | 5 | | DAT-4 | 3（英文） |

**合計 45 題次**（documents 15、writing 15、data 15）＋ 干擾 18 題次。

### 4.2 覆蓋矩陣

| | documents | writing | data | 小計 |
| --- | --- | --- | --- | --- |
| 直述・繁中（正向跨語言） | DOC-1(4)、DOC-2(5) | WRI-1(4)、WRI-2(5) | DAT-1(4)、DAT-2(5) | **27** |
| 口語模糊・繁中（正向跨語言） | DOC-3(3) | WRI-3(3) | DAT-3(3) | **9** |
| 英文・反向跨語言 | DOC-4(3) | — | DAT-4(3) | **6** |
| 英文・同語言對照組 | — | WRI-4(3) | — | **3** |
| **小計** | **15** | **15** | **15** | **45** |
| 干擾（繁中 X-1／英文 X-2） | — | — | — | **18** |

### 4.3 有人缺席怎麼辦

少 1 人（8 人 × 5 = 40 題次）：G1 門檻改為 **≥ 28/40**（70%），G2 改為 ≥ 6/8，G3 改為 ≥ 12/16（75%），G4 的類別門檻改為該類別實際題次的 50%。**優先補足缺席者所屬的成熟度層**——三層各 3 人是 G4 否決條款的前提。

少 2 人以上，或某一層只剩 1 人：**不得宣告通過**，只能宣告「樣本不足，需補場」。
