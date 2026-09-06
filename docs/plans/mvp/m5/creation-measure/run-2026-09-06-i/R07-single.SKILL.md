---
name: python-docstring-google-filler
description: Fill in missing Python function docstrings in Google style. Use when you have Python code with incomplete or absent docstrings and need concise, consistent documentation for parameters, returns, raises, and behavior.
---

## 目的

為 Python 函式補齊或改寫 docstring，使用 Google 風格，並只補文件，不改程式邏輯。

## 適用情境

- 函式沒有 docstring。
- docstring 有缺漏，例如少了 `Args`、`Returns`、`Raises`、`Yields`、`Attributes` 或摘要說明。
- 需要把現有 docstring 統一成 Google 風格。

## 作業原則

1. 先讀函式本體、型別註記、預設值、例外處理與回傳路徑。
2. 只根據程式碼可直接推得的資訊撰寫，不要臆測未出現的行為。
3. 若資訊不足，保留保守描述，或明確寫出「若…則…」的條件式說明。
4. 不要改變函式名稱、參數、程式碼或註解內容，只處理 docstring。
5. 若專案已有既定 docstring 風格細節，優先維持一致；若沒有，使用標準 Google 風格。

## 判讀步驟

### 1. 先確認函式類型

- 一般函式：通常需要摘要、`Args`、`Returns`，必要時加 `Raises`。
- 產生器函式：使用 `Yields`，若也可能 `return` 結束值，需依實際程式碼判斷是否同時保留 `Returns`。
- 方法：若有 `self` 或 `cls`，通常不需要在 `Args` 詳述，除非其意義特殊。
- 只做副作用的函式：若沒有明確回傳值，寫 `Returns: None` 或省略回傳段落，依專案慣例一致處理。

### 2. 從程式碼整理資訊

對每個參數確認：

- 參數用途。
- 型別註記是否可直接引用。
- 是否有預設值與其意義。
- 是否為可選參數。
- 是否會被修改、驗證或轉換。

對回傳值確認：

- 回傳型別。
- 回傳內容代表什麼。
- 是否有多種回傳分支。

對例外確認：

- 明確 `raise` 的例外類型。
- 觸發條件。

對副作用確認：

- 是否會寫檔、發送請求、修改物件、更新狀態或印出內容。

### 3. 以 Google 風格撰寫

使用以下結構，依實際需要保留或刪除段落：

```python
"""簡短摘要句。

更詳細的說明，若需要可補充一到兩句。

Args:
    param1 (type): 說明。
    param2 (type, optional): 說明。預設為 ...。

Returns:
    type: 說明。

Raises:
    ValueError: 觸發條件。

Yields:
    type: 說明。
"""
```

## 撰寫規則

- 摘要句用現在式、主動語態、簡潔明確。
- 參數說明要寫「做什麼」，不要只是重複參數名稱。
- 若參數有預設值，說明預設值的效果。
- `optional` 只在參數可省略時使用。
- 型別若已在註記中清楚，docstring 仍可保留；若專案慣例偏好簡潔，也可省略，但要一致。
- 回傳說明要描述語意，不只寫型別。
- 若函式沒有回傳值且只是執行動作，可寫 `Returns:
    None: 無回傳值。`，或依專案慣例省略 `Returns`。
- 若函式可能在多個條件下回傳不同型別或不同意義，分開說明。
- 若有 `*args`、`**kwargs`，說明其用途與內容格式。
- 若有 `**kwargs` 只是轉傳給其他函式，說明會原樣傳遞到哪裡。
- 若 docstring 已存在但缺段落，補齊缺少部分並維持原有語氣與內容。
- 若現有內容與程式碼不符，以程式碼為準，修正文件。

## 檢查清單

完成前確認：

- [ ] 摘要句清楚描述函式用途。
- [ ] 每個重要參數都有說明。
- [ ] 可選參數標示為 `optional`，並說明預設值。
- [ ] 回傳值或 `None` 已說明。
- [ ] 明確拋出的例外已列出。
- [ ] 產生器函式使用 `Yields`。
- [ ] 格式符合 Google 風格與專案既有慣例。
- [ ] 沒有加入程式碼未支持的推測內容。

## 不能做的事

- 不要替函式新增不存在的行為描述。
- 不要改寫程式碼邏輯。
- 不要把不確定的推測寫成事實。
- 如果函式資訊不足以安全補全 docstring，就只寫可確定的部分，並保留保守措辭。
