---
name: python-docstring-google-fixer
description: 審查 Python 函式的 docstring，補齊缺漏並輸出為 Google 風格；當你貼上一段 Python 函式或其 docstring、需要整理成可直接貼回原始碼的版本時使用。
---

# Python Docstring Google Fixer

當使用者貼上一段 Python 函式、方法，或其現有 docstring，並要求你「審查、補齊缺漏、改成 Google 風格」時，依照本 Skill 一次完成。

## 目標

- 檢查 docstring 是否缺少 Google 風格常見區塊。
- 保留原本已經正確且有用的內容。
- 只補齊缺漏與必要修正，不做無關重寫。
- 輸出可直接貼回 Python 原始碼使用的 docstring 文本。

## 執行方式

1. 讀取使用者提供的 Python 函式、方法，或現有 docstring。
2. 判斷這段 docstring 是否能用 Google 風格整理。
3. 以原內容為基礎，補上缺少但可從輸入合理推得的部分。
4. 若輸入已提供名稱、參數、回傳值、例外或屬性等資訊，保留其語意並改寫成 Google 風格的標題與格式。
5. 若輸入不足以安全推斷某個內容，維持中性表述，不自行捏造具體行為。
6. 只輸出整理後的 docstring；不要額外解釋、不要附加審查報告，除非使用者明確要求。

## Google 風格整理規則

- 使用清楚的摘要行開頭。
- 視內容加入下列區塊，且只加入與輸入相關者：
  - `Args:`
  - `Returns:`
  - `Yields:`
  - `Raises:`
  - `Attributes:`
  - `Examples:`
- 區塊名稱與縮排保持一致、可直接貼回 Python 三引號字串。
- 參數說明要簡潔，保留原本名稱與可確認的語意。
- 回傳說明只描述輸入可支持的內容。
- 若原 docstring 已有某些段落內容，優先沿用，再整理格式與缺漏。

## 輸出要求

- 預設輸出一段完整 docstring 文本。
- 若原始內容只有部分 docstring，輸出應補成完整且格式一致的版本。
- 若使用者要求「只審查不改寫」，則只指出缺漏與格式問題；但本 Skill 的預設任務是補齊並改成 Google 風格。
- 不要把 Markdown、程式碼框、或額外說明混進最終 docstring，除非使用者要求。

## 判斷原則

- 已有內容能保留就保留。
- 缺少的段落才補。
- 不確定的細節不要自行發明。
- 若輸入不是 Python 函式或沒有足夠 docstring 內容，直接根據可見材料整理；不要假設不存在的參數或回傳。

## 最終輸出格式

只輸出整理後的 docstring 文字本體，讓使用者能直接貼回 Python 原始碼。