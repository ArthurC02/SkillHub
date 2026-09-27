---
name: repair-appointment-sms-confirmation
description: 將到府維修預約資訊改寫成一則可直接傳送的確認簡訊；適用於需要在極短字數內保留日期、時間與訂金金額的情況。
---

# 到府維修預約確認簡訊

當使用者提供到府維修的預約日期、預約時間與應收訂金金額時，產生一則可直接傳送的確認簡訊。

## 做法
1. 讀取使用者輸入，找出三項資訊：預約日期、預約時間、應收訂金金額。
2. 只輸出簡訊本文，不加標題、說明、字數統計、前言、項目符號或其他補充文字。
3. 先用最短且自然的中文把三項資訊串成一句話，並盡量保留使用者原本提供的數字與單位。
4. 檢查最終結果的總字數（含標點與英數字）是否超過 30 字。
5. 若超過 30 字，進一步刪除冗字、縮短措辭或改用更精簡的標點，直到符合限制。
6. 若使用者提供的內容不足以寫出三項資訊，直接回覆缺少的那一項或多項，且仍只輸出簡訊可用文字。

## 輸出要求
- 必須包含：預約日期、預約時間、應收訂金金額。
- 不得加入任何解釋文字。
- 不得加入字數統計。
- 最終輸出必須不超過 30 字。

## 執行規則
直接輸出符合上述限制的簡訊本文，不要附加任何額外說明。

## Output check

Before answering, write the answer to `answer.txt`, run
`python scripts/check_output.py --max-chars 30 answer.txt`
from the directory holding this SKILL.md, and revise the file until it prints OK; then answer with the file's content and nothing else. When everything cannot fit, keep the limit and add one line after the answer saying what was left out.
