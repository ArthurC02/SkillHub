---
name: python-docstring-google-review
description: Review Python function docstrings and add missing documentation in Google style. Use when you are given Python code and need to check, complete, or normalize function docstrings without changing the code logic.
---

## 目的

檢查 Python 函式的 docstring，補齊缺漏內容，並統一成 Google 風格。

## 適用情況

- 使用者貼出 Python 程式碼，要你審查或補寫函式 docstring。
- 需要把現有 docstring 改成 Google 風格。
- 需要補上缺少的 `Args`、`Returns`、`Raises`、`Yields`、`Examples` 等區段。

## 工作流程

1. **先辨識範圍**
   - 只處理 Python 函式、方法、類別中的方法，以及必要時的模組層級函式。
   - 不改變程式邏輯、參數名稱、回傳值或例外行為。
   - 若使用者提供的是整個檔案，逐個函式檢查。

2. **讀取函式簽名與實作**
   - 從函式名稱、參數、型別註記、預設值、回傳敘述、`raise` 語句、`yield`、`return` 路徑推斷 docstring 內容。
   - 若有型別註記，docstring 不必重複冗長型別，但可在必要時補充可讀性資訊。
   - 若無法從程式碼確定某項行為，保守描述，不要臆測。

3. **判斷缺漏項目**
   - 檢查是否缺少簡短摘要。
   - 檢查是否缺少參數說明，包含關鍵字參數、可選參數、`*args`、`**kwargs`。
   - 檢查是否缺少回傳說明。
   - 檢查是否缺少可能拋出的例外。
   - 若函式是 generator，使用 `Yields`；若是一般函式，使用 `Returns`。
   - 若函式沒有回傳值，明確寫 `Returns: None` 或省略回傳段落，依專案慣例一致處理；若使用者未提供慣例，優先保留簡潔但清楚的寫法。

4. **以 Google 風格重寫或補齊**
   - 使用以下結構，依實際需要保留或省略區段：
     - 一行摘要。
     - 空行。
     - `Args:`
     - `Returns:` 或 `Yields:`
     - `Raises:`
     - `Examples:`
   - 每個參數一行，格式清楚一致。
   - 說明要具體、簡潔、可讀，避免重複函式名稱或程式碼字面內容。
   - 若原 docstring 已有內容但格式不符，保留其語意並改寫成 Google 風格。

5. **處理特殊情況**
   - 若函式名稱或實作不足以判斷用途，根據程式碼行為寫中性描述，不要猜測業務語意。
   - 若 docstring 缺少資訊且程式碼也無法推斷，明確指出無法從現有程式碼確認，而不是編造。
   - 若函式本身不適合寫詳細 docstring（例如極短的私有輔助函式），仍應至少提供準確摘要與必要參數說明。

## 輸出要求

- 直接提供修正後的 docstring，或逐個函式列出建議版本。
- 不要改寫函式程式碼，除非使用者明確要求。
- 不要加入與程式無關的解釋。
- 若一次處理多個函式，請清楚標示每個函式對應的 docstring。
- 保持與原始程式碼一致的語氣與技術層級。
