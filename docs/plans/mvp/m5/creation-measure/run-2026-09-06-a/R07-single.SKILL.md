---
name: python-docstring-google-filler
description: 補寫或修正 Python 函式的 docstring，特別是缺少參數、回傳值、例外或摘要時；當你拿到 Python 函式程式碼並需要補成 Google 風格 docstring 時使用。
---

# 目標

根據提供的 Python 函式程式碼，補齊缺少的 docstring，並統一成 Google 風格。

# 適用情境

- 函式已有部分 docstring，但缺少 `Args`、`Returns`、`Raises`、`Yields`、`Attributes` 或摘要。
- 函式完全沒有 docstring，需要新增。
- 需要把既有 docstring 改寫成 Google 風格，但不改變程式行為。

# 處理步驟

1. 先讀函式本體與簽名。
   - 找出函式名稱、參數、預設值、型別註記、回傳型別註記。
   - 觀察函式內部的 `return`、`yield`、`raise`、`assert`、`try/except`、副作用與資料流。

2. 判斷 docstring 應包含哪些區塊。
   - 有參數就寫 `Args:`。
   - 有回傳值且不是 `None` 就寫 `Returns:`。
   - 有 `yield` 就寫 `Yields:`，不要同時寫 `Returns:`，除非函式同時明確回傳與產生值。
   - 會主動丟出例外且這些例外對使用者有意義時，寫 `Raises:`。
   - 若函式是方法且有重要屬性初始化或設定，才考慮 `Attributes:`；一般函式通常不需要。

3. 撰寫摘要句。
   - 第一行用一句話說明函式做什麼。
   - 用現在式、第三人稱、簡潔明確。
   - 不要重複函式名稱，不要寫冗長背景。

4. 補齊 `Args:`。
   - 每個參數都要列出，順序與函式簽名一致。
   - 格式使用 `name: 說明`。
   - 說明要包含用途、單位、範圍、預期格式或限制。
   - 若參數是可選的，說明預設行為。
   - 若參數名稱已能清楚表意，說明可簡短，但不要空白。

5. 補齊 `Returns:` 或 `Yields:`。
   - 說明回傳值的型別與語意。
   - 若回傳 `None`，通常不寫 `Returns:`，除非要強調副作用或 API 約定。
   - 若回傳多個值，說明 tuple 或結構內容。
   - 若是 generator，寫 `Yields:` 並描述每次產生的值。

6. 補齊 `Raises:`。
   - 只列出程式碼中實際可能發生、且對呼叫者有意義的例外。
   - 不要把所有可能的內建例外都列出。
   - 每個例外都要說明觸發條件。

7. 保持內容與程式碼一致。
   - 不要猜測不存在的行為。
   - 不要新增程式碼沒有表現出的參數、回傳或例外。
   - 若資訊不足，寫最保守、可由程式碼直接支持的描述。

8. 輸出時只提供 docstring 內容。
   - 使用 Google 風格標題與縮排。
   - 不要包在 Markdown code fence 裡，除非使用者明確要求。
   - 不要改寫函式本體。

# Google 風格格式

```text
簡短摘要。

Args:
    param1: 說明。
    param2: 說明。

Returns:
    回傳值說明。

Raises:
    ValueError: 何時會發生。
```

# 判斷原則

- 以程式碼為準，不以註解、命名習慣或推測為準。
- 若原 docstring 已有正確內容，只補缺漏，不要無故改寫。
- 若原 docstring 與程式碼衝突，以程式碼為準並修正。
- 若函式過於簡單且 docstring 沒有實質資訊，可保留極簡摘要，但仍要符合 Google 風格。
- 若遇到重載、裝飾器或型別註記，優先從實際函式定義與函式體推斷。

# 產出要求

- 只輸出最終 docstring 文本。
- 保持語言與使用者提供的內容一致；若原始內容是中文，docstring 也用中文撰寫。
- 不要加入額外解釋。
