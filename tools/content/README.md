# tools/content

**權威資料在 DB／物件儲存，此處為種子、管線與證據快照。** 這裡的 JSON 不是目錄的事實來源——線上目錄才是；快照留著是為了讓報告裡的每個數字可以被重查，而不是為了給任何程式讀。

以下逐檔歸類。同一列是一條管線。

| 輸入 | 腳本 | 產出快照 |
| --- | --- | --- |
| [`seed-skills.json`](seed-skills.json)（45 筆精選的來源 repo、pin commit、路徑、授權與依賴欄位；正式清單見 [`plans/mvp/m1/curated-skill-list.md`](../../plans/mvp/m1/curated-skill-list.md)） | [`import_seed.py`](import_seed.py)（走公開 HTTP API：dev login ＋ 逐筆套件上傳，不碰 DB） | **無檔案**——產出直接進 dev DB 與物件儲存；實作與逐筆結果見 [`m1/import-report.md`](../../plans/mvp/m1/import-report.md)、[`m1/catalog-rebuild-report.md`](../../plans/mvp/m1/catalog-rebuild-report.md) |
| `seed-skills.json` ＋ 平台自己的 `POST /v1/enrich-skill` 回傳 | [`generate_summaries.py`](generate_summaries.py)（不重寫增強邏輯，只呼叫production endpoint 並收錄結果） | [`summaries.json`](summaries.json)——CONTENT-005 的增強產出快照；審核紀錄本體是 [`m1/content-summaries.md`](../../plans/mvp/m1/content-summaries.md) |
| **線上目錄**（刻意不是 `summaries.json`：要審的是實際入庫的文字） | [`review_summaries.py`](review_summaries.py)（Script ＋ 獨立 Judge 模型 ＋ 機械檢查，逐 KPI 判定） | [`review-results.json`](review-results.json)——CONTENT-005 審校的逐筆原始判定；結論與輪次見 [`m1/content-review-report.md`](../../plans/mvp/m1/content-review-report.md) |
| [`m2/content-baseline-report.md`](../../plans/mvp/m2/content-baseline-report.md) §4／§8／§12 的 45 筆基準試跑量測值 | [`backfill-agent-compatibility.sql`](backfill-agent-compatibility.sql)（可重跑；`capability` 由 trace 現查、不寫死） | **無檔案**——寫入 migration `0022` 的相容軸表，一次一組 (runtime image, run window) |
| [`m2/anthropic-sa-license-memo.md`](../../plans/mvp/m2/anthropic-sa-license-memo.md) §3 方案 C 的負責人裁定 | [`restrict-anthropic-sa-display.sql`](restrict-anthropic-sa-display.sql)（保全動作，非終判） | **無檔案**——寫入 migration `0023` 的 `skills.access_restriction`；**目前只能直接跑 SQL，沒有端點也沒有 audit event**，見 [`04` 殘項乙-10](../../plans/mvp/04-backlog-and-handoffs.md) |

三支 Python 都是**驗證工具，不是產品程式碼**：不進 CI、不被服務引用，重跑的代價是真實的模型費用。`__pycache__/` 是本機執行的副產物。
