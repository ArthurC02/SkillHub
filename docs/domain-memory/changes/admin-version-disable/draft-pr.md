# Draft PR: 停用 Skill Version 的跨界准入

## Business intent

operator 停用指定 Skill Version 後，新的 Run 不得引用它；既有 Run 與不可變版本內容保持原樣。

## Domain impact

Registry 擁有版本與停用事實，Run 擁有新 Run 的建立與生命週期。現有版本事實讀取在建立交易外，不能單靠它保證停用與建立並行時的結果。新准入判定須由 Registry 在 Run 建立交易內完成，與停用命令鎖住同一版本；狀態與 audit 同交易。

## Implementation handoff

方案：版本外的停用狀態、Registry 的窄交易內判定、組裝層注入 Run；不新增 Context，也不改寫 `skill_versions`。前端只使用核對目標所需的最少 metadata。不能以 preflight 舊結果、前端隱藏按鈕或異步投影作為唯一閘門；它們都無法排除停用提交後仍接受新 Run。

待裁定：停用能否恢復，以及 operator 是否可讀 ID／序號／狀態等最少量版本 metadata。兩者會影響公開契約與操作後果，未裁定前不實作。九項測試義務見 `test-obligations.json`；尚無突變結果。

## Proposal and approvals

`admin-version-disable` 目前是 draft，未提交審查、未獲 developer 核准，也未升格 Registry 記錄。不得把本文件當成已核准的規則。

## Contract impact

預計新增 operator-only 停用與最少量狀態讀取 API，並為 preflight／Run 建立提供具名拒絕；既有成功回應不移除欄位。Registry→Run 的既有版本事實讀取可保留作一般讀取，但不能代替交易內准入。暫無新事件需求；若後續消費者需要停用通知，須另定事件語意與 outbox。

## Verification

需求來源為 `02` 的 SEC-011 與版本／Run 不可變規則。現有後台 API 套件的整合測試曾在獨立 PostgreSQL 通過，但**未覆蓋本提案的新規則**；本包目前沒有可聲稱通過的新測試。實作時須驗證授權矩陣、並行序列化、交易 rollback、preflight 與 Run 建立拒絕、歷史 Run 保持不變，並對交易內准入閘門做一次會紅的突變檢查。

## Residual risks

產品選擇與 developer 審查未完成；OpenAPI、migration、Query owner、Go 與前端均未改動。內容來源白名單是另一項 SEC-011 缺口，不在本包內；正式告警部署也不在本包內。
