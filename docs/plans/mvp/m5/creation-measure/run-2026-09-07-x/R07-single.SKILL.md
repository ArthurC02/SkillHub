---
name: python-docstring-google-filler
description: Fill in missing Python function docstrings in Google style. Use when you have Python code and need to add or repair docstrings for functions, parameters, return values, and exceptions.
---

## 目標

為 Python 函式補齊或修正缺少的 docstring，並統一成 Google 風格。

## 適用情況

- 函式沒有 docstring。
- docstring 不完整，缺少 `Args`、`Returns`、`Raises` 等區塊。
- docstring 有內容但不是 Google 風格，需要整理成一致格式。

## 處理步驟

1. **先讀函式本體，不要只看簽名。**
   - 觀察函式名稱、參數、預設值、型別註記、回傳值、例外、以及函式內部實際做了什麼。
   - 以程式碼行為為準，不要憑名稱猜測。

2. **判斷 docstring 應包含哪些區塊。**
   - 有參數就寫 `Args:`。
   - 有回傳值就寫 `Returns:`。
   - 會丟出可預期例外就寫 `Raises:`。
   - 若函式只是就地修改物件且不回傳值，`Returns:` 可省略或寫明 `None`，依專案既有慣例一致處理。
   - 若函式有副作用但沒有明確回傳值，仍要在摘要中說清楚。

3. **撰寫摘要句。**
   - 第一行用一句話說明函式做什麼。
   - 用現在式、簡潔、具體。
   - 不要寫「This function...」這種冗詞，除非專案慣例如此。

4. **補 `Args:`。**
   - 每個參數都要列出。
   - 格式：`name: 說明。`
   - 說明要包含用途、單位、範圍、預期型別或格式、是否可為 `None`、預設值的意義。
   - 若參數名稱已足夠清楚，仍要補一句實際用途，不要只重述名稱。

5. **補 `Returns:`。**
   - 說明回傳值的型別與語意。
   - 若回傳多個值，說明 tuple 中每一項的意義。
   - 若回傳 `bool`、`int`、`str` 等，說明其代表什麼結果，不只寫型別。

6. **補 `Raises:`。**
   - 只列出函式實際可能拋出的、且對呼叫者有意義的例外。
   - 每個例外說明觸發條件。
   - 不要把所有內部可能例外都列出，除非函式明確傳遞出去。

7. **保持與程式碼一致。**
   - 不要捏造不存在的參數、回傳值或例外。
   - 不要把推測寫成事實。
   - 若資訊不足，寫出保守、可由程式碼直接支持的描述。

8. **遵守 Google 風格格式。**
   - 使用縮排區塊：`Args:`、`Returns:`、`Raises:`。
   - 每個區塊標題後空一行或依專案慣例保持一致。
   - 參數與說明對齊，保持簡潔。
   - 若原始碼已有 docstring，保留其正確內容，只補缺漏並整理格式。

## 輸出原則

- 只產出要放進函式的 docstring 內容，或在需要時提供修正後的完整 docstring。
- 不要改寫函式邏輯。
- 不要加入與函式無關的說明。
- 若函式資訊不足以安全補全，明確指出缺少哪些程式碼資訊，並只補可確定的部分。
