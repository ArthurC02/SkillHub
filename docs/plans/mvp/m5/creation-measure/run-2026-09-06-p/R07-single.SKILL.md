---
name: python-docstring-google-filler
description: Fill in missing Python function docstrings in Google style. Use when you are reviewing Python code and need to add or complete docstrings for functions, especially when the existing docstring is missing sections or is not in Google format.
---

## 目標

為 Python 函式補齊或改寫 docstring，並統一成 Google 風格。

## 適用情況

- 函式沒有 docstring。
- docstring 不完整，缺少 `Args`、`Returns`、`Raises` 等必要段落。
- docstring 有內容但不是 Google 風格，需要整理成一致格式。
- 需要根據函式簽名、型別註記、預設值與函式內容推斷說明。

## 處理步驟

1. 讀取函式定義、型別註記、預設值、裝飾器與函式內部邏輯。
2. 判斷函式用途：
   - 先看函式名稱與參數名稱。
   - 再看回傳值、例外、狀態變更、I/O、資料轉換。
   - 若現有 docstring 有部分內容，保留正確資訊並補齊缺漏。
3. 依 Google 風格撰寫或修正 docstring：
   - 第一行用一句話總結函式做什麼。
   - 空一行後，依需要加入段落。
   - 參數用 `Args:`。
   - 回傳值用 `Returns:`。
   - 例外用 `Raises:`。
   - 若有屬性、附註或範例，只有在函式確實需要時才加入。
4. 逐一對照函式簽名，確保每個參數都有說明：
   - 必填參數要寫清楚用途。
   - 可選參數要說明預設行為。
   - `*args`、`**kwargs` 要說明其內容與用途。
   - 若參數名稱已能清楚表意，說明要補充行為而不是重複名稱。
5. 根據實際程式行為決定是否需要 `Returns:`：
   - 有明確回傳值就寫。
   - 若只做副作用且回傳 `None`，可省略 `Returns:`，或明確寫 `None`，但要一致。
6. 根據程式中的 `raise`、驗證邏輯、外部呼叫失敗情況決定是否需要 `Raises:`。
7. 若函式有型別註記，docstring 不要重複寫型別，除非型別無法從簽名清楚看出或需要補充語意。
8. 保持簡潔、準確、可維護：
   - 不要猜測函式沒有證據支持的行為。
   - 不要加入與程式無關的說明。
   - 不要把實作細節寫得過度冗長。

## Google 風格格式

使用以下結構，依需要取用：

```python
"""Short summary.

Args:
    name: Description.
    other: Description.

Returns:
    Description of the return value.

Raises:
    ValueError: When ...
"""
```

## 判斷原則

- 若現有 docstring 已經是 Google 風格，只補缺少的段落或說明，不要重寫成不同風格。
- 若函式名稱含糊，優先根據程式邏輯推斷用途；仍不確定時，用保守、可驗證的描述。
- 若函式是私有輔助函式，也要寫清楚它在做什麼，但避免過度展開。
- 若函式是方法，說明參數時仍以方法簽名為準，必要時可提及 `self` 的作用，但通常不需要單獨描述 `self`。

## 輸出要求

- 只修改 docstring，不改函式邏輯。
- 保持原始程式碼風格與縮排。
- 若無法從提供的程式碼可靠判斷某段內容，寧可省略該段，也不要臆測。
