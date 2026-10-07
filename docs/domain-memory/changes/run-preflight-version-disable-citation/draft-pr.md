# Draft PR: 對齊版本停用後的 preflight 事實讀取引用

## Business intent

讓開發者仍能機器驗證已審查的 Registry→Run preflight 事實讀取，而不是依賴失效行號。

## Domain impact

既有互動 `registry-run-preflight-facts` 的語意和契約不變。新增的版本停用檢查讓同檔案原引用後移；本包只更新該互動的結構化 citation。

## Implementation handoff

保留 Registry 擁有 Version 事實、Run 透過注入的 reader 讀取，以及缺少 reader 時拒絕的既有邊界。新引用涵蓋目前的公開入口與版本讀取函式。不為了維持行號壓縮產品程式碼。

## Proposal and approvals

提案 `run-preflight-version-disable-citation` supersede 前次引用調整；developer `ArthurC` 已核准，測試證據與簽章 SCM 證明已驗證，引用已由受控流程套用。遠端 CI 尚未確認。

## Contract impact

沿用 `registry-run-facts-v1`，不改公開或內部契約。

## Verification

舊引用由 `verify-evidence` 回報 stale；新引用由 `cite` 算出並在暫存副本驗證。正式套用後重跑完整 Registry 與 audit 驗證，62 項引用全部 current；reader 缺席的拒絕測試也通過。

## Residual risks

版本停用本身的新交易准入與公開 API 由另一個 Change Package 驗證；此包只處理舊引用的準確性。
