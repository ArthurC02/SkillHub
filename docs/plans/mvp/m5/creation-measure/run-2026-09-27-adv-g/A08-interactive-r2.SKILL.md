---
name: translate-feedback-zh-tw-summary-three-points
description: 將英文客戶回饋翻成繁體中文，並在需要將回饋內容整理成三點重點摘要時使用。
---

# English customer feedback to Traditional Chinese translation and three-point summary

將使用者提供的英文客戶回饋，先翻成繁體中文，再整理成三點摘要。

## 執行方式
1. 直接讀取使用者提供的英文回饋內容。
2. 先輸出完整的繁體中文翻譯。
3. 再輸出恰好三點摘要。
4. 摘要只根據原文內容，不能新增原文沒有的事實、推測或延伸分析。
5. 若原文有多句或多段，請整體翻譯、整體摘要，不要只處理部分內容。

## 必須遵守的三條規則
- never invent a fact the input does not give — no name, date, figure or event — and write 'not given' only for such a missing fact;
- when a setting the work needs is missing (a working-day length, a tone, a format), use the common default, say which one you used, and finish the work rather than stopping;
- you cannot send, post, schedule, monitor or fetch anything, so when the request asks for that, deliver the content ready to use and say plainly that sending or scheduling is left to the person; and deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## 輸出格式
- 先標示「繁體中文翻譯」並提供完整翻譯。
- 再標示「三點摘要」並列出 3 點。
- 以繁體中文完成整體輸出。
- 若原文已有語氣或措辭風格，盡量維持自然且忠實。

## 自我檢查
- 翻譯是否忠於原文？
- 摘要是否剛好 3 點？
- 是否沒有新增原文未提及的資訊？
- 是否全程以繁體中文呈現？

## 失敗條件
若輸入不是英文客戶回饋、或內容不足以翻譯，請明確指出缺少什麼；除非缺少的內容本身就是輸入的一部分，否則不要擴寫成推測。