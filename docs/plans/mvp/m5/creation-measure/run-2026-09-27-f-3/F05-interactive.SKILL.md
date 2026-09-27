---
name: court-hearing-notification-sms
description: 撰寫可直接發送的開庭通知簡訊；當使用者提供案件資訊時，用於產出最多 3 句的正文，且必含案號與下一次開庭日期時間，並在需要時另外附上一句說明省略或合併了哪些資訊。
---

# 開庭通知簡訊

## 用途
當使用者提供案件資訊時，產出一則可直接發送的開庭通知簡訊。正文最多 3 句話，且一定要包含案號與下一次開庭日期時間。

## 輸入判讀
1. 讀取使用者提供的案件資訊，找出可用欄位：案號、下一次開庭日期時間、地點、當事人、聯絡窗口、提醒事項、其他補充。
2. 若案號或下一次開庭日期時間沒有提供，就在輸出中明確標示「未提供」，不要自行補造。
3. 若資訊彼此衝突，保留衝突並在輸出中指出「資訊矛盾，請確認」，不要自行選定一個版本。

## 產出規則
1. 正文最多 3 句話。
2. 正文一定要明確包含案號。
3. 正文一定要明確包含下一次開庭日期時間。
4. 其他資訊若無法都塞進 3 句話，可省略或合併成較短表述。
5. 若有省略或合併，另外加上一句說明，交代省略或合併了哪些資訊；這一句不算在 3 句話上限內。
6. 不要加入未提供的細節，也不要為了湊句數而擴寫。
7. 若使用者沒有提供足夠資訊，仍要輸出可用草稿，並把缺少的資訊標示為「未提供」。

## 寫作優先順序
1. 先把一定要保留的內容放進前 3 句。
2. 優先順序：案號、下一次開庭日期時間、必要提醒、其他資訊。
3. 能合併的資訊合併成一句，例如把地點與提醒事項放在同一句。
4. 如果資訊太多，保留最重要的內容，並在額外說明列出被省略或合併的項目。

## 輸出要求
1. 直接輸出可發送的簡訊內容。
2. 若有額外說明，與正文分開成獨立一句。
3. 不要輸出分析過程。

## Output check

Before answering, write the answer to `answer.txt`, run
`python scripts/check_output.py --max-sentences 3 answer.txt`
from the directory holding this SKILL.md, and revise the file until it prints OK; then answer with the file's content and nothing else. When everything cannot fit, keep the limit and add one line after the answer saying what was left out.
