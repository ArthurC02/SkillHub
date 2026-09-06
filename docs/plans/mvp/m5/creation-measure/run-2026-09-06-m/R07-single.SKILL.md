---
name: python-docstring-google-filler
description: Fill in missing docstrings for Python functions using Google style. Use when you have Python code with incomplete or absent function docstrings and need consistent parameter, return, and exception documentation.
---

## 目的

為 Python 函式補齊或修正 docstring，並統一成 Google 風格。

## 適用時機

- 你收到一段 Python 程式碼，要求檢查函式 docstring 是否缺漏。
- 需要補上參數、回傳值、例外、屬性或副作用說明。
- 需要把既有 docstring 改寫成 Google 風格。

## 工作流程

1. **先掃描所有函式**
   - 找出每個 `def`、`async def`、方法、巢狀函式。
   - 判斷哪些函式沒有 docstring、哪些 docstring 不完整、哪些格式不是 Google 風格。

2. **從程式碼推斷函式介面**
   - 讀取函式名稱、參數、預設值、型別註記、`*args`、`**kwargs`、關鍵字專用參數。
   - 讀取回傳敘述：是否有 `return`、是否可能回傳 `None`、是否會丟出例外。
   - 讀取函式內部行為：是否修改輸入、是否有副作用、是否依賴外部狀態。

3. **補齊 Google 風格 docstring**
   - 使用三引號字串。
   - 第一行是簡短摘要，直接說明函式做什麼。
   - 空一行後，依需要加入下列區塊：
     - `Args:`
     - `Returns:`
     - `Yields:`
     - `Raises:`
     - `Attributes:`
     - `Examples:`
   - 每個參數都要列出；名稱、型別、說明要一致。
   - 若函式沒有明確回傳值但有副作用，寫清楚 `Returns: None` 或省略回傳區塊，依程式語意決定。
   - 若有例外是程式碼明確會丟出的，列入 `Raises:`。

4. **保持內容準確，不要臆測**
   - 只能根據程式碼可直接推得的資訊撰寫。
   - 若無法確定型別、回傳或例外，使用保守描述，不要編造。
   - 不要加入與程式碼無關的功能說明。

5. **維持原始程式碼風格**
   - 不改變函式邏輯。
   - 不重命名參數或變數。
   - 只補 docstring 或修正文案。
   - 若原本已有 docstring，保留正確內容並補足缺漏。

## Google 風格規則

- 區塊標題首字母大寫，後面加冒號，例如 `Args:`。
- 參數說明格式：`name (type): description.`
- 回傳說明格式：`type: description.`
- 例外說明格式：`ExceptionType: description.`
- 說明文字要簡潔、具體、可直接對應程式碼。
- 多行說明要縮排對齊，避免混用其他 docstring 風格。

## 輸出要求

- 直接輸出修正後的 Python 程式碼，或只輸出需要替換的 docstring，依使用者要求而定。
- 若使用者只提供函式片段，就只修正該片段。
- 若程式碼中有多個函式，逐一處理，不要漏掉任何一個。
