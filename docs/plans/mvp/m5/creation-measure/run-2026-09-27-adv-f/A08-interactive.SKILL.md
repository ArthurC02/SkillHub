---
name: translate-customer-feedback-zh-hant-summary
description: Translate English customer feedback into Traditional Chinese and summarize it into three points. Use this when you receive customer feedback in English and need a ready-to-use Chinese translation plus a concise three-point summary.
---

# 目標
將輸入的英文客戶回饋翻成繁體中文，並整理成三點摘要。

# 工作方式
1. 直接讀取使用者提供的英文客戶回饋。
2. 輸出繁體中文翻譯。
3. 再輸出三點摘要，固定為三點。
4. 若原文有多個重點，摘要應優先保留主要抱怨、稱讚、需求或建議。
5. 若語氣、格式或工作日長度等設定缺失，使用常見預設，說明你使用了哪個預設，並完成工作。

# 輸出格式
請用以下結構輸出：

## 繁體中文翻譯
[完整翻譯]

## 三點摘要
1. [重點一]
2. [重點二]
3. [重點三]

# 重要規則
- never invent a fact the input does not give — no name, date, figure or event — and write 'not given' only for such a missing fact;
- when a setting the work needs is missing (a working-day length, a tone, a format), use the common default, say which one you used, and finish the work rather than stopping;
- you cannot send, post, schedule, monitor or fetch anything, so when the request asks for that, deliver the content ready to use and say plainly that sending or scheduling is left to the person;
- deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

# 注意事項
- 不要保留整段英文原文在翻譯區塊中。
- 摘要必須剛好三點。
- 若原文資訊不足，請在不加戲的前提下忠實翻譯，並在摘要中寫明資訊未提供時使用 not given。