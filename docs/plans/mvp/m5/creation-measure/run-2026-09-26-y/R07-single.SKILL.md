---
name: python-docstring-google-review
description: Review Python function docstrings and add missing documentation in Google style. Use when you have Python code and need to check or complete function docstrings, especially for missing Args, Returns, Raises, or other sections.
---

## 目的

檢查 Python 函式的 docstring，找出缺漏並補齊，統一成 Google 風格。

## 適用情境

- 使用者貼出 Python 函式或模組，要求補 docstring
- 需要把現有 docstring 改寫成 Google style
- 需要補上缺少的參數、回傳值、例外、屬性或其他說明

## 處理原則

1. 只根據提供的程式碼與上下文撰寫 docstring，不要臆測不存在的行為。
2. 若資訊不足，保留可確定的部分，並用最保守、最中性的描述。
3. 不要改動函式邏輯，只處理 docstring。
4. 以 Google 風格輸出：摘要行、空行、必要的區塊標題。
5. 若原本已有 docstring，保留正確內容，只補缺漏與修正格式。

## 檢查步驟

1. 找出函式名稱、參數、預設值、型別註記、回傳值、可能拋出的例外。
2. 比對現有 docstring 是否缺少以下項目：
   - 簡短摘要
   - `Args:`
   - `Returns:`
   - `Yields:`（若是 generator）
   - `Raises:`
   - `Attributes:`（若是 class docstring，不是函式則不適用）
   - `Examples:`（只有在程式碼或上下文已提供範例時才保留或補充）
3. 依函式實際行為決定是否需要 `Returns:` 或 `Raises:`：
   - 沒有明確回傳值但實際回傳 `None`，仍可寫 `Returns: None` 或省略，依上下文一致性決定。
   - 若函式可能丟出明確例外，列出最重要且可從程式碼確認的例外。
4. 參數說明要包含：
   - 參數名稱
   - 型別（若可從註記或程式碼推得）
   - 作用或用途
   - 預設值的語意（若有且重要）
5. 若有 `*args`、`**kwargs`、keyword-only 參數，也要在 `Args:` 中清楚說明。
6. 若函式是 property、classmethod、staticmethod、generator、async function，依其型態調整描述。

## Google 風格格式

- 摘要行：一句話說明函式做什麼。
- 空一行後再寫區塊。
- 區塊標題使用 `Args:`, `Returns:`, `Yields:`, `Raises:` 等。
- 每個參數用 `name: description`。
- 型別可寫在名稱後或描述中，但要一致且清楚。
- 多行描述要縮排對齊，保持可讀性。

## 輸出要求

- 直接輸出修正後的 docstring，或輸出完整函式與更新後的 docstring，依使用者提供內容而定。
- 不要額外解釋審查過程。
- 不要捏造未出現在程式碼中的行為。
- 若使用者只提供單一函式，優先只回傳該函式的 docstring 內容；若要求整段程式碼，則回傳更新後的程式碼。
