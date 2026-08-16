# CONTENT-007／008 內容基準試跑報告

- 日期：**2026-08-16**（§12 為同日稍晚的補跑，條件不同，獨立成節）
- 範圍：目錄內全部 **45 個 Skill**（精選 15、已索引 30），每個 Skill 一次平台基準 Run
- **Runtime Image：§1～§11 量測於 `2026.08-1`（無 python3）；§12（9 筆）＋§13（36 筆）於 `2026.08-2`（含 python3）；§14（41 筆，另 4 筆授權受限跳過）於 `2026.08-3`（deps 修正後）。最新一組量測為 `2026.08-3`，`docx`／`pdf`／`pptx`／`xlsx` 四筆停在 `2026.08-2`。**
- 路徑：完整平台路徑（fork → Test Case → Dataset → Preflight → 確認 → Run → Worker 派送 → Sandbox → Trace → Artifact → Cleanup），無任何一段是假的
- 依據：[`02` §4.7 CONTENT-007／008](../02-specifications-and-acceptance-criteria.md)、[m2/README.md 第四批交付摘要](README.md)
- 機器可讀原始資料：`results.json`／`rows.json`（scratchpad，未入庫；本報告的每個數字都可由下列 Run ID 在資料庫重查）

---

## 1. 一句話結論

**精選 15 個 Skill 全部完成基準試跑且結果為「符合」（15/15）；已索引 30 個中 20 個符合、9 個被平台自身的閘道預算計數中止、1 個 Run 成功但沒有產出。** 45 個 Run 全部完成 cleanup、全部 Trace 無斷號、全部 Virtual Key 已撤銷。閘道實付 **$3.3932**（硬上限 $5）。

**最重要的發現不是哪個 Skill 不行，而是三件平台自己的事**：

1. `cmd/worker` 從未把 River client 掛回 `run.Service.Queue`，導致**每一個 Run 結束後都不會排 cleanup**——沙箱與 Virtual Key 全部存活至今（RUN-007 在真實部署裡等於沒有生效）。已修，見 §6.1。
2. LiteLLM 以「Current cost ≈ 0.50」拒絕請求，而**同一把金鑰自己的 spend log 只有 $0.009–$0.24**，兩者相差最多 50 倍。16 個 Run 因此被中止；把 per-Run 上限改成 $2.00 重跑，**7 個精選全數一次通過**。這不是 Skill 的失敗，是計數的失敗（§6.2）。**根因已於同日破案並修復**（LiteLLM 的樂觀預算保留，commit `3906fe5`）——驗證、9 筆補跑與合併後數字見 **§12**。
3. 開發用資料庫落後 migration 集 **0016～0020 共 5 份**，`test_cases.deleted_at`、`run_attempts`、`outbox_events`、`run_permission_confirmations` 全部不存在，任何 Run 都跑不起來（§2.2）。

---

## 2. 前置作業與環境（含對既有 stack 的操作紀錄）

### 2.1 SeaweedFS 重建（唯一預先核准的既有 stack 操作）

`infra/compose/seaweedfs-s3.json` 已入庫但容器未重建，匿名 bucket 無金鑰可簽章，`PresignGet` 必然失敗。

```
docker compose --env-file ../../.env -f docker-compose.yml up -d seaweedfs   # 只此一個服務
```

| 檢查項 | 重建前 | 重建後 |
| --- | --- | --- |
| 匿名 `GET /skillhub/packages/<hash>.zip` | **200**（誰都讀得到整個 bucket） | **403** |
| 匿名 `GET /skillhub/`（ListBucket） | **200** | **403** |
| SigV4 簽章 `GET`（`skillhubdev`） | 未設定，無金鑰可簽 | **200** |
| **預簽 URL `GET`（SBX-008 實際用的形式）** | 無法產生 | **200，2928 bytes** |
| `skillhub-api` `/healthz` | 200 | **200** |
| `GET /api/skills/search?q=excel` | 20 筆 | **20 筆**（第一筆 `excel-format`） |

`postgres`／`api`／`litellm` 三個容器全程未動。

> **重建的副作用，必須交接**：既有的 `skillhub-api` 容器啟動時**沒有** `OBJSTORE_ACCESS_KEY`／`OBJSTORE_SECRET_KEY`（匿名存取），bucket 關閉匿名後它**已無法讀寫物件儲存**——詳情頁檔案樹（`GET /api/skills/{id}/files`）、匯入、Preflight 的套件掃描都會失效。搜尋與 `/healthz` 不碰物件儲存所以仍正常，上表的驗證也因此全綠。**本報告不動 api 容器**（依交辦範圍），但下一位接手者必須帶著這兩個環境變數重建它；正確的作法是把 api／worker 一併寫進 `infra/compose/docker-compose.yml`（ADR-019 的 CORE-001 已列此項）。

### 2.2 開發資料庫落後 migration 集（試跑的真正阻塞點）

第一次建立 Test Case 就 500。原因是線上 `skillhub` 資料庫的 schema 停在 0015 前後：

| Migration | 缺少的物件 | 後果 |
| --- | --- | --- |
| `0016_run_orchestration` | `run_attempts`、`outbox_events`、`runs.cancel_requested_at` | 派送無法建立 attempt；`run_supervise` 每次都以 `column "cancel_requested_at" does not exist` 失敗（佇列裡有 367 個 retryable job 就是它） |
| `0017_test_case_deletion` | `test_cases.deleted_at` | `GetTestCase`／`CreateTestCase` 的 `RETURNING *` 掃描失敗 → **建立 Test Case 一律 500** |
| `0018_run_scheduling` | `runs.failure_class` | 失敗分類無處可寫 |
| `0019_trace_ingestion` | `trace_events.event_id`／`attempt`／`seq` 索引／`late` | 冪等與斷號偵測不存在 |
| `0020_run_permission_confirmations` | 整張表 | SEC-002 gate B 無法確認 |

處置：`pg_dump` 備份後，以 `psql -v ON_ERROR_STOP=1 --single-transaction -f` 逐份套用 0016→0017→0018→0019→0020，全部成功、無資料遺失（`runs` 當時 0 列）。**這是既有 stack 的第二個變更，超出原本核准範圍，在此明列**：不套用就沒有任何 Run 能建立，CONTENT-007／008 無法執行。套用的是 repo 自己已入庫的 migration，非本次新增。

### 2.3 試跑用的執行環境

為了不動既有 api／worker，另起三個**臨時**容器（跑完即刪，見 §8）：

| 容器 | 內容 |
| --- | --- |
| `skillhub-sandboxd` | `services/sandbox/cmd/sandboxd`，`SKILLHUB_SANDBOX_NETWORK=skillhub_egress`、`SLOTS=3` |
| `skillhub-baseline-api` | `cmd/api`，帶 `OBJSTORE_*` 金鑰、`DEV_LOGIN=1`、trace 簽發密鑰、閘道位址與 Provider 註冊表 |
| `skillhub-baseline-worker` | `cmd/worker`，Provider／閘道／物件儲存／trace ingestion 全配置，`SKILLHUB_RUN_MODEL=gpt-5.4-mini`、per-Run `max_budget=$0.50`、`tpm_limit=200000` |

**api 也需要閘道環境變數**，這一點 m2/README 的「最短路徑」沒有寫：`defaultPolicy()`（`internal/run/service.go`）在 **API 程序**裡讀 `SKILLHUB_MODEL_GATEWAY_*` 來組 `policy_snapshot.egress.allow`。api 沒有它時，允許清單是空的 → 沙箱被派到 `--network none` → Agent SDK 對閘道空等 188 秒後 `Request timed out`。同理 api 需要 `SKILLHUB_SANDBOX_PROVIDERS` 與對應 token，否則 Preflight 的 Provider 摘要只會寫 `unassigned`。**建議把這兩段補進 m2/README 的環境變數表。**

---

## 3. 試跑方法（同一模板套 45 個，不為個別 Skill 調參）

**Workspace**：以 dev login 建立專用使用者 `content-baseline`，其個人 Workspace `91b951b3-ce71-4a4f-9e0b-c5548e133fe1` 即為**試跑用臨時 Workspace**；目錄 Workspace（`87401dad…`，`is_catalog`）全程唯讀，45 筆 Skill 與其版本一列未改（鐵律 4）。每個 Skill 以 `POST /skills/{id}/fork` 複製進臨時 Workspace（WS-001 的正規路徑，套件內容定址不複製位元組），Run 掛的是 fork 版本，血緣由 `forked_from_version_id` 指回目錄版本。

**Prompt 模板**（唯一變數是 Skill 名稱與該 Skill 自己的第一句任務範例句，即 CONTENT-005 索引增強的產物）：

```
請使用「{skill 名稱}」這個 Skill 完成以下任務:{該 Skill 的第一句任務範例句}

執行環境說明:
1. 輸入檔案只有兩個,都在 /work/data/:data.csv(表格資料)與 draft.md(一段文字草稿)。
   上面的任務若提到其他檔名,一律改用這兩個檔案裡合適的那一個。
2. 所有產出檔案必須寫到 /out/artifacts/ 目錄;寫在其他地方的檔案不會被保存。
3. 完成後用一行文字說明你產出了哪些檔案。
```

三條規則對應 m2/README 點名的兩個絆腳點：**點名 Skill**（PDM-011 實測自主觸發率為 0）、**明講寫到 `/out/artifacts/`**、**Dataset 在 `/work/data/<file_name>`**。

**Dataset（45 個 Run 完全相同）**：`data.csv`（8 列訂單資料，刻意含重複列、混合日期格式 `2024-01-05` 與 `05/02/2024`、含前後空白的金額、缺值、國名異形 `USA`／`United States`／`U.K.`／`Deutschland`）與 `draft.md`（一段有贅詞與自誇語氣的 Q2 更新草稿）。兩個檔一起給每個 Skill，是為了讓 data／documents／writing 三類共用同一組輸入而不必分類調參。

**驗收條件（每個 Test Case 三條，同一組）**：① trace 中出現對指定 Skill 的 `skill_activation`；② `/out/artifacts/` 至少一個檔案；③ 最終回覆說明產出。

**判定**：`符合` = Run 終態 `succeeded` **且** trace 有 `skill_activation` **且** artifact 封存內確有檔案。三者缺一即照實記為 `平台中止` 或 `未產出`，不四捨五入。

**併發**：2（sandboxd 宣告 3 個 slot；留一格給銷毀中的沙箱）。**成本**：per-Run `max_budget=$0.50`，總花費硬上限 $5。

---

## 4. 逐 Skill 結果

Trace 事件數欄位的 `⚠` 表示 `complete=false`（有斷號）；**45 個 Run 全部無斷號，故全表無 ⚠**。

| Skill | 層級 | 類別 | Run 終態 | failure_class | Artifact | in/out tokens | 成本 USD | Trace 事件 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| `ai-written-check` | 精選 | writing | succeeded | — | ai_written_check_report.md | 70691/537 | 0.0424 | 10 |
| `brand-guidelines` | 精選 | writing | succeeded | — | q2_update_anthropic_brand.html | 358223/26156 | 0.1687 | 44 |
| `csv-to-json` | 精選 | data | succeeded | — | data.json | 123793/2412 | 0.0233 | 14 |
| `data-analyst` | 精選 | data | succeeded | — | standardized_data.csv | 198638/8970 | 0.0784 | 21 |
| `data-cleanliness-scan` | 精選 | data | succeeded | — | cleanliness_report.json, cleanliness_report.md | 163547/15571 | 0.1013 | 16 |
| `excel-deduplicate` | 精選 | data | succeeded | — | sales_deduplicated.csv | 200735/6309 | 0.1118 | 43 |
| `excel-find-duplicates` | 精選 | data | succeeded | — | duplicate_row_numbers.txt, duplicate_rows_report.txt | 165997/7143 | 0.0552 | 17 |
| `excel-format` | 精選 | documents | succeeded | — | data_formatted.xlsx | 149799/11305 | 0.1208 | 38 |
| `excel-freeze` | 精選 | documents | succeeded | — | data_frozen.xlsx | 75206/10388 | 0.1015 | 23 |
| `excel-insert` | 精選 | documents | succeeded | — | data_backup_original.csv, data_with_formatted_date.csv | 93914/4574 | 0.0792 | 21 |
| `handoff` | 精選 | documents | succeeded | — | handoff-current-conversation.md | 51472/3964 | 0.0345 | 6 |
| `humanizer` | 精選 | writing | succeeded | — | humanized_draft.md | 133926/2054 | 0.0366 | 11 |
| `internal-comms` | 精選 | writing | succeeded | — | 3p_update.md | 52412/442 | 0.0243 | 14 |
| `line-edit` | 精選 | writing | succeeded | — | q2_update_polished.md | 87979/1821 | 0.0177 | 8 |
| `text-to-numeric` | 精選 | data | succeeded | — | text_to_numeric_report.md, data_numeric.csv | 52724/1014 | 0.0545 | 19 |
| `add-data-dictionary` | 已索引 | data | succeeded | — | data_dictionary.md | 106220/2981 | 0.0355 | 12 |
| `add-iso3166` | 已索引 | data | succeeded | — | data_iso3166.csv | — | — | 13 |
| `copyright-creative-work` | 已索引 | writing | failed | workload_error | （無） | 0/0 | 0.0222 | 15 |
| `course-quiz-builder` | 已索引 | documents | succeeded | — | quiz.html, questions.json | 233316/20228 | 0.1183 | 22 |
| `cringe-check` | 已索引 | writing | succeeded | — | draft_rewritten.md, cringe_check_report.md | 129652/6331 | 0.0426 | 15 |
| `data-comparability` | 已索引 | data | succeeded | — | comparability_plan.md | 244156/12834 | 0.0855 | 23 |
| `data-shape` | 已索引 | data | succeeded | — | column_mapping.md, schema.sql, schema_proposal.md | 190705/13053 | 0.0786 | 18 |
| `date-wrangling` | 已索引 | data | succeeded | — | **（無）** | 106888/1646 | 0.0186 | 11 |
| `document-format-skills` | 已索引 | documents | failed | workload_error | （無） | 70029/2958 | 0.0295 | 23 |
| `docx` | 已索引 | documents | failed | workload_error | （無） | 72341/1050 | 0.0255 | 17 |
| `excel-date-to-text` | 已索引 | data | succeeded | — | data_date_text.csv | 264649/2877 | 0.0914 | 23 |
| `excel-delete` | 已索引 | data | failed | workload_error | （無） | 36972/440 | 0.0208 | 15 |
| `excel-filter` | 已索引 | data | succeeded | — | filtered_data.csv | 162292/3078 | 0.0317 | 15 |
| `excel-mapping-replace` | 已索引 | data | succeeded | — | data_replaced.csv | 217177/4028 | 0.0580 | 20 |
| `excel-merge` | 已索引 | data | succeeded | — | merged.xlsx | 333620/22095 | 0.1348 | 41 |
| `excel-regex-clean` | 已索引 | data | succeeded | — | cleaning_report.txt | 174922/6047 | 0.0661 | 16 |
| `excel-scout` | 已索引 | data | succeeded | — | orders_date_scout_report.md | 172509/5108 | 0.0416 | 16 |
| `excel-sort` | 已索引 | data | failed | workload_error | （無） | 16559/119 | 0.0123 | 11 |
| `excel-split` | 已索引 | data | failed | workload_error | （無） | 35531/386 | 0.0262 | 12 |
| `excel-validate` | 已索引 | data | succeeded | — | data_quality_report.md | 193914/16559 | 0.1106 | 19 |
| `full-review` | 已索引 | writing | succeeded | — | full_review.md | 236671/11593 | 0.0878 | 23 |
| `json-restructure` | 已索引 | data | failed | workload_error | （無） | 0/0 | 0.0087 | 10 |
| `pdf` | 已索引 | documents | succeeded | — | merged.pdf, draft.pdf, data.pdf | 231116/16274 | 0.1378 | 28 |
| `pii-flag` | 已索引 | data | succeeded | — | pii_summary.md, pii_summary.json, pii_report.jsonl | 257842/13412 | 0.0901 | 21 |
| `pptx` | 已索引 | documents | failed | workload_error | generate_q2_sales.js | 279052/43701 | 0.2367 | 26 |
| `shorten` | 已索引 | writing | succeeded | — | shortened_email.md | 94611/2534 | 0.0224 | 10 |
| `sokrati` | 已索引 | writing | succeeded | — | rewrite_report.md | 159028/5804 | 0.0478 | 18 |
| `standardise-country-names` | 已索引 | data | succeeded | — | data_standardisation_report.md, data_standardised.csv | 195889/10376 | 0.0708 | 24 |
| `unicode-consistency` | 已索引 | data | succeeded | — | unicode_audit.json, unicode_report.md | 258280/3803 | 0.0925 | 29 |
| `xlsx` | 已索引 | documents | failed | workload_error | data_with_profit_margin.xlsx | 218534/27092 | 0.1934 | 28 |

> `pptx` 與 `xlsx` 的 artifact 欄有檔案但終態是 failed：工作負載在被閘道中止**之前**已經寫出檔案，收集照常發生。這正是「Run 失敗但仍留下部分產物」該有的樣子，不是矛盾。

---

## 5. 彙總統計

### 5.1 判定分布

| 判定 | 精選（15） | 已索引（30） | 合計（45） |
| --- | --- | --- | --- |
| **符合**（succeeded ＋ 有 activation ＋ 有 artifact） | **15（100%）** | 20（66.7%） | **35（77.8%）** |
| 平台中止（閘道預算計數，非 Skill 問題） | 0 | 9 | 9 |
| Run 成功但未產出 | 0 | 1 | 1 |

依類別：data 20/25 符合、writing 9/10、documents 6/10。

### 5.2 成本

| 指標 | 數值 |
| --- | --- |
| Trace 回報成本合計（44 個有 usage 事件的 Run） | **$3.0879** |
| 閘道 `LiteLLM_SpendLogs` 實際增量（本次試跑全期） | **$3.3932**（755 次呼叫） |
| 兩者差額 | $0.3053 |
| 單次 Run 中位數／平均／最大 | $0.0566／$0.0702／$0.2367 |
| 硬上限 | $5（未觸及；$2.5 中止規則亦未觸發——到達 $2.5 時已完成 43/45） |

差額的組成，逐項說明而不含糊：7 個精選重跑時，**第一次（被中止那次）的 $0.2527 仍計入閘道帳**但已不在 Trace 合計裡；兩把診斷用探針金鑰 $0.00006；其餘約 **$0.05** 是 ADR-017 早已寫明的「讀數不是帳本」——`usage.cost_usd` 是工作負載結束前對 `/key/info` 的一次讀取，最後一次 flush 若落在讀取之後就少算。**權威來源是閘道 per-key spend，不是 Trace。**

實際單價比第四批的參考值（$0.006–0.017）高一個量級，原因是真實 Skill 的 `SKILL.md` 遠大於 e2e 的示範套件：輸入 token 中位數 16.6 萬（最大 33.4 萬）。**45 個 Skill 一輪基準試跑的可預期成本是 $3–4，不是 $1。**

### 5.3 Trace 與資源

| 指標 | 數值 |
| --- | --- |
| Trace 事件總數 | 1112（全部 `masked = true`，0019 的 CHECK 使跳過遮罩在資料庫層不可能） |
| 事件數中位數／最少／最多 | 18／6／44 |
| **`complete = false`（有斷號）的 Run** | **0 / 45** |
| 缺 `usage` 事件的 Run | 1（`add-iso3166`） |
| Artifact 封存總量 | 301,568 bytes，共 51 個檔案（md 21、csv 10、json 5、xlsx 4、txt 3、pdf 3、html 2、其他 3） |
| Run 牆鐘中位數／最長 | 106 秒／462 秒 |
| `cleanup_status = cleaned` | **73 / 73**（含前置除錯與中止的 Run） |

---

## 6. 三個平台層發現（本批的主要價值）

### 6.1 `cmd/worker` 從不排程 cleanup —— RUN-007 在真實部署裡沒有生效（已修）

`run.Service` 的兩處清理路徑都以 `s.Queue != nil` 為前提：終態轉移在同一交易內排 `CleanupArgs`（`service.go:383`），supervisor 的積壓補掃在 `s.Queue == nil` 時直接 `break`（`supervisor.go:78`）。而 `cmd/worker/main.go` 建好 `run.Service` 時**沒有設定 `Queue`**——`queue.New()` 的 client 只交給 River 當消費者，沒有掛回服務。

後果：worker 驅動的每一個 Run 結束後都**不會**產生任何 `run_cleanup` job。實測佐證——前兩個 Run 結束後 `river_job` 裡 `run_cleanup` 是 0 列，`runs.cleanup_status` 停在 `pending`，`LiteLLM_VerificationToken` 裡 2 把 Virtual Key 依然有效。

整合測試看不到這個洞：`startWorkerWith` 有一行 `if svc.Queue == nil { svc.Queue = c }`，測試環境替 main 補上了 main 自己沒做的事。

修正是一行 wiring（`services/platform/cmd/worker/main.go`），修完立刻復測：`cleanup_status` 兩列同時變 `cleaned`，`LiteLLM_VerificationToken` 的 attempt 金鑰歸零。本批 73 個 Run **全部** `cleaned`。

### 6.2 閘道預算計數與其自身 spend log 不一致，造成 16 個 Run 被誤中止

第一輪 45 個 Run 中有 16 個以 `failed / workload_error` 結束，**16 個全部是同一個錯誤**：

```
API Error: Request rejected (429) · Budget has been exceeded!
Key=skillhub-attempt-03d6493d-… Current cost: 0.50056965, Max budget: 0.5
```

但查該金鑰自己的帳：

| 金鑰 | 閘道拒絕時宣稱的 Current cost | `LiteLLM_SpendLogs` 該金鑰實際合計 |
| --- | --- | --- |
| `skillhub-attempt-03d6493d…`（`brand-guidelines`） | 0.50057 | **0.02717**（17 次呼叫） |
| 全批最貴的一把金鑰 | — | 0.23675 |

**沒有任何一把金鑰的 spend log 接近 $0.50。** 兩個對照實驗也排除了「單次呼叫的預估扣款」：新鑄一把 `max_budget=0.5` 的金鑰，(a) 非串流、`max_tokens=32000` 一次呼叫後 `/key/info` spend = **$0.0000285**；(b) 串流、`max_tokens=64000` 一次呼叫後同樣是 **$0.0000285**。

決定性的驗證是重跑：把 per-Run `max_budget` 從 $0.50 提到 $2.00，**7 個受影響的精選 Skill 全數一次通過**，實際花費 $0.054–$0.169（沒有一個接近 $0.5）。

**結論**：這 16 個不是 Skill 的失敗，是平台把自己的 Run 掐掉。依交辦的試跑紀律，這屬 infra 類失敗，允許重試一次——重試對象取**精選 15 個**（CONTENT-008 的允收對象），9 個已索引的維持第一輪紀錄並記為「平台中止」，未再花錢重跑（成本上限考量）。

**待處理（不在本批範圍）**：LiteLLM 執行預算檢查所用的計數與它寫進 `LiteLLM_SpendLogs` 的數字為何不一致，尚未定位（可能在 `user_api_key_cache` 的累加路徑；本批未深入）。在查清之前，**per-Run `max_budget` 不是一個可信的成本閘門**——它會在遠低於名目上限處觸發。PDM-003 v5 的 $0.50 預設值需要在此之後重新檢視。

### 6.3 Runtime Image 沒有 Python，而 45 個 Skill 中 33 個的執行範例是 Python

沙箱內實測（trace `script_log` 事件原文）：

```
/bin/bash: line 75: python3: command not found
```

`infra/images/runtime-agent-sdk/Dockerfile` 基底是 `node:22-bookworm-slim`，只額外裝了 `unzip`。`tools/content/seed-skills.json` 記錄 45 個 Skill 中 **33 個 `deps_runtime = python`**（openpyxl／pandas／numpy），1 個 node，11 個無依賴。

**這不必然是失敗**：Agent 讀得懂 `SKILL.md` 的作法後改用 Node 重寫等效邏輯，33 個 Python 依賴的 Skill 仍有 24 個判定符合，`excel-format`／`excel-freeze`／`excel-merge` 甚至真的產出了 `.xlsx`。但這代表**執行的不是 Skill 帶的腳本，而是模型對腳本的轉譯**——同一個 Skill 在裝了 Python 的環境與這裡會是兩種行為，而目錄沒有任何欄位透露這件事。`xlsx` 的「限制」還明文要求 `LibreOffice` 與 `scripts/recalc.py` 重算公式，那在這個映像裡完全不存在。

---

## 7. 對 CONTENT-005 已入庫摘要的抽查對照

對照方式：把每個 Skill 的**已入庫** `enriched_summary` 與「限制」欄，逐一比對該 Skill 這次 Run 的實際行為（activation、產出檔案、失敗原因、trace 內的工具呼叫）。

### 7.1 摘要本身站得住

**沒有發現任何一筆摘要宣稱了 Run 證明不存在的能力。** 幾個具體對照：

- `excel-format` 的限制寫「執行範例使用 Python 與 openpyxl；選用自動欄寬時另使用 pandas 與 numpy」——實測 Run 確實嘗試 Python、確實失敗、確實改用其他方式完成。**摘要說的是對的。**
- `xlsx` 的限制寫「含公式的活頁簿必須使用 LibreOffice 與 `scripts/recalc.py` 重新計算」——實測環境沒有 LibreOffice。**摘要說的是對的，是平台達不到。**
- `csv-to-json` 的限制寫「使用 pandas 的型別推斷…前，需要先安裝 pandas」——實測 Run 沒有 pandas、也沒有裝，改用等效邏輯完成並產出 `data.json`。**一致。**

**這是 CONTENT-005 的一個正面結果**：45 筆摘要在對照真實執行後仍站得住，第三方（Judge 模型）審校的結論在行為層面得到支持。

### 7.2 不符清單

不符不在「摘要說謊」，而在**目錄呈現的完整度**與**平台能力**兩處：

| # | 類型 | 對象 | 內容 |
| --- | --- | --- | --- |
| 1 | **揭露缺口** | 11 個 Skill：`data-shape`、`docx`、`excel-delete`、`excel-filter`、`excel-find-duplicates`、`excel-mapping-replace`、`excel-merge`、`excel-sort`、`excel-split`、`excel-validate`、`pdf` | 套件的 `deps_runtime` 是 Python，但其「限制」欄**完全沒有提到** Python／openpyxl／pandas。同類的另外 22 個都有提。使用者從詳情頁看不出這 11 個需要一個平台目前給不了的執行環境。**處置屬 CONTENT-005 的 `需修改`**：調 prompt 後重跑索引增強，不得就地改寫審核紀錄（`02` §4.7）。 |
| 2 | **平台與文件的落差（非摘要問題）** | 33 個 Python 依賴 Skill | 摘要說要 Python，平台沒有 Python。目錄沒有任何欄位表達「這個 Skill 的腳本在本平台不會被執行」。DISC-002 的「Agent 相容」軸本應承載這件事——見 §8。 |
| 3 | **「成功」不等於「做到」** | `date-wrangling` | Run 終態 `succeeded`、Skill 有被啟用、Trace 完整，但 `/out/artifacts/` 是空的。最終回覆是**反問使用者**：「`data.csv` 裡沒有 `created_at`…`05/02/2024` 請確認是 DD/MM 還是 MM/DD」。這是模型合理的行為，但它揭露 `run.mjs` 的 `finish("succeeded")` 只代表「agent 這一輪沒有拋錯」，**與任務是否完成無關**。UI 若把 succeeded 呈現為「可用」會誤導；判定任務是否達成是 EVAL-001 的工作，本批只把這個語意落差記錄下來。 |
| 4 | **成本可能無聲缺席** | `add-iso3166` | Run 成功、產出 `data_iso3166.csv`、Trace 13 個事件且無斷號，但**沒有 `agent_output(final)` 也沒有 `usage` 事件**——SDK 這一輪沒有給出 `result` 訊息，而 `run.mjs` 只在 `result` 分支裡發 usage。結果是這個 Run 的 token 與成本**在平台端完全不存在**（本報告表格中的「—」）。TRACE-004 因此有一個未涵蓋的路徑：**沒有 `result` 訊息時，成本無人回報，而 Run 仍記為成功。** 閘道那邊當然有帳，落差就在這裡。 |

---

## 8. DISC-002「Agent 相容」軸：schema 沒有欄位，結果留在本報告

`02:DISC-002` 的分期表把「Agent 相容」標為 **M2（依 Sandbox 實測）**，本批就是它的資料來源。查過的結論：

- **資料庫沒有任何欄位可以回寫。** 全 schema 內沒有 compat／agent／capability／runtime／verified 相關欄位（`runs.runtime_snapshot` 是每個 Run 自己的排程快照，不是 Skill 層級的結論）。
- **值是寫死在 Go 裡的常數**：`internal/catalog/http.go` 的 `resultFacets()` 與 `detail.go` 的 `compatibility{}` 一律回 `Capability: "unverified"`、`Runtime: "unverified"`；`search_documents` 沒有對應欄位可讀。
- **API 端目前正確地拒絕該篩選**：`unavailableFilters["agent"]` 回 400 並附理由「Agent 相容狀態需要 Sandbox 試跑才有結果（M2），目前一律為未驗證」。

**因此本批不新增 migration、不發明 schema**（依交辦）。缺口與所需決策記錄如下，供接手者開工。

> **2026-08-16 後續**：下列四個待決已由 **migration 0022（`skill_runtime_compatibility`，鍵為 (Skill Version × Runtime Image)）** 逐條回答並實作，本節的 45 筆實測值已用 `tools/content/backfill-agent-compatibility.sql` 入庫（12 `native` ＋ 33 `transpiled`，全部掛在 `2026.08-1`）；`2026.08-2` 上的 9 筆見 §12.4。原文保留供回溯。

| 待決 | 說明 |
| --- | --- |
| 欄位歸屬 | 結論是 Skill Version 層級（同一版套件的相容性不隨 Run 變）還是 (Skill Version × Runtime Image) 層級？後者才誠實——本批的 33 個 Python 結論**只對 `skillhub/runtime-agent-sdk:2026.08-1` 成立**，換一個裝了 Python 的映像結論就變。 |
| 兩軸的判準 | `capability`：本批可直接供給——45/45 的 trace 都有 `skill_activation`，即「掛上去會被啟用」全數成立。`runtime`：需要「腳本的宣告執行環境 ⊆ 映像提供的執行環境」這個判定，而映像的能力清單目前沒有任何地方以資料形式存在。 |
| 樣本量 | 一個 Skill 一次 Run 不足以宣告 `passed`；要幾次、失敗一次是否降級，未定。 |
| 呈現 | 值域至少要有 `unverified` 以外的第三態表達「在此 Runtime 下腳本不會被執行、由模型轉譯」——它既不是 passed 也不是 failed。 |

**本批可直接回填的實測值（等欄位存在時）**：`capability = activated` 45/45；`runtime`：11 個無依賴 Skill 為原生可執行，1 個 node 可執行，33 個 Python 依賴為「腳本不可執行、模型轉譯」。

> **2026-08-16 追加（本節不改寫，只指路）**：上表四個待決已裁定並落地——欄位歸屬取
> **(Skill Version × Runtime Image)**、兩軸值域為 `activated`／`not_activated`／`unverified` 與
> `native`／`transpiled`／`failed`／`unverified`、樣本量 1 次基準 Run 即可寫入並帶 `source_run_id`。
> Migration `0022_agent_compatibility.sql` 建表，`tools/content/backfill-agent-compatibility.sql`
> 回填本節這 45 筆（`capability` 45/45、`runtime` 12／33），目錄的詳情與搜尋已讀真值，DISC-002 的
> 「Agent 相容」篩選維度啟用。決策理由見 migration 檔頂與 [`04` 殘項乙-4](../04-backlog-and-handoffs.md)。
> §11 第 7、8 兩項因此關閉（第 8 項「Runtime Image 要不要含 Python」的裁定是**加**，映像升
> `2026.08-2`，見殘項乙-6）——**連帶注意：本節的結論只對 `2026.08-1` 成立，新映像需要新的一輪基準。**

---

## 9. 可追溯性（CONTENT-008 允收第 3 條）

每次基準試跑都可追溯 Skill Version、Test Case 快照、Provider、Runtime 與 Trace：

- **Skill Version**：`runs.skill_version_id` → fork 版本 → `skills.forked_from_version_id` → 目錄版本（內容雜湊相同，套件物件同一個 key）。
- **Test Case 快照**：`runs.test_case_snapshot_id`，凍結 Prompt、三條驗收條件與兩個 Dataset 的檔名與內容雜湊（不可變，鐵律 4）。
- **Provider／Runtime**：`runs.provider = self_hosted`，`runs.runtime_snapshot` 記 `claude_agent_sdk 0.3.233`／`in_sandbox_sdk`／`isolation_level: container`／`rootless: true`／`model: gpt-5.4-mini`。
- **Trace**：`trace_events`（1112 列，全數 `masked`），`GET /runs/{id}/trace?mode=advanced` 可重建。
- **Artifact**：`run-artifacts/<run_id>/<attempt_id>/artifacts.tar`（SeaweedFS `skillhub` bucket），逐檔 manifest。

範例（前兩筆）：`add-data-dictionary` → run `f40ab760…`／version `8d660a32…`；`add-iso3166` → run `cbbe6606…`／version `f078b1d3…`。完整對照在 `results.json`。

**source-available 內容**：本批全部照常試跑，未產出任何 Download Artifact（Run 產物不是 Download Artifact，`PACK-001` 尚未實作），符合 CONTENT-004／ADR-012。

---

## 10. 允收對照與勾選

### CONTENT-007（範例資料、Prompt 與驗收條件）

| 允收準則 | 狀態 |
| --- | --- |
| 每個精選 Skill 至少一組範例 Dataset、User Prompt 與驗收條件，內容可散布 | ✅ 15/15；Dataset 為本報告自製的合成資料（無第三方內容），Prompt 由模板＋該 Skill 自己的任務範例句組成，驗收條件三條 |
| Prompt 必須明確點名該 Skill | ✅ 模板第一句即點名；未提供不點名變體 |
| `writing` 類的每個精選附一份可編輯 rubric，供 LLM Judge 逐項回傳證據引文 | ❌ **未做**。本批給的是三條通用驗收條件，不是逐項 rubric；EVAL 的 Judge 介面（EVAL-001／002）尚未實作，rubric 沒有消費端 |
| 範例資料不得包含 Secrets、憑證或個資 | ✅ 合成資料，人名為公眾歷史人物、無任何憑證；1112 個 trace 事件全數通過遮罩 |
| 實際執行使用的 Prompt 與驗收條件以快照保存 | ✅ `test_case_snapshots`，不可變 |

→ **不勾選**（rubric 一項未達成）。

### CONTENT-008（平台基準試跑）

| 允收準則 | 狀態 |
| --- | --- |
| 每個精選 Skill 至少完成一次基準試跑且整體結果為「符合」 | ✅ **15/15 符合** |
| 基準試跑在隔離 Sandbox 內執行，不在匯入或掃描階段執行 | ✅ 全部經 sandboxd 派送至獨立容器（`--network` 僅接 `skillhub_egress`，唯一可達位址是閘道） |
| 可追溯 Skill Version、Test Case 快照、Provider、Runtime 與 Trace | ✅ §9 |
| 未通過者不得標記為精選 | ✅ 無精選未通過。**另注**：目前沒有任何 Skill 在平台上被標記為精選——`tier` 全目錄同為「已索引」（`tierLabel()` 寫死），精選只存在於 `tools/content/seed-skills.json` 的策展判斷，這是 CONTENT-003／DISC-002「來源層級」維度的既有缺口 |
| source-available 內容照常試跑但不產出 Download Artifact | ✅ §9 |

→ **勾選**。九項精選檢查的 ⑧ 可由 `pending` 改記 `pass`（15/15）。

---

## 11. 留給後續的事

1. **閘道預算計數（§6.2）**——定位 LiteLLM 的預算計數為何與 spend log 差 50 倍；在此之前 per-Run `max_budget` 不可信，PDM-003 v5 的 $0.50 預設需重新檢視。
2. **`skillhub-api` 容器需帶 `OBJSTORE_*` 金鑰重建**（§2.1），並把 api／worker 寫進 compose（ADR-019 CORE-001）。
3. **m2/README 環境變數表補兩列**：api 也需要 `SKILLHUB_MODEL_GATEWAY_URL`／`_KEY`（否則 egress 允許清單是空的）與 `SKILLHUB_SANDBOX_PROVIDERS`／token（否則 Preflight 的 Provider 摘要是 `unassigned`）。
4. **11 個 Skill 的「限制」欄缺 Python 揭露**（§7.2 #1）——走 CONTENT-005 的 `需修改` 流程重跑增強。
5. **`run.mjs` 的 usage 事件只掛在 `result` 分支**（§7.2 #4）——沒有 result 訊息時成本無聲缺席，TRACE-004 有洞。
6. ~~**9 個已索引 Skill 尚未取得有效基準**（`copyright-creative-work`、`document-format-skills`、`docx`、`excel-delete`、`excel-sort`、`excel-split`、`json-restructure`、`pptx`、`xlsx`）——第一輪被閘道中止，未重跑；修好 §6.2 後補跑，預估 $1.0–1.5。~~ → **2026-08-16 補跑完成，見 §12。**
7. ~~**DISC-002「Agent 相容」欄位設計**（§8）——四個待決先答，再談 migration。~~ → **已由 migration 0022 回答並實作**（`skill_runtime_compatibility`，鍵為 (Skill Version × Runtime Image)）；本報告的實測值已全數入庫，見 §12.4。
8. ~~**Runtime Image 要不要含 Python**~~ → **已定案**：`2026.08-2` 加入 python3 與目錄宣告的依賴集（見 [infra/images/README.md](../../../infra/images/README.md)）。實測效果見 §12。

---

## 12. 補跑：9 個 Skill 於 `2026.08-2`（2026-08-16）

§1～§11 的 45 筆量測**全部在 `skillhub/runtime-agent-sdk:2026.08-1`（無 python3）上取得，上面的表格與統計不改寫**。本節是之後另外一批量測，條件與上面不同，因此獨立成節。

### 12.1 前置：根因修復與驗證

§6.2 記錄的「幽靈 429」根因已由 commit `3906fe5` 破案並修復：**LiteLLM 自 v1.84 起對每個進行中的請求做樂觀預算保留**——把「所有輸入 token 以未快取全價 ＋ `max_tokens` 以輸出價」的理論最大成本先扣進計數器。對 gpt-5.4-mini 而言，光是 `max_tokens=64000` 的輸出就是 $0.288，而實付只有幾分錢；兩個重疊請求就把計數器頂到整個 $0.50。修法是 `infra/compose/litellm-config.yaml` 的 `disable_budget_reservation: true`，恢復以**已記錄花費**做讀取時判定。

重建 litellm 容器（`docker compose up -d --force-recreate litellm`，只此一服務）後實測：

| 驗證 | 結果 |
| --- | --- |
| 一把 `max_budget=$0.50` 金鑰，4 個併發請求（`max_tokens=64000`） | **4/4 皆 200，0 次預算拒絕**（修復前同條件重現 `Current cost: 0.78845825`，實際花費 $0.00096） |
| 該金鑰記錄花費 | $0.000126 |
| 反向驗證：`max_budget=1e-06` 的金鑰 | 第一次呼叫 200，第二次**正確拒絕**：`Current cost: 5.925e-05, Max budget: 1e-06`——拒絕依據是真實花費 |

**預算閘門仍然有效，只是現在拒絕的理由是真的。**

### 12.2 補跑條件

與 §3 完全相同的 Prompt 模板、Dataset、驗收條件、per-Run `max_budget=$0.50`、併發 2。**唯一的差別是 Runtime Image：`skillhub/runtime-agent-sdk:2026.08-2`**（含 python3 3.11.2、openpyxl 3.1.5、pandas 3.0.5）。同時本批也吃到了 trace 批的修正（usage 事件不再只掛在 `result` 分支、輸入 token 上限開始生效，事件 schema 1.1）。

### 12.3 逐筆結果（image `2026.08-2`，2026-08-16）

| Skill | 類別 | Run 終態 | failure_class | Artifact | in/out tokens | 成本 USD | Trace 事件 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `copyright-creative-work` | writing | succeeded | — | song-split-sheet-template.md, us-copyright-song-registration-prep.md | 124813/6675 | 0.0513 | 15 |
| `document-format-skills` | documents | succeeded | — | **cleaned_official.docx** | 164395/5390 | 0.0548 | 19 |
| `docx` | documents | succeeded | — | **q2_business_update_report.docx** | 118599/4553 | 0.1476 | 35 |
| `excel-delete` | data | succeeded | — | data_rows_5_to_10_deleted_report.txt, **data_rows_5_to_10_deleted.xlsx** | 63533/573 | 0.0634 | 21 |
| `excel-sort` | data | succeeded | — | **data_sorted_by_public_date.xlsx**, data_sorted_by_public_date.csv | 111867/4004 | 0.0309 | 13 |
| `excel-split` | data | succeeded | — | split/ 下 6 個 CSV（逐 customer 一檔） | 113227/1809 | 0.0293 | 15 |
| `json-restructure` | data | succeeded | — | data_grouped_by_country.json, data_grouped_by_country.provenance.md | 92799/1666 | 0.0540 | 18 |
| `pptx` | documents | **failed** | workload_error | q2_sales_update.pptx | 333606/38702 | 0.3019 | 55 |
| `xlsx` | documents | succeeded | — | **data_with_profit_margin.xlsx** | 177382/2594 | 0.1143 | 30 |

**8/9 符合**（succeeded ＋ `skill_activation` ＋ artifact 有檔案）。

**唯一失敗的 `pptx` 換了一個誠實的失敗理由**：不是預算，而是 **PDM-005 5.2a 的輸入 token 上限**——`run stopped at its input token ceiling: 333606 of 300000 tokens`，trace 內有對應的 `token_budget_exceeded` 事件，且該上限本來就顯示在 Run 前的權限摘要裡。這是設計中的硬限制正常作動，`pptx` 在此模板下確實吃不下（它在 `2026.08-1` 那輪也是全批最貴的一個）。要取得它的基準需要縮小任務或提高該 Skill 的 token 配額，屬後續。

### 12.4 幽靈 429 是否絕跡、成本、相容軸回填

| 項目 | 數值 |
| --- | --- |
| **9 個 Run 中的預算拒絕次數** | **0**（§6.2 的失敗模式完全消失） |
| trace `script_log` 中的 `python3: command not found` | **0 / 36 條**（`2026.08-1` 那輪是這個發現的來源） |
| 9 筆 trace 回報成本合計 | $0.8474 |
| 閘道實際增量（含 §12.1 的三次探針） | **$0.8606**（總上限 $2 未觸及） |
| 相容軸回填 | `tools/content/backfill-agent-compatibility.sql` 以 `-v image=…2026.08-2 -v python_runtime=native -v since='2026-08-16 10:00:00+00'` 執行，**寫入 9 列** `(Skill Version × 2026.08-2)`，全部 `capability=activated`／`runtime=native`；`2026.08-1` 的 45 列（12 native ＋ 33 transpiled）**逐位元不變**（回填前後全表 md5 相同） |

腳本因此改為**一次呼叫對應一個 (image, Run 時間窗)**：Run 的映像不在控制平面任何欄位裡（映像是執行節點的設定，`RunResult` 也不回傳），所以由呼叫端指明，時間窗負責把兩批分開。不加這個窗直接重跑，會把這 9 筆新量測貼上 `2026.08-1` 的標籤——因為那 9 個版本的「最新 Run」現在在新映像上——並靜靜污染它本該原封不動的 36 列。

### 12.5 誠實註記：其餘 36 筆仍是舊映像上的數字

> **2026-08-16 已解除**：本節的保留條件由 §13 的全量重測解決；下文保留為當時的誠實狀態。

**本節只重測了 9 筆。**§4 表格中的其餘 36 筆量測於 `2026.08-1`（無 python3），其中 **33 筆屬「Python 依賴」類、相容軸記為 `transpiled`**（腳本沒被執行，結果來自模型重寫）。在 `2026.08-2` 上這些很可能會轉為 `native`——本批 9 筆中的 `docx`、`xlsx`、`excel-*` 都從只產出文字檔變成真的產出 `.docx`／`.xlsx`，就是同一個機制。

**全量重測不在本批範圍**，屬後續選項；在它完成前，目錄對那 36 筆顯示的仍是 `2026.08-1` 上的結論，且 0022 的讀取路徑會把映像標籤一起顯示，不會假裝那是新映像的答案。§5、§7 的統計與不符清單同理，全部是 `2026.08-1` 條件下的量測。

### 12.6 對 §5 彙總的影響（合併後）

45 筆取「每個 Skill 最新一次量測」合併後：**符合 43（95.6%）、失敗 1（`pptx`，token 上限）、Run 成功但未產出 1（`date-wrangling`）**；「平台中止」類**歸零**。精選 15/15 不變（全部於 `2026.08-1` 量測，CONTENT-008 的允收結論不受本節影響）。兩批合計閘道實付 **$4.25**。

> **2026-08-16 後續**：§12.5 標為「後續選項」的全量重測已由負責人裁定執行，其餘 36 筆已在 `2026.08-2` 上重跑完畢——見 **§13**。本節（§12）的 9 筆量測不變。

---

## 13. 全量重測：其餘 36 個 Skill 於 `2026.08-2`（2026-08-16，負責人裁定）

§12.5 記為「後續選項」的全量重測經負責人裁定執行。**§1～§11 的 45 筆（`2026.08-1`）與 §12 的 9 筆（`2026.08-2`）表格與統計一律不改寫**，本節是第三批量測。

### 13.1 條件

與 §3 完全相同的 Prompt 模板、Dataset、驗收條件、per-Run `max_budget=$0.50`、併發 2，Runtime Image `skillhub/runtime-agent-sdk:2026.08-2`。總上限 $6。對象是 §12 未涵蓋的 **36 個 Skill**（45 − §12 的 9）。

**授權受限（422）**：本批預期可能撞到平行進行中的 anthropic-sa 試跑封鎖。四個 anthropic-sa Skill 中 `docx`／`pptx`／`xlsx` 已在 §12 量測，本批只剩 **`pdf`** 會受影響。實際執行時封鎖尚未合併，**`pdf` 正常跑完、沒有任何一筆收到 422**；若封鎖先落地，該筆應記為 `restricted，未重測`。

### 13.2 逐筆結果（image `2026.08-2`）

最後一欄是同一個 Skill 在 `2026.08-1` 的成本，供對照。

| Skill | 層級 | 宣告 Runtime | 判定 | Artifact | in/out tokens | 08-2 成本 | （08-1 成本） |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `ai-written-check` | 精選 | none | 符合 | ai_written_check_report.md | 70282/2661 | 0.0199 | 0.0424 |
| `brand-guidelines` | 精選 | none | 符合 | **anthropic_brand_q2_update.pptx** | 81869/12962 | 0.1576 | 0.1687 |
| `csv-to-json` | 精選 | python | 符合 | data.json | 0/0 | 0.0275 | 0.0233 |
| `data-analyst` | 精選 | python | 符合 | standardized_data.csv | 62338/495 | 0.0397 | 0.0784 |
| `data-cleanliness-scan` | 精選 | python | 符合 | cleanliness_report.json, cleanliness_report.md | 38971/4390 | 0.0592 | 0.1013 |
| `excel-deduplicate` | 精選 | python | 符合 | data_deduplicated.csv | 73206/1047 | 0.0246 | 0.1118 |
| `excel-find-duplicates` | 精選 | python | 符合 | excel_find_duplicates_report.md | 37273/3135 | 0.0466 | 0.0552 |
| `excel-format` | 精選 | python | 符合 | **data_formatted.xlsx** | 77338/354 | 0.0570 | 0.1208 |
| `excel-freeze` | 精選 | python | 符合 | **frozen_header.xlsx** | 35813/1053 | 0.0249 | 0.1015 |
| `excel-insert` | 精選 | python | 符合 | data.csv, data_backup_20260816115730.csv | 74839/1354 | 0.0386 | 0.0792 |
| `handoff` | 精選 | none | 符合 | handoff-current-conversation.md | 51506/3320 | 0.0209 | 0.0345 |
| `humanizer` | 精選 | none | 符合 | q2_update_humanized.md | 62729/631 | 0.0274 | 0.0366 |
| `internal-comms` | 精選 | none | 符合 | 3p-update.md | 35749/110 | 0.0280 | 0.0243 |
| `line-edit` | 精選 | none | 符合 | polished_email.md | 34964/764 | 0.0251 | 0.0177 |
| `text-to-numeric` | 精選 | python | 符合 | text_to_numeric_report.md, data_numeric.csv | 143238/4529 | 0.0461 | 0.0545 |
| `add-data-dictionary` | 已索引 | python | 符合 | data_dictionary.md | 107214/2662 | 0.0341 | 0.0355 |
| `add-iso3166` | 已索引 | python | 符合 | data_iso3166.csv | 142793/3540 | 0.0413 | **—** |
| `course-quiz-builder` | 已索引 | node | 符合 | quiz.html, questions.json | 251573/20062 | 0.1648 | 0.1183 |
| `cringe-check` | 已索引 | none | 符合 | revised-draft.md, cringe-check-notes.md | 53688/1901 | 0.0468 | 0.0426 |
| `data-comparability` | 已索引 | python | 符合 | comparability_plan.md | 76672/10933 | 0.0975 | 0.0855 |
| `data-shape` | 已索引 | python | 符合 | mapping.csv, schema.sql, schema_proposal.md | 99341/7880 | 0.0591 | 0.0786 |
| `date-wrangling` | 已索引 | python | **未產出** | （無） | 70377/1769 | 0.0160 | 0.0186 |
| `excel-date-to-text` | 已索引 | python | 符合 | data_date_text.csv, data_backup.csv | 101254/4975 | 0.0389 | 0.0914 |
| `excel-filter` | 已索引 | python | 符合 | filtered_data.csv | 37336/92 | 0.0228 | 0.0317 |
| `excel-mapping-replace` | 已索引 | python | 符合 | data.csv | 37156/748 | 0.0317 | 0.0580 |
| `excel-merge` | 已索引 | python | 符合 | **merged.xlsx** | 20435/394 | 0.0337 | 0.1348 |
| `excel-regex-clean` | 已索引 | python | 符合 | processing_report.txt, data_cleaned.csv | 35751/414 | 0.0441 | 0.0661 |
| `excel-scout` | 已索引 | python | **未產出** | （無） | 18904/424 | 0.0408 | 0.0416 |
| `excel-validate` | 已索引 | python | 符合 | data_quality_report.md | 93988/1648 | 0.0519 | 0.1106 |
| `full-review` | 已索引 | none | 符合 | full_review_report.md | 80084/10469 | 0.0704 | 0.0878 |
| `pdf` | 已索引 | python | 符合 | **merged.pdf** | 216081/25273 | 0.1951 | 0.1378 |
| `pii-flag` | 已索引 | python | 符合 | pii_summary.json, pii_report.jsonl | 89505/3634 | 0.0219 | 0.0901 |
| `shorten` | 已索引 | none | 符合 | shortened_email.md | 94626/3332 | 0.0239 | 0.0224 |
| `sokrati` | 已索引 | none | 符合 | edit_summary_ru.md, rewritten_email_ru.md | 159438/5000 | 0.0402 | 0.0478 |
| `standardise-country-names` | 已索引 | python | 符合 | country_standardisation_report.md, data_standardised.csv | 143339/8764 | 0.0509 | 0.0708 |
| `unicode-consistency` | 已索引 | python | 符合 | unicode_report.md | 150960/5290 | 0.0474 | 0.0925 |

**36/36 Run 全部 `succeeded`，符合 34、未產出 2。** 沒有任何 Run 失敗，沒有任何預算拒絕，沒有任何 token 上限中止。

`add-iso3166` 的 `2026.08-1` 成本欄是「—」，因為那一輪它沒有產生 `usage` 事件（§7.2 #4 記錄的 TRACE-004 缺口）。本輪有值——trace 批的修正生效了。

兩筆「未產出」都是**同一種行為，不是壞掉**：Agent 判斷任務有歧義而先回問或先回報勘察結果，沒有寫檔。`date-wrangling` 兩輪都如此（問 `05/02/2024` 是 DD/MM 還是 MM/DD）；`excel-scout` 本輪明說「本次僅完成勘察，未寫入 `/out/artifacts/`」，上一輪則有寫。這是單輪基準模板遇上刻意含糊的 Dataset 的必然結果，判定它「做到了沒有」屬 EVAL-001。

### 13.3 transpiled → native：33 筆全數轉換，但要分清「規則」與「證據」

| | `2026.08-1` | `2026.08-2` |
| --- | --- | --- |
| `runtime = native` | 12 | **45** |
| `runtime = transpiled` | 33 | **0** |

**33 筆全部由 `transpiled` 轉為 `native`。** 但 `native` 是 0022 的規則判定（「套件宣告的 runtime 都由這個映像提供」），本批同時取得了行為證據，兩者要分開講：

- **證據面**：本批 25 個 Python 依賴 Skill 中，**23 個在 trace 裡有實際的 `script_log`**（即容器內真的執行了腳本），且**全批 36 條 `script_log` 中 `python3: command not found` 出現 0 次**（`2026.08-1` 那輪正是靠這行字發現問題的）。
- **沒有 script_log 的 2 筆**：`date-wrangling`（先回問，什麼都沒跑）與 `excel-find-duplicates`（用 Read／Grep 直接完成，沒動到腳本）。它們的 `native` 是規則判定成立、行為未觀察到——**規則說得對，但這兩筆沒有自己的證據**。

產物層面的轉換最直觀：`brand-guidelines` 從 `.html` 變成真的 `.pptx`，`excel-format`／`excel-freeze`／`excel-merge` 產出真的 `.xlsx`，`pdf` 產出真的 `.pdf`。

**成本與 token 一起掉下來**（同樣 36 個 Skill，同樣模板）：

| | `2026.08-1` | `2026.08-2` | 變化 |
| --- | --- | --- | --- |
| Trace 成本合計 | $2.5126 | **$1.8166** | **−28%** |
| 輸入 token 合計 | 5,942,513 | **2,960,630** | **−50%** |

跑腳本比讓模型重寫一份等效邏輯便宜——輸入 token 直接砍半。這是「裝 Python」這個決定最實際的回報，也修正了 §5.2「45 個 Skill 一輪基準成本 $3–4」的估計：在新映像上是 **$2.2 左右**（本批 36 筆 $1.85 ＋ §12 的 9 筆 $0.86 之中屬新映像的部分）。

### 13.4 pandas 3.x 與依賴集：目錄批標注「未實測」的那一項，實測結果

**pandas 3.0.5 本身幾乎沒有造成問題。** 全批只觸發一則 pandas 3 的行為變更告警，且是告警不是錯誤：

| Skill | 訊息 | 判讀 |
| --- | --- | --- |
| `data-analyst` | `Pandas4Warning: For backward compatibility, 'str' dtypes are included by select_dtypes when 'object' dtype is specified. This behavior is deprecated and will be removed in a future version.` | pandas 3 的**字串型別遷移**：`str` 已是獨立 dtype，而舊腳本用 `select_dtypes(include='object')` 當作「選出所有字串欄」。目前靠向後相容仍可運作，**下一個大版本會失效**。Run 未受影響 |

**真正的摩擦不是 pandas，是映像的依賴集少了腳本實際會 import 的套件。** 映像的安裝集是「目錄 `deps` 欄位的聯集」（`lxml`、`numpy`、`openpyxl`、`pandas`、`pdfplumber`、`pypdf`、`python-dateutil`、`python-docx`、`python-pptx`），而腳本 import 的比這份策展判斷多：

| Skill | 缺少的模組 | 目錄 `deps` 宣告的是 | 結果 |
| --- | --- | --- | --- |
| `add-iso3166` | `pycountry`（且 `pip` 已依設計移除，無法補裝：`No module named pip`） | `pandas` | 改用內嵌對照表完成，**符合** |
| `standardise-country-names` | `pycountry` | `pandas` | 同上，**符合** |
| `data-cleanliness-scan` | `chardet` | `pandas` | 改用內建編碼偵測，**符合** |
| `pdf` | `reportlab`、`fitz`(PyMuPDF)、`pikepdf`、`svglib`、`cairosvg`、`fpdf`、`weasyprint`、`matplotlib`（可用者：`PIL 12.3.0`、`pypdf 6.16.1`、`pdfplumber 0.11.10`） | `pypdf`、`pdfplumber` | 用可用的子集完成合併，**符合** |

**四筆全部仍然通過**——Agent 撞到 `ModuleNotFoundError` 後改用可用的手段。但這正是要記錄的事：**「安裝目錄宣告依賴的聯集」這條規則，其輸入是 CONTENT-003 的策展判斷，而實測證明該判斷少算了。** 這不是映像的 bug，是依賴清單的覆蓋率問題，處置有二：擴充 `seed-skills.json` 的 `deps` 後重建映像，或接受這個落差並在目錄上標示。兩者都需要決策，不在本批範圍。

> **2026-08-16 已裁定並執行（本節不改寫，下為後續）**：兩條處置**都採**。逐套件靜態掃描 45 個 pin commit 套件樹後，**實際短少的是 13 個 Skill、8 個套件**——本表只列出 4 個，因為只有那 4 個在本批 Dataset 上真的走到 `ModuleNotFoundError`；其餘的沒被這組資料觸發，不代表沒有缺。`deps` 已依掃描重新推導，映像升 `2026.08-3` 納入 8 個純 Python 套件並以**准入門檻**擋下重渲染／OCR／Arrow 堆疊（逐項具名於 [infra/images/README.md](../../../infra/images/README.md)「依賴集」節）；CONTENT-003／006 補上可機械驗證的準則與掃描 finding。詳見 `03` 的 SBX-002、CONTENT-003、CONTENT-006 與新增的 CONTENT-010～012。

沙箱**沒有網路**（SBX-007），`pip` 也已隨 npm 一起移除，所以執行期補裝不存在——這是刻意的，也代表依賴集只能在建映像時決定。

### 13.5 相容軸回填

```
psql -v image=skillhub/runtime-agent-sdk:2026.08-2 -v python_runtime=native \
     -v since='2026-08-16 10:00:00+00' -f tools/content/backfill-agent-compatibility.sql
```

**寫入 45 列**（本批 36 ＋ §12 的 9，同一個映像故同一個時間窗即可涵蓋），全部 `capability=activated`／`runtime=native`。`2026.08-1` 的 45 列回填前後**全表 md5 相同**（`c97409ae…`），未被觸碰。

現況：

| Runtime Image | capability | runtime | 列數 |
| --- | --- | --- | --- |
| `2026.08-1` | activated | native | 12 |
| `2026.08-1` | activated | transpiled | 33 |
| **`2026.08-2`** | **activated** | **native** | **45** |

0022 的讀取路徑取「該版本最新一次量測」，因此目錄現在對全部 45 筆顯示 `2026.08-2` 的結論，並附上映像標籤。

### 13.6 花費與合併統計

| 項目 | 數值 |
| --- | --- |
| 本批 trace 回報成本 | $1.8166 |
| **本批閘道實付** | **$1.8519**（總上限 $6 未觸及） |
| 三批累計閘道實付 | **$6.10** |
| 預算拒絕次數 | **0** |
| 收到授權受限 422 的筆數 | **0**（封鎖尚未合併；`pdf` 正常量測） |

**全 45 筆「最新量測」（全部在 `2026.08-2` 上）合併統計**：

| 判定 | 精選（15） | 已索引（30） | 合計（45） |
| --- | --- | --- | --- |
| **符合** | **15（100%）** | 27 | **42（93.3%）** |
| Run 成功但未產出 | 0 | 2（`date-wrangling`、`excel-scout`） | 2 |
| 失敗 | 0 | 1（`pptx`，PDM-005 5.2a 輸入 token 上限，§12.3） | 1 |

**「平台中止」類在新映像上維持歸零**，且精選 15/15 在**兩個映像上都是「符合」**——CONTENT-008 的允收結論不因換映像而改變，這次是有量測支撐的說法，不是推論。

> **2026-08-16 後續**：§13.4 指出的依賴集缺口已由 `b2180b2` 修正（14 個 Skill 的 `deps` 重新推導、映像升 `2026.08-3`），而修正本身**追溯推翻了本節的 `native` 判定**——當時的「宣告 ⊆ 提供」是拿抄漏的宣告算出來的。第四批重測見 **§14**。

---

## 14. CONTENT-010：deps 修正後於 `2026.08-3` 全量重測（2026-08-16）

### 14.1 為什麼要再測一次

§13.3 把 45 筆判為 `native`，依據是 0022 的規則「套件宣告的 runtime 都由映像提供」。`b2180b2` 之後這個依據不成立了：靜態掃描 45 個 pin 住的套件樹，發現 **`deps` 欄位手抄漏了 8 個套件、涉及 14 個 Skill**（§13.4 只看到 4 個，因為只有那 4 個在該批資料集上真的撞到 `ModuleNotFoundError`）。宣告變大了，「宣告 ⊆ 提供」就得重算——**§13 的 `native` 是拿錯的宣告算出來的，必須以新映像上的新量測取代**。

`2026.08-3` 補進 8 個套件：`pycountry`、`chardet`、`defusedxml`、`ftfy`、`confusable-homoglyphs`、`pytz`、`phonenumbers`、`python-stdnum`。條件其餘與 §3 相同（同模板、同 Dataset、per-Run `max_budget=$0.50`、併發 2），總上限 $4。

### 14.2 授權受限：4 筆如實跳過，未繞過

`0023_access_restriction` 的 licensing hold 已生效，4 個 anthropic-sa Skill（`docx`、`pdf`、`pptx`、`xlsx`）的 `access_restriction = 'license-review'`。實測封鎖確實擋在 Run 建立這一步：

```
POST /skills/{id}/runs → 422
this skill cannot be run while its source license is under review (license-review)
```

Fork、Test Case、Dataset 上傳、Preflight 都照常成立（0023 只關 `GET /files` 與 `POST /runs`），**只有 Run 被拒**。四筆一律記為 **`restricted`，未重測**，沒有任何繞過嘗試；它們在 `2026.08-3` 上**沒有相容列**，目錄對這四筆顯示的仍是 `2026.08-2` 的量測（§14.5）。

**代價要講明**：`defusedxml` 只有 `docx`／`pptx`／`xlsx` 三個 Skill 宣告，而這三個正好全在封鎖名單內——**「office 三件套的 validator 在 defusedxml 到位後是否真的執行」這個問題，在授權判定解除前無法用 Run 回答。** 能回答的只有映像層，見 §14.4。

### 14.3 逐筆結果（image `2026.08-3`，41 筆量測 ＋ 4 筆 restricted）

「deps 修正」欄標示該筆是否屬 `b2180b2` 更動的 14 個 Skill。最後一欄是同一 Skill 在 `2026.08-2` 的成本。

| Skill | deps 修正 | 層級 | 判定 | Artifact | 08-3 成本 | （08-2 成本） |
| --- | --- | --- | --- | --- | --- | --- |
| `data-cleanliness-scan` | **是** | 精選 | 符合 | cleanliness_report.json, cleanliness_report.md | 0.0573 | 0.0592 |
| `add-data-dictionary` | **是** | 已索引 | 符合 | data_dictionary.md | 0.0360 | 0.0341 |
| `add-iso3166` | **是** | 已索引 | 符合 | data_iso3166.csv | 0.0326 | 0.0413 |
| `data-comparability` | **是** | 已索引 | 符合 | comparability_plan.md | 0.0752 | 0.0975 |
| `date-wrangling` | **是** | 已索引 | **未產出** | （無） | 0.0081 | 0.0160 |
| `document-format-skills` | **是** | 已索引 | 符合 | **q2_update_official.docx** | 0.0775 | 0.0548 |
| `docx` | **是** | 已索引 | **restricted** | （未跑） | — | 0.1476 |
| `excel-validate` | **是** | 已索引 | 符合 | data_quality_report.md | 0.0352 | 0.0519 |
| `pdf` | **是** | 已索引 | **restricted** | （未跑） | — | 0.1951 |
| `pii-flag` | **是** | 已索引 | 符合 | pii_summary.md, pii_summary.json, pii_report.jsonl | 0.0774 | 0.0219 |
| `pptx` | **是** | 已索引 | **restricted** | （未跑） | — | 0.3019 |
| `standardise-country-names` | **是** | 已索引 | 符合 | data_standardised_report.md, data_standardised.csv | 0.0447 | 0.0509 |
| `unicode-consistency` | **是** | 已索引 | 符合 | unicode_report.md | 0.0327 | 0.0474 |
| `xlsx` | **是** | 已索引 | **restricted** | （未跑） | — | 0.1143 |
| `ai-written-check` | — | 精選 | 符合 | ai_written_check_report.md | 0.0347 | 0.0199 |
| `brand-guidelines` | — | 精選 | 符合 | anthropic_brand_update.pptx | 0.1034 | 0.1576 |
| `csv-to-json` | — | 精選 | 符合 | data.json | 0.0244 | 0.0275 |
| `data-analyst` | — | 精選 | 符合 | standardized_data.csv | 0.0409 | 0.0397 |
| `excel-deduplicate` | — | 精選 | 符合 | data_deduplicated.xlsx, data_deduplicated.csv | 0.0223 | 0.0246 |
| `excel-find-duplicates` | — | 精選 | 符合 | duplicate_rows_report.json | 0.0612 | 0.0466 |
| `excel-format` | — | 精選 | 符合 | data_formatted.xlsx | 0.0351 | 0.0570 |
| `excel-freeze` | — | 精選 | 符合 | frozen_header.xlsx | 0.0187 | 0.0249 |
| `excel-insert` | — | 精選 | 符合 | data_with_formatted_date.xlsx | 0.0417 | 0.0386 |
| `handoff` | — | 精選 | 符合 | handoff-q2-update.md | 0.0341 | 0.0209 |
| `humanizer` | — | 精選 | 符合 | q2_update_humanized.md | 0.0525 | 0.0274 |
| `internal-comms` | — | 精選 | 符合 | 3p-update.md | 0.0298 | 0.0280 |
| `line-edit` | — | 精選 | 符合 | polished_email.md | 0.0248 | 0.0251 |
| `text-to-numeric` | — | 精選 | 符合 | data_numeric_report.md, data_numeric.csv | 0.0455 | 0.0461 |
| `copyright-creative-work` | — | 已索引 | 符合 | song-registration-intake-template.md, song-registration-prep.md | 0.0540 | 0.0513 |
| `course-quiz-builder` | — | 已索引 | 符合 | questions.json, quiz.html | 0.1758 | 0.1648 |
| `cringe-check` | — | 已索引 | 符合 | cringe_check_report.md | 0.0258 | 0.0468 |
| `data-shape` | — | 已索引 | 符合 | column_mapping.md, schema.sql, schema_proposal.md | 0.0959 | 0.0591 |
| `excel-date-to-text` | — | 已索引 | 符合 | data_date_text.csv | 0.0432 | 0.0389 |
| `excel-delete` | — | 已索引 | 符合 | data_rows_5_to_10_deleted.csv | 0.0356 | 0.0634 |
| `excel-filter` | — | 已索引 | 符合 | filtered_data.xlsx, filtered_data.csv | 0.0468 | 0.0228 |
| `excel-mapping-replace` | — | 已索引 | 符合 | sales_country_mapped.csv | 0.0424 | 0.0317 |
| `excel-merge` | — | 已索引 | 符合 | merged.xlsx | 0.0220 | 0.0337 |
| `excel-regex-clean` | — | 已索引 | 符合 | notice.txt | 0.0450 | 0.0441 |
| `excel-scout` | — | 已索引 | **未產出** | （無） | 0.0249 | 0.0408 |
| `excel-sort` | — | 已索引 | 符合 | sorted_data.csv | 0.0567 | 0.0309 |
| `excel-split` | — | 已索引 | 符合 | split/ 下 6 個 **.xlsx**（逐 customer 一檔） | 0.0263 | 0.0293 |
| `full-review` | — | 已索引 | 符合 | full_review_report.md | 0.0619 | 0.0704 |
| `json-restructure` | — | 已索引 | 符合 | orders_by_country.json, orders_by_country_data_dictionary.md | 0.0287 | 0.0540 |
| `shorten` | — | 已索引 | 符合 | shortened_email.md | 0.0235 | 0.0239 |
| `sokrati` | — | 已索引 | 符合 | rewrite_ru.md, changes_ru.md | 0.0722 | 0.0402 |

**41 筆量測：符合 39、未產出 2（`date-wrangling`、`excel-scout`，與 §13 同兩筆、同一種「先回問／先勘察」行為）。零失敗、零預算拒絕、零 token 上限中止。**

### 14.4 defusedxml 與 validator：能證到哪裡，證不到哪裡

**Run 層面：證不到。** `defusedxml` 在整個目錄裡只有 `docx`／`pptx`／`xlsx` 宣告，三者全部 `restricted`；41 筆量測的 trace 中 `defusedxml` 出現 **0 次**。**office 三件套的 validator 在本批完全沒有被執行過**，這不是它壞了，是那三個 Skill 根本不允許起 Run。

**映像層面：證得到，而且是有效的。** 直接對映像做探針（不涉及任何受限 Skill 的內容）：

| 探針 | 結果 |
| --- | --- |
| `import defusedxml` | **0.7.1** |
| `import docx / pptx / openpyxl` | python-docx 1.2.0、python-pptx 1.0.2、openpyxl 3.1.5 |
| 對 `defusedxml.ElementTree` 餵一顆實體展開炸彈（`<!ENTITY a 'x'><!ENTITY b '&a;&a;'>`） | **`EntitiesForbidden`**——擋下 |

所以能說的是：**`2026.08-3` 確實提供了一個會動、且真的會拒絕實體展開的 defusedxml；validator 一旦可以跑，它所依賴的東西是就緒的。** 不能說的是「validator 跑起來會通過」——那要等授權判定解除後補一次 Run 才知道。這一條列為後續。

**間接旁證**：`document-format-skills`（宣告 `python-docx`，未受限）本批產出真正的 `q2_update_official.docx`，`excel-split` 產出 6 個真正的 `.xlsx`——OOXML 寫入路徑在這個映像上是通的。

### 14.5 deps 修正的實測效果

**`ModuleNotFoundError` 從 4 筆歸零。** §13.4 記錄的四個缺件（`pycountry`、`chardet`、`reportlab` 等）在本批**一次都沒有出現**——全批 script_log 中 `ModuleNotFoundError` 出現 **0 次**（`2026.08-2` 那批是 4 個 Skill）。

新裝套件確實被用上了（trace 內出現該模組名的 Run）：

| 新裝套件 | 實際使用的 Skill |
| --- | --- |
| `pycountry` | `add-iso3166`、`standardise-country-names` |
| `chardet` | `data-cleanliness-scan`、`unicode-consistency` |
| `phonenumbers`、`python-stdnum` | `pii-flag` |
| `defusedxml`、`ftfy`、`confusable-homoglyphs`、`pytz` | **未被使用**（宣告者受限，或該路徑未被本模板觸發） |

**刻意不裝的那些，本批沒有被觸發，因此也沒有推翻 [infra/images/README.md](../../../infra/images/README.md) 的預測**：`pyarrow`（Parquet）、`presidio-analyzer`／`presidio-anonymizer`（PII 引擎）、`pywin32`（Windows COM）、`pytesseract`／`pdf2image`／`reportlab` 仍不在映像內，實測確認 `import` 全部失敗。`pii-flag` 的 trace 裡 `presidio` 只出現在它讀自己 `SKILL.md` 的內容中，**沒有嘗試載入**——它改用有裝的 `phonenumbers`／`python-stdnum` 完成，正是 README 預測的降級路徑。**本批的 Dataset 是 CSV／Markdown，不含 Parquet、掃描件 PDF 或 `.doc`，所以那些缺口本來就不會被碰到——「沒出事」不等於「沒問題」，這點必須明說。**

### 14.6 與 `2026.08-2` 的對照

41 筆可比量測（排除 4 筆 restricted）：

| | `2026.08-2` | `2026.08-3` | 變化 |
| --- | --- | --- | --- |
| 判定改變的筆數 | — | **0** | 41 筆判定與上一批**完全一致** |
| Trace 成本合計 | $1.9051 | $1.9264 | +1% |
| 輸入 token 合計 | 3,415,183 | 2,880,314 | **−16%** |
| `ModuleNotFoundError` 筆數 | 4 | **0** | 歸零 |

**判定沒有一筆改變，token 少了 16%，成本持平。** 這是合理的：補進來的套件消除的是「撞牆後繞路」的來回，而繞路本來就繞得成功——所以省的是 token，不是把失敗變成成功。**deps 修正的價值在於讓 `native` 這個判定重新為真，而不是讓更多 Skill 通過。**

產物型別也更貼近 Skill 的本業：`excel-deduplicate`、`excel-filter`、`excel-insert`、`excel-split` 這一輪都產出了 `.xlsx`（上一輪部分只給 `.csv`）。

### 14.7 相容軸回填

```
psql -v image=skillhub/runtime-agent-sdk:2026.08-3 -v python_runtime=native \
     -v since='2026-08-16 13:00:00+00' -f tools/content/backfill-agent-compatibility.sql
```

**寫入 41 列**，全部 `capability=activated`／`runtime=native`。`2026.08-1` 與 `2026.08-2` 的既有 90 列回填前後**全表 md5 相同**（`3e7e4010…`），未被觸碰。

| Runtime Image | capability | runtime | 列數 |
| --- | --- | --- | --- |
| `2026.08-1` | activated | native | 12 |
| `2026.08-1` | activated | transpiled | 33 |
| `2026.08-2` | activated | native | 45 |
| **`2026.08-3`** | **activated** | **native** | **41** |

**`docx`／`pdf`／`pptx`／`xlsx` 在 `2026.08-3` 上沒有列**（restricted，無 Run 可依據）。0022 的讀取路徑取「最新一次量測」，所以目錄對這四筆顯示的仍是 **`2026.08-2` 的舊值**，且會連映像標籤一起顯示——讀者看得出那是舊映像上的結論。這四筆的 `native` 同樣是拿修正前的宣告算出來的，**在授權判定解除、補跑一次之前，它們是本報告唯一仍帶著已知過時依據的相容列**。

### 14.8 花費與合併統計

| 項目 | 數值 |
| --- | --- |
| 本批 trace 回報成本 | $1.9264 |
| **本批閘道實付** | **$2.0038**（總上限 $4 未觸及） |
| 四批累計閘道實付 | **$8.10** |
| 預算拒絕次數 | 0 |
| 授權受限 422 筆數 | **4**（`docx`、`pdf`、`pptx`、`xlsx`） |

**全 45 筆「最新量測」合併**（41 筆在 `2026.08-3`、4 筆停在 `2026.08-2`）：

| 判定 | 精選（15） | 已索引（30） | 合計（45） |
| --- | --- | --- | --- |
| **符合** | **15（100%）** | 27 | **42（93.3%）** |
| Run 成功但未產出 | 0 | 2（`date-wrangling`、`excel-scout`） | 2 |
| 失敗 | 0 | 1（`pptx`，停在 `2026.08-2` 的 token 上限結果） | 1 |

精選 15/15 在**三個映像上都是「符合」**；CONTENT-008 的允收結論連續三次量測都成立。

### 14.9 留給後續

1. **office 三件套的 validator 仍未被執行過**——`docx`／`pptx`／`xlsx` 的授權判定解除後，需補一次 Run 才能回答「defusedxml 到位後 validator 是否通過」。映像側已就緒（§14.4）。
2. **四筆 restricted 的相容列帶著過時依據**（§14.7）——解除後應以 `2026.08-3`（或當時映像）重測並回填。
3. **刻意不裝的套件所對應的降級路徑未被本模板觸發**（§14.5）——Parquet、掃描件 OCR、`.doc` 轉檔要有結論，需要對應型別的 Dataset，而那超出「同一模板套全部」的基準設計。
