# ADR-021：Skill License 溯源與多層 Provenance

- 狀態：Accepted
- 日期：2026-08-15
- 決策者：產品負責人、架構規劃
- 相關：ADR-003（不可變版本）、ADR-007（信任與供應鏈）、ADR-012（打包可攜性）、`02:DISC-003`／`DISC-004`、`plans/mvp/m1/import-report.md` §4／§8、`plans/mvp/m1/curated-skill-list.md` §5

## 背景

Skill Hub 的精選來源幾乎都是 **monorepo 的子目錄**：一個 repo 內含多個 `<dir>/SKILL.md`，平台把每個子目錄各自打包成一個 Skill 套件（`tools/content/import_seed.py`）。授權事實因此散落在三個層級：

1. `SKILL.md` frontmatter 的 `license` 欄位（作者對這個 Skill 的自宣告）；
2. Skill 目錄內的 `LICENSE` 檔（該目錄自帶的授權全文）；
3. **repo 根目錄的 `LICENSE` 檔**（涵蓋整個 repo，但不在任何 Skill 子目錄內）。

M1 首批匯入暴露了問題的規模：**45 個套件中 37 個在 frontmatter 未宣告授權**（import-report.md §4 Top-1），而其中多數 repo 的授權明白寫在根目錄 `LICENSE`。逐目錄打包會把第 3 層整個丟掉，目錄檢視因此對大量「授權其實寫得很清楚」的 Skill 顯示「授權未知」——而 `02:DISC-003` 規定授權未知即不得暗示可自由修改或再發佈，`ADR-012` 更據此封鎖 Download Artifact。低估授權＝可用內容被誤殺。

反方向的風險同樣真實。`curated-skill-list.md` §5.3 記錄了本次查核最重要的單一發現：**兩個 repo 的根目錄 `LICENSE` 是合法的 MIT 檔，但被它涵蓋的內容並非該作者所有**（實為 `anthropics/skills` 的 source-available 衍生物）。「repo 根有 MIT ⇒ 子目錄是 MIT」是錯的，而且錯在放行方向。

決策驅動因素：**既要回收第 2、3 層的真實授權事實，又不能讓較弱的證據冒充作者的自宣告。**

### 業界慣例調查

| 機制 | 「作者宣告」層 | 「掃描發現」層 | 「人工判定」層 | 明確優先序 |
| --- | --- | --- | --- | --- |
| SPDX 2.3／3.0 | `PackageLicenseDeclared` | `PackageLicenseInfoFromFiles` | `PackageLicenseConcluded` | Concluded 是最終答案；與其他兩層不符時**必須**在 `PackageLicenseComments` 書面說明 |
| GitHub／licensee | 無（不看 metadata） | `LICENSE` 檔比對，信心門檻 98% | 無 | 只有掃描層 |
| ClearlyDefined | `licensed.declared` | `licensed.facets.*.discovered` | curation | curation > 工具聚合；工具間依設定 precedence |
| npm | `package.json.license`（SPDX expression） | 無官方掃描 | 無 | metadata 唯一權威；`LICENSE` 檔僅在 `SEE LICENSE IN` 時被指向 |
| PyPI／PEP 639 | `License-Expression`（PyPI 驗證合法性） | 無官方掃描 | 無 | `License-Expression` **MUST** 覆蓋舊 `License` 欄位與 classifiers |

四項對本決策有直接約束力的事實：

- **沒有任何生態系會從父目錄繼承 `LICENSE`。** licensee 只掃 repo 根單層（`github_project.rb` 註解明載只掃根目錄）；`npm-packlist` 只收「套件根」的 license 檔，且不向上走；PEP 639 明文禁止 `license-files` glob 使用 `..`；ClearlyDefined 的 `isLicenseFile()` 只認套件根前綴。**繼承不是預設行為，要有就得由工具明確搬運。**
- **而搬運正是既有實務。** pnpm 已內建在 publish 前把 workspace 根的 `LICENSE` 複製進每個 workspace package；Yarn Berry 的同一需求至今仍是 open issue。這正好證明「預設不帶、需要工具層明確補上」是這個問題的標準解法。
- **「未知」與「不存在」必須可區分。** SPDX 的 `NOASSERTION`（無法或未判定）與 `NONE`（確定沒有授權資訊）語意不同；licensee 對外也是三態（`other`→`NOASSERTION`／`no-license`→`NONE`／欄位缺席）。
- **偵測結果不得靜默升格為宣告。** PEP 639 要求工具不得未經告知就用舊欄位或 classifier 填 `License-Expression`；SPDX 要求 Concluded 與 Declared 不一致時寫下理由。

值得注意的分歧：ClearlyDefined 的 curation 指引把「套件內 license 檔」排在「套件 metadata」**之前**，與 npm／PyPI 的 metadata 權威相反。差異來自階段不同——自動化管線信結構化欄位，人工稽核信授權全文。本 ADR 據此把「自動索引」與「人工複核」分成兩條獨立的軸（見下）。

## 評估選項

### 選項 A：只信 manifest

- 優點：實作最小，語意最乾淨，與 npm／PEP 639 的「metadata 唯一權威」一致。
- 缺點：**45 個套件中 37 個直接落到「授權未知」**，其中多數的授權在 repo 裡寫得清清楚楚。npm／PyPI 能只信 metadata，是因為其生態系的發布工具會強迫作者填欄位；Agent Skills 規格的 `license` 是選填，且現存內容普遍不填。照搬會讓目錄的主要價值（可再散布的 Skill）大面積消失。

### 選項 B：打包時把 repo 根 LICENSE 複製入包（附 provenance 註記檔）

- 優點：跟隨 pnpm 的既有解法；授權全文成為套件內容的一部分，因此進入不可變快照與內容雜湊（ADR-003），事後可稽核、可重現；對所有匯入路徑（上傳、URL）一視同仁，不需要匯入端額外配合。
- 缺點：單靠檔名無法表達「這是 repo 層、不是本目錄的」；若掃描器把它當成一般 `LICENSE`，就正中 §5.3 的誤判。**必須搭配獨立的來源層級標記才成立。**

### 選項 C：匯入 API 接受 curation 提供的 repo 層授權事實

- 優點：可承載人工判斷（例如 §5.3 那種「根有 MIT 但內容不是作者的」只有人看得出來）；ClearlyDefined 正是把 curation 放在最高優先。
- 缺點：**外部宣告在套件內不留任何證據**——內容雜湊、版本快照都證明不了它，日後無從稽核。且 ADR-011／鐵律 3 的既有立場是不信任呼叫端傳入的事實。ClearlyDefined 的 curation 之所以能有最高優先，是因為它是**經審核合併的 PR**，而不是 API 呼叫端隨手附帶的欄位——這兩者不是同一種東西。

### 選項 D：B＋C 分層（採用）

以 B 為搬運機制、以明確的**來源層級（license source tier）**表達強弱，C 作為模型中已定義但**暫不實作**的最低層。

## 決策

### 1. 授權以「運算式＋來源層級」成對記錄，永不壓成單一字串

`skill_versions` 同時記 `license_expression` 與 `license_source`；兩者同生同滅（`0012` 的 `CHECK` 約束）。理由：frontmatter 寫的 `MIT` 與 repo 根撿來的 `MIT` **不是同一個主張**，而 `02:DISC-003` 要求呈現的正是這個差別，不是把它抹平成一個字串。

### 2. 四層優先序，上層存在即停止搜尋

| 層 | `license_source` | 證據 | M1 實作 |
| --- | --- | --- | --- |
| 1 | `manifest` | `SKILL.md` frontmatter 的 `license`，作者對這個 Skill 的自宣告 | ✅ |
| 2 | `package-license-file` | 套件根的 `LICENSE`／`COPYING` 類檔案（**檔名大小寫不敏感**） | ✅ |
| 3 | `repo-license-file` | 打包器搬入的 repo 根授權，落於固定檔名 `LICENSE.repo` | ✅ |
| 4 | `curated-declared` | 策展流程宣告的 repo 層事實，套件內無對應檔案 | ⛔ 不實作（見下） |

排序準則是**證據可稽核的程度**，而非提供者的權威。第 1–3 層都留在套件內、進入內容雜湊與不可變快照，任何人拿到同一份套件都能重驗；第 4 層做不到。這也解釋了為何本 ADR 的第 4 層排在最低，而 ClearlyDefined 的 curation 排在最高——後者指的是經審核的修正，在本平台對應的是下述第 4 點的人工複核軸，不是 API 傳入的欄位。

**上層存在即停止，不向下回退**：即使第 2 層的授權全文無法辨識，也不採用第 3 層。套件自己聲明了授權（縱使讀不懂），就不該由 repo 的說法代答——那正是 §5.3 的誤判路徑。

### 3. 打包器搬運機制

打包器切出 monorepo 子目錄時，**若且唯若該目錄自身沒有任何 license 檔**，才把 repo 根的授權檔搬入，並寫入兩個檔案：

- `LICENSE.repo`——**逐位元組原樣複製**。授權是法律文件，在正文裡加註記等同竄改，因此註解另立檔案。
- `LICENSE.repo.provenance.json`——記 `carried_from`、`repo_url`、`commit`、`skill_path` 與說明文字。

檔名 `LICENSE.repo` 由工具產生而非作者撰寫，因此掃描器對它採**精確比對**；第 2 層的作者檔名則採大小寫不敏感比對（種子來源 `iamursky/sokrati` 使用小寫 `license`，原本的精確名單漏掉它，見 §5.1 第 8 列）。

### 4. 四層全部只到「已宣告」，升級到「已人工確認」是另一條軸

對應 SPDX 的 Declared／Concluded 之分：四層都是 `PackageLicenseDeclared` 等級的事實，一律對應既有的 `LicenseStatusDeclared`（`catalog/trust.go`）。**沒有任何自動層級可以把 Skill 推到 `LicenseStatusConfirmed`**，後者只能由人工複核給予，且如同 SPDX 對 Concluded 的要求，須留下判定理由。§5.3 的「宣稱 MIT 的 source-available 衍生物」就只能在這條軸上處理。

`LicenseStatusConfirmed` 仍不等於可再散布——source-available 授權可以通過人工確認卻依然封鎖 Download Artifact（`ADR-012`）。

### 5. 每一層搬運與回退都要揭露，不得靜默

第 2 層產生 `license-from-package-file`（info），第 3 層產生 `license-from-repo-file`（info，訊息明講「涵蓋 repository，不必然涵蓋本套件內容」）。此即 PEP 639「不得未經告知就把偵測結果升格為宣告」的落實。

### 6. SPDX 運算式正規化，但不猜測

manifest 是作者手寫的自由文字，正規化為 SPDX License List 的 canonical 大小寫（SPDX 3 起運算式大小寫不敏感，但 canonical 形式決定 License List URL，仍應存正規形）；另接受少量**無歧義**的俗寫（`apache 2.0`、`GPLv3`）。**無法對應者原樣保留**：裸寫的 `BSD`、`GPL` 沒說是哪個變體，猜一個比誠實回報作者原字串更糟（`02:DISC-003`）。

### 7. 未知以 NULL 表達，不寫 `NOASSERTION` 字串

SPDX 2.3 的運算式文法並未把 `NONE`／`NOASSERTION` 納入 `simple-expression`（3.0.1 才納入，且規定它們只能與 `AND` 併用、不得與 `OR` 併用）。為避免 `license_expression` 欄位同時承載「運算式」與「狀態」兩種語意，本平台以 **`license_expression IS NULL` ＋ `LicenseStatusUnknown`** 表達未知，欄位本身只存純運算式。對外輸出 SBOM／SPDX 文件時再映射為 `NOASSERTION`。

### 8. 第 4 層定義但不實作

`curated-declared` 在本 ADR 中已定義語意與排序，但 M1 **不開放 API 表面**，`0012` 的 `CHECK` 也不含此值。理由：目前沒有策展工作流會產生這種事實，而先鋪一個「呼叫端說什麼就記什麼」的欄位，違反鐵律 3 且無人稽核。啟用訊號是 `INGEST-010`／`CONTENT-009` 的失效與下架流程落地——屆時新增一次 `CHECK` 遷移即可，不需重做資料模型。

## 影響

### 正面

- **實測回收率（45 個種子套件離線重打包＋掃描）：可解析授權由 11／45（24.4%）提升至 45／45（100%）**，新回收 34 筆全部來自第 3 層且全為 MIT。分層後的分布為 `manifest` 8、`package-license-file` 3、`repo-license-file` 34。原本 37／45 的「未知」不再是資料缺口，而 34 筆的較弱證據仍以 `repo-license-file` 標明，未被冒充為作者自宣告。
- Download Artifact 的封鎖回到真正無授權的內容上，而非打包方式造成的假性未知。
- 授權證據留在不可變套件快照內，稽核與重現只需內容雜湊，不必回頭問上游 repo 當時長什麼樣。
- `license_source` 讓 §5.3 這類個案在資料層就可被挑出（「所有 `repo-license-file` 的 documents 類 Skill」是一句 SQL），而不是靠人工翻查。
- 與 SPDX 的 Declared／Concluded 分層對齊，日後輸出 SBOM 是欄位映射而非資料模型改造。

### 成本與限制

- 打包器與掃描器對「什麼算 license 檔」必須維持同一份名單（`import_seed.py:LICENSE_NAMES` ↔ `skillpkg.licenseFileNames`）；兩邊漂移會導致該搬的沒搬、或搬了卻蓋掉套件自己的授權。這是刻意接受的重複——跨語言共用常數的成本高於同步一份 8 個字串的名單。
- `LICENSE.repo.provenance.json` 內含 repo URL，會進入 `external-url` 揭露。事實正確但增加一筆雜訊。
- 授權文字辨識沿用 `skillpkg.detectLicense` 的固定 marker 比對，遠不及 licensee 的 98% 信心門檻＋Dice 相似度。認不出的授權停在未知（不會猜錯），但**召回率低於業界工具**；若誤判為未知的申訴出現，升級路徑是換成相似度比對，而非擴充 marker 清單。
- 既有版本列的 `license_source` 為 NULL（`0012` 的配對約束刻意標 `NOT VALID`）：為它們回填層級等同編造證據。要有溯源就得重新匯入。

## 待決策

- **manifest 寫的是「指標」而非授權時該如何處理。** 實測發現 `anthropics/skills` 的 `brand-guidelines`／`internal-comms` 在 frontmatter 寫 `license: Complete terms in LICENSE.txt`——那是指向檔案的指標，不是授權宣告。依本 ADR 的「上層存在即停止」，第 1 層勝出並原樣記下該字串，同目錄 `LICENSE.txt` 內真正的 Apache-2.0 因此未被採用。方向上是安全的（低估而非高估），但確實遺失了事實。npm 對此有既成慣例（`"SEE LICENSE IN <filename>"`，且該檔須位於套件頂層）。待決：是否辨識指標字串並回退至其所指的檔案，以及辨識範圍限於 npm 的字面形式或放寬到啟發式比對。
  → **已決（2026-08-15，commit `4815c5e`）：辨識，並回退至其所指的檔案。** 指標所指的套件內檔案以既有 marker 比對解析，成功則記為新的來源層級 **`manifest-referenced-file`**；`brand-guidelines`／`internal-comms` 因此解出 Apache-2.0。解析失敗（檔案不存在、文字無法辨識）則**維持原樣逐字記錄於第 1 層**，不改變既有行為、不猜測。
  - **另立層級而非併入第 1 層**：作者選定了該檔案（比「套件內剛好有 LICENSE」強），但運算式是從文字**讀出**而非**宣告**，兩者證據性質不同，依本 ADR §1「永不壓成單一字串」不得混同。優先序落在第 1 層與 `package-license-file` 之間。
  - **辨識範圍取保守的封閉正則集，不放寬到啟發式比對**：涵蓋 npm 的 `SEE LICENSE IN <file>` 慣例與實測到的 `Complete terms in <file>` 變體，且要求目標為套件根層的單一檔名（同 npm 對頂層的要求）。誤判的代價是把授權問題交給**另一個檔案**的文字，即授權誤標——方向上不再是本節原本「低估而非高估」的安全側，故從嚴。
  - 未涵蓋的措辭一律不視為指標，走原路徑逐字記錄；擴充觸發訊號同下一項，為誤判／漏判申訴量。
- 授權辨識是否從 marker 比對升級為相似度比對（licensee 式），觸發訊號為誤判申訴量。
- §5.3 的「宣稱 MIT 的 source-available 衍生物」偵測啟發式歸屬哪個需求 ID（`CONTENT-006` 或 `INGEST-010`），以及是否自動轉人工。
- 第 4 層 `curated-declared` 的啟用時點，與策展事實的審核流程形態。
- 對外輸出 SPDX／SBOM 文件的時點與欄位映射（`PACK-001` 之後）。
