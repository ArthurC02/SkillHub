# ADR-035：Read Ownership 開始強制，Context 對照表補上完整性檢查

- 狀態：Accepted
- 日期：2026-08-21
- 決策者：架構規劃
- 回答：[ADR-033](./ADR-033-sqlc-query-ownership-and-cross-context-write-enforcement.md) 待決策第 1 項（read ownership 何時開始強制）、[ADR-034](./ADR-034-cross-context-writes-close-by-inversion-not-by-events.md) 待決策第 2 項
- 補充：[ADR-032](./ADR-032-ddd-bounded-context-governance-for-platform.md) §1（context 對照表）、§2（關係只有四種）、§3（depguard 強制）

## 背景

ADR-033 為 `db/queries/*.sql` 的全部 query 宣告了 owner context，但**只強制 write**。當時的理由寫得很明白：「同一批把跨 context read 一起擋，會製造十幾條容忍條目，讓宣告本身失去訊號」。那是針對 write 存量還在（15 條 `allow:`）的當下說的——訊號會被兩種雜訊同時稀釋。

ADR-034 清完最後 6 條後，`allow:` 是 0 條。稀釋來源消失了，read 的存量因此可以有自己乾淨的一段。ADR-034 的待決策也明說了這件事：write 側存量清完後，read 是否強制「可以在乾淨的基礎上重新評估」。

另一個洞在旁邊：AGENTS.md 第 11 條寫著「新增套件必須先在 ADR-032 §1 對照表登記」，**但沒有任何東西強制它**。漏登記的套件會安靜地活在 `internal/` 底下——既不屬於任何 context，也不被 depguard 的任何一條規則涵蓋。現況是一致的（19 個目錄對得上 §1 的 17 列加 `platform/*`、`api/gen`），所以這是補檢查最便宜的時點，不必先清存量。

## 決策

### 1. read ownership 開始強制，棘輪形狀與 write 完全相同

ADR-032 §2 規定跨 context 取事實的合法機制是**import 對方套件的公開 Service API**。直接呼叫對方的 query 不在那四種關係裡的任何一種——它是 write 漂移的 read 版，只是後果不同：讀不會破壞別人的不變量，但會凍結別人的 schema——owner 一改欄位，不知情的呼叫端就壞了，而且沒有任何地方記載誰在讀。

因此：

- `db/query-owners.yaml` 新增 `read_allow:` 段落，形狀比照 `allow:`／`immutable_allow:`／`raw_sql_allow:`——**具名、有理由、失效即 FAIL**。key 是 query 名，value 是被容忍的呼叫端 context 清單，理由按成因分組寫在各組上方的註解（與清空前的 `allow:` 同一種寫法）。
- 新的跨 context read 一律 FAIL。已無呼叫點的條目一樣 FAIL。
- **條目放錯段落也 FAIL**，訊息直接指出該搬去哪一段（`read_allow.X is a write query; declare it in allow:`，反之亦然），而且**不會給予豁免**——放錯段落的條目不擋任何東西。

### 2. 判定式是唯一的差別，解析與掃描完全重用

read 與 write 走同一個迴圈，只有 `queries[name].write` 的比對值不同。`sqlQuery` 解析、`isWriteStatement`／`normalizeSQL`、以及「掃 import 了 `db/gen` 的非測試 Go 檔、由路徑推呼叫端 context」的呼叫點掃描全部不動。沒有第二套 parser，也沒有第二個掃描器。

**Generic 套件沿用 write 側的既有規則**（ADR-033 決策 5）：`audit`、`outbox`、`objreconcile`、`llmclient`、`skillpkg`、`platform/*`、`apiserver`、`api/gen` 視同 context 參與判定——讀自己領域的 query 合法，讀別人的一樣要進 `read_allow:`。不為 read 另立一套語意。

### 3. Context 對照表完整性：三份清單互相對帳

`automation-check` 新增 `contextMapProblems`，讓三者互相對得上：

| 清單 | 事實來源 |
| --- | --- |
| `apps/platform/internal/` 底下的套件目錄 | 檔案系統 |
| ADR-032 §1 對照表「internal/ 套件」欄 | ADR-032（context 清單的事實來源） |
| `apps/platform/.golangci.yml` 的 depguard 規則 | ADR-032 §3 的機器版 |

任一方向缺漏都 FAIL，訊息指出是哪個套件、缺在哪一側：

- 目錄存在但 §1 沒登記 → `apps/platform/internal/X is not listed in ADR-032 … §1; register it before adding the package (AGENTS.md 第 11 條)`
- §1 有登記但目錄不存在 → `§1 lists "X" but apps/platform/internal/X does not exist`
- §1 的**非 Generic** 列沒有 depguard 規則 → `§1 puts "X" in a bounded context but apps/platform/.golangci.yml has no depguard rule covering it`
- depguard 規則指著不存在的套件 → `… guards apps/platform/internal/X but that package does not exist`

**depguard 覆蓋只對非 Generic 的列強制。** §1 的 Generic 列裡，`apiserver` 是 composition root（它按設計 import 每一個 context，一條 deny 規則什麼都不能寫）、`api/gen` 是生成碼；兩者刻意沒有規則，強制它們有規則只會逼出一條空規則。其餘 Generic 套件（`audit`／`outbox`／`llmclient`／`skillpkg`／`platform`）實際上被 `.golangci.yml` 的 `generic` 那條規則涵蓋——檢查不反對，它只要求「領域 context 一定要有人管」，不禁止 Generic 也被管。

**§1 表格的解析要處理得了實際寫法，處理不了就報錯**：儲存格會夾全形括號的註解，而註解裡也有反引號（`` `ingest`（匯入管線；`SaveVersion` 是版本寫入的唯一驗證路徑） `` 裡的 `SaveVersion` 不是套件），所以先整段移除 `（…）` 再抽反引號 token；`platform/*` 與 `api/gen` 取第一段當目錄名；不符 `^[a-z][a-z0-9_]*(/(\*|[a-z][a-z0-9_]*))?$` 的 token **報錯而不是靜默跳過**——安靜跳過的解析器等於沒有這道檢查。表格範圍以 `### 1. Context 對照表` 到下一個 `### ` 標題為界，因為 ADR-032 有四張表。

`skillpkg` 同時出現在 §1 的 Skill Registry 列（Core）與 §2 的 Shared Kernel／附錄 A 的 Generic 語意裡。這道檢查對此不表態：**同一個套件出現在多列時，只要有一列非 Generic 就照非 Generic 要求**，於是 `skillpkg` 需要覆蓋，而它已被 `generic` 規則覆蓋，兩種讀法都通過。要不要把 §1 講一致是另一個決定，不由這道檢查代答。

### 4. 不新增 Taskfile task

兩道檢查都併入既有的 `automation-check`（CI 已在跑）。理由同 ADR-033：多一個入口只會多一個可以忘記跑的東西。`tools/devctl` 維持**零第三方依賴**。

## 已知盲點

- **裸 SQL 的 read 看不到。** `apps/platform/internal/eval/reconcile.go` 直接 `Pool.Query` 讀 `evaluations`（自己 context 的表，所以不是違規），但它繞過宣告——這條讀取不存在於 `db/query-owners.yaml`，本檢查看不到它。ADR-033「追加：裸 SQL 的盲點與 tripwire」已把它記為 read 盲點，並預告「read ownership 真的開始強制時得先變成一條 query」。**那一天就是今天，而這一條沒有跟著收**：它今天恰好讀自家表，所以強制與否不影響結果；但換成別人的表，`raw_sql_allow:` 的 tripwire 也不會響（那道 tripwire 只看 DML）。要封死只能走型別（把 pool 收在只暴露 sqlc 的 wrapper 後面），成本與「拆 sqlc per-context package」同級，維持 ADR-033 的判斷：等存量清完再評估。
- **呼叫點判定仍是文字比對**（`.<QueryName>(` ＋ `db/gen` import），不是型別解析。read 側繼承 write 側的這個限制，升級路徑不變（改用 `go/ast`，裸 SQL tripwire 已先行採用）。
- **`read_allow:` 的「理由」由分組註解承載，不由機器檢查。** 與 `allow:`／`immutable_allow:` 相同（只有 `raw_sql_allow:` 的 value 就是理由，因為它的 key 是檔案路徑、沒有第二個欄位可放）。機器強制的是「具名」與「失效即 FAIL」；理由的品質靠 review。

## 存量：35 條，七組

導入時量到 47 組 (query, caller) 跨 context read、共 56 個呼叫點，收斂為 `read_allow:` 的 35 個條目（同一 query 的多個呼叫端合成一行）。下表的「條」與 yaml 的分組標題一致，指的是 (query, caller) 組數。分組與清除方向如下——**逐組的判讀寫在 `db/query-owners.yaml` 的分組註解裡，本節不重寫成另一套說法**。

| 組 | 內容 | 成因 | 清除方向 |
| --- | --- | --- | --- |
| **A** | registry 的 Skill／Version 事實（21 條、6 個呼叫端） | `skills`／`skill_versions` 是全平台的共享事實；registry 至今只公開了寫入面（DDD-019／020 的三支 `*FromPackage` 與 `SetAccessRestriction`），沒有讀取面 | registry 開一支讀取面。這一組不是判定可商榷，是缺一支 API |
| **B** | `LockSkillForRestriction`（catalog）、`LockTestCase`（run） | 寫入已回到 owner，**同一筆動作的 `SELECT … FOR UPDATE` 沒跟著搬** | 鎖併進 owner 的寫入函式。**優先收，見下** |
| **C** | testlab 的 Test Case／Dataset 快照（6 條） | 與 A 同因，但成本低得多：附錄 A 已允許 `run`／`eval`／`packaging` → `testlab` 三條 import，testlab 也已有 Service 門面（DDD-007） | 直接走既有 import ＋ Service 門面。`ListWorkspaceObjectKeys`（identity 的帳號刪除盤點待刪物件鍵）方向與 ADR-034 的 `PurgeWorkspace` 一致，併進那支注入 |
| **D** | run 的 Run 事實（7 條） | 與 A、C 同因，但 import 方向被 depguard 擋著 | **注入，不是 import，見下** |
| **E** | objreconcile 代讀 owner 的清單（3 條） | DDD-020 反轉了掃描器的**寫入**半邊，**讀取半邊留著**——掃描器仍自己決定「哪些列該被掃」 | 注入形狀已經在 `cmd/worker` 的 `buildWorkers` 裡，補幾個 lister 欄位即可 |
| **F** | 單點跨界（6 條） | 各有各的成因，見 yaml 註解 | `ListPendingEnrichment` 是 ADR-034 那批的自然殘留（寫入反轉了、讀「還沒 enrich 的投影列」沒跟著走），補一個 lister 欄位；`ListSuggestionsAppliedToVersion`／`GetLineageSource` 走注入（`packaging → eval`／`packaging → ingest` 皆為 deny）；`CountArtifactsSharingObject` 是「別人還要不要用這個物件」，只有 owner 答得出來；`CountQuotaRuns` 維持按表歸屬（見「不變事項」）；`GetWorkspaceCreatedAt` 最好收——鐵律 3 讓每個 context 都已 import `identity`，只差 identity 公開一支讀取面 |
| **G** | trace 事件的讀取（2 條） | 與 C 同型：import 方向已合法（附錄 A 有 `run → trace`、`eval → trace`），trace 也已有 Service（DDD-004） | 純粹還沒搬，成本同 C |

### B 組優先，因為它是正確性問題

`LockSkillForRestriction` 與 `LockTestCase` 都是 `SELECT … FOR UPDATE`（`normalizeSQL` 會把 `FOR UPDATE` 消掉，所以本檢查正確判為 read）：呼叫端先鎖 owner 的列，再叫 owner 寫。DDD-020／ADR-034 反轉的是**寫入那一半**，鎖這一半沒跟著搬，於是「鎖誰」這個決定仍在呼叫端手上——**owner 無法保證自己的寫入被正確序列化**。第二個呼叫端只要鎖錯行、或忘了鎖，owner 這邊不會有任何跡象。

這與其他六組不同級：A／C／D／E／F／G 是整潔與 schema 耦合問題（今天不收，明天也只是更難改）；B 是**正確性**問題，而且是已經被反轉一半、留下不完整接縫的那種——比從未動過的更危險，因為它看起來已經收過了。

### D 組的正解是注入，不是 import

`eval → run`、`trace → run`、`analytics → run`、`policy → run` 全都在 depguard 的 deny 清單裡，而 `run → policy`、`run → trace` 是附錄 A 的既有合法方向。把 D 組改成 import 會**製造編譯期循環**，Go 直接拒絕。

這與 [ADR-034](./ADR-034-cross-context-writes-close-by-inversion-not-by-events.md)「為什麼是注入而不是 import」同源：循環是訊號不是障礙，它說的是這些 context 互為對等（peer）而非上下層——`run` 需要 `trace` 寫事件、`trace` 需要 `run` 的 Run 事實，誰也不在誰底下。依賴反轉是對等關係的標準表達，且本 repo 已有三次前例（DDD-020 的 objreconcile、ADR-034 的搜尋投影與帳號刪除 purge）。

## 考慮過但拒絕

- **read 只宣告不擋，維持 ADR-033 現狀**：ADR-033 的理由（訊號稀釋）已隨 `allow:` 清空而失效。留著等於讓「暫時」變成事實上的永久，而 read 的存量正在長——A 組的 21 條就是「沒人擋所以每個 context 各接一條線」的直接結果。
- **read 一律禁止、不設容忍段落**：需要先重構 56 處呼叫點才能讓 CI 變綠，而重構 `internal/` 與本批的目的（補檢查）無關，也會與正在改那裡的其他工作衝突。棘輪的意義就是「存量凍結、增量歸零」。
- **為 read 另立一套 Generic 語意**（例如讓 Generic 套件自由讀所有 context）：E 組正是反例——Generic 掃描器自己決定「哪些列該被掃」，那是領域決定不是機制。「Generic」指的是沒有領域規則，不是可以繞過所有權（ADR-033 決策 5 的原話）。
- **把對照表完整性做成獨立的 Taskfile task 或 lint plugin**：同決策 4。

## 影響

### 正面

- 「誰在讀誰的表」第一次有了單一、可被 CI 驗證的答案。35 條從隱形變成有名字、有成因分組、有清除方向的待辦。
- 新的跨 context read 在 CI 就被擋下，A 組不會再長。
- ADR-032 §1 對照表從「讀過 ADR 的人才知道要登記」變成「漏登記 CI 會紅」，AGENTS.md 第 11 條第一次有強制力。
- 不動 `db/sqlc.yaml`、`db/queries/*.sql`、任何 generated 目錄或 `apps/platform/internal/**`，沒有 codegen drift 風險。

### 代價

- 新增跨 context read 的成本從零變成「改程式或過一次 ADR」。這個摩擦是刻意的。
- `read_allow:` 的 35 條會在清除期間反覆被編輯；每次移除都要確認呼叫點真的沒了（失效即 FAIL 幫忙守這一半）。
- 對照表檢查以正規表示式讀 markdown 表格。§1 的欄位順序或反引號慣例若變動，檢查會報「讀不出來」而不是靜默通過——這是刻意選的失敗方向，但它意味著改 §1 的格式要順手跑一次 `automation-check`。

## 不變事項

- ADR-032 §1 仍是 context 清單的事實來源；新增 context 要同時登記在 §1、`.golangci.yml` 與 `tools/devctl/query_owners.go` 的 `platformContexts`。
- ADR-033 的 owner 判準（**該 query 主要資料表的擁有 context**）不變。`CountQuotaRuns` 這類「表歸 A、用途歸 B」的張力維持按表歸屬——表是 `runs` 故歸 `run`，即使 ADR-032 §1 把 quota 規則判給 `policy`、而這條 query 除了算配額沒有第二個用途。改判會讓判準出現例外。
- ADR-034 的兩個交易保證、depguard 的跨 context import 規則、generated file 禁止手改、單一 Writer、所有實作鐵律，均不變。

## 待決策

- 存量七組清完後，是否拆 sqlc per-context package（ADR-033「考慮過但拒絕」），或把 `pgxpool.Pool` 收在只暴露 sqlc 的 wrapper 後面以封死裸 SQL 盲點。兩者成本同級，且都要等存量清完。
- ADR-032 §1 對 `skillpkg` 的兩種讀法（Core 列的成員 vs Shared Kernel／Generic）是否要統一。本 ADR 的檢查對兩種讀法都通過，故不阻塞。
