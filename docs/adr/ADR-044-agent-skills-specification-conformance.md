# ADR-044：Agent Skills 規格的釘選、判準與「符合」這個詞可以說到哪裡

- 狀態：Accepted
- 日期：2026-08-22
- 相關：[ADR-012](./ADR-012-packaging-portability-and-agent-adapters.md)（可攜性三層與 Agent 轉接）、[ADR-027](./ADR-027-download-artifact-shape-reproducibility-and-integrity.md)（Download Artifact 的雜湊與可重現性）、`02:SKILL-002`、`02:PACK-001`／`PACK-007`

## 背景

一次針對交付物（下載回去的那個套件）的對帳，問了五個問題：完整性、準確性、一致性、即時性、有效性。**一致性那一題查出來的東西不是「做得不夠」，是「說了一句沒有東西支撐的話」。**

打包器產生的 `INSTALL.md` 對每一位下載者印這句：

> "The package is **valid against the Agent Skills specification**; that is a statement about its format and not about whether it installs or runs on your Agent."

而在這次之前，**這個 repo 裡沒有那份規格**——沒有 URL、沒有副本、沒有版本號、沒有任何 ADR 引用它，ADR-012 全文 grep `spec|規格|specification` 零命中。那句話唯一的實際意思是「通過了 `skillpkg.Validate`」，而 `Validate` 的必要條件只有三個：root 有 `SKILL.md`、frontmatter 有合法 `name`、有非空 `description`。

這是設計系統 §2.2「顯示與強制成對」的形狀，升級到產品層級——**一個沒有東西在強制的主張，而且它不是印在一個使用者可以爭辯的畫面上，是印在交出去的位元組裡**。`01` §12 風險表第 4 列（「Skill 符合規格但無法執行 → 下載後失敗」）談的是能力落差；這一條比那更基本：它是**主張本身沒有依據**。

規格是有的，而且是公開標準。它只是不在我們手上。

## 決策 1：釘選規格，用兩個雜湊而不是版本號

事實來源是 **<https://agentskills.io/specification>**（機器可讀：`/specification.md`；原始碼：`github.com/agentskills/agentskills` 的 `docs/specification.mdx`）。README 記載該格式 "was originally developed by Anthropic, released as an open standard"。

**`github.com/anthropics/skills` 已經不是規格所在地**：它的 `spec/agent-skills-spec.md` 現在整份只有 87 bytes，內容是一句指向 agentskills.io 的轉址；該 repo 現在是範例集合。任何仍指向它的引用都要改。

**規格沒有版本**——沒有 version string、沒有 `spec_version` 欄位、沒有任何 git tag、沒有任何 GitHub Release，`CONTRIBUTING.md` 也沒有 stability policy。所以**釘的是 commit SHA ＋ 規格檔的 blob SHA**，兩個都記，寫在 [`contracts/spec/SOURCE.json`](../../contracts/spec/SOURCE.json)，程式面以 `skillpkg.SpecRevision` 常數承載。

放在 `contracts/` 而不是 `docs/`，依 ADR-031 的收納語意：這是**跨程序介面的唯一來源**，而且是與 repo 外部的介面。它不是敘事文件。

**沒有 JSON Schema 可以引用。** 規格的唯一可執行陳述是參考實作 `skills-ref`（Python，`click` ＋ **strictyaml**）。

## 決策 2：規格散文與參考實作衝突時，以參考實作為準，並把偏離寫下來

規格的 `name` 條款自相矛盾——散文寫 "May only contain **unicode** lowercase alphanumeric characters (`a-z`, `0-9`)"，而括號裡的字集是 ASCII。參考實作 `validator.py` 的 docstring 逐字寫 `"Skill names support i18n characters (Unicode letters) plus hyphens."`，實作是 NFKC 正規化後 `all(c.isalnum() or c == "-" for c in name)`。

**取參考實作**：它是規格自己指定的驗證器，而一份沒有 schema 的規格，它的可執行陳述就是它的判準。

**本 ADR 不在這一批放寬 `name`**（本平台仍是 `^[a-z0-9]+(-[a-z0-9]+)*$`）。理由是順序：放寬永遠不會弄壞既有套件，所以它可以晚做；而**它會弄壞的是一致性本身**——同一個 Skill 在 Skill Hub 合法、在別處不合法，或反之，都要先有一個決定。列為待決策，不假裝已經解決。

## 決策 3：規格是六個欄位，而我們少認得一個

規格的 frontmatter 是**恰好六個**：`name`、`description`、`license`、`compatibility`、`metadata`、`allowed-tools`。

**`compatibility` 我們原本不認得**，所以一個正確宣告環境需求的 Skill（`compatibility: Requires Python 3.14+ and uv`）會被我們報成 `frontmatter-unknown-field` 警告。**對一個完全合法的套件報錯，比漏報更傷**——它教使用者忽略這一類警告。已修，並補上規格的 1–500 字元上限。

同批補上四條規格明訂而我們沒查的：

| 檢查 | 規格原文 | 本平台嚴重度 | 理由 |
| --- | --- | --- | --- |
| `description` 去空白後非空 | "Non-empty" | **error** | 這是 agent 決定要不要載入這個 Skill 的**唯一**依據（規格「Progressive disclosure」第 1 層）。一個只有空白的 description 不是風格問題，是一個永遠不會被撿起來的 Skill |
| `compatibility` ≤ 500 | "Must be 1-500 characters if provided" | warning | 超長不影響載入 |
| `metadata` 是 string→string | "A map from string keys to string values" | warning | 非字串值至少會被一個 client 丟掉，使用者會無聲失去資料 |
| `allowed-tools` 是空白分隔字串 | "A space-separated string" | warning | YAML list 是 client 擴充。**照樣解析而不是丟掉**——丟掉會無聲放寬這個 Skill 被允許做的事 |

`version` **不是** frontmatter 欄位。規格只在 `metadata` 範例裡示範 `version: "1.0"`，那是慣例。

## 決策 4：未知欄位維持 warning，但套件必須說出後果——而升級的解鎖條件是一次量測

規格的參考實作對未知欄位回傳 **error**（`f"Unexpected fields in frontmatter: ... Only {sorted(ALLOWED_FIELDS)} are allowed."`），至少一個主要 client 的上傳路徑也是硬錯。**所以現況是：我們發出去的套件，使用者拿去上傳時才會炸，而我們的 `INSTALL.md` 剛剛告訴他這份符合規格。**

**評估過的三個選項：**

| 選項 | 代價 |
| --- | --- |
| (a) 升級為 error | 正確地對齊規格，但**會回溯擋掉既有目錄內容**——驗證在讀取與打包時都會重算，而為 Claude Code 寫的 Skill 合法地帶著 `argument-hint`、`when_to_use` 等欄位。殺傷範圍**本機量不到**（QA 語料要抓 pin commit，需網路） |
| (b) 維持 warning，什麼都不說 | 現況。使用者在別處才發現 |
| (c) 維持 warning，**但把後果寫進 finding 訊息與 `INSTALL.md`** | 不回溯破壞任何東西，而缺的資訊補上了 |

**取 (c)，並且把 (a) 的解鎖條件寫死成一次量測**：對 45 個 pin commit 套件跑一次驗證，數出帶未知欄位的比例與欄位名分布。**那是一個查得出來的事實，不是一個要拍板的偏好**——所以它是工作項不是待決策（`04` 丙類）。量完之後若殺傷範圍為零，升級為 error 不需要再開一次 ADR。

**不接受的做法**：對「本平台的目標」放寬、對「別人的目標」收緊。同一個套件在三個 profile 下是同一份位元組，一份位元組不能有兩個合規判準。

## 決策 5：`INSTALL.md` 說的是「檢查了什麼、對照哪一版」，不是「符合規格」

假宣稱那一句刪掉，換成一節逐項列出**實際檢查的六件事**、點名規格 revision、並明說**沒有檢查也沒有主張**的三件事（你的 Agent 會不會載入、腳本會不會跑、它做不做得到它說的事）。最後一段告訴讀者：未知欄位在這裡是警告，在規格的參考驗證器與部分 client 的上傳路徑是硬錯，而如果這一份帶了一個，它在 manifest 的 `validation.warnings` 裡。

**判準是「讀者能不能自己重查」。** 一個讀者無法重查的主張，寫得再長也是同一個缺陷。

## 決策 6：打包器移除的檔案要列出來；它弄壞的引用要擋下來

匯出器一直會丟掉 `.git/`、`node_modules/`、`.ssh/`、`.env`、symlink 與不安全路徑——**排除本身是對的，錯的是它是無聲的**。manifest 只記 `excluded_test_cases`，原始碼檔案一個字都沒有，所以**一個掉了 vendored 依賴樹的套件，和一個從來就沒有的套件，長得一模一樣**。`excluded_test_cases` 一個欄位之外早就把這個論證做完了。

補：`excluded_files: [{path, reason, label, note}]`，四個原因（`excluded_dir`／`credential_file`／`not_a_regular_file`／`unsafe_path`），**進 manifest 也進打包預覽**——manifest 在使用者還沒決定要不要下載的那個東西**裡面**，只在那裡回答等於沒回答。措辭由伺服器給（設計系統 §4.4），且每一則 note 寫的是「怎麼修」而不是重述規則，因為讀者是那個少了一個檔案的作者。

**同時新增第五個 `blocked_reason`：`file_removed_by_packager`。** 判準是**誰造成的**：

- `SKILL.md` 指向的檔案**匯入時就不在** → 這是作者的套件，`file-ref-missing` 警告、照常出貨。`02:SKILL-002` 要的是兩種嚴重度分開呈現，不是要這一條擋人。
- `SKILL.md` 指向的檔案**在版本裡而被匯出器拿掉** → **這是平台弄壞的，拒絕出貨**。一個平台自己弄壞的套件不該長得像完好的。

這也是這張清單上唯一一個使用者一分鐘內修得好的（把檔案移出 `.git/`、別 vendor `node_modules/`、用實體檔案取代連結）。

**檔案引用的偵測同批放寬**：原本只認 markdown link（`[text]` 後面接括號路徑的那種寫法），而規格自己的檔案引用範例就是裸路徑（`scripts/extract.py`）。**只認 link 的檢查會驗證「寫成文件的 Skill」而放過「寫成指令的 Skill」——而後者才是有腳本的那種。** 防誤報的錨點不在正規表達式裡，在解析器裡：**第一段路徑必須是套件裡真的存在的目錄**。

## 影響

### 正面

- `INSTALL.md` 的合規宣稱第一次有依據，而且讀者查得動。
- 一個合法宣告 `compatibility` 的 Skill 不再被報成有未知欄位。
- 打包器不再無聲地把套件弄殘。
- 未知欄位的後果第一次寫在使用者看得到的地方。

### 成本與限制

- **規格沒有版本，所以釘選是脆的。** 上游改一行，我們的 blob SHA 就過期，而**沒有任何東西會自動告訴我們**。漂移偵測（CI 比對 raw URL 的 hash）是待補的工作項，不是本 ADR 交付的東西。
- **`skills-ref` 是 Python，本平台的驗證器是 Go。** 兩份實作，一份規格，沒有共用的 schema 可以對齊——這是規格本身的限制，不是本決策引入的，但它意味著**對齊只能靠人讀，並且會漂移**。
- **本 ADR 不解決「任何 agentic runtime 都能跑」**。它把「符合規格」變成一個真的、可查的、範圍明確的主張。能不能跑是 ADR-012 的另外兩層，而那兩層的答案對絕大多數版本仍然是「未量測」。

## 待決策

- **`name` 是否放寬到 Unicode**（決策 2）。參考實作允許，本平台不允許，而 Anthropic 自己的 `quick_validate.py` 也是 ASCII——**生態系本身不一致**。放寬不會弄壞既有套件，但會讓「同一個 Skill 在哪裡合法」多一種答案。
- **`name` 必須等於父目錄名**（規格 MUST，參考實作是 error）。本平台的 `PackageRoot` 會把單一頂層目錄剝掉，所以在驗證的當下**那個名字已經不在了**。要查它得把 root 名帶進 `Validate`，那是簽章改動。
- **未知欄位升級為 error 的殺傷範圍**（決策 4）——一次量測，已列入 `04` 丙類。
- **規格漂移偵測**要不要進 CI，以及失敗時是 fail 還是提醒。上游是別人的 repo，把我們的 CI 綁在它的 main 上是一個要想清楚的耦合。
