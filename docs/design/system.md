# 前端設計系統與 UI/UX 評估準則

本文件是 [ADR-039](../adr/ADR-039-frontend-design-system-and-ui-evaluation-criteria.md) 的操作手冊，**活文件**。ADR 記的是「為什麼要有一把尺」與否決了哪些做法；本檔記的是**現在這把尺長什麼樣**，隨 `apps/web` 一起改。

事實來源是 [`apps/web/src/index.css`](../../apps/web/src/index.css) 本身。本檔不重抄它的值以外的任何東西，抄了就會漂；每個數字下面都寫明它出現在哪一條規則，改了 CSS 就回來改這裡。

四層，由上往下：**義務**（產品推出來的，不可談判）→ **原則**（八條，全部早已寫在程式碼註解或規格裡）→ **系統**（字級、間距、表面、狀態、版面）→ **強制對照表**（誰在把關，以及誰沒有）。§3 的 checklist 是把前兩層折成一張逐頁可用的表。

---

## 1. 三個 UX 義務

這三條不是風格偏好，是 [`01-goals-and-plan.md`](../plans/mvp/01-goals-and-plan.md) 直接推出來的。違反它們是產品缺陷，不是品味爭議。

### 1.1 可判斷

願景是「成為個人與團隊探索、驗證、改善及發佈 Agent Skill 的**可信任基礎平台**」（`01` §3），而四個問題裡的第二個是「找到 Skill 後，**難以判斷是否可信**、是否適合自己的 Agent 與環境」（`01` §1）。使用者的核心動作是**決定要不要信任並執行別人寫的程式碼**。

所以每個畫面必須讓證據看得見，**也必須讓證據的缺席看得見**。缺席比證據更容易出錯，因為它的預設呈現方式（空白）會被讀成「通過」。

### 1.2 快到第一個判斷

MVP 承諾是「**10 分鐘內**找到 → 驗證 → 下載」（`01` §3）。10 分鐘要走完整個閉環，代表沒有任何一個畫面可以讓讀者為了拿到頭條而費力：頭條要在第一屏，識別碼與版本號不能排在答案前面。

### 1.3 一個介面服務三種成熟度

`01` §2.1 明文：「MVP 預設介面以**非技術使用者可理解的任務語言**呈現，並提供**可展開的進階技術資訊**」，且同節說目標使用者「**可能處於不同成熟度階段，而非固定角色**」（學習者／改善者／精深者）。

也就是說：**不做模式切換，做漸進式揭露**。同一個畫面，白話的答案在上面，`<details>` 裡放版本 id、雜湊、模型出處。這同時是 `01` §5 產品原則第 7 條。

---

## 2. 原則

八條。每一條都標了出處——它們不是新發明的，是已經散在程式碼註解、規格與殘項清單裡的判斷，本文件只是把它們收在同一個地方。

### 2.1 未知不是空白

缺資料的欄位顯示「未知」，不留白。空白會被讀成「通過」。

出處：[`02:DISC-004`](../plans/mvp/02-specifications-and-acceptance-criteria.md)「缺少資料的欄位顯示未知，**不得自行推定為通過**」；實作與推理見 [`Compare.tsx`](../../apps/web/src/pages/Compare.tsx) 檔頭——每一列宣告 `signature` 回傳 `undefined` 表示「沒有資料」，而不是空字串，正是為了讓「沒有」有一個渲染得出來的形狀。

同型例：[`RunEvaluation.tsx`](../../apps/web/src/pages/RunEvaluation.tsx) 的「**未評估不等於通過，也不等於未通過**」。伺服器對沒判過的 Run 回 404，空白的判定區會被讀成核可，所以缺席被寫成一句話。

### 2.2 顯示與強制成對

**不強制的東西就不要顯示。** 顯示一個平台不會執行的上限、鎖或政策，是最壞的一種狀態——比不做還壞，因為它讓使用者按下「我確認」。

出處：[`04` 乙-2](../plans/mvp/04-backlog-and-handoffs.md)——`TokenBudget` 曾經寫進 `policy_snapshot`、顯示在權限摘要上要求確認，而平台與沙箱都不強制；該列的原文是「**現狀（顯示但不強制）是兩者中最壞的一種**」，直接踩 `02:NFR-001`「UI 不得誤導」。相同形狀後來又出現兩次（丙-18 的安裝說明宣稱、丙-25 的保存政策頁），兩次都被同一句話判掉。

### 2.3 狀態先給文字，顏色是第二訊號

出處：[`02:NFR-007`](../plans/mvp/02-specifications-and-acceptance-criteria.md) 第 3 條「風險、相容與評估狀態必須**同時提供文字，不只依賴顏色**」。

實作上這條是逐條寫進 CSS 註解的：`.compare-differs` 的色底旁邊永遠有「有差異」徽章；`.badge-unverified`／`.badge-expired` 的註解寫「Both states carry their own word（已驗證/未驗證、已過期），so the tint is the second channel and not the fact」；`.criterion-unverifiable` 的虛線邊框註明「a third channel, **never the only one**」。

### 2.4 停用要說原因

停用而不說原因會被讀成 bug。**原因才是這個功能誠實的部分。**

出處：[`index.css`](../../apps/web/src/index.css) `.filter-bar` 上方註解——「the unavailable dimensions keep their reason visible rather than hiding it in a tooltip: **a disabled control with no stated cause reads as a bug, and the cause is the honest part of the feature**」。

推論：原因文字是這個控制項最重要的內容，不是它的註腳。QA-009 拔掉那段文字上的 `opacity: 0.65` 正是因為它一度是整個 filter bar 裡**最難讀**的東西（2.18:1）。

### 2.5 執行狀態與任務判定是兩軸

執行成功 ≠ 任務完成。兩者永遠兩列，任務判定排在前面。

出處：[ADR-025](../adr/ADR-025-run-terminal-state-and-evaluation-verdict-separation.md)；實作在 [`RunEvaluation.tsx`](../../apps/web/src/pages/RunEvaluation.tsx) 的 `RUN_STATUS_LABEL`，其宣告上方註明「Execution wording for a run's terminal state. **Never a pass/fail of the task.**」，檔頭第 1 條寫「the judgement is rendered first because that is the question the user actually asked」。

### 2.6 平面答案優先，識別符折疊

白話結論在上面；版本 id、內容雜湊、產生摘要的模型折進 `<details>`。

出處：[`01` §2.1 與 §5 原則 7](../plans/mvp/01-goals-and-plan.md)；實作在 [`SkillDetail.tsx`](../../apps/web/src/pages/SkillDetail.tsx)——檔頭寫「version ids, which model wrote the summary) sit behind `<details>`」，頁面上是「進階資訊（版本與識別碼）」「產生這段摘要的模型」「內容雜湊」三個折疊區。

### 2.7 禁止用 `opacity` 弱化文字

**次要文字只用字級降階，不用 `opacity`，也不另立一支更淡的灰。**

出處：[`index.css`](../../apps/web/src/index.css) 的 QA-009 註解與 [`contrast.test.ts`](../../apps/web/src/contrast.test.ts) 檔頭。舊值把 `--text` 壓到 **2.18:1 ~ 3.71:1**，全數低於 AA。理由寫在 `.note` 那組規則上方：「whatever value `--text` holds, **a multiplier lands somewhere else**」——`opacity` 是**任何色彩 token 都攔不住**的破壞方式，`contrast.test.ts` 量的是靜態 hex，量不到乘數（該檔「What it does not prove」第 2 點自己寫明）。

同一段推理也適用於 `border-style: dashed` 取代 `opacity: .5` 當停用態：註解寫「the obvious way to say "disabled" is `opacity: .5`, and that is exactly the move this file removed twice already」。

### 2.8 毀滅性動作兩段式，且第二段要說會毀掉什麼

出處：[`ConfirmDelete.tsx`](../../apps/web/src/components/ConfirmDelete.tsx) 檔頭——`02:WS-002` 第 3 條與 `02:SEC-006` 要求刪除**在執行前**說明範圍。機制共用，**只有範圍那句話每次不同，而那句話就是整個揭露**。

範本見 [`WorkspaceSkills.tsx`](../../apps/web/src/pages/WorkspaceSkills.tsx) 的 `scope`：它同時說了會消失什麼（清單、搜尋、試跑、打包）、**還救得回來多久**（版本快照凍結 30 天）、**什麼不受影響**（別人 Fork 的版本與歷史 Run 的溯源鏈）、以及**還要另外刪什麼**（下載檔案在另一頁）。

無障礙的兩個行為是承重的，不是裝飾：確認鈕 `autoFocus`（前一顆按鈕離開 DOM，焦點會掉回 `<body>`），以及 `aria-describedby` 指向範圍句（NFR-007）。

---

## 3. 評估準則（checklist）

**這是拿著逐頁走的那把尺。** 每一條都要能回答「這一頁過或不過」。標記說明：`自動` = 有測試會 FAIL；`手動` = 目前只能靠人看。

| # | 問題 | 不過的樣子 | 誰在看 |
| --- | --- | --- | --- |
| 1 | **頭條在第一屏嗎？** 讀者不捲動就看得到這一頁要回答的那個問題的答案嗎？ | 第一屏是識別碼、版本號、麵包屑或一整排控制項，答案在下面 | 手動（義務 1.1 + 1.2） |
| 2 | **每個「沒有資料」的地方，有說出來嗎？** | 欄位是空的、判定區是空的、清單是空的而沒有一句話 | 手動（原則 2.1） |
| 3 | **每個狀態拿掉顏色之後還讀得懂嗎？** 灰階列印還分得出通過與未通過嗎？ | 只有色底／只有邊框顏色／只有紅字，沒有詞 | 手動（原則 2.3；`02:NFR-007`） |
| 4 | **每個停用的控制項有說原因嗎？** 而且原因是看得見的文字，不是 `title` tooltip？ | 灰掉的 select 旁邊什麼都沒有 | 手動（原則 2.4） |
| 5 | **進階／識別符資訊折疊了嗎？平面答案在上面嗎？** | 雜湊、UUID、版本號與白話結論平鋪在同一層 | 手動（原則 2.6） |
| 6 | **標題層級反映真實結構嗎？** 從屬的段落是不是 `h3` 而不是第二個 `h2`？ | 一頁八個 `h2` 互為兄弟，其中七個其實是第一個的子節 | **手動——axe 抓不到**（見 §6） |
| 7 | **清單項目是「可獨立判斷的東西」嗎？是的話它是卡片嗎？** | 一則可以被單獨採信或拒絕的資料，長得像一疊段落 | 手動（§4.3） |
| 8 | **375px 下不橫捲嗎？** | `document.scrollWidth > clientWidth` | **自動**（`e2e/rendered.spec.ts`，全部 18 條路由） |
| 9 | **字級與間距在尺度上嗎？** 新加的值有沒有落在 §4.1／§4.2 的表裡？ | 出現第 10 個字級、第 12 個間距值 | **自動**（`apps/web/src/design-system.test.ts`） |

第 6、7 條是這張表最有價值的兩條，因為**它們是目前僅有的、沒有任何機器在看的**。第 9 條原本也是，已於 2026-08-22 補上守門（見 §6）——那也是這份文件的第一個用途：**先讓自己的規則之一變成可強制的，再拿它去量別人**。第 6 條有前科：`RunTrace` 八個該是 `h3` 的 `h2` 在 138 個 jsdom 測試下活了好幾週。

---

## 4. 系統

### 4.1 字級尺度

`index.css` 宣告了 12 個 `font-size` 值。其中 5 個是 `@media (max-width: 1024px)` 的響應式對應值（36←56、30←40、20←24、18←20、16←18），**不是獨立的階**。拆掉之後桌面實際是 **9 階**，而每一階都有明確工作。真正該合併的只有一組。

| 階 | 桌面 | ≤1024px | 工作 | 出處 |
| --- | --- | --- | --- | --- |
| hero | 56px | 36px | **只有首頁**那一個 `h1` | `.home h1`（「The one hero in the app」） |
| page | 40px | 30px | 一般 `h1` | `h1` |
| section | 24px | 20px | `h2` | `h2` |
| sub | 20px | 18px | `h3`、`.verdict` | `h3`（QA-010）、`.verdict` |
| body | 18px | 16px | 內文（`:root` 的 `font:`） | `:root` |
| ui | 15px | — | 控制項、`.app-nav`、行內 `code` | `button/select/textarea/input`、`code` |
| meta | 14px | — | `.note`／`caption`／label／次要資訊 | `.note, .rank, .risk-counts, .file-size` 等 |
| micro | 12px | — | **只給等寬字** | `.risk-code`、`.script-tag` |

兩點要記住：

- **hero 與 page 是兩個工作，不是同一個尺寸的兩次品味判定。** 56px 曾經套在每一條路由上，所以「資料保存政策」——一份文件標題壓在一張散文表格上面——跟搜尋頁那句一行邀請語同樣大。CSS 註解已把這件事寫成規則：「Those are two different jobs; **only the search page does the second one, and it opts in**」。同一個 `h1` 前後被判兩次而兩次結論相反，就是因為當時沒有這一行。
- **micro 只給等寬字，是因為 mono 在同字級下視覺偏大**：12px mono 約等於 14px sans。非等寬的東西降到 12px 就是把 §2.7 的問題換個做法再犯一次。

**唯一的變更**：`.badge` 目前是 13px，**併入 meta 的 14px**。badge 與 `.note` 差 1px，沒有人分得出來，而那 1px 讓尺度多一階。

### 4.2 間距尺度

**4px 網格：`4 / 8 / 12 / 16 / 20 / 24 / 32`。**

`index.css` 的 `padding`／`margin`／`gap` 共出現 11 個值，其中上面這 7 個在網格上（占絕大多數的使用次數：12px 用 19 次、8px 用 17 次）。偏離的是 **2 / 5 / 6 / 10** 四個，處理方式見 §5。

### 4.3 表面（surface）

**這是先前缺的一條規則。** 三種表面，各自回答一個不同的問題。

| 表面 | 樣式 | 什麼時候用 |
| --- | --- | --- |
| **卡片** | `border: 1px solid var(--border)` ＋ `border-radius: 8px` ＋ `padding: 10px 12px` | 清單裡**一則可以被獨立判斷的東西** |
| **notice** | `border-left: 3px solid var(--accent)` ＋ `background: var(--accent-bg)` ＋ `padding: 8px 12px` | **平台對這一頁講的話**：降級、部分索引、未評估、已被取代、不能打包的理由 |
| **裸區塊** | 無 | 敘述文字、說明段落 |

判準是「**這一則東西能不能被單獨採信或拒絕**」。目前的卡片有四族：`.search-result`（搜尋結果，同時也是 Test Case 列）、`.criterion` / `.suggestion`（驗收條件與改善建議）、`.packaging-target`（打包目標）、`.download-item`（下載紀錄）。

**這條規則正好解釋了上一批為什麼「Test Case 列不是卡片」是缺陷。** commit `90ade82` 的原文是：「Test Case rows were the only list in the app whose items were not the card every other list uses」——一則 Test Case 完全是可以被獨立判斷的東西（要不要拿它去試跑、要不要改它），卻只是一疊段落，而同一個 app 裡其他四族都有卡片。修法是套用既有的 `.search-result`，不是發明第五種樣式。這也是搜尋結果本身在更早一批被補上卡片的同一個理由（CSS 註解：「the search hit — **the thing the product is for** — was a bare stack of paragraphs」）。

**notice 不是卡片**，因為它不是清單的一則資料，而是平台的自述。它也不是「非阻斷」的同義詞：`Packaging.tsx` 用它承載「不能打包」，那是阻斷的。目前沒有 `--danger` 版的 notice；要不要有，見 §5 的開放項。

### 4.4 狀態語彙

| 視覺 | 主張 | 現行用處 |
| --- | --- | --- |
| **badge（永遠帶文字）** | 一個事實的標籤 | `LabelledBadge` 的文案**由伺服器給**，前端不留自己的 enum→中文對照表（NFR-001：兩個平面不得把同一件事講成兩種話） |
| **`--danger` 邊框＋文字** | 這件事不通過／會擋住你 | `.criterion-failed`、`.badge-severity-error`、`.badge-expired`、`.script-tag` |
| **`--accent-border`** | 這件事未知／未驗證 | `.criterion-undetermined`、`.badge-criterion-undetermined`、`.badge-severity-warning`、`.badge-untested`、`.badge-unverified`、`.badge-differs` |
| **虛線邊框** | 這個東西**不是完整有效的**：平台自己降級了判定，或控制項現在不能用 | `.criterion-unverifiable`（證據無法回驗，`04` 丙-10）、`button:disabled` 等 |

顏色與線型永遠是**第二或第三**訊號（§2.3）。`--accent` 本身（`#aa3bff`）在 `--bg` 上是 4.39:1，低於 AA，所以它可以描邊，**不能當文字顏色**；連結另有 `--link`。

### 4.5 版面

- `#root`：`width: 1126px`、`max-width: 100%`、置中、左右各一條 `--border`。
- `main`：`padding: 0 24px`。
- 斷點兩個：**1024px**（字級降階）與 **640px**（`main`／`.app-header`／`.app-footer` 的 `padding-inline` 改 12px）。640px 是後補的——註解寫「The file had three breakpoints before this and all three were 1024px, i.e. **the app had no phone layout at all**」。
- `.filter-bar` 的 320px grid track 是從 1126px 反推的：要讓一頁落三軌而不是四軌。改頁寬要一起重算。

---

## 5. 偏離清單

**形狀比照 [`db/query-owners.yaml`](../../db/query-owners.yaml) 的 `allow:`：具名、有理由、只能縮短不能增加。** 那個檔案對自己的容忍清單寫的是「`allow:` 是**存量漂移**，不是擴充點」，而它現在是零條。這裡的規矩一樣——**新的偏離一律改回尺度上，不准往下面加行**。

| 值 | 出現在 | 判定 |
| --- | --- | --- |
| **13px** | `.badge` | **要收**：併入 meta 14px（§4.1） |
| ~~**12px 非等寬**~~ | ~~`.filter-unavailable .note`~~ | **已收（2026-08-22）**：規則整條移除，該句改吃 `.note` 的 14px。它被兩個不同機制各降一次——先是 `opacity: 0.65`（QA-009 拔掉），再是這條 12px——而兩次降的都是原則 2.4 存在的理由那一句。`.filter-unavailable` 本身隨之成為無樣式的 class，一併從 markup 移除 |
| **5px / 10px** | `button/select/textarea/input` 的 `padding: 5px 10px` | **保留，有推導**：`20px` line-height ＋ 上下 5px ＋ 上下各 1px 邊框 = `min-height: 32px`（WCAG 2.2 2.5.8 的 24px 下限之上）。不是隨手挑的值 |
| **10px** | 卡片的 `padding: 10px 12px`（四族共用） | **保留，單一來源**：三條規則、一組值，改它就是改全部卡片 |
| **2px** | `.badge` 上下 padding、`.result-facets` 列間 gap、`.search-result p` 的 margin | **待收**：三處都是「比 4px 更緊一點」的視覺微調，沒有推導 |
| **6px** | 六條規則的 `gap`（`.compat-list, .file-tree, .search-results, .tag-list`／`.result-facets dd`／`.license-badge, .match-reason`／`.compare-pick`／`.evidence-list`）與 `.result-facets` 的 `margin` | **待收**：六處同值，等於一條沒登記的「緊密清單間距」。要嘛登記成尺度的一階，要嘛收成 4px 或 8px——**二選一，不要繼續兩邊都不是** |

開放項（不是偏離，是缺口）：**沒有 `--danger` 版的 notice**，所以阻斷性訊息（`Packaging.tsx` 的「不能打包」）與降級說明用同一個外觀。目前靠文字分辨（§2.3 允許），但這是 §4.3 那張表講不清楚的一格。

---

## 6. 強制對照表

**這一節的價值在誠實。** 有守門的寫誰在守，沒有的直說沒有。

| 規則 | 把關者 | 性質 |
| --- | --- | --- |
| 色彩對比（token 層） | [`apps/web/src/contrast.test.ts`](../../apps/web/src/contrast.test.ts) | 自動；值直接讀 `index.css`，不留第二份副本 |
| 色彩對比（合成像素、alpha） | [`apps/web/e2e/rendered.spec.ts`](../../apps/web/e2e/rendered.spec.ts)（[ADR-036](../adr/ADR-036-real-browser-verification-tier.md)） | 自動；三引擎。曾以故意打破 `--accent-bg` 反證這一格真的補上了 token 層的洞 |
| 375px 不橫向溢出 | 同上，**全部 18 條路由**（`e2e/routes.ts` 的 `PHONE_ROUTES`） | 自動 |
| 焦點環真的被畫出來 | 同上 | 自動 |
| Tab 順序不倒退 | 同上 | 自動；斷言「不往回跳」而非固定清單，因為 WebKit 預設不把連結放進序列 |
| 表單標籤、鍵盤可達、88 條 axe 規則 | [`apps/web/src/a11y.test.tsx`](../../apps/web/src/a11y.test.tsx) | 自動；新路由沒加案例會 FAIL |
| 標題**不跳級** | 同上（axe `heading-order`） | 自動 |
| **標題該降級而沒降** | **沒有人把關** | axe 只罰跳級，不罰「該往下一級卻沒往下」。`RunTrace` 八個該是 `h3` 的 `h2` 就是這樣活下來的（commit `90ade82`：「axe never saw it — `heading-order` fails a skipped level, not a level that should have gone down and didn't」） |
| 字級／間距在尺度上 | [`apps/web/src/design-system.test.ts`](../../apps/web/src/design-system.test.ts) | 自動；尺度外的值會被點名。偏離清單就是 §5，比照 `db/query-owners.yaml` 的 `allow:`——**只能縮短，不准為了讓 build 過而加行** |
| 卡片規則（§4.3） | **沒有** | 只能靠人看截圖 |
| 未知不是空白（§2.1） | **沒有**（單點有測試，規則沒有） | 逐頁的「哪裡該有話而沒有」是判斷題 |
| 停用要說原因（§2.4） | **沒有** | 同上 |

自動那一排全部來自 ADR-036 建的那一層瀏覽器測試；它刻意很窄，只做 jsdom 結構上做不到的三類判定。**不要因為它存在就假設 UI 有守門**——上表下半部說明了大多數設計規則仍然沒有。
