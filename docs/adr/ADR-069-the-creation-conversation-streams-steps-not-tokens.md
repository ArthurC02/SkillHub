# ADR-069：互動創作串流的是「步驟」，不是 token

- 狀態：**Accepted**（2026-09-09 負責人裁定「三者都簽署通過，立即執行」；`04` 丙-205 把「真串流」移出待辦時說它是「一份還沒寫的 ADR」，這份就是）
- 日期：2026-09-09
- 相關：ADR-067（互動創作的編排與事實來源）、ADR-016（Python 是能力提供者）、ADR-008（狀態機與 outbox）、ADR-068（Credit 計價）、`02` SEC-013、`04` 丙-205、`05` R-70

## 背景

### 今天的形狀

`apps/web` 在會話處於 `queued`／`working` 時每 **1 秒**輪詢一次 `GET /creation-sessions/{id}`（`CreationSession.tsx` 的 `refetchInterval`），拿回整份快照。等待期間畫面上有一句由快照推導出來的步驟描述（`04` 丙-205：連網＞流程圖＞需求＞首稿＞試跑後修訂＞修訂）。

**全 repo 今天沒有任何 SSE 或 WebSocket 端點**（已查證：`apps/`、`contracts/` 裡的相符字串全部落在 `node_modules` 與 FastAPI 自身）。任何串流都會是這個系統的第一條非請求／回應通道。

### 「真串流」在這個系統裡具體撞到什麼

這四件不是顧慮，是查證過的現行行為：

**一｜模型的回覆是提案，不是答案。** `job.go` 的 `proposal()` 有兩種完全不顯示模型輸出的路徑：整份退回（`ErrInvalidCommand`，畫面看到的是「這一步未完成：模型的回覆不符合會話規則」），以及 **nudge**——當草稿與試跑那份逐位元相同、缺流程圖節點、抄了評估文字裡的 marker（SEC-013／LLM01）、或多了沒人要求的工具時，Go 把模型的回覆**丟掉**、寫一則工具訊息說明理由、**重新排隊再問一次**（`MaxNudges = 2`）。串 token 等於把一份平台可能不採用的東西即時畫在畫面上；nudge 那條更糟——使用者會看著一份草稿被寫出來，然後它無聲消失、重寫一次。

**二｜一次 review 步驟有三次模型呼叫，而第三次回來的 body 會被第二次的取代。** `apps/llm/src/skillhub_llm/creation.py` 的修訂路徑：呼叫一列出要改哪裡，呼叫二**以純文字重寫 body**，呼叫三做決定並寫訊息——而**呼叫三回傳的 body 被呼叫二的輸出換掉**（那是 2026-09-06 run n 的修法：mini 模型會一邊描述修改一邊交回逐位元相同的草稿物件）。所以「串流模型正在寫的東西」在這一步沒有唯一解：串呼叫三會顯示一份不會被採用的 body。

**三｜回應是結構化物件，不是一段文字。** `CreationStepResponse` 有 `outcome`、`message`、`brief`、`acceptance_criteria`、`draft`、`tool_intent`……。串流結構化輸出要做 partial JSON：Anthropic 以 `partial_json` 增量交付工具輸入，而**chunk 不尊重 JSON 邊界**——一個 delta 可以斷在鍵中間或字串中間，官方 SDK 用 `jiter` 的 `trailing-strings` 部分解析，並建議在 `content_block_stop` 才真正解析。這是一整套新的失敗模式，換來的是「把一個物件的一半畫出來」。

**四｜跨三個行程，而寫的那個不是對外的那個。** 模型呼叫發生在 **Go Worker** 行程裡（鐵律 7），瀏覽器連的是 **API** 行程。Worker 沒有、也不該有一條到瀏覽器的路。

### 網路上的實務怎麼說

- **傳輸選 SSE 不選 WebSocket。** LLM 回應本質上是單向的；WebSocket 帶來雙向的複雜度而沒有對應的好處。SSE 在瀏覽器內建的客戶端會自動重連並帶 `Last-Event-ID`，伺服器可以從那個 id 之後續傳；WebSocket 的重連要自己寫序號追蹤與補送，「多數團隊乾脆跳過，直接重跑一次生成，浪費 token 和錢」。HTTP/2 之下 SSE 與其他請求多工在同一條 TCP 連線上；反向代理只需要關掉該路由的 buffering，不需要協定升級。要記得每 15–30 秒送一個 keepalive 註解行，否則中間的 proxy 會把閒置連線斷掉。
- **要能續傳就需要一個中繼儲存。** 業界的做法是把 chunk 寫進 Redis Stream（以 `chat_id` + `message_id` 為鍵），relay 訂閱後轉成 SSE，consumer group 保證不重不漏；沒有它，使用者一重新整理就是重跑一次生成。
- **串流的結構化輸出要 partial JSON**（見上）。
- **agent 框架串的通常不是 token。** LangGraph 有三種 `stream_mode`：`updates`（狀態變更，「哪個節點在跑」）、`custom`（應用自己的進度事件）、`messages`（token）。官方建議**不要每個 token 發一個 custom 事件**，那是 `messages` 的工作；`custom` 的自然頻率是「每個大動作一到兩個」。
- **使用者真正感覺到的是 TTFT，不是總時間。** 串流只掩蓋第一個 token 之後的時間——TTFT 三秒的話，使用者仍然看三秒白畫面然後一次爆出來。而**有進度指示時人願意等大約三倍久**；skeleton 比 spinner 感覺快約 20%。
- **agentic 系統串錯階段會洩漏中間推理或不安全的工具輸出。**
- **取消稅**：使用者按停，停的是 UI 不是 GPU；多數供應商仍然把生成跑完並計費，而「付費 token／送達 token」的比值在消費級聊天介面會漂到 1.5 以上而沒人發現。串流的用量也不在最後一個 chunk 就結束——只讀最後一包會少計。

## 決策（草案）

### 決策 1：不串 token

理由是上面的一、二、三，而第一條是決定性的：**模型的輸出在 Go 接受它之前不是這場對話的內容**。串流它就是在畫面上斷言一件平台還沒同意的事，而 nudge 與整份退回是每天都在發生的路徑，不是邊角。

這也不是把使用者體驗讓給安全：業界自己的數字說，讓等待變短的是 **TTFT 與進度指示**，而不是逐字打字的效果。

### 決策 2：串的是「步驟事件」

串流承載 LangGraph 意義下的 `updates`／`custom`，不是 `messages`：**這一步進到哪個階段了**（排隊中、已送出模型呼叫、正在讀網頁、正在搜尋目錄、正在驗證草稿、已交回）。

這一條之所以與**鐵律 5** 相容，是因為它不新增任何狀態：Go 的 `advance()` 已經把每次狀態轉移寫成 append-only 事件，串流只是把**已經是事實的東西**推出去。今天畫面上那句步驟描述是前端從快照**推導**的；事件讓它變成後端**說出來**的，而那正是丙-205 當時做不到的一半。

### 決策 3：傳輸是 SSE，一條唯讀端點

`GET /creation-sessions/{id}/events`，`text/event-stream`，每個事件帶 `id:` 為事件序號，客戶端重連時帶 `Last-Event-ID`，伺服器從那個序號之後補送。每 20 秒一個 keepalive 註解行。

**我們不需要 Redis。** 業界要中繼儲存是因為他們串的 token 沒有家——那些 chunk 不屬於任何資料庫。我們串的是事件，而事件**已經在 Postgres 裡**且已經有序號。續傳是一次 `WHERE session_id = $1 AND seq > $2` 的查詢，不是一套新的基礎設施。這是「串步驟不串 token」帶來的第二個好處，而它比第一個更省事。

### 決策 4：Worker 不推，API 讀

Worker 照舊只寫事件表（同交易，ADR-008）。API 行程的 SSE handler 讀那張表。跨行程的「有新事件了」用 Postgres `LISTEN`／`NOTIFY`；**若不做 NOTIFY，退化成 API 端每秒查一次事件表也成立**，而且仍然比今天好——今天輪的是整份快照，退化版輪的是幾個位元組的序號。這條退化路徑存在的意義是：這個 ADR 不必等 NOTIFY 才能開始。

### 決策 5：先量 TTFT，再決定要不要做決策 2–4

目前一個創作步驟從送出到畫面更新要多久，**沒有人量過**。如果它是兩秒，那 1 秒輪詢已經接近極限，這整份 ADR 的收益是零點幾秒；如果它是二十秒，那進度事件的價值很高而且與串流無關。**這是本 ADR 唯一的前置工作項**，而它要負責人授權付費跑（`task dev:model`）。

### 決策 6：token 串流若日後要做，三個前置條件寫在這裡

不關死這扇門，但關掉「順手做一下」這條路。要重開，先滿足：

1. **守門要能作用在串流上**：Go 對草稿的四道檢查（逐位元相同、缺流程圖節點、抄評估 marker、多出工具）都需要完整草稿。串流時畫面已經顯示的東西，若之後被 nudge 丟掉，畫面與紀錄就會不一致——**要有一個明確的裁定說那時畫面該怎麼辦**，而不是留給實作者。
2. **Python 要標註哪一次呼叫可串**：三次呼叫裡只有一次的輸出會成為使用者看到的東西，而目前那件事寫在註解裡不在契約裡。
3. **取消稅要進帳**：ADR-068 的 `cost_events` 記的是平台真實支出。串流中途取消時供應商仍會計費，`settleCost` 現在的 `UsageUnknown` 分支要能吃下「串到一半停掉」這個情形，而不是把它當成未知用量留著預留額。

## 後果

**得到**：等待期間畫面說的是後端真的知道的事，而不是前端猜的；事實來源不變；不引入 Redis、不引入第二套狀態機、不引入 partial JSON 解析。

**失去**：不會有逐字打字的效果。這是明確放棄的，理由寫在決策 1。

**新增的面**：這個產品的第一條 SSE 端點。它會是 `public.yaml` 裡第一個非 JSON 回應，契約要怎麼描述它需要一併決定；反向代理（`infra/images/web/nginx.conf`）要為那條路由關掉 buffering。**它是唯讀的，且只吐已經寫進資料庫的事件**——`05` R-70 的算繪白名單不受影響，因為事件不是訊息；`04` 丙-210 的不可見字元規則照舊套用在顯示層。

**風險**：SSE 連線在 API 行程裡是長連線，會佔一個連線與一個 goroutine。要有上限與逾時；一場創作最長就是 `SessionTimeout`，這給了天然的上界。

## 承接與驗證

需要負責人裁定三件：

1. **要不要做**——決策 5 說先量再決定；如果連量都不想花，那這份 ADR 就停在 Proposed，而現況（1 秒輪詢＋推導出來的步驟句）**今天並沒有壞掉**。
2. **SSE 端點進不進 `contracts/openapi/public.yaml`**——它是第一個這種形狀的端點，OpenAPI-first 是鐵律 12，但 OpenAPI 描述事件流的表達力有限。
3. **TTFT 量測的付費授權**。

落地時的驗證入口：契約 `gen:check`、API 行程的端點測試（含 `Last-Event-ID` 續傳與 keepalive）、`e2e` 一條「重新整理之後事件不重不漏」的路徑，以及 nginx 那條路由的 buffering 設定要有機器檢查——它是那種關掉之後沒有人會發現的東西。

## 落地補記（2026-09-09，同日實作）

草案的決策沒有被推翻，但**兩處的形狀在實作時比草案更小**，記在這裡而不是改寫上面。

**一｜決策 3 說「續傳是一次 `WHERE session_id = $1 AND seq > $2` 的查詢」，實際上連那張表都不必讀。** 查證 `creation_session_events` 的 schema 之後發現：它的主鍵是 `(session_id, workspace_id, revision)`，而 `revision` 就是 `creation_sessions` 那一列的版本號，由 `AdvanceCreationSession` 在同一個交易裡遞增。也就是說**序號本來就在會話自己那一列上**。而事件列缺一個 `state` 欄位，所以拿它重建一份 `View` 反而重建不出來。

於是落地成：SSE handler 監看會話列的 `revision`，超過客戶端的 `Last-Event-ID` 就送出當下那份文件。**這比草案更省，而且不漏**——快照是累積的（訊息只追加不改寫），所以第 N 版的文件包含第 1..N-1 版會說的一切；漏掉三個版本再接回來的客戶端是落後一份文件，不是三份。曾經加過的 `ListCreationEventsAfter` query 在確認這件事之後收回了，沒有留下沒人用的程式碼。

**二｜決策 3 的「SSE 端點進不進契約」（`05` R-71 簽名 2）落在一個 OpenAPI 表達不了的地方，答案是「進，但 200 不宣告 body schema」。** 先試過 `content: text/event-stream` 加 `$ref: CreationSession`——ogen 因此走進它的 SSE 路徑，生出 `initSSEStream(sseConnectFunc, sseClientConfig)`，而那兩個型別屬於被停用的 `paths/client`，整個 generated package 編不起來。曾短暫加過 `ignore_not_implemented: ["sse server response encoding"]` 讓它跳過，但那條路也不對——**因為那個 schema 本來就在說謊**：body 不是一份 `CreationSession`，是一串。

最後的形狀是：端點、參數、`Last-Event-ID`、四個錯誤回應全部寫進契約（鐵律 12 要的「先寫 schema」成立），200 只有 description，並在其中明說為什麼沒有 schema；**payload 由一支測試釘住**（`creation_stream_integration_test.go` 把 handler 寫出來的 `data:` 反解成 `GET` 回的同一個型別，逐位元比對兩份文件）。那比一個假的 schema 強：假 schema 會通過 lint 而永遠不會被執行。

**三｜決策 4 的退化路徑就是實際採用的路徑。** 沒有做 `LISTEN`／`NOTIFY`；handler 以 250 ms 輪詢會話列，那正是 `job.go` 既有的跨程序取消監看用的節拍與同一列。所以這不是新機制，是同一列多一個讀者。上限誠實且小：每條連線每個 tick 一次主鍵讀。

**四｜nginx 那條路由的 buffering 有機器檢查了**（`cmd/api/main_test.go` 的 `TestNginxDoesNotBufferTheEventStream`）。理由寫在測試裡：buffering 開著時這個端點仍然回 200、仍然送出每一個事件——全部在串流結束時一次到齊。沒有任何東西會報錯，畫面只是「和被它取代的輪詢一樣慢」，而那是一個沒有人在看的數字。

**五｜決策 5（先量 TTFT）的順序在簽署後倒過來了。** 那條原本是「決定之前先量」，而負責人已經決定；量測因此從「閘門」變成「基準線」，排在落地之後、與新路徑一起量，這樣同一次付費跑就同時得到前後兩個數字。
