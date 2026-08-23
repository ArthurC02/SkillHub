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

**T3(資源耗盡)刻意沒有寫成腳本**:它要量的是「限制由 runtime 強制生效」與「同節點其他 Run 劣化 < 20%」,而限制是 `sandboxd` 經 Docker runtime 套上去的,不是 `runsc do` 套的。用 `ulimit` 在沙箱裡自己設一個上限再驗證它生效,測到的是 `ulimit`。**那一項要一台跑著 sandboxd 的節點,不是一支 shell 腳本。**

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
