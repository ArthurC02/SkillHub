# ADR-018：Aggregate 與領域事件

- 狀態：Accepted
- 相關：[ADR-003](./ADR-003-run-orchestration-and-async-workflows.md)（Transactional Outbox、鐵律 9）、[ADR-016](./ADR-016-platform-bounded-contexts-and-context-map.md)（context 邊界、composition root、戰術 DDD 限縮）、[ADR-017](./ADR-017-query-and-write-ownership.md)（非 aggregate 之間的跨 context 寫入：搜尋投影、帳號刪除 purge，仍以依賴反轉收斂，不走本文的事件路徑）、[ADR-009](./ADR-009-evaluation-verdicts-and-judge-trust.md)、[platform-ddd-convergence.md](../development/platform-ddd-convergence.md)（Entity／Value Object 與識別碼型別化的判準）

## 背景

Skill（含 Skill Version）、Run（含 attempt）、Evaluation（含建議的決定）三個核心概念的規則曾經分散在 driver、handler 與 SQL 的 WHERE 子句裡：同一條規則在 Go 與 SQL 各寫一份，且往往只有 SQL 那份是真的守住的那份，Go 端只是行內判斷。這與「規則的定義只有一份、並有不連資料庫的測試」的目標不符。本文的 aggregate 規則現階段收斂這三者；creation context 的兩個具名概念之後也適用同一套規則，屆時比照決策 7 補特徵化測試再改寫，不預先展開。

同時，aggregate 之間原本是直接呼叫對方的方法或寫對方的列（例如套用建議：registry 建立版本、eval 在另一個交易把建議標成已套用），耦合點分散在呼叫端，難以看出一個 context 對另一個 context 的真實依賴。

## 決策

### 決策 1：三個 aggregate 各有一個 Go 型別當根

Skill、Run、Evaluation 各有一個 aggregate root 型別，狀態不匯出，外界只能經由方法改變它。載入是一支吃呼叫端交易的套件函式，以列鎖讀出 aggregate 需要的部分；存回只有一個地方呼叫 sqlc。交易由呼叫端擁有。

不引入 repository interface：載入與存回是具體函式，不是介面，單一實作的介面是投機抽象。

Entity 與 Value Object 只加在真正有不變量或行為要收斂的地方；識別碼不型別化——判準見 [platform-ddd-convergence.md](../development/platform-ddd-convergence.md)；不變量稀薄的 context 仍可用 transaction script，此為合法模式（詳見 [ADR-016](./ADR-016-platform-bounded-contexts-and-context-map.md)）。

### 決策 2：公開方法不回傳值，只改狀態、只記事件

Aggregate 的公開命令方法：

- 不帶 `context`、不做 I/O，是純函式。
- 不回傳值。命令成立：改變狀態，並記下一則領域事件。命令不成立：狀態不動，只記下一則帶理由的拒絕事件。
- 記下的事件以一個唯讀的存取方法取得；aggregate 目前的狀態也只從唯讀的存取方法觀察。

規則的定義因此只有一份，且不連資料庫就能測——測試給定狀態、呼叫命令、斷言唯讀狀態的變化或記下的事件（含拒絕事件與它的理由）。

應用服務與 handler 只做四件事：開交易、載入、呼叫命令、讀事件——讀到拒絕就回應拒絕，否則存回並提交；不讀 aggregate 的狀態自己做規則判斷。

### 決策 3：存回時事件寫進 outbox；拒絕的命令什麼都不寫

存回是同一個交易裡：寫入狀態的變更，並把命令記下的每一則事件依序寫進 outbox（[ADR-003](./ADR-003-run-orchestration-and-async-workflows.md) 的 Transactional Outbox、鐵律 9）。拒絕事件不存回——被拒絕的命令沒有改變任何東西，呼叫端讀到拒絕就直接回應。

領域事件是 aggregate 所屬套件的具名型別；wire 名稱（`event_type`）列在 [`contracts/events/domain-events.md`](../../contracts/events/domain-events.md) 的事件目錄，Go 常數在 `outbox`，與資料庫的 `CHECK` 三方對帳（新增或修改事件時三處同一個 commit 一起改）。

### 決策 4：Aggregate 之間只用領域事件溝通

一個 aggregate 不呼叫另一個 aggregate 的方法，也不寫它的列；一次交易只改一個 aggregate。唯一的例外：一條不變量由資料庫的 unique index 橫跨同一種 aggregate 的兩個 root 時（例如同一個 Run 的前後兩版評估——舊版被取代與新版建立必須同交易），兩個 root 可以在同一個交易存回。

送達的路徑：outbox publisher 送到 `outbox.Dispatcher` 的訂閱，再送進訂閱者的 Mailbox——訂閱者所屬 context 擁有的一條 River 佇列，以事件識別去重。Worker 從 Mailbox 取出事件，載入訂閱的 aggregate（或由 factory 新建），呼叫它消化這則事件的方法，存回；消化方法同樣不回傳值、同樣記下事件。

每一種事件都要有訂閱者，或者在 `Dispatcher.Validate` 留下具名的忽略理由。Aggregate 之間因此是最終一致：畫面與測試要容忍一段時間差，測試以排空 Mailbox 取代等待，不假裝它是同步的。

消化方法以 aggregate 自身的狀態判斷冪等，不依賴事件的到達順序；需要順序保證的訂閱者出現之前不預先設計。

### 決策 5：不是 aggregate 之間溝通的東西不套用本文

以下維持依賴反轉、同交易的既有形狀，理由與做法見 [ADR-017](./ADR-017-query-and-write-ownership.md)：

- 跨 context 的讀取由組裝層注入 Facts，owner 的判定以具名欄位送出。
- 搜尋投影 `search_documents` 是讀取模型，不是 aggregate：匯入成功的當下即可被搜尋（[`03`](../plans/03-work-items.md) INGEST-009）仍靠同交易寫入。
- 帳號刪除 purge 是合規上的全有全無（[`03`](../plans/03-work-items.md) CORE-007）：identity 在同一個交易依序呼叫各 context 的清除函式。改成事件等於把「單一交易清完」換成「陸續清完」，是對外行為的改變，仍待 [`05`](../plans/05-pending-rulings.md) 裁定。

### 決策 6：SQL 的原子性守衛全部保留

`WHERE status = @from`、`revision = @expected`、trigger、unique index、列鎖、advisory lock 都留在 SQL。Go 的方法是規則的定義與提早拒絕，SQL 是併發下的最後一道保證，兩者都在，不是二選一。

### 決策 7：改寫前先用特徵化測試釘住現況，收緊是另一個決定

把判斷從 driver／handler／SQL 搬進 aggregate 方法時，先用測試釘住每一條寫入路徑的現況，搬的時候特徵化測試一字不改。若某條規則今天只存在於 SQL，補上 Go 端的判斷會改變對外行為，照現況釘住並送 [`05`](../plans/05-pending-rulings.md) 裁定，不順手收緊。Run 的改寫另外要保留 `run-status-sql` 以 AST 讀取的名字與檔案（`trial/execution/statemachine.go` 的 `successors`），否則要同時改檢查器。


### 決策 8：帳號刪除是例外，維持同一個交易

帳號刪除作用在整個 Workspace，是一條合規流程，不是 aggregate 之間的溝通，所以它不走領域事件：identity 在一個交易裡依序呼叫各 context 的清除，全有全無。**順序本身承載規則**（registry 先刪版本，ingest 才刪得掉沒有版本引用的來源），而刪除版本要靠交易級的放行通過不可變 trigger——事件化之後這兩件事都要重新設計，換到的只是將來拆服務時比較好搬，而拆分的壓力條件沒有出現。**全有全無只涵蓋資料列**：物件儲存沒有回滾，所以位元組的刪除排在交易之前，失敗時留下的是指向已消失檔案的列，由下一輪清理收走，而不是沒有任何列指名的孤兒檔。

### 決策 9：判定失敗的評估與判定完成的一樣整列凍結

不可變 trigger 的條件涵蓋 `completed` 與 `failed` 兩種終態，放行的欄位不變（兩個回饋欄與兩個時間欄）。失敗的評估是「那次為什麼沒有結論」的紀錄，會被下一版取代但不該被改寫；今天沒有任何 Go 路徑寫它，**這一道是資料庫層的第二道保證**，擋的是將來某條直接寫入失敗列的程式。

## 影響

### 正面

- 每條規則在 Go 有唯一的定義，只從兩個地方觀察得到：aggregate 的唯讀狀態與它記下的事件；driver、handler 與 Service 只負責讀寫與編排。
- Aggregate 之間的依賴變成事件目錄上的一列，不是散落在組裝層的一條條注入線或互相呼叫。

### 成本與限制

- 每種新事件要在事件目錄、`outbox` 常數、migration 的 `CHECK` 同一個 commit 加上；每個訂閱要一條 Mailbox 佇列與對應的 Worker 消化邏輯。
- 原本同交易或緊接著發生的跨 aggregate 寫入變成最終一致，畫面與測試要容忍一段時間差。
- 改寫規則的落點本身可能改變行為，需要特徵化測試與突變驗證守住，一個 aggregate 驗完才做下一個。
