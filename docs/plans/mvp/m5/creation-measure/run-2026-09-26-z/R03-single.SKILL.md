---
name: daily-news-summary-email
description: Use when you need a daily digest of headlines from three news sites condensed into five summary bullets and sent by email. This skill covers gathering the headlines, selecting the most important items, writing the summary, and preparing the email content; it does not perform live browsing or send mail unless those tools are available.
---

# 每日新聞摘要寄送

## 這個技能要做什麼
將三個新聞網站的標題整理成 5 條摘要，並準備成可寄送到收件信箱的每日早報內容。

## 先確認的事項
如果以下資訊沒有提供，先向使用者確認：
1. 三個新聞網站的網址或名稱。
2. 收件人信箱地址。
3. 寄送時間與時區。
4. 是否有偏好的語言、摘要風格或主題排序。

## 執行流程
1. **取得當日標題**
   - 依序查看三個新聞網站的首頁、新聞列表頁或 RSS/公開摘要頁。
   - 只使用公開可見的標題與簡短導語；不要臆測內文。
   - 若某網站無法存取，記錄原因並改用可取得的公開頁面。

2. **整理與去重**
   - 合併三個來源的標題。
   - 移除重複或高度相似的標題。
   - 優先保留最具新聞價值、影響範圍較大、或最能代表當日重點的項目。

3. **寫成 5 條摘要**
   - 將標題改寫成 5 條精簡摘要。
   - 每條摘要應清楚、客觀、可獨立理解。
   - 若當日可用標題少於 5 則，則用現有內容寫出少於 5 條，不要硬湊。
   - 若標題很多，則按重要性排序，只保留最重要的 5 條。

4. **準備寄信內容**
   - 主旨建議格式：`每日新聞摘要 - YYYY-MM-DD`
   - 內文包含：
     - 簡短開頭
     - 5 條摘要清單
     - 三個來源名稱
     - 生成時間
   - 保持中性，不加入未驗證評論。

5. **寄送前檢查**
   - 確認日期正確。
   - 確認收件人信箱正確。
   - 確認沒有把未經證實的內容寫進摘要。

## 如果沒有可用的網路或寄信工具
這個技能無法自行完成即時抓取新聞或實際寄信。此時應：
- 明確告知需要可用的瀏覽與寄信能力。
- 先產出可寄送的摘要草稿與郵件內容。
- 列出還缺少的資訊或工具。

## 輸出格式建議
當無法直接寄送時，輸出以下內容：
- 日期
- 三個來源
- 5 條摘要
- 郵件主旨
- 郵件內文
- 待確認事項（如有）
