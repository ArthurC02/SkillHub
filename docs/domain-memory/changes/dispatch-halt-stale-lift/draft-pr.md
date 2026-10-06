# Draft PR: 防止過期解除派送煞車

## Business intent

讓 operator 只能解除自己已核對的那一代煞車。若同一目標在畫面讀取後重新宣告，保留新煞車並提示重讀。

## Domain impact

已審查的 `run` Context 擁有 Run 的派送與清理生命週期。解除前提由 Run Orchestration 在更新交易內判定，不由前端快取或另一個 Context 判定。現行 SQL 只以目標和來源更新，handler 對 operator 解除傳入所有來源，因此舊畫面可作用於新宣告的煞車（`db/queries/dispatch.sql:25-31`、`apps/platform/internal/trial/execution/halt.go:471-484`）。

## Implementation handoff

同列重新宣告沿用 ID，解除後同目標的新列又可能有相同初始代次，所以解除要同時核對煞車身分與重新宣告代次。維持原 owner 與同交易 audit，不新增 DDD pattern。只比來源、理由或 ID，以及只靠前端重讀，都不能阻止競態。`test-obligations.json` 列出同列重宣告、新列替換、成功解除、空解除、後台引導、授權與契約測試；反證尚未執行。

## Proposal and approvals

`dispatch-halt-stale-lift` 是 draft，需 developer 審查。尚待決定 operator 的「恢復派送」是解除所見那筆，還是解除目標當下的任何煞車；也要確認已部署 operator 呼叫端如何更新。

## Contract impact

`GET /admin/dispatch` 擬回傳可核對的身分與代次；`DELETE /admin/dispatch/halt` 擬要求兩者，過期回具名衝突，目標已無煞車仍回 204。這會改變舊 DELETE 請求的相容性，不能在未確認部署呼叫端前宣稱非破壞性。沒有新事件。

## Verification

本草案已由 `validate-change-package` 檢查結構；程式尚未修改，沒有可聲稱通過的實作測試。通過批准後，每條驗收條件都需留下可觀察的整合或前端斷言，並以移除交易前提檢查的反證測試確認會紅。

## Residual risks

在保護落地前，管理員不可把畫面中已選取的煞車視為解除時仍相同；事故期間須重新確認現況，但手動重讀本身也不能消除最後一段競態。SEC-010 的通知送達與 SEC-012 的生產節點驗證仍是獨立殘項。
