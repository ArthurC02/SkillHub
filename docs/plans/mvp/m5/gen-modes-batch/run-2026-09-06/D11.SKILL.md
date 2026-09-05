---
name: instagram-post-scheduler
description: Use this skill when you need to prepare a social media post draft for Instagram from a short article excerpt shown in a flowchart. It checks the length, adds hashtags, and schedules the post in the order shown.
---

# Instagram 貼文排程流程

依照圖中的流程處理收到的社群貼文文章稿：

1. **收到社群貼文文章稿**
   - 先確認輸入內容是要發布的貼文草稿。
   - 若內容不完整，先整理成可發布的短文版本。

2. **檢查字數不超過 150 字**
   - 計算貼文正文的字數。
   - 若超過 150 字，先刪減、濃縮或改寫，直到正文不超過 150 字。
   - 保留核心訊息、行動呼籲與必要資訊。

3. **加上 3 個 hashtag**
   - 在貼文末尾加入 3 個與內容高度相關的 hashtag。
   - hashtag 應簡短、明確、與主題一致。
   - 避免加入無關或過多標籤；只保留 3 個。

4. **排程到 Instagram**
   - 將完成的貼文交付為 Instagram 發布版本。
   - 若系統支援排程，設定發布時間；若不支援，則輸出可直接排程的最終文案。

## 輸出要求
- 最終內容必須是可直接用於 Instagram 的貼文。
- 正文不超過 150 字。
- 末尾固定包含 3 個 hashtag。
- 不要加入流程說明、分析或多餘註解。
