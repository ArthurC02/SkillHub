# 已勾選工作項稽核（2026-08-19）

## 結論

本輪逐項覆核 `03-work-items.md` 稽核開始時的 **123 個 `[x]`**。判定採「允收準則 → 實作 → 反例測試」三段追查，不以綠燈測試本身代替完成證據。

| 範圍 | 稽核數 | 本輪結束判定 |
| --- | ---: | --- |
| §0～§8（基線、PDM、設計、內容、核心、匯入、探索、Workspace） | 56 | 55 完成；`CONTENT-010` 部分完成並退勾 |
| §9～§13（Test Lab、Run、Sandbox、Trace、O11Y） | 40 | 40 完成（8 項在修正前為部分完成） |
| §14～§18（Evaluation、Packaging、QA、Release） | 27 | 27 完成（本輪修正後） |
| **合計** | **123** | **122 完成、1 部分完成** |

因此，現在仍為 `[x]` 的 122 項均已逐項核對；唯一不符合標題允收的 `CONTENT-010` 已改回 `[ ]`。§16 與 §18 在稽核開始時沒有 `[x]`，不是漏查。

## 退勾項目

### CONTENT-010 — 部分完成（41／45）

工作項要求以 `2026.08-3` 重跑 45 筆；報告實際只有 41 筆，另 4 筆因 License hold 沿用 `2026.08-2`。不能用不同 image 的「最新量測合併」宣稱同一軸完整，也不能為補數繞過 hold。本輪已退勾並明載 41／45；待限制解除後補跑，或由負責人正式修改允收準則。

## 發現與修正

| 類別／受影響工作項 | 問題 | 不改功能邊界的修正 |
| --- | --- | --- |
| `DISC-*` 搜尋降級 | FTS fallback 與 probe 吞掉 DB error，把搜尋故障回成正常無結果 | error 一律 500；保留只有向量腿不可用時的既定降級，加入取消 context 反例 |
| `RUN-005`、`RUN-009`、`SBX-006` | Platform 與 Sandbox 各自手寫且漏比 ResourceLimits；Provider 的 0 被當無限 | 兩端 fail-closed，比對全部欄位；OpenAPI required 與逐欄 table tests 同步 |
| `RUN-007`、`SBX-009`、`SBX-012`、`O11Y-001/003` | destroy 失敗前先刪 bookkeeping／釋放 slot，使仍存在的 sandbox 從 active/orphan 視野消失 | Driver Remove 成功後才移除；測試鎖住 GET、active list、capacity 與 retry 兩側狀態 |
| `TRACE-001`、`TRACE-008` | `(event_id, occurred_at)` 只能防完全相同重送，同 ID 換 timestamp 可重複；rolling migration 亦可能漏 writer | migration 鎖表、歷史重複 fail-fast、parent trigger＋domain-separated advisory lock；兩交易測試確認第二筆實際等鎖且只留一列 |
| `O11Y-004`、`NFR-002` | 回訪 SQL 結構上量不到 Workspace；session 跨帳號污染；日界線依 DB timezone；Retention 沒有執行清除 | 每 UTC visit-day 映射匿名 session 到同日 Workspace；明訂 best-effort marker；maintenance purge fail-closed 並測 cutoff |
| `EVAL-002` | `char_range` 使用 byte offset，非 ASCII 引文位置錯誤 | 改以 Unicode rune 計 character offset，CJK＋emoji regression |
| `EVAL-007`～`EVAL-010` | suggestion 可借用不相關 evidence；一字元引文可繞過；套用結果受 request order 影響；同 target 舊分支成 dead code | 引文至少 12 rune 且須回驗，否則不存 proposal；canonical sort、ID 去重、同 target 全拒絕；移除不可達 branch並同步契約 |
| `PACK-001`～`PACK-005` | `.ENV`／巢狀 credential 可漏入套件，`.env.example` 又被誤刪 | path case-fold、segment/suffix 精確規則，保留安全模板；加入大小寫與巢狀 fixture |
| `PACK-001/002` | manifest 使用 wall clock、ZIP 寫 extended timestamp，無法重現 | `packaged_at` 固定為來源 Version 建立時間；ZIP 使用固定 DOS date 且 `Extra` 必須空 |
| `PACK-001/002/008` | dedupe identity 漏掉會變動的 compatibility／portable Test Case；查詢與寫入分離有 race；DB 後段失敗可留 object orphan | 完整 zip `content_hash` 納入 lookup 與 advisory identity；先 Plan；Workspace/content object key、Exists fail-closed與有 deadline的補償刪除；race／mutation／fault injection tests |
| 文件與 dead code | `03` 保留已失效的「沒有 Prompt UI／只有上傳／契約未含 estimated_cost」長篇歷史；Eval 留未使用 source constants；TestLab 留過時 TODO | `03` 改為目前證據；刪除未使用常數；TODO 改為現況邊界；同步 packaging design/schema |

## 阻抗迴路

每批修正都交回不同 SubAgent 唯讀 Review；有 finding 即修正再送下一輪。最後閉合狀態：

- M0／M1：Catalog Review 第 3 輪 zero findings；Analytics Review 第 3 輪 zero findings。
- M2：Sandbox destroy 第 2 輪、ResourceLimits 第 4 輪、Trace 第 7 輪 zero findings。
- M3／M4：Evaluation／credential filtering 第 4 輪 zero findings；Packaging 第 8 輪 zero findings（含 race、mutable identity、ambiguous Put 與補償 timeout）。

## 驗證與限制

- Platform：`go test ./...`，277 passed；`go vet ./...` passed。
- Sandbox：`go test ./...`，55 passed；`go vet ./...` passed。
- Web：13 files／117 tests passed；production build passed。Lint 只有既有 Fast Refresh 檔案拆分警告，未為消除警告增加無功能價值的檔案抽象。
- LLM：62 passed；Ruff lint／format passed（Starlette/httpx 相依套件 deprecation warning 仍存在）。
- devctl 與 generated drift：passed。

本機未設定 `SKILLHUB_TEST_DATABASE_URL`，所以 DB-backed integration tests 只完成編譯並按設計 skip；沒有把 skip 宣稱為動態通過。Doctor 亦誠實回報本機 Go／Node 版本高於 repo pin，且 Task／golangci-lint 未安裝；因此 canonical container/toolchain 驗證仍應由 CI 或 Dev Container 補跑。歷史付費基準（`CONTENT-008/010`、`EVAL-013`）只核對既有報告、Run ID、資料形狀與可重跑 harness，本輪沒有自行啟動付費模型。
