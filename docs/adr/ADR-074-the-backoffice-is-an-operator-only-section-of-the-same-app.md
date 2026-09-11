# ADR-074：營運後台是同一個前端裡、只有 operator 進得去的一組畫面

- 狀態：**Proposed**（2026-09-12 起草，等 [`05` R-77](../plans/05-pending-rulings.md) 裁定）
- 日期：2026-09-12
- 相關：`02` SEC-011（operator 角色、窮舉動作、不讀私有資料）、`02` CRED-007（operator 授予是唯一的入帳入口）、[ADR-061](./ADR-061-the-clean-mode-release-lives-on-the-keyboard-not-in-the-product.md)（淨測試模式任何人都能以 operator 登入）、[ADR-029](./ADR-029-product-analytics-events-and-audit-trace-boundaries.md) 決策 6（分析不做即時儀表板）、[資訊架構](../design/information-architecture.md) §0

## 背景

Credit 收尾時（`05` R-73～R-76），負責人說：「從這個需求，意識到該有一個BackOffice。」2026-09-12 盤點現況如下。

- operator 能做的事有六條端點，都寫在 `contracts/openapi/public.yaml`，也都由 `router.go` 逐條套上 `RequireOperator`：授予點數、授權受限展示（設定／解除）、再散布判定、跨工作區下架、派送煞車（讀取／宣告／解除）。
- **沒有任何畫面**：`apps/web` 沒有一處呼叫 `/admin`。操作方式是 `02` SEC-011 寫的「單人團隊以 curl 直接操作，不做管理 UI」，以及 [p1-dispatch-halt](../runbooks/p1-dispatch-halt.md) 裡的 curl。
- **有端點也做不完的事**：
  - 授予點數要帶 `workspace_id`，但沒有端點能從 email 找到它，只能直接查資料庫；授予之後，也沒有地方看那個帳戶的分錄。
  - operator 自己做過的事記在 `audit_events`，但唯一的讀取 query（`ListWorkspaceAuditEvents`）限定在單一 workspace。
  - `04` 丙-233 要觀察評估成本，靠的是一段貼進 psql 的 SQL。
- 兩份名冊都在部署設定裡，改了要重啟：`OPERATOR_USER_IDS`（operator）與 `BETA_ALLOWLIST`（封測名單）。

一個人偶爾操作一次時，curl 夠用。封測開始後會有三件事：逐人授予報酬點數（R-73）、替點數不夠的人查帳、事故時停止派送。它們的頻率和壓力，會讓「先下 SQL 找 id、再手打 curl」變成錯誤的來源，而查 id 那一步沒有 audit。

## 決策（提案）

### 1. 後台放在 `apps/web` 的 `/admin/*`，不另開 app

沿用同一個 SPA、同一個登入 session、同一份契約生成的型別，後台路由整組延遲載入。**不新增 app、不新增部署單位、不新增登入方式。**

`GET /me` 多一個欄位，只告訴本人「你是不是 operator」，前端靠它決定要不要畫出後台入口。**真正擋人的仍是每條端點上的 `RequireOperator`**：前端的判斷只決定畫不畫，不是授權。member 直接輸入 `/admin/*` 網址，看到的是一般的 404 頁。

### 2. 有了畫面，權力也不會變大

- operator 能做的事，仍然只有 SEC-011 窮舉的那幾項。**後台每一顆寫入按鈕，都對應一條既有的端點，或同一批依 SEC-011 新增的端點**；畫面不發明新動作。
- 每一次寫入仍要填理由（空字串不成立），並寫一筆 audit，跟今天用 curl 一樣。
- SEC-011 列為私有的資料，後台一律不讀：Fork 內容、Test Case、Dataset、Run、Trace、Artifact、下載紀錄。

### 3. 後台只新增四種讀取

| 讀什麼 | 為什麼需要 | 由誰擁有 |
| --- | --- | --- |
| 以 email 找帳號：user id、workspace id、顯示名稱、建立時間、刪除狀態、是否在封測名單 | 授予點數之前要先找到帳號；今天只能查資料庫 | `creator/workspace` |
| 單一 workspace 的點數餘額與分錄 | 授予之後要能確認；使用者問「點數去哪了」時也要查得到 | `creator/credit` |
| operator 動作紀錄：全平台，但只列 operator 做的動作 | 知道誰在何時、以什麼理由做了什麼；今天只能查資料庫 | audit 的擁有者 |
| 成本統計（`cost_statistics`，不含使用者維度） | 丙-233 要看的數字 | `creator/credit` |

前兩種是個人資料，**每查一次就寫一筆 audit，記下誰查了誰**。有這筆紀錄，才能接受 operator 看得到別人的 email 與點數；它是條件，不是加分項。前兩種要不要開、開到哪裡，由 R-77 裁定。

### 4. 名冊仍在部署設定，後台只顯示

operator 名冊與封測名單，照舊改設定再重啟。SEC-011 與 ADR-061 選這個做法的理由沒有因為有了後台而消失：一個能授予角色的端點，唯一的用途就是讓帳號提拔自己。後台只顯示目前生效的兩份名冊。

### 5. 淨測試模式照常可用，但每頁說明

淨測試模式開著 `DEV_LOGIN=1`，任何人都能以 operator 身分登入（ADR-061）。那六條端點本來就在，後台在這個模式裡不會多給任何權力，所以照常可用；只是每一頁頂端固定一行字，說明「這個模式任何人都能以 operator 登入」。

### 6. 資訊架構

`/admin/*` 是第三種網址前綴。R2 說清單網址由「誰在問」決定，這裡的「誰」是 operator。

- 不進主導覽列：R7 規定導覽列只放「我的東西」。
- 入口放在帳號選單裡，只有 operator 看得到；後台自己有一條側邊導覽。
- 做法是在 §0 新增一條規則，不是在 §0.2 偏離帳加一列。

### 7. 在 DDD 裡的位置：後台不是一個 Bounded Context

判準照 [platform-ddd-practices](../development/platform-ddd-practices.md)：Bounded Context 是「事實、不變量與公開協作面的 owner」。把後台上的東西逐項對過：

| 後台上的東西 | 事實與不變量屬於誰（ADR-032 §1） | 為什麼不是後台的 |
| --- | --- | --- |
| 授予點數、餘額、分錄、成本統計 | Credit Ledger（`credit`） | 餘額等於分錄之和、理由必填、冪等，這些都是帳本的規則 |
| 限制展示、再散布判定、跨工作區下架 | Catalog（`catalog`） | SEC-011 規定下架與 `CONTENT-009`／`INGEST-010` 共用同一流程、同一組狀態，不得為 operator 另開第二套 |
| 派送煞車 | Run Orchestration（`run`） | 煞車是 Run 狀態機的一部分（鐵律 5） |
| 帳號查詢、operator 名冊、封測名單、`RequireOperator` | Identity & Workspace（`identity`） | 誰是誰、誰有什麼角色，本來就歸這裡 |
| operator 動作紀錄 | `audit`（Generic） | 只是依 action 名稱過濾的讀取，不含任何規則 |

對完一項都不剩。**operator 在這個模型裡是另一種使用者，不是另一個領域**：同一個 Skill、同一本帳，只是換一個權力不同的人來操作。

如果另立一個「後台 context」，它只可能是兩種樣子：一是只會轉呼叫各 owner 的空殼；二是直接寫別人的表。第二種正是 SEC-011「不得另開第二套」和 ADR-033 的 query ownership 會擋下的形狀。

因此：

- **operator 端點放在擁有那項事實的 context**，沿用現有做法（`skill/discovery/restriction.go`、`trial/execution/halt.go`）。決策 3 的四種讀取，各歸上表的 owner；新 query 登記在 `db/query-owners.yaml`，不需要 `allow:` 例外。
- **授予點數的 handler 可以留在 `apiserver`**：它目前在那裡，跟另外兩個 context 的做法不同。但它只做轉譯，再呼叫 `credit` 的 Service；理由檢查、冪等和 audit 都在 `credit` 裡，所以不必搬。
- **後台是組裝層**：
  - 後端由 `entrypoint/api/apiserver` 逐條掛上 `RequireOperator`；前端是 `apps/web` 的 `/admin/*`。
  - 一個畫面要同時顯示多個 context 的東西時（例如帳號頁同時有身分與點數），**由前端分別呼叫各條端點再拼起來**，不在後端新增聚合端點。這樣每條端點只有一個 owner，授權也只在一個地方。
- **operator 動作紀錄的過濾條件由組裝層傳入**：`audit` 是 Generic，不應該自己知道哪些 action 算 operator 動作。這份清單由 `apiserver` 依它掛了哪些 operator 路由來提供。
- **ADR-032 §1 不用改**：不新增套件、不新增 Boundary ID、不新增跨 context 的 import。產品語言上也不新增價值流（ADR-038）：那些價值流描述的是創作者能完成的事，operator 只是替它們開一扇門。

**什麼時候才會長出一個真正的 context**：當後台有了自己的事實與生命週期，而不只是替別人的事實開一扇門。

- 最可能的是**濫用檢舉案件**，SEC-011 說它要另立需求與授權。它有自己的流程：檢舉、分派、裁決、執行，還可以申訴。
- 那會是一個新的 Supporting context，擁有案件與裁決，再透過 Catalog 公開的下架 API 執行結果；它是下游的 Customer。屆時照 ADR-032 §1 先登記，再建目錄。

另外兩件常被當成後台的事，其實各有歸屬：

- 把 operator 名冊改成資料表，仍然歸 `identity`。
- 接真實金流，歸 `credit`，外加一層 payment 防腐層（ADR-032 §2 已經預留）。

## 第一批範圍（提案）

1. 後台外殼，以及 `GET /me` 的 operator 欄位。
2. 帳號與點數：用 email 找帳號，看餘額與分錄，再授予點數（填金額與理由）。
3. Skill 治理：用 id 或名稱找 Skill，看目前的受限、再散布與下架狀態，並提供三個既有動作。
4. 派送煞車：看目前狀態，宣告與解除。

第二批：operator 動作紀錄、成本統計。

**不在範圍內**：

- 編輯 operator 名冊或封測名單；
- 任何私有資料；
- 漏斗儀表板（ADR-029 決策 6）；
- 濫用檢舉的案件處理：SEC-011 要求它另立需求與授權；
- 下架後的恢復：`04` 丙-80 的恢復那一半還沒有端點；
- 精選層的寫入（丙-77）。

最後兩項等端點做好再加畫面。

## 影響

- **規格與文件**：本份取代 `02` SEC-011 裡「不做管理 UI」那一句。新需求 ID 與允收準則，在 R-77 裁定後補進 `02`／`03`，`01` §7 同步更新。需求 ID 擬用前綴 `OPS`，根 `AGENTS.md` 的前綴清單要一起改。
- **契約與授權測試**：新的讀取端點照 OpenAPI-first，先寫進 `public.yaml`。`authz_matrix_integration_test.go` 會讀路由表逐條比對，新路由忘了套 `RequireOperator` 就會紅。
- **Query 擁有權**：新 query 各歸原本的擁有者，登記在 `db/query-owners.yaml`；不新增 Bounded Context。
- **資訊架構文件**：`information-architecture.md` §0 新增一條規則，§1 路由表與 §2.4 條件入口一併更新，`ia.test.ts` 同步。

## 考慮過的替代方案

- **獨立的後台 app**（另一個 SPA、另一個網址，或只放在內網）：隔離較好，但要多一套建置、部署、登入與設計系統。單人團隊、而且權力已經由端點守住，隔離多換到的東西不多。日後如果有不寫程式的營運人員，或後台必須放在內網，再重新討論。
- **CLI**（把 curl 包成命令）：能避免打錯字，但解決不了查 id 和看分錄，而且只有會開終端機的人能用。
- **用資料庫管理工具直接連 Postgres**：會繞過 `RequireOperator` 和 audit，正是 SEC-011 當初要消滅的「沒有授權檢查、沒有稽核紀錄」的那條路。
