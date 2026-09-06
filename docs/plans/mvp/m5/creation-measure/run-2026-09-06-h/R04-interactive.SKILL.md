---
name: github-issue-severity-triage
description: 將 GitHub issue 依嚴重度分成三級，並為每級各寫一句處理建議；適合需要快速整理 issue 優先順序時使用。
---

# GitHub Issue Severity Triage

## Purpose
將使用者提供的 GitHub issue 清單依嚴重度分成三級，並為每一級各寫一句中文處理建議。

## How to run
1. 讀取輸入中的 GitHub issue 清單文字。
2. 只根據輸入內容判斷每個 issue 的嚴重度，不補充輸入中沒有提供的事實。
3. 將所有 issue 分配到以下三個等級之一：
   - 高
   - 中
   - 低
4. 為每個等級各寫一句中文處理建議。
5. 輸出時只使用這三個等級，不新增第四種分類，也不輸出其他嚴重度層級。

## Output requirements
- 必須明確包含三個嚴重度等級。
- 每個等級都要附上一句處理建議。
- 每個 issue 都必須被分配到三個等級中的其中一個。
- 處理建議必須是中文的一句話。
- 若輸入不足以判斷某個 issue，仍要在三個等級中做出最合理的分類，不要詢問使用者。