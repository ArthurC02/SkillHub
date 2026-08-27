# ADR-027：Download Artifact 的形狀、可重現性與完整性

- 狀態：Accepted
- 日期：2026-08-17
- 決策者：產品負責人、架構規劃
- 相關：[ADR-012](./ADR-012-packaging-portability-and-agent-adapters.md)（打包可攜性與三層相容性，本 ADR 回答其待決策 3 與「可重現性」一節留下的問題）、[ADR-003](./ADR-003-data-ownership-and-storage.md)（物件存取、短效授權與刪除可追溯性）、[ADR-021](./ADR-021-skill-license-provenance.md)（License 溯源與來源層級）、[ADR-007](./ADR-007-trust-security-and-supply-chain.md)（Artifact 與下載、稽核事件）、[ADR-022](./ADR-022-sandbox-deployment-topology-and-security-thresholds.md)（GHCR attestation，本 ADR 決策 3 的對照組）
- 設計來源：[docs/plans/mvp/m4/packaging-design.md](../plans/mvp/m4/packaging-design.md) §2.4、§4.1、§4.5，[docs/plans/mvp/m4/contract-deltas.md](../plans/mvp/m4/contract-deltas.md) §3、§4.1
- 承接需求：`02:PACK-001`、`02:PACK-002`、`02:SEC-007`、`02:CONTENT-002`／`CONTENT-004`

## 背景

ADR-012 的「可重現性」一節寫著：「相同輸入與版本應能產生語意等價的套件；若內容雜湊因壓縮時間等非語意 metadata 不同，需另有規範化 Manifest Hash。」那句話指出了問題，但**沒有回答它**——規範化雜湊算什麼、不算什麼、與 `artifacts.content_hash` 是什麼關係，全部空著。同一份 ADR 的待決策第 3 條「打包 Artifact 簽章、完整性驗證與撤銷方式」也一直開著。

M4 是第一次真的要產出 Download Artifact，三件事因此同時到期：

1. **zip 的位元組不穩定。** entry 的 mtime、外部屬性、entry 順序與壓縮器版本都會讓同一份內容打出不同的位元組。打兩次得到兩個雜湊，`artifacts.content_hash` 的去重與「這是不是我上次拿到的那個檔」兩件事同時失去意義。
2. **簽章要不要做，必須是一個明文的決定。** 留在待決策裡，等於讓第一個實作者用「先不做」的方式決定它，而一旦有 Download Artifact 存在，補簽章就要處理已發出的那些。
3. **可散布性目前資料庫回答不了。** `skills`／`skill_versions` 沒有 `redistributable`／`source_available`／`license_status` 欄位。「source-available 一律不產出任何 Download Artifact」（PDM-002、PDM-008、ADR-012）今天只活在三個資料庫以外的地方——`tools/content/seed-skills.json` 的策展欄位、`catalog/trust.go` 的註解，以及 `0023_access_restriction.sql` 第 22 行的「Download packaging is unaffected here」。**最後那句話在 M4 之前為真，只因為沒有打包功能。**

三件事都會寫進資料模型與對外契約，而 manifest 的消費者有一個在 repo 之外（拿到 zip 的使用者與他的工具），所以改起來的代價遠高於現在寫清楚。

## 決策 1：兩個雜湊，各自回答一個問題，永不互相代替

| 雜湊 | 算什麼 | 回答什麼 | 存哪裡 |
| --- | --- | --- | --- |
| `content_hash` | 匯出 zip 的**全部位元組**的 SHA-256 | 「這個檔案是不是那個檔案」 | `artifacts.content_hash`；下載頁與 manifest |
| `manifest_hash` | 對 `{path: sha256(bytes)}` 依 path 排序後的 canonical JSON 取 SHA-256，**不含任何 zip metadata、不含 manifest 自身** | 「兩次打包的**內容**是不是一樣」 | `download_artifacts.manifest_hash`；manifest 內 |

- **兩個都要在契約上出現**（`DownloadArtifact` schema 與 `skillhub-manifest.json` 皆宣告兩者）。只給一個，使用者就得拿它回答它答不了的那個問題。
- **`manifest_hash` 不含 manifest 自身**，否則它自我指涉、算不出來；這一條要寫進 schema 的 description，不是寫在註解裡——它是使用者側重算時的必要前提。
- **冪等鍵是 `manifest_hash` 這一側的語意**：同一個 (skill_version, target, include_test_cases, packager_version) 重打包，回既有那一筆並標 `duplicate: true`，不產生第二份位元組（沿用 `EVAL-010` 的 `duplicate` 前例）。
- **去重不跨打包器版本**：`packager_version` 是冪等鍵的一部分，理由見決策 2。

## 決策 2：zip 寫入規範化，`content_hash` 在同一個打包器版本內可重現

匯出時一律：entry 依 path 排序、mtime 一律寫 `1980-01-01T00:00:00Z`（zip 的最小合法值）、外部屬性固定、不寫 extra field、壓縮等級固定。

- 做到這一步之後，**同一個打包器版本對同一個來源版本重打包，得到逐位元組相同的 zip**。
- **跨打包器版本不保證，而且刻意不保證。** 壓縮器實作換版就可能改變位元組，而為了維持跨版本的位元組穩定，等於把壓縮器實作釘成公開契約。因此 manifest 記 `packager_version`，並在 schema description 寫明可重現性的範圍是「同一個打包器版本內」。
- **可重現性寫成契約文字，不寫成保證。** 下載頁不得出現「這個檔案永遠可以被重新產生」這類措辭；正確的措辭是「同一版本可隨時重新打包」——那句話為真，因為打包是冪等的（決策 1）。
- 這一條同時讓 `PACK-009` 的「可被重新驗證」有可機械判定的形式（packaging-design §2.3 的不變式：任一產出的 zip 丟回 `skillpkg.Validate` 必須 `Blocked == false`）。

## 決策 3：MVP 不對 Download Artifact 簽章——這是明文的「不做」，不是待辦

**回答 ADR-012 待決策 3。** MVP 不產生數位簽章、不維護撤銷清單、不散布公鑰。理由四條：

1. **下載的形態是登入後的短效授權，不是公開發布。** ADR-003 已定「下載使用短效授權，不公開永久物件 URL」，而 M4 取「平台代傳位元組」（packaging-design §7.1 選項 A）。簽章要解決的「拿到檔案的人怎麼知道是你給的」，在這個形態下已由 Session、TLS 與平台代傳回答——檔案是使用者自己從登入的平台上拉下來的。
2. **簽章不是一個欄位，是一組生命週期。** 產生、保管、輪替、撤銷清單、公鑰散布，其中撤銷清單本身需要一個公開端點與一份撤銷政策。MVP 沒有任何一項的維運對象。
3. **沒有驗證端。** 三個打包目標（`standard`／`claude-code`／`claude-agent-sdk`）全部是把檔案放進使用者自己的目錄，**沒有任何一個 Agent 會檢查簽章**。簽了而沒有人驗，得到的是安全劇場，而安全劇場的代價是它會被讀成一種背書。
4. **平台已經有一個對照組。** `SBX-011` 對 Runtime Image 走的是 Sigstore keyless attestation，那條路成立是因為**驗證端存在且有動機**（閘門 A 的節點准入探針）。Download Artifact 沒有那個驗證端。同一個機制在有驗證端時採用、沒有時不採用，是同一條判準的兩個結果。

**不做的代價寫清楚，不淡化**：

- 套件離開平台後，**平台無法證明某一份 zip 是它產的**，第三方也無法離線驗證來源。
- **撤銷在 MVP 只到「停止再發」**（`access_restriction`／`redistribution` 阻擋、到期刪除），**不到「使已發出的副本失效」**。這是本決策最實質的殘餘風險。
- 因此 manifest 的 schema description 與下載頁**都不得暗示套件帶有平台背書或完整性保證**；`content_hash` 是給使用者自己比對用的事實，不是簽章。

**重開的訊號（出現任一即另立 ADR）**：(a) 第三方 Agent 或 registry 要求簽章才收；(b) 平台開始公開發布而非登入後短效授權；(c) 出現冒名散布本平台套件的實例。屆時優先評估 Sigstore keyless（與 `SBX-011` 同一套機制，不新增金鑰保管責任）。

## 決策 4：可散布性是一個資料庫欄位——`skills.redistribution`，三態，預設 `unknown`

- 值域 **`allowed`／`blocked`／`unknown`**，NOT NULL，**預設 `unknown`**；**只有 `allowed` 放行**，`unknown` 視同 `blocked`。
- **放 `skills` 不放 `skill_versions`**：授權事實屬於來源，且它是一個**可撤銷的判定**——不可變表（鐵律 4）放不進可撤銷的東西。理由與 `0023` 的 `access_restriction` 完全相同，兩者同層。
- **隨 Fork 複製**（同 `access_restriction`）。未傳播即等於「Fork 一次就解除」。
- **判準（把既有政策寫成可判定的形式，不是新政策）**：
  1. `license_expression IS NULL` → `unknown` → 阻擋（`02:DISC-003`：授權未知不得暗示可自由修改或再發佈）。
  2. `license_expression` 在 OSI 允許再散布的清單內 → `allowed`。
  3. 已知的 source-available 條款 → `blocked`。
  4. 認得但無法歸類 → `unknown` → 阻擋。
  5. **`license_status = Confirmed` 不得成為放行條件**——`02:CONTENT-002` 明文「已人工確認不等於可再散布」。
- **預設 `unknown` 而不是 `allowed`**：ADR-021 §5.3 記錄的那個真實誤判（「repo 根有 MIT ⇒ 子目錄是 MIT」）錯的正是放行方向。在放行方向上錯不起的欄位，預設值必須是保守的那一個。
- **兩道鎖都要，不可互相取代**：

  | 鎖 | 性質 | 涵蓋 |
  | --- | --- | --- |
  | `access_restriction`（`0023`，既有） | 人工按下的**暫時性 hold** | 今天已知的個案（現行只有 `license-review` 一個原因碼） |
  | `redistribution`（本決策，新增） | **內容屬性** | 每一個 Skill，包括明天使用者自己上傳的那一個 |

  拿 hold 當可散布性判準，等於宣稱「沒有人特別擋它就是可以散布」。**未知原因碼一律 fail-closed 視為受限**（沿用 `0023` 讀取端的既有裁定）。
- **回填**：45 個種子 Skill 的事實已存在於 `tools/content/seed-skills.json` 的策展欄位；回填腳本比照 `tools/content/backfill-agent-compatibility.sql` 的既有形式（可重跑、逐列可追溯）。
- **本決策讓 `0023_access_restriction.sql` 第 22 行「Download packaging is unaffected here」在 M4 之後仍然為真**，但理由換了：不是「沒有打包功能」，而是「打包由兩道鎖各自擋住，`access_restriction` 不必兼差」。

## 決策 5：manifest 是對外契約，三條界線寫進 schema description 而非註解

`skillhub-manifest.json` 的 schema 落在 `contracts/packaging/download-manifest.schema.json`（`contracts/` 第一次承載非 OpenAPI、非事件的契約，形狀比照 `contracts/events/` 的既有慣例）。三條界線是**契約文字**，因為讀它的人有一個在 repo 之外：

1. **`compatibility` 三層分開（格式／能力／行為），且 `behaviour` 的值只能來自該 (Skill Version × Runtime Image) 的實測列**；沒有列就是 `unverified`，**不得從同一個 Skill 的別的版本或別的映像外推**（`04` 乙-4 的既有裁定：換映像即回到未驗證直到重測）。schema 要說明「本欄不是承諾，是一次量測的紀錄」。**不得因格式驗證通過而暗示裝得起來**（ADR-012）。
2. **`license.expression` 與 `license.source_tier` 同生同滅**（ADR-021 決策 1，DB 側的 CHECK 在 `0012`，這裡是它的對外形式）；`expression` 為 null 時 `source_tier` 必須為 null，且**不得填 `NOASSERTION` 字串**（ADR-021 決策 7）。
3. **`manifest_hash` 不含 manifest 自身，也不含任何 zip metadata**（決策 1）。

manifest 另必須記 ADR-012「可重現性」列舉的六項（來源 Skill Version、Profile 及版本、打包器版本、建立時間與內容雜湊、驗證結果、含／不含的 Test Case 清單），加上 `PACK-003` 的溯源（`origin.kind` 三態：`import`／`fork`／`improvement`，Fork 鏈走到底，鏈上任一跳來源已刪除時記 `"unavailable"` 而不是省略）。

## 影響

### 正面

- 「重打包會不會得到同一個檔」與「我改的是 Profile 還是 Skill」是兩個不同的問題，現在有兩個不同的欄位可以回答，不必靠讀者自己分辨。
- 冪等的下載端點成立：重按一次不產生第二份位元組，也不需要一個「重新打包」端點（多一個端點就多一條繞過四道檢查的路）。
- 可散布性從三個資料庫以外的地方收回到一個欄位，`SEC-007`「授權未知或不可散布者不得進入打包流程」第一次成為可以用一句 SQL 檢查的事。
- 簽章從一個沒有人負責的待決策，變成一個有理由、有代價、有重開訊號的「不做」。

### 成本與限制

- **規範化 zip 寫入是打包器的義務**，不是壓縮函式庫的預設行為；換函式庫或換版本時必須重驗這一條，否則 `content_hash` 會安靜地開始漂移。
- **可散布性判準只到「認不認得這個授權運算式」的程度**，繼承 ADR-021 已記錄的召回率限制（marker 比對而非相似度比對）。認不出的落在 `unknown` 因而被擋——方向安全，但會誤殺一部分其實可散布的內容。這是刻意接受的方向。
- **`redistribution` 是人工可改的判定**，因此它與 `access_restriction` 一樣需要 operator 動作與 audit（`SEC-011` 的窮舉清單目前不含它，啟用寫入端點時必須先擴充該清單，不得默默多一個 operator 能做的事）。
- **不簽章的殘餘風險是不可逆的**：已下載的副本無法撤回。這一條不會因為日後補上簽章而回溯解決。

## 待決策

- **PDM-006 的 Download Artifact 保存期限（提案 90 天）尚未追認。** 本 ADR 不代為定值；`expires_at` 的值放部署設定並在建立時計算，migration 只留註解指向 PDM-006（把「已定值」與「已被追認」分開，是 `03` §1 整節存在的理由）。
- ~~**`redistribution` 的寫入端**（operator 端點或策展腳本）與其 audit 形式：M4 首發只需回填與讀取，寫入端點屬 `SEC-011` 的窮舉清單擴充，屆時另議。~~
  → **已決，分兩次**：形式於 2026-08-23 補上（`PUT /admin/skills/{id}/redistribution`，operator-only、`note` 必填、欄位與 audit 同交易，[`05` R-3c](../plans/05-pending-rulings.md)）；**誰能改與要什麼證據**於 2026-08-27 由 [ADR-057](./ADR-057-releasing-content-takes-named-evidence-not-a-button.md) 裁定（operator-only ＋ 具名來源層級證據且必須與凍結快照相符）。**本項當時寫「屆時另議」是對的**——先補的是紀錄，後補的才是判準，而兩者之間那四天的路由刻意取窄，就是為了讓後面這一次不必收回任何東西。
- **對外輸出 SPDX／SBOM 文件的時點與欄位映射**（承 ADR-021 待決策最後一項）：本 ADR 的 manifest 是平台自訂格式而非 SPDX，兩者的映射在需求出現時再定。
- **簽章的重開訊號**見決策 3；出現任一即新增 ADR，不在本 ADR 內就地改寫。
