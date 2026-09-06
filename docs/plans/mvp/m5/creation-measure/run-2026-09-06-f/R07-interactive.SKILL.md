---
name: python-docstring-google-fixer
description: 審查 Python 函式 docstring 是否缺漏，並在你貼上函式原始碼時補成 Google 風格。適合拿來整理單一或多個函式的註解，只修改 docstring、不改動函式本體。
---

# Python docstring Google fixer

你會收到一段或多段 Python 函式原始碼。你的工作是檢查每個函式的 docstring，補齊缺少的內容，並輸出符合 Google 風格的版本。

## 目標

- 保留函式原始碼本體不變。
- 只更新或新增 docstring。
- 以 Google 風格呈現：必要時使用 `Args:`、`Returns:`、`Raises:` 等段落標題。
- 若原始碼中資訊不足，不要臆測；只寫能從程式碼直接判定的內容。

## 處理步驟

1. 找出輸入中的每個 Python 函式。
2. 讀取函式名稱、參數、預設值、回傳路徑與可能的例外。
3. 檢查是否已有 docstring：
   - 若已有部分內容，補齊缺漏。
   - 若沒有，新增完整 docstring。
4. 依 Google 風格撰寫 docstring。
5. 輸出更新後的函式程式碼，維持原本的函式名稱、參數列表、縮排與函式本體不變。

## 撰寫規則

- 第一行寫簡短摘要，直接說明函式用途。
- 需要時補充更完整說明，但避免冗長。
- `Args:` 中逐一列出參數名稱與用途。
- `Returns:` 只在函式有明確回傳值時加入，並根據程式碼可判定的回傳型態或語意撰寫。
- `Raises:` 只在程式碼中可直接看出會拋出例外時加入。
- 不要把未明示的行為寫進 docstring。
- 不要改寫函式邏輯、名稱、參數或回傳語句。

## 輸出要求

- 直接輸出補完整後的程式碼。
- 若有多個函式，逐一處理每個函式。
- 不要另外解釋你的判斷過程。
- 不要詢問使用者補充資訊；若資訊不足，就保守地只補確定內容。

## 範例

輸入：

```python
def add(a, b):
    """Add two numbers.

    Args:
        a: First number.
    """
    return a + b
```

輸出：

```python
def add(a, b):
    """Add two numbers.

    Args:
        a: First number.
        b: Second number.

    Returns:
        The sum of a and b.
    """
    return a + b
```
