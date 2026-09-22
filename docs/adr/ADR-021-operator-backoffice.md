# ADR-021：營運後台

- 狀態：Accepted
- 相關：[ADR-006 身分、Workspace、准入與額度](./ADR-006-identity-workspace-admission-and-allowances.md)、[ADR-010 產品分析與稽核邊界](./ADR-010-product-analytics-and-audit-boundaries.md)、[ADR-016 Platform Bounded Context 與 Context Map](./ADR-016-platform-bounded-contexts-and-context-map.md)、[ADR-017 Query 與寫入所有權](./ADR-017-query-and-write-ownership.md)、[ADR-020 設計系統、信任訊號與畫面用語](./ADR-020-design-system-trust-signals-and-screen-words.md)、[ADR-022 淨測試模式](./ADR-022-clean-test-mode.md)

## 背景

operator 能做的事窮舉在 `02` SEC-011：授予點數、設定／解除受限展示、再散布判定、跨工作區下架、派送煞車的讀取／宣告／解除。這些動作各自的端點都在 `contracts/openapi/public.yaml`，`router.go` 逐條套 `RequireOperator`，但沒有畫面——過去唯一的操作方式是直接下 SQL 找 id、再用 curl 打端點。

封測開始後，這幾件事會變成高頻操作：逐人授予報酬點數、替點數不夠的人查帳、事故時停止派送。單純的 curl 流程有兩個缺口：授予點數要帶 `workspace_id`，卻沒有端點能從 email 找到它，只能直接查資料庫；operator 自己做過的事雖然記在 `audit_events`，但唯一的讀取 query 限定在單一 workspace，看不到全平台的操作紀錄。查 id 那一步也完全沒有稽核痕跡。

後台必須解決這個操作缺口，但不能因此擴大 operator 的權力範圍，也不能違反既有的 Bounded Context 邊界——後台上的每一項事實（點數、Skill 治理狀態、Run 派送、身分與名冊、操作紀錄）都已經有明確的擁有者。

## 決策

### 決策 1：後台是 `apps/web` 裡的 `/admin/*`，不另開 app、不新增登入方式

沿用同一個 SPA、同一個登入 session、同一份契約生成的型別；後台路由整組延遲載入，一般使用者下載的程式不含它。

`GET /me` 多一個頂層欄位 `operator`，只告訴本人「你是不是 operator」，前端據此決定要不要畫出後台入口。**這個欄位只決定畫不畫，不是授權**：真正擋人的是每一條 `/admin/...` 端點各自的 `RequireOperator`，member 直接輸入 `/admin/*` 網址得到的是與不存在網址相同的頁面。

不選獨立後台 app：會多一套建置、部署、登入與設計系統，而權力已經由端點守住，隔離換到的東西不多。不選 CLI：解決不了查 id 和看分錄，而且只有會開終端機的人能用。不用資料庫管理工具直連 Postgres：會繞過 `RequireOperator` 與 audit。

### 決策 2：有了畫面，operator 的權力邊界不變

- operator 能做的事，仍然只有 SEC-011 窮舉的那幾項；後台每一顆寫入按鈕都對應一條既有或依 SEC-011 新增的端點，畫面本身不發明新動作。
- 每一次寫入都要填理由（空字串不成立），並在同一個交易裡寫一筆 audit，與過去用 curl 時相同。
- SEC-011 列為私有的資料，後台一律不讀：Fork 內容、Test Case、Dataset、Run 明細、Trace、Artifact、下載紀錄。

### 決策 3：後台的讀取範圍，個資讀取逐次記稽核

後台目前提供以下讀取與對應寫入（需求 ID 見 `02` §4.12 `OPS-001`～`OPS-008`）：

| 頁面 | 內容 | 事實 owner |
| --- | --- | --- |
| 帳號與點數 | 以 email 找帳號（user id、workspace id、顯示名稱、建立時間、刪除狀態、是否在封測名單）；查詢並授予某 workspace 的點數餘額與最近分錄 | `creator/workspace`、`creator/credit` |
| Skill 治理 | 以 id 或名稱找 Skill，查看受限展示、再散布判定、下架狀態，並執行既有動作 | `catalog` |
| 派送煞車 | 讀取、宣告、解除派送煞車狀態 | `run` |
| 名冊 | 唯讀顯示目前生效的 operator 名冊與封測名單 | `identity` |
| operator 動作紀錄 | 全平台範圍內、只列 operator 做過的動作 | `audit`（Generic） |
| 成本統計 | 依種類彙總的呼叫成本統計（不含使用者維度） | `creator/credit` |
| 趨勢圖 | 成本、點數異動、Run、operator 動作四組每日圖，各自歸事實 owner，見決策 9 | `creator/credit`、`run`、`audit`（Generic） |

以 email 找帳號、查詢點數分錄這兩種是個人資料讀取：**每查一次就在同一個交易裡寫一筆 audit，記下誰查了誰**。有這筆紀錄，才接受 operator 看得到別人的 email 與點數；這是條件，不是加分項。

後台明確不做：編輯 operator 名冊或封測名單（見決策 4）；任何 SEC-011 列為私有的資料；濫用檢舉案件的處理（需要另立需求與授權）；下架後的恢復；精選層的寫入（端點存在，由內容工具呼叫）。這些要嘛超出 operator 現有權力，要嘛對應端點尚未存在，要嘛是種入流程的一步而不是日常營運。

### 決策 4：operator 名冊與封測名單仍在部署設定，後台只顯示

`OPERATOR_USER_IDS`（operator 名冊）與 `BETA_ALLOWLIST`（封測名單）照舊改設定再重啟，後台只唯讀顯示目前生效的內容。一個能授予角色的端點，唯一的用途就是讓帳號提拔自己；有了後台不改變這個風險，所以名冊不開放後台寫入。若封測期間人員異動頻繁到部署重啟的代價太高，可以另案把封測名單（或 operator 名冊）改成資料表，但仍歸身分與准入的擁有者（[ADR-006](./ADR-006-identity-workspace-admission-and-allowances.md)）。

### 決策 5：淨測試模式下沿用同一套規則，並在畫面上說明

淨測試模式下任何人都能以 operator 身分登入（[ADR-022](./ADR-022-clean-test-mode.md)）。後台在這個模式裡不會多給任何權力——那些端點本來就存在——所以照常可用；每一頁頂端固定一行字，說明「這個模式任何人都能以 operator 登入」。

### 決策 6：`/admin/*` 是依角色分岔的第三種網址前綴

`/admin/*` 不進主導覽列（導覽列只放使用者自己的東西），入口放在帳號選單裡、只有 `operator` 為真時顯示；後台自己有一條側邊導覽。這是「清單網址由誰在問決定」第一次由角色而非帳號本身決定：同一個帳號，`GET /me` 的 `operator` 一旦翻值，眼前就多一組位址。`/admin/*` 底下沒有單筆位址——找帳號、找 Skill 都是清單頁上的查詢參數，不是路徑段。

圖表頁（趨勢圖）的時間範圍只有 7／30／90 天、預設 30，進網址 `?days=`，讓分享出去的連結重現同一段資料；帳號頁查詢用的 email 因為是他人的個人資料，刻意不進網址，只留在輸入框裡。

### 決策 7：後台是組裝層，不是獨立的 Bounded Context

判準是「誰擁有事實、不變量與公開協作面」。後台上的每一項都能對應到既有 owner：點數（授予、餘額、分錄、成本統計）屬於 Credit Ledger；受限展示、再散布判定、跨工作區下架屬於 Catalog；派送煞車屬於 Run Orchestration（Run 狀態機的一部分）；帳號查詢、名冊、`RequireOperator` 屬於 Identity & Workspace；operator 動作紀錄屬於 Generic 的 audit。**operator 在這個模型裡是另一種使用者，不是另一個領域**：同一個 Skill、同一本帳，只是換一個權力不同的人操作。

因此：

- operator 端點放在擁有那項事實的 context；各讀取歸各自 owner，新 query 登記在 `db/query-owners.yaml`，不需要例外清單。
- 授予點數的 handler 可以留在 `apiserver` 做轉譯，只要理由檢查、冪等與 audit 都留在 `credit` 的 Service 裡。
- 一個畫面要同時顯示多個 context 的東西時（例如帳號頁同時有身分與點數），由前端分別呼叫各條端點再拼起來，不在後端新增聚合端點；這樣每條端點只有一個 owner，授權也只在一個地方。
- operator 動作紀錄的過濾條件（哪些 action 算 operator 動作）由組裝層（`apiserver`）依它掛了哪些 operator 路由提供；`audit` 本身不知道這份清單。
- 不新增套件、不新增跨 context 的 import；架構 identity 的兩個家是經審查的 Domain Memory Registry 與 `apps/platform/architecture-identity.yaml`，後台不需要在那裡登記新項目。

一次全平台的 query 稽核，逐條比對每條 query 實際碰到的表（見 [ADR-017](./ADR-017-query-and-write-ownership.md)），確認後台上的每一項事實都已有明確擁有者，沒有一條需要移動 context 邊界。切 context 的依據是語言、資料、規則與組織是否不同，不是畫面或角色：operator 用的是與一般使用者相同的詞彙、同一個團隊、同一組規則，四個訊號一個也沒出現。

真正會讓後台長出自己 Bounded Context 的條件，是它擁有了自己的事實與生命週期，而不只是替別人的事實開一扇門——最可能的候選是濫用檢舉案件（檢舉、分派、裁決、執行、申訴自成一套流程），屆時它會是一個新的 Supporting context，透過 Catalog 公開的下架 API 執行結果。

### 決策 8：圖表用 Chart.js，是設計系統「不裝套件」規則的具名例外

趨勢圖頁需要座標軸、刻度、提示框與版面縮放，自己寫是一個小型繪圖庫的量。經授權與依賴逐層調查（MIT／傳遞依賴數／體積），採用 **Chart.js**（MIT，連同傳遞依賴只多兩個套件——Chart.js 本身與 `@kurkle/color`，皆為 MIT，約 67 KB min+gzip）。ApexCharts（依營收分級雙授權）、Highcharts（商業 EULA）、amCharts（免費版強制品牌標誌）三者因授權排除；Recharts、visx、Apache ECharts 因傳遞依賴數或體積不採用；uPlot 體積更小但長條圖與堆疊要另外寫繪製，不採用。

- 不裝 React 包裝套件：掛上與拆掉一個 Chart 實例是一個 `useEffect`。
- 只註冊用到的元件（長條圖 controller、element、兩個 scale），讓打包只帶這幾塊；只在後台的延遲載入 chunk 裡出現，一般使用者不下載。
- 升級大版本或換套件時，重跑一次逐層授權查詢。
- 這是設計系統「視覺效果不裝套件」規則（[ADR-020](./ADR-020-design-system-trust-signals-and-screen-words.md)）一個只限圖表的具名例外，其餘部分不變。圖表仍服從設計系統其餘規則：顏色只從 CSS token 讀（不出現色彩字面值，深淺色切換時重畫）；一個色相、一系列一張小圖，不靠顏色區分系列；不做動畫；每張圖旁邊附同一組數字的逐日表——canvas 對螢幕閱讀器不透明，表才是完整的讀法，圖只是另一種看法。

### 決策 9：趨勢圖是按需查詢，不是即時儀表板，只畫不指向帳號的彙總

- 打開頁面或換時間範圍才查一次；不輪詢、不在回到前景時重查、沒有推播——與「讀取面是後台查詢、不做即時儀表板」的產品分析原則相容（[ADR-010](./ADR-010-product-analytics-and-audit-boundaries.md)）。
- 以 UTC 分桶，畫面寫明；某一天沒有事件就是真的 0，整段範圍都沒有事件的種類不畫圖、只列名字。
- 四組每日圖，各自查詢歸事實 owner，不回 user id、workspace id 或 email，因此不算個資讀取、不寫 audit：
  - 每日成本（依種類）——`credit`
  - 每日點數異動（依分錄種類的淨額）＋全平台目前餘額總和——`credit`
  - 每日建立的 Run（依目前狀態）——`run`
  - 每日 operator 動作（依動作，動作清單與 operator 動作紀錄同一份）——`audit`
- Run 依「目前」狀態分組，是一張「那天建立的 Run 現在怎麼樣」的快照，不是狀態轉移的歷史；今天結束的 Run 會讓昨天那一格的狀態跟著變。
- 依帳號或工作區排行、下鑽到個別帳號，以及漏斗儀表板，都不在後台範圍內（見待決策）。

## 影響

### 正面

- 高頻的 operator 操作（逐人授予點數、查帳、事故時停止派送）從「先下 SQL 找 id、再手打 curl」變成有稽核的介面，查 id 這一步第一次有 audit。
- 點數與 Run 第一次有全平台的彙總讀取面，不必打開任何一個帳號就能看出走勢。
- 契約與授權測試同步收斂：新讀取端點依 OpenAPI-first 先進 `public.yaml`，`authz_matrix_integration_test.go` 逐條比對路由表，新路由忘了套 `RequireOperator` 會直接變紅。

### 成本與限制

- 多一個 runtime 依賴（Chart.js，約 67 KB），只在後台 chunk 裡；升級時要重查授權。
- canvas 對輔助科技不透明，完整讀法要靠旁邊的逐日表，表因此是規格的一部分而非裝飾。
- UTC 分桶與台灣時間有時差，一天的邊界在畫面上必須寫明，否則會跟 operator 的直覺對不上。
- 個資讀取（以 email 找帳號、查點數分錄）每查一次都要寫入 audit，比純讀取多一個交易寫入。

## 待決策

- 是否要做漏斗儀表板：`analytics_events` 的漏斗屬於產品分析範圍排除的項目（[ADR-010](./ADR-010-product-analytics-and-audit-boundaries.md)），要不要為後台另外開放，見 [`05` 待裁定清單](../plans/05-pending-rulings.md)。
- 是否要做依帳號或工作區的排行與下鑽：這會是新的個資讀取面，開放與否、要不要逐次寫 audit，同樣待裁定。
- 同意書是否要揭露「營運人員看得到你的 email 與點數紀錄」，待法務確認。
