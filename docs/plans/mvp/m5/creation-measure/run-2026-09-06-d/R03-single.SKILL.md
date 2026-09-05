---
name: daily-news-summary-email
description: Use when you need to collect headlines from three news websites each morning, condense them into five concise summaries, and send them by email. This skill is for planning and drafting the workflow; if live website access or email sending is unavailable, it should gather the needed inputs and explain the limitation.
---

# 每日新聞標題摘要寄信

## 這個技能要做什麼
每天早上從三個新聞網站收集標題，整理成 5 條摘要，並寄到指定信箱。

## 先確認的資訊
如果以下資訊不完整，先向使用者確認：
- 三個新聞網站的網址
- 收件人信箱地址
- 寄送時間與時區
- 摘要語言與語氣（例如：繁體中文、簡潔中性）
- 是否只看首頁標題，或包含特定版面/分類
- 是否需要保留原始標題連結

## 執行流程
1. 在每天早上指定時間開始任務。
2. 依序開啟三個新聞網站，擷取當日可見的主要標題。
3. 去除重複、過度相似或明顯非新聞內容的標題。
4. 綜合三個網站的標題，整理成 5 條摘要：
   - 每條摘要應簡短、清楚、可獨立理解。
   - 優先涵蓋不同主題，避免 5 條都來自同一事件。
   - 若某網站標題較多，可適度提高其權重，但仍以多樣性為主。
5. 撰寫郵件內容：
   - 主旨：每日新聞摘要（日期）
   - 內文：5 條摘要，必要時附上來源網站名稱與原始標題
   - 若使用者要求，附上每條摘要的原始連結
6. 寄送到指定信箱。

## 品質要求
- 摘要要忠於原標題，不要加入未確認的事實。
- 若標題資訊不足，使用保守措辭，例如「某公司宣布…」、「某地發生…」。
- 若三個網站當天可用標題少於 5 條，仍寄出，並明確說明可取得的標題數量不足。
- 若網站內容無法存取、被封鎖、或需要登入，記錄原因並通知使用者需要替代來源或手動提供標題。

## 無法自動完成時怎麼做
這個任務需要能讀取網站內容並寄送電子郵件；如果目前環境沒有這些能力，就不要假裝已完成。改為：
- 列出你需要的三個網站網址與收件人信箱
- 說明目前缺少哪一步（例如無法抓取網頁、無法發信）
- 提供可直接貼上的摘要草稿格式，讓使用者手動寄出
