# ADR-083：Aggregate 用一個 Go 型別擁有它的規則

- 狀態：**Superseded** by [ADR-084](./ADR-084-aggregates-speak-in-domain-events.md)（2026-09-15 同日；原裁定：負責人「進行 Aggregate Root、Entity／Value Object 的改寫」）
- 日期：2026-09-15
- 相關：[ADR-032](./ADR-032-ddd-bounded-context-governance-for-platform.md) §4（本份**只取代**第一條末句「這三處現有實作已是實質 aggregate，補文件與測試即可，不重寫」；§4 其餘三條——不引入 repository interface、不採 event sourcing／CQRS 框架、transaction script 是合法模式——不變，ADR-032 不 Superseded）、[ADR-003](./ADR-003-data-ownership-and-storage.md)、[ADR-008](./ADR-008-asynchronous-workflows-and-domain-events.md)、[ADR-026](./ADR-026-evaluation-reassessment-evidence-lifetime-and-judge-trust-boundary.md)、[ADR-034](./ADR-034-cross-context-writes-close-by-inversion-not-by-events.md)、[platform-ddd-convergence.md](../development/platform-ddd-convergence.md)

## 背景

ADR-032 §4 把 Run 狀態機、Skill Version 不可變性、Evaluation append-only 認定為「實質 aggregate」，只補文件與測試。2026-09-14 的戰術樣式盤點逐條追了三處的每一個寫入點，結果是三處都沒有一個 Go 型別擁有規則：

- **Skill**：`registry.Skill` 是資料列的副本。治理寫入（下架、存取限制、再散布、分類、刪除）各自鎖列、在行內判斷、呼叫 sqlc；版本有兩條建立路徑，只有匯入那條經過 manifest 驗證；「generated 不能被改寫」是先寫進去再回滾。
- **Run**：狀態轉移在 Go 與 SQL 各有一份，並有 `run-status-sql` 對帳；但「新的 Run 從 queued 開始」只是欄位預設值，「已結束的 Run 不能取消」只寫在 SQL 的 WHERE，同一個 attempt 可以被結束兩次，driver 在行內對狀態分支。
- **Evaluation**：`status.go` 有轉移表，寫入路徑卻沒有呼叫它，「pending 才能結算」只靠 SQL 的 WHERE；建議的決定與「套用之後不能撤回接受」是 handler 裡的條件式。

這與 convergence 的 C3（規則的定義寫在 Go，並有不連資料庫的測試）與 C4（SQL 不擁有領域概念）不一致。

## 決策

### 1. 三個 aggregate 各有一個 Go 型別當根

Skill（含 Skill Version）、Run（含 attempt）、Evaluation（含建議的決定）各有一個 aggregate root 型別：

- 狀態不匯出，外界只能經由方法改變它。
- 方法是純的：不帶 `context`、不做 I/O，做出決定或回報拒絕的理由。規則的定義只有這一份，並有不連資料庫的測試。
- 載入是一支吃交易的套件函式，以列鎖讀出整個 aggregate；存回只有一個地方呼叫 sqlc。交易由呼叫端擁有，所以 ADR-034 的反轉寫入（別的 context 開交易、呼叫 owner 的函式）照舊成立。

### 2. 不引入 repository interface

載入與存回是具體函式，不是介面。ADR-032 §4 第二條不變：單一實作的介面仍是投機抽象。

### 3. SQL 的原子性守衛全部保留

`WHERE status = @from`、`revision = @expected`、trigger、unique index、列鎖都留在 SQL（convergence C1、C2）。Go 的方法是規則的定義與提早失敗，SQL 是併發下的保證，兩者都在。

### 4. Aggregate 型別不跨 context

Aggregate 只在 owner 的套件內使用。其他 context 仍然透過組裝層收到自己宣告的 Facts（ADR-034），owner 的判定以具名欄位送出。

### 5. 特徵化先於改寫，收緊是另一個決定

先用測試釘住每一條寫入路徑的現況，再把判斷搬進方法，搬的時候特徵化測試一字不改。若某條規則今天只存在於 SQL，而 Go 補上之後會改變對外行為（例如 failed 的評估是否也要凍結），照現況釘住並送 [`05`](../plans/05-pending-rulings.md) 裁定，不順手收緊。

### 6. 其餘 context 照判準加型別

Entity 與 Value Object 只加在 convergence §2 的 J2 四問至少一問為是的地方；識別碼仍不型別化（convergence §5.4）；不變量稀薄的 context 仍可用 transaction script（ADR-032 §4 第四條）。

## 後果

- 正面：每條規則在 Go 有唯一的定義與不連資料庫的測試；driver、handler 與 Service 只負責讀寫與編排。
- 成本：三個套件的寫入路徑要改寫。`run-status-sql` 以 AST 讀 `trial/execution/statemachine.go` 的 `successors`，改寫要保留這個名字與檔案，否則要同時改檢查器。
- 風險：改寫本身可能改變行為。以特徵化測試與突變驗證（根 `AGENTS.md`〈開發自動化〉第 9 條）守住，一次一個 aggregate，驗完才做下一個。
