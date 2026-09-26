---
name: weekly-sales-announcement
description: 將使用者提供的本週業績資料整理成繁體中文電子郵件草稿與 Slack 公告。當你需要準備每週業績通知、但不應直接存取外部資料、寄信、發布 Slack 或執行排程時使用。
---

# 每週業績公告

請將使用者輸入的業績資料整理成兩個可直接複製使用的繁體中文草稿：一封電子郵件，以及一則 Slack 公告。只根據本次輸入內容產出，不執行外部資料讀取、排程、寄信或 Slack 發布。

## 處理步驟

1. 讀取輸入中的統計期間、業績數字、電子郵件收件對象與 Slack 頻道。
2. 若輸入未提供必要的業績數字，指出缺少哪些資料，且不自行捏造數字；仍只輸出可根據輸入完成的內容。
3. 產生電子郵件草稿，使用繁體中文，並包含：
   - 主旨
   - 收件對象
   - 統計期間
   - 輸入中提供的每一項業績數字
4. 產生 Slack 公告，使用繁體中文，並包含：
   - 發布頻道
   - 統計期間
   - 與電子郵件相同的每一項業績數字
5. 明確標示兩者都是待發送或待發布草稿，不聲稱已寄信或已發布 Slack。
6. 直接交付完成的電子郵件草稿與 Slack 公告，不提出後續問題。

use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent;

deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## 輸出格式

### 電子郵件草稿（待發送）

- 主旨：使用輸入內容擬定；若未提供可辨識的主旨資訊，寫「not given」。
- 收件對象：使用輸入內容中的部門或收件人範圍。
- 統計期間：照輸入原文呈現；若未提供，寫「not given」。
- 業績摘要：逐項列出輸入提供的數字與單位。

### Slack 公告（待發布）

- 發布頻道：照輸入原文呈現；若未提供，寫「not given」。
- 統計期間：照輸入原文呈現；若未提供，寫「not given」。
- 業績摘要：逐項列出與電子郵件相同的輸入數字與單位。

不得加入輸入未提供的日期、姓名、收件人、頻道、業績數字、比較結果、原因、結論或行動要求。