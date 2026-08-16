# EVAL-013：Judge 判準回歸報告

- 日期：**2026-08-17**
- 對應需求：[`02:EVAL-013`](../02-specifications-and-acceptance-criteria.md)（Judge 判準驗證）；相關 `02:EVAL-001` 第 5 條、[ADR-025](../../../adr/ADR-025-run-terminal-state-and-evaluation-verdict-separation.md)、[ADR-026](../../../adr/ADR-026-evaluation-reassessment-evidence-lifetime-and-judge-trust-boundary.md)
- 回歸集：M2 的 **45 筆基準 Run**（[m2/content-baseline-report.md](../m2/content-baseline-report.md)）
- Harness：[`tools/eval-regression/judge_regression.py`](../../../../tools/eval-regression/judge_regression.py)，逐筆結果 append-only 落在同目錄的 `results.jsonl`
- 本次共兩輪回歸：**v1**（`judge-run/v1`）與 **v2**（`judge-run/v2`，v1 找出的缺陷修好後重跑）。兩輪都留在 `results.jsonl` 裡，前後並存可比（`02:EVAL-013` 第 4 條）

---

## 1. 一句話結論

**Judge 的判定本身在這 45 筆上沒有錯過一次（90／90 的模型原始判定全部與標註一致，兩輪皆然）；但第一輪有 45 筆正確判定被平台自己的證據回驗機制丟掉**——原因是 Judge 送出的引文逐字抄自 prompt 印給它的那一行，而 Go 只拿事件 payload 本身去比對，行首的 `事件型別: ` 前綴讓每一筆引用都對不上。修掉 prompt 的排版與措辭（`judge-run/v2`）後，第二輪 **90／90 全部一致、0 降級、0 undetermined**。

閘道實付 **$1.4906**（94 次呼叫，含 3 次前置煙霧測試），與服務層回報的每次呼叫成本合計 **$1.4905** 只差 $0.0001。

**但這個 100% 要照它的範圍讀，不能當成「Judge 準」的通稿**——它證明的東西比字面小，見第 2 節。

---

## 2. 這次回歸測到什麼、沒測到什麼

先講界線，因為一個 100% 很容易被後續文件引用成別的意思。

| 測到了 | 沒測到 |
| --- | --- |
| Judge 讀得懂 trace digest 與 artifact manifest，並在 45 筆上一次都沒有讀錯 | **主觀的任務效果判定**——這 45 筆的驗收條件裡沒有一條需要價值判斷 |
| Judge 不會被「Run 自稱成功」帶著走：2 筆 `succeeded` 但沒有產出的 Run，它都判 `failed` 並說明理由 | **Prompt Injection 的實際抵抗力**——這 45 筆的內容沒有一筆試圖操縱 Judge |
| Judge 產出的證據引用在 v2 下 **142 筆全部通過 Go 側回驗**（defence 3） | **證據不完整與截斷路徑**——45 筆的 trace 全部 `complete = true`、截斷預算一次都沒觸發，所以「降為 `undetermined`」這條規則在本回歸中只被 v1 的缺陷觸發過，沒有被它該處理的情境觸發過 |
| 兩輪之間的可比性：換 prompt 版本＝另一次回歸，兩份結論並存 | **rubric**：`CONTENT-007` 的 writing rubric 尚未存在，本回歸 `rubric_version = null`；rubric 補完後必須再跑一次 |

最重要的一句：**本回歸的兩條可計分驗收條件（見第 3 節），其正確答案平台自己用規則就算得出來。** 換句話說這是一個**下界檢查**——它能證偽「Judge 連讀得到的事實都會讀錯」，不能證實「Judge 判得動它真正存在的那一類問題」。之所以只能測到這裡，是因為 M2 的 45 筆標註本來就只標到這裡（第 3 節），而 `02:EVAL-013` 明文把「為回歸集另建一套標註工具」排除在範圍外。

---

## 3. Ground truth：那 45 筆到底標了什麼，怎麼轉成可餵 `/judge-run` 的形狀

### 3.1 原始標註是什麼

M2 基準報告 §3 的判定規則是**每個 Run 一個結論**，值域是「符合／未產出／失敗／restricted」，判定式為：

> 符合 ＝ Run 終態 `succeeded` **且** trace 有 `skill_activation` **且** artifact 封存內確有檔案

而每個 Run 的 Test Case 快照裡有**三條驗收條件**（45 份快照逐字相同，可在 `test_case_snapshots.acceptance_criteria` 重查）：

| 代號 | 條件原文 |
| --- | --- |
| c1 | `Run 的 trace 中出現對指定 Skill 的 skill_activation` |
| c2 | `/out/artifacts/ 中至少產出一個檔案` |
| c3 | `最終回覆說明了產出的檔案` |

**注意這兩件事不是同一件事**：判定式的三個條件是「終態、c1、c2」，**c3 不在裡面**。所以那 45 筆的標註是「相容性／符合判定」層級的，**不是逐 criterion 的 met／not_met**。

### 3.2 轉換規則（本節即 `02:EVAL-013` 第 1 條要求的「期望答案與其出處可追溯」）

**規則 1｜回歸集的成員是查出來的，不是抄出來的。**
`skill_runtime_compatibility` 的每一列都記著它是哪一個 Run 量出來的（`source_run_id`）。取「每個 Skill Version 最新一次量測」即得 45 筆，其中 41 筆落在 `2026.08-3`、4 筆（`docx`／`pdf`／`pptx`／`xlsx`）因授權受限停在 `2026.08-2`——與基準報告 §14.7 的表完全一致。集合定義是一段 SQL，成員資格因此可稽核。

**規則 2｜驗收條件取自該 Run 自己的快照，不新編。**
`runs.test_case_snapshot_id` → `acceptance_criteria`，三條原封不動送進 `criteria[]`。Judge 回答的就是使用者當初定的那三條。

**規則 3｜c1 與 c2 的期望答案，由平台自己的事實重新導出。**

| 條件 | 期望答案怎麼來 | 為什麼可追溯到標註 |
| --- | --- | --- |
| c1 | `trace_events` 中存在 `event_type = skill_activation` 且 `payload.decision = activated` 且 `payload.skill_name` ＝該 Skill → `passed`，否則 `failed` | 這就是判定式的第二個條件本身。基準報告 §5.3／§8 已記「45／45 的 trace 都有 `skill_activation`」 |
| c2 | 該 Run 的 artifact 封存內有 ≥1 個檔案 → `passed`，否則 `failed` | 這就是判定式的第三個條件本身。實測導出結果為 43 有檔案、2 無檔案（`date-wrangling`、`excel-scout`），與報告的「符合者有產出、未產出 2 筆」逐筆吻合 |

**規則 4｜c3 對映不了，因此不計分，但仍逐筆列出 Judge 的答案。**
判定式用「終態 `succeeded`」佔了第三個位置，**c3 從來沒有被獨立查證過**。要給它期望答案就得現在重新標註 45 份最終回覆——那是製造 ground truth 而不是使用 ground truth，本報告不做。c3 在逐筆表裡照列，但不進符合率。

> **對映不了的筆數，誠實計數**：135 條（45 × 3）中，**90 條有可追溯的期望答案，45 條（全部是 c3）沒有**。

**規則 5｜artifact manifest 從封存的 tar 讀，因為資料庫裡沒有。**
`artifacts` 資料表**全表 0 列**——M2 的管線沒有寫這些列。封存本身還在物件儲存（`run-artifacts/<run_id>/<attempt_id>/artifacts.tar`），harness 以 SigV4 讀它的**索引**（檔名、大小），不解壓、不依副檔名解析、不把任何位元組送進 prompt——這正是設計 §2.2 允許的「讀 manifest 不是執行」。2 筆無產出的 Run **連封存物件都不存在**（404），與「沒有檔案」一致。

### 3.3 一個必須點名的錯位：`pptx`

`pptx` 的 Run 判定是**失敗**（`2026.08-2`，PDM-005 的輸入 token 上限中止），但它 **c1／c2 的期望答案都是 `passed`**——工作負載在被中止前已經寫出 `q2_sales_update.pptx`，基準報告 §12.3 明文記著這件事。

這不是標註可議，是**兩個問題兩個答案**：`runs.status` 回答「這次執行發生了什麼」，驗收條件回答「這條做到了沒有」。ADR-025 把它們拆成兩欄的理由，在這一筆上是看得見的。Judge 兩輪都判 c1／c2 `passed`、c3 `failed`（最終回覆是空的），整體提案 `partially_met`——完全正確。

---

## 4. 這次回歸的參數（換其中任何一項就是另一次回歸）

| 項目 | v1 | v2 |
| --- | --- | --- |
| `regression_id` | `2026-08-16T174047Z` | `2026-08-16T174718Z` |
| `judge_model` | `gpt-5.6-terra` | `gpt-5.6-terra` |
| `judge_prompt_version` | `judge-run/v1` | **`judge-run/v2`** |
| `rubric_version` | `null`（`CONTENT-007` 未完成，無 rubric 可用） | `null` |
| 截斷設定 | `final_output` 40,000／`criteria` 20／digest 每筆 2,000 × 100 筆／artifact 500 列 | 同左 |
| 呼叫路徑 | 本機起 `services/llm` 的 uvicorn（`skillhub_llm.app:app`），harness 以 HTTP POST `/judge-run`；閘道為本機 LiteLLM，每次呼叫帶 `run_id`／`evaluation_id` metadata | 同左（另起 port 8010，避免與既有服務搶埠） |

**截斷在本回歸中一次都沒有發生**：45 筆的 trace digest 最多 55 筆事件（上限 100）、最終輸出遠短於 40,000 字元、`truncation` 全部為空陣列、`trace_digest.complete` 全部為 `true`。這既是好消息（基準 Run 的證據是完整的），也是限制（第 2 節已列）。

**評分的是「儲存後的判定」，不是模型的原始回答。** harness 逐條重做 Go 側的兩個降級（ADR-026 defence 3 的引用回驗、以及證據不完整／截斷時 `passed` 一律降 `undetermined`），因為那才是使用者會看到的東西。

---

## 5. 逐筆結果

`期望` 兩欄是第 3 節規則導出的答案；`c3` 欄是 Judge 的答案，不計分。成本為該次呼叫的閘道實付。

| Skill | 映像 | 期望 c1 | 期望 c2 | v1 c1 | v1 c2 | v2 c1 | v2 c2 | c3（v1／v2） | v1 $ | v2 $ |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| `add-data-dictionary` | 2026.08-3 | passed | passed | **undetermined** | passed | passed | passed | passed／passed | 0.0059 | 0.0057 |
| `add-iso3166` | 2026.08-3 | passed | passed | **undetermined** | passed | passed | passed | passed／passed | 0.0057 | 0.0134 |
| `ai-written-check` | 2026.08-3 | passed | passed | **undetermined** | passed | passed | passed | passed／passed | 0.0115 | 0.0110 |
| `brand-guidelines` | 2026.08-3 | passed | passed | **undetermined** | passed | passed | passed | passed／passed | 0.0187 | 0.0186 |
| `copyright-creative-work` | 2026.08-3 | passed | passed | **undetermined** | passed | passed | passed | passed／passed | 0.0177 | 0.0177 |
| `course-quiz-builder` | 2026.08-3 | passed | passed | **undetermined** | passed | passed | passed | passed／passed | 0.0274 | 0.0275 |
| `cringe-check` | 2026.08-3 | passed | passed | **undetermined** | passed | passed | passed | passed／passed | 0.0123 | 0.0123 |
| `csv-to-json` | 2026.08-3 | passed | passed | **undetermined** | passed | passed | passed | passed／passed | 0.0108 | 0.0109 |
| `data-analyst` | 2026.08-3 | passed | passed | **undetermined** | passed | passed | passed | passed／passed | 0.0113 | 0.0115 |
| `data-cleanliness-scan` | 2026.08-3 | passed | passed | **undetermined** | passed | passed | passed | passed／passed | 0.0191 | 0.0190 |
| `data-comparability` | 2026.08-3 | passed | passed | **undetermined** | passed | passed | passed | passed／passed | 0.0157 | 0.0156 |
| `data-shape` | 2026.08-3 | passed | passed | **undetermined** | passed | passed | passed | passed／passed | 0.0214 | 0.0213 |
| `date-wrangling` | 2026.08-3 | passed | **failed** | **undetermined** | **failed** | passed | **failed** | failed／failed | 0.0115 | 0.0114 |
| `document-format-skills` | 2026.08-3 | passed | passed | **undetermined** | passed | passed | passed | passed／passed | 0.0234 | 0.0233 |
| `docx` | 2026.08-2 | passed | passed | **undetermined** | passed | passed | passed | passed／passed | 0.0274 | 0.0274 |
| `excel-date-to-text` | 2026.08-3 | passed | passed | **undetermined** | passed | passed | passed | passed／passed | 0.0141 | 0.0139 |
| `excel-deduplicate` | 2026.08-3 | passed | passed | **undetermined** | passed | passed | passed | passed／passed | 0.0127 | 0.0125 |
| `excel-delete` | 2026.08-3 | passed | passed | **undetermined** | passed | passed | passed | passed／passed | 0.0143 | 0.0143 |
| `excel-filter` | 2026.08-3 | passed | passed | **undetermined** | passed | passed | passed | passed／passed | 0.0161 | 0.0160 |
| `excel-find-duplicates` | 2026.08-3 | passed | passed | **undetermined** | passed | passed | passed | passed／passed | 0.0146 | 0.0146 |
| `excel-format` | 2026.08-3 | passed | passed | **undetermined** | passed | passed | passed | passed／passed | 0.0124 | 0.0125 |
| `excel-freeze` | 2026.08-3 | passed | passed | **undetermined** | passed | passed | passed | passed／passed | 0.0118 | 0.0119 |
| `excel-insert` | 2026.08-3 | passed | passed | **undetermined** | passed | passed | passed | passed／passed | 0.0132 | 0.0131 |
| `excel-mapping-replace` | 2026.08-3 | passed | passed | **undetermined** | passed | passed | passed | passed／passed | 0.0133 | 0.0133 |
| `excel-merge` | 2026.08-3 | passed | passed | **undetermined** | passed | passed | passed | passed／passed | 0.0107 | 0.0107 |
| `excel-regex-clean` | 2026.08-3 | passed | passed | **undetermined** | passed | passed | passed | passed／passed | 0.0150 | 0.0149 |
| `excel-scout` | 2026.08-3 | passed | **failed** | **undetermined** | **failed** | passed | **failed** | failed／**passed** | 0.0128 | 0.0155 |
| `excel-sort` | 2026.08-3 | passed | passed | **undetermined** | passed | passed | passed | passed／passed | 0.0172 | 0.0170 |
| `excel-split` | 2026.08-3 | passed | passed | **undetermined** | passed | passed | passed | passed／passed | 0.0123 | 0.0124 |
| `excel-validate` | 2026.08-3 | passed | passed | **undetermined** | passed | passed | passed | passed／passed | 0.0130 | 0.0131 |
| `full-review` | 2026.08-3 | passed | passed | **undetermined** | passed | passed | passed | passed／passed | 0.0181 | 0.0180 |
| `handoff` | 2026.08-3 | passed | passed | **undetermined** | passed | passed | passed | passed／passed | 0.0120 | 0.0119 |
| `humanizer` | 2026.08-3 | passed | passed | **undetermined** | passed | passed | passed | passed／passed | 0.0115 | 0.0127 |
| `internal-comms` | 2026.08-3 | passed | passed | **undetermined** | passed | passed | passed | passed／passed | 0.0132 | 0.0131 |
| `json-restructure` | 2026.08-3 | passed | passed | **undetermined** | passed | passed | passed | passed／passed | 0.0125 | 0.0127 |
| `line-edit` | 2026.08-3 | passed | passed | **undetermined** | passed | passed | passed | passed／passed | 0.0110 | 0.0110 |
| `pdf` | 2026.08-2 | passed | passed | **undetermined** | passed | passed | passed | passed／passed | 0.0234 | 0.0233 |
| `pii-flag` | 2026.08-3 | passed | passed | **undetermined** | passed | passed | passed | passed／passed | 0.0264 | 0.0262 |
| `pptx` | 2026.08-2 | passed | passed | **undetermined** | passed | passed | passed | failed／failed | 0.0405 | 0.0404 |
| `shorten` | 2026.08-3 | passed | passed | **undetermined** | passed | passed | passed | passed／passed | 0.0118 | 0.0117 |
| `sokrati` | 2026.08-3 | passed | passed | **undetermined** | passed | passed | passed | passed／passed | 0.0182 | 0.0208 |
| `standardise-country-names` | 2026.08-3 | passed | passed | **undetermined** | passed | passed | passed | passed／passed | 0.0136 | 0.0134 |
| `text-to-numeric` | 2026.08-3 | passed | passed | **undetermined** | passed | passed | passed | passed／passed | 0.0157 | 0.0158 |
| `unicode-consistency` | 2026.08-3 | passed | passed | **undetermined** | passed | passed | passed | passed／passed | 0.0144 | 0.0146 |
| `xlsx` | 2026.08-2 | passed | passed | **undetermined** | passed | passed | passed | passed／passed | 0.0263 | 0.0263 |

### 5.1 彙總

| 指標 | v1 | v2 |
| --- | --- | --- |
| 可計分條目 | 90（45 筆 × c1、c2） | 90 |
| **一致（符合率）** | **45 ／ 90 ＝ 50.0%** | **90 ／ 90 ＝ 100.0%** |
| **判錯（mismatch）** | **0** | **0** |
| **降為 `undetermined`（不計入判錯）** | **45**（全部是 c1，全部因引用回驗失敗） | **0** |
| 模型原始判定與期望一致 | **90 ／ 90** | **90 ／ 90** |
| 證據引用通過 Go 側回驗 | 97 ／ 142 | **142 ／ 142** |
| 不計分的 c3 | 42 passed、3 failed | 43 passed、2 failed |
| 模型的 `overall` 提案 | met 42、partially_met 3 | met 42、partially_met 3 |

---

## 6. 差異歸因

`02:EVAL-013` 要求每筆差異歸類為「Judge 判錯」或「標註本身可議」。本回歸的結果是**兩類都是 0**，差異全部落在第三類——**平台自己的缺陷**，所以另立一欄誠實命名。

| 類別 | 筆數 | 說明 |
| --- | --- | --- |
| Judge 判錯 | **0** | 兩輪 180 條可計分判定，模型的原始答案沒有一條與期望不符 |
| 標註本身可議 | **0** | c1／c2 的期望答案是平台自己的事實重新導出的，與 M2 判定逐筆吻合，沒有一筆需要爭論 |
| **平台缺陷（v1）** | **45** | 見 §6.1 |
| 無標註可比（c3） | 45 條 | 見 §3.2 規則 4；不計入上列任何一類 |

### 6.1 v1 的 45 筆全數降級：引用格式與回驗基準對不上

**現象**：v1 的每一筆 c1，模型都判 `passed` 且引用了正確的 `trace_event_id`，但 `quote` 一律以 `skill_activation: ` 開頭——**45／45 完全同一個形態**。Go 側的 `verify()` 是拿事件 payload 本身做子字串比對，前綴讓每一筆都比不到，於是每一筆都被 defence 3 降為 `undetermined`。

**根因不在模型。** `services/llm` 的 digest 排版把 header 與 payload 印在同一行：

```text
- [<event_id> @ <時間>] skill_activation: {"decision": "activated", "skill_name": "…"}
```

而 prompt 只說「`quote` 是從該來源逐字複製的文字」。對模型而言那一整行就是「該來源」，它照做了，做得很正確。**是排版與措辭讓「來源」的邊界不存在，不是模型抄錯。**

**修法（`judge-run/v2`）**：header 與 payload 拆成兩行，payload 獨佔一行；prompt 的 `trace_event` 那一項補明「引文必須來自該事件本身的內容——它 header 底下那一行縮排的文字——`[id @ 時間] 型別` 是平台給事件的標籤，不是事件的一部分，引文含到它就什麼都比不上」。

改在 `services/llm/src/skillhub_llm/evaluate.py`，prompt 版本**升版不就地改寫**（v1 產生的那 45 筆判定必須還認得出是誰寫的），並補一個具名測試鎖住「payload 行上不得出現 id 或型別」。

**這件事的意義大於它的修法**：四條防線的第 3 條（Go 逐條回驗引用）是**會誤傷的**，而它誤傷的時候看起來和「Judge 沒把握」一模一樣——都是 `undetermined`。如果沒有這次回歸，線上會呈現的是「Judge 對啟用類條件永遠說不知道」，而每一份判定的 `reason` 都寫著正確答案。**這正是 `02:EVAL-013` 存在的理由，而它在第一次執行就付清了自己的成本。**

### 6.2 c3 的兩輪不一致（不計分，但值得記）

`excel-scout` 的 c3 在 v1 判 `failed`、v2 判 `passed`：該 Run 的最終回覆明說「本次僅完成勘察，未寫入 `/out/artifacts/`」，而條件是「最終回覆說明了產出的檔案」。「說明了沒有產出」算不算「說明了產出」，兩種讀法都成立——**這正是一條需要價值判斷的條件長什麼樣子，也正是本回歸沒有 ground truth 可以評分的那一類。** 其餘 44 筆的 c3 兩輪一致。

一條沒有標註的條件出現兩輪不一致，說明的是**條件寫得含糊**（`TEST-012` 的介面要讓使用者寫得出更好的條件），不是 Judge 不穩。

### 6.3 undetermined 與判錯分開計數（`02:EVAL-013` 第 5 條）

| 輪次 | 判錯 | `undetermined`（安全預設） | 其中因引用回驗失敗 | 其中因證據不完整／截斷 |
| --- | --- | --- | --- | --- |
| v1 | 0 | **45** | 45 | 0 |
| v2 | 0 | **0** | 0 | 0 |

模型自己主動回 `undetermined` 的次數：**兩輪皆 0**。這一點要中性看待——本回歸的證據全部完整，沒有任何一筆該回 `undetermined`，所以「它會不會在該說不知道的時候說不知道」**本回歸沒有測到**。

---

## 7. 成本

| 項目 | 數值 |
| --- | --- |
| v1 45 次呼叫合計 | **$0.7119**（中位數 $0.0136、最低 $0.0057、最高 $0.0405） |
| v2 45 次呼叫合計 | **$0.7246**（中位數 $0.0139、最低 $0.0057、最高 $0.0404） |
| 前置煙霧測試 3 次 | $0.0365 |
| **服務層回報合計** | **$1.4905** |
| **閘道 `LiteLLM_SpendLogs` 實際增量** | **$1.4906**（94 次呼叫，與 harness 的呼叫次數相同） |
| 差額 | **$0.0001** |
| 輸入 token（v2 45 筆） | 251,330（中位數 4,624／筆，最大 15,563） |
| 輸出 token（v2 45 筆） | 18,975 |
| 單筆超過 $0.10 警戒線者 | **0**（最高 $0.0405） |

**設計 §6.3 的預估要回填。** 該節寫「單次評估約 $0.01–0.05、45 筆全量約 $0.5–2」，並註明是首發預設值非實測校準值。**實測落在區間內**：單次中位數 $0.0139、全量 $0.72。輸入 token 的實際量體（中位數 4.6K）也低於「5–20K token」估計的中段，因為只有任務效果一類上模型、且 artifact 只送 manifest 不送內容。

**成本歸因是可查的**：每次呼叫都帶 `run_id`／`evaluation_id` metadata，服務層回報值與閘道 per-key spend 差 $0.0001（浮點捨入），**權威來源仍是閘道**（ADR-017、丙-3）。這條路徑本身是本次一併補上的——`JudgeRunResponse.usage` 在此之前不存在於契約與 Python 端，而 Go 呼叫端（`internal/eval/judge.go`）早就在讀它了。

---

## 8. 對 Judge 可信度的結論與建議

### 8.1 結論

1. **在「讀得到的事實」這一層，`gpt-5.6-terra` 作為 Judge 是可用的。** 90 條可計分判定兩輪全對，沒有捏造過一個 `event_id`、一個路徑或一句引文（v2 的 142 筆引用全數通過回驗）。它也沒有被「Run 終態 `succeeded`」帶偏——2 筆自稱成功卻沒有產出的 Run，它照樣判 `failed`，而這正是 `EVAL-001` 存在的理由（丙-5）。
2. **可用的門檻不是這個 100% 給的。** 本回歸沒有測到主觀判定、注入抵抗、證據殘缺下的保守性。要說「Judge 可信」，還缺這三類的量測。
3. **四條防線的第 3 條會誤傷，而且誤傷得無聲。** v1 的形態是最壞的一種：判定正確、理由正確、引用的 id 正確，只有引文的邊界差一個前綴，結果整條被丟掉，且呈現出來與「Judge 沒把握」無法區分。**任何動到 digest 排版、prompt 措辭或 Go 側 `verify()` 的改動，都必須重跑本回歸**——這不是流程潔癖，這是它第一次跑就抓到的東西。

### 8.2 建議

| # | 建議 | 給誰 |
| --- | --- | --- |
| 1 | **`CONTENT-007` 的 rubric 補完後立刻重跑本回歸**（`rubric_version` 由 `null` 變成有值＝另一次回歸）。屆時 writing 類會第一次出現需要價值判斷的條目，是補上第 2 節「沒測到」第一格的機會 | M3 第 7 批 |
| 2 | **補一組注入樣本進回歸集**：現有 45 筆沒有一筆敵意內容，而 ADR-026 決策 3 的四條防線目前只有第 1、3 條有實測。樣本可合成（不需要真的跑 Run），但要與 45 筆分開統計 | M3 第 7 批或 M4 |
| 3 | **補一組證據殘缺樣本**：`trace_digest.complete = false` 與 `truncation` 非空的路徑目前零覆蓋，而它們決定「看不到全文卻判 `passed`」會不會發生 | 同上 |
| 4 | **Go 側 `verify()` 的失敗理由要能被使用者區分**：`evidence_unverifiable` 與模型自己說的 `undetermined` 在 UI 上必須看得出差別，否則 §6.1 那種缺陷在線上是隱形的 | 第 2／6 批（`internal/eval`、UI 文案） |
| 5 | **設計 §6.3 的成本預估回填實測值**（單次中位數 $0.0139、45 筆 $0.72），比照 O11Y-003 門檻值的處置 | 第 7 批收斂 |
| 6 | **c3 這類含糊條件是 `TEST-012` 的輸入**：§6.2 顯示條件本身寫得含糊時，兩輪會給出不同答案；介面應鼓勵可判定的措辭 | 第 6 批 |

---

## 9. 允收對照（`02:EVAL-013`）

| 允收準則 | 狀態 |
| --- | --- |
| 存在一組固定的回歸集＝M2 的 45 筆基準 Run；期望答案與其出處可追溯 | ✅ 集合由 `skill_runtime_compatibility` 的 `source_run_id` 查出（41 筆 `2026.08-3` ＋ 4 筆 `2026.08-2`），與基準報告 §14.7 逐列一致；轉換規則見 §3，逐筆導出結果與報告的判定吻合 |
| 逐筆可比對，產出符合率、逐筆差異清單、每筆差異歸因「Judge 判錯」或「標註可議」；不得只給總分 | ✅ §5 逐筆表、§5.1 符合率、§6 歸因表。**誠實補述**：135 條中 90 條可計分、45 條（c3）無標註可比，已逐條列出而非湊數；本輪差異既非 Judge 判錯也非標註可議，另立「平台缺陷」一類並具名根因 |
| 記錄 `judge_model`／`judge_prompt_version`／`rubric_version`／截斷設定；換其一即另一次回歸 | ✅ §4；四項逐筆寫進 `results.jsonl` 的每一列（`rubric_version` 為 `null`，因 rubric 尚不存在，理由已記） |
| Judge prompt 或 rubric 升版後必須重跑；重評 append-only，前後兩次並存可比 | ✅ v1 → v2 就是這條的實作演練：prompt 升版後全量重跑，兩輪同存於 `results.jsonl`，`regression_id` 區分，舊列一個位元組未改 |
| `undetermined` 與判錯分開計數，且逐筆列出 | ✅ §6.3 與 §5 逐筆表 |
| 回歸成本可估並記錄實付（走閘道、帶 `evaluation_id` metadata） | ✅ §7；閘道實付 $1.4906，與服務層回報差 $0.0001 |

→ **勾選 `03` 的 `EVAL-013`。** 六條允收準則全部滿足且有可重跑的產出。

**勾選同時要講清楚它不代表什麼**：它代表「回歸集與驗證流程建立起來、跑過兩輪、抓到並修掉一個真缺陷」，**不代表「Judge 的判準已被全面驗證」**——第 2 節的三個空格與 §8.2 的建議 1～3 是後續工作，記在此處供第 7 批收斂時取用。

---

## 10. 怎麼重跑

```bash
# 前置：postgres 與 litellm 容器在跑；起 services/llm
cd services/llm
LITELLM_BASE_URL=http://127.0.0.1:4000 LITELLM_API_KEY=<master key> \
  .venv/Scripts/python -m uvicorn skillhub_llm.app:app --host 127.0.0.1 --port 8010 --app-dir src

# 全量（45 次呼叫，約 $0.72）
python tools/eval-regression/judge_regression.py \
  --judge-url http://127.0.0.1:8010/judge-run --note "為什麼跑這一輪"

# 只組請求不呼叫模型，用來確認回歸集與期望答案沒有漂移
python tools/eval-regression/judge_regression.py --dry-run
```

結果**追加**到 `tools/eval-regression/results.jsonl`，一列一個 Run。**不要覆寫該檔**：`regression_id` 是區分兩輪的唯一依據，覆寫等於讓「尺變了」變成看不見的事（ADR-026 決策 1）。
