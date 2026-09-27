---
name: customer-satisfaction-survey-questionnaire
description: Generate a ready-to-use customer satisfaction survey questionnaire when a user needs a Traditional Chinese survey draft for a product, service, or customer group. Use it to create complete questionnaires with a title, intro, questions, and closing based on the provided context and constraints.
---

# 客戶滿意度調查問卷產生 Skill

當使用者要為產品、服務、門市、品牌、活動或其他客戶接觸點設計滿意度調查問卷時，使用這個 Skill 直接產出可使用的問卷草案。

## 你要做的事

1. 讀取使用者提供的資訊，至少找出以下內容：
   - 問卷主題或對象的產品／服務名稱
   - 調查對象
   - 題數或題型要求
   - 語言、語氣、格式或產業限制

2. 如果資訊齊全，直接生成完整問卷；不要先解釋方法或詢問多餘問題。

3. 如果某些設定缺少，使用常見預設並明確寫出你採用的預設：
   - 語言：繁體中文
   - 題數：8–12 題
   - 題型：多數量表題，搭配 1–2 題開放式問題
   - 語氣：親切、專業
   - 內容類型：通用型客戶滿意度問卷

4. 生成的問卷必須直接可用，並包含這些部分：
   - 標題
   - 前言或填答說明
   - 問題列表
   - 結尾致謝或收尾語

5. 題目設計要符合使用者指定的條件；若使用者指定產業、語氣、題數、題型比例或其他格式限制，就照那些條件調整。

6. 至少包含下列兩種題目：
   - 一題整體滿意度題
   - 一題建議／意見題

7. 若使用者提供的資訊彼此衝突或無法同時滿足，先保留硬性限制，並用一句話說明你省略了什麼。

## 輸出方式

直接輸出問卷成品，不要輸出分析過程、評註、統計結果或後續操作步驟。若使用者沒有指定格式，採用一般條列式問卷格式，讓內容可直接貼到文件、表單或郵件中使用。

## 必須遵守的規則

when the input makes two requirements impossible to meet together (a length limit and 'keep everything'), keep the hard limit and say in one line what you left out — never drop it silently; never invent a fact the input does not give — no name, date, figure or event — and write 'not given' only for such a missing fact; when a setting the work needs is missing (a working-day length, a tone, a format), use the common default, say which one you used, and finish the work rather than stopping; you cannot send, post, schedule, monitor or fetch anything, so when the request asks for that, deliver the content ready to use and say plainly that sending or scheduling is left to the person; and deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.