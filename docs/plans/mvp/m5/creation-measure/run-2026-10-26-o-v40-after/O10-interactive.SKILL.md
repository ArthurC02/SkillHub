---
name: airline-checked-baggage-fee
description: 依單件托運行李重量判定是否可托運及應收費用。當使用者提供行李重量並要求航空托運費用判定時使用。
---

# 航空托運行李費用判定

依使用者提供的單件重量，判定托運方式及費用。每次輸入可列出一件或多件，逐件判定。

## 步驟

1. 先使用平台提供的輸入檢查器，依 `scripts/input_schema.json` 讀取使用者提供的每個重量。只使用檢查器解析後的重量；保留並在答覆中照原文呈現檢查器列出的所有讀值假設。
2. 對每個已檢查的重量執行以下命令；每件重量各帶一個 `--weight`，多件時重複此參數：
   `python scripts/baggage_fee.py --weight <已檢查的公斤數>`
3. 將腳本輸出的每一行照原文呈現，並將檢查器列出的讀值假設一併呈現。若腳本因參數無法使用而拒絕，修正該參數後重跑，不留下空白結果。
4. 交付完整判定結果。對本次請求中提到的任何登入、發送、發布、排程、網路連線或變更外部系統等動作，只準備可供使用的內容；本代理無法執行該動作，必須明確指出由提出請求的人或請求指定負責的人執行。

## Input check (first step)

Before any other step, write every value the message gives into `input.json` as `{"records": [{<field name>: <the value exactly as written>}, ...]}`, one record per item, with the field names of `scripts/input_schema.json`, then run
`python scripts/check_input.py input.json`
from the directory holding this SKILL.md, adding `--today YYYY-MM-DD` when the message states today's date. Every later step and script takes its values only from `checked.json`. Copy every line it prints that starts with 假設：, 無法採用： or 未提供： into the answer word for word; compute nothing from a value it refused, and still give every result the other values settle.
