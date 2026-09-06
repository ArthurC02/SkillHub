---
name: python-docstring-google-filler
description: Fill in missing Python function docstrings in Google style. Use when you have Python code and need to add or repair docstrings so they describe parameters, returns, raises, and behavior consistently.
---

## 目標

為 Python 函式補齊或修正缺少的 docstring，並統一成 Google 風格。

## 適用情況

- 函式沒有 docstring。
- docstring 不完整，缺少 `Args`、`Returns`、`Raises`、`Yields`、`Examples` 等必要段落。
- docstring 有內容但格式不是 Google style，需要重寫。
- 需要根據函式簽名、型別註記、預設值與函式本體推斷文件內容。

## 處理步驟

1. 讀取函式定義、型別註記、預設值、裝飾器與函式本體。
2. 判斷函式用途：
   - 參數做什麼。
   - 回傳什麼。
   - 是否可能丟出例外。
   - 是否是 generator、context manager、async function。
3. 依照 Google 風格補齊 docstring：
   - 簡短摘要放第一行。
   - 需要時加空行後再寫詳細說明。
   - 參數用 `Args:`。
   - 回傳值用 `Returns:`。
   - 例外用 `Raises:`。
   - 產生器用 `Yields:`。
   - 若有使用範例且能從程式推得，才加 `Examples:`。
4. 只寫函式實際需要的段落，不要硬塞不相關內容。
5. 若無法從程式碼可靠推斷某個參數或回傳語意，保守描述其型別與可觀察行為，不要臆測業務背景。
6. 保持 docstring 與程式碼一致：
   - 參數名稱必須與函式簽名一致。
   - 型別描述要與註記一致；若沒有註記，根據用法寫出合理型別。
   - 若函式明顯不回傳值，寫 `Returns: None` 或省略 `Returns`，依專案慣例一致處理。
7. 若函式本身有錯誤或資訊不足，先指出缺口，再補上最合理的 docstring，不要捏造不存在的行為。

## Google 風格格式

- 第一行是摘要，句尾通常用句點。
- 摘要後空一行，再寫其他段落。
- 段落標題使用 `Args:`, `Returns:`, `Raises:`, `Yields:`, `Examples:`。
- `Args:` 下每個參數格式：`name: 說明。`
- 若需要描述型別，放在參數名稱後或說明中，但不要重複冗長。
- `Returns:` 下先寫型別或語意，再寫說明。
- `Raises:` 下列出例外名稱與觸發條件。

## 輸出原則

- 直接產出可貼回 Python 原始碼的 docstring 內容。
- 不要改寫函式程式碼本體。
- 不要加入與函式無關的評論。
- 若一次處理多個函式，逐個補齊，保持每個 docstring 獨立完整。
