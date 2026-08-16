# 內容策展來源

種子目錄的**活策展資料**：45 筆精選清單、白話摘要審核紀錄、`writing` 類的預設評估 rubric，以及它們的候選盤點來源。

| 檔案 | 內容 | 對應工作項 | 狀態 |
| --- | --- | --- | --- |
| [`curated-skill-list.md`](curated-skill-list.md) | 45 筆正式清單、九項精選檢查矩陣、License 合規總表 | CONTENT-003／004 | 活文件 |
| [`content-summaries.md`](content-summaries.md) | 白話摘要與任務範例句的審核紀錄與工序 | CONTENT-005 | 活文件 |
| [`writing-rubrics.md`](writing-rubrics.md) | `writing` 5 個精選的**預設 rubric**（`rubric_version = content-007/writing/v1`）與六項缺口 | CONTENT-007（**仍不勾**） | 活文件（2026-08-17 新增） |
| [`content-candidates.md`](content-candidates.md) | 已被 `curated-skill-list.md` 取代的候選盤點初稿 | CONTENT-003 前身 | 可視為凍結 |

- **出處**：M1 的 CONTENT-003／005（原 `m1/`），2026-08-16 依 [ADR-024](../../../adr/ADR-024-top-level-repository-layout.md) 歸位至此；`writing-rubrics.md` 為 2026-08-17 的 CONTENT-007 產出，依同一條規則落在本目錄而非 `m3/`。
- **為什麼不在 `m1/`**：這幾份在 M2 期間仍持續被改（相容欄位、增強重跑、審校追認），下一個里程碑也還會改——依 AGENTS.md「一份文件如果會被下一個里程碑繼續改，它就不屬於 `mX/`」。
- **相關**：產出這些內容的報告（匯入、重建、審校）留在 [`../m1/`](../m1/)，基準試跑的帳在 [`../m2/content-baseline-report.md`](../m2/content-baseline-report.md)，它們是某一時點的帳；rubric 的消費端設計見 [`../m3/evaluation-design.md`](../m3/evaluation-design.md) §6.4；閘門測試材料見 [`../gate-test/`](../gate-test/)。
