# Platform internal：產品價值流與 Bounded Context 導覽

先從創作者在 Skill Hub 能完成的事理解 Platform。下列名稱來自已接受的
[ADR-038](../../../docs/adr/ADR-038-platform-product-domain-language-and-value-stream-navigation.md)：它們是產品領域導覽的候選語彙，不是已改名的 Go package，也不改變資料所有權。

```text
創作者空間
└── 創作者帳戶與工作區

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

產品價值流不是 import 規則。以下是目前 Go layout 的 machine-readable identity；`Core`／`Supporting` 是治理 metadata，不是產品領域名稱。完整事實來源是 [ADR-032](../../../docs/adr/ADR-032-ddd-bounded-context-governance-for-platform.md)。

```text
internal/
├── creator/workspace skill/discovery skill/library skill/admission skill/delivery                    Bounded Context packages
├── trial/design trial/execution trial/evidence trial/improvement product/entitlements product/learning Bounded Context packages
├── shared/skillpkg                                                Shared Kernel
├── foundation/{persistence,messaging,storage,observability,integration,runtime}
                                                                    Generic mechanisms
├── foundation/persistence/db/gen                                   Generated persistence
└── entrypoint/api/{apiserver,gen}                                  HTTP composition / generated transport
```

`shared/skillpkg` 是共同語言的純函式庫；`foundation/*` 是機制、ACL 與技術基座；`foundation/persistence/db/gen` 與 `entrypoint/api/{apiserver,gen}` 分別是 generated persistence、composition root 與 generated transport，不是創作者直接選擇的產品領域。資料夾遷移已完成；現行拓撲、收斂範圍與驗證基準見 [DDD 邊界收斂報告](../../../docs/plans/mvp/m4/report-platform-ddd-boundary-convergence-2026-08-19.md) 與 ADR-032／038／040。

## 現行拓撲（待最終驗收）

產品領域、Shared Kernel、Foundation 與 Entrypoint 的路徑均已完成搬遷；下列即為目前 import path。詳細 gate 見已 Accepted 的 [ADR-040](../../../docs/adr/ADR-040-platform-foundation-shared-kernel-and-entrypoint-topology.md)：

```text
internal/
├── creator/  skill/  trial/  product/             # Bounded Contexts（現況）
├── shared/skillpkg/                               # 唯一 Shared Kernel（現況）
├── foundation/                                    # Generic 技術基座（含 persistence/db/gen）
│   ├── persistence/  messaging/  storage/
│   ├── observability/  integration/  runtime/
└── entrypoint/api/                                 # HTTP composition／generated transport
```

`cmd/` 留在 `apps/platform/cmd/`，因為它是可執行程序入口。Foundation、Shared Kernel 與 Entrypoint 仍必須留在 `internal/`，才能保有 Go 的編譯器級私有邊界；不可為了收納而移到 `apps/platform/` 直層。

## 跨 Context 的四種關係

1. **同步 owner facts（Customer–Supplier）**：當下請求必須知道的事實，consumer
   呼叫 owner 的窄 Service API／DTO；不跨界傳遞 sqlc generated row 或 `*gen.Queries`。
2. **領域事件（Published Language）**：後續反應走 Transactional Outbox，consumer
   必須冪等；事件不能取代當下需要的同步事實。
3. **Shared Kernel**：只有 `skillpkg` 可供 contexts 共同依賴，且只能放無狀態、無政策
   的共同語言；變更需考慮所有使用它的 context。
4. **Composition root 注入**：不能由 Generic package 反向 import domain，也不能形成
   cycle 時，在 process root 組裝 owner API／callback；這不是 consumer 自己建 Service。

跨 Context import 必須同一批更新 ADR-032 附錄 A 與
[`apps/platform/.golangci.yml`](../.golangci.yml)；資料 query 的 owner 由
[`db/query-owners.yaml`](../../../db/query-owners.yaml) 宣告與檢查。Read ownership 的
細節見 [ADR-035](../../../docs/adr/ADR-035-read-ownership-enforcement-and-context-map-completeness.md)，
每個 package 只能有一種 architecture identity 的規則見
[ADR-037](../../../docs/adr/ADR-037-product-analytics-and-package-architecture-identities.md)。

## 為何不建 `domain/application/infrastructure` 子目錄

本專案不把每個 context 拆成三層實體子目錄。多數流程以 transaction script 為合適
模式，額外 Repository interface／分層只會增加 Go export surface、import cycle 與搬移
成本，並不讓 Context 邊界更強。Aggregate 僅用在 Run、Skill Version、Evaluation 等確有
不變量的地方；package 本身就是模組邊界。

因此不要為了目錄外觀重命名或搬移 package，也**禁止**在 consumer 的方法內現場建構
另一個 context 的 `Service`。API process 的 wiring 在
[`apiserver.NewApp`](entrypoint/api/apiserver/app.go)，worker process 的 wiring 在
[`cmd/worker/main.go`](../cmd/worker/main.go)；maintenance 與 reindex 則各自於其 deployment
unit 的 root 建構所需服務。詳見 ADR-032 §5。

## 規劃中的互動創作（尚未實作）

[ADR-067](../../../docs/adr/ADR-067-interactive-skill-creation-with-langgraph.md) 規劃由 Python LangGraph 編排創作，Go 持有會話、授權、成本、版本與 Run 的事實。現有套件地圖不代表這些新能力已存在；實作前先定義契約，新增套件須依 ADR-032 登記 owner。[GEN-007～012](../../../docs/plans/02-specifications-and-acceptance-criteria.md) 是允收來源。
