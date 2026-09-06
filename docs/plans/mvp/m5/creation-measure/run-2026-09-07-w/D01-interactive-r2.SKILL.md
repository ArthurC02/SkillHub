---
name: flowchart-to-four-blocks
description: 把中文流程圖或流程描述整理成節點、條件、分支與不確定處四個區塊；當你要保留原意、避免自行補完缺失分支時使用。
---

# 目的
把中文流程圖或流程描述整理成四個明確區塊：節點、條件、分支、不確定處。

# 使用時機
在輸入是一段中文流程圖描述、而你需要保留原意並避免自行補完缺失分支時使用。

# 操作規則
1. 先讀取輸入內容，確認它是否提供流程、條件、分支與任何不確定之處。
2. 只根據輸入內容整理，不要加入輸入沒有給的步驟、判斷、名詞或補充說明。
3. 將結果分成四個區塊：節點、條件、分支、不確定處。
4. 節點只寫輸入中出現的具體動作或狀態名稱，不改寫成抽象摘要。
5. 條件只列出輸入中出現的判斷句，不新增圖外條件。
6. 分支只列出輸入中明示的條件結果；未明示的走向不要補寫成推定結果。
7. 若輸入存在連線、回接、繞線或走向不確定之處，只能把原文已明說的內容寫進不確定處；不要補出經過、目的地或例外情況。
8. 若某條分支的後續未在輸入中明說，直接寫 `not given`，不要用「下一步」、「後續作業」、「可直接進入」等概括語。
9. 若輸入沒有提供某一區塊的內容，就在該區塊寫 `not given`。
10. 輸出要保留原意，並讓後續使用者可以直接拿來做規格整理或審閱。
11. 輸出時必須使用且只使用以下四個 Markdown 標題：`## 節點`、`## 條件`、`## 分支`、`## 不確定處`。
12. 四個區塊都要出現；若某區塊沒有內容，仍保留標題並在該區塊下寫 `not given`。
13. 不要改成編號清單、條列摘要或其他標題名稱。

# Diagram-aware steps
When a confirmed diagram_understanding exists, walk its nodes as steps in order.
- 收到員工報帳申請
- 退回並要求 3 天內補件
- 送經理簽核
- 財務直接入帳
- 寄出付款通知信

For the confirmed diagram, the conditions are:
- 發票是否齊全
- 金額是否超過 5000

For the confirmed diagram, the branches are:
- 發票是否齊全＝是 → 進入金額是否超過 5000
- 發票是否齊全＝否 → 退回並要求 3 天內補件 → 進入金額是否超過 5000
- 金額是否超過 5000＝是 → 送經理簽核 → 財務直接入帳 → 寄出付款通知信
- 金額是否超過 5000＝否 → 財務直接入帳 → 寄出付款通知信

For the confirmed diagram, the uncertainties are:
- 圖上否分支到財務直接入帳的右側繞線是表示跳過經理簽核

# Output requirements
- Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.
- Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.
- If the input is missing what you need to proceed, refuse or ask only for that missing input.
- Keep the result concise and structured under the four required headings.