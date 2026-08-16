# M1：Skill Explorer — 執行計畫

- 日期：2026-08-15（起）／**2026-08-16 更新**
- 狀態：**程式碼面已收斂，驗證閘門尚未正式通過。** 工作項對帳見 [m1-work-items-audit.md](m1-work-items-audit.md)；閘門的使用者測試材料已備妥（[gate-test/](gate-test/)），**D 日待負責人宣告**。閘門與 M2 開發並行，正式通過與否以測試結果為準。
- 對應里程碑：M1（見 [../01-goals-and-plan.md](../01-goals-and-plan.md) §10）
- 殘項與待決：見 [../04-backlog-and-handoffs.md](../04-backlog-and-handoffs.md)（M1 遺留的法務判定與 D 日宣告記於「乙-10」）

## 範圍

M1 交付兩件互相依賴的東西：

- **內容**——依 PDM-001／002 精選 45 個 Skill，走完候選盤點 → 匯入 → 索引時 LLM 增強 → 摘要審校 → 可凍結的線上目錄基線（CONTENT-001～006、INGEST-001～009）。
- **探索**——DISC-001～005 的混合檢索、篩選維度、符合原因與排序理由。

里程碑結尾有一道**驗證閘門**（`01` §10）：測試使用者以自然語言能找到相關 Skill 並理解符合原因，否則先修正搜尋與內容，不進 M2。閘門的判準推導、招募、主持、記錄與分析材料整組放在 [gate-test/](gate-test/)。

## 檔案地圖

| 檔案 | 類型 | 一句話用途 | 狀態 |
| --- | --- | --- | --- |
| [`README.md`](README.md)（本檔） | 計畫 | M1 的計畫、狀態與本目錄導覽 | 活文件 |
| [`m1-work-items-audit.md`](m1-work-items-audit.md) | 審計 | M1 工作項逐項對帳，是某一時點的帳 | 凍結 |
| [`content-candidates.md`](content-candidates.md) | 策展資料 | CONTENT-003 的候選盤點與類別邊界原始論證；已由 `curated-skill-list.md` 升級為正式清單，保留為推導過程 | 凍結 |
| [`curated-skill-list.md`](curated-skill-list.md) | 策展資料 | CONTENT-003 的 45 筆正式精選清單（來源、pin commit、授權、依賴欄位） | **活文件**——M2 期間仍在更新（相容欄位、依賴校正）；位置待歸位，見 [`04` 檔-1](../04-backlog-and-handoffs.md) |
| [`content-summaries.md`](content-summaries.md) | 策展資料 | CONTENT-005 白話摘要的審核紀錄本體（逐筆增強產出與審核判定） | **活文件**——同上 |
| [`import-report.md`](import-report.md) | 報告 | 種子清單首次本機端到端匯入的實作紀錄（INGEST-001～009）；線上基線已由 `catalog-rebuild-report.md` 取代 | 凍結 |
| [`catalog-rebuild-report.md`](catalog-rebuild-report.md) | 報告 | 為閘門重建**可凍結的線上目錄基線**（實際 45 筆，非本檔前身所寫的 44 筆）的實作與實查紀錄 | 凍結 |
| [`content-review-report.md`](content-review-report.md) | 報告 | CONTENT-005 自動化審校：KPI 判定、重跑輪次、`enrich-skill` v4～v6 升版與逐筆結果 | 凍結 |
| [`golden-query-set.md`](golden-query-set.md) | 測試材料 | DISC-001 的 60 題 golden query、語料與向量距離門檻建議；工具在 [`tools/goldenset/`](../../../tools/goldenset/) | 凍結 |
| [`gate-test/README.md`](gate-test/README.md) | 測試材料 | 閘門測試目標、判準推導、執行前置（§3.1）與環境凍結（§3.2） | **閘門凍結標的**，變更需依 [§3.1](gate-test/README.md) 紀錄 |
| [`gate-test/recruit.md`](gate-test/recruit.md) | 測試材料 | 招募文案、篩選問卷、時程與報酬 | **閘門凍結標的**，變更需依 [§3.1](gate-test/README.md) 紀錄 |
| [`gate-test/moderator-guide.md`](gate-test/moderator-guide.md) | 測試材料 | 主持腳本、時間表、不可提示清單 | **閘門凍結標的**，變更需依 [§3.1](gate-test/README.md) 紀錄 |
| [`gate-test/task-cards.md`](gate-test/task-cards.md) | 測試材料 | 14 張情境卡（受測者版／主持人版分離）與分派表 | **閘門凍結標的**，變更需依 [§3.1](gate-test/README.md) 紀錄 |
| [`gate-test/recording-sheet.md`](gate-test/recording-sheet.md) | 測試材料 | 逐場記錄表與匯總表，欄位與分析模板對應 | **閘門凍結標的**，變更需依 [§3.1](gate-test/README.md) 紀錄 |
| [`gate-test/analysis.md`](gate-test/analysis.md) | 測試材料 | 判準計算、質性主題歸類、不通過的決策樹 | **閘門凍結標的**，變更需依 [§3.1](gate-test/README.md) 紀錄 |

> **「閘門凍結標的」是什麼意思**：`gate-test/README.md` §3.2 規定測試期間不得變更目錄內容、增強產出、排序管線與判準文案——任何變更都會讓前後場次不可比。D 日宣告之前的變更必須記在 §3.1 的前置表，D 日之後不得已而改（例如線上事故）則變更前後的場次分開統計，並在分析報告中明列。
