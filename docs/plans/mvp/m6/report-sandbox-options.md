# M6 前期量測：那台機器上還有沒有真的沙箱

- 日期：2026-08-28
- 姊妹報告：[report-inmemory-postgres.md](report-inmemory-postgres.md)（資料庫）、[report-object-storage.md](report-object-storage.md)（物件儲存）
- 相關：`02:PORT-006`（執行路徑屬清單外）、[ADR-015](../../../adr/ADR-015-sandbox-isolation-baseline.md)、[ADR-022](../../../adr/ADR-022-sandbox-deployment-topology-and-security-thresholds.md)

## 0. 這份報告最重要的一句話

**問題不是「找不到夠好的沙箱」，是「找不到把沙箱放上去的縫」。**

資料庫那一半有 PGlite 這種答案，是因為 PostgreSQL 是**一個已知的程式**，可以整個編譯成 WASM，編完它還是它自己。沙箱的處境相反：**要被關進盒子裡的東西是「使用者還沒寫出來的程式」**，沒有東西可以拿去編譯。

而真正卡死的那一條更前面：AppLocker／WDAC 管的是「哪個 PE 檔可以被執行」，所以**任何需要你自己帶一顆 `.exe` 上機的方案，全數在門口出局**——不論它的隔離多強。

## 1. 沙箱裡實際跑的是什麼（先確定被隔離的對象）

[`infra/images/runtime-agent-sdk/Dockerfile`](../../../../infra/images/runtime-agent-sdk/Dockerfile)：

```
FROM node:22-bookworm-slim                      ← 完整 Debian userland
ARG CLAUDE_AGENT_SDK_VERSION=0.3.233
apt-get install python3 python3-pip unzip
ln -s /usr/bin/python3 /usr/local/bin/python    ← 因為 Skill 會寫 `python foo.py`
pip3 install pandas numpy openpyxl lxml python-docx python-pptx pypdf pdfplumber …
```

一次 Run＝**Node 行程跑 agent loop，開子程序、執行任意 shell、跑帶 C 擴充的 Python、讀寫檔案系統、對外呼叫模型 API**。

**這一節決定了後面每一項的成敗**：任何候選要嘛能承載這個工作負載，要嘛就得縮小「不受信任的 Skill」的定義——而縮小定義等於改產品。

## 2. Windows 原生隔離：技術上成立，門口出局

| 候選 | 免安裝 | 免非白名單執行檔 | 真邊界 | 擋惡意還是只擋意外 |
| --- | --- | --- | --- | --- |
| **AppContainer / LPAC** | ✅ | ❌ | ✅ | 擋惡意（win32k 是真實逃逸面） |
| Low Integrity 行程 | ✅ | ❌ | ❌ | **只擋意外** |
| Job Objects（單用） | ✅ | ❌ | ❌ | **只擋失控**（不擋讀檔、不擋網路） |
| Restricted Token | ✅ | ❌ | ✅ | 擋惡意 |
| **Windows Sandbox** | ❌ 需 admin 開選用功能 | — | ✅ | 拿不到 |
| **Win32 App Isolation** | ❌ 需 MSIX 安裝 | — | ✅ | 拿不到 |

**一個反直覺的事實值得記下來**：Windows 建立 AppContainer profile 的那個 API **不需要管理員**——微軟文件的 Remarks 只說「為目前使用者建立 profile」，Chromium 的沙箱設計文件也逐字寫著「The user does not need to be an administrator in order for the sandbox to operate correctly」。**所以擋住這條路的不是權限，是那顆你必須自己帶上去的 launcher。**

同一份 Chromium 文件也給了必須照抄的分辨：「The token and the job object define a security boundary」，而整合性等級「don't define a security boundary in the strict sense」。**Low IL 與單用的 Job Object 因此不得被稱為沙箱。**

## 3. 只剩兩條縫

不受信任的內容如果以**資料**的身分進入一個**已經在白名單上**的行程，白名單的執行規則就管不到它。WASM module、JS 原始碼、Python 原始碼都不是 PE。實務上白名單裡一定有的宿主只有瀏覽器；PowerShell 在 script enforcement 下會被打進 Constrained Language Mode，連當宿主的能力都被拿掉。

### 3.1 縫一：瀏覽器分頁

Edge 已經在機器上，而瀏覽器分頁是**地球上被攻擊最兇、也因此被加固最狠的沙箱**：Chromium 在 Windows 上把 renderer 跑在 Untrusted 整合性等級，組合 Restricted Token ＋ Integrity Level ＋ Job Object ＋ alternate desktop，Site Isolation 再保證一個 renderer 行程只裝一個 site。

**⚠️ 一個會讓整條路歸零的陷阱**：**在你自己的頁面裡 `eval()` 使用者的 Skill，是零隔離。** 同源的 Web Worker **不是安全邊界**（同 origin、同 renderer、同記憶體空間可達）。要拿到行程級隔離，不受信任的程式碼必須位在**跨來源的 iframe**，或至少是不含 `allow-same-origin` 的 opaque origin sandboxed iframe。

它會破，而且在野破過：CVE-2025-2783（Chrome Mojo IPC 的 handle 驗證漏了 thread pseudo-handle，2025-03 於一次間諜行動中被捕獲）。

**裡面能跑什麼，是這條路的真正限制**：

| | 現況 |
| --- | --- |
| **Pyodide**（瀏覽器內 CPython） | `threading`／`multiprocessing`／`subprocess` **丟 RuntimeError**；**沒有 socket**；不能讀寫本機檔案；原生 C 擴充必須事先為 wasm32 編好並包進發行版 |
| **CPython on WASI** | 官方說明：「WASI does not have support for threads, subprocesses, or sockets」；不支援動態連結；pip 不能用 |
| **Javy／QuickJS（JS）** | **成熟且有生產先例**——Shopify Functions 用它跑第三方不受信任程式碼，Fastly Compute、Fermyon、Extism 同理 |

**對照第 1 節**：能跑的那個「Python」沒有 subprocess、沒有 pip、沒有原生套件、網路只能走宿主注入的函式。**`python foo.py` 這一行就不成立。**

### 3.2 縫一之二：在 WASM 裡模擬整台 x86

`v86`／`container2wasm` 把 **CPU 本身**搬進 WASM，在裡面開真的 Linux kernel，再用 runc 起容器。**guest 的 kernel exploit 只能拿到 WASM 線性記憶體裡那個假 kernel**；要真的逃出去得先打穿模擬器、再打穿 WASM 引擎、再打穿 renderer sandbox。**這是本報告裡對惡意 Skill 最強的隔離模型，也是唯一同時給你「真 Linux、真 subprocess、真檔案系統」與「真邊界」的組合。**

代價：

- **v86 的基準是原生的 44 倍時間**；不支援 64 位元 kernel；指令集約 Pentium 4 等級；**沒有 FPU**，浮點走軟體實作。`container2wasm` 用 Bochs 而非 v86 的 JIT，**更慢**。
- **網路是致命點**：v86 的對外網路要靠 WebSocket relay，而那台機器的出口只保證得到模型供應商——**relay 出不去**。guest 裡的 agent loop 沒辦法自己呼叫模型，要由宿主 JS 端代打再餵回去。
- ~~**本報告親自試過一次**：`pgmock` 在本機 10 分鐘沒有開起來。~~ **2026-08-29 更正：那個讀數是錯的。** 重跑之後 `pgmock` **1.017 秒開好**，而且兩條獨立的 pgx 連線拿到不同的 backend、advisory lock 真的互斥。**所以「x86-on-WASM 慢到不可用」這個推論的依據不成立**，逐項見 [report-inmemory-postgres.md](report-inmemory-postgres.md) §10。第一次為什麼會卡住至今未查明——**而那正是把一次失敗的執行當成一項性質的代價**。

### 3.3 縫二：那條唯一通得出去的路

**那台機器唯一可靠的出口是模型供應商的 API，而供應商自己就在那個 API 後面提供沙箱容器**：`Code Interpreter`（Python）與 `Shell tool`（「Runtime is currently based on Debian 12」）。

它同時滿足全部三條：

- 機器上**什麼都不裝、什麼二進位都不跑**（只發 HTTPS）
- 跑的是**真的任意程式碼**：真 Python、真 shell、真檔案系統、真 Debian
- **完全符合鐵律 1／2**：不受信任的程式碼從頭到尾沒有進入 Web/API 行程

**代價必須明講，而且第二條可能比第一條難過**：

1. **邊界的所有權不在你手上。** 產品承諾是 gVisor ＋ nftables default-deny；這裡換成「相信別人的容器」。**Demo 可以，寫進安全論述不行。**
2. **資料離開機構。** 不受信任的 Skill 內容、測試資料、輸出全部送出去——**這是金融機構**。
3. 容器內預設沒有對外網路；閒置 20 分鐘即失效。

## 4. 必須被拒絕的三個，因為它們看起來最像答案

**它們的共同特徵是：攔得住寫壞的 Skill，攔不住刻意害你的 Skill。** 而文件上這兩者必須用不同的詞——**只擋失誤的東西不可以叫沙箱**，一旦叫了，後面所有的安全論述都是假的。

| 候選 | 它自己怎麼說 |
| --- | --- |
| **RestrictedPython** | 專案首頁第一句：「**RestrictedPython is not a sandbox system or a secured environment**」。三個已編號逃逸：CVE-2023-37271（由 generator 取得 stack frame 走出邊界）、CVE-2025-22153、format string 逃逸 |
| **Deno 權限模型** | 官方文件叫你去用 gVisor：「Use a sandboxed environment like a VM or MicroVM (gVisor, Firecracker, etc)」；並自承 `--allow-run` 等同 `--allow-all`，FFI 直接繞過權限 |
| **isolated-vm** | **2026-08-08 剛修的 GHSA-864f-rcv7-6rh4**：`ExternalCopy` 的 type confusion，最大已展示影響是**宿主行程控制流劫持（RCE）**。V8 的 Isolate 邊界本身沒破，破的是跨界的 C++ 膠水碼 |

（`vm2` 已封存且有 critical 逃逸；Node 內建 `vm` 上游本來就聲明不是安全機制。標準 Lua 只要能載入 bytecode 就等於逃逸；`Luau`／`Starlark` 是真邊界但語言不對——拿 Starlark 跑 Agent Skill 等於重寫產品。）

## 5. 一個順帶查到的縫：隔離等級的閘門是黑名單

[`schedule.go`](../../../../apps/platform/internal/trial/execution/schedule.go) 的 `Match` 拒絕 `""`、`process`，以及非開發部署下的 `container`。**其餘一律通過。** 實測（探針已移除）：

```
isolation="gvisor"     -> 通過隔離關（卡在後面不相干的資源檢查）
isolation="gvsior"     -> 通過隔離關     ← 打錯字，待遇完全相同
isolation="banana"     -> 通過隔離關
isolation="hosted-vm"  -> 通過隔離關
isolation="container"  -> 在隔離關被擋
isolation="process"    -> 在隔離關被擋
isolation=""           -> 在隔離關被擋
```

**這不是今天的 bug，要說準確**：`sandboxd` 的宣告值是程式裡的兩路分支（`container`／`gvisor`），環境變數打錯只會退回 `container`，而那是會被擋的。**沒有現存路徑產得出未知值。**

**但它是引入新 Provider 型態時會踩到的縫**——例如第 3.3 節那種託管沙箱。而同一段程式的註解記著一次真實事故，形狀一模一樣：

> `container` was neither `""` nor `process` and so simply passed. A node that lost the variable ran every untrusted skill on a shared kernel and said so in one startup log line.

**當時的修法是往黑名單再加一個值，不是改成白名單。** 所以下一個新值會重演同一件事。**加任何新 Provider 之前，這個閘門要先改成白名單。**

## 6. 建議：先問行政問題，再談技術

**以上全部方案，都比不上一件更簡單的事——去問防火牆能不能多開一個網域。**

只要多一個出口，就能把 Run 送到自己的 gVisor 節點上跑：隔離與生產環境**一模一樣**、不必為 Demo 生出第二套安全論述、資料不離開機構。**那是一次行政溝通，其他每一條路都是一套要維護的新架構。**

而在那個答案到手之前，不需要沙箱、今天就能做的是**播放真實錄下的 Run**（`02:PORT-007` 已允許取用 [m2 的 45 筆逐筆試跑](../m2/content-baseline-report.md)、[m5 的生成基線](../m5/)）。**它比一個跑在假沙箱裡的現場示範更站得住腳**——後者會讓看的人以為安全邊界已經驗過了。

## 7. 沒有量到的、以及本報告不主張的

- **全部第 2～4 節都是文獻研究，不是實測。** 本報告在這台機器上實際跑過的只有第 5 節的閘門探針，以及 `pgmock`（**該筆讀數已於 2026-08-29 更正，見上**）。
- **未驗證**：模型供應商的 API 是否回傳允許瀏覽器直呼的 CORS 標頭。**任何「瀏覽器內 agent loop」的方案都押在這一點上**，必須在那台機器上先實測。
- **未驗證**：該機構的白名單是否允許 Edge 開啟本機 HTML／載入未簽章的本機 JS，以及企業瀏覽器政策是否封鎖擴充功能。**這決定第 3 節那條縫到底存不存在。**
- **未驗證**：`v86` 上跑 agent loop 的實際可用性——無公開實測。
- **不主張**：本報告不推薦任何一項。它主張的是**每一項各自不能被稱為什麼**，以及第 6 節那個行政問題應該先問。
