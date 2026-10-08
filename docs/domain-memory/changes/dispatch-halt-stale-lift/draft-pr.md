# Draft PR: 防止過期解除派送煞車

## Business intent

讓 operator 只能解除自己已核對的那一代煞車。若同一目標在畫面讀取後重新宣告，保留新煞車並提示重讀。

## Domain impact

已審查的 `run` Context 擁有 Run 的派送與清理生命週期。解除前提由 Run Orchestration 在更新交易內判定，不由前端快取或另一個 Context 判定。原本 operator 解除只以目標與來源更新，因此舊畫面可能作用於新宣告的煞車；現在 API 解除由同一筆 SQL 更新同時比對目標、ID、代次與未解除狀態。

## Implementation handoff

同列重新宣告沿用 ID，解除後同目標的新列又可能有相同初始代次，所以解除同時核對煞車身分與重新宣告代次。維持原 owner 與同交易 audit，不新增 DDD pattern。只比來源、理由或 ID，以及只靠前端重讀，都不能阻止競態。`test-obligations.json` 的八項檢查已執行；拿掉代次或 ID 條件、略過 audit 或前端 409 重置，都讓對應測試失敗，然後還原。

## Proposal and approvals

產品裁定「恢復派送」只解除所見那筆，過期即拒絕；舊呼叫端缺前提也拒絕，不維持危險的無條件解除。使用者已在對話中通過實作方向，但 `dispatch-halt-stale-lift` 的具名 developer approval 尚未記錄，仍是 `submitted`，不得稱為正式核准；repo 外已部署的 operator 呼叫端須在發佈前盤點與通知。

## Contract impact

`GET /admin/dispatch` 回傳可核對的身分與代次；`DELETE /admin/dispatch/halt` 要求兩者，過期回具名 409，缺少前提回具名 400。帶完整前提且目標已無煞車仍回 204。這是刻意的破壞性變更：舊 DELETE 不得落回按目標直接解除。沒有新事件。

Repo 內的 HTTP 解除呼叫端包括後台 `useDispatchHalt`、P1 runbook 的 `curl` 範例，以及 API 整合與瀏覽器流程測試，均已隨 API 更新；歷史 M4 上線清單不回溯修改，實際演練應依現行 runbook。自動門檻恢復直接呼叫來源受限的 `LiftHalt`，不經此 HTTP 請求。Repo 外已部署的 operator 腳本或整合仍待盤點，不能假設不存在。

## Verification

`validate-change-package`、`gen:check`、Go lint、Web lint／型別檢查與資料庫 API 整合測試通過；前端全套 69 檔、1,232 條測試及 Chromium 的後台派送流程也通過。八項 proof obligations 的結果、時間與輸出雜湊見 `evidence-bundle.json`。反證測試分別確認同列重新宣告、新列替換、P1 升級、audit 回滾與前端衝突後重新確認的保護真的會紅。這些是本機結果，尚非遠端 CI 或正式審查。

## Residual risks

正式套用 Change Package 與發佈前仍需具名 developer approval、遠端 CI、正式 SCM attestation，以及對 repo 外舊呼叫端的通知與遷移確認。SEC-010 的通知送達與 SEC-012 的生產節點驗證仍是獨立殘項。
