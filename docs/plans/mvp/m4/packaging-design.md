# M4 打包管線設計

- 日期：2026-08-17
- 狀態：**設計，未實作。** 本文件只寫形狀與理由；契約實體由第 1 批依 [contract-deltas.md](contract-deltas.md) 產出。
- 範圍：`02:PACK-001`（標準套件）、`02:PACK-002`（安裝說明）、`02:SEC-007`（不可散布者不得進入打包）、`02:WS-002` 的下載紀錄、`02:NFR-001` 的下載稽核。
- 前提閱讀：[ADR-012](../../../adr/ADR-012-packaging-portability-and-agent-adapters.md)（打包可攜性與三層相容性）、[ADR-021](../../../adr/ADR-021-skill-license-provenance.md)（License 溯源）、[ADR-003](../../../adr/ADR-003-data-ownership-and-storage.md)（物件存取與刪除可追溯性）、[ADR-007](../../../adr/ADR-007-trust-security-and-supply-chain.md)（Artifact 與下載、稽核事件）。

## 0. 一句話

> 打包是**匯入的逆向**：讀一個不可變 Skill Version 的套件位元組，套上目標 Profile 的差異，產出一個**丟回自家匯入端會得到同一個結論**的 zip，外加一份說明它是怎麼來的 manifest——而整條路徑上平台不執行任何東西。

## 1. `02:PACK-001`／`PACK-002` 允收準則 → 工作項 → 本文件章節

| 允收準則（`02` §4.6） | `03` 工作項 | 本文件 |
| --- | --- | --- |
| 下載內容保留正確的 Skill 目錄結構與 `SKILL.md` | PACK-001 | §2 |
| 打包前再次執行規格驗證；有阻擋錯誤則不得標示為有效套件 | PACK-002 | §3 |
| 套件保留必要 License、作者、原始來源及衍生關係資訊 | PACK-003 | §4.1、§4.2 |
| Secrets、測試憑證、內部路徑及不應散布的 Run 資料不得被打入套件 | PACK-004 | §4.3、§4.4 |
| 使用者可選擇是否包含可散布的 Test Case 與範例資料 | PACK-005 | §5 |
| 下載頁顯示目標 Agent、支援狀態、安裝位置、依賴與環境變數需求 | PACK-006、007 | §6 |
| 尚未驗證的 Agent 必須顯示未驗證，不得保證可正常運行 | PACK-008 | §6.3 |
| 提供至少一個安裝後驗證 Prompt 或檢查步驟 | PACK-007 | §6.2 |
| （跨兩者）可被解壓、重新驗證與依說明安裝 | PACK-009 | §2.3、§9 |

## 2. 匯出的形狀：匯入的逆向

### 2.1 既有匯入端的事實（不是本設計要決定的，是要對齊的）

| 事實 | 位置 | 對打包的意義 |
| --- | --- | --- |
| 驗證吃 `fs.FS`，**不吃 bytes 也不吃 reader** | `services/platform/internal/skillpkg` 的 `Validate(fsys fs.FS) Report` | 打包器產出的位元組要驗證，得先自己轉回 `fs.FS`——`ingest.PackageFS([]byte) (fs.FS, error)` 已匯出，**直接用它，不要另寫一個 reader** |
| 容器**只有 zip** | `ingest.PackageFS`；`MaxZipBytes = 32 MiB`（壓縮後）、`maxUnpackedBytes = 256 MiB`（zip bomb 防護） | 匯出也只產 zip。**不要為了「像 Agent Skills 目錄」而輸出 tar**——那樣匯出就不再是匯入的逆向，`PACK-009` 的「重新驗證」立刻失去定義 |
| root 由 `ingest.PackageRoot` 判定：頂層有 `SKILL.md` → root 為空；否則若只有單一頂層目錄 → 剝掉它 | 同上 | 匯出**一律產「頂層即 root」的形狀**（`SKILL.md` 在 zip 根）。單一頂層目錄那條路是為了收 GitHub archive，不是為了產它 |
| 目錄結構的唯一硬要求是 root 有 `SKILL.md` | `skillpkg.Validate` | 「保留正確的 Skill 目錄結構」在平台的既有判準下就是這一條 ＋ 檔案引用不逃逸。**不新增結構規則**——新增等於匯出比匯入嚴，會產生「平台自己收得下、卻標成無效」的套件 |
| **archive entry 沒有顯式 zip-slip 檢查**，靠的是平台從不解壓到磁碟 | `skillpkg` 只走 `fs.FS`；`WalkDir` 不會走到非法路徑 | **這層保證在 M4 消失**：使用者會把 zip 解壓到自己的磁碟。打包器因此**必須自己保證每一個 entry 名稱都是 `fs.ValidPath` 且不含 `..`、不是絕對路徑、不是符號連結**，見 §4.4 |

### 2.2 匯出套件的內容

```text
<zip root>/
  SKILL.md                        ← 來源版本的原始位元組（Profile 可加 frontmatter 欄位，見 §6.1）
  <來源套件的其餘檔案>              ← 白名單複製，逐位元組（§4.3）
  LICENSE / LICENCE / COPYING     ← 若來源有
  LICENSE.repo                    ← 若來源有（skillpkg.CarriedLicenseFile，逐位元組原樣）
  LICENSE.repo.provenance.json    ← 若來源有（skillpkg.CarriedProvenanceFile）
  skillhub-manifest.json          ← 平台新增（§4.1）
  INSTALL.md                      ← 平台新增（§6.2）
  test-cases/                     ← 選用，PACK-005（§5）
```

**三個刻意的決定**：

1. **平台新增的檔案只有三樣**（manifest、`INSTALL.md`、選用的 `test-cases/`），而且**檔名前綴或位置都可辨識**。多加一個檔案就多一次「這是作者寫的還是平台寫的」的混淆，而 ADR-021 已經為 `LICENSE.repo` 付過一次這個代價（它之所以要精確大小寫比對，正是因為要和作者的檔案分得開）。
2. **`SKILL.md` 的 frontmatter 在 `standard` 目標下一個位元組不改。** `standard` 是「Skill Hub 不綁定單一 Agent」這個承諾的可驗證證據（PDM-008），任何改寫都會讓它不再等於來源。Profile 的欄位差異只在 `claude-code`／`claude-agent-sdk` 兩個目標上發生，且必須是 **additive frontmatter 欄位**，不得移除或改寫既有欄位（ADR-012「Adapter 不得靜默改變 Skill 的任務意圖或移除必要安全限制」）。
3. **不重排、不重壓、不「整理」。** 來源檔案逐位元組複製，順序由排序後的路徑決定（§2.4）。看起來多餘的檔案（例如作者的 `.editorconfig`）留著——刪它是替作者做決定，而它不在任何一條排除規則裡。

### 2.3 「匯出必過自家匯入驗證」的可機械判定形式

這是 `PACK-009` 第一條的落點，也是本設計最重要的一條不變式：

> 對任一產出的 zip 位元組 `b`：`skillpkg.Validate(ingest.PackageFS(b))` 必須回傳 `Blocked == false`，且其 `Manifest.Name` 與來源版本相同。

具名測試（形狀，不是最終命名）：

- `TestEveryProfileProducesAPackageThePlatformWouldAccept`——三個目標各打一次 45 個種子套件裡的樣本，逐一丟回 `Validate`，斷言 `Blocked == false`。
- `TestTheStandardPackageRoundTripsToTheSameContentHash`——`standard` 目標**不含** `skillhub-manifest.json`／`INSTALL.md` 時（見 §4.1 的旗標）重新匯入應得到與來源相同的 `content_hash`；含平台檔案時則斷言「除平台新增檔案外逐位元組相同」。
- **反證測試**：刻意造一個含 `possible-secret` 的來源版本，斷言打包被拒且**沒有任何物件被寫入**。

### 2.4 規範化與可重現性

ADR-012 §「可重現性」要求「相同輸入與版本應能產生語意等價的套件；若內容雜湊因壓縮時間等非語意 metadata 不同，需另有**規範化 Manifest Hash**」。**這是一個真實的問題**：zip 的 entry mtime、外部屬性、壓縮器版本與 entry 順序都會讓位元組不同，而「打兩次得到兩個不同的雜湊」會讓 `artifacts.content_hash` 的去重與「這是不是我上次拿到的那個檔」都失去意義。

**設計取兩個雜湊，各自回答一個問題**：

| 雜湊 | 算什麼 | 回答什麼 |
| --- | --- | --- |
| `content_hash` | 匯出 zip 的全部位元組 SHA-256 | 「這個檔案是不是那個檔案」——給 `artifacts` 表與下載完整性用 |
| `manifest_hash`（規範化） | 對 `{path: sha256(bytes)}` 依 path 排序後的 canonical JSON 取 SHA-256，**不含任何 zip metadata、不含 manifest 自身** | 「兩次打包的**內容**是不是一樣」——給冪等、去重與「我改的是 Profile 還是 Skill」用 |

**同時把 zip 寫入規範化**（讓 `content_hash` 在同一台機器上也穩定）：entry 依 path 排序、mtime 一律寫 `1980-01-01T00:00:00Z`（zip 的最小合法值）、外部屬性固定、不寫 extra field、壓縮等級固定。做到這一步之後 `content_hash` 在**同一個打包器版本**上是可重現的；跨版本不保證，這正是 manifest 要記 `packager_version` 的理由。

> **這個決策需要一份 ADR（建議 ADR-027）**，因為它同時回答 ADR-012 的兩個待決策（規範化 Manifest Hash、簽章與完整性驗證），而且一旦有第一個 Download Artifact 存在，雜湊語意就不能再改。

## 3. 打包前的規格重驗（`PACK-002`）

**重用 `EVAL-010` 已經走過的那條路，不另寫判準**（`04` M4-1）。既有前例是 `internal/eval` 的 `validatePatched()`：打補丁 → `ingest.PackageFS` → `skillpkg.Validate` → **任一 `error` 級 finding 即整批拒絕且不建版本**。

打包的版本：

```text
讀 skill_versions.package_object_key 的位元組
→ 白名單過濾（§4.3）＋ Profile 差異（§6.1）＋ 平台新增檔案
→ 產出候選 zip 位元組
→ ingest.PackageFS → skillpkg.Validate
→ Blocked == true ？ → 拒絕，不產生 artifact，逐條回 findings
→ Blocked == false ？ → 寫物件、寫 artifacts 列
```

**三件必須明說的事**：

1. **驗的是產出的位元組，不是來源的位元組。** 來源版本在匯入時已經驗過一次，但打包會加檔案、可能改 frontmatter，所以要重驗——這正是 `PACK-002` 存在的理由。若只驗來源，那條準則等於什麼都沒做。
2. **判準完全來自 `skillpkg` 的 severity，打包器不得自己定義「阻擋」。** 兩套判準會漂移，而漂移的方向必然是打包側比較鬆（它在使用者最想成功的那一步）。
3. **警告不阻擋，但必須進 manifest 與下載頁。** `02:PACK-001` 說的是「若有**阻擋錯誤**則不得標示為有效套件」，warning 不在其中；但把它藏起來會踩 `02:SKILL-002`「分開呈現阻擋錯誤與可接受警告」。

## 4. 內容規則：進什麼、不進什麼、怎麼證明

### 4.1 `skillhub-manifest.json`：平台對這個檔案的自述

ADR-012 §「可重現性」列了 Download Artifact 必須記錄的六項，本設計逐項落地並補上溯源：

| 欄位 | 內容 | 出處 |
| --- | --- | --- |
| `schema_version` | manifest 自身的版本 | 比照 `contracts/events/` 的版本演進慣例 |
| `packaged_at`、`packager_version`、`profile_id`、`profile_version` | 打包時間、打包器版本、目標與其版本 | ADR-012 |
| `source.skill_id`／`skill_version_id`／`version_number`／`content_hash` | 來源不可變版本 | ADR-012、`02:DISC-003` |
| `source.origin` | **三條溯源路徑之一**（§4.2） | `PACK-003` |
| `license.expression`／`license.source_tier` | **成對，永不壓成單一字串** | ADR-021 決策 1；`02:CONTENT-002` |
| `license.disclosures[]` | `license-from-package-file`／`license-from-repo-file` 等資訊級揭露的原文 | ADR-021 決策 5；`02:SKILL-004` |
| `validation` | 重驗結果：`blocked: false`、warning 與 info 的完整 findings | `PACK-002`、`02:SKILL-002` |
| `compatibility` | **三層分開**：`format`（規格驗證）／`capability`（Profile 能力比對）／`behaviour`（該 Skill Version 在平台上的 Run 證據，含 `runtime_image` 與 `measured_at`） | ADR-012 三層相容性；`0022` 的 `skill_runtime_compatibility` |
| `included_test_cases[]`／`excluded_reason` | 含／不含的 Test Case 與範例資料清單 | ADR-012；`PACK-005` |
| `manifest_hash` | §2.4 的規範化雜湊（**不含自身**） | ADR-012 |

**`compatibility.behaviour` 有一條硬規則**：它的值只能來自 `skill_runtime_compatibility` 裡**該 Skill Version × 該 Runtime Image** 的實測列。沒有列就是 `unverified`，**不得從「同一個 Skill 的別的版本」或「別的映像」外推**——`04` 乙-4 已經為這個鍵付過一次代價（換映像即回到未驗證直到重測），打包不得把那個誠實抹掉。

### 4.2 溯源要走三條路（`PACK-003`）

「保留原始來源與衍生關係」在資料層不是一個欄位，是三條互斥的路徑：

| 來源形態 | 怎麼查 | manifest 記什麼 |
| --- | --- | --- |
| 直接匯入（URL 或上傳） | `skill_versions.source_id` → `skill_sources`（`source_type`／`source_url`／`source_ref` commit SHA／`fetched_at`／`content_hash`） | `origin.kind = "import"` ＋ 上述欄位 |
| Fork | `skills.forked_from_skill_id`／`forked_from_version_id`；**`source_id` 刻意為 NULL**（`skill_sources` 那一列屬於原 workspace） | `origin.kind = "fork"` ＋ 上游 skill／version ＋ **遞迴到最初的 import 來源** |
| 採納改善建議建出 | `evaluation_suggestions.applied_skill_version_id` 的**反向查詢**（沒有 `derived_from_evaluation_id` 欄位，且刻意不補——[../m3/evaluation-design.md](../m3/evaluation-design.md) §5.3 的 2026-08-17 更正） | `origin.kind = "improvement"` ＋ 哪份評估、哪幾條建議的 `category` 與 `target_path`（**不含 `problem`／`expected_impact`／證據 `excerpt`**，見 §4.3） |

**Fork 鏈必須走到底**：`02:DISC-003` 第 5 條要求「任何 Skill Hub 修改後的版本都能追溯到**原始來源**及 Fork 關係」，只記上一跳不算。鏈上任一跳的來源已被刪除時記 `"unavailable"` 而不是省略——省略讀起來像「沒有上游」。

### 4.3 排除規則採**白名單**，不採黑名單（`PACK-004`）

**規則**：只有下列來源的檔案可以進包。

1. 該 Skill Version 套件位元組裡的檔案（＝匯入時通過驗證的那一份），扣掉 §4.4 的硬性排除；
2. 平台產生的三個檔案（manifest、`INSTALL.md`、選用的 `test-cases/`），其內容由本設計逐欄位列舉。

**其他一切都不進包，不需要逐項禁止。** 這條規則的理由很短：黑名單漏一項就是洩漏，白名單漏一項只是少一個檔案。

`04` M4-4 點名的評估產物因此**自動不在包內**——它們從來就不是套件位元組的一部分：

| 不進包的東西 | 為什麼有人會以為它該進 |
| --- | --- |
| 改善建議的 `problem`／`expected_impact` | 它們描述的是這個版本為什麼比上一版好，聽起來像 changelog。**但它們是模型寫的散文**，且引用了該 Run 的私有輸入 |
| 證據 `excerpt` | 逐字來自 trace 或最終輸出，可能含使用者 Dataset 的內容 |
| rubric | 使用者寫的驗收標準，是 Test Case 的一部分不是 Skill 的一部分 |
| 評估報告、Trace、Run artifact | Run 資料 |
| Test Case 與 Dataset | **除非**走 `PACK-005` 的可散布路徑（§5），且那條路徑有自己的判準 |

### 4.4 硬性排除與必須自己做的檢查

即使在白名單內，下列 entry 一律不寫入匯出 zip：

| 排除項 | 理由 |
| --- | --- |
| 任何 `!fs.ValidPath(name)`、含 `..`、絕對路徑、或磁碟機代號 | §2.1 最後一列：平台從不解壓到磁碟，所以匯入端沒有 zip-slip 檢查；**使用者會解壓**，這層檢查在匯出端第一次成為必要 |
| 符號連結與非一般檔案（device、fifo）entry | 同上；zip 可以帶它們，解壓工具的行為各不相同 |
| 匯入時被標成 `possible-secret` 的檔案 | 這種來源版本根本打不了包（§3 會 `Blocked`），此條是縱深防禦 |
| `.git/`、`.github/`、`node_modules/`、`__pycache__/`、`.venv/` 等建置與版控殘留 | 它們不是 Skill 內容；且 `.git/` 可能帶著私有 remote URL 與憑證快取（＝`PACK-004` 的「內部路徑」） |
| 內容型別為 `application/x-executable` 類的檔案 | 匯入端已對 `binaryExts` 產生 warning；匯出端**不阻擋但要在下載頁與 manifest 顯著標示**——這是使用者要在自己機器上執行的東西 |

**具名測試要對匯出的位元組斷言，不對意圖斷言**（前例：`TestCostEstimateIsOutsideTheConfirmedHash` 直接對回應位元組重算 sha256）：

- `TestExportedPackageNeverContainsRunOrEvaluationData`——造一個由建議生成的版本、跑過評估、有 artifact，打包後**逐 entry 掃過 zip**，斷言沒有任何 entry 的內容出現評估文字。
- `TestExportedPackageHasNoPathThatEscapesOnExtraction`——對每個 entry 名稱斷言 `fs.ValidPath`。
- `TestASecretBearingVersionCannotBePackaged`。

### 4.5 授權閘門：可散布性目前**資料庫回答不了**

盤點的結論很直接：`skills`／`skill_versions` **沒有** `redistributable`／`source_available`／`license_status` 欄位。「source-available 一律不產出任何 Download Artifact」（PDM-008、PDM-002、ADR-012）目前只活在 `tools/content/seed-skills.json` 的策展欄位與文件裡，而 `0023_access_restriction.sql` 的註解**明說它不管打包**（「Download packaging is unaffected here because it is already blocked for this content by CONTENT-004/ADR-012」——那句話在 M4 之前為真是因為沒有打包功能）。

**設計取兩道鎖，方向相反，都要有**：

| 鎖 | 判準 | 失敗時 |
| --- | --- | --- |
| **鎖 A（既有旗標）** | `skills.access_restriction IS NOT NULL` → 拒絕。未知原因碼**仍然拒絕**（fail-closed，與 `restrictionOf` 讀取端同向） | 422 ＋ 可讀理由；不產生任何 artifact |
| **鎖 B（新欄位）** | 新增 `skill_versions.redistribution`（或 `skills` 層，見下）三態：`allowed`／`blocked`／`unknown`。**只有 `allowed` 放行**——`unknown` 視同 `blocked` | 同上 |

**為什麼一定要鎖 B**：鎖 A 是一個**人工按下的暫時性 hold**（`license-review`），它涵蓋的是今天已知的四筆。可散布性是一個**內容屬性**，它對每一個匯入的 Skill 都要有答案，包括明天使用者自己上傳的那一個。拿 hold 當可散布性判準，等於宣稱「沒有人特別擋它就是可以散布」——那正是 ADR-021 §5.3 記錄的那個錯誤方向（「repo 根有 MIT ⇒ 子目錄是 MIT」錯在放行方向）。

**欄位放哪一層需要拍板**（列入 `README` §8）：放 `skills` 與 `access_restriction` 同層、隨 Fork 複製、可撤銷；放 `skill_versions` 則不可變、精確對應那一份位元組但不可修正。**建議放 `skills`**，理由同 `0023` 的既有裁定（授權事實屬於來源；`skill_versions` 不可變，放不進一個可撤銷的判定）。

**判準的來源**（不是新的政策，是把既有政策寫成可判定的形式）：

1. `license_expression IS NULL` → `unknown` → 阻擋（`02:DISC-003`「授權未知……不得暗示可自由修改或再發佈」）。
2. `license_expression` 在 OSI 允許再散布的清單內 → `allowed`。
3. 已知的 source-available 條款（本專案目前只有 `anthropics/skills` 一類）→ `blocked`。
4. 認得但無法歸類 → `unknown` → 阻擋。
5. **`license_status = Confirmed` 不得成為放行條件**——`02:CONTENT-002` 明文「已人工確認不等於可再散布」。

**回填**：45 個種子 Skill 的可散布性事實已存在於 `tools/content/seed-skills.json` 的 `sources.<id>.redistributable`／`.source_available`，回填腳本比照 `tools/content/backfill-agent-compatibility.sql` 的既有形式（可重跑、逐列可追溯）。

### 4.6 Artifact 的 quarantine 狀態要不要走

`artifacts` 表從 `0004` 就有 `scan_status ∈ {quarantined, available, rejected}`，而**目前沒有任何程式碼會把它推進到 `available`**。ADR-003 的規則是「Artifact 上傳先進入隔離區，通過大小、類型與安全檢查後才可供下載」。

**設計：走，但檢查就是 §3 的重驗。** Download Artifact 的「安全檢查」不需要一套新的掃描器——它的內容全部來自一個已經通過匯入驗證的套件，再加上平台自己產生的三個檔案。因此狀態轉移是：

```text
建立 artifacts 列（quarantined）
→ 寫物件
→ 重驗通過 ＋ 大小上限通過 ＋ 硬性排除通過
→ 同交易內轉 available
```

**繞過它是錯的**：`scan_status` 存在的價值在於「下載端點只看 `available`」這一條可以被具名測試鎖住，而不是靠打包流程記得不要提早交出去。失敗時轉 `rejected` 並保留列（帶理由），不刪除——使用者要看得到「為什麼打不出來」。

## 5. 可散布的 Test Case 與範例資料（`PACK-005` ＋丙-12）

**兩個方向共用一份 schema**，這是 `04` M4-3 的重點：打包時**匯出**它，策展內容種入全新部署時**匯入**它。分開做會得到兩種格式，而丙-12 說的正是「策展側目前沒有可種入的形式」。

### 5.1 「可散布」的判準（`02:PACK-001` 目前沒有，建議補進 `02`）

| 可以進包 | 不可以進包 |
| --- | --- |
| 平台策展產生的範例 Dataset（合成內容、無 Secrets 與個資，`CONTENT-007` 已有這個判準） | **使用者上傳的 Dataset 位元組**——授權不明，且它是 `PACK-004` 要排除的「不應散布的 Run 資料」的近親 |
| User Prompt 與驗收條件的文字 | 使用者上傳檔案的檔名以外的內容 |
| rubric（若該 Test Case 有） | 任何 Run 結果、評估與 trace |

**使用者上傳的 Dataset 預設不入包，且不提供選項。** 給一個「要不要包含我上傳的檔案」的勾選框，看起來是尊重使用者，實際上是把一個授權判斷丟給不具備判斷材料的人（那些檔案可能是他公司的資料）。要放寬需要一個明確的產品決策，不是一個核取方塊。

### 5.2 形狀

`test-cases/<slug>/case.json` ＋ `test-cases/<slug>/data/<檔名>`：`case.json` 帶 `{name, user_prompt, criteria[{id, text}], rubric?, datasets[{file, sha256, content_type}]}`，`data/` 放範例檔案位元組。`slug` 由平台產生且穩定（同一個 Test Case 兩次打包同一個 slug）。

**匯入方向**（丙-12）由 `tools/content/` 的一支種入腳本消費同一份 schema，把策展 Test Case 寫進一個全新部署的 catalog workspace。這條路徑目前完全不存在——`CONTENT-007` 的範例 Dataset／Prompt／驗收條件當初都是在 M2 的臨時 Workspace 裡現做的。

## 6. 打包目標與安裝說明（`PACK-006`～`008`）

依 PDM-008（**尚未追認**）：**1 個標準套件 ＋ 2 個已驗證安裝 Profile**。對外一律表述為「1 標準套件 ＋ 2 Profile」，UI 顯示的「已驗證安裝 Profile 數量」為 **2**。

| id | 類型 | 安裝位置 | 三層相容性 | 安裝後驗證 |
| --- | --- | --- | --- | --- |
| `standard` | **標準套件** | — | 格式 | 附規格驗證報告（manifest 的 `validation`） |
| `claude-code` | Profile | 使用者層 `~/.claude/skills/<name>/` 或專案層 `.claude/skills/<name>/` | 格式 ＋ 能力 ＋ 行為（若該版本在平台試跑過） | 一句驗證 Prompt（取自 `CONTENT-007` 的範例 Prompt） |
| `claude-agent-sdk` | Profile | Agent 工作目錄 `.claude/skills/<name>/` | 同上 | 最小可執行 `query()` 片段，**必須示範 `cwd` 與 `setting_sources`** |

**兩個 Profile 的安裝路徑實際相同**（PDM-008 v4 依實測釐清），差別在「使用者層 vs 工作目錄層」與驗證方式。**文案必須把這個差異講清楚**，否則使用者會以為是重複選項。`claude-agent-sdk` 的安裝說明**只給路徑是不夠的**——載入條件是安裝步驟的一部分，缺任一項 Skill 就不會被載入（ADR-023 記錄的那次靜默失效）。

> **2026-08-17 更正（第 3 批前半）**：上表與本段原寫「必須示範 `cwd` 與 `setting_sources`」，那句話把 [ADR-023](../../../adr/ADR-023-agent-sdk-version-pinning-and-behaviour-revalidation.md) §2 測項 1 讀反了。實測在 SDK 0.3.233 上量到的四個條件是：`cwd` 指向放 `.claude/skills/` 的目錄、**`settingSources`／`setting_sources` 省略**（傳 `["project"]` 發現到**零個** skill，與 0.2.137 及官方文件的讀法相反）、`skills: "all"`、工具清單——**四者缺一即零個 skill**。因此 snippet 的正確形狀是**設 `cwd`、不傳 `setting_sources`，並在註解裡寫明為什麼不傳**（那句註解同時滿足 schema 的 lookahead）。落地實體見 [`contracts/packaging/profiles/claude-agent-sdk.json`](../../../../contracts/packaging/profiles/claude-agent-sdk.json)，判準與更正紀錄見 [README §13.3](README.md#133-adr-023-的更正本批發現契約寫反了一次)。

### 6.1 Profile 能改什麼、不能改什麼

| 可以 | 不可以 |
| --- | --- |
| 新增 frontmatter 欄位（additive） | 移除或改寫既有 frontmatter 欄位 |
| 新增 `INSTALL.md` 與設定範本檔 | 改寫 `SKILL.md` 正文 |
| 決定 zip 內的頂層目錄名（例如 `<name>/`） | 改變檔案內容 |
| 在 manifest 記錄自己的 `profile_version` | 移除任何 License 或 provenance 檔案 |

最後一列是 ADR-012 的「Adapter 不得靜默改變 Skill 的任務意圖或移除必要安全限制」的可判定形式。

### 6.2 `INSTALL.md` 的必要欄位（`02:PACK-002` 逐條）

目標 Agent 與版本範圍、**支援狀態**（已驗證／未驗證）、安裝位置（含使用者層 vs 專案層的差異說明）、依賴（來自 `CONTENT-003` 的依賴欄位與 `skillpkg` 的 `package-dependencies` finding，**且要標明哪些在平台的 Runtime Image 內被拒收**——`infra/images/README.md` 的拒收表）、環境變數需求、**至少一個安裝後驗證 Prompt 或檢查步驟**、已知限制與未驗證能力。

### 6.3 未驗證的呈現（`PACK-008`）

`02:PACK-002` 第 2 條：「尚未驗證的 Agent 必須顯示未驗證，不得保證可正常運行。」在 MVP 只有三個目標且兩個都已驗證的情況下，這條準則的實際落點有兩個：

1. **`standard` 的相容性只有格式一層**，下載頁必須寫「任何支援 Agent Skills 規格的 Agent 都可以試，但 Skill Hub 沒有在你的 Agent 上驗證過」，並連到規格說明（PDM-008 風險表已要求）。
2. **`behaviour` 層為 `unverified` 的 Skill Version**（沒有實測列的、或量測是在另一個 Runtime Image 上的）必須顯示為未驗證並附映像標籤，**不得因為 `format` 與 `capability` 都通過就寫成「可用」**。這是 `01` §12 風險表「Skill 符合規格但無法執行」的對策落點。

## 7. 下載授權（`ADR-003`：短效授權，不公開永久物件 URL）

### 7.1 兩個選項與取捨

| 選項 | 內容 | 評價 |
| --- | --- | --- |
| **A（建議）：平台代傳** | `GET /skills/{id}/downloads/{artifact_id}/content` 由 API 讀物件後串流回應，`Content-Disposition: attachment` | Workspace scope 就是既有的 session middleware，不需要第二套授權；URL 不是機密材料，可以進 log 與稽核；`access_restriction` 與 `scan_status` 在同一個 handler 裡檢查。代價是位元組經過 API 程序（**但那只是搬位元組，不是執行**，鐵律 1 不受影響），且大檔佔用連線 |
| B：預簽 URL 交給瀏覽器 | 用既有的 `objstore.PresignGet` | 省 API 頻寬。但**預簽 URL 是機密材料**：現有規則（`run/grants.go` 的註解）是「從不記錄、不進 trace、不進權限摘要」，而下載紀錄與稽核事件正好要記下這次下載——兩者直接衝突，得為它另立一套「記事件但不記 URL」的規則。另外它繞過 API，`scan_status`／`access_restriction` 的檢查只發生在簽發那一刻，TTL 內旗標改變也擋不住 |

**取 A。** 套件的量級（來源套件上限 32 MiB 壓縮後）與封測規模（PDM-010 提案 900 Run／月）下，B 省的東西不值得它帶進來的那條規則衝突。**若日後檔案變大或流量上來，換成 B 是一個 handler 的替換**，屆時 ADR-028 或後續 ADR 再議。

> 順帶關掉一個既有待辦：`services/sandbox/README.md` 記著「逐檔物件需 POST policy 前綴授權，留給 `PACK-001`」。取 A 之後**這一項不再是 PACK-001 的前置**——平台代傳不需要前綴授權。該註記應在 M4 收斂批就地更正。

### 7.2 授權、紀錄與稽核（同交易，鐵律 9）

一次成功的下載寫三件事：`artifacts` 的存取時間、`download_records`（`WS-004` 要列的東西：誰、何時、哪個 artifact、哪個 profile）、以及 `audit` 事件（`CORE-008`／ADR-007「Artifact 下載」）。

**下載紀錄與 audit event 是兩件事，不要合併**：前者是使用者自己看的產品功能（`02:WS-002` 第 1 條），後者是合規紀錄（PDM-006 提案給 audit 400 天、只存 actor／動作／資源 ID／時間戳、**不含內容**）。保存期限與可見性都不同，合併會讓兩邊都被較嚴的那一個綁住。

### 7.3 到期與刪除

`artifacts.expires_at` 對 `download_package` 依 PDM-006 提案為 **90 天**（未追認，見 `README` §8.1）。到期後：物件刪除、列標 `deleted_at`、**下載紀錄保留**（它記的是「你曾經下載過」，不是「檔案還在」）。下載頁顯示**絕對日期**不顯示相對天數（PDM-006 風險表已要求），且措辭為「將於 YYYY-MM-DD 自動刪除，同一版本可隨時重新打包」——因為打包是冪等的，這句話為真。

**刪除的既有陷阱**：`skill_versions.package_object_key` 在 Fork 時**共享同一個物件**（`registry.Fork` 不複製位元組），所以 `governance.sql` 刻意把 package object 排除在硬刪除範圍外。Download Artifact **不共享**——每次打包產生自己的物件——所以它可以被安全地硬刪。**這個差別要寫進刪除流程的註解**，否則下一個人會照抄 package object 的謹慎，或反過來把 package object 一起刪掉。

## 8. 這條路徑上不會發生的事（鐵律落點）

| 鐵律 | 落點 | 怎麼證明 |
| --- | --- | --- |
| 1／2 | 打包**不執行任何東西**：不解壓到磁碟、不跑 Script、不解析內嵌程式碼的語意。整個 package 只做「讀位元組 → 過濾 → 寫 zip → 交給 `skillpkg.Validate`」 | 整個 package 沒有 `os/exec`、沒有寫入本機檔案系統的路徑；套件只以 `fs.FS` 與 `[]byte` 流動 |
| 3 | 打包與下載端點的 workspace 一律取自 session；非擁有者 404 | 比照 `CORE-006` 既有慣例的具名測試 |
| 4 | 打包**不建立也不修改** Skill Version；Download Artifact 本身不可變（重打包＝新的一列） | `artifacts` 沒有 UPDATE 內容的路徑；`0005` 的 immutability trigger 不受影響 |
| 9 | 授權、紀錄、稽核同交易；到期刪除冪等 | 整合測試斷言三者同進同退 |
| 11 | **Secrets 不進包**——白名單 ＋ `possible-secret` 阻擋 ＋ 對匯出位元組的具名測試 | §4.3、§4.4 |
| 12 | manifest／Profile／可攜 Test Case 三份 schema 先寫 | 第 1 批 |

## 9. `PACK-009` 的三段驗證，以及最後一段為什麼要人

`02` 的字面是「驗證下載套件可被解壓、重新驗證與依說明安裝」。三段的可自動化程度不同：

| 段 | 誰做 | 形式 |
| --- | --- | --- |
| 可被解壓 | 程式 | 標準 zip 工具解得開；每個 entry 名稱 `fs.ValidPath`（§4.4） |
| 可被重新驗證 | 程式 | §2.3 的不變式：丟回 `skillpkg.Validate` 得 `Blocked == false` |
| **依說明安裝** | **人**（`README` §5.2 H-2 之外的一項） | 三個目標裡有兩個就是開發者手上的工具（Claude Code、Agent SDK），本機裝得起來；`standard` **不宣稱**在未驗證的 Agent 上可用（§6.3），所以它沒有這一段要驗 |

`UX-008`（「驗證下載與安裝說明能否讓使用者在目標 Agent 完成使用」）是封測要回答的，不是打包批要回答的——**它需要的是別人的機器**。
