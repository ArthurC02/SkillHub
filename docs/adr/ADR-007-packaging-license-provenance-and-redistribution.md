# ADR-007：打包、授權溯源與散布

- 狀態：Accepted
- 相關：[ADR-002｜資料所有權與核心基礎設施](./ADR-002-data-ownership-and-core-infrastructure.md)（下載短效授權、物件儲存）、[ADR-004｜Sandbox 隔離與執行安全](./ADR-004-sandbox-isolation-and-execution-security.md)（Runtime Image 的 Sigstore attestation，本 ADR 簽章決策的對照組）、[ADR-006｜身分、Workspace、准入與額度](./ADR-006-identity-workspace-admission-and-allowances.md)（operator 身分、`workspaces.is_catalog`）、[ADR-010｜產品分析與稽核邊界](./ADR-010-product-analytics-and-audit-boundaries.md)（audit event 的既有形式）、[ADR-011｜從描述生成 Skill](./ADR-011-generating-a-skill-from-a-description.md)（`skills.redistribution` 的 `generated` 值）

## 背景

Skill Hub 把外部與使用者自帶的 Skill 收進目錄、打包、交付下載，三件事因此必須同時成立：**授權事實要能回收而不能被冒充**、**「可不可以把這份內容交給別人」要有一個機器判得出來的答案**，以及**交出去的那個 zip 本身要可稽核、可重現，且它宣稱的每一句話都要有東西在背後強制**。三者共用同一組資料（`skills`／`skill_versions`）與同一段管線（打包器），任何一處鬆動都會讓另外兩處的保證失真。

## 決策

### 決策 1：標準 Skill 為核心事實來源，平台差異只交給 Agent Packaging Profile

- Skill Registry 以標準 Agent Skill Package 為核心事實來源；平台專屬差異（安裝路徑、工具命名、frontmatter 擴充欄位）由 **Agent Packaging Profile** 承載，不寫回標準 Skill Version。
- MVP 首批名單已追認：標準套件 ＋ 兩個已驗證安裝 Profile，各一份 `contracts/packaging/profiles/*.json`。
- Profile 是**資料**不是編譯常數：`contracts/packaging/profiles/*.json`（`schema_version 1.0`），程式面由 `skill/delivery/profile.go` 讀取。不引入外掛機制——MVP 打包目標全部內建（標準套件、`claude-code`、`claude-agent-sdk`），先做一套外掛系統是為一個不存在的第二方鋪路。
- Adapter 不得靜默改變 Skill 的任務意圖，也不得移除必要安全限制。標準套件、目標平台產物與安裝說明分開版本化及驗證。
- 相容性分三層，**不宣稱跨模型行為一致**：

  | 層級 | 定義 | 證據 |
  | --- | --- | --- |
  | 格式相容 | 套件與 `SKILL.md` 符合標準結構 | 規格驗證報告 |
  | 能力相容 | 目標 Agent 具備需要的工具、MCP、檔案與 Runtime | Capability Matching |
  | 行為相容 | 在特定 Agent、模型與環境完成 Test Case | Run 與 Evaluation 證據 |

  `manifest` 對外輸出時三層分開陳述，且 `behaviour` 只能來自該 (Skill Version × Runtime Image) 的實測列；沒有實測列即 `unverified`，**不得從同一 Skill 的別的版本或別的映像外推**——換映像即回到未驗證直到重測。格式驗證通過不得暗示裝得起來。

- 打包管線固定順序：選擇不可變 Skill Version → 驗證標準格式 → 檢查來源與 License → 掃描 Secrets、內部路徑與受保護資料 → 選擇 Agent Packaging Profile → 產生目標差異與安裝說明 → 驗證輸出套件 → 產生不可變 Download Artifact。

### 決策 2：授權以「運算式＋來源層級」成對記錄，永不壓成單一字串

`skill_versions` 同時記 `license_expression` 與 `license_source`，兩者同生同滅（`0012` 的 CHECK 約束）；frontmatter 寫的 `MIT` 與 repo 根撿來的 `MIT` 不是同一個主張，混成一個字串會抹平這個差別。

五層優先序，**上層存在即停止搜尋，不向下回退**（即使該層授權全文無法辨識也不採用下一層——套件自己聲明了授權，就不該由更弱的證據代答）：

| 層 | `license_source` | 證據 |
| --- | --- | --- |
| 1 | `manifest` | `SKILL.md` frontmatter 的 `license`，作者自宣告 |
| 2 | `manifest-referenced-file` | frontmatter 寫的是指向套件內某檔案的**指標**（如 `SEE LICENSE IN <file>`、`Complete terms in <file>`），指標所指檔案以既有 marker 成功解析出運算式 |
| 3 | `package-license-file` | 套件根的 `LICENSE`／`COPYING` 類檔案（檔名大小寫不敏感） |
| 4 | `repo-license-file` | 打包器搬入的 repo 根授權，固定檔名 `LICENSE.repo`（掃描器對此檔名採精確比對） |
| 5 | `curated-declared` | 策展流程宣告的 repo 層事實，套件內無對應檔案；**已定義語意與排序，但不開放 API 表面**（無稽核路徑） |

排序準則是**證據可稽核的程度**：第 1–4 層都留在套件內、進入內容雜湊與不可變快照，任何人拿到同一份套件都能重驗；第 5 層做不到。

打包器切出 monorepo 子目錄時，**若且唯若該目錄自身沒有任何 license 檔**，才把 repo 根授權搬入，寫出兩個檔案：`LICENSE.repo`（逐位元組原樣複製，不加註記）與 `LICENSE.repo.provenance.json`（記 `carried_from`、`repo_url`、`commit`、`skill_path`）。第 3、4 層每次搬運與回退都要對外揭露（`license-from-package-file`／`license-from-repo-file` info，後者訊息明講「涵蓋 repository，不必然涵蓋本套件內容」），不得靜默把偵測結果升格為宣告。

其餘規則：

- manifest 的自由文字正規化為 SPDX License List 的 canonical 大小寫，接受少量無歧義俗寫（`apache 2.0`、`GPLv3`）；**無法對應者原樣保留**，不猜測（裸寫的 `BSD`、`GPL` 沒說是哪個變體）。
- 未知以 `license_expression IS NULL` ＋ `LicenseStatusUnknown` 表達，欄位本身只存純運算式；對外輸出 SPDX／SBOM 時再映射為 `NOASSERTION`。
- 五層全部只到「已宣告」（`LicenseStatusDeclared`，`skill/discovery/trust.go`），升級到「已人工確認」（`LicenseStatusConfirmed`）是另一條軸，只能由人工複核給予並留下判定理由；**`Confirmed` 不等於可再散布**——source-available 授權可以通過人工確認卻依然被決策 3 的散布閘門擋下。

### 決策 3：可散布性是資料庫欄位 `skills.redistribution`，五態，兩道鎖分工不互相取代

- 值域 **`allowed`／`blocked`／`unknown`／`self_supplied`／`generated`**，NOT NULL，**預設 `unknown`**；只有 `allowed` 放行，`unknown` 視同 `blocked`。放在 `skills` 不放 `skill_versions`——這是一個可撤銷的判定，不可變表（見 [ADR-002｜資料所有權與核心基礎設施](./ADR-002-data-ownership-and-core-infrastructure.md)）放不進可撤銷的東西。隨 Fork 複製；未傳播即等於「Fork 一次就解除」。
- **策展目錄（`allowed`／`blocked`／`unknown`）的判準**：
  1. `license_expression IS NULL` → `unknown` → 阻擋。
  2. `license_expression` 在 OSI 允許再散布的清單內 → `allowed`。
  3. 已知的 source-available 條款 → `blocked`。
  4. 認得但無法歸類 → `unknown` → 阻擋。
  5. `license_status = Confirmed` 不得成為放行條件。
- **非策展工作區匯入的內容給 `self_supplied`**（判準是 `workspaces.is_catalog`，不是「是不是上傳」）：把一個工作區自己送上來的位元組交還給同一個工作區不是散布，是取回，沒有第三人收受。上傳（`upload`）與 URL 匯入（`git`）兩種來源都給 `self_supplied`——URL 匯入的位元組雖未經使用者的手，但是他指名要平台去抓、抓進他自己的私有工作區，平台沒有多做出一個他自己做不到的散布動作。**Fork 只讀呼叫者自己的工作區或策展工作區，策展工作區不可能持有這個值**，所以逐字複製這個值仍然安全。`self_supplied` 通過打包閘門但不主張任何關於授權的事，畫面措辭是「可下載（你自己帶進來的）」而非「可打包下載」。
  策展工作區的匯入維持 `unknown`，等人判定——**判準寫錯（例如寫成「上傳進來的就是 self-supplied」）會讓整個策展目錄與其全部 Fork 一次繞過授權判定**，這是本決策最主要的失效模式，需獨立測試覆蓋。
- **兩道鎖各自涵蓋不同範圍，不可互相取代**：

  | 鎖 | 性質 | 涵蓋 |
  | --- | --- | --- |
  | `access_restriction`（既有） | 人工按下的暫時性 hold | 今天已知的個案 |
  | `redistribution`（本決策） | 內容屬性 | 每一個 Skill |

  未知原因碼一律 fail-closed 視為受限；拿 hold 當可散布性判準等於宣稱「沒有人特別擋它就是可以散布」，方向錯誤。

### 決策 4：把別人的內容標成可散布，要具名的證據，不是一個按鈕

- **只有 operator 能把 `redistribution` 改為 `allowed`**。理由不是保守，是一個已發生過的真實誤判：兩個 repo 的根目錄 `LICENSE` 是合法的 MIT 檔，但其涵蓋的內容並非該作者所有（實為另一個開源專案的 source-available 衍生物）——「repo 根有 MIT ⇒ 子目錄是 MIT」錯在放行方向，而**這個判斷連做過授權查核的人都會做錯，交給按下按鈕的人不會更準**。代價：使用者匯入第三方內容而授權判定不足時沒有自助路徑，看到的就是「不能下載」。
- **改成 `allowed` 必須同時帶 `license_expression` 與 `license_source`，且兩者都要與該 Skill 最新版本凍結的快照相符**；不符即拒絕，並在拒絕訊息裡說出快照記的是什麼。三種拒絕分開表達：

  | 情況 | 說法 |
  | --- | --- |
  | 沒填 | 少了兩個欄位 |
  | 快照什麼都沒記 | 這個 Skill 的最新版本沒有任何授權紀錄，重新匯入才有 |
  | 填了但不符 | 快照記的是 X from Y |

  比對去空白且不分大小寫（SPDX 識別字本身不分大小寫）。這道檢查買到的是「可以被反駁」——一組具名的運算式與層級可以被平台當場否定，一個確認框不能。
- **只有 `allowed` 要證據；`blocked` 與 `unknown` 不要**——要求為了「擋」而先做一次授權判定，等於對「拒絕做判定」收費，方向錯誤。
- **稽核事件記的是快照的值，不是操作者打的字**（`license_expression`／`license_source` 取自版本列）；`blocked`／`unknown` 的事件不帶這兩個鍵，而不是帶空字串——空的授權擺在一次封鎖旁邊會被讀成「在沒有證據的情況下放行」。
- **不做**：不改成逐版本的判定——欄位在 Skill 上，判定就在 Skill 上；一份標成 `allowed` 的 Skill 匯入新版本之後，那個判定仍然有效，而新版本的授權可能不同，今天沒有機制會重新問一次。**證據檢查與寫入同交易，擋掉的只是「檢查期間有新版本落地」，不是「日後有新版本落地」**；重開訊號是同一個 Skill 出現第二個來源層級，或封測出現第一次版本間授權變更。不啟用決策 2 第 5 層 `curated-declared` 作為放行依據（它是唯一證據不隨套件走的層級，拿到位元組的人無法重驗）；不做自助放行路徑；`repo-license-file`（決策 2 第 4 層）仍可作為放行依據，不因本決策而降為不足——它正是決策 1 那個誤判的形狀，先讓「所有以此層為據放行的 Skill」查得出來，再決定要不要禁。

### 決策 5：Download Artifact 的完整性用兩個雜湊，各自回答一個問題

| 雜湊 | 算什麼 | 回答什麼 | 存哪裡 |
| --- | --- | --- | --- |
| `content_hash` | 匯出 zip 的全部位元組的 SHA-256 | 這個檔案是不是那個檔案 | `artifacts.content_hash` |
| `manifest_hash` | 對 `{path: sha256(bytes)}` 依 path 排序後的 canonical JSON 取 SHA-256，不含任何 zip metadata、不含 manifest 自身 | 兩次打包的內容是不是一樣 | `download_artifacts.manifest_hash`；manifest 內 |

- 兩者都在契約上出現，缺一個使用者就得拿它回答它答不了的問題。`manifest_hash` 不含 manifest 自身，否則自我指涉算不出來——這一條寫進 schema description。
- **冪等鍵是 `manifest_hash` 這一側的語意**：同一個 (skill_version, target, include_test_cases, packager_version) 重打包，回既有那一筆並標 `duplicate: true`，不產生第二份位元組；**去重不跨打包器版本**（`packager_version` 是冪等鍵的一部分）。
- **`expires_at` 是部署設定算出來的，不是 migration 寫死的**：值取自 `DOWNLOAD_ARTIFACT_RETENTION`，在建立 Artifact 的當下加到現在時間上。期限不得短於當期的使用者觀察窗，理由與其他保存期限的相互約束一起寫在規格裡。
- **zip 寫入一律規範化**：entry 依 path 排序、mtime 固定寫 `1980-01-01T00:00:00Z`、外部屬性固定、不寫 extra field、壓縮等級固定。同一個打包器版本對同一來源版本重打包，得到逐位元組相同的 zip；**跨打包器版本不保證，且刻意不保證**——為了位元組穩定把壓縮器實作釘成公開契約不值得。可重現性只寫成「同一版本可隨時重新打包」，不寫成「這個檔案永遠可以被重新產生」。

### 決策 6：MVP 不對 Download Artifact 簽章

不產生數位簽章、不維護撤銷清單、不散布公鑰。理由：

1. 下載的形態是登入後的短效授權，不是公開發布——平台代傳位元組，「拿到檔案的人怎麼知道是你給的」已由 Session、TLS 與平台代傳回答。
2. 簽章不是一個欄位，是一組生命週期（產生、保管、輪替、撤銷清單、公鑰散布），MVP 沒有任何一項的維運對象。
3. 三個打包目標全部是把檔案放進使用者自己的目錄，**沒有任何一個 Agent 會檢查簽章**——簽了而沒有人驗是安全劇場，代價是它會被讀成一種背書。
4. 平台已有對照組：Runtime Image 走 Sigstore keyless attestation 是因為驗證端存在且有動機（節點准入探針）；Download Artifact 沒有那個驗證端。

**代價不淡化**：套件離開平台後，平台無法證明某一份 zip 是它產的，第三方也無法離線驗證來源；撤銷在 MVP 只到「停止再發」（`access_restriction`／`redistribution` 阻擋、到期刪除），**不到「使已發出的副本失效」**，這是不可逆的殘餘風險，不會因日後補簽章而回溯解決。manifest schema 與下載頁不得暗示套件帶有平台背書或完整性保證。

**重開訊號**（出現任一即另立 ADR，屆時優先評估 Sigstore keyless）：第三方 Agent 或 registry 要求簽章才收；平台開始公開發布而非登入後短效授權；出現冒名散布本平台套件的實例。

### 決策 7：Manifest 是對外契約，界線寫進 schema description 而非註解

`skillhub-manifest.json` 落在 `contracts/packaging/download-manifest.schema.json`。因為讀它的人有一個在 repo 之外，以下三條是契約文字：

1. 決策 1 的相容性三層分開陳述，`behaviour` 只承載實測結果，不是承諾。
2. `license.expression` 與 `license.source_tier` 同生同滅（決策 2）；`expression` 為 null 時 `source_tier` 必須為 null，不得填 `NOASSERTION` 字串。
3. `manifest_hash` 不含 manifest 自身、不含任何 zip metadata（決策 5）。

manifest 另需記載：來源 Skill Version、Packaging Profile 及版本、打包器版本、建立時間與內容雜湊、驗證結果、包含／排除的 Test Case 清單，以及 Fork 溯源（`origin.kind` 三態：`import`／`fork`／`improvement`，Fork 鏈走到底，鏈上任一跳來源已刪除時記 `"unavailable"` 而不是省略）。

### 決策 8：Agent Skills 規格的釘選、判準與「符合規格」這句話可以說到哪裡

- **事實來源是公開規格站台**（機器可讀版本與參考實作原始碼各一份）；規格沒有版本號、沒有 git tag、沒有 GitHub Release，因此**釘選用 commit SHA ＋ 規格檔的 blob SHA**，兩個都記在 `contracts/spec/SOURCE.json`，程式面以 `skillpkg.SpecRevision` 常數承載。
- **規格散文與參考實作衝突時，取參考實作為準**：一份沒有 JSON Schema 的規格，它的可執行陳述就是它的判準；偏離處寫進 ADR 而非留白。
- **規格 frontmatter 恰好六個欄位**：`name`、`description`、`license`、`compatibility`、`metadata`、`allowed-tools`（`version` 不是 frontmatter 欄位，只是 `metadata` 範例的慣例寫法）。其中：
  - `description` 去空白後非空 → **error**（agent 決定要不要載入這個 Skill 的唯一依據）。
  - `compatibility` 1–500 字元、且能被辨識為合法環境需求宣告（不得誤報 `frontmatter-unknown-field`）→ warning。
  - `metadata` 非 string→string → warning（非字串值至少會被一個 client 丟掉）。
  - `allowed-tools` 非空白分隔字串（例如寫成 YAML list）→ warning，且**照樣解析而不是丟掉**——丟掉會無聲放寬這個 Skill 被允許做的事。
  - **frontmatter 出現規格六欄之外的未知欄位 → error**（訊息同時說出怎麼修：搬進 `metadata` 或刪掉）。這與參考實作及至少一個主要 client 的上傳路徑對齊，避免「我們發出去的套件，使用者拿去別處才發現不合規」。**不接受對「本平台的目標」放寬、對「別人的目標」收緊**：同一個套件在三個 profile 下是同一份位元組，一份位元組不能有兩個合規判準。
- **打包器的排除與破壞要揭露，不得無聲**：匯出器移除 `.git/`、`node_modules/`、`.ssh/`、`.env`、symlink 與不安全路徑時，`excluded_files: [{path, reason, label, note}]` 記四個原因（`excluded_dir`／`credential_file`／`not_a_regular_file`／`unsafe_path`），同時進 manifest 與打包預覽（使用者決定要不要下載之前就看得到）。
- **新增 `blocked_reason: file_removed_by_packager`**，判準是誰造成的：`SKILL.md` 指向的檔案匯入時就不存在 → 作者的套件，`file-ref-missing` 警告、照常出貨；`SKILL.md` 指向的檔案在版本裡而被匯出器拿掉 → 平台弄壞的，**拒絕出貨**，不讓一個平台自己弄殘的套件長得像完好的。
- **檔案引用偵測涵蓋裸路徑**（不只 markdown link），錨點是解析器檢查第一段路徑必須是套件裡真的存在的目錄，避免誤報。
- **規格漂移偵測排程執行、開 issue，不進 merge gate**——別人的 repo 今天改了一行不是我們程式碼能不能合併的事實；唯一例外是改到 `contracts/spec/SOURCE.json` 或偵測器本身的 PR，那時「釘選對不對」才是我們程式碼的事實。「連不上規格站台」視同已漂移（exit 1），不是綠燈。

### 決策 9：Agent Plugin 是匯入認得的來源形狀，不是第二種一等公民

平台收的是兩份公開規格的產物：**Agent Skill**（一個目錄，根有 `SKILL.md`）與 **Agent Plugin**（一個目錄，`.claude-plugin/plugin.json` 之下綑綁 `skills/` 與其他元件）。兩者是容器與內容物的關係，不是兩個並列的領域概念。

- **匯入接受三種來源形狀**，三種走同一條管線、同一組驗證：①單一 Agent Skill；②Agent Plugin；③沒有 plugin manifest、但目錄樹裡有一個以上 `SKILL.md` 的來源。**第三種是 GitHub repo URL 匯入的常態**——抓回來的是整個 repo 的位元組，而 repo 極少剛好在根目錄放一份 `SKILL.md`。
- **展開後的單位仍然是 Skill Version**：一個來源展開出 N 個 Skill，就建立 N 個獨立版本，各自帶自己的內容雜湊、授權事實與驗證報告。**不引入 Plugin 實體，也不引入「多 Skill 版本」的聚合**——容器格式不足以成為領域概念，而 Plugin 層的事實由每個 Skill Version 的來源欄位承載（plugin `name`、`version`、`repository`，以及該 Skill 在 Plugin 內的相對路徑）。日後真的需要「以 Plugin 為單位下架」再談，那時要的是一個聚合，不是今天這個標籤。
- **Skill 目錄的探索順序固定且窮舉**：plugin manifest 的 `skills` 欄位（字串或陣列，規格明定它**加到**預設掃描而非取代）→ `skills/` → `.claude/skills/` → 來源根目錄的 `SKILL.md`。`.claude/skills/` 不在兩份規格裡，是宿主慣例；收它是因為真實 repo 大量把 Skill 放在那裡，而使用者指的是整個 repo。
- **找不到任何 `SKILL.md` 時，失敗訊息要說出它找過哪些位置。** 今天的訊息只說缺少 `SKILL.md`，對一個指著整個 repo 的人那不構成任何可行動的資訊——他不知道系統要的是根目錄、是 `skills/`、還是他根本指錯 repo。
- **非 Skill 元件一律不匯入、只揭露、永不執行**：`commands/`、`agents/`、`workflows/`、`output-styles/`、`hooks/`（含 `hooks.json`）、`.mcp.json`、`.lsp.json`。理由不是它們沒有價值，是 hooks 與 MCP／LSP server 定義**本身就是執行指令**（`command` 加 `args`）；把它們收進一個之後會被複製進沙箱的套件，等於讓匯入替使用者決定要執行什麼。它們走既有的 `excluded_files` 揭露（決策 8），原因碼 `plugin_component`。
- **plugin manifest 的未知欄位是 info，不是 error**——與決策 8 對 `SKILL.md` frontmatter 的處置相反，而那個相反是有理由的：frontmatter 收緊是因為**那是我們會再發出去的位元組**，寬鬆會讓使用者拿到別處才發現不合規；plugin manifest 不進我們產出的套件，而且它的欄位集合公開可見地還在長。只有 `name` 缺少或不是 kebab-case 是 error，因為那是來源標示唯一取得的欄位。決策 8 的六欄收緊只管 `SKILL.md`，不延伸到這裡。
- **plugin manifest 的 `license` 不成為新的授權來源層級。** 它宣告的是 Plugin 的授權，而打包器不會把 plugin manifest 搬進 Skill 套件，所以拿到套件的人無法重驗它——那牴觸決策 2 的排序準則（證據可稽核的程度）。沒有這一層，這類套件退到 `repo-license-file`，而那一層**本來就會揭露「涵蓋 repository，不必然涵蓋本套件內容」**：方向保守，沒有安全損失，少一個要維護的層級。
- **逐個 Skill 獨立判定，不整批連坐**：五個 Skill 裡四個通過一個被擋，就建立四個並逐個報出第五個被擋的原因；全部被擋才算匯入失敗。整批連坐會讓一個 repo 裡任何一個壞 Skill 擋住其餘全部，而那些是不同的位元組、不同的作者宣告。
- **單次匯入建立的 Skill 數量有上限**，超過即整批拒絕並說出上限與實際數量。它與 `MaxZipBytes` 守的不是同一件事：一個兩 MB 的 repo 可以合法地含有數百個 Skill，而那一次匯入會一口氣寫進目錄、一口氣吃掉索引增強的模型額度。值放部署設定，不在本 ADR 定（同本 ADR 對其他上限數字的既有處置）。
- **我們產出的仍然只有標準 Agent Skill 套件。** 匯入認得 Plugin，不代表下載會還原成 Plugin——決策 1 的打包形狀一個字不變。把 N 個 Skill 重新組回一個 Plugin 是一個發佈功能，不是匯入的對稱操作。

## 影響

### 正面

- 授權事實從三個資料庫以外的地方（策展腳本欄位、程式註解、遷移檔註解）收回到成對欄位與稽核得到的分層記錄，可散布性第一次是一句 SQL 能回答的問題。
- 使用者第一次下載得了自己上傳或用平台功能改出來的 Skill，而策展目錄的授權閘門一個字都沒放鬆。
- 「重打包會不會得到同一個檔」與「我改的是 Profile 還是 Skill 本身」是兩個不同的問題，各自有欄位回答；打包端點天生冪等，不需要額外的「重新打包」端點。
- 打包器不再無聲地把套件弄殘或隱藏規格層面的問題；`INSTALL.md` 的每一句合規宣稱使用者都能自己重查。
- 把別人的內容放行變成一件可以被平台當場否定的事，而不是一個確認框。

### 成本與限制

- 打包器與掃描器對「什麼算 license 檔」必須維持同一份名單，兩邊漂移會導致該搬的沒搬、或蓋掉套件自己的授權。
- 授權文字辨識沿用固定 marker 比對，遠不及業界工具的相似度比對；認不出的授權停在未知（不會猜錯），但召回率偏低。
- 可散布性判準只到「認不認得這個授權運算式」的程度，認不出的一律落在 `unknown` 因而被擋——方向安全，但會誤殺一部分其實可散布的內容，這是刻意接受的方向。
- `redistribution` 是人工可改的判定，需要 operator 動作與稽核；判定黏在 Skill 上，新版本落地不會使既有判定失效，也不會觸發重新檢查。
- 規範化 zip 寫入是打包器自己的義務，不是壓縮函式庫的預設行為，換函式庫或版本時必須重驗，否則 `content_hash` 會安靜地開始漂移。
- 不簽章的殘餘風險不可逆：已下載的副本無法撤回，日後補簽章也無法回溯涵蓋。
- 規格沒有版本號，釘選本質上是脆的；上游改一行，記錄的 blob SHA 就過期，漂移偵測是通知不是保護——已發出去的套件仍是照舊版驗的。規格與本平台驗證器分屬不同語言實作，沒有共用 schema 可對齊，只能靠人讀並接受它會漂移。
- 一個來源展開成 N 個版本之後，**對來源的動作失去單一施力點**：下架、重抓與內容比對都變成 N 筆各自獨立的事，而它們來自同一次匯入。決策 9 刻意不建聚合換來的就是這個代價，它會在第一個「整個 Plugin 要下架」的真實案例上現形。
- `.claude/skills/` 是宿主慣例不是規格，收它等於押注一個會變的慣例；它改名或被別的慣例取代時，探索順序要跟著改，而沒有任何上游規格會通知我們。
- Plugin 的非 Skill 元件被揭露但不匯入，使用者拿到的是一個**比他指的那個 repo 少東西**的結果。揭露說得出少了什麼，但說不出那些元件在他原本的環境裡做了什麼——落差要由他自己補。

## 待決策

- 授權辨識是否從 marker 比對升級為相似度比對，觸發訊號為誤判／漏判申訴量。
- 「宣稱寬鬆授權但內容並非作者所有」的偵測啟發式歸屬哪個能力範圍，以及是否自動轉人工。
- 決策 2 第 5 層 `curated-declared` 的啟用時點與策展事實的審核流程形態。
- 對外輸出 SPDX／SBOM 文件的時點與欄位映射。
- 自助放行路徑的形態（承決策 4 的代價；觸發訊號是使用者因授權判定不足而卡住的第一個真實案例）。
- `repo-license-file`（決策 2 第 4 層）是否需要額外證據才能作為放行依據，觸發訊號是「所有以此層為據放行的 Skill」這句查詢查出的第一批結果。
- `self_supplied` 的內容要不要能發佈到目錄，以及那條路徑要求什麼——今天沒有這條路徑，只保證它出現時會被擋下來要求判定。
- Agent Skills 規格 `name` 欄位是否放寬到 Unicode（參考實作允許，本平台不允許，生態系本身不一致）。
- 規格要求 `name` 必須等於父目錄名。決策 9 的展開路徑讓這一條**在多 Skill 來源上天生可驗**（每個 `SKILL.md` 的父目錄名是探索的產物），但單一 zip 上傳仍會剝掉頂層目錄因而驗不了；要不要讓同一條規格要求在兩條路徑上有不同強度，尚未裁定。
- 單次匯入建立的 Skill 數量上限取什麼值（決策 9）。它同時是兩件事的閘門——目錄被一次灌入的量，以及索引增強被一次吃掉的模型額度——而今天沒有任何一筆真實的多 Skill 匯入讀數可以當依據。
- 升級未知欄位為 error 的既有量測樣本全部來自既有策展來源；封測開始後應回頭量一次使用者自行上傳套件的真實拒絕率，非零則重看決策 8 而非使用者。
