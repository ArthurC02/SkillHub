---
name: real-estate-commission-fee-calculator
description: Calculate a single real-estate brokerage commission fee from a transaction price using tiered percentage rates and rounding down to the nearest thousand. Use this skill when you need a ready-to-run fee calculator for one property transaction.
---

# 房仲成交服務費計算

你會收到一則使用者訊息，內容包含單筆成交價。請直接算出房仲成交服務費，並把最後金額回覆給使用者。

## 規則

1. 先讀出成交價。
2. 依下列級距選擇**單一**費率；不可累加各段：
   - 未滿 1000 萬元：1%
   - 1000 萬元以上、未滿 3000 萬元：1.5%
   - 3000 萬元以上：2%
3. 先依選到的費率計算服務費，再**無條件捨去到千元**。
4. 只輸出最後服務費金額。

## 你要怎麼做

- 直接從使用者訊息中找出成交價。
- 套用對應級距。
- 算出服務費後，向下捨去到最接近的 1000 元。
- 回覆最終金額，不要列出中間算式。

## 輸出格式

- 只輸出一個最終金額。
- 金額以元表示。
- 不要加說明文字。

## 注意

- 同一筆交易只套用一個級距。
- 不要做累進計算。
- 若使用者訊息沒有可辨識的成交價，請直接說明缺少成交價，並請對方提供可計算的數字。

## Scripts

Run each script below from this Skill's directory (the directory holding this SKILL.md) with the inputs taken from the message, and present what it prints; never work its result out yourself. `python <script> --help` lists its arguments.

- `python scripts/calc_commission.py`
