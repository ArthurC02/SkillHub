# Platform DDD 重構審視報告

- 審視日期：2026-08-20
- 審視範圍：近期 DDD-005～DDD-014 相關變更，重點為 `apps/platform`
- 審視方式：唯讀檢查 commit、ADR、import graph、composition root、Outbox、測試與 CI 設定

## 結論

這次重構方向正確，已經從「靠文件與註解維持邊界」進步到「package 邊界＋depguard＋CI 機械強制」。目前可以視為成熟度不錯的 modular monolith 第一版，但還不能稱為資料與事件邊界都完整封裝的 Bounded Context 架構。

最需要優先處理的不是 context package 是否拆開，而是：

1. 單一 sqlc package 仍讓所有 context 能直接取得全套 query。
2. `NewApp` 集中的是 API wiring，不是整個 platform 的唯一 composition root。
3. Outbox 目前是單一 callback consumer，尚未具備可擴充的 Published Language dispatcher 語意。

## 做得好的地方

### Context 邊界已經有機械化防線

ADR-032 已建立 context 對照表，並把跨 context import 規則落到 `apps/platform/.golangci.yml` 的 depguard。`automation-check` 也會檢查 ADR 與 lint 設定中的 drift marker 是否一致。

這比單純靠 code review 或註解可靠很多，尤其適合目前由多個 Coding Agent 平行修改的工作模式。

### `run` → `eval` 的依賴方向改善正確

Run 終態現在透過 transactional outbox 發出 `run.succeeded`／`run.failed`，由 Evaluation context 自己消費，而不是由 Run context 直接 import Evaluation 並入隊。這符合「後續反應事件化」的判準，也降低了上游知道下游實作的程度。

### Run state machine 的責任集中清楚

`internal/run/statemachine.go` 把合法轉移、不變量、status history、audit 與 outbox 寫入集中在一條 transaction path，且已有純規則測試。這是本次最接近 tactical DDD aggregate 的部分。

### Policy 抽離保留了真正的 enforcement point

Quota 規則移到 `internal/policy`，但仍由 Run 在 create-run transaction 與 advisory lock 內呼叫。這避免只搬移程式碼、卻改變實際強制時點，是正確的 context extraction 做法。

### 驗證狀態良好

本次審視期間確認：

- `go test ./...`：304 tests、30 packages 通過
- `golangci-lint run ./...`：通過，無問題
- `go -C tools/devctl run . automation-check`：通過
- `git diff --check`：通過
- 工作樹：乾淨

## 主要風險與建議

### P1：單一 sqlc package 是跨 context 的資料存取後門

ADR-032 說明採用「sqlc per-context queries＋Go package 邊界」；但實際 `db/sqlc.yaml` 只有一個設定，所有 query 都生成至同一個 package：

- [`db/sqlc.yaml`](db/sqlc.yaml#L1-L11)
- 輸出位置：`apps/platform/internal/platform/db/gen`

因此 depguard 雖然可以阻擋 `run` import `eval`，卻無法阻擋任何 context 直接呼叫另一個 context 的 query。現有例子包括：

- Analytics 直接查 Run ownership：[`apps/platform/internal/analytics/feedback.go`](apps/platform/internal/analytics/feedback.go#L125-L138)
- Run 直接查 Artifact object sharing：[`apps/platform/internal/run/service.go`](apps/platform/internal/run/service.go#L553-L567)
- Run 直接讀 SkillVersion 與 TestCaseSnapshot：[`apps/platform/internal/run/schedule.go`](apps/platform/internal/run/schedule.go#L239-L254)

這與 ADR-002 所說的「每個模組擁有自己的資料存取邊界」存在落差。這不是目前 lint 會報錯的問題，而是架構防線尚未涵蓋 persistence layer。

#### 建議

不必一次拆完所有 read query，建議先處理 write ownership：

1. 將各 context 的 write query 分開生成，或包在 context-owned persistence package。
2. 跨 context read 只透過公開 Service、ACL 或 read projection。
3. 為 query 加上 owner metadata，讓 `automation-check` 檢查呼叫 package 是否符合 ownership。
4. 先從最敏感的 `skills`、`skill_versions`、`runs`、`download_artifacts` 寫入路徑開始。

### P1/P2：`NewApp` 並不是整個 platform 的唯一 composition root

API 程式的註解稱 `apiserver.NewApp` 是 platform 唯一 composition root：

- [`apps/platform/cmd/api/main.go`](apps/platform/cmd/api/main.go#L1-L5)
- [`apps/platform/internal/apiserver/app.go`](apps/platform/internal/apiserver/app.go#L99-L102)

但 Worker、Maintenance、Reindex 都有自己的 Service 建構：

- [`apps/platform/cmd/worker/main.go`](apps/platform/cmd/worker/main.go#L86-L115)
- [`apps/platform/cmd/maintenance/main.go`](apps/platform/cmd/maintenance/main.go#L71-L101)
- [`apps/platform/cmd/reindex/main.go`](apps/platform/cmd/reindex/main.go#L72-L72)

這些 process 各自有 composition root 本身是合理的；問題是文件描述不精確，且長期可能造成配置與依賴 wiring 漂移。

#### 建議

- 將 `NewApp` 明確命名為 API composition root，或明確記錄四個 process roots。
- 抽出 process-neutral 的 wiring factory，避免每個 command 重複配置規則。
- 為 API、Worker、Maintenance、Reindex 各自加 wiring smoke test，確認關鍵服務都被注入。

### P2：Outbox 是單一 consumer callback，不是可擴充的 dispatcher

`outbox.Worker` 目前只有一個 `Deliver` callback；若沒有設定，會 fallback 到 log：

- [`apps/platform/internal/outbox/outbox.go`](apps/platform/internal/outbox/outbox.go#L60-L66)
- [`apps/platform/internal/outbox/outbox.go`](apps/platform/internal/outbox/outbox.go#L140-L142)

這代表某個 worker 若錯誤漏接 Evaluation consumer，事件仍可能被記錄並標記為 published，而 Evaluation 不會被排入。現在的 worker wiring 是正確的：

- [`apps/platform/cmd/worker/main.go`](apps/platform/cmd/worker/main.go#L125-L126)
- [`apps/platform/cmd/worker/main.go`](apps/platform/cmd/worker/main.go#L188-L188)

但 `RunEventConsumer` 對非 terminal event 直接回傳成功；若未來增加第二個 consumer，第一個 consumer 將事件視為已處理後，其他 consumer 沒有獨立 replay 或 offset。

#### 建議

1. Production wiring 下 `Deliver == nil` 應 fail closed；log-only 模式應限於明確的 development mode。
2. 在 composition root 建立 event dispatcher／fan-out，而不是只注入一個 callback。
3. 各 consumer 保有自己的 idempotency 事實或 replay 位置。
4. 加測試保證 `run.succeeded` 在 Evaluation enqueue wiring 缺失時不會被當作成功消費。

### P2：`App.Deps` 與 Service handles 公開，架構規則主要仍靠慣例

`App` 暴露 `Deps` 與多個 concrete Service，方便 integration test 調整路由與直接驅動 domain service：

- [`apps/platform/internal/apiserver/app.go`](apps/platform/internal/apiserver/app.go#L88-L97)

這個取捨目前可以接受，但也表示 production code 能重新替換 handler 或 Service，無法由型別保證「只能由 composition root wiring」。

#### 建議

後續可提供獨立 test constructor 或 test options，逐步把 production object graph 改為 private，避免測試需求擴大公開面。

## 建議優先順序

### 第一階段：先封 persistence ownership

- 列出每個 context 的 table／query ownership。
- 先限制跨 context write query。
- 建立 query ownership 的 CI check。

### 第二階段：整理 process composition roots

- 明確區分 API、Worker、Maintenance、Reindex。
- 抽出共用 wiring factory。
- 補 wiring smoke tests。

### 第三階段：強化 Outbox delivery semantics

- 移除 production 的 silent log fallback。
- 引入 dispatcher／fan-out。
- 定義多 consumer replay、idempotency 與 dead-letter 行為。

## 最終評語

這批重構不是表面上的目錄搬移；context 命名、依賴方向、Run aggregate、Policy extraction 與 API composition 都有實質改善。我會批准目前方向繼續前進，但會把「query ownership」列為下一輪架構工作，不建議在它尚未處理前宣稱 platform 的 DDD 邊界已完全封閉。
