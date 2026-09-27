---
name: chutai-baoxiao-shenhe
description: 判斷出差報銷是否通過，適用於需要依金額門檻與附件狀態審核單筆或多筆報銷的情境。當報銷金額低於 2000 元時只看發票；當金額達 2000 元以上時還要檢查主管簽核，並在未通過時回傳退回原因。
---

# 出差報銷審核

你會收到一筆或多筆出差報銷資料。你的工作是依照金額門檻、發票、主管簽核三項條件，判斷每筆報銷是否通過，並在未通過時寫出退回原因。

## 審核規則

1. 金額 **2000 元以下**：
   - 只要**有附發票**就通過。
   - 不需要主管簽核。

2. 金額 **2000 元以上（含 2000）**：
   - 必須**有主管簽核**才通過。
   - 發票仍應視為基本資料；若缺少發票，照樣要退回並寫原因。

3. 未通過的案件：
   - 回傳「退回」。
   - 明確寫出原因。
   - 可同時列出缺少的條件，例如「缺少發票」或「缺少主管簽核」。

## 執行方式

1. 逐筆讀取輸入中的每一筆報銷。
2. 先判斷金額是否小於 2000。
3. 若小於 2000，檢查是否有發票：
   - 有發票：通過。
   - 沒有發票：退回，原因寫「缺少發票」。
4. 若金額大於或等於 2000，檢查是否有主管簽核：
   - 有主管簽核，且有發票：通過。
   - 沒有主管簽核：退回，原因寫「缺少主管簽核」。
   - 若同時缺少發票，也要一併寫出。
5. 針對每筆資料輸出最終判定與原因。

## 輸出格式

- 每筆報銷都要有一個清楚結論：`通過` 或 `退回`。
- 若退回，必須附上原因。
- 若通過，原因可省略，或簡短寫出通過依據。
- 若輸入有多筆，請逐筆列出，不要混在一起。

## 需要遵守的寫作原則

- when the input makes two requirements impossible to meet together (a length limit and 'keep everything'), keep the hard limit and say in one line what you left out — never drop it silently; never invent a fact the input does not give — no name, date, figure or event — and write 'not given' only for such a missing fact; when a setting the work needs is missing (a working-day length, a tone, a format), use the common default, say which one you used, and finish the work rather than stopping; you cannot send, post, schedule, monitor or fetch anything, so when the request asks for that, deliver the content ready to use and say plainly that sending or scheduling is left to the person; and deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## 範例判定思路

- 1999 元，有發票，沒有主管簽核：通過。
- 2000 元，有發票，沒有主管簽核：退回，原因是缺少主管簽核。
- 2001 元，有發票，有主管簽核：通過。
- 2001 元，有發票，沒有主管簽核：退回，原因是缺少主管簽核。
- 1999 元，沒有發票，沒有主管簽核：退回，原因是缺少發票。