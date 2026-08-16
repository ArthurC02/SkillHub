# CONTENT-003／004 種子清單本機端到端匯入報告（M1）

- 日期：**2026-08-15**
- 對應：CONTENT-003／004／006、INGEST-001～009、SKILL-001／002、DISC-001／002、[ADR-013](../../../adr/ADR-013-intent-search-architecture.md)、[ADR-020](../../../adr/ADR-020-authentication-and-session-model.md)
- 輸入：[`tools/content/seed-skills.json`](../../../../tools/content/seed-skills.json)（45 筆、11 個 pin commit 的來源 repo）
- 工具：[`tools/content/import_seed.py`](../../../../tools/content/import_seed.py)（本次新增，驗證用）
- 對照：[curated-skill-list.md](../content/curated-skill-list.md)、[golden-query-set.md](golden-query-set.md)
- 基準：**commit `b144bea`**（`Wire index-time LLM enrichment and retune ranking from the golden set`）
- 狀態：**驗證完成，全程本機 dev 環境，資料已銷毀。**

> **本文件不定案、不修改既有文件。** 未動 `adr/`、`services/`、`apps/`、`db/`、`.github/`、`plans/mvp/02`、`plans/mvp/03`，亦未變更任何工作項目勾選狀態。第 6 節列出的平台缺陷**只記錄不修**。第 5 節的搜尋數字是 smoke 級抽樣，**不是** PDM-011 正式評測，不得用來取代 [`tools/goldenset/`](../../../../tools/goldenset/) 的量測。

> **關於基準版本。** 本次驗證期間 `b144bea` 落地，它同時改動匯入路徑（索引時 LLM 增強）與排序（改以向量距離為主）。整套流程已在該 commit 上**重跑一次**，本文的所有數字以重跑結果為準。前一版基準 `6690736` 的量測保留在 §5.1 作為對照，因為它記錄了「索引時增強尚未接線」時的搜尋行為，是理解 §5.2 改善幅度的必要基線。**兩個版本的匯入結果（狀態與 findings）逐筆完全相同**——`skillpkg` 驗證器與 `ingest.packageFS` 均未被該 commit 改動，因此第 3、4 節的統計對兩個版本同時成立。

---

## 1. 環境搭建結果

四層全部起得來，無須降級。

| 層 | 作法 | 結果 |
| --- | --- | --- |
| Postgres | `infra/compose/docker-compose.yml`，`pgvector/pgvector:pg17` | ✅ healthy |
| 物件儲存 | 同 compose，`chrislusf/seaweedfs:3.80`（匿名存取，dev 專用） | ✅ healthy，`EnsureBucket` 通過 |
| Schema | `db/migrations/0001`～`0011` 逐檔 `psql -v ON_ERROR_STOP=1` | ✅ 11 檔全數套用，僅 0009 有 ivfflat 低資料量 NOTICE |
| platform API | `docker run golang:1.25 go run ./cmd/api` | ✅ `/healthz` 200 |
| llm 服務 | `uv run uvicorn skillhub_llm.app:app --port 8000` | ✅ `/healthz`、`/embed`、`/match-reasons`、`/v1/enrich-skill` 皆有回應 |

platform API 的環境變數（依 `services/platform/cmd/api/main.go` 實讀變數名）：

```
DATABASE_URL=postgres://skillhub:skillhub@postgres:5432/skillhub
OBJSTORE_ENDPOINT=seaweedfs:8333        OBJSTORE_BUCKET=skillhub
DEV_LOGIN=1                             COOKIE_INSECURE=1
API_ADDR=:8080                          LLM_SERVICE_URL=http://host.docker.internal:8000
```

- 容器接 compose 網路 `skillhub_default`，`-p 8080:8080` 對外，`--add-host host.docker.internal:host-gateway` 讓 Go 打得到跑在 host 上的 llm 服務。
- **GitHub OAuth 憑證未設定**，改走 ADR-020 的 dev provider：`DEV_LOGIN=1` 才會掛載 `POST /auth/dev/login`，`COOKIE_INSECURE=1` 讓 session cookie 在 plain-http 下可用。匯入身分為 `seed-importer`，其個人 Workspace 即匯入目的地。

### 1.1 模型出口：本次刻意違反鐵律 8，僅限本機

llm 服務的 `/embed` 走 `litellm.aembedding(api_base=LITELLM_BASE_URL)`。本次**未**架設 LiteLLM 閘道（`spikes/pdm-003-litellm-gateway/`），而是把 `LITELLM_BASE_URL` 指向 `https://api.openai.com/v1`、`LITELLM_API_KEY` 帶入 `.env` 的 `OPENAI_API_KEY`，直連供應商。

> **這不是可接受的部署形態。** 鐵律 8（ADR-017）要求所有模型呼叫走 LiteLLM 閘道、供應商金鑰只存在閘道。本次是為了在單機取得真 Embedding 做的臨時取徑，金鑰只以環境變數進入 uvicorn 程序、未寫入任何檔案，驗證結束即隨程序消滅。正式環境必須以閘道 + 每 Run 短效 Virtual Key 取代。

`/v1/enrich-skill`（`ENRICH_MODEL` 預設 `gpt-5.6-sol`）在同一條臨時取徑下實測可回傳合格的 zh-Hant 摘要與雙語 task_examples，但**目前沒有任何 Go 端呼叫它**（見 §6.2）。

---

## 2. 匯入方法：為什麼走 INGEST-002 而不是 INGEST-001

先讀 `internal/ingest/fetch.go` 與 `service.go` 確認實作，再實打三個 URL 探針（`import_seed.py --probe-url`）：

| 探針 | 回應 |
| --- | --- |
| `raw.githubusercontent.com/.../SKILL.md` | `400 fetch failed: host "raw.githubusercontent.com" is not on the allowed source list` |
| `github.com/YuYY2004/excel-skills`（repo 根） | `422 skill-md-missing` |
| `codeload.github.com/.../zip/<pinned commit>` | `422 skill-md-missing` |

三點結論：

1. **allow-list 位置**：`ingest.DefaultAllowedHosts()`（`services/platform/internal/ingest/fetch.go:31`），清單只有 `github.com`、`codeload.github.com`、`objects.githubusercontent.com`。`raw.githubusercontent.com` 不在其中；可由 `IMPORT_EXTRA_HOSTS` 補，但**本次不改 Go 也不加白名單**。
2. **URL 匯入無法表達 pin**：`github.com/owner/repo` 會被 `candidates()` 展開成 `refs/heads/main` → `master` 探測，種子檔的 commit SHA 根本傳不進去；`INGEST-004` 記到的 `source_ref` 會是分支名而非 pin。
3. **monorepo 形狀不相容**：`packageFS()` 只接受 SKILL.md 位於壓縮檔根、或單一頂層目錄之下。種子清單的 `skill_md_path` 形如 `claude/skills/<name>/SKILL.md`，一個 repo zip 最多只能匯入一個 Skill。

因此改走 **INGEST-002 套件上傳**：`import_seed.py` 每個來源只抓一次 `codeload.github.com/<owner>/<repo>/zip/<pinned commit>`（快取於暫存目錄），再把每個 Skill 目錄重新打包成 SKILL.md 位於根的獨立 zip，POST 到 `/skills/import/upload`。

**打包必須位元決定性。** 首次執行時 `zipfile.writestr` 會把當下時間寫進每個 entry，同一 commit 兩次打包產生不同 sha256，於是 INGEST-005 的內容去重完全失效——重跑一次就替每個 Skill 憑空長出第二個 Version。工具已把 entry 時間戳固定為 `1980-01-01` 並排序檔名；`--selftest` 有對應斷言。修正後在乾淨 DB 上連跑兩輪，第二輪 **44 筆全部回報 `duplicate`、Version 數維持 44**，INGEST-005 行為確認正確。

---

## 3. 匯入結果

**成功 44／45，規格驗證失敗 1／45，靜態掃描零阻擋。**

DB 落地：`skills=44`、`skill_versions=44`、`search_documents=44`。skills 少 1 是因為被拒的 `sokrati/sokrati` 未建立任何列。

索引時增強（`b144bea` 起接線）在 44 筆上全數成功：`enrichment_status='enriched'` 44／44，`embedding` 非空 44／44，`task_examples` 非空 44／44，`enrichment_model=gpt-5.6-sol`、`enrichment_prompt_version=enrich-skill/v1`。事後再跑 `cmd/reindex` 回報 `enriched=0 still_pending=0`，代表匯入當下已無遺留，backfill 冪等。

### 3.1 逐類別統計

| 類別 | 筆數 | 成功 | 被拒 | license-unknown 警告 | 有 script-file 揭露 | 平均 external-url 揭露／筆 |
| --- | --- | --- | --- | --- | --- | --- |
| data | 25 | 25 | 0 | 25 | 1 | 0.3 |
| documents | 10 | 10 | 0 | 5 | 6 | 72.1 |
| writing | 10 | 9 | 1 | 7 | 1 | 1.9 |
| **合計** | **45** | **44** | **1** | **37** | **8** | **18.1** |

依 tier：`curated` 14／14 全數成功；`indexed` 30／31（被拒的是 indexed）。

### 3.2 逐來源統計

| 來源 | 筆數 | 成功 | 被拒 | license-unknown | external-url 總數 |
| --- | --- | --- | --- | --- | --- |
| anthropic | 2 | 2 | 0 | 0 | 4 |
| anthropic-sa | 4 | 4 | 0 | 0 | 690 |
| handoff | 1 | 1 | 0 | 1 | 1 |
| humanizer | 1 | 1 | 0 | 0 | 11 |
| kagura-docfmt | 1 | 1 | 0 | 1 | 18 |
| neon-jetpack | 5 | 5 | 0 | 5 | 0 |
| nqumich | 1 | 1 | 0 | 1 | 0 |
| quiz-builder | 1 | 1 | 0 | 0 | 12 |
| sokrati | 2 | 1 | 1 | 2 | 4 |
| wrangler | 12 | 12 | 0 | 12 | 0 |
| yuyy-excel | 15 | 15 | 0 | 15 | 7 |

> **anthropic-sa 的 4 筆（docx／pdf／pptx／xlsx）在種子檔標記 `redistributable: false`，法務判定未完成。** 本次為驗證仍予匯入，資料只存在於本機 dev DB 且已隨容器銷毀。正式目錄在 CONTENT-004 §5.2 結案前**不得**保存這 4 筆的內容快照，也不得產生 Download Artifact。

### 3.3 逐筆結果

| Skill ID | 類別 | tier | 結果 | 阻擋錯誤 | 警告 | 資訊揭露 |
| --- | --- | --- | --- | --- | --- | --- |
| nqumich/data-analyst | data | curated | 成功 | — | license-unknown | script-file×1 |
| wrangler/add-data-dictionary | data | indexed | 成功 | — | license-unknown | — |
| wrangler/add-iso3166 | data | indexed | 成功 | — | license-unknown | — |
| wrangler/csv-to-json | data | curated | 成功 | — | license-unknown | — |
| wrangler/data-cleanliness-scan | data | curated | 成功 | — | license-unknown | — |
| wrangler/data-comparability | data | indexed | 成功 | — | license-unknown | — |
| wrangler/data-shape | data | indexed | 成功 | — | license-unknown | — |
| wrangler/date-wrangling | data | indexed | 成功 | — | license-unknown | — |
| wrangler/json-restructure | data | indexed | 成功 | — | license-unknown | — |
| wrangler/pii-flag | data | indexed | 成功 | — | license-unknown | — |
| wrangler/standardise-country-names | data | indexed | 成功 | — | license-unknown | — |
| wrangler/text-to-numeric | data | curated | 成功 | — | license-unknown | — |
| wrangler/unicode-consistency | data | indexed | 成功 | — | license-unknown | — |
| yuyy-excel/excel-date-to-text | data | indexed | 成功 | — | license-unknown | — |
| yuyy-excel/excel-deduplicate | data | curated | 成功 | — | license-unknown | external-url×1 |
| yuyy-excel/excel-delete | data | indexed | 成功 | — | license-unknown | external-url×1 |
| yuyy-excel/excel-filter | data | indexed | 成功 | — | license-unknown | external-url×1 |
| yuyy-excel/excel-find-duplicates | data | curated | 成功 | — | license-unknown | — |
| yuyy-excel/excel-mapping-replace | data | indexed | 成功 | — | license-unknown | external-url×1 |
| yuyy-excel/excel-merge | data | indexed | 成功 | — | license-unknown | external-url×1 |
| yuyy-excel/excel-regex-clean | data | indexed | 成功 | — | license-unknown | external-url×1 |
| yuyy-excel/excel-scout | data | indexed | 成功 | — | license-unknown | — |
| yuyy-excel/excel-sort | data | indexed | 成功 | — | license-unknown | — |
| yuyy-excel/excel-split | data | indexed | 成功 | — | license-unknown | external-url×1 |
| yuyy-excel/excel-validate | data | indexed | 成功 | — | license-unknown | — |
| anthropic-sa/docx | documents | indexed | 成功 | — | — | external-url×321;script-file×15 |
| anthropic-sa/pdf | documents | indexed | 成功 | — | — | external-url×2;script-file×8 |
| anthropic-sa/pptx | documents | indexed | 成功 | — | — | external-url×186;script-file×15 |
| anthropic-sa/xlsx | documents | indexed | 成功 | — | — | external-url×181;script-file×12 |
| handoff/handoff | documents | curated | 成功 | — | license-unknown | external-url×1 |
| kagura-docfmt/document-format-skills | documents | indexed | 成功 | — | license-unknown | dependency-file×1;external-url×18;script-file×11 |
| quiz-builder/course-quiz-builder | documents | indexed | 成功 | — | — | external-url×12;script-file×5 |
| yuyy-excel/excel-format | documents | indexed | 成功 | — | license-unknown | — |
| yuyy-excel/excel-freeze | documents | curated | 成功 | — | license-unknown | — |
| yuyy-excel/excel-insert | documents | curated | 成功 | — | license-unknown | — |
| anthropic/brand-guidelines | writing | curated | 成功 | — | — | external-url×2 |
| anthropic/internal-comms | writing | curated | 成功 | — | — | external-url×2 |
| humanizer/humanizer | writing | curated | 成功 | — | — | external-url×11;script-file×1 |
| neon-jetpack/ai-written-check | writing | curated | 成功 | — | license-unknown | — |
| neon-jetpack/copyright-creative-work | writing | indexed | 成功 | — | license-unknown | — |
| neon-jetpack/cringe-check | writing | indexed | 成功 | — | license-unknown | — |
| neon-jetpack/full-review | writing | indexed | 成功 | — | license-unknown | — |
| neon-jetpack/line-edit | writing | curated | 成功 | — | license-unknown | — |
| sokrati/shorten | writing | indexed | 成功 | — | license-unknown | external-url×2 |
| sokrati/sokrati | writing | indexed | **規格驗證失敗** | description-too-long | license-unknown | external-url×2 |

### 3.4 唯一的規格驗證失敗

```
POST /skills/import/upload → 422
{"errors":[{"severity":"error","code":"description-too-long","path":"SKILL.md",
            "message":"description exceeds 1024 characters"}],
 "warnings":[{"code":"license-unknown",...}],"infos":[...]}
```

`sokrati/sokrati` 的 frontmatter description 是 **777 個字元、1432 個 UTF-8 位元組**的俄文段落。驗證器擋得對——這個 description 遠超「一句話說明用途」的合理長度，正是規格該擋的東西；但**擋的理由算錯了**（見 §6.1 bug 1）。

---

## 4. 常見失敗模式（CONTENT-006 輸入）

### Top-1：License 在 frontmatter 缺席——37／45（82%）

`license-unknown` 是本次唯一出現的警告種類，45 筆裡 37 筆中招。缺的是 **SKILL.md frontmatter 的 `license` 欄位**，不是 repo 沒有 LICENSE 檔：CONTENT-004 已逐一實查 11 個 repo 的 LICENSE 檔，`wrangler`、`yuyy-excel`、`neon-jetpack` 都是明確的 MIT。

只有 8 筆有宣告：anthropic 系 6 筆（自帶 per-skill LICENSE.txt）、`humanizer`、`course-quiz-builder`。

意涵：DISC-003「未知授權必須揭露、不得預設寬鬆」在套件層面是對的，但**套件內的宣告與 repo 層的授權事實脫節**。CONTENT-006 需要決定：目錄要顯示的是套件自宣告（37 筆會顯示「未知」，與 curated-skill-list.md §5 的合規表直接矛盾），還是策展階段登錄的 repo 層事實。目前 `skill_versions.license_expression` 只吃 manifest 的值，等於把 82% 的正確授權資訊丟掉。

### Top-2：靜態揭露看得見檔案、看不見 SKILL.md 內嵌的程式與相依——33／45

`INGEST-007` 的揭露完全以副檔名與檔名為準（`skillpkg.scanTree`）。實測落差：

- **相依**：45 筆裡 33 筆在種子檔登記了執行期相依（`openpyxl`、`pandas`、`lxml`…），但只有 **1 筆**（`kagura-docfmt`）觸發 `dependency-file`。其餘 32 筆把相依寫在 SKILL.md 的散文或程式碼區塊裡，套件內沒有 `requirements.txt`。
- **程式**：`yuyy-excel` 有 5 筆種子檔登記 `script_lines`（每筆約 180 行內嵌 Python），套件裡卻只有一個 SKILL.md，`script-file` 一次都沒觸發。整份清單只有 8 筆出現 `script-file`。

意涵：使用者看到「本套件不含腳本、未宣告外部相依」時，實際可能正要跑 180 行內嵌 Python 並要求 `openpyxl`。這不是誤判而是掃描面不足；CONTENT-006 的靜態檢查若只採信 INGEST-007 的輸出，會系統性低估 data 類別的執行風險。修補方向是掃 SKILL.md 的 fenced code block（語言標記 + `import`／`pip install` 行），但那是 INGEST-007 的範圍變更，需先補需求 ID。

### Top-3：資訊揭露被 external-url 淹沒——總計 816 條，其中 747 條是 URL

`anthropic-sa/docx` 單筆就吐出 **321 條** `external-url`，pptx 186、xlsx 181，四筆合計 690 條、佔全部 URL 揭露的 92%。這些多半是參考文件裡的 OOXML 規格連結、schema namespace URI 之類，對「這個 Skill 會連去哪裡」毫無資訊量。

意涵：`INGEST-008` 要求錯誤／警告／資訊分開呈現，本次確認格式正確；但 info 這一層在真實語料下的訊噪比使它無法直接當 UI。CONTENT-006 若要人工複查揭露內容，需要先聚合（依 host 去重、排除 schema／namespace URI），否則單筆 321 行的清單沒有人會讀。

### 補充：不算失敗模式的兩點

- **package 體積**：最小 1.2 KB、中位數 4.3 KB、最大 160 KB（`anthropic-sa/docx`），離 `MaxZipBytes` 32 MiB 極遠，PDM-005 的實際上限有很大餘裕。
- **名稱衝突**：44 筆匯入後 44 個 skills 列，manifest `name` 在單一 Workspace 內無碰撞。

---

## 5. 搜尋 smoke（非正式評測）

流程：先跑 `cmd/reindex`（INGEST-009 全量重建 + 增強 backfill），再把匯入 Workspace 的 `is_catalog` 設為 true（0010 遷移要求公開性必須明確授予，dev DB 直接 UPDATE），然後從 [`tools/goldenset/queries.json`](../../../../tools/goldenset/queries.json) 依「類別 × 語言」分層抽 10 條打 `GET /api/skills/search`。同一組 10 條查詢在兩個版本上各跑一次。

### 5.1 對照基線（`6690736`，索引時增強尚未接線）：10／10 零結果

10 條查詢全部回 `results: []`，且 `degraded` 全部為 `false`。兩條腿同時空轉：

- **向量腿結構性為空**：`search_documents.embedding` 44 列全是 NULL。當時的匯入路徑（`persistVersion` → `UpsertSearchDocument`）與 `cmd/reindex`（`ReindexAll`）都不寫 embedding，全 repo 沒有任何呼叫 `UpsertSearchDocumentWithEmbedding` 的地方。`PublicHybridSearchSkills` 的 `vec` CTE 有 `WHERE s.embedding IS NOT NULL`，因此永遠回 0 列。
- **FTS 腿對整句查詢無效**：`websearch_to_tsquery('english', ...)` 把整句話當成所有詞的 AND。golden query 是自然語句（例如「我要把一份會議記錄整理成有目錄和頁碼的 Word 文件」、`fill an existing Word template and gate-check the OpenXML against its schema`），沒有任何文件能同時命中全部詞。單詞查詢正常：`?q=excel duplicate rows` 立刻回 `excel-find-duplicates`。

補充實驗：以 llm 服務的 `/embed` 對 44 筆 `name + summary` 產生向量後直接 UPDATE 進 DB（一次性腳本，未進 repo），同 10 條得 hit@3 = **4／10**，但 **zh documents 類 D01～D04 全滅**——gold 答案 docx／pdf／pptx／xlsx 都在目錄裡，卻被 `yuyy-excel` 叢集整片吸走，因為那 15 筆是當時目錄裡唯一帶中文 summary 的文件。這與 golden-query-set.md §3.6 的診斷一致：缺的是索引時的雙語 task_examples。

### 5.2 現行基準（`b144bea`，索引時增強已接線）：hit@3 = 8／10

| ID | 語言 | Top-3 | gold_primary | 命中 |
| --- | --- | --- | --- | --- |
| D01 | zh | **docx**, document-format-skills, excel-format | docx | ✓（Top-1） |
| D02 | zh | **pdf**, excel-merge, excel-split | pdf | ✓（Top-1） |
| D03 | zh | **pptx**, internal-comms, cringe-check | pptx | ✓（Top-1） |
| D04 | zh | **xlsx**, data-shape, excel-format | xlsx | ✓（Top-1） |
| D05 | zh | line-edit, brand-guidelines, pptx | report-designer\* | ✗ |
| D13 | en | **docx**, xlsx, document-format-skills | minimax-docx\* | ✓（acceptable） |
| W01 | zh | **brand-guidelines**, docx, excel-format | brand-guidelines | ✓（Top-1） |
| W13 | en | humanizer, ai-written-check, shorten | humanize\* | ✗ |
| A01 | zh | excel-find-duplicates, **excel-deduplicate**, excel-delete | excel-deduplicate | ✓ |
| A13 | en | excel-format, **excel-deduplicate**, excel-delete | excel-deduplicate | ✓ |

\* gold 答案不在本次 44 筆目錄內（`report-designer`、`minimax-docx`、`humanize`／`ai-check` 屬 golden query set 的較大候選語料）。**扣掉這 3 條結構性不可能命中的查詢，實得 8／8**；`primary` Top-1 為 5／10。

結論（smoke 級，不可當評測）：

1. **§5.1 的中文文件類崩塌完全消失**：D01～D04 從全滅變成 4／4 Top-1。索引時的雙語 task_examples 是決定性因素，golden-query-set.md §3.6 的診斷得到獨立佐證。
2. **兩條未命中都是目錄缺件，不是排序問題**。D05 的 `report-designer` 與 W13 的 `humanize` 根本不在 45 筆種子清單裡；W13 回的 `humanizer` 是不同來源的近義 Skill，排序其實合理。
3. **`degraded` 全程 `false`，且與事實相符**（44 筆皆已 enriched，向量腿有完整資料）。
4. **match reason 全部標記 `model`，但內容仍是樣板句**（「docx may be relevant to your task.」）——此問題在 `b144bea` 上仍可重現（見 §6.1 bug 3）。

---

## 6. 平台缺陷與缺口清單（只記錄，不修）

### 6.1 缺陷

**Bug 1（中）— manifest 長度上限以位元組計，錯誤訊息卻寫「characters」**
`services/platform/internal/skillpkg/skillpkg.go:200-209`，`len(m.Name)`／`len(m.Description)` 在 Go 是位元組長度。`sokrati/sokrati` 的 description 是 777 字元／1432 位元組，被判 `description exceeds 1024 characters`。對一個以繁體中文為主要語言的產品而言，實際上限只有約 341 個中文字，且錯誤訊息會誤導作者去數字數。`maxNameLen = 64` 同樣受影響（中文名稱剩約 21 字）。修法是 `utf8.RuneCountInString`，或維持位元組上限但改寫訊息。

**Bug 2（低）— `degraded` 不反映索引覆蓋率，只反映 embed 呼叫是否成功**
`services/platform/internal/catalog/http.go:151-159`。`degradedReason` 只在 `LLMClient == nil` 或 embed 呼叫回錯時才設定，與 `search_documents` 是否真的有向量無關。在 `6690736` 上這造成 10 條查詢全數回 0 筆卻宣稱未降級（§5.1）。`b144bea` 讓匯入寫入 embedding 後，最嚴重的情境消失了，但殘留一個較窄的缺口：**`enrichment_status='pending'` 的文件沒有 embedding，對排序腿完全隱形，而回應仍是 `degraded: false`**。增強失敗是設計上允許的（匯入不因模型故障而失敗），因此這個狀態是常態而非例外。同 commit 新增的 `no_results` + `query_suggestion` 改善了「查無結果」的表達，但仍無法讓呼叫端得知目錄有多少比例尚未進入排序。

**Bug 3（中）— match reason 供應來源標記錯誤，且 LLM 輸出 100% 解析失敗**
兩個缺陷疊在一起：

- `services/llm/src/skillhub_llm/app.py:135` 送出 `response_format={"type":"json_object"}`，強制模型回傳物件；但 prompt 要求的是「a JSON array」。實測 `gpt-4o-mini` 把陣列包在 `{"skills": [...]}` 底下，而第 149 行只認 `reasons` 與 `results` 兩個 key，於是 `items` 恆為空，第 163-172 行的樣板句 100% 接管。直接打 `/match-reasons` 可穩定重現。
- `services/platform/internal/catalog/http.go:251-254` 把回傳清單裡任何非空字串一律標成 `reasonSourceModel`。Python 端的樣板句因此被當成模型生成內容回給使用者，違反 DISC-002／ADR-013「模型生成內容必須標示」。同時 Go 自己那套較好的 `templateMatchReason`（會列出實際詞彙重疊）永遠輪不到執行。

**Bug 4（低／設計取捨）— URL 匯入無法表達 commit pin**
`services/platform/internal/ingest/fetch.go:89-109`：`github.com/owner/repo` 只展開成 `refs/heads/main`／`master`。CONTENT-003 的來源全部以 40 碼 SHA pin 住（種子檔 schema 明訂「import MUST fetch this SHA, not the branch head」），但 INGEST-001 沒有任何輸入形式能把 SHA 傳進去，`skill_sources.source_ref` 只會記到分支名。目前只能靠上傳路徑外部保證 pin，INGEST-004 的來源可追溯性因此有缺口。

### 6.2 已於本次驗證期間關閉的缺口

在 `6690736` 上量到、`b144bea` 已修復，記錄於此僅為留痕：

- **索引時 embedding 沒有任何寫入者**：`UpsertSearchDocumentWithEmbedding` 由 sqlc 產生但全 repo 無呼叫端，`ReindexAll` 也只寫 name／summary，公開搜尋實質上是純 FTS。→ 現由 `ingest.enrich.go` 在匯入與 `SaveVersion` 時寫入，`cmd/reindex` 另有 backfill 階段。
- **`/v1/enrich-skill` 沒有 Go 呼叫端**：ADR-013 §1 的索引時增強只有 Python 側。→ 現已接線，本次 44／44 全數 `enriched`。

兩項均在 `b144bea` 的重跑中實測確認關閉，見 §3 與 §5.2。

---

## 7. 重現與清理

```bash
# 1. 基礎設施
docker compose -f infra/compose/docker-compose.yml up -d
docker cp db/migrations skillhub-postgres-1:/migrations
for f in db/migrations/*.sql; do
  docker exec skillhub-postgres-1 psql -v ON_ERROR_STOP=1 -U skillhub -d skillhub -f "/migrations/$(basename "$f")"
done

# 2. platform API（env 見 §1）
docker run -d --name skillhub-api --network skillhub_default -p 8080:8080 \
  -v "$PWD:/src" -w /src/services/platform --add-host host.docker.internal:host-gateway \
  -e DATABASE_URL=... -e OBJSTORE_ENDPOINT=seaweedfs:8333 -e DEV_LOGIN=1 -e COOKIE_INSECURE=1 \
  -e LLM_SERVICE_URL=http://host.docker.internal:8000 golang:1.25 go run ./cmd/api

# 3. llm 服務（金鑰只進程序 env，不落檔）
cd services/llm && uv run uvicorn skillhub_llm.app:app --port 8000

# 4. 匯入
python tools/content/import_seed.py --selftest     # 離線自我檢查
python tools/content/import_seed.py                # 45 筆
python tools/content/import_seed.py --probe-url    # INGEST-001 探針

# 5. 重新索引（第二階段的增強 backfill 另需 LLM_SERVICE_URL 與物件儲存）
docker run --rm --network skillhub_default -v "$PWD:/src" -w /src/services/platform \
  --add-host host.docker.internal:host-gateway \
  -e DATABASE_URL=... -e OBJSTORE_ENDPOINT=seaweedfs:8333 -e OBJSTORE_BUCKET=skillhub \
  -e LLM_SERVICE_URL=http://host.docker.internal:8000 golang:1.25 go run ./cmd/reindex
```

清理：`docker rm -f skillhub-api`、`docker compose -f infra/compose/docker-compose.yml down -v`、停止 uvicorn。**dev DB 與物件儲存的資料已全數刪除，不保留。** 匯入結果 JSON、搜尋 smoke 腳本與 §5.1 的一次性 embedding 補寫腳本留在暫存目錄，未進 repo。

---

## 8. 給 CONTENT-006 的三個待決事項

1. **授權顯示的事實來源**：manifest 自宣告（37／45 為未知）還是策展登錄的 repo 層事實？現行資料模型只存前者。
2. **靜態檢查是否延伸到 SKILL.md 內嵌程式碼**：不延伸的話，32／45 宣告相依的 Skill 在 UI 上會顯示為「無外部相依」。若要延伸，需先在 `plans/mvp/02` 補 INGEST-007 的需求 ID 與允收準則。
3. **anthropic-sa 4 筆的法務判定**：本次證明技術上可匯入且格式合規，但 §5.2 的 source-available 條款判定仍是 CONTENT-003／004 的阻塞項；documents 類的「已索引」計數在判定前不能視為達標。
