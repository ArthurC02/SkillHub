# ADR-084：Aggregate 以領域事件說話——命令不回傳值，事件送進訂閱者的 Mailbox

- 狀態：**Accepted**（2026-09-15 負責人裁定：「Aggregate Root 必定要深度綁定 Domain Event，Aggregate Root 之間是使用 Domain Event 溝通；聚合根的公開方法不回傳值，但是會拋出領域事件，送到對它感興趣的聚合根的 Mailbox，聚合根會消化這些領域事件；測試只確認聚合根唯讀狀態的變化，或直接觀測領域事件」）
- 日期：2026-09-15
- 取代：[ADR-083](./ADR-083-aggregates-own-their-rules-in-go-types.md)（全份；它對 [ADR-032](./ADR-032-ddd-bounded-context-governance-for-platform.md) §4 第一條末句的取代由本份承接，ADR-032 §4 其餘三條不變）
- 修訂：[ADR-034](./ADR-034-cross-context-writes-close-by-inversion-not-by-events.md)——aggregate 之間的寫入改用領域事件；ADR-034 處理的兩組（搜尋投影、帳號刪除 purge）不是 aggregate 之間的溝通，照舊，理由見決策 6
- 相關：[ADR-008](./ADR-008-asynchronous-workflows-and-domain-events.md)、[ADR-003](./ADR-003-data-ownership-and-storage.md)、[ADR-026](./ADR-026-evaluation-reassessment-evidence-lifetime-and-judge-trust-boundary.md)、[領域事件目錄](../../contracts/events/domain-events.md)、[platform-ddd-convergence.md](../development/platform-ddd-convergence.md)

## 背景

ADR-083 讓 Skill、Run、Evaluation 各有一個 Go 型別當 aggregate root，方法回報拒絕的理由，aggregate 之間照 ADR-034 以注入的函式互寫。負責人裁定 aggregate 的形狀要更進一步：命令不回傳值，結果只從唯讀狀態與領域事件觀察；aggregate 之間只用領域事件溝通。

這個形狀在 repo 裡已經有一條完整的路：Run 的終態轉移在同一個交易寫入 `run.succeeded`／`run.failed`（outbox），publisher 交給 `outbox.Dispatcher`，Evaluation 的訂閱把它排進 `evaluate_run` 佇列，Worker 取出後開始評估。其餘 aggregate 之間是直接呼叫，例如套用建議：先由 registry 建立版本，再由 eval 在另一個交易把建議標成已套用。

## 決策

### 1. Aggregate root 的形狀

- 狀態不匯出，只有唯讀的存取方法。
- 公開的命令方法不回傳值、不帶 `context`、不做 I/O。命令成立：改變狀態並記下領域事件。命令不成立：狀態不動，只記下一則拒絕事件（帶理由）。
- 記下的事件以一個唯讀的存取方法取得。
- 領域事件是 aggregate 所屬套件的具名型別；wire 名稱（`event_type`）列在事件目錄 §3，Go 常數在 `outbox`，與 DB 的 `CHECK` 三方對帳（目錄 §4 規則 2、4 照舊）。

### 2. 載入與存回

- 載入是吃呼叫端交易的套件函式，以列鎖讀出命令需要的部分。
- 存回只有一個地方：同一個交易寫入狀態的變更，並把記下的事件逐則寫進 outbox（ADR-008、鐵律 9）。
- 拒絕事件不存回：被拒絕的命令什麼都沒改變，呼叫端讀到拒絕就回應，不寫入。
- 不引入 repository interface（ADR-032 §4 第二條不變：單一實作的介面是投機抽象）。

### 3. Aggregate 之間只用領域事件

- 一個 aggregate 不呼叫另一個 aggregate 的方法，也不寫它的列。一次交易只改一個 aggregate。唯一的例外：一條不變量由 DB 的 unique index 橫跨同一種 aggregate 的兩個 root 時（同一個 Run 的前後兩版評估：舊版被取代與新版建立必須同交易），兩個 root 在同一個交易存回。
- 送達的路徑：outbox publisher → `outbox.Dispatcher` 的訂閱 → 訂閱者的 Mailbox。Mailbox 是訂閱者所屬 context 擁有的一條 River 佇列，以事件識別去重。Worker 從 Mailbox 取出事件，載入訂閱的 aggregate（或由 factory 新建），呼叫它消化這則事件的方法，存回。消化方法一樣不回傳值，一樣記下事件。
- 每一種事件都要有訂閱者或具名的忽略理由（`Dispatcher.Validate`，照舊）。
- Aggregate 之間是最終一致。

### 4. 應用服務與 handler

只做四件事：開交易、載入、呼叫命令、讀事件——讀到拒絕就回應拒絕，否則存回並提交。不讀 aggregate 的狀態自己做規則判斷。

### 5. 測試

Aggregate 的測試不連資料庫：給定狀態、呼叫命令、斷言唯讀狀態的變化或記下的事件（含拒絕事件與它的理由）。既有的特徵化測試（連資料庫的與整合的）改寫前後一字不改。

### 6. 不是 aggregate 之間溝通的東西照舊

- 跨 context 的讀取仍由組裝層注入 Facts（ADR-034 的讀取鏡像、ADR-035）。
- 搜尋投影 `search_documents` 是讀取模型，不是 aggregate：「匯入的當下就搜得到」（INGEST-009）仍靠同交易寫入。
- 帳號刪除 purge 是合規上的全有全無（CORE-007）。改成事件等於把「單一交易清完」換成「陸續清完」，是對外行為的改變：照現況保留，送 [`05`](../plans/05-pending-rulings.md) R-83 裁定。

### 7. 承接 ADR-083 仍成立的部分

- SQL 的原子性守衛全部保留（`WHERE status = @from`、trigger、unique index、列鎖、advisory lock）。Go 是規則的定義與提早拒絕，SQL 是併發下的保證。
- 特徵化先於改寫。Go 補上的判斷若會改變今天的對外行為，照現況釘住並送 `05`，不順手收緊。
- Entity 與 Value Object 只加在 convergence §2 J2 至少一問為是的地方；識別碼不型別化（convergence §5.4）；不變量稀薄的 context 仍可用 transaction script（ADR-032 §4 第四條）。
- `run-status-sql` 以 AST 讀 `trial/execution/statemachine.go` 的 `successors`；Run 的改寫保留這個名字與檔案。
- 一次一個 aggregate，驗完才做下一個：Evaluation → Skill → Run，最後是 creation 的兩個具名概念。

## 後果

- 正面：aggregate 的每條規則在 Go 有唯一的定義，而且只從兩個地方觀察得到：唯讀狀態與事件。aggregate 之間的依賴是事件目錄上的一列，不是組裝層的一條注入線。
- 成本：每種事件要在事件目錄、`outbox` 常數、migration 的 `CHECK` 同一個 commit 加上（目錄 §4 規則 4）；每個訂閱要一條 Mailbox 佇列與 Worker。原本同交易或緊接著發生的跨 aggregate 寫入變成最終一致，畫面與測試要容忍一段時間差，測試以排空 Mailbox 取代等待。
- 風險：改寫可能改變行為。以特徵化測試與突變驗證（根 `AGENTS.md`〈開發自動化〉第 9 條）守住。
- 事件目錄 §5 第 6 項（aggregate version）仍 open：消化方法以 aggregate 的狀態判斷冪等，不依賴事件順序；第一個需要順序的訂閱者出現時再補。
