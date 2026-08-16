# CONTENT-007：`writing` 類精選的預設 rubric

- 日期：**2026-08-17**
- 對應需求：[`02` §4.7 CONTENT-007](../02-specifications-and-acceptance-criteria.md) 第 3 條——「`writing` 類的每個精選附一份可編輯 rubric，供 LLM Judge 逐項回傳證據引文」
- `rubric_version`：**`content-007/writing/v1`**（本文件即該版本的內容。任一條目的文字、`weight` 或 `evidence_required` 變更＝**升版**，並依 [`02:EVAL-013`](../02-specifications-and-acceptance-criteria.md) 第 3、4 條重跑 Judge 回歸）
- 對象：`curated-skill-list.md` §2.2 的 **5 個 `writing` 精選**——`brand-guidelines`、`internal-comms`、`humanizer`、`line-edit`、`ai-written-check`
- 消費端：[`contracts/openapi/llm-internal.yaml`](../../../../contracts/openapi/llm-internal.yaml) 的 `Rubric`／`RubricItem`、`POST /judge-run`
- ~~**CONTENT-007 仍不勾**，缺口逐項見 §6。本文件關掉的是「rubric 的內容不存在」，**沒有**關掉「rubric 存不進平台、使用者改不到、產品路徑上送不出去」。~~
- **2026-08-17 更新：接線已完成，`CONTENT-007` 已勾。** G1～G4 全部關閉（§6.2），rubric 現在存得進 Test Case、跟得上快照、送得到 Judge、回歸 harness 吃得下。A 輪回歸（[report-judge-regression.md §11](../m3/report-judge-regression.md)）另外查出一個**新缺口 G7**，以及 §2.2 的一處事實錯誤——**Judge 看得到的證據不只最終回覆與 manifest，trace 的 `tool_call` payload 是第三條路徑**，見 §2.2 的更正註記。

---

## 1. 這份文件是什麼

`writing` 類的**預設 rubric**——策展提供的起手值。依 [m3/evaluation-design.md §6.4](../m3/evaluation-design.md)，rubric 是**驗收條件的一種強化形式，不是第二套機制**：它逐項對應到 `criterion_results` 的條目，只是額外要求該條回傳證據引文。「可編輯」的語意是它跟著 Test Case 走、使用者改得動，**策展只負責預設值**。

它不是什麼：

- **不是**評分表。平台目前不對 `weight` 做任何算術（§2.4）。
- **不是** `data`／`documents` 兩類的 rubric。`02` 只對 `writing` 要求 rubric，本文件照它做，不擴張。
- **不是**六類問題的全部。規格、啟用、執行、相容、成本五類由 Go 的規則腿判定（設計 §2.5），**rubric 只寫「任務效果」這一類**——那是唯一該上模型的一類。

---

## 2. 條目形狀為什麼長這樣

四條形狀規則不是風格偏好，是照已上線的管線逐條推出來的。**推導的依據寫在這裡，是為了讓下一個改 rubric 的人知道哪一條動了會讓證據無聲消失。**

### 2.1 rubric 條目的 `id` 必須等於某一條驗收條件的 `id`

`/judge-run` 的回應是**每條驗收條件一個判定**（`JudgeVerdict.criterion_results`），而 `JudgeCriterion.id` 的契約說明寫著：**沒被送出去的 id 會被 Go 丟掉**。所以一個只存在於 `rubric.items[]`、不在 `criteria[]` 裡的 id，它的判定不會被儲存，「逐項回傳證據引文」在資料上不成立。

因此本文件每個 Skill 給的是**一組成對的東西**：`criteria[]`（送進去要判的條件）與 `rubric.items[]`（同一組 id 的強化措辭）。兩者不是重複——prompt 把它們排在兩個區塊，條件是「要判什麼」，rubric 是「判到什麼程度算過、要不要出示引文」。

### 2.2 要求引文的條目，其判定依據必須出現在**最終回覆**

Go 對三種證據的回驗方式不同（`internal/eval/judge.go` 的 `verify()`）：

| `kind` | Go 驗什麼 | 對 rubric 的意義 |
| --- | --- | --- |
| `agent_output` | 引文必須**逐字出現在 `final_output`** 裡 | 這是文字品質類條目**唯一**可用的證據型別 |
| `artifact` | 只驗**路徑在 manifest 上**；引文不比對 | 只能證明「檔案存在」，證明不了檔案裡寫了什麼 |
| `trace_event` | id 必須在本次 digest 內，且引文是該事件 payload 的子字串 | 適合啟用／工具類，**那些是規則腿的事** |

關鍵事實：`buildRequest` **只送 artifact 的 manifest 列，不送任何位元組**（註解寫明理由：讀檔內容等於在控制平面解開封存）。所以~~**產出只寫進 `/out/artifacts/` 而沒有出現在最終回覆時，文字品質類條目沒有任何可引用的證據**，四條防線的第 3 條會把它降成 `undetermined`~~。

這直接決定 §3 的 Prompt 必須改一句。

> **2026-08-17 更正（A 輪實測，[report-judge-regression.md §11.1](../m3/report-judge-regression.md)）**：上一段刪除線內的推論**是錯的**。上表漏了第三條證據路徑——**agent 寫檔的 `tool_call` 事件，其 payload 帶著寫入當下的正文，而 `trace_event` 型證據是逐字回驗的**。實測 5 筆基準 Run 的最終回覆確實都只有一行檔名（33～49 字元），但 22 個 rubric 條目只有 3 條回 `undetermined`，其餘都引得到證據。<br>**§3 的 Prompt 第 4 條仍然保留**：trace 這條路徑取決於 agent 恰好用了寫檔工具、且該事件恰好落在 digest 尾 100 筆內，兩者都不是契約保證的。但它的「非加不可」要降級為「加了比較穩」，代價（輕度投其所好）的論述不變。

### 2.3 「沒有 X」型條目不要求引文

引文證明得了存在，證明不了不存在。所以純缺席型條目（例如「不使用破折號」）一律 `evidence_required: false`，並在條目文字裡寫明：**判 `failed` 時才給引文，引文指向違規的那一句**。這讓「不出示引文」不再等於「偷懶」，也不會逼模型去為一個缺席捏造一段引文。

### 2.4 `weight` 現在只是 prompt 上的一個字

`services/llm` 把 `weight` 渲染成 `(weight N)` 印給模型看，**平台端沒有任何地方拿它做加權運算**——`overall` 由 Go 合併規則腿之後自行重算。本文件仍然填 `weight`，因為它是給模型的相對重要性訊號，也是給下一個改 rubric 的人看的優先序；**但不要把它當分數**。值域約定：**3 ＝ 不符即整體不可能 `met`**、**2 ＝ 主要**、**1 ＝ 次要**。

---

## 3. 共用的輸入、Prompt 與一句必要的修改

**Dataset**：沿用 M2 基準的 `draft.md`（一段有贅詞與自誇語氣的 Q2 更新草稿，合成內容、無 Secrets 與個資），見 [m2/content-baseline-report.md §3](../m2/content-baseline-report.md)。`writing` 五筆不需要 `data.csv`，但一併給不影響判定。

**Prompt**：沿用 M2 的模板（首句點名 Skill ＋ 該 Skill 自己的第一句任務範例句 ＋ 三條環境規則），**`writing` 類另加第 4 條**：

```
4. 最終回覆必須完整貼出這次產出的正文,不能只說明檔名。
   評估只讀得到你的最終回覆與檔案清單,讀不到檔案內容。
```

**為什麼非加不可**：§2.2。不加這句，rubric 逐項都會因為沒有可引用的證據而降為 `undetermined`——那不是 Judge 判不動，是平台沒把東西送給它。

**這句話的代價要講清楚**：它等於告訴受評的 agent「評估看得到什麼」，屬於輕度的投其所好。接受它，是因為另一個選項（把 artifact 位元組送進控制平面解封存）會踩到設計 §2.2 明文劃下的界線。**替代解法是讓規則腿讀 artifact 文字**——那是 M3 之後的事，記在 §6 的 G5。

**連帶後果**：Prompt 改了就是新的 Test Case 快照，M2 那 5 筆 `writing` 基準 Run **不是**在這個 Prompt 下跑的。§5 因此把重跑拆成兩種模式。

---

## 4. 五份預設 rubric

每個 Skill 一段依據說明 ＋ 一段可直接貼進 `JudgeRunRequest` 的 JSON。條目內容一律只取該 Skill **自己的文件所載**的行為（來源為 `tools/content/summaries.json` 的該筆摘要與限制欄，該批已通過 CONTENT-005 的忠實性審校，精選 15/15）——**不寫套件文件沒有寫的能力**，與 `02:CONTENT-005` 否決條件 (a) 同一條紀律。

編號說明：`r1`～`rN` 是本 rubric 新增的任務效果條件；M2 的三條基準條件（`skill_activation`、`/out/artifacts/` 有檔案、最終回覆說明產出）**照舊保留**，不進 rubric——前兩條是規則腿的事，第三條被上面的 Prompt 第 4 條吸收。

### 4.1 `humanizer`

依據：摘要載明它「檢查文字中的常見 AI 寫作痕跡……並將內容改得更自然」，限制欄載明「必須保留原文中的所有主張，不得加入……未提供的事實、姓名、數字、日期、引文」與「最終改寫不使用 em dash 或 en dash；若使用者提供的寫作樣本本身使用 em dash，則依樣本的頻率保留」。本測試**不提供寫作樣本**，故該例外不適用，條目文字寫明這一點。

```json
{
  "criteria": [
    {"id": "humanizer-r1", "text": "最終回覆含改寫後的完整正文。"},
    {"id": "humanizer-r2", "text": "改寫保留原草稿的每一項主張，且未加入草稿沒有的事實、姓名、數字、日期或引用。"},
    {"id": "humanizer-r3", "text": "改寫移除或重寫了草稿中的贅詞、空泛歸因或公式化結尾。"},
    {"id": "humanizer-r4", "text": "最終改寫不使用 em dash（—）或 en dash（–）。"}
  ],
  "rubric": {
    "items": [
      {"id": "humanizer-r1", "weight": 3, "evidence_required": true,
       "text": "最終回覆必須含改寫後的完整正文，而不只是「已完成改寫並存檔」這類說明。引文請取改寫正文的開頭一句。只給檔名或摘要視為未達成——本條不成立時，其餘三條沒有可判的對象。"},
      {"id": "humanizer-r2", "weight": 3, "evidence_required": true,
       "text": "改寫必須保留原草稿的每一項主張，且不得加入草稿裡沒有的事實、姓名、數字、日期、引文或引用資料。判 passed 時，引文請取改寫中承載原草稿主張的一句；判 failed 時，引文請取那句新增的內容。語氣與措辭的改變不算新增事實，新的數字、機構名或因果宣稱算。"},
      {"id": "humanizer-r3", "weight": 2, "evidence_required": true,
       "text": "改寫必須實際處理掉草稿裡的 AI 寫作痕跡，至少涵蓋贅詞、空泛歸因（沒有主體的「業界普遍認為」類說法）或公式化結尾其中一類。引文請取改寫後對應的那一句，並在 reason 裡點出它取代了原草稿的哪一處。整篇只換同義詞而句構與套語照舊者判 failed。"},
      {"id": "humanizer-r4", "weight": 2, "evidence_required": false,
       "text": "最終改寫的正文不得使用 em dash（—）或 en dash（–）。套件文件的樣本頻率例外在本測試不適用，因為本次沒有提供寫作樣本。判 failed 時引文請取含破折號的那一句；判 passed 時不需引文——缺席無法引用。"}
    ]
  }
}
```

### 4.2 `line-edit`

依據：摘要載明它「輕度潤飾作者已有的草稿，修正文法、拼字、標點、冗詞、彆扭句子與不清楚的表達，同時盡量保留原意和個人語氣」，輸出「會先給可直接複製的完整修訂版，再列出涉及語意取捨的修改與原因，並把無法確認的意思標為問題」。限制欄載明它**不代筆**、不做語氣定位評估、不做多輪審閱。

```json
{
  "criteria": [
    {"id": "line-edit-r1", "text": "最終回覆先給一份可直接複製的完整修訂全文。"},
    {"id": "line-edit-r2", "text": "修訂版保留原作者的原意與語氣，未替作者新增內容或改變立場。"},
    {"id": "line-edit-r3", "text": "修訂版消除了草稿中的贅詞或彆扭句子，且可逐處對回原句。"},
    {"id": "line-edit-r4", "text": "全文之後另列出涉及語意取捨的修改與其原因。"},
    {"id": "line-edit-r5", "text": "無法從草稿確認原意之處以問題形式標出，而非逕自猜寫。"}
  ],
  "rubric": {
    "items": [
      {"id": "line-edit-r1", "weight": 3, "evidence_required": true,
       "text": "最終回覆的第一個區塊必須是可直接複製貼上的完整修訂全文，而不是修改清單或說明。引文請取該全文的開頭一句。先列清單後補全文者仍可 passed，但全文缺席判 failed。"},
      {"id": "line-edit-r2", "weight": 3, "evidence_required": true,
       "text": "修訂必須保留原作者的原意與個人語氣，且不得替作者新增草稿沒有的內容、承諾或立場——本 Skill 的文件明載它不代筆、只處理已寫好的草稿。判 passed 時，引文請取一句保留了原作者措辭的修訂句；判 failed 時，引文請取那句被改掉立場或憑空補上的內容。"},
      {"id": "line-edit-r3", "weight": 2, "evidence_required": true,
       "text": "修訂版必須實際處理掉草稿中的贅詞、彆扭句子或不清楚的表達，且每一處都對得回原句。引文請取修訂後的那一句，reason 裡寫出它對應原草稿的哪一句。與原文逐字相同者判 failed。"},
      {"id": "line-edit-r4", "weight": 2, "evidence_required": true,
       "text": "全文之後必須另有一份清單，列出涉及語意取捨的修改及其原因——不是逐字修改的完整 diff，而是「改了意思、所以要交代」的那幾處。引文請取清單中的一項。清單缺席或只寫「已修正文法」這類無指涉的句子，判 failed。"},
      {"id": "line-edit-r5", "weight": 1, "evidence_required": false,
       "text": "遇到無法從草稿確認原意的地方，回覆必須以問題形式標出，而不是自行猜測後直接改寫。若本次草稿沒有這類地方，回覆未憑空製造問題亦視為 passed。判 failed 的情形是：修訂中出現了草稿無從支持的具體內容，而回覆沒有把它標為待確認；此時引文請取該處。"}
    ]
  }
}
```

### 4.3 `ai-written-check`

依據：摘要載明它「逐項列出規則名稱、問題原文與具體改寫，最後給出『乾淨』、『輕微修整』或『需大幅調整』的結論」，且「依模式的重複次數與既定門檻判斷，而不是一律刪除所有類似表達」。限制欄載明它**不重寫全文**、不判斷語氣與自大感（屬 `cringe-check`）、不驗證事實（屬 `full-review`）。

```json
{
  "criteria": [
    {"id": "ai-written-check-r1", "text": "回覆逐項列出發現，每項含規則名稱、問題原文與具體改寫建議。"},
    {"id": "ai-written-check-r2", "text": "每一處「問題原文」逐字取自草稿，而非轉述。"},
    {"id": "ai-written-check-r3", "text": "回覆最後給出「乾淨／輕微修整／需大幅調整」三者之一的整體結論。"},
    {"id": "ai-written-check-r4", "text": "結論的理由連結到模式的出現次數或門檻，而非只給形容詞。"},
    {"id": "ai-written-check-r5", "text": "回覆未越界重寫整篇草稿，也未對語氣、自大感或事實正確性下判斷。"}
  ],
  "rubric": {
    "items": [
      {"id": "ai-written-check-r1", "weight": 3, "evidence_required": true,
       "text": "回覆必須逐項列出發現，每一項都齊備三個部分：規則或模式的名稱、引自草稿的問題原文、針對該處的具體改寫。引文請取其中結構最完整的一項。只有名稱沒有改寫、或只有改寫沒有指出原文者，該項不算數；一項都不齊備判 failed。"},
      {"id": "ai-written-check-r2", "weight": 3, "evidence_required": true,
       "text": "每一處被標為「問題原文」的文字，必須是草稿裡逐字存在的句子，不能是轉述或已經改寫過的版本——這個 Skill 的價值全在「指得出是哪一句」。引文請取一處問題原文。發現有任何一處是模型自行改寫後才引用的，判 failed 並以該處為引文。"},
      {"id": "ai-written-check-r3", "weight": 2, "evidence_required": true,
       "text": "回覆結尾必須給出整體結論，且落在套件文件定義的三個值之一：乾淨、輕微修整、需大幅調整（同義的中英措辭可接受）。引文請取該結論句。沒有結論、或給出三值以外的自創等第，判 failed。"},
      {"id": "ai-written-check-r4", "weight": 1, "evidence_required": true,
       "text": "結論必須說得出依據——引用某個模式的出現次數，或點名跨過了哪一條門檻。引文請取交代依據的那一句。只寫「整體看起來不錯」這類沒有指涉的評語判 failed。"},
      {"id": "ai-written-check-r5", "weight": 2, "evidence_required": false,
       "text": "本 Skill 的文件明載它只標記問題並提出改寫、由作者決定是否採用，且不判斷語氣、自大感或事實正確性。因此回覆不得直接交出一份重寫好的全文，也不得對草稿的語氣或事實真偽下判斷。判 failed 時，引文請取那段越界的整篇重寫或語氣／事實判斷；判 passed 時不需引文。"}
    ]
  }
}
```

### 4.4 `internal-comms`

依據：摘要載明它依溝通類型套用 `examples/` 對應指南的格式、語氣與內容蒐集方式，支援 3P 進度更新等文體；限制欄載明「若溝通類型不符合現有指南，需先取得對期望格式的澄清或更多背景資訊」。本測試的 Prompt 取該 Skill 的第一句任務範例句（3P 更新），故條目以 3P 為準；**換 Prompt 就要換這組條目**。

```json
{
  "criteria": [
    {"id": "internal-comms-r1", "text": "最終回覆含完整的 3P 更新成品全文。"},
    {"id": "internal-comms-r2", "text": "成品具備 3P 的三個區塊：進度、後續計畫、阻礙，且每塊都有內容。"},
    {"id": "internal-comms-r3", "text": "成品的事實全部來自草稿，沒有虛構的日期、數字、人名或專案。"},
    {"id": "internal-comms-r4", "text": "草稿資訊不足以填滿某一區塊時，回覆明說缺什麼，而非自行編造。"}
  ],
  "rubric": {
    "items": [
      {"id": "internal-comms-r1", "weight": 3, "evidence_required": true,
       "text": "最終回覆必須含 3P 更新的完整成品全文，而不是產出說明或檔名。引文請取成品的開頭一句。"},
      {"id": "internal-comms-r2", "weight": 3, "evidence_required": true,
       "text": "成品必須具備 3P 的三個區塊——進度（Progress）、後續計畫（Plans）、阻礙（Problems）——且三塊都有實際內容，不是留白的標題。標題用詞可在地化。引文請取其中一個區塊的標題與其第一項內容。缺任一區塊判 failed。"},
      {"id": "internal-comms-r3", "weight": 2, "evidence_required": true,
       "text": "成品裡的事實必須來自草稿：日期、數字、人名、專案名一律不得是模型補的。措辭與分段的改寫不算新增事實。判 passed 時，引文請取一句可回溯到草稿的內容；判 failed 時，引文請取那句虛構的內容。"},
      {"id": "internal-comms-r4", "weight": 1, "evidence_required": false,
       "text": "草稿的資訊不足以填滿某一區塊時，回覆必須說明缺的是什麼並要求補充——套件文件明載沒有相符指南或資訊時要回頭問，而不是自行補齊。若草稿資訊足夠、三區塊都填得出來，本條視為 passed。判 failed 的情形是資訊明顯不足卻沉默地編出內容，此時引文請取該處。"}
    ]
  }
}
```

### 4.5 `brand-guidelines`

依據：摘要載明它把 Anthropic 官方色彩與字體套用到視覺成品——Poppins 用於 24pt 以上標題、Lora 用於內文、橙／藍／綠循環處理非文字圖形，**指定字型不可用時自動改用 Arial 與 Georgia**；色彩透過 `python-pptx` 的 `RGBColor` 套用。

**這一份要先講一件事**：品牌套用的事實在產出檔案的位元組裡，而 Judge 結構上看不到 artifact 內容（§2.2）。所以下面四條判的是**這個 Run 對自己做了什麼的陳述**，加上一條可機械回驗的「宣稱的檔案真的在 manifest 上」——**不是**「.pptx 裡真的是 Poppins」。這個界線寫進條目文字，並記為 §6 的 G6，免得下次有人把它讀成別的意思。

```json
{
  "criteria": [
    {"id": "brand-guidelines-r1", "text": "最終回覆逐項說明套用了哪些品牌設定：標題字體、內文字體與色彩。"},
    {"id": "brand-guidelines-r2", "text": "回覆點名的字體與套件文件一致，或說明字型不可用而改用替代字體。"},
    {"id": "brand-guidelines-r3", "text": "回覆說明非文字圖形的強調色處理，或說明本次輸入沒有非文字圖形。"},
    {"id": "brand-guidelines-r4", "text": "回覆宣稱產出的每個檔案，都出現在本次 Run 的 artifact 清單上。"}
  ],
  "rubric": {
    "items": [
      {"id": "brand-guidelines-r1", "weight": 3, "evidence_required": true,
       "text": "最終回覆必須逐項說明這次套用了哪些品牌設定：標題字體、內文字體、以及套到哪些元素的色彩。引文請取說明這些設定的那一段。只寫「已套用品牌樣式」而不說套了什麼，判 failed。注意本條判的是回覆的陳述，不是產出檔案的位元組——後者不在本評估的可見範圍內。"},
      {"id": "brand-guidelines-r2", "weight": 3, "evidence_required": true,
       "text": "回覆點名的字體必須與套件文件一致：24pt 以上標題用 Poppins、內文用 Lora；若環境沒有這兩個字型，回覆必須說明已改用 Arial 與 Georgia。引文請取點名字體的那一句。換成文件沒有指定的第三種字體且未說明理由，判 failed。"},
      {"id": "brand-guidelines-r3", "weight": 2, "evidence_required": true,
       "text": "回覆必須說明非文字圖形如何以橙、藍、綠強調色循環處理；若本次輸入沒有非文字圖形，明說這件事同樣算符合。引文請取相關的那一句。兩者都沒有交代，判 failed。"},
      {"id": "brand-guidelines-r4", "weight": 2, "evidence_required": true,
       "text": "回覆宣稱產出的每一個檔案，都必須出現在本次 Run 的 artifact 清單上。請以 kind = artifact 的證據引用逐一指名路徑（平台會逐條回驗路徑是否在清單內）。宣稱了清單上沒有的檔案，判 failed。這是本份 rubric 中唯一由平台機械回驗的條目。"}
    ]
  }
}
```

---

## 5. 怎麼把它餵給 `/judge-run`

`rubric` 與 `criteria` 是 `JudgeRunRequest` 的兩個並列欄位，把 §4 的 JSON 兩個鍵直接展開即可：

```json
{
  "run_id": "<平台 run_id>",
  "evaluation_id": "<evaluation_id>",
  "skill": {"name": "humanizer", "summary": "<目錄摘要>"},
  "user_prompt": "<該 Run 快照的 Prompt>",
  "criteria": [{"id": "humanizer-r1", "text": "..."}],
  "rubric": {"items": [{"id": "humanizer-r1", "text": "...", "weight": 3, "evidence_required": true}]},
  "final_output": "<最終回覆>",
  "artifacts": [{"path": "...", "size_bytes": 0, "content_type": ""}],
  "trace_digest": {"complete": true, "entries": []},
  "truncation": []
}
```

`rubric` 全文計入輸入預算，設計 §6.3 對它的上限是「全文」（策展產物本來就短）。本文件五份 rubric 最長一份約 1.2K 字元，相對於單筆中位數 4.6K token 的輸入量可忽略。

### 5.1 [`report-judge-regression.md` §8.2](../m3/report-judge-regression.md) 建議 1 的重跑，實際上是兩輪不是一輪

| 模式 | 拿什麼跑 | 預期看到什麼 | 它補上 §2 哪一格 |
| --- | --- | --- | --- |
| **A：rubric 套既有基準 Run** | M2 那 5 筆 `writing` 基準 Run（Prompt 沒有 §3 第 4 條，最終回覆只有一行檔名說明） | **絕大多數條目應為 `undetermined`**——證據不在最終回覆裡 | 「證據殘缺下的保守性」（§6.3 記為零覆蓋：模型主動回 `undetermined` 的次數兩輪皆 0）。**A 輪若判出一片 `passed`，那才是壞消息** |
| **B：新跑 5 筆再評** | 依 §3 修訂 Prompt 後重跑的 5 筆 Run | 逐項有引文的實判定 | 「主觀的任務效果判定」（§2「沒測到」的第一格） |

**兩輪都要跑，順序不能顛倒**：A 便宜（不必發 Run，只多 5 次 Judge 呼叫，以 §7 的中位數估約 $0.07），而且它測的東西 B 測不到——B 的證據是完整的，永遠問不出「該說不知道的時候會不會說不知道」。

**harness 側需要的最小改動**（`tools/eval-regression/judge_regression.py`，**不在本文件的改動範圍**，屬 M3 第 7 批）：目前 `rubric_version` 寫死 `None`、`criteria` 一律取自快照的三條。需要一個 `--rubric <file>` 之類的入口，讀進 §4 的 JSON，把 `criteria` 併入請求、把 `rubric` 帶上、把 `rubric_version` 記成 `content-007/writing/v1`。**換 `rubric_version` 就是另一次回歸**（`02:EVAL-013` 第 3 條），結果照舊 append 進 `results.jsonl`，不覆寫。

> **2026-08-17 更新**：上述 harness 入口已實作（`--rubric`，輸入檔為 `tools/eval-regression/rubric-content-007-writing-v1.json`，內容逐字取自 §4），**A 輪已跑完**——結果與上表的預期**不一致**，`undetermined` 只有 13.6% 而非「絕大多數」。原因、代價與由此查出的 G7，見 [report-judge-regression.md §11](../m3/report-judge-regression.md) 與本文件 §2.2 的更正註記。**B 輪仍未跑**：它要重發 5 次真實 Run（沙箱＋映像＋閘道費用），屬 Run 批而非接線批。

---

## 6. 允收對照與缺口

### 6.1 `02:CONTENT-007` 五條

| 允收準則 | 狀態 |
| --- | --- |
| 每個精選 Skill 至少一組範例 Dataset、User Prompt 與驗收條件，且內容可散布 | ✅ 15/15，M2 已達成（[content-baseline-report.md §10](../m2/content-baseline-report.md)）。本文件為 `writing` 5 筆再加一組任務效果條件 |
| 範例 Prompt 必須明確點名該 Skill | ✅ 模板首句即點名；§3 新增的第 4 條不影響點名 |
| **`writing` 類的每個精選附一份可編輯 rubric，供 LLM Judge 逐項回傳證據引文** | ~~⚠️ 部分達成~~ **✅ 2026-08-17 達成**。5/5 有預設 rubric（§4，另以機器可讀形式落在 [`tools/eval-regression/rubric-content-007-writing-v1.json`](../../../../tools/eval-regression/rubric-content-007-writing-v1.json)）；「可編輯」由 `0026` ＋ `PATCH /test-cases/{id}` ＋ TestCases 詳情頁成立；「供 Judge 逐項回傳證據引文」由 `buildRequest` 送出 rubric 成立，並經 A 輪實測 22 個條目逐項回判定與引用（[report §11](../m3/report-judge-regression.md)）。**尚有 G7，見 §6.2** |
| 範例資料不得包含 Secrets、憑證或個資 | ✅ 沿用 M2 的合成 Dataset，未新增任何資料 |
| 實際執行時使用的 Prompt 與驗收條件以快照保存 | ✅ 既有機制（`test_case_snapshots` 不可變）。~~**但 rubric 進不了快照**，見 G1~~ **rubric 已一併凍結**（`0026`，並計入 `content_hash`；無 rubric 的快照雜湊與 `0026` 之前逐位元相同） |

### 6.2 缺口現況

| # | 缺口 | 狀態 | 誰接 |
| --- | --- | --- | --- |
| **G1** | **rubric 沒有儲存位置。** `test_cases` 與 `test_case_snapshots` 只有 `acceptance_criteria`，沒有 rubric 欄位 | **✅ 已關閉（2026-08-17）**：`db/migrations/0026_test_case_rubric.sql` 為兩表各加一個可為 NULL 的 `rubric jsonb`（形狀 `{version, items[]}`，CHECK 擋掉讀不回來的形狀）。不另建表——rubric 是驗收條件的加強形式，不是第二套機制（evaluation-design §6.4）。**刪掉某條驗收條件會連帶清掉指向它的 rubric 條目**，在同一交易內完成 | — |
| **G2** | **產品路徑上 rubric 到不了 Judge。** `buildRequest` 從不設 `req.Rubric`；`evaluation_started` 的 `rubric_version` 寫死 `nil` | **✅ 已關閉**：`buildRequest` 由快照讀 rubric 並只送出「id 對得上本次送出條件」的條目，對不上的丟棄並記 `warning` finding；`evaluation_started` 填快照的真值；`evaluations.rubric_version` 記**實際生效**的版本（全部條目都被丟棄＝沒有 rubric 生效，不記版本） | — |
| **G3** | **沒有編輯介面。** | **✅ 已關閉**：`apps/web` 的 Test Case 詳情頁新增 rubric 區塊。**版面是「一條驗收條件一格」而不是自由條目清單**——item 的 id 因此由結構決定，使用者不可能填錯 | — |
| **G4** | **回歸 harness 不吃 rubric。** | **✅ 已關閉**：`judge_regression.py --rubric <file>`，`rubric_version` 記真值；已用它跑完 A 輪（[report §11](../m3/report-judge-regression.md)） | — |
| **G5** | **Judge 讀不到 artifact 內容**，只有 manifest 列 | 仍開著（**但 §2.2 的更正註記縮小了它的影響**：trace 的 `tool_call` payload 是第三條可引用路徑） | 後 MVP：規則腿讀 artifact 文字，另立需求 ID |
| **G6** | **`brand-guidelines` 的四條判的是自述不是位元組。** | 仍開著。A 輪實測那四條有 3 條判 `failed`、引用的是那句只講檔名的最終回覆，**與本文件 §4.5 的設計本意相符** | 同 G5 |
| **G7** | **`artifact` 型引用的引文從來沒有被回驗。** `verify()` 只檢查路徑在 manifest 上，引文不比對（因為請求沒送位元組）。於是 `evidence_required: true` 的條目可以用一段**沒人驗過**的引文通過，而存進報告的 `excerpt` 是平台自己的 manifest 行 | **新開（A 輪查出）**。A 輪 9 筆 `passed` 有 6 筆只靠這種引用；逐筆追查確認**模型沒有捏造**（引文逐字存在於該 Run 的 trace），但**平台分不出「歸錯類」與「編的」**。處置兩選一見 [report §11.2](../m3/report-judge-regression.md)，**動的是 ADR-026 defence 3 的判準，需要拍板** | `04` 乙-13 |

**判斷（2026-08-17）**：允收準則第 3 條的三個要件——「每個精選都有」「可編輯」「供 Judge 逐項回傳證據引文」——**三個都成立**，五條允收全數符合，**`03` 的 CONTENT-007 改勾**。判斷理由與保留意見寫在 `03` 該項的註記裡，其中最需要被下一個人看見的是：**G7 不是 CONTENT-007 的缺口而是 EVAL-001／ADR-026 的**（它對任何有 `evidence_required` 的 rubric 一視同仁，與 rubric 內容無關），因此記在 `04` 而不是留著擋這一項。
