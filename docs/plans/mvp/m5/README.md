# M5：從任務描述生成 Skill

- 狀態：**未開工**（2026-08-23 建立目錄）
- 決策：[ADR-046](../../../adr/ADR-046-generating-a-skill-from-a-task-description.md)
- 規格：[`02` §4.9](../../02-specifications-and-acceptance-criteria.md)（`GEN-001`～`004`）
- 工作項：[`03` §19](../../03-work-items.md)（`GEN-001`～`010`，全部未勾）

## 為什麼一個未開工的里程碑已經有目錄

因為它已經有一份量測。[ADR-046](../../../adr/ADR-046-generating-a-skill-from-a-task-description.md) 的決策 3 與決策 6 各壓了一個經驗假設，而那兩個假設在寫 ADR 的當下**只有推理沒有數字**。`report-generate-spike.md` 是那批數字，它是 `GEN-009` 的前三分之一，在 M5 開工前就跑掉，因為它會決定 ADR-046 那兩個決策的措辭撐不撐得住。

**這不表示 M5 開工了。** `01` §10 的啟動條件三項一項都還沒成立：MVP 封測尚未結束、漏斗第一段沒有讀數、`GEN-009` 的另外三分之二（試跑後的評估判定分布、人的接受率）沒有跑。

## 啟動條件

| # | 條件 | 現況 |
| --- | --- | --- |
| 1 | MVP 封測結束 | 未開始（`03` §18 `RELEASE-009` 未勾） |
| 2 | `01` §11.2 漏斗第一段有讀數 | 未量。**閘門的 D 日先於封測**，而 [gate-test 的 D 段](../gate-test/moderator-guide.md) 已加掛一句追問（ADR-046 前期驗證 2），那是這個訊號的前半 |
| 3 | `GEN-009` 生成品質基線 | **前三分之一已完成** → `report-generate-spike.md` |

## 檔案地圖

| 檔案 | 內容 |
| --- | --- |
| `report-generate-spike.md` | `GEN-009` 前置量測：一次呼叫產出的套件過不過 `skillpkg.Validate`、是不是空殼、實付成本 |

量測 harness 在 [`apps/platform/internal/shared/skillpkg/generate_spike_test.go`](../../../../apps/platform/internal/shared/skillpkg/generate_spike_test.go)（env-gated，形狀照 `spec_census_test.go`）。生成端的腳本是一次性的，不進 repo——它呼叫的端點還不存在（`GEN-001`／`GEN-002` 未做），腳本直接打閘道，逐字內容記在報告的附錄。
