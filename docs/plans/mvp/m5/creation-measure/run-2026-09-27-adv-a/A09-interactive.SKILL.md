---
name: attendance-overtime-46-hours
description: Calculate each person's overtime hours from a provided monthly attendance record and flag anyone whose total exceeds 46 hours. Use this skill when you need a direct per-person overtime summary from attendance data you are given.
---

# 目的
根據使用者提供的當月出勤紀錄，計算每個人的加班時數，並標出總加班時數超過 46 小時的人。

# 執行方式
1. 讀取使用者提供的出勤紀錄。
2. 只使用輸入內的資料計算，不查外部資料。
3. 依每位人員彙總當月加班時數。
4. 找出總加班時數超過 46 小時的人，並明確標示。
5. 輸出每位人員的加班時數清單；未超過 46 小時的人也要列出，但不要標示為超過。

# 輸出要求
- 逐一列出輸入中出現的每位人員。
- 顯示每人的加班時數。
- 對總加班時數超過 46 小時的人加上明確標示。
- 只根據輸入資料計算，不加入輸入外的新資訊。

# 執行準則
- use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent;
- deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

# 注意
如果輸入缺少計算所需的出勤紀錄，就只能回覆資料不足，並說明缺少哪些欄位或紀錄。