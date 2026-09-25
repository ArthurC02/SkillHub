# Platform internal：產品價值流與 Bounded Context 導覽

先從創作者在 Skill Hub 能完成的事理解 Platform。下列名稱來自已接受的
[Platform Bounded Context 與 Context Map](../../../docs/adr/README.md#platform-bounded-context-與-context-map)：它們是產品領域導覽的候選語彙，不是已改名的 Go package，也不改變資料所有權。

```text
創作者空間
├── 創作者帳戶與工作區
├── 互動 Skill 創作
└── 創作者 Credit 帳務

Skill 生命週期
├── Skill 探索
├── Skill 資產與版本歷史
├── Skill 接納與信任
└── Skill 交付與安裝

試跑與改善
├── 試跑情境設計
├── Skill 試跑執行
├── 執行證據
└── 成果判定與改善

產品營運
├── 創作者使用權益與資料生命週期
└── 創作者旅程學習
```

## 產品領域對照與入口

| 價值流 | 候選產品領域 | stable Boundary ID／現有 Go package | 擁有的事實或規則 | 主要入口 |
| --- | --- | --- | --- | --- |
| 創作者空間 | 創作者帳戶與工作區 | `identity`／`creator/workspace` | user、session、workspace scope、帳號刪除協調 | `Service`、`Handler` |
| 創作者空間 | 互動 Skill 創作 | `creation`／`creator/creation` | 創作會話、步驟與產出候選 | `Service`、River workers |
| 創作者空間 | 創作者 Credit 帳務 | `credit`／`creator/credit` | credit 帳戶、分錄與成本統計 | `Service`、River workers |
| Skill 生命週期 | Skill 探索 | `catalog`／`skill/discovery` | search document、搜尋投影、搜尋與顯示語意 | `Service`、`Handler` |
| Skill 生命週期 | Skill 資產與版本歷史 | `registry`／`skill/library` | Skill identity、不可變 Skill Version aggregate | `Service`、版本寫入 API |
| Skill 生命週期 | Skill 接納與信任 | `ingest`／`skill/admission` | package provenance、靜態驗證與唯一匯入路徑 | `Service.SaveVersion`、匯入 Handler |
| Skill 生命週期 | Skill 交付與安裝 | `packaging`／`skill/delivery` | downloadable artifact、release gate、manifest | `Service`、`Handler` |
| 試跑與改善 | 試跑情境設計 | `testlab`／`trial/design` | Test Case、dataset、不可變 execution snapshot | `Service`、`Handler` |
| 試跑與改善 | Skill 試跑執行 | `run`／`trial/execution` | Run 狀態機、attempt、sandbox scheduling | `Service`、`Handler`、River workers |
| 試跑與改善 | 執行證據 | `trace`／`trial/evidence` | masked trace event、trace ingest credential、trace read model | `Service`、`Handler` |
| 試跑與改善 | 成果判定與改善 | `eval`／`trial/improvement` | evaluation、suggestion、採納流程 | `Service`、`Handler`、event consumer |
| 產品營運 | 創作者使用權益與資料生命週期 | `policy`／`product/entitlements` | quota、retention 與未來計費判定 | `EnforceQuota`、`DownloadRetention` |
| 產品營運 | 創作者旅程學習 | `analytics`／`product/learning` | funnel event、feedback report、analytics retention | `Service`、`Handler` |

各 leaf Context 的 `doc.go` 是該產品領域的本地導覽：先說創作者成果，再列 owner facts、跨界關係、公開面與刻意不擁有的事。

## 架構治理位置

產品價值流不是 import 規則。以下是目前 Go layout 的 machine-readable identity；`Core`／`Supporting` 是治理 metadata，不是產品領域名稱。完整事實來源是 [Domain Memory](../../../docs/domain-memory/) 的已審查 Context 與 [`architecture-identity.yaml`](../architecture-identity.yaml)。

```text
internal/
├── creator/{workspace,creation,credit}                             Bounded Context packages
├── skill/{discovery,library,admission,delivery}                    Bounded Context packages
├── trial/{design,execution,evidence,improvement}                   Bounded Context packages
├── product/{entitlements,learning}                                 Bounded Context packages
├── shared/skillpkg                                                 Shared Kernel
├── foundation/{persistence,messaging,storage,observability,integration,runtime}
                                                                    Generic mechanisms
├── foundation/persistence/db/gen                                   Generated persistence
└── entrypoint/{api/apiserver,api/gen,wiring,worker}                Composition roots and generated transport
```

`shared/skillpkg` 是共同語言的純函式庫；`foundation/*` 是機制、ACL 與技術基座；`foundation/persistence/db/gen` 是 generated persistence。`entrypoint/` 底下四件事各自分開：`api/apiserver` 是 API process 的組裝根、`api/gen` 是 generated transport、`wiring` 讀環境變數並交出設定好的相依、`worker` 是 worker process 的組裝根。這些都不是創作者直接選擇的產品領域。

`cmd/` 留在 `apps/platform/cmd/`，因為它是可執行程序入口。Foundation、Shared Kernel 與 Entrypoint 仍必須留在 `internal/`，才能保有 Go 的編譯器級私有邊界；不可為了收納而移到 `apps/platform/` 直層。收斂範圍與驗證基準見 [DDD 邊界收斂報告](../../../docs/plans/mvp/m4/report-platform-ddd-boundary-convergence-2026-08-19.md)，逐條 gate 見 [Platform Bounded Context 與 Context Map](../../../docs/adr/README.md#platform-bounded-context-與-context-map)。

## 跨 Context 的四種關係

1. **同步 owner facts（Customer–Supplier）**：當下請求必須知道的事實，consumer
   呼叫 owner 的窄 Service API／DTO；不跨界傳遞 sqlc generated row 或 `*gen.Queries`。
2. **領域事件（Published Language）**：後續反應走 Transactional Outbox，consumer
   必須冪等；事件不能取代當下需要的同步事實。
3. **Shared Kernel**：只有 `skillpkg` 可供 contexts 共同依賴，且只能放無狀態、無政策
   的共同語言；變更需考慮所有使用它的 context。
4. **Composition root 注入**：不能由 Generic package 反向 import domain，也不能形成
   cycle 時，在 process root 組裝 owner API／callback；這不是 consumer 自己建 Service。

跨 Context import 必須先在 Registry 立下 [dependency policy](../../../docs/domain-memory/registry/dependency-policies.json)，同一批更新
[`apps/platform/.golangci.yml`](../.golangci.yml)；資料 query 的 owner 由
[`db/query-owners.yaml`](../../../db/query-owners.yaml) 宣告與檢查。Read ownership 的
細節見 [Query 與寫入所有權](../../../docs/adr/README.md#query-與寫入所有權)，
每個 package 只能有一種 architecture identity 的規則見
[Platform Bounded Context 與 Context Map](../../../docs/adr/README.md#platform-bounded-context-與-context-map)。

## 為何不建 `domain/application/infrastructure` 子目錄

本專案不把每個 context 拆成三層實體子目錄。多數流程以 transaction script 為合適
模式，額外 Repository interface／分層只會增加 Go export surface、import cycle 與搬移
成本，並不讓 Context 邊界更強。Aggregate 僅用在 Run、Skill Version、Evaluation 等確有
不變量的地方；package 本身就是模組邊界。

因此不要為了目錄外觀重命名或搬移 package，也**禁止**在 consumer 的方法內現場建構
另一個 context 的 `Service`。API process 的 wiring 在
[`apiserver.NewApp`](entrypoint/api/apiserver/app.go)，worker process 的在
[`worker.BuildWorkers`](entrypoint/worker/worker.go)；兩者都不讀環境變數，讀的那一層是 [`entrypoint/wiring`](entrypoint/wiring/)，`cmd/` 底下的 `main.go` 只把兩者接起來。maintenance 與 reindex 則各自於其 deployment
unit 的 root 建構所需服務。詳見 [Platform Bounded Context 與 Context Map](../../../docs/adr/README.md#platform-bounded-context-與-context-map)。

## 互動創作的後續能力

[互動創作](../../../docs/adr/README.md#互動創作) 由 Python LangGraph 編排創作，Go 持有會話、授權、成本、版本與 Run 的事實。尚未實作的新能力必須先定義契約，新增套件須先登記 owner（Bounded Context 進 [Domain Memory](../../../docs/domain-memory/)，技術套件進 [`architecture-identity.yaml`](../architecture-identity.yaml)）。[GEN-007～012](../../../docs/plans/02-specifications-and-acceptance-criteria.md) 是允收來源。
