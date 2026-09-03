# ADR-063：帳號清除 fence 上線必須先排空舊程序

- 狀態：Accepted
- 日期：2026-09-03
- 關聯：ADR-004、ADR-008、ADR-022、ADR-062

## 背景

ADR-062 增加 Workspace advisory lock、物件收集 intent 與 Run Attempt 的預簽 URL 到期狀態。這些保護跨越資料庫與 Object Store，不能把「migration 已執行」誤當成「所有程序都已遵守新協定」。舊 Maintenance 不會取得 account purge 的 exclusive lock；舊 Worker 也不會把已簽發 URL 的期限寫回新欄位。

## 決策

部署 `0049`～`0051` 前先停止所有舊版 Maintenance、Worker，以及所有可能產生物件的舊版 API request／instance，且 migration 後不得再啟動這些舊版程序。舊 API 是先把 bytes 寫入 Object Store、再寫資料庫列；資料庫 trigger 無法 fence 尚未留下列的 upload，因此 API 不可滾動混部。先部署只會在 Workspace shared lock 內寫入的新 API 與新版 Worker，完成下述修復後才可重新啟動 account purge。

新版建立 Run Attempt 時明寫 `unissued`，並在把已簽發 URL 交付 Provider 前改成 `recorded`；只有 `unissued` 能自動關閉。migration 前既有或舊 Worker 建立的 Attempt 一律是 `legacy_unknown`；新版不得猜測它是否曾簽發 URL。

舊 Worker 全部停止後，等待 21 分鐘（15 分鐘 hard wall、5 分鐘 grant slack、1 分鐘 clock skew），再依 Runbook 的限定 SQL 關閉全部 `legacy_unknown`。未完成這個步驟時，帳號清除 fail-closed 是正確結果；即使 migration 曾替舊列寫入有限期限，也不得只憑期限推定舊 Worker 沒在稍後簽發 URL。

## 後果

- 混合版本期間不會因新版誤判舊版 Attempt 而提早刪除仍可下載的私人資料。
- 上線需要可稽核的 drain 時點，且 21 分鐘等待不能省略。
- 新 intent 只保護新寫入；重啟 account purge 前還必須完成舊版可能留下的 Object Store 孤兒盤點與對帳，證明無未配對物件。
- 若跳過修復，安全性仍成立，但相關帳號清除會持續被擋住。

## 驗證

部署負責人逐項記錄於 [`account-purge-write-fence-rollout.md`](../runbooks/account-purge-write-fence-rollout.md)。程式測試另證明 current `unissued` 可被修復，而 `legacy_unknown` 不會被自動關閉。
