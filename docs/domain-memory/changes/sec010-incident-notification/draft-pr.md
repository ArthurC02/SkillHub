# Draft PR：控制平面安全事件的分級建單與送達

## Business intent

`SEC-010` 要求每個 P1／P2 事件有帶分級、觸發判準、自動動作結果與現場位置的 GitHub issue；P1 還要在非工作時間送達手機。現有控制平面只寄 Alertmanager email，兩支供應鏈 Workflow 只處理各自的掃描事件，因此不能把「有告警」當成「有人能接手事件」。

## Domain impact

Run 仍擁有派送煞車與解除，不讓事件建單的成敗改動這份狀態；Trace 與 Sandbox 等訊號維持各自的事實 owner。跨來源的事件生命週期目前沒有經審查的 Context，提議由 `incident` Supporting Context 擁有，或由審查者明確裁定只作部署層整合能力；裁定前不新增平台套件或 Registry 紀錄。任何方案都不得把 `severity=critical` 直接翻成 P1：Provider 長時間不可用是 P2，Reconciler／Trace 遮罩停擺是 P1。

## Implementation handoff

候選接法是內部網路上的 Alertmanager webhook 接收端，依具名訊號對照 SEC-010 分級；以 alert fingerprint 與事件窗口辨認重送，再透過窄的「記錄事件」Port 交給 GitHub Issues Adapter。外部建單失敗回錯並保留可觀測的重試，不寫「已通知」；訊號若未帶足自動動作或現場證據，issue 寫「未確認」並指向核對位置。控制平面的派送停止仍獨立於 GitHub。此處不預設額外套件或新的平台內資料表。

尚待審查：事件 owner、真實 repository 與具 Issues write 權限的憑證、維運者帳號、各訊號的自動動作證據來源，以及手機推播設定。不能用程式通過來代替這幾個答案。

| SEC-010 判準 | 現有訊號 | 建單前仍缺的證據 |
| --- | --- | --- |
| P1：逃逸疑慮／逃逸類 CVE | 人工判斷；`gvisor-baseline.yml` 已對疑似逃逸類公告開暫定 P1 issue | 人工事件如何與既有 issue 關聯；控制平面是否已停派送 |
| P1：P-02 連到核心資料庫 | Run watcher 可宣告 `p1_incident` 煞車；`DispatchHaltedForIncident` 告警可見結果 | 探針只在實際節點運作才有證據；原因與現場未進 webhook |
| P1：Trace 遮罩停擺 | `TraceMaskingStopped` 與煞車 watcher；`DispatchHaltedForIncident` | 兩條 alert 是否同一事故、動作成功與受影響窗口 |
| P1：Reconciler 停擺 | `OrphanScanNotRunning` 與 API watcher | watcher 本身停擺時須記動作未確認，不得假稱已停 |
| P2：遺留資源達門檻 | `LeakedSandboxStillPresent`、`DispatchHalted` 的 `orphan_threshold` | 兩個訊號的事故關聯與現場位置 |
| P2：連續三輪撤銷失敗／清理連續失敗 | `CredentialRevokeFailing`、`CleanupFailing` | 目前是聚合計數或速率，不能證明同一資源連續失敗 |
| P2：Provider 長時間不可用 | `ProviderCapabilityUnreachable` | 需要明確 P2 映射；該告警雖標 critical 但不是 P1 |
| P2：掃描過期且映像仍被引用 | `runtime-scan-expiry.yml` 只查目前版本並開 P3 | 現役節點 digest 的權威清單與逐 digest 稽核 |

`DispatchHaltedForIncident` 與根因告警可能同時描述同一 P1；只用各自的 alert fingerprint 查重會開兩張單。事件識別與關聯須先裁定，不能靠字串相似度猜測。

## Proposal and approvals

`sec010-incident-notification` 是 draft，需 developer 審查；沒有任何核准或實作證據。這份包不將 `SEC-010` 判為完成。

## Contract impact

只新增內部 Alertmanager webhook → 事件接收端 → GitHub Issues 的契約，不改公開 API。Alertmanager 官方文件列出 webhook JSON 的 `version: "4"`、逐筆 `fingerprint` 與重送時須能容忍的 payload；[GitHub Issues REST](https://docs.github.com/en/rest/issues/issues) 可建立帶 label 與 assignee 的 issue，但文件也指出權限不足時兩欄可能被靜默忽略，實作必須讀回驗證。[GitHub Mobile 通知設定](https://docs.github.com/en/subscriptions-and-notifications/get-started/configuring-notifications)包含 issue 指派推播及工作時間排程，送達須真人演練。

## Verification

`test-obligations.json` 對應分級、建單、重送、秘密邊界、真實送達與全部 P1／P2 判準覆蓋。實作後應暫時把 Provider 不可用映射成 P1，確認分級測試會紅，再還原；目前沒有測試結果，`evidence-bundle.json` 留空而非假綠。

## Residual risks

現有 P2 告警有些只證明「一段時間內曾失敗」，不能證明規格要求的同一資源連續三輪失敗；單靠 label 會誤報。真實控制平面、GitHub 權限與手機通知未演練。未補齊前 Alertmanager email 仍是告警，不是事件單與送達證據。
