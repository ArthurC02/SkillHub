# ADR-085：樣式分兩層——專案統一風格放 `styles/`，元件特化放元件旁

- 狀態：**Accepted**（2026-09-15，負責人指示：「不該是只有一個樣式檔案，否則隨著專案變大，他會無限膨脹；但是要注意拆分成：1. 專案統一風格 2. Component 各別特化」）
- 日期：2026-09-15
- 修訂：[`system.md`](../design/system.md) §6 強制對照表的「`index.css` 是唯一的樣式表」一列，改由本文決策 1–4 取代。那張表是 [ADR-039](./ADR-039-frontend-design-system-and-ui-evaluation-criteria.md) 決策 4 的表，ADR-039 其餘內容不變
- 相關：[ADR-082](./ADR-082-frontend-features-own-their-pages-sub-components-and-services.md)（feature 收納：本文沿用它的資料夾）、[ADR-064](./ADR-064-the-visual-layer-is-hierarchy-carried-by-tokens.md)（層級由 token 承載）

## 背景

`apps/web/src/index.css` 有 3485 行，是全 app 唯一的樣式表。裡面有 167 個 class：109 個只有一個元件在用，36 個跨 feature 共用。

只准一個檔是為了守門。字級尺度、間距網格、色彩字面值、`opacity`、「markup 的 class 都有規則」這幾道檢查，以及 `contrast.test.ts`，都只讀 `index.css`。多出第二份樣式表，就會悄悄躲過全部檢查，所以 `design-system.test.ts` 有一條斷言直接禁止它。

這個做法有三個代價：

- 每加一頁，就往同一個檔追加規則。
- 兩個 Writer 各改一頁，也得同時改同一個檔。
- 元件刪掉了，它的樣式還留在原處，沒有人會發現。

## 決策 1：樣式表只有兩種位置

| 位置 | 放什麼 | 誰載入 |
| --- | --- | --- |
| `styles/tokens.css` | `:root` 的設計 token，以及暗色與減少動態時的覆寫 | `main.tsx`，第一個 |
| `styles/base.css` | HTML 元素本身的樣式（標題、連結、表單、表格、`details`），以及讓連結、檔案選擇器看起來像按鈕的控制層 | `main.tsx`，第二個 |
| `styles/layout.css` | 每一頁都在裡面的外框：頁首、導覽、頁尾、`main` 的欄寬 | `main.tsx`，第三個 |
| `styles/patterns.css` | 跨 feature 的樣式詞彙：badge、note、notice、清單、卡片、動作 | `main.tsx`，第四個 |
| `<元件>.css` | 只屬於那個元件的特化。和元件放在同一個目錄、用同一個名字（`Home.page.tsx` 對 `Home.page.css`） | 只有旁邊那個 `.tsx`，寫法是 `import "./X.css"` |

- **四層的載入順序就是層疊順序。**
- **元件樣式表跟著元件所在的 chunk 載入**，延遲載入的頁面會連同自己的樣式一起下載。
- **不用 CSS Modules，也不加依賴。** class 名稱全部不變（測試與 e2e 靠 class 找元素），Vite 原生就支援這種 import。

## 決策 2：token 與色彩字面值只在 `tokens.css`

`:root` 上的設計 token 只宣告在 `tokens.css`；任何色彩字面值也只能出現在那裡，自訂屬性的值同樣算在內。這條補的是一個缺口：

- `contrast.test.ts` 只讀 `tokens.css`。
- §2.7 的色彩檢查會先略過自訂屬性的宣告。

所以如果元件樣式表裡寫一條 `--x: #f00`，以前兩道檢查都看不到它。

配方或元件自己的區域變數，可以宣告在它自己的選擇器上，例如門卡的 `--tone`、對話區的 `--avatar`，但值只能引用 token 或長度。

## 決策 3：元件樣式表不得外洩

CSS 沒有範圍。元件樣式表裡如果寫了一條 `.note { … }`，那一頁載入之後，每一頁的 `.note` 都會被改掉，而且哪一頁先開，決定畫面長什麼樣。

所以元件樣式表的**每一個選擇器，都要含有一個「只有這個資料夾在用、別的樣式表都沒提到」的 class 或 id**：

- 在這個範圍內可以特化全域 class，例如 `.filter-bar .note`；但不能單獨改全域 class。
- 同一資料夾的父子元件共用一份樣式表。例如 `FilterBar.css` 也裝了 `FilterControls` 的 class。

## 決策 4：全域層只放共用的東西，例外要登記

全域四層裡，如果某個 class 只有一個資料夾在用，就必須登記在 `design-system.test.ts` 的 `GLOBAL_BY_RECIPE`，並寫明它和哪一條全域規則共用同一個配方。名單只能變短；其他這類 class 一律搬到元件旁邊。

今天名單有 22 個，全都是跟全域規則寫在同一條規則裡：

| 類別 | 個數 | 共用的全域規則 |
| --- | --- | --- |
| app 外框 | 5 | 每一頁都在裡面的外框 |
| 基礎控制層 | 2 | 與 `button` 同一條規則 |
| 評估清單 | 4 | 與 `.criterion-list`／`.criterion` 同一個配方 |
| 次要文字 | 2 | 與 `.note` 共用 14px |
| 評估回饋 | 1 | 與 `.diff` 共用全寬規則 |
| 打包目標 | 2 | 沿用下載清單的配方 |
| 工作區與創作入口 | 4 | 共用同一種門卡 |
| badge 例外 | 2 | badge 規則的例外 |

## 「誰在用」怎麼判斷

以非測試的 `.ts`／`.tsx` 為範圍，下面兩種都算這個檔在用那個 class：

- 字串字面值裡、用空白隔開的詞。例如 `className="a b"`，或 model 裡的 `"badge badge-danger"`。
- 樣板字串的前綴。例如 `` `badge-${kind}` `` 代表所有 `badge-` 開頭的 class 都算。

搬移腳本和守衛用的是同一套判斷，所以搬完的結果和守衛看到的一致。

## 影響

### 正面

- **全域層不再跟著頁數膨脹。** 3485 行分成全域四層共 1836 行（tokens 167、base 574、layout 218、patterns 877），其餘分到 23 個元件旁的樣式表。
- **元件刪掉，它的樣式表會一起刪。** 兩個 Writer 改兩頁，也不會碰到同一個檔。
- **搬移沒有改變任何畫面。** `e2e/shots.spec.ts` 的 81 張截圖（27 個位址 × 桌面亮、桌面暗、手機）在 chromium 下與搬移前逐位元組相同。同一份建置連拍兩次也是 81 張全同，所以這個比對確實分辨得出差異。

### 成本與限制

- **層疊順序變成「四層順序＋chunk 載入順序」。** 以前是單一檔案裡的先後，現在元件樣式表永遠排在四層之後，所以同一個元素上、特異度相同時，元件規則會贏過全域規則。搬移時已用截圖證明沒有翻轉；之後新寫的規則要自己留意。
- **全域四層是跨頁的尺，屬於 coordinator。** 見 `apps/web/AGENTS.md`。
- **守衛只看得到字串裡的 class。** 如果某個 class 從頭到尾都靠變數組出來、沒有出現在任何字串字面值裡，守衛就找不到誰在用它。
- **舊的長註解原封不動搬了過去。** `comment-budget` 的副檔名清單沒有 `.css`，所以 `index.css` 裡帶日期與編號的長註解，現在隨規則原樣留在新檔裡。要不要把 `.css` 納入註解預算，另案處理。

## 後續工作

- `CreationSession.css` 797 行、`WorkspaceSkills.page.css` 279 行：等 ADR-082 後續拆 `CreationSession` 時，照元件一起拆。
