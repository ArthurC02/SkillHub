---
name: python-docstring-google-filler
description: Fill in missing Python function docstrings in Google style. Use when you have Python code and need to add or repair docstrings so they describe parameters, returns, raises, and behavior consistently.
---

## 目標

為 Python 函式補齊或修正 docstring，並統一成 Google 風格。

## 使用時機

當你看到一段 Python 程式碼，且函式沒有 docstring、docstring 不完整、或格式不是 Google style 時，使用這個技能。

## 處理步驟

1. **先讀函式本體**
   - 找出函式名稱、參數、預設值、型別註記、回傳值、可能丟出的例外、以及函式實際做的事。
   - 如果有型別註記，docstring 仍要用自然語言補充用途與限制，不要只重複型別。

2. **判斷 docstring 需要包含哪些區塊**
   - `Args:`：只列出有意義的參數。
   - `Returns:`：只在函式有回傳值時加入。
   - `Raises:`：只有在函式明確可能丟出例外，且從程式碼可看出來時加入。
   - `Yields:`：只有 generator / iterator 函式才使用。
   - `Examples:`：只有原始程式碼或上下文已提供可確定的使用範例時才加入；不要自行捏造。

3. **撰寫摘要句**
   - 第一行用一句話說明函式做什麼。
   - 用現在式、主動語態、簡潔明確。
   - 不要寫「此函式」開頭的空話，直接描述行為。

4. **補齊參數說明**
   - 每個參數一行，格式為 `name: 說明`。
   - 說明要寫用途、預期內容、重要限制或副作用。
   - 若參數是可選的，說明預設行為。
   - 若參數名稱已能清楚表意，仍要補上它在此函式中的角色。

5. **補齊回傳說明**
   - 說明回傳值代表什麼，而不是只寫型別。
   - 若回傳 `None`，只有在這是函式行為的一部分時才寫 `Returns:`，否則可省略。

6. **補齊例外說明**
   - 只列出從程式碼可直接推知的例外。
   - 說明什麼情況會發生，不要只列例外名稱。

7. **維持 Google 風格格式**
   - 使用標準區塊標題：`Args:`, `Returns:`, `Raises:`, `Yields:`。
   - 區塊標題後空一行再開始內容。
   - 每個條目縮排一致，避免混用其他風格。
   - 不要使用 reStructuredText、NumPy style、Markdown 表格或條列混搭。

8. **避免補過頭**
   - 不要臆測函式沒有明示的行為。
   - 不要加入與程式碼無關的設計意圖、歷史背景或未證實的限制。
   - 如果資訊不足以確定某一段內容，就只寫已知部分，或省略該區塊。

## 輸出原則

- 直接產出可貼回程式碼的 docstring 內容。
- 保持簡潔、準確、與函式實作一致。
- 若原本已有部分 docstring，保留正確內容並補齊缺漏，必要時修正成 Google 風格。
