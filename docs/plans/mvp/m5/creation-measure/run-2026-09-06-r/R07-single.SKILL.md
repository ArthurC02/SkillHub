---
name: python-docstring-google-filler
description: Fill in missing Python function docstrings in Google style. Use when you have Python code and need to add or repair docstrings for functions, methods, or callables without changing behavior.
---

## 目的

為 Python 函式補齊或修正文檔字串，使用 Google 風格，並保持程式行為不變。

## 適用情況

- 函式、方法、類別中的方法、`@staticmethod`、`@classmethod`、lambda 以外的可呼叫物件需要 docstring。
- 現有 docstring 缺少參數、回傳值、例外、屬性說明，或格式不是 Google 風格。
- 需要統一專案內 Python docstring 的寫法。

## 工作流程

1. **先讀函式本體，不先寫字。**
   - 看函式名稱、參數、預設值、型別註記、回傳型別註記、`raise`、`yield`、`return`、`await`、內部呼叫與分支。
   - 若有現成 docstring，先判斷哪些資訊缺漏、過時或與程式不一致。

2. **整理函式介面。**
   - 列出所有公開參數，包含位置參數、關鍵字參數、`*args`、`**kwargs`。
   - 若參數名稱可從程式推知用途，寫清楚用途；若無法可靠推知，只描述其角色，不要臆測業務語意。
   - 若有型別註記，優先沿用型別註記，不重複寫成冗長敘述。

3. **判斷需要哪些 Google 風格區塊。**
   - `Args:`：有參數就寫。
   - `Returns:`：函式有明確回傳值、`yield`、或回傳 `None` 但需要說明副作用時寫。
   - `Raises:`：函式內明確 `raise` 例外，且例外是介面的一部分時寫。
   - `Yields:`：生成器函式使用 `yield` 時寫，不用 `Returns:`。
   - `Attributes:`：只在類別 docstring 或資料類別需要時使用；一般函式不要用。
   - `Examples:`：只有在原始需求或程式上下文明顯需要時才加，不要自行發明。

4. **撰寫內容時遵守事實。**
   - 只寫程式碼能支持的內容。
   - 不確定的地方，用保守描述，例如「傳入的值」、「要處理的項目」、「若驗證失敗則拋出例外」。
   - 不要補不存在的參數，不要猜測隱含行為，不要把實作細節寫成 API 保證。

5. **使用 Google 風格格式。**
   - 第一行是簡短摘要句，通常是祈使句或描述句。
   - 空一行後再寫區塊。
   - 區塊標題使用 `Args:`, `Returns:`, `Yields:`, `Raises:`。
   - 每個參數一行：`name: 說明。`
   - 若需要型別，寫成 `name (type): 說明。`，但若型別註記已清楚且專案慣例不重複型別，可省略。
   - 回傳區塊寫明回傳內容與條件；若回傳 `None`，說明其副作用或明確寫「無回傳值」。
   - 例外區塊寫 `ExceptionType: 觸發條件。`

6. **保持簡潔。**
   - 每個說明以一句到兩句為主。
   - 避免重複函式名稱、避免空話、避免把每行程式碼翻成文字。
   - 若函式很簡單，docstring 也應簡短。

7. **修正文檔字串時同步一致化。**
   - 若現有 docstring 與程式不符，以程式為準。
   - 若缺少參數說明，補齊所有參數。
   - 若有多餘或錯誤的區塊，刪除或改正。
   - 若函式名稱、參數或回傳型別改了，docstring 也要跟著更新。

## 輸出原則

- 只產出修好的 docstring 內容，或在需要時產出可直接貼回原始碼的完整 docstring 區塊。
- 不要改寫函式程式碼。
- 不要加入與 docstring 無關的說明。
- 不要使用 reStructuredText、NumPy 風格或 Markdown 標題來取代 Google 風格。
