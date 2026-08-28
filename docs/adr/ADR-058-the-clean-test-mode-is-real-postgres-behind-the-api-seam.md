# ADR-058：乾淨測試模式用真 PostgreSQL（PGlite），Adapter 畫在 API 接縫而不是資料庫層

- 狀態：Proposed
- 日期：2026-08-28
- 決策者：產品負責人、架構規劃
- 相關：[ADR-018](./ADR-018-data-platform-and-storage.md)（PostgreSQL 中心）、[ADR-016](./ADR-016-language-split-and-cross-language-contracts.md)（語言分工與契約）、[ADR-003](./ADR-003-immutable-versions-and-snapshots.md)（不可變版本，鐵律 4 的來源）、[ADR-030](./ADR-030-portable-developer-automation-and-contract-code-generation.md)（可攜開發自動化）、[ADR-006](./ADR-006-provider-port-and-runtime-adapters.md)（Provider Port 與 Adapter）

## 背景

**這份 ADR 的觸發點不是技術偏好，是一個部署環境的事實**：MVP 的 Pitch 在金融機構環境進行，那台機器**只能執行白名單內的程式**，不能安裝軟體，對外網路除了模型供應商之外都不保證。同一個環境也是部分同仁的日常開發環境。

在那台機器上，今天的系統**一行都跑不起來**：`task dev` 要 Docker、DB 測試要 PostgreSQL、沙箱要容器 runtime。而量測顯示這件事的規模是可判定的：

| 模組 | 無容器環境 | 有 |
| --- | --- | --- |
| `apps/platform` | **562** | **889** |
| `apps/sandbox` | 靜默 skip | 109 |
| `apps/llm` | 128（不受影響） | 128 |
| `apps/web` | 220（不受影響） | 220 |

**287 筆 skip 記錄（有 DB 時展開成 327 支 pass），其中 284 筆的唯一原因是資料庫**，而 267 筆（94%）集中在 `apiserver` 一個套件。剩下 3 支本來就要花錢。**兩個模組都是靜默跳過、畫面全綠**——一位沒有容器環境的同仁看到的綠燈，量的是六成三。

**物件儲存不在這個問題裡**：那些測試用的是行程內的 `map[string][]byte`（滿足全部 6 個方法），碰 `OBJSTORE_*` 的只有兩支已另行 gate 的付費 E2E。模型服務也不在：`apiserver.Config` 的 `LLM` 為 nil 是**可運作的部署**（搜尋誠實降級為 FTS-only）。

**所以問題收斂成一句**：在一台不能安裝東西的機器上，怎麼得到一個能跑 `db/migrations/` 全部 42 支的 PostgreSQL。

## 評估選項

**每一個選項都以同一個判準檢驗：`db/migrations/` 的 42 支能不能一字不改地套用，而且套用之後那些由資料庫強制的規則（鐵律 4 的 10 個 trigger、鐵律 5 的狀態機 trigger）是不是真的還在守。**

### 選項 A：手寫一個純 Go 的 in-memory 資料層

逐條實作 sqlc 生成的 `Querier` 介面。

- 優點：純 Go、零外部依賴、任何環境都跑得起來。
- 缺點：**這條路有人走完過並且回頭了。** `coder/coder`（Go ＋ sqlc ＋ pgx ＋深度依賴 Postgres 的 schema，與本專案幾乎同構）手寫了 `dbmem` 並使用兩年，於 2024-11 刪除（issue #15109、PR #15291）。刪除理由逐條：**假實作與真 Postgres 的行為分歧**、**部分測試只在 dbmem 上跑過而從未在 Postgres 上驗證**、以及「nobody uses dbmem」。取代方案是 template database 快取＋在 macOS/Windows CI 用 embedded-postgres。
- **對本專案的具體代價**：192 條 named query 要各自有第二份實作，而正確性有一大塊寄存在 10 個 plpgsql trigger、generated 欄位與約束裡——**那些在假實作裡不存在，測試會照樣全綠**。

### 選項 B：SQLite（`modernc.org/sqlite`，純 Go 無 cgo）

- 優點：純 Go、`:memory:` 一行啟動、無 cgo（Windows 上無 C toolchain 痛點）、2026-04 起內建 sqlite-vec。
- 缺點：**實測 0/42**（2026-08-28，見 [report-inmemory-postgres.md](../plans/mvp/m6/report-inmemory-postgres.md) §2）。`0001_init.sql` 第 3 行 `near "EXTENSION": syntax error`，之後全部連鎖。
- **一個會誤導人的數字**：42 支裡有 21 支完全不含 PostgreSQL 專屬語法，看起來一半可移植；**實際上一支都跑不起來**，因為 schema 是一條鏈——前面建不出 `skills`，後面要加欄位就沒有對象。
- 缺的能力逐項：pgvector（`vector(1536)`／`ivfflat`／`<=>`）、generated `tsvector` 欄位與 `websearch_to_tsquery`／`ts_rank_cd`、RANGE 分割表、advisory lock、plpgsql `RAISE EXCEPTION` 帶 SQLSTATE、`jsonb @>`、`FOR UPDATE`、ENUM 型別、`gen_random_uuid()`。換 sqlc engine 另需 8,731 行生成碼重生、559 處 `pgtype.*` 全改，以及 151 處測試 seed SQL 逐處重寫。

### 選項 C：`fergusstrange/embedded-postgres`（下載真 PG 二進位、以子程序執行）

- 優點：**是真 Postgres**，所以能力清單 100% 成立；支援 Windows；Coder 已在 CI 這樣用兩年。
- 缺點：**在本 ADR 的目標環境裡不可行**——它要**下載並執行一個外來二進位**，那正是白名單環境禁止的行為。此外 zonky 的 bundle **不含 pgvector**（issue #163），繞法是自建二進位包並以 `BinariesPath` 指過去，ABI 必須與其 MSVC 工具鏈對上，**沒有保證，且是持續維護成本**。

### 選項 D：每台機器安裝原生 PostgreSQL ＋ pgvector

- 優點：十分鐘、零程式碼、與 CI 同一個引擎。
- 缺點：**需要管理員權限與安裝行為**，在目標環境裡不成立。**在沒有該限制的機器上，這仍然是最快的一步**（見「影響」）。

### 選項 E：PGlite（真 PostgreSQL 編譯為 WASM）

`electric-sql/pglite` ＋ `@electric-sql/pglite-pgvector`。

- 優點：**它不是相容實作，它就是 PostgreSQL**（自報 `PostgreSQL 18.3 (PGlite 0.5.8) on wasm32-unknown-emscripten`）。**實測 42/42 乾淨套用**，且逐項驗證通過：pgvector `<=>` 算得出距離、`ivfflat` 索引真的建起、`trace_events` 是真的 RANGE 分割表（`relkind='p'`，2 個分割）、generated `tsvector` 欄位成立、advisory lock 三種形式全支援，**而且不可變性 trigger 真的擋下 UPDATE 與 DELETE**（`row in public.skill_versions is immutable and cannot be updated (ADR-003)`）。3.7 MB，**原生棲息地是瀏覽器**——零安裝、零網路、無需任何白名單以外的程式。
- 缺點：**單連線**。經 `@electric-sql/pglite-socket` 以 Postgres wire protocol 對外時，pgx 連得上，但 Go 測試套件的 `TestMain` 會**死鎖**——`lockTestSchema` 握著一條連線持 advisory lock、再向只有一條的池子要第二條跑 migration。這不是缺功能（advisory lock 完全支援），是**這段程式假設「握著一條、再開一條」**。此外前一個連線異常結束會讓伺服器進入不可用狀態。<br>**2026-08-28 補測，兩項**：①那個死鎖**不是天花板**——不長期握住連線之後，同一包從整包死鎖變成 16 passed（實驗修改已還原）；②該套件的 multiplexer（`maxConnections > 1`）讓 N 個 client **共用同一個 Postgres session**，實測兩條連線的 `pg_backend_pid()` 同為 42、**同時持有同一把 advisory lock 且無任何錯誤**、且互相看得見 temp table。**本專案有三處產品程式靠 advisory lock 互斥，在該組態下會全綠而毫無保證——所以 `maxConnections > 1` 是禁用組態，不是效能取捨。** 逐項見報告 §5.1、§5.2。

### 選項 F：DoltgreSQL（Go 寫的 PostgreSQL 相容資料庫）

- 優點：Go 原生，1.0 於 2026-08-06 發佈，sqllogictest 99%。
- 缺點：**擴充套件機制自我抵銷**——它的做法是 bridge 到**真 Postgres 編出來的 `.so`**，也就是說仍需一份真 Postgres；`dolthub/doltgresql` **issue #3014（2026-08-01 開啟，至今未關）就是 `CREATE EXTENSION IF NOT EXISTS vector;` 失敗**，正是本 repo 的第一支 migration 第一行。另：**只能起 server，不能 in-process 嵌入**；分割表與 generated `tsvector` 欄位未查到支援聲明。

## 決策

### 決策 1：乾淨測試模式的資料庫是 PGlite（選項 E），不是任何形式的假資料層

**理由不是它最方便，是它是唯一一個「不安裝任何東西」且「規則還在守」的組合。** 選項 A 與 B 都能讓那 327 支變綠，而它們變綠的時候證明的是另一個系統的行為。**這個 repo 已經三度被同一種缺陷咬過（invalidate 用錯 query key、六條 placeholder 規則零正面測試、兩條速率限制包裝被無聲刪除），不在 M6 親手再造第四個。**

判準寫成一句話留給後人：**任何取代 PostgreSQL 的候選，必須能讓 `UPDATE skill_versions` 失敗。** 那是鐵律 4 由資料庫強制的形式，也是最便宜的真偽測試。

### 決策 2：Adapter 畫在 `apiFetch`，不畫在資料庫層

`gen.DBTX` 只有三個方法（`Exec`／`Query`／`QueryRow`），看起來是天然的接縫。**它不是**——25 處非測試程式碼拿的是具體的 `*pgxpool.Pool`（要 `Begin()`，River 亦然），而真正的衝突是**連線數不是 SQL**。在資料庫層包一個 Adapter，等於與連線池打架，而且贏不了：session 範圍的東西（交易、advisory lock）無法多工到一條實體連線上。

`apps/web/src/api/client.ts` 的 `apiFetch` 是**單一出入口**，全部 API 呼叫都經過它。

```
Port:      apiFetch(path, init) → T
Adapter A: HTTP → 真後端（今天）
Adapter B: PGlite → 同一份 SQL，瀏覽器內（本 ADR 新增）
```

### 決策 3：SQL 只有一份，兩個消費端

```
db/queries/*.sql ──sqlc──┬─→ gen/*.sql.go        （Go 後端執行）
                         └─→ 抽出 const 字串 → sql.ts（瀏覽器執行）
```

`db/queries/*.sql` 帶 `sqlc.arg`／`sqlc.narg` 註解、**不是合法 SQL**；而生成的 Go 檔內是**最終 SQL 含 `$1, $2` 佔位符**。抽取步驟從生成碼取字串，**因此不存在第二份查詢**。抽取掛在既有的 `task gen:check` 上，漂移由機器抓。

**這一條是決策 1 的延伸**：不做第二份實作的規矩，同樣適用於查詢本身。

### 決策 4：Adapter 必須宣告自己是什麼，且宣告出現在畫面上

照 `isolation.level` 的既有紀律：沙箱 Provider 宣告 `container` 而不謊稱 `gvisor`，由 `Match` 決定它能不能用（[ADR-015](./ADR-015-sandbox-isolation-technology.md)、`schedule.go`）。資料層的 Adapter 一樣：

| Adapter B 有 | Adapter B 沒有 |
| --- | --- |
| 真 schema、真 trigger、真 FTS、真向量排序 | **Go 那一側的商業邏輯**：授權、政策、狀態機、評估判定 |
| 讀取路徑（搜尋、詳情、既有 Run 的 trace 與評估） | 需要真正執行的東西（開一個 Run、打包下載） |
| 單連線（瀏覽器內剛好合適） | 併發語意 |

**右欄要寫在畫面上，不是寫在註解裡。** 理由與 `02:GEN-004` 對生成物的要求同源：一個看起來像產品的東西，如果不說自己是什麼，就會被當成產品——而這一次的觀眾是金融機構。

### 決策 5：Go 測試套件在受限環境內跑，不在本 ADR 的承諾範圍

它需要兩件本 ADR 沒有解決的事：①Node 在該環境的白名單上（**事實未知，需向 IT 取得**）；②解掉「握一條、開一條」的假設，而其中有些測試（advisory lock 序列化、並行首次登入）**本質上需要多於一條連線**。

**在無此限制的機器上，選項 D（安裝原生 PostgreSQL ＋ pgvector）仍是最快的一步**，十分鐘、零程式碼、解鎖 281 支。兩者不互斥。

## 影響

### 正面

- 金融環境的零安裝展示成立，而且展示的是**真資料庫執行真查詢**——搜尋排名是生產那條 query 算出來的，不是預先排好的順序。對一個以「證據」為主張的產品，這個差別是底線而非加分。
- 讀取面的「乾淨測試模式」有了可執行的形式，不必等任何安裝或網路白名單。
- **一個副產品**：`db/queries` 的 SQL 第一次有了瀏覽器端的消費者，這讓「同一份查詢在兩個執行環境行為一致」成為可被測試的性質。

### 成本與限制

- **PGlite 是單連線**，且異常結束的連線會使伺服器不可用。瀏覽器內無此問題（一頁一連線），socket 模式下有。
- **Adapter B 沒有 Go 的商業邏輯**，所以它能展示的是讀取路徑與既有證據，不是完整產品。決策 4 的宣告是這件事的機械形式。
- **`@electric-sql/pglite-pgvector` 是 0.0.9（2026-08-26 發佈）**——極新。升版風險要釘住版本並納入 `tools/toolchain.yaml` 的對帳。
- 抽取步驟增加一個生成產物，**而生成產物禁止手改**（AGENTS.md 紅線 5）；它必須進 `gen:check`。

## 待決策

**（2026-08-28 新增，且它動的是決策 2 的層次）** 負責人定調：淨測試模式**不取代**原有能力，是一個**平行、不干擾、以旗標切換**的模式，原模式隨時可照常運行；因此 M6 在**資料庫存取、檔案存取、沙箱**三條軸上各是一個 Strategy。

**這與本 ADR 決策 2 不是同一個形狀。** 決策 2 把資料庫的 Adapter 畫在 `apiFetch`——瀏覽器那一層；而檔案（`apiserver.ObjectStore`）與沙箱（`sandbox.Driver`）的接縫在 **Go 行程內**。一個旗標切三條軸，與「瀏覽器繞過整個後端」不能同時成立。

要裁定的是哪一種：

1. **瀏覽器單機模式**（本 ADR 現況）：沒有 Go 後端，另外兩條軸的 Strategy 在該模式下不會被用到——**包含 [ADR-059](./ADR-059-the-clean-mode-execution-driver-is-honest-about-not-being-a-sandbox.md) 的本機 Driver**。
2. **三軸 Strategy**：Go 後端在受限機器上真的跑起來。**那資料庫需要一個 Go 用得了的實作，而目前沒有候選通過**——PGlite 單連線（實測死鎖）、multiplexer 讓 advisory lock 安靜失效（實測）、原生二進位過不了白名單。<br>**一個未量到的可能**：`pool_max_conns=1` 且不開 multiplexer 是誠實的序列化。**但 River 的 notifier 會以 LISTEN 長期握住一條連線**，而池子只有一條——**沒有實測過，而它決定這條路成不成立。**

**不決定的代價**：`02:PORT-010b`（本機執行 Driver）動工的前提是形狀 2，而形狀 2 還沒被簽——**先寫再說的話，選項 1 若中選，那份程式碼一行都用不到。**

### 續（2026-08-28 稍晚）：形狀 2 被資料庫擋住，而且擋住它的比 River 更前面

把「三軸 Strategy」實際攤開之後，卡點不需要實測 River 就能定出來——**它是行程數對連線數**：

- `cmd/api` 與 `cmd/worker` 是**兩個獨立的二進位，各自 `pgxpool.New`**（`api/main.go:45`、`worker/main.go:47`）。
- **要真的跑完一次 Run 就需要 worker**：API 只建立 Run，派送與清理在 worker（River 在那裡，不在 API）。
- 而 **PGlite 的 socket 一次只收一個 client**（實測：第二條連線被直接踢掉），**開 multiplexer 則讓 advisory lock 安靜失效**（實測：兩條連線同時拿到同一把鎖）。

**兩個行程對一條連線，怎麼配都不成立**：不開 multiplexer 就有一個行程連不上，開了就沒有互斥。River 的 LISTEN 只是同一件事的第二個實例，**不是它自己的問題**。

**所以三條軸裡，沙箱那條有答案（ADR-059），資料庫那條沒有。** 擋住「在受限機器上真的跑完一次 Run」的不是隔離，是**沒有一個那台機器跑得動、又能同時接受兩個行程的 PostgreSQL**。

**還沒被排除的一種形狀**：把 api 與 worker 合成單一行程只在淨測試模式用。**但那是第三個二進位**，而且 River 的 notifier 仍會以 LISTEN 長期佔住那唯一的連線，HTTP 那側就沒得用了——**這一項仍未實測**，寫在這裡是因為它是選項 2 唯一剩下的路，不是因為它看起來會成。

### 訂正（2026-08-28 稍晚，同日）：形狀 2 沒有被擋住，而擋住它的那個理由是我沒去試

上一段把「api 與 worker 合成單一行程」寫成「唯一剩下的路，不是因為它看起來會成」，理由是 River 的 notifier 會以 LISTEN 佔住那唯一的連線。**那句話是推論，而推論錯了。**

**River 有 `PollOnly`，而且它的文件就是為這個情況寫的**（`client.go:309`）：

> PollOnly starts the client in "poll only" mode, which avoids issuing `LISTEN` statements… The upside is that it makes River compatible with systems where listen/notify isn't available. **For example, PgBouncer in transaction pooling mode.**

**實測**（單一行程、`pool_max_conns=1`、PGlite 的 multiplexer **關閉**、`PollOnly: true`）：

```
pool max conns = 1
river schema applied
river started in PollOnly mode on a one-connection pool
job inserted
JOB WORKED n=42, and 2 plain queries were served meanwhile
```

**River 的 schema 套用成功、client 起得來、工作被領走並執行完、而且「HTTP 那一側」在同一條連線上同時查得到資料。** 沒有餓死，沒有死鎖。

**突變驗證**（把 `PollOnly` 改成 `false`，其餘不動）：

```
start: failed to connect to `user=postgres database=skillhub_test`: … An established
connection was aborted by the software in your host machine.
```

**紅得正是預期的原因**——River 去要第二條連線做 LISTEN，而 PGlite 只收一個 client。兩邊對上了。

**所以要更正的結論是**：資料庫**沒有**擋住形狀 2。擋住的是「兩個二進位」這個部署形狀，而那是可以改的——**淨測試模式把 api 與 worker 收在同一個行程裡，River 走 poll-only，一條連線就夠**。這與 multiplexer 那條路不同：**這裡真的只有一個 session，所以互斥是自然成立而不是被偽造的**。

**仍然沒有量到的，逐條**：完整的 API server 沒有起來過（本次只跑了 queue 這一層）；派送到沙箱那一段沒有走過；沒有做長時間或多使用者的壓力；`pool_max_conns=1` 之下真實請求量的延遲未知。**這一段證明的是「那個阻礙不存在」，不是「這條路已經可行」。**

### 續（2026-08-29）：多連線這個限制，`pgmock` 身上不存在

上面把「兩個行程對一條連線」寫成資料庫這一軸的結構性限制。**那是對 PGlite 成立，不是對所有候選成立。**

`pgmock`（v86 模擬 x86，裡面跑未修改的 postmaster）實測：**兩條獨立的 pgx 連線拿到不同的 backend（1600／1603）、`pg_try_advisory_lock` 一真一假、temp table 互不可見**；42 支 migration **37 過，5 支全部只敗在 pgvector，零個版本語法失敗**；**判準一過**（UPDATE 與 DELETE 都被擋，訊息帶 ADR-003）。逐項見 [m6/report-inmemory-postgres.md](../plans/mvp/m6/report-inmemory-postgres.md) §10。

**同時要更正一筆自己的讀數**：本 repo 先前記過「`pgmock` 在本機 10 分鐘沒有開起來」，並據此把這一類技術歸為量級不可用。**重跑是 1.017 秒**，第一次為什麼卡住未查明。

**所以候選集比本 ADR 評估時多了一個，而它的形狀不同**：

| | PGlite | pgmock |
| --- | --- | --- |
| 多 session | ❌（multiplexer 會偽造互斥） | ✅ 真的 fork |
| 42 支 migration | 42/42 | 37/42（缺口只有 pgvector） |
| 判準一 | ✅ | ✅ |
| 版本 | PostgreSQL 18.3 | PostgreSQL **14.5**，i686 |
| 開機／套用 | 快 | 1 秒開機，但 migration 229 秒（可存快照只付一次） |
| 成熟度 | 活躍 | npm 最後發版 2024-05，18 個 open issue |

**這不改變本 ADR 的決策，但它改變「不決定的代價」**：形狀 2（三軸 Strategy、Go 後端真的跑起來）現在有**兩條**可能的路——單行程＋PollOnly＋PGlite（已實測可行），或多行程＋pgmock（多 session 已實測，缺 pgvector）。**兩條都還沒走完，而選哪一條決定 `PORT-010b` 要不要寫。**



- **M6 是否計入 MVP 完成度。** M5 的先例是不計（`01` §7.3）。M6 是開發者體驗與測試可信度，不是產品功能——**但它改變「綠燈是什麼意思」，而那是允收的一部分**。→ `05` 待裁定。
- **受限環境的白名單內容**（Node 在不在上面）。這是一個要向 IT 取得的事實，不是一個可以設計的東西；三種答案會導出三份不同的計畫。
- **Adapter B 的端點範圍**：只涵蓋展示路徑，或涵蓋全部讀取端點。範圍越大，決策 4 的宣告越容易被忽略。
