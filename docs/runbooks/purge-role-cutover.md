# 清除工作切到 skillhub_purge 角色 Runbook

適用於部署 migration `0059`（`skillhub_purge` 角色）之後，把 `cmd/maintenance` 的清除工作從 API 角色換成最小權限角色。R-25、OWASP GenAI LLM01 的縱深防禦立場都是同一句話：清除工作只該有它需要的權限，應用角色不該對每張表都有 `DELETE`。`0059` 只建立角色與授權，不指派給任何登入帳號、也不 revoke 任何東西——那兩步都是這份 runbook 的操作者動作。

每一步都要保存執行時間、操作者與輸出；任何一步不成立就停止，不要跳到下一步。

## 0. 前置確認

1. 確認 `0059_skillhub_purge_role.sql` 已在目標環境套用（`skillhub_purge` 角色存在，且擁有本檔列出的 GRANT）。
2. 確認正在跑的是包含本批修改的 `maintenance` 版本（讀取 `SKILLHUB_PURGE_DATABASE_URL`，見 `apps/platform/cmd/maintenance/main.go` 的 `purgeDatabaseURL`）。
3. 記下目前 `maintenance` 用哪個登入帳號連線（也就是現在的 `DATABASE_URL` 或部署密鑰管理裡對應的值）——回退時要用同一個帳號。

## 1. 切到新角色

1. 建立（或挑選）一個**只給清除工作用**的登入帳號，例如 `skillhub_purge_login`；不要重用 API 或 Worker 的登入帳號，否則角色權限的邊界形同虛設。
2. 對這個登入帳號執行：
   ```sql
   GRANT skillhub_purge TO skillhub_purge_login;
   ```
3. `rotate-partitions` 不在 `0059` 的授權範圍內——它對 `trace_events`／`analytics_events` 的月分區做 `CREATE TABLE`／`DROP TABLE`，這是 schema 層級的 DDL，不是這個角色要有的表層級 `SELECT`／`DELETE`。兩個選項擇一：
   - （建議）繼續讓 `rotate-partitions` 用現有的 `DATABASE_URL`／API 角色連線，不要把它排進走 `SKILLHUB_PURGE_DATABASE_URL` 的那次呼叫；或
   - 額外對 `skillhub_purge_login` 授予這兩張表的 `CREATE`／建表權限（依部署的 schema 擁有權模型決定怎麼授予），並在本檔備註中記下你做了什麼。
   `cmd/maintenance` 目前是一個 process 一個 pool，八個子命令加 `rotate-partitions` 共用同一條連線字串；沒有依子命令切換連線字串的程式碼，所以哪個環境變數指到哪個帳號，是部署設定的責任，不是程式的。
4. 在部署設定（cron、secret、環境變數）裡把 `SKILLHUB_PURGE_DATABASE_URL` 指到 `skillhub_purge_login` 的連線字串。`DATABASE_URL` 維持不動——它仍是 API／Worker 在用的角色，這一步不影響它們。

## 2. 驗證

在關掉 API 角色的 `DELETE` 之前，下面每一項都要有證據（時間、輸出、操作者），缺一項就不算完成，不能靠「應該沒問題」跳過：

1. **啟動檢查**：`maintenance` 任一子命令的啟動日誌**不是** `SKILLHUB_PURGE_DATABASE_URL not set; purging under the API role (DATABASE_URL)`——沒有這行代表確實吃到新的環境變數。
2. **七個子命令逐一跑過一輪成功**，且輸出的計數合理（不是因為權限不足而回傳 0 或直接失敗）：
   `purge-accounts`、`purge-audit`、`purge-feedback`、`purge-run-artifacts`、`purge-datasets`、`purge-deleted-skills`、`collect-objects`、`check-sources`。
3. 任何一個子命令若回報 Postgres `42501 permission denied`，代表 `0059` 的授權清單漏了某張表或某個動詞（`SELECT`／`INSERT`／`UPDATE`／`DELETE`）——回去改 migration 補授權，重新從第 0 步開始，不要在這個角色上手動 `GRANT` 補丁而不回頭改 migration，否則下一個環境重建資料庫時會漏掉。
4. 確認 API／Worker 的既有刪除路徑（例如使用者自己刪除 Dataset、刪除 Skill、登出清 session）在這段驗證期間仍然正常——它們此時仍走 API 角色，第 3 節之前不受影響，但值得跑一次確認沒有被步驟 1 的部署變更意外波及。

## 3. 驗證過了才 revoke

只有第 2 節四項都留下證據之後，才進行這一步；理由是：任何一個現存部署，只要 `maintenance` 還沒切過去就先 revoke，等於當場拔掉那個部署刪資料的能力。

1. 再次確認**所有**會跑清除子命令的 `maintenance` 部署（不只是你剛測的那一個）都已完成第 1、2 節。多環境／多叢集部署逐一列出來勾選，不要用「應該都一樣」代替逐一確認。
2. 確認之後，對 API 角色執行（把 `skillhub_api_role` 換成實際的登入或群組角色名）：
   ```sql
   REVOKE DELETE ON
       datasets, dataset_object_cleanup_intents, test_cases, run_artifact_upload_intents,
       artifacts, download_object_cleanup_intents, download_records, download_artifacts,
       creation_sessions, skill_versions, skills, skill_sources, user_identities, sessions,
       object_collection_queue, object_reconcile_sightings, audit_events, feedback_reports
   FROM skillhub_api_role;
   ```
   這張表清單刻意跟 `0059` 授予 `skillhub_purge` 的 `DELETE` 清單一模一樣——這就是「把 DELETE 從一個角色搬到另一個角色」的意思，多 revoke 一張表都可能打斷 API 自己的刪除端點。
3. Revoke 後，重新跑一次 API／Worker 自己的刪除路徑（使用者刪除自己的 Dataset、刪除自己的 Skill、登出），確認**沒有**任何一個因為這次 revoke 而 42501——如果有，代表這張表其實同時被 API 自己的端點用到，不該出現在 revoke 清單裡，先把它從 revoke 移除、回去確認 `0059` 是否也該補這張表給 `skillhub_purge`（如果 API 跟清除工作真的共用同一個刪除語句），再重新走一次本節。
4. 記錄 revoke 執行的時間、SQL、操作者與驗證結果。

## 4. 回退

1. 任何一步失敗，先把 `SKILLHUB_PURGE_DATABASE_URL` 從部署設定移除（或指回空字串）——`purgeDatabaseURL()` 會自動退回 `DATABASE_URL`，`maintenance` 立刻回到用 API 角色跑，不需要重新部署程式碼。
2. 如果已經執行過第 3 節的 revoke 才發現問題，用第 3 節 revoke 清單的表反過來 `GRANT DELETE ... TO skillhub_api_role`，把 API 角色的權限復原到 revoke 之前的樣子，再回到第 2 節重新驗證。
3. `0059` 本身（`CREATE ROLE`、`GRANT ... TO skillhub_purge`）不需要回退：留著這個角色不指派給任何登入帳號，就等於這個角色從未被用過，不影響任何現有連線。
