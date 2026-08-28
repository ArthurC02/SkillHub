# M6：在不能安裝東西的機器上跑起來

- 狀態：**進行中（十一項：完成兩項、撤回兩項、剩七項）**——`PORT-004`（讓跳過出聲）已於 2026-08-28 完成並在 CI 生效；**它是九項裡唯一不依賴受限環境任何答案的一項**。其餘八項仍未開工。本目錄先於開工存在，理由與 M5 相同：它已經有一份量測（[report-inmemory-postgres.md](report-inmemory-postgres.md)），而那份量測決定了 [ADR-058](../../../adr/ADR-058-the-clean-test-mode-is-real-postgres-behind-the-api-seam.md) 在兩個候選之間怎麼選
- 決策：**[ADR-060](../../../adr/ADR-060-the-clean-test-mode-is-the-real-system-with-three-strategies-swapped.md)（Proposed）**——淨測試模式是**同一套產品程式以旗標切換三個實作**；[ADR-058](../../../adr/ADR-058-the-clean-test-mode-is-real-postgres-behind-the-api-seam.md) 已被它取代（決策 1／4／5 延續，量測全部仍有效）、[ADR-059](../../../adr/ADR-059-the-clean-mode-execution-driver-is-honest-about-not-being-a-sandbox.md)（沙箱那一軸）
- 規格：[`02` §4.10](../../02-specifications-and-acceptance-criteria.md)（`PORT-001`～`PORT-010`）
- 工作項：[`03` §20](../../03-work-items.md)
- **不計入 MVP 完成度**（同 M5 的先例，`01` §7.3）——**但這一條有一個待裁定的例外**，見下方 §待裁定

## 為什麼有這個里程碑

**它不是一個願望，是一個部署環境的事實。**

MVP 的 Pitch 在**金融機構環境**進行。那台機器：

- **只能執行白名單內的程式**——未簽章的執行檔會被 AppLocker／WDAC 擋下
- **不能安裝軟體**——沒有管理員權限
- **對外網路只保證得到模型供應商**，其餘看白名單，而負責人明示**不想賭**

同一個環境也是部分同仁的日常開發環境。

在那台機器上，今天的系統**一行都跑不起來**：`task dev` 要 Docker、資料庫測試要 PostgreSQL、沙箱要容器 runtime。而「Pitch 過關才有機會做正式環境」這個順序意味著——**M6 不是為了上線做準備，它是上線的前置條件的前置條件**。

## 這個里程碑有兩半，難度差一個數量級

### A. 乾淨測試模式（有解，且比預期小）

> **A 半有兩個承載，形狀不同**：資料庫（[報告](report-inmemory-postgres.md)）與物件儲存（[報告](report-object-storage.md)）。**後者不是「測試跑不起來」的原因**——那 287 筆不需要物件儲存，它們用的是行程內的 `map[string][]byte`；物件儲存是**系統跑起來**的需求。兩件事不要混為一談。

那 287 筆靜默跳過的測試，[逐條查證的結果](report-inmemory-postgres.md)是：**外部依賴只有 PostgreSQL 一項**。物件儲存已經是行程內的 `map[string][]byte`、模型服務為 nil 是合法部署、`apiFetch` 是單一出入口。

而「在不能安裝的機器上得到一個真 PostgreSQL」有答案：**PGlite——真 PostgreSQL 18.3 編譯成 WASM，3.7 MB，42/42 乾淨套用，而且不可變性 trigger 真的會擋人**。

### B. 受限環境內的 Go 測試（沒有承諾）

它需要兩件本里程碑沒有解決的事：

1. **Node 在該環境的白名單上**——這是一個要向 IT 取得的**事實**，不是一個可以設計的東西。三種答案（Go 在白名單上／只有瀏覽器與編輯器／有內部核准的容器平台）會導出三份完全不同的計畫。
2. **解掉「握一條連線、再開一條」的假設**——實測顯示 `lockTestSchema` 在單連線下死鎖，而其中有些測試（advisory lock 序列化、並行首次登入）**本質上就需要多於一條連線**。

**B 半刻意不給時程。** 在那個事實到手之前，任何時程都是猜的——而這份 repo 在 2026-08-28 這一天已經因為未確認的環境前提改過兩次計畫。

## ⛔ 邊界：Adapter B 不是產品，而且它要自己說出來

[ADR-058](../../../adr/ADR-058-the-clean-test-mode-is-real-postgres-behind-the-api-seam.md) 決策 4。**這一條的形式與 `02:GEN-004` 對生成物的要求同源**，理由也一樣：一個看起來像產品的東西，如果不說自己是什麼，就會被當成產品——而這一次的觀眾是金融機構。

| Adapter B 有 | Adapter B 沒有 |
| --- | --- |
| 真 schema、真 trigger、真 FTS、真向量排序 | **Go 那一側的商業邏輯**：授權、政策、狀態機、評估判定 |
| 讀取路徑（搜尋、詳情、既有 Run 的 trace 與評估） | 需要真正執行的東西（開一個 Run、打包下載） |
| 單連線（瀏覽器內剛好合適） | 併發語意 |

**右欄必須出現在畫面上**，不是寫在程式註解裡。沙箱的先例是：假 Provider 宣告 `container` 而不謊稱 `gvisor`，由 `Match` 決定它能不能用——**那個保證由既有程式碼提供，不靠任何人記得**。資料層的 Adapter 應該照抄那個紀律。

> **⛔ 第二條邊界（2026-08-28 補入）：物件儲存的替身不是 `SBX-008` 的證據。**
>
> in-process 的假 S3 讓七個方法十項全過，**而突變測試顯示它什麼都不驗**——簽章被竄改、簽章整個拿掉、已過期三秒、拿「讀」的票去「寫」，**四種都回 HTTP 200，最後一種還真的覆寫了物件**。
>
> **今天的基線是零覆蓋**（平台測試裡沒有任何一行 fetch 過 presigned URL）。**引入一個不驗簽的替身，會把「零覆蓋」變成「看起來有覆蓋」——那比零覆蓋更糟。** 短效授權的證明只能來自一次對真 S3 的執行，且該測試要被明文標記為不可 skip（`02:PORT-009`）。
>
> **這一條是自己抓到的**：本里程碑的物件儲存報告第一版差點以相反的結論收尾，抓到它的方式是對自己的實驗做突變。

## 這個模式是平行的，不是取代（2026-08-28 負責人定調）

**淨測試模式不取代原有系統的任何能力。** 它是一個**平行、不干擾**的模式，以旗標切換，**原模式隨時可以照常運行**。因此 M6 在三條軸上各是一個 Strategy：**資料庫存取、檔案存取、沙箱**。

**這個定調照出一件事：三條軸今天不在同一層，而且成熟度差很多。**

| 軸 | 接縫在哪 | 是不是 Strategy | 缺什麼 |
| --- | --- | --- | --- |
| **檔案存取** | `apiserver.ObjectStore` 介面（由四個 context 的切片組成） | ✅ **是**——整合測試已經在替換它 | **選擇點**。`cmd/api` 呼叫的 `objstore.FromEnv()` **永遠回傳真的 S3 client，沒有分支**；今天只有測試會注入別的 |
| **沙箱** | `sandbox.Driver`（11 個方法） | ✅ **是** | **第二個實作**（`PORT-010b`）與選擇點。`sandboxd` 目前寫死 `dockerdrv` |
| **資料庫存取** | **沒有接縫** | ❌ **不是**——`Config.Pool` 是具體的 `*pgxpool.Pool`，不是介面 | 整條軸 |

**兩條軸是「介面已經在了，缺第二個實作和一個旗標」；第三條軸連介面都沒有。**

### 而資料庫那一條與 ADR-058 的形狀不一致，這要裁定

[ADR-058](../../../adr/ADR-058-the-clean-test-mode-is-real-postgres-behind-the-api-seam.md) 決策 2 把資料庫的 Adapter 畫在 **`apiFetch`**——也就是**瀏覽器**那一層，而不是 Go 行程內。理由是實測的：衝突不在 SQL 而在**連線數**，PGlite 是單連線。

**但那代表資料庫那一條軸與另外兩條不在同一個地方**：檔案與沙箱是 Go 行程內的 Strategy，資料庫是「瀏覽器繞過整個後端」。**一個旗標切三條軸**的形狀，與 ADR-058 目前的決策**不是同一件事**。

**兩者要對齊，而對齊的方向沒有被裁定過**：

- **走 ADR-058 的形狀**：淨測試模式是**瀏覽器單機模式**，沒有 Go 後端。那另外兩條軸的 Strategy 在該模式下**根本不會被用到**——包含 `PORT-010b` 那個本機 Driver。
- **走三軸 Strategy 的形狀**：Go 後端在那台機器上真的跑起來，三條軸各換一個實作。**那資料庫要有一個 Go 用得了的實作，而目前沒有候選通過**：PGlite 單連線（實測死鎖），multiplexer 會讓 advisory lock 安靜失效（實測），原生二進位過不了白名單。<br>**一個未量到的可能**：`pool_max_conns=1` 且**不開** multiplexer 是誠實的序列化（只有一件事在跑，互斥自然成立）。**但 River 的 notifier 會長期握住一條連線做 LISTEN**，而池子只有一條——**這一項沒有實測過，它決定第二條路成不成立。**

→ 已記為 [ADR-058 待決策](../../../adr/ADR-058-the-clean-test-mode-is-real-postgres-behind-the-api-seam.md)。**在它有答案之前，`PORT-010b` 動工的前提是「走三軸形狀」，而那還沒被簽。**

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



## 三個必須守住的判準

**判準一：任何取代 PostgreSQL 的候選，必須能讓 `UPDATE skill_versions` 失敗。**

那是鐵律 4 由資料庫強制的形式，也是最便宜的真偽測試。手寫的假資料層與 SQLite 都過不了這一關——而它們會**照樣全綠**，因為要被擋的那道門已經不在了。**這個 repo 已經三度被同一種缺陷咬過**（invalidate 用錯 query key、六條 placeholder 規則零正面測試、兩條速率限制包裝被無聲刪除），M6 不再造第四個。

**判準二：SQL 只有一份。**

```
db/queries/*.sql ──sqlc──┬─→ gen/*.sql.go        （Go 後端執行）
                         └─→ 抽出 const 字串 → sql.ts（瀏覽器執行）
```

`db/queries/*.sql` 帶 `sqlc.arg`／`sqlc.narg`、**不是合法 SQL**；生成的 Go 檔內是**最終 SQL 含 `$1, $2`**。抽取從生成碼取字串，**所以不存在第二份查詢**，且抽取掛在既有的 `task gen:check` 上，漂移由機器抓。

**判準三：跳過要出聲。**

**（2026-08-28：本判準已由 `PORT-004` 實作，見 `03` §20。）** 今天 287 筆靜默消失、畫面全綠。**但要小心界線**：現在的跳過本身是誠實的（訊息明說環境變數未設）；**任何讓畫面更綠、卻沒有真的執行斷言的改動，都比現狀更糟**。要的是讓「我這台跑不了什麼」變成可見的事實，不是讓它變綠。

## 啟動條件

| # | 條件 | 現況 |
| --- | --- | --- |
| 1 | ADR-058 由 `Proposed` 轉 `Accepted` | **未定**。它動的是測試可信度的定義，不只是工具選擇 |
| 2 | 受限環境的白名單內容已知（Node 在不在上面） | **未取得，而且它現在擋住的比原本記的多**。資料庫那一軸的**兩條候選路都跑在 Node 裡**（PGlite 與 pgmock 都是 npm 套件），所以 Q1 是「否」的話 A 半也要重新設計。**已備妥可直接交出去的清單：[environment-probe.md](environment-probe.md)**——五個問題、十分鐘、全部唯讀、不安裝任何東西、不做任何繞過 |
| 3 | Pitch 的日期與形式已定 | **未知**。A 半的優先序完全取決於它——如果 Pitch 還有很久，`01` §11 那些從未被問過的問題比 M6 更急 |

**條件 3 是這個里程碑最該被質疑的地方，所以寫在這裡而不是藏在別處**：`01` §11 的成功指標到今天**一個數字都沒有**，因為沒有跟任何一個真人說過話。而 Pitch 一定會被問「有人要嗎」。[ask-5](../m5/ask-5.md)（5 人 × 8 分鐘）與 M1 閘門（材料 2026-08-16 已備妥，只等 D 日）加起來不到兩天，**而它們是唯一能把「機器說它可以」翻譯成「人說他要」的東西**。M6 讓 demo 跑得起來，但它不產生任何一個那樣的數字。

## 不在 M6 範圍內的

- **正式環境的任何東西**——Hetzner 節點、cloud-init、SEC-009 的 45 項門檻。那些是 Pitch 過關之後第一週要打開的資料夾（相關調查已完成，見 `05` 與 `04` 的對應列）。
- **在受限機器上執行不受信任的 Skill**（`02:PORT-006`）。**2026-08-28 訂正了這一條的措辭**：擋的是**內容的來源**不是動作本身——策展過的展示素材可以在 `PORT-010` 的本機 Driver 上真的跑完一次生命週期（[ADR-059](../../../adr/ADR-059-the-clean-mode-execution-driver-is-honest-about-not-being-a-sandbox.md)，[report-local-driver.md](report-local-driver.md)）。**不受信任的內容仍然一律不行，由派送閘門強制。****這一條 2026-08-28 補上了它的根據**：[report-sandbox-options.md](report-sandbox-options.md) 逐項查過，**結論是「沒有」，而且卡住的不是隔離技術的品質，是把隔離器放上去的縫**——白名單管的是哪顆 PE 能執行，所以任何要你自己帶一顆 `.exe` 的方案都在門口出局。剩下兩條縫（瀏覽器分頁內的 WASM、以及唯一通得出去的那條網路），代價分別是「能力大幅縮水」與「邊界不歸你管、資料離開機構」。**而報告的建議是先問一個行政問題**：防火牆能不能多開一個網域——有了它就能把 Run 送回自己的 gVisor 節點，隔離與生產一模一樣。<br>**⚠️ 順帶查到一件與此直接相關的事**：隔離等級的閘門是**黑名單**，`gvsior`（打錯字）與 `gvisor` 待遇完全相同。今天沒有路徑產得出未知值（宣告值是程式裡的兩路分支），**但加任何新 Provider 型態之前要先改成白名單**——同一段程式的註解記著一次形狀一模一樣的事故。
- **在沒有安裝限制的機器上的最佳解**。那條路是「裝一份原生 PostgreSQL 17 ＋ pgvector」：十分鐘、零程式碼、解鎖 281 支。**它與 M6 不互斥，而且該先做**——但它在受限環境裡不成立，所以不是 M6 的答案。
- **`SBX-008` 短效授權的測試覆蓋**。平台測試裡沒有任何一行 fetch 過 presigned URL，所以「過期即失效」「簽章不可竄改」「GET 的票不能用來 PUT」三個性質今天**零覆蓋**。**這個缺口不是「沒有 Docker」造成的**，做一個檔案系統版物件儲存補不上它，**用一個不驗簽的假 S3 更會讓它從「零覆蓋」變成「看起來有覆蓋」**（[報告](report-object-storage.md) §3 的四個突變）。→ 已記為 [`04` 丙-81](../../04-backlog-and-handoffs.md)；`PORT-009` 只負責**不讓替身假裝證明了它**，補上證據是丙-81 的事。

## 待裁定

- **M6 是否計入 MVP 完成度。** M5 的先例是不計（`01` §7.3）。M6 是開發者體驗與測試可信度，不是產品功能——**但它改變「綠燈是什麼意思」，而那是允收的一部分**。→ [`05`](../../05-pending-rulings.md)
- **Adapter B 的端點範圍**：只涵蓋展示路徑，或涵蓋全部讀取端點。**範圍越大，⛔ 邊界那張表越容易被忽略。**
- **受限環境的白名單內容**（見啟動條件 2）。這是要取得的事實，不是要做的決定。

## 檔案地圖

| 檔案 | 內容 |
| --- | --- |
| `README.md` | 本檔。計畫、狀態、邊界、檔案地圖 |
| [report-inmemory-postgres.md](report-inmemory-postgres.md) | 2026-08-28 的前期量測：SQLite 0/42、PGlite 42/42、逐項行為驗證（含不可變性 trigger 真的擋人）、wire protocol 與單連線死鎖、兩個被排除的候選及根據 |
| [environment-probe.md](environment-probe.md) | **要交出去給坐在那台機器前面的人的清單**：Node 在不在白名單、使用者目錄的未簽章執行檔跑不跑得起來（以 `go test` 當探針）、實際生效的政策與是否只開稽核、Edge 開不開得了本機 HTML、對外通得到哪些網域。**每一項都是唯讀，且刻意不做任何繞過** |
| [report-local-driver.md](report-local-driver.md) | 2026-08-28 的前期量測（本機執行 Driver）：**沒有值得加的相依**。逐一裁決十三個候選，含一個 1027★、README 承諾三平台、而 Windows 端是空殼的套件；**實測**只 kill 父行程會留下存活的孫行程，改用 Job Object 歸零。含動工前該知道的三件事（`Adopt()` 回空、資源上限兩平台不對稱、grace 不是合作式窗口） |
| [report-sandbox-options.md](report-sandbox-options.md) | 2026-08-28 的前期量測（沙箱那一半）：**沒有 PGlite 等價物，而原因不是隔離技術不夠好**。Windows 原生隔離（AppContainer 不需管理員，但需要一顆過不了白名單的 launcher）、瀏覽器內 WASM（Pyodide 沒有 subprocess／pip／原生套件）、WASM 裡模擬 x86（最強邊界，44 倍慢，網路出不去）、託管沙箱（真 Debian，但邊界不歸你管且資料離開機構）；含三個「看起來像答案但不是沙箱」的逐條拒絕，以及隔離閘門是黑名單這個順帶發現 |
| [report-object-storage.md](report-object-storage.md) | 2026-08-28 的前期量測（物件儲存那一半）：in-process 假 S3 七方法十項全過，**但突變顯示它什麼都不驗**——竄改／無簽章／已過期／GET 的票做 PUT，四種都回 200。含 `SBX-008` 的證據該從哪來、瀏覽器端 `blob:` 與 presigned 的三個差別、業界避免假真分歧的三種手法 |

**尚未存在**：`audit.md`（里程碑完結時才產出，同 M3 起的骨架規定）、`report-*` 的其餘報告。
