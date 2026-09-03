# Platform DDD 開發心得與實務指南

本指南把 Platform 在 MVP 期間收斂 Bounded Context 的實作經驗，轉成後續開發時可執行的判斷與檢查。它不是新的架構決策，也不取代任何 ADR。

## 定位與事實來源

遇到衝突時，依下列優先序判斷：

1. Accepted ADR 是已定案的架構事實；Boundary 對照與依賴規則以 [ADR-032](../adr/ADR-032-ddd-bounded-context-governance-for-platform.md) 為準，query 規則以 [ADR-033](../adr/ADR-033-sqlc-query-ownership-and-cross-context-write-enforcement.md) 與 [ADR-035](../adr/ADR-035-read-ownership-enforcement-and-context-map-completeness.md) 為準。
2. [產品目標](../plans/01-goals-and-plan.md)、[規格](../plans/02-specifications-and-acceptance-criteria.md)、[工作項目](../plans/03-work-items.md) 與 [待辦](../plans/04-backlog-and-handoffs.md) 說明產品範圍、允收與尚未完成的事。
3. [Platform internal 導覽](../../apps/platform/internal/README.md) 說明目前可走讀的程式位置；[M4 邊界收斂報告](../plans/mvp/m4/report-platform-ddd-boundary-convergence-2026-08-19.md) 是搬遷時點的證據，不是新的規則來源。
4. 本文件與 [開發自動化手冊](./automation.md) 是操作說明；發現其與 ADR 不一致時，先依 ADR 行動，再修正文檔。

不要把已完成的程式收斂誤寫成 MVP 或部署驗收已完成；現行未結項目仍以待辦與 release checklist 為準。

## 本專案的 DDD 定義

DDD 在這裡首先是**產品事實與規則的 owner boundary**，不是把每個資料夾套成 `domain/application/infrastructure`，也不是把技術共用碼統稱為 Shared Kernel。每個 Bounded Context 對其事實、不變量與公開協作面負責；package 是 Go 中可被檢查的模組邊界。

產品文件應優先使用創作者能理解的領域語言，例如「Skill 接納與信任」或「試跑執行」。這套受控語言與價值流由 [ADR-038](../adr/ADR-038-platform-product-domain-language-and-value-stream-navigation.md) 定義。架構治理則使用 stable Boundary ID、package identity、dependency rule 與 query owner；兩者互相對照，但不能互相取代。`Core`、`Supporting`、`Generic` 是治理 metadata，不是產品領域名稱（[ADR-037](../adr/ADR-037-product-analytics-and-package-architecture-identities.md)）。

## 從產品語言走到程式碼

先從使用者成果與被改變的事實找 owner，再查 ADR-032 的對照表取得 stable Boundary ID 與當前 Go path。路徑可因收納改善而改變，stable ID、資料 owner、需求 ID 與 Go package clause 不會因搬遷自動改名（[ADR-040](../adr/ADR-040-platform-foundation-shared-kernel-and-entrypoint-topology.md)）。

實作前至少回答：

- 哪一個產品領域對這條規則或狀態負最終責任？
- 它是該 context 的不變量，還是對方 owner 的事實？
- 公開給別人的最窄協作面是什麼，而不是哪個 table 或 sqlc row 最方便？

答案若只是「這段用 PostgreSQL／HTTP／queue」，那是在描述技術，不足以決定 Context。

## 跨 Context 協作：先選關係，再寫 import

| 情境 | 合法協作 | 實作重點 | 不要做 |
| --- | --- | --- | --- |
| 當下請求必須取得另一方的權威事實 | 同步呼叫 owner 的窄 Service API／DTO | consumer 只依公開語意；保留 owner 的驗證與授權 | 讀對方 table、傳遞 `*gen.Queries` 或 sqlc row |
| 對方狀態變更後可非同步反應 | Transactional Outbox 領域事件 | 事件與狀態變更同交易；consumer 冪等；事件是 Published Language | 以事件取代當下必需的權威查詢 |
| 多個 context 真的共享無狀態的套件語言 | `shared/skillpkg` | 只放套件格式、讀取與純驗證；變更視為多方協議 | 把 Service、資料存取、政策或 Generic 工具塞進去 |
| 需要避免反向依賴或 cycle，且由程序負責組裝 | composition injection | 在 `entrypoint`／process root 注入 owner API 或 callback | 在 consumer 方法中現場建構另一個 context 的 Service |

前兩種關係的理由與強制規則見 ADR-032；Shared Kernel 的單一範圍與組裝位置見 ADR-040。若需要新增跨界 import，必須同批更新 ADR-032 附錄 A 與 `apps/platform/.golangci.yml`，讓 depguard 反映新關係。

### 同步 owner 讀取的兩種形狀（2026-09-03 新增）

上表第一列在程式裡有兩種寫法，**判準是對方回給你的是不是 generated row，或建立在 generated row 之上的型別**：

- **owner 的公開面本身就是領域語言** → consumer 直接持有 `*owner.Service` 欄位，呼叫那個窄方法（例如 Evaluation 持有 `TestLab` 取不可變快照）。
- **owner 的回傳是 sqlc row 或以它為基礎的型別，或直接 import 會成環** → consumer **自己宣告 `XxxFacts` 與 `Read*` 函式欄位**，由 `apiserver.NewApp` 裡的 `wireXxxReaders(...)` 翻譯後注入；領域檔因此完全不 import 對方。Catalog、Packaging、Evaluation 的 Registry 讀取都是這一種。這是 [ADR-034](../adr/ADR-034-cross-context-writes-close-by-inversion-not-by-events.md) 的**讀取側鏡像**——寫入側用同一手法拆掉 `registry -> catalog` 的編譯期環。

兩件容易做錯的事：

- **Facts 只放你真的會用到的欄位。** 同樣是讀 Skill，`catalog.SkillFacts` 有十二個欄位、`eval.SkillFacts` 只有四個。把 owner 的欄位抄滿，等於把它的形狀複製一份放在自己家，對方改欄位你還是得跟著改——ACL 的意義就沒了。
- **翻譯只發生在 composition root。** `wireXxxReaders(...)` 與 `xxxFacts(...)` 轉換函式住在 `app.go`；一旦下放到領域套件，那個套件就得 import 對方，整個做法就白做了。

同理，**不要靠讀合作者的內部欄位來確認它接好了**（`s.TestLab.Pool != nil` 這種寫法）：知道對方有一個 `Pool`，就是知道對方怎麼實作。守衛只判 `s.TestLab == nil`。

## Query ownership 不等於資料庫方便性

`db/query-owners.yaml` 宣告每條 query 的 owner；直接透過另一個 context 的 query 讀寫都是架構邊界問題。新的跨 context write 與 read 都會被強制拒絕，不能靠擴增 allowlist 過關。應改為 owner Service 的公開操作、自己的投影，或在確有理由時先提出 ADR。[ADR-033](../adr/ADR-033-sqlc-query-ownership-and-cross-context-write-enforcement.md) 解釋 write，[ADR-035](../adr/ADR-035-read-ownership-enforcement-and-context-map-completeness.md) 解釋 read 與完整性對帳。

修改或新增 `db/queries/*.sql` 時，同批更新 owner 宣告；generated persistence 仍是產物，依 [自動化手冊](./automation.md) 的單一 Writer 流程生成，不手改。

## 四種資料夾角色

`apps/platform/internal/` 的目錄先表達角色，再表達技術：

- `creator/`、`skill/`、`trial/`、`product/`：產品領域的 Bounded Context packages；其 leaf package 才是 owner 邊界。
- `shared/skillpkg/`：唯一 Shared Kernel，放共同領域語言的純函式。
- `foundation/`：持久化、訊息、儲存、觀測、整合與 runtime 等 Generic 機制；不得承載領域政策或反向 import domain。
- `entrypoint/api/`：HTTP composition 與 generated transport；`cmd/` 則是可執行程序入口。兩者負責組裝，不擁有產品規則。

完整拓撲理由、禁止把所有共用碼塞入 Shared Kernel 的原因，見 ADR-040。不要為了外觀再建立技術三層目錄；這不會強化 owner boundary，反而擴大 Go export surface 與 cycle 風險。

### 第一層目錄 → 底下有哪些 package → 它們是什麼邊界（2026-08-29 新增）

上面四段是規則，這張表是**規則的當前實例**——`apps/platform/internal/` 的七個第一層目錄，一列一個。**寫它的理由是稽核指出的一件事**：一個新來的人（或 Agent）要判斷「我這支檔案該放哪」時，讀到的是四段散文加一份 ADR 的十九列對照表，而**兩者之間沒有一個「先看目錄」的入口**。

| 第一層目錄 | 底下的 package | 邊界類型 | 這一層的判準（一句話） |
| --- | --- | --- | --- |
| `creator/` | `workspace` | **Core Context** | 帳戶與工作區的身分與歸屬。**Workspace Scope 是這裡定義的**，其他 context 只是遵守它（鐵律 3） |
| `skill/` | `admission`、`discovery`、`library`、`delivery` | **Core Context**（四個，各自是 owner 邊界） | Skill 這個資產的一生：**進來**（接納與信任）、**被找到**（探索）、**被保存**（版本歷史，不可變）、**被帶走**（打包與安裝）。**四者不共用 owner**——`skill/` 只是路徑前綴，不是一個邊界 |
| `trial/` | `design`、`execution`、`improvement`、`evidence` | **Core Context** ×3 ＋ **Supporting** ×1（`evidence`） | 「跑一次」的一生：**設計情境**、**執行與狀態機**（唯一事實來源，鐵律 5）、**判定與改善**、**留下證據**。`evidence` 是 Supporting——它服務前三個，不定義它們 |
| `product/` | `entitlements`、`learning` | **Supporting Context** | 使用者的權益與資料生命週期、以及產品自己的學習（分析與漏斗）。**Supporting 的意思是它們可以晚一步、也可以被換掉**，Core 不可以 |
| `shared/` | `skillpkg` | **Shared Kernel（唯一一個）** | 共同領域語言的**純函式**。**新增第二個 Shared Kernel 要先改 ADR-032／040**——把共用碼往這裡塞是這份文件明文禁止的那條路 |
| `foundation/` | `persistence`、`messaging`、`observability`、`storage`、`integration`、`runtime` | **Generic** | 機制不是政策。**不得承載領域規則，不得反向 import domain**——這一條由 depguard ＋ `devctl automation-check` 兩道守著 |
| `entrypoint/` | `api`、`worker` | **組裝，不是邊界** | 只做 composition：`api` 是 HTTP 與 generated transport，`worker` 是佇列消費者（鐵律 7）。**兩者不擁有任何產品規則**；領域 Service 一律由 `apiserver.NewApp` 注入，**禁止在方法內現場建構其他 context 的 Service** |

**這張表怎麼用，以及它怎麼會過期**

- **它是導覽，不是事實來源。** 逐 package 的 Boundary ID、需求 ID 前綴與跨 context 例外清單以 **[ADR-032](../adr/ADR-032-ddd-bounded-context-governance-for-platform.md) §1 為準**，那一份受 `devctl automation-check` 機械對帳；**這一張表沒有機器在守**。兩者不一致時，以 ADR-032 為準並回來修這一張。
- **新增一個 package 的順序是硬的**：先在 ADR-032 §1 登記 → 再建目錄。反過來做的話 CI 會在你寫第一行程式之前就紅。
- **跨 context 的新 import 要同一個 commit 改兩處**：ADR-032 附錄 A 與 `apps/platform/.golangci.yml` 的 depguard 規則。
- **一個好用的自我檢查**：如果你正在想「這支檔案放 `foundation/` 比較方便」，先問它有沒有一個**領域**的理由會讓它變。**會變的東西不屬於 Generic**——那正是這一層與 Supporting 的分界。

## 新增、搬遷或改變 Context 的 checklist

1. 以產品語言寫出使用者成果、owner facts、不變量與不擁有的事；確認需求 ID 與計畫文件的允收準則。
2. 查 ADR-032 對照表：既有 context 就留在其 owner；新 package 或新 boundary 先更新該表，並符合 ADR-037 的單一 architecture identity。
3. 選定跨界關係：同步 owner DTO、outbox event、`skillpkg` 或 composition injection。若是 import，同批調整 ADR-032 附錄 A 與 depguard。
4. 為新增／刪除 query 更新 `db/query-owners.yaml`；不要新增例外清單來掩蓋跨 owner 存取。
5. 依目錄角色放置程式；generated source 與 output 依 automation 規範由主 Writer 串行處理。
6. 更新受影響的 `doc.go`、internal 導覽與計畫／ADR 引用，使產品語言、stable ID、路徑三者可追溯。
7. 跑受影響測試、`go -C tools/devctl run . automation-check`、必要的 generation check 與 `git diff --check`；再審查是否意外產生跨界 import 或 query ownership 漂移。

## 常見反模式與處置

| 反模式 | 為何是問題 | 處置 |
| --- | --- | --- |
| 以 `repository`、transport 或資料庫技術決定資料夾 | 技術依賴不等於產品 owner | 回到被改變的事實與不變量，放入 owner context |
| 消費者直接碰別人的 table、query 或 generated row | 繞過 owner 規則，讓 query ownership 失效 | 改呼叫 owner API、建立合法投影或以事件更新 read model |
| 把所有重用碼塞進 Shared Kernel | 讓共用工具反向污染所有 context | Generic 放 `foundation/`；只有共同領域語言放 `shared/skillpkg` |
| 為取得協作功能在方法內 `NewService` | 隱藏依賴、製造反向 import 與 lifecycle 不一致 | 由 composition root 注入窄介面、DTO 或 callback |
| 用大規模目錄搬遷宣稱完成 DDD | 路徑不能證明 ownership、事件與資料邊界正確 | 先完成 Context Map、依賴／query 檢查與可驗證的小批搬遷 |
| 用 allowlist 讓新跨界存取通過 | 把短期例外變成長期架構債 | 改協作設計；無法裁定時提 ADR |

## 最小驗證與何時需要 ADR

一般 context 內的行為變更，至少做 scoped test、automation check 與 diff check；觸及 OpenAPI、SQL 或 generated output 時，遵循 [自動化手冊](./automation.md) 的 generation 順序。CI gate 是防回歸，不是「產品或部署已完成」的宣告。

以下任一情況先寫或更新 ADR，再開始跨界實作：新增或拆分 Bounded Context、改變 owner facts／不變量、增加新的 context dependency、改變跨界協作型態、擴大 Shared Kernel 語意、修改 query ownership 原則，或改變 `internal/` 四角色拓撲。若只是依已定規則新增 owner 內行為，則更新對應計畫、程式與本地導覽即可。
