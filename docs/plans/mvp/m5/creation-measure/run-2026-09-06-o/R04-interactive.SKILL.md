---
name: github-issue-severity-triage
description: 將使用者提供的 GitHub issue 文字依嚴重度分成三級，並為每一級各寫一句處理建議；當你需要快速整理 issue 優先順序時使用。
---

# GitHub issue severity triage

你會收到一段 GitHub issue 文字或 issue 清單。請把內容依嚴重度分成三級，並為每一級各寫一句處理建議。

## 執行原則
- 只根據輸入文字判斷。
- 使用者未提供的背景、優先順序依據、欄位名稱，一律視為 not given。
- 不要補充外部知識、不要查資料、不要自行假設專案情境。
- 直接輸出整理後的結果，不要解釋你的處理過程。

## 兩條必守規則
- use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.
- deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## 任務步驟
1. 讀取使用者提供的 GitHub issue 文字。
2. 依內容可見的嚴重程度線索，將每個 issue 分到三級嚴重度之一。
3. 為每一級撰寫一句處理建議。
4. 以清楚、可直接貼用的格式輸出結果。

## 分類與輸出要求
- 嚴重度只使用三級；若輸入明確指明其他名稱，就沿用輸入中的名稱；若未明確指明，使用「高／中／低」作為三級標示。
- 每一級都要有一行或一段對應的處理建議。
- 若某級在輸入中沒有對應 issue，仍要標示為 not given，而不是補造內容。
- 若輸入不足以判斷某個 issue 的嚴重度，寫 not given。
- 輸出格式要整潔，便於直接貼用到文件或回覆中。

## 失敗條件
- 不可要求使用者補資料，除非輸入本身缺少到無法完成。
- 不可輸出與輸入無關的內容。
- 不可把推測當成事實。