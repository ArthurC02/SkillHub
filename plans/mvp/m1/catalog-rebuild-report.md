# 線上目錄重建報告（M1 閘門基線）

- 日期：**2026-08-16**
- 基準 commit：**`b2a7690`**（`Add CONTENT and SEC acceptance criteria to the MVP spec`）
- 目的：為 [M1 驗證閘門使用者測試](gate-test/README.md)建立**可凍結的線上目錄基線**。前次驗證（[`import-report.md`](import-report.md)）的資料已隨容器銷毀，[`content-summaries.md` §2.4](content-summaries.md) 實查確認線上只剩 2 筆整合測試殘留。
- 對應：[`content-summaries.md` §7 第 5 項](content-summaries.md)、[`gate-test/README.md` §3.1／§3.2](gate-test/README.md)、`02` §4.7 CONTENT-005／006、`02` DISC-001～003
- 狀態：**重建完成，stack 保持運行。**「凍結」尚未生效——**需負責人確認 D 日**（§7）。

> **本文件不定案、不修改既有規格。** 未動 `adr/`、`services/`、`apps/`、`db/`、`.github/`、`plans/mvp/02`、`plans/mvp/03`，亦未變更任何工作項目勾選狀態。第 6 節的搜尋數字是 **smoke 級抽樣，不是 PDM-011 正式評測**，不得用來取代 [`tools/goldenset/`](../../../tools/goldenset/) 的量測。

---

## 1. 一句話結論

**45／45 匯入成功、45／45 `enriched`（`enrich-skill/v2`）、45／45 有向量、golden query smoke hit@3 = 8／10、篩選器行為正確。** 相對 [`import-report.md`](import-report.md) 的 44／45，差別是 `sokrati/sokrati` 的規格驗證失敗已被平台修復解掉（§4.1）——**線上目錄實數自此為 45 筆，不再是 44**。

---

## 2. 環境搭建

四層全部起得來，無須降級。

| 層 | 作法 | 結果 |
| --- | --- | --- |
| Postgres | `infra/compose/docker-compose.yml`，`pgvector/pgvector:pg17` | ✅ healthy |
| 物件儲存 | 同 compose，`chrislusf/seaweedfs:3.80` | ✅ **healthy（healthcheck 修復已生效）** |
| Schema | `db/migrations/0001`～`0015` 逐檔 `psql -v ON_ERROR_STOP=1` | ✅ **15 檔全數套用**，僅 0009 有 ivfflat 低資料量 NOTICE |
| platform API | `docker run golang:1.25 go run ./cmd/api` | ✅ `/healthz` 200 |
| llm 服務 | `uv run uvicorn skillhub_llm.app:app --port 8000`（跑在 host） | ✅ `/healthz`、`/embed`、`/v1/enrich-skill` 皆有回應 |

**與前次的環境差異**：

1. **migrations 由 11 檔增為 15 檔**（0012 license provenance、0013 governance、0014 license manifest reference、0015 search result facets）。0015 為 `search_documents` 新增 `limitations` 與 `scan` 兩欄，是 DISC-003「限制」區塊的儲存基礎。
2. **seaweedfs healthcheck 不再卡住**。前次需人工確認可用性；本次 compose 自身即回報 healthy（探針改打 `127.0.0.1:8333/status`，繞開容器內 `localhost` 先解析到 `::1` 的問題）。
3. **資料完全重建**：先 `down -v` 銷毀 volume 再 `up -d`，因此 `content-summaries.md` §2.4 記錄的 2 筆整合測試殘留不在本基線內。

platform API 的環境變數：

```
DATABASE_URL=postgres://skillhub:skillhub@postgres:5432/skillhub
OBJSTORE_ENDPOINT=seaweedfs:8333        OBJSTORE_BUCKET=skillhub
DEV_LOGIN=1                             COOKIE_INSECURE=1
API_ADDR=:8080                          LLM_SERVICE_URL=http://host.docker.internal:8000
```

匯入身分為 `seed-importer`（ADR-020 dev provider），其個人 Workspace 即匯入目的地，事後以 `is_catalog = true` 明確授予公開性（0010 遷移要求公開必須明確授予）。

### 2.1 模型出口：沿用前次的本機例外，僅限本機

`LITELLM_BASE_URL` 直接指向 `https://api.openai.com/v1`、`LITELLM_API_KEY` 由 repo 根 `.env` 的 `OPENAI_API_KEY` 匯出進 uvicorn 行程環境，**未經 LiteLLM 閘道**。

> **這不是可接受的部署形態。** 鐵律 8（ADR-017）要求所有模型呼叫走 LiteLLM 閘道、供應商金鑰只存在閘道。本次與 [`import-report.md` §1.1](import-report.md)、[`content-summaries.md` §2.3](content-summaries.md) 同性質，是離線／本機工序的既有例外：金鑰只以環境變數進入行程、未寫入任何檔案，驗證結束即隨行程消滅。**產品實作不得比照。**

---

## 3. 匯入結果

**成功 45／45，阻擋錯誤 0，`error_fetch` 0。**

DB 落地：`skills = 45`、`skill_versions = 45`、`search_documents = 45`。

| 類別 | 筆數 | 成功 | 被拒 | `embedded-script` 警告 | `script-file` 揭露 |
| --- | --- | --- | --- | --- | --- |
| data | 25 | 25 | 0 | 12 | 1 |
| documents | 10 | 10 | 0 | 3 | 6 |
| writing | 10 | 10 | 0 | 0 | 1 |
| **合計** | **45** | **45** | **0** | **15** | **8** |

依 tier：`curated` 15／15、`indexed` 30／30，**全數成功**。

依來源：`anthropic` 2、`anthropic-sa` 4、`handoff` 1、`humanizer` 1、`kagura-docfmt` 1、`neon-jetpack` 5、`nqumich` 1、`quiz-builder` 1、`sokrati` **2**、`wrangler` 12、`yuyy-excel` 15 —— 全部成功。

套件體積：最小 2.4 KB、中位數 5.3 KB、最大 160 KB（`anthropic-sa/docx`），離 `MaxZipBytes` 32 MiB 極遠。體積較前次略增，因為 ADR-021 的 `LICENSE.repo` 與 provenance 檔會一併打包。

> **`anthropic-sa` 的 4 筆（docx／pdf／pptx／xlsx）在種子檔標記 `redistributable: false`，法務判定未完成。** 與前次相同，本次為驗證仍予匯入，資料只存在於本機 dev DB。**正式目錄在 CONTENT-004 §5.2 結案前不得保存這 4 筆的內容快照，也不得產生 Download Artifact。** 閘門測試同樣不得引導受測者往下載走（`gate-test/README.md` §5 限制 4）。

### 3.1 與前次匯入的逐項對照

| 項目 | 前次（`b144bea`，`import-report.md`） | 本次（`b2a7690`） | 差異來源 |
| --- | --- | --- | --- |
| 匯入成功 | 44／45 | **45／45** | `sokrati/sokrati` 的長度計算修復（§4.1） |
| 阻擋錯誤 | 1（`description-too-long`） | **0** | 同上 |
| `license-unknown` 警告 | **37** | **0** | ADR-021 分層授權：打包時帶入 repo 根 LICENSE（§4.2） |
| 授權來源揭露 | 無此分類 | `repo-file` 34、`package-file` 3、`manifest-reference` 2 | ADR-021 新增 |
| `embedded-script` 警告 | **0（掃不到）** | **15** | CONTENT-006 掃描面擴大（§4.3） |
| `external-url` 揭露總數 | **816** | **87** | 依 host 聚合去重（§4.4） |
| 揭露總筆數 | 816+ | 210 | 同上 |
| `enrichment_status='enriched'` | 44／44 | **45／45** | — |
| `enrichment_prompt_version` | `enrich-skill/v1` | **`enrich-skill/v2`** | 服務端 prompt 已升版（新增 `limitations`） |
| golden smoke hit@3 | 8／10 | **8／10** | 持平（§6.1） |
| 其中 gold 為 Top-1 | 5／10 | **7／10** | 排序改善（§6.1） |

---

## 4. 前次四個失敗模式的現況

[`import-report.md` §4](import-report.md) 記錄的三個常見失敗模式與 §3.4 的驗證失敗，**四項全部已由平台修復關閉**。本節只做實測確認，不重述修法。

### 4.1 §3.4 規格驗證失敗 → 已解決

`sokrati/sokrati` 的 frontmatter description 是 **777 字元／1432 位元組**的俄文段落，前次以位元組計長被判 `description exceeds 1024 characters`。`skillpkg.checkManifest` 現已改用 `utf8.RuneCountInString`（`name` 與 `description` 皆是），777 < 1024，**本次通過**，findings 只剩 `license-from-repo-file`（MIT）與 2 條 `external-url`。

**這是本次 45／45 的唯一原因。** 前次報告的「擋得對但理由算錯」判斷成立：修的是計長單位，不是放寬上限。

### 4.2 Top-1「License 在 frontmatter 缺席 37／45」→ 已解決

`license-unknown` 警告 **0 筆**。45 筆全部有可呈現的授權來源：

| 授權來源層級 | 筆數 | 意義 |
| --- | --- | --- |
| `manifest`（frontmatter 直接宣告） | 6 | 最強 |
| `manifest-reference`（frontmatter 指向套件內檔案） | 2 | ADR-021 新增 |
| `package-file`（套件根自帶 LICENSE） | 3 | — |
| `repo-file`（打包時帶入的 `LICENSE.repo`） | **34** | ADR-021 tier 3 |

實查 `excel-deduplicate` 詳情頁：`expression: MIT`、`source: repo-license-file`、`status: declared`，並附說明「來自 repo 根目錄的 LICENSE，涵蓋整個 repo，不必然涵蓋此子目錄的內容」。**前次報告指出的「套件內宣告與 repo 層授權事實脫節」不再成立**，且沒有把 repo 授權偽裝成套件自身的授權。

### 4.3 Top-2「掃描看不見 SKILL.md 內嵌的程式」→ 已解決

新增 `embedded-script` 警告，本次命中 **15 筆**，與前次報告點名的缺口**逐筆吻合**：`yuyy-excel` 13 筆、`nqumich/data-analyst`、`anthropic-sa/pdf`。

前次的擔憂是「使用者看到『本套件不含腳本』時，實際可能正要跑 180 行內嵌 Python」。現在這 15 筆都會出示警告，且該訊號已接進詳情頁的限制區塊——實查 `excel-deduplicate` 的限制含 `scan` 來源的「SKILL.md 內嵌可執行程式碼，需可執行該語言的環境，且應先自行檢視內容」。

### 4.4 Top-3「資訊揭露被 external-url 淹沒（816 條）」→ 已解決

`external-url` 揭露總數由 **816 條降為 87 條**，訊息改為依 host 聚合（例：「references 2 external URL(s) on github.com: …」）。前次單筆 321 條的 `anthropic-sa/docx` 不再產生無法閱讀的清單。全部揭露筆數 210，已是人可以逐筆讀完的量級。

---

## 5. Enrichment 版本基線，與 `summaries.json` 的關係

### 5.1 基線

| 項目 | 值 |
| --- | --- |
| `enrichment_status` | `enriched` **45／45** |
| `enrichment_prompt_version` | **`enrich-skill/v2` 45／45** |
| `enrichment_model` | `gpt-5.6-sol` 45／45 |
| `embedding` 非空 | **45／45** |
| `cmd/reindex` 事後複查 | `documents=45 pruned=0`、`enriched=0 still_pending=0` |

`reindex` 回報 `enriched=0` 代表**匯入當下已無遺留**，backfill 冪等，與前次結論一致。

**`enrich-skill/v2` 是本次目錄的唯一增強版本。** 相對 v1 的實質差異是新增 `limitations` 欄位，因此 02 §DISC-003 要求的「一般模式顯示限制」在**線上 45／45 全部有資料**。實查詳情頁，限制區塊同時呈現兩個來源並各自標示：

- `model`：增強產出的限制（例：「公式健康檢查僅檢查工作表前 49 列、前 9 欄中是否出現 `#REF!`」）
- `scan`：靜態掃描推導的限制（例：「SKILL.md 內嵌可執行程式碼……」）

### 5.2 與審核中的 `summaries.json` 是什麼關係

**線上為準。`summaries.json` 不決定線上文字。**

| | [`tools/content/summaries.json`](../../../tools/content/summaries.json) | 線上目錄（本次基線） |
| --- | --- | --- |
| 是什麼 | **人工審核的紀錄本**（[`content-summaries.md`](content-summaries.md) 的機器可讀產出） | 使用者與閘門受測者實際看到的東西 |
| 怎麼來的 | 部分重用 golden set 既有增強、部分新呼叫 | 匯入時由 `ingest/enrich.go` 全部重新呼叫 |
| prompt 版本 | 精選 15 筆為 v2；**已索引 30 筆中 8 筆仍為 v1** | **45 筆全部 v2** |
| 逐字相同嗎 | — | **否** |

**為什麼不逐字相同，而且不該期望相同。** 增強是每次匯入重新生成的，LLM 輸出非決定性——這是 [`content-summaries.md` §2.4](content-summaries.md) 早已記錄的**結構性上限**，不是本次的疏漏。實例：`excel-deduplicate` 的限制在 `summaries.json` 寫「關鍵欄位中的多個空值只會保留第一筆」，線上寫「關鍵欄位中的多個空值會被視為重複值，只保留第一個空值」——**同一件事、不同措辭**。

`content-summaries.md` §2.5 對此有實測：8 筆同版本重跑，語意涵蓋 8／8 一致，漂移落在措辭與 `tags` 粒度。**同版本重跑的品質可複現，逐字內容不可複現。**

> **這對審核工序的意涵（`02` §4.7 CONTENT-005 非決定性上限）。** 該準則明定「審核判定只對**已入庫的該版文字**生效；任何重跑增強後，須重新確認入庫文字與審核過的版本一致，否則該筆退回 `待審`」。
>
> 目前 45 筆審核狀態**全部為 `待審`**，因此本次重建**不造成任何退回**。但它決定了審核該對著什麼看：
>
> 1. **審核人應以線上詳情頁為判定對象**，`content-summaries.md` 作為對照與留痕。
> 2. **審核完成後不得再重新匯入**，否則入庫文字改變、全部判定失效。這與 §7 的凍結聲明是同一條紀律的兩面。
> 3. `summaries.json` 那 8 筆 v1（皆為 `indexed` 層）在線上都是 v2，因此線上不存在缺 `limitations` 的筆。

---

## 6. 搜尋 smoke（非正式評測）

**流程**：匯入 → `cmd/reindex` → 將匯入 Workspace 的 `is_catalog` 設為 true → 從 [`tools/goldenset/queries.json`](../../../tools/goldenset/queries.json) 取**與 [`import-report.md` §5.2](import-report.md) 完全相同的 10 條**打公開端點 `GET /api/skills/search`，使前後兩次可直接對照。

### 6.1 golden query 抽樣：hit@3 = 8／10

| ID | 語言 | Top-3 | gold_primary | 命中 | 前次 |
| --- | --- | --- | --- | --- | --- |
| D01 | zh | **docx**, document-format-skills, pptx | docx | ✓ Top-1 | ✓ Top-1 |
| D02 | zh | **pdf**, excel-merge, excel-split | pdf | ✓ Top-1 | ✓ Top-1 |
| D03 | zh | **pptx**, internal-comms, cringe-check | pptx | ✓ Top-1 | ✓ Top-1 |
| D04 | zh | **xlsx**, data-shape, excel-scout | xlsx | ✓ Top-1 | ✓ Top-1 |
| D05 | zh | line-edit, brand-guidelines, pptx | report-designer\* | ✗ | ✗ |
| D13 | en | **docx**, document-format-skills, xlsx | minimax-docx\* | ✓ acceptable | ✓ acceptable |
| W01 | zh | **brand-guidelines**, docx, document-format-skills | brand-guidelines | ✓ Top-1 | ✓ Top-1 |
| W13 | en | humanizer, ai-written-check, sokrati | humanize\* | ✗ | ✗ |
| A01 | zh | **excel-deduplicate**, excel-find-duplicates, excel-delete | excel-deduplicate | ✓ **Top-1** | ✓（第 2 名） |
| A13 | en | **excel-deduplicate**, excel-split, excel-insert | excel-deduplicate | ✓ **Top-1** | ✓（第 2 名） |

\* gold 答案不在本次 45 筆目錄內（`report-designer`、`minimax-docx`、`humanize` 屬 golden query set 的較大候選語料）。**扣掉這 3 條結構性不可能命中的查詢，實得 8／8。**

**`degraded` 全程 `false`，且與事實相符**（45 筆皆已 enriched、向量腿有完整資料）。

**唯一的變化是排序品質**：`primary` 為 Top-1 由 5／10 升為 **7／10**（A01、A13 兩條由第 2 名升至第 1 名）。兩條未命中仍是**目錄缺件而非排序問題**，與前次結論相同。

> **這不是評測，讀作量級。** 10 條是分層抽樣，golden set 的正式量測（60 條／31 份語料、recall@5 = 48／48）在 [`golden-query-set.md`](golden-query-set.md)。閘門的量化前置以那份為準，本節只確認**重建後的目錄沒有比前次退步**。

### 6.2 篩選器 smoke（DISC-003）

以 `q=excel duplicate rows`（無篩選 28 筆結果）為基準：

| 篩選 | 結果數 | `filtered_out` | 判讀 |
| --- | --- | --- | --- |
| （無） | 28 | — | 基準 |
| `script=yes` | 17 | false | — |
| `script=no` | 11 | false | **17 + 11 = 28，切分完整不重疊** ✅ |
| `validation=passed` | 28 | false | 45 筆皆通過規格驗證，故等同無篩選 ✅ |
| `validation=unverified` | **0** | **true** | **空結果，且誠實標示「是被篩掉的、不是查無此物」** ✅ |

`validation=unverified` 這一格是本次最值得記的一筆：結果為空**是正確的**（目錄裡沒有未通過驗證的 Skill），而回應以 `filtered_out: true` 與「查無結果」區分開來——正是 DISC-005「無結果＋改寫建議」不該誤用的情境。

**錯誤輸入與停用維度的處理**（各打一次）：

| 請求 | 回應 |
| --- | --- |
| `script=maybe` | `400 script must be "yes" or "no"` |
| `validation=bogus` | `400 validation must be "passed" or "unverified"` |
| `category=data` | `400 filter not available: category — 類別尚未存入平台…（CONTENT-003）` |
| `tier=curated` | `400 filter not available: tier — 來源層級目前全目錄同為「已索引」…` |
| `agent=claude` | `400 filter not available: agent — Agent 相容狀態需要 Sandbox 試跑才有結果(M2)…` |
| `mcp=no` | `400 filter not available: mcp — 是否需要 MCP 沒有任何來源資料…` |

四個停用維度**全部以 400 拒絕並附上逐項的誠實理由**，不是靜默忽略——這正是 `gate-test/README.md` §1.1 要觀察「停用的篩選器是否被讀成系統壞了」的那個設計。**受測者會看到的就是這些文案。**

> ⚠️ **一個給閘門的提醒**：`tier` 的停用理由寫「全目錄同為『已索引』」，但種子清單其實有 15 筆 `curated`。這是**平台端沒有持久化 tier**（與 `category` 同因，見 CONTENT-003），文案描述的是平台事實而非策展事實。不影響本次凍結，但若受測者追問，主持人不應解釋（屬不可提示清單的性質）。

### 6.3 `match_reason`：前次的 Bug 3 已修，但有一個延遲風險

[`import-report.md` §6.1 Bug 3](import-report.md) 記錄兩個疊在一起的缺陷：LLM 輸出 100% 解析失敗，以及 Go 把樣板句一律標成 `model`。**兩者本次實測皆已修復**：

- 逐筆 `match_reason` 是真正的模型產出且針對該查詢（例：`pptx` 對「整理成 Word 文件」的查詢回「此技能專注於 PowerPoint 簡報的處理，**與用戶的需求不符**」——會說出不符合的理由，不是樣板句）。
- 來源標示誠實：模型產出標 `model`，逾時退回的樣板句標 `template`（`catalog/http.go:630`、`634`），不再混為一談。

⚠️ **但 `/match-reasons` 會在單頁結果多時逾時**。§6.2 的篩選器 smoke 以 `limit=50` 連打，19 次呼叫中出現 **5 次** `context deadline exceeded`，該頁即整批退回 `template`。一般查詢（`limit` 5～20）實測延遲 **4～9 秒**，未觀察到逾時。

**對閘門的意涵**：受測者用的是預設頁面大小，落在未逾時的區間，風險低；但**若某場次出現樣板句，那一題的「符合原因」品質與其他場次不可比**。記錄員若看到 `match_reason` 突然變成通用句式，應在該題次註記。**本次不修**（屬平台缺陷，只記錄）。

---

## 7. 閘門凍結聲明

### 7.1 凍結標的

自凍結生效起至使用者測試完成，**下列一律不得變更**：

| # | 標的 | 現值／基線 |
| --- | --- | --- |
| 1 | **目錄內容** | **45 筆**（非 `import-report.md` 與 `gate-test/README.md` 所寫的 44 筆） |
| 2 | **增強版本基線** | ~~`enrich-skill/v2`~~ **`enrich-skill/v2` × 26 ＋ `v3` × 5 ＋ `v4` × 3 ＋ `v5` × 11**、模型 `gpt-5.6-sol`、45／45 `enriched`（2026-08-16 兩次重跑後的現值，見 §7.3） |
| 3 | **入庫的增強文字** | ~~本次匯入所生成的該版文字~~ **現行線上文字**：本次匯入的 v2 文字，加上 2026-08-16 兩次重跑（審校輪 10 筆、CONTENT-005 修正輪 11 筆，其中 2 筆重疊）改寫過的 **19 筆**。**不得重新匯入、不得重跑增強、不得 reindex** |
| 4 | `MaxCosineDistance` 門檻 | **0.75** |
| 5 | 排序管線 | 現行混合檢索與 RRF 設定 |
| 6 | DISC-002 七欄與逐筆 `match_reason` 文案 | 現行 |
| 7 | DISC-003 篩選器（含四個停用維度的文案） | §6.2 所列 |
| 8 | 基準 commit | **`b2a7690`** |

**stack 保持運行中**（`skillhub-postgres-1`、`skillhub-seaweedfs-1`、`skillhub-api`、host 上的 uvicorn）。**不要 `docker compose down`，不要 `down -v`** —— 這是閘門環境本身，銷毀即需整套重建，且重建後的增強文字必然與本基線不同（§5.2）。

### 7.2 凍結**尚未**生效

> **凍結生效需負責人確認 D 日。** 本報告只建立基線並聲明標的，**不代行宣告凍結開始**。
>
> 在負責人確認 D 日之前，本目錄仍可修改（例如審核發現需改 prompt 而重跑增強）。**D 日一經確認，§7.1 的八項即凍結，其後任何變更都會使前後場次不可比**（`gate-test/README.md` §3.2）。
>
> 若不得不改（例如線上事故），依 `gate-test/README.md` §3.2：**變更前後的場次分開統計**，並在分析報告中明列。

### 7.3 凍結前仍未解除的前置

| 前置 | 狀態 | 依據 |
| --- | --- | --- |
| 量化前置（檢索品質） | ✅ 已過 | `golden-query-set.md` §10.8 |
| 索引管線就緒 | ✅ **本次重建完成**：45／45 匯入、45／45 `enriched` | 本報告 §3、§5 |
| UI 阻擋（符合原因、詳情頁） | ✅ 已關閉 | `m1-work-items-audit.md` §5.1／§5.2 |
| ~~**CONTENT-005 人工審核**~~ **CONTENT-005 審核** | ⚠️ **已完成，數字已變**（~~45/45 通過~~ → **44/45 通過、1 筆 `需修改`**） | [`content-review-report.md`](content-review-report.md)（§11） |

~~**CONTENT-005 的白話摘要已全數產生且線上可見，但 45 筆的審核狀態全部仍是「待審」。**~~

> **2026-08-16 更新。** 負責人授權以自動化審校取代人工審核，45 筆已全量審完（**45/45 通過**），審校確實是**對著線上詳情頁**做的（`GET /api/skills/{id}`），符合本報告 §5.2 的要求。
>
> **本基線的一項變動**：審校過程中共 13 筆依處置規則重跑增強（10 筆 `enrich-skill/v3`，其中 3 筆再以 **v4** 重跑；一律改以 `search_documents.enrichment_status='pending'` ＋ `cmd/reindex` 完成，**未重新匯入、未動 Skill Version 與 `skill_id`**）。因此線上 45 筆的 prompt 版本 ~~現為 **v2 × 35 ＋ v3 × 7 ＋ v4 × 3**~~，不再是本報告 §1 所記的「45/45 v2」。筆數、`enriched` 比例、向量覆蓋不變（45／45／45）。重跑清單見 [`content-review-report.md` §4](content-review-report.md)。
>
> **2026-08-16 第二項變動（CONTENT-005 修正輪，commit `afb5767`）**：CONTENT-007／008 基準試跑查出 11 筆的「限制」欄漏掉套件宣告的 Python 執行依賴，依 `02` §4.7 `需修改` 的處置升 `enrich-skill/v5` 重跑增強與重新索引（同一 `pending` ＋ `cmd/reindex` 路徑，**未重新匯入、未動 Skill Version 與 `skill_id`**）。因此：
>
> - 線上 prompt 版本現為 **v2 × 26 ＋ v3 × 5 ＋ v4 × 3 ＋ v5 × 11**；筆數、`enriched` 比例、向量覆蓋仍為 45／45／45。
> - 審校結果由 45/45 通過變為 **44/45 通過、1 筆 `需修改`**（`docx`，理由與 Python 揭露無關，兩輪重跑未收斂，留給負責人裁定）。
> - 本次動的是 §7.1 凍結標的第 2、3 項；**執行時 D 日尚未宣告（§7.2 仍成立），故合法**。逐筆結果與入庫文字指紋見 [`content-review-report.md` §11](content-review-report.md)。

### 7.4 凍結期間的回歸把關

沿用 `gate-test/README.md` §4.3，兩項照舊：

1. 每次疑似動到語料或索引管線，跑 `python tools/goldenset/evaluate.py --selfcheck`（零成本、零網路）確認**確實沒被動過**。
2. `golden-query-set.md` §4.4 的門檻過期觸發條件 1（語料成長一個數量級）與 3（查詢改寫上線）仍然有效；測試期間若任一觸發，本次結論作廢。

額外一項（本次新增，因為目錄實數改變）：

3. 隨時可用下列一行複查目錄基線是否仍是 ~~45 筆 v2~~ **45 筆、版本分布如下**（期望值 2026-08-16 隨修正輪更新）：

```bash
docker exec skillhub-postgres-1 psql -U skillhub -d skillhub \
  -c "select enrichment_status, enrichment_prompt_version, count(*), count(embedding)
      from search_documents sd join workspaces w on w.id = sd.workspace_id
      where w.is_catalog group by 1,2;"
# 期望：enriched | v2 26|26、v3 5|5、v4 3|3、v5 11|11（合計 45|45）
```

> 查詢加了 `is_catalog` 條件：M2 基準試跑在臨時 Workspace 留下 45 筆 fork 的 `search_documents`（`pending`、無向量），不加條件會把它們一起算進來。目錄本身仍是 45 筆。

---

## 8. 重現步驟

```bash
# 1. 基礎設施（重建會銷毀現有閘門環境——凍結期間不要執行）
docker compose -f infra/compose/docker-compose.yml down -v
docker compose -f infra/compose/docker-compose.yml up -d
docker cp db/migrations skillhub-postgres-1:/migrations
# 逐檔套用 0001~0015，ON_ERROR_STOP=1

# 2. llm 服務（金鑰只進行程 env，不落檔；鐵律 8 的本機例外，見 §2.1）
set -a && . ./.env && set +a
export LITELLM_BASE_URL=https://api.openai.com/v1 LITELLM_API_KEY="$OPENAI_API_KEY"
cd services/llm && uv run uvicorn skillhub_llm.app:app --host 0.0.0.0 --port 8000

# 3. platform API（env 見 §2）
docker run -d --name skillhub-api --network skillhub_default -p 8080:8080 \
  -v "$PWD:/src" -w /src/services/platform --add-host host.docker.internal:host-gateway \
  -e DATABASE_URL=... -e OBJSTORE_ENDPOINT=seaweedfs:8333 -e OBJSTORE_BUCKET=skillhub \
  -e DEV_LOGIN=1 -e COOKIE_INSECURE=1 -e LLM_SERVICE_URL=http://host.docker.internal:8000 \
  golang:1.25 go run ./cmd/api

# 4. 匯入 45 筆
python tools/content/import_seed.py --selftest    # 離線自我檢查
python tools/content/import_seed.py               # 45 筆

# 5. 公開性與重新索引
#    UPDATE workspaces SET is_catalog = true WHERE id IN (SELECT workspace_id FROM skills);
docker run --rm --network skillhub_default -v "$PWD:/src" -w /src/services/platform \
  --add-host host.docker.internal:host-gateway \
  -e DATABASE_URL=... -e OBJSTORE_ENDPOINT=seaweedfs:8333 -e OBJSTORE_BUCKET=skillhub \
  -e LLM_SERVICE_URL=http://host.docker.internal:8000 golang:1.25 go run ./cmd/reindex
```

**匯入結果 JSON 與 smoke 腳本留在暫存目錄，未進 repo**（與 `import-report.md` 同慣例）：它們是一次性驗證產物，且每次重建的內容都會不同——留在 repo 只會產生一份會過期的假事實來源。

---

## 9. 本次未做、未決的事

1. **人工審核**（§7.3）——唯一實質前置，屬內容負責人。
2. **`excel-format` 字型在地化裁定**——見 [`content-summaries.md` C-4](content-summaries.md)。屬審核判斷，不是生成錯誤。
3. **`anthropic-sa` 4 筆的法務判定**——技術上可匯入且格式合規，但 source-available 條款判定仍是 CONTENT-003／004 的阻塞項（§3 註）。
4. **`gate-test/` 與 `import-report.md` 的「44 筆」字樣未修**——本報告不代改閘門套件；請負責人於 D 日一併處理（[`content-summaries.md` §7](content-summaries.md) 亦記此項）。
5. **`tier` 篩選器停用理由的文案與策展事實不一致**（§6.2 註）——只記錄，不修。
6. **`/match-reasons` 在單頁 50 筆時會逾時退回樣板句**（§6.3）——只記錄，不修。屬平台缺陷，非本次基線問題。
