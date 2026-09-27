---
name: weekly-performance-broadcast
description: Use when you need to automate a weekly Friday afternoon report that emails the week’s performance numbers to an entire department and posts the same announcement in Slack. This skill only covers planning, data gathering, message drafting, and automation setup guidance; it cannot send messages without access to the required email/Slack systems and permissions.
---

# 每週五下午業績公告流程

## 先確認這件事是否可執行
這個需求需要能存取：
- 業績數字來源（例如 CRM、BI 報表、試算表、資料庫）
- 部門群組的寄信名單或郵件群組
- Slack 工作區與要公告的頻道
- 排程工具或自動化平台（例如 cron、Zapier、Make、Power Automate、Apps Script、Slack workflow、郵件排程）

如果其中任何一項沒有權限或登入，先不要假設可自動完成；改為收集缺少的資訊並說明需要哪些存取權限。

## 執行步驟
1. **確認報告範圍**
   - 定義「本週」的時間區間，通常是週一 00:00 到週五下午寄送前的最新資料。
   - 確認業績指標：例如營收、訂單數、轉換率、達成率、與上週比較。
   - 確認是否需要附上明細、圖表或只要摘要。

2. **確認收件與公告對象**
   - 取得全部門的 email 群組或收件名單。
   - 取得 Slack 頻道名稱或頻道 ID。
   - 確認是否需要抄送主管、是否允許外部收件者。

3. **取得本週業績數字**
   - 從指定來源抓取最新數據。
   - 若資料來源有延遲，記錄資料截止時間。
   - 檢查數字是否完整、是否與前一週口徑一致。

4. **整理成一致的公告內容**
   - 先寫一版簡短摘要：
     - 本週總結
     - 主要數字
     - 與上週或目標的差異
     - 必要的補充說明
   - 讓 email 與 Slack 使用同一份核心內容，但格式可不同：
     - Email 可較完整
     - Slack 版要更短、更易掃讀

5. **檢查內容品質**
   - 核對所有數字、單位、百分比與日期。
   - 確認沒有洩漏不該公開的個資、客戶資料或敏感資訊。
   - 確認語氣專業、清楚、無歧義。

6. **設定寄送與公告時間**
   - 排程在每週五下午執行。
   - 若有明確時間，使用該時間；若沒有，先向使用者確認，例如 15:00 或 17:00。
   - 若遇到假日或資料尚未更新，定義是否順延或照常發送。

7. **發送 Email**
   - 主旨建議包含日期與週次，例如「本週業績更新｜YYYY/MM/DD」。
   - 收件者使用部門群組或完整名單。
   - 內文包含：
     - 簡短開頭
     - 本週業績數字
     - 與上週/目標比較
     - 補充說明與聯絡窗口
   - 發送前先預覽，確認收件者與內容無誤。

8. **在 Slack 公告**
   - 發到指定頻道。
   - 內容保持精簡，重點放在數字與結論。
   - 若需要，可附上報表連結或完整 email 連結。
   - 使用一致的日期標示，避免與 email 內容不一致。

9. **記錄與追蹤**
   - 記錄發送時間、收件名單、Slack 頻道、使用的數據版本。
   - 若發送失敗，保留錯誤訊息並重試或通知負責人。
   - 若數字後續修正，補發更正公告並標明版本。

## 建議的訊息格式
### Email
- 主旨：`本週業績更新｜YYYY/MM/DD`
- 內文：
  - 各位同仁好，
  - 以下為本週業績摘要：
  - 指標 1：數值
  - 指標 2：數值
  - 與上週比較：差異
  - 備註：資料截止時間 / 重要說明
  - 謝謝

### Slack
- `本週業績更新（YYYY/MM/DD）：`
- `- 指標 1：數值`
- `- 指標 2：數值`
- `- 與上週比較：差異`
- `完整內容已寄送 email。`

## 如果無法自動化
如果沒有 email、Slack 或資料來源的存取權限，這個需求不能直接完成自動發送。此時應改為：
- 列出缺少的系統與權限
- 提供可手動執行的寄送與公告模板
- 請使用者補齊登入、群組名單、頻道與資料來源後再建立自動化
