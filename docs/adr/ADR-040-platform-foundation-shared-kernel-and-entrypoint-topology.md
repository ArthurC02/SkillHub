# ADR-040：Platform 的 Foundation、Shared Kernel 與 Entrypoint 拓撲

- 狀態：Accepted
- 日期：2026-08-22
- 決策者：產品負責人、架構規劃
- 關係：補充 [ADR-031](./ADR-031-artifact-role-repository-layout.md)、[ADR-032](./ADR-032-ddd-bounded-context-governance-for-platform.md)、[ADR-033](./ADR-033-sqlc-query-ownership-and-cross-context-write-enforcement.md)、[ADR-035](./ADR-035-read-ownership-enforcement-and-context-map-completeness.md)、[ADR-037](./ADR-037-product-analytics-and-package-architecture-identities.md) 與 [ADR-038](./ADR-038-platform-product-domain-language-and-value-stream-navigation.md)；不回寫其既有決策。

## 背景

ADR-038 已使 `internal/creator`、`internal/skill`、`internal/trial` 與 `internal/product` 直接呈現創作者價值流；11 個 Bounded Context 已位於其產品領域路徑。此 ADR 提出時，`audit`、`outbox`、`objreconcile`、`llmclient`、`platform/*`、`apiserver` 與 `api/gen` 仍散在 `internal/` 第一層，容易被誤認為未命名的領域；下列決策已在同日依序落實。

它們不是 Bounded Context，也不是 Shared Kernel：它們提供 Generic 技術機制、組裝或 generated transport。唯有 `skillpkg` 是跨 Context 共用且帶有 Skill 套件格式語言的 Shared Kernel。

## 決策

### 1. 保留 `apps/platform/internal/` 的 Go 私有邊界

不把 Foundation、Shared Kernel 或 Entrypoint 的 Go package 直接移到 `apps/platform/` 第一層。Go 的 `internal` 規則會限制這些 package 只被 `apps/platform/` 樹內程式 import；移到直層會把平台實作意外變成其他 app 或 library 可依賴的公開 surface，與 ADR-031 的產物角色及 ADR-032 的平台邊界相違。

`apps/platform/cmd/` 保持 process entrypoint；它不是可被 import 的底層 library，也不屬於 `internal/` 的收納問題。

### 2. 採用四個清楚的 `internal/` 收納角色

```text
internal/
├── creator/  skill/  trial/  product/             # Bounded Contexts
├── shared/skillpkg/                               # 唯一 Shared Kernel
├── foundation/                                    # Generic 技術基座，無領域規則
│   ├── persistence/{db,partition,pgconv}/
│   ├── messaging/{queue,outbox}/
│   ├── storage/{objstore,objreconcile}/
│   ├── observability/{metrics,audit}/
│   ├── integration/llmclient/
│   └── runtime/{envx,httpx}/
└── entrypoint/api/{apiserver,gen}/                 # HTTP composition 與 generated transport
```

`shared/skillpkg` 只收納共同的 Skill 套件格式、讀取與純驗證；不得以「共用」為理由收納 Generic 技術碼、領域 Service、資料存取或政策。Foundation 的依賴方向只向外部技術與 Shared Kernel（若確有純格式需求）開放；不得 import Bounded Context 或 Entrypoint。Entrypoint 只能組裝 Context 與 Foundation，不擁有領域規則。

### 3. 現有 package 的目標歸屬

| 現行位置 | 目標位置 | 角色 |
| --- | --- | --- |
| `skillpkg` | `shared/skillpkg`（已完成） | 唯一 Shared Kernel |
| `platform/db/gen` | `foundation/persistence/db/gen`（已完成；generated persistence） | generated persistence；只由 sqlc 更新 |
| `platform/partition`、`platform/pgconv` | `foundation/persistence/*`（已完成） | 持久化機制與轉換 |
| `platform/queue`、`outbox` | `foundation/messaging/*`（已完成） | 佇列與可靠訊息傳遞 |
| `platform/objstore`、`objreconcile` | `foundation/storage/*`（已完成） | 物件儲存與收斂 |
| `platform/metrics`、`audit` | `foundation/observability/*`（已完成） | 可觀測性與稽核機制 |
| `llmclient` | `foundation/integration/llmclient`（已完成） | 外部模型閘道 ACL |
| `platform/envx`、`platform/httpx` | `foundation/runtime/*`（已完成） | 程序環境與 HTTP 共用機制 |
| `apiserver`、`api/gen` | `entrypoint/api/{apiserver,gen}`（已完成） | HTTP composition root 與 generated transport |

Go `package` 名稱、stable Boundary ID、需求 ID、資料 owner 與跨 Context 規則不因路徑搬遷而改名。2026-08-22 已完成 Shared Kernel、Foundation、generated persistence 與 API Entrypoint 的搬遷；generated output 的內容仍只由既有產生器管理，目錄移動不改變其來源所有權。

### 4. 搬遷的串行規則與 gate

每一批只能搬一個收納角色或其內低耦合子組，並必須由單一 Writer 串行完成：

1. 先在 ADR-032 的可解析 Context Map、ADR-037 package architecture identity、`.golangci.yml` depguard、`db/query-owners.yaml` 與 `devctl automation-check` 中加入「stable identity 對應現行／目標 nested path」的支援；不得以放寬 deny 或新增 allow 取代。
2. `db/queries/`、`db/migrations/`、`db/sqlc.yaml` 與所有 generated output 不隨目錄外觀手改或分拆。若來源確需調整，主 Agent 依 ADR-030／AGENTS.md 串行執行 `task gen:sql`、`task gen:openapi` 與 `task gen:check`；generated 檔案只由生成器更新。
3. 每批在 `git mv` 後，同一批同步更新所有 Go import、depguard glob、raw-SQL `path@function` allow、architecture identity mapping、文件及必要測試 fixture；不更動 Boundary ID、query owner 或領域 API。
4. 每批完成前必過 `go -C apps/platform test ./...`、`go -C tools/devctl run . automation-check`、`go -C tools/devctl run . gen --check --scope=all` 與 `git diff --check`；有 PostgreSQL 時另跑 disposable integration baseline。
5. 任一項出現 import cycle、ownership 漂移、generated drift、integration baseline 失敗或需要新增跨 Context allow，即停止該批、先修治理或設計，不夾帶下一批。

## 考慮過的替代方案

- **所有共用程式放 `sharedkernel/`**：拒絕。DDD Shared Kernel 是共同領域模型，不是技術工具箱；這會混淆 `skillpkg` 與 Generic 技術基座。
- **直接放在 `apps/platform/foundation/`**：拒絕。失去 Go `internal` 的編譯器級私有邊界，讓其他 app 可建立不受治理的相依。
- **每個 Context 建 `domain/application/infrastructure`**：拒絕。這是技術分層，不會加強本專案已以 package、ownership、Service API 與 outbox 表達的 Bounded Context；還會增加 cycle 與儀式化抽象。

## 影響

- 正面：目錄可同時表達產品領域、Shared Kernel、技術基座與程序入口；Generic package 不再看似隱藏領域。
- 代價：這是純路徑重構，但觸及 import、CI glob、raw-SQL 精確例外與 generated 輸出路徑，必須分批且串行。
- 不變：DDD 的資料 owner、query ownership、交易邊界、Published Language、OpenAPI-first、單一 Writer 與所有安全鐵律維持不變。

## 待決策

- 產品負責人已於 2026-08-22 接受本 ADR。Phase 5 的 Shared Kernel、Foundation、generated persistence 與 API Entrypoint 已依單一 Writer 逐批搬遷完成，保留 `package` 名稱、公開 API 與 stable Boundary ID。待最終全域驗收（Go tests、automation check、generated drift check、PostgreSQL integration baseline 與 diff check）確認後，才把此遷移批標記為完全驗收。
