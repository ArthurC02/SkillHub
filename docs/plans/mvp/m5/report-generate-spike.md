# 生成品質基線（前置）：一次呼叫產得出什麼

- 日期：2026-08-23
- 依據：[ADR-046](../../../adr/ADR-046-generating-a-skill-from-a-task-description.md) 前期驗證 1／[`03` GEN-009](../../03-work-items.md) 的前三分之一
- Harness：[`apps/platform/internal/shared/skillpkg/generate_spike_test.go`](../../../../apps/platform/internal/shared/skillpkg/generate_spike_test.go)（env-gated，形狀照 `spec_census_test.go`）
- 模型：`gpt-5.6-sol`（flagship）。**不用 `gpt-5.6-terra`**——[litellm-config.yaml](../../../../infra/compose/litellm-config.yaml) 逐字寫它是「judge tier, **separated from the generating model**」，拿評審的模型去生成會把 ADR-026 想分開的兩件事併回去。
- 閘道實付：**$2.3684**（21 次呼叫＝20 次生成＋1 次連通測試），平均 **$0.113／次**

## 0. 這批要回答什麼

ADR-046 的決策 3 與決策 6 各壓了一個經驗假設，寫 ADR 當下**只有推理沒有數字**：

1. 模型單次呼叫產得出通過 `skillpkg.Validate` 的套件嗎？（決策 3 的「不做重試迴圈」靠這個）
2. 通過的那些是實質內容還是格式正確的空殼？（決策 6 的「唯一的門是驗證器」靠這個）

**兩個假設都沒有完全成立，而錯的方向比對的方向有用。**

## 1. 語料

20 段任務描述，三組：

| 組 | 數 | 來源 |
| --- | --- | --- |
| `in_category` | 12 | [gate-test/task-cards.md](../gate-test/task-cards.md) §2 的 DOC-1～4／WRI-1～4／DAT-1～4，**逐字**（含兩張英文卡） |
| `out_of_category` | 6 | 本批新寫：補習班錯題歸類、咖啡店排班、React 提交前自檢、慢性病用藥提醒、桌遊選擇、三方對帳。**刻意落在 `documents`／`writing`／`data` 三類之外**——`04` 乙-21 的前提①講的正是這種人 |
| `not_skill_shaped` | 2 | gate-test 的兩張干擾卡 X-1（日本七天機票＋租車）、X-2（正時皮帶）。**一個 Skill 本來就做不到這兩件事** |

生成端沒有經過 `apps/llm` 也沒有經過 Go 控制平面：這批量的是「模型產得出什麼」，不是端點寫得對不對——端點還不存在（`GEN-001`／`GEN-002` 未做）。prompt 逐字見 §7。

## 2. 結果

| 指標 | 數 |
| --- | --- |
| 呼叫 | 20 |
| **產出套件** | **19／20** |
| **通過 `skillpkg.Validate`** | **16／19（端到端 16／20 ＝ 80%）** |
| 阻擋 | 3 |
| 阻擋 code 分布 | `frontmatter-invalid-yaml` = 3（**全部同一個**） |
| 警告 code 分布 | `license-unknown` = 16、`undeclared-dependency` = 2 |
| **emitted `license` 欄位** | **0／19** |
| 產出多於一個檔案 | 8／19 |
| SKILL.md 本文長度 | 中位數約 3.6k runes（min 1107、max 8388） |
| **空殼** | **0／19** |

## 3. 四個發現

### 3.1 「通過率接近 100%」錯了，是 80%——但四次失敗沒有一次是「模型答不出來」

ADR-046 決策 6 的查證補記推論「阻擋級檢查全是結構性的，一個被告知格式的模型幾乎必定全數通過」。**實測 80%**，而四次失敗拆開來是兩種，兩種都不是能力問題：

**三次 `frontmatter-invalid-yaml`（DAT-1、DAT-4、DOC-3）——同一個手滑，逐字如下：**

```
'---\nname: csv-to-jsonl\n description: 將含有標題列的 CSV 轉成 JSON Lines…'
                        ↑ 第二個鍵前面多一個空格
```

frontmatter 的第一個鍵頂格，**第二個鍵前面多一個空格**，YAML 的 mapping 縮排就不一致了。三次都是這個形狀。**不是內容錯，是排版手滑**，而它撞的正是驗證器唯一擋得住的那一類東西。

**一次截斷（DOC-1）——`max_tokens=8000` 上限吃掉整個結果。** 那是全批最長的一次（會議大綱→Word，模型連 `.docx` 產生腳本一起寫），`finish_reason` 到頂、JSON 不完整、**什麼都沒有留下**。一個被截斷的生成不是「品質差的套件」，是零。

### 3.2 空殼沒有出現——而我用來抓空殼的那把尺是壞的，這件事要一起記

19 個套件本文中位數約 3.6k runes，最短的 DAT-1 也有 1107 runes 加一支 4135 字元的 Python 腳本。**沒有一個是「標題加待填欄位」那種形狀。**

placeholder 偵測器報了 2 個（DOC-4、OUT-3），**逐一看過，兩個都是偽陽性**：

- **OUT-3** 的 `TODO`／`FIXME` 是那個 Skill 的**題材**——它就是一個「提交前掃 staged code 找有沒有遺留 TODO」的 Skill。
- **DOC-4** 的 `<original-name>`／`<extension>` 是輸出檔名樣板的說明文字，不是沒填完的欄位。

**這把尺 0 真陽性、2 偽陽性。** 記在這裡是因為 `GEN-009` 的完整版還要用它——**照現在的樣子用會系統性高估空殼率**，而「有 placeholder 字樣」與「是空殼」本來就是兩件事。下一版要嘛改成人看，要嘛換一個真的量得到「這份指示可執行嗎」的指標。

### 3.3 最重要的一個：X-1 與 X-2 都通過了

兩張干擾卡——**一個 Skill 本來就做不到的兩件事**——各自產出了一個套件，**兩個都通過驗證**。X-2 的本文 8388 runes 是全批最長。

X-1 產出的東西叫 `japan-flight-car-planner`，它自己的 `compatibility` 欄位逐字寫著：

> 需要網路存取才能查詢即時航班、租車供應、價格、營業時間及入境規定；無法連網時只能提供查詢策略與待確認清單。

**而平台的 Sandbox 預設封鎖網路**（ADR-005／ADR-022，沙箱層 nftables default-deny）。所以這是一個：使用者付了錢、通過了每一道檢查、寫進了他的工作區、**而且在這個平台的執行環境裡結構性地跑不起來**的 Skill。

這就是 ADR-046 決策 6 那條補記說的「拒絕率接近 0，門幾乎不擋東西」**具體長什麼樣**。驗證器沒有任何一條能發現這件事，因為它不是格式問題。**`GEN-004` 那句「沒有經過任何人工檢視、沒有任何試跑證據」不是禮貌用語，它是這個產品對 X-1 唯一說得出口的話。**

### 3.4 Script 是常態，不是例外

8／19 產出多於一個檔案，內容是 Python 與 shell 腳本（`scripts/csv_to_jsonl.py`、`scripts/format_stock_sheet.py`、`scripts/scan-staged.sh`…）。其中 2 個帶 `undeclared-dependency` 警告。

**ADR-046 決策 6 的「生成的 Script 不享有任何寬待」不是理論條款**——第一批就有將近一半用得上它。

## 4. 對 ADR-046 的回寫

| 原文 | 實測 | 處置 |
| --- | --- | --- |
| 決策 6 補記：「通過率預期接近 100%」 | **80%** | 已回寫為實測值 |
| 待決策 2：「重試次數上限大概率是一個不存在的問題」 | **問題存在，但很便宜** | 已回寫：三次失敗是隨機縮排手滑不是系統性缺陷，**單次重試就夠**；但「平台要不要直接修掉那個空格」是政策不是 bug fix，留在待決策 |
| 決策 6：「門是語法門不是品質門」 | **證實，且比預期嚴重**（X-1／X-2 通過） | 已補上 X-1 這個具體案例 |
| （無）`max_tokens` 上限 | **1／20 被截斷** | 新增：`GEN-001` 需要一條關於輸出上限與截斷處置的準則 |
| （無）成本 | **$0.113／次** | 新增：生成比一次 mini 級試跑貴，ADR-028 的配額設計要知道 |

## 5. 這批**沒有**回答的

- **生成的 Skill 跑起來好不好用。** 要 Sandbox ＋ 評估管線，是 `GEN-009` 的第②③項。
- **人會不會留著它。** 要人（`GEN-009` 第④項），刻意不用機器指標頂替——同 `04` 丙-38 的既有紀律。
- **同一段描述重跑會不會穩定。** 每段只跑一次，`frontmatter-invalid-yaml` 的 3／19 是不是隨機，沒有第二次觀察可以佐證。**這是重試決策的直接輸入**，`GEN-009` 要補。
- **三類之外的六段是不是真的更難。** 6 段全部通過，但樣本太小；`out_of_category` 與 `in_category` 的通過率差異在這個 n 下不可分辨。

## 6. 邊界

- **每段描述只跑一次**，n=20，沒有一個數字帶信賴區間。
- 語料的 12 段來自閘門情境卡，那些卡是**照著目錄裡有的東西設計的**（有 gold skill）——拿它們當生成語料會偏向「這件事平台本來就做得到」。六段 `out_of_category` 是為了抵銷這個偏差而加的。
- 生成端**沒有走 `apps/llm` 也沒有走 Go**，所以這批不涵蓋端點的任何行為（逾時、取消、配額、稽核）。
- 閘道成本取自 LiteLLM 的 `/spend/logs`，篩 `openai/gpt-5.6-sol` 且日期為 2026-08-23 的 21 列。

## 7. 附錄：prompt 逐字

生成端腳本是一次性的，不進 repo（它呼叫的端點還不存在）。system prompt 逐字：

```
You generate a complete Agent Skill package from a task description.

Return ONLY a JSON object, no prose, no code fences:
{"files": [{"path": "SKILL.md", "content": "..."}, ...]}

Rules for SKILL.md:
- It MUST start with YAML frontmatter delimited by --- lines.
- Frontmatter may contain ONLY these fields: name, description, compatibility,
  metadata, allowed-tools. Any other field is invalid.
- Do NOT emit a `license` field under any circumstances.
- `name`: required, lowercase letters, digits and hyphens only, max 64 chars.
- `description`: required, max 1024 chars, states what the skill does and when
  to use it.
- `allowed-tools`: a single string if present, not a list.
- `metadata`: a map of string keys to string values if present.
- After the frontmatter, write the actual instructions the agent will follow.

Other files are optional. Paths must be relative and must not contain `..`.
Write a skill that would actually accomplish the task, not a template with
placeholders.
```

參數：`max_tokens=8000`，無 `response_format`，user message 是任務描述原文。

**`license` 那一條的遵守率是 19／19。** 但那只證明 prompt 有效，**不證明機制**——`03:GEN-001` 要求這條在 schema 上成立而不是靠 prompt，理由現在有了：一批沒違反不代表下一批不會，而這個欄位一旦寫進去，它佔用的是「已宣告」那個狀態（ADR-046 決策 5）。
