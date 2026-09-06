---
name: python-docstring-google-style-filler
description: Fill in missing docstrings for Python functions using Google style. Use when you have Python code with incomplete or absent function docstrings and need concise, consistent documentation.
---

## 目標

為 Python 函式補齊缺少的 docstring，並統一成 Google 風格。

## 使用時機

當輸入是 Python 程式碼，且你需要：
- 為沒有 docstring 的函式新增說明
- 補完不完整的 docstring
- 將既有 docstring 改寫成 Google 風格

## 處理流程

1. **先找出所有需要處理的函式**
   - 包含一般函式、類別方法、`@staticmethod`、`@classmethod`、巢狀函式。
   - 不處理模組層級說明、類別 docstring，除非使用者明確要求。

2. **判斷每個函式的用途與介面**
   - 從函式名稱、參數名稱、型別註記、預設值、回傳值、例外、內部邏輯推斷用途。
   - 若資訊不足，保留保守描述，不要臆測細節。
   - 若無法可靠判斷，明確寫出「用途不明」或只描述可確定的行為。

3. **依 Google 風格補齊內容**
   - 使用三引號 docstring。
   - 內容順序通常為：
     - 一行摘要
     - 必要時的補充說明
     - `Args:`
     - `Returns:`
     - `Raises:`
     - `Yields:`（若為 generator）
     - `Examples:`（只有在原始程式已有或使用者要求時才加）
   - 每個區塊標題使用 Google 風格的英文標籤。
   - 參數名稱、型別、回傳型別、例外名稱要與程式碼一致。

4. **摘要句寫法**
   - 第一行用祈使或描述式短句，簡潔說明函式做什麼。
   - 盡量以動詞開頭，例如：`Parse...`、`Return...`、`Validate...`。
   - 不要把實作細節塞進摘要。

5. **Args 區塊**
   - 每個參數都要列出，順序與函式簽名一致。
   - 格式：`name: 說明。`
   - 若有型別註記，可在說明中自然提及，不必重複堆疊。
   - 對可選參數說明預設行為。
   - 對 `*args`、`**kwargs` 也要說明其用途。

6. **Returns / Yields 區塊**
   - 若函式有明確回傳值，寫 `Returns:`。
   - 若回傳 `None` 且用途明確，可省略 `Returns:`，但若函式有副作用且回傳值重要，仍可寫明。
   - 若是 generator，改用 `Yields:`。
   - 說明回傳內容、結構與條件，不要只寫型別名。

7. **Raises 區塊**
   - 只有在程式碼明確會拋出，或從邏輯上可確定會拋出的例外才寫。
   - 每個例外都要說明觸發條件。

8. **保持一致與精簡**
   - 不要改動函式實作。
   - 不要新增不存在的行為。
   - 不要把推測寫成事實。
   - 文字以清楚、簡潔、可維護為主。

## 編輯原則

- 若原 docstring 已有部分內容，保留正確資訊並補齊缺漏。
- 若原 docstring 與程式碼不一致，以程式碼為準，必要時修正描述。
- 若函式名稱或參數語意模糊，優先描述可直接觀察到的行為。
- 若專案已有既定 docstring 風格，盡量維持一致，但輸出仍以 Google 風格為準。

## 輸出要求

- 只輸出更新後的 docstring，或只輸出已補齊 docstring 的程式碼片段，依使用者要求而定。
- 不要額外解釋修改原因，除非使用者要求。
- 不要輸出與 docstring 無關的內容。
