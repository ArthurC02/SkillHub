# ADR-059：乾淨測試模式的執行 Driver 自己宣告「不是沙箱」，派送閘門改為白名單

- 狀態：**Accepted**（2026-08-28 Proposed → **2026-08-30 Accepted**）

> **落地與驗證（2026-08-30 補記，不改動以下任何決策內容）。** 本 ADR 的決策由 M6 的 `PORT-010a`／`PORT-010b`／`PORT-009` 實作並實測證實，`03` §20 十一項全部收束。三點值得記在狀態旁邊：
>
> 1. **決策 3（派送閘門改白名單）成立，但它擋的比這份 ADR 當時以為的少。** 白名單決定「能派給哪個 Driver」，它**沒有任何分支看得到 skill、version、workspace 或內容來源**——所以「本模式只跑策展素材」這條規格在 2026-08-30 之前**沒有強制點**，三份文件互相指認對方是強制點而沒有一個是（`04` 丙-85）。真正的閘門是同日補上的 `trial/execution/schedule.go` 的 `requireCuratedContent()`，由 `dispatch()` 在選 provider 之前呼叫。
> 2. **決策 5 的三項「做不到」全部照實落地並被機器押著**：`Reaping()` 宣告 `{一般子孫, 刻意 detach 的子孫}`，Windows `{✅, ✅}`、Linux `{✅, ❌}`，而**測試按宣告斷言**——哪天有人把 Linux 修好，紅的是宣告不是沉默。
> 3. **待決策 1 已裁（2026-08-29，環境變數契約複製一份）；待決策 3 仍然開著**——`Adopt()` 回空（服務重啟會殺掉跑到一半的 Run）要不要在畫面上說。**它不擋 `02:PORT-010` 的允收**：那條要的是「明文記載，不得只寫在程式註解裡」，已由本 ADR 決策 5 與 [m6/report-local-driver.md](../plans/mvp/m6/report-local-driver.md) §2 滿足；「上畫面」是另一件事。
- 日期：2026-08-28
- 決策者：產品負責人、架構規劃
- 相關：[ADR-015](./ADR-015-sandbox-isolation-technology.md)（gVisor 基線）、[ADR-022](./ADR-022-sandbox-deployment-topology-and-security-thresholds.md)（安全門檻定值）、[ADR-006](./ADR-006-local-runner-for-local-resources.md)（Provider Port 與 Adapter）、[ADR-058](./ADR-058-the-clean-test-mode-is-real-postgres-behind-the-api-seam.md)（乾淨測試模式的資料庫那一半）、[ADR-004](./ADR-004-provider-neutral-run-orchestration.md)（Run 生命週期與 Provider 契約）

## 背景

[ADR-058](./ADR-058-the-clean-test-mode-is-real-postgres-behind-the-api-seam.md) 解決了乾淨測試模式的資料庫那一半。執行那一半沒有等價的答案，理由寫在 [m6/report-sandbox-options.md](../plans/mvp/m6/report-sandbox-options.md)：**PGlite 成立是因為 PostgreSQL 是一個已知的程式，可以整個編譯成 WASM；而沙箱要關住的是使用者還沒寫出來的程式，沒有東西可以拿去編譯。** 真正卡死的更前面——白名單管的是哪顆 PE 可以執行，所以任何要你自己帶一顆執行檔上機的方案，都在門口出局。

但**乾淨測試模式要的不是隔離，是生命週期**：能 spawn、能跑完、能取回成果、能回收資源。這兩件事的難度差一個數量級，而把它們混為一談是這份 ADR 存在的理由。

執行平面的接縫已經存在：`apps/sandbox/internal/sandbox` 的 `Driver` 介面，`dockerdrv` 是它今天唯一的實作。工作負載本身是**單一個 `node /opt/skillhub/run.mjs`**，所以第二個實作 spawn 的是完全相同的東西，只是把 `/work`、`/out` 換成主機目錄。

## 決策

### 決策 1：新增 `clean` 隔離等級，它的名字說的是「沒有邊界」而不是「比較弱的邊界」

Driver 向 `Match` 宣告 `isolation.level = "clean"`。**這不是一個比較弱的沙箱，是沒有沙箱**，而名字必須這樣讀。它只承載**策展過的展示素材**（`02:PORT-007`），**永遠不承載不受信任的內容**。

先例是沙箱自己：dev 的 DockerProvider 宣告 `container` 而不謊稱 `gvisor`，由消費端決定它能不能用。**那個保證由既有程式碼提供，不靠任何人記得。**

### 決策 2：它的開關是自己的變數，不是 `DEV_LOGIN`

`SKILLHUB_CLEAN_MODE=1`。**理由不是潔癖**：`clean` 比 `container` 更弱，而 `DEV_LOGIN` 幾乎每一台開發機都開著——沿用它等於把「沒有邊界」發給所有人。已有一支測試斷言「光有 `DEV_LOGIN` 不夠」。

### 決策 3：派送閘門從黑名單改成白名單

`Match` 原本拒絕 `""`、`process`、非開發部署下的 `container`，**其餘一律通過**。實測（`73af5b8`）：`gvsior`、`banana`、`hosted-vm`、`GVISOR`、`gvisor `（尾隨空白）**全部通過隔離那一關**。

沒有任何現存路徑產得出未知值——`sandboxd` 的宣告值是程式裡的兩路分支——**但同一段程式的註解記著一次形狀一模一樣的事故**（一台掉了環境變數的節點在共用核心上跑了每一個不受信任的 Skill，而它有在啟動 log 裡說），**而當時的修法是往黑名單再加一個值，不是改成白名單**。加一個新等級的同時不修它，等於預約下一次。

改法：`gvisor` 各處皆可、`container` 要 `DEV_LOGIN`、`clean` 要 `SKILLHUB_CLEAN_MODE`、**其餘一律拒絕**。新增等級從此是「必須先寫下來」。

### 決策 4：不引入第三方套件；用 `golang.org/x/sys` 加約 120 行

逐一查證見 [m6/report-local-driver.md](../plans/mvp/m6/report-local-driver.md)。**沒有成熟、活躍、真正跨 Windows 與 Linux 的 Go 函式庫做這件事。**

最該記住的一項：`go-cmd/cmd`（1027 stars）的 README 寫著三平台皆可、且宣稱實作了殺行程群組的 low-level magic，**而它的 `cmd_windows.go` 全文是 `p.Kill()` 加一個空的 `SysProcAttr`**。Linux 那半是對的，Windows 那半是空殼。**本專案實測過那會產生什麼**：只殺父行程時孫行程存活（`leaked=1`），改用 Job Object 後歸零。

`golang.org/x/sys` **已經在相依樹裡**（indirect），且已出好 Job Object 全套 API；缺的只有 `OpenJobObject`、`IsProcessInJob` 與 CPU rate 的結構。參考實作是 Buildkite Agent 的 `internal/process`（MIT，`internal/` 不可 import 但可複製）。tar 那一半用標準庫 `archive/tar` 約 40 行。

### 決策 5：四個「做不到」寫在介面上，不寫在註解裡

1. **`Adopt()` 在乾淨模式回空。** `KILL_ON_JOB_CLOSE` 的語義是「最後一個 handle 關閉即殺光」，所以**服務重啟＝跑到一半的 Run 全部跟著死**。要讓它們活過重啟得改用具名 job 並放棄那個旗標，代價是被硬殺時留下孤兒樹。**本 ADR 選擇前者**：乾淨模式是展示與開發用途，留下孤兒行程樹比丟掉一次 Run 糟。
2. **資源上限兩個平台不對稱。** Windows 的 Job Object 可設記憶體、行程數與 CPU 上限且不需管理員；**Linux 無 root 時沒有等價物**——Go 的 `SysProcAttr` **沒有 RLimit 欄位**，可用的是 `UseCgroupFD`／`CgroupFD`（Go 1.20 起）而那要 systemd 有委派 memory controller，**必須 runtime 偵測後降級**。最省的共同底線是 `node --max-old-space-size`。**能力宣告要照實反映實際偵測到的結果，不是照實反映意圖。**
3. **Linux 回收不到一個刻意 detach 自己的子孫，Windows 回收得到。**（**2026-08-29 補入，本項由 CI 加上去的**——`tree_unix.go` 把 `setsid()` 這個缺口正確地寫進了註解，而 `TestReapsWholeProcessTree` 仍然無條件斷言整棵樹被回收，於是 Linux CI 紅了。**一個測試不知道的限制，等於沒有人在守。**）<br>
**兩個平台差的正好是這個 Driver 存在的那個案例**：Job Object 不問子孫願不願意就擁有它們；POSIX 的行程群組是子孫呼叫 `setsid()` 就能離開的東西。**所以「刻意脫離的那個行程」——正常工作負載不會產生、有敵意或粗心的會——在 Windows 上被回收，在 Linux 上不會。**<br>
**沒有繞過，是因為每條繞法都比這個 Driver 的用途貴**：`PR_SET_CHILD_SUBREAPER` 加 `/proc` 掃描會在一個本套件並不擁有的二進位上設行程層級的全域狀態；PID namespace 走 user namespace 會改變工作負載看到的世界，還要賭 unprivileged userns 有開。**M6 的目標機器是 Windows，生產的 Linux 用 gVisor 走 `dockerdrv`，而本 Driver 在兩個平台都不承載不受信任的內容**——所以誠實的「不行」是代價最小且什麼都沒藏的答案。<br>
**形式**：`Driver.Reaping()` 回 `{Descendants, Detached}`，Windows `{true, true}`、Linux `{true, false}`，而**測試按這個宣告斷言**——宣告說不行的平台，測試就斷言它真的不行，所以哪天有人把 Linux 修好了，紅的是宣告而不是沉默。
4. **`Stop(grace)` 的 grace 在兩個平台都不是合作式窗口，而這是既有行為不是新引入的。** `run.mjs` **沒有任何訊號 handler**（`grep process.on` 零筆），而 `dockerdrv.Stop` 走 `ContainerStop`（SIGTERM 後 SIGKILL）——**今天就沒有人在聽**。真正的收集窗口是 `WorkloadDone`／marker 檔那個機制，本機 Driver 照做即可。

## 評估選項

| 選項 | 內容 | 為什麼不採 |
| --- | --- | --- |
| **A. 本機行程 Driver ＋ `clean` 等級（採用）** | 第二個 `Driver` 實作，spawn 同一個 `node run.mjs` | — |
| B. 沿用 `process` 等級並加例外 | 不新增等級 | 把一條目前「無條件永不」的規則變成有條件，**而那條規則的絕對性正是它的價值** |
| C. 讓乾淨模式繞過 `Match` | 另一條派送路徑 | 複製狀態機，且繞過的正是唯一會說「這不是沙箱」的那道門 |
| D. 引入第三方行程管理套件 | 用現成的輪子 | **沒有輪子**（決策 4）。最像的那一個在 Windows 上是空殼 |
| E. 不做，執行永遠屬清單外 | 維持 `02:PORT-006` | 產品 DoD 第一條要求走完含「試跑」的六段；**一個不能試跑的 Skill Hub，示範的是它的搜尋引擎** |

## 後果

- **可以**在不能安裝任何東西的機器上跑完一次完整的 Run 生命週期，並取回 trace 與產出。
- **不可以**在那上面執行不受信任的內容——由 `Match` 強制，不靠任何人記得。
- **多一個實作要跟著契約走**。工作負載的環境變數契約目前寫死在 `dockerdrv` 裡，兩個實作要嘛共用一份、要嘛分岔——**`02:PORT-008` 的禁令是同一個形狀**。
- **`02:PORT-006` 的範圍要改**：執行不再一律屬清單外，改為「不受信任內容的執行屬清單外」。

## 退場條件（2026-08-29 追記）

本 Driver 存在的理由是**一台不能安裝任何東西的機器**。所以它應該有一個退場條件，而不是靠沒有人想起它而永久存在。

**它原本有一條最短的退場路徑，而那條路今天關了**：[`05` R-23](../plans/05-pending-rulings.md) 的選項 (b)——**向機構爭取防火牆多開一個網域**，把 Run 送回自己的 gVisor 節點。若 (b) 成立，本 Driver 與 `clean` 等級**整批不需要存在**：隔離與生產一模一樣、資料不離開機構、不生第二套安全論述。**2026-08-29 負責人答覆：無法多開網域，(b) 否決。** 所以這條退場路徑是關的，記在這裡是為了下一個人不必再問一次。

**同日答覆的另外三個事實，方向與 (b) 相反且對本 ADR 有利**：機構內**前後端所需套件都取得得到**、**OpenAI API 打得通**、網頁搜尋大部分可達。前者讓離線 bundle 的重要性下降，後者讓**本機起一個模型閘道成為可能**——**但要不要做、怎麼做，本 ADR 不決定**（`PORT-007`／`PORT-011` 的規劃事實登錄在 [m6/README](../plans/mvp/m6/README.md)）。

**仍然有效的退場條件（滿足任一即應評估移除本 Driver 與 `clean` 等級）**

1. **Demo 之後不再需要在受限機器上跑 Run。** Demo 排在 2026 年 9 月中下旬；**M6 真正完成後凍結新功能**（負責人 2026-08-29 裁定）。凍結不等於移除，但它是重新問這個問題的自然時點。
2. **那台機器的白名單接受任何一顆我們可以自帶的 launcher。** 那時真的隔離器就放得上去，而 `clean`「沒有邊界」這個等級失去理由（[m6/report-sandbox-options.md](../plans/mvp/m6/report-sandbox-options.md)）。
3. **`02:PORT-010` 第 5 條的內容來源限制取得真正的強制點。** 今天它**沒有強制點**（見該條 2026-08-29 的補記）；一旦平台側有了那道判準，`clean` 的風險面才第一次是被機器界定的而不是被操作習慣界定的。

**退場的成本是低的，而這是刻意的**：本 Driver 沒有第二條派送路徑、沒有第三方相依、白名單少一個字串就整條路關閉。**移除它不需要一份新的 ADR，只需要這一節被引用一次。**

## 待決策

1. **工作負載的環境變數契約要抽到共用套件、還是在新 Driver 複製一份。** 抽出來要動 `dockerdrv`（測試很密，漂移會被抓到）；複製等於兩份契約，而它們一定會分岔。**建議抽出來。**
2. **乾淨模式的 Run 要不要寫進正式的 Run 歷史。** 它們是真的 Run 但跑在沒有邊界的地方；混進同一張表會讓「歷史 Run 可追溯 Provider 與 Runtime」這句話多一種讀法。 **→ 2026-09-03 註記：已由 [ADR-060](./ADR-060-the-clean-test-mode-is-the-real-system-with-three-strategies-swapped.md) 待決策 2 逐字承接**（兩份的措辭幾乎相同且互不引用）。**答案只留在 ADR-060 那一處**——兩邊各自被回答就會分岔。
3. **`Adopt()` 回空是否需要在畫面上說。** 服務重啟會殺掉跑到一半的 Run——展示途中重啟會看到什麼，未定義。
