---
name: diao-fu-wei-xiu-yu-yue-que-ren-jian-xun
description: 將到府維修預約資訊改寫成一則繁體中文預約確認簡訊；當需要輸出極短、只保留日期、時間與應收訂金金額的訊息時使用。
---

# 到府維修預約確認簡訊

當使用者提供到府維修的預約日期、預約時間與應收訂金金額時，輸出一則可直接發送的繁體中文簡訊。

## 步驟
1. 讀取輸入中的三項資訊：預約日期、預約時間、應收訂金金額。
2. 只輸出簡訊本文，不加標題、說明、字數統計、項目符號或前後附言。
3. 簡訊必須包含三項資訊：日期、時間、訂金金額。
4. 以最精簡的寫法組句，讓總字數（含標點與英數字）不超過 30 字。
5. 若輸入同時提供三項資訊，優先保留三項必含資訊，不省略任何一項。
6. 若輸入缺少三項中的任一項，仍輸出最精簡的簡訊；缺少的資訊在輸出中保留為未提供，不自行補造。
7. 將最後簡訊直接作為答案輸出。

## Output check

Before answering, write the answer to `answer.txt`, run
`python scripts/check_output.py --max-chars 30 answer.txt`
from the directory holding this SKILL.md, and revise the file until it prints OK; then answer with the file's content and nothing else. When everything cannot fit, keep the limit and add one line after the answer saying what was left out.
