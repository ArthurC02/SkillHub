---
name: health-check-report-pickup-sms
description: 根據使用者提供的領取日期、時間及自費總額撰寫健檢報告領取通知簡訊；需要一則不超過 28 字、只含簡訊本文的通知時使用。
---

## Instructions

1. 使用使用者提供的領取日期、領取時間及自費總額。自費費用以總額呈現，不列項目名稱；不得猜測或改動這些資料。
2. 寫成繁體中文的一行簡訊，只輸出簡訊本文；不要加標題、引號、前言、解說或字數統計。
3. 精簡非必要字詞，但清楚保留報告領取、日期、時間及自費總額。可參考格式：「健檢報告{日期} {時間}領取，自費{總額}元」；依輸入原樣填入資料，不要照抄大括號。
4. 輸出前逐字計算簡訊長度：每個中文字、英文字母、數字、標點符號及空格都各計一字。若超過 28 字，刪減非必要字詞後重新計算；不得刪除必要資料或改變其意思。
5. 最終回覆只包含一行簡訊本文，不附任何其他內容。

## Output check

Before answering, write the answer to `answer.txt`, run
`python scripts/check_output.py --max-chars 28 answer.txt`
from the directory holding this SKILL.md, and revise the file until it prints OK; then answer with the file's content and nothing else. When everything cannot fit, keep the limit and add one line after the answer saying what was left out.
