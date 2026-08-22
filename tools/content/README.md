# tools/content

## 凍結審校證據

`review-results.json` 是當時內容審校的凍結證據。`review_summaries.py` 仍具寫入能力，因此重新審校
必須先取得明確批准，並另存具日期或版本識別的結果；不得把新結果覆寫成舊審校的歷史結論。

**權威資料在 DB／物件儲存，此處為種子、管線與證據快照。** 這裡的 JSON 不是目錄的事實來源——線上目錄才是；快照留著是為了讓報告裡的每個數字可以被重查，而不是為了給任何程式讀。

以下逐檔歸類。同一列是一條管線。

| 輸入 | 腳本 | 產出快照 |
| --- | --- | --- |
| [`seed-skills.json`](seed-skills.json)（45 筆精選的來源 repo、pin commit、路徑、授權與依賴欄位；正式清單見 [`docs/plans/mvp/content/curated-skill-list.md`](../../docs/plans/mvp/content/curated-skill-list.md)） | [`import_seed.py`](import_seed.py)（走公開 HTTP API：dev login ＋ 逐筆套件上傳，不碰 DB） | **無檔案**——產出直接進 dev DB 與物件儲存；實作與逐筆結果見 [`m1/import-report.md`](../../docs/plans/mvp/m1/import-report.md)、[`m1/catalog-rebuild-report.md`](../../docs/plans/mvp/m1/catalog-rebuild-report.md) |
| `seed-skills.json` ＋ 平台自己的 `POST /v1/enrich-skill` 回傳 | [`generate_summaries.py`](generate_summaries.py)（不重寫增強邏輯，只呼叫production endpoint 並收錄結果） | [`summaries.json`](summaries.json)——CONTENT-005 的增強產出快照；審核紀錄本體是 [`content/content-summaries.md`](../../docs/plans/mvp/content/content-summaries.md) |
| **線上目錄**（刻意不是 `summaries.json`：要審的是實際入庫的文字） | [`review_summaries.py`](review_summaries.py)（Script ＋ 獨立 Judge 模型 ＋ 機械檢查，逐 KPI 判定） | [`review-results.json`](review-results.json)——CONTENT-005 審校的逐筆原始判定；結論與輪次見 [`m1/content-review-report.md`](../../docs/plans/mvp/m1/content-review-report.md) |
| [`m2/content-baseline-report.md`](../../docs/plans/mvp/m2/content-baseline-report.md) §4／§8／§12 的 45 筆基準試跑量測值 | [`backfill-agent-compatibility.sql`](backfill-agent-compatibility.sql)（可重跑；`capability` 由 trace 現查、不寫死） | **無檔案**——寫入 migration `0022` 的相容軸表，一次一組 (runtime image, run window) |
| [`governance/anthropic-sa-license-memo.md`](../../docs/plans/mvp/governance/anthropic-sa-license-memo.md) §3 方案 C 的負責人裁定 | [`restrict-anthropic-sa-display.sql`](restrict-anthropic-sa-display.sql)（保全動作，非終判） | **無檔案**——寫入 migration `0023` 的 `skills.access_restriction`；**目前只能直接跑 SQL，沒有端點也沒有 audit event**，見 [`04` 殘項乙-10](../../docs/plans/04-backlog-and-handoffs.md) |
| [`m2/content-baseline-report.md` §3](../../docs/plans/mvp/m2/content-baseline-report.md) 的 Prompt 模板、`summaries.json` 的第一句 `task_examples`、[`content/writing-rubrics.md`](../../docs/plans/mvp/content/writing-rubrics.md) §3／§4（rubric 全文讀 [`eval-regression/rubric-content-007-writing-v1.json`](../eval-regression/rubric-content-007-writing-v1.json)）、[`seed-testcases/`](seed-testcases/) 的兩個範例 Dataset | [`seed_testcases.py`](seed_testcases.py)（走公開 HTTP API；**必須以目錄策展帳號登入**，種進非目錄 Workspace 的 Test Case 會被 `PACK-005` 以 `not_curated` 排除；冪等取「跳過」，`--replace` 走刪除再建立） | **無檔案**——15 筆策展 Test Case ＋ 67 條驗收條件 ＋ 5 份 rubric 直接進部署。丙-12 的種入路徑；驗證紀錄與 live 套用待辦見 [`m4/README` §14.2](../../docs/plans/mvp/m4/README.md) |
| **DB 自己的** `skill_versions.license_expression`（判準見 [ADR-027](../../docs/adr/ADR-027-download-artifact-shape-reproducibility-and-integrity.md) 決策 4；`seed-skills.json` 的策展欄位只當人工對照，不進腳本） | [`backfill-redistribution.sql`](backfill-redistribution.sql)（可重跑；三態由授權運算式推導，Fork 遞迴繼承，末尾兩個 SELECT 印出分佈與剩餘 `unknown`） | **無檔案**——寫入 migration `0027` 的 `skills.redistribution`；**需先套用 `0027`**。dry-run 結果與待套用狀態見 [`m4/README` §13](../../docs/plans/mvp/m4/README.md) |

四支 Python 都是**驗證工具，不是產品程式碼**：不進 CI、不被服務引用。前三支重跑的代價是真實的模型費用；`seed_testcases.py` 不呼叫模型，代價只有寫入。`__pycache__/` 是本機執行的副產物。

`seed-testcases/` 的兩個檔案是**本批新做的合成資料**：[`m2/content-baseline-report.md` §3](../../docs/plans/mvp/m2/content-baseline-report.md) 只留下對它們的描述，位元組從未留存，所以這裡重建的是符合該描述的新資料，不是 M2 那兩份。無 Secrets、憑證與個資（`--selftest` 有斷言）。
