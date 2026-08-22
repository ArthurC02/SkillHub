# ADR-037：Product Analytics 與 package architecture identity

- 狀態：Accepted
- 日期：2026-08-22
- 修訂：[ADR-002](./ADR-002-domain-boundaries-and-ownership.md)、[ADR-032](./ADR-032-ddd-bounded-context-governance-for-platform.md)、[ADR-035](./ADR-035-read-ownership-enforcement-and-context-map-completeness.md)

## 背景

ADR-032 的文字已說明 `analytics` 不併入 `policy`，但 §1 仍把兩個 package 放在同一個 Policy & Usage 列；同一張表又把 `skillpkg` 放進 Skill Registry，§2 卻稱它為 Shared Kernel。舊版 `devctl` 以 package slug 的手寫集合判斷 query ownership，並用布林值把所有非 Generic package 視為 Bounded Context。這三種表示法無法回答「兩個 package 是否屬於同一個 context」，也會接受同一 package 的重複身分。

## 決策

### 1. 每個 package 只有一個 architecture identity

ADR-032 §1 是唯一可機器讀取的 package 對照表，`類型` 採封閉集合：`Core`、`Supporting`、`Shared Kernel`、`Generic`。

- `Core`、`Supporting` 必須有 Bounded Context 名稱；同名表示同一 context。
- `Shared Kernel`、`Generic` 的 Context 欄必須是破折號，機器內正規化為空字串。
- 同一 package 重複出現、未知類型、未登記 package 或缺少必要 depguard coverage 一律使 CI 失敗。
- `entrypoint/api/apiserver` 是 composition root、`entrypoint/api/gen` 是 generated transport，兩者是 Generic coverage 的唯二例外。

本次身分修訂如下：

| package | Context | 類型 |
| --- | --- | --- |
| `registry` | Skill Registry & Versioning | Core |
| `policy` | Policy & Usage | Supporting |
| `analytics` | Product Analytics | Supporting |
| `skillpkg` | — | Shared Kernel |

`analytics` 擁有產品漏斗事件、回饋與分析保存政策；`policy` 擁有 quota、retention 規則與未來計費政策。兩者 vocabulary、資料用途與變更節奏不同，因此是兩個 Supporting Bounded Context。`skillpkg` 僅提供套件格式與驗證純函式，不擁有 Skill Registry 的領域狀態。

### 2. Query ownership 比較 architecture boundary

`db/query-owners.yaml` 的既有 slug 不改名；合法 slug 直接由 ADR-032 §1 的 package mapping 取得，不再維護第二份手寫集合。比較邊界時：

- Core／Supporting 使用 `context:<Context>`。
- Shared Kernel／Generic 使用 `package:<slug>`。

因此同一 Bounded Context 未來可由多個 Go package 組成而不被誤判為跨 context；Shared Kernel 與 Generic 仍逐 package 隔離。呼叫 sqlc 的未登記 package 會 fail closed。

### 3. `skillpkg` 有獨立 depguard 規則

各 Bounded Context 可以 import `skillpkg`；`skillpkg` 不得反向 import 任何 Bounded Context。它不再借用 `generic` 規則，避免 Shared Kernel 與 Generic 在機器治理上再次合併。

## 2026-08-22 產品領域名稱釐清（ADR-038 Accepted）

本 ADR 的 `Context` 欄與 `Core`／`Supporting`／`Shared Kernel`／`Generic` 是 architecture identity，不是產品領域名稱。它們繼續作為 ADR-032 §1、depguard 與 query ownership 的穩定事實來源。 [ADR-038](./ADR-038-platform-product-domain-language-and-value-stream-navigation.md) 已接受產品價值流與產品領域名稱：例如 `analytics` 的產品導覽為「創作者旅程學習」，`policy` 為「創作者使用權益與資料生命週期」。這項釐清不改 stable Boundary ID、identity 類型或 coverage 判定；實體路徑由 ADR-032 §1 的現行 path 欄位決定。

## 考慮過的替代方案

- **保留 package slug 等同 context**：無法表達同一 Bounded Context 的多 package，也延續文件與 checker 的雙重事實來源。
- **把 `analytics` 留在 Policy & Usage**：與 ADR-029 已定義的產品分析用途、揭露與 audit 邊界不符。
- **把 `skillpkg` 算 Registry 或 Generic**：前者讓純函式庫假裝擁有領域狀態，後者失去 Shared Kernel 的雙向治理語意。

## 影響

- Context Map、depguard 與 query ownership 使用同一份 package identity。
- 新增或移動 package 必須先更新 ADR-032 §1；錯字、重複與未覆蓋不再靜默通過。
- 本 ADR 只修正 architecture identity，不拆 package、不改 query slug，也不引入 Repository 或 framework。
