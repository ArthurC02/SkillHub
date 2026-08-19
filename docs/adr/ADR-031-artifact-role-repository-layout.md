# ADR-031：依產物角色劃分 Repository 頂層目錄

- 狀態：Accepted
- 日期：2026-08-19
- 決策者：產品負責人、架構規劃
- 取代：[ADR-024](./ADR-024-top-level-repository-layout.md) 的頂層收納語意
- 修訂：[ADR-019](./ADR-019-monorepo-structure-and-cicd.md) §1 的目錄結構與相關路徑

## 背景

ADR-024 先把「會跑的」與「給人讀的」分開，但 `apps/` 與 `services/` 同時承載可部署程式，名稱無法回答一個元件應放哪裡。結果同樣具有啟動入口與獨立部署生命週期的 Web、Platform、LLM、Sandbox 被分在兩個頂層；新增元件時仍須靠既有案例猜測。

本次只修正收納語意，不改變服務邊界、部署拓撲、語言分工、Go module 邊界或產物名稱。

## 決策

頂層目錄依「產物在系統中的角色」判斷，不依語言、團隊或是否含程式碼判斷：

| 目錄 | 唯一判準 | 現有內容 |
| --- | --- | --- |
| `apps/` | 可獨立啟動、建置或部署的產品程式；同一部署單元可有多個 `cmd/` 入口 | `web`、`platform`、`llm`、`sandbox` |
| `packages/` | 被其他程式 import／link 的可重用 library；本 repo 目前主要是提交版 generated client／transport model | `api-client-ts`、`api-stub-py` |
| `contracts/` | 跨程序或跨語言介面的唯一來源，不是 generated output | OpenAPI、event schema、packaging schema |
| `db/` | 持久化結構與查詢的來源 | migration、sqlc query/config、資料庫測試 |
| `infra/` | 部署、runtime image、網路、節點與 observability 設定 | Compose、image、egress、alerts |
| `tools/` | 開發、CI、資料維護與維運命令，以及只能隨該命令演進的緊密 fixture | `devctl`、codegen、QA corpus、golden set |
| `docs/` | 敘事性文件與歷史紀錄，不被產品程式 import 或執行 | 計畫、ADR、手冊、spike 墓碑 |

`.github/`、`.devcontainer/` 與 repo 根層的 `Taskfile.yml`、語言版本檔、環境範本等是 repository 控制面與入口，保留在工具預期的位置，不另包一層。

判斷順序如下：可啟動產品程式進 `apps/`；只供 import 的 library 進 `packages/`；介面或資料庫來源分別進 `contracts/`、`db/`；部署與 runtime 設定進 `infra/`；操作者主動執行的 repo 命令與其不可分 fixture 進 `tools/`；純敘事進 `docs/`。因此「含程式碼」本身不是放進 `apps/` 的充分條件。

`services/` 不再是有效頂層。既有目錄原樣搬為 `apps/platform/`、`apps/llm/`、`apps/sandbox/`；內部名稱、執行入口與責任邊界不變，不提供相容 symlink 或重複目錄。

## 歷史文件與執行路徑

- 會被建置、測試、codegen、CI、Compose 或開發工具讀取的路徑必須使用新位置。
- ADR 內文與已凍結里程碑文件保留當時的路徑，避免改寫歷史；由本 ADR 與被取代標記說明後續位置。
- generated file 不手改；其來源與輸出位置仍由 ADR-030 的 generation contract 管理。

## 後果

### 正面

- 新元件可用單一角色判準定位，不再猜 `apps` 與 `services` 的差異。
- 四個產品程式位於同一頂層，CI path filter、Task 與 module path 能直接反映部署單元。
- library、介面來源、持久化來源、部署設定與命令仍各自保有清楚所有權，沒有把「所有程式碼」混成單一目錄。

### 代價

- Go module/import path 與本機 Python 虛擬環境需隨搬移更新或重建。
- 外部書籤若指向舊 GitHub path，必須改用新路徑；不以永久相容層換取短期便利。

## 不變事項

- Platform 與 Sandbox 仍是彼此獨立的 Go module；執行平面不得存取核心資料庫。
- Python 仍只提供能力，政策與狀態機仍由 Go 擁有。
- OpenAPI-first、generated file 禁止手改、單一 Writer 與所有安全鐵律不變。
