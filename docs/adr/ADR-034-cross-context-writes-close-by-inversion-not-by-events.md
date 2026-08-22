# ADR-034：剩餘跨 Context 寫入以依賴反轉收斂，不事件化

- 狀態：Accepted
- 日期：2026-08-20
- 決策者：架構規劃
- 修訂：[ADR-033](./ADR-033-sqlc-query-ownership-and-cross-context-write-enforcement.md) 的「存量漂移與清除路徑」第 1、3 條（ADR-033 的**決策本身不變**，被取代的是它為那兩組提出的補法）
- 相關：[ADR-032](./ADR-032-ddd-bounded-context-governance-for-platform.md)（context 對照表與 §4 戰術限縮）、[ADR-008](./ADR-008-asynchronous-workflows-and-domain-events.md)（Transactional Outbox）、[ADR-010](./ADR-010-mvp-deployment-and-evolution-path.md)（拆分條件）

## 背景

ADR-033 導入 query ownership 強制時，記錄了 15 條存量跨 context write。其中 6 條已由清除路徑 2 與 4 消除（DDD-019、DDD-020），做法一致：**寫入回到擁有那張表的 context，呼叫端的交易原地不動**。

剩下 9 條分兩組，而 ADR-033 為它們提的補法是**事件化**：

- **帳號刪除 purge（6 條）**：`identity` 的 purge 交易直接清掉 analytics／testlab／run／registry／ingest 的列。ADR-033 提議改為 purge 只發 `account.deletion_due`，各 context 訂閱後自清。
- **搜尋索引投影（3 條）**：`registry`／`ingest` 直接 upsert `catalog` 的 `search_documents`。ADR-033 提議把索引寫入收到 catalog 的 projection 服務後面，上游只發事件。

兩條都被標記為「需要先有產品或合規決策才能執行」——因為事件化會把兩個**目前是交易保證的東西**換成最終一致：

1. 帳號刪除從「單一交易、要嘛全清要嘛全不清」變成「陸續清完」，於是 CORE-007 的硬刪除承諾需要重新定義「清完了」的判準與負責回答的人。
2. 匯入從「成功的當下即可被搜尋」（INGEST-009 明文要求同交易）變成「匯入後有一段搜不到的窗口」，於是 M1 Explorer 的可發現性允收準則需要鬆綁。

## 決策

**剩下 9 條照清除路徑 2／4 的同一個做法收斂：寫入回到 owner，交易原地不動。不事件化。**

因為兩組的 import 方向都會造成編譯期循環（見下），owner 的寫入函式由 **composition root 注入**，而不是由呼叫端 import：

| 組 | 誰擁有 | 收斂方式 |
| --- | --- | --- |
| 搜尋索引投影 | `catalog` | `catalog` 公開收 `pgx.Tx` 的投影寫入函式；`ingest`／`registry` 由 `apiserver.NewApp` 與 `cmd/reindex` 注入後在既有交易內呼叫 |
| 帳號刪除 purge | `analytics`／`testlab`／`run`／`registry`／`ingest` 各自 | 五個 context 各公開收 `pgx.Tx` 的 purge 函式；`identity` 由 `apiserver.NewApp` 與 `cmd/maintenance` 注入後在既有 purge 交易內依序呼叫 |

**未注入即拒絕執行**，不得跳過缺的那一步。對 purge 而言這不是潔癖而是合規屬性：少清一個 context 的靜默成功，比整批失敗嚴重得多。

**2026-08-20 已執行（搜尋索引投影，3 條）**：

- **catalog 公開三支投影寫入函式**（`apps/platform/internal/catalog/projection.go`，全部收呼叫端的 `*gen.Queries`、不自開交易）：
  - `IndexSkill(ctx, q *gen.Queries, arg gen.UpsertSearchDocumentParams) error`
  - `IndexSkillEnriched(ctx, q *gen.Queries, arg gen.UpsertSearchDocumentEnrichedParams) error`
  - `RemoveSkillFromIndex(ctx, q *gen.Queries, skillID pgtype.UUID) error`

  收 `*gen.Queries` 而非 `pgx.Tx`，因為兩個呼叫端手上已經是 `gen.New(tx)` 的 `q`（`registry` 的三支方法、`ingest.persistVersion`），而 `ReindexPending` 手上是 `gen.New(s.Pool)`——同一支函式要同時服務交易內與 pool 上的寫入，形狀與 DDD-020 的 `MarkFunc` 一致。參數維持生成的 params struct 而不另造一份同名 13 欄位的 catalog 型別：enrichment 管線與 package reader 同在 `ingest`，值全由呼叫端算出，catalog 這邊沒有任何轉換規則可寫，複製一份只會多一個要同步的地方。

- **注入形狀**：`registry.Service` 取得 `IndexSkill`／`RemoveFromIndex` 兩個 function 欄位，`ingest.Service` 取得 `IndexSkill` 一個。刻意不為單一實作鋪 interface。欄位名不沿用 query 名——ownership 檢查以文字比對呼叫點，叫 `UpsertSearchDocument` 會讀成 registry 仍在呼叫它。

- **fail closed 的位置**：`registry.requireProjection()` 在 `Fork`／`Delete`／`Takedown` 三支方法開頭、`Pool.Begin` 之前呼叫（其餘方法不寫投影，不製造假失敗）；`ingest.requireProjection()` 在 `persistVersion` 開頭（`importZip` 與 `SaveVersion` 兩條匯入路徑都經過它，且在第一次寫列之前）與 `ReindexPending` 開頭（該路徑不經 `persistVersion`，且要在花掉 enrichment 呼叫之前拒絕）。`CheckSources` 不寫投影，未被納入。

- **注入點**：`apiserver.NewApp`（`registry.Service` 與 `ingest.Service` 各一處）與 `cmd/reindex/main.go`（backfill 的 `ingest.Service`）。**`cmd/maintenance` 判定不注入**：它建的 `ingest.Service` 只跑 `CheckSources`（探測來源可用性、`MarkSourceChecked` 與 audit），全程不碰 `search_documents`；注入一個該路徑永遠不會呼叫的函式，會讓下一個讀者以為維運命令也在維護投影。日後若該命令新增寫投影的路徑，`requireProjection` 會當場擋下來，而不是靜默漏寫。

- **交易語意前後對照**（本 ADR 要保住的兩個保證之一）：
  - `registry.Fork`：投影 upsert 原本是 `q.UpsertSearchDocument(...)`，`q` 來自 `gen.New(tx)`；現在是 `s.IndexSkill(ctx, q, ...)`，傳的是**同一個 `q`**，仍夾在 `CreateSkillVersion` 與 `audit.Log` 之間、同一個 `tx.Commit` 之前。
  - `registry.Delete`／`Takedown`：同理，`DeleteSearchDocument` 換成 `s.RemoveFromIndex(ctx, q, ...)`，仍與 `SoftDeleteSkill`／`TakedownSkill` 及 audit 事件同一個 commit。
  - `ingest.persistVersion`：`upsertProjection` 由 package 函式改為方法，內部由 `q.UpsertSearchDocumentEnriched` 換成 `s.IndexSkill(ctx, q, ...)`，`q` 仍是呼叫端交易的 `gen.New(tx)`——INGEST-009 要求的「版本列與投影同交易」未動。
  - `ingest.ReindexPending`：`q` 仍是 `gen.New(s.Pool)`，逐列自主提交的語意未動。
  - 證明方式：三支 catalog 函式全部只有 `return q.XXX(...)` 一行，沒有 `Begin`／`Commit`／`Pool`，也沒有拿到 pool 的管道（簽名裡沒有）；`golangci-lint`（含 depguard）零 issue，故沒有新增任何跨 context import；`go test ./...` 全綠，其中 `apiserver` 的匯入／fork／takedown 整合測試與 `db/tests` 的既有斷言都在驗同交易後的可見性。

- **反向驗證**：拿掉 `NewApp` 裡 `registry.Service` 的 `IndexSkill` 後，`TestForkCatalogSkillIntoCallerWorkspace` 由 201 變 500（fork 被拒絕而非「成功但搜不到」），`TestNewAppWiresEveryRouteAndService` 同時報 `the registry service is missing a search projection write`；拿掉 `ingest` 的注入則報 `the import path is missing catalog's search projection write`。另有兩支不需資料庫的單元測試守住執行期拒絕：`registry.TestWritesRefuseWithoutTheProjectionWrites`、`ingest.TestImportPathsRefuseWithoutTheProjectionWrite`（兩者都在沒有 pool／交易的情況下呼叫，所以任何越過檢查的路徑會 panic 而非回錯，這同時證明了檢查的順序）。

- **未改動**：`.golangci.yml` 與 ADR-032 附錄 A 都不需要動——注入沒有製造任何新的跨 context import，這正是選它的理由。`db/query-owners.yaml` 的 `allow:` 移除 `UpsertSearchDocument`／`DeleteSearchDocument`／`UpsertSearchDocumentEnriched` 三條，**剩 6 條**（帳號刪除 purge，下一批）。

**2026-08-20 已執行（帳號刪除 purge，6 條）**：

- **五個 owner context 各公開一支 `PurgeWorkspace`**，全部收呼叫端的 `*gen.Queries`、不自開交易，簽名一致：

  ```go
  func PurgeWorkspace(ctx context.Context, q *gen.Queries, workspaceID pgtype.UUID) error
  ```

  | 檔案 | 做的事 |
  | --- | --- |
  | `apps/platform/internal/analytics/purge.go` | `DetachWorkspaceAnalytics` ＋ `DetachWorkspaceFeedback`（兩條 query 同屬 analytics，合成一步） |
  | `apps/platform/internal/testlab/purge.go` | `DeleteWorkspaceDatasets` |
  | `apps/platform/internal/run/purge.go` | `DeleteWorkspaceRunArtifacts` |
  | `apps/platform/internal/packaging/purge.go` | `DeleteWorkspaceDownloadArtifacts` |
  | `apps/platform/internal/registry/purge.go` | `PurgeUnreferencedSkills` |
  | `apps/platform/internal/ingest/purge.go` | `PurgeUnreferencedSkillSources` |

  名稱一律 `PurgeWorkspace`：analytics 那支實際做的是去識別化而非刪除（ADR-029 決策 5），差異寫在它自己的 doc comment 裡；函式名描述的是「帳號刪除掃到你了」這個觸發，不是機制。**analytics 是兩條 query 一步**，因為 owner 是同一個 context，切成兩步只會讓 identity 那邊多一個它無權解釋的區分。

- **注入形狀**：`identity.Service` 取得六個具名 function 欄位（含 packaging 的 `PurgeDownloads`，型別皆為 `identity.WorkspacePurge`）。欄位名不沿用 query 名，理由同第一組。六個欄位由 `purgeSteps()` 收成一份**有序的具名步驟清單**，fail-closed 檢查與交易內的迴圈都讀同一份；三個 owner object-key reader 另由 `objectKeySteps()` 在交易前執行並去重，漏接任一項都拒絕整批。

- **順序是 load-bearing 的，而且被守住**：`registry` 必須在 `ingest` 之前。`PurgeUnreferencedSkillSources` 只刪「沒有任何 `skill_versions` 指著」的 source，而刪掉那些 version 列的正是 `PurgeUnreferencedSkills`；倒過來跑，該帳號的每一條 source 都還被活著的 version 引用，於是一條都不刪，**沒有錯誤、沒有異常的 row count，只有留下來的匯入溯源**。理由寫在 `purgeSteps()` 的行內；**並且有測試**——`TestAccountPurgeHardDeletesPrivateContentAndDeIdentifiesTheRest` 現在會 seed 一個帶 source 的版本並斷言 source 消失，把那兩行對調就會紅（實測輸出：`import source of a purged version survived; the purge steps ran out of order`）。這是刻意不重蹈 DDD-023「順序 load-bearing 但沒人知道」。其餘三步之間沒有相依，順序即宣告順序。

- **fail closed 的位置**：`identity.requirePurgeSteps()`，在 `PurgeExpiredAccounts` 的第一行、**連 worklist 都還沒讀**之前。訊息列出缺的是哪幾個 context：`identity: account purge steps not injected for registry; refusing to purge`。這一條與第一組的性質不同——第一組防的是答案不對，這一條防的是**清了一半卻回報成功**：使用者被告知資料已刪除、實際還在，而且沒有任何地方會紅。整批失敗反而是可回復的：刪除請求留在 worklist 上，下一次在正確接線的部署上跑完。

- **注入點**：`apiserver.NewApp`（一處，`identity.Service` 的 literal）與 `cmd/maintenance` 的 `purgeService`。覆核過的其他 `identity.Service` 建構點只有 `authz_integration_test.go` 的 session 測試，它們不跑 purge，因此不接。

- **`cmd/maintenance` 補了 wiring smoke test**（`main_test.go`，不需要資料庫）。DDD-018 當初判定它不需要，理由是「單一 struct literal 緊接唯一使用它的呼叫」；本批之後那個理由不再成立——**真正執行 purge 的是這個程序，不是 API**，而它的 literal 現在帶著五條注入。為此把 literal 抽成 `purgeService(pool)`，測試以反射掃 `identity.Service` 中型別為 `WorkspacePurge` 的欄位斷言非 nil，所以第六個 context 加進來的當天自動被涵蓋。`apiserver` 的 `TestNewAppWiresEveryRouteAndService` 用同一招。

- **交易語意前後對照**：`purgeAccount` 的交易邊界一個字沒動——`Pool.Begin` →「`SET LOCAL skillhub.purge = 'on'`」→ `q = q.WithTx(tx)` → 每個 workspace 的六條清除 → `DeleteUserIdentities`／`DeleteUserSessions`／`Anonymize*` → `audit.Log` → `tx.Commit`。改的只是中間那段從六個 `q.XXX(...)` 變成對 `purgeSteps()` 的迴圈，傳的是**同一個 `q`**。`SET LOCAL` 的作用範圍因此完全不變：它是交易級的，而 owner 的函式跑在同一個交易裡，所以 `registry.PurgeWorkspace` 硬刪 `skill_versions` 仍然被 0013 的 trigger 放行。反過來說，如果哪一支 owner 函式將來自己開交易，它會**當場被不可變 trigger 擋下**——那是 registry 這一支獨有的第二道保險，另外四支沒有，所以規則仍然靠註解與 code review。

- **反向驗證**：
  1. **fail-closed**：拿掉 `NewApp` 的 `PurgeSkills` 注入後，`TestNewAppWiresEveryRouteAndService` 報 `identity.Service.PurgeSkills is nil: the account purge would refuse to run`，purge 整合測試報 `identity: account purge steps not injected for registry; refusing to purge`——**是拒絕，不是少清一個 context 然後回報成功**。已還原。
  2. **交易原子性**：`TestAccountPurgeRollsBackEveryContextWhenOneStepFails`（`internal/apiserver/governance_integration_test.go`，**常設**）把最後一步（ingest）的注入欄位換成必定失敗的函式——不需要改生產程式，這正是注入設計換來的。斷言分兩半：**失敗步驟之前已經跑過的步驟其效果全部不見**（skill、skill_versions、artifacts 各 1 列仍在），**且 identity 自己的去識別化也沒生效**（`users.email` 仍含 `alice-rollback`）。少了後半，「整批回滾」與「只有那一步沒做」看起來會一樣。最後把注入還原再跑一次，確認帳號仍可被清掉——否則「下一次重試」不算是 fail-closed 宣稱的復原。

     這支測試守的就是本 ADR「單一交易、全有全無」那個主張本身，所以**刪掉它等於放棄拒絕事件化的主要論據**，測試的 doc comment 也是這樣寫的。反向驗證：把 `purgeAccount` 的步驟錯誤改成吞掉（正是本 ADR 擔心的失效模式之一），四項斷言全部轉紅，`purged` 由 0 變 1、`users.email` 變成 `deleted-...@deleted.invalid`。已還原。

- **已知缺口（本批不改）**：`PurgeExpiredAccounts` 對**單一帳號**的失敗是 `slog.Error` ＋ `continue`，回傳 `(purged, nil)`。原意是「一個壞帳號不該卡住整批」，但代價是 `maintenance purge-accounts` 在每個帳號都失敗時仍以 exit 0 結束，只印 `account purge complete accounts_purged=0`。注意這**不影響**未注入的情況（`requirePurgeSteps` 的錯誤是直接回傳的，整個命令會失敗）；受影響的是 owner 步驟在執行期失敗。建議：讓 `PurgeExpiredAccounts` 在有任何帳號失敗時回傳一個彙總錯誤（仍然跑完整批），使維運看得到紅燈。屬於既有行為，非本批引入，未在本批修改。

- **未改動**：`.golangci.yml` 與 ADR-032 附錄 A 同樣不需要動——注入沒有製造任何新的跨 context import。`db/query-owners.yaml` 的 `allow:` 移除最後 6 條後**清空**（保留 section 標頭；`devctl` 的解析器對空 section 正確處理，見下）。

- **`allow:` 清空後的 devctl 行為**：`parseOwnerDeclaration` 對 `allow:` 這一行就建立空 map，必要 section 檢查因此仍然通過，後續所有以 `allow` 為底的迴圈都是零次——與早已為空的 `raw_sql_allow:` 同一條路徑。**`tools/devctl` 不需要任何修改**，`automation-check` 通過。

- **`immutable_allow:` 的 `PurgeUnreferencedSkills`**：查詢本身與它寫的 `skill_versions` 都沒變，呼叫點搬到 `registry.PurgeWorkspace` 不影響該檢查（它只比對 query 與表）。條目仍然正確。但它的註解原本寫著「等 ADR-033 的事件式自清落地後，這條 CTE 連同本豁免一起消失」——**本 ADR 否決了事件化，那條清除路徑已不存在**，因此把註解改成記錄現況：豁免沒有預定消失時程，要移除得先讓帳號刪除不再硬刪 `skill_versions`。

**2026-08-22 DDD-051 carrier 收斂註記**：上述 purge 的同步與單交易決策不變，但跨 context contract 已由 `*gen.Queries` 收窄為 caller 的 `pgx.Tx`。六個 owner `Service.PurgeWorkspace` 在自身 package 內 `gen.New(tx)`；identity 仍以同一個 tx 依序呼叫，registry-before-ingest 不變。交易前的 dataset／run output／download package object-key reads 則改為三個 owner Service 的 pool-backed bound methods，只收 `ctx/workspaceID`。因此 object bytes 仍先刪，DB rows／identity 去識別化／audit 仍在其後同一 transaction；API 與 maintenance roots 注入 bound methods，缺任一 dependency 仍整批拒絕。

## 為什麼不事件化

**同一個資料庫、同一個程序。** ADR-010 的拆分條件（負載、組織、安全需求）尚未觸發，四個 process 共用一個 Postgres。事件化在這個前提下買到的是「未來拆分時比較好搬」，付出的是兩個**現在就成立**的保證。用還沒發生的搬遷去換已經在手上的原子性，是拿確定的東西賭不確定的收益。

**ADR-032 §4 已經表過態**：「不採 event sourcing、不引 CQRS 框架：Catalog 的搜尋投影（`reindex`）已是夠用的手工 CQRS」。同交易維護的投影正是那句話描述的東西；把它改成事件驅動，等於在 ADR-032 說不需要框架的地方引入框架的複雜度而不引入框架。

**Outbox 不是為了這個。** ADR-008 的 Transactional Outbox 解決的是「領域狀態變更與**對外**事件同交易」，用於觸發後續反應（Run 終態 → 排評估）。ADR-032 §2 的判準寫得很清楚：跨 context 的呼叫若失敗會導致「當下這筆請求無法正確回應」→ 同步；若只是「之後該發生的事沒發生」→ 事件。**匯入完成卻搜不到，是當下這筆請求沒有被正確完成**；**帳號刪除只清了一半，是當下這筆請求沒有被正確完成**。兩者都落在同步那一側，不落在事件那一側。

**最終一致的刪除是被迫接受的，不是被選擇的。** 硬刪除跨越多個資料庫或服務時，你別無選擇只能最終一致，並補上對帳與「清完了」的判定機制。在單一資料庫裡主動放棄原子性，等於自己製造一個需要對帳的問題，再自己去解它。

## 為什麼是注入而不是 import

兩組的 import 方向都成環：

- `catalog → registry` 自 DDD-020 起存在（下架旗標寫入回到 owner），因此 `registry → catalog` 是循環，Go 編譯期即拒絕。
- 每一個 context 都 import `identity`（鐵律 3 的 Workspace scope 入口），因此 `identity → 任何人` 都是循環。

**循環是訊號，不是障礙。** 它說的是這些 context 互為對等（peer），不是上下層——`catalog` 需要 `registry` 的旗標寫入，`registry` 需要 `catalog` 的投影寫入，兩者誰也不在誰底下。依賴反轉正是對等關係的標準表達：呼叫端宣告它需要一個「把投影寫進去」的能力，由知道全貌的 composition root 提供，沒有任何一方被迫假裝自己在另一方之下。

`ingest → catalog` 其實不成環（catalog 不 import ingest），技術上可以直接 import。**仍然採注入**，因為同一組漂移用兩種形狀收斂，會讓下一個讀者以為那個差異有意義。

這個做法在本 repo 已有前例：DDD-020 的 `objreconcile` 半邊就是注入（generic 掃描器不得反向依賴領域 context），且已驗證可行。

## 考慮過但拒絕

- **事件化（ADR-033 原提案）**：理由如上。若日後 ADR-010 的拆分條件觸發、某個 context 搬到自己的資料庫或部署單元，**被注入的那個函式正是事件要取代的接縫**——屆時改動範圍是 composition root 加該 context，不是整條呼叫鏈。這是刻意留下的升級路徑。
- **把 `search_documents` 改由 `registry`／`ingest` 擁有**：投影的欄位語意（enriched summary、task examples、tags）由檢索管線決定，擁有權跟著讀取者走才對。搬擁有權只是把同一條跨界移到另一個方向。
- **把 purge 的六條 query 併成一條巨型 CTE 放在 `identity`**：能讓 ownership 檢查閉嘴，但「刪除一個 workspace 的 dataset 是什麼意思」是 testlab 的領域知識，不是 identity 的。那會是把漂移藏進 SQL，不是消除它。
- **維持現狀、讓 9 條留在 `allow:`**：`allow:` 的棘輪確實讓它們不會惡化，但存量不會自己消失，而 ADR-033 明說 `allow:` 是清單不是擴充點。留著等於讓「暫時」變成事實上的永久。

## 影響

### 正面

- 9 條存量漂移清零，`db/query-owners.yaml` 的 `allow:` 清空——ownership 的宣告從此描述現況，不再描述現況加一份例外。
- 兩個交易保證原封不動：帳號刪除仍是單一交易的全有全無，匯入完成的當下仍然搜得到。
- 兩個原本卡住的決策（CORE-007 的「清完了」判準、M1 的可發現性窗口）**不需要被回答**——它們是事件化才會產生的問題。
- 每個 context 的「我的資料被刪除時該做什麼」回到自己手上，那本來就是領域知識。

### 代價

- Composition root 變大：`apiserver.NewApp`、`cmd/reindex`、`cmd/maintenance` 各要多接幾條線。DDD-018 建立的 wiring smoke test 是對應的防線，新注入點必須進那些測試。
- 依賴從編譯期變成執行期。fail-closed（未注入即拒絕執行）與 wiring 測試是補償；**沒有這兩者的注入是把靜態錯誤換成半夜的靜默錯誤**，不接受。
- `identity` 的 purge 交易會依序呼叫五個 context 的函式，交易變長。purge 是離峰的維運命令（`cmd/maintenance`），不在請求路徑上，可接受。

## 不變事項

- ADR-033 的決策（query ownership 以宣告檔強制、先鎖 write）不變；本 ADR 只取代它為第 1、3 組提出的補法。清除路徑 2、4 已執行且與本 ADR 一致。
- ADR-008 的 Transactional Outbox 用途不變：對外領域事件仍走它，本 ADR 不把任何既有事件改回同步。
- 鐵律 9（領域狀態變更與對外事件同交易）不變，且本決策強化了它的精神：同交易的範圍變大而不是變小。

## 待決策

- 若日後 `catalog` 的檢索需要脫離主資料庫（例如專用檢索引擎），投影就不再能同交易寫入，屆時事件化成為必要而非選擇。觸發條件與 ADR-010 的拆分條件一併評估。
- ~~read ownership 是否開始強制（ADR-033 待決策）：本 ADR 清掉 write 側存量後，該問題可以在乾淨的基礎上重新評估。~~ → [ADR-035](./ADR-035-read-ownership-enforcement-and-context-map-completeness.md)：已在乾淨的基礎上重評並**決定強制**。其中 D 組（`run` 的 Run 事實）的正解沿用本 ADR「為什麼是注入而不是 import」——`eval`／`trace`／`analytics`／`policy` → `run` 都在 deny，而 `run → policy`、`run → trace` 是既有合法方向，加白名單會製造循環。
