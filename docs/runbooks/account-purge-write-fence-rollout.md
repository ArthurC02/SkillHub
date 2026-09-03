# Account purge write fence 上線 Runbook

適用於第一次部署 migration `0049`～`0051` 與相容的新版本。每一步都要保存執行時間、操作者與輸出；任何一步不成立就停止部署。

## 1. Migration 前

1. 停止 account purge 排程、所有舊版 `maintenance`、舊 Worker，以及所有可能產生物件的舊版 API request／instance，確認舊程序與進行中 request 都是零。舊 API 會先寫 Object Store 再寫資料庫，migration trigger 無法保護尚未留下資料庫列的 bytes，因此不可混部滾動更新。
2. 執行資料庫 migration `0049`～`0051`。
3. 部署包含 ADR-062 shared lock／intent 協定的新 API、新 Worker 與新 Maintenance；完成第 3 節修復前不得重新啟動 account purge。禁止回滾成舊版物件 writer 或 Maintenance；若必須回滾，先停止 account purge 排程。

## 2. Worker 排空

1. 停止派送到舊 Worker，確認所有舊 Worker instance 數量為零並記錄證據。
2. 部署只會建立 `object_grants_state = 'unissued'`、且在把已簽發 URL 交付 Provider 前記錄期限的新版 Worker。
3. 從最後一台舊 Worker 停止起等待至少 21 分鐘。這段時間不得執行下一節 SQL。

## 3. 限定修復

在同一 transaction 先檢查再更新。因為此時舊 Worker 已為零且等待窗已滿，而新版只建立 `unissued`，所以必須處理全部仍為 `legacy_unknown` 的 Attempt。migration 寫入的有限期限也可能早於舊 Worker 實際簽發 URL 的時間，不得用 expiry 或外部主機時間縮小候選集。

```sql
BEGIN;

SELECT count(*) AS repair_candidates
FROM run_attempts
WHERE object_grants_state = 'legacy_unknown';

UPDATE run_attempts
SET object_grants_expire_at = now() - interval '2 minutes',
    object_grants_state = 'closed'
WHERE object_grants_state = 'legacy_unknown';

COMMIT;
```

更新 expiry 會由 `0051` trigger 建立 Run Artifact cleanup intent。修復後全域確認 `legacy_unknown` 為零；若仍有，停止 account purge 並調查。

## 4. 歷史孤兒物件對帳

這是一次性上線門檻，不能用 migration 成功取代。舊版 Dataset 與 Download 會先 `Put` 再插入資料庫列，因此當時在兩步之間中斷的 bytes 不會被新 intent 回溯發現。

1. 用 Object Store 原生的 list/inventory 功能導出 `datasets/`、`downloads/` 與 `runs/` 三個 prefix 的完整 key 清單，保存包含產生時間、bucket/version 與筆數的證據。盤點期間保持舊 writer 為零，只讓新版 writer 運行。
2. 匯出資料庫所有合法引用：`datasets.object_key`、`artifacts.object_key`、三種 `*_object_cleanup_intents.object_key`，以及所有 Run Attempt 依現行規則可導出的 archive key。
3. 以完整 key 做差集：Object Store 有、但資料庫引用與 cleanup intent 都沒有的 key 就是孤兒候選。不可用數量相等代替 key-by-key 對帳。
4. 逐筆確認候選不屬於進行中寫入後，為它建立對應 cleanup intent 並由新版 Reconciler 清除，或依當值變更程序直接刪除；保存 key、方式、時間與結果。
5. 重跑 inventory 與差集，只有未配對 key 為零，且新版 Reconciler 已將所有回填 intent 收旂，才能進入完成條件。如果 Object Store 無法產生完整 inventory，停止上線，不可抽樣代替。

## 5. 完成條件

- 舊版 Maintenance、Worker、API instance 與進行中物件寫入 request 都是零。
- 等待窗已滿 21 分鐘。
- 限定修復已完成，舊候選數為零。
- 三個 Object Store prefix 已完成 key-by-key 對帳，歷史孤兒候選已處理，未配對 key 為零且證據已存檔。
- 新版 Maintenance 的 object reconciliation 與 account purge 各成功跑過一輪。
