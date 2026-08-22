# ADR-038：Platform 產品領域語言與價值流導覽

- 狀態：Accepted
- 日期：2026-08-22
- 決策者：產品負責人（2026-08-22 授權自動完成規劃與工作項目）
- 關係：補充 [ADR-002](./ADR-002-domain-boundaries-and-ownership.md)、[ADR-032](./ADR-032-ddd-bounded-context-governance-for-platform.md)、[ADR-035](./ADR-035-read-ownership-enforcement-and-context-map-completeness.md) 與 [ADR-037](./ADR-037-product-analytics-and-package-architecture-identities.md)；不取代其既有治理與所有權決策

## 背景

ADR-032、ADR-035 與 ADR-037 已將 Platform 收斂為受 CI 強制的 Bounded-Context modular monolith：每個 package 有唯一 architecture identity、跨 Context 讀寫由 owner API 與 query ownership 管理、composition root 組裝跨界協作。這些是必要的架構治理事實。

但目前 Context Map 的首要閱讀單位仍多為 `registry`、`ingest`、`orchestration`、`policy` 等平台內部能力詞，並以 Core／Supporting／Shared Kernel／Generic 作第一層分組。它們適合回答「誰擁有資料、CI 要守哪條依賴」，卻不直接回答個人創作者「我在這個產品能完成什麼」。這使目錄與文件即使有正確邊界，仍難呈現 MVP 所承諾的「探索、試跑、評估、下載」價值閉環。

ADR-037 已將 architecture identity 與 package mapping 集中於 ADR-032 §1；若把人讀的產品語言直接改成 package slug，又會讓產品命名被 Go 路徑綁死，並破壞既有 parser 與 CI 的穩定事實來源。

## 決策

### 1. 採用雙軌 Context Map

Platform 應同時維護兩種不能互相取代的表示：

| 表示 | 回答的問題 | 穩定識別 | 主要讀者 |
| --- | --- | --- | --- |
| 產品領域地圖 | 創作者能完成什麼成果？各能力如何組成旅程？ | 產品領域名稱、價值流、受控用語 | 產品、設計、領域協作者、新加入的工程師與 Agent |
| 架構治理地圖 | 哪個 Bounded Context 擁有事實？程式如何受 CI 約束？ | Boundary ID、package slug、architecture identity | 程式、depguard、query ownership、composition root |

`Core`、`Supporting`、`Shared Kernel`、`Generic` 是架構治理 metadata，不是產品導覽的第一層。產品文件先以價值流與創作者成果敘述；需要精確實作位置時才連到 stable Boundary ID／package slug。

Boundary ID 與 package slug 在本提案中保持既有值，例如 `run`、`registry`、`identity`。它們是治理與遷移期間的穩定鍵，不因產品用語評審而自動改名。

### 2. 正式產品領域名稱與價值流

下表是 Platform 的受控產品語言。它以 MVP 旅程定義人讀的產品領域；stable Boundary ID 與現行 Go path 的機械對照由 ADR-032 §1 擁有。

| 價值流 | 產品領域名稱 | 現有 Boundary ID／package slug | 創作者成果 |
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

`skillpkg` 是 Shared Kernel；`audit`、`outbox`、`objreconcile`、`llmclient`、`queue`、`objstore`、`metrics`、`partition`、`pgconv`、`envx`、`httpx`、`platform`、`apiserver` 與 `api` 是機制、ACL、技術基座或 composition root（上列皆為 Boundary ID；對應的 `internal/` 路徑由 [ADR-040](./ADR-040-platform-foundation-shared-kernel-and-entrypoint-topology.md) 定義，機械對照見 [ADR-032](./ADR-032-ddd-bounded-context-governance-for-platform.md) §1）。它們不是創作者可直接選擇的產品領域，文件應誠實以「共同語言與技術機制」導覽，不硬湊成價值流。

### 3. 受控用語規則

產品領域地圖、各 Context `doc.go`、`internal/README.md` 與後續資料夾導覽採以下規則：

1. 第一個句子以創作者成果或產品行為說明 Context，不以 HTTP、SQL、worker、queue 或 package 名稱開頭。
2. 「Skill 版本」是不可變內容快照；「Skill」是可持續演進、可 Fork 的資產身分；「試跑」是一次 Run；「執行證據」是 Trace；「成果判定」是 Evaluation。這些定義須與 MVP 規格 §2 相容。
3. 不將「接納與信任」等產品語言誤解為單一資料表或單一步驟；它描述對創作者的成果，底層可包含匯入、驗證與 provenance。
4. Boundary ID／package slug 以 code font 顯示，僅在治理、實作位置或遷移對照需要時使用；產品文字不以 slug 作主語。
5. 新產品用語必須先能回對 [MVP 目標與計畫](../plans/01-goals-and-plan.md) 的使用者成果或核心旅程，再納入受控用語表。

### 4. 文件與遷移邊界

本 ADR 授權文件語言與導覽調整；**不因此自動改動** package 路徑、import path、Go package clause、資料表、API 名稱或 Context 資料所有權。實體資料夾遷移仍必須先通過獨立計畫的治理工具前置與每批驗收 gate。

ADR-032 §1 仍是 architecture identity 的唯一機器可讀取來源。其 `### 1. Context 對照表` 標題與固定五欄（產品／Bounded Context、類型、Boundary ID、現行 internal path、需求 ID 前綴）由 `automation-check` 解析；產品名稱與 stable Boundary ID 分欄，避免以 package slug 取代產品語言。

2026-08-22（Phase 3、Batch 1）：創作者帳戶與工作區已由 `internal/identity` 遷移至 `internal/creator/workspace`。這是首個實體遷移；`identity` 仍是 stable Boundary ID 與 Go package clause，並未變更其資料或 API 所有權。

2026-08-22（Phase 3、Batch 2）：Skill 資產與版本歷史已由 `internal/registry` 遷移至 `internal/skill/library`。`registry` 仍是 stable Boundary ID 與 Go package clause，並未變更其資料或 API 所有權。

2026-08-22（Phase 3、Batch 3）：Skill 接納與信任已由 `internal/ingest` 遷移至 `internal/skill/admission`。`ingest` 仍是 stable Boundary ID 與 Go package clause，並未變更其資料或 API 所有權。

2026-08-22（Phase 3、Batch 4）：創作者旅程學習已由 `internal/analytics` 遷移至 `internal/product/learning`。`analytics` 仍是 stable Boundary ID 與 Go package clause，並未變更其資料或 API 所有權。

2026-08-22（Phase 3、Batch 5）：Skill 探索已由 `internal/catalog` 遷移至 `internal/skill/discovery`。`catalog` 仍是 stable Boundary ID 與 Go package clause，並未變更其資料或 API 所有權。

2026-08-22（Phase 3、Batch 6）：試跑情境設計已由 `internal/testlab` 遷移至 `internal/trial/design`。`testlab` 仍是 stable Boundary ID 與 Go package clause，並未變更其資料或 API 所有權。

2026-08-22（Phase 3、Batch 7）：創作者使用權益與資料生命週期已由 `internal/policy` 遷移至 `internal/product/entitlements`。`policy` 仍是 stable Boundary ID 與 Go package clause，並未變更其資料或 API 所有權。

2026-08-22（Phase 3、Batch 8）：Skill 交付與安裝已由 `internal/packaging` 遷移至 `internal/skill/delivery`。`packaging` 仍是 stable Boundary ID 與 Go package clause，並未變更其資料或 API 所有權。

2026-08-22（Phase 3、Batch 9）：執行證據已由 `internal/trace` 遷移至 `internal/trial/evidence`。`trace` 仍是 stable Boundary ID 與 Go package clause，並未變更其資料或 API 所有權。

2026-08-22（Phase 3、Batch 10）：Skill 試跑執行已由 `internal/run` 遷移至 `internal/trial/execution`（包含 `providertest`）。`run` 仍是 stable Boundary ID 與 Go package clause，並未變更其資料或 API 所有權。

2026-08-22（Phase 3、Batch 11）：成果判定與改善已由 `internal/eval` 遷移至 `internal/trial/improvement`。`eval` 仍是 stable Boundary ID 與 Go package clause，並未變更其資料或 API 所有權。

## 考慮過的替代方案

- **只重寫 `internal/README.md`**：拒絕。導覽會改善，但 ADR 與計畫仍以技術能力詞為主，下一位作者仍無法知道哪套產品語言是正式受控用語。
- **把 Core／Supporting 改名成產品價值流**：拒絕。兩者回答的問題不同；混合後會讓 CI metadata 被產品組織方式牽動。
- **立即把 package 搬進 `skill/*`、`trial/*` 等資料夾**：延後。先搬路徑會把已定案的產品語言、治理改造與 import 大搬家混成一次高風險修改；須先通過資料夾遷移計畫的工具與遷移 gate。
- **採每個 Context 的 domain/application/infrastructure 三層**：拒絕。ADR-032 已限定 tactical DDD；這不能改善產品語言，卻會增加 Go export surface、import cycle 與儀式成本。

## 影響

- 正面：文件、目錄導覽與未來資料夾地址能先呈現創作者旅程；架構治理仍有不受語彙調整影響的穩定鍵。
- 成本：後續文件要同時維護產品名稱與 Boundary ID 對照；新增或改名都要回對 MVP 使用者成果與核心旅程。
- 風險控制：受控產品名稱不改變 ADR-032 的 Boundary ID、depguard、query ownership 或資料 owner。

## 2026-08-22 授權與範圍釐清

產品負責人授權自動完成本計畫與後續工作項目，因此上表產品領域名稱、以「創作者」作 MVP 主要使用者稱呼，以及 `creator/`、`skill/`、`trial/`、`product/`、`shared/` 作為遷移規劃地址均在此日定案。這項定案當時**不是**「程式目錄已移動」的宣告：當時的 Go package 與 internal path 仍以 ADR-032 §1 的現況為準；實際遷移依當時的 Phase gate 分批執行。

> 2026-08-22 實作註記：上述規劃地址、`shared/skillpkg`、Foundation 與 Entrypoint 的實體遷移已完成；現行拓撲與驗收邊界見 [M4 DDD 邊界收斂報告](../plans/mvp/m4/report-platform-ddd-boundary-convergence-2026-08-19.md) 及 ADR-032／040。此註記不改寫本 ADR 的原始決策時點。
