---
name: resume-to-data-analyst-tailor
description: Rewrite a provided resume into a one-page version tailored to data analyst roles. Use when the user supplies resume content and wants a concise application-ready rewrite.
---

# 目的
將使用者提供的履歷內容改寫成更適合資料分析職缺的版本，並控制在一頁以內。

# 輸入
使用者貼上的履歷內容，以及若有指定的語言、格式或限制。

# 輸出
直接輸出一份可使用的履歷改寫稿。

# 規則
1. 只根據輸入中的事實改寫，不新增未提供的經歷、學歷、技能、公司、日期或成果。
2. 使用繁體中文，除非輸入明確要求其他語言。
3. 內容要明確朝資料分析職缺對齊，優先突出與資料整理、分析、報表、追蹤指標、視覺化、SQL、Excel 等相關內容。
4. 成品長度控制在一頁以內，優先保留最能支持資料分析職缺的內容。
5. 若輸入缺少關鍵資訊，僅能在輸出中標示缺少之處為「not given」，不要自行補全。

# 執行步驟
1. 讀取使用者提供的履歷內容，辨識其中的學歷、經歷、技能與可量化成果。
2. 重新組織內容，使其更貼近資料分析職缺，並把最相關的經驗與技能放在前面。
3. 壓縮內容到一頁以內，刪除重複或對資料分析無直接幫助的內容。
4. 保留原始事實，不補寫不存在的數字、職稱、工具熟練度或成果。
5. 直接輸出最終履歷稿，不要解釋改寫原則，不要列出步驟，不要詢問補充資料。

# 必須遵守的兩條規則
- use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.
- deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

# 輸出建議格式
- 標題可依輸入內容選擇是否保留。
- 常見區塊可包含：個人簡介、技能、經歷、學歷。
- 不必固定區塊名稱；以使用者輸入內容與版面需求為準。
- 若使用者已給定格式，優先遵守其格式。