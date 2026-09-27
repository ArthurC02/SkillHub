---
name: weekly-performance-broadcast
description: Use when you need to prepare and send a weekly performance summary to an entire department by email and Slack on a recurring Friday afternoon schedule. This skill covers the workflow, checks, and message preparation, but it cannot itself schedule jobs or send messages without the required connected tools and access.
---

# 每週業績公告流程

此技能用於「每週五下午自動把本週業績數字寄給全部門，同時在 Slack 公告」這類需求。

## 先確認可執行性
這個任務通常需要以下能力：
- 讀取本週業績數字的來源（報表、資料庫、試算表或 BI 工具）
- 寄送群組電子郵件
- 發送 Slack 訊息到指定頻道
- 排程在每週五下午自動執行

如果目前沒有這些工具、權限或排程機制，先不要假設可以自動完成；改為收集缺少的資訊並回報需要哪些整合。

## 執行步驟
1. **確認資料來源**
   - 找出本週業績數字的唯一來源。
   - 確認統計區間是「本週」的定義：例如週一 00:00 到週五下午寄送前的時間點，或依公司財務週期。
   - 確認要公告的指標：例如營收、訂單數、轉換率、達成率、與上週比較。

2. **整理內容**
   - 以簡短、可掃讀的格式整理數字。
   - 若有必要，加入：
     - 本週總結
     - 與上週或目標的比較
     - 重要備註或異常說明
   - 避免放入未經確認的推測。

3. **確認收件與發布對象**
   - 取得全部門郵件群組或通訊錄清單。
   - 取得 Slack 發布頻道名稱或頻道 ID。
   - 確認是否需要抄送主管、是否需要匿名化或限制敏感資訊。

4. **產生兩份訊息**
   - **Email 版本**：標題清楚標示週次與主題，例如「本週業績更新（YYYY-MM-DD）」。
   - **Slack 版本**：更精簡，重點數字優先，必要時附上完整報表連結。

5. **發送前檢查**
   - 數字是否與來源一致。
   - 日期區間是否正確。
   - 收件人與 Slack 頻道是否正確。
   - 是否包含敏感資訊或未授權內容。

6. **發送**
   - 寄送 Email 給全部門。
   - 同步在 Slack 公告相同或精簡版內容。
   - 若系統支援，記錄發送時間與結果。

7. **失敗處理**
   - 若資料來源無法讀取，先停止發送並回報缺失。
   - 若 Email 或 Slack 任一管道失敗，記錄失敗原因並重試一次；仍失敗則通知負責人。
   - 若排程失效，改為手動提醒並要求修復排程設定。

## 建議的訊息格式
### Email
主旨：本週業績更新（YYYY-MM-DD）

內文：
- 本週營收：XXX
- 本週訂單數：XXX
- 達成率：XX%
- 與上週相比：+/-XX%
- 備註：...

### Slack
本週業績更新：
- 營收 XXX
- 訂單數 XXX
- 達成率 XX%
- 詳細報表：<連結>

## 若缺少自動化能力
如果目前沒有可用的排程、Email 或 Slack 發送工具，這個技能只能協助你：
- 整理每週公告模板
- 列出需要的整合項目
- 檢查發送前的內容

它不能憑空建立排程、登入外部服務或代替你完成實際發送。
