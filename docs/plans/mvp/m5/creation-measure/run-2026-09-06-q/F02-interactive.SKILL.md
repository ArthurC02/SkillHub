---
name: webpage-change-checklist
description: Create a checklist from a webpage’s actual visible text for later comparison when you need to verify whether the page content has changed.
---

# 用途
將使用者指定的網頁內容整理成一份可逐項比對的檢查清單，用於日後確認頁面文字是否被改動。

# 作業原則
- use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent;
- deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.
- 只依據目前輸入中提供的頁面實際文字撰寫。
- 若輸入已附上頁面文字，就直接產出檢查清單。
- 若輸入要求依網址取得文字，先抓取該頁面可見內容，再據此整理。
- 若頁面文字無法取得或回傳空白，只輸出 `not given`，不要推測頁面內容。
- 不要加入未出現在輸入中的日期、背景、解釋或延伸說明。

# 步驟
1. 讀取使用者提供的頁面文字；若有網址且沒有文字，先取得該頁面的可見文字。
2. 擷取頁面中的可見重要文字，保留標題、正文、連結文字與其他足以比對變更的內容。
3. 轉寫為繁體中文的檢查清單，每一項都要能逐項核對。
4. 每一條只寫可直接對照原文的陳述句，不寫推論、不寫摘要式評論。
5. 若頁面內容不足以形成清單，回覆 `not given`。

# 輸出格式
- 直接輸出檢查清單。
- 使用條列或編號皆可，但每一項都必須是可核對的條目。
- 若頁面有多段重要文字，清單要涵蓋它們。
- 若文字中有明顯可辨識的標題、段落或連結文字，應保留在清單中。

# 判斷準則
- 清單必須只反映輸入與取得文字中實際出現的內容。
- 清單的用途必須明確指向「比對頁面內容是否改動」。
- 不要自行補齊缺漏資訊；缺漏處寫 `not given`。