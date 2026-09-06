---
name: resume-to-data-analyst-resume
description: 將使用者提供的履歷改寫成適合資料分析職缺的一頁內中文履歷草稿；當輸入是一份履歷原文時使用。
---

# 目的
把使用者提供的履歷改寫成適合資料分析職缺的中文履歷版本，並控制在一頁以內。

# 使用時機
當使用者貼上履歷原文，要求改寫成資料分析職缺可用的履歷草稿時使用。

# 你要直接做的事
1. 讀取使用者提供的履歷原文。
2. 只使用輸入中已提供的資訊，將內容重寫成更貼近資料分析職缺的表述。
3. 保留與資料分析相關的經歷、技能、工具、成果與量化資訊；如果輸入沒有提供，就寫 `not given`。
4. 將成品控制在一頁以內，優先保留最有關聯的內容，刪除與資料分析無關或重複的敘述。
5. 直接輸出可貼到求職文件中的履歷草稿，不要解釋改寫策略。

# 產出要求
- 輸出必須是中文。
- 輸出要針對資料分析職缺改寫，而不是原樣重述履歷。
- 輸出要能直接作為求職用履歷草稿。
- 不要新增輸入未提供的經歷、學歷、公司名稱、日期、證照、數字或技能。
- 如果某個必要欄位或資訊在輸入中缺失，寫 `not given`。

# 執行規則
- use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.
- deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

# 輸出格式
直接輸出改寫後的一頁內履歷草稿。若輸入本身缺少關鍵資訊，仍先完成改寫，並以 `not given` 標示缺漏處。