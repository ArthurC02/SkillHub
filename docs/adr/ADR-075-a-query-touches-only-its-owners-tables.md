# ADR-075：一條 query 只碰它擁有者的表——Bounded Context 稽核與收斂

- 狀態：**Accepted**（2026-09-12，[`05` R-77](../plans/05-pending-rulings.md) 負責人指示「先穩定 BC 再去思考和設計 BackOffice」）
- 日期：2026-09-12
- 相關：[ADR-032](./ADR-032-ddd-bounded-context-governance-for-platform.md)（context 對照表與協作方向）、[ADR-033](./ADR-033-sqlc-query-ownership-and-cross-context-write-enforcement.md)／[ADR-035](./ADR-035-read-ownership-enforcement-and-context-map-completeness.md)（query 擁有權、跨 context 取事實）、[ADR-074](./ADR-074-the-backoffice-is-an-operator-only-section-of-the-same-app.md) 決策 7、[platform-ddd-practices](../development/platform-ddd-practices.md)

## 背景

`db/query-owners.yaml` 規定每條 query 屬於哪個 context，`devctl` 會擋下別的 context 呼叫它。但檢查只看**誰呼叫**，不看 **SQL 碰了哪些表**。一條 registry 擁有的 query 可以在自己的 SQL 裡 JOIN `runs`、`test_cases`、`workspaces`，檢查照樣通過。

2026-09-12 以工作流對全平台做 DDD 稽核，依表的擁有者逐條比對 SQL，找到 **31 條 query、55 處讀寫別人的表**：catalog 11 條、packaging 7 條、registry 5 條、run 4 條、testlab 2 條、eval 與 ingest 各 1 條，依修法分成六批。它們都是在大量開發中為了一次拿齊資料而順手 JOIN 的，沒有一條經過 ADR。

## 決策

### 1. 表也有擁有者，query 只碰擁有者的表

`db/query-owners.yaml` 新增 `tables:`，每張表登記一個擁有者；只有 `artifacts` 由 run 與 packaging 共有（兩種產物共用一張表，各自擁有自己的列）。

`devctl` 的 query-owners 檢查從此多一條：一條 query 的 SQL 碰到的每張表，擁有者都必須是這條 query 的擁有者。解析方式是對正規化後的 SQL（去掉註解與字串）取 `FROM`／`JOIN`／`INTO`／`UPDATE`／`USING` 後的表名，以及 `FROM`／`USING` 後以逗號串接的表，排除 CTE 名稱；migration 裡 `PARTITION OF` 建出的子表不算一張獨立的表。新增一張表卻沒登記擁有者，檢查也會紅。`allow:`／`read_allow:` 仍然是空的，這條規則沒有例外清單。

### 2. 跨 context 拿事實的三種形狀

改掉的 31 條，最後都落在下面三種之一。選哪一種看的是「要什麼、在哪裡用」，不是看哪一種最省事。

| 形狀 | 什麼時候用 | 例子 |
| --- | --- | --- |
| **擁有者的批次讀取**，由組裝層注入：`func(ctx, db gen.DBTX, ids []UUID)` | 要一批列各自的事實，或要在同一個交易裡問「這些 id 你還在用嗎」 | Run 歷史的 Skill 名稱（`VersionSummaries`）、下載紀錄的顯示名稱（`DisplayNames`）；清除前問 run／packaging／testlab 是否仍引用 |
| **範圍當參數**：擁有者回答「哪些是」，查詢者把範圍留在自己的 SQL | 事實是一個集合，而且會被應用程式以外的方式改動 | 目錄 workspace（`CatalogWorkspaceIDs`）：registry 與 catalog 的查詢都以 `workspace_id = ANY(...)` 自己把範圍 |
| **讀取模型**：擁有者寫入時，把事實投影到查詢者自己的表 | 事實要拿來過濾、排序又要分頁，逐列去問會 N+1 | catalog 的 `search_documents` 帶上 registry 的列表事實（見決策 6） |

批次讀取吃 `gen.DBTX` 而不是自己開連線，理由是清除流程：帳號清除在一個交易裡依序跑各 context 的步驟，後面的步驟必須看得到前面步驟還沒提交的刪除。用連線池去問，會看到舊資料而少清。

「範圍當參數」不複製旗標：`workspaces.is_catalog` 只由營運 SQL 與 seed 工具改動，應用程式從不寫它。把它複製到 `search_documents`，第一次有人在資料庫裡翻轉它，公開目錄就會悄悄漏出私有 Skill，或整個消失。

### 3. 先決定、再刪，靠外鍵兜底

清除類的 query 原本在一條 SQL 裡同時決定「還有沒有人用」與執行刪除。拆開之後，決定是好幾次讀取，刪除是一條語句，中間有空檔。這個空檔是安全的：

- `runs.skill_version_id`、`download_artifacts.skill_version_id`、`test_cases.skill_id`、`skill_versions.source_id`、兩個 `forked_from_*` 都是 `NO ACTION` 外鍵。空檔裡新提交的引用會讓刪除失敗、整個清除回滾，不會刪掉仍被引用的東西。
- 刪除語句仍自己再檢查**擁有者自己的**條件（fork、寬限期）。`READ COMMITTED` 下每條語句拿新的快照，不能相信上一條語句讀到的清單。
- 有上限的掃除，上限要在過濾之後才套用。先套上限，會讓一直被引用的舊資料佔滿每一格，掃除從此停住。

### 4. 缺注入就拒絕

每個新注入的讀取都有一道守衛：沒接上就回錯誤，不以「當作沒有引用」或「當作空範圍」繼續。每個組裝根（apiserver、worker、`cmd/maintenance`、`cmd/reindex`）都要接，並各有一條測試在沒接時變紅。

### 5. 這次刻意不搬的

- `ingest.redistributionFor` 留在 ingest：它決定「要寫哪個再散布分類」，用的是 ingest 自己的來源類型與 identity 的 `IsCatalog`；registry 擁有的是寫入閘門與詞彙。搬過去反而要 registry 認得 ingest 的來源詞彙。
- `skillpkg.Report.Blocked` 留在 Shared Kernel：它只是對格式與資安基準線的邏輯 OR，要不要據此拒絕，仍由讀它的 context 決定。
- Trace 的 `assign_trace_ingest_seq` trigger（migration 0034）讀 `runs` 設定 `late`：這是資料庫層的耦合，不在 query 規則的範圍內。本次只移除 Go 端算了又被 trigger 覆寫的那個值，trigger 留著，列為已知例外。

### 6. 決定「誰看得到」的事實即時讀，只描述一列的事實才投影

catalog 的列表要依分類、策展層級、Agent 相容軸與版本時間過濾、排序又要分頁，這些事實全在 registry 的表上。它們分兩種，處理方式不同：

- **決定一列能不能被看到的事實，永遠向擁有者即時讀。**
  - 目錄 workspace 走決策 2 的「範圍當參數」。
  - Skill 是否還活著（`deleted_at`／`takedown_at`）改在寫入時守住：`catalog.IndexSkill`／`IndexSkillEnriched` 寫入前，在同一個交易裡用 registry 的 `GetLiveSkillListingFacts` 鎖住 skill 列（`FOR NO KEY UPDATE`）並確認它還活著，不活就不寫。
  - 這關掉了既有的復活競態：增強回填先認領、再在交易外呼叫模型，回來時 Skill 可能已被刪除，舊的 upsert 會把文件重新插回去。
  - 競態關掉之後，「有 `search_documents` 列 ⇔ Skill 還活著」成立，`ListPendingEnrichment`、`ResetCatalogueEnrichmentBefore` 不再需要 join `skills`。
- **只描述一列的事實，投影到 `search_documents`**（migration `0065`）：最新版本（id、時間、套件位置）、分類、策展版本 id、Agent 相容軸，以及一個不會變的 `generated` 布林。
  - 應用程式內的寫入者在同一交易裡刷新投影：建版本與 Fork 走 `IndexSkill`；擁有者改分類的 `SetCategory` 改成交易，並呼叫注入的 `RefreshListing`。
  - 刷新一次重寫整列，但先鎖住 skill 列，兩個同時刷新同一個 Skill 的交易因此排隊，不會互相蓋掉。
  - `generated` 只在建立時決定：`redistribution = generated` 不能被營運覆寫，營運也不能把別的值改成它。所以它不需要跟著 `SetRedistribution` 刷新，也不必複製整個 `redistribution`。

**操作員手跑的 SQL 改了描述性事實，要接著跑 `go run ./cmd/reindex`。** `tools/content` 的 `backfill-category.sql`、`backfill-curation-tier.sql`、`backfill-agent-compatibility.sql` 直接寫 `skills` 與 `skill_runtime_compatibility`，應用程式看不到。

- 過期的後果只是列表顯示舊值，不會讓不該看的人看到東西，這正是它與 `is_catalog` 分開處理的理由。
- `catalog.RebuildIndex` 依序：以 registry 的存活清單批次補回文件、刪掉 registry 說已不存活的文件、逐一刷新列表事實。
- 刪除用「registry 確認已不存活的 id」而不是「不在存活清單裡的 id」，兩次讀取之間新建的 Skill 因此不會被誤刪。

### 7. Bounded Context 的劃分不變，後台仍是組裝層

31 條都是「SQL 越界」，不是「context 切錯」：每一條的事實都有明確的擁有者，修法是讓查詢回到擁有者，而不是移動 context 的邊界。所以 ADR-032 §1 不改、不新增套件。

這也回答了 R-77 第五題：稽核之後，後台上的每一項（點數、Skill 治理、派送煞車、帳號與名冊、動作紀錄）仍各有擁有者，後台依 ADR-074 決策 7 維持為組裝層。

## 影響

- 程式：分七個提交（b2fbdf9、03940aa、e450ab7、87cc8fa、61eca1c、f551cd6、d2b0ebb），每個附突變證明；表的擁有者檢查與本 ADR 同一個提交。稽核前的基準（在 b2fbdf9 之前的樹上套用同一個檢查器）是 31 條 query、55 處。
- 容量上限（接受，寫在這裡而不是程式碼）：
  - 刪除掃除一次讀出所有過了寬限期的候選（原本的計數 query 本來就掃全部）。
  - Run 歷史以 snapshot id 陣列過濾，陣列長度等於一個 Test Case 的快照數。
  - 目錄查詢每次請求多一次 identity 讀取，取回的是目錄 workspace 清單（目前一個）。
- 文件：`02`、`03`、`04` 裡點名已移除 query 的列，改指取代它們的 query。

## 考慮過的替代方案

- **把越界的 query 列進 `read_allow:`**：最省事，但 ADR-033 已經說它是存量漂移清單、不是擴充點；31 條全列進去等於取消規則。
- **跨 context 的資料庫 view 或 trigger**：可以讓 SQL 看起來只碰一張表，但耦合只是換到檢查看不到的地方。
- **registry 提供不帶 workspace 範圍的「以 id 取 Skill」**：比傳入目錄清單少一個參數，但會讓 registry 自己的 SQL 失去 workspace 範圍（鐵律 3）。
