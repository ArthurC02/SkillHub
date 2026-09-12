# ADR-076：後台的圖表用 Chart.js，只畫不指向任何帳號的每日彙總

- 狀態：**Accepted**（2026-09-12，負責人指示：「需要圖表的查詢和展示，這個不使用套件難度較高，你可以考慮安裝如chart.js這類圖表套件，也可能有更好的你可以網路搜尋，但是要注意授權問題」）
- 日期：2026-09-12
- 相關：[ADR-074](./ADR-074-the-backoffice-is-an-operator-only-section-of-the-same-app.md)（營運後台是組裝層）、[ADR-075](./ADR-075-a-query-touches-only-its-owners-tables.md)（query 只碰擁有者的表）、[ADR-029](./ADR-029-product-analytics-events-and-audit-trace-boundaries.md) 決策 6（不做即時儀表板）、[system.md §4.8](../design/system.md)（只取技法，不取套件）、[`02` OPS-008](../plans/02-specifications-and-acceptance-criteria.md)

## 背景

- 後台七頁全是表格。成本統計只看得到每一種呼叫最新的一個統計窗，看不到逐日的變化；點數怎麼流進流出、每天建立多少 Run、operator 每天做了多少事，沒有任何彙總的讀取面。
- [system.md §4.8](../design/system.md)（2026-09-11 負責人明示）：「仍是維持不安裝額外套件」。那一條管的是視覺效果的範例庫；圖表的座標軸、刻度、數值格式、提示框與版面縮放自己寫，是一個小型繪圖庫的量，負責人這次明示可以裝套件，條件是注意授權。
- ADR-029 決策 6：「讀取面是後台查詢，**不做即時儀表板**」，理由是一直開著的畫面會產生「盯著它」的成本。
- 平台送出的 CSP 是 `script-src 'self'`＋雜湊、`style-src 'self'`（擋執行時注入 `<style>` 與字串形式的 `style` 屬性）、`connect-src 'self'`；`npm run build` 之後 `scripts/check-bundle-origins.mjs` 掃打包檔裡的絕對網址，名單外的一律讓建置失敗。
- `apps/web` 的 runtime 依賴只有 `react`、`react-dom` 與兩個 TanStack 套件。repo 沒有前端依賴的授權檢查工具。

## 決策 1：用 Chart.js（MIT），連同傳遞依賴只多兩個套件

2026-09-12 的候選調查。授權以 GitHub 上的 LICENSE 原文與 npm registry 逐層查詢（`npm view <pkg> license dependencies`，不安裝）為準：

| 套件 | 授權 | 傳遞 runtime 依賴 | 體積（min＋gzip） | 結論 |
| --- | --- | --- | --- | --- |
| **Chart.js 4.5** | MIT | 1 個（`@kurkle/color`，MIT） | 約 67 KB | **採用** |
| uPlot 1.6 | MIT | 0 | 約 21 KB | 長條圖與堆疊要另外寫繪製 |
| visx 4（選用四個套件） | MIT | 37 個（MIT／ISC／Unlicense） | 依選用 | 依賴數與目前的精簡度不相稱 |
| Recharts 3 | MIT | 11 個直接依賴（含 Redux Toolkit） | 約 144 KB | 依賴數 |
| Apache ECharts 6 | Apache-2.0 | 2 個 | 全量約 359 KB | 體積 |
| ApexCharts | npm 授權欄是「SEE LICENSE IN LICENSE」：依營收分級的雙授權 | — | — | **授權排除** |
| Highcharts | 商業 EULA，內部營運工具也要付費 | — | — | **授權排除** |
| amCharts 5 | 免費版必須在每張圖顯示其標誌 | — | — | **授權排除** |

- **不裝 React 包裝套件**：掛上與拆掉一個 Chart 實例是一個 `useEffect`，自己寫比多一個依賴便宜。
- **只註冊用到的元件**（長條圖的 controller、element 與兩個 scale），讓打包只帶這幾塊。
- **只在後台的延遲載入 chunk 裡**：一般使用者下載的程式不含它。
- 升級大版本或換套件時，重跑一次逐層授權查詢，結果寫在那個 commit。

## 決策 2：這是 §4.8 一個只限圖表的具名例外

- §4.8 其他部分不變：視覺效果仍然不裝套件。system.md §4.8 補一條指回本 ADR。
- 圖表照樣服從設計系統：
  - **顏色只從 token 讀**：畫之前以 `getComputedStyle` 讀 `--accent`、`--text`、`--border`，原始碼不出現色彩字面值；深淺色切換時重畫。
  - **一個色相，一個系列一張小圖**（small multiples）：不以顏色區分系列——§4.6.5 保留了第四個語意色相，§4.7 要求不靠顏色也讀得懂。
  - **不動畫**（§4.6.4）。
  - **每張圖旁邊有同一組數字的逐日表**：canvas 對螢幕閱讀器不透明，所以 canvas 帶 `role="img"` 與一句說明，表才是完整的讀法；圖是表的另一種看法，不是唯一的呈現。

## 決策 3：圖表是按需查詢，不是即時儀表板

與 ADR-029 決策 6 相容，不推翻它：

- 打開頁面或換時間範圍才查一次；不輪詢、不在視窗回到前景時重查、沒有推播。
- 時間範圍只有 7／30／90 天，預設 30，進網址（`?days=`）——它回答「你在看哪一段資料」（資訊架構 R4），分享出去的連結要重現同一段。
- 以 UTC 的日期分桶，畫面寫明。某一天沒有事件就是真的 0（system.md §2.9 的第一列）；整段範圍都沒有事件的種類不畫圖，列出名字說明。
- **漏斗仍不在範圍**：`analytics_events` 的漏斗儀表板是 ADR-029 決策 6 與 `02` §4.12 範圍表都排除的東西，要不要做交 [`05` R-78](../plans/05-pending-rulings.md)。

## 決策 4：只畫不指向任何帳號的彙總，每條查詢歸事實 owner

| 圖 | 表 | owner | 端點 |
| --- | --- | --- | --- |
| 每日成本（依種類，美元） | `cost_events` | `credit` | `GET /admin/trends/cost` |
| 每日點數異動（依分錄種類，淨額）＋全平台目前餘額總和 | `credit_entries`、`credit_accounts` | `credit` | `GET /admin/trends/credits` |
| 每日建立的 Run（依目前狀態） | `runs` | `run` | `GET /admin/trends/runs` |
| 每日 operator 動作（依動作） | `audit_events`；action 清單由 `apiserver` 提供，與 `OPS-006` 同一份 | `audit` | `GET /admin/trends/operator-actions` |

- 每條 query 只依日期與種類／狀態／動作分組，**不回 user id、workspace id 或 email**。它不指向任何人，所以不是 ADR-074 決策 3 所說的個資讀取，不寫 audit。
- 依帳號或工作區排行、下鑽到個別帳號是個資讀取，要不要做交 [`05` R-78](../plans/05-pending-rulings.md)。
- `SEC-011` 列為私有的內容一律不讀：Run 只取狀態與建立時間。
- 四條端點都在 `router.go` 逐條套 `RequireOperator`，列入 `authz_matrix_integration_test.go`；HTTP 與日期範圍在組裝層（`apiserver`）處理一次，事實由各 owner 的函式提供，query 登記在 `db/query-owners.yaml`。

## 影響

### 正面

- 丙-233 那種「看數字再裁定」的事，從讀一張最新窗的表，變成看得到逐日走勢。
- 點數與 Run 第一次有全平台的彙總讀取面，而且不必打開任何一個帳號。

### 成本與限制

- 多一個 runtime 依賴、約 67 KB，只在後台 chunk 裡；升級時要重查授權。
- canvas 對輔助科技不透明，完整讀法靠旁邊的表；表因此是規格的一部分，不是裝飾。
- UTC 分桶在台灣時間早上八點換日；一天的邊界與 operator 的直覺差八小時，畫面必須寫明。
- Run 依「目前」狀態分組：昨天建立、今天才結束的 Run，昨天那一格的狀態會跟著變。這是一張「那天建立的 Run 現在怎麼樣」的圖，不是狀態轉移的歷史。

## 待決策

- 漏斗儀表板要不要做，以及依帳號／工作區的排行與下鑽要不要做：[`05` R-78](../plans/05-pending-rulings.md)。
