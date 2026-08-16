# Runtime Image 升級紀錄（ADR-023 §4）

本檔是 [ADR-023](../../../adr/ADR-023-agent-sdk-version-pinning-and-behaviour-revalidation.md)
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
| 映像 digest | 見 `../README.md`「已發佈的 digest」；GHCR tag `2026.08-2` |
| 證據來源 | [content-baseline-report.md §12／§13](../../../plans/mvp/m2/content-baseline-report.md) |
| 實測規模 | **45 個 Skill 全量**（§12 的 9 筆 ＋ §13 的 36 筆），閘道實付 $0.86 ＋ $1.85 |

| # | 結果 | 關鍵輸出 |
| --- | --- | --- |
| 1 | **通過** | 45/45 Run 皆有 `skill_activation`（§12.3、§13.2 逐筆表）。相容軸回填 45 列 `capability=activated`（§13.5） |
| 2 | **通過** | 45 個 Run 的模型呼叫全數經 LiteLLM；trace 回報成本與閘道 per-key spend 對帳（§13.6：trace $1.8166 vs 閘道實付 $1.8519）。**撤銷後 401 當時未單獨驗證**——該斷言的既有可執行形式是 `e2e_gateway_integration_test.go`，本次（`-3`）已補跑，見下節 |
| 3 | **通過（且記錄為缺欄）** | `cache_read_input_tokens`／`cache_creation_input_tokens` 在 LiteLLM 的 `/v1/messages` 路由上**缺欄不是 0**，與 PDM-005 §5.2a-7 記載一致，未變動。`input_tokens` 上限強制點因此仍等於 summary 顯示的數字 |
| 4 | **行為已改變，且是本批修的** | §13.2 `add-iso3166` 在 `2026.08-1` 那輪**沒有 `usage` 事件**（成本無聲缺席，§7.2 #4），本批修正後有值。`run.mjs` 現在於 `result`／`accumulated` 兩條路徑都發出，`token_source` 欄位區分兩者（事件 schema 1.1） |

**是否推翻既有文件敘述**：是——`run.mjs` 檔頭原本記載 usage 只掛 `result` 分支，該敘述已
在同批修正（見檔頭 token accounting 節）。`services/sandbox/README.md` 無需更動。

**附帶量測**（非四項清單，但屬同一次升級的結論）：33 筆 `transpiled` → `native`；輸入
token −50%、成本 −28%（§13.3）；`pandas` 3.x 只觸發一則相容性告警。

---

## `2026.08-2` → `2026.08-3`（2026-08-16）

| 欄位 | 值 |
| --- | --- |
| 變更 | **依賴集**：新增 8 個純 Python 套件（`pycountry` 26.2.16、`chardet` 7.6.0、`defusedxml` 0.7.1、`ftfy` 6.3.1、`confusable-homoglyphs` 3.3.1、`pytz` 2026.3.post1、`phonenumbers` 9.0.37、`python-stdnum` 2.2）。既有 9 個版本未變，**0 移除** |
| 起因 | 目錄 `deps` 欄位漏抄，映像照抄了漏的那份（`../README.md`「依賴集」節） |
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
`services/sandbox/README.md` 均無需更動。

### 本次未涵蓋的（明說）

- **45 筆全量相容軸重測未跑**：`skill_runtime_compatibility` 對 `2026.08-3` 目前 **0 列**，
  目錄仍顯示 `2026.08-2` 的結論並附映像標籤。依賴集只增不減使結論極可能不變，但
  0022 的鍵不接受「極可能」。約 $2.2（§13.6 實測基準），已列 `03` 工作項。
- **被拒收依賴的影響未做行為量測**：`pdf`／`pii-flag`／`document-format-skills` 失去哪些
  支路是由靜態掃描推得，非實跑；三筆在 `2026.08-2` 上皆判「符合」（走可用子集）。
