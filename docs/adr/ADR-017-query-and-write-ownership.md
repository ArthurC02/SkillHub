# ADR-017：Query 與寫入所有權

- 狀態：Accepted
- 相關：[ADR-016 Platform Bounded Context 與 Context Map](./ADR-016-platform-bounded-contexts-and-context-map.md)、[ADR-018 Aggregate 與領域事件](./ADR-018-aggregates-and-domain-events.md)

## 背景

Platform 的 Bounded Context 邊界（見 [ADR-016](./ADR-016-platform-bounded-contexts-and-context-map.md)）以 depguard 擋下跨 context 的 Go import，但 `db/sqlc.yaml` 只有一份設定，`db/queries/*.sql` 的每一條 query 全部生成到同一個 Go package。depguard 只看得到「import 了 `db/gen`」，看不到呼叫的是誰擁有的 query——任何 context 只要 import 這個共用 package，就能直接讀寫別人的資料表。同一個洞還有第二層：就算一條 query 的呼叫端正確，它的 SQL 本身仍可以 JOIN 別人擁有的表。這兩層都需要獨立於 import 邊界的強制機制。

## 決策

### 決策 1：每張表與每條 query 都有唯一的 owner，owner 是 Context Map 的 Boundary ID

`db/query-owners.yaml` 用兩層宣告：

- `tables:`：每一張資料庫表登記一個 owner。唯一的共有例外是 `artifacts`——它被兩種產物共用，依 `kind` 逐列區分 owner（Run 輸出歸試跑執行 context、下載封裝歸交付 context），而不是整張表歸一方。
- 每條 query 也宣告 owner。owner 判準是**該 query 主要資料表的擁有 context**；同一張表的擁有者不會因為讀它的 query 屬於哪個用途而改判——即使某個 Supporting context 是這條 query 唯一的使用者，owner 仍看表不看用途。

`automation-check` 比對兩層宣告：一條 query 的 SQL 碰到的每一張表，owner 都必須等於這條 query 宣告的 owner；owner 本身也必須是這條 query實際呼叫端允許使用的 context。解析對象是正規化後的 SQL（先去除註解與字串字面值，`FOR UPDATE` 之類的鎖修飾詞會被消掉，因此鎖定查詢仍正確判為 read），取 `FROM`／`JOIN`／`INTO`／`UPDATE`／`USING` 之後的表名，`FROM`／`USING` 後以逗號串接的多表寫法一併解析，CTE 名稱排除在外；migration 用 `PARTITION OF` 建出的子表不算獨立的表。新增一張表卻沒登記 owner，或宣告檔與 `db/queries/*.sql` 兩邊互相缺漏（有 query 沒宣告、有宣告沒 query、已無呼叫點的容忍條目），均使檢查失敗——宣告檔要能一直被信任，就不能有安靜漂移的空間。

宣告檔以自解析的 YAML 子集撰寫（`tools/devctl` 維持零第三方依賴），子集外的寫法直接報錯而非略過。

本節管的是表與 query 的 owner 宣告本身；Context 對照表跟套件目錄、depguard 規則是否互相對得上，是另一道獨立檢查，見 [ADR-016 決策 1](./ADR-016-platform-bounded-contexts-and-context-map.md)。

### 決策 2：Write 與 Read 各自強制，棘輪形狀相同

- **Write**（INSERT／UPDATE／DELETE，含以 CTE 形式寫入）：owner 與呼叫端不一致即 FAIL。
- **Read**：owner 與呼叫端不一致同樣 FAIL。read 不因為「只是讀」而被輕放——它雖然不會直接破壞別人的不變量，但會讓 owner 無法安全改動自己的 schema，且沒有任何地方記載誰在讀。

兩者共用同一個判定迴圈與同一套呼叫點掃描，只有「這條 query 是否為 write」這一個比對值不同；沒有第二套 SQL 解析器。呼叫點認定方式：掃描 import 了 `db/gen` 的非測試 Go 檔，比對呼叫語法與 owner 是否一致，把 owner 之外的呼叫端解析成它所屬的 context。**Generic 套件**（技術基座、組裝層、防腐層——完整清單與判準見 [ADR-016](./ADR-016-platform-bounded-contexts-and-context-map.md)）視同 context 參與判定：呼叫自己領域的 query 合法，呼叫別人的 write 或 read 一樣要走下一段的豁免機制；「Generic」指的是沒有領域規則，不是可以繞過所有權。

新的跨 context write 或 read 一律直接判定為違規，不得先斬後奏再補宣告。存量或有正當理由的跨 context 存取，以 `db/query-owners.yaml` 的 `allow:`（write）／`read_allow:`（read）具名登記，且：

- 每條必須附理由；理由的品質靠審查，機器只保證「具名」與「失效即 FAIL」——條目對應的呼叫點一旦消失，留著就是失敗。
- 條目放錯段落（write 條目寫進 `read_allow:` 或反之）視為未登記，直接 FAIL，且不給予豁免。
- 這兩份清單是**已核准的存量與例外清單，不是常態的擴充點**：新的跨 context 存取預設要改用決策 4 的三種形狀之一或走決策 5 的注入，走豁免清單需要說明為什麼那三種形狀都不適用。

### 決策 3：鎖跟著寫入的 owner 走

`SELECT … FOR UPDATE` 類型的 query 在決策 2 的判定裡是 read，但它的語意屬於寫入的一部分：誰要寫、就該由誰取鎖，不能由呼叫端先鎖住 owner 的列、再請 owner 去寫。呼叫端先鎖的形狀下，owner 無法保證自己的寫入被正確序列化——第二個呼叫端只要鎖錯列或忘了鎖，owner 這邊不會有任何跡象。因此：涉及後續寫入的鎖定查詢，鎖定的權責搬到 owner，呼叫端不再自行取鎖，owner 收呼叫端的交易、不自己開交易。多數情況下鎖定與寫入合併成 owner 匯出的單一函式；當呼叫端的臨界區必須在確認權限之前就開始（寫入本身要等權限確認後才能執行），owner 才另外匯出一支只取鎖的函式供呼叫端先呼叫，寫入函式仍自己重新取鎖——同一交易內對同一列重複取鎖在 Postgres 是 no-op，兩支函式並存不是成本。owner 匯出的函式命名刻意避開被取代的 query 名稱，避免呼叫點判定把它誤認成違規本身的重現。

### 決策 4：跨 context 取事實只用三種形狀

不同的資料需求對應不同的形狀，選哪一種看「要什麼、在哪裡用」，不是看哪一種最省事：

| 形狀 | 適用時機 | 例子 |
| --- | --- | --- |
| **owner 的批次讀取**，由組裝層注入 `func(ctx, db gen.DBTX, ids []UUID)` | 要一批列各自的事實，或需要在同一交易裡看到別人尚未提交的變更 | 清除流程裡問其他 context「這些 id 你還在用嗎」 |
| **範圍當參數**：owner 回答「哪些屬於這個範圍」，查詢者把範圍留在自己的 SQL 裡 | 事實是一個集合，且會被應用程式以外的方式改動 | 目錄可見的 workspace 範圍 |
| **讀取模型**：owner 寫入時把事實投影到查詢者自己的表 | 事實要拿來過濾、排序又要分頁，逐列去問會造成 N+1 | 搜尋索引帶著來源事實 |

批次讀取一律吃呼叫端傳入的交易控制代碼，不是自己開連線——原因是清除流程在單一交易內依序執行多個 context 的步驟，後面的步驟必須看得到前面步驟尚未提交的刪除；用連線池另開連線會看到舊資料而少清。「範圍當參數」不把範圍複製進查詢者自己的表：範圍如果只由維運或種子工具改動、應用程式從不寫它，複製一份會在第一次有人直接改資料庫時悄悄失真。

決定「這一列看不看得到」的事實永遠向 owner 即時讀（多半走「範圍當參數」，或在寫入時於同一交易鎖列確認存活）；只描述一列內容、不影響可見性的事實才投影成讀取模型。過期的投影後果只應是顯示舊值，不能是讓不該看見的人看見。

### 決策 5：跨 context 寫入以依賴反轉注入收斂，不事件化

當某個跨 context 動作的 import 方向會造成編譯期循環——即兩個 context 互為對等關係、誰也不在誰之下——owner 公開一個收呼叫端交易控制代碼（`pgx.Tx` 或同交易的查詢代理）的函式，由組裝層（API 組裝根、Worker 組裝根，或維運／重建索引等命令行入口）在初始化時把函式注入呼叫端，呼叫端在自己既有的交易裡呼叫它。這不是 import，也不是事件：呼叫端與 owner 之間沒有新增依賴方向，動作也不離開原本的交易邊界。

適用場景是那些請求当下就必須完成、失敗了就代表「這次請求沒有被正確處理」的動作——例如搜尋索引要跟著匯入結果同交易更新；帳號刪除目前也是在單一交易內清乾淨每個 context 的資料，這個形狀是否維持由 [ADR-018](./ADR-018-aggregates-and-domain-events.md) 的待決策承接。判準是：**失敗的後果若是「當下這筆請求無法正確回應」，走同步注入；若只是「之後該發生的事沒發生」，才適合交由事件驅動**（同步與事件的完整判準見 [ADR-016 Platform Bounded Context 與 Context Map](./ADR-016-platform-bounded-contexts-and-context-map.md) 決策 2）。這與 aggregate 之間改用領域事件溝通（見 [ADR-018](./ADR-018-aggregates-and-domain-events.md)）是兩件事：搜尋投影與帳號清除不是 aggregate 對 aggregate 的溝通，而是讀取模型維護與合規上的全有全無要求，因此維持同步注入。

配套規則：

- **未注入即拒絕執行**，不得以「當作沒有引用」「當作空範圍」之類的預設值繼續跑下去——尤其是清除類動作，少清一個 context 卻回報成功，比整批失敗更嚴重。
- 每一個組裝根都要有對應的接線測試，缺任何一條注入都必須讓測試變紅。
- 多個 owner 步驟同屬一筆跨 context 交易、彼此之間有依賴時，執行順序本身是 load-bearing 的規則，要記錄下來並用測試守住：把順序對調要能讓對應測試變紅，不能只靠註解自我聲明。順序錯誤的後果可能沒有任何錯誤或反常的 row count 可供察覺，只留下悄悄殘留的資料。
- 清除類 query 拆成「先決定、再刪除」兩步：先讀出還在被引用的對象，再各自執行刪除。中間的空檔由 owner 表上的 `NO ACTION` 外鍵兜底——空檔裡新提交的引用會讓刪除失敗、整批回滾，不會刪掉仍被引用的東西；刪除語句仍要重新檢查 owner 自己的條件（例如寬限期），不能只信任先前讀到的清單。有上限的清除必須先套用過濾條件、再套用筆數上限，否則一直被引用的舊資料會佔滿每一格、讓清除永遠掃不到後面。

### 決策 6：強制的前提是「所有讀寫都經過 sqlc」，此前提本身要有防線

決策 1、2 的檢查只看得到 `db/queries/*.sql` 定義的 query；一行把字面 SQL 直接交給 pgx 執行的程式碼，會同時繞過 ownership 與寫入序列化兩道防線，且沒有任何地方會發現。因此 `automation-check` 另外掃描 `apps/platform/internal/**` 與 `apps/platform/cmd/**` 的非測試 Go 檔：只要 SELECT／INSERT／UPDATE／DELETE／SET／CREATE／ALTER／DROP／TRUNCATE 或 WITH 的字面 SQL（直接字面值、package 層常數、函式內區域變數、或 `fmt.Sprintf` 組出的樣板）被交給 pgx 的 `Exec`／`Query`／`QueryRow`／`Queue`，一律 FAIL；生成目錄與測試檔不在掃描範圍內。具名豁免登記在 `db/query-owners.yaml` 的 `raw_sql_allow:`，key 是精確到函式的原始碼位置、value 是理由，理由留空或該函式已無裸 SQL 均視為失效並 FAIL。

這道檢查是防護網（tripwire），不是完備證明：跨函式傳遞組好的 SQL、`database/sql` 或其他非 pgx 路徑、更複雜的動態拼接仍可能看不到。要完全封死唯一的路徑是把資料庫連線池收進只暴露 sqlc 產出的介面之後，讓「直接拿到連線池」變成不可能——這與拆分每個 context 專屬的 sqlc 產出成本相當，留待下一段的待決策。

## 影響

### 正面

- 「哪個 context 擁有哪張表、哪條 query」第一次有單一、可被 CI 驗證的答案，且答案同時涵蓋呼叫端與 SQL 本身兩個層面。
- 新的跨 context 寫入與讀取在 CI 就被擋下，不必依賴審查人力抓到。
- 不更動 `db/sqlc.yaml`、`db/queries/*.sql` 或任何 generated 目錄，沒有程式碼生成產生落差的風險。
- 跨 context 取事實收斂為三種可預期的形狀，新加入的存取不必每次重新設計介面。

### 成本與限制

- 新增或修改 query／表都要同步維護宣告檔；漏了 CI 直接失敗，這是刻意的摩擦。
- 呼叫點判定維持純文字比對，不是完整的型別解析；owner 匯出函式命名若不小心與被取代的 query 同名，判定會失去意義。升級路徑是改用 go/ast 解析，裸 SQL tripwire（決策 6）的呼叫點判定已先行採用。
- 裸 SQL 防護網已知有看不到的形狀（見決策 6），在完全封死之前不能當作完備的保證。
- 跨 context 寫入的依賴從編譯期檢查變成執行期注入，靠 fail-closed 與接線測試補償；沒有這兩者的注入等於把靜態錯誤換成運行期才會發現的靜默錯誤，不可接受。
- 涉及多個 context 的交易（例如帳號清除依序呼叫多個 owner 的函式）會變長；這類動作屬於離峰的維運或背景流程，不在使用者請求的關鍵路徑上，可接受。

## 待決策

- 是否要拆分成每個 context 專屬的 sqlc 產出，或把資料庫連線池收進只暴露 sqlc 的介面之後以封死裸 SQL 的殘餘盲點——兩者成本相當，且都建議在跨 context 存量降到可控範圍後再評估。
- 若日後搜尋索引因效能需求必須脫離目前的主資料庫，決策 5 的同步注入寫法將無法再與領域寫入同交易完成，屆時是否改為事件驅動需要與拆分服務的時機一併重新評估。
