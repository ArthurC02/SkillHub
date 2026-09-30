# 平台體驗模型

本文件把 Skill Hub 的產品願景、領域物件與使用者生命週期整理成一個**平台化目標模型**。它回答「整個產品應該如何連成一體」，不取代兩份現行規格：一頁與一頁之間仍由[資訊架構](./information-architecture.md)規範，一頁之內仍由[前端設計系統](./system.md)規範。

目前程式仍有多個以表單或單一操作命名的頁面；本文件描述它們應逐步收斂的方向。每一批實作落地時，才同步修改資訊架構的規則與現況表、設計系統、路由、測試和畫面。**目標模型不能用來宣稱尚未落地的體驗已經存在。**

產品領域語言與責任邊界以 [Platform Bounded Context 與 Context Map](../adr/README.md#platform-bounded-context-與-context-map)為準；創作、執行、評估、發佈與曝光的安全邊界仍以各主題的現行決策為準。本文件只重新安排使用者如何理解和操作這些能力，不把領域規則搬進前端。

---

## 0. 設計方法：從產品策略走到畫面

本次重構採用 Jesse James Garrett 的 [The Elements of User Experience](https://www.pearson.com/en-us/subject-catalog/p/Garrett-Elements-of-User-Experience-The-User-Centered-Design-for-the-Web-and-Beyond-2nd-Edition/P200000000272) 五層模型。五層由抽象走向具體：**Strategy → Scope → Structure → Skeleton → Surface**。下層決策限制上層能成立的形狀；上層驗證失敗也可以反推下層需要重想。它不是把設計拆成五個互不往來的階段，更不是先畫完所有文件才寫程式。

Garrett 的五層與 [system.md](./system.md) 開頭的「優先序、義務、原則、系統、強制對照表」不是同一件事：前者管理產品從目的到介面的推導，後者管理已經進入單頁設計後的品質與強制方式。

| Garrett 層次 | 本產品要回答的問題 | 本文件的證據 |
| --- | --- | --- |
| Strategy 策略 | 使用者與平台各自要得到什麼 | §1 |
| Scope 範圍 | 平台提供哪些能力與內容，哪些不進主導覽 | §2 |
| Structure 結構 | 物件如何關聯，使用者如何在生命週期中移動 | §3～§4 |
| Skeleton 骨架 | 導覽、工作區、資訊與操作如何佈局 | §5 |
| Surface 表層 | 視覺與文案如何讓平台可信、可辨識、可長期使用 | §6 |

---

## 1. Strategy：平台替誰解什麼問題

### 1.1 使用者需要

使用者成熟度會隨任務改變，不是三個互斥帳號類型。同一個人可能在陌生領域是學習者，在自己的 Skill 上是改善者，在 Trace 與相容性判斷上是精深者。平台必須讓三種狀態共享同一份事實，再提供不同的揭露深度。

| 成熟度 | 首要問題 | 平台承諾 |
| --- | --- | --- |
| 學習者 | 這個小工具能不能安全地幫我完成任務 | 先給結論、限制、來源與可試跑入口，不要求先理解版本或 Trace |
| 改善者 | 哪裡沒達到要求，下一步怎麼修 | 把測試題、試跑、評估、差異與修訂留在同一個小工具脈絡 |
| 精深者 | 判斷依據是否可靠，實際執行發生了什麼 | 保留不可變版本、原始 Trace、強制者、識別碼與完整溯源 |

使用者不是來「填一張 Run 表單」「管理 Test Case」或「建立下載紀錄」。他們要完成的是：

1. 找到或建立一個能解決任務的小工具。
2. 判斷它是否可信、適用且可以安全執行。
3. 用自己的情境驗證結果，而不是只相信描述。
4. 根據證據改善，保留版本與來源關係。
5. 把確認過的成果帶走、發佈或組成可散布的產品。
6. 回來時知道哪些工作仍在執行、哪些地方需要自己決定。

### 1.2 平台目標

Skill Hub 的本體是 **Skill 資產及其生命週期**，不是一組功能入口。平台成功時，使用者能在同一條可追溯鏈上完成：

```text
發現或建立 → 納入工作區 → 建構版本 → 設計驗證 → 試跑 → 判定與改善 → 發佈或帶走 → 持續營運
```

每一段都必須保留三件事：目前在處理哪一個物件、使用哪一份不可變事實、下一個需要人做的決定是什麼。

### 1.3 不用來衡量成功的東西

- 首頁有多少卡片或功能捷徑。
- 導覽列是否把所有能力都列出來。
- 使用者完成了多少次表單提交。
- 單一總分、人氣排行或沒有證據的「推薦」。

首頁的價值由「能否快速續作與處理真正的待決策事項」判斷；小工具工作台的價值由「能否不丟失脈絡地完成一次改善循環」判斷。

---

## 2. Scope：平台空間、能力與內容

### 2.1 六個穩定平台空間

全域導覽只容納會長期存在、能承接多個物件與多次工作階段的空間。一次性動作不得因為有一張表單就升格為平台區域。

| 空間 | 回答的問題 | 包含 | 不包含 |
| --- | --- | --- | --- |
| 首頁 | 我現在最值得處理什麼 | 待決策、續作、執行中、最近使用 | 一般性指標牆、所有功能捷徑 |
| Catalog | 外面有哪些可採用的能力 | 搜尋、瀏覽、比較、公開詳情 | 私人草稿、工作區管理 |
| Library | 我擁有哪些小工具資產 | 匯入、複製、建立、收藏後的資產與保存檢視 | Run 歷史、孤立下載清單 |
| Studio | 我正在創作或修訂什麼 | 創作會話、草稿修訂、參考確認、驗收條件 | 沒有物件脈絡的生成表單 |
| Activity | 哪些工作在跑、失敗或等我決定 | 試跑、掃描、打包、發佈的跨物件活動 | 取代物件內的完整證據與歷史 |
| 發佈 | 哪些成果正在準備或已對外 | Release、Publication、Bundle、曝光狀態 | Catalog 排名、未經擁有者確認的自動發佈 |

Catalog 是否進全域導覽是平台化後的新方向；落地時必須正式改寫 information-architecture.md 的 R7 與守衛，不能只在現有導覽偷偷多一個連結。

### 2.2 脈絡內指令

以下能力仍然存在，但不再是全域導覽項目：

- 匯入一個 Skill。
- 建立或選擇測試題。
- 上傳測試資料。
- 選擇版本並開始試跑。
- 比較兩次試跑。
- 打包與下載。
- 上傳新版本、設定類別或撤回發佈。

它們從全域命令列、Library 的新增選單、單一小工具工作台或 Activity 的續作入口進入。**表單仍可存在，但只負責輸入資料，不負責承載整段旅程。**

### 2.3 全域命令列

命令列同時搜尋可開啟的物件與可執行的指令。它是第二條快速路徑，不取代可見導覽，也不能繞過權限、曝光旗標或確認步驟。

命令結果至少分為：

- 前往：打開 Catalog、Library、Activity 或最近物件。
- 建立：匯入、開始創作、建立測試題。
- 在目前物件執行：試跑、建立新版、比較、打包、發佈。

沒有目前物件時，不顯示必須依附物件的指令；不可用的安全操作要說明原因，不以命令列捷徑跳過 preflight。

---

## 3. Structure：平台概念模型

### 3.1 三層脈絡

```text
平台空間
└─ 小工具工作台（Skill identity）
   ├─ 可變草稿或創作會話
   ├─ 不可變 Skill Version
   ├─ 驗證設計（Test Case、Dataset）
   ├─ 試跑與證據（Run、Trace、Evaluation）
   └─ 發佈成果（Release、Publication、Bundle）
```

第一層讓使用者知道自己在哪一種工作空間；第二層保存「正在處理哪一個小工具」；第三層才切換版本、試跑、證據或發佈成果。不能要求使用者在第三層每換一頁就重新選回第一、第二層的脈絡。

### 3.2 物件關係

- **Skill** 是會演進的資產身分，也是工作台的主體。
- **Draft／Creation Session** 是可修改中的工作，不是正式版本。
- **Skill Version** 是不可變快照；試跑、評估與發佈都指向精確版本。
- **Test Case** 描述想證明的任務與驗收條件，可以被同一個 Skill 的多次驗證重用。
- **Run** 是一次執行；**Trace** 是執行證據；**Evaluation** 是成果判定。三者在 UI 上相鄰，但不能合併成一個狀態。
- **Release** 是擁有者選定的不可變發布內容；**Publication** 是對外身分；Catalog 曝光是另一個 operator 決策。
- **Activity Item** 是上述物件的跨平台投影，不是新的領域真相，也不得成為另一份狀態機。

### 3.3 網址與畫面狀態

網址保存可分享、可重開的身分與工作脈絡：平台空間、Creation Session、Skill、Version、Run、Publication。顯示密度、展開狀態和個人排序不進網址。

目標不是讓每個分頁都新增一條網址，而是讓網址回答一個耐久問題：

- `/library`：我的小工具資產。
- `/workspace/creations?session=$sessionId`：Studio 的會話清單，以及目前正在續作的單一可變會話。
- `/skills/$skillId`：這個小工具的工作台或公開可見摘要，由權限決定能力。
- `/skills/$skillId/versions/$versionId`：需要精確引用的不可變版本。
- `/runs/$runId`：一次試跑的狀態、證據與成果判定。
- `/activity`：跨物件活動與待決策事項。
- `/releases`：工作區的發佈管線。

上面是目標形狀，不是已上線路由表。遷移期間舊網址可以保留並導向對應脈絡；任何正式增刪仍必須先修改 information-architecture.md §0，再更新路由、e2e 位址與雙向守衛。

---

## 4. Structure：六段使用者生命週期

### 4.1 發現與採用

```text
描述任務 → 看到候選與解釋 → 比較證據 → 打開一個候選 → 試跑或複製到 Library
```

- Catalog 先回答適用性、限制、來源、相容性與風險，不先推安裝。
- 公開 Skill 被複製後進入 Library，來源關係持續可見。
- 沒有結果時才出現受旗標保護的創作入口；搜尋與創作不被混成同一件事。

### 4.2 建立與修訂

```text
說明目標 → 釐清與確認參考 → 搜尋既有能力 → 定義驗收 → 產生草稿 → 驗證 → 修訂 → 明確保存
```

- Studio 顯示目前創作階段、已確認事實、待回答問題與草稿差異。
- 模型提出內容，Go 與 Postgres 保有會話與狀態事實。
- 儲存動作綁定畫面上可見的 revision／hash；背景工作只顯示已驗證事件，不模擬 token 串流。
- 採用既有 Skill 或複製一份是成功結果，不強迫所有人走生成流程。

### 4.3 驗證與改善

```text
選擇草稿或版本 → 選擇測試題 → 確認權限與成本 → 試跑 → 查看兩軸結果 → 定位證據 → 修訂或保留
```

- 使用者從小工具工作台開始，因此不再重新挑選 Skill。
- Preflight 是工作台內的決策步驟，不是一個需要自行組裝三個 query param 的孤立頁面。
- 執行狀態與驗收判定並列呈現；「執行完成、部分符合」是合法組合。
- 比較兩次 Run 時，左右兩側各自連回自己的 Run 與不可變 Skill Version；不同 Skill 的證據不能因為並排呈現而共用一邊的脈絡。
- 改善建議建立新草稿或版本，不改寫歷史結果。

### 4.4 發佈與帶走

```text
選定版本 → 檢查再散布與風險 → 預覽交付內容 → 建立 Release／套件 → 發佈或下載
```

- 打包目標、測試題是否隨附與相容性都留在同一個版本脈絡。
- Bundle 的每個成員都保留 owner 回傳的精確 Skill 與 Version 身分；成員名稱不是不可變來源的替代品。
- 建立公開網址不等於進 Catalog；曝光審核仍針對精確 Release。
- 擁有者判斷交付範圍時，畫面分開回答三件事：公開頁誰能讀、套件目前是否提供與取得條件、精確 Release 能否從搜尋與 Catalog 找到。可用性與取得說明由 Publishing 回傳；前端不以 `available` 推成「任何人可下載」，缺少投影時也不推成不可取得。
- 下載紀錄是發佈與交付活動的結果，不是一個主要產品空間。

### 4.5 返回與續作

```text
首頁看到待決策 → 打開 Activity Item → 回到原物件與精確階段 → 做出決定 → 狀態留在物件歷史
```

- 首頁只顯示對下一步有幫助的工作，不用總量卡片填滿版面。
- 背景工作可以安全離開；Activity 顯示最後一次已知更新與真實狀態。
- 完成項目回到物件歷史，Activity 只保留可尋找的投影。

### 4.6 營運

Operator 使用獨立的營運殼層處理帳號、成本、派送、稽核與曝光，不與一般使用者的 Studio 或 Library 混在同一個主導覽。Operator 看到的是跨 Workspace 的工作佇列和決策證據，不是技術 Bounded Context 的選單。

---

## 5. Skeleton：平台殼層與工作區

### 5.1 平台殼層

桌面版的穩定骨架：

```text
┌─────────────────────────────────────────────────────────┐
│ 產品／Workspace │ 搜尋或執行指令 │ 全域狀態 │ 帳號       │
├───────────────┬─────────────────────────────────────────┤
│ 平台空間導覽   │ 目前頁面或物件工作台                    │
│ 最近物件       │                                         │
└───────────────┴─────────────────────────────────────────┘
```

- 頂列最終承擔 Workspace、全域搜尋／命令、帳號與真正的跨產品狀態；第一個切片先提供 Catalog 全域搜尋，Catalog 本頁已有完整搜尋時，頂列讓位而不重複同一工作。跨空間命令要等到至少有一個真實指令及其權限模型後再加入，不先建立空殼。
- 側欄承擔六個平台空間與少量最近物件；不放無上限的使用者內容。
- 手機版把主要平台空間收成可辨識的導覽，不把桌面側欄原封不動縮窄。
- 全域殼層不顯示物件專屬按鈕；物件操作留在工作台標題列。

### 5.2 首頁

首頁由優先序排列的四種內容組成：

1. 需要留意：執行失敗、逾時、無法產生判定，以及仍需人判斷的證據；只有契約已提供真實動作時才顯示處理按鈕。
2. 繼續進行：最近正在編輯、驗證或準備發佈的物件。
3. 執行中：可離開的背景工作與最後更新。
4. 建議探索：只有在沒有更高優先工作時，才放與目前目標相關的 Catalog 入口。

沒有待辦時，首頁應顯示「目前沒有需要處理的事」與最近物件，而不是用虛構統計填空。

### 5.3 小工具工作台

工作台的穩定標題列包含名稱、摘要、目前版本脈絡、信任／可見性狀態，以及少量高頻操作。內容分為：

| 分頁 | 主要問題 |
| --- | --- |
| 總覽 | 這個小工具現在是什麼、能做什麼、下一步是什麼 |
| 建構 | 草稿內容、檔案、創作會話與版本差異是什麼 |
| 驗證 | 哪些要求已被證明，哪裡失敗，證據在哪裡 |
| 版本與發佈 | 有哪些不可變版本，哪一版正在發佈或可帶走 |
| 活動 | 這個小工具發生過哪些背景工作與人為決策 |

右側脈絡區只放當前分頁的下一步、限制或摘要；不能再堆成另一張完整管理表單。窄螢幕時它移到主內容之後，判斷與必要安全資訊的優先序仍遵守 system.md。

現行工作台以同一個 Skill 的穩定局部導覽串起總覽、檔案、驗證與「版本與發佈」，並在 Test Case、preflight、Run 證據、比較與打包畫面保留這組物件出口。當 owner facts 已提供 Test Case 身分時，「驗證」會回到精確 Test Case 並保留當次 Version；歷史資料沒有 `test_case_id` 時才退回該 Skill／Version 的 Test Case 清單，不拿 snapshot ID 冒充可開啟的草稿。不可變版本已有 `/skills/$skillId/versions/$versionId` 的可分享脈絡：它從版本歷史、Run 或 preflight 接住精確版本，再把同一個 `version_id` 帶到驗證、打包與 Release；Skill 詳情的驗證入口會帶入 owner-scoped 最新版本，preflight 改選版本也同步寫回包含 Skill、Version 與 Test Case 的網址。Run 比較的每一側分別回到自己的 Run 與不可變 Version，不因比較畫面共用目前 Skill 的脈絡。發佈送出時明確指名畫面上的版本，不讓伺服器另選最新版本。只有 owner-scoped 版本清單真正回傳的版本能顯示發佈、上傳與打包入口；未知或不屬於此 Skill 的版本不顯示操作。尚未把創作修訂或單一 Skill 的 Activity 搬進同一頁，也不把這組導覽當成階段二已完成。

### 5.4 Activity

Activity 先依使用者能否採取行動分組，再依時間排序：

- 需要留意：必須判斷、重新授權、查看失敗原因或確認結果；不把「值得查看」誇大成使用者一定能修復。
- 執行中：平台正在處理，可以離開。
- 最近結束：可回到來源物件的結果，也包含已取消但沒有其他待判事實的執行。

每列至少顯示物件、工作種類、狀態、最後更新與一個明確下一步。Activity 不複製 Trace、完整評估或套件內容；點開後回到來源物件的精確脈絡。

`/activity` 是跨物件 Activity 的可信投影：Identity 決定 Workspace，Run、Evaluation、Creation、Packaging 與 Publishing 五個 owner 各自回傳分類、權威活動時間、穩定識別與續作目的地，Activity 只依固定優先序合併 Run 與 Evaluation，再作全域排序和 keyset 續讀。任何 reader 失敗都回覆不完整來源並拒絕部分清單，避免把短暫少一類資料畫成「目前沒有」。每列只保留定位與續作所需的精簡 facts，Trace、完整 Evaluation、Snapshot 與套件內容仍回到來源物件閱讀；Creation 的續作入口也繼續受原本兩個曝光旗標約束。舊 `/workspace/runs` 保留為只看 Run 的保存檢視，並明確連到跨來源活動，不再承擔主要導覽。

### 5.5 微觀互動契約

| 情境 | 互動方式 |
| --- | --- |
| 建立、匯入、試跑、打包 | 從目前脈絡開啟短流程；只有內容超過一個可理解決策時才獨立成頁 |
| 背景工作 | 送出後立刻建立真實 Activity；顯示最後更新與可取消條件，不做假進度 |
| 草稿修改 | 明示未儲存／已儲存 revision；正式保存前綁定使用者看到的內容 |
| 不可逆動作 | 先說影響範圍，再要求第二次確認；可逆動作優先提供復原 |
| 停用動作 | 在按鈕附近說明缺少的條件和下一步，不只呈現灰色 |
| 空、未知、無權與失敗 | 使用既有缺席型別與 `ReadFailure`；不得共用「沒有資料」 |
| 證據揭露 | 先顯示判斷，再顯示可定位的證據；安全關鍵內容遵守永不折疊清單 |
| 物件切換 | 保留平台空間與最近使用脈絡；不可悄悄把未送出的草稿套到另一個物件 |

---

## 6. Surface：平台視覺與語氣

Surface 不另建一套 token、元件庫或圖示系統；沿用 system.md 與現有四層 CSS。平台化主要靠資訊層級、穩定位置和一致互動，不靠新增套件。

- **密度**：首頁與 Activity 採清楚的列與分組；卡片只表示真正獨立的物件或有邊界的決策。
- **層級**：平台殼層安靜，物件名稱與目前決策最突出；識別碼、技術中繼資料退到進階層。
- **狀態**：文字是第一通道，顏色與圖示是輔助；執行狀態、評估判定與信任狀態使用不同詞彙。
- **語氣**：先回答「發生什麼、影響什麼、下一步是什麼」，再提供技術原因。
- **動效**：只表達空間或狀態轉移，不用循環動畫製造忙碌感；尊重 reduced motion。
- **可及性**：所有主要路徑可由鍵盤完成，動態結果有狀態宣告，手機觸控目標與文字尺度遵守既有規格。

外部產品模式只作結構參考：GitHub Projects 的同一物件多視圖、VS Code 的穩定容器與脈絡操作、Linear 的保存檢視，以及設計系統對全域殼層與物件內導覽的分工。Primer 的導覽模式把 parent-detail 導覽留在受影響內容旁，分頁緊貼同層內容；Carbon 的 UI shell 在次級項目多且需要頻繁切換時採左側區域，但避免三層導覽；PatternFly 的 Page 也把 masthead、sidebar 與 main 定義成一個可聚焦的頁面骨架（[Primer Navigation](https://primer.style/product/ui-patterns/navigation/)、[Carbon UI shell left panel](https://carbondesignsystem.com/components/UI-shell-left-panel/usage/)、[PatternFly Page](https://www.patternfly.org/components/page/)）。這些原則落在 Skill 工作台、Test Case 內的 Dataset 與 Activity 來源連結；Skill Hub 不複製它們的視覺語言，也不因此引入套件。

空狀態則遵守 Carbon 的結構原則：答案留在資料原本會出現的位置，說明原因與下一步，並避免同一個空狀態堆多個主要行動；同層切換參考 Material UI Tabs 對相關、同階檢視與鍵盤焦點的要求（[Carbon Empty states](https://carbondesignsystem.com/patterns/empty-states-pattern/)、[Material UI Tabs](https://mui.com/material-ui/react-tabs/)）。這裡採用的是資訊層級與互動契約，不是元件實作。

異常狀態參考 Primer 的通知與降級體驗原則：失敗必須維持可見，訊息說明結果與脈絡，且不能淡化平台確實發生的問題；對應到 Activity，就是把失敗與逾時提升到「需要留意」，但只提供契約能保證的查看入口（[Notification messaging](https://www.primer.style/product/ui-patterns/notification-messaging/)、[Degraded experiences](https://primer.style/product/ui-patterns/degraded-experiences/)）。

不可變證據的續接另參考 GitHub Actions：workflow run 以自己的識別與 ref／SHA 保留執行脈絡，artifact 也明確連到產生它的 workflow run，而不是只顯示檔名後讓人猜來源（[Workflow runs API](https://docs.github.com/en/rest/actions/workflow-runs)、[Workflow artifacts](https://docs.github.com/en/actions/concepts/workflows-and-actions/workflow-artifacts)）。Skill Hub 對應只使用各 owner 已回傳的 Run、Skill、Version、Test Case 與 Bundle member 識別來建立導覽，不由前端推測缺席關係。

長工作續作另參考 Backstage Software Templates：每次執行都有 task ID，工作清單以該 ID 連到單一 task，完成畫面再提供產物連結；恢復機制則從已完成 checkpoint 繼續，而不是把前端暫存當真相（[工作清單程式碼](https://github.com/backstage/backstage/blob/master/plugins/scaffolder/src/components/ListTasksPage/ListTasksPage.tsx)、[Task Recovery](https://backstage.io/docs/next/features/software-templates/configuration/#task-recovery)）。Skill Hub 對應採既有 session ID 與伺服器 revision 恢復可變工作，再以既有 version ID 交接不可變產物，不複製它的表單流程。

---

## 7. 不可因平台化而破壞的邊界

1. M5 創作入口在曝光條件成立前仍由伺服器旗標控制；命令列、首頁與 Studio 都不能繞過。
2. 執行前權限摘要、成本與同意綁定仍在建立 Run 前完成，並綁定精確內容。
3. Skill Version、歷史 Run、Trace、Evaluation 與 Release 的不可變性不變。
4. Run 狀態由 Go 擁有；Activity 只是投影，不自行推演或修正狀態。
5. 執行成功與成果符合是兩個維度，不能合成一枚成功徽章。
6. 建立公開 Publication 不等於取得 Catalog 曝光；operator 審核精確 Release 的規則不變。
7. Secret、Trace 遮罩與 Workspace scope 不因為跨頁工作台而轉移到前端判斷。
8. 不可信 Skill、Script 或資料仍不得在 Web／API 程序內執行。

---

## 8. 現況到目標的遷移

### 8.1 能力落點

| 現行入口 | 目標落點 | 遷移方式 |
| --- | --- | --- |
| `/workspace/skills` | `/library` | 已換成平台資產庫名稱與 canonical 位址；舊網址保留 hash 後作相容導向 |
| `/workspace/import` | Library 的新增／匯入流程 | 先讓舊頁接受並保留返回脈絡，再收進 Library |
| `/workspace/creations` | Studio 與 Skill 工作台的建構分頁 | 保留旗標；會話列表在 Studio，單一會話回到 Skill 脈絡 |
| `/lab/test-cases`、`/lab/test-cases/$testCaseId/datasets` | Skill 工作台的驗證分頁 | Test Case 已固定在 canonical 路徑，Dataset 保留同一個 Skill／Version／Test Case 工作脈絡；舊 `/lab/datasets` 只作相容導向 |
| `/lab/run` | 驗證分頁內的 preflight | 已收進 `/skills/$skillId/test-cases/$testCaseId/runs/new`；原 preflight 服務與同意流程不變，舊網址只作相容導向 |
| `/workspace/runs` | 「試跑活動」保存檢視 | `/activity` 已接手主要導覽；Run-only 清單保留精細的 Run 狀態與來源連結，完整內容仍由 `/runs/$runId` 提供 |
| `/runs/$runId` | 工作台驗證脈絡中的精確 Run | URL 保留，增加返回 Skill／Version／Test Case 的持續脈絡 |
| `/skills/$skillId/files` | 工作台建構分頁 | URL 可作進階檔案檢視的深連結 |
| `/skills/$skillId/package` | 工作台版本與發佈分頁 | 保留精確 version；把打包結果送入 Activity |
| `/workspace/downloads` | 發佈／交付活動 | 先變成保存檢視，再移除主導覽地位 |
| `/admin/*` | 獨立營運殼層 | 不併入一般使用者六個平台空間 |

### 8.2 分階段落地

**階段一：平台殼層與續作。** 第一個切片建立全域殼層、平台空間名稱與 Catalog 搜尋入口；首頁先用 Run owner facts 呈現需要留意與執行中的工作，再以 Creation owner 的最近會話清單補上第一個真實續作來源。創作續作只在兩個曝光旗標都開啟時請求，只連回已有且仍可操作的精確 session；不提供開始入口、沒有項目時不渲染空卡，也不冒充完整 Activity。現有功能頁仍可在新殼層中開啟。有至少一個跨空間指令及其權限模型後，再補上命令入口。完成條件是所有現有路由都能從新導覽找到，而且沒有安全資訊或曝光入口被移動到錯誤層級。

**階段二：Skill 工作台。** 詳情、檔案、Test Case、Dataset、preflight、Run 證據與打包已共享同一個 Skill 導覽；preflight 的 canonical URL 把 Skill 與 Test Case 固定在路徑，只讓可替換的 Version 留在 query，舊 `/lab/run` 只負責改寫舊深連結。Dataset 也已從假全域頁搬到 `/lab/test-cases/$testCaseId/datasets`，先讀 Test Case owner facts 再顯示內容，並保留精確 Version；舊 `/lab/datasets` 只作相容導向。已知 `test_case_id` 的頁面會回到精確 Test Case，Run 比較的兩側也各自回到自己的 Run 與不可變 Version。精確版本頁已把版本清單、差異、驗證、打包與 Release 收在同一個不可變版本脈絡。創作會話以 `session` 保存可變工作的精確身分，保存完成後直接交接到該候選的不可變 `version_id`；兩者仍是不同生命週期，沒有把 revision 當成 Version。Creation owner 的既有清單契約現在可用 `version_id` 查出同一 Workspace、仍在保存期內且目前候選相符的會話；Version 工作台只在創作入口真的開啟時顯示「Studio 歷程」，並把命中、可信空態與讀取失敗分開。查詢回傳清單而不是假設一對一，因為重用內容或採用既有 Skill 都可能讓多場會話連到同一個 Version；介面因此只說「相連的會話」，不宣稱永久來源。會話到期後關係導覽消失，但不可變 Version 不受影響；若未來產品需要永久 provenance，必須另立保存、刪除與跨 Context 契約，不能拿過期 session URL 冒充。全程重用既有 hook、service 與元件，不建立第二套狀態。完成條件仍是從一個 Skill 開始可以走完一次「驗證 → 看證據 → 修訂或打包」，過程不用重新選 Skill 或版本。

**階段三：Studio 與 Activity。** 創作會話已能用網址恢復伺服器上的 session 與 revision；恢復後以 owner 回傳的 `pending_action`、確認狀態、草稿與終態，先呈現單一「目前待決定」及可到達的證據卡，再保留完整對話和已確認內容。Studio 同時把現有事實投影為「探索 → 定義 → 建構 → 版本」四段旅程；這只是讓人理解進度的骨架，不是前端新增的狀態機，未知待辦也不會被推測成下一步。Workspace Activity 的受審查契約、Run 列級活動時間、五個 owner fact readers、全來源 fail-closed 聚合、全域排序與 keyset 續讀都已落地；主要導覽改指向 `/activity`，舊 Run 清單仍是保存檢視。畫面分成需要處理、平台處理中、最近完成與其他活動，並以來源 status、權威時間和 typed continuation 回到精確 Run、Creation Session、Artifact 或 Publication。前端不再以多個 API 扇出猜分類，也不會在 reader 缺席時冒充完整活動；完整證據仍由來源頁負責。完成條件所要求的來源物件、真實狀態、最後更新與可續作入口已具備。

這個 Studio 切片依 Garrett 五層維持同一條推導：策略層讓作者回來就能續作並看見眼前決定；範圍層只使用 Creation 已擁有的會話與候選事實，不增加命令、自動確認或永久 provenance；結構層把可變工作清單、目前決策、歷史對話、確認內容與仍可回訪的不可變 Version 脈絡串起來，Registry 不反向依賴 Creation；骨架層在桌面讓工作清單與目前會話並列，手機則在兩個完整工作檢視間切換，Version 頁把 Studio 歷程放在版本事實之後、驗證證據之前；表面層以 brief、文字狀態、更新時間、「目前」語意、可信空態與可辨識焦點共同表達狀態，不只靠顏色。清單忠實保留 owner 回傳順序，不把 `updated_at` 擅自當成排序契約；超過 50 場時明示可見上限，清單讀取失敗也不遮蔽已能精確載入的目前會話。Version 反查同樣由 Creation owner 在 Workspace 與保存期限內回答，最多呈現十場最近更新的相連會話；未知、外部 Workspace 與過期會話都不洩漏存在。Workspace Activity 已接住這些 owner facts；若要把短期 Creation 導覽升成永久 lineage，仍必須重新通過 Strategy、Scope 與 Domain Memory，而不是在 Surface 多留一條連結。

**階段四：發佈與交付。** 舊 `/workspace/downloads` 已先成為「發佈與交付」平台空間，把發佈者身分、跨 Skill 的 Publication／最新 Release、Bundle 與下載紀錄收回同一條旅程；單一 Skill 的 Publication 仍從精確版本工作台建立，首次使用也在該版本脈絡內完成 Publisher 註冊，不再離開工作回到無關的帳號設定。頁首以可定位的三條物件軌道區分「不可變 Skill Version → Publication → Release」「成員版本 → Bundle Version → Release」與「Artifact → 可得條件 → 下載紀錄」，讓使用者先選工作脈絡，再進入保存清單或建立操作。管理清單保留已撤下項目，並明示公開位址不等於 Catalog 曝光。擁有者清單現在也顯示最新 Release 的有效 Catalog 曝光狀態；Bundle 成員可由 owner facts 回到各自被釘選的 Skill Version，而不是只留下名稱與版本號。建立 Bundle 也已改成逐一選擇不可變成員版本，精確 Version 可直接續接並取得焦點，找不到或讀取失敗時不會偷偷改選最新版本。打包、公開取得與 Bundle 匯出都把 owner API 回傳的 Artifact UUID 帶到保存列，版本內的 Publication 操作也用 `publisher/name` 續接到精確項目，成功命中才標示並移動焦點，找不到、讀取失敗與含糊連結維持三種不同答案。Skill 與 Bundle 的 owner 畫面已使用 Publishing 的同一份 availability 與 acquisition 投影，把「任何人可讀公開頁」「目前是否提供套件及登入／邀請條件」「是否能從 Catalog 找到」分成三個判斷；舊服務未提供投影時顯示無法確認，不把未知畫成不可取得。Bundle 也已有跨 Bundle 的 owner 概覽：畫面以 Bundle 為營運主體，在一張卡內分開最新建立的不可變版本、目前公開的最新 Release、交付條件與所有版本操作，瀏覽器不再為每個版本各自查 Publication。保存列能以既有 owner facts 回到精確 Skill Version；這些都是導覽脈絡，不宣稱 Artifact、Release 與 Bundle 成員之間已新增跨 Context 的領域關係。平台仍沒有作者可見、能按 Publication／Release 正確歸因的實際取得者身分或下載總數；既有 Artifact 下載數屬於接收者 Workspace，不能拼成作者採用指標。下一步必須先裁定「取得」「不同接收者」或「真正送出檔案」哪一個是產品指標，以及身分揭露、保存與刪帳後語意，再建立 Publishing 與 Packaging 的受審查協作；在此之前畫面明示未量測。之後再評估把相容網址導向 `/releases`。完成條件仍是擁有者能清楚回答「哪個版本、誰符合交付條件、現在公開到哪裡」；「實際交付給誰」只有在可信契約完成後才加入。

關係導覽只出現在沒有精確續接目標，或連結同時指定多個目標而無法安全判定的狀態；從 Version、Publication、Bundle Version 或 Artifact 續接時，骨架優先把該物件帶入首屏並聚焦，避免平台總覽阻斷當下任務。

這個切片依 Garrett 五層作同一條推導：策略層要讓作者管理持續演進的發佈產品，而不是處理散落表單；範圍層只納入 Publishing 已擁有的版本、Release、狀態與交付條件，採用證據維持未量測；結構層把單一 Skill、Bundle 與 Artifact 三條軌道分開，並以 Bundle 為父物件、不可變版本為子物件，最新建立與最新發佈維持兩個事實；骨架層在一般入口先用關係導覽選定軌道，精確續接則優先呈現目標，再以一個 Bundle 一張營運卡，先放公開狀態與交付對象，後列各版本的匯出與發佈操作；表面層用文字、箭頭、徽章與明確時間說明，不以顏色或空白暗示狀態。任何後續採用分析都必須從策略與範圍重新通過，而不是直接在表面層加一個數字。

**階段五：移除舊殼。** 依使用者驗證與路由證據移除重複導覽、孤立表單入口與已無主體的舊頁；保留必要深連結或導向。完成條件是 information-architecture.md 的偏離帳沒有因遷移變長，e2e 涵蓋所有保留路由，舊入口不再是完成關鍵旅程的唯一方法。

---

## 9. 驗證方式

### 9.1 巨觀驗證

- 使用者能否不理解 `/workspace`、`/lab` 或後端 context，就說出六個平台空間的差異。
- 從首頁、Catalog、Library、Activity 任一處，是否能在兩步內回到正在處理的 Skill。
- 每個跨頁流程是否持續顯示目前 Skill、版本與工作階段，而不是依記憶重選。
- 一個能力是否只有一個主要落點，其他入口都回到同一個物件脈絡。

### 9.2 微觀驗證

- 每個狀態是否先說發生什麼，再說原因與下一步。
- 停用、失敗、無權、未測量與真正的零是否能被分辨。
- 鍵盤、手機寬度、reduced motion 與動態狀態宣告是否完整。
- 執行狀態、評估判定、信任與發布狀態是否沒有共用含糊的「成功」。
- 離開背景工作再回來時，是否看到伺服器事實而不是前端猜測。

### 9.3 代表性任務

每個階段至少用以下任務驗證，而不是只看靜態首頁：

1. 從 Catalog 找到一個候選，判斷風險後複製並完成一次試跑。
2. 從失敗的驗收條件定位證據，建立修訂，再用同一題重跑。
3. 離開執行中的工作，從 Activity 回來並處理結果。
4. 選定不可變版本，確認再散布限制，建立可帶走套件或 Publication。
5. 在 375px 與鍵盤操作下完成上述任務，且必要安全資訊從未只存在 hover 或摺疊區。

只有畫面變漂亮、route 測試通過或某一條 happy path 可走，都不足以證明平台化完成。
