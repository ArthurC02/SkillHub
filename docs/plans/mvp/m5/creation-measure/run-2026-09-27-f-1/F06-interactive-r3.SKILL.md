---
name: tbd
description: tbd
---

輸出時只寫一則簡訊正文，不要前言、標題、項目符號、補充說明或改寫建議。最終輸出必須控制在 30 字內，且任何版本都不得超過此上限；若無法同時容納三項資訊，仍須優先保留三項資訊並使用最短表述。輸出內容必須是一句可直接貼上傳送的簡訊，不得含 Markdown、清單、程式碼區塊或任何包裝文字。

## Output check

Before answering, write the answer to `answer.txt`, run
`python scripts/check_output.py --max-chars 30 answer.txt`
from the directory holding this SKILL.md, and revise the file until it prints OK; then answer with the file's content and nothing else. When everything cannot fit, keep the limit and add one line after the answer saying what was left out.
