# Draft PR: 停用 Skill Version 的跨界准入

## Business intent

operator 停用指定 Skill Version 後，新的 Run 不得引用它；既有 Run 與不可變版本內容保持原樣。

## Domain impact

Registry 擁有版本與停用事實，Run 擁有新 Run 的建立與生命週期。現有版本事實讀取在建立交易外，不能單靠它保證停用與建立並行時的結果。新准入判定須由 Registry 在 Run 建立交易內完成，與停用命令鎖住同一版本；狀態與 audit 同交易。

## Implementation handoff

方案：版本外的停用狀態、Registry 的窄交易內判定、組裝層注入 Run；不新增 Context，也不改寫 `skill_versions`。前端只使用核對目標所需的最少 metadata。不能以 preflight 舊結果、前端隱藏按鈕或異步投影作為唯一閘門；它們都無法排除停用提交後仍接受新 Run。

產品裁定：停用不可恢復，只阻止新 Run；重複停用回已停用並記錄嘗試。operator 只能以精確 ID 查 ID、版本序號與停用狀態，不提供私有內容或跨 Workspace 版本清單。測試義務見 `test-obligations.json`；交易內拒絕條件的突變讓既有兩個整合測試由 422 變成 201，還原後皆通過。

## Proposal and approvals

`admin-version-disable-v2` 承接已過時提案的同一實作與十項本機證據；developer `ArthurC` 已就目前 Registry 修訂版重新核准，測試與簽章 SCM 證據已驗證。此包沒有 Registry 更新，因此保持已核准而非已套用狀態；遠端 CI 尚未確認，不能當成可部署證明。

## Contract impact

已新增 operator-only 停用與最少量狀態讀取 API，並為 preflight／Run 建立提供具名拒絕；既有成功回應不移除欄位。Registry→Run 的既有版本事實讀取保留作一般讀取，交易內准入由獨立的注入介面負責。暫無新事件需求；若後續消費者需要停用通知，須另定事件語意與 outbox。

## Verification

需求來源為 `02` 的 SEC-011 與版本／Run 不可變規則。獨立 PostgreSQL 中，版本停用的授權矩陣、並行序列化、audit 失敗回滾、preflight 與 Run 建立拒絕、歷史 Run／版本／佇列任務不變、owner 讀取失敗 fail-closed 整合測試均通過；前端管理流程與無障礙測試也已執行。交易內准入閘門的突變得到兩條明確失敗斷言，還原後通過。全套及治理檢查完成前不提升正式狀態。

## Residual risks

OpenAPI、migration、Query owner、Go 與前端均已實作；新提案已通過本機證據與簽章 SCM 驗證，但遠端 CI 尚未確認。內容來源白名單是另一項 SEC-011 缺口，不在本包內；正式告警部署也不在本包內。
