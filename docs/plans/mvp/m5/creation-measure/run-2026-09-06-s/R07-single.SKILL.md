---
name: python-docstring-google-filler
description: Fill in missing Python function docstrings in Google style. Use when you have Python code and need to add or repair docstrings for functions, methods, or modules without changing behavior.
---

## 目的

為 Python 程式中的函式、方法或模組補齊缺失的 docstring，並統一成 Google 風格。

## 適用時機

- 你收到一段 Python 程式碼，要補上沒有寫的 docstring。
- 你要把既有 docstring 改寫成 Google 風格。
- 你要補齊參數、回傳值、例外、屬性或簡短說明，但不能改動程式行為。

## 處理原則

1. 只根據程式碼本身推斷，不要臆測不存在的行為。
2. 若資訊不足，寫出保守、可驗證的描述；不要編造參數用途、回傳內容或例外條件。
3. 保持原始程式碼不變，只新增或修正文檔字串。
4. 以 Google 風格撰寫，必要時使用下列區塊：
   - `Args:`
   - `Returns:`
   - `Yields:`
   - `Raises:`
   - `Attributes:`
   - `Examples:`
5. 內容要簡潔、精準、與程式碼一致。

## 審查步驟

1. 找出目標是函式、方法、類別還是模組。
2. 讀取簽名與函式內部邏輯，確認：
   - 每個參數的名稱、型別線索與用途
   - 是否有預設值
   - 是否回傳值、回傳型態線索
   - 是否會 `raise` 例外
   - 是否為 generator / iterator（使用 `yield`）
   - 是否有副作用或重要前置條件
3. 檢查現有 docstring：
   - 若缺少，新增完整 docstring。
   - 若格式不是 Google 風格，改寫成 Google 風格。
   - 若已有部分內容，保留正確資訊並補齊缺漏。
4. 若是類別：
   - 說明類別用途。
   - 若有公開屬性，補 `Attributes:`。
   - 若有重要初始化行為，簡述即可。
5. 若是模組：
   - 說明模組用途。
   - 不要把模組 docstring 寫成操作手冊。

## Google 風格寫法

### 函式 docstring 基本結構

```python
"""簡短摘要。

更詳細說明（可選）。

Args:
    name (type): 說明。
    flag (bool, optional): 說明。預設為 True。

Returns:
    type: 說明。

Raises:
    ValueError: 何時會發生。
"""
```

### 寫作規則

- 第一行是 1 句摘要，使用現在式、第三人稱、簡短明確。
- 若需要補充，第二段再寫細節。
- `Args:` 依參數順序列出。
- 參數名稱要與函式簽名一致。
- 若型別可從程式碼明確看出，就寫；看不出來可省略型別，但內容要一致。
- 預設值只在必要時提及，例如 `optional` 或在描述中說明預設行為。
- `Returns:` 只在有明確回傳值時使用；若回傳 `None` 且這是預期行為，可省略或寫明。
- `Yields:` 用於 generator。
- `Raises:` 只列出程式碼中實際可能拋出的例外。
- 不要寫與程式碼無關的背景故事、使用教學或過度冗長的說明。

## 產出要求

- 只輸出要補上的 docstring 內容，或已修正後的完整 docstring。
- 不要改寫函式程式碼。
- 不要加入與 Google 風格無關的格式。
- 若無法可靠判斷某項資訊，寧可省略，也不要猜測。
