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

## `2026.08-3` → `2026.08-5`（2026-08-29；2026-08-30 實測完成）— **四項實測全跑，跑在 GHCR 上的那個 digest**

> **這一節為什麼跨兩版**：`-4` 在 2026-08-29 隨 04 丙-82 落地（拿掉 `unzip`、`run.mjs`
> 自寫 ZIP 解析器），**但本檔當時沒有補節**——`.github/workflows/runtime-image.yml`
> 只檢查「Dockerfile 內容變了就要有 `ARG IMAGE_VERSION=` 的 diff」，沒有任何 job 檢查
> UPGRADES.md 有沒有跟上，也沒有任何 job 檢查四項實測有沒有跑（稽核 04 D3）。
> `-5` 在同一批補上解壓上界。所以這裡合成一節，並把 `-4` 的內容誠實列在變更欄裡，
> 而不是假裝它當時被記錄過。
>
> **2026-08-30 訂正一句**：上一段原本寫「兩次變更之間**沒有任何映像被建置或發佈**」。
> 那是**推理，不是查證**——`runtime-image.yml` 由 `infra/images/` 的 diff 觸發，`-4` 與
> `-5` 都改了 `run.mjs`，所以它跑了，而且成功發佈了。`-5` 從 2026-08-29 起就在 GHCR 上。
> 這句錯話直接造成隔天的兩個誤動作：四項實測先跑在一份本機建置上（等於白跑），
> 而 `main.go` 的預設映像被留在 `-3`，理由是「`-5` 拉不到」——它一直拉得到。

| 欄位 | 值 |
| --- | --- |
| 變更（`-4`） | `Dockerfile`：移除 `unzip`。`run.mjs`：`extractPackage` 改為自寫 ZIP 解析器（中央目錄 + local header），逐條拒絕絕對路徑／`..` 路徑段（含反斜線變體）／非一般檔（symlink・device・fifo，以 external_attr 判）／zip64／加密／不支援的壓縮法 |
| 變更（`-5`） | `run.mjs`：解壓炸彈上界三道——①每個 deflate 條目以中央目錄宣告的 `uncompressedSize` 當 `inflateRawSync` 的 `maxOutputLength`；②中央目錄宣告總量上界 `MAX_PACKAGE_TOTAL_BYTES = 256 MiB`，**在解壓與寫檔之前**就拒；③條目數上界 `MAX_PACKAGE_ENTRIES = 1000`（與 `artifacts.go` 的 `artifactMaxEntries` 同值同理由）。附帶：stored（method 0）條目的兩個 size 欄位必須相等，否則拒——這是讓①②對 stored 分支也成立的那一條 |
| 為什麼 `-5` 是升級而不是整理 | `unzip` 對壓縮比同樣沒有防護，所以這不是回歸；但 `-4` 把這道邊界搬到了**沒有 cgroup 接住的平台**（clean mode 是主機行程：Linux 無 root 時零資源強制，Windows 的 Job Object 會直接終結整個 job，Run 變成沒有 trace、沒有產出、沒有原因的失敗）。接手一道邊界的時機就是加上界的時機 |
| SDK 版本 | `0.3.233`（**未變**） |
| 基底 digest | `sha256:d649c27dae7ba0137b3cef5dd75baa422c08dc3d9e3fc0c23dfb172dc3cc6436`（**未變**） |
| 映像 digest | `sha256:ba2bc95e5ff65510369a7b366f96eed629a8128e1c585bd5d1db168eceb7d13e`（`ghcr.io/arthurc02/skillhub-runtime-agent-sdk:2026.08-5`，由 [runtime-image #33258533387](https://github.com/ArthurC02/SkillHub/actions/runs/33258533387) 於 2026-08-29 隨 `f3f8bb0` 發佈）。<br>**2026-08-30 訂正**：本節原記為「本機建置、尚未推上 GHCR」，`sha256:37c08f48…`。那句是錯的，而且錯得有代價——**registry 一直有這個 tag**，是 `-4`／`-5` 的 `run.mjs` 變更觸發了 `runtime-image.yml`。四項實測當天先跑在本機那份上，發現之後**整組重跑在上面這個 digest 上**，因為 ADR-023 決策 1 說事實來源是 digest，而沒有人會去 `docker run` 一台開發機上的建置產物。兩份不可能位元組相同（本專案的映像不是可重現建置），所以「本機跑過」不算數。 |
| 依賴集 | **未變**（`-3` 的 17 個 Python 套件，0 增 0 減） |
| commit | `-4`／`-5` 的 `run.mjs` 在 2026-08-29 之前的批次落地；本節的實測紀錄與 `main.go`／`ci.yml` 的預設值調整同批 |
| CI | [runtime-image #33258533387](https://github.com/ArthurC02/SkillHub/actions/runs/33258533387)（`publish` **success**；`rescan`／`review` skipped） |

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

### 2026-08-30：ADR-023 §2 四項，全部跑在 `sha256:ba2bc95e…` 上

跑法與 `-3` 那一節同規模（單一 Run、`gpt-5.4-mini`、每次 $0.01～0.03）。**環境**：本機
LiteLLM（`task dev:model`）＋ `skillhub_egress` ＋ 主機上的 `sandboxd`
（`SKILLHUB_SANDBOX_IMAGE=ghcr.io/arthurc02/skillhub-runtime-agent-sdk:2026.08-5`）。

| 項次 | 狀態 | 實測輸出 / 判定 |
| --- | --- | --- |
| **1. 依賴集以新 digest 跑 import 檢查** | ✅ **通過** | `docker run --rm --network none --user 65532:65532 --entrypoint python3 ghcr.io/…:2026.08-5 -c 'import pandas, …, stdnum'` → `OK 17/17`（exit 0）。**在無網路、非 root 下**。 |
| **2. 全數經閘道；金鑰撤銷後回 401** | ✅ **通過** | `TestEndToEndRunCallsTheModelThroughItsOwnVirtualKey` PASS：真套件進 SeaweedFS → preflight → 確認 → 派送 → 沙箱容器上 `skillhub_egress`（唯一可達位址是閘道）→ `cost_source=gateway`、`cost_usd=$0.022106` → artifact 收回 → `cleanup_status=cleaned`。<br>撤銷單獨量了一次：同法鑄的金鑰在撤銷前 `/key/info` **200**，撤銷後 `/key/info` 與 `/v1/messages` 皆 **401**。 |
| **3. usage 發出條件、caching 欄位與對帳** | ✅ **通過** | `TestHarnessReportsUsageForACompletedTurn` PASS（`in=16408 out=25 token_source=result cost=0.01277925/gateway`）；`TestHarnessStopsAtTheTokenCeilingAndStillReportsUsage` PASS（撞上限仍回報 `in=16408 out=28 cost=0.025572`）。<br>**對帳逐分錢一致**：閘道 `LiteLLM_SpendLogs` 該金鑰四列合計 **$0.02557200**＝harness 回報值。<br>**caching 欄位仍全為 `null`**（不是 0），與 `-3` 同；PDM-005 §5.2a-7 的假設在這一版依然不成立。 |
| **4. Skill 載入條件（含套件內腳本真的被執行）** | ✅ **通過** | trace：`skill_activation {"skill_name":"run-marker","decision":"activated"}` → `tool_call` `Skill` → `script_log {"stream":"stdout","message":"SKILLHUB-SCRIPT-RAN py3.11"}` → `tool_call` `Bash` `cd /work/.claude/skills/run-marker && python3 scripts/check.py` → 兩次 `Write` → `agent_output` final `DONE`。**套件是兩個條目、第二個在子目錄下**，所以這一項同時走過 `-5` 改的那段 ZIP 讀取。 |

**本項自 2026-08-30 起有可執行形式**，這是 `-5` 這一節與前幾節最大的差別。以前第 4 項
逐版都記著「人手跑」，於是它逐版都靠一個人記得去跑。現在
`e2e_gateway_integration_test.go` 的 `e2eSkillPackage` 帶了一支 `scripts/check.py`，
測試斷言封存裡出現 `SKILLHUB-SCRIPT-RAN py3.`——**版本尾巴是關鍵**，那個字串只有真的執行
過映像自己的直譯器才生得出來，讀原始碼抄不出來。**做過突變**：把 SKILL.md 那三行指令
拿掉重跑，該斷言變紅（`the archive carries no output from the package's own script`），
marker 那一半仍綠，然後改回來。

**另外兩項不在四項清單上、但一併量了**：

| 量了什麼 | 結果 | 為什麼要量 |
| --- | --- | --- |
| `command -v nc` | **`NO_NC`** | 這個映像從來沒有 `nc`，所以 P-02 探針改用 `node -e` 的修法在這一版仍然必要且成立。 |
| `node --version` / `python3 --version` | `v22.23.2` / `3.11.2` | 上一格那個修法的前提，以及第 4 項那個版本尾巴的來源。 |

### 同批修好的一件事：那兩支付費測試從 A1-e 落地之日起就跑不動

`harness_token_e2e_test.go` 建 `sandbox.Manager` 時**沒有給 `EgressAllow`**。ADR-022 A1-e
之後，沒有 rendered destination 的節點會在任何容器啟動之前就拒絕派送
（`capability_mismatch: … it routes to nothing (no destination has a pinned address)`）。
**而這件事一直沒有人看見，因為沒設那兩個 gateway 變數時這兩支會 skip，而 skip 長得像 pass。**
現在 destination 由受測的 URL 推導出來，不是寫死的。

### 預設映像同批從 `-3` 移到 `-5`

`apps/sandbox/cmd/sandboxd/main.go` 的預設、`.github/workflows/ci.yml` 的
`RUNTIME_IMAGE_FOR_PROBE` 與它 `docker tag` 出來的名字、以及
`p02_docker_test.go` 的 `runtimeImage` 常數——**四處同批**。第三與第四處必須一起動：CI 拉下
映像後改的那個 tag，就是 P-02 探針測試 inspect 的那個名字，只改一邊探針會退回 busybox，
而 busybox 有 `nc`，正是這支測試存在的理由。

### 本次未涵蓋的（明說）

- **UPGRADES.md 與 `ARG IMAGE_VERSION` 的機械對帳已經存在**（`devctl automation-check` 的
  `image-version`，讀 Dockerfile 的版本字串、斷言本檔有同名章節）。它**只查得出章節在不在**，
  查不出四項有沒有真的跑——`image_version.go` 自己的檔頭就寫著這一點。本節上面那張表是人手填的。
- **四項實測本身仍然沒有機器會催**。要讓它有，得有一支像 `test_gateway_live.py` 那樣的付費
  opt-in 測試，把四項串成一次 `go test`；今天第 1、2、3、4 項各自有可執行形式，但**沒有一個
  入口把它們綁在一起**，所以下一次仍然靠一個人記得。

## `2026.08-5` → `2026.08-6`（2026-09-02）— **四項實測尚未跑;預設映像刻意留在 `-5`**

> **這一節的狀態與上一節相反,而那正是要寫清楚的地方**:`-5` 是「實測全跑完才寫」,
> 本節是「變更已合、實測還沒跑」。ADR-023 §3 明文禁止以推理或既有證據代替新 digest
> 上的實測,所以本節不主張任何一項通過,並在 [`04` 丙-125](../../../docs/plans/04-backlog-and-handoffs.md) 開一列等人做。
>
> **為什麼還是合了**:這次改的是 `run.mjs` 的 agent 選項,而**淨測試模式不經過映像**
> ——`localdrv` 直接以主機路徑執行 repo 裡的 `run.mjs`(`Config.RunnerScript`),所以
> demo 那條路徑立刻拿到修正,不必等映像。容器那條路徑等本節的四項。

| 欄位 | 值 |
| --- | --- |
| 變更 | `run.mjs`:新增 `outputContract()` 與 `agentOptions()`,並把 SDK turn 的 `systemPrompt` 從**隱含的空字串**改成一段**只講產出目錄**的文字。修的是 [`04` 丙-106](../../../docs/plans/04-backlog-and-handoffs.md):兩個 driver 都只收 `<outDir>/artifacts`,而**從來沒有人告訴 agent 這件事** |
| 為什麼是升級而不是整理 | 它改變**每一次 Run 的模型輸入**。在此之前 agent 收到的 system prompt 是空的;之後是一段文字。M2 的 45 筆基準 Run 與 M3 的評估都是在**空的**那一版下產生的,重跑同一批可能得到不同的產出位置與敘述——**那是預期中的改善,不是回歸,但它使「與基準逐字比對」不再是同一個實驗** |
| SDK 版本 | `0.3.233`(**未變**) |
| 基底 digest | **未變** |
| 映像 digest | **待填**——`runtime-image.yml` 由 `infra/images/` 的 diff 觸發,所以推上去就會建並發佈;**四項實測必須跑在那個 digest 上**,不是本機建置(`-5` 那次的教訓逐字在上一節) |
| 依賴集 | **未變**(0 增 0 減) |
| 預設映像 | **刻意仍是 `-5`**:`sandboxd/main.go` 的 `SKILLHUB_SANDBOX_IMAGE` 預設、`ci.yml` 的 `RUNTIME_IMAGE_FOR_PROBE`、`p02_docker_test.go` 的常數三處都沒有動。移到 `-6` 是四項實測通過之後的動作,不是這一批的 |

### 為什麼 `systemPrompt` 是字串而不是 `preset` + `append`

**量出來的,不是推的**——SDK `0.3.233` 自己的映射是
`if (s === undefined) p = ""; else if (typeof s === "string") p = s; else if (s.type === "preset") { f = s.append }`。
所以:

- **省略 = 送出一個空的 system prompt**,不是 Claude Code 的預設提示詞。這是本檔案自
  2026-08 以來每一次 Run 的實況。
- **字串 = 用一段文字取代那個空字串**,其他一律不變。這是最小的差。
- **`{ type: "preset", preset: "claude_code", append }` = 把整份 Claude Code 系統提示詞
  接上**。那是另一個產品的行為,以「順便講一下產出目錄」的名義進來,連 token 帳都會變。

### 已經跑過的(不是四項清單,但先記著)

**2026-09-02,對本機 LiteLLM 的 `gpt-5.4-mini` 各跑一次真的 Run**,同一個任務
(「建立 announcement.md」)、同一份 harness、只差 `systemPrompt` 一行:

| | 檔案落點 | `GET /runs/{id}/artifacts` 會拿到 | Run 狀態 |
| --- | --- | --- | --- |
| **有** `outputContract` | `…\out\artifacts\announcement.md` | 一個檔 | `succeeded` |
| **沒有**(還原成今天的映像) | `…\work\announcement.md`(cwd) | **空的** | `succeeded` |

**下面那一列就是 `-5` 映像今天的行為**,而它**不是 Windows 專屬**:agent 用的是工作目錄,
在容器裡那是 `/work`,一樣不會被收走。丙-106 記成「`/out` 只在容器裡成立」是對的但不夠——
**真正的洞是產出目錄從來沒有被告訴任何人**,而 Windows 只是讓它現形。

**這兩次跑不算四項實測**:它們跑在**主機的 `run.mjs`** 上,不是新 digest 的容器裡。
