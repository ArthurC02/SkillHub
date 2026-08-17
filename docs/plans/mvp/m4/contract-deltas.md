# M4 契約增量清單（第 1 批）

- 日期：2026-08-17
- 狀態：**清單，未實作。** 依鐵律 12「跨語言介面先寫 OpenAPI schema 再實作；CI 以 codegen 檢查 drift」。
- 用法：本文件只列**要動哪個檔、加哪些 path／schema、每個 schema 的形狀與存在理由**。**不寫 YAML／JSON 實體**——實體由第 1 批的三個 agent 各自在自己的檔案內產出。
- 三組檔案互不重疊，可三個 agent 平行：

  | # | 檔案 | 節 |
  | --- | --- | --- |
  | 1 | `contracts/openapi/public.yaml` | §1、§2 |
  | 2 | `contracts/packaging/*.schema.json` ＋同目錄 README（**新目錄**） | §3 |
  | 3 | `db/migrations/0027_packaging.sql` ＋ `db/queries/` | §4（非契約，但同批） |

- 形狀依據見 [packaging-design.md](packaging-design.md) 與 [beta-design.md](beta-design.md)；里程碑範圍與未決點見 [README.md](README.md)。

---

## 0. 先講三件已經在庫裡、不要重做的事

第 1 批最容易犯的錯是「照 ADR 從零設計資料模型」，而 M0～M2 已經先鋪好了三樣東西：

| 已存在 | 位置 | 對第 1 批的意義 |
| --- | --- | --- |
| **`artifacts` 表已經預留了下載套件** | `0004_test_lab_and_runs.sql`：`kind IN ('run_output','download_package')`、`run_id` **可為 NULL**（註解逐字寫「NULL for packaging downloads (PACK-001)」）、`scan_status` 三態、`expires_at` 註解已寫「Run output 30 days, download package 90 days (PDM-006 6)」 | **不要新建一張 `download_artifacts` 表。** `0027` 只補這張表答不出來的東西（§4） |
| **License 溯源四層已在 `skill_versions`** | `0012` ＋ `0014`：`license_expression`／`license_source ∈ {manifest, manifest-referenced-file, package-license-file, repo-license-file}`，配對 CHECK | manifest 的 `license` 區塊直接映射這兩欄，**不重新定義層級** |
| **打包目標檔名常數已匯出** | `skillpkg.CarriedLicenseFile = "LICENSE.repo"`、`CarriedProvenanceFile` | 匯出器引用常數，不寫字面字串（ADR-021 已為打包器↔掃描器的名單漂移付過一次代價） |

---

## 1. `public.yaml`：新增 path

所有端點皆為 **workspace-scoped，scope 取自 session**（鐵律 3）；非擁有者一律 **404**（沿用 `CORE-006` 既有慣例，不新增 403 語意）。

### 1.1 打包

| Path | Method | 形狀摘要 | 需求 |
| --- | --- | --- | --- |
| `/packaging/targets` | GET | 回 `PackagingTarget[]`：三個打包目標的 id、類型（`standard` \| `profile`）、版本、安裝位置說明、支援狀態。**是端點不是前端常數**——`02:PACK-002` 要求下載頁顯示「目標 Agent、支援狀態」，而支援狀態會隨實測改變，寫死在前端等於讓兩份真相 | `02:PACK-002` |
| `/skills/{id}/versions/{versionId}/packaging/preview` | GET | query `target=`。回 `PackagingPreview`：`allowed: boolean` ＋ `blocked_reason?` ＋ 重驗結果 ＋ 將包含／排除的 Test Case 清單。**預覽跑的是打包會跑的同一套檢查**（同 `EVAL-009` 的 diff 預覽與套用共用 `check()` 的既有前例），否則預覽說可以、打包時被拒 | `02:PACK-001` |
| `/skills/{id}/versions/{versionId}/packaging` | POST | body `{target, include_test_cases: boolean}` → 建立一筆 Download Artifact。回 `DownloadArtifact`。**冪等**：同一 (version, target, include_test_cases, packager_version) 已有未過期且 `available` 的 artifact 時回既有那一筆並標 `duplicate: true`（同 `EVAL-010` 的 `duplicate` 前例）——打包是冪等的，重按一次不該產生第二份位元組 | `02:PACK-001` |
| `/downloads` | GET | 回 `DownloadArtifact[]`，本 workspace 的全部。這是 `02:WS-002` 第 1 條「使用者可查看……下載紀錄」的列表面 | `02:WS-002`、`03` WS-004 |
| `/downloads/{artifactId}` | GET | 回單筆 `DownloadArtifact`（含到期日與 `download_count`） | 同上 |
| `/downloads/{artifactId}/content` | GET | **平台代傳位元組**（`Content-Disposition: attachment`），不回預簽 URL——理由見 [packaging-design.md](packaging-design.md) §7.1。`scan_status != 'available'`、已過期、或 `access_restriction` 非 NULL 一律 **404**（不是 403：存在性本身是私有的，沿用既有慣例） | `02:PACK-001`、ADR-003 |
| `/downloads/{artifactId}` | DELETE | 使用者主動刪除自己的 Download Artifact（`02:WS-002` 第 3 條、`NFR-002`）。冪等：不存在也回 204 | `02:SEC-006` |

**刻意不新增的端點**：

- **沒有「重新打包」端點**。重打包就是再 POST 一次 `/packaging`，冪等鍵不同（例如換了 target）就得到新的一筆。多一個端點等於多一條繞過 §1.1 那四道檢查的路——同 M3「不新增重跑端點」的既有裁定。
- **沒有預簽 URL 端點**。見上。

### 1.2 封測面（依 [beta-design.md](beta-design.md)）

| Path | Method | 形狀摘要 | 需求 |
| --- | --- | --- | --- |
| `/me/quota` | GET | 回 `RunQuota`：`remaining_today`／`remaining_window`／`window_resets_at`／`limits`。**先做強制點再做這個端點**（乙-2 的教訓：顯示一個不會被執行的上限是最壞的一種） | PDM-010 |
| `/feedback` | POST | body `{kind: blocking_issue\|need_signal, message, page_path?, run_id?}`。三個入口（未受邀、額度耗盡、阻斷回報）共用一支端點，由 `kind` 分流 | `03` BETA-003／004／005 |

**`RunPermissionSummary` 加一個 additive 欄位**：`quota`（剩餘次數與重置時間）。放進權限摘要是因為它與 `estimated_cost` 是同一類資訊（這次 Run 要花掉什麼），**但同 `estimated_cost` 一樣刻意不進 `summary_hash`**——配額是狀態不是權限，它變動不該作廢使用者手上的確認（`03` TEST-011 已立下這條規則）。

---

## 2. `public.yaml`：新增 schema

| Schema | 形狀要點 | 為什麼是這個形狀 |
| --- | --- | --- |
| `PackagingTarget` | `id`(`standard`\|`claude-code`\|`claude-agent-sdk`)、`kind`(`standard_package`\|`profile`)、`version`、`display_name`、`install_location`、`support_status`(`verified`\|`unverified`)、`notes[]` | PDM-008：對外一律表述為「1 標準套件 ＋ 2 已驗證安裝 Profile」，所以 `kind` 必須分得出標準套件與 Profile，**不能用一個 `profiles[]` 陣列把三者混為一談** |
| `PackagingPreview` | `allowed`、`blocked_reason?`、`validation: PackageValidation`、`included_test_cases[]`、`excluded_test_cases[{name, reason}]` | `blocked_reason` 的值域是四道鎖：`license_hold`（`access_restriction`）／`not_redistributable`／`license_unknown`／`validation_blocked`。**四個分開而不是一個 `blocked: true`**——使用者能不能自己解決取決於是哪一個 |
| `PackageValidation` | `blocked: boolean`、`errors[]`、`warnings[]`、`infos[]`（每筆 `{code, path, message}`） | 直接映射 `skillpkg.Report` 的既有三級，**不重新定義 severity**（`02:SKILL-002`「分開呈現阻擋錯誤與可接受警告」） |
| `DownloadArtifact` | `artifact_id`、`skill_id`、`skill_version_id`、`target`、`file_name`、`size_bytes`、`content_hash`、`manifest_hash`、`status`(`quarantined`\|`available`\|`rejected`)、`expires_at`、`created_at`、`download_count`、`includes_test_cases` | **兩個雜湊都在**（[packaging-design.md](packaging-design.md) §2.4）：`content_hash` 答「是不是這個檔」，`manifest_hash` 答「內容是不是一樣」。`status` 直接是 `artifacts.scan_status`，**不映射成 `ready: boolean`**——`rejected` 與 `quarantined` 對使用者是兩件事 |
| `SkillCompatibility` | `format`(`valid`\|`invalid`)、`capability`(`activated`\|`not_activated`\|`unverified`)、`behaviour`(`native`\|`transpiled`\|`failed`\|`unverified`) ＋ `runtime_image?`／`measured_at?` | ADR-012 的三層相容性。**後兩軸的值域與 `0022` 的 `skill_runtime_compatibility` 逐字相同**，不另造一套；`behaviour` 沒有實測列就是 `unverified`，**不得外推**（乙-4 的既有裁定） |
| `RunQuota` | `remaining_today`、`remaining_window`、`window_resets_at`、`limits: {daily, window, window_days, concurrent}` | 值來自 PDM-010；`concurrent` 是**既有**的強制（`gateb.go`），列在這裡是為了讓四個限制在同一個地方被看到 |

---

## 3. `contracts/packaging/`：新目錄，三份 JSON Schema

**這是 `contracts/` 第一次承載非 OpenAPI、非 trace 事件的契約。** 形狀比照 `contracts/events/` 的既有慣例：JSON Schema ＋同目錄 README ＋明文的版本演進規則（additive、舊版仍為新版的合法實例、只寫舊欄位的產生者繼續宣告舊版本）。

理由是這三份 schema 的消費者**有一個在 repo 外**：拿到 zip 的使用者（和他的工具）要讀 manifest。放進 OpenAPI 表達不了「這是一個檔案的格式」。

| 檔案 | 內容 | 消費者 |
| --- | --- | --- |
| `download-manifest.schema.json` | `skillhub-manifest.json` 的形狀，欄位逐項見 [packaging-design.md](packaging-design.md) §4.1 | Go（產生）、`apps/web`（顯示）、**使用者與其工具**（驗證） |
| `packaging-profile.schema.json` | Agent Packaging Profile 的版本化設定：安裝位置、frontmatter additive 欄位、環境變數範本、驗證 Prompt、已知限制。**只描述設定，不描述 Adapter 程式**（ADR-012「Profile 應為版本化設定與程式 Adapter 的組合」） | Go（讀設定）；三個內建 Profile 各一份實體 |
| `portable-test-case.schema.json` | `test-cases/<slug>/case.json` 的形狀（[packaging-design.md](packaging-design.md) §5.2） | **兩個方向**：打包器匯出、`tools/content/` 的種入腳本匯入（丙-12） |

**`download-manifest` 有三條必須寫進 schema description 的界線**（是契約文字，不是註解）：

1. **`compatibility.behaviour` 的值只能來自該 (Skill Version × Runtime Image) 的實測列**；沒有列就是 `unverified`。schema 要說明「本欄不是承諾，是一次量測的紀錄」。
2. **`license.expression` 與 `license.source_tier` 同生同滅**（ADR-021 決策 1 的 CHECK 在 DB，這裡是它的對外形式）。`expression` 為 null 時 `source_tier` 必須為 null，且**不得填 `NOASSERTION` 字串**（ADR-021 決策 7）。
3. **`manifest_hash` 不含 manifest 自身**，且不含任何 zip metadata——否則它答不了「兩次打包的內容一不一樣」（[packaging-design.md](packaging-design.md) §2.4）。

---

## 4. `0027_packaging.sql`：只補 `artifacts` 答不出來的東西

`artifacts` 已經能存一筆下載套件（§0）。**缺的是三樣**：

| # | 內容 | 為什麼 `artifacts` 答不出來 |
| --- | --- | --- |
| 4-a | `download_artifacts` 的**打包側屬性**：`artifact_id`（PK／FK）、`skill_version_id`、`target`、`profile_version`、`packager_version`、`manifest_hash`、`includes_test_cases` | `artifacts` 是「一個檔案」的通用表（Run 產出與下載套件共用），把打包專屬的六個欄位塞進去會讓 `run_output` 那半永遠是 NULL。**一對一側表，不是第二張 artifacts 表** |
| 4-b | `download_records`：誰、何時、下載了哪一筆 | **`02:WS-002` 要的「下載紀錄」與 audit event 是兩件事**，保存期限與可見性都不同（[packaging-design.md](packaging-design.md) §7.2）。合併會讓兩邊都被較嚴的那個綁住 |
| 4-c | **可散布性欄位**（見下） | 目前**資料庫回答不了「這個 Skill 可不可以再散布」** |

### 4.1 可散布性：`0027` 最需要拍板的一列

盤點結論：`skills`／`skill_versions` **沒有** `redistributable`、`source_available` 或 `license_status` 欄位。可散布性目前只活在三個地方，沒有一個在資料庫裡：

- `tools/content/seed-skills.json` 的策展欄位 `redistributable`（其註解逐字寫「true = Download Artifact may be produced (02:PACK-001)」）；
- `catalog/trust.go` 的**註解**（「Source-available licenses reach `LicenseStatusConfirmed` but still block Download Artifact generation — Confirmed means "verified", not "redistributable"」）；
- `0023_access_restriction.sql` 第 22 行的一句話：「**Download packaging is unaffected here**」——那句話在 M4 之前為真，只因為沒有打包功能。

**建議形狀**：`skills.redistribution text CHECK (redistribution IN ('allowed','blocked','unknown')) NOT NULL DEFAULT 'unknown'`，與 `access_restriction` 同層、隨 Fork 複製、**只有 `allowed` 放行**。

**為什麼是 `skills` 不是 `skill_versions`**：同 `0023` 的既有裁定（授權事實屬於來源，且 `skill_versions` 不可變、放不進一個可撤銷的判定）。**為什麼預設 `unknown` 而不是 `allowed`**：`02:DISC-003` 的「授權未知不得暗示可自由修改或再發佈」在放行方向上錯不起——ADR-021 §5.3 記錄的那個誤判（「repo 根有 MIT ⇒ 子目錄是 MIT」）錯的正是這個方向。

**兩道鎖都要**（[packaging-design.md](packaging-design.md) §4.5）：`access_restriction`（人工 hold，涵蓋今天已知的四筆）與 `redistribution`（內容屬性，對每一個 Skill 都要有答案）。拿 hold 當可散布性判準，等於宣稱「沒有人特別擋它就是可以散布」。

**回填**：45 個種子 Skill 的事實已在 `seed-skills.json`，腳本比照 `tools/content/backfill-agent-compatibility.sql` 的既有形式（可重跑、逐列可追溯）。

> **這一列的欄位歸屬與預設值需要拍板**（`README` §8），因為它同時是一個資料模型決策與一個法遵預設。建議併入 **ADR-027**。

### 4.2 `artifacts` 上要動的一件小事

`expires_at` 是 `NOT NULL` 而 PDM-006 的 90 天**尚未追認**（`README` §8.1）。**不要在 `0027` 裡寫死 90**——比照既有慣例把它放進部署設定並在建立時計算，migration 只留註解指向 PDM-006。寫死一個未追認的值，就是把「已定值」與「已被追認」混為一談，而 `03` §1 的整節存在的理由正是把這兩件事分開。

---

## 5. 明確不動的契約

| 檔案／schema | 為什麼不動 |
| --- | --- |
| `contracts/openapi/sandbox-provider.yaml` | 打包**不進執行平面**（[packaging-design.md](packaging-design.md) §8）。M4 對 provider 契約一個位元組不改 |
| `contracts/openapi/llm-internal.yaml` | 打包不呼叫模型。安裝說明由 Profile 設定 ＋ 既有欄位組出來，**不生成散文**——讓模型寫安裝路徑會產生一段沒有人驗過的指示 |
| `contracts/events/trace-event.schema.json` | 打包不是 Run，不產生 trace 事件。**下載的紀錄走 audit 與 `download_records`，不走 trace**（ADR-009 的既有邊界） |
| `skill_versions` 的既有 license 欄位 | ADR-021 的四層已經正確；manifest 是它的對外映射，不是第二份定義 |
| `run_status` enum、`0005` 的 immutability trigger | 打包不建立也不修改 Skill Version（鐵律 4） |

## 6. 順手要補的既有欠帳（若第 1 批動到 `public.yaml`）

比照 M3 第 1 批「順手補兩筆欠帳」的既有做法，先查一次還有沒有 spec 落後實作的地方。**M3 已把 `estimated_cost` 與 `/admin/skills/{id}/restriction` 兩筆補完**（[../m3/audit.md](../m3/audit.md) §4.1 的 1-c），所以目前**沒有已知的欠帳**。第 1 批仍應跑一次 `contracts-drift`（該 job 在 `contracts/**` 變更時必跑）確認，若查到新的一筆就一併補上並記在交付摘要裡，**不得靜默略過**。
