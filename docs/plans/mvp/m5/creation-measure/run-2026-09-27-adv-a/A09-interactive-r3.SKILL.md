---
name: attendance-overtime-46-hours
description: Calculate each person's overtime hours from a provided monthly attendance record and flag anyone whose total exceeds 46 hours. Use this skill when you need a direct per-person overtime summary from attendance data you are given.
---

# 目的
根據使用者提供的當月出勤紀錄，計算每個人的加班時數，並標出總加班時數超過 46 小時的人。

# 執行方式
1. 先列出輸入中出現的所有人員，再依每人可得資料計算加班時數；不得因某欄不足而省略人員。
2. 只使用輸入中明確提供的欄位與規則計算；若輸入沒有提供足以計算加班的基準，仍要列出所有人員與可直接由輸入得到的欄位，並把加班時數欄位標成「無法計算」，不要自行補任何假設。即使缺少加班基準，也要先輸出輸入中每位人員的加班時數欄位；若無法由輸入唯一算出，欄位值寫成「無法計算」。
3. 依每位人員彙總當月加班時數。
4. 找出總加班時數超過 46 小時的人，並明確標示。
5. 輸出每位人員的加班時數清單；未超過 46 小時的人也要列出，且在超過 46 小時欄位明確顯示「否」；只有超過者顯示「是」。

# 輸出要求
- 逐一列出輸入中出現的每位人員。
- 顯示每人的加班時數。
- 對總加班時數超過 46 小時的人加上明確標示。
- 即使某人的加班時數無法唯一計算，也要保留該人的列；若能計算且未超過 46 小時，超標欄位必須顯示「否」。
- 只根據輸入資料計算，不加入輸入外的新資訊。

# 標示規則
- 只要某人已能依輸入內的規則算出總加班時數，且其值大於 46 小時，就在同一列明確標示為超過。
- 對每位已能計算出加班時數的人員，若總加班時數大於 46 小時，就在同一列標示為「是」；否則標示為「否」。
- 若輸入沒有提供足以計算加班的基準，仍要保留所有人員列與超過 46 小時標示欄位，標示內容改為「無法判定」，不得省略。

# 執行準則
- use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent;
- deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access。

# 注意
如果輸入缺少計算所需的出勤紀錄，仍要先列出輸入中出現的所有人員與可取得的原始欄位，再標示哪些欄位不足以完成加班計算；不要因為缺少基準就省略人員。