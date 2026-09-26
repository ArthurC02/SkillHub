---
name: calculate-overtime-hours
description: 當使用者提供包含姓名、日期、實際工時與應工作時數的 CSV 或 Markdown 出勤表格，且需要計算每人當月加班時數並標示超過 46 小時的人員時使用。
---

## 任務

處理使用者貼上的 CSV 或 Markdown 出勤表格，計算每位人員的當月加班總時數，並標示總時數嚴格超過 46 小時的人員。

use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent;

deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## 執行步驟

1. 讀取輸入中的出勤表格。預期欄位為「姓名」、「日期」、「實際工時」與「應工作時數」；接受直接貼上的 CSV 或 Markdown 表格。日期只用於辨識輸入中的紀錄，不自行補日期或改變資料範圍。工時單位採用輸入所指定的單位；若輸入未指定，依已確認規則視為小時，並在結果中以小時呈現。

2. 對每一筆出勤紀錄計算單日加班時數：`max(實際工時 − 應工作時數, 0)`。實際工時不超過應工作時數的紀錄，其單日加班時數為 0。不要加入輸入中沒有的休息扣除、四捨五入、倍率或其他計算規則。

3. 依輸入中出現的姓名彙總其所有紀錄的單日加班時數，得到每人的當月加班總時數。保留輸入中每位人員各自一列，即使其加班總時數為 0。

4. 將加班總時數嚴格大於 46 小時的人員標示為「超過 46 小時」；加班總時數等於 46 小時或低於 46 小時者標示為「未超過 46 小時」。46 小時本身不得標示為超過 46 小時。

5. 直接輸出完成的 Markdown 表格，不輸出計算計畫或規則說明。表格欄位固定為「姓名」、「加班總時數（小時）」與「是否超過 46 小時」，並列出輸入中的所有人員。依姓名首次在輸入中出現的順序排列；輸入沒有提供的姓名、日期或數值寫 `not given`，不得自行填補。

## 輸入缺漏

只有在使用者完全沒有提供可計算的出勤表格時，才指出缺少輸入資料；有表格但某欄位的值未提供時，照上述規則在相應結果中寫 `not given`，不要捏造資料或提出額外問題。