# 打包契約（PACK-001／002／003／005）

- 檔案：[download-manifest.schema.json](download-manifest.schema.json)、[packaging-profile.schema.json](packaging-profile.schema.json)、[portable-test-case.schema.json](portable-test-case.schema.json)（皆為 JSON Schema 2020-12，內文英文）
- 驗證：`python tools/contracts/validate_packaging.py`（驗三份 schema 的 `check_schema`、全部 `examples`、以及 18 個反例必須被擋）
- CI：`.github/workflows/ci.yml` 的 `contracts-drift` job，比照 M3 為 `contracts/events` 補的那一步（同一個 pinned `jsonschema`）
- 狀態：**M4 第 1 批產出，實作未落地。** 形狀依 [m4/contract-deltas.md](../../docs/plans/mvp/m4/contract-deltas.md) §3 與 [m4/packaging-design.md](../../docs/plans/mvp/m4/packaging-design.md)。

## 1. 為什麼這三份不在 OpenAPI 裡

`contracts/` 至此只有兩類：OpenAPI 規格（服務之間）與 trace 事件（管線之間）。這三份是**第三類：檔案格式**。

理由是消費者名單裡有一個在 repo 外——拿到 zip 的使用者和他的工具要能讀 manifest、驗 hash、照 `INSTALL.md` 安裝。OpenAPI 表達不了「這是一個檔案的形狀」，而把它寫成 Go struct 的註解，等於讓 repo 外的人沒有契約可讀。

三份分別是：

| 檔案 | 描述什麼 | 誰讀 |
| --- | --- | --- |
| `download-manifest.schema.json` | 套件根的 `skillhub-manifest.json`：來源版本、溯源、License、重驗結果、三層相容性、規範化 hash | Go（產生）、`apps/web`（顯示）、**使用者與其工具**（驗證） |
| `packaging-profile.schema.json` | 一個打包目標的**版本化設定**（安裝位置、additive frontmatter、環境變數、驗證 Prompt、已知限制）。**不描述 Adapter 程式**，也不定義 plugin 機制 | Go（讀設定）；三個內建目標各一份實體 |
| `portable-test-case.schema.json` | `test-cases/<slug>/case.json` | **兩個方向**：打包器匯出、`tools/content/` 的種入腳本匯入（`04` 丙-12） |

## 2. 敏感欄位規約（鐵律 11、NFR-002）

`contracts/events` 的做法是把危險欄位標成 `sensitivity: secret_bearing`，遮罩器據以工作。**這裡的問題是反過來的**：套件是平台把位元組交出去的地方，所以規約不是「哪些欄位要遮罩」，而是**整份文件都不得承載 Secret**。

三份 schema 的根層都帶 `$comment: "sensitivity: no_secrets"`。其可判定形式有三條，全部是 schema 本身在擋，不是註解在提醒：

1. **每一層都 `additionalProperties: false`。** 這條在 `origin.kind = "improvement"` 上最要緊：改善建議的 `problem`／`expected_impact` 與證據 `excerpt` 是模型寫的散文、引用該 Run 的私有輸入，manifest 只記 `category` 與 `target_path`。封閉 schema 讓「多寫一個欄位」變成契約變更，而不是一次疏忽。
2. **`env_vars[].example` 拒收憑證樣式**（`sk-(proj|ant)-`，與本 repo push 前的檢查同一組）。這欄會被原樣渲染進 `INSTALL.md`，而 `INSTALL.md` 隨套件出門。
3. **`portable-test-case` 的 `origin` 是只有一個值的 enum（`curated`）。** 使用者上傳的 Dataset 位元組永遠不入包，且**不提供勾選框**——勾選框看起來像尊重使用者，實際上是把一個授權判斷丟給不具備判斷材料的人。要放寬得改契約。

`download-manifest` 內唯一非平台自產的文字是 `validation` 的 findings 與 `license.disclosures`，兩者都由 `skillpkg` 產生、講的是套件自己的檔案，不引用 Run 輸出或使用者資料。

## 3. `download-manifest` 的三條界線

這三條是 [contract-deltas.md](../../docs/plans/mvp/m4/contract-deltas.md) §3 點名「必須寫進 schema description」的，全部同時有機器可判定的形式：

### 3.1 `compatibility.behaviour` 是一次量測的紀錄，不是承諾

值只能來自 `skill_runtime_compatibility` 裡**該 Skill Version × 該 Runtime Image** 的實測列（`0022`）。沒有列就是 `unverified`，**不得從同一個 Skill 的別的版本或別的映像外推**——換映像即回到未驗證直到重測（`04` 乙-4）。

可判定形式：`capability` 與 `behaviour` 只要有一軸不是 `unverified`，`runtime_image` 與 `measured_at` 就是必填。一個沒有映像的判定，就是它禁止的那種外推。

值域逐字取自 `0022`，**不另造一套**。命名有一處差異：ADR-012 的第三層叫 `behaviour`，DB 欄位叫 `runtime`——這裡跟 ADR-012 與 `public.yaml` 的 `SkillCompatibility` 一致用 `behaviour`，值域相同。

### 3.2 `license.expression` 與 `license.source_tier` 同生同滅

ADR-021 決策 1 的對外形式，`0012` 的 CHECK 是它在 DB 的那一半。`oneOf` 兩支：兩者皆 null，或兩者皆非 null；一支有值一支沒有一律不合法。

`expression` 另外拒收 `NOASSERTION`／`NONE`（ADR-021 決策 7）：這個欄位只承載運算式，不承載狀態，未知就是 NULL。輸出 SPDX／SBOM 時再映射。

`source_tier` 的值域逐字取自 `skill_versions.license_source`（`0012` ＋ `0014`）。ADR-021 的第 4 層 `curated-declared` 已定義但不實作，**這裡也不收**。

**這份 schema 不決定哪些 License 可以打包。** 可散布性是 Skill 上的另一道鎖（`skills.redistribution`，三態、預設 `unknown`，見 ADR-027 決策 4 與 [packaging-design.md](../../docs/plans/mvp/m4/packaging-design.md) §4.5），且 `license_status = Confirmed` **明文不得成為放行條件**（`02:CONTENT-002`）。manifest 報告授權證據，閘門拒絕套件——兩件事。

### 3.3 `manifest_hash` 不含 manifest 自身，也不含任何 zip metadata

`$defs/manifestHashInput` 把**被雜湊的那個東西**寫成可驗證的形狀：`{package-relative path: sha256(bytes)}`，canonical JSON（key 依 path 排序）後取 SHA-256。

排除 zip metadata（entry 順序、mtime、外部屬性、壓縮方式、extra field）是因為它們會在內容沒動的情況下讓位元組改變；排除 `skillhub-manifest.json` 自身是因為 manifest 帶著這個 hash，而且帶著 `packaged_at`——把它算進去會讓兩次位元組相同的打包得出不同的雜湊，正好抹掉這個欄位存在的理由。

兩個雜湊各答一個問題（[packaging-design.md](../../docs/plans/mvp/m4/packaging-design.md) §2.4、ADR-027 決策 1）：`artifacts.content_hash` 答「這個檔案是不是那個檔案」，`manifest_hash` 答「兩次打包的**內容**是不是一樣」。可重現性的範圍是**同一個打包器版本內**（ADR-027 決策 2），這正是 manifest 要記 `packager_version` 的理由。

**manifest 不是簽章，也不是背書**（ADR-027 決策 3）：MVP 不簽章，雜湊是使用者可自行重算的事實，不是平台對內容安全或未被竄改的保證；撤銷只到「停止再發」，已下載的副本無法作廢。schema 的根層 description 逐字寫了這一條。

`manifestHashInput` 的 path key 另外套用與 zip entry 相同的逃逸規則（無開頭斜線、無 `..` 區段、無磁碟機代號、無反斜線）。這層檢查在匯入端不存在，因為平台從不解壓到磁碟；**使用者會解壓**，所以它在匯出端第一次成為必要。

## 4. `packaging-profile` 的兩條可判定規則

- **`standard` 目標不得改 `SKILL.md` 一個位元組**：`id = standard` 時 `kind` 必為 `standard_package`、`frontmatter_additions` 必為空、`install.locations` 必為空、`top_level_dir` 必為 null。它是「Skill Hub 不綁定單一 Agent」這個承諾的可驗證證據（PDM-008）。
- **Profile 只能 additive**：`frontmatter_additions` 的 key 不得是 `name`／`description`／`license`／`allowed-tools`／`allowed_tools`。這是 ADR-012「Adapter 不得靜默改變 Skill 的任務意圖或移除必要安全限制」的可判定形式。
- **`claude-agent-sdk` 的 `snippet` 必須同時出現 `cwd` 與 `setting_sources`**。缺任一項 Skill 就不會被載入，而且**沒有錯誤訊息**——ADR-023 記錄的那次靜默失效。schema 的 pattern 是覆核的底線，不是實跑的替代。
- `verification_prompt` 與 `verification_steps` 至少要有一個（`PACK-007`）。`standard` 走 steps（重新匯入得同一個雜湊），兩個 Profile 走 prompt。

兩個 Profile 的安裝路徑實際相同，差別在**使用者層 vs 工作目錄層**與驗證方式（PDM-008 v4 依實測釐清），`known_limitations` 必須把這件事講清楚，否則使用者會以為是重複選項。

`mcp_config` 是型別佔位、恆為 null：遠端 MCP 已移出 MVP 首發，同 `TRACE-003` 的 `mcp_call` 與 `EVAL-008` 的既有做法。

## 5. 版本演進規則

三份都用 `schema_version`，形如 `MAJOR.MINOR`，目前皆為 `1.0`。規則與 [contracts/events](../events/README.md) §9 相同：

**Minor bump（additive，消費者不需改）——允許：**

- 新增 optional 欄位。
- 放寬既有限制（`maxLength` 調大、enum 加值）。

**Major bump（breaking，需新 `$id` 與轉譯層）——以下任一即是：**

- 移除或改名任何欄位。
- 把 optional 改成 required。
- 收緊既有值域（enum 刪值、nullable 改 non-null）。
- 改變既有欄位的語意。

**版本宣告的規則**：產生者宣告的是它「照哪一版契約寫」；只寫舊欄位的產生者繼續宣告舊版本是正確的，而舊版本的實例必然是新 minor 的合法實例。消費端接受任何 `1.x`。

**`manifest_hash` 的語意一旦有第一個 Download Artifact 存在就不能再改**（[packaging-design.md](../../docs/plans/mvp/m4/packaging-design.md) §2.4）。改它是 major，而且是需要 ADR 的那一種。

**值域鏡像**：`license.source_tier` 是 `skill_versions.license_source` CHECK 的複本，`compatibility.capability`／`behaviour` 是 `skill_runtime_compatibility` 兩欄 CHECK 的複本，`origin.improvement.suggestions[].category` 是 `evaluation_suggestions.category` CHECK 的複本。**DB 是定義，這裡是複本**；改 DB 的 CHECK 必須同步這裡，並依上述規則判定是 minor 還是 major。

## 6. 反例清單（validator 的另一半）

`examples` 只證明 schema 收得下對的東西。以下 18 個反例證明它擋得住錯的東西，全部在 `tools/contracts/validate_packaging.py`：

| schema | 反例 | 擋的是什麼 |
| --- | --- | --- |
| manifest | 有 expression 沒有 source_tier／有 tier 沒有 expression | ADR-021 決策 1 |
| manifest | `expression: "NOASSERTION"` | ADR-021 決策 7 |
| manifest | `behaviour: native` 但沒有 `runtime_image` | 外推一次沒發生過的量測（`04` 乙-4） |
| manifest | improvement 溯源夾帶 `problem`／`expected_impact` | 鐵律 11 |
| manifest | `validation.blocked: true` | 被拒的套件不會有 manifest（`PACK-002`） |
| manifest | fork 只記一跳、沒有原始來源 | `02:DISC-003` 第 5 條 |
| manifest hash 輸入 | 含 `skillhub-manifest.json`／含 `../` 路徑／值不是 sha256 | §3.3 |
| profile | `standard` 加 frontmatter | PDM-008 |
| profile | `frontmatter_additions` 覆寫 `description` | ADR-012 |
| profile | `claude-agent-sdk` 的 snippet 沒有 `setting_sources` | ADR-023 的靜默失效 |
| profile | 既無驗證 Prompt 也無檢查步驟 | `PACK-007` |
| profile | `env_vars[].example` 放真的金鑰樣式 | 鐵律 11 |
| portable test case | dataset 路徑 `data/../../...` | 解壓逃逸（§3.3 同一條理由） |
| portable test case | `origin: "user_upload"` | `PACK-005` 的可散布判準 |
| portable test case | rubric item 缺 `evidence_required` | `0026` CHECK 的對外形式 |
