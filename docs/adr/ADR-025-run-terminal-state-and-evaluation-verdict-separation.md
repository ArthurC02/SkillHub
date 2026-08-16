# ADR-025：Run 終態與 Evaluation 判定的分離

- 狀態：Accepted
- 日期：2026-08-17
- 決策者：產品負責人、架構規劃
- 相關：[ADR-009](./ADR-009-observability-trace-and-evaluation-boundaries.md)（O11y／Trace／Evaluation 三分）、[ADR-004](./ADR-004-provider-neutral-run-orchestration.md)（Run 生命週期與狀態）、[ADR-008](./ADR-008-asynchronous-workflows-and-domain-events.md)（狀態機是事實來源）、[ADR-003](./ADR-003-data-ownership-and-storage.md)（不可變快照）
- 設計來源：[docs/plans/mvp/m3/evaluation-design.md §4](../plans/mvp/m3/evaluation-design.md)、[04-backlog-and-handoffs.md 丙-5](../plans/mvp/04-backlog-and-handoffs.md)

## 背景

M2 結束時已經量到一個具體實例：`date-wrangling` 的終態是 `succeeded`、Skill 有啟用、Trace 完整，而 `/out/artifacts/` 是空的——agent 的最終回覆是反問使用者。沙箱 harness 的 `finish("succeeded")` 只代表「agent 這一輪沒有拋錯」，與任務是否完成無關（丙-5）。

同時，控制平面的程式碼裡有一個相反方向的實作意圖。`services/platform/internal/run/job.go:419`：

```text
// TODO(EVAL-001): evaluation runs here and decides succeeded vs failed.
// Until it exists, `succeeded` means the workload ran to its own end and
// said so - not that anything checked the acceptance criteria.
return "workload reported success; no evaluator is configured yet (EVAL-001)"
```

M3 的第 2 批就要把評估接到這個位置上。這條 TODO 說評估決定 `succeeded` 還是 `failed`，而 ADR-009 的邊界劃分與 M2 的資料形態指向相反答案。兩者只能留一個，且**推翻一個既有的實作意圖必須有決策紀錄，不能靠一份設計文件帶過**。

## 決策

`runs.status` 回答「這次執行發生了什麼」；`evaluations.overall` 回答「任務達成了嗎」。**兩個問題、兩個欄位、兩個表：Evaluation 的判定不回寫 `runs.status`，也不回寫 `runs.failure_class`。**

ADR-004 的狀態機不變，`evaluating → succeeded` 的既有路徑照走；評估是這條路徑上的一個步驟，不是第二個狀態機。

### 理由 1：失敗分類會被汙染

`runs.failure_class` 的語意是 `provider_error`（我們的問題，可重試）對 `workload_error`（Skill 的問題，不重試），它是 `RUN-006` 重試決策的依據。讓「輸出不符驗收條件」也變成 `failed`，等於把一個**不該重試、也不是故障**的結果塞進重試分類器——重試只會用同樣的 Skill 版本跑出同樣不符合的輸出。

### 理由 2：資料庫已經替我們回答了

`0005` 的 `runs_terminal_immutable` trigger 本來就禁止改寫已終態的 Run。而 [ADR-026](./ADR-026-evaluation-reassessment-evidence-lifetime-and-judge-trust-boundary.md) 的重評是 append-only：同一個 Run 在 rubric 或 Judge prompt 升版後會產生新的判定。若終態由評估決定，那個 Run 的終態就得跟著變——直接撞上 trigger。

連帶的是歷史：M2 的 73 筆 Run 沒有評估，若終態由評估決定，它們的 `succeeded` 會失去意義。

### 理由 3：ADR-009 的三分本來就是這樣畫的

Run Trace 是**執行事實**，Evaluation 是**判斷**。把判斷寫回執行事實，等於把三分合回兩分，也讓「判斷來源」這個 ADR-009 明文要求的欄位失去落點。

## 本 ADR 推翻的既有實作意圖

`services/platform/internal/run/job.go` 的 `successReason()` 中，第 419 行的 `TODO(EVAL-001)` 及其回傳文字所描述的方向與本決策相反，**自本 ADR 起不再是待辦，而是已被否決的選項**。

**本 ADR 不改程式碼。** 改寫排在 M3 第 2 批（[m3/README.md §6](../plans/mvp/m3/README.md)）：該 TODO 移除，`successReason` 的文字改為執行語意（「執行完成，任務判定另見 evaluation」），不再暗示有一個尚未實作的評估會回來改終態。在那之前，現行文字仍然是誠實的——它說的是「沒有東西檢查過驗收條件」，那是事實。

## 落地要求

| 面向 | 要求 |
| --- | --- |
| 狀態機 | `evaluating → succeeded` 路徑不變；評估寫入 `evaluations`，不 UPDATE `runs` 的任何欄位 |
| 沒有評估的 Run | UI 顯示「**未評估**」，**不是**通過。M2 的 73 筆與所有未來的失敗 Run 都落在這裡 |
| 評估失敗的 Run | `evaluations.status = failed` → UI 顯示「**評估未完成**」，與「未評估」分開顯示 |
| UI 文案 | Run 終態改用執行語意（「執行完成」／「執行失敗」），任務判定**另起一列**顯示四態（符合／部分符合／未符合／無法判斷） |
| 一般模式 | 使用者看到的第一行是任務判定，不是 Run 終態（`01` §11.3：所有「通過」或「未通過」結論都能展開查看依據） |

判準是 NFR-001「UI 不得誤導」：把 `succeeded` 呈現為「可用」與乙-2 的 `TokenBudget`「顯示但不強制」是同一種錯誤的兩個面向。

## 影響

### 正面

- 重試分類器只處理它該處理的事，`failure_class` 的值域不必為了評估而擴張。
- 重評（ADR-026）與歷史 Run 的不可變性不衝突，兩者可以同時成立。
- 「執行成功但任務沒完成」變成一個**說得出口的狀態**，而不是一個要靠使用者自己讀 Trace 才發現的落差。

### 成本與限制

- UI 要同時顯示兩個狀態，一般模式的資訊密度上升；`DESIGN-010` 必須處理「兩列狀態不打架」的呈現問題。
- 對外部消費者（未來的 API 使用者）而言，「Run 成功」不再是一個可以單獨判斷結果的欄位，必須一併讀 evaluation。這是誠實的代價，不是缺陷。
- 既有的 73 筆 M2 Run 永久停在「未評估」，除非有人另行決定補評——那是產品決策，不在本 ADR。

## 待決策

- 一般模式與比較畫面上兩列狀態的實際文案與版面，由 `DESIGN-010`／`DESIGN-011` 承接。
- 歷史 Run 是否補做評估（M2 的 73 筆）：屬產品決策，成本可估（見 [evaluation-design.md §6.3](../plans/mvp/m3/evaluation-design.md)），本 ADR 不代為決定。
