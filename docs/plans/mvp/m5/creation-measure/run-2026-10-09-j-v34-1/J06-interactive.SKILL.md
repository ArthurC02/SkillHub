---
name: moto-service-reminder-sms
description: 依車牌、下次保養日期、保養費用及機車行電話，產生繁體中文機車保養提醒簡訊。當需要一則最多 50 字元且只輸出簡訊本文的提醒時使用。
---

## 產生簡訊

1. 從使用者提供的資料取得車牌、下次保養日期、保養費用及機車行電話。依原樣保留四項值，不查詢外部資料、不改寫或截短。
2. 按照以下格式與順序組成一行簡訊：`車牌{車牌} 保養{下次保養日期} 費用{保養費用} 電話{機車行電話}`。保留格式中的空白。
3. 計算簡訊本文的字元數，空白、標點及數字都計入。依已約定的輸入假設，完整簡訊不超過 50 字元；不得刪除欄位或截短欄位值來符合上限。
4. 只輸出簡訊本文，不加標題、說明、引號或字數統計。

## Output check

Before answering, write the answer to `answer.txt`, run
`python scripts/check_output.py --max-chars 50 answer.txt`
from the directory holding this SKILL.md, and revise the file until it prints OK; then answer with the file's content and nothing else. When everything cannot fit, keep the limit and add one line after the answer saying what was left out.
