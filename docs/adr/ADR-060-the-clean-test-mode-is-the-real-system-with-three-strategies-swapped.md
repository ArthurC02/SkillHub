# ADR-060：乾淨測試模式是「同一個系統換掉三個實作」，不是另一個系統

- 狀態：Proposed
- 日期：2026-08-29
- 決策者：產品負責人、架構規劃
- 取代：[ADR-058](./ADR-058-the-clean-test-mode-is-real-postgres-behind-the-api-seam.md)（其決策 1／4／5 由本 ADR 延續，決策 2／3 被推翻）
- 相關：[ADR-059](./ADR-059-the-clean-mode-execution-driver-is-honest-about-not-being-a-sandbox.md)（沙箱那一軸）、[ADR-006](./ADR-006-provider-port-and-runtime-adapters.md)（Provider Port）、[ADR-016](./ADR-016-language-split-and-cross-language-contracts.md)（語言分工）、[ADR-018](./ADR-018-data-platform-and-storage.md)（PostgreSQL 中心）、[ADR-003](./ADR-003-immutable-versions-and-snapshots.md)（鐵律 4 的來源）

## 背景

[ADR-058](./ADR-058-the-clean-test-mode-is-real-postgres-behind-the-api-seam.md) 把乾淨測試模式設計成**瀏覽器裡的一個 Adapter**：資料層換成 PGlite、SQL 從生成碼抽出來在瀏覽器內執行、`apiFetch` 是唯一接縫。那個形狀的隱含前提是**沒有 Go 後端**——因為當時量到 PGlite 是單連線，而後端拿的是具體的連線池。

**負責人於 2026-08-28 定調了一個不同的形狀**：淨測試模式**不取代**原有系統的任何能力，它是一個**平行、不干擾、以旗標切換**的模式，**原模式隨時可以照常運行**；因此 M6 在**資料庫存取、檔案存取、沙箱**三條軸上各是一個 Strategy。

**那個定調與 ADR-058 決策 2 不能同時成立**：「一個旗標切三條 in-process Strategy」與「瀏覽器繞過整個後端」是兩個系統，不是一個系統的兩種設定。

而擋住前者的技術理由，在定調之後被實測推翻了兩次（見下）。

## 決策

### 決策 1：乾淨測試模式是同一套產品程式，以旗標切換三個實作

**不新增第二套系統、不新增第二條派送路徑、不新增第二份 SQL。** 旗標未設時，行為與今天**逐位元相同**——這是「平行、不干擾」的可檢驗形式。

| 軸 | Port | 生產實作 | 淨測試實作 |
| --- | --- | --- | --- |
| 資料庫存取 | 連線字串（同一個 `*pgxpool.Pool`） | 原生 PostgreSQL | **PGlite over socket，`pool_max_conns=1`**（承載在 `tools/pglite`，PGlite **0.4.6 = PostgreSQL 17.5**，與 CI 的 pg17 同 major——**2026-08-29 訂正，前期量測用的 0.5.8 是 18.3 而本 ADR 的允收要求對得上 CI**） |
| 檔案存取 | `apiserver.ObjectStore`（**介面已存在**） | S3 相容（SeaweedFS） | **in-process 承載**（`02:PORT-009`） |
| 沙箱 | `sandbox.Driver`（**介面已存在**） | `dockerdrv`（gVisor） | **本機行程 Driver**（[ADR-059](./ADR-059-the-clean-mode-execution-driver-is-honest-about-not-being-a-sandbox.md)、`02:PORT-010`） |

**三條軸裡有兩條的介面今天就在**，缺的是第二個實作與**選擇點**——`objstore.FromEnv()` 目前永遠回傳真的 S3 client，沒有分支。**第三條軸不需要介面**：它換的是連線字串，不是型別。

### 決策 2：資料庫那一軸走 PGlite，而讓它成立的是「單行程 ＋ River poll-only」

**推翻的是部署形狀，不是資料庫。** ADR-058 量到的死鎖與本 repo 記過的「兩個行程對一條連線」都成立——**但那是對「api 與 worker 是兩個二進位」這個形狀成立**。

**實測**（單一行程、`pool_max_conns=1`、PGlite 的 multiplexer **關閉**、River `PollOnly: true`）：

```
pool max conns = 1
river schema applied
river started in PollOnly mode on a one-connection pool
job inserted
JOB WORKED n=42, and 2 plain queries were served meanwhile
```

**突變**（只把 `PollOnly` 改成 `false`）：River 去要第二條連線做 LISTEN，PGlite 拒絕，`Start` 失敗——**紅得正是預期的原因**。

`PollOnly` 是 River 的官方能力，文件寫明它是為「listen/notify 不可用的系統」設計的。

**因此淨測試模式把 api 與 worker 收在同一個行程裡。** 這與 PGlite 的 multiplexer 有本質差別：**那裡是 N 個 client 假裝成 N 個 session，互斥被偽造；這裡真的只有一個 session，互斥因為是真的才成立。**

**`maxConnections > 1` 是禁用組態**（ADR-059 決策 3 已將閘門改為白名單，同一個理由）。

### 決策 3：不採 `pgmock`，但把它記成有效的備援

`pgmock`（v86 模擬 x86，內跑未修改的 postmaster）**實測給得出 PGlite 給不出的東西**：兩條獨立 pgx 連線拿到不同 backend（1600／1603）、`pg_try_advisory_lock` 一真一假、temp table 互不可見。**42 支 migration 37 過，判準一過**（UPDATE／DELETE 都被擋且訊息帶 ADR-003）。

**不採用的理由不是它做不到，是它剩下的缺口大小未知**：隨附映像是 PostgreSQL 14.5、i686、**沒有 pgvector**，而 guest 裡沒有編譯器。補上它要在別的機器為 i686-musl 編一顆 `vector.so`，而 **pgvector 在 32-bit 有 runtime 失敗前科**，且 `ivfflat` 是 C 寫的 index access method，**沒有 SQL shim 的可能**。npm 最後發版 2024-05，18 個 open issue。

**對照之下，決策 2 剩下的是「要寫多少行」那種問題，不是「風險多大」那種。**

**它被記在這裡而不是被刪掉**，是因為若決策 2 的單行程形狀在實作中垮掉，`pgmock` 是唯一已知還站著的替代品。

**2026-08-29 補記，把它的觸發條件收窄成一個**：先前這一條有兩個可能的理由——單行程形狀垮掉，或展示需要真正的併發語意。**後者由負責人裁定不成立（展示現場不會有兩個人同時操作），因此撤除。** 同日的量測也指向同一個方向：一條連線上十個併發請求各約 2 ms，五十個時的拒絕來自速率限制器而非資料庫。**`pgmock` 從此只為「單行程形狀垮掉」而留著。**

### 決策 4：ADR-058 的決策 2 與 3 隨之作廢，並帶走三項允收

淨測試模式下**瀏覽器對話的是真的本機後端**，所以：

- **不需要把 SQL 抽出來給瀏覽器執行**（ADR-058 決策 3 作廢，`02:PORT-002` 撤回）。**這也移除了「兩個消費端會漂移」這整類風險。**
- **不需要端點範圍表**（`02:PORT-006` 撤回）——跑的是真的後端，端點就是端點。
- **交付形式不是 `file://` 的單一 HTML**（`02:PORT-005` 改寫）——本模式**依賴 localhost 上的一個服務**，那正是被換掉實作的那個系統本身。**`file://` 那批實測結論不作廢**（Chromium 擋 module import、WASM 從 bytes 可跑），它們移交給任何未來的離線交付提案。

**ADR-058 決策 1（PGlite 而非假資料層，含判準一）、決策 4（Adapter 必須在畫面上宣告自己是什麼）、決策 5（受限環境內的 Go 測試不在承諾範圍）由本 ADR 延續，逐字不變。**

### 決策 5：三條軸都必須誠實宣告，而宣告的形式各自已經存在

- **沙箱**：`isolation.level = "clean"`，由白名單閘門強制（ADR-059）。
- **檔案存取**：承載必須明文記載它不驗證 presigned URL，且不得作為 `SBX-008` 的證據（`02:PORT-009`，已實測 gofakes3 四種破壞全回 200）。
- **資料庫**：**它是真的 PostgreSQL，所以沒有什麼要否認的**——但**單連線是要說出來的性質**：併發語意與生產不同，而那會影響任何以「在淨測試模式下沒重現」為由關掉的缺陷。

**畫面上的揭露義務**（ADR-058 決策 4）**適用於整個模式，不只適用於資料層**。

### 決策 6：旗標只有一個，切的是「模式」，選擇點只有一個（2026-08-29 負責人裁定）

**裁定原文的形狀**：可設定性是系統品質之一，**但不是每樣東西都要可設定**——只有**模式切換這種概念層次**才需要。不希望整個系統被挖洞挖得坑坑疤疤，**只在關鍵處開孔**。

因此：

1. **合併入口是既有二進位加一個旗標，不是第三個二進位。**（待決策 1 的答案。）承載那個旗標的是 `cmd/api`——淨測試模式下它額外在同一個行程裡起 worker。理由是這個模式的使用者面對的是一個瀏覽器，而瀏覽器對話的那一顆就是它。
2. **旗標只有 `SKILLHUB_CLEAN_MODE` 一個，而且它是模式不是能力。** **不得**為「用行程內物件儲存」「用本機 Driver」「用單連線」各開一個環境變數。三條軸一起換或一起不換，**因為它們描述的是同一件事：這台機器裝不了東西**。分開開關會讓「一半淨測試、一半生產」成為一個可達的組態，而那個組態沒有人設計過、也沒有人會測。
3. **選擇點只有一個，在 composition root。**（待決策 3 的答案。）**不是** `objstore.FromEnv()` 內部分支——那是在一個 Generic 套件裡挖一個洞，而那個套件的四個呼叫端有三個與本模式無關。三條軸的實作在 `cmd/api` 的 main 裡被選定一次，往下注入。
4. **旗標未設時，那一整段分支不執行，行為與今天逐位元相同**（`02:PORT-005`，且要有測試在守）。

**這條決策的代價要寫下來，因為它動到一句既有的架構主張**：[ADR-032](./ADR-032-ddd-bounded-context-governance-for-platform.md) §5 明文記著 `cmd/api` 與 `cmd/worker` 是兩個 deployment unit、**共享零個物件**。淨測試模式下它們共享一個行程與一個連線池。<br>
**收斂的方式不是推翻那句話，是限制它的例外範圍**：合併只在旗標設定時發生，生產路徑的兩個二進位一個字不改；而 worker 的 composition root（`buildWorkers`，無 I/O、無環境變數讀取）**搬到 `internal/entrypoint/worker`**，與既有的 `internal/entrypoint/api/apiserver` 對稱。**搬移是純搬移**，而 `cmd/worker/main_test.go` 那四支不需要資料庫的 wiring smoke test 跟著搬——它們正是當年漏設 `run.Service.Queue` 導致每個 Run 都沒清理、而沒有任何測試會紅的那次事故留下的防線。

## 評估選項

| 選項 | 內容 | 為什麼不採 |
| --- | --- | --- |
| **A. 同一系統換三個實作（採用）** | 旗標切換，原模式不受影響 | — |
| B. 瀏覽器單機 Adapter（ADR-058 原案） | 沒有 Go 後端 | **與負責人定調的「平行、不干擾、旗標切換」不是同一件事**；且它讓 ADR-059 的本機 Driver 完全用不到 |
| C. 兩個行程 ＋ `pgmock` | 保持 api／worker 分離 | 缺口是 pgvector，**而修補它的風險大小未知**（決策 3） |
| D. 兩個行程 ＋ PGlite multiplexer | 最小改動 | **實測：兩條連線同時拿到同一把 advisory lock。** 互斥被偽造，這是本 repo 反覆記載的那種缺陷 |
| E. 不做 | — | 那台機器上今天一行都跑不起來 |

## 影響

- **淨測試模式的產出是「真的走完一次」**：真的 Run、真的狀態機、真的評估寫入——**只有隔離、儲存與併發不是真的**。
- **需要一個新的組合入口**（api ＋ worker 在同一行程）。**那是第三個二進位或一個既有二進位的旗標**，尚未決定哪一種。
- **`02` §4.10 要改**：撤回 `PORT-002`、`PORT-006`，改寫 `PORT-005`，其餘保留。
- **併發語意與生產不同**，且這是**永久**的性質不是暫時的缺陷。任何「在淨測試模式下沒重現」的結論都要先排除這一項。
- **Node 必須在那台機器的白名單上**，否則本 ADR 的決策 2 不成立。**這個事實至今未取得**——見 [m6/environment-probe.md](../plans/mvp/m6/environment-probe.md)。

## 待決策

1. ~~**合併入口是第三個二進位還是既有二進位的旗標。**~~ **2026-08-29 裁定：既有二進位加旗標**，見決策 6。
2. **淨測試模式的 Run 要不要進正式的 Run 歷史。** 它們是真的 Run，但跑在沒有隔離的地方。
3. ~~**`objstore` 的選擇點放哪裡。**~~ **2026-08-29 隨決策 6 一併回答：在 composition root，不在 `FromEnv()` 內部**——後者是在一個 Generic 套件裡挖洞，而它的四個呼叫端有三個與本模式無關。
