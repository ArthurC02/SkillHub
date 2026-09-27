---
name: bookstore-order-summary
description: 整理書店客人傳來的訂書留言，輸出結構化訂購清單並計算書籍總金額；當你需要把一段留言轉成可直接下單的明細時使用。
---

# Bookstore Order Summary

用這個 Skill 來整理客人傳來的訂書留言，輸出可直接使用的訂購明細。

## 何時使用
當輸入是一段或多段客人留言，且你需要：
- 逐項整理出書名、數量、單價與小計
- 計算書籍總金額
- 另外列出留言中提到的運費，但不把運費算進書籍總金額

## 處理步驟
1. 讀取客人留言全文，抓出每一本書的資訊。
2. 對每筆書籍資訊整理出：書名、數量、單價、小計。
3. 若留言中有提供數量與單價，先計算該筆小計。
4. 將所有書籍小計加總，得到書籍總金額。
5. 若留言中有提到運費，獨立列出運費金額；不要把運費加進書籍總金額。
6. 輸出整理好的訂購清單與金額摘要，讓使用者可直接查看。

## 輸出內容
輸出時必須包含：
- 書籍清單：每本書一列，包含書名、數量、單價、小計
- 書籍總金額：只加總書籍小計，不含運費
- 運費：只有在留言有提到時才另外列出

## 注意事項
- 不追查外部資料。
- 只根據客人留言中明確提供的內容整理。
- 運費一律獨立處理，不併入書籍總金額。
- 若留言同時包含多本書與運費，要兩者都保留。

## Scripts

Paths below are relative to the directory holding this SKILL.md: run each script from there (or by its full path) with the inputs taken from the message, and present what it prints; never work its result out yourself. `python <script> --help` lists its arguments.

- `python scripts/validate_order_summary.py`
