# M6 前期量測：沒有容器的機器上，能不能得到一個真的 PostgreSQL

- 日期：2026-08-28
- 決策落點：[ADR-058](../../../adr/ADR-058-the-clean-test-mode-is-real-postgres-behind-the-api-seam.md)
- 執行環境：Windows 11、Docker Desktop 在跑（但**每一項實驗都刻意不使用它**）、Go 1.27.0、Node v25.0.0

## 0. 這份報告是什麼、不是什麼

**是**：一批可重跑的實測，用來回答一個是非題——**在一台不能安裝軟體的機器上，`db/migrations/` 的 42 支能不能一字不改地跑起來，而且跑起來之後那些由資料庫強制的規則是不是還在守。**

**不是**：M6 的允收。允收準則在 [`02` §4.10](../../02-specifications-and-acceptance-criteria.md)（`PORT-001`～），工作項在 [`03` §20](../../03-work-items.md)。本報告只提供那些準則所依據的數字。

**為什麼先跑**：與 M5 的 `report-generate-spike.md` 同一個理由——ADR-058 要在兩個候選之間選，而在跑之前**兩邊都只有推理沒有數字**。其中一個候選（手寫 in-memory 資料層）如果選錯，代價是兩年後才會發現，而業界已經有人替我們付過那筆學費。

## 1. 現況：今天少跑多少

`go test -count=1 ./...`（**`-count=1` 是必要的**：快取會讓無資料庫的那次回報上一次有資料庫的結果，我第一次就踩到了）。

| 模組 | 無容器環境 | 有 | 差 |
| --- | --- | --- | --- |
| `apps/platform` | **562** | **889** | 287 筆 skip 記錄（有 DB 時展開成 327 支 pass——子測試只在有 DB 時才被建立） |
| `apps/sandbox` | 靜默 skip | 109 | `t.Skipf("no docker daemon reachable")` |
| `apps/llm` | 128 | 128 | 不受影響（mock） |
| `apps/web` | 220 | 220 | 不受影響（jsdom） |

**284 筆的唯一原因是資料庫**，其中 267 筆（94%）集中在 `apiserver` 一個套件。其餘 3 筆是語料庫缺席（`GENERATED_CORPUS`／`SEED_CORPUS`／`QA002_CORPUS`）。

**就算有了資料庫也跑不了的，只有 3 支**，全部與 DB 無關且都會花錢：`TestGeneratedSkillsRunAndAreJudged`、`TestEndToEndRunCallsTheModelThroughItsOwnVirtualKey`、`TestARealGatewayGenerationRecordsWhatItActuallyCost`。

**兩個模組都是靜默跳過、畫面全綠。** 一位沒有容器環境的同仁跑完看到的綠燈，量的是六成三，而沒有任何東西告訴他。**要記住的是：今天的跳過本身是誠實的**（訊息明說 `SKILLHUB_TEST_DATABASE_URL not set`）；任何讓畫面更綠卻沒有真的執行斷言的改動，都比現狀更糟。

## 2. 依賴收斂：那 284 筆缺的只有一樣東西

逐條檢查全部 `t.Skip` 的觸發條件，以及誰引用 `OBJSTORE_*`：

- **五處 skip 訊息全部是「`%s not set`」，指的都是資料庫 URL 環境變數。**
- 引用 `OBJSTORE_*` 的只有 `e2e_gateway_integration_test.go` 與 `gen009_baseline_test.go`——**兩支都是另行 gate、會花錢的 E2E**。
- 一般整合測試用的物件儲存是**行程內的 `map[string][]byte`**（`disc_detail_integration_test.go:26` 的 `packageStore`），實作 `Get`／`Put`／`Remove`／`Exists`／`PresignGet`／`PresignPut` 六個方法，正好是真實 client 的全部表面。
- 模型服務不在依賴裡：`apiserver.Config` 的 `LLM` 為 nil **是一個可運作的部署**（搜尋誠實降級為 FTS-only、enrichment 標 pending、建議回報自己不可用）。

**結論：那 284 筆的外部依賴是 PostgreSQL，僅此一項。**

> **一個順帶查到、與 Docker 無關的洞**：整個平台測試套件裡**沒有任何一行 fetch 過那個假的 presigned URL**（`grep objects.test` 只有產生它的兩行）。`SBX-008` 短效授權的「短效」與「單一路徑」兩個性質，今天沒有任何自動測試在證明。**這個缺口不是「沒有 Docker」造成的，是本來就不存在**，而且做一個「檔案系統版物件儲存」不會補上它——檔案系統一樣簽不出通行證，只是讓那個 fake 看起來比較不像 fake。

## 3. SQLite：0 / 42

純 Go、無 cgo 的 `modernc.org/sqlite`，`:memory:`，依序套用真正的 42 支 migration。

```
FAIL 0001_init.sql                     SQL logic error: near "EXTENSION": syntax error (1)
     ^ 第一次失敗：之後每一支都跑在一個已經錯掉的 schema 上
FAIL 0005_immutability.sql             SQL logic error: near "FUNCTION": syntax error (1)
FAIL 0008_skill_deletion.sql           SQL logic error: no such table: skills (1)
FAIL 0009_search_vector_index.sql      SQL logic error: near "USING": syntax error (1)
...
applied cleanly: 0 / 42
```

**一個會誤導人的數字**：42 支裡有 **21 支完全不含 PostgreSQL 專屬語法**，看起來一半可移植。**實際上一支都跑不起來**——後面那一大批 `no such table` 全是因為前面沒建起來。**schema 是一條鏈，不是一疊各自獨立的檔案。**

專屬能力在 migration 裡的分布（任何取代品都得先實作出來）：

| 能力 | 出現在幾支 migration |
| --- | --- |
| `gen_random_uuid()` | 10 |
| plpgsql 函式／trigger | 9 |
| RANGE 分割表 | 4 |
| `GENERATED ALWAYS AS IDENTITY` | 3 |
| `vector` 型別／`ivfflat` 索引 | 2 |
| `CREATE EXTENSION`／ENUM／generated `tsvector` 欄位／FTS | 各 1 |

## 4. PGlite：42 / 42

`@electric-sql/pglite` 0.5.8 ＋ `@electric-sql/pglite-pgvector` 0.0.9（**2026-08-26 發佈，兩天前**）。同一支腳本、同一批 migration。

```
applied cleanly: 42 / 42
```

**但「沒報錯」不等於「真的做了」。** 逐項驗證：

| 驗證項 | 結果 |
| --- | --- |
| pgvector `<=>` 餘弦距離 | ✅ `SELECT ('[1,0,0]'::vector <=> '[0,1,0]'::vector)` → `1` |
| `ivfflat` 索引 | ✅ `search_documents_embedding_idx` 存在於 `pg_indexes` |
| `trace_events` 是 RANGE 分割表 | ✅ `relkind='p'`，2 個子分割 |
| generated `tsvector` 欄位 | ✅ 欄位定義成立 |
| **不可變性 trigger（鐵律 4）** | ✅ `UPDATE skill_versions` → **被擋**：`row in public.skill_versions is immutable and cannot be updated (ADR-003)` |
| **同上，DELETE** | ✅ **被擋**：`row in public.skill_versions is immutable and cannot be deleted (ADR-003)` |
| advisory lock | ✅ `pg_try_advisory_lock` → true、`pg_advisory_unlock` → true、阻塞式 `pg_advisory_lock` → 取得、`pg_locks` 回報 1 筆 |

**這一列是整份報告的重點**：鐵律 4 由資料庫強制的那個機制，**在 PGlite 上真的會擋人，而且錯誤訊息裡帶著當初寫下它的 ADR 編號**。這正是選項 A／B 會讓它人間蒸發的東西。

**它不是相容實作，它就是 PostgreSQL**——自報版本字串：

```
PostgreSQL 18.3 (PGlite 0.5.8) on wasm32-unknown-emscripten,
compiled by emcc (Emscripten gcc/clang-like replacement + linker emulating GNU ld) 3.1.74, 32-bit
```

## 5. Wire protocol：Go 連得上，但只有一條線

`@electric-sql/pglite-socket` 0.2.11 讓 PGlite 以 Postgres wire protocol 對外監聽。pgx 直連：

```
connected. public tables = 35
server: PostgreSQL 18.3 (PGlite 0.5.8) on wasm32-unknown-emscripten
```

**然後把真正的測試套件指過去，它掛住了。** 用 Go 自己的 `-timeout` 取堆疊：

```
goroutine 1 [running]:
...library.lockTestSchema(...)
	aggregate_test.go:369
...library.TestMain(...)
	aggregate_test.go:95
```

**卡在 `lockTestSchema`——測試 harness 自己拿的 advisory lock。** 而第 4 節已經證明 advisory lock 完全支援，所以這不是缺功能。**是這段程式握著一條連線持鎖、再向只有一條的池子要第二條去跑 migration。**

加上 `pool_max_conns=1` 之後不再 panic（進步），但改為死鎖 238 秒。

### 5.1 補測（2026-08-28）：死鎖不是天花板，是這段測試程式的寫法

本報告第一版把那個死鎖當成「Go 這條路沒有解」。**那是錯的，而且是把量測結果當成了不可能。** 把 `lockTestSchema` 改成不長期握住連線（其餘一字未改），同一包從**整包死鎖**變成：

```
library  16 passed, 3 failed      （失敗三支全是 prepared statement 快取，不是鎖）
```

實驗用的修改**已還原**，`git diff` 為空。真正需要多於一條連線的是 6 支用 goroutine 併發的測試檔，不是整個套件。

### 5.2 補測（2026-08-28）：打開 multiplexer 之後，鎖安靜地不再是鎖

`pglite-socket` 預設 `maxConnections: 1`——第二條連線**直接被踢掉**（實測：`An existing connection was forcibly closed`）。這是誠實的失敗。

但它有一個 multiplexer，設 `maxConnections: 10` 就會收下 N 個 client。**它不是連線池**：N 個 client 共用**同一個 Postgres session**。兩條獨立 pgx 連線實測：

```
backend pid A=42  B=42                    ← 同一個 session
A pg_try_advisory_lock(4242) = true       （預期 true）
B pg_try_advisory_lock(4242) = true       （真 PostgreSQL 應為 FALSE）
>>> 兩邊同時持有同一把鎖，互斥消失，沒有任何錯誤訊息
>>> B 看得見 A 的 TEMP TABLE
```

（走 extended protocol 時 B 會先撞上 `prepared statement "stmtcache_…" already exists`——prepared statement 在真 PostgreSQL 是 session-local，撞名本身就是同一 session 的證據。）

**這正是本 repo 反覆記載的那種缺陷的教科書形式**：不是「不支援」而報錯，是**看起來支援、而保證是假的**。PostgreSQL 官方文件寫明「若 session 已持有某 advisory lock，它後續的請求一律成功」——multiplexer 讓所有 client 變成同一個 session，於是這句話從保護變成漏洞。

**對本專案特別致命**，因為產品程式有三處靠 advisory lock 互斥：`creator/workspace/service.go`（`pg_advisory_xact_lock`）、`foundation/messaging/outbox`（publisher 單一化）、`db/gen/runs.sql.go`。它們的測試會在這個組態下**全綠而毫無保證**。

**所以 `maxConnections > 1` 在本專案是禁用組態，不是效能取捨。**

**另一個要記住的操作性質**：前一個連線異常結束會讓伺服器進入不可用狀態，下一次連線得到 `An established connection was aborted`。本報告的實驗中出現過兩次，兩次都要重啟才能繼續。

## 6. 兩個被排除的候選，以及排除的根據

**`fergusstrange/embedded-postgres`**（下載真 PG 二進位、以子程序執行）：支援 Windows、是真 Postgres，**但它要下載並執行一個外來二進位**——那正是目標環境禁止的行為。此外 zonky 的 bundle 不含 pgvector（issue #163），繞法是自建二進位包並以 `BinariesPath` 指過去，ABI 必須與其 MSVC 工具鏈對上。

**DoltgreSQL**（Go 寫的 PostgreSQL 相容資料庫，1.0 於 2026-08-06 發佈）：擴充套件的做法是 bridge 到**真 Postgres 編出來的 `.so`**——借得到就不需要它。`dolthub/doltgresql` **issue #3014（2026-08-01 開啟，未關）就是 `CREATE EXTENSION IF NOT EXISTS vector;` 失敗**，正是本 repo 第一支 migration 的第一行。另：只能起 server，不能 in-process 嵌入。

## 7. 沒有量到的、以及本報告不主張的

- **Adapter B 實際跑起來的樣子沒有量過。** 本報告證明的是「PGlite 能承載這個 schema」，不是「瀏覽器裡的 demo 已經可行」。端點範圍、SQL 抽取步驟、`file://` 下的路由與打包，全部未驗證。
- **受限環境的白名單內容未知。** Node 在不在上面決定 wire protocol 那條路能不能用；本報告在一台可以自由安裝的機器上執行，**所以它證明的是技術可行性，不是該環境的可行性**。
- **未量寫入路徑。** 全部驗證都在 schema 與讀取語意上；一次完整的 Run 建立、狀態轉移與評估寫入沒有在 PGlite 上跑過。
- **單連線對 Adapter B 沒有影響**（瀏覽器一頁一連線），對 socket 模式有。兩者不要混為一談。
- **`@electric-sql/pglite-pgvector` 是 0.0.9。** 極新。本報告的所有 pgvector 結論都綁在這個版本上。

## 9. 補測（2026-08-28）：`file://` 下的交付形式，Chromium 與 Firefox 不一樣

`PORT-005` 要的是「以瀏覽器開啟本機檔案即可運作」，而本報告第一版**完全沒有量過瀏覽器**。補測結果推翻了那個假設可以無條件成立：

以 playwright 開同一個本機 HTML，一個 inline module 內含 `import { x } from "./mod.js"`：

```
chromium  #out = "NOT RUN"        error = Access to script at 'file:///…' blocked by CORS policy (origin 'null')
firefox   #out = "module loaded"  error = none
```

第二輪確認可用的形式（兩個瀏覽器都通過）：

```
chromium  classic inline ok | classic external ok | inline module ok
firefox   classic inline ok | classic external ok | inline module ok
```

**所以界線不在「module 不能用」，在「不能 `import`」。** 一個沒有 import 的 inline module 是可以的；只要去抓同目錄的另一個檔案，Chromium 就以 opaque origin 擋下。

**這對本專案是直接的**：Vite 的產出是 `<script type="module" src="/assets/index-*.js">`——**外部 module，必掛**。而受限環境的瀏覽器極可能只有 Edge（Chromium 系）。**交付形式因此必須是全部內嵌的單一檔案**，不能只是「把 dist 資料夾拷過去」。

### 9.1 續測：WASM 本身沒問題，被擋的是「去抓旁邊的檔案」

上一段把 PGlite 能否在 `file://` 下起來列為推導。續測把機制量清楚了：

| 動作 | Chromium | Firefox |
| --- | --- | --- |
| `fetch("./檔案")` | **擋**（`Fetch API cannot load file:///…`） | OK 200 |
| `fetch("data:…")` | OK | OK |
| `WebAssembly.instantiate(bytes)` | **OK**（回傳 42） | OK |
| `WebAssembly.instantiateStreaming(fetch("./x.wasm"))` | **擋** | **擋**（Firefox 也擋） |

**所以 WASM 不是障礙**——把 bytes 直接交給它，兩個瀏覽器都執行得好好的。障礙只有一個：**取得那些 bytes**。

而 `@electric-sql/pglite` 的 `dist/index.js` 裡，`fetch(`、`new URL(…, import.meta.url)` 與 `WebAssembly.instantiateStreaming` **三者都在**（各 4／7／2 處）——**正好是被擋的那三條路**。

**因此 `PORT-005` 需要一個尚不存在的建置步驟**：把 PGlite 的 WASM 與檔案系統映像以 base64（或 `Uint8Array` 字面值）內嵌，並繞開 `instantiateStreaming`。**這一步沒有做過，也沒有查證 PGlite 是否提供注入 bytes 的入口。** 已知的成本是體積：3.7 MB 的 WASM 以 base64 內嵌約 +33%，加上檔案系統映像。

**這一段仍未量到的**：PGlite 有沒有官方支援的 bytes 注入路徑；以及全部內嵌之後單檔的實際大小與首次載入時間。

## 8. 可重跑

實驗腳本置於 scratchpad，未進 repo（它們依賴 npm 套件且只是一次性驗證）。要重跑的話，四支各自的形狀是：

1. **SQLite**：Go 程式，`modernc.org/sqlite` 開 `:memory:`，`filepath.Glob` 取 `db/migrations/*.sql` 排序後逐支 `db.Exec`，記錄成功與第一個錯誤。
2. **PGlite**：Node ESM，`new PGlite({ extensions: { vector } })`，同樣逐支 `db.exec`。
3. **行為驗證**：接續 2，逐條查 `pg_indexes`／`pg_class.relkind`／`pg_inherits`，並實際 `INSERT` 一列 `skill_versions` 後嘗試 `UPDATE` 與 `DELETE`（**種子資料需要 `users(email, display_name)` 與 `workspaces(owner_user_id, name)`**，我第一次漏了 `owner_user_id`，那次的紅是我的測試寫錯不是 trigger 沒擋）。
4. **wire protocol**：`PGLiteSocketServer({ db, port, host })`，另一側用 pgx 連。
5. **session 共用**（§5.2）：同上但加 `maxConnections: 10`，用**兩條獨立的 pgx 連線**各查 `pg_backend_pid()`、各要一次 `pg_try_advisory_lock(4242)`、其中一條建 temp table 另一條讀它。**兩條都拿到鎖就代表互斥已經不存在。**
6. **`file://`**（§9）：playwright 分別以 chromium 與 firefox `goto("file:///…/index.html")`，頁內放四種 script 形式，比對哪一種真的執行。
