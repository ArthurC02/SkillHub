---
name: manual-traditional-chinese-translator
description: 將英文使用手冊翻成繁體中文，適合在需要保留技術說明語氣、並於專有名詞首次出現時附上原文的情境使用。
---

# 目的
將使用者提供的英文使用手冊翻譯成繁體中文，並在專有名詞第一次出現時保留原文以便對照。

## 兩條必守規則
- use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent;
- deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

# 執行方式
1. 直接閱讀使用者提供的英文使用手冊全文。
2. 將內容翻成繁體中文，維持原本的說明手冊語氣、技術性與操作順序。
3. 專有名詞第一次出現時，以「繁體中文（原文）」或等效清楚格式呈現原文；之後再次出現時只用繁體中文即可。
4. 不要摘要、改寫、補充背景知識，也不要加入原文沒有的步驟、警告或解釋。
5. 若輸入中有不清楚、缺漏或無法辨識之處，只能寫 `not given`，不要自行推測。
6. 輸出只交付翻譯完成的手冊內容本身。

# 輸出要求
- 使用繁體中文。
- 保留段落、標題、條列與操作順序；若原文是連續段落，就以自然段輸出。
- 保持術語一致。
- 若原文已有單位、數字、按鍵名稱或介面文字，盡量保留其原意與格式。
- 不要在譯文外加註解、說明或前言。

# 結束條件
當整份手冊已完整翻譯成繁體中文，且所有首次出現的專有名詞已附原文時，即完成。