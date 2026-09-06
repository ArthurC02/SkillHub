---
name: flowchart-to-structured-summary
description: 將流程圖文字整理為結構化摘要，適合在需要保留節點、判斷、分支順序與不確定處時使用。
---

# 目的
將輸入的流程圖內容整理為四個區塊：`nodes`、`conditions`、`branches`、`uncertainties`。

# 執行規則
1. 先找出流程中的主要節點，依實際流程順序列成 `nodes`。
2. 再找出所有判斷節點，列成 `conditions`。
3. 接著把每個條件的「是 / 否」走向寫成 `branches`，並確保分支順序能對回 `nodes`。
4. 最後列出圖中沒有明講、但在整理時必須保留的假設或不確定處，放入 `uncertainties`。

# 輸出格式
請只輸出下列四個區塊，並使用清楚的條列或 JSON 皆可，但四個欄位都必須存在：

- nodes
- conditions
- branches
- uncertainties

# 寫作要求
- 保持原流程順序，不要自行改寫流程先後。
- 不要刪減圖中的關鍵步驟。
- 不要把判斷節點混進一般節點中；判斷節點要同時出現在 `conditions` 與對應的 `branches`。
- 若圖中有未明示的連接方式、回跳、或中間步驟，必須在 `uncertainties` 明確說出來。

# 如果輸入不足
若使用者提供的流程圖內容不足以判定節點、條件或分支，請直接指出缺少哪些資訊，並停止，不要自行補完。