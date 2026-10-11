---
name: health-check-report-pickup-sms
description: 依使用者提供的領取日期、領取時間與自費項目費用，撰寫繁體中文健檢報告領取通知簡訊。當你需要一則最多 28 字元、只含簡訊本文的通知時使用。
---

## 撰寫步驟

1. 從使用者訊息中取得領取日期、領取時間及自費項目費用；只使用提供的資料，不查詢或補造資訊。
2. 將完整日期縮寫為月／日，例如將 `2026/01/05` 寫成 `1/5`。保留完整領取時間；以 24 小時制呈現。費用未附幣別時，在金額後加「元」。
3. 用最精簡、自然的繁體中文組成一則健檢報告領取通知。日期、完整時間及自費費用三項都必須保留，並讓文字清楚表達是領取健檢報告。可參考此格式與長度：`健檢報告1/5 09:00-17:00領取自費1200元`。
4. 輸出前逐字計數：每個字元都計入，包括空格、標點、英文字母、數字與符號。不得以排除空格的方式計數，也不得超過 28 字元。若超過，精簡連接用語，但不可刪除或改造日期、完整時間或費用。
5. 最終回覆只輸出一則簡訊本文；不加引號、說明文字或字數統計。

## Output check

Before answering, write the answer to `answer.txt`, run
`python scripts/check_output.py --max-chars 28 answer.txt`
from the directory holding this SKILL.md, and revise the file until it prints OK; then answer with the file's content and nothing else. When everything cannot fit, keep the limit and add one line after the answer saying what was left out.
