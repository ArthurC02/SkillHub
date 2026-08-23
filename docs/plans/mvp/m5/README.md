# M5：從任務描述生成 Skill

- 狀態：**未開工**（2026-08-23 建立目錄）
- 決策：[ADR-046](../../../adr/ADR-046-generating-a-skill-from-a-task-description.md)
- 規格：[`02` §4.9](../../02-specifications-and-acceptance-criteria.md)（`GEN-001`～`004`）
- 工作項：[`03` §19](../../03-work-items.md)（`GEN-001`～`011`，全部未勾）

## 為什麼一個未開工的里程碑已經有目錄

因為它已經有一份量測。[ADR-046](../../../adr/ADR-046-generating-a-skill-from-a-task-description.md) 的決策 3 與決策 6 各壓了一個經驗假設，而那兩個假設在寫 ADR 的當下**只有推理沒有數字**。`report-generate-spike.md` 是那批數字，它是 `GEN-009` 的前三分之一，在 M5 開工前就跑掉，因為它會決定 ADR-046 那兩個決策的措辭撐不撐得住。

**這不表示 M5 開工了。** `01` §10 的啟動條件三項一項都還沒成立：MVP 封測尚未結束、漏斗第一段沒有讀數、`GEN-009` 的另外三分之二（試跑後的評估判定分布、人的接受率）沒有跑。

## 啟動條件

| # | 條件 | 現況 |
| --- | --- | --- |
| 1 | MVP 封測結束 | 未開始（`03` §18 `RELEASE-009` 未勾） |
| 2 | `01` §11.2 漏斗第一段有讀數 | **⏳ 暫時放行（2026-08-23，PDM），只放行這一列**——見 [`04` 乙-10](../../04-backlog-and-handoffs.md) 的完整範圍與終止條件。**放行不等於閘門通過**：D 日未宣告、9 場一場都沒跑、G1～G4 沒有任何一個數，前期驗證 2 也一次都沒問過。效力隨 D 日宣告而終止，屆時本列回到原本的形式。**本表第 1、3 列不在放行範圍內，所以 M5 仍未解除封鎖。** |
| 3 | `GEN-009` 生成品質基線 | **①②與 (甲)(乙) 已完成** → `report-generate-spike.md`（A 輪）、`report-generate-baseline.md`（B 輪＋mini）。**剩 ③④**——要 Sandbox、評估管線與人 |

## 前期驗證

ADR-046 的決策 3 與決策 6 各壓了一個經驗假設，而 `01` §7.3 把這條路排在封測之後——兩件事合起來的風險是：**規格已經寫死，而支撐它的兩個假設要等一年後才被檢驗**。所以在 M5 開工前先跑掉兩件，各自的編號在本節定義（**ADR-046 本身沒有這個章節**，2026-08-23 稍早幾份文件誤引為「ADR-046 前期驗證 N」，已一併改指這裡）：

| # | 驗證什麼 | 成本 | 結果 |
| --- | --- | --- | --- |
| **前期驗證 1** | 模型單次呼叫產得出通過 `skillpkg.Validate` 的套件嗎？通過的是實質內容還是空殼？ | 閘道實付 **$2.3684** | 已完成 → `report-generate-spike.md`。**兩個假設都沒有完全成立**，詳見該報告 §4 的回寫表 |
| **前期驗證 2** | 使用者搜不到之後，**自發**說出的下一步是什麼？ | **零**——掛在本來就要跑的 M1 閘門上 | **未執行。** 材料已就位（[gate-test/moderator-guide.md §3.D](../gate-test/moderator-guide.md)），**待 D 日**。<br>**2026-08-23 的暫時放行沒有放行這一件**——它放行的是啟動條件第 2 列，不是這個量測。這一題一次都沒問過，所以 §0 那個「使用者搜不到會自發想生成」的假設**至今零證據**（見 [`04` 乙-10](../../04-backlog-and-handoffs.md)） |

**兩件都不是 `GEN-009` 的替代品**：前期驗證 1 只涵蓋 `GEN-009` 的①②兩個數，而且每段描述只跑一次。

## 檔案地圖

| 檔案 | 內容 |
| --- | --- |
| `report-generate-spike.md` | A 輪：一次呼叫產出的套件過不過 `skillpkg.Validate`、是不是空殼、實付成本 |
| `report-generate-baseline.md` | B 輪＋mini 對照：**失敗是不是隨機的**（不完全是）、**mini 夠不夠用**（更好且便宜 21 倍 → [ADR-051](../../../adr/ADR-051-the-cheaper-model-generated-better-packages.md)）、`possible-secret` 命中率（0／59） |

量測 harness 在 [`apps/platform/internal/shared/skillpkg/generate_spike_test.go`](../../../../apps/platform/internal/shared/skillpkg/generate_spike_test.go)（env-gated，形狀照 `spec_census_test.go`）。生成端的腳本是一次性的，不進 repo——它呼叫的端點還不存在（`GEN-001`／`GEN-002` 未做），腳本直接打閘道，逐字內容記在報告的附錄。
