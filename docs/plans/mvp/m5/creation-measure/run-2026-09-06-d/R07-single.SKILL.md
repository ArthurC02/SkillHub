---
name: python-docstring-review
description: Review Python function docstrings and add missing documentation in Google style. Use when you have Python code with incomplete or inconsistent function docstrings and need them completed or normalized.
---

## 目標

檢查 Python 函式的 docstring，補齊缺少的內容，並統一成 Google 風格。

## 適用範圍

- 只處理 Python 函式的 docstring。
- 若程式碼沒有提供函式本體、參數型別、回傳值或例外資訊，先從程式碼與上下文推斷；推斷不了就明確標示需要人工確認，不要亂補。
- 若 docstring 已經存在，只補缺漏與修正格式，不要改變原意。

## 處理步驟

1. 找出每個函式的 docstring。
2. 檢查是否缺少以下項目：
   - 簡短摘要
   - 更完整的說明（若函式行為不夠清楚）
   - `Args:`
   - `Returns:`
   - `Raises:`
   - `Examples:`（只有在原始程式碼或上下文已經提供可直接整理的範例時才加）
3. 依函式實際行為補齊內容：
   - 參數名稱、型別、用途要與函式簽名一致。
   - 回傳值要描述實際回傳內容；若沒有回傳，寫明 `Returns: None` 或省略 `Returns:`，依專案慣例一致處理。
   - 例外只列出函式明確可能拋出的例外，不要猜測。
4. 將 docstring 整理成 Google 風格：
   - 第一行是簡短摘要，使用祈使句或描述句皆可，但要精簡。
   - 空一行後再寫詳細說明。
   - 區塊標題使用 `Args:`, `Returns:`, `Raises:`。
   - 每個參數一行，格式為 `name (type): description`。
   - 多行描述要縮排對齊。
5. 保留原本程式碼語意，不改函式邏輯。
6. 若發現以下情況，先停止並回報需要確認：
   - 函式用途無法從程式碼判斷
   - 參數型別或回傳型別無法合理推斷
   - docstring 與程式碼行為明顯衝突

## 輸出原則

- 直接輸出修正後的 docstring，或輸出包含修正後 docstring 的函式片段，依使用者提供內容而定。
- 不要改寫與 docstring 無關的程式碼。
- 不要加入與程式碼無關的推測。
- 若資訊不足，清楚列出缺少哪些資訊，並說明無法安全補寫的原因。
