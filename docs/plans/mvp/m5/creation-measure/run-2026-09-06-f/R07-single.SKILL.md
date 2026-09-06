---
name: python-docstring-google-filler
description: Fill in missing Python function docstrings in Google style. Use when you have Python code and need to add or repair docstrings so they describe parameters, returns, raises, and behavior consistently.
---

## 目標

為 Python 函式補齊或修正 docstring，並統一成 Google 風格。

## 使用時機

當你看到一段 Python 程式碼，且函式沒有 docstring、docstring 不完整、或格式不是 Google 風格時，套用這個技能。

## 處理步驟

1. 找出每個需要處理的函式。
   - 只處理函式、方法、`@property`、`@staticmethod`、`@classmethod`。
   - 不要替模組、類別屬性、變數或測試資料亂加說明。

2. 讀函式本體與型別資訊。
   - 依參數名稱、預設值、型別註記、回傳值、例外、以及函式內部邏輯，推斷 docstring 內容。
   - 若程式碼不足以確定某項資訊，保守描述可觀察到的行為，不要臆測。

3. 產生或補齊 Google 風格 docstring。
   - 第一行用一句話總結函式做什麼。
   - 若需要補充背景，再加一段簡短說明。
   - 接著依需要加入以下區塊：
     - `Args:`：列出所有參數，包含型別與用途。
     - `Returns:`：說明回傳值與型別。
     - `Yields:`：若是生成器，改用這個區塊。
     - `Raises:`：列出函式明確會拋出的例外。
     - `Attributes:`：只在類別 docstring 才用；函式不要用。
   - 參數順序要和函式簽名一致。
   - 若函式沒有參數，就不要硬寫 `Args:`。
   - 若沒有回傳值或明確回傳 `None`，可省略 `Returns:`，或寫明 `None`，但要一致。

4. 保持內容貼近程式碼。
   - 說明實際行為，不要寫空泛口號。
   - 不要加入程式碼沒有證據支持的副作用、效能保證或業務規則。
   - 若函式名稱、參數名、回傳型別已足夠清楚，docstring 可以簡潔。

5. 修正格式。
   - 使用 Google 風格標題與縮排。
   - 每個參數一行，格式為 `name: description`。
   - 型別寫在描述前或括號中，保持一致且清楚。
   - 保持行長適中，避免過長句子。
   - 若原本已有 docstring，保留正確資訊，只補缺漏與修正格式。

6. 輸出結果。
   - 直接提供修正後的 docstring，或提供包含 docstring 的函式片段，依使用者要求而定。
   - 不要額外解釋規則，除非使用者要求說明。

## 判斷原則

- 以程式碼為準，不以註解或猜測為準。
- 若參數是 `*args`、`**kwargs`，只在能從程式碼看出用途時才描述。
- 若函式是簡單轉發、包裝或屬性存取器，docstring 可短，但仍要準確。
- 若函式名稱與行為不一致，以行為為準。
- 若無法確定某個例外是否會發生，不要列入 `Raises:`。

## 範例結構

```python
def add_user(name: str, active: bool = True) -> User:
    """Create a new user record.

    Args:
        name: The user's display name.
        active: Whether the user starts as active.

    Returns:
        The created user record.
    """
```

## 注意事項

- 不要改動函式邏輯。
- 不要把 docstring 寫成 Markdown。
- 不要使用 reStructuredText 標記。
- 不要補與函式無關的模組說明。
