# ADR-016：Platform Bounded Context 與 Context Map

- 狀態：Accepted
- 相關：[ADR-017](./ADR-017-query-and-write-ownership.md)（Query 與寫入所有權）、[ADR-018](./ADR-018-aggregates-and-domain-events.md)（Aggregate 與領域事件）、[ADR-003](./ADR-003-run-orchestration-and-async-workflows.md)（Run 編排與非同步工作流程，Transactional Outbox 的定義處）、[ADR-020](./ADR-020-design-system-trust-signals-and-screen-words.md)（設計系統、信任訊號與畫面用語）

## 背景

Platform 是一個模組化單體，會隨時間持續吸收新的領域知識與變動，且主要作者包含 Coding Agent。只靠文件與命名慣例維持的邊界會漂移而不自知：同一個 package 可能被多個呼叫端當成不同性質的東西用、依賴方向可能反轉成下游 context 被上游 import、composition root 可能被繞過而在方法內現場建構其他 context 的 Service、同一份職責可能同時散落在多個 package。這些問題沒有機器檢查就等於沒有規則——尤其對讀不到「言下之意」的 Agent 而言。

本 ADR 把 Bounded Context 的邊界、彼此的合法互動方式，以及「一個 package 屬於哪個 context」這件事本身，收斂成可由 CI 機械驗證的規則。實際的 Context 對照表（每個 package 屬於哪個 Bounded Context、其 Boundary ID 與現行路徑）與跨 context import 白名單是會隨程式演進的存量清單，維護在 [docs/development/platform-context-map.md](../development/platform-context-map.md)；本 ADR 只定治理規則本身。

## 決策

### 決策 1：每個 package 恰好一個 architecture identity，由 CI 三方對帳

Context 對照表是唯一機器可讀的 package 對照來源，欄位固定為：產品／Bounded Context 名稱、類型、Boundary ID、現行 internal path、需求 ID 前綴。`類型` 是封閉集合 `Core`、`Supporting`、`Shared Kernel`、`Generic`：

- `Core`、`Supporting` 必須有 Bounded Context 名稱；同名代表同一個 context，即使日後由多個 Go package 組成。
- `Shared Kernel`、`Generic` 沒有 Bounded Context 名稱。
- 同一個 package 在表上重複出現、類型未知、package 未登記，或缺少 depguard coverage，一律視為 CI 失敗。
- `Boundary ID` 是遷移期間不變的機械鍵：實體路徑搬遷只改「現行 internal path」欄，不改 Boundary ID、Go package 名稱、公開 API 或資料 owner。

`devctl automation-check` 讓三份清單互相對帳：實際的 Go package 目錄、對照表的 Boundary ID／現行 path、`apps/platform/.golangci.yml` 的 depguard `files` glob；任一方向缺漏即 FAIL。新增或搬移 package 必須先改對照表、再動目錄——「登記先於建目錄」因此有機械強制力；不需要 depguard coverage 的例外，以 devctl 的組裝套件名單（`compositionRoots`，見決策 5）與 generated transport（`entrypoint/api/gen`）為準。

### 決策 2：Context 間關係只有四種，各有固定機制

| 關係 | 機制 | 適用 |
| --- | --- | --- |
| 同步查詢（Customer–Supplier） | import 對方套件的公開 Service API | 當下這筆請求需要的事實 |
| 領域事件（Published Language） | Transactional Outbox（[ADR-003](./ADR-003-run-orchestration-and-async-workflows.md)） | 觸發之後該發生的反應 |
| Shared Kernel | `skillpkg`：套件格式與驗證的純函式庫 | 無狀態、無政策的共用碼；擴充需使用它的 context 共同同意 |
| 防腐層（ACL） | 外部系統一律經手寫轉譯層（`llmclient`、`run` 的 provider gateway、未來的 payment 等） | 外部契約不得滲入領域模型，外部型別止於 ACL |

判準：跨 context 呼叫若失敗會讓「當下這筆請求無法正確回應」→ 同步；若只是「之後該發生的事沒發生」→ 事件。

當雙方都是 Aggregate Root（見決策 4）時，這個判準再收斂一層：一次交易只能改一個 aggregate，即使某次跨 aggregate 寫入乍看像「當下決策需要」，也一律走領域事件——送達路徑是 outbox publisher → Dispatcher 的訂閱 → 訂閱者 context 自己的 Mailbox（River 佇列，以事件識別去重）。唯一允許同交易寫入兩個 root 的情形，是一條不變量由資料庫 unique index 橫跨同一種 aggregate 的兩個 root。事件消化與 aggregate 內部形狀見 [ADR-018](./ADR-018-aggregates-and-domain-events.md)。

`skillpkg` 有獨立的 depguard 規則：各 context 可以 import 它，它不得反向 import 任何 Bounded Context，避免 Shared Kernel 與 Generic 在治理上被混為一談。

### 決策 3：邊界以 depguard 機械強制，移出的關係不得靜默加回

`.golangci.yml` 的 depguard 規則是 Context 對照表與跨 context import 白名單的 CI 表述。任何跨 context 的新 import，必須在同一個 commit 內同時更新 [docs/development/platform-context-map.md](../development/platform-context-map.md) 的白名單與 depguard 規則——等於強制先過一次架構決策，才過得了編譯。曾經因治理問題移出白名單的跨 context 關係不得再加回；要恢復，必須有新的 ADR。

### 決策 4：戰術 DDD 刻意限縮

- **Aggregate 只用於不變量密集的 context**：Skill Registry & Versioning（Skill，含 Skill Version）、Run Orchestration（Run）、Evaluation & Improvement（Evaluation）三者。這三者的不變量另外有資料庫層級的機械防線（`WHERE status = @from`、trigger、unique index、列鎖、advisory lock 等）與之並存，Go 是規則的定義與提早拒絕，SQL 是併發下的最終保證；aggregate 的完整形狀、命令與事件的關係見 [ADR-018](./ADR-018-aggregates-and-domain-events.md)。
- **不引入 repository interface 層**：sqlc 的 per-context query 加上 Go package 邊界已提供等價封裝；在其上再鋪一層介面，是只有單一實作的投機抽象。
- **不採 event sourcing、不引入 CQRS 框架**：Catalog 手工重建的搜尋投影已是足夠的 CQRS。
- **transaction script 在不變量稀薄的 context 是合法模式**：例如 analytics、audit 的寫入。

### 決策 5：Composition root 是唯一的跨 context wiring 地點，沒有 context 可以 import 它

每個可獨立部署的 process（`cmd/api`、`cmd/worker`、`cmd/maintenance`、`cmd/reindex`）各有自己的 composition root，彼此不共享任何執行期物件；領域 Service 只由該 process 的 composition root 注入，禁止在方法內現場建構其他 context 的 Service。唯一例外見[淨測試模式](./README.md#淨測試模式)：該模式下 `cmd/api` 額外在同一個行程裡啟動 worker、共享一個連線池，生產路徑的兩個二進位不受影響。`entrypoint/wiring` 是 API 與 Worker 兩個 composition root 共用的接線套件（例如把 Credit 接到創作、Run 與顯示上），與 `apiserver`、`worker` 同屬組裝角色：可以在方法內建構各 context 的 Service、不受 depguard 的 deny 檢查限制，但沒有任何 context 可以 import 它。devctl 的組裝套件名單（`compositionRoots`）是唯一的機器判準，depguard coverage 的例外範圍以該名單為準。

### 決策 6：`internal/` 依收納角色分四種，Go 私有邊界不外移

```text
internal/
├── creator/  skill/  trial/  product/   # Bounded Context
├── shared/skillpkg/                     # 唯一 Shared Kernel
├── foundation/                          # Generic 技術基座，不含任何領域規則
│   ├── persistence/{db,partition,pgconv}/
│   ├── messaging/{queue,outbox}/
│   ├── storage/{objstore,objreconcile}/
│   ├── observability/{metrics,audit}/
│   ├── integration/llmclient/
│   └── runtime/{envx,httpx}/
└── entrypoint/{api/{apiserver,gen},worker,wiring}/   # Composition 與 generated transport
```

這些 package 不移出 `apps/platform/internal/`：Go 的 `internal` 規則是唯一能阻止其他 app 或 library 意外依賴平台實作的機制，移到外層等於把它們變成公開 surface。`apps/platform/cmd/` 是 process entrypoint，不是可被 import 的 library，不屬於這個收納問題。Foundation 的依賴方向只能朝外部技術與 Shared Kernel（若確有純格式需求）；不得 import Bounded Context 或 Entrypoint。Entrypoint 只組裝 Context 與 Foundation，不擁有領域規則。

日後若要在這四種角色之間搬遷 package，遵守：先在 Context 對照表、depguard、query ownership 與 `automation-check` 加入「新舊路徑並存」的支援，不得用放寬 deny 或新增 allow 取代；`db/` 與所有 generated 目錄不得因搬遷手改；同一批完成 `git mv`、Go import、depguard glob、query ownership 例外與文件的更新；每批完成前需通過完整測試、`automation-check`、generated drift check 與 diff check；出現 import cycle、ownership 漂移、generated drift、integration baseline 失敗，或需要新增跨 context allow，就停止該批、先解決治理或設計問題，不與下一批夾帶。

### 決策 7：Context Map 分兩軌，產品領域名稱與治理識別分開維護

同時維護兩種不能互相取代的表示：

| 表示 | 回答的問題 | 穩定識別 | 主要讀者 |
| --- | --- | --- | --- |
| 產品領域地圖 | 創作者能完成什麼成果？各能力如何組成旅程？ | 產品領域名稱、價值流、受控用語 | 產品、設計、領域協作者、新加入的工程師與 Agent |
| 治理地圖（Context 對照表） | 哪個 Bounded Context 擁有事實？程式如何受 CI 約束？ | Boundary ID、package slug、architecture identity | 程式、depguard、query ownership、composition root |

產品領域地圖是受控產品語言，以創作者成果定義各 Bounded Context：

| 價值流 | 產品領域名稱 | Boundary ID／package slug | 創作者成果 |
| --- | --- | --- | --- |
| 創作者空間 | 創作者帳戶與工作區 | `identity` | 在自己的工作區中安全地保存、管理與刪除成果。 |
| Skill 生命週期 | Skill 探索 | `catalog` | 以任務語言找到候選 Skill，理解其符合原因與限制。 |
| Skill 生命週期 | Skill 資產與版本歷史 | `registry` | 建立或 Fork 可追溯的 Skill，保有不可變版本與來源關係。 |
| Skill 生命週期 | Skill 接納與信任 | `ingest` | 在試跑或交付前確認來源、授權、格式與靜態風險。 |
| Skill 生命週期 | Skill 交付與安裝 | `packaging` | 取得可安裝、可追溯且符合散布條件的 Skill 套件。 |
| 試跑與改善 | 試跑情境設計 | `testlab` | 設計 Prompt、資料、驗收條件與權限確認。 |
| 試跑與改善 | Skill 試跑執行 | `run` | 在受控環境中執行、取消或恢復一次 Skill 試跑。 |
| 試跑與改善 | 執行證據 | `trace` | 查看已遮罩的執行過程、成本、錯誤與可追溯證據。 |
| 試跑與改善 | 成果判定與改善 | `eval` | 依驗收條件理解結果、比較試跑並選擇是否採納改善。 |
| 產品營運 | 創作者使用權益與資料生命週期 | `policy` | 在可理解的額度、保存與未來方案規則下使用平台。 |
| 產品營運 | 創作者旅程學習 | `analytics` | 讓產品依匿名化且受控的旅程訊號與回饋持續改善。 |

`skillpkg` 是 Shared Kernel；其餘 Generic 與 composition root 是機制、防腐層或技術基座，不是創作者可直接選擇的產品領域，文件以「共同語言與技術機制」導覽，不硬湊成價值流。Boundary ID／package slug 與現行 Go path 的機械對照由 [docs/development/platform-context-map.md](../development/platform-context-map.md) 擁有，本表只維護價值流與創作者成果的產品語言。

`Core`、`Supporting`、`Shared Kernel`、`Generic` 是治理 metadata，不是產品導覽的第一層分類；產品文件與 `doc.go`／`README` 一類的導覽先以創作者成果與價值流敘述 context，需要精確實作位置時才連到 Boundary ID／package slug，且不以 slug 作主語。「Skill 接納與信任」一類的產品領域名稱描述的是對創作者的成果，不是單一資料表或單一步驟，底層可能包含匯入、驗證與 provenance 等多個環節。新的產品用語必須先能回對產品目標與核心旅程，才納入受控用語表。

### 決策 8：受控術語只管跨文件與跨程序的指稱一致，不管畫面文字

`Skill`、`Run`、`Test Case`、`Trace`（執行證據）、`Evaluation`（成果判定）等詞在 `contracts/`、Go 識別字、資料庫欄位、ADR 內文與需求 ID 上維持一致指稱，避免同一個東西在不同文件裡有不同名字：「Skill 版本」是不可變內容快照，「Skill」是可持續演進、可 Fork 的資產身分，「試跑」是一次 Run，「執行證據」是 Trace，「成果判定」是 Evaluation，且這些定義須與規格允收準則相容。這套一致性只及於「機器與跨文件讀者需要對齊」的場合；使用者在畫面上實際看到的文字，走另一套規則（見 [ADR-020](./ADR-020-design-system-trust-signals-and-screen-words.md)），因為沒有任何機器讀畫面上的名詞，跨文件一致性的理由在那裡不成立，兩邊可以用不同的詞講同一件事。

## 影響

### 正面

- 邊界從「讀過文件的人才知道」變成「CI 會擋」：新增套件、跨 context import 與搬遷路徑都有機械檢查，對 Coding Agent 與人類作者一視同仁。
- Context 對照表是唯一事實來源，避免同一個 package 有兩種身分、或同一個判斷散落在多份手寫清單裡。
- Composition root 與 `internal/` 四種角色讓「誰負責 wiring」「誰是純技術基座」不必靠猜。

### 成本與限制

- 白名單與對照表需要隨架構演進持續維護；同步改動多處（表格、depguard、query ownership）的紀律有摩擦，這個摩擦是刻意的。
- 產品領域名稱與治理識別分軌，代表文件要同時維護兩套對照；新增或改名都要回頭確認兩邊一致。
- 不引入 repository interface、event sourcing 或 CQRS 框架，代表未來若真的出現需要這些機制的場景，得先論證現有方案不足，而不是預先鋪好。
