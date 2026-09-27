---
name: weekly-sales-report-broadcast
description: Use when you need to automate a recurring weekly sales-performance update that is emailed to a department and posted in Slack. This skill covers the planning, data gathering, message drafting, and scheduling steps, but it cannot itself send messages unless the required email/Slack access and automation tools are available.
---

# 每週業績公告流程

這個任務涉及定期寄信與 Slack 公告；如果目前沒有可用的郵件、Slack 或排程工具，請先確認可用性，再執行下列流程。若沒有登入權限或自動化工具，這個技能只能協助你整理內容與確認所需資訊，不能直接完成發送。

## 1. 確認固定規則
- 確認「每週五下午」的具體時間與時區。
- 確認收件範圍：是否為全部門名單、群組信箱，或特定 Slack 頻道。
- 確認本週業績數字的來源系統與口徑：
  - 使用哪個報表或儀表板
  - 統計區間是否為週一到週五
  - 是否以已入帳、已成交、或其他定義為準
- 確認公告內容是否需要包含：
  - 本週總業績
  - 與上週比較
  - 與目標比較
  - Top items / 重點說明
  - 附件或報表連結

## 2. 取得本週數字
- 從指定來源擷取本週業績數字。
- 檢查數字是否完整、是否與上週或其他報表一致。
- 若數字有異常，先回頭確認資料來源與統計條件，再繼續。

## 3. 準備兩份訊息
### Email 版本
- 主旨要清楚標示週次與主題，例如：`本週業績更新｜YYYY/MM/DD–YYYY/MM/DD`
- 內文建議包含：
  - 簡短開頭
  - 本週業績數字
  - 必要的比較資訊
  - 相關說明或連結
  - 結尾致意

### Slack 版本
- 內容要比 email 更精簡。
- 建議包含：
  - 一句摘要
  - 核心數字
  - 連結到完整報表或 email
- 若公司習慣，可加上 @channel 或指定公告格式；但只有在團隊規範允許時才使用。

## 4. 發送前檢查
- 確認收件人與 Slack 頻道正確。
- 確認數字、日期、單位一致。
- 確認沒有把敏感資訊發到不該看的範圍。
- 確認附件或連結可開啟。

## 5. 發送與公告
- 在週五下午依排程寄出 email。
- 同步在 Slack 公告相同週次的重點。
- 若使用自動化流程，確認排程已建立且下次執行時間正確。

## 6. 發送後記錄
- 記錄本次發送時間、收件範圍、Slack 頻道、以及使用的數字版本。
- 若有失敗或延遲，記下原因並通知負責人。

## 7. 若無法自動化時
- 先整理成可直接貼上的 email 與 Slack 文案。
- 列出需要人工完成的步驟：
  1. 取得最新業績數字
  2. 寄送 email 給全部門
  3. 在 Slack 公告
  4. 確認送達與回覆

## 8. 建議的輸出格式
- `週次：`
- `統計區間：`
- `本週業績：`
- `與上週比較：`
- `與目標比較：`
- `備註：`

依照以上流程，先確認資料來源與發送管道，再產出可直接寄送與公告的內容。
