---
name: kaiting-tongzhi-jianxun
description: 將使用者提供的案件資訊改寫成可直接傳送的開庭通知簡訊，適合在需要保留案號與下一次開庭日期時間、並以三句內精簡其他資訊時使用。
---

# 開庭通知簡訊

## 你要做的事
把使用者提供的案件資訊改寫成一則可直接傳送的開庭通知簡訊。

## 何時使用
當使用者要你產出開庭通知，且內容必須保留案號與下一次開庭日期時間、並限制在 3 句內時使用。

## 步驟
1. 讀取使用者提供的案件資訊。
2. 先找出必須保留的兩項：案號、下一次開庭日期時間。
3. 以繁體中文寫出簡潔正式的簡訊正文。
4. 正文最多 3 句；如果資訊太多而需要合併或省略，正文之外必須再輸出 1 句說明，清楚指出哪些資訊被合併或省略；這 1 句不計入正文 3 句上限。
5. 若使用者沒有指定語氣，預設為簡潔正式。
6. 輸出時分成兩段：先是正文，再是一句額外說明。

## 注意事項
- 不要漏掉案號。
- 不要漏掉下一次開庭日期時間。
- 正文句數不能超過 3 句。
- 額外說明句不算進正文上限。
- 若資訊不足以寫成完整通知，就直接使用已提供的資訊，不要自行補造案件內容。

## Output check

Before answering, write the answer to `answer.txt`, run
`python scripts/check_output.py --max-sentences 3 answer.txt`
from the directory holding this SKILL.md, and revise the file until it prints OK; then answer with the file's content and nothing else.