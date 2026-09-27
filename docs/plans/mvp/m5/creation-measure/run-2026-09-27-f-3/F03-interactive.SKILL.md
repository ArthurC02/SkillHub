---
name: book-order-message-organizer
description: 整理客人傳來的訂書留言，輸出訂購清單與書籍總金額；當留言提到運費時，將運費另外列出但不計入書籍總金額。
---

# 做什麼
把客人傳來的訂書留言整理成可直接使用的訂購摘要：逐項列出書名、數量、單價與小計，並計算「書籍總金額」（不含運費）。如果留言有提到運費，另外列出運費金額，但不要把運費算進書籍總金額。

# 什麼時候用
當你收到一段客人訂書文字，裡面可能混有多本書、數量、單價、運費或其他說法時，就用這個 Skill 先整理成清楚的明細。

# 步驟
1. 讀取客人留言，找出所有明確的書籍項目。
2. 對每個書籍項目，整理出：書名、數量、單價、小計。
3. 小計用 `數量 × 單價` 計算。
4. 把所有書籍小計加總，得到「書籍總金額」。
5. 如果留言中有運費，另外列出「運費」；不要把它加進書籍總金額。
6. 如果留言沒有提供某項必要資訊，就在輸出中明確寫出「未提供」，不要自行補值。
7. 如果留言內容彼此矛盾或無法同時成立，就直接指出矛盾，並保留無法安全判定的部分。
8. 輸出成適合直接貼給店內人員使用的整理結果，保留明細與總額。

## Scripts

Paths below are relative to the directory holding this SKILL.md: run each script from there (or by its full path) with the inputs taken from the message, and present what it prints; never work its result out yourself. `python <script> --help` lists its arguments.

- `python scripts/placeholder.py`
