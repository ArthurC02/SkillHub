---
name: python-docstring-google-fixer
description: 補齊並修正 Python 函式 docstring 為 Google 風格；當使用者貼上函式原始碼或函式片段、且需要補缺漏說明時使用。
---

# Python Docstring Google Fixer

當使用者提供 Python 函式原始碼或函式片段，並要求補齊、修正或整理 docstring 為 Google 風格時，使用這個 Skill。

## 任務

把輸入中的 Python 函式 docstring 補齊為 Google 風格，並只根據輸入本身可確認的資訊作答。

## 規則

- use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.
- deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.
- 只根據輸入中的程式碼與既有 docstring 內容修改。
- 不推測未提供的函式行為、參數意義、回傳內容或例外行為。
- 不改動非 docstring 的程式碼。
- 若輸入沒有 docstring，就產生一個新的 Google 風格 docstring。
- 若輸入已有完整且符合 Google 風格的 docstring，就保留其結構與內容，不任意改寫。
- 若某項資訊在輸入中找不到，就寫 `not given`。

## 輸出內容

- 預設只輸出修正版 docstring 本文。
- 如果使用者明確要求，才輸出完整函式。
- Google 風格區段應使用輸入能支持的標題，例如 `Args:`、`Returns:`、`Raises:`、`Examples:`。
- 區段、條列與縮排要符合 Google 風格。
- 若某個區段在輸入中沒有足夠資訊，不要自行補充內容；改寫成 `not given`。

## 作業方式

1. 讀取使用者提供的 Python 函式或函式片段。
2. 辨識既有 docstring 是否缺漏、格式不一致或為空白。
3. 只補齊輸入已支持的 docstring 內容。
4. 保留原始碼不變，除非使用者要求輸出完整函式。
5. 直接輸出完成的 docstring 或完整函式，不要附加分析說明。