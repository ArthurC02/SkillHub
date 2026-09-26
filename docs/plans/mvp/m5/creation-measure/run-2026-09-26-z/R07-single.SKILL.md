---
name: python-docstring-google-filler
description: Fill in missing Python function docstrings in Google style. Use when you have Python code and need to add or repair docstrings for functions, methods, or modules without changing behavior.
---

## 目標

為 Python 程式中的函式、方法或模組補齊缺少的 docstring，並統一成 Google 風格。只處理文件字串，不改動程式邏輯、參數名稱、回傳值或例外行為。

## 適用情況

- 函式沒有 docstring。
- docstring 不完整，缺少 `Args`、`Returns`、`Raises`、`Yields`、`Examples` 等必要段落。
- docstring 格式混亂，需要改成 Google 風格。
- 需要根據程式碼內容推斷參數、回傳值與例外。

## 處理步驟

1. 逐個檢查目標函式或方法。
2. 讀取函式簽名、型別註記、預設值、內部邏輯與 `raise` 敘述，推斷文件內容。
3. 依 Google 風格補齊或重寫 docstring。
4. 若原本已有部分內容，保留正確資訊，修正格式與缺漏。
5. 若資訊無法從程式碼確定，不要猜測；改寫成保守、可驗證的描述，或省略不確定的細節。

## Google 風格規則

### 1. 總述

- 第一行用一句話簡述函式用途。
- 若需要補充背景，可在空一行後加第二段說明。
- 避免冗長敘述，優先清楚、具體。

### 2. `Args`

- 依函式參數順序列出。
- 每個參數格式：`name: 說明。`
- 若有型別註記，可在說明中自然描述，不必重複型別名稱，除非有助理解。
- 說明參數用途、限制、單位、預期格式或特殊行為。
- 若參數未使用，不要硬寫用途；如確實存在但無意義，可簡短說明其保留原因。

### 3. `Returns`

- 函式有明確回傳值時使用。
- 說明回傳內容與條件，不只寫型別。
- 若回傳 `None` 且這是刻意設計，可省略 `Returns`，除非文件需要強調副作用。

### 4. `Yields`

- 若函式是 generator，使用 `Yields` 而不是 `Returns`。
- 說明每次產出的內容與產出條件。

### 5. `Raises`

- 只有在程式碼明確會拋出例外時才寫。
- 每個例外一行，說明觸發條件。
- 不要列出所有可能的 Python 內建例外，僅列出與函式行為相關者。

### 6. `Examples`

- 只有在程式碼或上下文已明確提供範例時才加入。
- 不要憑空編造使用範例。

## 判斷原則

- 以程式碼實際行為為準，不以函式名稱猜測。
- 若函式是簡單包裝器，docstring 應說明它包裝了什麼與差異。
- 若函式有副作用，需在總述或 `Returns` 前後說明。
- 若是屬性、setter、property 或特殊方法，依其實際用途撰寫簡潔說明。
- 若模組層級 docstring 缺失，簡述模組職責與主要內容。

## 輸出要求

- 只輸出修正後的 docstring 內容，或只輸出已補齊 docstring 的程式片段，依上游要求而定。
- 不要解釋你做了什麼。
- 不要改寫與 docstring 無關的程式碼。
- 不要加入與程式碼不符的資訊。
