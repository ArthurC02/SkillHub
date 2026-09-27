---
name: deposit-payment-followup-process
description: Automates the post-deposit workflow for confirmed orders: when a customer has fully paid the deposit, update the order status to 已確認 and prepare the confirmation email. Use this skill when you need a ready-to-use process for handling paid-deposit orders in a company order system.
---

# 客戶付清訂金後處理流程

當你收到一筆「客戶已付清訂金」的訂單資料時，請直接依照以下流程處理，並把結果整理成可執行的處理說明。

## 步驟

1. 取出輸入中的訂單資訊、客戶聯絡方式，以及訂金是否已付清的狀態。
2. 確認該筆訂單屬於已付清訂金的情況後，登入公司訂單系統。
3. 將對應訂單的狀態改成「已確認」。
4. 寄出一封確認信給該訂單對應的客戶。
5. 若輸入中有多筆已付清訂金的訂單，請逐筆重複上述流程，直到全部處理完成。

## 輸出要求

- 請直接輸出每一筆訂單的處理結果，包含：
  - 對應的訂單資訊
  - 已登入訂單系統
  - 已將狀態更新為「已確認」
  - 已寄出確認信給對應客戶
- 若輸入同時提供多筆資料，請逐筆列出，不要只寫通用流程。
- 請保留「已確認」這個狀態名稱，不要改寫。

## 資料不足時

- 如果輸入沒有可識別訂單的資訊，請明確指出缺少哪一項資料。
- 如果輸入沒有客戶聯絡方式，請明確指出無法寄出確認信。
- 如果輸入不是已付清訂金的訂單，請不要執行狀態更新與寄信流程。

## 注意事項

- 這個 Skill 只處理訂單狀態更新與確認信寄送流程。
- 不包含退款、改期或其他例外處理。
- 不要自行補寫未提供的訂單號、姓名、Email 或其他聯絡資料。