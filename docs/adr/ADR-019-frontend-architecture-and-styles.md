# ADR-019：前端架構與樣式分層

- 狀態：Accepted
- 相關：[ADR-013](./ADR-013-repository-layout-ci-and-verification-tiers.md)（頂層目錄收納語意、真實瀏覽器驗證層）、[ADR-020](./ADR-020-design-system-trust-signals-and-screen-words.md)（設計系統與 token 規則，決策 6 與其銜接）

## 背景

`apps/web/src` 需要一套讓「這個子元件還有誰在用」「這一頁的資料從哪裡來」可以只靠目錄結構回答的分法，否則這兩個問題只能靠全 repo 搜尋。約束包括：Coding Agent 與人類都要能沿路徑判斷歸屬，不必讀懂邏輯；伺服器狀態（fetch、快取、失效）與畫面狀態必須可分離追蹤，否則寫入之後哪些畫面該重抓、有沒有重抓，只能一頁一頁翻程式碼確認；樣式表若不分層，會隨頁數線性膨脹成單一巨檔，兩個 Writer 改兩頁也會衝突到同一份檔案，元件刪除後死樣式也不會被發現。

## 決策

### 決策 1：四個區，import 只往內或往下

| 區 | 放什麼 | 可以 import |
| --- | --- | --- |
| `app/` | 組裝：`App.tsx`、`router.tsx`、`shell/`（頁首身分、回報入口、淨模式橫幅） | 自己、`core/`、`shared/`、各 feature 的 `*.page.tsx` 與 `index.ts` |
| `features/<name>/` | 一個功能的頁面、子元件、service 與 model | 自己、`shared/`、`core/`；別的 feature **只能經過它的 `index.ts`**；`app/router.tsx` 只能 `import type`（路由 search 參數的形狀） |
| `shared/` | 跨功能重用、自己不擁有伺服器資料的東西：`ui/`（`Loading`、`Timestamp`、`LoginRequired`……）、`format.ts` | `shared/`、`core/` |
| `core/` | 沒有畫面的單例：`api/`（`client`、`queryClient`、`queryKeys`、與契約對照的 `types.ts`）、`session/`（登入身分、點數） | 只有 `core/` |

另有兩個非產品程式的區：`guards/`（量整個 app 的尺）、`testing/`（fixtures）。`src/` 根層只留 `main.tsx` 與樣式表入口。

八個 feature 各自對到使用者的一段旅程：`catalog`（首頁、Skill 比較）、`skill`（Skill 詳情、檔案）、`creation`（建立、匯入、生成）、`lab`（Test Case、Dataset、執行前確認）、`runs`（Trace、Run 比較、Run 歷史、評估）、`packaging`（打包、下載）、`workspace`（我的 Skill、帳號、資料政策）、`admin`（營運後台）。

feature 之間強制經過 `index.ts`：一個 feature 對外提供什麼要一眼看完，改動內部也不會意外弄壞別人；`index.ts` 裡只准出現 `export { … } from "./…";` 這一種句子。平台共用、跨 feature 都在用的型別（例如匯入結果的形狀）歸位到 `core/api/types.ts`，不是任何單一 feature 的私有物。

### 決策 2：feature 內的角色，看檔名就知道

| 位置與檔名 | 角色 | 規則 |
| --- | --- | --- |
| `*.page.tsx` | 頁面：有位址的畫面，持有這一頁的 UI 狀態，組合子元件 | 只有 `app/` 可以 import；**每一個都必須被路由掛載**，沒有位址的就不是頁面 |
| `<頁面資料夾>/components/*.tsx` | 子元件：只屬於那一頁 | 只能被同一個頁面資料夾裡的檔使用 |
| feature 根層的 `components/*.tsx` | feature 內多頁共用，或經 `index.ts` 提供給別的 feature 的元件 | 只能被這個 feature 使用，對外一律經過 `index.ts` |
| `*.service.ts` | 伺服器狀態：查詢、寫入，以及寫入後讓哪些資料失效 | 見決策 3；不含畫面，不 import 任何 `.tsx` |
| `*.model.ts` | 型別與純函式：標籤表、閘門判斷 | 不需要 React 就能測 |
| `index.ts` | 這個 feature 對外提供什麼 | 只有 export 清單 |

子元件可以直接呼叫自己 feature 的 service：寫入的處理中與錯誤狀態通常只有顯示它的那個元件用得到，一路用 props 往下傳只會讓大頁面更難讀，而快取鍵去重保證同一份資料不會被重覆抓取。

沒有位址、但擁有一群只屬於它的子元件的區塊（例如評估面板、生成流程的一段），各自成為一個元件資料夾，子元件放在該資料夾的 `components/` 下，純邏輯放進同名的 `.model.ts`。

### 決策 3：伺服器狀態只住在 service 層

- react-query 的 hook 只出現在 `*.service.ts`，以及建立 `QueryClient` 的 `core/api/`；其餘位置一律不 import react-query 的 hook。
- **每一個寫入都是 service 裡的一個 hook，並且自己宣告讓哪些資料失效**（`onSuccess`／`onSettled`）——「這個動作會讓什麼重抓」的答案只有一處。元件對寫入結果的畫面反應（顯示一句話、關掉確認框、導頁）留在呼叫端的 `mutate(vars, { onSuccess })`；失效的責任在 hook。
- **每一個快取鍵都由 `core/api/queryKeys.ts` 產生**。鍵是路徑，讓一個鍵失效會連帶讓所有以它為前綴的鍵失效；鍵的值本身視為既有行為的一部分，不可隨意改動。快取鍵集中一份、不拆給各 feature，因為跨 feature 的失效需要看得到別人的鍵。
- **伺服器回應不複製進 `useState`**：寫入的結果與錯誤直接讀 `mutation.data`／`mutation.error`；「換了參數就重來」用 `key` 讓元件重新掛載，不用 effect 逐一清 state。例外是使用者自己在這一頁的操作紀錄（例如這次造訪上傳過的檔案清單、還沒採納也還沒忽略的建議）——那不是伺服器資料的副本，可以留在 `useState`；以 effect 同步網址的輸入框（例如搜尋草稿）也不套用「換參數即重掛」，否則會搶走焦點。
- **`retry: false` 只寫在 `core/api/queryClient.ts`** 這一處預設值。

### 決策 4：測試跟著被測物走

- 測某個功能的測試放在該 feature 的根層，檔名與功能一致。
- 測整個 app 流程的測試（App、critical-flows、淨測試模式）放在 `app/`；App 外殼（回報入口等）的測試放在 `app/shell/`。
- 量所有頁面的尺放在 `guards/`：架構、資訊架構、設計系統、無障礙、對比、契約、不受信任文字，以及跨頁的 session／401 規則；無障礙的標題樹快照放在 `guards/` 底下。
- fixtures 放在 `testing/fixtures/`。

### 決策 5：樣式表只有兩種位置

| 位置 | 放什麼 | 誰載入 |
| --- | --- | --- |
| `styles/tokens.css` | `:root` 的設計 token，以及暗色與減少動態時的覆寫 | `main.tsx`，第一個 |
| `styles/base.css` | HTML 元素本身的樣式（標題、連結、表單、表格、`details`），以及讓連結、檔案選擇器看起來像按鈕的控制層 | `main.tsx`，第二個 |
| `styles/layout.css` | 每一頁都在裡面的外框：頁首、導覽、頁尾、`main` 的欄寬 | `main.tsx`，第三個 |
| `styles/patterns.css` | 跨 feature 的樣式詞彙：badge、note、notice、清單、卡片、動作 | `main.tsx`，第四個 |
| `<元件>.css` | 只屬於那個元件的特化，和元件同目錄、同名（`X.page.tsx` 對 `X.page.css`） | 只有旁邊那個 `.tsx`，`import "./X.css"` |

四層的載入順序即層疊順序；元件樣式表跟著元件所在的 chunk 載入，延遲載入的頁面連同自己的樣式一起下載。不用 CSS Modules，也不加依賴——class 名稱全部不變（測試與 e2e 靠 class 找元素），打包工具原生支援這種 import。

### 決策 6：Token 與色彩字面值只在 `styles/tokens.css`

`:root` 上的設計 token 只宣告在 `tokens.css`；任何色彩字面值（含自訂屬性的值）也只能出現在那裡。對比檢查與色彩檢查只讀這一份檔案，元件樣式表若自訂一條色彩相關的自訂屬性，會完全逃過這兩道檢查。配方或元件自己的區域變數可以宣告在它自己的選擇器上，但值只能引用 token 或長度，不可以是新的色彩字面值。

### 決策 7：元件樣式表不得外洩

CSS 沒有範圍：元件樣式表裡若寫一條裸的全域 class，那一頁載入之後每一頁的同名 class 都會被改掉，而且哪一頁先載入決定畫面長什麼樣。因此元件樣式表的**每一個選擇器都要含有一個只有這個資料夾在用、別的樣式表都沒提到的 class 或 id**；在這個範圍內可以特化全域 class（例如 `.filter-bar .note`），但不能單獨改全域 class。同一資料夾的父子元件可以共用一份樣式表。

### 決策 8：全域層只放共用的東西，例外要登記且只能變短

全域四層裡若某個 class 只有一個資料夾在用，必須在守衛裡登記為具名例外，並寫明它與哪一條全域規則共用同一個配方；名單只能變短，其餘這類 class 一律搬到元件旁邊。

## 影響

### 正面

- 目錄本身回答「誰擁有它」：子元件屬於它的頁面資料夾，service 屬於它的 feature，跨 feature 共用一律看得到 `index.ts`。
- 資料新鮮度變成可檢查的規則而不是各自記憶：一個寫入是否讓對的資料失效，答案就在該 service hook 旁邊。
- 全域樣式層不再隨頁數膨脹；元件刪除時它的樣式表會一併刪除；兩個 Writer 改兩頁不會再衝突到同一個樣式檔。
- 行為不因分層而改變——搬遷本身不觸碰任何邏輯或畫面。

### 成本與限制

- **「這個 state 是不是伺服器資料的副本」沒有機器可判，只能靠審查**——決策 3 的 `useState` 例外清單天生需要讀懂語意才能判斷。
- 層疊順序變成「四層順序 ＋ chunk 載入順序」：元件樣式表永遠排在全域四層之後，同特異度時元件規則會贏過全域規則，新規則需要自行留意有沒有翻轉既有畫面。
- 全域四層是跨頁的尺，其變更屬於協調者層級的責任，不是任一 feature 能單方決定。
- 守衛只認得出現在字串字面值（含樣板字串前綴）裡的 class；完全由變數動態組出、沒有任何字面片段的 class，守衛看不到誰在用它，也就沒辦法列入例外清單或判斷是否成為死樣式。
- `.css` 不在程式碼註解預算的副檔名清單內，樣式檔案裡的註解目前不受該項檢查約束。
