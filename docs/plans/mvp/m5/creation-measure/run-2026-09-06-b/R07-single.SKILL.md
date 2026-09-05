---
name: python-docstring-google-review
description: Review Python function docstrings and fill in missing parts using Google style. Use when you have Python code or function signatures with incomplete, inconsistent, or absent docstrings and need them standardized.
---

## 目標

檢查 Python 函式的 docstring，補齊缺漏內容，並統一成 Google 風格。

## 適用情況

- 函式已有 docstring，但缺少 `Args`、`Returns`、`Raises`、`Yields`、`Examples` 等區塊。
- 函式沒有 docstring，需要根據函式名稱、參數、回傳型別與程式內容補寫。
- 既有 docstring 不是 Google 風格，需要改寫成 Google 風格。

## 處理步驟

1. 先讀函式簽名與函式本體。
   - 找出參數名稱、預設值、型別註記、回傳值、可能拋出的例外。
   - 觀察函式實際行為，不要只依賴名稱。

2. 判斷 docstring 應包含哪些區塊。
   - 有參數就寫 `Args:`。
   - 有回傳值就寫 `Returns:`。
   - 會丟例外就寫 `Raises:`。
   - 若是 generator 或使用 `yield`，寫 `Yields:`，不要同時寫 `Returns:`。
   - 若函式有副作用但不回傳值，`Returns:` 可省略或寫明 `None`，依專案慣例一致處理。

3. 補齊內容時，優先以程式實際行為為準。
   - 參數說明要交代用途、限制、單位、格式或是否可為 `None`。
   - 回傳說明要寫清楚回傳資料的型別與語意。
   - 例外說明只列出函式內明確可能發生、且對使用者有意義的例外。
   - 不要憑空新增程式沒有的行為。

4. 使用 Google 風格格式化。
   - 第一行是簡短摘要，句尾通常加句點。
   - 空一行後再寫區塊。
   - 區塊標題使用 `Args:`, `Returns:`, `Raises:`, `Yields:`, `Examples:`。
   - 參數條目格式為 `name: 說明`，必要時可分行縮排補充。
   - 保持縮排一致，讓 docstring 可直接貼回 Python 程式。

5. 若原 docstring 有內容但不完整或不一致，保留正確資訊並修正問題。
   - 修正參數名稱與簽名不一致的地方。
   - 刪除與程式不符的描述。
   - 合併重複或衝突的說明。
   - 讓語氣、術語與整體格式一致。

6. 若資訊不足以安全補寫，先指出缺口。
   - 例如函式本體未提供、型別與行為無法判定、或有多種合理解讀時，不要硬編。
   - 改為列出需要確認的資訊，例如回傳型別、例外條件、參數語意。

## 輸出原則

- 直接產出可用的 Google 風格 docstring，或產出修訂後的完整 docstring。
- 不要解釋規則，不要額外評論程式碼品質，除非資訊不足需要先確認。
- 不要改寫函式程式碼本身。
- 不要加入與函式無關的內容。
