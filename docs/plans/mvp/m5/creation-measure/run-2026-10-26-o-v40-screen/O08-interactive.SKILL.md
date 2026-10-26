---
name: course-waitlist-sms
description: 依學員姓名、課程名稱、開課日期、回覆期限及請求中提供的今天日期，撰寫課程候補遞補通知簡訊。當你需要產生完整且不超過 60 字的通知時使用；回覆期限過早或內容超長時，會說明原因而不產生簡訊。
---

## 用途與規則
依使用者提供的資料準備一則課程候補遞補通知簡訊。簡訊必須保留學員姓名、課程名稱、開課日期及回覆期限，全文（含標點、空白與數字）最多 60 字。不得為了符合字數限制而省略或改寫這些資料。回覆期限不得早於使用者提供的今天日期。

## 步驟
1. 先以 `scripts/input_schema.json` 為準，使用平台提供的輸入檢查器核對資料。只在後續步驟使用核對後的值；輸入檢查器列出的讀值假設也一併保留。若必填資料缺漏，提供可直接補填的簡訊格式，並明確標示哪些資料未提供，不要猜測。
2. 資料齊全時，從放有本 `SKILL.md` 的目錄執行：
   `python scripts/compose_sms.py --today <核對後的今天日期> --name <核對後的學員姓名> --course <核對後的課程名稱> --start-date <核對後的開課日期> --reply-deadline <核對後的回覆期限>`
   保留每個值的完整內容；含空白的值要用引號包住。腳本會比較日期並計算簡訊字數。
3. 若腳本輸出 `SMS\t`，取其後的簡訊全文寫入檔案，執行 `python scripts/check_output.py --max-chars 60 --require <姓名> --require <課程名稱> --require <開課日期> --require <回覆期限> <檔案路徑>`。若未印出 `OK`，依檢查結果修正並重跑檢查；成功後只回覆檔案中的簡訊全文。
4. 若腳本輸出 `ERROR\t`，將其後的原因寫入檔案，執行 `python scripts/check_output.py --max-chars 60 --require <原因核心文字> <檔案路徑>`。成功後只回覆檔案中的原因說明；不要產生簡訊。
5. 若輸入資料有值無法使用，指出是哪個值需要請求者確認；不要給出依賴該值的結果。其餘已能判定的內容照常處理。

缺少姓名、課程名稱、開課日期或回覆期限時，提供可用格式：`【候補遞補】[姓名]，[課程名稱]於[開課日期]開課，請於[回覆期限]前回覆。` 對缺漏欄位明確註明「未提供」。

任何本次請求提到、但此 Skill 無法代為執行的外部動作，都只準備好可供使用的內容；答案最後逐項用一行指出該動作無法由此 Skill 執行，並說明須由請求者或請求指定的負責人執行。

## Input check (first step)

Before any other step, write every value the message gives into `input.json` as `{"records": [{<field name>: <the value exactly as written>}, ...]}`, one record per item, with the field names of `scripts/input_schema.json`, then run
`python scripts/check_input.py input.json`
from the directory holding this SKILL.md, adding `--today YYYY-MM-DD` when the message states today's date. Every later step and script takes its values only from `checked.json`. Copy every line it prints that starts with 假設：, 無法採用： or 未提供： into the answer word for word; compute nothing from a value it refused, and still give every result the other values settle.
