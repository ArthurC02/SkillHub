# SEC-009 的可執行化（不是 SEC-009 的驗收）

## 這裡是什麼

`02:SEC-009` 要求 SelfHostedProvider 驗收的 45 項全數通過，可執行的測項清單、通過判準與證據要求由 [ADR-022](../../docs/adr/ADR-022-sandbox-deployment-topology-and-security-thresholds.md) 第三部分給出（10 個測項、45 項覆蓋核對、全數 pass 且 0 unknown 才放行，證據存 `docs/plans/mvp/m4/sec-009-acceptance/`）。

在 2026-08-23 之前，那 10 個測項**一行可執行的東西都沒有**，而擋住它的一直被記成「要一台 Linux 機器」。

## 已經查明的事實（2026-08-23）

**gVisor 在這台開發機上跑得起來，而且是真的 gVisor。** 不需要 WSL 發行版——`wsl --list` 上只有 Docker Desktop 自用的 `docker-desktop`，沒有 Ubuntu。走的是 Docker：

| 位置 | 核心 |
| --- | --- |
| 宿主容器 | `6.6.87.2-microsoft-standard-WSL2` |
| 沙箱內 | **`4.19.0-gvisor`** |

`dmesg` 是 gVisor 自己的開機訊息、`--network=none` 下零個網路介面、沙箱內 PID 1 是被沙箱的那個 process。`runsc release-20260817.0`，下載後對過 sha512。

跑法：`tools/sec009/gvisor-smoke.sh`

## 這裡**不是**什麼

**不是 SEC-009 的驗收，而且不可能是。** 兩個理由，第二個比第一個重要：

1. ADR-022 要的是**獨立 VM 池**上的 10 個測項與 45 項覆蓋，證據落檔。這支腳本只跑一個 smoke test。
2. **它是巢狀的**：Windows 宿主 → Docker Desktop 的 WSL2 VM → privileged 容器 → gVisor 沙箱。在這裡跑逃逸測試，量的是「沙箱」與「一個被刻意賦予全部 capability 的容器」之間的邊界，而且核心不是生產跑的那個。**這裡過了只代表流程跑得完，不代表邊界守得住。**

AGENTS.md 第 7 條同一件事的另一種說法：Dev Container 不取代真實 Linux／gVisor 部署驗收。

## 一個要進部署 runbook 的發現

privileged 容器裡第一次跑 runsc 會失敗在：

```
cannot set up cgroup for root: configuring cgroup: write /sys/fs/cgroup/cgroup.subtree_control: device or resource busy
```

**顯而易見的修法是 `--ignore-cgroups`，它會過——而它關掉的正是 SEC-009 資源耗盡測項要量的那個東西。** 綠燈會出現，測項會變成空的。

腳本走的是另一條：把 root cgroup 裡的 process 移進 leaf，讓 `subtree_control` 可寫（cgroup v2 的 no-internal-process 規則）。**保住 `+cpu +memory +pids` 才有東西可以量。**

這一條要寫進部署 runbook：任何在容器裡跑 runsc 的環境都會撞到它，而兩個修法的差別在告警面上看不出來。

## 下一步

把 ADR-022 第三部分的 10 個測項逐條寫成腳本，在這裡 dry run。**跑得過的記成「程序可執行」，不記成測項通過**；真的驗收要一台獨立 Linux 節點，而 `infra/nodes/gvisor-baseline.txt` 現在還是 `unset`（＝還沒有任何沙箱節點被開出來）。

---

## T1 的通用嘗試(2026-08-23 新增)

`tools/sec009/t1-escape-attempts.sh`——ADR-022 第三部分 T1 裡**通用嘗試**那一半,八項:寫 `core_pattern`、`mount(2)`、載入核心模組、讀 `/dev/mem`、看見宿主程序、找 unix socket、碰 docker socket、在節點上放一個檔案;外加 T1 判準要求的兩項節點側觀察(沒有檔案被放上來、`dmesg` 沒有 taint)。

**T1 點名的四個 CVE PoC 不在裡面**,而且刻意不放:它們需要真的 exploit 程式碼打真的宿主,在一個巢狀的開發機容器裡跑別人的 PoC,對一台它從沒碰過的節點證明不了任何事。那四項屬部署批。

### 它有一個負對照,而且那不是裝飾

```bash
tools/sec009/t1-escape-attempts.sh                    # 沙箱內,期望全部 REFUSED
SEC009_NO_SANDBOX=1 tools/sec009/t1-escape-attempts.sh  # 無沙箱,期望至少一項 ESCAPED
```

**一個不管有沒有沙箱都會綠的探針組,量的是零。** 負對照把期望反過來:沒有沙箱時至少要有一項成功,否則腳本自己判失敗。實跑結果——無沙箱下三項逃得出去:`core_pattern` 寫得進去、`/dev/mem` 讀得到、而 `mount` 掛上來的 proc **報的是宿主核心 `6.6.87.2-microsoft-standard-WSL2`**。加上沙箱之後這三項全部 REFUSED。

### 第一次跑抓到的三件事,全部是這支腳本自己的問題

| # | 症狀 | 真相 |
| --- | --- | --- |
| 1 | `mount(2)` 判為 ESCAPED | **gVisor 有實作 mount**,給的是它自己的 procfs。**syscall 成功不等於逃逸**;探針改成把掛上來的 proc 讀回來,只有在它報出非 gVisor 核心時才算逃逸 |
| 2 | 找到 unix socket | 那是 `runsc do` 自己的控制通道 `/tmp/runsc-do*/runsc-*.sock`。**生產不走 `do`**(sandboxd 走 Docker runtime),所以那是**探針量到自己**;現以路徑窄範圍排除 |
| 3 | 全部通過而且**什麼都沒跑** | `bash -s` 從 stdin 讀腳本,而 `docker run` 少了 `-i` 時 stdin 什麼都沒有——容器跑了一個空腳本,**exit 0**。一個從沒執行過的測試給出綠燈,正是這個目錄存在的理由 |

**這三件全都會在部署日當天發生**,差別是那天有時間壓力、有一台剛建好沒人除錯過的機器,而且第 3 件會讓人以為驗收過了。

### 它仍然不是 SEC-009 的驗收

一格都不是。巢狀環境量的是「沙箱」與「一個被刻意給了全部 capability 的容器」之間的邊界,核心也不是生產那個。ADR-022 把 Suite 2 的受測物定義成**即將加入池的那台節點**——換一台機器就換了受測物。

~~**T3(資源耗盡)刻意沒有寫成腳本**:它要量的是「限制由 runtime 強制生效」與「同節點其他 Run 劣化 < 20%」,而限制是 `sandboxd` 經 Docker runtime 套上去的,不是 `runsc do` 套的。用 `ulimit` 在沙箱裡自己設一個上限再驗證它生效,測到的是 `ulimit`。**那一項要一台跑著 sandboxd 的節點,不是一支 shell 腳本。**~~

**2026-08-26 更正:前半是對的,結論是錯的,而且錯得讓一個跑得動的測項被歸檔了三天。**

「限制是 `sandboxd` 經 Docker runtime 套的」正是為什麼 T3 **不該**是這個目錄裡的一支 shell 腳本——但它也不需要一台節點。`apps/sandbox/internal/dockerdrv` 的 live 測試本來就開真的容器、套的就是 `dockerdrv` 生產那份 `HostConfig`，而 `SKILLHUB_SANDBOX_TEST_RUNTIME` 這個開關**在 SBX-001 就存在**。CI 的 `sandbox` job 會 `sudo runsc install` 之後把整包再跑一次——**那就是一台在跑 sandboxd 那份 HostConfig 的 Linux 機器**，而且每次 `apps/sandbox/**` 變更都跑，正是 ADR-022 §4 指派 Suite 1 的方式。

缺的從來不是節點，是那個套件裡沒有 **C-11／C-12／C-14 的行為測試**:只有宣告面(讀回 driver 要求的欄位)。而在 runsc 上「要求」與「強制」會分家——`TestPidsLimitStopsAForkBomb` 的註解記的就是欄位設了、上限沒觸及 guest task 的那一次。三支已補進 `docker_test.go`:

| 檢查 | 測試 | 觀測值 |
| --- | --- | --- |
| C-11 記憶體上限 | `TestMemoryCeilingStopsAWorkloadThatExceedsIt` | 寫 512 MiB 進 256 MiB 上限的沙箱**不得**回報成功。兩種讀法都算強制(runc 下 OOM killer 收走 `dd`、shell 活著回報非零；runsc 下 sentry 自己可能就是死掉的那個，於是沒有完成行)——**不可接受的只有 `rc=0`** |
| C-12 暫存配額 | `TestScratchQuotaRefusesAWriteWithoutKillingTheRun` | 128 MiB 寫進 48 MiB 的 `/work` 必須失敗，**且 Run 要活著把失敗說出來**(ENOSPC 是工作負載該看到並處理的錯誤)，檔案大小不得超過配額 |
| C-14 開檔上限 | `TestOpenFileCeilingReachesGuestTasks` | 沙箱內 `ulimit -n` 報得出上限(**那是 guest kernel 自己的說法**，在 runsc 上等於 sentry 真的把 rlimit 套到 task 上)，且開 200 個檔的工作負載跑不完 |

三支都照 C-13 那套「兩個探針」的規矩寫:先證明同一個上限下跑得動一個瑣碎的工作負載,才分得出「上限沒咬」與「什麼都沒跑」。每一支都以**把強制拿掉**突變驗過會紅(`Memory` 改 0、tmpfs 的 `size=` 拿掉、`nofile` ulimit 刪掉),不是編譯錯誤那種紅。

**T3 剩下的一半仍然沒有量**:「同節點其他 Run 劣化 < 20%」。那要一台安靜的專用節點——在 2 核的 CI runner 與巢狀 WSL2 VM 上，量測噪音大於它要判的門檻，寫出來會是擲硬幣不是檢查。

---

## T8 的映像半（2026-08-23 新增）

`tools/sec009/t8-image-audit.py`——ADR-022 第三部分 T8 裡**不需要節點**的那一半，外加整批的三個前置條件。

```bash
python tools/sec009/t8-image-audit.py               # 實測
python tools/sec009/t8-image-audit.py --json        # 給證據落檔用
python tools/sec009/t8-image-audit.py --self-check  # 離線，不碰網路
```

**它是 Suite 1 裡唯一一項從映像發佈那天起就跑得了的**：查的是 GHCR 上公開的東西，不需要節點、不需要 Linux、不需要任何憑證。而在今天之前沒有人跑過。

### 第一次實測的結果

| | |
| --- | --- |
| 映像 | `ghcr.io/arthurc02/skillhub-runtime-agent-sdk:2026.08-3` |
| digest | `sha256:6a99b373bf04…` |
| I-01 版本化映像已發佈 | **PASS** |
| I-02 base 以 digest 釘選 | **PASS** |
| I-03 SBOM attestation | **PASS**（SPDX 2.3） |
| I-04 掃描 attestation 在有效期內 | **PASS**（`2026-08-21T10:57:21Z`，2 天，2026-09-20 到期） |
| I-06 可修的 Critical／High | **PASS**（0；總計 320 筆 finding） |

**45 項基線裡有 5 項因此第一次有了機器可查的證據。**

### 三個前置條件裡，只有第一個是成立的

ADR-022 把三個前置寫成「缺一即 T8 判 `unknown` ＝ fail」，也就是**缺一即整批 45 項全部 fail**：

| # | 現況 |
| --- | --- |
| ① 映像已發佈至 GHCR 且附 SBOM 與掃描 attestation | **PASS**——本次實測確認，而在此之前它的狀態是「沒人查過」 |
| ② `infra/nodes/gvisor-baseline.txt` 已填實際版本 | **FAIL**，仍是 `unset` |
| ③ `infra/egress/allowlist.yaml` 的 `pinned_ip` 已填 | **FAIL**，仍是 `unset` |

**②③ 兩項都要等第一台節點**（②是那台機器上 `runsc --version` 的值，③是閘道的沙箱面位址）。所以這支腳本的結論是一句很具體的話：**SEC-009 目前缺的不是十個測項，是一台機器**——而它同時證明了第①項不在那台機器的等待清單上。

### 為什麼查的是 registry 而不是建置流水線

`runtime-image.yml` 在**建置時**就斷言過同一組閘門。這支查的是**現在 registry 上還成不成立**，而那才是閘門 A 問的問題：

- **attestation 曾經被產生**與**這個 digest 現在還帶著它**是兩件不同的事。版本 tag 移動過之後，舊 digest 就成為孤兒——它的 attestation 還在，但描述的是別的位元組。腳本因此**逐筆比對 attestation 的 subject digest 與已發佈 digest**，不只確認「索引裡有兩份 bundle」。
- I-04 的 30 天時效只對後者有意義。沒有人重建的映像不會被重新掃描，所以**一個從沒失敗過的建置流水線，正好是最會讓映像悄悄過期的那一種**。

### 兩件實作上的坑（部署日會再遇到）

1. **GHCR 不支援 OCI referrers API。** 對一個確實帶著 attestation 的 digest 打 `/v2/<repo>/referrers/<digest>`，回的是空集合——**一個會產生假 FAIL 的正確答案**。`actions/attest` 走的是 tag `sha256-<digest>`，腳本照那條路查。
2. **GHCR 走 HTTP/2，header 名稱是小寫的。** `dict(resp.headers)["Docker-Content-Digest"]` 取不到值。第一次跑就是這樣掛的——它報了 `UNKNOWN` 而不是 PASS，**判定是對的但理由是錯的**，這正是 `unknown` 不等於 pass 這條規矩擋下來的東西。

### 它有一個離線自檢，理由和 T1 的負對照一樣

`--self-check` 拿六組時間戳去驗 I-04 的判定：新鮮的、警告窗外的、警告窗內的、過期的、沒有時間戳的、時間戳解析不了的。

**因為接下來一個月內的每一次實測都會舒服地落在有效期內**，比較式寫反了也看不出來——這條規則只在它真的該擋的那一天才會被行使一次，而那一天沒有人在旁邊看。

### 它仍然不是 SEC-009 的驗收

T8 的節點半（C-01、P-01、P-03～P-05）不在裡面，那要一台節點。**綠燈的意思是 45 項裡有 5 項有證據，另外 40 項沒有。**

---

## T2 的 syscall 煙霧級 fuzz（2026-08-23 新增）

`tools/sec009/t2-syscall-fuzz.sh` ＋ `_syscall_fuzz.py`。

```bash
tools/sec009/t2-syscall-fuzz.sh                            # 4 × 60s，程序試跑
SEC009_FUZZ_SECONDS=1800 tools/sec009/t2-syscall-fuzz.sh   # ADR-022 的 4 × 30 分鐘
```

**腳本自己會說它跑的是哪一種**：低於 4 × 1800s 就印「a procedure run, NOT T2」。ADR-022 在 T2 的通過判準裡自己寫了「**誠實標示：這是煙霧級，不是研究級**」——研究級的 Sentry fuzzing 是 gVisor 上游的責任。

T2 的判準有三半，腳本三半都判：Sentry 沒掛（沙箱撐完全程）、宿主核心沒有 oops（節點側 `dmesg` 前後比對）、沒有取得非預期權限（uid／gid 前後比對，**外加把 T1 的三個不變量在壓力之後重驗一次**——只在 30 分鐘之後才出現的權限正是 T2 該抓的，只比 uid 的檢查看不到它）。

### 第一次完整實測（4 × 60s）

| | |
| --- | --- |
| syscall 數 | **211,222**（4/4 worker 都回報） |
| 相異 errno | 26 |
| 子程序切片 | 219 個，其中 **48 個死於訊號** |
| Sentry | 撐完 61s |
| uid／gid | 未變 |
| T1 三個不變量 | 壓力後仍成立 |
| 節點 `dmesg` | 無新增 |

### 為什麼是「一個監督者 ＋ 一串短命子程序」

**因為亂打 syscall 的程序會先殺死自己。** 實測：**約五秒內 SIGSEGV**（遲早會把自己位址空間的指標餵給 `munmap` 或 `mprotect`），偶爾 SIGKILL（`mmap` 要到一塊大得該被殺的記憶體）。

**那不是 gVisor 的發現，是 fuzzer 就長那樣**——但它的後果很嚴重：最直覺的寫法（一個程序、fuzz N 秒）**不管 N 寫多少都只量到約五秒**，而且**一個數字都回報不出來**，因為死掉的那個程序正是拿著計數器的那個。**一次 30 分鐘的 T2 會安靜地變成 5 秒的 T2。**

所以：不打任何隨機 syscall 的父程序負責監督與計數，子程序一次 fuzz 一個切片、寫下計數、退出；父程序加總後再開下一個，直到牆鐘到期。子程序中途死掉只損失一個切片，並記成一次 crash——**是預期的、以數字回報的，本身不構成 T2 失敗**。T2 問的是 **Sentry** 活不活著，而父程序還能開下一個子程序這件事就是答案。

**48／219 這個數字有印出來，而且要一直印。** 哪天它變成 0，意思不是子程序變強壯了，是**它們不再 fuzz 了**。

### 這一支抓到的四件事，全部是它自己的問題

| # | 症狀 | 真相 |
| --- | --- | --- |
| 1 | 容器只印一行 `runsc: OK` 就 exit 0 | **`apt-get` 把腳本剩下的部分從 stdin 吃掉了。** 呼叫端用 `bash -s` 把腳本用管道餵給 bash，bash 是**從 stdin 讀腳本本身**的，而 apt 也讀 stdin。修法是每個 `apt-get` 都加 `</dev/null`（已改進 `_prepare-runsc.sh`，T1 同受益）。**這是 `-i` 那個坑的雙胞胎**：跑了前半、exit 0、對沒跑到的後半一個字都不說 |
| 2 | 第一個 FAIL 之後就沒有任何檢查了 | `_prepare-runsc.sh` 留著 `set -e`，那對 setup 是對的、對評分段是錯的——**評分段的每一行都「允許失敗」，失敗就是結果**。`grep -c` 在計數為 0 時回傳 1，於是整支腳本在那裡結束。評分段現在明確 `set +e` |
| 3 | 容器還沒啟動，宿主就先跑了 `grep -c` 與 `runsc do` | `<<INNER` 是**不加引號**的 heredoc（要讓 `$PREPARE` 展開），於是**裡面的反引號會在宿主上執行，註解也不例外**。三段解釋用反引號括了 shell 名稱。**這個 heredoc 裡的散文是可執行的** |
| 4 | uid 變成 `4086022094`；stderr 檔案長到 **1.5 GB** | 兩件都是 fuzzer 打到自己：隨機 `setuid(2)` 讓「有沒有取得非預期權限」這個判準**變成量不到而不是不通過**（因此排除九個 credential setter）；隨機 `write(2)` 打中 fd 1／2（因此子程序把自己的 1／2 指到 `/dev/null`，計數改走檔案） |

### 同批修掉 T1 的一個自相矛盾（**先於本批就存在**）

T1 的 `plant a file on the node` **每一次都判 ESCAPED**，而同一次執行的節點側觀察說節點是乾淨的——**腳本在相鄰兩行自相矛盾，而錯的那一半正是看不到答案的那一半**：它評的是沙箱**裡面**的 `touch`，而沙箱本來就有自己可寫的視圖，在裡面建檔不是逃逸。T1 的判準寫的是「**節點**檔案系統無新增檔案」。

現在嘗試從裡面做、判定只從外面下。**這是同一份檔案裡第三次同型錯誤**：一個看不到自己所評對象的探針，照樣會很有把握地評下去。

已用 `git show` 取出本批之前的版本重跑確認**這個 FAIL 不是本批造成的**（附帶證據：修 `</dev/null` 之前那次重跑**少了兩行標頭**，正是第 1 項的截斷）。

順帶補上負對照的一個缺口：`no file planted on the node` 原本在 `SEC009_NO_SANDBOX=1` 下也照樣走沙箱，因此**整個測試套件裡沒有任何輸入能讓它變紅**。現在無沙箱時真的把檔案放到節點上，該列如期 FAIL。

---

## 2026-08-26 的兩次執行,兩支腳本都被自己抓到

證據落在 [`docs/plans/mvp/m4/sec-009-acceptance/2026-08-26-nested-dev-container/`](../../docs/plans/mvp/m4/sec-009-acceptance/2026-08-26-nested-dev-container/)。

### T1:`no file planted on the node` 在這台機器上從寫出來就是假綠

`MARKER` 以 `docker run -e` 傳入,而 Git Bash 會把 `/tmp/...` 改寫成 `C:/Users/.../Temp/...`;沙箱裡的 `touch` 因此失敗。**兩個後果疊在一起才被看見**:評分段還繼承著 `_prepare-runsc.sh` 的 `set -e`,那次失敗直接殺掉腳本,**節點側兩列觀察一行都沒跑**,rc=1 被報成「setup failed」。少了其中任一個,`[ -e "$MARKER" ]` 都會印 **PASS**——一個從沒執行過的動作拿到綠燈。

**負對照同樣是死的**:無沙箱時同一個 `touch` 也失敗,所以**整個套件裡沒有任何輸入能讓那一列變紅**。README 先前寫「現在無沙箱時真的把檔案放到節點上,該列如期 FAIL」——那句話在這台機器上不成立。

修法:MARKER 是常數,定義移進容器內(本來就不必跨主機邊界);評分段補 `set +e`(T2 早就學過這一課,T1 沒補)。重跑後負對照那一列如期紅。

**這是同一份檔案裡第四次同型錯誤**,而這一次的成因是**宿主**——前三次都在腳本裡面。

### T2:第一次 4 × 1800s 在 48 分鐘處卡死,而腳本原本會說它 PASS

**它為什麼是「卡住」而不是「慢」**:它跑了 48 分鐘,預算是 30 分鐘,而且沒有任何一個 worker 留下紀錄。

**當時用來判定的兩個讀數是量錯的東西,寫在這裡免得下次再用**:「所有 sentry 執行緒在 S」與「`runsc` 程序十秒內 0 個 CPU tick」——後續一次**健康**的執行同樣是 21 個 `exe` 全在 S、`runsc` 父程序 0 ticks(它只是監督者,工作在 sentry 執行緒裡),而 `docker stats` 顯示 45% CPU、跨 `exe` 累計 112,278 ticks。**要看的是累計 ticks 或容器的 CPU%,不是父程序也不是執行緒狀態。**

真正的證據來自修好之後那次全規模執行本身:**每個 worker 有 12～16 個切片超出預算被 `SIGKILL` 收掉**(4 × 60s 的程序試跑裡也已經有 4 個)。切片預算是 1 秒 ＋ 5 秒寬限,所以那個數字嚴格說是「超過六秒的切片」,不必然每一個都永不返回——**但 48 分鐘那次證明了至少有一次是無界的**,而在舊的 `os.waitpid(pid, 0)` 下,第一個這樣的切片就會讓那個 worker 停在那裡,而父程序的 `wait` 會跟著停。

切片的時間上界靠 0.25s 的 `SIGALRM`,而 fuzzer 打得到武裝它的 syscall——一次隨機 `rt_sigprocmask(SIG_BLOCK, …)` 或 `rt_sigaction(SIGALRM, SIG_IGN)` 之後計時器就不再到達,下一個阻塞式 syscall 永不返回,監督者的 `os.waitpid(pid, 0)` 就永遠等下去。**與 DENY 裡那段 setuid 的註解同一個形狀:fuzzer 關掉量測自己的工具。**

**刻意不用把那兩個 syscall 加進 DENY 的方式修**:blocklist 只涵蓋有人預料到的掛法,而每一條都是永久讓出的 fuzz 表面。`SIGKILL` 擋不掉、忽略不了、也接不住,所以改由監督者持有時間預算,fuzzer 一個 syscall 都不用讓。被殺掉的切片有計數並印出來,理由與 `child_crashes` 一樣:哪天它等於切片數,意思是 worker 不再 fuzz 了。

**更要緊的是第二半**:elapsed 檢查原本**只找提早返回**(「跑了 3 秒、預期 1800 秒」＝ sentry 死了),所以一個跑成兩倍半長度的 run 每一行都會印 PASS。**遲到現在也是 finding。**

```bash
python tools/sec009/_syscall_fuzz.py --self-check   # 需要 Linux
```

自檢**製造**那個 hang 而不是等它出現:子程序照隨機 `rt_sigprocmask` 的方式擋掉 SIGALRM 再睡十分鐘,`reap` 必須在預算內收掉它;反方向也驗(正常結束的子程序要被 reap 不是被殺),免得「reap 殺掉所有東西」冒充成修好了。把 `reap` 還原成原本的 `waitpid`,自檢會掛到 `timeout` 把它殺掉(rc=124)。

## T5 的網路外洩（2026-08-27 新增）

[`t5-network-egress.sh`](t5-network-egress.sh) 在一個 privileged 容器裡把節點的網路形狀搭出來——一張 `skillhub-sbx` bridge、兩個當成 Run 的 netns、一條假上行後面放著 T5 點名的每一個目的地——然後**把 `tools/egress/render.py` 渲出來的 ruleset 真的 `nft -f` 進去**，跑 T5-1～T5-9 與 ADR-022 要求的兩個反向驗證。

證據：[`docs/plans/mvp/m4/sec-009-acceptance/2026-08-27-nested-dev-container-t5/`](../../docs/plans/mvp/m4/sec-009-acceptance/2026-08-27-nested-dev-container-t5/)。

### 為什麼要在沒有節點的時候先寫

**在這一天之前，`infra/egress/rendered/nftables.conf` 沒有被任何核心載入過。** CI 有的是 `render.py --check`，它證明「檔案與 allow-list 一致」，**證明不了 nft 收不收得下這個檔**——而一個載入失敗的 ruleset 留給節點的是一條規則都沒有，那是失效**開放**的方向。

### 每一個 drop 都由「本來該擋它的那條規則」的計數器判定

不是由「連線失敗」判定。**沒有路由也會讓連線失敗**，而分不出這兩者的測試，在一台完全沒有網路的節點上一樣全綠。腳本因此在每次嘗試前後讀該規則的 counter，只有計數真的動了才算 PASS——這正是 ADR-022 T5 的判準（「沒有記錄 ＝ N-06 未成立，即使流量確實被擋」）。

### 它有一個負對照，而負對照當場抓到三個假綠

`SEC009_NO_NFT=1` 不載入任何規則，期望反轉：每一項都該到得了。第一次跑，三列是 `unreachable even with no rules`——DNS、東西向、入站三個目標**根本沒有人在聽**，所以「被 DROP」與「連接埠關著」對 `nc -z` 是同一件事。三者在正向那一輪全都是綠的。補上 listener 之後負對照才成立（11 項到達）。

### 它還有一個突變開關，因為沒紅過的評分器不算評分器

`SEC009_T5_DROP_RULE=metadata` 在載入前把該條規則拿掉，對應的探針必須轉 FAIL。實測：`T5-3 ... FAIL refused, but skillhub-drop-metadata counted nothing`——**流量仍然被擋**（policy drop 接住了），紅的是歸因。這正是這支腳本與「連線失敗就算過」的差別。

### 三件量到的事實，寫在證據目錄裡，這裡只列標題

1. **T5-4 的東西向阻擋依賴 `net.bridge.bridge-nf-call-iptables=1`**，而 repo 裡沒有任何地方寫著這件事。設成 `0` 的節點上，那條規則的計數永遠是零，兩個 Run 不需要逃逸就互通。
2. **T5-5 的三個半邊由三個不同的地方擋下**：轉送流量由 forward 鏈、節點自己的位址由 input 鏈、**節點 loopback 由核心路由層——在 nftables 之前，一筆紀錄都不留**。forward 鏈裡那條 `ip daddr 127.0.0.0/8` 對這個方向永遠不會觸發。節點版的判定表要為這一項寫具名例外，否則它會永遠停在 `unknown`。
3. **實驗室測的是 ruleset，不是節點**：`dockerdrv` 今天不指定 netns 也不指定 DNS，所以每 Run 一個 netns 這件事**是實驗室自己建的，不是產品程式建的**。

### 它仍然不是 SEC-009 的驗收

ADR-022 把 T5 歸在 Suite 2，而 Suite 2 的受測物是**即將入池的那台節點**。這裡是 Windows → WSL2 VM → privileged 容器 → 三個 netns，目的地是假的、`pinned_ip` 是實驗室填的、閘道是一個 python listener。**N-01～N-08 的判定一格都沒有改。**
