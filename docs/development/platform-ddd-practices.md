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
