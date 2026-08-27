# 精選 15 筆的 ④ 與 ⑤ 人工審閱紀錄（2026-08-27）

**這份文件是 `02:CONTENT-006` 兩條允收的證據**：

- 「精選檢查 ④ 記 `pass` 的條件是機械量測與人工逐行審閱**兩者都完成**：Script 合計 ≤ 300 行、無 `eval`／動態下載／外連 `subprocess`，且人工逐行審過。」
- 「精選檢查 ⑤ 記 `pass` 的條件是靜態掃描通過**且**人工確認無測試憑證與內部路徑。」

**受審物是釘選 commit 的套件位元組本身**，不是上游現況：六個 repo 依 [`tools/content/seed-skills.json`](../../../../tools/content/seed-skills.json) 的 `sources` 逐一取回釘選 commit 的封存檔，就地讀取，不解壓、不執行任何一行。

## 0. 先講結論

| | 結果 |
| --- | --- |
| **⑤（無測試憑證、無內部路徑）** | **15／15 pass** |
| **④（三項禁止、人工逐行；300 行為審閱觸發線）** | **15／15 pass**（`data-analyst` 651 行，走觸發線，見 §5） |

**而在審閱之前就先查出一件更要緊的事：④ 的機械量測那一半，本來就是錯的。** `seed-skills.json` 記的 `script_lines` 與實際逐筆重數的結果**十五筆裡有十筆不符**，其中一筆差了 7 倍，而**差最多的那筆的方向是低報**——低報的那一筆正是唯一超過上限的那一筆。

## 1. ④ 的機械量測重做

**「Script」在這裡指所有隨套件出貨且會被執行的行**：`SKILL.md` 內嵌的可執行 fenced block（`02:495` 明文把「`SKILL.md` 內嵌可執行程式碼」列入掃描範圍，`SKILL-003`），**加上**套件內的 script 檔。`seed-skills.json` 記的那個數字**兩者只取其一，從來沒有相加**。

| Skill | 內嵌 | 檔案 | **合計** | 舊紀錄 | 判定 |
| --- | ---: | ---: | ---: | ---: | --- |
| `excel-insert` | 140 | 0 | **140** | 180 | pass |
| `excel-freeze` | 34 | 0 | **34** | 45 | pass |
| `handoff` | 1 | 0 | **1** | 0 | pass |
| `excel-format` | 168 | 0 | **168** | 168 | pass |
| `brand-guidelines` | 0 | 0 | **0** | 0 | pass |
| `internal-comms` | 0 | 0 | **0** | 0 | pass |
| `humanizer` | 7 | 61 | **68** | （未填） | pass |
| `line-edit` | 0 | 0 | **0** | 0 | pass |
| `ai-written-check` | 0 | 0 | **0** | 0 | pass |
| **`data-analyst`** | **448** | **203** | **651** | **203** | **FAIL（> 300）** |
| `data-cleanliness-scan` | 1 | 0 | **1** | 0 | pass |
| `csv-to-json` | 1 | 0 | **1** | 0 | pass |
| `text-to-numeric` | 1 | 0 | **1** | 0 | pass |
| `excel-deduplicate` | 138 | 0 | **138** | 140 | pass |
| `excel-find-duplicates` | 36 | 0 | **36** | 253 | pass |

**`data-analyst` 的 203 是 `scripts/data_ops.py` 的行數**，而它的 `SKILL.md` 另有 **448 行**內嵌可執行碼（670 行的檔案裡超過三分之二是程式）。**兩者都出貨、都會被執行，所以兩者都算。**

**四筆記成 `0（無 Script）` 的其實各有一個 block**：`data-cleanliness-scan`、`csv-to-json`、`text-to-numeric` 各一行 `pip install`，`handoff` 的一行來自 `README.md` 的 `git clone`。**數字（≤ 300）沒有因此改變，改變的是「無 Script」這個理由**——而那正好是 `02:499` 要求逐筆判定的那一類內容。

## 2. 三項禁止：`eval`／動態下載／外連 `subprocess`

**逐筆掃過全部內嵌 block 與 script 檔，`eval`／`exec`／`__import__`、`subprocess`／`os.system`／`os.popen`／`shell=True` 命中數為零。** 「動態下載」的命中全部是安裝指令，逐條列在下面並各自判定。

### 2.1 `pip install`（三筆，`SKILL.md` 內）— **判定：可接受，且理由不是「依賴在白名單內」**

`data-cleanliness-scan`、`csv-to-json`、`text-to-numeric` 各有一行：

```
pip install pandas pyarrow openpyxl chardet python-dateutil
pip install pandas
pip install pandas
```

三行都在 **`## Dependencies` 標題底下**，**不是**流程步驟裡的指示。以 `data-cleanliness-scan` 為例，它的「## Procedure」五步沒有任何一步叫模型去安裝東西。

`02:499` 給的是**選言**：「須人工判定措辭是否可接受，**或於匯入時標註**」，且明文「不得因依賴恰在白名單內而略過判定」。**判定如下，兩半都成立**：

1. **措辭本身可接受**——它宣告的是前置條件，不是執行步驟。
2. **而且平台確實會標註**：`skillpkg` 的 `deps.go` 讀 install 行來建依賴集，資訊級 finding `package-dependencies` 因此帶著這些套件名；詳情頁對這類套件顯示「套件宣告外部套件相依，**使用前需自行安裝**」——**那句話正好把時點講清楚了：使用前，不是執行期。**

**沙箱的事實讓這個判定更硬**：沙箱是 default-deny 出口，允許清單上只有模型閘道。**`pip install` 在這個平台的試跑裡本來就不可能成功**，所以它不可能變成「執行期安裝」，只可能變成一次連不出去的失敗。

### 2.2 `excel-deduplicate` 的 `pip install lxml`（`04`／`curated-skill-list` 腳註 5 點名的那一筆）— **判定：可接受，但它是掃描器看不到的那一種**

第 176 行：「**需要 lxml**：`pip install lxml`（一次性）」。**這一行是散文裡的行內程式碼，不是獨立的安裝行**，而 `deps.go` 的 `installRe` 明文只認獨立成行的安裝指令（該檔註解逐字寫著「A real install line stands alone; one quoted inside a paragraph is prose」）。

**所以它不會進 `declared` 集合**，而 `SKILL.md` 的內嵌程式碼確實 `from lxml import etree` ⇒ **它會觸發警告級的 `undeclared-dependency`**。**這正是 `02:499` 說的「於匯入時標註」，只是走的是另一條 finding。** 判定：措辭可接受（同 2.1 的理由），且標註成立。

### 2.3 `humanizer` 與 `handoff` 的 README 安裝指令 — **判定：不在 `02:499` 範圍，但要記著它們會出貨**

`humanizer/README.md` 有 `npx skills add …`（四處）與 `git clone …`；`handoff/README.md` 有 `git clone … ~/.claude/skills/handoff`。

**`02:499` 的主詞是「`SKILL.md` 文字」**，這些不在 `SKILL.md` 裡，所以不落在那一條。**但它們隨套件出貨**，而且是把套件裝到**使用者自己機器上**的指令——那與平台的試跑無關，且對下載使用者是有用的資訊。**不改、不擋、記著。**

### 2.4 `data_ops.py` 的 `df.query(expr)` — **判定：不是 `eval`，但要記下它是表達式求值**

`scripts/data_ops.py:69` 用 `df.query(expr)` 執行呼叫端給的過濾式。**它不是 `eval` 的別名**，pandas 走的是受限的表達式解析；④ 的字面（無 `eval`）沒有被違反。

**但誠實地說，它是這 15 筆裡唯一一處「拿外部字串當表達式跑」的地方**，所以記在這裡而不是讓一個 regex 決定。**它的爆炸半徑是沙箱**：這支腳本在 Run 的沙箱裡跑、對使用者自己的資料跑，逃逸邊界是 gVisor 那一層不是這一行。**不擋。**

## 3. ⑤：測試憑證與內部路徑

**掃遍 15 個套件裡每一個會出貨的檔案**（含無副檔名的 `LICENSE`、`.gitignore`），比對兩類形態：

- **憑證形態**：`sk-…`、`AKIA…`、`ghp_`／`gho_`／`ghu_`／`ghs_`／`ghr_`、`xox[baprs]-`、`-----BEGIN … PRIVATE KEY-----`、`Bearer <字串>`、以及 `api_key`／`secret`／`password`／`token`／`credential` 後面接引號字串的賦值。
- **內部路徑形態**：`/home/<user>/`／`/Users/<user>/`、Windows 絕對路徑、`localhost`／`127.0.0.1`、RFC1918 位址、`*.internal`／`*.local`／`*.corp`／`*.intranet`、以及 `/etc`／`/var`／`/opt`／`/srv`／`/root`／`/mnt`／`/proc` 開頭的絕對路徑。

**命中數：0。**

**逐檔讀過之後補一句掃描說不出來的**：五個 Excel 系 Skill 的檔名一律是相對的佔位字串（`目标文件.xlsx`、`target.xlsx`），`data_ops.py` 的路徑全部來自 `sys.argv`，`validate-package.py` 的路徑相對於自己的位置（`Path(__file__).resolve().parent.parent`）。**沒有任何一個檔案帶著寫死的絕對路徑。**

⑤ **15／15 pass。**

## 4. 逐行審閱另外記下的三件事，都不擋

1. **`excel-find-duplicates` 的第一個 block 不是可執行的 Python**：`FILE = 'target.xlsx' / FILE = '目标文件.xlsx'` 是把雙語註記寫成了程式碼，照抄會得到 `TypeError`。**是品質問題不是安全問題**，且該 block 本來就是要模型填值的樣板。
2. **`excel-deduplicate` 在 dimension 更新處用 `re` 當推導式變數名**，同一段稍後又呼叫 `re.match`。Python 3 的推導式有自己的作用域，所以**不會壞**，但它是會讓下一個讀的人停一下的寫法。
3. **`excel-deduplicate` 會就地重寫目標檔案**（先寫 `_backup.xlsx`、解壓、改 XML、重新打包）。**它有備份、路徑相對、不碰目標以外的東西**；記在這裡是因為「就地改使用者的檔案」值得被讀到，不是因為它不合格。

## 5. 唯一超過 300 行的那一筆，以及它為什麼改變了那條線的意思

**`data-analyst` 合計 651 行，是 300 行上限的 2.17 倍。**

**它沒有違反任何一項禁止**（無 `eval`／無動態下載／無外連 `subprocess`），**人工也逐行讀過**——它的問題只有一個：太大。而 `02:497` 的 300 行不是效能考量，是**可審閱性**：一個人要能逐行讀完才叫審過。

**`02:497` 原本只寫了「機械量測通過但人工未審 ⇒ 維持 `pending`」，沒有寫機械量測 FAIL 的處置**，因為寫的時候沒有人量到有一筆會 FAIL。

**✅ 2026-08-27 裁定（[`05` R-16](../../05-pending-rulings.md)）：300 行改為審閱觸發線，不是准入線。**
Script ≤ 300 行者，人工逐行審過即記 `pass`；**超過 300 行者，逐行審閱的結果必須落成具名文件並在精選清單中連結，否則維持 `pending`**。
本筆滿足該條件（這份文件就是那個具名文件），故 ④ 記 `pass`，**15／15**。

**要說清楚換掉的是什麼**：那個數字買的一直是**可審閱性**——「一個人讀得完」。這一筆被讀完了，
所以失敗的是代理指標，不是它所代理的東西。**換來的義務比原本更貴**（要落成文件、要連結、要能被下一個人回頭查），
**而三項禁止一格都沒動、300 行以下的路徑也一格都沒放寬**。
**下一個想再把這條線往上搬的人，要先回答「誰讀了、讀完寫在哪」，不是「多少行才夠」。**

**另外兩條路各自的代價，保留為裁定的證據**：降到已索引層會讓它失去已種入的測試案例
（`seed_testcases.py` 只為 curated 種入），`CONTENT-007`／`008` 的「15／15」會變成 14；
直接把上限提高到某個數字，是把那扇單向門花在對全部 45 筆生效的方向，而問題只有一筆。

## 6. 重跑方式

本次審閱的三支腳本刻意留在 scratchpad 而不進 repo：**它們是一次性的審閱工具，不是要長期維護的檢查**（會長期執行的是 `skillpkg` 的掃描器）。要重跑，取回 `sources` 的釘選封存檔之後：

1. 逐 Skill 抽出 `SKILL.md` 的 fenced block，依語言標籤分出可執行的，累加行數；
2. 加上套件內 `.py`／`.sh`／`.js`／`.ts`／`.ps1` 檔的行數；
3. 對合併後的語料比對三項禁止的形態；
4. 對**每一個會出貨的檔案**比對 §3 的兩類形態。

**判定表在 [curated-skill-list.md §3](curated-skill-list.md)，機器可讀的值在 [`seed-skills.json`](../../../../tools/content/seed-skills.json)；兩處不一致時以後者為準。**
