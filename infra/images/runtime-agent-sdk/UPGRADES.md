# Runtime Image 升級紀錄（ADR-023 §4）

本檔是 [ADR-023](../../../docs/adr/ADR-023-agent-sdk-version-pinning-and-behaviour-revalidation.md)
要求的**行為重驗證據落點**，**append-only**：每次升級加一節，既有節不改寫。

**「升級」的定義是任何會改變 image digest 的變更**（ADR-023 §1）：SDK 版本、基底映像、
映像內任何套件。三者都要跑 §2 的四項清單，缺一項不得合併。

**為什麼要有這個檔**：ADR-023 §3 的理由是 `0.3.233` 那次——`settingSources` 的語意反轉了，
而 changelog 讀起來完全合理。**上表每一項失敗時都不會拋錯**，只會讓某個行為消失而 Run 仍
`succeeded`。所以這裡收的是**實測輸出**，不是推理、不是 release notes。

四項測項（ADR-023 §2 原文，此處只記編號）：

| # | 測項 |
| --- | --- |
| 1 | Skill 載入條件（`init` 的 skills 清單 ＋ `skill_activation` 事件） |
| 2 | 閘道相容（全數經 LiteLLM；金鑰撤銷後回 401） |
| 3 | Prompt caching 計費欄位（`cache_*` 是否出現；`usage` 與閘道 spend 對帳） |
| 4 | `usage` 事件的發出條件 |

**與 SBX-002 掃描的分工**：`../README.md` 的掃描與豁免清單管**供應鏈風險**，本檔管
**行為回歸**，兩者不覆蓋對方（ADR-023 §影響）。

---

## `2026.08-1` → `2026.08-2`（2026-08-16）— 補記

> **補記說明**：本節記錄的升級發生在 ADR-023 訂立**之前**（ADR-023 日期同為 2026-08-16），
> 當時證據落在 M2 基準報告而非本檔。這裡不重跑，而是把**當時實際跑過的量測**對應回四項
> 清單並附引用——ADR-023 §3 禁止的是「以 changelog 代替實測」，不是禁止引用既有實測。
> 凡當時未涵蓋的項目，下方誠實記為未涵蓋。

| 欄位 | 值 |
| --- | --- |
| 變更 | 加入 `python3` 3.11.2 ＋ 目錄宣告的 9 個 Python 套件（`pip` 隨後移除） |
| SDK 版本 | `0.3.233`（未變） |
| 基底 digest | `sha256:d649c27dae7ba0137b3cef5dd75baa422c08dc3d9e3fc0c23dfb172dc3cc6436`（未變） |
| 映像 digest | `sha256:61ef902ffed4e1a66dcc3bc61684f4253721e96b6ea2dfa370bbe43d7da5f7fe`（GHCR tag `2026.08-2`，2026-08-16 觀測；升 `-3` 後仍指向它，未被孤立） |
| commit | `b0c270b`（映像變更本體）；閘道預算根因修復 `3906fe5` 為其前置 |
| 證據來源 | [content-baseline-report.md §12／§13](../../../docs/plans/mvp/m2/content-baseline-report.md) |
| 實測規模 | **45 個 Skill 全量**（§12 的 9 筆 ＋ §13 的 36 筆），閘道實付 $0.86 ＋ $1.85 |

| # | 結果 | 關鍵輸出 |
| --- | --- | --- |
| 1 | **通過** | 45/45 Run 皆有 `skill_activation`（§12.3、§13.2 逐筆表）。相容軸回填 45 列 `capability=activated`（§13.5） |
| 2 | **通過** | 45 個 Run 的模型呼叫全數經 LiteLLM；trace 回報成本與閘道 per-key spend 對帳（§13.6：trace $1.8166 vs 閘道實付 $1.8519）。**撤銷後 401 當時未單獨驗證**——該斷言的既有可執行形式是 `e2e_gateway_integration_test.go`，本次（`-3`）已補跑，見下節 |
| 3 | **通過（且記錄為缺欄）** | `cache_read_input_tokens`／`cache_creation_input_tokens` 在 LiteLLM 的 `/v1/messages` 路由上**缺欄不是 0**，與 PDM-005 §5.2a-7 記載一致，未變動。`input_tokens` 上限強制點因此仍等於 summary 顯示的數字 |
| 4 | **行為已改變，且是本批修的** | §13.2 `add-iso3166` 在 `2026.08-1` 那輪**沒有 `usage` 事件**（成本無聲缺席，§7.2 #4），本批修正後有值。`run.mjs` 現在於 `result`／`accumulated` 兩條路徑都發出，`token_source` 欄位區分兩者（事件 schema 1.1） |

**是否推翻既有文件敘述**：是——`run.mjs` 檔頭原本記載 usage 只掛 `result` 分支，該敘述已
在同批修正（見檔頭 token accounting 節）。`apps/sandbox/README.md` 無需更動。

**附帶量測**（非四項清單，但屬同一次升級的結論）：33 筆 `transpiled` → `native`；輸入
token −50%、成本 −28%（§13.3）；`pandas` 3.x 只觸發一則相容性告警。

---

## `2026.08-2` → `2026.08-3`（2026-08-16）

| 欄位 | 值 |
| --- | --- |
| 變更 | **依賴集**：新增 8 個純 Python 套件（`pycountry` 26.2.16、`chardet` 7.6.0、`defusedxml` 0.7.1、`ftfy` 6.3.1、`confusable-homoglyphs` 3.3.1、`pytz` 2026.3.post1、`phonenumbers` 9.0.37、`python-stdnum` 2.2）。既有 9 個版本未變，**0 移除** |
| 起因 | 目錄 `deps` 欄位漏抄，映像照抄了漏的那份（`../README.md`「依賴集」節） |
| commit | `b2180b2` |
| 映像 digest | `sha256:5bcbca884feaccb4bf1cfb437644f87627898216fde7ba428228712635c9b23d`（GHCR tag `2026.08-3`，2026-08-16 觀測） |
| CI | [runtime-image #31949830049](https://github.com/ArthurC02/SkillHub/actions/runs/31949830049)（`review` job **success**，含 SBOM 與掃描兩份 attestation） |
| 孤兒影響 | **無新增。** 版本 tag 一併改，`2026.08-2` 仍解析到 `sha256:61ef902f…`（發佈後實測），孤兒清單維持 3 筆 |
| SDK 版本 | `0.3.233`（**未變**） |
| 基底 digest | `sha256:d649c27dae7ba0137b3cef5dd75baa422c08dc3d9e3fc0c23dfb172dc3cc6436`（**未變**） |
| 映像大小 | 1.24 GB → **1.35 GB** |
| I-06 閘門 | **通過**，可修的 Critical／High ＝ **0**；各等級件數與 `2026.08-2` **逐格相同**，豁免清單無新增列（`../README.md`「當前掃描結果」） |

### 重驗策略：哪些沿用、哪些實跑

ADR-023 §2 要求四項**在新 digest 上實跑**。本次變更的性質是**依賴集只增不減，SDK 與基底
均未變**，因此：

- 測項 1／3／4 的失效機制（SDK 選項語意、閘道欄位、`usage` 發出條件）**與依賴集無因果
  關係**——但 ADR-023 §3 明文禁止用推理代替實測，所以**四項全部在新 digest 上實跑一次**，
  不採「沿用上一節結論」。跑的是**煙霧規模**而非 45 筆全量：全量是**目錄事實**
  （相容軸回填），與**行為回歸**是兩件事，後者一個 Run 就能觀察到全部四項。
- 額外加測**依賴集本身**，因為那才是本次真正改的東西。

**成本：$0.0202**（上限 $0.3）。單一 Run，`gpt-5.4-mini`。

### 實測：依賴集（本次變更的直接標的）

真容器、`--network none`、非 root（`--user 65532:65532`）——即執行期的隔離條件：

```
docker run --rm --network none --entrypoint python3 skillhub/runtime-agent-sdk:2026.08-3 -c '...'
```

**19/19 import 成功，0 失敗**：`pandas` 3.0.5、`numpy` 2.4.6、`openpyxl` 3.1.5、`lxml` 6.1.1、
`dateutil` 2.9.0.post0、`docx` 1.2.0、`pptx` 1.0.2、`pypdf` 6.16.1、`pdfplumber` 0.11.10、
`pycountry` 26.2.16、`chardet` 7.6.0、`defusedxml`、`ftfy` 6.3.1、`confusable_homoglyphs`、
`pytz` 2026.3.post1、`phonenumbers` 9.0.37、`stdnum` 2.2，＋傳遞依賴 `PIL` 12.3.0、
`charset_normalizer` 3.5.1。

三則功能斷言（import 成功不等於可用）：

- `pycountry.countries.lookup("Taiwan").alpha_3 == "TWN"` ✔（資料表有真的載入）
- `chardet.detect("héllo".encode()) ` 回傳非空 encoding ✔
- `phonenumbers.is_valid_number(parse("+886223456789"))` ✔（metadata 有隨 wheel 進來）

### 實測：ADR-023 四項（單一真實 Run，`run_id=smoke-2026-08-3`）

以真映像跑一次完整的 `run.mjs` 迴圈：掛一個最小 Skill 套件（`dep-smoke`，SKILL.md ＋
一支 `scripts/check.py`），經 LiteLLM 短效 Virtual Key（`max_budget=$0.10`、`duration=20m`）。

| # | 斷言 | 實測輸出 | 判定 |
| --- | --- | --- | --- |
| 1 | Skill 被發現並啟用 | trace 有 `skill_activation`：`{"skill_name":"dep-smoke","decision":"activated"}`；且 `script_log` 顯示套件內腳本**真的被執行**（`RESULT: 17/17 importable, missing=none`），非模型轉譯 | **通過** |
| 2 | 全數經閘道；撤銷後 401 | `cost_source="gateway"`；閘道 per-key spend **$0.02021295**（`max_budget` 0.1）。`key/delete` 後同一把金鑰打 `/v1/messages` → **HTTP 401** | **通過** |
| 3 | caching 欄位與對帳 | `cache_read_input_tokens: null`、`cache_write_input_tokens: null` —— **仍是缺欄不是 0**，與 PDM-005 §5.2a-7 及上一節一致，**語意未變**，`input_tokens` 上限強制點不受影響。對帳：trace `cost_usd` **$0.0180054** vs 閘道 **$0.0202**，差額是 `gatewaySpend()` 輪詢在最後一次 flush 前收斂，即 `run.mjs` 已記載的已知低估方向，非新行為 | **通過** |
| 4 | `usage` 發出條件 | 本 Run 走 `result` 分支，`token_source: "result"`，`input_tokens: 49719`／`output_tokens: 959`，**恰好一次** `usage` 事件。`accumulated` 路徑未在本 Run 觸發（需要無 result 的收尾），其存在由 `-2` 節的 `add-iso3166` 修正與 `run.mjs` 的 `usageEmitted` 保護涵蓋 | **通過（result 路徑）** |

trace 完整性：7 個事件、`seq` 1..7 **無缺口**（`tool_call` ×2、`agent_output` ×2、
`skill_activation`、`script_log`、`usage`）。

**是否推翻任何既有文件敘述**：**否**。`run.mjs` 檔頭列出的四個 Skill 載入條件、caching
欄位缺欄的記載、`usage` 的雙路徑，本次實測全部一致，`run.mjs` 與
`apps/sandbox/README.md` 均無需更動。

### 本次未涵蓋的（明說）

- **45 筆全量相容軸重測未跑**：`skill_runtime_compatibility` 對 `2026.08-3` 目前 **0 列**，
  目錄仍顯示 `2026.08-2` 的結論並附映像標籤。依賴集只增不減使結論極可能不變，但
  0022 的鍵不接受「極可能」。約 $2.2（§13.6 實測基準），已列 `03` 工作項。
- **被拒收依賴的影響未做行為量測**：`pdf`／`pii-flag`／`document-format-skills` 失去哪些
  支路是由靜態掃描推得，非實跑；三筆在 `2026.08-2` 上皆判「符合」（走可用子集）。

---

## `2026.08-3` → `2026.08-5`（2026-08-29；2026-08-30 建置）— **映像已建，四項實測完成 1/4，不得合併到 main**

> **這一節為什麼跨兩版**：`-4` 在 2026-08-29 隨 04 丙-82 落地（拿掉 `unzip`、`run.mjs`
> 自寫 ZIP 解析器），**但本檔當時沒有補節**——`.github/workflows/runtime-image.yml`
> 只檢查「Dockerfile 內容變了就要有 `ARG IMAGE_VERSION=` 的 diff」，沒有任何 job 檢查
> UPGRADES.md 有沒有跟上，也沒有任何 job 檢查四項實測有沒有跑（稽核 04 D3）。
> `-5` 在同一批補上解壓上界。兩次變更之間**沒有任何映像被建置或發佈**，所以這裡合成
> 一節，並把 `-4` 的內容誠實列在變更欄裡，而不是假裝它當時被記錄過。

| 欄位 | 值 |
| --- | --- |
| 變更（`-4`） | `Dockerfile`：移除 `unzip`。`run.mjs`：`extractPackage` 改為自寫 ZIP 解析器（中央目錄 + local header），逐條拒絕絕對路徑／`..` 路徑段（含反斜線變體）／非一般檔（symlink・device・fifo，以 external_attr 判）／zip64／加密／不支援的壓縮法 |
| 變更（`-5`） | `run.mjs`：解壓炸彈上界三道——①每個 deflate 條目以中央目錄宣告的 `uncompressedSize` 當 `inflateRawSync` 的 `maxOutputLength`；②中央目錄宣告總量上界 `MAX_PACKAGE_TOTAL_BYTES = 256 MiB`，**在解壓與寫檔之前**就拒；③條目數上界 `MAX_PACKAGE_ENTRIES = 1000`（與 `artifacts.go` 的 `artifactMaxEntries` 同值同理由）。附帶：stored（method 0）條目的兩個 size 欄位必須相等，否則拒——這是讓①②對 stored 分支也成立的那一條 |
| 為什麼 `-5` 是升級而不是整理 | `unzip` 對壓縮比同樣沒有防護，所以這不是回歸；但 `-4` 把這道邊界搬到了**沒有 cgroup 接住的平台**（clean mode 是主機行程：Linux 無 root 時零資源強制，Windows 的 Job Object 會直接終結整個 job，Run 變成沒有 trace、沒有產出、沒有原因的失敗）。接手一道邊界的時機就是加上界的時機 |
| SDK 版本 | `0.3.233`（**未變**） |
| 基底 digest | `sha256:d649c27dae7ba0137b3cef5dd75baa422c08dc3d9e3fc0c23dfb172dc3cc6436`（**未變**） |
| 映像 digest | `sha256:37c08f48413f1c0c3cbc50730fef83c108d20e618c0b5ce9a9ededf333139eac`（本機建置 2026-08-30；**尚未推上 GHCR**） |
| 依賴集 | **未變**（`-3` 的 17 個 Python 套件，0 增 0 減） |
| commit | 待填（本節與程式同批） |
| CI | 待填 |

### 新增的可執行證據（不是四項清單，但已經跑過）

`run.test.mjs` 由 13 支加到 19 支，新增的 6 支全部針對本次的上界，且每一支都做過突變驗證
（把修正那一行拿掉、確認變紅、再改回來）：

| 測試 | 斷言 |
| --- | --- |
| `refuses an entry that inflates far past its declared size` | 中央目錄宣告 100 bytes、實際 deflate 出 10 MB → 拒，且 `destDir` 為空（拒在寫檔之前） |
| `accepts an entry that deflates honestly, so the bound is not just a wall` | 同樣 10 MB、誠實宣告 → 正常解出。**沒有這一支，上一支會因為「任何大檔都被拒」而假綠** |
| `refuses a package with more entries than the limit` | 1001 個空檔 → 拒（空檔不佔位元組，只佔條目，所以位元組上界單獨不成立） |
| `accepts a package at exactly the entry limit` | 1000 個 → 通過（邊界值不歪一格） |
| `refuses a package whose declared sizes add up past the total limit` | 3 × 100 MiB 宣告 → 拒，且沒有任何條目被解壓或寫入 |
| `refuses a stored entry whose two sizes disagree` | method 0、宣告 1 byte 實載 1 MiB → 拒 |

執行方式：`node --test infra/images/runtime-agent-sdk/run.test.mjs`（19/19 通過，不需要
容器、不需要金鑰、不花錢）。

### 2026-08-30：映像建好了，四項實測跑了第一項

| 項次 | 狀態 | 實測輸出 / 判定 |
| --- | --- | --- |
| **1. 依賴集以新 digest 跑 import 檢查** | ✅ **通過** | `docker run --rm --network none --user 65532:65532 --entrypoint python3 skillhub/runtime-agent-sdk:2026.08-5 -c 'import pandas, …, stdnum; print("OK")'` → `OK`（exit 0）。17 個套件全數 import 成功，**在無網路、非 root 下**。 |
| **2. 全數經閘道；金鑰撤銷後回 401** | ⛔ **未跑** | 需要模型棧（`task dev:model`），會產生費用。 |
| **3. usage 發出條件、caching 欄位與對帳＋trace 管線** | ⛔ **未跑** | 同上。 |
| **4. Skill 載入條件（人手跑）** | ⛔ **未跑** | 同上，且本項無可執行形式。 |

**另外兩項不在四項清單上、但這次一併量了，因為它們是別處的前提**：

| 量了什麼 | 結果 | 為什麼要量 |
| --- | --- | --- |
| `command -v nc` | **`NO_NC`** | 2026-08-29 的稽核記載 P-02 探針曾經用 `nc`，而這個映像**從來沒有 `nc`**——探針因此永遠回報通過。`-5` 沒有把它加回來，所以 A1 改用 `node -e` 的那個修法在這一版仍然必要且成立。 |
| `node --version` | `v22.23.2` | 上一格那個修法的前提。 |

**這一節仍然是紅燈，`-5` 仍然不得合併到 main**：四項裡跑完的是最便宜的一項，而**它證明的是依賴集沒壞，不是這次變更（解壓上界）在真實執行下沒有回歸**。剩下三項都要模型棧。

**`apps/sandbox/cmd/sandboxd/main.go:45` 的預設映像仍是 `2026.08-3`，`.github/workflows/ci.yml` 的 `RUNTIME_IMAGE_FOR_PROBE` 也仍是 `-3`。** 兩處要與 digest、剩下三項實測同批改——`-5` 今天只存在於一台開發機上，registry 沒有它。

### `schema_version` 未動：仍是 `1.1`

稽核 04 D2 記錄了「`run.mjs` 宣告 1.1 而 schema 已到 1.2」。查過
`contracts/events/README.md` §「版本宣告的規則」後**確認不改**：producer 宣告的是它
「照哪一版契約寫」，1.2 新增的是 `evaluation_started`／`evaluation_completed` 兩個型別，
且兩者的 `emitted_by` 限 `orchestrator`——沙箱 harness 永遠不會發它們。harness 會寫
`usage.token_source`（1.1 新增），所以 `1.1` 正是規則要它宣告的那一版。改成 `1.2` 會宣告
一個這個 producer 不寫的版本，是把一致性做壞而不是做好。

### ⚠️ 四項實測待跑（需要閘道費用，約 $0.02）— 未跑前不得合併到 main

ADR-023 §1 的定義：**任何會改變 image digest 的變更都是升級**，§2 的四項清單要在**新
digest 上實跑**，§3 明文禁止以推理或既有證據代替。本次改的是 `run.mjs`，而 `run.mjs` 是
`COPY` 進映像的（`Dockerfile:141`），所以 digest 會變，四項一項都不能省。

尤其不能省的是**測項 1**：`-4`／`-5` 改的正是 skill 套件解壓，也就是「Skill 載入條件」的
**上游**。`04` 丙-82 做過的差分（同一個 zip 兩種解法逐位元組相同）是**針對解壓行為**的
證明，不是四項；ADR-023 §3 禁止的就是拿它頂替。

跑法（比照 `-3` 那一節的煙霧規模：單一 Run、`gpt-5.4-mini`、實測 $0.0202，上限 $0.3）：

```bash
# 0. 建映像並記下 digest（digest 才是事實來源，ADR-023 決策 1）
docker build -t skillhub/runtime-agent-sdk:2026.08-5 infra/images/runtime-agent-sdk
docker image inspect skillhub/runtime-agent-sdk:2026.08-5 --format '{{.Id}}'

# 1. 依賴集沒變，但仍以新 digest 跑一次 import 檢查（與 -3 節同一條）
docker run --rm --network none --user 65532:65532 --entrypoint python3 skillhub/runtime-agent-sdk:2026.08-5 -c 'import pandas, numpy, openpyxl, lxml, dateutil, docx, pptx, pypdf, pdfplumber, pycountry, chardet, defusedxml, ftfy, confusable_homoglyphs, pytz, phonenumbers, stdnum; print("OK")'

# 2. 起模型棧（會產生費用，必須由負責人執行，不得由唯讀 SubAgent 啟動）
task dev:model

# 3. 測項 3／4（usage 發出條件、caching 欄位與對帳）＋ trace 管線，跑在真映像上
export SKILLHUB_SANDBOX_TEST_IMAGE=skillhub/runtime-agent-sdk:2026.08-5
go -C apps/sandbox test ./internal/dockerdrv/ -count=1 -v -run 'TestHarnessReportsUsageForACompletedTurn|TestHarnessStopsAtTheTokenCeilingAndStillReportsUsage|TestTraceEventsReachTheCollectorFromARealContainer'

# 3b. 測項 1（Skill 載入條件）今天沒有可執行形式，是人手跑的：掛一個最小 Skill 套件
#     （SKILL.md ＋ 一支 scripts/check.py），確認 trace 有 skill_activation
#     {"decision":"activated"}，且 script_log 顯示套件內的腳本真的被執行過（不是模型
#     轉譯的重寫）。這一項是本次變更的直接下游，最不能省。比照 -3 節「實測：ADR-023
#     四項」那張表逐項記下輸出。

# 4. 測項 2（全數經閘道；金鑰撤銷後回 401）的既有可執行形式
go -C apps/platform test ./internal/entrypoint/api/apiserver/ -count=1 -v -run TestEndToEndRunCallsTheModelThroughItsOwnVirtualKey
```

跑完要回填的欄位：映像 digest、commit、CI run、以及上方四項的逐項實測輸出（比照 `-3`
那一節的表格形式：斷言／實測輸出／判定）。**在那之前這一節就是紅燈**，`-5` 不得合併。

### 本次未涵蓋的（明說）

- **`apps/sandbox/cmd/sandboxd/main.go:45` 的預設映像仍是 `2026.08-3`**，本批**沒有**改。
  理由是 `-5` 的映像還沒被建置，把預設指向一個本機與 registry 都不存在的 tag，會把
  「文件落後」換成「非 runsc 節點一啟動就拉不到映像」。這一項的處置與 digest 一起做：
  建好 `-5`、跑完四項、再同批改預設值（或依稽核 04 D1 的建議改成「沒有預設，未設就退出」）。
- **UPGRADES.md 與 `ARG IMAGE_VERSION` 的機械對帳仍不存在**。稽核 04 D3 的建議①
  （`devctl automation-check` 讀 Dockerfile 的版本字串、斷言本檔有同名章節，形式比照
  `isolation_levels.go`）**本批未做**——`tools/` 不在本次的可寫範圍內。這一節是人手補的，
  下一次漏掉時仍然沒有機器會說話。
