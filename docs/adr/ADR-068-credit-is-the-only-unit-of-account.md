# ADR-068：Credit 是平台唯一的計價單位——兩本帳、三道閘、由統計推導門檻

- 狀態：Accepted（2026-09-08）
- 日期：2026-09-08
- 決策者：產品負責人、架構規劃
- 相關：[ADR-017](./ADR-017-model-gateway-and-llm-observability.md)（LiteLLM 為唯一模型出口、每 Run／每次呼叫短效 Virtual Key 與 `max_budget`）、[ADR-028](./ADR-028-beta-admission-and-quota-enforcement-points.md)（配額強制點的既有判準：閘道上的機制都不是配額，強制點必須在平台自己的計數器）、[ADR-011](./ADR-011-workspace-tenancy-policy-and-usage.md)（Workspace 為租戶邊界、Policy & Usage 模組的既有預留）、[ADR-032](./ADR-032-ddd-bounded-context-governance-for-platform.md) §1（本 ADR 登記新套件 `creator/credit`）、[ADR-029](./ADR-029-product-analytics-events-and-audit-trace-boundaries.md)（分析事件「不留查詢內容」的既有紀律，本 ADR 對成本事件套用同一條）、[ADR-067](./ADR-067-interactive-skill-creation-with-langgraph.md)（互動創作會話的 `settleCost`／快照交易，本 ADR 接在它後面扣點）
- 承接需求：`02` 新增 `CRED-001`～`CRED-009`；`01` §10 第十次放行（新功能凍結期間的負責人本輪指示即為放行依據）
- 待其他批次承接：`05` R-2（封測報酬的措辭要從金額改為點數與加成）不在本 ADR 的可修改範圍內，留給下一批負責

## 背景

平台今天沒有任何使用者可見的「這次要花多少」與「我還能花多少」——`RUN_QUOTA`／`GENERATE_QUOTA`（ADR-028、[ADR-056](./ADR-056-the-generation-allowance-is-its-own-switch-and-it-is-off.md)）出貨值都是 `off`，且它們量的是**次數**不是**成本**；LiteLLM 閘道上的 `max_budget`／`tpm_limit`（ADR-017）管的是單次呼叫與速率，ADR-028 §2.1 已經把「閘道上的機制沒有一個回答『這個帳號還能跑幾次』」寫死過一次，本 ADR 面對的是同一類問題的第三次出現：**閘道上的機制也沒有一個回答『這個帳號還能花多少』**。互動創作（ADR-067）已經有實測成本（`docs/plans/mvp/m5/creation-measure/` 一系列報告），但那些數字只存在於報告與 `Snapshot.SpentUSD` 這個會話內部欄位裡，會話結束後不轉成任何使用者資產或負債。

負責人本輪逐字裁定（下引五句是決策的唯一文字依據，本 ADR 不改寫其意）：

1. 「這個系統只能使用 Credit，而 Credit 只有在這個系統才能使用。」
2. 「每個帳號都需要紀錄 Credit Balance，每次創建 Agent Skill 都會依照 LLM 使用量再乘上一個比例的額外費用換算 Credit 的扣除額之後，扣除使用者帳戶的 Credit，有可能發生低於 0 的情境，等待下次充值會扣除負額。」
3. 「負債不能超過 -50。」
4. 「當使用者的 Credit 低於某個閥值，你可以在開始前就阻擋。」
5. 「把每次創建和搜尋的實際費用都記錄，並且在特定時機和固定時間進行統計，之後就能夠依照統計值進行使用者行為的控制。」

同批附帶一個產品後果：封測受測者的報酬不再是現金，而是一筆 Credit 授予——這把 `05` R-2（報酬金額）從「要簽一個金額」改寫為「要簽一組點數與加成」，但 `05` 不在本 ADR 的可修改檔案清單內，改寫留給下一批。

量到的真實成本（唯一可引用的數字，來源：[m5/creation-measure/](../plans/mvp/m5/) 系列報告 §5～§14）：

| 項目 | 中位 | 最大 |
| --- | ---: | ---: |
| 一場互動創作會話（不含試跑與評審） | $0.023–0.030 | $0.041–0.050 |
| 評審每場（14 場 $0.26 攤） | ≈$0.019 | — |
| 單次生成對照 | $0.0045 | — |
| 目錄搜尋一次 embedding | $0.00001 | — |
| 會話上限 `max_cost_usd`（R-45 已定，[ADR-067](./ADR-067-interactive-skill-creation-with-langgraph.md)） | — | $1.00 |

## 決策

### 1. Credit 是唯一計價單位，且只在這個系統內有意義——不可兌出

裁定 1 逐字劃了範圍：**平台面對使用者的每一個成本呈現，只有一種單位**——不顯示美元、不顯示 token 數當作「額度」、不做第二種積分或等級。反過來，**Credit 不是貨幣的影子**：它不能被兌換回現金、不能轉給另一個帳號、不能跨系統使用（不對接任何第三方支付或積分系統）。這與「MVP 不接金流」（決策 10）是同一個判斷的兩面——前者是產品邊界（Credit 進不了系統外），後者是實作範圍（系統外的錢也進不了 Credit，唯一入口是 operator 授予）。**這不是一句免責聲明，是一句設計約束**：任何未來提案一旦讓 Credit 可以兌現或跨系統流通，就必須先推翻本決策、新增 ADR，而不是在既有欄位上加一個 `redeemable` 旗標。

### 2. 面額與加成——1 credit = US$0.001，加成 1.3 存成 basis points

面額定為 **1 credit = US$0.001**，換算對照上表：一場互動創作會話含評審，實測成本約 $0.03–0.05，套用加成後約落在**30–65 credits**；`max_cost_usd = $1.00` 換算為 1000 credits，是單場硬上限而非常態值。

加成套用在「平台真實支出」換算「使用者扣除額」這一步，**出貨預設 1.3**，存成 basis points（`13000`），理由是整數運算不失真、且與 LiteLLM 既有 `max_budget` 的美分計價同一慣例。**每一筆分錄記下寫入當時生效的加成值**，日後調價（改設定值）不改寫既有分錄——這與鐵律 4「歷史 Run 不可變」是同一種紀律的貨幣版本：一筆已經發生的扣款,不因為明天的定價政策改變而回頭變成另一個數字。面額與加成本身的最終值留待負責人裁定（見「待決策」）。

### 3. 兩本帳，各管各的事實，不合成一張表

**`cost_events`：平台的真實支出。** 每一次付費的模型／embedding 呼叫都記一筆，不分這筆呼叫最終有沒有人買單：互動創作會話的每一步（`creator/creation` 的 `settleCost`）、目錄搜尋的 embedding（`skill/discovery`）、索引增強、評審（`trial/improvement` 的 judge／suggest-improvements）、單次生成對照（`skill/admission` 的 `/v1/generate-skill`）。欄位至少含：用途 `kind`（`creation_step`／`search_embedding`／`index_enrich`／`review`／`suggestion`／`generate`）、模型識別、提示詞版本、token 數（輸入／輸出）、`usd_micros`（微美元整數，避免浮點誤差）、`workspace_id`（**可為 null——匿名搜尋沒有 Workspace**）、`user_id`（同理可為 null）、來源 ref（session id／run id／skill version id 三選一，依 `kind` 決定哪一個有值）、冪等鍵。**不存查詢字**：這是 [ADR-029](./ADR-029-product-analytics-events-and-audit-trace-boundaries.md) 對分析事件「不留查詢內容」規則的同一條紀律套用到成本事件——一次搜尋花了多少錢是平台的事實，使用者搜了什麼字不是這張表要回答的問題。

**`credit_entries`：使用者餘額的異動。** 只記四種動作：扣點（`debit`）、授予（`grant`，operator 動作）、充值（`topup`，MVP 期間與 `grant` 同一機制）、調整（`adjustment`，人工更正）。每一筆扣點分錄**指向它來自哪一筆 `cost_events`**（一對一或一對多，視是否合併結算），沒有對應 `cost_events` 的扣點分錄不成立——這防的是「扣了錢但答不出為什麼」，是財務對帳的最低要求。分錄一經寫入不可修改（同 `skill_versions` 等八張表的不可變紀律，鐵律 4 的貨幣版本），更正一律用新的 `adjustment` 分錄,不得原地改寫。

兩本帳分工的判準：**`cost_events` 回答「平台花了多少」，`credit_entries` 回答「使用者被記了多少」**——兩者的金額在有加成時不相等（`credit_entries` 的扣點 = 對應 `cost_events` 的 `usd_micros` × 當時加成，換算 credit 並無條件進位），把它們合成一張表會讓「加成」這個中間步驟消失，之後想調整加成而不動歷史事實就做不到。

### 4. 餘額是分錄的和；物化欄位只是讀取捷徑，不是事實來源

一個帳號（本 MVP 即一個 Workspace，[ADR-011](./ADR-011-workspace-tenancy-policy-and-usage.md)「每位使用者一個個人 Workspace」）的 Credit 餘額，**定義上是它名下 `credit_entries` 全部分錄的和**。為了不讓每次讀取都掃全表，`workspaces`（或一張一對一的 `credit_balances`）上放一個物化欄位，**在寫入分錄的同一個交易內同步更新**——這與 ADR-028 §2.2「餘額是可以漂移的快取，配額用非終態計數不用 counter」故意反著做：配額的計數對象（非終態 Run）本身會變動,而 Credit 分錄一旦寫入永不變動，物化欄位漂移的唯一原因是「有分錄沒有同交易更新它」,這是一個可以被交易保證消滅的錯誤，不是本質上的不精確。**讀取用物化欄位（畫面顯示、三道閘的判斷都讀它），對帳與稽核用分錄**——兩者不同步時,以分錄的和為準,物化欄位的角色只是效能捷徑。

### 5. 扣點接在既有的會話結算之後，同一交易，(session, revision) 冪等

互動創作會話每一步已經有 `settleCost`（`apps/platform/internal/creator/creation/job.go`）把已知的呼叫成本累加進 `Snapshot.SpentUSD`，並在讀不到成本時把 `Snapshot.UsageUnknown` 設真（而不是把那一步當成花費 $0）。本 ADR 的扣點**接在這一步之後**：`settleCost` 算出這一步的實際或預留成本後，在**與快照前進（`AdvanceCreationSession`）同一個資料庫交易內**寫入一筆 `cost_events` ＋一筆對應的 `credit_entries` 扣點分錄。**冪等鍵是 `(session_id, revision)`**——沿用會話既有的樂觀鎖 `Revision` 欄位（`creation.go` 的 `ExpectedRevision`），同一個 revision 的結算重跑（例如 Worker 因逾時重試同一個 Job）必須是同一筆分錄，不得重複扣款。

**用量未知時按預留額扣，並標記 `estimated`。** `settleCost` 已有的 `reserved` 參數（呼叫端傳入 `e.Limits.MaxCallCostUSD`）就是這裡要用的預留額——讀不到 LiteLLM 回報的實際成本時（`usage.CostUSD == nil`），扣點分錄以這個預留額計算，並在分錄上標記 `estimated = true`。**絕不因為讀不到成本而扣 0**：這與既有規則「未知成本不得變成餘額」（`UsageUnknown` 標記本身的存在理由）是同一條——讀不到成本最可能的原因是呼叫失敗前已經花了錢而回報管道斷了，把它當成免費的那一格,是這條規則要堵的漏洞。

單次生成對照（`skill/admission` 的 `/v1/generate-skill`）與目錄的評審／建議（`trial/improvement`）沒有「revision」這個概念，各自的冪等鍵是**該次呼叫的既有唯一識別**（生成請求 id、評估／建議的 attempt id）——形狀相同，鍵不同。

### 6. 無條件進位——平台不因四捨五入吃虧

任何「美元轉 credit」「credit 轉整數扣除額」的換算，**一律無條件進位（ceiling）**，不做四捨五入、不做無條件捨去。理由是單向的：捨去或四捨五入在大量小額呼叫上會系統性地讓平台的實際支出高於記錄下來的扣除額（使用者這一分錢的零頭永遠對平台不利），而進位的系統性誤差方向相反，且每筆金額本身是微美元等級（`usd_micros`），累積誤差對使用者體感可忽略。這條規則**適用於本 ADR 定義的每一處換算**：`usd_micros` → credit 的扣點換算、`cost_events.usd_micros` × 加成 → 扣除額、統計門檻（決策 8）的 p95 × 加成。

### 7. 三道閘各自獨立，不互相取代

裁定 3、4 各自對應一道閘，且**三道閘檢查的時機、依據與後果都不同，不得合併成一次判斷**：

| 閘 | 檢查時機 | 判準 | 後果 |
| --- | --- | --- | --- |
| ① 開始前 | 建立新會話／新一次生成之前 | 餘額 < 決策 8 推導出的門檻 | 拒絕開始，不消耗任何成本 |
| ② 每步扣款前 | 決策 5 的每步結算，在**呼叫模型之前**用該步的預留額試算 | 餘額 − 本步預估 < **−50** | 停止該會話，不再進行下一步；已發生的成本仍照決策 5 結算 |
| ③ 單場上限 | 既有機制,本 ADR 不動 | `max_cost_usd`／`raise_budget`（ADR-067、R-45 已定，出貨值 $1.00） | 既有行為：中止並要求使用者主動加碼預算 |

閘 ①、② 是本 ADR 新增，閘 ③ 原樣保留——**三者疊加生效，任一道擋下就停，不是取最寬鬆的一道**。這與 ADR-028 §2.3「顯示與強制的順序是硬的」同一條紀律：閘 ①、② 上線那天就必須是真的會擋,不得先顯示門檻與 −50 字樣、之後才補強制（乙-2 的教訓)。

閘 ② 的「−50」是裁定 3 的逐字值，單位是 credit,對應「欠一場」的量級（決策 2 換算：一場 30–65 credits）——**這不是一個任意選的緩衝**,它讓使用者在餘額見底時仍能完成手上這一場、而不是在跑到一半被腰斬,同時把負債鎖在「最多再欠一場」的量級。閘 ② 判斷用的是**該步的預留額**（決策 5 的 `reserved`)而非事後的實際花費——用預留額試算是因為判斷必須發生在花錢之前,實際花費永遠只能事後才知道。

閘 ①、② 與既有的 `RUN_QUOTA`／`GENERATE_QUOTA`（次數配額,ADR-028、ADR-056,出貨值皆 `off`）是**平行機制,不是替代關係**：次數配額問「這個月還能跑幾次」,Credit 閘問「餘額還夠不夠」,两者可以同時生效或同時關閉,互不依賴。

### 8. 門檻由統計推導，不是寫死的常數

裁定 4「低於某個閥值可以擋」與裁定 5「統計之後依統計值控制」合起來,回答的是**閥值從哪裡來**：不是一個寫死在設定檔的數字,而是**`cost_statistics` 表存的滾動窗統計**——p50、p90、p95、max 與樣本數,依決策 9 的兩種時機重算。閘 ① 的門檔 = **最近一個滾動窗的 p95 × 加成,無條件進位**——用 p95 而非 p50 或 max,是因為它要擋的是「明顯不夠跑完典型一場」而不是「擋掉一切比中位數貴的使用」,也不是「等到真的一毛不剩才擋」（那樣閘 ① 就形同虛設,閘 ② 要單獨扛下所有情況）。

**樣本數不足（< 20）時退回設定的保守常數**,並在畫面上明確標示這是估計值而非依實測推導的門檻——這與 `PORT-003`「說出它沒有什麼」同一種措辭紀律：一個少於 20 筆樣本推出來的 p95 統計上不可靠,誠實地說「這是估計值」比呈現一個看似精確的數字更不誤導。

### 9. 統計兩種時機——事件驅動與固定時間

裁定 5「特定時機和固定時間」對應兩種各自獨立的觸發：

- **事件驅動**：一場互動創作會話終結（保存、放棄或逾時,ADR-067 既有的終態）時,把該場的全部 `cost_events` 收斂成**一列摘要**（session id、總花費、步驟數、是否曾標記 `estimated`）寫入統計的輸入來源——這是把「一場多少錢」從多筆逐步事件變成一個可以直接進滾動窗的樣本點,不必在算分位數時重新展開每一步。
- **固定時間**：既有的 River 排程佇列（[ADR-008](./ADR-008-asynchronous-workflows-and-domain-events.md)）新增一個 periodic job,**每日**重算滾動窗的 p50／p90／p95／max 與樣本數,寫入 `cost_statistics` 的最新一列。選「每日」而非更即時,是因為閘 ① 的門檻不需要分鐘級的新鮮度——一天內的成本分布不會劇烈漂移,而更頻繁的重算只會增加資料庫負擔換不到閘門判斷的品質提升。

兩種時機各自寫**各自的資料**（前者是原始樣本輸入,後者是推導出的統計列）,不互相取代——事件驅動確保每一場結束都貢獻一個樣本點（不必等到隔天),固定時間確保即使某一天沒有任何會話結束,統計仍然會依既有樣本重新計算（例如樣本數跨過 20 筆的門檻)。

### 10. MVP 不接金流——充值就是 operator 授予

MVP 期間不存在任何真實付款路徑：充值 = operator 在既有 `/admin/…`（`RequireOperator`,[ADR-032](./ADR-032-ddd-bounded-context-governance-for-platform.md) §1「各 context → identity」的既有授權入口）下手動寫入一筆 `grant` 分錄,與 [ADR-028](./ADR-028-beta-admission-and-quota-enforcement-points.md) 決策 1 對封測允許清單「異動 = 改設定並重啟」同一種「單人團隊先用最小機制」的紀律,以及 `SEC-011` 對 operator 動作「必填理由、audit event、不可自助授予」的既有規則全套適用——**每一筆 `grant`／`adjustment` 分錄都是一個 operator 動作,寫 audit event,含理由**。封測受測者的報酬即是這個機制的第一個使用者：一筆帶理由的 `grant` 分錄,而不是一筆金額轉帳。決策 1 已經說明為什麼這不是「先做一個閹割版的金流」——Credit 從設計上就不打算連接任何金流,不是「還沒接」。

### 11. 保存與刪除——新表併入既有清冊與 purge 路徑

`cost_events` 與 `credit_entries` 是使用者資料,適用 `NFR-002` 既有的保存清冊與刪除規則,不另立一套：兩張表併入既有的保存期限對帳（同 `SEC-006` 已經在管的 `DOWNLOAD_ARTIFACT_RETENTION`／`TRACE_RETENTION` 那組變數的既有形狀,值留待負責人與既有保存清冊一併裁定),帳號刪除時的清除路徑併入 `cmd/maintenance` 既有的 `skillhub_purge` 角色（今日剛完成涵蓋新表的收斂)。**`cost_events` 不存查詢字**（決策 3)這件事本身也降低了它的刪除急迫性——沒有使用者輸入的原文,遮罩與刪除的壓力比 Trace／對話快照小,但欄位仍可能間接透過 `source_ref` 指回一個已刪除的會話或版本,清除時比照既有「歷史列不刪、但失去可讀取的下游」的既有處置（同 `CONTENT-009` 下架不改寫既有版本的既有紀律),不是本 ADR 新開一套刪除語意。

## 考慮過的替代方案

- **只做一本帳（餘額直接記在 `cost_events` 上,不分兩張表）**：拒絕。加成、estimated 標記與「使用者被扣了多少」跟「平台花了多少」在有加成時是兩個數字,合成一張表會讓調整加成這個動作失去「不改寫歷史事實」的性質（決策 3)。
- **門檻寫死一個常數,不做統計推導**：拒絕,直接違反裁定 5 的逐字指示——負責人明確要「統計之後依統計值控制」,不是「先猜一個數字」。
- **配額（次數)與 Credit（金額)合併成一套機制**：拒絕。ADR-028 的次數配額問的是「幾次」,Credit 問的是「多少錢」——一場創作可能因為模型重試而比另一場貴數倍,次數配額擋不住這種情況,金額配額擋不住「很多次但每次很便宜」的情況,兩者是互補而非互斥的機制（決策 7 末段)。
- **餘額即時算(不放物化欄位,每次讀取都掃 `credit_entries`)**：拒絕在畫面熱路徑上這樣做——分錄數量隨使用時間線性成長,掃描成本非常數,而交易保證讓物化欄位的漂移風險趨近於零（決策 4)。對帳與稽核仍然掃分錄,不受影響。

## 影響

- 正面：使用者第一次能在動作之前看到「這個會話大概花多少 Credit、我還剩多少」；平台第一次有機制在餘額見底前主動擋下開新會話,而不是無上限地累積負債；成本事件與扣點分錄分開,未來調整加成或引入真實金流時,歷史分錄不必回頭改寫。
- 成本：新增兩個核心表與一個統計表,`creation`／`ingest`／`eval`／`catalog` 四個既有 context 都要新增對 `credit` 的同步呼叫（依 [ADR-032](./ADR-032-ddd-bounded-context-governance-for-platform.md) 的 Customer–Supplier 機制,見該 ADR §1／附錄 A 的同批修訂);`credit` 套件登記進 ADR-032 §1 之後,`apps/platform/internal/creator/credit` 目錄與對應的 depguard 規則要在後續實作批次補上——**在那之前,`devctl automation-check` 的 `context-map` 檢查會回報「§1 列出的 Boundary ID 沒有對應的 Go package 目錄」與「沒有 depguard 規則涵蓋這個路徑」兩項,這是先登記後建目錄的既有工作流程(AGENTS.md 第 11 條)預期中的過渡態,不是本次文件工作的缺陷**。
- 風險：三道閘任一道的門檻設得太保守,會讓使用者在餘額仍充足時被擋——這正是決策 8「門檔由統計推導、樣本不足退回保守常數並標示估計值」要緩解的問題,但緩解不等於消除,面額與加成的最終值仍待負責人裁定。

## 待決策（留給負責人）

- **面額與加成的最終值**：本 ADR 採用的 1 credit = US$0.001、加成 1.3（13000 bp）是主線工作依「量到的真實成本」與「一場 30–65 credits」的產品體感反推的預設值,不是負責人逐字裁定的數字——正式數值需要負責人核可,或明確授權維持本 ADR 的預設值。
- **要不要對搜尋與試跑也收點**：裁定 5 要求記錄搜尋的成本事件（決策 3 已落地),但沒有裁定搜尋本身要不要扣使用者 Credit——今天的設計是搜尋的 `cost_events` 只餵統計,不產生對應的 `credit_entries` 扣點。試跑（Run)呢? 目前 Run 本身不呼叫模型(除非 Skill 本身在沙箱內呼叫,那筆成本走 Virtual Key 的 `max_budget`,不在本 ADR 兩本帳的既有覆蓋範圍內)。這兩項是否要納入 Credit 扣點,留待負責人裁定。
- **Credit 過期政策**：本 ADR 沒有定義 Credit 是否有效期——`grant`／`topup` 分錄是否應該連帶一個 `expires_at`,逾期未用的 Credit 如何處理(作廢?轉入某種歸零分錄?),留待負責人裁定。在裁定之前,預設行為是 Credit 不過期。

## 2026-09-08 補記：交件後對抗性審查修掉的四個缺陷,以及決策 4 一句用詞的更正

**這一節不推翻任何決策,它記錄實作交件後的審查結果,並訂正一個前提用詞。**

主代理在本 ADR 對應的實作批次交件後,對抗性審查在四處找出缺陷,均已修正：

- **migration 的 `CHECK (balance_credits >= -50)` 與閘 ②（決策 7）的業務下限用了同一個數字**：閘 ② 要在花錢**之前**擋,但已經花掉的成本按決策 5、7 仍必須入帳；CHECK 卡在同一個 `-50`,會讓那筆「入帳後跌破下限」的分錄連同會話快照一起被資料庫回滾,真實支出因此查無此筆。已放寬為 `CHECK (balance_credits >= -1000000)` 的防呆護欄（理由寫在 migration 註解),業務下限仍由決策 7 的應用層邏輯守。
- **`BilledMicros` 的 `usdMicros × markupBps` 是 int64 乘法**：金額超過約 7.09×10^14 微美元時會溢位成負數,再被 `ceilDiv` 的非正數守門轉成 0——一筆天文數字的成本反而被記成免費,正是決策 6「平台不因換算吃虧」要防的方向。已加上 `MaxBillableMicros`（US$1,000)上限與 `ErrAmountOutOfRange`,函式簽章改為回傳 error,三個呼叫端一律 fail closed。
- **`finish()` 在 receipt 狀態為 `unknown`（Worker 中斷、逾時、跨行程取消）時直接 return,跳過扣點**：模型呼叫已經發生、平台已經付了錢,卻不寫任何 `cost_events`／`credit_entries` 分錄——這正是決策 3「沒有對應 `cost_events` 的扣點分錄不成立」那條紀律要堵的反面情況（該有分錄卻沒有)。已補上同一把 `(session_id, revision)` 冪等鍵的結算呼叫,確保中斷路徑也入帳。
- **`store.go` 的 `PurgeUser` 註解宣稱呼叫了決策 11 保存期限對應的兩條 query,但那兩條只用 `created_at` 過濾、沒有 `user_id`**：帳號刪除當下不會清掉該帳號自己的支出紀錄,會一直留到保存期限自然到期才消失。已新增兩條 user-scoped 的 query 並更正註解。

以上四項均以鐵律 9 的還原—變紅—改回流程驗證過紅／綠證據。

**用詞更正**：決策 4 寫「一個帳號（本 MVP 即一個 Workspace,[ADR-011](./ADR-011-workspace-tenancy-policy-and-usage.md)「每位使用者一個個人 Workspace」)」的 Credit 餘額——這句把帳戶的鍵誤植為 Workspace。落地的鍵是**使用者**：`credit_accounts.user_id`,不是 workspace id。負責人裁定 2 逐字說的是「每個帳號」,帳號在這個系統裡是使用者,不是 Workspace；MVP 期間一位使用者剛好只有一個個人 Workspace,那只是巧合的基數對應,不代表兩個 id 可以互換或省略轉換這一步。`creator/creation` 既有的三個掛勾（`CreditCanStart`／`CreditReserve`／`CreditSettle`）傳的是 `WorkspaceID`,把它換成 `user_id` 是組裝根（`apiserver.NewApp`)明確要做的一步,不能假設兩個 id 相同——這個轉換今天仍未落地,見 [`docs/development/interactive-creation.md`](../development/interactive-creation.md) 2026-09-08 條目。

## 2026-09-10 補記：貨幣是 Credit，Money 是美元——以及決策 1 兩天來沒有被強制

負責人本日逐字：「**整個系統對於錢的概念都改以 Credit 作為貨幣，美金才是 Money**」。

**這不是新決策，是決策 1 的詞彙定版，外加一個查證結果。** 定版的部分：

| | 單位 | 出現在哪 | 誰看得到 |
| --- | --- | --- | --- |
| **貨幣（currency）** | **Credit** | `credit_entries`、餘額、扣點、三道閘、報酬 | **使用者** |
| **Money** | **美元** | `cost_events`、LiteLLM 閘道的 `max_budget`／spend、成本模型 | **平台自己** |

**因此本 ADR §「決策 1」裡「Credit 不是貨幣的影子」那句話，措辭與本補記相反而意思相同**，就地說明而不改寫：那句話裡的「貨幣」指的是**系統外的真實金錢**（它要說的是 Credit 不可兌現），本補記裡的「貨幣」指的是**系統內的計價單位**。**兩個詞今天起分開**——`Credit` 是這個系統的貨幣，`Money` 專指美元。

**查證結果，也是開這一則補記的真正理由**：決策 1 逐字寫著「平台面對使用者的每一個成本呈現，只有一種單位——**不顯示美元**」。那是 2026-09-08 寫的。**2026-09-10 逐條查過契約，六個欄位一個都沒有動**：

1. `RunCostEstimate.currency`（**釘死 `USD`**）＋ `low`／`typical`／`high`
2. `evaluation_usd`（評估成本）
3. Run 自己的 `usd` ＋ `is_lower_bound`
4. trace `usage.cost_usd`
5. `CreationSnapshot.budget_usd`／`reserved_usd`／`spent_usd`
6. `CreationLimits.min_budget_usd`／`max_budget_usd`

**最刺眼的是同一個畫面上今天有兩種單位**：`apps/web/src/components/CreationSession.tsx` 右上角，預算選單印的是 `$0.50`，而**緊接在它下面那一行**印的是「餘額 N 點 · 這場約 a–b 點」。決策 1 要防的正是這個。

**一條會被誤讀成「換算已經被否決過」的既有理由，寫在這裡**：`RunCostEstimate.currency` 的契約註解說「a converted number would present an exchange rate the platform does not own as a fact about a run」。**那句話對外幣成立，對 Credit 不成立**——面額 1 credit = US$0.001 與加成 1.3 都是**平台自己訂的常數**，而且決策 2 已經要求每一筆分錄記下寫入當時生效的加成。**平台不擁有的是台幣兌美元，不是自己的定價。**

**遷移規則沿用既有的，不發明新的**：換算取寫入當時的加成、無條件進位（決策 2）；`cost_events` 與閘道那一側**維持美元不動**，那一側是 Money。

**今天做不了的理由，與 [`05` R-73](../plans/05-pending-rulings.md) 是同一件**：`credit.Service` 尚未接進 `apiserver.NewApp`／`entrypoint/worker`、路由未掛，所以伺服器算不出要送出去的 credit 值。落點與逐條清單記在 [`04` 丙-231](../plans/04-backlog-and-handoffs.md)。

**接線之後建議補一條機器檢查**：使用者可達的 response schema 不得出現 `*_usd` 欄位。理由與本 repo 既有的每一條機器檢查相同——**決策 1 已經被寫下來兩天而沒有任何東西會為它出聲**，這則補記本身就是那個空缺的證據。

## 2026-09-11 補記：決策 1 的四個欄位換完了，剩下的三件套為什麼一起換

前一則補記列了六個仍以美元示人的契約欄位，並說阻塞在 `credit.Service` 沒有接線。線接上了（CRED-005 同批），四個已經換成 Credit：

| 原本 | 現在 | 換算發生在 |
| --- | --- | --- |
| `RunCostEstimate.currency`／`low`／`typical`／`high` | `low_credits`／`typical_credits`／`high_credits`（`currency` 消失，因為只剩一種單位） | `trial/execution` 的 `defaultCostEstimate` |
| `evaluation_usd` | `evaluation_credits` | `trial/improvement` 的 `costViewOf` |
| Run 自己的 `usd` | `credits` | `trial/improvement` 的 `comparisonSide` |
| trace `usage.cost_usd` | `cost_credits` | `trial/evidence` 的 `General` |

三件事值得寫下來。

**換算只有一個地方，而且是注入的。** `credit.Service.CreditsForUSD` 由兩個組裝根交給 `run`、`trace`、`eval`。前兩個 context 的 depguard 不准 import `credit`（ADR-032 附錄 A 沒有那兩列），所以注入不是風格選擇；而第三個明明可以直接 import，仍然收同一個函式，因為三個畫面各自換算就是三個會分岔的匯率。同一條理由 `GET /me/quota` 已經用過一次：顯示不重算規則。

**「換算會呈現一個平台不擁有的匯率」這句反對意見，在這裡是不成立的。** 它原本寫在 `RunCostEstimate.currency` 的欄位說明裡，而它講的是外幣：平台不決定歐元兌美元。Credit 不是外幣——US$0.001 與 1.3 倍加成都是平台自己公告的常數，決策 2 還要求每一筆分錄記下當時的加成。平台擁有這個匯率，只是不擁有那個。

**資料庫仍然存美元，這不是遺漏。** `cost_events`、`evaluations.cost_usd`、`trace_events` 的 usage payload 全部照舊，因為決策 3 說得很清楚：那是平台自己的帳，帳記在閘道計價的單位上。換算發生在回應邊界，一次，不回寫——一個被寫回資料表的換算結果會變成匯率的第二份副本，和真正扣點的那一份各自老去。

**剩下的三件套為什麼沒有一起做。** `CreationSnapshot.budget_usd`／`reserved_usd`／`spent_usd`、`CreationLimits.min_budget_usd`／`max_budget_usd`，以及 `raise_budget` 請求裡的 `budget_usd`，是同一個滑桿的顯示值、上下界與送出值。前四個是回應、最後一個是請求，拆開換就會出現「畫面講點數、請求送美元」這種一半的狀態，而那正是這次要消滅的東西。留在 [`04` 丙-231](../plans/04-backlog-and-handoffs.md)，收窄成一批。

## 2026-09-11 補記：「要不要對搜尋與試跑也收點」已裁定——搜尋不扣，試跑扣

[`05` R-74](../plans/05-pending-rulings.md) 回答了「待決策」第二項。那一項原文不改寫，答案記在這裡：

**搜尋不扣點**，而且這一半不需要改程式——`catalog` 對 `credit` 本來就只寫成本事件、不查門檻。不扣點不等於不記帳：搜尋的 `cost_events` 照樣進滾動窗。

**試跑扣點**，形狀與創作的每步結算刻意不同，差在三處：

| | 互動創作的一步 | 一次 Run |
| --- | --- | --- |
| 開始前的保留額 | 滾動窗 p95（花多少由模型回答多長決定） | 閘道上界 `max_budget`（ADR-055，平台自己設的硬上界，所以不需要估） |
| 讀不到花費時 | 扣保留額（決策 7：一步的保留額就是一次呼叫的錢） | **只記不扣**：一筆 `estimated`、0 元的成本事件。Run 的保留額是整個上界，因為讀不到自家閘道的回答就扣 650 點，錯的是平台 |
| 在哪裡結算 | 會話結算的同一交易 | Worker 的 `run_cleanup`，在撤銷 Virtual Key **之前**——撤銷之後就問不到那把 key 花了多少 |

Run 屬於 `trial/execution`，而附錄 A 沒有 `run → credit` 這一列，所以兩個動作都是組裝根注入的函式，與 `creation` 的三個掛勾同一個慣例。成本種類多了 `run`（migration 0062，兩張表的 CHECK 一起改）。

**落地時抓到的一個缺陷值得記一筆**：開始前檢查第一版經連線池讀餘額，而 `create()` 的交易此時已經拿著唯一一條連線（淨測試模式與多支整合測試的 `MaxConns=1`），於是整套測試卡死到逾時。修法是讓 `Store.Balance` 接受呼叫端的交易（`CanAffordStepIn`）。這與批次一帳號清除的死鎖是同一種形狀：**交易內的閘門不能再去要第二條連線。**

## 2026-09-11 補記：決策 9 的事件驅動那一半，以及閘 ① 一直讀錯了統計

**閘 ① 原本讀的是「一步」的 p95。** 決策 8 寫的是「擋明顯不夠跑完典型一場」，但接線時 `CanStart` 拿的是 `creation_step`——每一步結算各算一個樣本。一步的 p95 只是一場的零頭，所以這道閘實際上比決策 8 寬鬆得多。決策 9 的事件驅動摘要本來就是為了把「一場多少錢」變成一個樣本點，它沒做，閘 ① 就只能讀一步。兩件事同一批修：

- **一場一列摘要**（migration 0063 的 `cost_session_summaries`）：會話 id、總花費、步數、是否有任何一步是估計值、最後一步的時間。摘要只從 `cost_events` 彙總，不另存金額，所以它是帳的衍生物，不是第二本帳。
- **事件驅動**：所有狀態轉換都經過 `creation` 的同一個推進點，摘要就掛在那裡。會話進入 `saved` 或 `cancelled` 時寫一列，而且放在 savepoint 裡：摘要寫失敗不會連帶讓保存或取消失敗。
- **`failed` 不觸發。** `raise_budget` 可以把 `failed` 的會話帶回 `waiting_input`，所以 `failed` 不是終點。逾時也沒有任何寫入動作可以掛。這兩種都交給決策 9 的每日重算：它先把「最後一步早於會話逾時」的會話補寫或更新摘要，再算分位數。兩條路徑都用 upsert，所以取消之後才結算的那一步，會在隔天被補進總額。
- **閘 ① 改讀 `creation_session` 的 p95。** 樣本少於 20 時仍退回保守常數（決策 8 原文），所以上線初期的行為不變。
- `creation_session` **只存在於 `cost_statistics`**，不是 `cost_events` 的 kind——沒有任何一次付費呼叫叫做「一場」。0060 說兩張表的詞彙相同，從這裡起刻意不再相同。
- 摘要併入保存期限掃描與帳號清除（決策 11），和它彙總的成本事件同一個保存期限。

**同日另一件：單次生成的閘 ①。** 決策 7 的表格寫的是「建立新會話／新一次生成之前」，但生成那一半一直沒有接線。現在生成在額度檢查之後、呼叫閘道之前先查餘額，被擋下時在失敗紀錄記為 `credit`，而不是 `quota`。

## 2026-09-11 補記：創作預算在 API 邊界換算，以及三個方向各自怎麼捨入

決策 1 的最後一批：創作畫面的預算、占用與花費，連同上下界與送出的值，全部換成點數（`04` 丙-231 結案）。

- **領域與資料庫不動，換算只在 API 邊界做一次。** `creation` 的快照以 JSON 存在資料庫，欄名是 `budget_usd`／`reserved_usd`／`spent_usd`，限制也以美元設定。改領域就要遷每一列已寫下的快照；而「使用者看到什麼單位」本來就是呈現的事，和 Run／trace／評估那一批同一個做法。讀出去時（含串流）三個 `*_usd` 換成 `*_credits`、原鍵刪掉；請求帶進來的 `budget_credits` 換回美元再交給領域。
- **三個方向，各自朝使用者保守的那一邊捨入。**
  - 花費與占用換成點數：**進位**，和扣點同一條換算，畫面不會少報。
  - 使用者輸入的點數換成預算美元：**捨去**，預算的價值不會超過他輸入的點數。
  - 上限 `max_budget_credits` 取「價值不超過 `MaxCostUSD` 的最大點數」（捨去）；下限 `min_budget_credits` 取單次呼叫上限換算後的進位值。
- **一個浮點陷阱。** 點數換成美元後存成 float64，換回來時 `usd×1e6` 帶著 1e-11 量級的雜訊，無條件進位把它當成真的零頭，多算一微美元——42 點存進去、畫面回來 43 點。進位前先扣掉 1e-6 微美元的容差：真實的零頭照樣進位，只有雜訊被吸收。單元測試逐一驗 1～20000 點來回相等。這條換算也是 Run／trace／評估的顯示在用，差別只在雜訊那一位。
- **丙-231 入列時承諾的機器檢查**：公開契約逐行檢查，任何叫 `usd` 或以 `_usd` 結尾的欄位都讓測試變紅。美元只存在帳本那一側（`cost_events`、閘道），那一側是 Money。
