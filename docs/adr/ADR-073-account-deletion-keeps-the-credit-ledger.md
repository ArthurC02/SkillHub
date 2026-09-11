# ADR-073：帳號刪除不清 Credit 紀錄

- 狀態：**Accepted**（2026-09-11 負責人裁定「全部保留」，[`05` R-75](../plans/05-pending-rulings.md)）
- 日期：2026-09-11
- 相關：[ADR-068](./ADR-068-credit-is-the-only-unit-of-account.md) 決策 11（本份**只取代帳號刪除那一半**，保存期限那一半與 ADR-068 其餘決策不變、不 Superseded）、`02` CRED-008、[同意書與資料保存](../plans/mvp/gate-test/consent-and-data-policy.md)

## 背景

ADR-068 決策 11 把 `cost_events` 與 `credit_entries` 當成一般使用者資料：併入保存期限對帳，帳號刪除時也一起清。實作是帳號清除流程的最後一步 `credit`，刪掉這個帳號的扣點分錄、成本事件與會話摘要。

2026-09-11 負責人說「清帳號，不要碰Credit資料」，問過範圍之後選「全部保留」。

## 決策

### 1. 帳號清除不碰任何 Credit 紀錄

`credit_accounts`、`credit_entries`、`cost_events`、`cost_session_summaries` 在帳號清除後原樣保留。清除流程不再有 `credit` 那一步，只為它存在的三條刪除 query 一併移除。

### 2. 保存期限照舊

時間視窗的保存掃描（`maintenance purge-credit`，期限 `CREDIT_RETENTION`）不變：紀錄在期滿時照樣清掉，只是不因帳號刪除而提前清。`CREDIT_RETENTION` 的值仍待裁定，未設時掃描拒絕啟動，所以在那之前紀錄不會被清。

### 3. 紀錄仍指得回那個帳號

`users` 那一列在清除後不刪、只標記 `deleted_at`，所以外鍵不受影響，紀錄裡的 `user_id`／`workspace_id` 仍指向它。`cost_events` 不存查詢原文（ADR-068 決策 3），這些紀錄只有金額、token 數、模型名稱與時間，沒有使用者輸入的文字。

## 影響

- `02` CRED-008 第二條允收準則改寫：清除之後這些紀錄仍在。
- 同意書的保存表新增一列揭露這件事，並標明待法務確認。
- 一條整合測試守這件事：刪除帳號並跑完清除之後，四張表裡這個帳號的列數不變；讓清除流程再刪一次扣點分錄，它就會紅。

## 考慮過的替代方案

- **只保留 `credit_accounts`**（負責人第一句話的字面範圍）：扣點分錄、成本事件與會話摘要照舊刪。負責人問過之後沒有選。
- **保留但去識別化**（把 `user_id` 設為空）：`credit_entries.user_id` 是指向 `credit_accounts` 的必填外鍵，要改 schema；而負責人要的是不碰。

## 2026-09-12 補記：決策 2 的待裁值是「永遠不清」，保存掃描因此移除

[`05` R-76](../plans/05-pending-rulings.md)：負責人裁定 Credit 紀錄永遠不清。決策 2 的原文不改，答案記在這裡。

- 值是「永遠」，時間視窗的保存掃描就沒有事可做：`maintenance purge-credit`、按時間刪除的三條 query 與 `CREDIT_RETENTION` 一併移除。只把值留空也不會清，但 release-checklist 要求把它接上 cron，照著部署的人會填值；移除之後，沒有任何一條路徑刪這幾張表。
- `cost_events` 與 `credit_entries` 是不可變表，`db/query-owners.yaml` 的 `immutable_allow` 不再列任何刪除它們的 query，新增一條會被 query-owners 檢查擋下。
- 本份因此取代 ADR-068 決策 11 的整條：帳號刪除那一半（決策 1），與保存期限那一半（本補記）。
