# ADR-081：前端分三層組件，伺服器狀態只住在 `api/`

- 狀態：**Accepted**（2026-09-14，負責人指示：「希望能夠真正落實 Component-Based 的概念。讓 Component 得以提高複用性，頁面的 Component 之間的狀態管理是有效率並且具備可解釋性，讓 Coding Agent 方便追蹤」）
- 日期：2026-09-14
- 相關：[ADR-031](./ADR-031-artifact-role-repository-layout.md)（收納語意）、[ADR-039](./ADR-039-frontend-design-system-and-ui-evaluation-criteria.md)（前端設計系統）、[information-architecture.md](../design/information-architecture.md) §5 IA-6（401 由 `ReadFailure` 說一次）

## 背景

`apps/web/src` 已經有 `api/`、`components/`、`pages/` 三個目錄，但**目錄只是位置，不是規則**。本批開工前的盤點：

- **伺服器狀態散在頁面裡。** 127 處 query／mutation 呼叫中，有一半以上直接寫在頁面或元件裡；18 處快取失效（`invalidateQueries`）各自手寫一個陣列當鍵，同一份資料的鍵只靠「大家都記得寫成一樣」維持一致。
- **有的寫入沒有讓讀到它的畫面失效。** 上傳 Dataset 之後，Test Case 頁的檔案清單要等到下次重新整理才看得到；由改善建議建出新版本之後，版本清單同樣是舊的；建立 Test Case 之後回到清單也是舊的。這些都不是 bug 報告找到的，是把每個寫入集中到一處之後才看得出來。
- **頁面互相 import。** 10 處 page → page 的 import，其中一個「頁面」`RunEvaluation.tsx` 根本沒有位址，卻把 `RUN_STATUS_LABEL` 供給另外四個檔；`SkillVersionPicker`、`VersionDiff`、`bytes`、`packagingGate`、`CLEANUP_BADGE`、`MAX_COMPARE` 都住在某一頁裡，被其他頁借用。
- **伺服器回應被複製進 `useState`，再用 effect 重設。** 上傳結果、開始 Run 的回應、打包結果各自有一份 state 與一個「參數變了就清掉」的 effect，資料的真相有兩份，要追的人也得看兩處。
- `retry: false` 在每一個 hook 各寫一次。

Coding Agent 追一個「按下去之後畫面怎麼更新」的問題，要讀頁面、找到手寫的鍵、再全 repo 搜同一個陣列，而且搜得到的只是「字面上長得一樣」的那幾處。

## 決策 1：三層，只能往下 import

| 層 | 放什麼 | 可以 import |
| --- | --- | --- |
| `pages/` | 有位址的畫面（每個檔對到 `router.tsx` 的一條路由） | `components/`、`api/` |
| `components/` | 可重用的 UI 與共用詞彙（標籤表、格式化、閘門判斷），以及沒有位址的區塊 | `components/`、`api/` |
| `api/` | 伺服器狀態：fetch、query／mutation hook、快取鍵 | 只有 `api/`，且不放 `.tsx` |

- **頁面不 import 另一頁。** 兩頁都要用的東西就是共用的東西，搬到 `components/`。
- 三層都不 import `router.tsx`、`App.tsx` 這些組裝檔；唯一例外是 **`import type` 路由的 search 型別**（例如首頁的 `HomeSearch`）——網址的形狀由路由表擁有，頁面讀它是讀自己的輸入，而型別 import 在執行期不存在，不會形成 router → page → router 的循環。
- **沒有位址的東西不是頁面。** `RunEvaluation.tsx` 改名為 `components/EvaluationPanel.tsx`；執行狀態措辭與清理狀態徽章搬到 `components/runStatus.ts`。
- 本批搬家：`components/SkillVersionPicker.tsx`、`components/VersionDiff.tsx`、`components/format.ts`（`bytes`）、`components/packagingGate.ts`；`MAX_COMPARE` 搬到 `api/skills.ts`（它是比較頁一次讀幾個 Skill 的上限，跟著讀取走）。

## 決策 2：伺服器狀態只住在 `api/`

- `api/` 以外**不 import** react-query 的 hook；只允許 `App.tsx` 的 `QueryClientProvider` 與型別。
- **每一個寫入都是一個 `api/` hook，而且自己宣告它讓哪些資料失效**（`onSuccess`／`onSettled`）。「刪掉一個 Dataset 會讓什麼重抓」的答案只在一個地方，就在寫入旁邊。
- 元件對寫入結果的**畫面反應**（顯示一句話、關掉確認框、導頁）寫在呼叫端的 `mutate(vars, { onSuccess })`；失效寫在 hook。前者是這一頁的事，後者是資料的事。
- 會串流的創作會話由 `useLiveCreationSession` 擁有串流、輪詢與寫回快取；元件只拿到一個 query。

## 決策 3：每一個快取鍵都由 `api/queryKeys.ts` 產生

- 鍵是路徑：讓一個鍵失效，會連帶讓所有以它為前綴的鍵失效（例如 `skills.detail(id)` 涵蓋該 Skill 的檔案與版本）。這一條寫在 `queryKeys.ts` 開頭，因為它決定了一個鍵該長在誰底下。
- 鍵的**值**不變：既有測試用字面值預先填快取，改鍵值等於改行為。

## 決策 4：不把伺服器回應複製進 `useState`

- 寫入的結果與錯誤直接讀 `mutation.data`／`mutation.error`；「換了參數就重來」用 `key` 讓元件重新掛載，而不是用 effect 逐一清 state。本批套用在 Dataset 上傳與執行前權限確認（兩者的參數都在 search param，路由不會自己重掛）；打包結果只在「是這個 Skill、這個版本打的」時顯示，直接從 `build.variables` 判斷。
- **使用者自己的工作集不在此限**：這次造訪上傳過的檔案清單、還沒採納也還沒忽略的建議，是使用者在這一頁的操作紀錄，不是伺服器資料的副本。
- 首頁的搜尋草稿與篩選列刻意不改：它們以 effect 同步網址，改成 `key` 重掛會搶走輸入框的焦點。

## 決策 5：`retry: false` 只寫一次

寫在 `api/queryClient.ts` 的預設值。原本每個 query 都各自設了同一個值，所以這是行為不變的收斂。

## 決策 6：守它們的機器

`apps/web/src/architecture.test.ts` 逐檔檢查決策 1（import 方向、`api/` 沒有 `.tsx`）、決策 2（`api/` 以外的 react-query import）、決策 3（手寫的陣列鍵）與決策 5（`retry:` 只出現在 `queryClient.ts`）。**決策 4 沒有機器**：「這個 state 是不是伺服器資料的副本」要讀懂才判斷得了，只能靠審查。

## 影響

- 行為上的變化都是「少一個過期畫面」或「少一段重複狀態」：
  - 新增三條失效：上傳 Dataset → 該 Test Case 的檔案清單；由建議建出新版本 → 該 Skill 的版本清單；建立 Test Case → Test Case 清單。帳號刪除申請與取消的 `/me` 失效從頁面移進 hook。
  - 寫入 hook 的 `onSuccess` 回傳失效的 promise，所以按鈕的「處理中」會延續到重抓完成；呼叫端的畫面反應在重抓之後才出現，看到的訊息與看到的清單是同一個時間點。
  - 匯入進行中不再顯示上一次的結果；評估回饋送出後若再送一次而失敗，「已送出回饋」會消失，只剩失敗那一句。
- 全部既有測試（33 個檔、618 條）不改斷言即通過；只有三個測試檔的 import 路徑跟著搬家。

## 後續工作

- 大檔拆分：`components/CreationSession.tsx`、`pages/TestCases.tsx`、`pages/Admin.tsx` 仍各自承擔太多區塊，拆成 `components/` 下的具名區塊是下一批。
- 送出按鈕（`isPending` 時換字、停用時說原因）與「刪除前先確認」已經有 `ConfirmDelete`，其餘確認型動作還各寫各的，可以收斂成一個元件。
- `Packaging` 的驗證發現清單與 `components/Findings` 標題與文案不同，`RunTrace` 的產出檔案卡與 `DownloadArtifactFacts` 型別與內容不同——兩組都**不是**純重構就能合併，要先決定文案。
- 未登入（401）目前由每個 `ReadFailure` 各自說一次（IA-6）；集中處理與那條規則衝突，要動就先改 IA-6。
