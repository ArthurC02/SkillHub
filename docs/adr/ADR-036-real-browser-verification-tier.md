# ADR-036：前端的真實瀏覽器驗證層 — 引入 Playwright，不採人工走查落檔

- 狀態：Accepted
- 日期：2026-08-22
- 決策者：產品負責人、架構規劃
- 相關：[ADR-016](./ADR-016-language-and-framework-selection.md)（前端選型）、[`02:NFR-007`](../plans/02-specifications-and-acceptance-criteria.md)、[`03` `QA-008`／`QA-009`／`DESIGN-013`](../plans/03-work-items.md)、[`04` 丙-21](../plans/04-backlog-and-handoffs.md)

## 背景

M4 對帳（[m4/audit.md](../plans/mvp/m4/audit.md)）逐項量到一件事：**`apps/web` 的全部前端測試都跑在 jsdom 裡，從來沒有在任何真實瀏覽器引擎上執行過**。這不是覆蓋率不足，是**有一整類判定在那個環境裡結構上做不出來**：

1. **合成像素**。jsdom 不做版面計算，axe-core 的 `color-contrast` 對每一個節點一律回 `incomplete`——它解不出某個元素背後疊的是什麼。`src/contrast.test.ts` 因此改量 `index.css` 的**靜態 hex token**，並在自己的檔頭寫明它證不了三件事：alpha（`--accent-bg`／`--accent-border`／`--social-bg`／`--shadow` 四個 `rgba()` token，`.notice` 因此未檢）、`opacity` 乘數、以及 `PAIRS` 這份手寫配對清單沒聽過的新組合。
2. **真實版面**。`table-layout: fixed` 在窄視窗不會溢位，它會把欄位擠爛；jsdom 兩者都不算。
3. **真實的 Tab 鍵**。jsdom 不實作它，所以 `src/a11y.test.tsx` 斷言的是「在 tab 序列裡、拿得到焦點」，而不是「真瀏覽器的 Tab 順序等於 DOM 順序」——這句界線寫在該檔檔頭第 3 點。

這三件事構成 `QA-008` 的缺口，並且**是 `QA-009`／`DESIGN-013` 第三個不勾理由的天花板**（[`04` 丙-21 ③](../plans/04-backlog-and-handoffs.md)）。M4 對帳當時把它記成「需要一個決策而不只是一段程式」：要為 MVP 引入瀏覽器驅動（CI 時間、維護成本、單人團隊），還是改以人工在三個瀏覽器各走一次主要流程並落檔。

決策驅動因素：

1. **一個不會失敗的守門不是守門。** 手工量測是一次快照，下一個 commit 改壞了不會有人知道。
2. **單人團隊**，每多一套工具鏈就多一份維護成本；CI 時間也是實質成本。
3. 前端已有紮實的 jsdom 層：12 條路由 × 88 條 axe 規則（一條都沒關）、0 violations，另加對比 token 層與六支鍵盤旅程斷言。**新層要補的是那一層結構上做不到的部分，不是它的第二份副本。**
4. 這一層不得把前端測試綁上 Postgres／物件儲存／模型金鑰——那會讓它在筆電上跑不動，在 CI 上又貴又慢。

## 評估選項

### 選項 A：人工在三個瀏覽器各走一次主要流程並落檔

- 優點：零工具鏈、零 CI 時間、當天就能做完。
- 缺點：**產不出自動守門**。丙-21 ③ 會永遠開著——它缺的正是「改壞了會有人知道」這件事，而一次性快照給不了。走查落檔證明的是**某一個 commit 當下**的狀態；下一個 commit 把 `--text` 調亮兩階，沒有任何東西會失敗。同一個理由已經在 `contrast.test.ts` 的存在理由裡寫過一次：手工量測「decays the first time somebody edits `index.css`」。
- **否決。** 不是因為成本高，是因為**它交不出這一項要的東西**。

### 選項 B：Puppeteer

- 優點：成熟、API 簡潔、Chrome 團隊維護。
- 缺點：**引擎覆蓋是它的弱項**——Chromium 為主，Firefox 支援為實驗性質，WebKit 沒有。而本項的字面是「主要瀏覽器」，一個引擎答不了。無障礙整合需自行接 axe。

### 選項 C：Cypress

- 優點：開發體驗好、除錯介面完整、社群大。
- 缺點：跨引擎需要使用者自備已安裝的瀏覽器（不自帶引擎），CI 上要另外處理安裝；WebKit 支援長期為實驗性質。執行模型（測試碼跑在瀏覽器內）對「攔截所有請求並依 `resourceType` 過濾」這種需求較迂迴。

### 選項 D：Playwright（採用）

- 優點：
  - **三個引擎、單一 API**：chromium／firefox／webkit，`projects` 一行一個，同一份測試跑三次。
  - **自帶瀏覽器**：`npx playwright install` 由工具自己管版本，不依賴 runner 或開發者桌面上裝了什麼。
  - **與既有 axe-core 同版整合**：`@axe-core/playwright` `4.13.0` 對上 repo 已釘死的 `axe-core` `4.13.0`，jsdom 層與瀏覽器層掃的是**同一份規則實作**，兩層的結論可以互相對照而不是各說各話。
  - **原生請求攔截**（`page.route`）：不需要起後端就能把整個 API 面餵完，這是「不綁 Postgres／物件儲存／模型金鑰」得以成立的關鍵。
  - 自帶 `webServer`，可直接驅動 production build。
- 缺點：引擎是 Playwright 自行編譯的版本，不是使用者桌面上真正的 Chrome／Safari／Edge；首次執行需下載約 400MB。

## 決策

**引入 Playwright `1.62.1` ＋ `@axe-core/playwright` `4.13.0` 作為前端的真實瀏覽器驗證層，跑 chromium／firefox／webkit 三個引擎。** 人工走查落檔的路線不採用。

### 1. 這一層刻意很窄

只測 jsdom 結構上判定不了的三件事：**合成像素、真實版面、真實 Tab 鍵**。**不重跑 jsdom 已覆蓋的 12 條路由 × 88 條規則**——那會用三個引擎的成本，買同一個答案的第二份副本。每支測試的檔頭寫明它屬於哪一件。

現況：**4 支測試 × 3 引擎 = 12 次，本機 22 秒**；既有 jsdom 層 **138 個測試未受影響**。

### 2. 不需要任何後端

API 一律以 `page.route` 攔截。攔截器**先註冊 catch-all 再註冊具名路由**（Playwright 以反向註冊順序比對），且**不以路徑前綴劃分**——平台同時擁有 `/me`、`/policy/*` 與 `/api/*`，任何漏過去的請求都會打到 preview server，而它對未知路徑一律回 `index.html`：一個 200、滿是 HTML、被當成 JSON 解析，失敗點離成因很遠。因此 catch-all 依 `resourceType` 放行 document／stylesheet／script／font（那是 app 自己在載入），其餘一律擋掉。

### 3. 驅動 production build，走同源

`webServer` 跑 `npm run build && npm run preview`，並以 `VITE_API_BASE_URL=""` 讓前端呼叫**同源路徑**。兩個理由：**那是生產環境的形狀**（SPA 與 API 同源）；而且**它讓 CORS 完全不進入這一層**——`apiFetch` 送 `credentials: "include"`，帶憑證的回應不得配 `Access-Control-Allow-Origin: *`，dev 的跨源設定會讓每一個 stub 在任何斷言跑到之前就被瀏覽器擋下。

### 4. 反空轉：`color-contrast` 先斷言「它做了判定」

**「零違規」本身證明不了任何事——jsdom 就是靠從不判定來報零違規的。** 因此該測試**先斷言 `color-contrast` 已離開 `incomplete`**（並確認它出現在 `passes` 裡），再斷言 `violations` 為空。少了這一步，這一層會安靜地變成它本來要關掉的那個洞。

### 5. 兩個「破壞它來證明它有效」的實測

這是本 ADR 最重要的證據，兩次都是實際改壞再跑：

| 破壞方式 | jsdom 層（`contrast.test.ts`） | 瀏覽器層 |
| --- | --- | --- |
| `--accent-bg` 改成深色 alpha | **19 個全過（完全瞎掉）** | **失敗** |
| `--text` 改亮 | — | **報 58 個違規**（預期 0） |

第一列正是 [`04` 丙-21 ③](../plans/04-backlog-and-handoffs.md) 記載的洞：`contrast.test.ts` 檔頭自陳四個 `rgba()` token 不解析、`.notice` 因此未檢——**那個洞現在被關上了，而且是被證明關上的，不是被宣稱關上的**。第二列證明綠燈代表規則真的跑了並做了判定。

### 6. 跨引擎的真實發現：Tab 測試斷言性質，不比對清單

**WebKit 預設不把連結放進 tab 序列**（對應 Safari「鍵盤導覽」設定預設關閉），Chromium 與 Firefox 會。因此 Tab 測試斷言的是「**焦點在文件中不會往回跳**」，而不是比對一份寫死的順序清單——**寫死的清單等於把 Chromium 的答案當標準，然後因為 WebKit 是 Safari 而判它失敗**。焦點環那支同理：按 Tab 直到頁面內有東西拿到焦點為止，而不是固定按一次。

### 7. CI：獨立 job，不併進 `web`

`.github/workflows/ci.yml` 新增 `web-browser` job（`needs: changes`，僅在 web 有變更時執行），與既有 `web` job **並列而不合併**，讓快的那一層先回報。失敗時上傳 `playwright-report`，保留 7 天；綠燈不留。

**沒有做瀏覽器快取。** `actions/cache` 不在這個 workflow 已釘死 SHA 的清單裡，**為了省約 90 秒去憑空填一個未經驗證的 SHA，對這個檔案其餘部分維持的供應鏈紀律是壞交易**。要做的話先決定並驗證那個 SHA（見待決策）。

### 8. `test:e2e` 刻意不併進 `npm test`

`package.json` 新增 `test:e2e` script，`npm test` 維持只跑 jsdom 層。jsdom 層不需要裝瀏覽器、幾秒就跑完；**讓每一次 `npm test` 都去拉 400MB 的引擎，等於在新貢獻者和他的第一次執行之間放一道下載**。

### 9. 檔案

| 檔案 | 說明 |
| --- | --- |
| `apps/web/playwright.config.ts` | 三個 project、`webServer`、baseURL |
| `apps/web/e2e/rendered.spec.ts` | 4 支測試，依三件事分三個 `describe` |
| `apps/web/e2e/fixtures.ts` | 攔截用的固定回應 |
| `apps/web/tsconfig.e2e.json` | e2e 自己的 tsconfig——它同時需要 DOM 與 node types；已驗證 e2e 確實被 `tsc -b` 涵蓋 |

## 影響

### 正面

- **`QA-009`／`DESIGN-013` 的第三個不勾理由關閉**：合成像素從此有守門，且該守門被兩次破壞實測證明會失敗。丙-21 ①②③ 因此全數關閉。
- **`QA-008` 的三個不勾理由縮成一個**：基礎設施有了、路線決策做了，只剩 OS 矩陣。
- 真實 Tab 順序第一次被量到，`a11y.test.tsx` 檔頭第 3 點自陳證不了的那件事現在有地方證了。
- 三個引擎的差異被記錄下來（WebKit 的連結 tab 序列），而不是被一份寫死的期望值掩蓋成一次失敗。

### 成本與限制（**必須誠實記著**）

- **`QA-008` 的字面判準是「主要瀏覽器**與目標作業系統**測試」，作業系統那一半沒有做。** CI 仍然只有 `ubuntu-latest`，**沒有 OS 矩陣**。**`QA-008` 因此仍然不勾。**
- **Playwright 用的是它自己編譯的引擎，不是使用者桌面上真正的 Chrome／Safari／Edge。** 引擎行為極接近但不等同，且**完全不涵蓋各家瀏覽器的 UI 層與擴充套件**。
- **沒有涵蓋行動裝置真機**；`setViewportSize(375)` 量的是版面，不是真實裝置。
- **CI 每次跑 web 相關變更多約 1–2 分鐘**，主要是引擎下載（因為沒快取）。
- 單人團隊要付的維護成本是真的：多一套工具鏈、多一份會過期的引擎版本。
- 這一層**刻意不覆蓋** jsdom 已覆蓋的全部 12 條路由。`color-contrast` 目前在 **2 條**上判定（`/` 的搜尋結果與 `/policy` 的保存政策表），也就是不必為了掃描而現編 fixture 的那兩條。
  **但「2 / 12」這個數字要照著實際的色彩用法讀，不能照著路由數讀。** 丙-21③ 點名的四個 `rgba()` token 逐一查證後：`--social-bg` 與 `--shadow` 在兩個主題都宣告了卻**從未被任何規則使用**（沒有東西畫它們，自然沒有東西需要合成）；`--accent-border` 只出現在 `border-color`，那是 1.4.11 的非文字對比，`color-contrast` 在任何引擎都不判定它；**真正落在文字後面的只有 `--accent-bg`**，用在 `.notice` 與 `.compare-differs` 兩處，兩處都是疊在 `--bg` 上，而 `.notice` 就在被掃的那條路由上。
  **所以「合成像素沒有守門」這件事已經關閉，剩下的十條路由會補上的是它們自己的文字疊在一般背景上**——那是靜態層已經在量的東西。擴充掃描的邊際成本確實接近零（加第二條路由後總時間不變，仍是 22 秒），因此這條限制**不是成本問題，是 fixture 問題**：`/compare` 那類頁面要三份四十餘欄的 `SkillDetail`，而手寫的 fixture 會比它支撐的斷言更早爛掉。
- **順帶查出兩個死 token**：`--social-bg` 與 `--shadow` 無人使用。本 ADR 不處置它們（刪除是獨立一批的事），但記在這裡，因為它們是上面那段論證的一部分。

## 待決策

1. **OS 矩陣要不要做**（`windows-latest`／`macos-latest`）。這是 `QA-008` 目前唯一的不勾理由。取捨：macOS runner 分鐘數計價較高，而三個引擎在三個 OS 上等於 9 組；也可能只補 WebKit-on-macOS 一格（那是唯一一個「引擎行為可能真的隨 OS 不同」的組合）。**在做這個決定之前 `QA-008` 不得勾選。**
2. **要不要加瀏覽器快取。** 前提是先決定一個 `actions/cache` 的釘死 SHA 並驗證它——本 workflow 的規矩是每個 action 都釘 SHA，不能為了省 90 秒破例。
3. **要不要涵蓋行動裝置**（Playwright 的 device emulation，或真機服務）。目前只有一格 375px 視窗。
4. 引擎版本的升級節奏與觸發條件（比照 [ADR-023](./ADR-023-agent-sdk-version-pinning-and-behaviour-revalidation.md) 對 Agent SDK 的處理方式，還是跟著 `@playwright/test` 的 semver 走）。
