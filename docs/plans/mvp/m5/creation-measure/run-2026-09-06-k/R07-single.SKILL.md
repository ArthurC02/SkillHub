---
name: python-docstring-google-filler
description: Fill in missing Python function docstrings in Google style. Use when you have Python code with incomplete or absent docstrings and need concise, consistent documentation for parameters, returns, and exceptions.
---

## 目的

為 Python 函式補齊或改寫 docstring，使用 **Google 風格**，並只補文件，不改程式邏輯。

## 適用情況

- 函式沒有 docstring。
- docstring 不完整，缺少 `Args`、`Returns`、`Raises` 等區塊。
- 需要把既有 docstring 統一成 Google 風格。

## 工作原則

1. 先讀函式簽名與函式內容，理解它實際做什麼。
2. 只根據程式碼可證實的行為撰寫，不要臆測未出現的副作用。
3. 若資訊不足，保守描述，避免編造。
4. 保持 docstring 與程式碼一致，尤其是：
   - 參數名稱與型別
   - 回傳值
   - 可能拋出的例外
   - 是否可為 `None`
5. 不要修改函式邏輯、命名、縮排或其他程式碼。

## Google 風格規則

### 1. 簡短摘要

第一行用一句話說明函式用途，使用祈使句或描述句皆可，但要簡潔。

### 2. 詳細說明

若函式行為較複雜，可在摘要後空一行補充 1–3 句說明。

### 3. Args 區塊

對每個參數逐一列出：

```python
Args:
    name (type): 說明。
```

規則：
- 參數順序與函式簽名一致。
- 型別盡量從註解、型別標註或程式碼推斷。
- 若型別無法確定，可省略型別，但仍要寫說明。
- 若參數有預設值，說明其預設行為即可，不必重複預設值。

### 4. Returns 區塊

若函式有回傳值，寫：

```python
Returns:
    type: 說明。
```

規則：
- 若回傳 `None` 且這是函式正常行為，可省略 `Returns`，或明確寫 `None`。
- 若回傳多種型別，簡潔描述實際可能值。

### 5. Raises 區塊

若函式明確會拋出例外，寫：

```python
Raises:
    ValueError: 何種情況會發生。
```

只列出程式碼中可確認的例外，不要猜測。

### 6. 其他區塊

只有在程式碼明確需要時才加入，例如：
- `Yields`
- `Attributes`
- `Examples`

不要為了湊格式而加入不必要區塊。

## 補寫步驟

1. 找出函式名稱、參數、回傳值與例外。
2. 判斷 docstring 是否缺漏或不符合 Google 風格。
3. 以最少必要文字補齊內容。
4. 若原 docstring 有正確內容，保留其意思並改成 Google 風格。
5. 若函式很簡單，保持 docstring 簡短，不要過度解釋。

## 品質檢查

完成後確認：

- 第一行是否清楚描述函式用途。
- `Args` 是否涵蓋所有參數。
- `Returns` 是否與實際回傳一致。
- `Raises` 是否只包含程式碼可證實的例外。
- 格式是否符合 Google 風格縮排。
- 沒有加入與程式碼無關的推測內容。

## 輸出要求

只輸出補好的 docstring 內容，或在需要時輸出已整理好的函式 docstring 區塊；不要解釋過程，不要附加多餘文字。
