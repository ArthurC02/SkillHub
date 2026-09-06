---
name: server-log-error-triage
description: Use when a process flow shows how to handle server log errors, especially to decide whether to escalate after filtering ERROR lines and counting incidents. It turns the flowchart into step-by-step operational instructions.
---

# 伺服器 log 錯誤處理流程

依照下列流程處理伺服器 log：

1. **讀取伺服器 log 檔**
   - 取得指定時間範圍或指定來源的 log 檔。
   - 若有多個檔案，先合併或逐一檢視，確保不漏掉同一事件的相關紀錄。

2. **篩出 `ERROR` 行**
   - 只保留包含 `ERROR` 的紀錄行。
   - 若需要，可同時保留時間戳、主機名、服務名稱、錯誤訊息與 request id，方便後續追蹤。

3. **判斷錯誤是否超過 10 筆**
   - 計算篩出的 `ERROR` 行數。
   - 以「超過 10 筆」作為分流條件。

4. **若錯誤超過 10 筆：建立 Jira 問題單**
   - 建立一張 Jira issue，內容至少包含：
     - 錯誤摘要
     - 錯誤筆數
     - 影響服務或系統名稱
     - 發生時間範圍
     - 代表性的 `ERROR` 範例行
     - 初步判斷與已知影響
   - 若可辨識重複模式，將相同錯誤歸類後再描述。

5. **通知值班工程師**
   - 將 Jira 單號、錯誤摘要、嚴重程度與是否持續發生通知值班工程師。
   - 通知內容要能讓接手者快速判斷是否需要立即處理。

6. **寫入每日摘要**
   - 不論是否超過 10 筆，都要將結果寫入每日摘要。
   - 每日摘要至少記錄：
     - 日期
     - `ERROR` 筆數
     - 是否已建立 Jira
     - 是否已通知值班工程師
     - 重要觀察或後續追蹤事項

7. **若錯誤不超過 10 筆：直接寫入每日摘要**
   - 不建立 Jira。
   - 不通知值班工程師，除非其他內部規範另有要求。
   - 仍需在每日摘要中記錄錯誤內容與筆數，方便日後回溯。

8. **輸出結果時保持一致格式**
   - 用同一套欄位記錄每次處理結果，避免摘要與 Jira 內容不一致。
   - 若資訊不足，先列出缺少的欄位，再繼續補齊可取得的資料。
