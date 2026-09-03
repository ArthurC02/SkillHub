# SEC-009 T8 節點半 — 巢狀開發容器，2026-08-27

## ⛔ 這不是驗收，而且這一次連「受測物」都是假的

[ADR-022 §T8](../../../../../../adr/ADR-022-sandbox-deployment-topology-and-security-thresholds.md) 把 T8 的節點半（**C-01、P-01、P-03～P-05**）歸在 **Suite 2**，受測物是**即將加入池的那一台節點**。

本次的「節點」是 Windows → Docker Desktop 的 WSL2 VM → 一個 `python:3.12-slim`／`docker:cli` 容器。**沒有節點、沒有 gVisor、沒有 cloud-init。** 第 2 次執行裡的 node facts 是我用 heredoc 寫出來的，`runsc` 是一支 `echo` 兩行字的 shim。

**C-01、P-01、P-03～P-05 的判定一格都沒有改，仍然是 `unknown`。** 理由不是「沒跑」，是**跑的不是那個東西**。

所以本目錄不叫 `YYYY-MM-DD-<node-id>/`（ADR-022 §5 給正式證據的命名），它掛在 T5 那次實驗室證據目錄底下，名字裡寫著 `nested-dev-container`。

## 那為什麼要跑

因為在今天之前，**「閘門 A 節點准入探針」是一個名詞，不是一支程式**。ADR-022 §5 說節點側要留 `probe.json`，而在此之前沒有任何東西會產生那個檔案。

部署批原本要在一台剛開出來、沒人除錯過的機器上，在節點已經在佈建的時間壓力下第一次把它寫出來——而它同時是 P-04（阻擋級）與 P-03（7 天重建）唯一的量測點。

## 五次執行

| 檔案 | 是什麼 | rc | 結果 |
| --- | --- | --- | --- |
| [`1-bare-container.txt`](1-bare-container.txt) | 沒有 node facts、沒有 `runsc`、沒有 docker | 2 | 6 項 `unknown`、P-04 `fail`、P-05 `pass` |
| [`2-synthetic-facts.txt`](2-synthetic-facts.txt) | 合成 node facts ＋ 假 `runsc` shim ＋ 掛上宿主 docker socket（唯讀） | 2 | P-01a／P-03／P-05 `pass`，C-01a／C-01b／P-01b／P-04 `fail`，C-01c `unknown` |
| [`probe.json`](probe.json) | 第 2 次的 `--json` 輸出，即 ADR-022 §5 的 `probe.json` 形式 | — | — |
| [`3-self-check.txt`](3-self-check.txt) | `--self-check`，離線，不碰節點也不碰 docker | 0 | 17 組判定案例全數如預期 |
| [`4-p05-negative-control.txt`](4-p05-negative-control.txt) | P-05 負對照：種兩個假憑證 | 2 | P-05 轉 `FAIL`，且輸出裡 **0 次**出現該憑證值 |
| [`5-mutation.txt`](5-mutation.txt) | 四次突變，每次弄壞一條規則 | — | 四次全部讓 `--self-check` 變紅 |

## 第 1 次抓到的：`unknown` 佔了六格，而那正是對的

沒有 node facts 的容器裡，八列有六列是 `unknown`。這不是探針壞了，是 ADR-022 T8 的判準（「`unknown` 視同 fail」）在正常運作：**一台說不出自己是誰的機器不進池。**

值得記下來的是 **P-05 在這一次是 `PASS`**——而它是這張表上最弱的一格：一個幾乎空的容器裡當然找不到憑證。**一個不管有沒有憑證都會綠的探針量的是零**，所以才有第 4 次。

## 第 2 次抓到的三件事，全部是真的量到的

1. **C-01a `FAIL`：這台 dockerd 沒有註冊 `runsc`**（只有 `io.containerd.runc.v2 nvidia runc`）。假的 `runsc` shim 讓 P-04 的比對路徑跑得完，**但它騙不過 C-01a**——因為 C-01a 問的是 dockerd，不是 `$PATH`。兩個問題分開問是有價值的。
2. **P-01b `FAIL`：5 個容器沒有一個帶 `skillhub.sandbox.managed` label**，其中兩個是 Postgres。這台開發機**確實是一台混排工作負載的機器**，而 P-01 禁止的正是這個。這一格是本次唯一一個「受測物是假的、但判定是真的」的地方。
3. **C-01b `FAIL`：`skillhub-api` 把 repo 根目錄以可寫方式 bind mount 進容器**，而**探針容器自己也被抓到**（它為了寫 `probe.json` 掛了 `/out`）。後者是自指的，但它證明這條檢查不會放過執行它的那個容器。

## 第 2 次**沒有**證明的事

- 沒有證明任何一台機器上的 gVisor 版本。`runsc` 是 shim。
- 沒有證明 cloud-init 會寫出探針要的那份 facts。**那是一份需求，還沒有任何 IaC 實作它**（見 `t8-node-probe.py` 檔頭的 THE INPUT CONTRACT）。
- P-03 的 `PASS` 讀的是我三分鐘前用 `datetime.now() - 3 天` 算出來的字串。

## 第 4 次：P-05 的負對照，兼鐵律 11 的驗證

種兩個假憑證（`/opt/skillhub/node.env` 一個、程序環境變數 `DATABASE_URL` 一個），P-05 如期轉 `FAIL` 並指出**位置與命中的樣式**：

```
P-05 FAIL ... 2 hit(s): /opt/skillhub/node.env contains a postgres:// URL; pid 9 env DATABASE_URL
```

同一次執行的最後一行是對輸出 `grep -c` 那個假密碼的結果：**`0`**。

**一支會把找到的憑證印進 log 的洩漏偵測器，本身就是那個洩漏**（AGENTS.md 鐵律 11）。這一行是那條鐵律在這支腳本上唯一的證據。

## 第 5 次：四次突變，四次紅

AGENTS.md 第 9 條。`--self-check` 只驗兩個評分函式，而那兩個函式管的規則**都只在它們真的該擋的那一天被行使一次**——在那之前，比較式寫反了也看不出來。

| 突變 | 弄壞了什麼 | `--self-check` |
| --- | --- | --- |
| `got >= want` → `got <= want` | P-04 的版本比較方向 | **4 case(s) wrong** |
| `REBUILD_DAYS = 7` → `700` | 7 天重建週期不再被強制 | **1 case(s) wrong** |
| `SELF_REPORT_TOLERANCE_SECONDS = 2` → `0` | 自報時戳從 `unknown` 變成 `PASS` | **1 case(s) wrong** |
| `if baseline == "unset"` → `and False` | `unset` 基線從阻擋降級成往下走 | **1 case(s) wrong** |

第三個突變是最值得看的：它拿掉的是 ADR-022 §1 明文排除的那件事（「非節點自報的當下時間」），而**拿掉之後每一次實測都會更好看**——P-03 會從 `unknown` 變成 `PASS`。這種突變不會有人在 code review 抓到。

執行後檔案與突變前的副本 `diff` 為空（見檔尾 `file identical to pre-mutation copy`）。

## 給部署批的三件事

1. **cloud-init 要寫 `/etc/skillhub/node.json`**，四個欄位、`node_created_at` 必須是**建置時戳且跨重開機不變**。沒有這個檔，閘門 A 的四項直接 `unknown` ＝ fail。
2. **`infra/nodes/gvisor-baseline.txt` 仍然是 `unset`**，所以 P-04 在**任何**節點上都會 `fail`。這是 fail-closed 的正確方向，但它的意思是：**第一台節點開出來之後，那個檔案要在它入池之前先被填上**，順序不能反。
3. **`EXPECTED_ROLE = "sandbox-exec"` 是這支腳本單方面定的**，repo 裡沒有別的地方寫著節點角色叫什麼（`infra/compose/sandbox-node.yml` 目前不存在）。IaC 要嘛照這個字串寫，要嘛改腳本——但要有人決定。

## 一個 ADR-022 需要回答的問題

**C-01c 永遠是 `unknown`，所以這支探針在一台完全正確的節點上也會 exit 2。**

C-01 的後半（「每個 Run 使用獨立暫存工作區，不與其他 Run 共用可寫路徑」）是關於**兩個並行 Run** 的敘述，而閘門 A 拍的是一台閒置節點的照片。這一項由 SBX-005 的整合測試覆蓋，不是這支探針。

探針選擇**把它印成 `unknown` 而不是省略**——省略會讓一個綠燈看起來像是 C-01 被整條檢查過了。但代價是閘門 A 永遠不會回 0，而**一個永遠紅的閘門會被值班的人關掉**。

ADR-022 §4 的覆蓋表把 C-01 整條指派給 T8。這需要一次裁定：要嘛在覆蓋表裡把 C-01 拆成「宣告面（T8）／執行期（SBX-005）」，要嘛明說閘門 A 只判宣告面。**不該由這支腳本自己決定，所以它照實印。**
