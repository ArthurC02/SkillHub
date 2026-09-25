# 現行 Judge 回歸與重複性驗證

量測已完成。需求為 [EVAL-013](../../02-specifications-and-acceptance-criteria.md#eval-013judge-判準驗證m3)。本報告不改寫 [歷史量測](../m3/report-judge-regression.md)，也不把模型的讀值等同於真人對內容品質的評價。

結論：固定基準的 90 個計分條件中，88 個符合、0 個判錯、2 個無法判定；全體符合率與可判定覆蓋率均為 **97.78%**，可判定部分符合率為 **88／88**。同輸入重複時，模型計分答案沒有改變，但引用的變動使平台採信結果有 **6／40＝15%** 不一致；不能宣稱同條件必然可重現。

原始證據：[65 次主要呼叫](report-judge-regression-2026-09-26.jsonl)、[證據缺漏與引用失效](report-judge-incomplete-2026-09-26.jsonl)、[統計與檔案雜湊](report-judge-summary-2026-09-26.json)。每次主要呼叫包含 Run ID、evaluation ID、完整請求、模型原始回應、平台採信結果及成本；既有 [append-only 結果](../../../../tools/eval-regression/results.jsonl) 同時追加 65 列。

## 樣本與預先定義的量法

固定回歸集為 M2 的 45 筆原始 Run，不重新執行 Skill、不替換成新 Run。開跑前已逐筆比對歷史 D 輪的 Run ID 集合、產物數量與標註：45 筆均一致；43 筆有可讀封存，`date-wrangling` 與 `excel-scout` 的封存 404 與原始「未產出」標註一致。這不表示任意物件讀取失敗都可當作沒有產物。

| 面向 | 事前定義 |
| --- | --- |
| 完整基準 | 45 筆各評一次，c1 activation、c2 artifact 各有期望值，共 90 個計分條件 |
| 不計分條件 | c3 最終回覆沒有原始獨立標註，只列結果，不併入符合率 |
| 重複性 | `add-data-dictionary`、`docx`、`date-wrangling`、`excel-scout`、`pptx` 各合計 5 次，涵蓋正常、長證據、無產物及執行失敗但已有產物 |
| 相同輸入 | 同一份請求僅換 `evaluation_id`；其餘欄位的 SHA-256 必須相同 |
| 不一致率 | 每筆 Run 以第 1 次為基準，比較後 4 次的逐 criterion 判定；模型原始判定與平台採信後結果分開計算，並列出有波動的 Run 與條件 |
| 主要量測呼叫數 | 45 ＋ 5 × 4 ＝ 65，不因看到差異而挑選重跑樣本 |
| 模型與版本 | `gpt-5.6-terra`、`judge-run/v2`；rubric 為 null，沿用原始 Test Case 條件 |
| 採樣參數 | 要求 temperature 0、seed 20260829；閘道可能丟棄不支援的參數，不保證確定性 |
| 截斷預算 | final output 40000、criteria 20、每個 digest excerpt 8000、digest 100 筆、artifact manifest 500 列 |
| 費用 | 以歷史 D 輪每次 US$0.0194 估算 65 次約 US$1.26；短效 Virtual Key 上限 US$3，另有單次 US$0.10、累積 US$2.80 的本機停止警戒 |
| 保存 | 每次完成立即 append 原始請求、回應及採信結果；既有 `results.jsonl` 歷史列不覆寫 |

符合、判錯、模型無法判定、平台降級分列；無法判定不列為判錯。符合率同時交代全體覆蓋與可判定部分，避免以排除不確定值的分母營造高準確度。

## 量測工具與平台對帳

現行控制平面對單一 Trace excerpt 截斷採來源局部降級，不影響只引用完整來源的條件；整批遺漏仍保守降級。回歸工具原先沿用整批降級，且漏掉無可驗證引用的拒絕條件，本次已同步。模型自己回答 `undetermined` 時也改為獨立分類，不混入判錯。

免費測試共 16 條、0 failed、0 skipped。8000／8001 字元邊界、引用來源、passed／failed、整批截斷由決策表涵蓋。移除 Trace 完整性檢查、恢復整批截斷降級、停用缺引用拒絕時，3 條對應測試失敗；把模型無法判定重新歸為判錯時，分類測試失敗；各次恢復後全數通過。`gen:check` 確認四組生成物一致，`automation-check` 確認 50 個任務契約通過。遠端 CI 以提交的完整 SHA 另行查證，不把本機結果當作遠端證據。

## 已取得的量測證據

主要 65 次呼叫已完成；閘道資料庫依本次專用 key alias 對帳得到 65 筆、US$1.0498126，與逐筆 response header 回報的費用合計一致。三次受控證據缺漏呼叫另有 3 筆、US$0.0426412，總計 US$1.0924538。這些是兩組專用 key 的實付對帳，不宣稱閘道資料庫保存了每筆自訂 `evaluation_id` metadata；請求與回應配對證據另保存該識別碼及 `cost_source=gateway`。

45 筆首輪共 90 個可計分條件：88 個符合、0 個判錯、2 個無法判定。`date-wrangling` 的無產物判定因未提供可驗證引用而降級；`xlsx` 有一個條件未收到對應的回答。未計分的 c3 不納入上述分母。

額外三個 live 情境分別移除 Trace 事件並宣告缺漏、截斷最終輸出、截斷整批 digest；三者採信後都沒有 `passed`。另外把已取得的模型回應之引用替換為不存在的事件與文字，離線重放後所有條件均為 `undetermined/evidence_unverifiable`。離線故障注入不是模型抵抗 prompt injection 的證據。

兩次量測的 Python 服務均已停止、兩把短效 Virtual Key 均成功撤銷。Node 25 與 repo 要求的 Node 24 不符，doctor 仍屬失敗；本次 Python 回歸成功不會把這個環境缺口改寫為通過。

## 逐筆結果與差異歸因

標註來源及 c1／c2 轉換規則沿用歷史報告 §3；43 筆有產物、2 筆沒有產物的標註均可由現存 Trace、快照及封存索引重查。計分條件沒有相反判定，因此「Judge 判錯」0 筆、「標註本身可議」0 筆；兩個無法判定不硬塞進任一類：

- `date-wrangling` c2：模型判 failed，與標註一致，但没有可驗證引用；平台拒絕採信。後四次引用最終回覆等有效來源後得到 failed，差異來自引用，不是產物事實變了。
- `xlsx` c2：回應沒有與該 criterion ID 對應的結果，平台記 unanswered／undetermined；不是把未回答當作 failed，也不以另一個 ID 的答案代填。
- `pptx` c3：最終輸出為空；模型多次判 failed 卻無可驗證引用，故為 undetermined。c3 原本沒有獨立標註，不計分。

| Skill | c1 期望／採信 | c2 期望／採信 | c3（不計分） |
| --- | --- | --- | --- |
| add-data-dictionary | passed／passed | passed／passed | passed |
| add-iso3166 | passed／passed | passed／passed | passed |
| ai-written-check | passed／passed | passed／passed | passed |
| brand-guidelines | passed／passed | passed／passed | passed |
| copyright-creative-work | passed／passed | passed／passed | passed |
| course-quiz-builder | passed／passed | passed／passed | passed |
| cringe-check | passed／passed | passed／passed | passed |
| csv-to-json | passed／passed | passed／passed | passed |
| data-analyst | passed／passed | passed／passed | passed |
| data-cleanliness-scan | passed／passed | passed／passed | passed |
| data-comparability | passed／passed | passed／passed | passed |
| data-shape | passed／passed | passed／passed | passed |
| date-wrangling | passed／passed | failed／undetermined | failed |
| document-format-skills | passed／passed | passed／passed | passed |
| docx | passed／passed | passed／passed | passed |
| excel-date-to-text | passed／passed | passed／passed | passed |
| excel-deduplicate | passed／passed | passed／passed | passed |
| excel-delete | passed／passed | passed／passed | passed |
| excel-filter | passed／passed | passed／passed | passed |
| excel-find-duplicates | passed／passed | passed／passed | passed |
| excel-format | passed／passed | passed／passed | passed |
| excel-freeze | passed／passed | passed／passed | passed |
| excel-insert | passed／passed | passed／passed | passed |
| excel-mapping-replace | passed／passed | passed／passed | passed |
| excel-merge | passed／passed | passed／passed | passed |
| excel-regex-clean | passed／passed | passed／passed | passed |
| excel-scout | passed／passed | failed／failed | failed |
| excel-sort | passed／passed | passed／passed | passed |
| excel-split | passed／passed | passed／passed | passed |
| excel-validate | passed／passed | passed／passed | passed |
| full-review | passed／passed | passed／passed | passed |
| handoff | passed／passed | passed／passed | passed |
| humanizer | passed／passed | passed／passed | passed |
| internal-comms | passed／passed | passed／passed | passed |
| json-restructure | passed／passed | passed／passed | passed |
| line-edit | passed／passed | passed／passed | passed |
| pdf | passed／passed | passed／passed | passed |
| pii-flag | passed／passed | passed／passed | passed |
| pptx | passed／passed | passed／passed | undetermined |
| shorten | passed／passed | passed／passed | passed |
| sokrati | passed／passed | passed／passed | passed |
| standardise-country-names | passed／passed | passed／passed | passed |
| text-to-numeric | passed／passed | passed／passed | passed |
| unicode-consistency | passed／passed | passed／passed | passed |
| xlsx | passed／passed | passed／undetermined | passed |

## 重複性：同條件不代表同答案

5 筆各 5 次，共 20 個相對首輪的重複比較。已核對各 Run 的 5 個 request hash 相同；只排除每次不同的 evaluation ID。計分条件為 20 × 2＝40 個比較，含不計分 c3 則為 60 個。

| 範圍 | 模型原始判定不一致 | 平台採信判定不一致 |
| --- | --- | --- |
| 只看 c1／c2 | 0／40＝0% | 6／40＝15% |
| 包含未標註的 c3 | 5／60＝8.33% | 10／60＝16.67% |

所有變動均可歸因：`date-wrangling` c2 的第 2～5 次補上有效引用，產生 4 個採信差異；`pptx` c2 的第 3、5 次引用被截斷事件，產生 2 個採信差異。`excel-scout` c3 第 2～5 次把「說明沒有產物」解釋為符合，與第 1 次解釋不同，產生 4 個原始與採信差異；`pptx` c3 第 4 次沒有對應答案，產生 1 個原始差異，但採信結果仍為 undetermined。

這是固定 5 筆樣本的波動讀數，不是全產品失敗率。採樣參數已要求固定，仍無法消除引用選擇與語義解釋的波動；不為了提高分數放寬引用回驗，也不把未標註的 c3 波動追認為已證明的準確率問題。

## 可重跑邊界

先用 `judge_regression.py --dry-run` 查資料，再以原始證據中的 45 個 Run ID 逐筆傳給 `--run-id` 固定集合；改模型、prompt、rubric 或截斷規則即為新的量測，不覆蓋本次。正常服務經 LiteLLM，攜帶 LLM service token，使用有費用上限的 Virtual Key。要防中斷遺失，可每次 CLI 呼叫只評一個 `--run-id`，完成即追加結果，再依事前指定的五個 Run 各追加四次。

原始證據缺漏的控制是有意修改 Judge 收到的請求，不更改資料庫中的歷史 Run。核對時分別看模型原始回答與 `store()` 的保守採信；引用失效重放不呼叫模型。本報告證明的是既有標註的低階事實判準與保守性，不證明主觀內容品質、未量測模型或所有 prompt injection 都可靠。
