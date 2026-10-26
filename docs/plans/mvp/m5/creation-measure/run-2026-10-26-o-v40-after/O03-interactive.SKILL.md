---
name: credit-card-minimum-payment
description: 依照指定門檻試算單筆信用卡本期帳單的最低應繳金額，並將結果無條件進位至整數元。當使用者提供帳單金額並要求最低應繳試算時使用；本 Skill 不涵蓋銀行或發卡機構的其他費用、利息或個別規定。
---

# 信用卡最低應繳金額試算

依下列步驟處理使用者提供的本期帳單總額。每筆金額獨立試算；若同一帳單出現互相矛盾的金額，不替該帳單計算，並請提出請求的人確認。其他可獨立辨認的帳單照常試算。

1. 讀取每筆帳單總額原文。若未提供金額，直接交付可用的結果格式，將未提供的資料標明為「未提供」，不要追問或自行補值。不要改寫或省略已提供的數值。
2. 對每筆金額先執行平台提供的輸入檢查器，依 `scripts/input_schema.json` 檢查；後續只使用檢查後的值。它會解析常見單位及文字寫法，並列出解析假設；保留這些假設供結果一併呈現。若檢查器拒絕某值，指出原值和拒絕原因；對零或負值提示「帳單總額須大於 0，請重新輸入」。不要用未通過檢查的值計算。
3. 對同一帳單的金額逐一比對原始紀錄。若有兩個不同值，明確列出兩值並請提出請求的人確認；略過該帳單，不把它的任一數值送去計算。不要因此略過其他獨立帳單。
4. 將每筆已檢查、為正數且無矛盾的金額以 `--amount` 傳入計算器；多筆時重複此參數。從放置本 SKILL.md 的目錄執行：
   `python scripts/minimum_payment.py --amount <已檢查金額> [--amount <已檢查金額> ...]`
   只傳入已檢查的數值，不要傳入未提供、被拒絕或有矛盾的帳單。若沒有可計算的金額，不執行計算器。
5. 將計算器印出的每一行結果原樣呈現；同時保留輸入檢查器印出的假設。輸出簡潔，並清楚區分帳單。金額為零或負數時使用指定提示，不輸出最低應繳金額。
6. 這是依照本 Skill 規則所作的試算，不判斷銀行或發卡機構的其他費用、利息或個別規定；如需這些資訊，請由提出請求的人向相關機構確認。

## Input check (first step)

Before any other step, write every value the message gives into `input.json` as `{"records": [{<field name>: <the value exactly as written>}, ...]}`, one record per item, with the field names of `scripts/input_schema.json`, then run
`python scripts/check_input.py input.json`
from the directory holding this SKILL.md, adding `--today YYYY-MM-DD` when the message states today's date. Every later step and script takes its values only from `checked.json`. Copy every line it prints that starts with 假設：, 無法採用： or 未提供： into the answer word for word; compute nothing from a value it refused, and still give every result the other values settle.
